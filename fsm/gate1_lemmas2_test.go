package fsm

import (
	"fmt"
	"math/bits"
	"testing"
)

// Theorem C, continued: F2 to F6 (see gate1_lemmas_test.go for the pool, the
// gadget plans and the meaning of "closed").

// lkey is the whole local state of a service as a comparable value.
type lkey struct {
	s                         State
	reason                    Reason
	cause                     Cause
	exit                      Exit
	wexp, armed, tree, mainUp bool
	spawning                  bool
	live                      uint8
	nR                        int
	h                         [2]bool
}

func lk(s *SvcState) lkey {
	return lkey{s.S, s.Reason, s.Cause, s.Exit, s.Wexp, s.Armed, s.Tree, s.MainUp, s.Spawning, s.Live, len(s.R), s.H}
}

// readSet is what the cells of service i read of the others: its targets, and
// its uncut dependents (the cut of E2/N1). It returns a mask.
func readSet(p *Plan, i int) (m uint32) {
	p.edges(func(a, b int, _ edgeKind) {
		if a == i {
			m |= 1 << b
		}
		if b == i && !p.cut(a, b) {
			m |= 1 << a
		}
	})
	return
}

type f2key struct {
	i      int
	free   bool
	l      lkey
	nb     [2]State // the state of each service of the read set, 255 elsewhere
	goal   uint8    // goal of i and of its targets
	mode   Mode
	status int
	rin    int
}

type f2out struct {
	ok   bool
	l    lkey
	effs string
	esc  bool
	st   int
}

func effBytes(r *Result) string {
	var b []byte
	for _, e := range r.Effects {
		b = append(b, byte(e.Kind), byte(e.Timer), byte(e.Delay), byte(e.Report))
	}
	return string(b)
}

// F2, fire(i): for i in O the next local state of i (and what fire does for it)
// is a function of i's local state, the state of its targets and uncut
// dependents, its goal and its targets' goals, mode and |R_inst|; for i in G,
// fire(i) writes no pi component and raises no escalation. Neighbourhood: the
// WHOLE product of the two services' local states (853 x 853) x 4 values of
// (mode, |R_inst|) x 2 of |R_p| x both guard phases x both services, closed at
// 4 fires a world: the dependence on everything outside the key (the rest of
// the other service's state, R_p, the holders of services it does not read) is
// tested by the function check across the whole product.
func f2Plan(p *Plan, pool []SvcState) (cases, want int, red string) {
	pools := [2][]SvcState{pool, pool}
	want = forPiSize(pools, 2) * 4
	ref := map[f2key]f2out{}
	forPi(p, pools, 2, func(w *World, _ bool) {
		if red != "" {
			return
		}
		goal := p.Goals(w)
		for i := range p.Svc {
			rs := readSet(p, i)
			var nb [2]State
			gm := goal & (1 << i)
			for j := range nb {
				nb[j] = 255
				if rs&(1<<j) != 0 {
					nb[j] = w.Svc[j].S
					if p.Svc[i].Req != nil || p.Svc[i].Want != nil || p.Svc[i].Ord != nil {
						// goal of a target: read by G_ok
						if listHas(p.Svc[i].Want, j) || listHas(p.Svc[i].Ord, j) {
							gm |= goal & (1 << j)
						}
					}
				}
			}
			for _, free := range []bool{false, true} {
				cases++
				r := &run{p: p, w: w.Clone(), res: &Result{}}
				ok := r.fire(i, free)
				if p.Svc[i].Grant {
					if !piEq(p, w, r.w) || r.escPending || effO(p, r.res) != "" {
						red = fmt.Sprintf("F2: fire(%d) of a service of G writes pi or raises an escalation", i)
						return
					}
					continue
				}
				k := f2key{i, free, lk(&w.Svc[i]), nb, uint8(gm), w.Mode, w.Status, len(w.RInst)}
				o := f2out{ok, lk(&r.w.Svc[i]), effBytes(r.res), r.escPending, r.escStatus}
				if prev, seen := ref[k]; seen && prev != o {
					red = fmt.Sprintf("F2: fire(%d, free=%t) of a service of O depends on something outside its read set: %+v / %+v", i, free, prev, o)
					return
				}
				ref[k] = o
			}
		}
	})
	return
}

