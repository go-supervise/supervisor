package fsm

import (
	"fmt"
	"strings"
	"testing"
)

// Gate 1, Theorem U (draft 17 section 3.8): the unwinding conditions of the
// 2-safety property, checked on the whole state space of one copy.
//
// pi is the low part of a world: the state of every service of O (outside
// every grant), the owner's transactions on O, the mode, the status and
// |R_inst|. With the token classes of highInput (L low, Z low but naming a
// service of G, H high), the conditions are
//
//	OC  obs_O is a function of pi;
//	EC  for an L token, whether it is accepted, refused (and why) or stale is a
//	    function of (pi, token), and so is pi of the target and the effects on
//	    the trees of O (SC: the step consistency);
//	LR  an H or Z token that is accepted or stale leaves pi unchanged and has no
//	    effect on a tree of O (local respect).
//
// Together they imply Gate 1 for every run of every length: a pair of worlds
// with equal pi stays equal in pi under the same L tokens whatever H tokens
// each copy takes. The pair searches of gate1_test.go and gate1_abs_test.go
// are smoke tests; only this theorem and Theorem C (gate1_compose_test.go) are
// Gate 1.

// piKey is pi of a world.
func piKey(p *Plan, w *World) string {
	var b strings.Builder
	fmt.Fprintf(&b, "m%d.%d|i%d", w.Mode, w.Status, len(w.RInst))
	for i, s := range w.Svc {
		if p.Svc[i].Grant {
			continue
		}
		t := w.Tx[p.TxIndex(Owner, i)]
		fmt.Fprintf(&b, "|%d:%d.%d.%d.%d.%t%t%t%t%t.%d.%d.%v/%t.%d.%d.%d.%t", i, s.S, s.Reason, s.Cause, s.Exit, s.Wexp, s.Armed, s.Tree, s.MainUp, s.Spawning, s.Live, len(s.R), s.H, t.Active, t.Out, t.Members, t.T, t.NoRollback)
	}
	return b.String()
}

// effO is the signature of the effects on the trees of O: kind and service,
// no epoch and no generation.
func effO(p *Plan, r *Result) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	for _, e := range r.Effects {
		switch e.Kind {
		case EfSpawn, EfTerm, EfKill:
			if !p.Svc[e.Svc].Grant {
				fmt.Fprintf(&b, "%d.%d,", e.Kind, e.Svc)
			}
		}
	}
	return b.String()
}

// tokClass is the class of a token: 'L', 'Z' or 'H'.
func tokClass(p *Plan, tok Tok) byte {
	var high, exempt bool
	if tok.K == ageTok {
		high = ageHigh(p, int(tok.A))
	} else {
		high, exempt = highInput(p, tok)
	}
	switch {
	case high:
		return 'H'
	case exempt:
		return 'Z'
	}
	return 'L'
}

type unwindSig struct {
	kind  uint8
	class int32
	pi    int32
	eff   string
}

