package supervisor

import (
	"reflect"
	"testing"
)

// FuzzParse: no input may panic the parser, and a document parses to the same
// thing twice.
func FuzzParse(f *testing.F) {
	f.Add(svcRecipe("web", ""))
	f.Add(baseline + svcSite("web", ""))
	f.Fuzz(func(t *testing.T, src string) {
		for _, a := range []Authority{Recipe, Site} {
			l := Layer{Name: "f.hcl", Authority: a, Admitted: true, Source: []byte(src)}
			d1, e1 := ParseLayer(l)
			d2, e2 := ParseLayer(l)
			if len(e1) != len(e2) || !reflect.DeepEqual(d1, d2) {
				t.Fatalf("parse is not deterministic for %q", src)
			}
		}
	})
}

// FuzzCompile: no pair of layers may panic the compiler, the verdict does not
// depend on the order of the layers, and an accepted plan holds no conflict.
func FuzzCompile(f *testing.F) {
	f.Add(svcRecipe("web", ""), baseline+svcSite("web", ""))
	f.Add(svcRecipe("web", ""), baseline+svcSite("web", `stop { timeout = "20s" }`))
	f.Fuzz(func(t *testing.T, recipe, siteSrc string) {
		ls := []Layer{rec("r.hcl", recipe), site("s.hcl", siteSrc)}
		p1, e1 := compile(ls...)
		p2, e2 := compile(ls[1], ls[0])
		if (e1 == nil) != (e2 == nil) || (e1 != nil && e1.Error() != e2.Error()) {
			t.Fatalf("order changed the verdict:\n%v\n%v", e1, e2)
		}
		if e1 != nil {
			return
		}
		if !reflect.DeepEqual(p1, p2) {
			t.Fatal("order changed the plan")
		}
		for s, sp := range p1.Services {
			for f, v := range sp.Fields {
				if v.IsErr() {
					t.Fatalf("an accepted plan holds a conflict at %s/%s", s, f)
				}
			}
		}
	})
}
