package fsm

import (
	"errors"
	"testing"
)

// Regression: a Leaked member fails the transaction, as a Failed one does.
func TestLeakedMemberFailsTheTransaction(t *testing.T) {
	p := newPlan(t, Plan{Svc: []SvcPlan{{Restart: Always}}})
	h := newH(t, p)
	h.start(Owner, 1)
	h.w.Svc[0].S, h.w.Svc[0].MainUp, h.w.Svc[0].Live = Leaked, false, 0
	h.tick(1)
	tx := h.w.Tx[p.TxIndex(Owner, 0)]
	if tx.Active || tx.Out != TxFailed {
		t.Fatalf("a Leaked member must fail the transaction: %+v", tx)
	}
}

// Regression: an escalation raised inside a round is applied between rounds,
// so the outcome does not depend on where the escalating service sits in the
// evaluation order.
func TestEscalationWaitsForTheEndOfTheRound(t *testing.T) {
	// Independent services, so every order of them is topological. One is
	// Terminal and one Restart-Always with a tight budget: both raise an
	// escalation inside a round.
	p := newPlan(t, Plan{MaxInst: 1, Svc: []SvcPlan{
		{Restart: Always},
		{Restart: Never, Terminal: true},
		{Restart: Always},
	}})
	orders := [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	quiesced := false
	explore(t, p, p.NewWorld(), exploreOpts{maxNow: 1, maxNodes: 600}, func(from *World, tok Tok, to *World, r *Result) {
		in := decode(p, from, tok)
		for _, o := range orders {
			for _, rev := range []bool{false, true} {
				w, _, err := p.step(from, in, o, rev)
				if err != nil || key(w) != key(to) {
					t.Fatalf("order %v rev %t disagrees with Step on %+v from %s", o, rev, tok, key(from))
				}
			}
		}
		quiesced = quiesced || (to.Mode == Quiescing && from.Mode == Running)
	})
	if !quiesced {
		t.Fatal("the search never escalated: the test exercises nothing")
	}
}

// Regression: the grid is validated by NewPlan.
func TestPlanRefusesAMalformedGrid(t *testing.T) {
	base := func() Plan {
		return Plan{MaxInst: 2, BudgetP: 2, Window: 3, Initial: 1, Factor: 2, BackoffMax: 4,
			Svc: []SvcPlan{{Restart: Always, MaxA: 2}}, Tables: Default()}
	}
	present := func(p *Plan) (s int, gs []Guard) {
		for s = range p.Tables.Grid {
			gs = gs[:0]
			for g := Guard(0); g < NumGuards; g++ {
				if p.Tables.Grid[s][g].Reach != Absent {
					gs = append(gs, g)
				}
			}
			if len(gs) >= 2 {
				return s, gs
			}
		}
		t.Fatal("no row with two guards")
		return 0, nil
	}
	cases := map[string]func(p *Plan){
		"prio 0":    func(p *Plan) { s, gs := present(p); p.Tables.Grid[s][gs[0]].Prio = 0 },
		"prio 9":    func(p *Plan) { s, gs := present(p); p.Tables.Grid[s][gs[0]].Prio = NumGuards + 1 },
		"dup prio":  func(p *Plan) { s, gs := present(p); p.Tables.Grid[s][gs[1]].Prio = p.Tables.Grid[s][gs[0]].Prio },
		"reach 0":   func(p *Plan) { s, gs := present(p); p.Tables.Grid[s][gs[0]].Reach = ReachInvalid },
		"reach big": func(p *Plan) { s, gs := present(p); p.Tables.Grid[s][gs[0]].Reach = Absent + 1 },
		"event":     func(p *Plan) { p.Tables.Event[Waiting][0].Reach = Absent + 3 },
	}
	for name, mutate := range cases {
		p := base()
		mutate(&p)
		noPanic(t, name, func() {
			if _, err := NewPlan(p); !errors.Is(err, ErrPlan) {
				t.Errorf("%s: got %v, want ErrPlan", name, err)
			}
		})
	}
	if _, err := NewPlan(base()); err != nil {
		t.Fatalf("the default grid is well formed: %v", err)
	}
}
