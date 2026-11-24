package fsm

import (
	"errors"
	"fmt"
)

// Principal is the class of the caller of a write: the owner, or the one
// delegated principal p of the model (section 3.3; N principals reduce to one
// by Theorem G's symmetry).
type Principal uint8

// The principals.
const (
	Owner Principal = iota
	Delegate
)

// SvcPlan is the static part of one service.
type SvcPlan struct {
	Kind     Kind
	Restart  RestartOn
	Req      []int // require targets
	Want     []int // want targets
	Ord      []int // order-only targets
	Grant    bool  // the service is in grant(p)
	Terminal bool  // a terminal service reaching Failed escalates
	MaxA     int   // intensity.max(A)
}

// Quirks are the deliberate defects of Gate 1's witnesses (section 3.8). The
// zero value is the draft-13 model; Compile never sets one.
type Quirks struct {
	GoalNoCut      bool // draft 8's goal: holders and propagation ignore the grant cut
	NoWantClosure  bool // grants are not closed under want edges (ErrGrantNotClosed dropped)
	RetryPayCaller bool // draft 8's Retry cell: pay(caller)
	FreeAcrossCut  bool // G_free reads dependents across a cut edge
}

// Plan is the static description of an instance: services, budgets, tables.
type Plan struct {
	Svc        []SvcPlan
	MaxInst    int   // instance.intensity.max
	BudgetP    int   // budget_p of the delegated principal
	Window     int64 // intensity window, in the same unit as Input.Now
	Initial    int64 // backoff.initial
	Factor     int64 // backoff.factor
	BackoffMax int64 // backoff.max
	Quirks     Quirks
	Tables     Tables

	order     [NumStates][]Guard // present guards of each row, by Prio
	freeReq   [][]int            // dependents read by G_free through require
	freeOther [][]int            // dependents read by G_free through want or order
}

// Errors of the model.
var (
	ErrQuiescing       = errors.New("fsm: the instance is quiescing")
	ErrGrant           = errors.New("fsm: target outside the principal's grant")
	ErrLeaked          = errors.New("fsm: the service is leaked")
	ErrPrincipalBudget = errors.New("fsm: the principal's budget is spent")
	ErrTxBusy          = errors.New("fsm: a transaction of this principal is pending")
	ErrUnreachable     = errors.New("fsm: the cell is unreachable")
	ErrInput           = errors.New("fsm: malformed input")
	ErrTime            = errors.New("fsm: time went backwards")
	ErrNoFixpoint      = errors.New("fsm: propagation did not reach a fixpoint")
	ErrPlan            = errors.New("fsm: invalid plan")
	ErrGrantNotClosed  = errors.New("fsm: grant not closed under the reverse of an edge")
)

// MaxServices is the bound of the service bitmasks.
const MaxServices = 32

// NewPlan validates p, fills the default tables when none are set, and builds
// the dependent indexes.
func NewPlan(p Plan) (*Plan, error) {
	n := len(p.Svc)
	if n == 0 || n > MaxServices {
		return nil, fmt.Errorf("%w: %d services", ErrPlan, n)
	}
	if p.MaxInst < 1 || p.BudgetP < 1 || p.Window < 1 || p.Initial < 1 || p.Factor < 1 || p.BackoffMax < 1 {
		return nil, fmt.Errorf("%w: a budget, window or delay below 1", ErrPlan)
	}
	const maxDelay = 1 << 31 // initial*factor stays below 2^62
	if p.Initial > maxDelay || p.Factor > maxDelay || p.BackoffMax > maxDelay {
		return nil, fmt.Errorf("%w: a delay above 2^31", ErrPlan)
	}
	if p.Tables.Event[0][0].Reach == ReachInvalid {
		p.Tables = Default()
	}
	if last := len(p.Tables.Settle) - 1; last < 0 || p.Tables.Settle[last].Cond != 0 {
		return nil, fmt.Errorf("%w: the last settle rule must be unconditional", ErrPlan)
	}
	for a, s := range p.Svc {
		if s.MaxA < 1 {
			return nil, fmt.Errorf("%w: service %d has intensity.max < 1", ErrPlan, a)
		}
		for _, list := range [][]int{s.Req, s.Want, s.Ord} {
			for _, b := range list {
				if b < 0 || b >= n || b == a {
					return nil, fmt.Errorf("%w: service %d has edge to %d", ErrPlan, a, b)
				}
			}
		}
	}
	if err := p.checkAcyclic(); err != nil {
		return nil, err
	}
	if err := p.checkClosed(); err != nil {
		return nil, err
	}
	p.build()
	return &p, nil
}

