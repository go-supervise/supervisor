package supervisor

import (
	"reflect"
	"strings"
	"testing"
)

// Guards of P0 that the narrower first sweep never mutated.

func TestADuplicateLayerIsOneLayer(t *testing.T) {
	once, err := Compile(base(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	l := base()
	twice, err := Compile(append(l, l...), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(once, twice) {
		t.Error("the same layers given twice compile to another plan")
	}
}

func TestARefusedLayerStopsCompilationThere(t *testing.T) {
	// the only site layer is refused at parse: nothing further is judged, so no
	// second complaint about a missing baseline
	l := []Layer{rec("recipe.hcl", svcRecipe("web", "")), site("site.hcl", "bogus = 1\n")}
	for _, e := range errsOf(mustErr(t, l)) {
		if _, ok := e.(ErrNoBaseline); ok {
			t.Errorf("compilation went on after a refused layer: %v", e)
		}
	}
}

func TestIntervalConflictNamesItsBounds(t *testing.T) {
	l := plus(plus(base(), site("tune.hcl", "service \"web\" {\n  stop {\n    timeout = \"200s\"\n  }\n}")), site("noise.hcl", "api {\n}\n"))
	e := wantErr[ErrConflict](t, l)
	// a layer that does not state the field is not named among those that do
	hasBaseline := false
	for _, n := range e.Layers {
		if n == "noise.hcl" {
			t.Errorf("noise.hcl does not state stop.timeout: %v", e.Layers)
		}
		hasBaseline = hasBaseline || n == "site.hcl"
	}
	if !hasBaseline {
		t.Errorf("the baseline's all block states stop.timeout: %v", e.Layers)
	}
	if len(e.Values) == 0 {
		t.Fatalf("%+v", e)
	}
	for _, v := range e.Values {
		if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
			t.Errorf("value %q is not an interval", v)
		}
	}
}

func TestAFieldNewerThanTheBaselineIsNotRequired(t *testing.T) {
	sc := defaultSchema().clone()
	sc.fields["x.mode"] = fieldDef{Kind: K1, Class: ClassG, Since: 2, Enum: []string{"a"}, Req: ReqAlways}
	if _, err := Compile(base(), Options{Running: 2, sc: sc}); err != nil {
		t.Errorf("a schema-1 baseline must not need a schema-2 field: %v", err)
	}
}

func TestWaitBudgetBindsOnlyServicesWithARequireTarget(t *testing.T) {
	// E9: a only wants b, so no wait budget is owed even though b's kill is slow
	l := []Layer{
		rec("recipe.hcl", svcRecipe("a", "want = [\"b\"]\n  wait {\n    timeout = \"5s\"\n  }")+slowKill("b")),
		site("site.hcl", baseline+svcSite("a", "")+svcSite("b", "")),
	}
	mustCompile(t, l...)
}

func TestUnlabelledBlocksAreRefusedOnce(t *testing.T) {
	for _, src := range []string{
		"service {\n  bogus = 1\n}\n",
		"api {\n  grant {\n    nope = 1\n  }\n}\n",
	} {
		if _, errs := ParseLayer(site("s.hcl", src)); len(errs) != 1 {
			t.Errorf("%q: %d errors: %v", src, len(errs), errs)
		}
	}
}
