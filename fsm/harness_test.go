package fsm

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Tok is one step of a test, small enough to be a fuzz input and a corpus
// file: four bytes. decode turns it into an Input against the current world,
// so a token names "the Rise of service 2" and not an epoch.
type Tok struct{ K, A, B, C byte }

const (
	kEvent byte = iota
	kRetry
	kRestart
	kStart
	kStop
	kTerm
	kTick
	kDeadline
	numKinds
)

// decode builds the Input of a token against w. For an event, C bit 0 is the
// status pipe's perm, and bits 1-2 pick the epoch and generation: 0 current,
// 1 one ahead, 2 one behind.
func decode(p *Plan, w *World, t Tok) Input {
	in := Input{Now: w.Now}
	n := len(p.Svc)
	switch t.K % numKinds {
	case kEvent:
		i := int(t.A) % n
		in.Kind, in.Svc, in.Event = InEvent, i, Event(int(t.B)%NumEvents)
		in.Perm = t.C&1 != 0
		in.Epoch, in.Gen = w.Svc[i].Epoch, w.Svc[i].Gen
		switch (t.C >> 1) & 3 {
		case 1:
			in.Epoch, in.Gen = in.Epoch+1, in.Gen+1
		case 2:
			in.Epoch, in.Gen = in.Epoch-1, in.Gen-1
		}
		if in.Event == Ry || in.Event == Rs {
			in.Kind, in.Perm = InRetry, false
			if in.Event == Rs {
				in.Kind = InRestart
			}
		}
	case kRetry, kRestart:
		in.Kind = InRetry
		if t.K%numKinds == kRestart {
			in.Kind = InRestart
		}
		in.Svc, in.By = int(t.A)%n, Principal(t.B&1)
	case kStart:
		in.Kind, in.Targets = InStart, uint32(t.A)&(1<<n-1)
		in.By, in.NoRollback = Principal(t.B&1), t.B&2 != 0
		in.Deadline = w.Now + 1 + int64(t.C&3)
	case kStop:
		in.Kind, in.Targets, in.By = InStop, uint32(t.A)&(1<<n-1), Principal(t.B&1)
	case kTerm:
		in.Kind = InTerm
	case kTick:
		in.Kind, in.Now = InTick, w.Now+1+int64(t.C&1)
	case kDeadline:
		in.Kind, in.Slot = InDeadline, int(t.B)%3
	}
	return in
}

func encode(toks []Tok) []byte {
	var b bytes.Buffer
	for _, t := range toks {
		b.Write([]byte{t.K, t.A, t.B, t.C})
	}
	return b.Bytes()
}

func decodeBytes(b []byte) []Tok {
	var toks []Tok
	for ; len(b) >= 4; b = b[4:] {
		toks = append(toks, Tok{b[0], b[1], b[2], b[3]})
	}
	return toks
}

// allTokens is every token whose targets are plausible, for enumeration. All
// are "current" (C picks no stale mode) except where perm matters.
func allTokens(p *Plan) []Tok {
	n := byte(len(p.Svc))
	var toks []Tok
	for i := byte(0); i < n; i++ {
		for e := byte(0); e < NumEvents; e++ {
			if Event(e) == Ry || Event(e) == Rs {
				continue
			}
			toks = append(toks, Tok{kEvent, i, e, 0})
			if Event(e) == X1 {
				toks = append(toks, Tok{kEvent, i, e, 1})
			}
		}
		for by := byte(0); by < 2; by++ {
			toks = append(toks, Tok{kRetry, i, by, 0}, Tok{kRestart, i, by, 0}, Tok{kStart, 1 << i, by, 0}, Tok{kStop, 1 << i, by, 0})
		}
	}
	toks = append(toks, Tok{kTerm, 0, 0, 0}, Tok{kTick, 0, 0, 0}, Tok{kDeadline, 0, 0, 0}, Tok{kDeadline, 0, 1, 0}, Tok{kDeadline, 0, 2, 0})
	return toks
}