// targets calls f for every edge a -> b, with its kind.
func (p *Plan) edges(f func(a, b int, k edgeKind)) {
	for a, s := range p.Svc {
		for _, b := range s.Req {
			f(a, b, edgeReq)
		}
		for _, b := range s.Want {
			f(a, b, edgeWant)
		}
		for _, b := range s.Ord {
			f(a, b, edgeOrd)
		}
	}
}

type edgeKind uint8

const (
	edgeReq edgeKind = iota
	edgeWant
	edgeOrd
)

func (p *Plan) checkAcyclic() error {
	n := len(p.Svc)
	const (
		white = iota
		grey
		black
	)
	colour := make([]int, n)
	var visit func(a int) bool
	visit = func(a int) bool {
		colour[a] = grey
		s := p.Svc[a]
		for _, list := range [][]int{s.Req, s.Want, s.Ord} {
			for _, b := range list {
				if colour[b] == grey || (colour[b] == white && !visit(b)) {
					return false
				}
			}
		}
		colour[a] = black
		return true
	}
	for a := range p.Svc {
		if colour[a] == white && !visit(a) {
			return fmt.Errorf("%w: dependency cycle", ErrPlan)
		}
	}
	return nil
}

// checkClosed is the grant closure of E2: a service with an edge of any kind
// to a granted one is granted. Quirks.NoWantClosure drops it for want edges.
func (p *Plan) checkClosed() error {
	var err error
	p.edges(func(a, b int, k edgeKind) {
		if err == nil && p.Svc[b].Grant && !p.Svc[a].Grant && !(k == edgeWant && p.Quirks.NoWantClosure) {
			err = fmt.Errorf("%w: %d has an edge to granted %d", ErrGrantNotClosed, a, b)
		}
	})
	return err
}

// cut is cut(A, B): A in grant(p) and B not (section 2.3).
func (p *Plan) cut(a, b int) bool { return p.Svc[a].Grant && !p.Svc[b].Grant }

func (p *Plan) build() {
	n := len(p.Svc)
	p.freeReq = make([][]int, n)
	p.freeOther = make([][]int, n)
	p.edges(func(a, b int, k edgeKind) {
		if !p.cut(a, b) || p.Quirks.FreeAcrossCut {
			if k == edgeReq {
				p.freeReq[b] = append(p.freeReq[b], a)
			} else {
				p.freeOther[b] = append(p.freeOther[b], a)
			}
		}
	})
	for s := range p.order {
		p.order[s] = nil
		for pr := 1; pr <= NumGuards; pr++ {
			for g := Guard(0); g < NumGuards; g++ {
				c := p.Tables.Grid[s][g]
				if c.Reach != Absent && int(c.Prio) == pr {
					p.order[s] = append(p.order[s], g)
				}
			}
		}
	}
}

// TxOut is the outcome of a transaction.
type TxOut uint8

// The outcomes.
const (
	TxNone    TxOut = iota
	TxOK            // every service of T is Ready or Done
	TxFailed        // a member failed or the deadline passed; rolled back unless no-rollback
	TxAborted       // the instance began quiescing
)

// Tx is the transaction of one principal (section 2.4): one at a time.
type Tx struct {
	Active     bool
	Out        TxOut
	Members    uint32 // services where this request created the holder
	T          uint32 // services whose goal rose because of the request
	Deadline   int64
	NoRollback bool
}

// SvcState is the dynamic part of one service.
type SvcState struct {
	S        State
	Reason   Reason
	Cause    Cause
	Exit     Exit
	Wexp     bool
	Armed    bool
	Tree     bool // the tree exists and is not yet reported empty
	MainUp   bool // the main process has not been reported exited
	Spawning bool // no event of the new process has arrived yet
	Epoch    uint32
	Gen      uint32
	Live     uint8   // pending timers of the current generation, bit per Timer
	R        []int64 // charged restarts
	H        [2]bool // holder of each principal
}

// World is the whole mutable state.
type World struct {
	Svc    []SvcState
	Mode   Mode
	Status int
	Now    int64
	RInst  []int64
	RP     []int64
	Tx     [2]Tx
}

// NewWorld is the initial world: every service Inactive, no holder.
func (p *Plan) NewWorld() *World {
	return &World{Svc: make([]SvcState, len(p.Svc))}
}

// Clone returns a deep copy.
func (w *World) Clone() *World {
	c := *w
	c.Svc = make([]SvcState, len(w.Svc))
	for i, s := range w.Svc {
		s.R = append([]int64(nil), s.R...)
		c.Svc[i] = s
	}
	c.RInst = append([]int64(nil), w.RInst...)
	c.RP = append([]int64(nil), w.RP...)
	return &c
}

