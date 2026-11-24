package fsm

import (
	"errors"
	"reflect"
	"testing"
)

// refuses runs one step and asserts its error. A refusal that is missing may
// crash the step instead (an index out of range): the recover turns that into
// an assertion, so the test says what the guard is for.
func refuses(t *testing.T, p *Plan, w *World, in Input, want error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%+v: got a crash (%v), want %v", in, r, want)
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
	refuses(t, p, w, Input{Kind: InDeadline, Svc: 3}, ErrInput)
	refuses(t, p, w, Input{Kind: InDeadline, Svc: -1}, ErrInput)
	refuses(t, p, w, Input{Kind: InDeadline, By: 2}, ErrInput)
	refuses(t, p, &World{}, Input{Kind: InTick}, ErrInput)
	short := p.NewWorld()
	short.Tx = short.Tx[:3]
	refuses(t, p, short, Input{Kind: InTick}, ErrInput)
	refuses(t, p, w, Input{Kind: InKind(0)}, ErrInput)
	refuses(t, p, w, Input{Kind: InDeadline + 1}, ErrInput)
}

func TestPlanEdgesOutsideThePlanAreRefused(t *testing.T) {
	for _, e := range []int{-1, 1, 0} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("edge to %d: got a crash (%v), want ErrPlan", e, r)
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
	h.do(Input{Kind: InDeadline, By: Delegate, Svc: 1, Now: 1})
	refuses(t, p, h.w, Input{Kind: InStart, By: Delegate, Targets: 2, Deadline: 5, Now: 1}, ErrPrincipalBudget)
}

// TestTransactionsArePerHolder is draft 16, section 2.3: the transaction is
// indexed by the holder h(q, X). The same principal starting another service
// is not refused, and a repeated start is not an error and opens nothing.
func TestTransactionsArePerHolder(t *testing.T) {
	p := mixedPlan(t)
	h := newH(t, p)
	h.start(Owner, 0b100) // S2 is in grant(p)
	if !h.w.Tx[p.TxIndex(Owner, 2)].Active {
		t.Fatalf("no transaction at h(owner, 2): %+v", h.w.Tx)
	}
	h.start(Owner, 0b001)
	if !h.w.Tx[p.TxIndex(Owner, 0)].Active || !h.w.Tx[p.TxIndex(Owner, 2)].Active {
		t.Fatalf("one transaction per holder, both open: %+v", h.w.Tx)
	}
	refuses(t, p, h.w, Input{Kind: InStart, By: Owner, Targets: 0b101, Deadline: 20}, ErrInput)
	before := h.w.Clone()
	w, _, err := p.Step(h.w, Input{Kind: InStart, By: Owner, Targets: 0b001, Deadline: 99})
	if err != nil || !reflect.DeepEqual(w.Tx, before.Tx) {
		t.Fatalf("a repeated start changed the transactions or failed: %v %+v", err, w.Tx)
	}
}

// TestRepeatedStartOpensNoEmptyTransaction: an existing holder whose
// transaction already ended must not open a new, empty one that succeeds at
// once and writes an outcome nobody asked for.
func TestRepeatedStartOpensNoEmptyTransaction(t *testing.T) {
	p := newPlan(t, Plan{Svc: []SvcPlan{{Restart: Always}}})
	h := newH(t, p)
	h.start(Owner, 1)
	h.w.Tx[0] = Tx{Out: TxFailed}
	w, _, err := p.Step(h.w, Input{Kind: InStart, By: Owner, Targets: 1, Deadline: 5})
	if err != nil || w.Tx[0] != (Tx{Out: TxFailed}) {
		t.Fatalf("a repeated start by a holder reopened a transaction: %v %+v", err, w.Tx[0])
	}
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
