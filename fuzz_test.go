package supervisor

import (
	"math/big"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// checkDraft9Invariants states what an accepted plan guarantees about the
// rules of draft 9, independently of how Compile decides them.
func checkDraft9Invariants(t *testing.T, p *Plan) {
	t.Helper()
	granted := map[string]string{}
	for k, g := range p.API {
		pr, ok := strings.CutPrefix(k, "api.grant/")
		if !ok || len(g.Strs) == 0 {
			continue
		}
		if slices.Contains(p.API["api.admin"].Strs, pr) {
			t.Fatalf("accepted: %s is owner-class and delegated", pr)
		}
		for _, s := range g.Strs {
			granted[s] = pr
			c := p.Services[s].Fields["allow.capabilities"]
			if !c.Set || len(c.Strs) != 0 {
				t.Fatalf("accepted: granted %s holds capabilities %v", s, c.Strs)
			}
			for _, f := range budgetFields {
				if !p.API["api.budget/"+pr+"/"+f].Set {
					t.Fatalf("accepted: delegated %s has no budget %s", pr, f)
				}
			}
		}
	}
	for _, r := range p.API["api.read"].Strs {
		if slices.Contains(p.API["api.admin"].Strs, r) {
			continue
		}
		for _, f := range readerBudgets {
			if !p.API["api.budget/"+r+"/"+f].Set {
				t.Fatalf("accepted: reader %s has no budget %s", r, f)
			}
		}
		for s, sp := range p.Services {
			if u := sp.Fields["run_as.user"]; u.Set && len(u.Strs) == 1 && u.Strs[0] == r && granted[s] != r {
				t.Fatalf("accepted: %s runs as reader %s", s, r)
			}
		}
	}
	for s, sp := range p.Services {
		r := sp.Resolved
		mx, ok := r["intensity.max"]
		if !ok {
			continue
		}
		u := r["start.timeout"] + r["stop.timeout"] + r["kill.timeout"] + r["backoff.max"]
		span := new(big.Int).Mul(big.NewInt(mx), big.NewInt(u))
		if big.NewInt(r["intensity.window"]).Cmp(span) <= 0 {
			t.Fatalf("accepted: %s can drain its restart window", s)
		}
	}
}

// FuzzParse: no input may panic the parser, and a document parses to the same
// thing twice.
func FuzzParse(f *testing.F) {
	f.Add(svcRecipe("web", ""))
	f.Add(baseline + svcSite("web", ""))
	f.Add("api {\n budget \"p\" {\n  pids = 2147483647\n  memory = 0\n }\n}\n")
	f.Add("api {\n budget \"p\" {\n  pids = -1\n }\n}\n")
	f.Add(svcRecipe("web", "intensity {\n    max = 3\n    window = \"165s\"\n  }"))
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
	// one seed per draft-9 rule
	f.Add(svcRecipe("web", "intensity {\n    max = 3\n    window = \"165s\"\n  }"), baseline+svcSite("web", ""))
	f.Add(svcRecipe("a", asOps()), strings.Replace(baseline, "api {", "api {\n  "+opsBudget+"\n  "+grantA, 1)+svcSite("a", ""))
	f.Add(svcRecipe("a", asOps()), strings.Replace(strings.Replace(baseline, `admin = ["alice"]`, `admin = ["alice", "ops"]`, 1), "api {", "api {\n  "+grantA, 1)+svcSite("a", ""))
	f.Add(svcRecipe("a", "run_as {\n    user = \"bob\"\n  }"), baseline+svcSite("a", ""))
	f.Add(svcRecipe("a", asOps()+"\n  require = [\"b\"]\n  wait {\n    timeout = \"10m\"\n  }")+svcRecipe("b", ""),
		strings.Replace(baseline, "api {", "api {\n  "+opsBudget+"\n  "+grantA, 1)+svcSite("a", "")+strings.Replace(svcSite("b", ""), "enabled  = true", "enabled  = false", 1))
	f.Add(svcRecipe("web", ""), strings.Replace(baseline, "    refusals    = 10\n", "", 1)+svcSite("web", ""))
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
		checkDraft9Invariants(t, p1)
		for s, sp := range p1.Services {
			for f, v := range sp.Fields {
				if v.IsErr() {
					t.Fatalf("an accepted plan holds a conflict at %s/%s", s, f)
				}
			}
		}
	})
}
