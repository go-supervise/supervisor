package supervisor

import (
	"fmt"
	"math/big"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// sortedKeys returns the keys of m in order, so that a parser that stops at the
// first refusal stops at the same one every time.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Authority of a layer (section 1.7). It comes from where the layer was
// loaded and how it was verified, never from its content.
type Authority uint8

const (
	Recipe Authority = iota + 1
	Site
)

func (a Authority) String() string {
	switch a {
	case Recipe:
		return "recipe"
	case Site:
		return "site"
	}
	return "unknown"
}

// Layer is one source file with its authority. Admitted is set by the loader
// once the admission check of section 1.7 passed: the signature for a recipe,
// the ownership walk for a site layer.
type Layer struct {
	Name      string
	Authority Authority
	Admitted  bool
	Source    []byte
}

// Doc is a parsed layer: values by field path. Paths of a service are the
// field names of the schema plus the dynamic families env/, param/, paramdecl/,
// cred/, edge/ and protect/.
type Doc struct {
	Layer    Layer
	Schema   Value            // K1, the schema statement
	API      map[string]Value // api.admin, api.read, api.grant/<principal>
	HasAll   bool
	All      map[string]Value
	Services map[string]map[string]Value
}

type parser struct {
	l       Layer
	sc      *schema
	running int
	errs    []error
}

// ParseLayer parses one layer under the default schema. It is the entry point
// of the parser fuzz target.
func ParseLayer(l Layer) (*Doc, []error) { return parseLayer(l, defaultSchema(), CurrentSchema) }

func parseLayer(l Layer, sc *schema, running int) (*Doc, []error) {
	p := &parser{l: l, sc: sc, running: running}
	d := &Doc{Layer: l, Schema: Top(K1), API: map[string]Value{}, All: map[string]Value{},
		Services: map[string]map[string]Value{}}
	f, diags := hclsyntax.ParseConfig(l.Source, l.Name, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, []error{ErrSyntax{Layer: l.Name, Msg: diags.Errs()[0].Error()}}
	}
	body := f.Body.(*hclsyntax.Body) // the native parser always returns one
	for _, name := range sortedAttrNames(body) {
		attr := body.Attributes[name]
		if name != "schema" {
			p.unknown(name)
			continue
		}
		if l.Authority == Recipe {
			p.errs = append(p.errs, ErrFieldAuthority{Field: "schema", Authority: "recipe"})
			continue
		}
		v, ok := p.literal(attr.Expr, name)
		if !ok {
			continue
		}
		n, ok := intOf(v)
		if !ok || n < 1 {
			p.bad(name, "schema is an integer >= 1")
			continue
		}
		d.Schema = Flat(strconv.FormatInt(n, 10))
	}
	for _, b := range body.Blocks {
		switch b.Type {
		case "service":
			name, ok := p.label(b, "service")
			if !ok {
				continue
			}
			m := d.Services[name]
			if m == nil {
				m = map[string]Value{}
				d.Services[name] = m
			}
			p.serviceBody(b.Body, m, false, name)
		case "all":
			if l.Authority == Recipe {
				p.errs = append(p.errs, ErrFieldAuthority{Field: "all", Authority: "recipe"})
				continue
			}
			if len(b.Labels) != 0 {
				p.unknown("all")
				continue
			}
			d.HasAll = true
			p.serviceBody(b.Body, d.All, true, "all")
		case "api":
			if l.Authority == Recipe {
				p.errs = append(p.errs, ErrFieldAuthority{Field: "api", Authority: "recipe"})
				continue
			}
			if len(b.Labels) != 0 {
				p.unknown("api")
				continue
			}
			p.apiBody(b.Body, d.API)
		default:
			p.unknown(b.Type)
		}
	}
	if len(p.errs) > 0 {
		return nil, p.errs
	}
	return d, nil
}

func sortedAttrNames(b *hclsyntax.Body) []string {
	names := make([]string, 0, len(b.Attributes))
	for n := range b.Attributes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (p *parser) unknown(key string) {
	p.errs = append(p.errs, ErrUnknownKey{Layer: p.l.Name, Key: key})
}

func (p *parser) bad(key, why string) {
	p.errs = append(p.errs, ErrNotALatticeValue{Layer: p.l.Name, Key: key, Why: why})
}

func (p *parser) label(b *hclsyntax.Block, what string) (string, bool) {
	if len(b.Labels) != 1 {
		p.unknown(what)
		return "", false
	}
	if !nameRE.MatchString(b.Labels[0]) {
		p.bad(what, "name outside [A-Za-z0-9_.@-]{1,128}")
		return "", false
	}
	return b.Labels[0], true
}

func put(m map[string]Value, path string, v Value) {
	if old, ok := m[path]; ok {
		m[path] = Meet(old, v)
		return
	}
	m[path] = v
}

// literal accepts only literal expressions (section 1.10): constants,
// tuples and objects of constants, and a negated number. No variables, no
// functions, no operators, no interpolation.
func (p *parser) literal(e hclsyntax.Expression, key string) (cty.Value, bool) {
	if !isLiteral(e) {
		p.errs = append(p.errs, ErrNotLiteral{Layer: p.l.Name, Key: key})
		return cty.NilVal, false
	}
	v, diags := e.Value(nil)
	if diags.HasErrors() || v.IsNull() || !v.IsWhollyKnown() {
		p.errs = append(p.errs, ErrNotLiteral{Layer: p.l.Name, Key: key})
		return cty.NilVal, false
	}
	return v, true
}

func isLiteral(e hclsyntax.Expression) bool {
	switch x := e.(type) {
	case *hclsyntax.LiteralValueExpr:
		return true
	case *hclsyntax.TemplateExpr:
		for _, part := range x.Parts {
			if _, ok := part.(*hclsyntax.LiteralValueExpr); !ok {
				return false
			}
		}
		return true
	case *hclsyntax.TupleConsExpr:
		for _, it := range x.Exprs {
			if !isLiteral(it) {
				return false
			}
		}
		return true
	case *hclsyntax.ObjectConsExpr:
		for _, it := range x.Items {
			// HCL wraps every object key; a bare name is a traversal that the
			// evaluation below refuses unless it is a single word.
			k := it.KeyExpr.(*hclsyntax.ObjectConsKeyExpr)
			if _, bare := k.Wrapped.(*hclsyntax.ScopeTraversalExpr); !bare && !isLiteral(k.Wrapped) {
				return false
			}
			if !isLiteral(it.ValueExpr) {
				return false
			}
		}
		return true
	case *hclsyntax.UnaryOpExpr:
		_, ok := x.Val.(*hclsyntax.LiteralValueExpr)
		return ok && x.Op == hclsyntax.OpNegate
	}
	return false
}

func intOf(v cty.Value) (int64, bool) {
	if v.Type() != cty.Number {
		return 0, false
	}
	n, acc := v.AsBigFloat().Int64()
	return n, acc == big.Exact
}

func strOf(v cty.Value) (string, bool) {
	if v.Type() != cty.String {
		return "", false
	}
	return v.AsString(), true
}

func strList(v cty.Value) ([]string, bool) {
	if !(v.Type().IsTupleType() || v.Type().IsListType()) {
		return nil, false
	}
	var out []string
	for it := v.ElementIterator(); it.Next(); {
		_, e := it.Element()
		s, ok := strOf(e)
		if !ok || s == "" || strings.ContainsRune(s, 0) {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// serviceBody parses the body of a service block or of the all block.
func (p *parser) serviceBody(b *hclsyntax.Body, m map[string]Value, inAll bool, svc string) {
	for _, name := range sortedAttrNames(b) {
		attr := b.Attributes[name]
		switch name {
		case "require", "want", "order":
			p.edges(attr, name, m, inAll)
		case "env":
			p.object(attr, name, m, inAll, "env/", true)
		case "params":
			p.object(attr, name, m, inAll, "param/", false)
		default:
			if _, ok := p.sc.fields[name]; !ok {
				p.unknown(name)
				continue
			}
			p.field(attr, name, m, inAll)
		}
	}
	for _, blk := range b.Blocks {
		switch blk.Type {
		case "param":
			p.paramBlock(blk, m, inAll, svc)
		case "credential":
			p.credBlock(blk, m, inAll)
		case "protect":
			p.protectBlock(blk, m, inAll)
		default:
			if len(blk.Labels) != 0 || !p.sc.isBlock(blk.Type) || len(blk.Body.Blocks) != 0 {
				p.unknown(blk.Type)
				continue
			}
			for _, name := range sortedAttrNames(blk.Body) {
				path := blk.Type + "." + name
				if _, ok := p.sc.fields[path]; !ok {
					p.unknown(path)
					continue
				}
				p.field(blk.Body.Attributes[name], path, m, inAll)
			}
		}
	}
}

// gate applies the authority table of section 1.7 and the restrictions of
// all {} (section 1.5). It reports whether the field may be stated here.
func (p *parser) gate(path string, def fieldDef, inAll bool) bool {
	if p.l.Authority == Recipe && (def.Class == ClassG || def.Class == ClassP) {
		p.errs = append(p.errs, ErrFieldAuthority{Field: path, Authority: "recipe"})
		return false
	}
	if inAll {
		switch def.Class {
		case ClassH, ClassT, ClassPolicy:
			if def.Needs {
				p.errs = append(p.errs, ErrFieldAuthority{Field: path, Authority: "all"})
				return false
			}
		default:
			p.errs = append(p.errs, ErrFieldAuthority{Field: path, Authority: "all"})
			return false
		}
	}
	return true
}

func (p *parser) field(attr *hclsyntax.Attribute, path string, m map[string]Value, inAll bool) {
	// every caller has looked the path up already, so classOf cannot fail here
	def, _ := classOf(p.sc, path)
	if def.Since > p.running {
		p.unknown(path)
		return
	}
	if !p.gate(path, def, inAll) {
		return
	}
	cv, ok := p.literal(attr.Expr, path)
	if !ok {
		return
	}
	if v, ok := p.convert(path, def, cv, inAll); ok {
		put(m, path, v)
	}
}

func (p *parser) convert(path string, def fieldDef, cv cty.Value, inAll bool) (Value, bool) {
	switch def.Kind {
	case K1:
		return p.convertFlat(path, def, cv)
	case K2:
		return p.convertInterval(path, def, cv, inAll)
	case K3:
		n, ok := intOf(cv)
		if !ok || n < 0 {
			p.bad(path, "a non-negative integer")
			return Value{}, false
		}
		return Bound(n), true
	case K4:
		if cv.Type() == cty.Bool && cv.True() {
			return Level(K4, LevelRequired), true
		}
		s, ok := strOf(cv)
		switch {
		case ok && s == "required":
			return Level(K4, LevelRequired), true
		case ok && s == "waived":
			return Level(K4, LevelWaived), true
		}
		// the literal false is refused, as is anything else (section 1.3)
		p.errs = append(p.errs, ErrNotALatticeValue{Layer: p.l.Name, Key: path, Why: `"required" or "waived"; false is not a lattice value`})
		return Value{}, false
	case K5, K5P, K6:
		l, ok := strList(cv)
		if !ok {
			p.bad(path, "a list of non-empty strings")
			return Value{}, false
		}
		return SetOf(def.Kind, l...), true
	}
	p.bad(path, "unsupported kind")
	return Value{}, false
}

func (p *parser) convertFlat(path string, def fieldDef, cv cty.Value) (Value, bool) {
	var s string
	switch {
	case path == "exec":
		l, ok := strList(cv)
		if !ok || len(l) == 0 {
			p.bad(path, "a non-empty list of non-empty strings")
			return Value{}, false
		}
		s = strings.Join(l, "\x00")
	case cv.Type() == cty.Bool:
		s = strconv.FormatBool(cv.True())
	case cv.Type() == cty.String:
		s = cv.AsString()
		if s == "" || strings.ContainsRune(s, 0) {
			p.bad(path, "a non-empty string")
			return Value{}, false
		}
	case cv.Type() == cty.Number:
		n, ok := intOf(cv)
		if !ok {
			p.bad(path, "an integer")
			return Value{}, false
		}
		s = strconv.FormatInt(n, 10)
	default:
		p.bad(path, "a string, bool or number")
		return Value{}, false
	}
	if path == "kind" && s == "forking" {
		p.errs = append(p.errs, ErrForkingDaemon{Layer: p.l.Name, Service: path})
		return Value{}, false
	}
	if len(def.Enum) > 0 && !slices.Contains(def.Enum, s) {
		p.bad(path, "one of "+strings.Join(def.Enum, ", "))
		return Value{}, false
	}
	return Flat(s), true
}

// point reads one bound of an interval. It does not report: it returns the
// reason it refused, "" when it did not, and the caller reports it.
func point(def fieldDef, cv cty.Value) (int64, string) {
	switch def.Unit {
	case UnitDuration:
		s, ok := strOf(cv)
		if !ok {
			return 0, `a duration string such as "30s"`
		}
		d, err := time.ParseDuration(s)
		if err != nil || d < 0 || int64(d) > durationMax {
			return 0, "a duration between 0 and one year"
		}
		return int64(d), ""
	default:
		n, ok := intOf(cv)
		if !ok || n < 0 || n > countMax {
			return 0, "an integer between 0 and 2^31-1"
		}
		return n, ""
	}
}

// convertInterval reads a K2 value: the point sugar "60s" is
// { min = 60s, max = 60s, prefer = 60s } (N10), the object form states
// min, max and an optional prefer.
func (p *parser) convertInterval(path string, def fieldDef, cv cty.Value, inAll bool) (Value, bool) {
	var lo, hi int64
	var pref *int64
	if cv.Type().IsObjectType() {
		fields := map[string]cty.Value{}
		for it := cv.ElementIterator(); it.Next(); {
			k, e := it.Element()
			fields[k.AsString()] = e
		}
		for _, k := range sortedKeys(fields) {
			if k != "min" && k != "max" && k != "prefer" {
				p.unknown(path + "." + k)
				return Value{}, false
			}
		}
		mn, ok1 := fields["min"]
		mx, ok2 := fields["max"]
		if !ok1 || !ok2 {
			p.bad(path, "an interval states both min and max")
			return Value{}, false
		}
		var why string
		if lo, why = point(def, mn); why != "" {
			p.bad(path, why)
			return Value{}, false
		}
		if hi, why = point(def, mx); why != "" {
			p.bad(path, why)
			return Value{}, false
		}
		if lo > hi {
			p.bad(path, "min is greater than max")
			return Value{}, false
		}
		if pv, has := fields["prefer"]; has {
			if inAll {
				p.errs = append(p.errs, ErrFieldAuthority{Field: path + ".prefer", Authority: "all"})
				return Value{}, false
			}
			n, why := point(def, pv)
			if why != "" {
				p.bad(path, why)
				return Value{}, false
			}
			if n < lo || n > hi {
				p.bad(path, "prefer lies outside [min, max]")
				return Value{}, false
			}
			pref = &n
		}
	} else {
		n, why := point(def, cv)
		if why != "" {
			p.bad(path, why)
			return Value{}, false
		}
		lo, hi = n, n
		if !inAll {
			pref = &n
		}
	}
	v := Interval(lo, hi)
	if pref != nil {
		if p.l.Authority == Recipe {
			v.PrefR = []int64{*pref}
		} else {
			v.PrefS = []int64{*pref}
		}
	}
	return v, true
}

func (p *parser) edges(attr *hclsyntax.Attribute, kind string, m map[string]Value, inAll bool) {
	def := fieldDef{Class: ClassE}
	if !p.gate(kind, def, inAll) {
		return
	}
	cv, ok := p.literal(attr.Expr, kind)
	if !ok {
		return
	}
	l, ok := strList(cv)
	if !ok {
		p.bad(kind, "a list of service names")
		return
	}
	lvl := map[string]int64{"require": LevelRequired, "want": LevelWaived, "order": LevelOrder}[kind]
	for _, t := range l {
		if !nameRE.MatchString(t) {
			p.bad(kind, "a service name outside [A-Za-z0-9_.@-]{1,128}")
			continue
		}
		put(m, "edge/"+t, Level(K7, lvl))
	}
}

// object reads env = { KEY = "v" } and params = { name = "v" }.
func (p *parser) object(attr *hclsyntax.Attribute, name string, m map[string]Value, inAll bool, prefix string, envKeys bool) {
	class := ClassD
	if !envKeys {
		class = ClassP
	}
	if !p.gate(name, fieldDef{Class: class}, inAll) {
		return
	}
	cv, ok := p.literal(attr.Expr, name)
	if !ok {
		return
	}
	if !cv.Type().IsObjectType() && !cv.Type().IsMapType() {
		p.bad(name, "an object")
		return
	}
	for it := cv.ElementIterator(); it.Next(); {
		k, e := it.Element()
		key := k.AsString()
		if envKeys && !envKeyRE.MatchString(key) || !envKeys && !nameRE.MatchString(key) {
			p.bad(name, "key "+strconv.Quote(key))
			continue
		}
		s, ok := strOf(e)
		if !ok || strings.ContainsRune(s, 0) {
			p.bad(name+"."+key, "a string")
			continue
		}
		put(m, prefix+key, Flat(s))
	}
}

func (p *parser) paramBlock(blk *hclsyntax.Block, m map[string]Value, inAll bool, svc string) {
	if inAll {
		p.errs = append(p.errs, ErrFieldAuthority{Field: "param", Authority: "all"})
		return
	}
	name, ok := p.label(blk, "param")
	if !ok {
		return
	}
	vals := map[string]cty.Value{}
	for _, an := range sortedAttrNames(blk.Body) {
		cv, ok := p.literal(blk.Body.Attributes[an].Expr, "param."+name+"."+an)
		if !ok {
			return
		}
		vals[an] = cv
	}
	typ, _ := strOf(vals["type"])
	unknownAttrs := func(allowed ...string) bool {
		for _, k := range sortedKeys(vals) {
			if !slices.Contains(allowed, k) {
				p.unknown("param." + name + "." + k)
				return false
			}
		}
		return true
	}
	var enc string
	switch typ {
	case "int", "duration":
		if !unknownAttrs("type", "min", "max") {
			return
		}
		mn, ok1 := vals["min"]
		mx, ok2 := vals["max"]
		if !ok1 || !ok2 {
			p.bad("param."+name, "an interval domain states min and max")
			return
		}
		def := fieldDef{Unit: UnitCount}
		if typ == "duration" {
			def.Unit = UnitDuration
		}
		lo, whyLo := point(def, mn)
		hi, whyHi := point(def, mx)
		for _, why := range []string{whyLo, whyHi} {
			if why != "" {
				p.bad("param."+name, why)
			}
		}
		if whyLo != "" || whyHi != "" {
			return
		}
		if lo > hi {
			p.bad("param."+name, "min is greater than max")
			return
		}
		enc = fmt.Sprintf("%s:%d:%d", typ, lo, hi)
	case "enum":
		if !unknownAttrs("type", "values") {
			return
		}
		l, ok := strList(vals["values"])
		if !ok || len(l) == 0 {
			p.bad("param."+name, "enum states a non-empty list of values")
			return
		}
		for _, s := range l {
			if strings.ContainsAny(s, ",") {
				p.bad("param."+name, "enum value contains a comma")
				return
			}
		}
		enc = "enum:" + strings.Join(sortedDistinct(l), ",")
	case "port", "path":
		if !unknownAttrs("type") {
			return
		}
		enc = typ
	default:
		// no type, "string", or anything free-form: section 1.6 (N7)
		p.errs = append(p.errs, ErrUntypedParam{Service: svc, Param: name})
		return
	}
	put(m, "paramdecl/"+name, Flat(enc))
}

func (p *parser) credBlock(blk *hclsyntax.Block, m map[string]Value, inAll bool) {
	if inAll {
		p.errs = append(p.errs, ErrFieldAuthority{Field: "credential", Authority: "all"})
		return
	}
	name, ok := p.label(blk, "credential")
	if !ok {
		return
	}
	put(m, "cred/"+name, Flat("1"))
	for _, an := range sortedAttrNames(blk.Body) {
		if an != "source" {
			p.unknown("credential." + an)
			continue
		}
		if p.l.Authority == Recipe {
			p.errs = append(p.errs, ErrFieldAuthority{Field: "credential." + name + ".source", Authority: "recipe"})
			continue
		}
		cv, ok := p.literal(blk.Body.Attributes[an].Expr, "credential."+name+".source")
		if !ok {
			continue
		}
		s, ok := strOf(cv)
		if !ok || s == "" || strings.ContainsRune(s, 0) {
			p.bad("credential."+name+".source", "a non-empty string")
			continue
		}
		put(m, "cred/"+name+"/source", Flat(s))
	}
}

func (p *parser) protectBlock(blk *hclsyntax.Block, m map[string]Value, inAll bool) {
	name, ok := p.label(blk, "protect")
	if !ok {
		return
	}
	pd, ok := p.sc.protection(name)
	if !ok || pd.Since > p.running {
		p.unknown("protect." + name)
		return
	}
	for _, an := range sortedAttrNames(blk.Body) {
		attr := blk.Body.Attributes[an]
		switch an {
		case "level":
			p.field(attr, "protect/"+name, m, inAll)
		case "exempt_on":
			path := "protect/" + name + "/exempt_on"
			cv, ok := p.literal(attr.Expr, path)
			if !ok {
				continue
			}
			l, ok := strList(cv)
			if !ok {
				p.bad(path, "a list of OS names")
				continue
			}
			if i := slices.IndexFunc(l, func(s string) bool { return !slices.Contains(osNames, s) }); i >= 0 {
				p.bad(path, "unknown OS "+strconv.Quote(l[i]))
				continue
			}
			put(m, path, SetOf(K5, l...))
		default:
			p.unknown("protect." + name + "." + an)
		}
	}
}

func (p *parser) apiBody(b *hclsyntax.Body, api map[string]Value) {
	for _, an := range sortedAttrNames(b) {
		if an != "admin" && an != "read" {
			p.unknown("api." + an)
			continue
		}
		cv, ok := p.literal(b.Attributes[an].Expr, "api."+an)
		if !ok {
			continue
		}
		l, ok := strList(cv)
		if !ok {
			p.bad("api."+an, "a list of principals")
			continue
		}
		put(api, "api."+an, SetOf(K5, l...))
	}
	for _, blk := range b.Blocks {
		if blk.Type != "grant" {
			p.unknown("api." + blk.Type)
			continue
		}
		name, ok := p.label(blk, "grant")
		if !ok {
			continue
		}
		for _, an := range sortedAttrNames(blk.Body) {
			if an != "services" {
				p.unknown("grant." + an)
				continue
			}
			cv, ok := p.literal(blk.Body.Attributes[an].Expr, "grant."+name)
			if !ok {
				continue
			}
			l, ok := strList(cv)
			if !ok {
				p.bad("grant."+name, "a list of service names")
				continue
			}
			put(api, "api.grant/"+name, SetOf(K5, l...))
		}
		if _, has := api["api.grant/"+name]; !has && len(blk.Body.Attributes) == 0 {
			// a grant block with no services is the empty grant, not no grant
			put(api, "api.grant/"+name, SetOf(K5))
		}
	}
}
