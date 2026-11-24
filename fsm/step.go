package fsm

import "math/bits"

// InKind is the kind of an Input.
type InKind uint8

// The inputs. Retry and Restart are the API writes behind the table's Ry and
// Rs columns; Event delivers every other column, filtered by epoch or
// generation.
const (
	InEvent    InKind = iota + 1 // Event, Svc, Epoch or Gen, Perm
	InRetry                      // Svc, By
	InRestart                    // Svc, By
	InStart                      // Targets, By, Deadline, NoRollback
	InStop                       // Targets, By
	InTerm                       // SIGTERM: Quiescing(0)
	InTick                       // time passes
	InDeadline                   // By, Svc: the request deadline of holder h(By, Svc)
)

// Input is one step's input. Now is the time of the step and may not
// decrease.
type Input struct {
	Kind       InKind
	Now        int64
	Svc        int
	Event      Event
	Epoch      uint32
	Gen        uint32
	Perm       bool // the status pipe holds perm (X1 only)
	By         Principal
	Targets    uint32
	Deadline   int64
	NoRollback bool
	// WithDependents is `stop --with-dependents` (section 2.3, N12).
	WithDependents bool
}

// Result is what a step produced besides the new world.
type Result struct {
	Effects []Effect
	Moves   []MoveRec
	Stale   bool // the filter dropped the input: the world is unchanged but for Now
}

type run struct {
	p   *Plan
	w   *World
	res *Result
	// escPending is an escalation raised inside a round: it is applied between
	// rounds, never in the middle of one (section 2.3, draft 16).
	escPending bool
	escStatus  int
}

// ctx is who called, for the Retry cell's payment (F9).
type ctx struct {
	caller  Principal
	apiPaid bool
}

// Step applies one input to w and returns the new world; w is not modified.
// On an error the world is not changed and the result is nil.
func (p *Plan) Step(w *World, in Input) (*World, *Result, error) {
	return p.step(w, in, p.sequence(), false)
}

// sequence is the order of phase 1 of a round (section 3.5, draft 16): targets
// before their dependents over require, want and order edges, ties by index.
// Every such order gives the same world; an arbitrary order does not.
func (p *Plan) sequence() []int {
	n := len(p.Svc)
	done := make([]bool, n)
	seq := make([]int, 0, n)
	for len(seq) < n {
		for i := 0; i < n; i++ {
			if done[i] {
				continue
			}
			ready := true
			for _, list := range [][]int{p.Svc[i].Req, p.Svc[i].Want, p.Svc[i].Ord} {
				for _, b := range list {
					ready = ready && done[b]
				}
			}
			if ready {
				done[i] = true
				seq = append(seq, i)
				break
			}
		}
	}
	return seq
}

// step runs one input. order is the targets-first order of phase 1, and rev
// reverses the order in which phase 2 signals its services (it must not
// matter; the tests flip it).
func (p *Plan) step(w *World, in Input, order []int, rev bool) (*World, *Result, error) {
	if len(w.Svc) != len(p.Svc) || len(w.Tx) != 2*len(p.Svc) {
		return nil, nil, ErrInput
	}
	if in.Now < w.Now {
		return nil, nil, ErrTime
	}
	r := &run{p: p, w: w.Clone(), res: &Result{}}
	r.w.Now = in.Now
	if err := r.input(in); err != nil {
		return nil, nil, err
	}
	// A step is rounds. A round runs to its fixpoint with the goals it started
	// with; then the pending rollbacks and the escalation are applied and a new
	// round starts, until a round ends with nothing pending. Each boundary
	// consumes a transaction or the escalation.
	for {
		if err := r.round(order, rev); err != nil {
			return nil, nil, err
		}
		again := r.resolveTx()
		if r.escPending {
			r.escalate(r.escStatus)
			r.escPending, r.escStatus = false, 0
			again = true
		}
		if !again {
			break
		}
	}
	return r.w, r.res, nil
}

func (r *run) input(in Input) error {
	switch in.Kind {
	case InEvent:
		return r.event(in)
	case InRetry:
		return r.write(in, Ry)
	case InRestart:
		return r.write(in, Rs)
	case InStart:
		return r.start(in)
	case InStop:
		return r.stop(in)
	case InTerm:
		r.escalate(0)
		return nil
	case InTick:
		return nil
	case InDeadline:
		return r.deadline(in)
	}
	return ErrInput
}

