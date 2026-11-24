package fsm

import (
	"fmt"
	"strings"
)

// The finite abstraction of Gate 1 (draft 16 section 3.8).
//
// Time is abstracted away. Now stays 0, so a window holds only entries that
// never expire on their own; an explicit AGE input drops the oldest entry of
// one window, which is what the passing of time does to a count. A window is
// kept as a count capped at max+1: every guard compares a count with its
// maximum, so the cap loses nothing. Deadlines are inputs (kDeadline). Epochs
// and generations are not part of the key: a token is current or one
// generation behind (staleBehind), and a stale one is dropped before it reads
// the world. The state space of ONE copy is then finite. Theorem U
// (gate1_unwind_test.go) is checked over this graph; the pair search below is
// a smoke test and is never cited as Gate 1.

// ageTok is the K of an AGE token. It is not a Tok of the harness: decode
// never sees it.
const ageTok byte = 255

// ageHigh says who controls the window an AGE token drops from: the budget
// of the delegate and the windows of services of a grant are p's; the
// instance window and the windows of O are the owner's.
func ageHigh(p *Plan, idx int) bool {
	n := len(p.Svc)
	switch {
	case idx < n:
		return p.Svc[idx].Grant
	case idx == n:
		return false
	}
	return true
}

// staleBehind is the C of an event token one epoch and one generation behind
// the service's (decode): a stale delivery, which the filter must drop without
// reading the world.
const staleBehind byte = 4

func abstractTokens(p *Plan) []Tok {
	var toks []Tok
	for _, t := range allTokens(p) {
		if t.K != kTick {
			toks = append(toks, t)
		}
	}
	// The stale-generation variants of every event (draft 17): an input of the
	// abstraction, not a counter, so each is one more token.
	for i := byte(0); i < byte(len(p.Svc)); i++ {
		for e := byte(0); e < NumEvents; e++ {
			if Event(e) != Ry && Event(e) != Rs {
				toks = append(toks, Tok{kEvent, i, e, staleBehind})
			}
		}
	}
	for idx := 0; idx < len(p.Svc)+2; idx++ {
		toks = append(toks, Tok{K: ageTok, A: byte(idx)})
	}
	return toks
}

