package fsm

import (
	"errors"
	"reflect"
	"testing"
)

func armDelay(r *Result, tm Timer) int64 {
	for _, e := range r.Effects {
		if e.Kind == EfArm && e.Timer == tm {
			return e.Delay
		}
	}
	return -1
}

func has(r *Result, k EffectKind) int {
	n := 0
	for _, e := range r.Effects {
		if e.Kind == k {
			n++
		}
	}
	return n
}

func one(t testing.TB, always RestartOn, mod func(*Plan)) *Plan {
	t.Helper()
	p := Plan{Svc: []SvcPlan{{Restart: always}}}
	if mod != nil {
		mod(&p)
	}
	return newPlan(t, p)
}

// crashFromReady kills the main process of a Ready service and reports the
// tree empty: the step that settles.
func (h *H) crash(i int) *Result {
	h.t.Helper()
	h.ev(i, X1)
	return h.ev(i, TE)
}

func TestWindowArithmetic(t *testing.T) {
	if got := count([]int64{0}, 3, 3); got != 0 {
		t.Fatalf("a charge exactly one window old is still counted: %d", got)
	}
	if got := count([]int64{0, 1, 2}, 2, 3); got != 3 {
		t.Fatalf("count = %d, want 3", got)
	}
	if got := count([]int64{0, 1}, 3, 3); got != 1 {
		t.Fatalf("count = %d, want 1", got)
	}
	got := appendWindow([]int64{0, 1}, 3, 3)
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("appendWindow = %v, want [1 3]", got)
	}
	in := []int64{1}
	_ = appendWindow(in, 2, 3)
	if len(in) != 1 || in[0] != 1 {
		t.Fatal("appendWindow modified its argument")
	}
}

func TestBackoffDelaysThenIntensity(t *testing.T) {
	p := one(t, Always, func(p *Plan) { p.MaxInst = 10 })
	h := newH(t, p)
	r := h.start(Owner, 1)
	if has(r, EfSpawn) != 1 {
		t.Fatalf("start did not spawn once: %+v", r.Effects)
	}
	h.want(0, Starting)
	h.ev(0, Ri)
	h.want(0, Ready)
	if h.w.Tx[Owner].Out != TxOK || h.w.Tx[Owner].Active {
		t.Fatalf("the start did not succeed: %+v", h.w.Tx[Owner])
	}
	for _, want := range []int64{1, 2} {
		r := h.crash(0)
		h.want(0, Backoff)
		if got := armDelay(r, TBackoff); got != want {
			t.Fatalf("backoff delay = %d, want %d", got, want)
		}
		h.ev(0, BTO)
		h.want(0, Starting)
		h.ev(0, Ri)
	}
	h.crash(0)
	h.want(0, Failed)
	if h.w.Svc[0].Reason != ReasonIntensity || h.w.Mode != Running {
		t.Fatalf("want local Failed(intensity), got %+v mode %v", h.w.Svc[0], h.w.Mode)
	}
}

func TestBackoffCapsAtMax(t *testing.T) {
	p := one(t, Always, func(p *Plan) {
		p.Svc[0].MaxA, p.MaxInst, p.Window = 9, 20, 100
		p.Initial, p.Factor, p.BackoffMax = 3, 3, 10
	})
	h := newH(t, p)
	h.start(Owner, 1)
	h.ev(0, Ri)
	for _, want := range []int64{3, 9, 10, 10} {
		r := h.crash(0)
		if got := armDelay(r, TBackoff); got != want {
			t.Fatalf("delay %d, want %d", got, want)
		}
		h.ev(0, BTO)
		h.ev(0, Ri)
	}
}

func TestChargesExpireWithTheWindow(t *testing.T) {
	p := one(t, Always, func(p *Plan) { p.MaxInst = 10 })
	h := newH(t, p)
	h.start(Owner, 1)
	h.ev(0, Ri)
	h.crash(0)
	h.ev(0, BTO)
	h.ev(0, Ri)
	h.tick(3)
	r := h.crash(0)
	if got := armDelay(r, TBackoff); got != 1 {
		t.Fatalf("a charge one window old must not count: delay %d, want 1", got)
	}
	if n := p.RCount(h.w, 0); n != 1 {
		t.Fatalf("|R_A| = %d, want 1", n)
	}
}

