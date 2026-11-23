package supervisor

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const sec = int64(time.Second)

func TestCompileHappyPath(t *testing.T) {
	p := mustCompile(t, base()...)
	s := p.Services["web"]
	if s == nil || p.Schema != 1 {
		t.Fatalf("plan: %+v", p)
	}
	// recipe [5s,120s] met with the baseline [1s,60s] is [5s,60s]; prefer 30s.
	if got := s.Resolved["stop.timeout"]; got != 30*sec {
		t.Errorf("stop.timeout = %d, want 30s", got)
	}
	if s.Source["stop.timeout"] != "recipe-prefer" {
		t.Errorf("source = %q", s.Source["stop.timeout"])
	}
	if s.Resolved["kill.timeout"] != 5*sec || s.Source["kill.timeout"] != "singleton" {
		t.Errorf("kill: %d %q", s.Resolved["kill.timeout"], s.Source["kill.timeout"])
	}
	// a field no layer mentions takes the baseline's value, never the universe (C1).
	if v := s.Fields["allow.capabilities"]; !v.Set || len(v.Strs) != 2 {
		t.Errorf("allow.capabilities = %+v", v)
	}
	if v := s.Fields["limits.nofile"]; !v.Set || v.Lo != 1024 {
		t.Errorf("limits.nofile = %+v", v)
	}
}

func TestSiteNarrowsAndMovesPrefer(t *testing.T) {
	// A site point inside the interval wins (D-M2): [5s,120s] meet "60s".
	l := plus(base(), site("tune.hcl", `service "web" {
  stop {
    timeout = "20s"
  }
}`))
	if got := mustCompile(t, l...).Services["web"].Resolved["stop.timeout"]; got != 20*sec {
		t.Errorf("point: %d", got)
	}
	// A site prefer moves the point inside the met interval.
	l = plus(base(), site("tune.hcl", `service "web" {
  stop {
    timeout = { min = "10s", max = "40s", prefer = "15s" }
  }
}`))
	p := mustCompile(t, l...).Services["web"]
	if p.Resolved["stop.timeout"] != 15*sec || p.Source["stop.timeout"] != "site-prefer" {
		t.Errorf("prefer: %d %q", p.Resolved["stop.timeout"], p.Source["stop.timeout"])
	}
	// A site prefer outside the met interval is a conflict: it may not widen.
	l = plus(base(), site("tune.hcl", `service "web" {
  stop {
    timeout = { min = "10s", max = "100s", prefer = "90s" }
  }
}`))
	wantErr[ErrConflict](t, l) // the met interval is [10s,60s]; 90s is outside
	// Two site layers preferring different points disagree.
	l = plus(plus(base(), site("a.hcl", `service "web" {
  stop {
    timeout = { min = "10s", max = "40s", prefer = "15s" }
  }
}`)), site("b.hcl", `service "web" {
  stop {
    timeout = { min = "10s", max = "40s", prefer = "25s" }
  }
}`))
	wantErr[ErrConflict](t, l)
	// A site cannot widen: the empty meet is a conflict.
	l = plus(base(), site("tune.hcl", `service "web" {
  stop {
    timeout = "200s"
  }
}`))
	wantErr[ErrConflict](t, l)
}

func TestIncompleteWhenNoPreferInsideInterval(t *testing.T) {
	// The recipe's prefer (30s) falls outside the met interval [40s,60s] and no
	// site chose a point.
	l := plus(base(), site("tune.hcl", `service "web" {
  stop {
    timeout = { min = "40s", max = "60s" }
  }
}`))
	e := wantErr[ErrIncomplete](t, l)
	if e.Path != "web/stop.timeout" {
		t.Errorf("path %q", e.Path)
	}
	// Two recipe prefers inside the interval are a conflict.
	r := with(base(), "recipe.hcl", svcRecipe("web", "")+`service "web" {
  stop {
    timeout = { min = "5s", max = "120s", prefer = "31s" }
  }
}`)
	wantErr[ErrConflict](t, r)
}

func TestNoBaseline(t *testing.T) {
	l := []Layer{rec("recipe.hcl", svcRecipe("web", "")), site("site.hcl", "schema = 1\n"+svcSite("web", ""))}
	wantErr[ErrNoBaseline](t, l)
	wantErr[ErrNoBaseline](t, nil)
	// a recipe cannot supply the baseline
	wantErr[ErrFieldAuthority](t, []Layer{rec("r.hcl", svcRecipe("web", "")+"all {\n}\n")})
}

