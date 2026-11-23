package supervisor

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"slices"
	"strings"
	"testing"
)

// The draft-9 rules of Compile: principal classes (F4), the reader direction
// of E1, the privilege of a granted service (F2a), the budgets of H_OS-E5
// (F1), the never-failing loop (P2b, F5) and the unheld outside target (F3).

// errsOf lists every refusal of an error, flattened.
func errsOf(err error) []error {
	if err == nil {
		return nil
	}
	var ce *CompileError
	if errors.As(err, &ce) {
		return ce.Errs
	}
	return []error{err}
}

func zedBudget(fields ...string) string {
	var b strings.Builder
	b.WriteString("budget \"zed\" {\n")
	for _, f := range fields {
		fmt.Fprintf(&b, "    %s = 5\n", f)
	}
	b.WriteString("  }")
	return b.String()
}

const noCaps = "allow {\n    capabilities = []\n  }"

func asZed() string { return "run_as {\n    user = \"zed\"\n  }\n  " + noCaps }

// TestPrincipalClassesProperty enumerates every combination of the facts that
// decide the class of one principal, "zed", and compares the refusals about
// zed with an oracle written from the definitions of section 1.6.
func TestPrincipalClassesProperty(t *testing.T) {
	for n := 0; n < 2*2*3*3*2*2; n++ {
		x := n
		pick := func(k int) int { v := x % k; x /= k; return v }
		admin, read := pick(2) == 1, pick(2) == 1
		grant := pick(3)  // 0 none, 1 empty block, 2 services = [a]
		budget := pick(3) // 0 none, 1 reader budgets, 2 all budgets
		aRuns, bRuns := pick(2) == 1, pick(2) == 1
		name := fmt.Sprintf("admin=%v/read=%v/grant=%d/budget=%d/a=%v/b=%v", admin, read, grant, budget, aRuns, bRuns)
		t.Run(name, func(t *testing.T) {
			api := ""
			switch budget {
			case 1:
				api += zedBudget(readerBudgets...) + "\n  "
			case 2:
				api += zedBudget(budgetFields...) + "\n  "
			}
			switch grant {
			case 1:
				api += "grant \"zed\" {\n  }\n  "
			case 2:
				api += "grant \"zed\" {\n    services = [\"a\"]\n  }\n  "
			}
			b := strings.Replace(baseline, "api {", "api {\n  "+api, 1)
			if admin {
				b = strings.Replace(b, `admin = ["alice"]`, `admin = ["alice", "zed"]`, 1)
			}
			if read {
				b = strings.Replace(b, `read  = ["alice", "bob"]`, `read  = ["alice", "bob", "zed"]`, 1)
			}
			ra, rb := noCaps, noCaps
			if aRuns {
				ra = asZed()
			}
			if bRuns {
				rb = asZed()
			}
			layers := []Layer{
				rec("recipe.hcl", svcRecipe("a", ra)+svcRecipe("b", rb)),
				site("site.hcl", b+svcSite("a", "")+svcSite("b", "")),
			}
			_, err := compile(layers...)

			got := map[string]bool{}
			for _, e := range errsOf(err) {
				switch e := e.(type) {
				case ErrPrincipalClass, ErrDelegationUid:
					got[e.Error()] = true
				case ErrIncomplete:
					if strings.HasPrefix(e.Path, "api.budget/") {
						got[e.Error()] = true
					}
				}
			}

			want := map[string]bool{}
			delegated := grant == 2 && !admin
			if grant == 2 && admin {
				want[ErrPrincipalClass{Principal: "zed"}.Error()] = true
			}
			if !(admin && grant != 2) { // an owner-class principal with an empty grant is no delegate
				if grant != 0 {
					if grant == 2 && !aRuns {
						want[ErrDelegationUid{Service: "a", Principal: "zed", Direction: "granted"}.Error()] = true
					}
					if bRuns {
						want[ErrDelegationUid{Service: "b", Principal: "zed", Direction: "outside-grant"}.Error()] = true
					}
					if aRuns && grant == 1 {
						want[ErrDelegationUid{Service: "a", Principal: "zed", Direction: "outside-grant"}.Error()] = true
					}
				}
			}
			if !admin && read && grant == 0 {
				for _, s := range []string{"a", "b"} {
					if (s == "a" && aRuns) || (s == "b" && bRuns) {
						want[ErrDelegationUid{Service: s, Principal: "zed", Direction: "reader"}.Error()] = true
					}
				}
			}
			if !admin && (read || grant != 0) {
				need := readerBudgets
				if delegated {
					need = budgetFields
				}
				have := readerBudgets
				if budget == 2 {
					have = budgetFields
				}
				if budget == 0 {
					have = nil
				}
				for _, f := range need {
					if !slices.Contains(have, f) {
						want[ErrIncomplete{Path: "api.budget/zed/" + f}.Error()] = true
					}
				}
			}
			if !maps_equal(got, want) {
				t.Fatalf("refusals about zed differ\n got %v\nwant %v", keys(got), keys(want))
			}
		})
	}
}

