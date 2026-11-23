package supervisor

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Options configures Compile. The zero value compiles for a system instance
// at CurrentSchema.
type Options struct {
	// Running is the schema version of the binary; 0 means CurrentSchema.
	Running int
	// UserMode is mode U; Owner is the instance owner's account name.
	UserMode bool
	Owner    string

	sc *schema // tests substitute a later schema
}

// Plan is the only product of Compile (section 1.10).
type Plan struct {
	Schema   int
	API      map[string]Value
	Services map[string]*ServicePlan
}

// ServicePlan is one service after the meet and the resolution.
type ServicePlan struct {
	Name   string
	Fields map[string]Value
	// Resolved holds each class-T field at its resolved point (section 1.4,
	// phase 2); Source says which rule chose it.
	Resolved map[string]int64
	Source   map[string]string
	// Legacy lists the fields of a schema newer than the baseline's that took
	// their legacy value (N9, D5).
	Legacy []string
}

// Epsilon is the minimal wait of section 3.6 (SW = epsilon for a service
// without dependents). The design does not give it a value; it is 1 ms.
const Epsilon = int64(time.Millisecond)

var placeholderRE = regexp.MustCompile(`\{\{([A-Za-z0-9_.@-]+)\}\}`)

type stmt struct {
	layer string
	v     Value
}

type compiler struct {
	opts    Options
	sc      *schema
	running int
	errs    collector
	docs    []*Doc // sorted by authority then name
	nb      int
}

// Compile is a function of the set of (layer, authority) pairs: the order of
// layers and repeated layers do not change the result (laws A, C, I).
func Compile(layers []Layer, opts Options) (*Plan, error) {
	c := &compiler{opts: opts, sc: opts.sc, running: opts.Running}
	if c.sc == nil {
		c.sc = defaultSchema()
	}
	if c.running == 0 {
		c.running = CurrentSchema
	}
	if !c.admit(layers) {
		return nil, c.errs.result()
	}
	if !c.instance() {
		return nil, c.errs.result()
	}
	plan := c.services()
	if err := c.errs.result(); err != nil {
		return nil, err
	}
	return plan, nil
}

// admit checks admission and parses; it reports whether compilation can go on.
func (c *compiler) admit(layers []Layer) bool {
	type key struct {
		a    Authority
		n, s string
	}
	seen := map[key]bool{}
	for _, l := range layers {
		k := key{l.Authority, l.Name, string(l.Source)}
		if seen[k] {
			continue
		}
		seen[k] = true
		switch {
		case l.Authority != Recipe && l.Authority != Site:
			c.errs.add(ErrNotALatticeValue{Layer: l.Name, Key: "authority", Why: "a layer is a recipe or a site layer"})
		case !l.Admitted && l.Authority == Recipe:
			c.errs.add(ErrUnsignedRecipe{Layer: l.Name})
		case !l.Admitted:
			c.errs.add(ErrUnsafePath{Path: l.Name})
		default:
			d, errs := parseLayer(l, c.sc, c.running)
			for _, e := range errs {
				c.errs.add(e)
			}
			if d != nil {
				c.docs = append(c.docs, d)
			}
		}
	}
	sort.Slice(c.docs, func(i, j int) bool {
		a, b := c.docs[i].Layer, c.docs[j].Layer
		return cmp.Or(cmp.Compare(a.Authority, b.Authority), strings.Compare(a.Name, b.Name),
			strings.Compare(string(a.Source), string(b.Source))) < 0
	})
	return len(c.errs.errs) == 0
}

func (c *compiler) siteDocs() []*Doc { return c.docsOf(Site) }
func (c *compiler) docsOf(a Authority) []*Doc {
	var out []*Doc
	for _, d := range c.docs {
		if d.Layer.Authority == a {
			out = append(out, d)
		}
	}
	return out
}

// instance resolves the baseline's schema version and refuses a missing,
// partial or too new baseline (section 1.5, C1 and N9).
func (c *compiler) instance() bool {
	hasAll := false
	sv := Top(K1)
	for _, d := range c.siteDocs() {
		hasAll = hasAll || d.HasAll
		sv = Meet(sv, d.Schema)
	}
	if !hasAll {
		c.errs.add(ErrNoBaseline{})
		return false
	}
	switch {
	case !sv.Set:
		c.errs.add(ErrIncomplete{Path: "schema"})
		return false
	case sv.IsErr():
		c.errs.add(ErrConflict{Path: "schema", Values: sv.Strs, Layers: c.siteLayerNames()})
		return false
	}
	c.nb, _ = strconv.Atoi(sv.Strs[0])
	if c.nb > c.running {
		c.errs.add(ErrSchemaTooNew{Baseline: c.nb, Running: c.running})
		return false
	}
	return true
}

