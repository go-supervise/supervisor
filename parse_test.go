package supervisor

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// refuses1 asserts that the layer parses to exactly one refusal, of type E.
// "Exactly one" is the point: a guard that stops at the first refusal and the
// guard behind it that would report a second, spurious one are told apart.
func refuses1[E error](t *testing.T, a Authority, src string) E {
	t.Helper()
	d, errs := ParseLayer(Layer{Name: "p.hcl", Authority: a, Admitted: true, Source: []byte(src)})
	var e E
	if d != nil || len(errs) != 1 {
		t.Fatalf("want one %T for %q, got doc=%v errs=%v", e, src, d != nil, errs)
	}
	if !errors.As(errs[0], &e) {
		t.Fatalf("want %T for %q, got %v", e, src, errs[0])
	}
	return e
}

func accepts(t *testing.T, a Authority, src string) *Doc {
	t.Helper()
	d, errs := ParseLayer(Layer{Name: "p.hcl", Authority: a, Admitted: true, Source: []byte(src)})
	if len(errs) != 0 {
		t.Fatalf("refused %q: %v", src, errs)
	}
	return d
}

func svc(body string) string { return "service \"x\" {\n" + body + "\n}\n" }

func TestParseRefusals(t *testing.T) {
	type c struct {
		src string
		key string // the Key (ErrUnknownKey, ErrNotALatticeValue, ErrNotLiteral) when non-empty
	}
	notLit := map[string]c{
		"null":            {svc(`working_dir = null`), "working_dir"},
		"template":        {svc(`working_dir = "a${1+1}"`), "working_dir"},
		"tuple element":   {svc(`require = ["a", 1+1]`), "require"},
		"object value":    {svc(`env = { A = 1+1 }`), "env"},
		"object key":      {svc(`env = { (1+1) = "x" }`), "env"},
		"dotted key":      {svc(`env = { a.b = "x" }`), "env"},
		"variable":        {svc(`working_dir = var.x`), "working_dir"},
		"negated nonlit":  {svc(`limits {` + "\n" + `nofile = -(1)` + "\n}"), "limits.nofile"},
		"bad field":       {svc("limits {\n nofile = var.x\n}"), "limits.nofile"},
		"interval field":  {svc("stop {\n timeout = var.x\n}"), "stop.timeout"},
		"set field":       {svc("allow {\n capabilities = var.x\n}"), "allow.capabilities"},
		"edge":            {svc(`require = [x]`), "require"},
		"env":             {svc(`env = var.x`), "env"},
		"param attribute": {svc("param \"p\" {\n type = var.x\n}"), "param.p.type"},
		"api":             {"api {\n admin = var.x\n}", "api.admin"},
		"credential":      {svc("credential \"c\" {\n source = var.x\n}"), "credential.c.source"},
		"protect":         {svc("protect \"nnp\" {\n level = var.x\n}"), "protect/nnp"},
		"exempt_on":       {svc("protect \"nnp\" {\n exempt_on = var.x\n}"), "protect/nnp/exempt_on"},
		"grant":           {"api {\n grant \"g\" {\n services = var.x\n }\n}", "grant.g"},
		"schema":          {"schema = var.x", "schema"},
	}
	for name, x := range notLit {
		t.Run("not literal/"+name, func(t *testing.T) {
			if e := refuses1[ErrNotLiteral](t, Site, x.src); e.Key != x.key {
				t.Errorf("key %q, want %q", e.Key, x.key)
			}
		})
	}
	bad := map[string]c{
		"label chars":         {"service \"a b\" {\n}", "service"},
		"set not a list":      {svc("allow {\n capabilities = \"x\"\n}"), "allow.capabilities"},
		"set number":          {svc("allow {\n capabilities = [1]\n}"), "allow.capabilities"},
		"set empty string":    {svc("allow {\n capabilities = [\"\"]\n}"), "allow.capabilities"},
		"set nul":             {svc("allow {\n capabilities = [\"a\\u0000b\"]\n}"), "allow.capabilities"},
		"exec string":         {svc(`exec = "x"`), "exec"},
		"exec empty":          {svc(`exec = []`), "exec"},
		"exec number":         {svc(`exec = [1]`), "exec"},
		"kind empty":          {svc(`kind = ""`), "kind"},
		"dir empty":           {svc(`working_dir = ""`), "working_dir"},
		"dir nul":             {svc(`working_dir = "a\u0000"`), "working_dir"},
		"dir fractional":      {svc(`working_dir = 1.5`), "working_dir"},
		"kind nul":            {svc(`kind = "a\u0000b"`), "kind"},
		"kind enum":           {svc(`kind = "daemon"`), "kind"},
		"fractional":          {svc(`terminal = 1.5`), "terminal"},
		"flat object":         {svc(`kind = {}`), "kind"},
		"bound negative":      {svc("limits {\n nofile = -1\n}"), "limits.nofile"},
		"bound string":        {svc("limits {\n nofile = \"1\"\n}"), "limits.nofile"},
		"level false":         {svc("protect \"nnp\" {\n level = false\n}"), "protect/nnp"},
		"level word":          {svc("protect \"nnp\" {\n level = \"maybe\"\n}"), "protect/nnp"},
		"duration number":     {svc("stop {\n timeout = 5\n}"), "stop.timeout"},
		"duration negative":   {svc("stop {\n timeout = \"-1s\"\n}"), "stop.timeout"},
		"duration too long":   {svc("stop {\n timeout = \"99999h\"\n}"), "stop.timeout"},
		"duration garbage":    {svc("stop {\n timeout = \"soon\"\n}"), "stop.timeout"},
		"count fractional":    {svc("check {\n rise = 1.5\n}"), "check.rise"},
		"count negative":      {svc("check {\n rise = -1\n}"), "check.rise"},
		"count huge":          {svc("check {\n rise = 3000000000\n}"), "check.rise"},
		"count string":        {svc("check {\n rise = \"1\"\n}"), "check.rise"},
		"interval no max":     {svc("stop {\n timeout = { min = \"1s\" }\n}"), "stop.timeout"},
		"interval no min":     {svc("stop {\n timeout = { max = \"1s\" }\n}"), "stop.timeout"},
		"interval bad min":    {svc("stop {\n timeout = { min = 5, max = \"2s\" }\n}"), "stop.timeout"},
		"interval bad max":    {svc("stop {\n timeout = { min = \"1s\", max = 5 }\n}"), "stop.timeout"},
		"interval reversed":   {svc("stop {\n timeout = { min = \"9s\", max = \"1s\" }\n}"), "stop.timeout"},
		"interval bad prefer": {svc("stop {\n timeout = { min = \"1s\", max = \"2s\", prefer = 5 }\n}"), "stop.timeout"},
		"interval prefer out": {svc("stop {\n timeout = { min = \"1s\", max = \"2s\", prefer = \"3s\" }\n}"), "stop.timeout"},
		"edge not a list":     {svc(`require = "a"`), "require"},
		"edge name":           {svc(`require = ["a b"]`), "require"},
		"env not an object":   {svc(`env = "x"`), "env"},
		"env key":             {svc(`env = { "a b" = "x" }`), "env"},
		"env value":           {svc(`env = { A = 1 }`), "env.A"},
		"env nul":             {svc(`env = { A = "a\u0000" }`), "env.A"},
		"params key":          {svc(`params = { "a b" = "x" }`), "params"},
		"param min missing":   {svc("param \"p\" {\n type = \"int\"\n min = 1\n}"), "param.p"},
		"param min bad":       {svc("param \"p\" {\n type = \"int\"\n min = \"a\"\n max = 2\n}"), "param.p"},
		"param max bad":       {svc("param \"p\" {\n type = \"int\"\n min = 1\n max = \"a\"\n}"), "param.p"},
		"param reversed":      {svc("param \"p\" {\n type = \"int\"\n min = 5\n max = 2\n}"), "param.p"},
		"enum no values":      {svc("param \"p\" {\n type = \"enum\"\n}"), "param.p"},
		"enum empty":          {svc("param \"p\" {\n type = \"enum\"\n values = []\n}"), "param.p"},
		"enum comma":          {svc("param \"p\" {\n type = \"enum\"\n values = [\"a,b\"]\n}"), "param.p"},
		"credential source":   {svc("credential \"c\" {\n source = \"\"\n}"), "credential.c.source"},
		"credential number":   {svc("credential \"c\" {\n source = 1\n}"), "credential.c.source"},
		"exempt not a list":   {svc("protect \"nnp\" {\n exempt_on = \"linux\"\n}"), "protect/nnp/exempt_on"},
		"exempt unknown os":   {svc("protect \"nnp\" {\n exempt_on = [\"plan9x\"]\n}"), "protect/nnp/exempt_on"},
		"api not a list":      {"api {\n admin = \"a\"\n}", "api.admin"},
		"grant not a list":    {"api {\n grant \"g\" {\n services = \"a\"\n }\n}", ""},
		"schema zero":         {"schema = 0", "schema"},
		"schema string":       {"schema = \"1\"", "schema"},
	}
	for name, x := range bad {
		t.Run("bad/"+name, func(t *testing.T) {
			e := refuses1[ErrNotALatticeValue](t, Site, x.src)
			if x.key != "" && e.Key != x.key {
				t.Errorf("key %q, want %q", e.Key, x.key)
			}
		})
	}
	unknown := map[string]c{
		"service label":      {"service {\n}", "service"},
		"service labels":     {"service \"a\" \"b\" {\n}", "service"},
		"top key":            {"bogus = 1", "bogus"},
		"top block":          {"bogus {\n}", "bogus"},
		"all label":          {"all \"x\" {\n}", "all"},
		"api label":          {"api \"x\" {\n}", "api"},
		"api key":            {"api {\n bogus = 1\n}", "api.bogus"},
		"api block":          {"api {\n bogus {\n }\n}", "api.bogus"},
		"grant key":          {"api {\n grant \"g\" {\n bogus = 1\n }\n}", "grant.bogus"},
		"service key":        {svc(`bogus = 1`), "bogus"},
		"block key":          {svc("stop {\n bogus = 1\n}"), "stop.bogus"},
		"block unknown":      {svc("bogus {\n}"), "bogus"},
		"block labelled":     {svc("stop \"x\" {\n}"), "stop"},
		"block nested":       {svc("stop {\n bogus {\n }\n}"), "stop"},
		"interval key":       {svc("stop {\n timeout = { min = \"1s\", max = \"2s\", foo = 1 }\n}"), "stop.timeout.foo"},
		"param label bare":   {svc("param {\n}"), "param"},
		"protect label bare": {svc("protect {\n}"), "protect"},
		"credential bare":    {svc("credential {\n source = \"x\"\n}"), "credential"},
		"grant label bare":   {"api {\n grant {\n services = [\"a\"]\n }\n}", "grant"},
		"param label":        {svc("param {\n type = \"port\"\n}"), "param"},
		"param attr port":    {svc("param \"p\" {\n type = \"port\"\n min = 1\n}"), "param.p.min"},
		"param attr int":     {svc("param \"p\" {\n type = \"int\"\n min = 1\n max = 2\n foo = 1\n}"), "param.p.foo"},
		"param attr enum":    {svc("param \"p\" {\n type = \"enum\"\n values = [\"a\"]\n foo = 1\n}"), "param.p.foo"},
		"credential label":   {svc("credential {\n}"), "credential"},
		"credential attr":    {svc("credential \"c\" {\n foo = 1\n}"), "credential.foo"},
		"protect label":      {svc("protect {\n}"), "protect"},
		"protect unknown":    {svc("protect \"nope\" {\n}"), "protect.nope"},
		"protect attr":       {svc("protect \"nnp\" {\n foo = 1\n}"), "protect.nnp.foo"},
		"grant label":        {"api {\n grant {\n }\n}", "grant"},
	}
	for name, x := range unknown {
		t.Run("unknown/"+name, func(t *testing.T) {
			if e := refuses1[ErrUnknownKey](t, Site, x.src); e.Key != x.key {
				t.Errorf("key %q, want %q", e.Key, x.key)
			}
		})
	}
	t.Run("authority", func(t *testing.T) {
		refuses1[ErrFieldAuthority](t, Recipe, "schema = 1")
		refuses1[ErrFieldAuthority](t, Recipe, "all {\n}")
		refuses1[ErrFieldAuthority](t, Recipe, "api {\n}")
		refuses1[ErrFieldAuthority](t, Recipe, svc("credential \"c\" {\n source = \"s\"\n}"))
		refuses1[ErrFieldAuthority](t, Site, "all {\n credential \"c\" {\n }\n}")
		refuses1[ErrFieldAuthority](t, Site, "all {\n param \"p\" {\n type = \"port\"\n }\n}")
		refuses1[ErrFieldAuthority](t, Site, "all {\n stop {\n timeout = { min = \"1s\", max = \"2s\", prefer = \"1s\" }\n }\n}")
	})
	t.Run("unlabelled credential in a recipe", func(t *testing.T) {
		// the label is refused once; the body of the block is not read as well
		refuses1[ErrUnknownKey](t, Recipe, svc("credential {\n source = \"x\"\n}"))
	})
	t.Run("forking", func(t *testing.T) { refuses1[ErrForkingDaemon](t, Recipe, svc(`kind = "forking"`)) })
	t.Run("syntax", func(t *testing.T) { refuses1[ErrSyntax](t, Site, "service \"x\" {") })
}

