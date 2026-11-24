package fsm

import "testing"

// The tests of this file use only Step and the harness's helpers, so each can
// be copied onto an older commit and run there: each is red on d74ceab (the
// commit before draft 16) and green here, which is what makes it a regression
// test and not a description.

// TestEscalationWaitsForTheEndOfTheRound: an escalation raised inside a round
// is applied between rounds (section 2.3, draft 16).
//
// The case: T (terminal) waits on D and fails by WaitTO; X, independent of T,
// has the owner's holder and has not started yet. The round runs with the
// goals it started with, so X starts in the round T fails in (its spawn is
// emitted); the escalation clears the holders afterwards. An escalation applied
// in the middle of the round (d74ceab) clears X's holder before X's turn, so X
// never starts.
func TestEscalationWaitsForTheEndOfTheRound(t *testing.T) {
	p := newPlan(t, Plan{Svc: []SvcPlan{
		{Restart: Never},
		{Restart: Never, Terminal: true, Req: []int{0}},
		{Restart: Always},
	}})
	h := newH(t, p)
	h.start(Owner, 0b010) // T held; D is raised through the require edge
	h.want(1, Waiting)
	h.w.Svc[2].H[Owner] = true // X's holder, with X still Inactive
	h.want(2, Inactive)
	r := h.do(Input{Kind: InEvent, Now: h.w.Now, Svc: 1, Event: WTO, Epoch: h.w.Svc[1].Epoch, Gen: h.w.Svc[1].Gen})
	if h.w.Mode != Quiescing || h.w.Status != 1 {
		t.Fatalf("T's failure did not escalate: mode %v status %d", h.w.Mode, h.w.Status)
	}
	spawned := false
	for _, e := range r.Effects {
		spawned = spawned || e.Kind == EfSpawn && e.Svc == 2
	}
	if !spawned || h.w.Svc[2].Epoch != 1 {
		t.Fatalf("X did not start in the round the escalation was raised in (spawn %v, epoch %d): the escalation was applied mid-round", spawned, h.w.Svc[2].Epoch)
	}
	if h.w.Svc[2].H[Owner] || h.w.Svc[2].S == Starting {
		t.Fatalf("after the boundary X's holder is cleared and X is stopping: %+v", h.w.Svc[2])
	}
}

// TestPhaseOneNeverFiresGFree: a service whose only enabled guard is G_free
// does not move in phase 1 (section 3.5, draft 16), and a service with no
// enabled guard does not move at all.
//
// B, and A with `order` on B, both Ready; the owner stops both. Phase 1 moves
// both to StopWait; phase 2 evaluates G_free in the world phase 1 ended in, where
// A (not B) is free. Every G_free move therefore comes after every other move,
// and B, which A's departure from Ready has not freed in that world, stays in
// StopWait. d74ceab fired G_free inside its sweep, interleaved with the other
// guards, and freed B in the same step.
func TestPhaseOneNeverFiresGFree(t *testing.T) {
	p := newPlan(t, Plan{Svc: []SvcPlan{{Restart: Always}, {Restart: Always, Ord: []int{0}}}})
	h := newH(t, p)
	h.start(Owner, 0b11)
	h.ev(0, Ri)
	h.ev(1, Ri)
	r := h.do(Input{Kind: InStop, Now: h.w.Now, By: Owner, Targets: 0b11})
	free := false
	for _, m := range r.Moves {
		if free && m.Guard != GFree {
			t.Fatalf("a %v move follows a G_free move: %+v", m.Guard, r.Moves)
		}
		free = free || m.Guard == GFree
	}
	if !free {
		t.Fatalf("no G_free move: the test exercises nothing: %+v", r.Moves)
	}
	if h.state(0) != StopWait || h.state(1) != Stopping {
		t.Fatalf("A %v, B %v, want A Stopping and B StopWait", h.state(1), h.state(0))
	}
	// no enabled guard: a tick moves nothing
	before := h.w.Clone()
	r = h.do(Input{Kind: InTick, Now: h.w.Now})
	if len(r.Moves) != 0 || !sameWorld(before, h.w) {
		t.Fatalf("a tick moved services: %+v", r.Moves)
	}
}