func (c *compiler) siteLayerNames() []string {
	var out []string
	for _, d := range c.siteDocs() {
		out = append(out, d.Layer.Name)
	}
	return sortedDistinct(out)
}

func meetMaps(dst map[string]Value, src map[string]Value) {
	for k, v := range src {
		put(dst, k, v)
	}
}

// stated lists every layer statement of path for service s ("" for the
// baseline), in a deterministic order.
func (c *compiler) stated(s, path string) []stmt {
	var out []stmt
	for _, d := range c.docs {
		if m, ok := d.Services[s]; ok {
			if v, ok := m[path]; ok {
				out = append(out, stmt{d.Layer.Name, v})
			}
		}
		if v, ok := d.All[path]; ok && d.Layer.Authority == Site {
			out = append(out, stmt{d.Layer.Name, v})
		}
	}
	return out
}

func (c *compiler) conflict(s, path string, v Value) ErrConflict {
	var layers, vals []string
	for _, st := range c.stated(s, path) {
		layers = append(layers, st.layer)
		if st.v.Kind == K2 {
			vals = append(vals, fmt.Sprintf("[%d,%d]", st.v.Lo, st.v.Hi))
		} else {
			vals = append(vals, st.v.Strs...)
		}
	}
	return ErrConflict{Path: s + "/" + path, Values: sortedDistinct(vals), Layers: sortedDistinct(layers)}
}

// baselineRequired lists the fields B must state at version nb (totality).
func (c *compiler) baselineRequired() []string {
	var out []string
	for p, f := range c.sc.fields {
		if f.Req == ReqHardening && f.Since <= c.nb && !f.Needs {
			out = append(out, p)
		}
	}
	for _, pr := range c.sc.protections {
		if pr.Since <= c.nb {
			out = append(out, "protect/"+pr.Name, "protect/"+pr.Name+"/exempt_on")
		}
	}
	sort.Strings(out)
	return out
}