func TestOwnerOverflowQuiesces(t *testing.T) {
	p := one(t, Always, func(p *Plan) { p.MaxInst = 1 })
	h := newH(t, p)
	h.start(Owner, 1)
	h.ev(0, Ri)
	h.crash(0)
	h.ev(0, BTO)
	h.ev(0, Ri)
	r := h.crash(0)
	if h.w.Mode != Quiescing || h.w.Status != 1 || has(r, EfQuiesce) != 1 {
		t.Fatalf("R_inst overflow must escalate: mode %v status %d", h.w.Mode, h.w.Status)
	}
	h.want(0, Inactive)
	if !p.Exited(h.w) {
		t.Fatal("every service is Inactive: the instance has exited")
	}
	h.fail(Input{Kind: InStart, By: Owner, Targets: 1}, ErrQuiescing)
}

func TestDelegatedOverflowIsLocal(t *testing.T) {
	p := one(t, Always, func(p *Plan) { p.Svc[0].Grant, p.BudgetP, p.MaxInst, p.Svc[0].MaxA = true, 2, 1, 5 })
	h := newH(t, p)
	h.start(Delegate, 1)
	h.ev(0, Ri)
	h.crash(0)
	if h.w.Svc[0].S != Backoff {
		t.Fatalf("first crash: %v", h.w.Svc[0].S)
	}
	h.ev(0, BTO)
	h.ev(0, Ri)
	h.crash(0)
	h.want(0, Failed)
	if h.w.Svc[0].Reason != ReasonIntensity || h.w.Mode != Running {
		t.Fatalf("p's overflow is Failed(intensity) and never Quiescing: %+v %v", h.w.Svc[0], h.w.Mode)
	}
	if p.InstCount(h.w) != 0 || p.PCount(h.w) != 3 {
		t.Fatalf("R_inst %d (want 0), R_p %d (want API 1 + crashes 2)", p.InstCount(h.w), p.PCount(h.w))
	}
}

// Regression: REVIEW-3 D4 (Retry clears R_A and charges once) and REVIEW-6 N4
// (the Retry payment, F9).
func TestRetryPaysOnce(t *testing.T) {
	failedGrant := func(t *testing.T, mod func(*Plan)) *H {
		p := one(t, Always, func(p *Plan) { p.Svc[0].Grant = true; p.BudgetP = 3; mod(p) })
		h := newH(t, p)
		sv := &h.w.Svc[0]
		sv.S, sv.Reason, sv.H[Delegate] = Failed, ReasonExited, true
		sv.R = []int64{0, 0}
		return h
	}
	for _, by := range []Principal{Owner, Delegate} {
		h := failedGrant(t, func(*Plan) {})
		h.do(Input{Kind: InRetry, By: by, Svc: 0})
		h.want(0, Starting)
		if h.p.PCount(h.w) != 1 || h.p.InstCount(h.w) != 0 || len(h.w.Svc[0].R) != 0 {
			t.Fatalf("by %v: R_p %d (want 1), R_inst %d (want 0), R_A %v (want empty)", by, h.p.PCount(h.w), h.p.InstCount(h.w), h.w.Svc[0].R)
		}
	}
	// REVIEW-6 section 4 witness: pay(caller) charges R_inst for an owner caller.
	h := failedGrant(t, func(p *Plan) { p.Quirks.RetryPayCaller = true })
	h.do(Input{Kind: InRetry, By: Owner, Svc: 0})
	if h.p.InstCount(h.w) != 1 || h.p.PCount(h.w) != 0 {
		t.Fatalf("the pay(caller) mutant must pay R_inst: %d %d", h.p.InstCount(h.w), h.p.PCount(h.w))
	}
}

// Regression: REVIEW-5 F9 and the owner-Retry finding G1 (section 3.3).
func TestRetryOfOwnerServicePaysInstAndEscalates(t *testing.T) {
	p := one(t, Always, func(p *Plan) { p.MaxInst = 1 })
	h := newH(t, p)
	fail := func() {
		sv := &h.w.Svc[0]
		sv.S, sv.Reason, sv.H[Owner] = Failed, ReasonExited, true
	}
	fail()
	h.do(Input{Kind: InRetry, By: Owner, Svc: 0})
	if p.InstCount(h.w) != 1 || h.w.Mode != Running {
		t.Fatalf("first retry: R_inst %d mode %v", p.InstCount(h.w), h.w.Mode)
	}
	h.w.Svc[0] = SvcState{S: Failed, Reason: ReasonExited, H: [2]bool{true, false}, Epoch: h.w.Svc[0].Epoch, Gen: h.w.Svc[0].Gen}
	h.do(Input{Kind: InRetry, By: Owner, Svc: 0})
	if h.w.Mode != Quiescing || h.w.Status != 1 {
		t.Fatalf("an overflowing retry escalates: %v %d", h.w.Mode, h.w.Status)
	}
}

