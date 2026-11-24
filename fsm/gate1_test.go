package fsm

import (
	"fmt"
	"strings"
	"testing"
)

// Gate 1 of section 3.8: Theorem G item 1 as a 2-safety property, checked by
// self-composition. Two copies of one plan run side by side. A LOW input (the
// owner's operations, O's own events, time, SIGTERM) goes to both copies and
// must be enabled in both or neither. A HIGH input (the principal p's
// operations, G's events, p's request deadline) goes to copy 1, copy 2 or
// both. After every step obs_O must be equal.

// highInput says whether a token is an input p controls. exempt says that a
// low token names a service of G, whose outcome is G's (section 3.3, item 1)
// and is outside obs_O, so a refusal on one copy only is not a divergence.
func highInput(p *Plan, tok Tok) (high, exempt bool) {
	n := len(p.Svc)
	grant := func(i int) bool { return p.Svc[i].Grant }
	var gmask uint32
	for i := range p.Svc {
		if grant(i) {
			gmask |= 1 << i
		}
	}
	switch tok.K % numKinds {
	case kEvent:
		return grant(int(tok.A) % n), false
	case kRetry, kRestart:
		if Principal(tok.B&1) == Delegate {
			return true, false
		}
		return false, grant(int(tok.A) % n)
	case kStart, kStop:
		if Principal(tok.B&1) == Delegate {
			return true, false
		}
		return false, uint32(tok.A)&(1<<n-1)&gmask != 0
	case kDeadline:
		// the delegate's slot is p's; the owner's slot on a grant is G's
		// too, and its outcome is outside obs_O
		return tok.B%3 == byte(Delegate), tok.B%3 == OwnerInGrant
	}
	return false, false
}

// obsO is obs_O of a world: for every service outside every grant, its state,
// goal, cause, exit, wexp, armed, reason, |R|, tree bit, main-process bit,
// spawn bit and pending timers; with mode, |R_inst| and the owner's
// transaction outcome when its members lie in O.
func obsO(p *Plan, w *World) string {
	var b strings.Builder
	goal := p.Goals(w)
	for i, s := range w.Svc {
		if p.Svc[i].Grant {
			continue
		}
		fmt.Fprintf(&b, "%d:%d.%t.%d.%d.%t.%t.%d.%d.%t.%t.%t.%d|", i, s.S, goal&(1<<i) != 0, s.Cause, s.Exit, s.Wexp, s.Armed, s.Reason, p.RCount(w, i), s.Tree, s.MainUp, s.Spawning, s.Live)
	}
	fmt.Fprintf(&b, "m%d.%d|inst%d", w.Mode, w.Status, p.InstCount(w))
	fmt.Fprintf(&b, "|tx%t.%d", w.Tx[Owner].Active, w.Tx[Owner].Out)
	return b.String()
}

// gate1MaxNow bounds abstract time: the search stops ticking at this instant,
// so the pair space is finite.
const gate1MaxNow = 2

// gate1Nodes bounds the pairs a green run expands. The pair space of even
// three services is far beyond exhaustive search (300000 pairs left the
// frontier open), so the green run is a bounded breadth-first search, which
// finds every divergence reachable in the fewest steps first; the witnesses
// below are the evidence that the check can see the defects it is for.
const gate1Nodes = 3000