func TestBaselineNotTotal(t *testing.T) {
	cut := func(s string) string { return strings.Replace(baseline, s, "", 1) }
	cases := map[string]string{
		"limits.nproc":               "    nproc  = 100\n",
		"allow.syscalls":             `    syscalls         = ["read", "write", "futex"]` + "\n",
		"allow.read_write":           `    read_write       = ["/var/lib"]` + "\n",
		"protect/seccomp":            "    level     = \"waived\"\n",
		"protect/landlock/exempt_on": `    exempt_on = ["darwin", "windows"]` + "\n    level     = \"required\"\n",
		"api.read":                   "  read  = [\"alice\", \"bob\"]\n",
	}
	for field, c := range cases {
		t.Run(field, func(t *testing.T) {
			src := cut(c)
			if field == "protect/landlock/exempt_on" {
				src = strings.Replace(baseline, `    level     = "required"
    exempt_on = ["darwin", "windows"]
`, `    level     = "required"
`, 1)
			}
			l := with(base(), "site.hcl", src+svcSite("web", ""))
			e := wantErr[ErrBaselineNotTotal](t, l)
			found := false
			for _, f := range e.Fields {
				found = found || f == field
			}
			if !found {
				t.Errorf("%v does not name %s", e.Fields, field)
			}
		})
	}
	// totality is checked on the meet: a second site layer completes it.
	half := strings.Replace(baseline, `    nproc  = 100
`, "", 1)
	l := plus(with(base(), "site.hcl", half+svcSite("web", "")), site("more.hcl", "all {\n  limits {\n    nproc = 50\n  }\n}\n"))
	mustCompile(t, l...)
}

func TestSchemaVersion(t *testing.T) {
	l := with(base(), "site.hcl", strings.Replace(baseline, "schema = 1", "schema = 2", 1)+svcSite("web", ""))
	e := wantErr[ErrSchemaTooNew](t, l)
	if e.Baseline != 2 || e.Running != 1 {
		t.Errorf("%+v", e)
	}
	// no schema statement: required
	l = with(base(), "site.hcl", strings.Replace(baseline, "schema = 1", "", 1)+svcSite("web", ""))
	wantErr[ErrIncomplete](t, l)
	// two site layers disagreeing about the schema
	l = plus(base(), site("other.hcl", "schema = 2\n"))
	wantErr[ErrConflict](t, l)
	// a recipe cannot state the schema
	wantErr[ErrFieldAuthority](t, with(base(), "recipe.hcl", "schema = 1\n"+svcRecipe("web", "")))
	for _, bad := range []string{"schema = 0", `schema = "x"`, "schema = 1.5"} {
		wantErr[ErrNotALatticeValue](t, plus(base(), site("s.hcl", bad)))
	}
}

// A schema one version newer, with a K5 field, a K3 field and a protection.
func schemaV2() *schema {
	sc := defaultSchema().clone()
	sc.fields["allow.ioctls"] = fieldDef{Kind: K5, Class: ClassH, Since: 2, Req: ReqHardening}
	sc.fields["limits.sigpending"] = fieldDef{Kind: K3, Class: ClassH, Since: 2, Req: ReqHardening}
	sc.protections = append(sc.protections, protDef{Name: "mountns", Since: 2})
	return sc
}

func TestSchemaEvolutionNeverWidens(t *testing.T) {
	// Theorem E: a schema-1 baseline under a schema-2 binary is accepted, the
	// new fields take their legacy values and are tagged.
	p, err := Compile(base(), Options{Running: 2, sc: schemaV2()})
	if err != nil {
		t.Fatal(err)
	}
	s := p.Services["web"]
	want := []string{"allow.ioctls", "limits.sigpending", "protect/mountns", "protect/mountns/exempt_on"}
	if strings.Join(s.Legacy, ",") != strings.Join(want, ",") {
		t.Errorf("legacy = %v, want %v", s.Legacy, want)
	}
	if v := s.Fields["protect/mountns"]; !v.Set || v.Lo != LevelWaived {
		t.Errorf("a new protection resolves to waived: %+v", v)
	}
	if v := s.Fields["allow.ioctls"]; v.Set {
		t.Errorf("a new allow-set resolves to the universe: %+v", v)
	}
	// A recipe that states the new field narrows below the legacy value and is
	// not tagged legacy.
	r := with(base(), "recipe.hcl", svcRecipe("web", `allow {
    ioctls = ["TCGETS"]
  }`))
	p, err = Compile(r, Options{Running: 2, sc: schemaV2()})
	if err != nil {
		t.Fatal(err)
	}
	if v := p.Services["web"].Fields["allow.ioctls"]; !v.Set || len(v.Strs) != 1 {
		t.Errorf("recipe value kept: %+v", v)
	}
	for _, l := range p.Services["web"].Legacy {
		if l == "allow.ioctls" {
			t.Error("a stated field is not legacy")
		}
	}
	// A baseline that declares schema 2 must state the new fields.
	b2 := strings.Replace(baseline, "schema = 1", "schema = 2", 1)
	e := func() ErrBaselineNotTotal {
		_, err := Compile(with(base(), "site.hcl", b2+svcSite("web", "")), Options{Running: 2, sc: schemaV2()})
		var e ErrBaselineNotTotal
		if !errors.As(err, &e) {
			t.Fatalf("got %v", err)
		}
		return e
	}()
	for _, f := range []string{"allow.ioctls", "limits.sigpending", "protect/mountns", "protect/mountns/exempt_on"} {
		if !strings.Contains(strings.Join(e.Fields, ","), f) {
			t.Errorf("%s missing from %v", f, e.Fields)
		}
	}
	// The parser of a schema-1 binary does not know the field.
	wantErr[ErrUnknownKey](t, with(base(), "recipe.hcl", svcRecipe("web", `allow {
    ioctls = ["TCGETS"]
  }`)))
}

