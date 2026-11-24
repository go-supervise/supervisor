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
	unixPath + ".RawSyscallNoError":     true,
	"internal/runtime/syscall.Syscall6": true,
	// the all-threads variants (set*id and set groups go through them); the
	// number is the first argument of each, and the bodyless target is the
	// runtime's own entry.
	"syscall.AllThreadsSyscall": true, "syscall.AllThreadsSyscall6": true,
	"syscall.runtime_doAllThreadsSyscall": true,
}

// stdlibPath reports whether an import path belongs to the Go standard
// library (no dot in its first element); a module path always has one.
func stdlibPath(p string) bool {
	first, _, _ := strings.Cut(p, "/")
	return !strings.Contains(first, ".")
}

// bodylessAllowed are the exact functions outside the standard library that may
// have no Go body (assembly or linkname). A reached function with no body is
// invisible to the call graph, so each one must be listed; the standard
// library's own are the runtime's and are judged by the dynamic half (R_go).
var bodylessAllowed = map[string]bool{}

// noBody reports whether f is a reached Go function declared without a body
// that is neither a sink, nor the standard library's, nor allowlisted.
func noBody(f *ssa.Function) bool {
	if f.Pkg == nil || f.Synthetic != "" || len(f.Blocks) > 0 {
		return false
	}
	name := qname(f)
	return !sinks[name] && !stdlibPath(f.Pkg.Pkg.Path()) && !bodylessAllowed[name]
}

// openers are the dirfd-taking open calls whose first argument must not be
// the constant AT_FDCWD (-100).
var openers = map[string]bool{
	"syscall.openat": true, "syscall.Openat": true,
	unixPath + ".openat": true, unixPath + ".Openat": true,
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
	a.reach(roots)
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

func (a *analysis) pos(p token.Pos) string {
	s := a.prog.Fset.Position(p).String()
	if abs, err := filepath.Abs(a.o.Dir); err == nil {
		s = strings.TrimPrefix(s, abs+"/")
	}
	return s
}

func (a *analysis) mine(f *ssa.Function) bool {
	return f.Pkg != nil && a.module != "" && strings.HasPrefix(f.Pkg.Pkg.Path(), a.module+"/")
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
		if noBody(f) {
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
			case sinks[callee] && !sinks[qname(f)]:
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
			case openers[callee] && len(args) > 0:
				if v, ok := a.constInt(args[0]); ok && v == -100 && !forkChild(f) && !a.onlyFromForkChild(f) {
					a.fail("openat with AT_FDCWD at %s", a.callSite(e))
					a.res.Chains["openat(AT_FDCWD)"] = a.chain(f)
				}
			}
		}
	}
	for n := range reached {
		a.res.Reached = append(a.res.Reached, n)
	}
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

// checkAttr refuses, in every reached function outside the standard library,
// each access to a syscall.SysProcAttr field outside AllowedAttr, whatever the
// base (a local, a package variable, a struct holding one, a pointer loaded
// back), and every SysProcAttr allocated there that is not a plain literal.
func (a *analysis) checkAttr() {
	ok := Set(AllowedAttr)
	field := func(in ssa.Instruction, x ssa.Value, idx int) {
		t := x.Type()
		if p, isPtr := t.Underlying().(*types.Pointer); isPtr {
			t = p.Elem()
		}
		if !isSysProcAttrStruct(t) {
			return
		}
		if name := t.Underlying().(*types.Struct).Field(idx).Name(); !ok[name] {
			a.fail("SysProcAttr field %s set at %s", name, a.pos(in.Pos()))
		}
	}
	for _, f := range a.order {
		if f.Pkg == nil || stdlibPath(f.Pkg.Pkg.Path()) {
			continue
		}
		for _, b := range f.Blocks {
			for _, in := range b.Instrs {
				switch in := in.(type) {
				case *ssa.FieldAddr:
					field(in, in.X, in.Field)
				case *ssa.Field:
					field(in, in.X, in.Field)
				case *ssa.Alloc:
					if !isSysProcAttr(in.Type()) {
						continue
					}
					for _, r := range *in.Referrers() {
						switch r.(type) {
						case *ssa.FieldAddr, *ssa.Store, *ssa.DebugRef:
						default:
							a.fail("SysProcAttr is not a composite literal at %s", a.pos(r.Pos()))
						}
					}
				}
			}
		}
	}
}

func isSysProcAttrStruct(t types.Type) bool {
	n, ok := t.(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "syscall" && n.Obj().Name() == "SysProcAttr"
}

func isSysProcAttr(t types.Type) bool {
	p, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	n, ok := p.Elem().(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "syscall" && n.Obj().Name() == "SysProcAttr"
}

// checkImports refuses cgo and unsafe in svd's own packages.
func (a *analysis) checkImports(pkgs []*packages.Package) {
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if a.module == "" || !strings.HasPrefix(p.PkgPath, a.module+"/") {
			return
		}
		// assembly can issue any system call with no Go call to see
		for _, o := range p.OtherFiles {
			switch strings.ToLower(filepath.Ext(o)) {
			case ".s", ".syso":
				a.fail("%s holds the non-Go file %s", p.PkgPath, filepath.Base(o))
			}
		}
		for path := range p.Imports {
			if path == "unsafe" || path == "C" {
				a.fail("%s imports %s", p.PkgPath, path)
			}
		}
		for _, s := range p.Syntax {
			for _, im := range s.Imports {
				if im.Path.Value == `"C"` || im.Path.Value == `"unsafe"` {
					a.fail("%s imports %s", p.PkgPath, im.Path.Value)
				}
			}
		}
	})
}
