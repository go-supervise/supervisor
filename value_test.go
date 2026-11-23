package supervisor

import (
	"math/rand"
	"reflect"
	"slices"
	"testing"
)

var members = []string{"a", "b", "c", "d"}

// gen returns a random value of kind k over a small domain, so that equal
// values, subsets and empty intersections all occur often.
func gen(r *rand.Rand, k Kind, prefs bool) Value {
	if r.Intn(5) == 0 {
		return Top(k)
	}
	switch k {
	case K1:
		return Flat(members[r.Intn(3)])
	case K2:
		v := Interval(int64(r.Intn(6)), int64(r.Intn(6)))
		if prefs && r.Intn(2) == 0 {
			v.PrefR = []int64{int64(r.Intn(6))}
		}
		if prefs && r.Intn(2) == 0 {
			v.PrefS = []int64{int64(r.Intn(6))}
		}
		return v
	case K3:
		return Bound(int64(r.Intn(4)))
	case K4, K7:
		return Level(k, int64(r.Intn(3)))
	}
	var m []string
	for _, x := range members {
		if r.Intn(2) == 0 {
			m = append(m, x)
		}
	}
	return SetOf(k, m...)
}

var allKinds = []Kind{K1, K2, K3, K4, K5, K5P, K6, K7}

func noPrefs(v Value) Value { v.PrefR, v.PrefS = nil, nil; return v }

func TestMeetLaws(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, k := range allKinds {
		for i := 0; i < 3000; i++ {
			a, b, c := gen(r, k, true), gen(r, k, true), gen(r, k, true)
			if !Meet(Meet(a, b), c).Equal(Meet(a, Meet(b, c))) {
				t.Fatalf("kind %d: not associative: %+v %+v %+v", k, a, b, c)
			}
			if !Meet(a, b).Equal(Meet(b, a)) {
				t.Fatalf("kind %d: not commutative: %+v %+v", k, a, b)
			}
			if !Meet(a, a).Equal(a) {
				// the canonical empty interval is its own meet
				t.Fatalf("kind %d: not idempotent: %+v", k, a)
			}
			if !Meet(a, Top(k)).Equal(a) || !Meet(Top(k), a).Equal(a) {
				t.Fatalf("kind %d: top is not the identity: %+v", k, a)
			}
		}
	}
}

func TestConflictAbsorbs(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for i := 0; i < 2000; i++ {
		x := Meet(Flat("x"), Flat("y"))
		if !x.IsErr() {
			t.Fatal("x meet y is not a conflict")
		}
		if m := Meet(x, gen(r, K1, false)); m.Set && m.Kind == K1 && !m.IsErr() {
			t.Fatalf("a conflict was repaired: %+v", m)
		}
		e := Interval(3, 1)
		if !e.IsErr() {
			t.Fatal("empty interval is not a conflict")
		}
		if m := Meet(e, gen(r, K2, false)); !m.IsErr() && m.Set {
			t.Fatalf("an empty interval was repaired: %+v", m)
		}
	}
	if Flat("x").IsErr() || Top(K1).IsErr() || Bound(1).IsErr() || Interval(2, 2).IsErr() || Top(K2).IsErr() {
		t.Error("a non-conflict reports as a conflict")
	}
}

func TestOrderAgreesWithMeet(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for _, k := range allKinds {
		for i := 0; i < 3000; i++ {
			a, b := noPrefs(gen(r, k, false)), noPrefs(gen(r, k, false))
			m := Meet(a, b)
			if Leq(a, b) != m.Equal(a) {
				t.Fatalf("kind %d: Leq(%+v,%+v)=%v but meet=%+v", k, a, b, Leq(a, b), m)
			}
			if !Leq(m, a) || !Leq(m, b) {
				t.Fatalf("kind %d: the meet is not a lower bound: %+v", k, m)
			}
			// monotonicity: a <= b implies a meet c <= b meet c
			c := noPrefs(gen(r, k, false))
			if Leq(a, b) && !Leq(Meet(a, c), Meet(b, c)) {
				t.Fatalf("kind %d: meet is not monotone: %+v %+v %+v", k, a, b, c)
			}
		}
	}
}

func TestMeetKindMismatchPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a meet of two kinds did not panic")
		}
	}()
	Meet(Bound(1), Flat("x"))
}

