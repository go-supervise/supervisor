package surface

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Options select what the source half analyses.
type Options struct {
	Dir    string   // module root
	Entry  string   // package pattern of the entry, default ./cmd/svd-monitor
	GOARCH string   // amd64 or arm64
	Tags   []string // build tags (the canary_* tags of the negative control)
	// AttrConstructors are the functions (a trailing dot: every function of a
	// package) that may build a syscall.SysProcAttr; nil means the monitor's one,
	// spawn.Start. Every other reference to the type is refused.
	AttrConstructors []string
}

// spawnStart is the one function of the monitor that builds a SysProcAttr.
const spawnStart = "github.com/go-supervise/supervisor/internal/spawn.Start"

// importAllowed are the imports of unsafe, reflect and C that a package outside
// the standard library may make ("<package> <import>"); asmAllowed likewise its
// assembly files ("<package> <file>"). Everything else is refused: reflect can
// set any field, unsafe any memory, and assembly issues any system call.
//
// golang.org/x/sys/unix is the system-call layer itself: its unsafe pointers
// and its assembly trampolines are the sinks, whose numbers are all checked.
var importAllowed = map[string]bool{
	unixPath + " unsafe": true,
}
var asmAllowed = map[string]bool{
	unixPath + " asm_linux_amd64.s": true,
	unixPath + " asm_linux_arm64.s": true,
}

// Result is a half's verdict: Failures empty means green.
type Result struct {
	Reached  []string // system calls reached, sorted
	Failures []string // one line each, sorted
	// Chains is, for each reached system call, one call path that reaches it
	// (a diagnosis aid; not part of the verdict).
	Chains map[string][]string
}

const unixPath = "golang.org/x/sys/unix"

// sinks are the raw entry points; a call from one of them to another is the
// wrapper chain itself and is exempt (its number is a parameter there).
var sinks = map[string]bool{
	"syscall.Syscall": true, "syscall.Syscall6": true, "syscall.Syscall9": true,
	"syscall.RawSyscall": true, "syscall.RawSyscall6": true,
	"syscall.RawSyscallNoError": true, "syscall.SyscallNoError": true,
	"syscall.rawSyscallNoError": true, "syscall.rawVforkSyscall": true,
	unixPath + ".Syscall": true, unixPath + ".Syscall6": true,
	unixPath + ".Syscall9": true, unixPath + ".RawSyscall": true,
	unixPath + ".RawSyscall6": true, unixPath + ".SyscallNoError": true,
	unixPath + ".RawSyscallNoError":           true,
	"internal/runtime/syscall/linux.Syscall6": true,
	// the all-threads variants (set*id and set groups go through them); the
	// number is the first argument of each, and the bodyless target is the
	// runtime's own entry.
	"syscall.AllThreadsSyscall": true, "syscall.AllThreadsSyscall6": true,
	"syscall.runtime_doAllThreadsSyscall": true,
}

// pkgOf is the package a function was written in: an instantiated generic has
// none of its own, its origin does.
func pkgOf(f *ssa.Function) *ssa.Package {
	if f.Pkg == nil {
		if o := f.Origin(); o != nil {
			return o.Pkg
		}
	}
	return f.Pkg
}

// topOf is the outermost function f was written in (a closure's enclosing
// function, an instantiation's generic).
func topOf(f *ssa.Function) *ssa.Function {
	for {
		if o := f.Origin(); o != nil {
			f = o
		}
		if f.Parent() == nil {
			return f
		}
		f = f.Parent()
	}
}

// bodylessAllowed are the exact functions outside the standard library that may
// have no Go body (assembly or linkname). A reached function with no body is
// invisible to the call graph, so each one must be listed; the standard
// library's own are the runtime's and are judged by the dynamic half (R_go).
var bodylessAllowed = map[string]bool{}

// noBody reports whether f is a reached Go function declared without a body
// that is neither a sink, nor the standard library's, nor allowlisted.
func (a *analysis) noBody(f *ssa.Function) bool {
	if f.Pkg == nil || f.Synthetic != "" || len(f.Blocks) > 0 {
		return false
	}
	name := qname(f)
	return !sinks[name] && !a.std[f.Pkg.Pkg.Path()] && !bodylessAllowed[name]
}