func maps_equal(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func TestPrincipalClassNamesThePrincipal(t *testing.T) {
	l := grantsLayers(asOps(), "", grantA)
	l[1].Source = []byte(strings.Replace(string(l[1].Source), `admin = ["alice"]`, `admin = ["alice", "ops"]`, 1))
	e := wantErr[ErrPrincipalClass](t, l)
	if e.Principal != "ops" {
		t.Errorf("%+v", e)
	}
	// both names of the refusal are about p, and nothing else is refused
	for _, err := range errsOf(mustErr(t, l)) {
		if _, ok := err.(ErrPrincipalClass); !ok {
			t.Errorf("unexpected %v", err)
		}
	}
}

func mustErr(t *testing.T, layers []Layer) error {
	t.Helper()
	_, err := compile(layers...)
	if err == nil {
		t.Fatal("Compile accepted")
	}
	return err
}

func TestReaderRunningAServiceIsRefused(t *testing.T) {
	// bob only reads (the fixture's api.read): a service run as bob is E1's reader direction
	l := grantsLayers("run_as {\n    user = \"bob\"\n  }", "", "")
	e := wantErr[ErrDelegationUid](t, l)
	if e.Service != "a" || e.Principal != "bob" || e.Direction != "reader" {
		t.Errorf("%+v", e)
	}
	// the owner-class principal alice may run services
	mustCompile(t, grantsLayers("run_as {\n    user = \"alice\"\n  }", "", "")...)
	// bob who is also in api.admin is owner-class, not a reader
	l = grantsLayers("run_as {\n    user = \"bob\"\n  }", "", "")
	l[1].Source = []byte(strings.Replace(string(l[1].Source), `admin = ["alice"]`, `admin = ["alice", "bob"]`, 1))
	mustCompile(t, l...)
}

func TestDelegationPrivilege(t *testing.T) {
	type what = string
	cases := []struct {
		name     string
		recipeA  string
		baseline func(string) string
		want     []what
	}{
		{name: "none"},
		{name: "capability held", recipeA: "run_as {\n    user = \"ops\"\n  }\n  allow {\n    capabilities = [\"CAP_CHOWN\"]\n  }", want: []what{"capabilities"}},
		{name: "foreign group", recipeA: "run_as {\n    user = \"ops\"\n    group = \"wheel\"\n  }\n  " + noCaps, want: []what{"groups"}},
		{name: "own named group", recipeA: "run_as {\n    user = \"ops\"\n    group = \"ops\"\n  }\n  " + noCaps},
		{name: "nnp waived", baseline: func(s string) string {
			return strings.Replace(s, "level     = \"required\"\n    exempt_on = [\"windows\"]", "level     = \"waived\"\n    exempt_on = [\"windows\"]", 1)
		}, want: []what{"nnp"}},
		{name: "nnp exempt on linux", baseline: func(s string) string {
			return strings.Replace(s, `exempt_on = ["windows"]`, `exempt_on = ["linux", "windows"]`, 1)
		}, want: []what{"nnp"}},
		{name: "nnp exempt on darwin and windows", baseline: func(s string) string {
			return strings.Replace(s, `exempt_on = ["windows"]`, `exempt_on = ["darwin", "windows"]`, 1)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ra := tc.recipeA
			if ra == "" {
				ra = asOps()
			}
			l := grantsLayers(ra, "", grantA)
			if tc.baseline != nil {
				l[1].Source = []byte(tc.baseline(string(l[1].Source)))
			}
			_, err := compile(l...)
			var got []what
			for _, e := range errsOf(err) {
				pe, ok := e.(ErrDelegationPrivilege)
				if !ok {
					t.Fatalf("unexpected refusal %v", e)
				}
				if pe.Service != "a" || pe.Principal != "ops" {
					t.Errorf("%+v", pe)
				}
				got = append(got, pe.What)
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDelegationPrivilegeOnlyBindsGrantedServices(t *testing.T) {
	// b holds a capability and is outside every grant: the owner's service, no refusal
	l := grantsLayers(asOps(), "allow {\n    capabilities = [\"CAP_CHOWN\"]\n  }", grantA)
	mustCompile(t, l...)
	// every kind at once, and one error per kind
	l = grantsLayers("run_as {\n    user = \"ops\"\n    group = \"wheel\"\n  }\n  allow {\n    capabilities = [\"CAP_CHOWN\"]\n  }", "", grantA)
	l[1].Source = []byte(strings.Replace(string(l[1].Source), "level     = \"required\"\n    exempt_on = [\"windows\"]", "level     = \"waived\"\n    exempt_on = [\"windows\"]", 1))
	var got []string
	for _, e := range errsOf(mustErr(t, l)) {
		got = append(got, e.(ErrDelegationPrivilege).What)
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{"capabilities", "groups", "nnp"}) {
		t.Errorf("got %v", got)
	}
}

func TestBudgetsAreRequiredPerClass(t *testing.T) {
	// a reader without pids and memory is complete; a delegated principal is not
	mustCompile(t, grantsLayers(asOps(), "", grantA)...)
	l := grantsLayers(asOps(), "", grantA)
	l[1].Source = []byte(strings.Replace(string(l[1].Source), "pids = 50\n", "", 1))
	e := wantErr[ErrIncomplete](t, l)
	if e.Path != "api.budget/ops/pids" {
		t.Errorf("%+v", e)
	}
	// a reader that states nothing
	l = grantsLayers(asOps(), "", grantA)
	l[1].Source = []byte(strings.Replace(string(l[1].Source), "    inflight    = 8\n", "", 1))
	e = wantErr[ErrIncomplete](t, l)
	if e.Path != "api.budget/bob/inflight" {
		t.Errorf("%+v", e)
	}
	// a budget of zero is a stated budget
	l = grantsLayers(asOps(), "", grantA)
	l[1].Source = []byte(strings.Replace(string(l[1].Source), "inflight    = 8", "inflight    = 0", 1))
	mustCompile(t, l...)
	// the budgets only shrink across site layers (K3): a later layer lowers one
	l = plus(grantsLayers(asOps(), "", grantA), site("shrink.hcl", "api {\n  budget \"ops\" {\n    pids = 7\n  }\n}\n"))
	p := mustCompile(t, l...)
	if v := p.API["api.budget/ops/pids"]; v.Lo != 7 {
		t.Errorf("pids = %d, want the smaller 7", v.Lo)
	}
}

func TestBudgetSyntax(t *testing.T) {
	for _, src := range []string{
		"api {\n budget \"p\" {\n  nope = 1\n }\n}\n",
		"api {\n budget \"p\" {\n  pids = -1\n }\n}\n",
		"api {\n budget \"p\" {\n  pids = \"many\"\n }\n}\n",
		"api {\n budget \"p\" {\n  pids = 4294967296\n }\n}\n",
		"api {\n budget \"p\" {\n  pids = 1.5\n }\n}\n",
		"api {\n budget {\n  pids = 1\n }\n}\n",
		"api {\n budget \"p\" {\n  pids { x = 1 }\n }\n}\n",
		"api {\n budget \"p\" {\n  inner {\n  }\n }\n}\n",
	} {
		_, errs := ParseLayer(site("s.hcl", src))
		if len(errs) == 0 {
			t.Errorf("%q parsed", src)
		}
	}
	d, errs := ParseLayer(site("s.hcl", "api {\n budget \"p\" {\n  pids = 2147483647\n  memory = 0\n }\n}\n"))
	if len(errs) != 0 || d.API["api.budget/p/pids"].Lo != math.MaxInt32 || !d.API["api.budget/p/memory"].Set {
		t.Errorf("%v %+v", errs, d.API)
	}
}

// loopLayers builds one service with the given timers; max and window are
// the intensity statement, written as HCL literals.
func loopLayers(startS, stopS int, max, window string) []Layer {
	extra := fmt.Sprintf("intensity {\n    max    = %s\n    window = %s\n  }", max, window)
	site1 := baseline + svcSite("web", fmt.Sprintf("start {\n    timeout = \"%ds\"\n  }\n  stop {\n    timeout = \"%ds\"\n  }", startS, stopS))
	return []Layer{rec("recipe.hcl", svcRecipe("web", extra)), site("site.hcl", site1)}
}

func TestNeverFailingLoopBoundary(t *testing.T) {
	// U = start 10s + stop 30s + kill 5s + backoff.max 10s = 55s; max 3 -> 165s
	l := base()
	l[0].Source = []byte(svcRecipe("web", "intensity {\n    max = 3\n    window = \"165s\"\n  }"))
	e := wantErr[ErrNeverFailingLoop](t, l)
	if e.Service != "web" || e.Window != 165e9 || e.Need != 166e9 {
		t.Errorf("%+v", e)
	}
	// anything longer is accepted
	l[0].Source = []byte(svcRecipe("web", "intensity {\n    max = 3\n    window = \"165000000001ns\"\n  }"))
	mustCompile(t, l...)
	// below the boundary
	l[0].Source = []byte(svcRecipe("web", "intensity {\n    max = 3\n    window = \"1s\"\n  }"))
	wantErr[ErrNeverFailingLoop](t, l)
	// max = 0 and window = 0 is still refused (0 <= 0), need one second
	l[0].Source = []byte(svcRecipe("web", "intensity {\n    max = 0\n    window = \"0s\"\n  }"))
	if e := wantErr[ErrNeverFailingLoop](t, l); e.Need != 1e9 {
		t.Errorf("%+v", e)
	}
	l[0].Source = []byte(svcRecipe("web", "intensity {\n    max = 0\n    window = \"1ns\"\n  }"))
	mustCompile(t, l...)
}

func TestNeverFailingLoopNeedsBothOrNeither(t *testing.T) {
	mustCompile(t, base()...) // no intensity at all: nothing to check
	l := base()
	l[0].Source = []byte(svcRecipe("web", "intensity {\n    max = 3\n  }"))
	e := wantErr[ErrIncomplete](t, l)
	if e.Path != "web/intensity.window" {
		t.Errorf("%+v", e)
	}
	l[0].Source = []byte(svcRecipe("web", "intensity {\n    window = \"1h\"\n  }"))
	e = wantErr[ErrIncomplete](t, l)
	if e.Path != "web/intensity.max" {
		t.Errorf("%+v", e)
	}
}

func TestNeverFailingLoopWaitsForResolution(t *testing.T) {
	// an unresolved operand is already ErrIncomplete; no second refusal is invented
	for _, recipe := range []string{
		"intensity {\n    max = 3\n    window = { min = \"1s\", max = \"10s\" }\n  }",
		"intensity {\n    max = { min = 1, max = 5 }\n    window = \"1s\"\n  }",
	} {
		l := base()
		l[0].Source = []byte(svcRecipe("web", recipe))
		for _, e := range errsOf(mustErr(t, l)) {
			if _, ok := e.(ErrIncomplete); !ok {
				t.Errorf("unexpected %v", e)
			}
		}
	}
	// a timer that does not resolve
	l := base()
	l[0].Source = []byte(svcRecipe("web", "intensity {\n    max = 3\n    window = \"1s\"\n  }"))
	l[1].Source = []byte(strings.Replace(string(l[1].Source), "service \"web\" {", "service \"web\" {\n  stop {\n    timeout = { min = \"6s\", max = \"7s\", prefer = \"30s\" }\n  }", 1))
	for _, e := range errsOf(mustErr(t, l)) {
		if _, ok := e.(ErrNeverFailingLoop); ok {
			t.Errorf("a loop was judged on an unresolved timer: %v", e)
		}
	}
}

func TestNeverFailingLoopSaturates(t *testing.T) {
	// 2^31-1 charges of four year-long timers overflow int64: the product saturates
	huge := `"8760h"`
	recipe := fmt.Sprintf(`intensity {
    max = 2147483647
    window = "8760h"
  }
  kill {
    timeout = %s
  }`, huge)
	l := base()
	l[0].Source = []byte(strings.Replace(svcRecipe("web", recipe), `kill {
    timeout = "5s"
  }`, "", 1))
	e := wantErr[ErrNeverFailingLoop](t, l)
	if e.Need != math.MaxInt64 {
		t.Errorf("need = %d, want saturation", e.Need)
	}
}

func TestSatMul(t *testing.T) {
	for _, c := range []struct{ a, b, want int64 }{
		{0, 5, 0}, {5, 0, 0}, {0, 0, 0}, {1, math.MaxInt64, math.MaxInt64}, {math.MaxInt64, 1, math.MaxInt64},
		{2, math.MaxInt64 / 2, math.MaxInt64 - 1}, {2, math.MaxInt64/2 + 1, math.MaxInt64},
		{3, 7, 21}, {1 << 31, 1 << 31, 1 << 62}, {1 << 32, 1 << 31, math.MaxInt64},
	} {
		if got := satMul(c.a, c.b); got != c.want {
			t.Errorf("satMul(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestNeverFailingLoopProperty compares Compile with exact arithmetic on
// random timers, counts and windows.
func TestNeverFailingLoopProperty(t *testing.T) {
	r := rand.New(rand.NewSource(9))
	for i := 0; i < 400; i++ {
		start, stop := 1+r.Intn(30), 5+r.Intn(56) // seconds, inside the fixture's intervals
		max := int64(r.Intn(40))
		u := int64(start+stop+5+10) * 1e9
		span := new(big.Int).Mul(big.NewInt(max), big.NewInt(u))
		// windows around the boundary, and far from it
		var window int64
		switch r.Intn(4) {
		case 0:
			window = span.Int64()
		case 1:
			window = span.Int64() + 1
		case 2:
			window = span.Int64() - 1
		default:
			window = int64(r.Intn(4000)) * 1e9
		}
		if window < 0 {
			window = 0
		}
		l := loopLayers(start, stop, fmt.Sprint(max), fmt.Sprintf("%q", fmt.Sprintf("%dns", window)))
		_, err := compile(l...)
		wantRefused := big.NewInt(window).Cmp(span) <= 0
		var got *ErrNeverFailingLoop
		for _, e := range errsOf(err) {
			switch e := e.(type) {
			case ErrNeverFailingLoop:
				got = &e
			default:
				t.Fatalf("start %d stop %d max %d window %d: unexpected refusal %v", start, stop, max, window, e)
			}
		}
		if (got != nil) != wantRefused {
			t.Fatalf("start %d stop %d max %d window %dns: refused=%v, want %v", start, stop, max, window, got != nil, wantRefused)
		}
		if got != nil && (got.Need != span.Int64()+1e9 || got.Window != window) {
			t.Fatalf("%+v for span %v", got, span)
		}
	}
}

// setEnabled flips `enabled` of one service of the site layer.
func setEnabled(l []Layer, svc string, on bool) []Layer {
	out := append([]Layer(nil), l...)
	out[1].Source = []byte(strings.Replace(string(out[1].Source),
		fmt.Sprintf("service %q {\n  enabled  = true", svc), fmt.Sprintf("service %q {\n  enabled  = %v", svc, on), 1))
	return out
}

func TestWarnUnheldOutsideTarget(t *testing.T) {
	requires := asOps() + "\n  require = [\"b\"]\n  wait {\n    timeout = \"10m\"\n  }"
	l := grantsLayers(requires, "", grantA)

	// b is enabled: nothing to say
	if w := mustCompile(t, l...).Warnings; len(w) != 0 {
		t.Errorf("%v", w)
	}
	// b is not enabled: a warning, and the plan is still valid
	p := mustCompile(t, setEnabled(l, "b", false)...)
	want := WarnUnheldOutsideTarget{Service: "a", Target: "b"}
	if len(p.Warnings) != 1 || p.Warnings[0] != Warning(want) {
		t.Fatalf("%v", p.Warnings)
	}
	if !strings.Contains(want.String(), "a requires b") {
		t.Error(want.String())
	}
	// a itself is not enabled: it does not come up at boot anyway
	if w := mustCompile(t, setEnabled(setEnabled(l, "b", false), "a", false)...).Warnings; len(w) != 0 {
		t.Errorf("%v", w)
	}
	// b inside the grant is not outside
	both := grantsLayers(requires, asOps(), "grant \"ops\" {\n    services = [\"a\", \"b\"]\n  }")
	if w := mustCompile(t, setEnabled(both, "b", false)...).Warnings; len(w) != 0 {
		t.Errorf("%v", w)
	}
	// want and order edges are not require edges
	for _, kind := range []string{"want", "order"} {
		l := grantsLayers(asOps()+"\n  "+kind+" = [\"b\"]", "", grantA)
		if w := mustCompile(t, setEnabled(l, "b", false)...).Warnings; len(w) != 0 {
			t.Errorf("%s: %v", kind, w)
		}
	}
	// an owner service that requires an outside one is nobody's grant
	l = grantsLayers("", "", "")
	l[0].Source = []byte(svcRecipe("a", "require = [\"b\"]\n  wait {\n    timeout = \"10m\"\n  }") + svcRecipe("b", ""))
	if w := mustCompile(t, setEnabled(l, "b", false)...).Warnings; len(w) != 0 {
		t.Errorf("%v", w)
	}
}

func TestWarningsAreSortedAndDistinct(t *testing.T) {
	// a requires b and c, both outside and not enabled; two warnings in order
	l := grantsLayers("", "", "")
	l[0].Source = []byte(svcRecipe("a", asOps()+"\n  require = [\"c\", \"b\"]\n  wait {\n    timeout = \"10m\"\n  }") +
		svcRecipe("b", "") + svcRecipe("c", ""))
	l[1].Source = []byte(strings.Replace(string(l[1].Source), "api {", "api {\n  "+grantA, 1) + svcSite("c", ""))
	l = setEnabled(setEnabled(l, "b", false), "c", false)
	w := mustCompile(t, l...).Warnings
	if len(w) != 2 || w[0] != Warning(WarnUnheldOutsideTarget{"a", "b"}) || w[1] != Warning(WarnUnheldOutsideTarget{"a", "c"}) {
		t.Errorf("%v", w)
	}
}

func TestWarningsAcrossPrincipalsAreSorted(t *testing.T) {
	// aaron (granted b) is visited before ops (granted a); the plan lists a first
	aliceBudget := strings.Replace(opsBudget, "\"ops\"", "\"aaron\"", 1)
	aliceAs := strings.Replace(asOps(), "\"ops\"", "\"aaron\"", 1)
	wait := "\n  wait {\n    timeout = \"10m\"\n  }"
	l := grantsLayers("", "", "")
	l[0].Source = []byte(svcRecipe("a", asOps()+"\n  require = [\"x\"]"+wait) +
		svcRecipe("b", aliceAs+"\n  require = [\"y\"]"+wait) + svcRecipe("x", "") + svcRecipe("y", ""))
	l[1].Source = []byte(strings.Replace(string(l[1].Source), "api {",
		"api {\n  "+aliceBudget+"\n  "+grantA+"\n  grant \"aaron\" {\n    services = [\"b\"]\n  }", 1) +
		svcSite("x", "") + svcSite("y", ""))
	l = setEnabled(setEnabled(l, "x", false), "y", false)
	w := mustCompile(t, l...).Warnings
	if len(w) != 2 || w[0] != Warning(WarnUnheldOutsideTarget{"a", "x"}) || w[1] != Warning(WarnUnheldOutsideTarget{"b", "y"}) {
		t.Errorf("%v", w)
	}
}

func TestBudgetBlockNeedsItsParts(t *testing.T) {
	// a non-literal value and an unlabelled block are refused, and keep no budget
	for _, src := range []string{
		"api {\n budget \"p\" {\n  pids = var.x\n }\n}\n",
		"api {\n budget \"p\" {\n  pids = 1\n  memory = var.x\n }\n}\n",
	} {
		d, errs := ParseLayer(site("s.hcl", src))
		if len(errs) == 0 {
			t.Errorf("%q parsed: %+v", src, d.API)
		}
	}
}

func TestErrorTextsNameTheirFields(t *testing.T) {
	for _, c := range []struct {
		e    error
		part string
	}{
		{ErrPrincipalClass{Principal: "p"}, "p"},
		{ErrDelegationPrivilege{Service: "s", Principal: "p", What: "nnp"}, "s (nnp) granted to p"},
		{ErrNeverFailingLoop{Service: "s", Window: 5, Need: 9}, "5ns"},
		{ErrNeverFailingLoop{Service: "s", Window: 5, Need: 9}, "9ns"},
	} {
		if !strings.Contains(c.e.Error(), c.part) {
			t.Errorf("%q lacks %q", c.e.Error(), c.part)
		}
	}
}