func capWindow(s []int64, n int) []int64 {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// normalise caps the windows and forgets what the abstraction drops.
func normalise(p *Plan, w *World) *World {
	w.Now = 0
	for i := range w.Svc {
		w.Svc[i].R = capWindow(w.Svc[i].R, p.Svc[i].MaxA+1)
	}
	w.RInst = capWindow(w.RInst, p.MaxInst+1)
	w.RP = capWindow(w.RP, p.BudgetP+1)
	for q := range w.Tx {
		w.Tx[q].Deadline = 0
	}
	return w
}

// age is the AGE input: the oldest entry of one window leaves it.
func age(w *World, idx int) (*World, bool) {
	c := w.Clone()
	n := len(c.Svc)
	win := &c.RP
	switch {
	case idx < n:
		win = &c.Svc[idx].R
	case idx == n:
		win = &c.RInst
	}
	if len(*win) == 0 {
		return nil, false
	}
	*win = append([]int64(nil), (*win)[1:]...)
	return c, true
}

// abstractKey is the key of a world under the abstraction: windows as
// counts, no clock, no epoch, no deadline.
func abstractKey(w *World) string {
	var b strings.Builder
	fmt.Fprintf(&b, "m%d.%d|", w.Mode, w.Status)
	for _, s := range w.Svc {
		fmt.Fprintf(&b, "%d.%d.%d.%d.%t%t%t%t%t.%d.%d.%v|", s.S, s.Reason, s.Cause, s.Exit, s.Wexp, s.Armed, s.Tree, s.MainUp, s.Spawning, s.Live, len(s.R), s.H)
	}
	fmt.Fprintf(&b, "%d|%d", len(w.RInst), len(w.RP))
	for _, t := range w.Tx {
		fmt.Fprintf(&b, "|%t.%d.%d.%d.%t", t.Active, t.Out, t.Members, t.T, t.NoRollback)
	}
	return b.String()
}

// edge is one transition of the single-copy graph. kind: 0 enabled, 1
// refused (class names why), 2 stale (a no-op).
type edge struct {
	to    int32
	kind  uint8
	class int32
}

type absGraph struct {
	p      *Plan
	toks   []Tok
	ids    map[string]int32
	worlds []*World
	obs    []int32
	obsIDs map[string]int32
	classs map[string]int32
	next   [][]edge
}

func (g *absGraph) intern(w *World) int32 {
	k := abstractKey(w)
	if id, ok := g.ids[k]; ok {
		return id
	}
	id := int32(len(g.worlds))
	g.ids[k] = id
	g.worlds = append(g.worlds, w)
	o := obsO(g.p, w)
	oid, ok := g.obsIDs[o]
	if !ok {
		oid = int32(len(g.obsIDs))
		g.obsIDs[o] = oid
	}
	g.obs = append(g.obs, oid)
	return id
}

func (g *absGraph) class(e error) int32 {
	k := errClass(e).Error()
	id, ok := g.classs[k]
	if !ok {
		id = int32(len(g.classs))
		g.classs[k] = id
	}
	return id
}

// build expands every state reachable from init.
func buildGraph(p *Plan, init *World, limit int) *absGraph {
	g := &absGraph{p: p, toks: abstractTokens(p), ids: map[string]int32{}, obsIDs: map[string]int32{}, classs: map[string]int32{}}
	g.intern(normalise(p, init.Clone()))
	for id := 0; id < len(g.worlds); id++ {
		if id >= limit {
			// the frontier is not empty: rows past the limit stay nil
			g.next = append(g.next, nil)
			continue
		}
		w := g.worlds[id]
		row := make([]edge, len(g.toks))
		for ti, tok := range g.toks {
			var nw *World
			var err error
			stale := false
			if tok.K == ageTok {
				var ok bool
				if nw, ok = age(w, int(tok.A)); !ok {
					err = ErrInput
				}
			} else {
				var r *Result
				nw, r, err = p.Step(w, decode(p, w, tok))
				stale = err == nil && r.Stale
			}
			switch {
			case err != nil:
				row[ti] = edge{kind: 1, class: g.class(err)}
			case stale:
				row[ti] = edge{kind: 2}
			default:
				row[ti] = edge{to: g.intern(normalise(p, nw))}
			}
		}
		g.next = append(g.next, row)
	}
	return g
}

// gate1Abstract searches the pairs of the abstraction until the frontier is
// empty, or maxPairs pairs have been seen. It returns the first red pair, the
// number of single-copy states, the number of pairs and whether the frontier
// was emptied.
func gate1Abstract(p *Plan, init *World, maxStates, maxPairs int) (red string, states, pairs int, exhaustive bool) {
	g := buildGraph(p, init, maxStates)
	states = len(g.worlds)
	seen := map[uint64]bool{}
	var queue [][2]int32
	exhaustive = states <= maxStates
	push := func(a, b int32) {
		k := uint64(a)<<32 | uint64(b)
		if seen[k] {
			return
		}
		if len(seen) >= maxPairs {
			exhaustive = false
			return
		}
		seen[k] = true
		queue = append(queue, [2]int32{a, b})
	}
	push(0, 0)
	desc := func(tok Tok) string {
		if tok.K == ageTok {
			return fmt.Sprintf("age(%d)", tok.A)
		}
		return fmt.Sprintf("%+v", tok)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if g.next[cur[0]] == nil || g.next[cur[1]] == nil {
			continue // past the state limit
		}
		for ti, tok := range g.toks {
			var high, exempt bool
			if tok.K == ageTok {
				high = ageHigh(p, int(tok.A))
			} else {
				high, exempt = highInput(p, tok)
			}
			e1, e2 := g.next[cur[0]][ti], g.next[cur[1]][ti]
			if !high {
				if !exempt {
					if e1.kind != e2.kind || e1.kind == 1 && e1.class != e2.class {
						return fmt.Sprintf("low %s enabled in one copy only (%d/%d), from %s", desc(tok), e1.kind, e2.kind, abstractKey(g.worlds[cur[0]])), states, len(seen), exhaustive
					}
				}
				if e1.kind == 1 && e2.kind == 1 {
					continue
				}
				n1, n2 := cur[0], cur[1]
				if e1.kind == 0 {
					n1 = e1.to
				}
				if e2.kind == 0 {
					n2 = e2.to
				}
				if g.obs[n1] != g.obs[n2] {
					return fmt.Sprintf("low %s: obs_O differs from %s\n copy 1 %s\n copy 2 %s", desc(tok), abstractKey(g.worlds[cur[0]]), obsO(p, g.worlds[n1]), obsO(p, g.worlds[n2])), states, len(seen), exhaustive
				}
				push(n1, n2)
				continue
			}
			for _, c := range [3]struct{ one, two bool }{{true, false}, {false, true}, {true, true}} {
				n1, n2 := cur[0], cur[1]
				if c.one {
					if e1.kind == 1 {
						continue
					}
					if e1.kind == 0 {
						n1 = e1.to
					}
				}
				if c.two {
					if e2.kind == 1 {
						continue
					}
					if e2.kind == 0 {
						n2 = e2.to
					}
				}
				if g.obs[n1] != g.obs[n2] {
					return fmt.Sprintf("high %s (copy1=%t copy2=%t): obs_O differs from %s\n copy 1 %s\n copy 2 %s", desc(tok), c.one, c.two, abstractKey(g.worlds[cur[0]]), obsO(p, g.worlds[n1]), obsO(p, g.worlds[n2])), states, len(seen), exhaustive
				}
				push(n1, n2)
			}
		}
	}
	return "", states, len(seen), exhaustive
}