func listHas(l []int, x int) bool {
	for _, v := range l {
		if v == x {
			return true
		}
	}
	return false
}

func TestGate1LemmaF2(t *testing.T) {
	pool := lemmaPool(t)
	n := eachPlan(t, lemmaPlans(t, true), func(p *Plan) (int, int, string) { return f2Plan(p, pool) })
	t.Logf("F2: %d local states, 15 plans, %d (world, service, phase) cases over the whole product, closed", len(pool), n)
}

func TestGate1LemmaF2Witness(t *testing.T) {
	p := newPlan(t, Plan{Quirks: Quirks{FreeAcrossCut: true}, MaxInst: 1, BudgetP: 1, Svc: []SvcPlan{{Restart: OnFailure, MaxA: 1}, {Restart: Always, MaxA: 1, Grant: true, Req: []int{0}}}})
	_, _, red := f2Plan(p, lemmaPool(t))
	t.Logf("FreeAcrossCut: %.140s", red)
	if red == "" {
		t.Error("F2 does not see G_free reading a dependent across the cut")
	}
}

// f3Check is F3 on one plan: goal(C) for every C in O is a function of the
// holders on O, over every assignment of the 2 x n holder bits.
func f3Check(p *Plan) (cases int, red string) {
	o := omask(p)
	n := len(p.Svc)
	seen := map[uint32]uint32{}
	for h := uint32(0); h < 1<<(2*n); h++ {
		w := p.NewWorld()
		var onO uint32
		for i := 0; i < n; i++ {
			w.Svc[i].H[Owner] = h&(1<<(2*i)) != 0
			w.Svc[i].H[Delegate] = h&(1<<(2*i+1)) != 0
			if o&(1<<i) != 0 {
				onO |= (h >> (2 * i) & 3) << (2 * i)
			}
		}
		cases++
		g := p.Goals(w) & o
		if prev, ok := seen[onO]; ok && prev != g {
			return cases, fmt.Sprintf("goals on O depend on a holder in G: holders %b", h)
		}
		seen[onO] = g
	}
	return cases, ""
}

// TestGate1FrameF3Goals: F3 over every labelled DAG of 3 services and every
// closed grant assignment, every assignment of the 2 x 3 holder bits; closed
// when the case count is plans x 2^6.
func TestGate1FrameF3Goals(t *testing.T) {
	plans, cases := 0, 0
	dagPlans(3, func(p *Plan, _ Plan) {
		plans++
		c, red := f3Check(p)
		if red != "" {
			t.Fatalf("plan %+v: %s", p.Svc, red)
		}
		cases += c
	})
	if plans == 0 || cases != plans*64 {
		t.Fatalf("not closed: %d plans, %d cases", plans, cases)
	}
	t.Logf("F3: %d plans (every labelled DAG of 3 services, every closed grant assignment), %d (plan, holder assignment) cases, closed", plans, cases)
}

// TestGate1FrameF3Witness: the draft-8 goal (holders and propagation ignoring
// the cut) makes the goal of a service of O depend on a holder in G.
func TestGate1FrameF3Witness(t *testing.T) {
	p := newPlan(t, Plan{Quirks: Quirks{GoalNoCut: true}, MaxInst: 1, BudgetP: 1, Svc: []SvcPlan{{Restart: OnFailure, MaxA: 1}, {Restart: OnFailure, MaxA: 1, Grant: true, Req: []int{0}}}})
	_, red := f3Check(p)
	t.Logf("GoalNoCut: %s", red)
	if red == "" {
		t.Error("F3 does not see a goal read across the cut")
	}
}