// Regression: REVIEW-3 D2 (a Retry with the goal down pays nothing).
func TestRetryWithGoalDownPaysNothing(t *testing.T) {
	p := one(t, Always, nil)
	h := newH(t, p)
	h.w.Svc[0].S, h.w.Svc[0].Reason = Failed, ReasonExited
	h.do(Input{Kind: InRetry, By: Owner, Svc: 0})
	if p.InstCount(h.w) != 0 || len(h.w.RP) != 0 {
		t.Fatal("a Retry that does not leave Failed pays nothing")
	}
	h.want(0, Inactive) // G_down
}

func TestAPIRefusalsInOrder(t *testing.T) {
	p := newPlan(t, Plan{BudgetP: 1, Svc: []SvcPlan{{Restart: Always}, {Restart: Always, Grant: true}}})
	h := newH(t, p)
	h.fail(Input{Kind: InStart, By: Owner, Targets: 0}, ErrInput)
	h.fail(Input{Kind: InStart, By: Owner, Targets: 4}, ErrInput)
	h.fail(Input{Kind: InStart, By: 2, Targets: 1}, ErrInput)
	h.fail(Input{Kind: InStop, By: Owner, Targets: 0}, ErrInput)
	h.fail(Input{Kind: InRetry, By: Owner, Svc: 2}, ErrInput)
	h.fail(Input{Kind: InRetry, By: Owner, Svc: -1}, ErrInput)
	h.fail(Input{Kind: InRestart, By: Owner, Svc: 2}, ErrInput)
	h.fail(Input{Kind: 0}, ErrInput)
	h.fail(Input{Kind: InDeadline, Svc: 3}, ErrInput)
	h.fail(Input{Kind: InDeadline, Svc: -1}, ErrInput)
	h.fail(Input{Kind: InStart, By: Delegate, Targets: 1}, ErrGrant)
	h.fail(Input{Kind: InStop, By: Delegate, Targets: 3}, ErrGrant)
	h.fail(Input{Kind: InRetry, By: Delegate, Svc: 0}, ErrGrant)
	h.fail(Input{Kind: InRestart, By: Delegate, Svc: 0}, ErrGrant)
	h.fail(Input{Kind: InStart, Now: -1, By: Owner, Targets: 1}, ErrTime)

	// A repeated start is not an error and is not charged (draft 16).
	h.start(Delegate, 2)
	h.do(Input{Kind: InStart, By: Delegate, Targets: 2})
	// The budget refuses a stop before it takes effect, and the world is intact.
	before := h.w.Clone()
	h.fail(Input{Kind: InStop, By: Delegate, Targets: 2}, ErrPrincipalBudget)
	h.fail(Input{Kind: InRetry, By: Delegate, Svc: 1}, ErrPrincipalBudget)
	if !sameWorld(before, h.w) {
		t.Fatal("a refused call changed the world")
	}
	// The owner is never charged an API budget.
	h.start(Owner, 1)
	h.do(Input{Kind: InStop, By: Owner, Targets: 1})
	h.tick(3)
	now := h.w.Now
	h.do(Input{Kind: InStop, Now: now, By: Delegate, Targets: 2})

	// Retry and Restart on a Leaked service are refused, before any charge.
	h.w.Svc[1].S = Leaked
	h.fail(Input{Kind: InRetry, Now: now, By: Delegate, Svc: 1}, ErrLeaked)
	h.fail(Input{Kind: InRestart, Now: now, By: Owner, Svc: 1}, ErrLeaked)

	// Quiescing beats every other check.
	h.do(Input{Kind: InTerm, Now: now})
	h.fail(Input{Kind: InStart, Now: now, By: Owner, Targets: 0}, ErrQuiescing)
	h.fail(Input{Kind: InRetry, Now: now, By: 2, Svc: 9}, ErrQuiescing)
}

// Draft 16 section 2.3: stop clears every holder of the stopped service and
// supersedes the transactions on those holders.
func TestStopClearsEveryHolderAndSupersedes(t *testing.T) {
	p := newPlan(t, Plan{Svc: []SvcPlan{{Restart: Always, Grant: true}}})
	h := newH(t, p)
	h.start(Owner, 1)
	h.start(Delegate, 1)
	// The delegate's start adds no state to the target: its holder is set,
	// and its transaction is already settled.
	if !h.w.Tx[p.TxIndex(Owner, 0)].Active || h.w.Tx[p.TxIndex(Delegate, 0)].Active || !h.w.Svc[0].H[Delegate] {
		t.Fatalf("owner open, delegate settled: %+v", h.w.Tx)
	}
	h.do(Input{Kind: InStop, By: Owner, Targets: 1})
	if h.w.Svc[0].H != [2]bool{} {
		t.Fatalf("every holder must go: %+v", h.w.Svc[0])
	}
	if tx := h.w.Tx[p.TxIndex(Owner, 0)]; tx.Active || tx.Out != TxSuperseded {
		t.Fatalf("a stop supersedes the open transaction: %+v", h.w.Tx)
	}
	h.want(0, Stopping)
}

