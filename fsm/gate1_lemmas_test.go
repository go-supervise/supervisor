package fsm

import (
	"fmt"
	"math/bits"
	"runtime"
	"sort"
	"sync"
	"testing"
)

// Gate 1, Theorem C (draft 17 section 3.8): the frame lemmas F1 to F6, each
// checked EXHAUSTIVELY on its neighbourhood by enumeration, not by search.
//
// The neighbourhood of a lemma is one service's local state and the abstract
// values of the services it reads. The local states are the pool below: every
// distinct local state (state, reason, cause, exit, the five sojourn bits, the
// live timers, |R_A|, the holders) that a one-copy search reaches, taken from
// the closed N = 1 graphs of the 18 plans of Theorem U and from the first
// lemmaExpand states of the N = 2 graphs of the gadget plans. A neighbourhood
// is a product of such pools with the values of the globals the lemma reads
// (mode, |R_inst|, |R_p|, a transaction), and it is closed when the enumeration
// finishes with exactly the number of cases the product of its dimensions
// says: no cap, no sample. The plans are the 15 gadgets of composePool: two
// services, one edge 1 -> 0 of each kind, every grant assignment the closure
// allows, service 0 terminal or not. Together they hold every role of a
// service in the structure the lemmas assume (closure under every edge kind,
// the cut, terminal not in G): a target or a dependent, in O or in G, with its
// neighbour in O or in G. The induction on N that lifts the result to every N
// is the proof file's (section 3.8), and is not re-done here.

const lemmaExpand = 20000

var (
	poolOnce sync.Once
	poolVal  []SvcState
)

// lemmaPool is the pool of local states, in a fixed order, with epoch and
// generation zero and R holding |R_A| entries at time 0.
func lemmaPool(t testing.TB) []SvcState {
	poolOnce.Do(func() {
		seen := map[string]SvcState{}
		add := func(s SvcState) {
			s.Epoch, s.Gen = 0, 0
			s.R = make([]int64, len(s.R))
			k := fmt.Sprintf("%d.%d.%d.%d.%t%t%t%t%t.%d.%d.%v", s.S, s.Reason, s.Cause, s.Exit, s.Wexp, s.Armed, s.Tree, s.MainUp, s.Spawning, s.Live, len(s.R), s.H)
			seen[k] = s
		}
		for _, p := range unwindPlans(t) {
			for _, w := range buildGraph(p, p.NewWorld(), 100000).worlds {
				add(w.Svc[0])
			}
		}
		for _, p := range composePool(t) {
			g := buildGraph(p, p.NewWorld(), lemmaExpand)
			for _, w := range g.worlds[:min(len(g.worlds), lemmaExpand)] {
				for _, s := range w.Svc {
					add(s)
				}
			}
		}
		keys := make([]string, 0, len(seen))
		for k := range seen {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			poolVal = append(poolVal, seen[k])
		}
	})
	return poolVal
}

// mrCfg is a value of the globals pi reads: the mode and |R_inst|.
type mrCfg struct {
	mode        Mode
	status, rin int
}

var lemmaMR = []mrCfg{{Running, 0, 0}, {Running, 0, 1}, {Quiescing, 1, 0}, {Quiescing, 1, 2}}

func cloneSvc(s SvcState) SvcState {
	s.R = append([]int64(nil), s.R...)
	return s
}

// lemmaWorld is a two-service world: local states a and b, the globals m, and
// rp entries in R_p.
func lemmaWorld(p *Plan, a, b SvcState, m mrCfg, rp int) *World {
	w := p.NewWorld()
	w.Svc[0], w.Svc[1] = cloneSvc(a), cloneSvc(b)
	w.Mode, w.Status = m.mode, m.status
	w.RInst = make([]int64, m.rin)
	w.RP = make([]int64, rp)
	return w
}

// lemmaPlans are the gadgets, without the terminal variants unless term.
func lemmaPlans(t testing.TB, term bool) map[string]*Plan {
	m := map[string]*Plan{}
	for name, p := range composePool(t) {
		if term || !p.Svc[0].Terminal {
			m[name] = p
		}
	}
	return m
}

// eachPlan runs f on every plan, in parallel, and fails on the first red (in
// name order) or on an enumeration that did not finish: cases must equal want.
func eachPlan(t *testing.T, plans map[string]*Plan, f func(p *Plan) (cases, want int, red string)) (total int) {
	t.Helper()
	names := make([]string, 0, len(plans))
	for n := range plans {
		names = append(names, n)
	}
	sort.Strings(names)
	type res struct {
		cases, want int
		red         string
	}
	out := make([]res, len(names))
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for i, n := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			c, w, r := f(plans[n])
			out[i] = res{c, w, r}
		}()
	}
	wg.Wait()
	for i, n := range names {
		switch {
		case out[i].red != "":
			t.Fatalf("%s: %s", n, out[i].red)
		case out[i].cases != out[i].want || out[i].want == 0:
			t.Fatalf("%s: the neighbourhood is not closed: %d cases enumerated, %d expected", n, out[i].cases, out[i].want)
		}
		total += out[i].cases
	}
	return total
}

