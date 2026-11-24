package fsm

import (
	"fmt"
	"sort"
	"testing"
)

type cellID struct {
	grid bool
	s    State
	k    int // Event or Guard
}

type exploreOpts struct {
	maxNow   int64
	maxNodes int
	maxDepth int
}

type stats struct {
	nodes     int
	steps     int
	maxMoves  int
	hit       map[cellID][]Tok // shortest trace to the first firing of each cell
	exhausted bool             // every node was expanded
}

type node struct {
	w     *World
	trace []Tok
}

// explore is a breadth-first search of the worlds reachable from the initial
// one by enabled tokens. For every enabled step it calls visit.
func explore(t testing.TB, p *Plan, init *World, o exploreOpts, visit func(from *World, tok Tok, to *World, r *Result)) stats {
	t.Helper()
	st := stats{hit: map[cellID][]Tok{}, exhausted: true}
	seen := map[string]bool{key(init): true}
	queue := []node{{w: init}}
	toks := allTokens(p)
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		st.nodes++
		checkInvariants(t, p, n.w)
		if o.maxDepth > 0 && len(n.trace) >= o.maxDepth {
			st.exhausted = false
			continue
		}
		for _, tok := range toks {
			if tok.K == kTick && n.w.Now >= o.maxNow {
				continue
			}
			in := decode(p, n.w, tok)
			w, r, err := p.Step(n.w, in)
			if err == ErrUnreachable || err == ErrNoFixpoint || err == ErrTime {
				t.Fatalf("%v reached from %s by %+v: a cell marked unreachable was visited", err, key(n.w), tok)
			}
			if err != nil || r.Stale {
				continue
			}
			st.steps++
			if len(r.Moves) > st.maxMoves {
				st.maxMoves = len(r.Moves)
			}
			trace := append(append([]Tok(nil), n.trace...), tok)
			recordHits(&st, n.w, in, r, trace)
			if visit != nil {
				visit(n.w, tok, w, r)
			}
			if k := key(w); !seen[k] {
				if len(seen) >= o.maxNodes {
					st.exhausted = false
					continue
				}
				seen[k] = true
				queue = append(queue, node{w: w, trace: trace})
			}
		}
	}
	return st
}

func recordHits(st *stats, from *World, in Input, r *Result, trace []Tok) {
	mark := func(c cellID) {
		if _, ok := st.hit[c]; !ok {
			st.hit[c] = trace
		}
	}
	switch in.Kind {
	case InEvent:
		mark(cellID{false, from.Svc[in.Svc].S, int(in.Event)})
	case InRetry:
		mark(cellID{false, from.Svc[in.Svc].S, int(Ry)})
	case InRestart:
		mark(cellID{false, from.Svc[in.Svc].S, int(Rs)})
	}
	for _, m := range r.Moves {
		mark(cellID{true, m.From, int(m.Guard)})
	}
}

// checkInvariants states what must hold of every reachable world.
func checkInvariants(t testing.TB, p *Plan, w *World) {
	t.Helper()
	live := [NumStates]uint8{
		Waiting:  1 << TWait,
		Starting: 1 << TStart,
		Ready:    1 << TLiveness,
		StopWait: 1 << TSW,
		Stopping: 1 << TStop,
		Killing:  1 << TKill,
		Backoff:  1 << TBackoff,
	}
	for i, s := range w.Svc {
		fail := func(msg string) {
			t.Helper()
			t.Fatalf("invariant %s: service %d %+v in %s", msg, i, s, key(w))
		}
		running := s.S == Starting || s.S == Ready || s.S == StopWait || s.S == Stopping || s.S == Killing || s.S == Leaked
		if s.Tree != running {
			fail("the tree exists exactly in the running states")
		}
		if s.MainUp && !s.Tree || s.Spawning && !s.MainUp {
			fail("a main process and a spawning one need their tree")
		}
		if s.S == Starting && !s.MainUp && !s.Spawning {
			fail("Starting has a live main process or a pending spawn")
		}
		if s.S == Ready && !s.MainUp || s.S == Ready && s.Spawning {
			fail("Ready has an exec'd, live main process")
		}
		if s.Live&^live[s.S] != 0 {
			fail("only the state's own timer is pending")
		}
		if s.S != Waiting && (s.Wexp || s.Armed) {
			fail("wexp and armed are Waiting's")
		}
		if (s.S == Failed) != (s.Reason != ReasonNone) {
			fail("a reason exists exactly in Failed")
		}
		if w.Mode == Quiescing && (s.H[Owner] || s.H[Delegate]) {
			fail("Quiescing clears every holder")
		}
		if w.Mode == Running && w.Status != 0 {
			fail("a status needs Quiescing")
		}
	}
	for q, tx := range w.Tx {
		if tx.Active && w.Mode == Quiescing {
			t.Fatalf("an active transaction %d while quiescing", q)
		}
	}
	r := &run{p: p, w: w.Clone(), res: &Result{}}
	for i := range w.Svc {
		if r.fire(i, false) || r.fire(i, true) {
			t.Fatalf("propagation is not at a fixpoint: service %d can still move in %s", i, key(w))
		}
	}
}

// perServiceBound checks section 3.5: with fixed inputs a service moves at
// most twice, self-moves included.
func perServiceBound(t testing.TB, r *Result) {
	t.Helper()
	if msg := perServiceViolation(r); msg != "" {
		t.Fatal(msg)
	}
}

// perServiceViolation is "" when no service moved more than twice with fixed
// inputs, and says which did otherwise.
func perServiceViolation(r *Result) string {
	type run struct {
		sig   uint8
		count int
	}
	last := map[int]run{}
	for _, m := range r.Moves {
		l := last[m.Svc]
		if l.count > 0 && l.sig == m.Sig {
			l.count++
		} else {
			l = run{m.Sig, 1}
		}
		last[m.Svc] = l
		if l.count > 2 {
			return fmt.Sprintf("service %d moved %d times with fixed inputs: %+v", m.Svc, l.count, r.Moves)
		}
	}
	return ""
}

func sortedCells(m map[cellID][]Tok) []cellID {
	var ids []cellID
	for c := range m {
		ids = append(ids, c)
	}
	sort.Slice(ids, func(a, b int) bool {
		x, y := ids[a], ids[b]
		if x.grid != y.grid {
			return !x.grid
		}
		if x.s != y.s {
			return x.s < y.s
		}
		return x.k < y.k
	})
	return ids
}