// A service another holder still wants cannot be stopped without
// WithDependents; with it, the reverse closure goes.
func TestStopRefusedWhileWanted(t *testing.T) {
	p := newPlan(t, Plan{Svc: []SvcPlan{
		{Restart: Always},
		{Restart: Always, Req: []int{0}},
	}})
	h := newH(t, p)
	h.start(Owner, 2)
	h.ready(0)
	before := h.w.Clone()
	_, _, err := p.Step(h.w, Input{Kind: InStop, Now: h.w.Now, By: Owner, Targets: 1})
	var sw *StillWantedError
	if !errors.Is(err, ErrStillWanted) || !errors.As(err, &sw) || sw.Others != 0 {
		t.Fatalf("want ErrStillWanted, got %v", err)
	}
	if !sameWorld(before, h.w) {
		t.Fatal("a refused stop must not change the world")
	}
	h.do(Input{Kind: InStop, Now: h.w.Now, By: Owner, Targets: 1, WithDependents: true})
	h.want(0, StopWait) // waits for its dependent
	h.want(1, Stopping)
}

func TestRestartAndDone(t *testing.T) {
	p := newPlan(t, Plan{Svc: []SvcPlan{{Kind: Oneshot}}})
	h := newH(t, p)
	h.start(Owner, 1)
	h.ev(0, Ri)
	h.ev(0, X0)
	h.ev(0, TE)
	h.want(0, Done)
	if p.InstCount(h.w) != 0 || len(h.w.Svc[0].R) != 0 {
		t.Fatal("a oneshot that succeeds is never charged")
	}
	// Done x Restart re-runs it with cause restart, uncharged.
	h.do(Input{Kind: InRestart, By: Owner, Svc: 0})
	h.want(0, Starting)
	if p.InstCount(h.w) != 0 {
		t.Fatal("a re-run is uncharged")
	}
	h.ev(0, Ri)
	h.do(Input{Kind: InRestart, By: Owner, Svc: 0})
	h.want(0, Stopping) // StopWait, then G_free
	if h.w.Svc[0].Cause != CauseRestart {
		t.Fatalf("cause %v, want restart", h.w.Svc[0].Cause)
	}
}

func TestTransactionSuccessFailureAbort(t *testing.T) {
	plan := func() *Plan {
		return newPlan(t, Plan{Svc: []SvcPlan{{Restart: Always}, {Kind: Oneshot, Req: []int{0}}}})
	}
	t.Run("success", func(t *testing.T) {
		h := newH(t, plan())
		h.start(Owner, 0b10)
		if tx := h.w.Tx[1]; !tx.Active || tx.T != 0b11 || tx.Members != 0b10 {
			t.Fatalf("tx %+v", tx)
		}
		h.ev(0, Ri)
		h.want(1, Starting)
		h.ev(1, Ri)
		if tx := h.w.Tx[1]; tx.Active || tx.Out != TxOK {
			t.Fatalf("tx %+v", tx)
		}
	})
	t.Run("a failed member rolls back", func(t *testing.T) {
		h := newH(t, plan())
		h.start(Owner, 0b10)
		h.ev(0, SEp)
		h.want(0, Inactive)
		h.want(1, Inactive)
		if tx := h.w.Tx[1]; tx.Active || tx.Out != TxFailed || h.w.Svc[1].H[Owner] {
			t.Fatalf("tx %+v holder %v", tx, h.w.Svc[1].H)
		}
	})
	t.Run("no-rollback keeps the holder", func(t *testing.T) {
		h := newH(t, plan())
		h.do(Input{Kind: InStart, By: Owner, Targets: 0b10, Deadline: 9, NoRollback: true})
		h.ev(0, SEp)
		if tx := h.w.Tx[1]; tx.Out != TxFailed || !h.w.Svc[1].H[Owner] {
			t.Fatalf("tx %+v holder %v", tx, h.w.Svc[1].H)
		}
		h.want(0, Failed)
		h.want(1, Failed)
	})
	t.Run("deadline", func(t *testing.T) {
		h := newH(t, plan())
		h.do(Input{Kind: InStart, By: Owner, Targets: 0b10, Deadline: 10})
		h.stale(Input{Kind: InDeadline, Now: 9, By: Owner, Svc: 1})
		h.stale(Input{Kind: InDeadline, Now: 9, By: Delegate, Svc: 1})
		h.do(Input{Kind: InDeadline, Now: 10, By: Owner, Svc: 1})
		if tx := h.w.Tx[1]; tx.Active || tx.Out != TxFailed || h.w.Svc[1].H[Owner] {
			t.Fatalf("tx %+v", tx)
		}
		h.want(0, Stopping) // goal fell: stopwait(none) then G_free
	})
	t.Run("abort", func(t *testing.T) {
		h := newH(t, plan())
		h.start(Owner, 0b10)
		r := h.do(Input{Kind: InTerm})
		if tx := h.w.Tx[1]; tx.Active || tx.Out != TxAborted || h.w.Mode != Quiescing || h.w.Status != 0 || has(r, EfQuiesce) != 1 {
			t.Fatalf("tx %+v mode %v status %d", tx, h.w.Mode, h.w.Status)
		}
		if r := h.do(Input{Kind: InTerm}); has(r, EfQuiesce) != 0 {
			t.Fatal("a second SIGTERM must not announce Quiescing again")
		}
	})
}