// dirfdParams are the indexes of the parameters of f that are directory
// descriptors, by the name the system-call wrappers give them ("dirfd",
// "olddirfd", "newdirfd"): the argument there must not be the constant AT_FDCWD
// (-100). The receiver of a method counts as the first parameter.
func dirfdParams(f *ssa.Function) []int {
	var out []int
	for i, p := range f.Params {
		if strings.HasSuffix(strings.ToLower(p.Name()), "dirfd") {
			out = append(out, i)
		}
	}
	return out
}

// unresolvable calls make a reached call the analysis cannot resolve.
var unresolvable = map[string]bool{
	"(reflect.Value).Call": true, "(reflect.Value).CallSlice": true,
	"(reflect.Value).MethodByName": true,
}

// qname is the function's full name: "path.Name", or "(T).Name" for a method.
func qname(f *ssa.Function) string { return f.String() }

// goVersionOf names the toolchain that builds dir under env; a variable so a
// test can stand another toolchain in.
var goVersionOf = func(dir string, env []string) (string, error) {
	gv := exec.Command("go", "env", "GOVERSION")
	gv.Dir, gv.Env = dir, env
	out, err := gv.Output()
	return strings.TrimSpace(string(out)), err
}

// SourceHalf is half 1 of 7.1.2: the system calls reachable from the entry's
// main over the VTA call graph of the whole program, each with a constant
// number, checked against A_M u R_go and Never.
func SourceHalf(o Options) (*Result, error) {
	if o.Entry == "" {
		o.Entry = "./cmd/svd-monitor"
	}
	env := append(os.Environ(), "GOOS=linux", "GOARCH="+o.GOARCH, "CGO_ENABLED=0")
	// the analysed program is the one this toolchain builds: it must be the
	// toolchain the R_go table was generated for
	built, err := goVersionOf(o.Dir, env)
	if err != nil {
		return nil, err
	}
	if err := CheckToolchain(built, o.GOARCH); err != nil {
		return nil, err
	}
	var flags []string
	if len(o.Tags) > 0 {
		flags = []string{"-tags=" + strings.Join(o.Tags, ",")}
	}
	// x/sys/unix is loaded beside the entry, whether or not the entry imports
	// it, because its SYS_* constants are the table of system-call names.
	all, err := packages.Load(&packages.Config{Mode: packages.LoadAllSyntax | packages.NeedModule, Dir: o.Dir, Env: env, BuildFlags: flags}, o.Entry, unixPath)
	if err != nil {
		return nil, err
	}
	bad := 0
	packages.Visit(all, nil, func(p *packages.Package) { bad += len(p.Errors) })
	if bad > 0 {
		return nil, fmt.Errorf("surface: %d package errors loading %s", bad, o.Entry)
	}
	var pkgs []*packages.Package
	mainIdx := -1
	for _, p := range all {
		if p.PkgPath == unixPath {
			pkgs = append(pkgs, p)
			continue
		}
		if mainIdx >= 0 {
			return nil, fmt.Errorf("surface: %s matched more than one package", o.Entry)
		}
		mainIdx = len(pkgs)
		pkgs = append(pkgs, p)
	}
	if mainIdx < 0 {
		return nil, fmt.Errorf("surface: %s matched no package", o.Entry)
	}
	prog, ssaPkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	mainPkg := ssaPkgs[mainIdx]
	roots := []*ssa.Function{mainPkg.Func("init"), mainPkg.Func("main")}
	g := vta.CallGraph(ssautil.AllFunctions(prog), cha.CallGraph(prog))

	a := &analysis{o: o, prog: prog, g: g, names: sysNames(o.GOARCH), parent: map[*ssa.Function]*callgraph.Edge{}}
	a.res.Chains = map[string][]string{}
	a.module = modulePath(pkgs[mainIdx])
	// the standard library is what no module holds; a path's spelling says
	// nothing (a module may be called "evil", and replaced by a directory)
	a.std = map[string]bool{}
	packages.Visit(all, func(p *packages.Package) bool {
		if p.Module == nil {
			a.std[p.PkgPath] = true
		}
		return true
	}, nil)
	// a function the runtime calls with no call edge (a finalizer, a cleanup)
	// is reached too: every function of the module is a root
	a.rt = a.runtimePackages()
	funcs := ssautil.AllFunctions(prog)
	a.reach(append(roots, a.moduleRoots(funcs)...))
	a.checkSinks(funcs)
	a.checkSites()
	a.checkAttr()
	a.checkImports(pkgs)
	a.compare()
	a.res.Failures = sortedUnique(a.res.Failures)
	sort.Strings(a.res.Reached)
	return &a.res, nil
}

