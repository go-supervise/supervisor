package fsm

import (
	"errors"
	"testing"
)

// refuses runs one step and asserts its error. A refusal that is missing may
// crash the step instead (an index out of range): the recover turns that into
// an assertion, so the test says what the guard is for.
func refuses(t *testing.T, p *Plan, w *World, in Input, want error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%+v panicked (%v), want %v", in, r, want)
		}
	}()
	if _, _, err := p.Step(w, in); !errors.Is(err, want) {
		t.Errorf("%+v: got %v, want %v", in, err, want)
	}
}

func TestInputsOutsideThePlanAreRefused(t *testing.T) {
	p := mixedPlan(t)
	w := p.NewWorld()
	for _, svc := range []int{-1, 3} {
		refuses(t, p, w, Input{Kind: InEvent, Svc: svc, Event: Ri}, ErrInput)
		refuses(t, p, w, Input{Kind: InRetry, Svc: svc}, ErrInput)
		refuses(t, p, w, Input{Kind: InRestart, Svc: svc}, ErrInput)
	}
	for _, ev := range []Event{Ry, Rs, NumEvents} {
		refuses(t, p, w, Input{Kind: InEvent, Svc: 0, Event: ev}, ErrInput)
	}
	refuses(t, p, w, Input{Kind: InDeadline, Slot: 3}, ErrInput)
	refuses(t, p, w, Input{Kind: InDeadline, Slot: -1}, ErrInput)
	refuses(t, p, w, Input{Kind: InKind(0)}, ErrInput)
	refuses(t, p, w, Input{Kind: InDeadline + 1}, ErrInput)
}

func TestPlanEdgesOutsideThePlanAreRefused(t *testing.T) {
	for _, e := range []int{-1, 1, 0} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("edge to %d panicked: %v", e, r)
				}
			}()
			_, err := NewPlan(Plan{MaxInst: 1, BudgetP: 1, Window: 1, Initial: 1, Factor: 1, BackoffMax: 1,
				Svc: []SvcPlan{{MaxA: 1, Req: []int{e}}}})
			if !errors.Is(err, ErrPlan) {
				t.Errorf("edge to %d: got %v, want ErrPlan", e, err)
			}
		}()
	}
}

func TestDelegatedStartPaysTheBudget(t *testing.T) {
	p := newPlan(t, Plan{BudgetP: 1, Svc: []SvcPlan{{Restart: Always}, {Restart: OnFailure, Grant: true}}})
	h := newH(t, p)
	h.do(Input{Kind: InStart, By: Delegate, Targets: 2, Deadline: 1})
	h.do(Input{Kind: InTick, Now: 1})
	h.do(Input{Kind: InDeadline, Slot: int(Delegate), Now: 1})
	refuses(t, p, h.w, Input{Kind: InStart, By: Delegate, Targets: 2, Deadline: 5, Now: 1}, ErrPrincipalBudget)
}

// TestOwnerTransactionOnAGrantDoesNotBlockO is the finding of Gate 1 against
// "one transaction per principal": an owner transaction on grant(p) ends when
// p's service settles, so it must not decide whether the owner's next start
// on O is accepted (section 2.4: one open transaction per holder).
func TestOwnerTransactionOnAGrantDoesNotBlockO(t *testing.T) {
	p := mixedPlan(t)
	h := newH(t, p)
	h.start(Owner, 0b100) // S2 is in grant(p)
	if !h.w.Tx[OwnerInGrant].Active || h.w.Tx[Owner].Active {
		t.Fatalf("the grant transaction is in slot %d: %+v", OwnerInGrant, h.w.Tx)
	}
	h.start(Owner, 0b001)
	refuses(t, p, h.w, Input{Kind: InStart, By: Owner, Targets: 0b100, Deadline: 20}, ErrTxBusy)
	refuses(t, p, h.w, Input{Kind: InStart, By: Owner, Targets: 0b101, Deadline: 20}, ErrInput)
}

// Regression: REVIEW-4 E1 (the goal across the cut) and REVIEW-5 F3 route 1.
func TestGoalStopsAtTheCutAndCrossesAnUncutEdge(t *testing.T) {
	cut := newPlan(t, Plan{Svc: []SvcPlan{{Restart: Always}, {Restart: OnFailure, Grant: true, Req: []int{0}}}})
	w := cut.NewWorld()
	w.Svc[1].H[Delegate] = true
	if g := cut.Goals(w); g != 2 {
		t.Fatalf("goal across a cut edge = %b, want 10", g)
	}
	open := newPlan(t, Plan{Svc: []SvcPlan{{Restart: Always}, {Restart: OnFailure, Req: []int{0}}}})
	w = open.NewWorld()
	w.Svc[1].H[Owner] = true
	if g := open.Goals(w); g != 3 {
		t.Fatalf("goal across an uncut edge = %b, want 11", g)
	}
}