// F4, transactions: resolveTx and a rollback (endTx, by a resolveTx or by the
// request deadline) of a transaction of a holder on a service of O read and
// write only O; those of a holder on a service of G write no pi component.
// Neighbourhood: every holder slot a request can create (the owner's on any
// service, p's on a service of G) x every transaction value over its own
// partition (members, T, no-rollback) x every state (11) and holder bits (16) of
// the two services x escalating or not x both ways of ending it.
func f4Plan(p *Plan) (cases, want int, red string) {
	n := len(p.Svc)
	part := func(x int) uint32 {
		var m uint32
		for j, s := range p.Svc {
			if s.Grant == p.Svc[x].Grant {
				m |= 1 << j
			}
		}
		return m
	}
	type key struct {
		s     [2]State
		h     [2][2]bool
		tx    Tx
		esc   bool
		path  bool
		slotX int
		by    Principal
	}
	type out struct {
		h      [2][2]bool
		tx     Tx
		rolled bool
	}
	ref := map[key]out{}
	for x := 0; x < n; x++ {
		for by := Owner; by <= Delegate; by++ {
			if by == Delegate && !p.Svc[x].Grant {
				continue
			}
			pm := part(x)
			want += (1 << bits.OnesCount32(pm)) * (1 << bits.OnesCount32(pm)) * 2 * 121 * 16 * 2 * 2
			q := p.TxIndex(by, x)
			for members := uint32(0); members <= pm; members++ {
				if members&^pm != 0 {
					continue
				}
				for tset := uint32(0); tset <= pm; tset++ {
					if tset&^pm != 0 {
						continue
					}
					for nr := 0; nr < 2; nr++ {
						for ss := 0; ss < 121; ss++ {
							for hh := 0; hh < 16; hh++ {
								w := p.NewWorld()
								st := [2]State{State(ss / 11), State(ss % 11)}
								for j := range st {
									w.Svc[j].S = st[j]
									w.Svc[j].H = [2]bool{hh&(1<<(2*j)) != 0, hh&(1<<(2*j+1)) != 0}
								}
								tx := Tx{Active: true, Members: members, T: tset, NoRollback: nr == 1}
								w.Tx[q] = tx
								for _, esc := range []bool{false, true} {
									for _, path := range []bool{false, true} {
										cases++
										r := &run{p: p, w: w.Clone(), res: &Result{}}
										var rolled bool
										if path {
											r.deadline(Input{Kind: InDeadline, By: by, Svc: x})
										} else {
											rolled = r.resolveTx(esc)
										}
										if p.Svc[x].Grant {
											if !piEq(p, w, r.w) {
												return cases, want, fmt.Sprintf("F4: the transaction of holder (%d, %d) on a service of G writes a pi component (members %b T %b nr=%t, deadline=%t)", by, x, members, tset, nr == 1, path)
											}
											continue
										}
										if !gEq(p, w, r.w) {
											return cases, want, fmt.Sprintf("F4: the transaction of holder (%d, %d) on a service of O writes a G component (members %b T %b)", by, x, members, tset)
										}
										k := key{esc: esc, path: path, slotX: x, by: by, tx: tx}
										var o out
										for j := range st {
											k.s[j], k.h[j] = 255, [2]bool{}
											if !p.Svc[j].Grant {
												k.s[j], k.h[j] = st[j], w.Svc[j].H
												o.h[j] = r.w.Svc[j].H
											}
										}
										o.tx, o.rolled = r.w.Tx[q], rolled
										if prev, seen := ref[k]; seen && prev != o {
											return cases, want, fmt.Sprintf("F4: the transaction of holder (%d, %d) on a service of O reads G: %+v / %+v", by, x, prev, o)
										}
										ref[k] = o
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return cases, want, ""
}

func TestGate1LemmaF4(t *testing.T) {
	n := eachPlan(t, lemmaPlans(t, false), f4Plan)
	t.Logf("F4: 9 plans, %d (holder slot, transaction, state, holders, ending) cases, closed", n)
}

func TestGate1LemmaF4Witness(t *testing.T) {
	p := newPlan(t, Plan{Quirks: Quirks{RollbackAll: true}, MaxInst: 1, BudgetP: 1, Svc: []SvcPlan{{Restart: OnFailure, MaxA: 1}, {Restart: OnFailure, MaxA: 1, Grant: true, Req: []int{0}}}})
	_, _, red := f4Plan(p)
	t.Logf("RollbackAll: %.140s", red)
	if red == "" {
		t.Error("F4 does not see a rollback of a G transaction clearing a holder of O")
	}
}

// F5, escalation: only a cell of a service in O raises Quiescing, and it does
// so only by entering Failed when terminal or by charging |R_inst| past its
// maximum. Neighbourhood: the local state of the service the trigger names x
// the abstract values of the other (its state and holders) x 4 values of
// (mode, |R_inst|) x 2 of |R_p| x every trigger: every handler token naming it
// (SIGTERM excepted: it is the operator, not a cell) and both guard phases.
func f5Plan(p *Plan, pool []SvcState) (cases, want int, red string) {
	r := reps(pool)
	for s, pools := range [][2][]SvcState{{pool, r}, {r, pool}} {
		toks, _ := handlerTokensFor(p, s)
		var tr []Tok
		for _, tok := range toks {
			if tok.K != kTerm {
				tr = append(tr, tok)
			}
		}
		want += forPiSize(pools, 2) * (len(tr) + 2)
		check := func(w *World, rn *run, what string) {
			if !rn.escPending && rn.w.Mode == w.Mode {
				return
			}
			switch {
			case p.Svc[s].Grant:
				red = fmt.Sprintf("F5: %s of a service of G raises an escalation", what)
			case !(p.Svc[s].Terminal && rn.w.Svc[s].S == Failed && w.Svc[s].S != Failed) && p.InstCount(rn.w) <= p.MaxInst:
				red = fmt.Sprintf("F5: %s of a service of O raises an escalation that is neither a terminal Failed nor an R_inst overflow", what)
			}
		}
		forPi(p, pools, 2, func(w *World, _ bool) {
			if red != "" {
				return
			}
			for _, tok := range tr {
				cases++
				rn, _ := runHandler(p, w, tok)
				check(w, rn, fmt.Sprintf("the token %+v", tok))
			}
			for _, free := range []bool{false, true} {
				cases++
				rn := &run{p: p, w: w.Clone(), res: &Result{}}
				rn.fire(s, free)
				check(w, rn, fmt.Sprintf("fire(%d, free=%t)", s, free))
			}
		})
		if red != "" {
			return
		}
	}
	return
}

func TestGate1LemmaF5(t *testing.T) {
	pool := lemmaPool(t)
	n := eachPlan(t, lemmaPlans(t, true), func(p *Plan) (int, int, string) { return f5Plan(p, pool) })
	t.Logf("F5: 15 plans (terminal or not), %d (world, trigger) cases, closed", n)
}

func TestGate1LemmaF5Witnesses(t *testing.T) {
	for name, q := range map[string]struct {
		q    Quirks
		term bool
	}{"RetryPayCaller": {Quirks{RetryPayCaller: true}, false}, "terminal service in G": {Quirks{}, true}} {
		g := SvcPlan{Restart: OnFailure, MaxA: 1, Grant: true, Req: []int{0}, Terminal: q.term}
		p := newPlan(t, Plan{Quirks: q.q, MaxInst: 1, BudgetP: 1, Svc: []SvcPlan{{Restart: Always, MaxA: 1, Terminal: true}, g}})
		_, _, red := f5Plan(p, lemmaPool(t))
		t.Logf("%s: %.140s", name, red)
		if red == "" {
			t.Errorf("F5 does not see %s", name)
		}
	}
}

// stable says w is a fixpoint of propagation: a tick changes nothing.
func stable(p *Plan, w *World) bool {
	nw, r, err := p.Step(w, Input{Kind: InTick, Now: w.Now})
	return err == nil && len(r.Effects) == 0 && abstractKey(nw) == abstractKey(w)
}

// F6, rounds: a round boundary caused by a G rollback leaves pi unchanged, and
// the phase-1 fixpoint of O is the same whatever the number of rounds.
// Neighbourhood: every propagation-stable world (the named service's whole
// local state x the other's abstract values x 4 values of (mode, |R_inst|)),
// with and without an open transaction on a service of G (each holder slot
// alone, and all together: failing at once, so that its rollback starts a
// round), x every handler token naming the service: an H or Z token leaves pi
// and the O trees alone, an L token gives the same pi and effects as without
// the transactions.
func f6Plan(p *Plan, pool []SvcState) (cases, want, stableWorlds int, red string) {
	r := reps(pool)
	_, gs := sides(p)
	type slot struct {
		by Principal
		x  int
	}
	var slots []slot
	for _, x := range gs {
		slots = append(slots, slot{Owner, x}, slot{Delegate, x})
	}
	variants := [][]slot{nil}
	for _, s := range slots {
		variants = append(variants, []slot{s})
	}
	if len(slots) > 1 {
		variants = append(variants, slots)
	}
	for s, pools := range [][2][]SvcState{{pool, r}, {r, pool}} {
		toks, classes := handlerTokensFor(p, s)
		forPi(p, pools, 1, func(w *World, _ bool) {
			want += len(variants) * len(toks)
			if red != "" {
				return
			}
			if !stable(p, w) {
				cases += len(variants) * len(toks) // a world outside the neighbourhood: counted, not checked
				return
			}
			stableWorlds++
			var plain []inputOutcome
			var plainWorld []*World
			for _, tok := range toks {
				nw, rs, err := p.Step(w, decode(p, w, tok))
				switch {
				case err != nil:
					plain = append(plain, inputOutcome{kind: 1, class: errClass(err).Error()})
				case rs.Stale:
					plain = append(plain, inputOutcome{kind: 2})
				default:
					plain = append(plain, inputOutcome{w: nw, eff: effO(p, rs)})
				}
				plainWorld = append(plainWorld, w)
			}
			for vi, v := range variants {
				wv := w.Clone()
				for _, sl := range v {
					wv.Tx[p.TxIndex(sl.by, sl.x)] = Tx{Active: true, Members: 1 << sl.x, T: 1 << sl.x}
				}
				for ti, tok := range toks {
					cases++
					nw, rs, err := p.Step(wv, decode(p, wv, tok))
					if vi == 0 || err != nil || rs.Stale {
						continue
					}
					if classes[ti] == 'L' {
						b := plain[ti]
						if b.kind != 0 || !piEq(p, b.w, nw) || b.eff != effO(p, rs) {
							red = fmt.Sprintf("F6: the L input %+v gives another pi or O effects when a G transaction is open (variant %v)", tok, v)
							return
						}
					} else if !piEq(p, w, nw) || effO(p, rs) != "" {
						red = fmt.Sprintf("F6: the %c input %+v moves pi or an O tree when a G transaction is open (variant %v)", classes[ti], tok, v)
						return
					}
				}
			}
		})
		if red != "" {
			return
		}
	}
	return
}

func TestGate1LemmaF6(t *testing.T) {
	pool := lemmaPool(t)
	plans := map[string]*Plan{}
	for n, p := range lemmaPlans(t, false) {
		if _, g := sides(p); len(g) > 0 {
			plans[n] = p
		}
	}
	var st int
	var mu = make(chan struct{}, 1)
	n := eachPlan(t, plans, func(p *Plan) (int, int, string) {
		c, w, s, red := f6Plan(p, pool)
		mu <- struct{}{}
		st += s
		<-mu
		return c, w, red
	})
	t.Logf("F6: 6 plans with a service in G, %d stable worlds, %d (world, transaction variant, token) cases, closed", st, n)
}

func TestGate1LemmaF6Witnesses(t *testing.T) {
	for name, q := range map[string]Quirks{"GoalNoCut": {GoalNoCut: true}, "RollbackAll": {RollbackAll: true}} {
		p := newPlan(t, Plan{Quirks: q, MaxInst: 1, BudgetP: 1, Svc: []SvcPlan{{Restart: OnFailure, MaxA: 1}, {Restart: OnFailure, MaxA: 1, Grant: true, Req: []int{0}}}})
		c, wn, sw, red := f6Plan(p, lemmaPool(t))
		t.Logf("%s: cases %d of %d, %d stable worlds: %.140s", name, c, wn, sw, red)
		if red == "" {
			t.Errorf("F6 does not see %s", name)
		}
	}
}
