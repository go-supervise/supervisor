package fsm

import (
	"fmt"
	"testing"
)

// Gate 1, Theorem C (draft 17 section 3.8): the frame lemmas, for N >= 2.
//
// U does not close beyond one service, so the conditions SC and LR are checked
// where they can fail, by perturbation. Two worlds with the same pi and
// different gamma (every service of G, every holder and owner transaction on
// G, R_p) are two points of the one-copy space R; EC and SC say an L input
// treats them alike, LR says an H or Z input moves neither. The checks below
// build such pairs from the one-copy graph itself (the G part of one reachable
// world put into another reachable world of the same mode), and so reach
// combinations no run reaches, which makes them stronger than U on R.
//
// What is exhaustive and what is sampled is stated by each test: F3 and the
// closure are exhaustive over every labelled DAG of three services; the
// perturbation lemmas (F1, F2, F4, F5, F6 together: they are the clauses of one
// step, and a violation of any of them is a violation of SC or LR) are
// exhaustive over a stated pool of N = 2 worlds.

// omask is the set of services outside every grant.
func omask(p *Plan) uint32 {
	var m uint32
	for i, s := range p.Svc {
		if !s.Grant {
			m |= 1 << i
		}
	}
	return m
}

// closureViolation is the static check of E2: an edge from a service of O into
// a service of G (so a cell of O reads G). "" when there is none.
func closureViolation(p *Plan) string {
	var v string
	p.edges(func(a, b int, k edgeKind) {
		if v == "" && p.Svc[b].Grant && !p.Svc[a].Grant {
			v = fmt.Sprintf("service %d of O has an edge (kind %d) to service %d of G", a, k, b)
		}
	})
	return v
}

// dagPlans calls f for every labelled DAG of n services (every edge kind on
// every ordered pair) and every grant assignment, with the plans NewPlan
// accepts; with accept false it also gives the ones it refuses.
func dagPlans(n int, f func(p *Plan, spec Plan)) {
	type pair struct{ a, b int }
	var pairs []pair
	for a := 0; a < n; a++ {
		for b := 0; b < n; b++ {
			if a != b {
				pairs = append(pairs, pair{a, b})
			}
		}
	}
	total := 1
	for range pairs {
		total *= 4
	}
	for code := 0; code < total; code++ {
		for grant := 0; grant < 1<<n; grant++ {
			spec := Plan{MaxInst: 1, BudgetP: 1, Window: 3, Initial: 1, Factor: 2, BackoffMax: 4, Svc: make([]SvcPlan, n)}
			for i := range spec.Svc {
				spec.Svc[i] = SvcPlan{Restart: OnFailure, MaxA: 1, Grant: grant&(1<<i) != 0}
			}
			c := code
			for _, pr := range pairs {
				switch c % 4 {
				case 1:
					spec.Svc[pr.a].Req = append(spec.Svc[pr.a].Req, pr.b)
				case 2:
					spec.Svc[pr.a].Want = append(spec.Svc[pr.a].Want, pr.b)
				case 3:
					spec.Svc[pr.a].Ord = append(spec.Svc[pr.a].Ord, pr.b)
				}
				c /= 4
			}
			p, err := NewPlan(spec)
			if err != nil {
				continue // a cycle, or a grant not closed
			}
			f(p, spec)
		}
	}
}

// TestGate1FrameClosure (the static part of F2): on every plan NewPlan
// accepts, no service of O has an edge into G, so a cell of O never reads G;
// and the mutant that drops ErrGrantNotClosed for want edges is red.
func TestGate1FrameClosure(t *testing.T) {
	plans := 0
	dagPlans(3, func(p *Plan, _ Plan) {
		plans++
		if v := closureViolation(p); v != "" {
			t.Fatalf("an accepted plan breaks E2: %s", v)
		}
	})
	t.Logf("closure: %d accepted plans (labelled DAGs of 3 services x grant assignments), none has an edge from O into G", plans)
	if plans == 0 {
		t.Fatal("no plan was checked")
	}
	spec := Plan{MaxInst: 2, BudgetP: 2, Window: 3, Initial: 1, Factor: 2, BackoffMax: 4, Quirks: Quirks{NoWantClosure: true},
		Svc: []SvcPlan{{Restart: Always, MaxA: 2, Want: []int{1}}, {Restart: OnFailure, MaxA: 2, Grant: true}}}
	p, err := NewPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	if closureViolation(p) == "" {
		t.Fatal("the mutant NoWantClosure is not red under the static check")
	}
}