func (c *compiler) services() *Plan {
	B := map[string]Value{}
	api := map[string]Value{}
	R := map[string]map[string]Value{}
	S := map[string]map[string]Value{}
	for _, d := range c.siteDocs() {
		meetMaps(B, d.All)
		meetMaps(api, d.API)
		for s, m := range d.Services {
			if S[s] == nil {
				S[s] = map[string]Value{}
			}
			meetMaps(S[s], m)
		}
	}
	for _, d := range c.docsOf(Recipe) {
		for s, m := range d.Services {
			if R[s] == nil {
				R[s] = map[string]Value{}
			}
			meetMaps(R[s], m)
		}
	}

	// Totality of the baseline.
	var missing []string
	for _, p := range c.baselineRequired() {
		if v, ok := B[p]; !ok || !v.Set {
			missing = append(missing, p)
		}
	}
	for _, p := range []string{"api.admin", "api.read"} {
		if v, ok := api[p]; !ok || !v.Set {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		c.errs.add(ErrBaselineNotTotal{Fields: missing})
		return nil
	}

	// Class D: the service set and its fields belong to recipes.
	for _, d := range c.siteDocs() {
		for s, m := range d.Services {
			if _, ok := R[s]; !ok {
				c.errs.add(ErrWideningField{Service: s, Field: "service", Layer: d.Layer.Name})
				continue
			}
			for p, v := range m {
				if def, _ := classOf(c.sc, p); def.Class != ClassD {
					continue
				}
				if rv, ok := R[s][p]; !ok || !subset(v.Strs, rv.Strs) {
					c.errs.add(ErrWideningField{Service: s, Field: p, Layer: d.Layer.Name})
				}
			}
		}
	}

	plan := &Plan{Schema: c.nb, API: api, Services: map[string]*ServicePlan{}}
	names := make([]string, 0, len(R))
	for s := range R {
		names = append(names, s)
	}
	sort.Strings(names)
	for _, s := range names {
		plan.Services[s] = c.service(s, B, R[s], S[s])
	}
	c.graph(plan, names)
	c.grants(plan, names)
	return plan
}

// service meets B, the recipe fields and the site fields of one service,
// resolves class T and applies the per-service predicates.
func (c *compiler) service(s string, B, r, site map[string]Value) *ServicePlan {
	sp := &ServicePlan{Name: s, Fields: map[string]Value{}, Resolved: map[string]int64{}, Source: map[string]string{}}
	meetMaps(sp.Fields, B)
	meetMaps(sp.Fields, r)
	meetMaps(sp.Fields, site)

	// Fields newer than the baseline's schema take their legacy value.
	for _, p := range c.newerFields() {
		def, _ := classOf(c.sc, p)
		if v, ok := sp.Fields[p]; ok && v.Set {
			sp.Fields[p] = Meet(def.legacyValue(), v)
			continue
		}
		sp.Fields[p] = def.legacyValue()
		sp.Legacy = append(sp.Legacy, p)
	}
	sort.Strings(sp.Legacy)

	paths := make([]string, 0, len(sp.Fields))
	for p := range sp.Fields {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		v := sp.Fields[p]
		if v.IsErr() && v.Kind == K1 {
			c.errs.add(c.conflict(s, p, v))
		}
	}

	// Phase 2: resolve class T.
	for _, p := range paths {
		def, _ := classOf(c.sc, p)
		v := sp.Fields[p]
		if def.Kind != K2 || !v.Set {
			continue
		}
		pt, src, err := resolveInterval(v)
		if err != "" {
			if err == "incomplete" {
				c.errs.add(ErrIncomplete{Path: s + "/" + p})
			} else {
				c.errs.add(c.conflict(s, p, v))
			}
			continue
		}
		sp.Resolved[p], sp.Source[p] = pt, src
	}

	c.complete(s, sp)
	c.predicates(s, sp)
	return sp
}

// legacyValue is the legacy value of a field; for a field without one it is the
// top element of its kind.
func (f fieldDef) legacyValue() Value {
	if f.Legacy.Kind != 0 {
		return f.Legacy
	}
	return Top(f.Kind)
}

func (c *compiler) newerFields() []string {
	var out []string
	for p, f := range c.sc.fields {
		if f.Since > c.nb && f.Since <= c.running {
			out = append(out, p)
		}
	}
	for _, pr := range c.sc.protections {
		if pr.Since > c.nb && pr.Since <= c.running {
			out = append(out, "protect/"+pr.Name, "protect/"+pr.Name+"/exempt_on")
		}
	}
	sort.Strings(out)
	return out
}

// resolveInterval is phase 2 of section 1.4. It returns the point, the rule
// that chose it, or "incomplete" / "conflict".
func resolveInterval(v Value) (int64, string, string) {
	if v.IsErr() {
		return 0, "", "conflict"
	}
	if v.Lo == v.Hi {
		return v.Lo, "singleton", ""
	}
	in := func(x int64) bool { return v.Lo <= x && x <= v.Hi }
	if len(v.PrefS) > 0 {
		if slices.ContainsFunc(v.PrefS, func(x int64) bool { return !in(x) }) {
			return 0, "", "conflict" // a site preference may not widen
		}
		if len(v.PrefS) > 1 {
			return 0, "", "conflict" // two site preferences disagree
		}
		return v.PrefS[0], "site-prefer", ""
	}
	var inside []int64
	for _, x := range v.PrefR {
		if in(x) {
			inside = append(inside, x)
		}
	}
	switch len(inside) {
	case 0:
		return 0, "", "incomplete"
	case 1:
		return inside[0], "recipe-prefer", ""
	}
	return 0, "", "conflict"
}

// complete is the completeness predicate over the met service (section 1.5):
// every field of Q(N_B) is non-top.
func (c *compiler) complete(s string, sp *ServicePlan) {
	need := func(p string) {
		if v, ok := sp.Fields[p]; !ok || !v.Set {
			c.errs.add(ErrIncomplete{Path: s + "/" + p})
		}
	}
	hasReq := false
	for p, v := range sp.Fields {
		if strings.HasPrefix(p, "edge/") && v.Set && v.Lo == LevelRequired {
			hasReq = true
		}
	}
	check := false
	for p, def := range c.sc.fields {
		if def.Req == ReqCheck && sp.Fields[p].Set {
			check = true
		}
	}
	for p, def := range c.sc.fields {
		if def.Since > c.nb {
			continue
		}
		switch def.Req {
		case ReqAlways:
			need(p)
		case ReqIfRequireEdges:
			if hasReq {
				need(p)
			}
		case ReqCheck:
			if check {
				need(p)
			}
		}
	}
	for p := range sp.Fields {
		switch {
		case strings.HasPrefix(p, "paramdecl/"):
			need("param/" + strings.TrimPrefix(p, "paramdecl/"))
		case strings.HasPrefix(p, "cred/") && !strings.HasSuffix(p, "/source"):
			need(p + "/source")
		}
	}
}

// predicates are the post-meet predicates of section 1.6 that concern one
// service.
func (c *compiler) predicates(s string, sp *ServicePlan) {
	f := sp.Fields
	// needs must lie inside allows.
	for _, x := range []string{"capabilities", "syscalls", "address_families"} {
		n, a := f["needs."+x], f["allow."+x]
		if n.Set && a.Set && !subset(n.Strs, a.Strs) {
			var miss []string
			for _, m := range n.Strs {
				if !slices.Contains(a.Strs, m) {
					miss = append(miss, m)
				}
			}
			c.errs.add(ErrConflict{Path: s + "/needs." + x, Values: miss, Layers: c.layerNames(s, "needs."+x, "allow."+x)})
		}
	}
	// check timing, on resolved values.
	if _, ok := sp.Resolved["check.timeout"]; ok {
		t, p := sp.Resolved["check.timeout"], sp.Resolved["check.period"]
		if !(0 < t && t < p) {
			c.errs.add(ErrCheckTiming{Service: s, Why: "0 < timeout < period"})
		}
		if sp.Resolved["check.rise"] < 1 || sp.Resolved["check.fall"] < 1 {
			c.errs.add(ErrCheckTiming{Service: s, Why: "rise and fall are at least 1"})
		}
	}
	// environment.
	for p := range f {
		if strings.HasPrefix(p, "env/") && looksSecret(strings.TrimPrefix(p, "env/")) {
			c.errs.add(ErrSecretInEnvironment{Service: s, Key: strings.TrimPrefix(p, "env/")})
		}
	}
	// run_as in mode U.
	if u := f["run_as.user"]; c.opts.UserMode && u.Set && len(u.Strs) == 1 && u.Strs[0] != c.opts.Owner {
		c.errs.add(ErrRunAsInUserMode{Service: s, RunAs: u.Strs[0], Owner: c.opts.Owner})
	}
	c.params(s, f)
}

func (c *compiler) layerNames(s string, paths ...string) []string {
	var out []string
	for _, p := range paths {
		for _, st := range c.stated(s, p) {
			out = append(out, st.layer)
		}
	}
	return sortedDistinct(out)
}

func (c *compiler) params(s string, f map[string]Value) {
	declared := map[string]string{}
	for p, v := range f {
		if strings.HasPrefix(p, "paramdecl/") && len(v.Strs) == 1 {
			declared[strings.TrimPrefix(p, "paramdecl/")] = v.Strs[0]
		}
	}
	// a value for a parameter nobody declared
	for p := range f {
		if n, ok := strings.CutPrefix(p, "param/"); ok {
			if _, ok := declared[n]; !ok {
				c.errs.add(ErrUnknownKey{Layer: strings.Join(c.layerNames(s, p), ","), Key: s + "/" + p})
			}
		}
	}
	for n, dom := range declared {
		v := f["param/"+n]
		if v.Set && len(v.Strs) == 1 && !inDomain(dom, v.Strs[0]) {
			c.errs.add(ErrConflict{Path: s + "/param/" + n, Values: v.Strs, Layers: c.layerNames(s, "param/"+n)})
		}
	}
	// placeholders must name declared parameters; none may sit in a shell
	// command string (R3-L4).
	var args []string
	if e := f["exec"]; e.Set && len(e.Strs) == 1 {
		args = strings.Split(e.Strs[0], "\x00")
	}
	use := func(text string) {
		for _, m := range placeholderRE.FindAllStringSubmatch(text, -1) {
			if _, ok := declared[m[1]]; !ok {
				c.errs.add(ErrUntypedParam{Service: s, Param: m[1]})
			}
		}
	}
	for _, a := range args {
		use(a)
	}
	for p, v := range f {
		if strings.HasPrefix(p, "env/") && len(v.Strs) == 1 {
			use(v.Strs[0])
		}
	}
	if len(args) > 0 {
		base := strings.ToLower(args[0])
		if i := strings.LastIndexAny(base, `/\`); i >= 0 {
			base = base[i+1:]
		}
		if slices.Contains(shells, base) {
			for i, a := range args {
				if a != "-c" && a != "/c" {
					continue
				}
				last := i + 1
				if base != "sh" && base != "bash" && base != "dash" && base != "zsh" {
					last = len(args) - 1 // cmd and powershell: the rest is the command
				}
				for j := i + 1; j <= last && j < len(args); j++ {
					for _, m := range placeholderRE.FindAllStringSubmatch(args[j], -1) {
						c.errs.add(ErrParamInShell{Service: s, Param: m[1]})
					}
				}
			}
		}
	}
}

// inDomain checks a parameter value against its encoded domain (N7).
func inDomain(dom, v string) bool {
	parts := strings.Split(dom, ":")
	switch parts[0] {
	case "int", "duration":
		lo, _ := strconv.ParseInt(parts[1], 10, 64)
		hi, _ := strconv.ParseInt(parts[2], 10, 64)
		var n int64
		if parts[0] == "int" {
			x, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return false
			}
			n = x
		} else {
			d, err := time.ParseDuration(v)
			if err != nil {
				return false
			}
			n = int64(d)
		}
		return lo <= n && n <= hi
	case "enum":
		return slices.Contains(strings.Split(dom[len("enum:"):], ","), v)
	case "port":
		n, err := strconv.ParseUint(v, 10, 16)
		return err == nil && n >= 1
	case "path":
		return pathAlphabet.MatchString(v)
	}
	return false
}
