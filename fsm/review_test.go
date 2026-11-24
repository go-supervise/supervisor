package fsm

import "testing"

// Named regressions for the counter-examples of REVIEW-3 .. REVIEW-8 that the
// FSM can express. Tests elsewhere carry the item they pin in a comment
// ("Regression:").

// TestREVIEW3_D1_OneshotIsDoneOnlyByItsOwnSuccess: a oneshot killed by
// StartTO, by the loss of a dependency or by a restart is never Done; Done
// needs a natural exit with status ok. Checked over every world the search
// reaches.
func TestREVIEW3_D1_OneshotIsDoneOnlyByItsOwnSuccess(t *testing.T) {
	p := newPlan(t, Plan{Svc: []SvcPlan{
		{Restart: Always, Terminal: true},
		{Kind: Oneshot, Req: []int{0}},
	}})
	doneSeen := false
	explore(t, p, p.NewWorld(), exploreOpts{maxNow: 3, maxNodes: 4000}, func(from *World, tok Tok, to *World, r *Result) {
		if s := to.Svc[1]; s.S == Done {
			doneSeen = true
			if s.Cause != CauseNatural || s.Exit != ExitOK {
				t.Fatalf("oneshot Done with cause %v exit %v after %+v from %s", s.Cause, s.Exit, tok, key(from))
			}
		}
	})
	if !doneSeen {
		t.Fatal("no oneshot reached Done: the property was not exercised")
	}
}

// TestREVIEW6_N1_StopDoesNotWaitForADependentAcrossTheCut: C in O stops while
// A in grant(p), which requires C, still runs; A does not hold C up (I4), so C
// leaves StopWait at once, and A loses C.
func TestREVIEW6_N1_StopDoesNotWaitForADependentAcrossTheCut(t *testing.T) {
	p := fixturePlan(t, Quirks{})
	h := newH(t, p)
	h.start(Owner, 0b011)
	h.ready(0)
	h.ready(1)
	h.do(Input{Kind: InStart, By: Delegate, Targets: 0b100, Now: h.w.Now, Deadline: h.w.Now + 10})
	h.ready(2)
	h.do(Input{Kind: InStop, By: Owner, Targets: 0b011, Now: h.w.Now})
	if h.state(1) != Stopping {
		t.Fatalf("C is %v, want Stopping: a dependent across the cut held it in StopWait", h.state(1))
	}
	if h.state(2) == Ready {
		t.Fatal("A is still Ready although its require-target C stopped")
	}
}

// TestREVIEW4_E1_E2_NoHolderCrossesTheCut: the delegate's holder on A in
// grant(p) never raises the goal of A's targets in O (F3 route 1), and the
// owner's hold on A does not either; the owner's own hold on C does.
func TestREVIEW4_E1_E2_NoHolderCrossesTheCut(t *testing.T) {
	p := fixturePlan(t, Quirks{})
	w := p.NewWorld()
	w.Svc[2].H[Delegate] = true
	if g := p.Goals(w); g != 0b100 {
		t.Fatalf("delegate hold on A: goal %b, want 100", g)
	}
	w.Svc[2].H[Owner] = true
	if g := p.Goals(w); g != 0b100 {
		t.Fatalf("owner hold on A: goal %b, want 100", g)
	}
	w = p.NewWorld()
	w.Svc[1].H[Owner] = true
	if g := p.Goals(w); g != 0b011 {
		t.Fatalf("owner hold on C: goal %b, want 011", g)
	}
}