// count is |R(now)|: the timestamps inside the window.
func count(r []int64, now, window int64) int {
	n := 0
	for _, t := range r {
		if t > now-window {
			n++
		}
	}
	return n
}

// appendWindow appends now and drops what the window no longer reads.
func appendWindow(r []int64, now, window int64) []int64 {
	out := make([]int64, 0, len(r)+1)
	for _, t := range r {
		if t > now-window {
			out = append(out, t)
		}
	}
	return append(out, now)
}

// RCount is |R_A(now)| of service i.
func (p *Plan) RCount(w *World, i int) int { return count(w.Svc[i].R, w.Now, p.Window) }

// InstCount is |R_inst(now)|.
func (p *Plan) InstCount(w *World) int { return count(w.RInst, w.Now, p.Window) }

// PCount is |R_p(now)|.
func (p *Plan) PCount(w *World) int { return count(w.RP, w.Now, p.Window) }

// Goals is goal(A) for every A as a bitmask: the least fixpoint of "A has a
// holder, or an uncut require or want dependent has goal up" (section 2.3).
func (p *Plan) Goals(w *World) uint32 {
	var goal uint32
	var work []int
	for i, s := range w.Svc {
		if s.H[Owner] || s.H[Delegate] {
			goal |= 1 << i
			work = append(work, i)
		}
	}
	for len(work) > 0 {
		a := work[len(work)-1]
		work = work[:len(work)-1]
		for _, b := range p.Svc[a].Req {
			work = p.raise(&goal, work, a, b)
		}
		for _, b := range p.Svc[a].Want {
			work = p.raise(&goal, work, a, b)
		}
	}
	return goal
}

func (p *Plan) raise(goal *uint32, work []int, a, b int) []int {
	if *goal&(1<<b) != 0 || (p.cut(a, b) && !p.Quirks.GoalNoCut) {
		return work
	}
	*goal |= 1 << b
	return append(work, b)
}

func isRD(s State) bool { return s == Ready || s == Done }

func settledState(s State) bool {
	return s == Ready || s == Done || s == Failed || s == Backoff || s == Leaked
}

// pending is G_lost (and pending in Waiting): a require-target is not Ready or Done.
func (p *Plan) pending(w *World, i int) bool {
	for _, b := range p.Svc[i].Req {
		if !isRD(w.Svc[b].S) {
			return true
		}
	}
	return false
}

func (p *Plan) failedReq(w *World, i int) bool {
	for _, b := range p.Svc[i].Req {
		if s := w.Svc[b].S; s == Failed || s == Leaked {
			return true
		}
	}
	return false
}

func (p *Plan) gOk(w *World, goal uint32, i int) bool {
	if p.pending(w, i) {
		return false
	}
	for _, list := range [][]int{p.Svc[i].Want, p.Svc[i].Ord} {
		for _, b := range list {
			if goal&(1<<b) != 0 && !settledState(w.Svc[b].S) {
				return false
			}
		}
	}
	return true
}

func (p *Plan) gFree(w *World, i int) bool {
	for _, a := range p.freeReq[i] {
		switch w.Svc[a].S {
		case Starting, Ready, StopWait, Stopping, Killing, Leaked:
			return false
		}
	}
	for _, a := range p.freeOther[i] {
		switch w.Svc[a].S {
		case StopWait, Stopping, Killing, Leaked:
			return false
		}
	}
	return true
}

// Guard evaluates guard g of service i at goal.
func (p *Plan) Guard(w *World, goal uint32, i int, g Guard) bool {
	sv := &w.Svc[i]
	switch g {
	case GUp:
		return goal&(1<<i) != 0
	case GDown:
		return goal&(1<<i) == 0
	case GOk:
		return p.gOk(w, goal, i)
	case GFailed:
		return p.failedReq(w, i)
	case GLost:
		return p.pending(w, i)
	case GFree:
		return p.gFree(w, i)
	case GWexp:
		return sv.Wexp && p.pending(w, i)
	default: // GArm
		return p.pending(w, i) && !sv.Armed
	}
}

// Exited is the instance's exit condition: quiescing, and every service in
// {Inactive, Failed, Leaked} (section 3.3).
func (p *Plan) Exited(w *World) bool {
	if w.Mode != Quiescing {
		return false
	}
	for _, s := range w.Svc {
		if s.S != Inactive && s.S != Failed && s.S != Leaked {
			return false
		}
	}
	return true
}