func sortedUnique(l []string) []string {
	sort.Strings(l)
	out := l[:0]
	for i, s := range l {
		if i == 0 || s != l[i-1] {
			out = append(out, s)
		}
	}
	return out
}

type analysis struct {
	o       Options
	prog    *ssa.Program
	g       *callgraph.Graph
	names   map[int64]string
	module  string
	std     map[string]bool                   // import paths no module holds: the standard library
	rt      map[*ssa.Package]bool             // the runtime's own packages
	parent  map[*ssa.Function]*callgraph.Edge // BFS tree: how a function was first reached
	order   []*ssa.Function
	inOrder map[*ssa.Function]bool
	res     Result
	first   map[string]string // syscall name -> position of its first reported site
}

func modulePath(p *packages.Package) string {
	if p.Module != nil {
		return p.Module.Path
	}
	return ""
}

func (a *analysis) reach(roots []*ssa.Function) {
	seen := map[*ssa.Function]bool{}
	var queue []*ssa.Function
	for _, r := range roots {
		if r != nil && !seen[r] {
			seen[r] = true
			queue = append(queue, r)
		}
	}
	for len(queue) > 0 {
		f := queue[0]
		queue = queue[1:]
		a.order = append(a.order, f)
		n := a.g.Nodes[f]
		if n == nil {
			continue
		}
		for _, e := range n.Out {
			if c := e.Callee.Func; !seen[c] {
				seen[c] = true
				a.parent[c] = e
				queue = append(queue, c)
			}
		}
	}
}