func (r *run) event(in Input) error {
	if in.Svc < 0 || in.Svc >= len(r.p.Svc) || in.Event == Ry || in.Event == Rs || in.Event >= NumEvents {
		return ErrInput
	}
	sv := &r.w.Svc[in.Svc]
	if !accepts(sv, in) {
		r.res.Stale = true
		return nil
	}
	c := &r.p.Tables.Event[sv.S][in.Event]
	if c.Reach != Reachable {
		return ErrUnreachable
	}
	if in.Perm && (in.Event != X1 || c.Pre&(ARecFail|ARecSigPerm) == 0) {
		return ErrInput
	}
	switch in.Event {
	case SEt, SEp:
		sv.Spawning, sv.MainUp, sv.Tree = false, false, false
	case X0, X1:
		sv.Spawning, sv.MainUp = false, false
	case TE:
		sv.Tree = false
	case Ri:
		sv.Spawning = false
	}
	if t := EventTimer[in.Event]; t.Has {
		sv.Live &^= 1 << t.Timer
	}
	r.cell(in.Svc, c, in.Perm, ctx{})
	return nil
}

// accepts is the epoch and generation filter, with the O1 and O2 orders of
// section 3.1 as bits of the service.
func accepts(sv *SvcState, in Input) bool {
	switch in.Event {
	case SEt, SEp:
		return in.Epoch == sv.Epoch && sv.Spawning
	case X0, X1:
		return in.Epoch == sv.Epoch && sv.MainUp
	case TE:
		return in.Epoch == sv.Epoch && sv.Tree && !sv.MainUp
	case Ri:
		return in.Epoch == sv.Epoch && sv.Tree && sv.MainUp
	}
	t := EventTimer[in.Event].Timer
	return in.Gen == sv.Gen && sv.Live&(1<<t) != 0
}

// write is Retry or Restart by an API caller (section 3.3, 3.4).
func (r *run) write(in Input, ev Event) error {
	if r.w.Mode == Quiescing {
		return ErrQuiescing
	}
	if in.Svc < 0 || in.Svc >= len(r.p.Svc) {
		return ErrInput
	}
	if err := r.writable(in.By, 1<<in.Svc); err != nil {
		return err
	}
	sv := &r.w.Svc[in.Svc]
	if sv.S == Leaked {
		return ErrLeaked
	}
	if in.By == Delegate {
		if err := r.apiCharge(); err != nil {
			return err
		}
	}
	r.cell(in.Svc, &r.p.Tables.Event[sv.S][ev], false, ctx{caller: in.By, apiPaid: in.By == Delegate})
	return nil
}

// writable is the check every API write passes first, in this order:
// quiescing, malformed targets, then the grant.
func (r *run) writable(by Principal, targets uint32) error {
	if r.w.Mode == Quiescing {
		return ErrQuiescing
	}
	n := len(r.p.Svc)
	if by > Delegate || targets == 0 || targets>>n != 0 {
		return ErrInput
	}
	if by == Delegate && targets&^r.grantMask() != 0 {
		return ErrGrant
	}
	return nil
}

func (r *run) grantMask() uint32 {
	var m uint32
	for i, s := range r.p.Svc {
		if s.Grant {
			m |= 1 << i
		}
	}
	return m
}

// apiCharge is the API payment of a delegated write: refused, before it takes
// effect, when it would take |R_p| past budget_p.
func (r *run) apiCharge() error {
	if count(r.w.RP, r.w.Now, r.p.Window)+1 > r.p.BudgetP {
		return ErrPrincipalBudget
	}
	r.w.RP = appendWindow(r.w.RP, r.w.Now, r.p.Window)
	return nil
}