func TestWideningFields(t *testing.T) {
	cases := map[string]struct{ src, field string }{
		"exec":        {`exec = ["/bin/evil"]`, "exec"},
		"run_as":      {"run_as {\n    user = \"root\"\n  }", "run_as.user"},
		"working_dir": {`working_dir = "/"`, "working_dir"},
		"env key":     {`env = { LD_PRELOAD = "/tmp/x.so" }`, "env/LD_PRELOAD"},
		"kind":        {`kind = "oneshot"`, "kind"},
		"readiness":   {"readiness {\n    channel = \"none\"\n  }", "readiness.channel"},
		"signal":      {"stop {\n    signal = \"INT\"\n  }", "stop.signal"},
		"credential":  {"credential \"db\" {\n  }", "cred/db"},
		"param decl":  {"param \"other\" {\n    type = \"port\"\n  }", "paramdecl/other"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			l := plus(base(), site("evil.hcl", "service \"web\" {\n  "+c.src+"\n}\n"))
			e := wantErr[ErrWideningField](t, l)
			if e.Field != c.field || e.Service != "web" || e.Layer != "evil.hcl" {
				t.Errorf("%+v", e)
			}
		})
	}
	// a new service key
	e := wantErr[ErrWideningField](t, plus(base(), site("new.hcl", svcSite("ghost", ""))))
	if e.Field != "service" || e.Service != "ghost" {
		t.Errorf("%+v", e)
	}
	// restating a class-D value is idempotent
	mustCompile(t, plus(base(), site("same.hcl", `service "web" {
  kind = "simple"
  readiness {
    channel = "fd"
  }
}`))...)
}

func TestFieldAuthority(t *testing.T) {
	recipeCases := map[string]string{
		"enabled":  `enabled = true`,
		"terminal": `terminal = false`,
		"params":   `params = { port = "1" }`,
		"cred src": "credential \"db\" {\n    source = \"x\"\n  }",
		"api":      "}\napi {\n  admin = [\"x\"]\n",
	}
	for name, extra := range recipeCases {
		t.Run("recipe "+name, func(t *testing.T) {
			e := wantErr[ErrFieldAuthority](t, with(base(), "recipe.hcl", svcRecipe("web", extra)))
			if e.Authority != "recipe" {
				t.Errorf("%+v", e)
			}
		})
	}
	allCases := map[string]string{
		"exec":        `exec = ["/x"]`,
		"env":         `env = { A = "b" }`,
		"require":     `require = ["web"]`,
		"enabled":     `enabled = true`,
		"needs":       "needs {\n    capabilities = [\"CAP_CHOWN\"]\n  }",
		"free prefer": "stop {\n    timeout = { min = \"1s\", max = \"2s\", prefer = \"1s\" }\n  }",
		"param":       "param \"p\" {\n    type = \"port\"\n  }",
		"credential":  "credential \"p\" {\n  }",
	}
	for name, extra := range allCases {
		t.Run("all "+name, func(t *testing.T) {
			src := baseline + "all {\n  " + extra + "\n}\n" + svcSite("web", "")
			e := wantErr[ErrFieldAuthority](t, with(base(), "site.hcl", src))
			if e.Authority != "all" {
				t.Errorf("%+v", e)
			}
		})
	}
	// a point in all {} states an interval without a prefer: allowed.
	src := baseline + "all {\n  start {\n    timeout = \"20s\"\n  }\n}\n" + svcSite("web", "")
	mustCompile(t, with(base(), "site.hcl", src)...)
}

func TestAdmission(t *testing.T) {
	l := base()
	l[0].Admitted = false
	if e := wantErr[ErrUnsignedRecipe](t, l); e.Layer != "recipe.hcl" {
		t.Errorf("%+v", e)
	}
	l = base()
	l[1].Admitted = false
	if e := wantErr[ErrUnsafePath](t, l); e.Path != "site.hcl" {
		t.Errorf("%+v", e)
	}
	l = base()
	l[1].Authority = 0
	wantErr[ErrNotALatticeValue](t, l)
}