// The reason names the guard that fired, so a guard that is deleted while the
// code behind it refuses with the same error type is still told apart.
func TestParseReasons(t *testing.T) {
	for src, why := range map[string]string{
		svc("stop {\n timeout = { min = \"1s\" }\n}"):                           "both min and max",
		svc("stop {\n timeout = { min = \"1s\", max = 5 }\n}"):                  "duration string",
		svc("stop {\n timeout = 5\n}"):                                          "duration string",
		svc("stop {\n timeout = { min = 5, max = 6 }\n}"):                       "duration string",
		svc("stop {\n timeout = { min = \"1s\", max = \"2s\", prefer = 5 }\n}"): "duration string",
		svc("check {\n rise = \"x\"\n}"):                                        "integer",
		svc("param \"p\" {\n type = \"int\"\n min = 1\n}"):                      "min and max",
		svc("param \"p\" {\n type = \"duration\"\n min = \"1s\"\n}"):            "min and max",
		svc("param \"p\" {\n type = \"enum\"\n}"):                               "non-empty list",
		svc("param \"p\" {\n type = \"int\"\n min = \"x\"\n max = 2\n}"):        "integer",
		svc("param \"p\" {\n type = \"duration\"\n min = \"1s\"\n max = 2\n}"):  "duration string",
	} {
		_, errs := ParseLayer(Layer{Name: "p.hcl", Authority: Site, Admitted: true, Source: []byte(src)})
		if len(errs) != 1 {
			t.Fatalf("%q: %v", src, errs)
		}
		var e ErrNotALatticeValue
		if !errors.As(errs[0], &e) || !strings.Contains(e.Why, why) {
			t.Errorf("%q: got %v, want a reason containing %q", src, errs[0], why)
		}
	}
	// two bad bounds are one refusal, not two
	refuses1[ErrNotALatticeValue](t, Site, svc("stop {\n timeout = { min = 5, max = 6 }\n}"))
}

