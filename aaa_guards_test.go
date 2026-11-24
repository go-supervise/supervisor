package supervisor

import (
	"strings"
	"testing"
)

// The guards below used to be held only by a crash: a mutant that removed one
// made the code after it panic, and the suite went red with no assertion. Each
// test runs under safely, which turns a panic into an assertion failure, and
// the file sorts first so its verdict is printed before any other test crashes.

func safely(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("panic: %v", r)
		}
	}()
	f()
}

func TestGuardCompileDefaults(t *testing.T) {
	safely(t, func() {
		p, err := Compile(base(), Options{})
		if err != nil || p == nil {
			t.Fatalf("a valid layer set must compile with default options: %v", err)
		}
		if p.Services["web"] == nil {
			t.Error("web missing")
		}
	})
}

func TestGuardParserTypes(t *testing.T) {
	for _, src := range []string{
		"schema = \"x\"\n", // intOf: a string is not a number
		"api {\n budget \"p\" {\n  pids = \"x\"\n }\n}\n", // intOf in a budget
		"api {\n admin = \"x\"\n}\n",                      // strList: a string is not a list
		"api {\n admin = [1]\n}\n",                        // strOf: a number is not a string
	} {
		safely(t, func() {
			if _, errs := ParseLayer(site("s.hcl", src)); len(errs) == 0 {
				t.Errorf("%q parsed", src)
			}
		})
	}
	for _, body := range []string{
		"restart {\n    on = 5\n  }", // strOf
		"require = \"b\"",            // strList
		"env = \"x\"",                // object: not an object
		"env = {\n    A = 1\n  }",    // object value: not a string
	} {
		safely(t, func() {
			if _, errs := ParseLayer(rec("r.hcl", svcRecipe("web", body))); len(errs) == 0 {
				t.Errorf("%q parsed", body)
			}
		})
	}
}

func TestGuardParserLabels(t *testing.T) {
	for _, src := range []string{
		"service {\n}\n",
		"service \"a\" \"b\" {\n}\n",
	} {
		safely(t, func() {
			if _, errs := ParseLayer(rec("r.hcl", src)); len(errs) != 1 {
				t.Errorf("%q: %v", src, errs)
			}
		})
	}
}

func TestGuardServiceBlocksMerge(t *testing.T) {
	safely(t, func() {
		d, errs := ParseLayer(rec("r.hcl", "service \"web\" {\n  require = [\"a\"]\n}\nservice \"web\" {\n  want = [\"b\"]\n}\n"))
		if len(errs) != 0 {
			t.Fatal(errs)
		}
		m := d.Services["web"]
		if !m["edge/a"].Set || !m["edge/b"].Set {
			t.Errorf("two blocks of one service must both count: %+v", m)
		}
	})
}

func TestGuardLayersOfOneServiceMeet(t *testing.T) {
	safely(t, func() {
		l := plus(plus(base(),
			site("t1.hcl", "service \"web\" {\n  stop {\n    timeout = { min = \"10s\", max = \"40s\", prefer = \"20s\" }\n  }\n}")),
			site("t2.hcl", "service \"web\" {\n  kill {\n    timeout = \"5s\"\n  }\n}"))
		p := mustCompile(t, l...)
		r := p.Services["web"].Resolved
		if r["stop.timeout"] != 20e9 || r["kill.timeout"] != 5e9 {
			t.Errorf("both site layers count: %v", r)
		}
		l = plus(plus(base(),
			rec("r2.hcl", "service \"web\" {\n  stop {\n    timeout = { min = \"10s\", max = \"20s\", prefer = \"15s\" }\n  }\n}")),
			rec("r3.hcl", "service \"web\" {\n  kill {\n    timeout = { min = \"5s\", max = \"9s\" }\n  }\n}"))
		p = mustCompile(t, l...)
		r = p.Services["web"].Resolved
		if r["stop.timeout"] != 15e9 || r["kill.timeout"] != 5e9 {
			t.Errorf("both recipe layers count: %v", r)
		}
	})
}

func TestGuardCycleComponentIsExact(t *testing.T) {
	safely(t, func() {
		// c hangs off the cycle a <-> b and is not part of it
		l := []Layer{
			rec("recipe.hcl", svcRecipe("a", `require = ["b"]`)+svcRecipe("b", `want = ["a", "c"]`)+svcRecipe("c", "")),
			site("site.hcl", baseline+svcSite("a", "")+svcSite("b", "")+svcSite("c", "")),
		}
		e := wantErr[ErrCycle](t, l)
		if strings.Join(e.Services, ",") != "a,b" {
			t.Errorf("%+v", e)
		}
	})
}

func TestGuardUnknownTargets(t *testing.T) {
	safely(t, func() {
		e := wantErr[ErrUnknownService](t, twoServices("", `require = ["nowhere"]`))
		if e.Service != "nowhere" {
			t.Errorf("%+v", e)
		}
	})
	safely(t, func() {
		e := wantErr[ErrUnknownService](t, grantsLayers(asOps(), "", "grant \"ops\" {\n    services = [\"a\", \"zzz\"]\n  }"))
		if e.Service != "zzz" {
			t.Errorf("%+v", e)
		}
	})
}