func TestTerminalFailureEscalatesAndStatusIsTheMax(t *testing.T) {
	p := one(t, Never, func(p *Plan) { p.Svc[0].Terminal = true })
	h := newH(t, p)
	h.start(Owner, 1)
	r := h.ev(0, SEp)
	if h.w.Mode != Quiescing || h.w.Status != 1 || has(r, EfQuiesce) != 1 {
		t.Fatalf("mode %v status %d", h.w.Mode, h.w.Status)
	}
	h.do(Input{Kind: InTerm})
	if h.w.Status != 1 {
		t.Fatalf("SIGTERM after Quiescing(1) lowered the status to %d", h.w.Status)
	}
	if !p.Exited(h.w) {
		t.Fatal("Failed is an exit state")
	}
}

func TestExitedPredicate(t *testing.T) {
	p := newPlan(t, Plan{Svc: []SvcPlan{{}, {}}})
	w := p.NewWorld()
	if p.Exited(w) {
		t.Fatal("Running is never exited")
	}
	w.Mode = Quiescing
	for _, s := range []State{Inactive, Failed, Leaked} {
		w.Svc[0].S, w.Svc[1].S = s, Inactive
		if !p.Exited(w) {
			t.Fatalf("%v must count as exited", s)
		}
	}
	for s := Waiting; s <= Backoff; s++ {
		if s == Failed {
			continue
		}
		w.Svc[1].S = s
		if p.Exited(w) {
			t.Fatalf("%v must not count as exited", s)
		}
	}
}

// Regression: REVIEW-3 D3 (a stale WTO leaves the state unchanged) and D7 (a
// late SEp leaves no tree).
func TestStaleFilters(t *testing.T) {
	p := one(t, Always, nil)
	h := newH(t, p)
	h.start(Owner, 1)
	e, g := h.w.Svc[0].Epoch, h.w.Svc[0].Gen
	mk := func(ev Event, de, dg uint32) Input {
		return Input{Kind: InEvent, Svc: 0, Event: ev, Epoch: e + de, Gen: g + dg}
	}
	h.stale(mk(Ri, 1, 0))
	h.stale(mk(X1, 1, 0))
	h.stale(mk(TE, 0, 0))
	h.stale(mk(StartTO, 0, 1))
	h.stale(mk(StartTO, 0, ^uint32(0)))
	h.stale(mk(SWTO, 0, 0))   // not the state's timer
	h.stale(mk(BTO, 0, 0))    // not live
	h.stale(mk(WTO, 0, 0))    // not live
	h.stale(mk(StopTO, 0, 0)) // not live
	h.stale(mk(KTO, 0, 0))
	h.stale(mk(Fa, 0, 0)) // liveness not armed
	h.ev(0, Ri)
	h.stale(mk(StartTO, 0, 1))
	h.stale(Input{Kind: InEvent, Svc: 0, Event: SEp, Epoch: e}) // Spawning is over
	h.stale(Input{Kind: InEvent, Svc: 0, Event: StartTO, Epoch: e, Gen: g})
	h.ev(0, X0)
	h.stale(Input{Kind: InEvent, Svc: 0, Event: X0, Epoch: e})
	h.stale(Input{Kind: InEvent, Svc: 0, Event: X1, Epoch: e})
	h.stale(Input{Kind: InEvent, Svc: 0, Event: Ri, Epoch: e})
	h.ev(0, TE)
	h.stale(Input{Kind: InEvent, Svc: 0, Event: TE, Epoch: e})
}