func TestParseAccepts(t *testing.T) {
	d := accepts(t, Site, svc(`working_dir = "/srv"
terminal = true
limits {
  nofile = 0
}
check {
  rise = 2147483647
}
protect "nnp" {
  level = true
}
stop {
  timeout = "8760h"
}`))
	if v := d.Services["x"]["protect/nnp"]; !v.Set || v.Lo != LevelRequired {
		t.Errorf("level = true: %+v", v)
	}
	if v := d.Services["x"]["stop.timeout"]; v.Lo != int64(8760*time.Hour) {
		t.Errorf("one year: %+v", v)
	}
	if v := d.Services["x"]["check.rise"]; v.Lo != math.MaxInt32 {
		t.Errorf("count bound: %+v", v)
	}
	if v := d.Services["x"]["terminal"]; len(v.Strs) != 1 || v.Strs[0] != "true" {
		t.Errorf("terminal: %+v", v)
	}
	// the point sugar in all {} states an interval with no preference
	d = accepts(t, Site, "all {\n stop {\n timeout = \"3s\"\n }\n}")
	if v := d.All["stop.timeout"]; v.Lo != v.Hi || len(v.PrefS) != 0 || len(v.PrefR) != 0 {
		t.Errorf("all point: %+v", v)
	}
	// a recipe point is a recipe preference; a site point is a site preference
	if v := accepts(t, Recipe, svc("stop {\n timeout = \"3s\"\n}")).Services["x"]["stop.timeout"]; len(v.PrefR) != 1 || len(v.PrefS) != 0 {
		t.Errorf("recipe point: %+v", v)
	}
	if v := accepts(t, Site, svc("stop {\n timeout = \"3s\"\n}")).Services["x"]["stop.timeout"]; len(v.PrefS) != 1 || len(v.PrefR) != 0 {
		t.Errorf("site point: %+v", v)
	}
	accepts(t, Site, "api {\n admin = [\"a\"]\n grant \"a\" {\n services = [\"x\"]\n }\n}")
	accepts(t, Site, svc("params = { p = \"1\" }"))
	accepts(t, Recipe, svc("env = { A_1 = \"v\" }\nrequire = [\"a\"]\nwant = [\"b\"]\norder = [\"c\"]"))
	accepts(t, Recipe, svc("param \"p\" {\n type = \"enum\"\n values = [\"a\", \"b\"]\n}"))
	accepts(t, Site, svc("protect \"nnp\" {\n exempt_on = [\"linux\"]\n}"))
}

