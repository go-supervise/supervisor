package fsm

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestTablesTotality(t *testing.T) {
	tb := Default()
	if got := len(tb.Event) * len(tb.Event[0]); got != 165 {
		t.Fatalf("event cells = %d, want 165", got)
	}
	if got := len(tb.Grid) * len(tb.Grid[0]); got != 88 {
		t.Fatalf("grid cells = %d, want 88", got)
	}
	for s := range tb.Event {
		for e, c := range tb.Event[s] {
			switch c.Reach {
			case Reachable, UnreachableFilter, UnreachableO2, UnreachableLiveness:
			default:
				t.Errorf("event cell %v x %v has reach %v", State(s), Event(e), c.Reach)
			}
			if c.Reach != Reachable && (c.Pre != 0 || c.Cond != 0 || c.Then != (Move{}) || c.Else != (Move{})) {
				t.Errorf("event cell %v x %v is unreachable but carries behaviour", State(s), Event(e))
			}
			if c.Reach == Reachable && c.Cond == 0 && c.Else != (Move{}) {
				t.Errorf("event cell %v x %v has an Else without a condition", State(s), Event(e))
			}
		}
	}
	present := 0
	for s := range tb.Grid {
		prios := map[uint8]bool{}
		for g, c := range tb.Grid[s] {
			switch c.Reach {
			case Absent:
				if c.Prio != 0 || c.Then != (Move{}) {
					t.Errorf("absent guard cell %v x %v carries behaviour", State(s), Guard(g))
				}
			case Reachable:
				present++
				if c.Prio == 0 || prios[c.Prio] {
					t.Errorf("guard cell %v x %v has priority %d, zero or repeated in its row", State(s), Guard(g), c.Prio)
				}
				prios[c.Prio] = true
			default:
				t.Errorf("guard cell %v x %v has reach %v", State(s), Guard(g), c.Reach)
			}
		}
		for k := uint8(1); int(k) <= len(prios); k++ {
			if !prios[k] {
				t.Errorf("row %v: priorities are not 1..%d", State(s), len(prios))
			}
		}
	}
	if present != 15 {
		t.Fatalf("present guard cells = %d, want 15", present)
	}
	if n := len(tb.Settle); n != 8 {
		t.Fatalf("settle rules = %d, want 8", n)
	}
	if tb.Settle[len(tb.Settle)-1].Cond != 0 {
		t.Fatal("the last settle rule must be unconditional")
	}
}

// TestTablesAreCopied: a Plan's tables are its own, so a test mutating one
// cell cannot leak into Default.
func TestTablesAreCopied(t *testing.T) {
	a, b := Default(), Default()
	a.Settle[0].Name = "x"
	a.Event[0][0].Reach = Absent
	if b.Settle[0].Name == "x" || b.Event[0][0].Reach == Absent || Default().Settle[0].Name == "x" {
		t.Fatal("Default shares storage")
	}
}

// TestEventReachCounts pins the reachable/unreachable census of section 3.4.
func TestEventReachCounts(t *testing.T) {
	tb := Default()
	var n [8]int
	for s := range tb.Event {
		for e := range tb.Event[s] {
			n[tb.Event[s][e].Reach]++
		}
	}
	got := fmt.Sprintf("reachable=%d filter=%d o2=%d liveness=%d", n[Reachable], n[UnreachableFilter], n[UnreachableO2], n[UnreachableLiveness])
	if got != "reachable=53 filter=109 o2=2 liveness=1" {
		t.Fatalf("census %s", got)
	}
}

func render(tb Tables) string {
	var b strings.Builder
	for s := range tb.Event {
		for e, c := range tb.Event[s] {
			fmt.Fprintf(&b, "event %v %v: %+v\n", State(s), Event(e), c)
		}
	}
	for s := range tb.Grid {
		for g, c := range tb.Grid[s] {
			if c.Reach != Absent {
				fmt.Fprintf(&b, "grid %v %v: %+v\n", State(s), Guard(g), c)
			}
		}
	}
	for _, r := range tb.Settle {
		fmt.Fprintf(&b, "settle %+v\n", r)
	}
	fmt.Fprintf(&b, "permits %v\nstatetimer %+v\neventtimer %+v\n", Permits, StateTimer, EventTimer)
	return b.String()
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := "testdata/" + name
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("%s differs from the tables (run with -update after reading the diff)", path)
	}
}

func TestTablesGolden(t *testing.T) { golden(t, "tables.golden", render(Default())) }

// TestTablesDeterministic: the tables are data; two renders agree and the
// transition function is a function of (world, input).
func TestStepDeterministic(t *testing.T) {
	p := mixedPlan(t)
	var path []Input
	h := newH(t, p)
	h.start(Owner, 0b111)
	path = append(path, Input{Kind: InStart, By: Owner, Targets: 0b111, Deadline: 10})
	for i := 0; i < 8 && h.w.Mode == Running; i++ {
		for s := range h.w.Svc {
			if h.w.Svc[s].S == Starting {
				h.ev(s, Ri)
			}
		}
	}
	w0 := p.NewWorld()
	var a, b *World = w0, w0
	for _, in := range path {
		var ra, rb *Result
		var ea, eb error
		a, ra, ea = p.Step(a, in)
		b, rb, eb = p.Step(b, in)
		if ea != nil || eb != nil || !sameWorld(a, b) || fmt.Sprint(ra) != fmt.Sprint(rb) {
			t.Fatalf("two runs of the same input differ")
		}
	}
}