// svcPiEq is piKey's service part: no epoch, no generation, |R_A| only.
func svcPiEq(a, b *SvcState) bool {
	return a.S == b.S && a.Reason == b.Reason && a.Cause == b.Cause && a.Exit == b.Exit && a.Wexp == b.Wexp && a.Armed == b.Armed &&
		a.Tree == b.Tree && a.MainUp == b.MainUp && a.Spawning == b.Spawning && a.Live == b.Live && len(a.R) == len(b.R) && a.H == b.H
}

func txPiEq(a, b *Tx) bool {
	return a.Active == b.Active && a.Out == b.Out && a.Members == b.Members && a.T == b.T && a.NoRollback == b.NoRollback
}

// piEq is piKey(a) == piKey(b), without the strings.
func piEq(p *Plan, a, b *World) bool {
	if a.Mode != b.Mode || a.Status != b.Status || len(a.RInst) != len(b.RInst) {
		return false
	}
	for i := range a.Svc {
		if !p.Svc[i].Grant && (!svcPiEq(&a.Svc[i], &b.Svc[i]) || !txPiEq(&a.Tx[p.TxIndex(Owner, i)], &b.Tx[p.TxIndex(Owner, i)])) {
			return false
		}
	}
	return true
}

func sliceEq(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// gEq is gamma(a) == gamma(b): every service of G in full, every holder and
// transaction on G, R_p.
func gEq(p *Plan, a, b *World) bool {
	if !sliceEq(a.RP, b.RP) {
		return false
	}
	for i := range a.Svc {
		if !p.Svc[i].Grant {
			continue
		}
		x, y := &a.Svc[i], &b.Svc[i]
		if !svcPiEq(x, y) || x.Epoch != y.Epoch || x.Gen != y.Gen || !sliceEq(x.R, y.R) {
			return false
		}
		for _, q := range []Principal{Owner, Delegate} {
			if a.Tx[p.TxIndex(q, i)] != b.Tx[p.TxIndex(q, i)] {
				return false
			}
		}
	}
	return true
}

// handlerTokens are the tokens of a plan that reach a handler: every token of
// the abstraction but AGE (a window operation, not an input of Step).
func handlerTokens(p *Plan) (toks []Tok, classes []byte) {
	for _, tok := range abstractTokens(p) {
		if tok.K != ageTok {
			toks = append(toks, tok)
			classes = append(classes, tokClass(p, tok))
		}
	}
	return
}

// tokNames is the service a token names, or -1.
func tokNames(p *Plan, tok Tok) int {
	n := len(p.Svc)
	switch tok.K % numKinds {
	case kEvent, kRetry, kRestart, kDeadline:
		return int(tok.A) % n
	case kStart, kStop:
		if m := uint32(tok.A) & (1<<n - 1); bits.OnesCount32(m) == 1 {
			return bits.TrailingZeros32(m)
		}
	}
	return -1
}

// handlerTokensFor are the handler tokens of the sweep whose named service is
// s: those that name s and those that name none.
func handlerTokensFor(p *Plan, s int) (toks []Tok, classes []byte) {
	all, cl := handlerTokens(p)
	for i, tok := range all {
		if k := tokNames(p, tok); k == s || k == -1 {
			toks = append(toks, tok)
			classes = append(classes, cl[i])
		}
	}
	return
}

// inputOutcome is what an input handler did, as far as pi sees it.
type inputOutcome struct {
	kind  uint8 // 0 accepted, 1 refused, 2 stale
	class string
	w     *World
	eff   string
}

func runHandler(p *Plan, w *World, tok Tok) (*run, inputOutcome) {
	in := decode(p, w, tok)
	r := &run{p: p, w: w.Clone(), res: &Result{}}
	r.w.Now = in.Now
	err := r.input(in)
	switch {
	case err != nil:
		return r, inputOutcome{kind: 1, class: errClass(err).Error()}
	case r.res.Stale:
		return r, inputOutcome{kind: 2}
	}
	return r, inputOutcome{w: r.w, eff: effO(p, r.res)}
}

// sides splits the services of a gadget into O and G.
func sides(p *Plan) (o, g []int) {
	for i, s := range p.Svc {
		if s.Grant {
			g = append(g, i)
		} else {
			o = append(o, i)
		}
	}
	return
}

// forPi enumerates the worlds of the neighbourhood pool^2 x mrs x rps with the
// pi part (the states of the services of O, the globals mr) in the OUTER loops
// and the gamma part (the states of G, R_p) in the inner ones: pi(w) is the
// same for every world between two calls of first. first is true on the first
// world of each pi.
func forPi(p *Plan, pools [2][]SvcState, rps int, f func(w *World, first bool)) {
	o, g := sides(p)
	idx := func(set []int, v int) []int { // v-th assignment of pool indexes to the services of set
		out := make([]int, len(set))
		for k, i := range set {
			out[k] = v % len(pools[i])
			v /= len(pools[i])
		}
		return out
	}
	count := func(set []int) int {
		n := 1
		for _, i := range set {
			n *= len(pools[i])
		}
		return n
	}
	for ov := 0; ov < count(o); ov++ {
		oi := idx(o, ov)
		for _, m := range lemmaMR {
			first := true
			for gv := 0; gv < count(g); gv++ {
				gi := idx(g, gv)
				for rp := 0; rp < rps; rp++ {
					w := lemmaWorld(p, pools[0][0], pools[1][0], m, rp)
					for k, i := range o {
						w.Svc[i] = cloneSvc(pools[i][oi[k]])
					}
					for k, i := range g {
						w.Svc[i] = cloneSvc(pools[i][gi[k]])
					}
					f(w, first)
					first = false
				}
			}
		}
	}
}

// forPiSize is the size of forPi's enumeration.
func forPiSize(pools [2][]SvcState, rps int) int {
	return len(lemmaMR) * rps * len(pools[0]) * len(pools[1])
}

// reps keeps one local state per abstract value (state, holders): what a
// handler or a guard of ANOTHER service reads of this one (Goals reads the
// holders, pending, G_ok, G_free the state).
func reps(pool []SvcState) []SvcState {
	var out []SvcState
	seen := map[[3]uint8]bool{}
	for _, s := range pool {
		k := [3]uint8{uint8(s.S), b2u(s.H[Owner]), b2u(s.H[Delegate])}
		if !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

func b2u(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// F1, input handler: an L input writes only pi components and reads only pi;
// an H or Z input writes no pi component, charges no window of pi (so moves no
// |R_inst|), and drops no stale event wrongly. Neighbourhood: every pair of
// local states of the two services x the 4 values of (mode, |R_inst|) x 2 of
// |R_p| x every token (stale ones included), at the handler, before any
// propagation. The reads-only-pi half is a function check: for one pi, every
// gamma gives the same outcome.
//
// Measured: the full product (853 x 853 local states x 8 x 83 tokens) is 483 M
// handler calls per plan, 138 s wall on 16 cores for 9 plans. A handler reads
// the local state of the service its token names and, of every other service,
// only its state and holders (Goals, pending, dependents), so the sound bound
// is the two sweeps below: the named service over the whole pool against the
// other's abstract values, in each of the two roles. The one-sided reads of the
// complement are asserted by TestGuardsReadWhatTheTableSays and by F2, which
// does enumerate the whole product for the cells.
//
// An L input MAY write gamma: SIGTERM (kTerm) clears every holder, a service of
// G's included. U needs only that pi' is a function of pi, which the
// function check asserts; "writes only pi" of the draft is checked for every L
// input but that one.
func f1Plan(p *Plan, pool []SvcState) (cases, want int, red string) {
	r := reps(pool)
	sweeps := [][2][]SvcState{{pool, r}, {r, pool}}
	for s, pools := range sweeps {
		toks, classes := handlerTokensFor(p, s)
		ref := make([]inputOutcome, len(toks))
		want += forPiSize(pools, 2) * len(toks)
		forPi(p, pools, 2, func(w *World, first bool) {
			if red != "" {
				return
			}
			for ti, tok := range toks {
				cases++
				run, o := runHandler(p, w, tok)
				switch {
				case isStaleTok(tok) && o.kind != 2:
					red = fmt.Sprintf("F1/SF: the stale event %+v was not dropped", tok)
				case classes[ti] != 'L' && o.kind == 0 && (!piEq(p, w, o.w) || o.eff != ""):
					red = fmt.Sprintf("F1: the %c input %+v writes a pi component or an O tree (effects %q)", classes[ti], tok, o.eff)
				case classes[ti] == 'L' && o.kind == 0 && tok.K != kTerm && !gEq(p, w, run.w):
					red = fmt.Sprintf("F1: the L input %+v writes a gamma component", tok)
				case classes[ti] == 'L':
					if first {
						ref[ti] = o
					} else if b := ref[ti]; b.kind != o.kind || b.class != o.class || b.eff != o.eff || (o.kind == 0 && !piEq(p, b.w, o.w)) {
						red = fmt.Sprintf("F1: the L input %+v reads gamma: two worlds with the same pi answer it differently", tok)
					}
				}
				if red != "" {
					return
				}
			}
		})
	}
	return
}

func TestGate1LemmaF1(t *testing.T) {
	pool := lemmaPool(t)
	n := eachPlan(t, lemmaPlans(t, false), func(p *Plan) (int, int, string) { return f1Plan(p, pool) })
	t.Logf("F1: %d local states (%d abstract), 9 plans, %d (world, token) cases, closed", len(pool), len(reps(pool)), n)
}

func TestGate1LemmaF1Witness(t *testing.T) {
	p := newPlan(t, Plan{Quirks: Quirks{RetryPayCaller: true}, MaxInst: 1, BudgetP: 1, Svc: []SvcPlan{{Restart: Always, MaxA: 1, Terminal: true}, {Restart: OnFailure, MaxA: 1, Grant: true, Req: []int{0}}}})
	_, _, red := f1Plan(p, lemmaPool(t))
	t.Logf("RetryPayCaller: %.140s", red)
	if red == "" {
		t.Error("F1 does not see an owner Retry of a G service paying R_inst")
	}
}