func TestFlatConflict(t *testing.T) {
	l := plus(base(), rec("other.hcl", `service "web" {
  exec = ["/bin/other"]
}`))
	e := wantErr[ErrConflict](t, l)
	if e.Path != "web/exec" || len(e.Layers) != 2 || len(e.Values) != 2 {
		t.Errorf("%+v", e)
	}
	// restating an equal value in a second recipe is idempotent
	mustCompile(t, plus(base(), rec("dup.hcl", `service "web" {
  kind = "simple"
}`))...)
	// a global restart.on is a K1 assertion: it conflicts with a recipe that differs
	l = with(base(), "site.hcl", strings.Replace(baseline, "all {", "all {\n  restart {\n    on = \"always\"\n  }", 1)+svcSite("web", ""))
	wantErr[ErrConflict](t, l)
}

func TestNeedsWithinAllows(t *testing.T) {
	l := with(base(), "recipe.hcl", svcRecipe("web", `needs {
    address_families = ["inet6"]
  }`))
	e := wantErr[ErrConflict](t, l)
	if e.Path != "web/needs.address_families" || len(e.Values) != 1 || e.Values[0] != "inet6" {
		t.Errorf("%+v", e)
	}
	// a recipe narrowing allows below its own needs
	l = with(base(), "recipe.hcl", svcRecipe("web", `needs {
    capabilities = ["CAP_CHOWN"]
  }
  allow {
    capabilities = ["CAP_NET_BIND_SERVICE"]
  }`))
	wantErr[ErrConflict](t, l)
	mustCompile(t, with(base(), "recipe.hcl", svcRecipe("web", `needs {
    capabilities = ["CAP_CHOWN"]
  }`))...)
}

func TestHardeningOnlyShrinks(t *testing.T) {
	l := plus(base(), site("tight.hcl", `all {
  limits {
    nofile = 512
  }
  allow {
    capabilities = ["CAP_CHOWN", "CAP_KILL"]
  }
  protect "seccomp" {
    level = "required"
  }
}`))
	s := mustCompile(t, l...).Services["web"]
	if s.Fields["limits.nofile"].Lo != 512 {
		t.Errorf("nofile %+v", s.Fields["limits.nofile"])
	}
	if got := s.Fields["allow.capabilities"].Strs; len(got) != 1 || got[0] != "CAP_CHOWN" {
		t.Errorf("capabilities %v", got)
	}
	if s.Fields["protect/seccomp"].Lo != LevelRequired {
		t.Errorf("seccomp %+v", s.Fields["protect/seccomp"])
	}
	// a looser statement changes nothing
	l = plus(base(), site("loose.hcl", `all {
  limits {
    nofile = 99999
  }
  protect "nnp" {
    level = "waived"
  }
}`))
	s = mustCompile(t, l...).Services["web"]
	if s.Fields["limits.nofile"].Lo != 1024 || s.Fields["protect/nnp"].Lo != LevelRequired {
		t.Errorf("a loose layer widened: %+v %+v", s.Fields["limits.nofile"], s.Fields["protect/nnp"])
	}
	// the literal false is not a lattice value
	l = plus(base(), site("false.hcl", "all {\n  protect \"nnp\" {\n    level = false\n  }\n}"))
	wantErr[ErrNotALatticeValue](t, l)
}

func TestLiteralsOnly(t *testing.T) {
	for name, expr := range map[string]string{
		"variable": `kind = var.x`,
		"function": `kind = upper("simple")`,
		"operator": `working_dir = "a" == "b"`,
		"interp":   `working_dir = "${var.x}"`,
		"parens":   `working_dir = ("a")`,
	} {
		t.Run(name, func(t *testing.T) {
			src := svcRecipe("web", "") + "service \"x\" {\n  " + expr + "\n}\n"
			wantErr[ErrNotLiteral](t, with(base(), "recipe.hcl", src))
		})
	}
	wantErr[ErrSyntax](t, with(base(), "recipe.hcl", `service "web" {`))
	wantErr[ErrUnknownKey](t, with(base(), "recipe.hcl", svcRecipe("web", "bogus = 1")))
	wantErr[ErrUnknownKey](t, with(base(), "recipe.hcl", svcRecipe("web", "stop {\n    bogus = 1\n  }")))
	wantErr[ErrUnknownKey](t, with(base(), "recipe.hcl", "weird \"x\" {\n}\n"))
	wantErr[ErrUnknownKey](t, with(base(), "recipe.hcl", svcRecipe("web", "protect \"nope\" {\n  }")))
	wantErr[ErrNotALatticeValue](t, with(base(), "recipe.hcl", svcRecipe("web", `stop {
    timeout = { min = "9s", max = "1s" }
  }`)))
	wantErr[ErrNotALatticeValue](t, with(base(), "recipe.hcl", svcRecipe("web", `stop {
    timeout = { min = "1s", max = "9s", prefer = "20s" }
  }`)))
	wantErr[ErrNotALatticeValue](t, with(base(), "recipe.hcl", svcRecipe("web", `stop {
    timeout = { min = "1s" }
  }`)))
	wantErr[ErrNotALatticeValue](t, with(base(), "recipe.hcl", svcRecipe("web", `stop {
    timeout = "forever"
  }`)))
	wantErr[ErrNotALatticeValue](t, with(base(), "recipe.hcl", svcRecipe("web", `limits {
    nofile = -1
  }`)))
	wantErr[ErrNotALatticeValue](t, with(base(), "recipe.hcl", svcRecipe("web", `restart {
    on = "sometimes"
  }`)))
}