func TestEventInputErrors(t *testing.T) {
	p := one(t, Always, nil)
	h := newH(t, p)
	h.fail(Input{Kind: InEvent, Svc: -1, Event: Ri}, ErrInput)
	h.fail(Input{Kind: InEvent, Svc: 1, Event: Ri}, ErrInput)
	h.fail(Input{Kind: InEvent, Svc: 0, Event: Ry}, ErrInput)
	h.fail(Input{Kind: InEvent, Svc: 0, Event: Rs}, ErrInput)
	h.fail(Input{Kind: InEvent, Svc: 0, Event: NumEvents}, ErrInput)
	// An inconsistent world: the filter passes but the cell is marked unreachable.
	h.w.Svc[0].Spawning = true
	h.fail(Input{Kind: InEvent, Svc: 0, Event: SEt}, ErrUnreachable)
	h.fail(Input{Kind: InEvent, Svc: 0, Event: SEp}, ErrUnreachable)
	h.w.Svc[0].Spawning = false
	h.w.Svc[0].Tree, h.w.Svc[0].MainUp = true, true
	h.fail(Input{Kind: InEvent, Svc: 0, Event: X0}, ErrUnreachable)
	h.fail(Input{Kind: InEvent, Svc: 0, Event: Ri}, ErrUnreachable)
	h.w.Svc[0].MainUp = false
	h.fail(Input{Kind: InEvent, Svc: 0, Event: TE}, ErrUnreachable)
	h.w.Svc[0].Live = 1 << TWait
	h.fail(Input{Kind: InEvent, Svc: 0, Event: WTO}, ErrUnreachable) // Inactive x WTO
}

// Regression: REVIEW-4 E8 (StopWait x X1 records perm only from the pipe).
func TestPermOnlyWhereThePipeCanHoldIt(t *testing.T) {
	p := one(t, Never, nil)
	h := newH(t, p)
	h.start(Owner, 1)
	e := h.w.Svc[0].Epoch
	h.fail(Input{Kind: InEvent, Svc: 0, Event: Ri, Epoch: e, Perm: true}, ErrInput)
	h.fail(Input{Kind: InEvent, Svc: 0, Event: X0, Epoch: e, Perm: true}, ErrInput)
	h.ev(0, Ri)
	h.fail(Input{Kind: InEvent, Svc: 0, Event: X1, Epoch: e, Perm: true}, ErrInput) // Ready: no pipe
	// Starting x X1 with perm: settle sends it to Failed(spawn), uncharged.
	h2 := newH(t, p)
	h2.do(Input{Kind: InStart, By: Owner, Targets: 1, Deadline: 5, NoRollback: true})
	in := h2.evIn(0, X1)
	in.Perm = true
	h2.do(in)
	h2.ev(0, TE)
	h2.want(0, Failed)
	if h2.w.Svc[0].Reason != ReasonSpawn || h2.w.Svc[0].Exit != ExitPerm {
		t.Fatalf("%+v", h2.w.Svc[0])
	}
	if p.InstCount(h2.w) != 0 {
		t.Fatal("perm is never charged")
	}
}

// Regression: REVIEW-3 D1 (a signalled oneshot is not Done) and REVIEW-5 D1
// (a StartTO oneshot restart is charged).
func TestSettleRulesInOrder(t *testing.T) {
	type tc struct {
		name   string
		kind   Kind
		on     RestartOn
		goalUp bool
		state  SvcState
		want   State
		reason Reason
		delay  int64
	}
	cases := []tc{
		{"1 goal down beats everything", Oneshot, Always, false, SvcState{Exit: ExitPerm, Cause: CauseNatural}, Inactive, 0, -1},
		{"2 perm", Daemon, Always, true, SvcState{Exit: ExitPerm, Cause: CauseNatural}, Failed, ReasonSpawn, -1},
		{"3 oneshot ok", Oneshot, Always, true, SvcState{Exit: ExitOK, Cause: CauseNatural}, Done, 0, -1},
		{"3 a signalled oneshot is not Done", Oneshot, Never, true, SvcState{Exit: ExitSig, Cause: CauseRestart}, Waiting, 0, -1},
		{"4 dep crash is charged", Daemon, Never, true, SvcState{Exit: ExitFail, Cause: CauseDep}, Waiting, 0, -1},
		{"4 restart, not a crash", Daemon, Never, true, SvcState{Exit: ExitSig, Cause: CauseRestart}, Waiting, 0, -1},
		{"4 none", Daemon, Never, true, SvcState{Exit: ExitSig, Cause: CauseNone}, Waiting, 0, -1},
		{"5 cause fail restarts", Daemon, Never, true, SvcState{Exit: ExitSig, Cause: CauseFail}, Backoff, 0, 1},
		{"6 natural fail, on-failure", Daemon, OnFailure, true, SvcState{Exit: ExitFail, Cause: CauseNatural}, Backoff, 0, 1},
		{"6 natural ok, always", Daemon, Always, true, SvcState{Exit: ExitOK, Cause: CauseNatural}, Backoff, 0, 1},
		{"7 natural ok, on-failure", Daemon, OnFailure, true, SvcState{Exit: ExitOK, Cause: CauseNatural}, Failed, ReasonExited, -1},
		{"7 natural fail, never", Daemon, Never, true, SvcState{Exit: ExitFail, Cause: CauseNatural}, Failed, ReasonExited, -1},
		{"7 natural sig, always", Daemon, Always, true, SvcState{Exit: ExitSig, Cause: CauseNatural}, Failed, ReasonExited, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newPlan(t, Plan{Svc: []SvcPlan{{Kind: c.kind, Restart: c.on}}})
			h := newH(t, p)
			sv := &h.w.Svc[0]
			*sv = c.state
			sv.S, sv.Tree = Stopping, true
			sv.H[Owner] = c.goalUp
			sv.Live = 1 << TStop
			r := h.ev(0, TE)
			if c.want == Waiting {
				c.want = Starting // G_ok fires in the same step
			}
			h.want(0, c.want)
			if got := h.w.Svc[0].Reason; c.want == Failed && got != c.reason {
				t.Fatalf("reason %v, want %v", got, c.reason)
			}
			if got := armDelay(r, TBackoff); got != c.delay {
				t.Fatalf("backoff delay %d, want %d", got, c.delay)
			}
		})
	}
}