// TestGate1FrameF3Goals: goal(C) for C in O is a function of the holders on O.
// Exhaustive: every labelled DAG of 3 services, every closed grant assignment,
// every assignment of the 2 x 3 holder bits.
func TestGate1FrameF3Goals(t *testing.T) {
	plans, cases := 0, 0
	dagPlans(3, func(p *Plan, _ Plan) {
		plans++
		o := omask(p)
		n := len(p.Svc)
		seen := map[uint32]uint32{}
		for h := uint32(0); h < 1<<(2*n); h++ {
			w := p.NewWorld()
			var onO uint32
			for i := 0; i < n; i++ {
				w.Svc[i].H[Owner] = h&(1<<(2*i)) != 0
				w.Svc[i].H[Delegate] = h&(1<<(2*i+1)) != 0
				if o&(1<<i) != 0 {
					onO |= (h >> (2 * i) & 3) << (2 * i)
				}
			}
			cases++
			g := p.Goals(w) & o
			if prev, ok := seen[onO]; ok && prev != g {
				t.Fatalf("goals on O depend on a holder in G: plan %+v, holders %b", p.Svc, h)
			}
			seen[onO] = g
		}
	})
	t.Logf("F3: %d plans, %d (plan, holder assignment) cases", plans, cases)
	if plans == 0 {
		t.Fatal("no plan was checked")
	}
}

// gPart is gamma of a world.
type gPart struct {
	svc  map[int]SvcState
	tx   map[int]Tx
	rp   []int64
	mode Mode
	stat int
	key  string
}

func gOf(p *Plan, w *World) gPart {
	g := gPart{svc: map[int]SvcState{}, tx: map[int]Tx{}, rp: append([]int64(nil), w.RP...), mode: w.Mode, stat: w.Status}
	for i, s := range w.Svc {
		if p.Svc[i].Grant {
			s.R = append([]int64(nil), s.R...)
			g.svc[i] = s
			g.tx[p.TxIndex(Owner, i)] = w.Tx[p.TxIndex(Owner, i)]
			g.tx[p.TxIndex(Delegate, i)] = w.Tx[p.TxIndex(Delegate, i)]
		}
	}
	g.key = fmt.Sprintf("%v|%v|%v|%d.%d", g.svc, g.tx, g.rp, g.mode, g.stat)
	return g
}

func (g gPart) into(w *World) *World {
	c := w.Clone()
	for i, s := range g.svc {
		s.R = append([]int64(nil), s.R...)
		c.Svc[i] = s
	}
	for q, t := range g.tx {
		c.Tx[q] = t
	}
	c.RP = append([]int64(nil), g.rp...)
	return c
}

// frameOutcome is what an input does, as far as pi sees it.
type frameOutcome struct {
	kind  uint8
	class string
	pi    string
	eff   string
}

func stepOutcome(p *Plan, w *World, tok Tok) frameOutcome {
	if tok.K == ageTok {
		nw, ok := age(w, int(tok.A))
		if !ok {
			return frameOutcome{kind: 1, class: ErrInput.Error(), pi: piKey(p, w)}
		}
		return frameOutcome{pi: piKey(p, normalise(p, nw))}
	}
	nw, r, err := p.Step(w, decode(p, w, tok))
	switch {
	case err != nil:
		return frameOutcome{kind: 1, class: errClass(err).Error(), pi: piKey(p, w)}
	case r.Stale:
		return frameOutcome{kind: 2, pi: piKey(p, w)}
	}
	return frameOutcome{pi: piKey(p, normalise(p, nw)), eff: effO(p, r)}
}

// composePool is the N = 2 pool: one service per position, with an edge from
// service 1 to service 0 of every kind, every grant assignment the closure
// allows (both in O, 1 in G, both in G) and service 0 terminal when it is in O.
func composePool(t testing.TB) map[string]*Plan {
	m := map[string]*Plan{}
	for _, edge := range []string{"req", "want", "ord"} {
		for _, assign := range []string{"OO", "GO", "GG"} {
			for _, term := range []bool{false, true} {
				if term && assign == "GG" {
					continue
				}
				s0 := SvcPlan{Restart: OnFailure, MaxA: 1, Grant: assign == "GG", Terminal: term}
				s1 := SvcPlan{Restart: Always, MaxA: 1, Grant: assign != "OO"}
				switch edge {
				case "req":
					s1.Req = []int{0}
				case "want":
					s1.Want = []int{0}
				default:
					s1.Ord = []int{0}
				}
				name := fmt.Sprintf("%s/%s/terminal=%t", edge, assign, term)
				m[name] = newPlan(t, Plan{MaxInst: 1, BudgetP: 1, Svc: []SvcPlan{s0, s1}})
			}
		}
	}
	return m
}

const (
	composeExpand = 6000 // one-copy states expanded per plan to build the pool
	composeWorlds = 400  // worlds perturbed per plan
	composeParts  = 16   // gamma parts put into each
)

// pick returns n elements of s spread evenly.
func pick[T any](s []T, n int) []T {
	if len(s) <= n {
		return s
	}
	out := make([]T, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, s[i*len(s)/n])
	}
	return out
}