func TestForkingDaemon(t *testing.T) {
	r := strings.Replace(svcRecipe("web", ""), `kind = "simple"`, `kind = "forking"`, 1)
	wantErr[ErrForkingDaemon](t, with(base(), "recipe.hcl", r))
}

func TestSecretInEnvironment(t *testing.T) {
	r := with(base(), "recipe.hcl", svcRecipe("web", `env = { DB_PASSWORD = "x" }`))
	if e := wantErr[ErrSecretInEnvironment](t, r); e.Key != "DB_PASSWORD" {
		t.Errorf("%+v", e)
	}
	mustCompile(t, with(base(), "recipe.hcl", svcRecipe("web", `env = { LANG = "C" }`))...)
}

func TestRunAsInUserMode(t *testing.T) {
	r := with(base(), "recipe.hcl", svcRecipe("web", "run_as {\n    user = \"daemon\"\n  }"))
	e := wantErr[ErrRunAsInUserMode](t, r, Options{UserMode: true, Owner: "alice"})
	if e.RunAs != "daemon" || e.Owner != "alice" {
		t.Errorf("%+v", e)
	}
	if _, err := Compile(r, Options{UserMode: true, Owner: "daemon"}); err != nil {
		t.Error(err)
	}
	if _, err := Compile(r, Options{}); err != nil {
		t.Error(err)
	}
}

func TestParams(t *testing.T) {
	// untyped: no type, or a free-form string
	for _, decl := range []string{"param \"port\" {\n  }", "param \"port\" {\n    type = \"string\"\n  }"} {
		src := strings.Replace(svcRecipe("web", ""), "param \"port\" {\n    type = \"port\"\n  }", decl, 1)
		wantErr[ErrUntypedParam](t, with(base(), "recipe.hcl", src))
	}
	// a placeholder naming no declared parameter
	wantErr[ErrUntypedParam](t, with(base(), "recipe.hcl", svcRecipe("web", `env = { X = "{{nope}}" }`)))
	// a value outside the domain
	for _, v := range []string{"0", "70000", "http", "-1", ""} {
		src := baseline + svcSite("web", "") + `service "web" {
  params = { port = "` + v + `" }
}`
		if v == "" {
			continue
		}
		wantErr[ErrConflict](t, with(base(), "site.hcl", src))
	}
	// the value is required
	wantErr[ErrIncomplete](t, with(base(), "site.hcl", baseline+"service \"web\" {\n  enabled = true\n  terminal = false\n}\n"))
	// a value for an undeclared parameter
	wantErr[ErrUnknownKey](t, plus(base(), site("p.hcl", "service \"web\" {\n  params = { ghost = \"1\" }\n}")))
	// the other domains
	dom := func(decl, v string) []Layer {
		r := strings.Replace(svcRecipe("web", ""), "param \"port\" {\n    type = \"port\"\n  }", decl, 1)
		r = strings.ReplaceAll(r, "{{port}}", "{{p}}")
		r = strings.Replace(r, `param "p"`, `param "p"`, 1)
		s := baseline + `service "web" {
  enabled = true
  terminal = false
  params = { p = "` + v + `" }
}`
		return []Layer{rec("recipe.hcl", r), site("site.hcl", s)}
	}
	ok := func(decl, v string) {
		t.Helper()
		mustCompile(t, dom(decl, v)...)
	}
	bad := func(decl, v string) {
		t.Helper()
		wantErr[ErrConflict](t, dom(decl, v))
	}
	intDom := "param \"p\" {\n    type = \"int\"\n    min = 2\n    max = 4\n  }"
	ok(intDom, "2")
	ok(intDom, "4")
	bad(intDom, "1")
	bad(intDom, "5")
	bad(intDom, "x")
	zeroDom := "param \"p\" {\n    type = \"int\"\n    min = 0\n    max = 4\n  }"
	bad(zeroDom, "x") // not a number is not the number 0
	bad(zeroDom, "")
	zeroDur := "param \"p\" {\n    type = \"duration\"\n    min = \"0s\"\n    max = \"4s\"\n  }"
	bad(zeroDur, "soon")
	enumDom := "param \"p\" {\n    type = \"enum\"\n    values = [\"a\", \"b\"]\n  }"
	ok(enumDom, "a")
	bad(enumDom, "c")
	durDom := "param \"p\" {\n    type = \"duration\"\n    min = \"1s\"\n    max = \"2s\"\n  }"
	ok(durDom, "1500ms")
	bad(durDom, "3s")
	bad(durDom, "soon")
	pathDom := "param \"p\" {\n    type = \"path\"\n  }"
	ok(pathDom, "/var/lib/x-1.2")
	for _, v := range []string{"/a b", "/a;b", "-rf", "/a$b", "/a`b", "/a\\nb", "/a'b"} {
		bad(pathDom, v)
	}
	ok("param \"p\" {\n    type = \"port\"\n  }", "65535")
}