// TestArbitraryOrderIsNotConfluent pins a finding against section 3.5, which
// asserts that the BFS finds every evaluation order of the services to agree.
// It does not: with S2 Waiting under a Delegate holder and S1 Starting, the
// owner's Stop of S1 lets S2 read S1's transient state when S1 is evaluated
// last. The package therefore fixes the order (targets before dependents, see
// Plan.sequence) and this test requires (a) every topological order of the
// plan to agree with Step, (b) at least one other order to disagree, so the
// finding stays true or the fix is noticed.
func TestArbitraryOrderIsNotConfluent(t *testing.T) {
	p := mixedPlan(t)
	topo := [][]int{{0, 1, 2}}
	other := [][]int{{0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	if got := p.sequence(); !reflect.DeepEqual(got, topo[0]) {
		t.Fatalf("sequence = %v, want %v", got, topo[0])
	}
	disagree := 0
	explore(t, p, p.NewWorld(), exploreOpts{maxNow: 1, maxNodes: 3000}, func(from *World, tok Tok, to *World, r *Result) {
		in := decode(p, from, tok)
		for _, rev := range []bool{false, true} {
			for _, o := range topo {
				w, _, err := p.step(from, in, o, rev)
				if err != nil || key(w) != key(to) {
					t.Fatalf("stratified order %v (rev %v) disagrees with Step on %+v from %s", o, rev, tok, key(from))
				}
			}
		}
		for _, o := range other {
			if w, _, err := p.step(from, in, o, false); err != nil || key(w) != key(to) {
				disagree++
			}
		}
	})
	if disagree == 0 {
		t.Fatal("every order agrees: the stratification is unnecessary, drop it")
	}
}

// TestStratifiedPropagationKeepsAWantTargetWaiting is the repro of the review
// of PR #5: B, and A with `order` (or `want`) on B, both Ready, and the owner
// stops both. A's move to StopWait is visible to B's G_free in the same round,
// so B stays in StopWait while A is signalled (section 3.5, draft 16). The
// unstratified targets-first sweep of d74ceab signalled B in the same step.
func TestStratifiedPropagationKeepsAWantTargetWaiting(t *testing.T) {
	for _, edge := range []string{"ord", "want"} {
		t.Run(edge, func(t *testing.T) {
			a := SvcPlan{Restart: Always}
			if edge == "ord" {
				a.Ord = []int{0}
			} else {
				a.Want = []int{0}
			}
			p := newPlan(t, Plan{Svc: []SvcPlan{{Restart: Always}, a}})
			h := newH(t, p)
			h.start(Owner, 0b11)
			h.ev(0, Ri)
			h.ev(1, Ri)
			h.want(0, Ready)
			h.want(1, Ready)
			for _, rev := range []bool{false, true} {
				w, _, err := p.step(h.w, Input{Kind: InStop, By: Owner, Targets: 0b11}, p.sequence(), rev)
				if err != nil {
					t.Fatal(err)
				}
				if w.Svc[1].S != Stopping || w.Svc[0].S != StopWait {
					t.Fatalf("rev %v: A %v B %v, want A Stopping and B StopWait", rev, w.Svc[1].S, w.Svc[0].S)
				}
			}
		})
	}
}

func TestNoFixpointIsRefusedAtTheCap(t *testing.T) {
	// A grid that loops (Backoff -> Waiting -> Backoff) for as long as the goal
	// is up, and pays R_inst on each lap: the holders fall when R_inst
	// overflows, so the lap count is MaxInst + 1 and the number of moves is
	// 2*MaxInst + 4. The cap is 32*N*N = 32 for one service.
	loop := func(maxInst int) Tables {
		tb := Default()
		tb.Grid[Inactive][GUp].Then = Move{Target: Backoff}
		tb.Grid[Backoff][GUp] = Cell{Reach: Reachable, Prio: 1, Then: Move{Target: Waiting}}
		tb.Grid[Backoff][GDown].Prio = 2
		tb.Grid[Waiting][GOk].Then = Move{Target: Backoff, Actions: ARetryPay}
		return tb
	}
	// Draft 16: an escalation raised inside a round is applied between
	// rounds, so the overflow of R_inst no longer ends the lap inside the
	// round: the loop runs to the cap for every MaxInst, and the world is
	// refused, never half-applied.
	for _, maxInst := range []int{1, 14, 15} {
		p := newPlan(t, Plan{MaxInst: maxInst, Window: 100, Tables: loop(maxInst), Svc: []SvcPlan{{Restart: Always}}})
		if _, _, err := p.Step(p.NewWorld(), Input{Kind: InStart, By: Owner, Targets: 1, Deadline: 5}); !errors.Is(err, ErrNoFixpoint) {
			t.Fatalf("MaxInst %d: a looping grid must be refused at the cap, got %v", maxInst, err)
		}
	}
	for n, want := range map[int]int{1: 32, 2: 128, 3: 288, 32: 32768} {
		p := &Plan{Svc: make([]SvcPlan, n)}
		if p.moveCap() != want {
			t.Fatalf("moveCap(%d) = %d, want %d", n, p.moveCap(), want)
		}
	}
}

func TestPlanValidation(t *testing.T) {
	ok := Plan{MaxInst: 1, BudgetP: 1, Window: 1, Initial: 1, Factor: 1, BackoffMax: 1, Svc: []SvcPlan{{MaxA: 1}}}
	if _, err := NewPlan(ok); err != nil {
		t.Fatal(err)
	}
	mut := func(f func(*Plan)) Plan {
		p := ok
		p.Svc = append([]SvcPlan(nil), ok.Svc...)
		f(&p)
		return p
	}
	bad := map[string]Plan{
		"no service":     mut(func(p *Plan) { p.Svc = nil }),
		"33 services":    mut(func(p *Plan) { p.Svc = make([]SvcPlan, 33) }),
		"max_inst":       mut(func(p *Plan) { p.MaxInst = 0 }),
		"budget_p":       mut(func(p *Plan) { p.BudgetP = 0 }),
		"window":         mut(func(p *Plan) { p.Window = 0 }),
		"initial":        mut(func(p *Plan) { p.Initial = 0 }),
		"factor":         mut(func(p *Plan) { p.Factor = 0 }),
		"backoff max":    mut(func(p *Plan) { p.BackoffMax = 0 }),
		"delay too long": mut(func(p *Plan) { p.BackoffMax = 1<<31 + 1 }),
		"max_a":          mut(func(p *Plan) { p.Svc[0].MaxA = 0 }),
		"edge range":     mut(func(p *Plan) { p.Svc[0].Req = []int{1} }),
		"negative edge":  mut(func(p *Plan) { p.Svc[0].Want = []int{-1} }),
		"self edge":      mut(func(p *Plan) { p.Svc[0].Ord = []int{0} }),
		"cycle": mut(func(p *Plan) {
			p.Svc = []SvcPlan{{MaxA: 1, Req: []int{1}}, {MaxA: 1, Want: []int{0}}}
		}),
		"open grant": mut(func(p *Plan) {
			p.Svc = []SvcPlan{{MaxA: 1}, {MaxA: 1, Req: []int{0}, Grant: false}, {MaxA: 1, Grant: true}}
			p.Svc[0].Req = []int{2}
		}),
		"settle not total": mut(func(p *Plan) {
			p.Tables = Default()
			p.Tables.Settle = p.Tables.Settle[:len(p.Tables.Settle)-1]
		}),
	}
	for name, p := range bad {
		if _, err := NewPlan(p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// 2^31 is the longest delay.
	long := ok
	long.BackoffMax = 1 << 31
	if _, err := NewPlan(long); err != nil {
		t.Errorf("2^31: %v", err)
	}
	// 32 services is the most.
	many := ok
	many.Svc = make([]SvcPlan, 32)
	for i := range many.Svc {
		many.Svc[i].MaxA = 1
	}
	if _, err := NewPlan(many); err != nil {
		t.Errorf("32 services: %v", err)
	}
}
