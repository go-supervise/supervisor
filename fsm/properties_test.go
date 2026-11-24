package fsm

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
)

// The breadth-first search is run once and shared by the tests below.
var (
	bigOnce sync.Once
	big     struct {
		st    stats
		bound string
	}
)

func bigRun(t *testing.T) *stats {
	t.Helper()
	bigOnce.Do(func() {
		p := mixedPlan(t)
		big.st = explore(t, p, p.NewWorld(), exploreOpts{maxNow: 3, maxNodes: 12000}, func(from *World, tok Tok, to *World, r *Result) {
			if msg := perServiceViolation(r); msg != "" && big.bound == "" {
				big.bound = fmt.Sprintf("%s (from %s, token %+v)", msg, key(from), tok)
			}
		})
	})
	return &big.st
}

// TestUnreachableCellsAreNeverVisited: explore fails the test when a step
// lands on an Unreachable event cell, a missing fixpoint or a time error;
// every enabled token of every visited world is tried.
func TestUnreachableCellsAreNeverVisited(t *testing.T) {
	if st := bigRun(t); st.steps == 0 {
		t.Fatal("the search took no step")
	}
}

// TestAtMostTwoMovesPerService is section 3.5's bound, asserted at every step
// the search takes.
func TestAtMostTwoMovesPerService(t *testing.T) {
	bigRun(t)
	if big.bound != "" {
		t.Fatal(big.bound)
	}
}

// TestEveryPresentCellIsHit: the search fires every Reachable event cell and
// every present guard cell of the default tables, or the missing cell is
// reported.
func TestEveryPresentCellIsHit(t *testing.T) {
	st := bigRun(t)
	tb := Default()
	hits := map[cellID][]Tok{}
	for c, tr := range st.hit {
		hits[c] = tr
	}
	for c, tr := range seedHits(t) {
		if _, ok := hits[c]; !ok {
			hits[c] = tr
		}
	}
	// Retry and Restart of a Leaked service are refused by the API before the
	// cell is read (ErrLeaked, I6); the refusal is asserted here, and the two
	// cells count as exercised.
	p := mixedPlan(t)
	w := p.NewWorld()
	w.Svc[0].S = Leaked
	for _, k := range []InKind{InRetry, InRestart} {
		for _, by := range []Principal{Owner, Delegate} {
			svc := 0
			if by == Delegate {
				svc = 2
				w.Svc[2].S = Leaked
			}
			refuses(t, p, w, Input{Kind: k, By: by, Svc: svc}, ErrLeaked)
		}
	}
	hits[cellID{false, Leaked, int(Ry)}], hits[cellID{false, Leaked, int(Rs)}] = nil, nil
	for s := range tb.Event {
		for e, c := range tb.Event[s] {
			if _, ok := hits[cellID{false, State(s), e}]; c.Reach == Reachable && !ok {
				t.Errorf("event cell %v x %v was never hit", State(s), Event(e))
			}
		}
	}
	for s := range tb.Grid {
		for g, c := range tb.Grid[s] {
			if _, ok := hits[cellID{true, State(s), g}]; c.Reach == Reachable && !ok {
				t.Errorf("guard cell %v x %v was never hit", State(s), Guard(g))
			}
		}
	}
}

// seedHits runs the sequences of the cells the breadth-first search does not
// reach within its node budget, on plans that make them reachable.
func seedHits(t *testing.T) map[cellID][]Tok {
	t.Helper()
	out := map[cellID][]Tok{}
	for _, p := range []*Plan{fixturePlan(t, Quirks{}), mixedPlan(t)} {
		st := explore(t, p, p.NewWorld(), exploreOpts{maxNow: 3, maxNodes: 6000}, nil)
		for c, tr := range st.hit {
			if _, ok := out[c]; !ok {
				out[c] = tr
			}
		}
	}
	return out
}

// TestRandomSequences drives random token sequences: every step keeps the
// invariants, never mutates its input world, stays within the move cap, and
// never lands on an unreachable cell.
func TestRandomSequences(t *testing.T) {
	plans := []*Plan{mixedPlan(t), fixturePlan(t, Quirks{})}
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 400; round++ {
		p := plans[round%len(plans)]
		h := newH(t, p)
		for i := 0; i < 60; i++ {
			tok := Tok{byte(rng.Intn(256)), byte(rng.Intn(256)), byte(rng.Intn(256)), byte(rng.Intn(256))}
			in := decode(p, h.w, tok)
			before := h.w.Clone()
			w, r, err := p.Step(h.w, in)
			if !sameWorld(before, h.w) {
				t.Fatalf("Step changed its input world on %+v", tok)
			}
			switch err {
			case nil:
			case ErrUnreachable, ErrNoFixpoint, ErrTime:
				t.Fatalf("round %d step %d: %v on %+v from %s", round, i, err, tok, key(h.w))
			default:
				continue
			}
			if len(r.Moves) > p.moveCap() {
				t.Fatalf("%d moves exceed the cap", len(r.Moves))
			}
			if msg := perServiceViolation(r); msg != "" {
				t.Fatal(msg)
			}
			checkInvariants(t, p, w)
			h.w = w
		}
	}
}

// TestMeasuredMaximumMoves records the largest number of guard moves one step
// took in the search: a change that raises it must update the golden file in
// review (section 3.5, R3-L8).
func TestMeasuredMaximumMoves(t *testing.T) {
	golden(t, "moves.golden", fmt.Sprintf("max moves per step: %d\n", bigRun(t).maxMoves))
}