func TestParamInShell(t *testing.T) {
	mk := func(exec string) []Layer {
		src := strings.Replace(svcRecipe("web", ""), `exec = ["/bin/web", "--port", "{{port}}"]`, "exec = "+exec, 1)
		return with(base(), "recipe.hcl", src)
	}
	wantErr[ErrParamInShell](t, mk(`["/bin/sh", "-c", "echo {{port}}"]`))
	wantErr[ErrParamInShell](t, mk(`["bash", "-c", "x {{port}}"]`))
	wantErr[ErrParamInShell](t, mk(`["C:\\Windows\\System32\\cmd.exe", "/c", "echo", "{{port}}"]`))
	wantErr[ErrParamInShell](t, mk(`["pwsh", "-c", "a", "{{port}}"]`))
	// positional arguments after the command string stay allowed
	mustCompile(t, mk(`["/bin/sh", "-c", "exec x \"$1\"", "sh", "{{port}}"]`)...)
	mustCompile(t, mk(`["/bin/sh", "echo", "{{port}}", "-c"]`)...)
	// not a shell
	mustCompile(t, mk(`["/bin/web", "-c", "{{port}}"]`)...)
}

func TestCheckTiming(t *testing.T) {
	chk := func(timeout, period string, rise, fall int) []Layer {
		return with(base(), "recipe.hcl", svcRecipe("web", "check {\n    timeout = \""+timeout+"\"\n    period = \""+period+
			"\"\n    rise = "+itoa(rise)+"\n    fall = "+itoa(fall)+"\n  }"))
	}
	mustCompile(t, chk("1s", "5s", 1, 1)...)
	wantErr[ErrCheckTiming](t, chk("5s", "5s", 1, 1))
	wantErr[ErrCheckTiming](t, chk("6s", "5s", 1, 1))
	wantErr[ErrCheckTiming](t, chk("0s", "5s", 1, 1))
	wantErr[ErrCheckTiming](t, chk("1s", "5s", 0, 1))
	wantErr[ErrCheckTiming](t, chk("1s", "5s", 1, 0))
	// a partial check block is incomplete
	wantErr[ErrIncomplete](t, with(base(), "recipe.hcl", svcRecipe("web", "check {\n    period = \"5s\"\n  }")))
}

func itoa(n int) string { return strings.TrimSpace(strings.Repeat(" ", 0) + string(rune('0'+n))) }

func twoServices(extraB, extraA string) []Layer {
	return []Layer{
		rec("recipe.hcl", svcRecipe("a", extraA)+svcRecipe("b", extraB)),
		site("site.hcl", baseline+svcSite("a", "")+svcSite("b", "")),
	}
}

func TestCycleAndUnknownService(t *testing.T) {
	e := wantErr[ErrCycle](t, twoServices(`require = ["a"]`, `want = ["b"]`))
	if strings.Join(e.Services, ",") != "a,b" {
		t.Errorf("%+v", e)
	}
	// every edge kind takes part, and order edges too
	wantErr[ErrCycle](t, twoServices(`order = ["a"]`, `order = ["b"]`))
	wantErr[ErrCycle](t, twoServices("", `order = ["a"]`)) // self edge
	e2 := wantErr[ErrUnknownService](t, twoServices("", `require = ["nowhere"]`))
	if e2.Service != "nowhere" || e2.Referrer != "a" {
		t.Errorf("%+v", e2)
	}
	// a site layer may add an edge; it adds activation, never privilege
	l := plus(twoServices("", ""), site("edge.hcl", `service "a" {
  want = ["b"]
}`))
	mustCompile(t, l...)
	// ... and it may make a cycle, which is refused
	l = plus(twoServices("", `want = ["b"]`), site("edge.hcl", `service "b" {
  order = ["a"]
}`))
	wantErr[ErrCycle](t, l)
	// edges meet: require below want below order
	l = plus(twoServices("", `order = ["b"]`), site("edge.hcl", `service "a" {
  require = ["b"]
  want = ["b"]
  wait {
    timeout = "10m"
  }
}`))
	if lv := mustCompile(t, l...).Services["a"].Fields["edge/b"].Lo; lv != LevelRequired {
		t.Errorf("edge level %d", lv)
	}
}