func (r *run) start(in Input) error {
	if err := r.writable(in.By, in.Targets); err != nil {
		return err
	}
	// One partition per request (section 2.3, draft 10): the targets of a
	// transaction lie in O or in one grant, never across the cut.
	if g := r.grantMask(); in.Targets&g != 0 && in.Targets&^g != 0 {
		return ErrInput
	}
	// A start by q on X while h(q, X) is held adds nothing and opens no
	// transaction (section 2.3, N4); it is never an error. A repeated
	// --no-rollback marks the open transaction of the holder permanent.
	var fresh uint32
	for i := range r.w.Svc {
		if in.Targets&(1<<i) == 0 {
			continue
		}
		if !r.w.Svc[i].H[in.By] {
			fresh |= 1 << i
		} else if t := &r.w.Tx[r.p.TxIndex(in.By, i)]; t.Active && in.NoRollback {
			t.NoRollback = true
		}
	}
	if fresh == 0 {
		return nil
	}
	if in.By == Delegate {
		if err := r.apiCharge(); err != nil {
			return err
		}
	}
	for i := range r.w.Svc {
		if fresh&(1<<i) == 0 {
			continue
		}
		before := r.p.Goals(r.w)
		r.w.Svc[i].H[in.By] = true
		after := r.p.Goals(r.w)
		r.w.Tx[r.p.TxIndex(in.By, i)] = Tx{Active: true, Members: 1 << i, T: after &^ before, Deadline: in.Deadline, NoRollback: in.NoRollback}
	}
	return nil
}

// dependents is the set of services with an uncut require or want edge to a
// member of set, and with goal up in goal: D(X) of section 2.3 (N12).
func (p *Plan) dependents(set, goal uint32) uint32 {
	var d uint32
	p.edges(func(a, b int, k edgeKind) {
		if k != edgeOrd && set&(1<<b) != 0 && goal&(1<<a) != 0 && !p.cut(a, b) {
			d |= 1 << a
		}
	})
	return d
}

// reverseClosure is set plus every service that reaches it through uncut
// require and want edges.
func (p *Plan) reverseClosure(set uint32) uint32 {
	for {
		more := set | p.dependents(set, ^uint32(0))
		if more == set {
			return set
		}
		set = more
	}
}

// stop is `stop X` of section 2.3 (N12): refused with ErrStillWanted while an
// uncut require or want dependent keeps X up, unless --with-dependents; it
// clears EVERY holder of the cleared services, and a transaction that held one
// ends superseded, without rollback.
func (r *run) stop(in Input) error {
	if err := r.writable(in.By, in.Targets); err != nil {
		return err
	}
	clear := in.Targets
	if in.WithDependents {
		clear = r.p.reverseClosure(in.Targets)
	} else if d := r.p.dependents(in.Targets, r.goalsWithout(in.Targets)); d != 0 {
		e := &StillWantedError{By: d}
		if in.By == Delegate {
			e.By, e.Others = d&r.grantMask(), bits.OnesCount32(d&^r.grantMask())
		}
		return e
	}
	if in.By == Delegate {
		if err := r.apiCharge(); err != nil {
			return err
		}
	}
	for i := range r.w.Svc {
		if clear&(1<<i) == 0 {
			continue
		}
		r.w.Svc[i].H = [2]bool{}
		for by := Owner; by <= Delegate; by++ {
			if t := &r.w.Tx[r.p.TxIndex(by, i)]; t.Active {
				t.Active, t.Out = false, TxSuperseded
			}
		}
	}
	return nil
}

// goalsWithout is the goal vector with the holders of set removed.
func (r *run) goalsWithout(set uint32) uint32 {
	w := r.w.Clone()
	for i := range w.Svc {
		if set&(1<<i) != 0 {
			w.Svc[i].H = [2]bool{}
		}
	}
	return r.p.Goals(w)
}

func (r *run) deadline(in Input) error {
	if in.By > Delegate || in.Svc < 0 || in.Svc >= len(r.p.Svc) {
		return ErrInput
	}
	q := r.p.TxIndex(in.By, in.Svc)
	if t := &r.w.Tx[q]; !t.Active || r.w.Now < t.Deadline {
		r.res.Stale = true
		return nil
	}
	r.endTx(q, TxFailed)
	return nil
}

// endTx ends the transaction at index q; it reports whether a rollback
// removed holders.
func (r *run) endTx(q int, out TxOut) bool {
	t := &r.w.Tx[q]
	t.Active, t.Out = false, out
	if out != TxFailed || t.NoRollback {
		return false
	}
	by := Principal(q / len(r.p.Svc))
	for i := range r.w.Svc {
		if t.Members&(1<<i) != 0 {
			r.w.Svc[i].H[by] = false
		}
	}
	return t.Members != 0
}