// key serialises a world, without epoch and generation: two worlds with the
// same key accept the same current inputs and reach the same keys.
func key(w *World) string {
	var b strings.Builder
	fmt.Fprintf(&b, "m%d.%d.t%d|", w.Mode, w.Status, w.Now)
	for _, s := range w.Svc {
		fmt.Fprintf(&b, "%d.%d.%d.%d.%t%t%t%t%t.%d.%v.%v|", s.S, s.Reason, s.Cause, s.Exit, s.Wexp, s.Armed, s.Tree, s.MainUp, s.Spawning, s.Live, s.R, s.H)
	}
	fmt.Fprintf(&b, "%v|%v|%+v", w.RInst, w.RP, w.Tx)
	return b.String()
}

func sameWorld(a, b *World) bool { return reflect.DeepEqual(a, b) }

// plans used by several tests.

func newPlan(t testing.TB, p Plan) *Plan {
	t.Helper()
	if p.MaxInst == 0 {
		p.MaxInst = 2
	}
	if p.BudgetP == 0 {
		p.BudgetP = 2
	}
	if p.Window == 0 {
		p.Window = 3
	}
	if p.Initial == 0 {
		p.Initial, p.Factor, p.BackoffMax = 1, 2, 4
	}
	for i := range p.Svc {
		if p.Svc[i].MaxA == 0 {
			p.Svc[i].MaxA = 2
		}
	}
	q, err := NewPlan(p)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// mixedPlan: S0 daemon (always, terminal); S1 oneshot requiring S0; S2 in
// grant(p), a daemon that wants S1, requires S0 and is ordered after S1.
func mixedPlan(t testing.TB) *Plan {
	return newPlan(t, Plan{Svc: []SvcPlan{
		{Restart: Always, Terminal: true},
		{Kind: Oneshot, Req: []int{0}},
		{Restart: OnFailure, Grant: true, Req: []int{0}, Want: []int{1}},
	}})
}

// H drives one world step by step.
type H struct {
	t testing.TB
	p *Plan
	w *World
}

func newH(t testing.TB, p *Plan) *H { return &H{t: t, p: p, w: p.NewWorld()} }

func (h *H) do(in Input) *Result {
	h.t.Helper()
	w, r, err := h.p.Step(h.w, in)
	if err != nil {
		h.t.Fatalf("%+v: %v", in, err)
	}
	h.w = w
	return r
}

func (h *H) fail(in Input, want error) {
	h.t.Helper()
	w, r, err := h.p.Step(h.w, in)
	if err != want || w != nil || r != nil {
		h.t.Fatalf("%+v: got (%v, %v, %v), want error %v", in, w, r, err, want)
	}
}

func (h *H) ev(i int, e Event) *Result {
	h.t.Helper()
	return h.do(h.evIn(i, e))
}

func (h *H) evIn(i int, e Event) Input {
	s := h.w.Svc[i]
	return Input{Kind: InEvent, Now: h.w.Now, Svc: i, Event: e, Epoch: s.Epoch, Gen: s.Gen}
}

func (h *H) start(by Principal, mask uint32) *Result {
	h.t.Helper()
	return h.do(Input{Kind: InStart, Now: h.w.Now, By: by, Targets: mask, Deadline: h.w.Now + 10})
}

func (h *H) tick(n int64) { h.t.Helper(); h.do(Input{Kind: InTick, Now: h.w.Now + n}) }

func (h *H) state(i int) State { return h.w.Svc[i].S }

func (h *H) want(i int, s State) {
	h.t.Helper()
	if got := h.w.Svc[i].S; got != s {
		h.t.Fatalf("service %d is %v, want %v (%+v)", i, got, s, h.w.Svc[i])
	}
}

func (h *H) stale(in Input) {
	h.t.Helper()
	before := h.w.Clone()
	before.Now = in.Now
	r := h.do(in)
	if !r.Stale || !sameWorld(before, h.w) {
		h.t.Fatalf("%+v was not dropped: stale=%v", in, r.Stale)
	}
}

// up brings a plan to the state where every service of mask is Ready, for a
// plan without dependencies between services of different order than index.
func (h *H) ready(i int) {
	h.t.Helper()
	for h.state(i) != Ready {
		switch h.state(i) {
		case Starting:
			h.ev(i, Ri)
		default:
			h.t.Fatalf("service %d is %v, cannot become Ready", i, h.state(i))
		}
	}
}