func TestCanonical(t *testing.T) {
	if !SetOf(K6, "b", "a", "b").Equal(SetOf(K6, "a", "b")) {
		t.Error("a set is not canonical")
	}
	if v := SetOf(K5); !v.Set || v.Strs != nil {
		t.Errorf("the empty allow-set is the empty set, not the universe: %+v", v)
	}
	if !Interval(5, 1).Equal(Interval(9, 2)) {
		t.Error("empty intervals are not canonical")
	}
	if Meet(SetOf(K5, "a"), SetOf(K5, "b")).Strs != nil {
		t.Error("an empty intersection is not nil")
	}
	if SetOf(K5, []string{}...).Strs != nil || sortedDistinctInt([]int64{}) != nil {
		t.Error("a non-nil empty slice is not canonical")
	}
	// the universe is not the empty set
	if Meet(Top(K5), SetOf(K5)).Equal(Top(K5)) {
		t.Error("top and the empty set were confused")
	}
}

// The compile-level laws.

func richLayers() []Layer {
	return []Layer{
		rec("r1.hcl", svcRecipe("a", "")+svcRecipe("b", `want = ["a"]`)),
		rec("r2.hcl", `service "a" {
  needs {
    capabilities = ["CAP_CHOWN"]
  }
}`),
		site("s1.hcl", baseline+svcSite("a", "")+svcSite("b", "")),
		site("s2.hcl", `all {
  limits {
    nofile = 512
  }
}
service "a" {
  stop {
    timeout = { min = "10s", max = "40s", prefer = "15s" }
  }
}`),
	}
}

func perms(n int) [][]int {
	var out [][]int
	var rec func(p []int, used int)
	rec = func(p []int, used int) {
		if len(p) == n {
			out = append(out, slices.Clone(p))
			return
		}
		for i := 0; i < n; i++ {
			if used&(1<<i) == 0 {
				rec(append(p, i), used|1<<i)
			}
		}
	}
	rec(nil, 0)
	return out
}

func TestPermutationAndDuplicateInvariance(t *testing.T) {
	for name, layers := range map[string][]Layer{
		"accepted": richLayers(),
		"refused": plus(richLayers(), site("evil.hcl", `service "a" {
  exec = ["/x"]
}`)),
		"conflict": plus(richLayers(), rec("r3.hcl", `service "a" {
  kind = "oneshot"
}`)),
	} {
		t.Run(name, func(t *testing.T) {
			want, werr := compile(layers...)
			for _, p := range perms(len(layers)) {
				var in []Layer
				for _, i := range p {
					in = append(in, layers[i])
				}
				got, gerr := compile(in...)
				if (werr == nil) != (gerr == nil) || (werr != nil && werr.Error() != gerr.Error()) {
					t.Fatalf("perm %v changed the refusal:\n%v\n%v", p, werr, gerr)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("perm %v changed the plan", p)
				}
			}
			dup := append(slices.Clone(layers), layers[0], layers[len(layers)-1])
			got, gerr := compile(dup...)
			if (werr == nil) != (gerr == nil) || (werr != nil && werr.Error() != gerr.Error()) || !reflect.DeepEqual(got, want) {
				t.Fatalf("duplicates changed the outcome: %v vs %v", werr, gerr)
			}
		})
	}
	if _, err := compile(richLayers()...); err != nil {
		t.Fatalf("the accepted case is not accepted: %v", err)
	}
}

// Adding a site layer never widens a hardening field (monotonicity), and
// never changes a class D field: those equal the recipe-only values.
func TestSiteLayersOnlyNarrow(t *testing.T) {
	before := mustCompile(t, richLayers()...)
	extras := []string{
		"all {\n  limits {\n    nproc = 10\n  }\n  allow {\n    syscalls = [\"read\"]\n  }\n  protect \"seccomp\" {\n    level = \"required\"\n  }\n}",
		"all {\n  limits {\n    nproc = 100000\n  }\n  protect \"nnp\" {\n    level = \"waived\"\n  }\n}",
		"service \"a\" {\n  allow {\n    capabilities = [\"CAP_CHOWN\"]\n  }\n}",
		"service \"b\" {\n  kind = \"simple\"\n}",
	}
	for i, x := range extras {
		after := mustCompile(t, plus(richLayers(), site("extra.hcl", x))...)
		for s, sp := range before.Services {
			for f, v := range sp.Fields {
				nv, ok := after.Services[s].Fields[f]
				if !ok {
					t.Fatalf("extra %d dropped %s/%s", i, s, f)
				}
				if !Leq(nv, v) {
					t.Errorf("extra %d widened %s/%s: %+v -> %+v", i, s, f, v, nv)
				}
			}
			for _, f := range []string{"exec", "kind", "working_dir", "readiness.channel", "stop.signal"} {
				if !after.Services[s].Fields[f].Equal(sp.Fields[f]) {
					t.Errorf("extra %d changed class D field %s/%s", i, s, f)
				}
			}
		}
	}
}