// resolveTx ends every transaction that has succeeded or failed; it reports
// whether a rollback changed the holders, so that propagation must run again.
func (r *run) resolveTx() bool {
	rolled := false
	for q := range r.w.Tx {
		t := &r.w.Tx[q]
		if !t.Active {
			continue
		}
		allUp := true
		failed := false
		for i, s := range r.w.Svc {
			if t.T&(1<<i) != 0 {
				// Leaked fails the transaction as Failed does (draft 16,
				// section 2.4): it is stable and never Ready.
				failed = failed || s.S == Failed || s.S == Leaked
				allUp = allUp && isRD(s.S)
			}
		}
		switch {
		case failed:
			rolled = r.endTx(q, TxFailed) || rolled
		case allUp:
			r.endTx(q, TxOK)
		}
	}
	return rolled
}

// raiseEscalation records an escalation raised inside a round; it is applied
// between rounds (step).
func (r *run) raiseEscalation(status int) {
	r.escPending = true
	if status > r.escStatus {
		r.escStatus = status
	}
}

// escalate is mode := Quiescing(max(status, current)): holders are cleared
// and open transactions aborted without rollback (section 3.3).
func (r *run) escalate(status int) {
	w := r.w
	if w.Mode != Quiescing {
		w.Mode = Quiescing
		r.res.Effects = append(r.res.Effects, Effect{Kind: EfQuiesce})
	}
	if status > w.Status {
		w.Status = status
	}
	for i := range w.Svc {
		w.Svc[i].H = [2]bool{}
	}
	for q := range w.Tx {
		if w.Tx[q].Active {
			w.Tx[q].Active, w.Tx[q].Out = false, TxAborted
		}
	}
}

// cond evaluates a conjunction of predicates for service i.
func (r *run) cond(i int, c Cond) bool {
	sv := &r.w.Svc[i]
	var goal uint32
	if c&(CGoalUp|CGoalDown) != 0 {
		goal = r.p.Goals(r.w)
	}
	want := func(bit Cond, v bool) bool { return c&bit == 0 || v }
	return want(CGoalUp, goal&(1<<i) != 0) &&
		want(CGoalDown, goal&(1<<i) == 0) &&
		want(CPending, r.p.pending(r.w, i)) &&
		want(CReasonRecoverable, sv.Reason == ReasonDep || sv.Reason == ReasonWait) &&
		want(COneshot, r.p.Svc[i].Kind == Oneshot) &&
		want(CExitOK, sv.Exit == ExitOK) &&
		want(CExitFail, sv.Exit == ExitFail) &&
		want(CExitPerm, sv.Exit == ExitPerm) &&
		want(CCauseDepRestartNone, sv.Cause == CauseDep || sv.Cause == CauseRestart || sv.Cause == CauseNone) &&
		want(CCauseFail, sv.Cause == CauseFail) &&
		want(CCauseNatural, sv.Cause == CauseNatural) &&
		want(CPermits, r.permits(i))
}

// permits is permits(exit) of section 3.2; only ok and fail can be permitted.
func (r *run) permits(i int) bool {
	switch r.w.Svc[i].Exit {
	case ExitOK:
		return Permits[r.p.Svc[i].Restart][0]
	case ExitFail:
		return Permits[r.p.Svc[i].Restart][1]
	}
	return false
}

// cell runs one event-table cell: its Pre actions, then its Then or Else move.
func (r *run) cell(i int, c *Cell, perm bool, x ctx) {
	sv := &r.w.Svc[i]
	switch {
	case c.Pre&ASetWexp != 0:
		sv.Wexp = true
	case c.Pre&ARecOK != 0:
		r.record(sv, ExitOK)
	case c.Pre&ARecFail != 0 && perm, c.Pre&ARecSigPerm != 0 && perm:
		r.record(sv, ExitPerm)
	case c.Pre&(ARecFail|ARecFailNoPipe) != 0:
		r.record(sv, ExitFail)
	case c.Pre&(ARecSig|ARecSigPerm) != 0:
		r.record(sv, ExitSig)
	}
	m := c.Then
	if c.Cond != 0 && !r.cond(i, c.Cond) {
		m = c.Else
	}
	r.apply(i, m, x)
}