func TestWaitBudget(t *testing.T) {
	// a requires b. U(b) = W(b) + SW(b) + stop + kill + backoff + start
	//   = 0 + (SW(a)+stop(a)+kill(a)+eps) + 30s + 5s + 10s + 10s
	// SW(a) = eps, stop(a) = 30s, kill(a) = 5s.
	need := (Epsilon + 30*sec + 5*sec + Epsilon) + 30*sec + 5*sec + 10*sec + 10*sec
	mk := func(wait string) []Layer {
		return twoServices("", "require = [\"b\"]\n  wait {\n    timeout = \""+wait+"\"\n  }")
	}
	e := wantErr[ErrWaitTooShort](t, mk("1s"))
	if e.Service != "a" || e.Need != need || e.Have != 1*sec {
		t.Errorf("%+v want need %d", e, need)
	}
	wantErr[ErrWaitTooShort](t, mk(time.Duration(need-1).String()))
	mustCompile(t, mk(time.Duration(need).String())...)
	// wait.timeout is required when there is a require edge
	wantErr[ErrIncomplete](t, twoServices("", `require = ["b"]`))
	// but not for want or order edges
	mustCompile(t, twoServices("", `want = ["b"]`)...)
	mustCompile(t, twoServices("", `order = ["b"]`)...)
}

// An unresolved timing field is reported once, as itself; the budget that would
// read it is not computed with a made-up zero.
func TestUnresolvedTimingSkipsTheBudget(t *testing.T) {
	noPref := strings.Replace(svcRecipe("b", ""), `timeout = { min = "5s", max = "120s", prefer = "30s" }`,
		`timeout = { min = "5s", max = "120s" }`, 1)
	l := []Layer{
		rec("recipe.hcl", svcRecipe("a", "require = [\"b\"]\n  wait {\n    timeout = \"1ms\"\n  }")+noPref),
		site("site.hcl", baseline+svcSite("a", "")+svcSite("b", "")),
	}
	_, err := compile(l...)
	var ce *CompileError
	if !errors.As(err, &ce) || len(ce.Errs) != 1 {
		t.Fatalf("want one refusal, got %v", err)
	}
	var e ErrIncomplete
	if !errors.As(err, &e) || e.Path != "b/stop.timeout" {
		t.Errorf("got %v", err)
	}
}

// slowKill is a recipe service whose kill.timeout is 40s instead of 5s.
func slowKill(name string) string {
	return strings.Replace(svcRecipe(name, ""), "kill {\n    timeout = \"5s\"", "kill {\n    timeout = \"40s\"", 1)
}

func TestWaitBudgetRecursesOverEveryEdge(t *testing.T) {
	// REVIEW-3 D6: a requires b, b wants c; c's attempt is inside U(b).
	three := func(wait string) []Layer {
		return []Layer{
			rec("recipe.hcl", svcRecipe("a", "require = [\"b\"]\n  wait {\n    timeout = \""+wait+"\"\n  }")+
				svcRecipe("b", `want = ["c"]`)+slowKill("c")),
			site("site.hcl", baseline+svcSite("a", "")+svcSite("b", "")+svcSite("c", "")),
		}
	}
	// without c, U(b) is well under 120s; c's 40s kill timeout pushes W(b) up.
	wantErr[ErrWaitTooShort](t, three("100s"))
	mustCompile(t, three("10m")...)
	// an order edge counts as well
	l := []Layer{
		rec("recipe.hcl", svcRecipe("a", "require = [\"b\"]\n  wait {\n    timeout = \"100s\"\n  }")+
			svcRecipe("b", `order = ["c"]`)+slowKill("c")),
		site("site.hcl", baseline+svcSite("a", "")+svcSite("b", "")+svcSite("c", "")),
	}
	wantErr[ErrWaitTooShort](t, l)
}