// TestEscalationAbortsAFailingTransaction: at a round boundary the escalation
// is applied before the rollback, so a transaction whose member is the failing
// terminal service ends aborted, as every transaction Quiescing clears does,
// and not failed.
func TestEscalationAbortsAFailingTransaction(t *testing.T) {
	p := newPlan(t, Plan{Svc: []SvcPlan{
		{Restart: Never},
		{Restart: Never, Terminal: true, Req: []int{0}},
	}})
	h := newH(t, p)
	h.start(Owner, 0b10)
	h.want(1, Waiting)
	h.do(Input{Kind: InEvent, Now: h.w.Now, Svc: 1, Event: WTO, Epoch: h.w.Svc[1].Epoch, Gen: h.w.Svc[1].Gen})
	if h.w.Mode != Quiescing {
		t.Fatalf("the terminal's failure did not escalate: %v", h.w.Mode)
	}
	if tx := h.w.Tx[p.TxIndex(Owner, 1)]; tx.Active || tx.Out != TxAborted {
		t.Fatalf("the transaction of the failing terminal is %+v, want aborted", tx)
	}
}

// TestASucceededTransactionSurvivesAnEscalationInTheSameRound: the escalation
// aborts only the transactions still open; one whose members are all up in
// the round's fixpoint has succeeded.
func TestASucceededTransactionSurvivesAnEscalationInTheSameRound(t *testing.T) {
	p := newPlan(t, Plan{Svc: []SvcPlan{
		{Restart: Never},
		{Restart: Never, Terminal: true, Req: []int{0}},
		{Restart: Always},
	}})
	h := newH(t, p)
	h.start(Owner, 0b010)
	h.want(1, Waiting)
	// X is Ready under its own holder and its transaction is open
	h.start(Owner, 0b100)
	h.ev(2, Ri)
	h.w.Tx[p.TxIndex(Owner, 2)] = Tx{Active: true, Members: 4, T: 4, Deadline: 100}
	h.do(Input{Kind: InEvent, Now: h.w.Now, Svc: 1, Event: WTO, Epoch: h.w.Svc[1].Epoch, Gen: h.w.Svc[1].Gen})
	if tx := h.w.Tx[p.TxIndex(Owner, 2)]; tx.Active || tx.Out != TxOK {
		t.Fatalf("a transaction that succeeded in the round is %+v, want ok", tx)
	}
	if tx := h.w.Tx[p.TxIndex(Owner, 1)]; tx.Out != TxAborted {
		t.Fatalf("the failing terminal's transaction is %+v, want aborted", tx)
	}
}

// TestARepeatedStartIsAnAPICharge: every write by a delegated principal pays
// R_p once (section 3.3), a repeated start included, and the refusal comes
// before the write takes effect.
func TestARepeatedStartIsAnAPICharge(t *testing.T) {
	p := newPlan(t, Plan{BudgetP: 2, Svc: []SvcPlan{{Restart: Always}, {Restart: OnFailure, Grant: true}}})
	h := newH(t, p)
	start := func(noRollback bool) Input {
		return Input{Kind: InStart, Now: h.w.Now, By: Delegate, Targets: 0b10, Deadline: 100, NoRollback: noRollback}
	}
	h.do(start(false))
	if len(h.w.RP) != 1 {
		t.Fatalf("the first start paid %d, want 1", len(h.w.RP))
	}
	h.do(start(false))
	if len(h.w.RP) != 2 {
		t.Fatalf("a repeated start paid %d in total, want 2", len(h.w.RP))
	}
	// the budget is spent: a repeated start is refused, and its --no-rollback
	// does not mark the transaction
	refuses(t, p, h.w, start(true), ErrPrincipalBudget)
	if h.w.Tx[p.TxIndex(Delegate, 1)].NoRollback {
		t.Fatal("a refused write took effect")
	}
}
