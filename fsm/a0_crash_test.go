package fsm

import (
	"errors"
	"testing"
)

// These tests run before every other test of the package (file name order),
// and each turns a crash into an assertion failure: a guard that protects
// against a crash is held by an assertion, and the crash it prevents does not
// end the test binary before another test can say what the guard is for.

func noPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s crashed instead of answering: %v", what, r)
		}
	}()
	f()
}

// Regression: a state with no enabled guard does not fire (and does not
// dereference the cell it did not find).
func TestFireWithNoEnabledGuardIsFalse(t *testing.T) {
	p := newPlan(t, Plan{Svc: []SvcPlan{{Restart: Always}}})
	r := &run{p: p, w: p.NewWorld(), res: &Result{}}
	noPanic(t, "fire", func() {
		if r.fire(0, false) || r.fire(0, true) {
			t.Error("an Inactive service with no goal must not move")
		}
	})
}

// Regression: a Prio outside 1..NumGuards is refused by NewPlan and does not
// crash it.
func TestPlanRefusesAPrioOutsideTheRange(t *testing.T) {
	for _, prio := range []uint8{0, NumGuards + 1, 255} {
		tb := Default()
	rows:
		for s := range tb.Grid {
			for g := Guard(0); g < NumGuards; g++ {
				if tb.Grid[s][g].Reach != Absent {
					tb.Grid[s][g].Prio = prio
					break rows
				}
			}
		}
		noPanic(t, "NewPlan", func() {
			_, err := NewPlan(Plan{MaxInst: 2, BudgetP: 2, Window: 3, Initial: 1, Factor: 2, BackoffMax: 4,
				Svc: []SvcPlan{{Restart: Always, MaxA: 2}}, Tables: tb})
			if !errors.Is(err, ErrPlan) {
				t.Errorf("Prio %d: got %v, want ErrPlan", prio, err)
			}
		})
	}
}

// Regression: String never answers with a crash for a value outside the
// enumeration.
func TestStringsOfInvalidValuesDoNotPanic(t *testing.T) {
	noPanic(t, "State.String", func() {
		if got := State(200).String(); got != "invalid(200)" {
			t.Errorf("State: %q", got)
		}
	})
	noPanic(t, "Reach.String", func() {
		if got := Reach(200).String(); got != "invalid(200)" {
			t.Errorf("Reach: %q", got)
		}
	})
}