func grantsLayers(extraRecipeA, extraRecipeB, grant string) []Layer {
	b := strings.Replace(baseline, `admin = ["alice"]`, `admin = ["alice", "ops"]`, 1)
	b = strings.Replace(b, "api {", "api {\n  "+grant, 1)
	return []Layer{
		rec("recipe.hcl", svcRecipe("a", extraRecipeA)+svcRecipe("b", extraRecipeB)),
		site("site.hcl", b+svcSite("a", "")+svcSite("b", "")),
	}
}

const grantA = "grant \"ops\" {\n    services = [\"a\"]\n  }"

func asOps() string { return "run_as {\n    user = \"ops\"\n  }" }

func TestGrantsRunAsBothWays(t *testing.T) {
	// consistent: a is granted to ops and runs as ops; b does not
	mustCompile(t, grantsLayers(asOps(), "", grantA)...)
	// E1, one way: granted but runs as another uid
	e := wantErr[ErrDelegationUid](t, grantsLayers("", "", grantA))
	if e.Service != "a" || e.Principal != "ops" || e.Direction != "granted-not-p" {
		t.Errorf("%+v", e)
	}
	// E1, the other way: runs as ops but is outside grant(ops)
	e = wantErr[ErrDelegationUid](t, grantsLayers(asOps(), asOps(), grantA))
	if e.Service != "b" || e.Direction != "p-not-granted" {
		t.Errorf("%+v", e)
	}
	// an empty grant still delegates p: nothing may run as p
	e = wantErr[ErrDelegationUid](t, grantsLayers(asOps(), "", "grant \"ops\" {\n  }"))
	if e.Direction != "p-not-granted" {
		t.Errorf("%+v", e)
	}
	// a grant to a principal who is not an admin
	wantErr[ErrConflict](t, grantsLayers(asOps(), "", strings.Replace(grantA, "ops", "mallory", 1)))
	// a grant naming no service
	wantErr[ErrUnknownService](t, grantsLayers(asOps(), "", "grant \"ops\" {\n    services = [\"a\", \"zzz\"]\n  }"))
	// no grant, no delegation: run_as is free
	mustCompile(t, grantsLayers(asOps(), asOps(), "")...)
}

func TestGrantClosureOverEveryEdgeKind(t *testing.T) {
	// b reaches the granted a; REVIEW-4 E2 refuses it for every kind, naming it
	for _, kind := range []string{"require", "want", "order"} {
		t.Run(kind, func(t *testing.T) {
			extra := kind + " = [\"a\"]"
			if kind == "require" {
				extra += "\n  wait {\n    timeout = \"10m\"\n  }"
			}
			e := wantErr[ErrGrantNotClosed](t, grantsLayers(asOps(), extra, grantA))
			if e.Principal != "ops" || e.Granted != "a" || e.Requirer != "b" || e.Edge != kind {
				t.Errorf("%+v", e)
			}
		})
	}
	// the requirer inside the grant is fine, and by induction so is a chain
	l := grantsLayers(asOps(), asOps()+"\n  require = [\"a\"]\n  wait {\n    timeout = \"10m\"\n  }",
		"grant \"ops\" {\n    services = [\"a\", \"b\"]\n  }")
	mustCompile(t, l...)
	// the edge the other way (the granted service reaches an outside one) is fine
	mustCompile(t, grantsLayers(asOps()+"\n  want = [\"b\"]", "", grantA)...)
	// a site layer adding the edge is refused as well
	l = plus(grantsLayers(asOps(), "", grantA), site("edge.hcl", `service "b" {
  order = ["a"]
}`))
	wantErr[ErrGrantNotClosed](t, l)
}

func TestTerminalNeverGranted(t *testing.T) {
	l := grantsLayers(asOps(), "", grantA)
	l[1].Source = []byte(strings.Replace(string(l[1].Source), "terminal = false\n  params   = { port = \"8080\" }\n  \n}\n\nservice \"b\"", "terminal = true\n  params   = { port = \"8080\" }\n  \n}\n\nservice \"b\"", 1))
	e := wantErr[ErrFieldAuthority](t, l)
	if e.Authority != "grant" || e.Field != "a.terminal" {
		t.Errorf("%+v", e)
	}
}

func TestErrorsAreOrderIndependentAndAggregated(t *testing.T) {
	l := plus(base(), site("evil.hcl", `service "web" {
  exec = ["/bin/evil"]
  env  = { A = "b" }
}`))
	_, err := compile(l...)
	var ce *CompileError
	if !errors.As(err, &ce) || len(ce.Errs) < 2 {
		t.Fatalf("want an aggregate of refusals, got %v", err)
	}
	rev := []Layer{l[2], l[1], l[0]}
	_, err2 := compile(rev...)
	if err == nil || err2 == nil || err.Error() != err2.Error() {
		t.Errorf("order changed the refusal:\n%v\n%v", err, err2)
	}
}