// record is record(x) of section 3.2.
func (r *run) record(sv *SvcState, x Exit) {
	if sv.Exit == ExitNone {
		sv.Exit = x
	}
	if sv.Cause == CauseNone && x != ExitSig {
		sv.Cause = CauseNatural
	}
}

// apply performs a move: settle if asked, the cause, the payments, the state
// entry, then the effects.
func (r *run) apply(i int, m Move, x ctx) {
	sv := &r.w.Svc[i]
	if m.Actions&ASettle != 0 {
		m = r.settle(i)
	}
	if m.Cause != CauseNone && (m.Force || sv.Cause == CauseNone) {
		sv.Cause = m.Cause
	}
	var delay int64
	if m.Actions&ARetryPay != 0 {
		sv.R = nil
		r.payRetry(i, x)
	}
	if m.Actions&ADelayMax != 0 {
		delay = r.p.BackoffMax
	}
	if m.Actions&ADelayRestart != 0 {
		delay = r.backoffDelay(i)
	}
	if m.Target != Same {
		r.enter(i, m.Target, m.Reason, delay)
	}
	r.post(i, m)
}

// post performs the effects of a move, after the state entry.
func (r *run) post(i int, m Move) {
	sv := &r.w.Svc[i]
	ef := func(e Effect) {
		e.Svc, e.Epoch, e.Gen = i, sv.Epoch, sv.Gen
		r.res.Effects = append(r.res.Effects, e)
	}
	if m.Actions&ASpawn != 0 {
		sv.Epoch++
		sv.Exit, sv.Cause = ExitNone, CauseNone
		sv.Tree, sv.MainUp, sv.Spawning = true, true, true
		ef(Effect{Kind: EfSpawn})
	}
	if m.Actions&ATerm != 0 {
		ef(Effect{Kind: EfTerm})
	}
	if m.Actions&AKill != 0 {
		ef(Effect{Kind: EfKill})
	}
	if m.Actions&AArmLiveness != 0 {
		sv.Live |= 1 << TLiveness
		ef(Effect{Kind: EfArm, Timer: TLiveness})
	}
	if m.Actions&AArmWait != 0 {
		sv.Armed = true
		sv.Live |= 1 << TWait
		ef(Effect{Kind: EfArm, Timer: TWait})
	}
	if m.Actions&AReport != 0 {
		ef(Effect{Kind: EfReport, Report: m.Report})
	}
}

// enter is a state entry: gen++, wexp and armed reset, the state's timer armed.
func (r *run) enter(i int, to State, reason Reason, delay int64) {
	sv := &r.w.Svc[i]
	sv.S, sv.Reason = to, reason
	sv.Gen++
	sv.Wexp, sv.Armed, sv.Live = false, false, 0
	if to != Starting {
		// The spawn outcome is delivered before anything else of the epoch
		// (O3), so SEt and SEp exist only while the state is Starting; a
		// late one is dropped by the filter.
		sv.Spawning = false
	}
	if st := StateTimer[to]; st.Has {
		sv.Live = 1 << st.Timer
		r.res.Effects = append(r.res.Effects, Effect{Kind: EfArm, Svc: i, Epoch: sv.Epoch, Gen: sv.Gen, Timer: st.Timer, Delay: delay})
	}
	if to == Failed && r.p.Svc[i].Terminal {
		r.raiseEscalation(1)
	}
}

// settle is settle(A) of section 3.2: the first rule whose condition holds.
// The last rule is unconditional, which NewPlan checks.
func (r *run) settle(i int) Move {
	rules := r.p.Tables.Settle
	k := 0
	for !r.cond(i, rules[k].Cond) {
		k++
	}
	if rules[k].Charge && r.charge(i) {
		return rules[k].Over
	}
	return rules[k].Then
}