// unwind builds the single-copy graph from init breadth first and checks OC,
// EC/SC and LR on every (state, token) case, stopping at the first red. It
// returns the red case, the number of states, the number of cases and whether
// the frontier was emptied (closed) within limit states.
func unwind(p *Plan, init *World, limit int) (red string, states, cases int, closed bool) {
	toks := abstractTokens(p)
	classes := make([]byte, len(toks))
	for i, tok := range toks {
		classes[i] = tokClass(p, tok)
	}
	ids := map[string]int{}
	var worlds []*World
	intern := func(w *World) int {
		k := abstractKey(w)
		if id, ok := ids[k]; ok {
			return id
		}
		ids[k] = len(worlds)
		worlds = append(worlds, w)
		return len(worlds) - 1
	}
	piIDs := map[string]int32{}
	piID := func(w *World) int32 {
		k := piKey(p, w)
		id, ok := piIDs[k]
		if !ok {
			id = int32(len(piIDs))
			piIDs[k] = id
		}
		return id
	}
	errIDs := map[string]int32{}
	obsOf := map[int32]string{}
	table := map[[2]int32]unwindSig{}
	intern(normalise(p, init.Clone()))
	for id := 0; id < len(worlds); id++ {
		if id >= limit {
			return "", len(worlds), cases, false
		}
		w := worlds[id]
		pi := piID(w)
		o := obsO(p, w)
		if prev, ok := obsOf[pi]; ok && prev != o {
			return fmt.Sprintf("OC: pi %q has two observations:\n %s\n %s", piKey(p, w), prev, o), len(worlds), cases, false
		}
		obsOf[pi] = o
		for ti, tok := range toks {
			cases++
			var nw *World
			var r *Result
			var err error
			stale := false
			if tok.K == ageTok {
				var ok bool
				if nw, ok = age(w, int(tok.A)); !ok {
					err = ErrInput
				}
			} else {
				nw, r, err = p.Step(w, decode(p, w, tok))
				stale = err == nil && r.Stale
			}
			s := unwindSig{pi: pi}
			switch {
			case err != nil:
				k := errClass(err).Error()
				c, ok := errIDs[k]
				if !ok {
					c = int32(len(errIDs))
					errIDs[k] = c
				}
				s.kind, s.class = 1, c
			case stale:
				s.kind = 2
			default:
				nw = normalise(p, nw)
				s.pi = piID(nw)
				s.eff = effO(p, r)
				intern(nw)
			}
			desc := fmt.Sprintf("%+v (class %c) from %s", tok, classes[ti], piKey(p, w))
			if tok.K == ageTok {
				desc = fmt.Sprintf("age(%d) (class %c) from %s", tok.A, classes[ti], piKey(p, w))
			}
			if classes[ti] == 'L' {
				key := [2]int32{pi, int32(ti)}
				if prev, ok := table[key]; ok && prev != s {
					return fmt.Sprintf("EC/SC: two worlds with the same pi answer %s differently: %+v / %+v", desc, prev, s), len(worlds), cases, false
				}
				table[key] = s
			} else if s.kind != 1 && (s.pi != pi || s.eff != "") {
				return fmt.Sprintf("LR: %s changes pi or an O tree (effects %q)", desc, s.eff), len(worlds), cases, false
			}
		}
	}
	return "", len(worlds), cases, true
}

// unwindPlans are the 18 plans of Theorem U at N=1: one service, daemon or
// oneshot, with each restart policy, in a grant, in O, or in O and terminal.
func unwindPlans(t testing.TB) map[string]*Plan {
	m := map[string]*Plan{}
	for _, kind := range []Kind{Daemon, Oneshot} {
		for _, rp := range []RestartOn{Never, OnFailure, Always} {
			for _, pos := range []string{"grant", "O", "O+terminal"} {
				sv := SvcPlan{Kind: kind, Restart: rp, MaxA: 1, Grant: pos == "grant", Terminal: pos == "O+terminal"}
				name := fmt.Sprintf("%v/%v/%s", kind, rp, pos)
				m[name] = newPlan(t, Plan{MaxInst: 1, BudgetP: 1, Svc: []SvcPlan{sv}})
			}
		}
	}
	return m
}

func TestGate1Unwind(t *testing.T) {
	var states, cases int
	for name, p := range unwindPlans(t) {
		red, st, cs, closed := unwind(p, p.NewWorld(), 100000)
		if red != "" {
			t.Fatalf("%s: Theorem U is red: %s", name, red)
		}
		if !closed {
			t.Fatalf("%s: the single-copy graph did not close within 100000 states", name)
		}
		states += st
		cases += cs
	}
	t.Logf("Theorem U, N=1: 18 plans, %d states, %d (state, token) cases, all closed", states, cases)
}

// The witnesses: each defect is red under the unwinding conditions, on the
// part of the graph reached before the first red.
func TestGate1UnwindWitnesses(t *testing.T) {
	for name, tc := range map[string]struct {
		p    *Plan
		init func(*Plan) *World
	}{
		"GoalDraft8":     {fixturePlan(t, Quirks{GoalNoCut: true}), preloaded},
		"FreeAcrossCut":  {fixturePlan(t, Quirks{FreeAcrossCut: true}), preloaded},
		"RetryPayCaller": {newPlan(t, Plan{Quirks: Quirks{RetryPayCaller: true}, Svc: []SvcPlan{{Restart: Always, Terminal: true}, {Restart: OnFailure, Grant: true, MaxA: 1}}}), func(p *Plan) *World { return p.NewWorld() }},
	} {
		red, st, cs, _ := unwind(tc.p, tc.init(tc.p), 400000)
		t.Logf("%s: red=%t states=%d cases=%d: %.120s", name, red != "", st, cs, red)
		if red == "" {
			t.Errorf("%s: the unwinding conditions do not see the defect", name)
		}
	}
}