func TestParseVersionGate(t *testing.T) {
	l := func(src string) Layer {
		return Layer{Name: "p.hcl", Authority: Recipe, Admitted: true, Source: []byte(src)}
	}
	// a schema-1 binary does not know a schema-2 field or protection
	for _, src := range []string{
		svc("allow {\n ioctls = [\"x\"]\n}"),
		svc("protect \"mountns\" {\n level = \"required\"\n}"),
	} {
		if _, errs := parseLayer(l(src), schemaV2(), 1); len(errs) != 1 {
			t.Errorf("a schema-1 binary accepted %q: %v", src, errs)
		}
		if _, errs := parseLayer(l(src), schemaV2(), 2); len(errs) != 0 {
			t.Errorf("a schema-2 binary refused %q: %v", src, errs)
		}
	}
}

func TestDeterministicFirstRefusal(t *testing.T) {
	// objects are read in key order, so the refusal that stops the read is the same
	for i := 0; i < 50; i++ {
		e := refuses1[ErrUnknownKey](t, Site, svc("stop {\n timeout = { min = \"1s\", max = \"2s\", zz = 1, aa = 2 }\n}"))
		if e.Key != "stop.timeout.aa" {
			t.Fatalf("got %q", e.Key)
		}
		e = refuses1[ErrUnknownKey](t, Site, svc("param \"p\" {\n type = \"port\"\n zz = 1\n aa = 2\n}"))
		if e.Key != "param.p.aa" {
			t.Fatalf("got %q", e.Key)
		}
	}
}