// framePerturb checks SC, EC and LR on w against every gamma part in parts:
// the L inputs must give the same outcome, the H and Z inputs must leave pi
// and the trees of O alone. It returns the first violation and the number of
// (world, input) cases.
func framePerturb(p *Plan, toks []Tok, classes []byte, w *World, parts []gPart) (string, int) {
	cases := 0
	base := make([]frameOutcome, len(toks))
	for ti, tok := range toks {
		base[ti] = stepOutcome(p, w, tok)
	}
	pi0 := piKey(p, w)
	for _, g := range parts {
		if g.mode != w.Mode || g.stat != w.Status {
			continue
		}
		v := g.into(w)
		for ti, tok := range toks {
			cases++
			o := stepOutcome(p, v, tok)
			if classes[ti] == 'L' {
				if o != base[ti] {
					return fmt.Sprintf("SC/EC: %+v differs under another gamma: %+v / %+v, pi %s", tok, base[ti], o, pi0), cases
				}
			} else if o.kind != 1 && (o.pi != pi0 || o.eff != "") {
				return fmt.Sprintf("LR: %+v (class %c) changes pi or an O tree under gamma: %+v, pi %s", tok, classes[ti], o, pi0), cases
			}
		}
	}
	return "", cases
}

// frameWorlds expands the one-copy graph of p and returns its worlds.
func frameWorlds(p *Plan) []*World {
	g := buildGraph(p, p.NewWorld(), composeExpand)
	return g.worlds[:min(len(g.worlds), composeExpand)]
}

// frameParts adds to the gamma parts of the worlds the one where an open
// transaction sits on every holder slot of G (F4): the transaction of a G
// holder must write no pi component, so its presence must change nothing.
func frameParts(p *Plan, worlds []*World) []gPart {
	var parts []gPart
	seen := map[string]bool{}
	add := func(g gPart) {
		if !seen[g.key] {
			seen[g.key] = true
			parts = append(parts, g)
		}
	}
	for _, w := range worlds {
		g := gOf(p, w)
		add(g)
		inj := gOf(p, w)
		for i := range p.Svc {
			if !p.Svc[i].Grant {
				continue
			}
			for _, q := range []Principal{Owner, Delegate} {
				ix := p.TxIndex(q, i)
				if !inj.tx[ix].Active {
					inj.tx[ix] = Tx{Active: true, Members: 1 << i, T: 1 << i, Deadline: 100}
				}
			}
		}
		inj.key = fmt.Sprintf("inj%v", inj.tx) + g.key
		add(inj)
	}
	return parts
}

// TestGate1FramePerturbation covers F1, F2, F4, F5 and F6 on the N = 2 pool:
// for every plan, a sample of reachable one-copy worlds has its gamma replaced
// by that of other reachable worlds (and by one with an open transaction on
// every G holder, F4), and SC, EC and LR must hold. A service of G whose fire
// wrote a pi component, an O service whose cell read a G service, a G
// transaction rolling back into O, a G event that escalated or a round
// boundary caused by G that moved pi would each be one of these reds.
func TestGate1FramePerturbation(t *testing.T) {
	total, plans, withG := 0, 0, 0
	for name, p := range composePool(t) {
		toks := abstractTokens(p)
		classes := make([]byte, len(toks))
		for i, tok := range toks {
			classes[i] = tokClass(p, tok)
		}
		worlds := frameWorlds(p)
		parts := pick(frameParts(p, worlds), composeParts*8)
		hasG := false
		for _, s := range p.Svc {
			hasG = hasG || s.Grant
		}
		cases := 0
		for _, w := range pick(worlds, composeWorlds) {
			red, c := framePerturb(p, toks, classes, w, pick(parts, composeParts))
			if red != "" {
				t.Fatalf("%s: %s", name, red)
			}
			cases += c
		}
		if cases == 0 {
			t.Fatalf("%s: no case was checked", name)
		}
		total += cases
		plans++
		if hasG {
			withG++
		}
	}
	t.Logf("perturbation: %d plans (%d with a service in G), %d (world, gamma, input) cases", plans, withG, total)
}

// The mutants of the lemmas stay red under the perturbation: the check sees a
// wrong frame, not only a wrong state.
func TestGate1FramePerturbationWitnesses(t *testing.T) {
	for name, q := range map[string]Quirks{"FreeAcrossCut": {FreeAcrossCut: true}, "GoalNoCut": {GoalNoCut: true}} {
		p := newPlan(t, Plan{Quirks: q, MaxInst: 1, BudgetP: 1, Svc: []SvcPlan{
			{Restart: Always, Terminal: true, MaxA: 1},
			{Restart: OnFailure, MaxA: 1, Req: []int{0}},
			{Restart: OnFailure, MaxA: 1, Grant: true, Req: []int{1}},
		}})
		toks := abstractTokens(p)
		classes := make([]byte, len(toks))
		for i, tok := range toks {
			classes[i] = tokClass(p, tok)
		}
		g := buildGraph(p, p.NewWorld(), 3000)
		worlds := g.worlds[:min(len(g.worlds), 3000)]
		parts := frameParts(p, worlds)
		red := ""
		for _, w := range worlds {
			if red, _ = framePerturb(p, toks, classes, w, pick(parts, 40)); red != "" {
				break
			}
		}
		t.Logf("%s: red=%t %.100s", name, red != "", red)
		if red == "" {
			t.Errorf("%s: the perturbation does not see the defect", name)
		}
	}
}