// charge is charge(A) then overflow(A): it reports Overflow, and escalates
// itself when the owner's counter overflowed (E3).
func (r *run) charge(i int) bool {
	w, p := r.w, r.p
	sv := &w.Svc[i]
	sv.R = appendWindow(sv.R, w.Now, p.Window)
	oA := count(sv.R, w.Now, p.Window) > p.Svc[i].MaxA
	var oP bool
	delegated := p.Svc[i].Grant
	if delegated {
		w.RP = appendWindow(w.RP, w.Now, p.Window)
		oP = count(w.RP, w.Now, p.Window) > p.BudgetP
	} else {
		w.RInst = appendWindow(w.RInst, w.Now, p.Window)
		oP = count(w.RInst, w.Now, p.Window) > p.MaxInst
	}
	if oP && !delegated {
		r.raiseEscalation(1)
	}
	return oA || (oP && delegated)
}

// payRetry is the Retry cell's R_A := empty; pay(payer(A)) (F9, G1): the
// payment is made once, the API having made it already for a delegated caller.
func (r *run) payRetry(i int, x ctx) {
	w, p := r.w, r.p
	payer := Owner
	if p.Svc[i].Grant {
		payer = Delegate
	}
	if p.Quirks.RetryPayCaller {
		payer = x.caller
	}
	switch {
	case payer == Delegate:
		if !x.apiPaid {
			w.RP = appendWindow(w.RP, w.Now, p.Window)
		}
	default:
		w.RInst = appendWindow(w.RInst, w.Now, p.Window)
		if count(w.RInst, w.Now, p.Window) > p.MaxInst {
			r.raiseEscalation(1)
		}
	}
}

// backoffDelay is min(initial * factor^(|R_A|-1), max).
func (r *run) backoffDelay(i int) int64 {
	p := r.p
	d := p.Initial
	for k := 1; k < p.RCount(r.w, i) && d < p.BackoffMax; k++ {
		d *= p.Factor
	}
	if d > p.BackoffMax {
		d = p.BackoffMax
	}
	return d
}

// moveCap bounds the guard moves of one step. The measured maximum is far
// below it (see testdata/moves.golden); only a defective grid reaches it.
func (p *Plan) moveCap() int { return 32 * len(p.Svc) * len(p.Svc) }

// round is one round of propagation (section 3.5, draft 16), in two phases.
// Phase 1 fires every guard except G_free, service by service in a
// targets-first order, each stabilised before the next: one pass reaches the
// fixpoint. Phase 2 fires G_free once, for the services of StopWait whose
// G_free holds in the world phase 1 ended in; it is order independent.
func (r *run) round(order []int, rev bool) error {
	for _, i := range order {
		for r.fire(i, false) {
			if len(r.res.Moves) > r.p.moveCap() {
				return ErrNoFixpoint
			}
		}
	}
	goal := r.p.Goals(r.w)
	var f []int
	for i := range r.w.Svc {
		if _, _, ok := r.next(i, goal, true); ok {
			f = append(f, i)
		}
	}
	if rev {
		for a, b := 0, len(f)-1; a < b; a, b = a+1, b-1 {
			f[a], f[b] = f[b], f[a]
		}
	}
	for _, i := range f {
		r.fire(i, true)
	}
	return nil
}

// signature is the inputs of service i's guards, one bit each.
func (r *run) signature(i int, goal uint32) uint8 {
	var s uint8
	for g := Guard(0); g < NumGuards; g++ {
		if r.p.Guard(r.w, goal, i, g) {
			s |= 1 << g
		}
	}
	return s
}

// next is the first enabled guard of service i's row, in Prio order, among
// G_free alone (free) or every other guard (not free).
func (r *run) next(i int, goal uint32, free bool) (Guard, *Cell, bool) {
	sv := &r.w.Svc[i]
	for _, g := range r.p.order[sv.S] {
		c := &r.p.Tables.Grid[sv.S][g]
		if (g == GFree) != free {
			continue
		}
		if r.p.Guard(r.w, goal, i, g) && (c.Cond == 0 || r.cond(i, c.Cond)) {
			return g, c, true
		}
	}
	return 0, nil, false
}

// fire applies the first enabled guard of service i (see next).
func (r *run) fire(i int, free bool) bool {
	goal := r.p.Goals(r.w)
	sv := &r.w.Svc[i]
	g, c, ok := r.next(i, goal, free)
	if !ok {
		return false
	}
	rec := MoveRec{Svc: i, From: sv.S, Guard: g, Sig: r.signature(i, goal)}
	r.apply(i, c.Then, ctx{})
	rec.To = sv.S
	r.res.Moves = append(r.res.Moves, rec)
	return true
}