// gate1 explores the pairs of worlds and returns the first red pair, "" when
// every pair reached is green, and whether the search was exhaustive.
func gate1(t testing.TB, p *Plan, init *World, maxNodes int) (red string, exhaustive bool) {
	t.Helper()
	toks := allTokens(p)
	type pair struct{ a, b *World }
	seen := map[string]bool{}
	pkey := func(a, b *World) string { return key(a) + "##" + key(b) }
	queue := []pair{{init, init.Clone()}}
	seen[pkey(init, queue[0].b)] = true
	exhaustive = true
	push := func(a, b *World) {
		k := pkey(a, b)
		if seen[k] {
			return
		}
		if len(seen) >= maxNodes {
			exhaustive = false
			return
		}
		seen[k] = true
		queue = append(queue, pair{a, b})
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, tok := range toks {
			if tok.K == kTick && cur.a.Now >= gate1MaxNow {
				continue
			}
			high, exempt := highInput(p, tok)
			in1, in2 := decode(p, cur.a, tok), decode(p, cur.b, tok)
			w1, r1, e1 := p.Step(cur.a, in1)
			w2, r2, e2 := p.Step(cur.b, in2)
			if !high {
				if !exempt {
					if e1 != e2 {
						return fmt.Sprintf("low %+v enabled in one copy only: %v / %v, from %s", tok, e1, e2, key(cur.a)), exhaustive
					}
					if e1 == nil && r1.Stale != r2.Stale {
						return fmt.Sprintf("low %+v stale in one copy only, from %s", tok, key(cur.a)), exhaustive
					}
				}
				n1, n2 := cur.a, cur.b
				if e1 == nil {
					n1 = w1
				}
				if e2 == nil {
					n2 = w2
				}
				if e1 != nil && e2 != nil {
					continue
				}
				if o1, o2 := obsO(p, n1), obsO(p, n2); o1 != o2 {
					return fmt.Sprintf("low %+v: obs_O differs from %s\n copy 1 %s\n copy 2 %s", tok, key(cur.a), o1, o2), exhaustive
				}
				push(n1, n2)
				continue
			}
			// a high input: copy 1 only, copy 2 only, or both
			for _, c := range [3]struct {
				one, two bool
			}{{true, false}, {false, true}, {true, true}} {
				n1, n2 := cur.a, cur.b
				if c.one && e1 == nil {
					n1 = w1
				} else if c.one {
					continue
				}
				if c.two && e2 == nil {
					n2 = w2
				} else if c.two {
					continue
				}
				if o1, o2 := obsO(p, n1), obsO(p, n2); o1 != o2 {
					return fmt.Sprintf("high %+v (copy1=%t copy2=%t): obs_O differs from %s\n copy 1 %s\n copy 2 %s", tok, c.one, c.two, key(cur.a), o1, o2), exhaustive
				}
				push(n1, n2)
			}
		}
	}
	return "", exhaustive
}

// fixturePlan is REVIEW-7 N12's fixture: B(0) and C(1) in O, C requires B, and
// A(2) in grant(p) requires C.
func fixturePlan(t testing.TB, q Quirks) *Plan {
	return newPlan(t, Plan{Quirks: q, Svc: []SvcPlan{
		{Restart: Always, Terminal: true},
		{Restart: OnFailure, Req: []int{0}},
		{Restart: OnFailure, Grant: true, Req: []int{1}},
	}})
}

// preloaded is a world with |R_inst| = max - 1 at time 0.
func preloaded(p *Plan) *World {
	w := p.NewWorld()
	w.RInst = make([]int64, p.MaxInst-1)
	return w
}

func TestGate1Green(t *testing.T) {
	for name, p := range map[string]*Plan{
		"fixture": fixturePlan(t, Quirks{}),
		"mixed":   mixedPlan(t),
	} {
		red, exhaustive := gate1(t, p, preloaded(p), gate1Nodes)
		if red != "" {
			t.Fatalf("%s: Gate 1 is red on draft 14: %s", name, red)
		}
		t.Logf("%s: green, exhaustive=%t", name, exhaustive)
	}
}

func TestGate1WitnessGoalDraft8(t *testing.T) {
	p := fixturePlan(t, Quirks{GoalNoCut: true})
	if red, _ := gate1(t, p, preloaded(p), 6000); red == "" {
		t.Fatal("Gate 1 is green on draft 8's goal formula (REVIEW-5 F3 route 1)")
	}
}

func TestGate1WitnessNoWantClosure(t *testing.T) {
	plan := Plan{Svc: []SvcPlan{
		{Restart: Always, Want: []int{1}},
		{Restart: OnFailure, Grant: true},
	}}
	plan.MaxInst, plan.BudgetP, plan.Window, plan.Initial, plan.Factor, plan.BackoffMax = 2, 2, 3, 1, 2, 4
	for i := range plan.Svc {
		plan.Svc[i].MaxA = 2
	}
	if _, err := NewPlan(plan); err == nil || !strings.Contains(err.Error(), ErrGrantNotClosed.Error()) {
		t.Fatalf("a want edge into a grant was accepted: %v", err)
	}
	plan.Quirks.NoWantClosure = true
	p, err := NewPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if red, _ := gate1(t, p, p.NewWorld(), 6000); red == "" {
		t.Fatal("Gate 1 is green when grants are not closed under want edges")
	}
}