func TestSaturatingAdd(t *testing.T) {
	if satAdd(math.MaxInt64, 1) != math.MaxInt64 || satAdd(1, math.MaxInt64) != math.MaxInt64 {
		t.Error("overflow is not saturated")
	}
	if satAdd(math.MaxInt64-1, 1) != math.MaxInt64 || satAdd(3, 4, 5) != 12 || satAdd() != 0 {
		t.Error("a sum below the bound changed")
	}
}

func TestResolveIntervalConflict(t *testing.T) {
	if _, _, st := resolveInterval(Interval(5, 1)); st != "conflict" {
		t.Errorf("empty interval: %q", st)
	}
	v := Interval(1, 9)
	v.PrefR = []int64{3}
	if p, src, _ := resolveInterval(v); p != 3 || src != "recipe-prefer" {
		t.Errorf("%d %q", p, src)
	}
}

// Without the memo the wait budget walks every path of the graph: a ladder of
// services each wanting the next two has fibonacci(n) of them.
func TestWaitBudgetIsLinear(t *testing.T) {
	const n = 70
	var recipe, siteSrc string
	for i := 0; i < n; i++ {
		extra := ""
		if i == 0 {
			extra = "require = [\"s1\"]\nwait {\n timeout = \"8760h\"\n}\n"
		}
		if i+1 < n && i > 0 {
			extra += "want = [\"s" + itoaN(i+1) + "\"]\n"
		}
		if i+2 < n {
			extra += "order = [\"s" + itoaN(i+2) + "\"]\n"
		}
		recipe += svcRecipe("s"+itoaN(i), extra)
		siteSrc += svcSite("s"+itoaN(i), "")
	}
	done := make(chan error, 1)
	go func() {
		_, err := compile(rec("r.hcl", recipe), site("s.hcl", baseline+siteSrc))
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the wait budget is not memoised")
	}
}

func itoaN(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