// moduleRoots are the functions of the module (methods, closures and
// instantiations included), in a fixed order.
func (a *analysis) moduleRoots(funcs map[*ssa.Function]bool) []*ssa.Function {
	var out []*ssa.Function
	for f := range funcs {
		if a.mine(f) {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// absentSinks are the sinks go1.27 does not define on the linux ports checked
// (amd64, arm64): kept as guards in case another toolchain or port brings them
// back. Every other sink must resolve in the program.
var absentSinks = map[string]bool{
	"syscall.Syscall9": true, "syscall.RawSyscallNoError": true,
	"syscall.SyscallNoError": true, unixPath + ".Syscall9": true,
}

// checkSinks refuses a sink that names no function of the analysed program: a
// stale name (a package renamed by a toolchain) would guard nothing, silently.
func (a *analysis) checkSinks(funcs map[*ssa.Function]bool) {
	have := map[string]bool{}
	for f := range funcs {
		have[qname(f)] = true
	}
	for s := range sinks {
		if !have[s] && !absentSinks[s] {
			a.fail("the sink %s names no function of the analysed program", s)
		}
	}
}

func (a *analysis) pos(p token.Pos) string {
	s := a.prog.Fset.Position(p).String()
	if abs, err := filepath.Abs(a.o.Dir); err == nil {
		s = strings.TrimPrefix(s, abs+"/")
	}
	return s
}

func (a *analysis) mine(f *ssa.Function) bool {
	p := pkgOf(f)
	return p != nil && inModule(p.Pkg.Path(), a.module)
}

// inModule reports whether the package path is the module's root package or one
// below it ("mod" and "mod/x", never "modx").
func inModule(path, module string) bool {
	return module != "" && (path == module || strings.HasPrefix(path, module+"/"))
}

// callSite names where M_p's own code reaches the call that e makes: the
// nearest call, walking up the BFS tree from e, whose caller is svd code and
// whose callee is not; else the site of e itself.
func (a *analysis) callSite(e *callgraph.Edge) string {
	best := e.Site
	for cur := e; cur != nil; cur = a.parent[cur.Caller.Func] {
		if a.mine(cur.Caller.Func) && !a.mine(cur.Callee.Func) {
			best = cur.Site
			break
		}
	}
	if best == nil {
		return "(no position)"
	}
	return a.pos(best.Pos())
}

// constGlobals are the two package variables that hold a constant system-call
// number (fcntl64Syscall = SYS_FCNTL on every 64-bit port); no other global is
// resolved, so a variable of svd's own is a non-constant number.
var constGlobals = map[string]bool{"syscall.fcntl64Syscall": true, unixPath + ".fcntl64Syscall": true}

// globalConst reads the value a package's init stores into g.
func (a *analysis) globalConst(g *ssa.Global) (int64, bool) {
	if !constGlobals[g.Pkg.Pkg.Path()+"."+g.Name()] {
		return 0, false
	}
	// every ssa package has its synthetic init, empty when nothing is stored
	for _, b := range g.Pkg.Func("init").Blocks {
		for _, in := range b.Instrs {
			if s, ok := in.(*ssa.Store); ok && s.Addr == g {
				return a.constInt(s.Val)
			}
		}
	}
	return 0, false
}

func (a *analysis) constInt(v ssa.Value) (int64, bool) {
	for {
		switch x := v.(type) {
		case *ssa.UnOp:
			g, ok := x.X.(*ssa.Global)
			if x.Op != token.MUL || !ok {
				return 0, false
			}
			return a.globalConst(g)
		case *ssa.Const:
			if x.Value == nil || x.Value.Kind() != constant.Int {
				return 0, false
			}
			return constant.Int64Val(x.Value)
		case *ssa.Convert:
			v = x.X
		case *ssa.ChangeType:
			v = x.X
		default:
			return 0, false
		}
	}
}

func (a *analysis) fail(format string, args ...any) {
	a.res.Failures = append(a.res.Failures, "source: "+fmt.Sprintf(format, args...))
}

func (a *analysis) checkSites() {
	gated := Set(ForkChildGated)
	reached := map[string]bool{}
	a.first = map[string]string{}
	for _, f := range a.order {
		if unresolvable[f.String()] {
			a.fail("unresolved call %s is reachable (%s)", f.String(), a.chainSite(f))
		}
		if a.noBody(f) {
			a.fail("reached function %s has no Go body (%s)", f.String(), a.chainSite(f))
		}
		n := a.g.Nodes[f]
		if n == nil {
			continue
		}
		for _, e := range n.Out {
			callee := qname(e.Callee.Func)
			if e.Site == nil {
				continue
			}
			args := e.Site.Common().Args
			switch {
			case sinks[callee] && !sinks[qname(f)] && !a.runtimeOwn(f):
				nr, ok := a.constInt(args[0])
				if !ok {
					a.fail("non-constant system-call number at %s (in %s)", a.pos(e.Site.Pos()), f.String())
					continue
				}
				name, known := a.names[nr]
				if !known {
					name = fmt.Sprintf("syscall#%d", nr)
				}
				if gated[name] && forkChild(f) {
					continue // behind a SysProcAttr field, which checkAttr refuses
				}
				if !reached[name] {
					reached[name] = true
					a.first[name] = a.callSite(e)
					a.res.Chains[name] = a.chain(f)
				}
			}
			// a call that takes a directory descriptor (openat, mkdirat, unlinkat,
			// fchownat, newfstatat, statx, ...) must not be given AT_FDCWD
			if !forkChild(f) && !a.onlyFromForkChild(f) {
				for _, i := range dirfdParams(e.Callee.Func) {
					if i < len(args) {
						if v, ok := a.constInt(args[i]); ok && v == -100 {
							name := strings.ToLower(e.Callee.Func.Name())
							a.fail("%s with AT_FDCWD at %s", name, a.callSite(e))
							a.res.Chains[name+"(AT_FDCWD)"] = a.chain(f)
						}
					}
				}
			}
		}
	}
	for n := range reached {
		a.res.Reached = append(a.res.Reached, n)
	}
}

// runtimeOwn reports whether f is the Go runtime's own code, whose system calls
// are what R_go records and the dynamic half judges. Only a package of the
// standard library counts: no module can pass for it by its name.
func (a *analysis) runtimeOwn(f *ssa.Function) bool {
	return a.rt[pkgOf(f)]
}

// runtimePackages are the packages of the program that make up the Go runtime.
func (a *analysis) runtimePackages() map[*ssa.Package]bool {
	out := map[*ssa.Package]bool{}
	for _, p := range a.prog.AllPackages() {
		path := p.Pkg.Path()
		if a.std[path] && (path == "runtime" || strings.HasPrefix(path, "internal/runtime/")) {
			out[p] = true
		}
	}
	return out
}

// forkChild reports whether f is part of Go's fork child or its ID-mapping
// writers, the code that SysProcAttr fields switch on.
func forkChild(f *ssa.Function) bool {
	if f.Pkg == nil || f.Pkg.Pkg.Path() != "syscall" {
		return false
	}
	switch f.Name() {
	case "writeUidGidMappings", "writeIDMappings", "writeSetgroups":
		return true
	}
	return strings.HasPrefix(f.Name(), "forkAndExecInChild")
}

// onlyFromForkChild reports whether every reachable caller of f is itself
// fork-child code (syscall.Open, reached only from the ID-mapping writers).
func (a *analysis) onlyFromForkChild(f *ssa.Function) bool {
	n := a.g.Nodes[f]
	if n == nil || len(n.In) == 0 {
		return false
	}
	if a.inOrder == nil {
		a.inOrder = map[*ssa.Function]bool{}
		for _, g := range a.order {
			a.inOrder[g] = true
		}
	}
	found := false
	for _, e := range n.In {
		c := e.Caller.Func
		if !a.inOrder[c] {
			continue
		}
		if !forkChild(c) {
			return false
		}
		found = true
	}
	return found
}

// chain is the BFS path from a root to f, for diagnosis (the CLI's --why).
func (a *analysis) chain(f *ssa.Function) []string {
	var out []string
	for cur := f; cur != nil; {
		out = append([]string{cur.String()}, out...)
		e := a.parent[cur]
		if e == nil {
			break
		}
		cur = e.Caller.Func
	}
	return out
}

func (a *analysis) chainSite(f *ssa.Function) string {
	if e := a.parent[f]; e != nil {
		return a.callSite(e)
	}
	return "(root)"
}

// compare checks the reached set against Never and A_M u R_go.
func (a *analysis) compare() {
	allowed, never := Allowed(a.o.GOARCH), Set(Never)
	for _, n := range a.res.Reached {
		switch {
		case never[n] && !(n == "tgkill" && allowed[n]):
			a.fail("reached system call %q at %s (never allowed)", n, a.first[n])
		case !allowed[n]:
			a.fail("reached system call %q at %s (in none of A_M and R_go)", n, a.first[n])
		}
	}
}

// The kinds of reference to the types that carry the fork child's privileges.
const (
	kindNone  = iota
	kindAttr  // syscall.SysProcAttr itself
	kindOther // syscall.Credential, or a type whose struct is identical to either
)

// attrTypes holds the underlying structs of syscall.SysProcAttr and
// syscall.Credential, to recognise a defined type or an alias that is one.
type attrTypes struct{ attr, cred *types.Struct }

func (a *analysis) attrTypes() attrTypes {
	var k attrTypes
	if p := a.prog.ImportedPackage("syscall"); p != nil {
		if o := p.Pkg.Scope().Lookup("SysProcAttr"); o != nil {
			k.attr, _ = o.Type().Underlying().(*types.Struct)
		}
		if o := p.Pkg.Scope().Lookup("Credential"); o != nil {
			k.cred, _ = o.Type().Underlying().(*types.Struct)
		}
	}
	return k
}

// kind says whether t is, or is made of, one of the two types: through
// aliases, pointers, slices, arrays, maps, channels and result tuples, and by
// layout for a defined or unnamed struct.
func (k attrTypes) kind(t types.Type) int { return k.walk(t, map[types.Type]bool{}) }

// isAttr reports whether t is a struct of the layout of syscall.SysProcAttr
// (struct tags ignored: a tag changes nothing about a conversion).
func (k attrTypes) isAttr(t types.Type) bool {
	s, ok := types.Unalias(t).Underlying().(*types.Struct)
	return ok && k.attr != nil && types.IdenticalIgnoreTags(s, k.attr)
}

// walk is kind, with the types already entered (a recursive type ends there).
// A type that holds one of the two, in a field, an element or a result, counts
// as that one: syscall.ProcAttr and exec.Cmd carry a *SysProcAttr.
func (k attrTypes) walk(t types.Type, seen map[types.Type]bool) int {
	switch x := types.Unalias(t).(type) {
	case *types.Pointer:
		return k.walk(x.Elem(), seen)
	case *types.Slice:
		return k.walk(x.Elem(), seen)
	case *types.Array:
		return k.walk(x.Elem(), seen)
	case *types.Chan:
		return k.walk(x.Elem(), seen)
	case *types.Map:
		return max(k.walk(x.Key(), seen), k.walk(x.Elem(), seen))
	case *types.Tuple:
		r := kindNone
		for i := 0; i < x.Len(); i++ {
			r = max(r, k.walk(x.At(i).Type(), seen))
		}
		return r
	case *types.Named:
		if seen[x] {
			return kindNone
		}
		seen[x] = true
		if o := x.Obj(); o.Pkg() != nil && o.Pkg().Path() == "syscall" {
			switch o.Name() {
			case "SysProcAttr":
				return kindAttr
			case "Credential":
				return kindOther
			}
		}
		return k.walk(x.Underlying(), seen)
	case *types.Struct:
		if k.attr != nil && types.IdenticalIgnoreTags(x, k.attr) || k.cred != nil && types.IdenticalIgnoreTags(x, k.cred) {
			return kindOther
		}
		r := kindNone
		for i := 0; i < x.NumFields(); i++ {
			r = max(r, k.walk(x.Field(i).Type(), seen))
		}
		return r
	}
	return kindNone
}

func isDebugRef(in ssa.Instruction) bool { _, ok := in.(*ssa.DebugRef); return ok }

// isAttrField reports whether fa is the address of the named field of a
// SysProcAttr (or of a type of its layout).
func (k attrTypes) isAttrField(fa *ssa.FieldAddr, name string) bool {
	pt, ok := types.Unalias(fa.X.Type()).Underlying().(*types.Pointer)
	if !ok || !k.isAttr(pt.Elem()) {
		return false
	}
	s := types.Unalias(pt.Elem()).Underlying().(*types.Struct)
	return s.Field(fa.Field).Name() == name
}

// cloneflagsWithin says whether every value v can take is a constant inside
// mask: a constant, a conversion of one, an Or of two, an And with one, a phi
// of such. Anything else (a parameter, a load, a call result) is not.
func cloneflagsWithin(v ssa.Value, mask uint64) bool {
	return cloneflagsWithinSeen(v, mask, map[ssa.Value]bool{})
}

func cloneflagsWithinSeen(v ssa.Value, mask uint64, seen map[ssa.Value]bool) bool {
	if seen[v] {
		return true // a loop of phis adds no value of its own
	}
	seen[v] = true
	switch x := v.(type) {
	case *ssa.Const:
		// a value that is no integer (or none at all) converts to the unknown
		// constant, which has no Uint64 value
		u, ok := constant.Uint64Val(constant.ToInt(x.Value))
		return ok && u&^mask == 0
	case *ssa.Convert:
		return cloneflagsWithinSeen(x.X, mask, seen)
	case *ssa.ChangeType:
		return cloneflagsWithinSeen(x.X, mask, seen)
	case *ssa.Phi:
		for _, e := range x.Edges {
			if !cloneflagsWithinSeen(e, mask, seen) {
				return false
			}
		}
		return true
	case *ssa.BinOp:
		switch x.Op {
		case token.OR:
			return cloneflagsWithinSeen(x.X, mask, seen) && cloneflagsWithinSeen(x.Y, mask, seen)
		case token.AND:
			return cloneflagsWithinSeen(x.X, mask, seen) || cloneflagsWithinSeen(x.Y, mask, seen)
		}
	}
	return false
}

// origin says whether v is a value the allowed constructor may hold: an
// allocation made in f, a field address of one, or the nil constant. A
// parameter, a free variable, a global, a conversion, a load, a phi of
// anything and a call result are not.
func (k attrTypes) origin(v ssa.Value, f *ssa.Function) bool {
	switch x := v.(type) {
	case *ssa.Alloc:
		return x.Parent() == f
	case *ssa.FieldAddr:
		a, ok := x.X.(*ssa.Alloc)
		return ok && a.Parent() == f
	case *ssa.Const:
		return x.IsNil()
	}
	return false
}

// ctorAllowed reports whether f is written inside one of the functions allowed
// to build a SysProcAttr.
func ctorAllowed(f *ssa.Function, ctors []string) bool {
	return nameAllowed(qname(topOf(f)), ctors)
}

// nameAllowed matches a function's name against the allowed constructors: an
// exact name, or, for an entry ending in a dot, any function of that package.
func nameAllowed(top string, ctors []string) bool {
	for _, c := range ctors {
		if top == c || strings.HasSuffix(c, ".") && strings.HasPrefix(top, c) {
			return true
		}
	}
	return false
}

// checkAttr refuses every reference to syscall.SysProcAttr or syscall.Credential
// (an alias, a defined type of the same struct, a conversion, a literal, a
// field, a variable holding one) in reached code outside the standard library,
// except in the allowed constructor, where only the fields of AllowedAttr may
// be set and the value must be a plain composite literal. Following each write
// would always miss one; refusing the type leaves none.
func (a *analysis) checkAttr() {
	ok := Set(AllowedAttr)
	ctors := a.o.AttrConstructors
	if ctors == nil {
		ctors = []string{spawnStart}
	}
	k := a.attrTypes()
	for _, f := range a.order {
		p := pkgOf(f)
		if p == nil || a.std[p.Pkg.Path()] {
			continue
		}
		allowed := ctorAllowed(f, ctors)
		field := func(in ssa.Instruction, x ssa.Value, idx int) {
			t := types.Unalias(x.Type())
			if p, isPtr := t.Underlying().(*types.Pointer); isPtr {
				t = types.Unalias(p.Elem())
			}
			if n, isNamed := t.(*types.Named); isNamed && k.kind(n) == kindAttr && k.isAttr(n) {
				if name := n.Underlying().(*types.Struct).Field(idx).Name(); !ok[name] {
					a.fail("SysProcAttr field %s set at %s", name, a.pos(in.Pos()))
				}
			}
		}
		for _, b := range f.Blocks {
			for _, in := range b.Instrs {
				worst := kindNone
				if v, isVal := in.(ssa.Value); isVal {
					worst = k.kind(v.Type())
				}
				for _, op := range in.Operands(nil) {
					if *op != nil {
						worst = max(worst, k.kind((*op).Type()))
					}
				}
				switch {
				case worst == kindOther:
					a.fail("syscall.Credential, or a type of the layout of it or of SysProcAttr, is used in %s at %s", f, a.pos(in.Pos()))
				case worst == kindAttr && !allowed:
					a.fail("syscall.SysProcAttr is used in %s at %s, outside the one function that may build it", f, a.pos(in.Pos()))
				}
				if !allowed {
					continue
				}
				// every value of the types that carry the privileges must come
				// from a literal built in this function: not from a parameter,
				// a conversion, a load, a phi or a call
				if v, isVal := in.(ssa.Value); isVal && k.kind(v.Type()) != kindNone && !k.origin(v, f) {
					a.fail("a value of SysProcAttr or Credential (or holding one) comes from %T in %s at %s, not from a literal built there", v, f, a.pos(in.Pos()))
				}
				for _, op := range in.Operands(nil) {
					if *op != nil && k.kind((*op).Type()) != kindNone && !k.origin(*op, f) {
						a.fail("a value of SysProcAttr or Credential (or holding one) comes from %T in %s at %s, not from a literal built there", *op, f, a.pos(in.Pos()))
					}
				}
				switch in := in.(type) {
				case *ssa.FieldAddr:
					field(in, in.X, in.Field)
				case *ssa.Field:
					field(in, in.X, in.Field)
				case *ssa.MakeInterface:
					// a decoder or any reflective callee writes through an
					// interface, whatever the allocation was
					if k.kind(in.X.Type()) != kindNone {
						a.fail("a value of SysProcAttr or Credential (or holding one) is converted to an interface at %s", a.pos(in.Pos()))
					}
				case *ssa.Alloc:
					pt, isPtr := in.Type().(*types.Pointer)
					if !isPtr || k.kind(pt.Elem()) == kindNone {
						continue
					}
					// the allocation of the attribute, and of anything holding
					// one (a ProcAttr, a wrapper struct): only its fields are
					// written, and a holder may be passed to a call by type
					for _, r := range *in.Referrers() {
						switch r.(type) {
						case *ssa.FieldAddr, *ssa.Store, *ssa.DebugRef:
						case *ssa.Call:
							if k.isAttr(pt.Elem()) {
								a.fail("SysProcAttr is not a composite literal at %s", a.pos(r.Pos()))
							}
						default:
							a.fail("SysProcAttr is not a composite literal at %s", a.pos(r.Pos()))
						}
					}
				case *ssa.Store:
					if fa, isField := in.Addr.(*ssa.FieldAddr); isField && k.isAttrField(fa, "Cloneflags") {
						if !cloneflagsWithin(in.Val, AllowedCloneflags) {
							a.fail("SysProcAttr Cloneflags set at %s to a value that is not a constant within the mask %#x", a.pos(in.Pos()), uint64(AllowedCloneflags))
						}
						// and the field is only ever written: a loaded or escaping
						// address could change the flags after the check
						for _, r := range *fa.Referrers() {
							if s, isStore := r.(*ssa.Store); (!isStore || s.Addr != fa) && !isDebugRef(r) {
								a.fail("SysProcAttr Cloneflags is used other than by a store at %s", a.pos(r.Pos()))
							}
						}
					}
				}
			}
		}
	}
}

// checkImports refuses, in every package outside the standard library, cgo,
// unsafe and reflect, and assembly files, unless listed: none of them leaves a
// call or a field write for the other checks to see.
func (a *analysis) checkImports(pkgs []*packages.Package) {
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.Module == nil {
			return
		}
		for _, o := range p.OtherFiles {
			switch strings.ToLower(filepath.Ext(o)) {
			case ".s", ".syso":
				if !asmAllowed[p.PkgPath+" "+filepath.Base(o)] {
					a.fail("%s holds the non-Go file %s", p.PkgPath, filepath.Base(o))
				}
			}
		}
		refused := func(path string) bool {
			return (path == "unsafe" || path == "C" || path == "reflect") && !importAllowed[p.PkgPath+" "+path]
		}
		for path := range p.Imports {
			if refused(path) {
				a.fail("%s imports %s", p.PkgPath, path)
			}
		}
		for _, s := range p.Syntax {
			for _, im := range s.Imports {
				if path := strings.Trim(im.Path.Value, `"`); refused(path) {
					a.fail("%s imports %s", p.PkgPath, im.Path.Value)
				}
			}
		}
	})
}