func TestGate1WitnessRetryPayCaller(t *testing.T) {
	q := Quirks{RetryPayCaller: true}
	p := newPlan(t, Plan{Quirks: q, Svc: []SvcPlan{
		{Restart: Always, Terminal: true},
		{Restart: OnFailure, Grant: true, MaxA: 1},
	}})
	if red, _ := gate1(t, p, p.NewWorld(), 20000); red == "" {
		t.Fatal("Gate 1 is green on the Retry cell pay(caller) (section 3.3, G1)")
	}
}

// Regression: REVIEW-7 N12 (the corrected fixture) and REVIEW-6 N1.
func TestGate1WitnessFreeAcrossCut(t *testing.T) {
	p := fixturePlan(t, Quirks{FreeAcrossCut: true})
	if red, _ := gate1(t, p, preloaded(p), 20000); red == "" {
		t.Fatal("Gate 1 is green when G_free reads a dependent across a cut edge (REVIEW-7 N12)")
	}
}

// TestSerialisationReadSets is the static check of O5': a timer cell is
// delivered after the sources of everything it reads are drained.
func TestSerialisationReadSets(t *testing.T) {
	const beyondSelf = RReq | RTargets | RDependents
	for e := Event(0); e < NumEvents; e++ {
		if !EventTimer[e].Has || e == Fa {
			continue
		}
		if need := EventReads[e] &^ RGoal; TimerDrain[e]&need != need {
			t.Errorf("%v reads %b but drains only %b", e, need, TimerDrain[e])
		}
	}
	for e := Event(0); e < NumEvents; e++ {
		if TimerDrain[e] != 0 && !EventTimer[e].Has {
			t.Errorf("%v drains but is not a timer event", e)
		}
		if EventReads[e]&RSelf == 0 {
			t.Errorf("%v does not read its own service", e)
		}
	}
	// No event reads a dependent: only G_free, a guard, does.
	for e := Event(0); e < NumEvents; e++ {
		if EventReads[e]&(RDependents|RTargets) != 0 {
			t.Errorf("%v reads %b: an event cell reads C and req(C) only", e, EventReads[e]&beyondSelf)
		}
	}
}

// TestGuardsReadWhatTheTableSays perturbs, for every guard of every service,
// the state of each service outside the guard's read set; the guard must not
// change. It ties GuardReads to the evaluator.
func TestGuardsReadWhatTheTableSays(t *testing.T) {
	p := fixturePlan(t, Quirks{})
	reads := func(i int, r Reads) map[int]bool {
		set := map[int]bool{i: true}
		sv := p.Svc[i]
		if r&RReq != 0 {
			for _, b := range sv.Req {
				set[b] = true
			}
		}
		if r&RTargets != 0 {
			for _, list := range [][]int{sv.Req, sv.Want, sv.Ord} {
				for _, b := range list {
					set[b] = true
				}
			}
		}
		if r&RDependents != 0 {
			for a := range p.Svc {
				for _, list := range [][]int{p.Svc[a].Req, p.Svc[a].Want, p.Svc[a].Ord} {
					for _, b := range list {
						if b == i && !p.cut(a, b) {
							set[a] = true
						}
					}
				}
			}
		}
		return set
	}
	n := len(p.Svc)
	for goal := uint32(0); goal < 1<<n; goal++ {
		for i := 0; i < n; i++ {
			for g := Guard(0); g < NumGuards; g++ {
				set := reads(i, GuardReads[g])
				base := p.NewWorld()
				for _, s := range [...]State{Ready, Waiting} {
					for j := range base.Svc {
						base.Svc[j].S = s
					}
					for j := 0; j < n; j++ {
						if set[j] {
							continue
						}
						want := p.Guard(base, goal, i, g)
						for s2 := State(0); s2 < NumStates; s2++ {
							w := base.Clone()
							w.Svc[j].S = s2
							if got := p.Guard(w, goal, i, g); got != want {
								t.Fatalf("guard %v of service %d reads service %d (state %v), outside %b", g, i, j, s2, GuardReads[g])
							}
						}
					}
				}
			}
		}
	}
}
