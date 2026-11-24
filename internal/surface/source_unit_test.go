package surface

import (
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
)

// noPanic runs f and turns a crash into a failed assertion.
func noPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("%s crashed: %v", what, p)
		}
	}()
	f()
}

func TestNoBodyIgnoresWhatHasNoPackage(t *testing.T) {
	noPanic(t, "noBody of a function with no package", func() {
		if noBody(&ssa.Function{}) {
			t.Error("a function with no package is not a bodyless one")
		}
	})
}

func TestSourceHalfRefusesAnotherToolchain(t *testing.T) {
	was := goVersionOf
	defer func() { goVersionOf = was }()
	o := Options{Dir: "../..", Entry: fixPkg + "ok", GOARCH: "amd64"}

	goVersionOf = func(string, []string) (string, error) { return "go1.25.4", nil }
	if _, err := SourceHalf(o); err == nil || !strings.Contains(err.Error(), "go1.25") {
		t.Errorf("a toolchain the table was not generated for: %v", err)
	}
	goVersionOf = func(string, []string) (string, error) { return "", os.ErrNotExist }
	if _, err := SourceHalf(o); err != os.ErrNotExist {
		t.Errorf("a toolchain that cannot be named: %v", err)
	}
}

func TestSourceHalfRefusesWhatItCannotLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages (the gate1 job runs it)")
	}
	load := func(o Options) (err error) {
		defer func() { // a crash is a failed assertion
			if p := recover(); p != nil {
				t.Errorf("%+v crashed: %v", o, p)
			}
		}()
		_, err = SourceHalf(o)
		return err
	}
	err := load(Options{Dir: filepath.Join(t.TempDir(), "absent"), GOARCH: "amd64"})
	if err == nil || strings.HasPrefix(err.Error(), "surface:") {
		t.Errorf("an absent directory: %v", err)
	}
	err = load(Options{Dir: "../..", Entry: fixPkg + "absent", GOARCH: "amd64"})
	if err == nil || !strings.HasPrefix(err.Error(), "surface: ") || !strings.HasSuffix(err.Error(), " package errors loading "+fixPkg+"absent") {
		t.Errorf("an absent entry: %v", err)
	}
	// x/sys/unix is always loaded beside the entry: an entry that is only it
	// leaves no package of its own
	err = load(Options{Dir: "../..", Entry: unixPath, GOARCH: "amd64"})
	if want := "surface: " + unixPath + " matched no package"; err == nil || err.Error() != want {
		t.Errorf("an entry that is only x/sys/unix: %v, want %q", err, want)
	}
	err = load(Options{Dir: "../..", Entry: fixPkg + "...", GOARCH: "amd64"})
	if want := "surface: " + fixPkg + "... matched more than one package"; err == nil || err.Error() != want {
		t.Errorf("a pattern matching several: %v, want %q", err, want)
	}
}

func TestCallSiteWithNoPositionSaysSo(t *testing.T) {
	a := &analysis{parent: map[*ssa.Function]*callgraph.Edge{}}
	e := &callgraph.Edge{Caller: &callgraph.Node{Func: &ssa.Function{}}, Callee: &callgraph.Node{Func: &ssa.Function{}}}
	noPanic(t, "callSite", func() {
		if got := a.callSite(e); got != "(no position)" {
			t.Errorf("callSite = %q", got)
		}
	})
}

func TestConstIntOnlyReadsIntegerConstants(t *testing.T) {
	a := &analysis{}
	str := ssa.NewConst(constant.MakeString("x"), types.Typ[types.String])
	nilc := ssa.NewConst(nil, types.NewPointer(types.Typ[types.Int]))
	five := ssa.NewConst(constant.MakeInt64(5), types.Typ[types.Int])
	for _, c := range []struct {
		name string
		v    ssa.Value
		n    int64
		ok   bool
	}{
		{"integer", five, 5, true},
		{"string", str, 0, false},
		{"nil", nilc, 0, false},
		{"a load of a constant, not of a global", &ssa.UnOp{Op: token.MUL, X: five}, 0, false},
		{"a negation of a global", &ssa.UnOp{Op: token.SUB, X: &ssa.Global{}}, 0, false},
	} {
		noPanic(t, c.name, func() {
			if n, ok := a.constInt(c.v); n != c.n || ok != c.ok {
				t.Errorf("%s: constInt = %d, %v; want %d, %v", c.name, n, ok, c.n, c.ok)
			}
		})
	}
	// through conversions
	cv := &ssa.Convert{X: &ssa.ChangeType{X: five}}
	if n, ok := a.constInt(cv); n != 5 || !ok {
		t.Errorf("converted: %d, %v", n, ok)
	}
}

func TestGlobalConstWithNothingStored(t *testing.T) {
	prog := ssa.NewProgram(token.NewFileSet(), 0)
	pkg := types.NewPackage("syscall", "syscall")
	pkg.Scope().Insert(types.NewVar(token.NoPos, pkg, "fcntl64Syscall", types.Typ[types.Int]))
	pkg.Scope().Insert(types.NewVar(token.NoPos, pkg, "other", types.Typ[types.Int]))
	sp := prog.CreatePackage(pkg, nil, nil, true)
	a := &analysis{}
	noPanic(t, "globalConst", func() {
		if n, ok := a.globalConst(sp.Members["fcntl64Syscall"].(*ssa.Global)); n != 0 || ok {
			t.Errorf("an empty init: %d, %v", n, ok)
		}
		if n, ok := a.globalConst(sp.Members["other"].(*ssa.Global)); n != 0 || ok {
			t.Errorf("a global that is not a constant: %d, %v", n, ok)
		}
	})
}

func TestForkChildNeedsAPackage(t *testing.T) {
	noPanic(t, "forkChild", func() {
		if forkChild(&ssa.Function{}) {
			t.Error("a function with no package is not fork-child code")
		}
	})
}

func TestForkChildIsSyscallsOwnCode(t *testing.T) {
	prog := ssa.NewProgram(token.NewFileSet(), 0)
	pkgOf := func(path string) *ssa.Package {
		return prog.CreatePackage(types.NewPackage(path, path), nil, nil, true)
	}
	sig := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	fn := func(p *ssa.Package, name string) *ssa.Function {
		f := prog.NewFunction(name, sig, "test")
		f.Pkg = p
		return f
	}
	sys, other := pkgOf("syscall"), pkgOf("example.test/syscall")
	for name, want := range map[string]bool{
		"writeUidGidMappings": true, "writeIDMappings": true, "writeSetgroups": true,
		"forkAndExecInChild": true, "forkAndExecInChild1": true,
		"forkExec": false, "Kill": false, "": false,
	} {
		if got := forkChild(fn(sys, name)); got != want {
			t.Errorf("syscall.%s: %v, want %v", name, got, want)
		}
		if forkChild(fn(other, name)) {
			t.Errorf("another package's %s is not fork-child code", name)
		}
	}
}

func TestOnlyFromForkChildNeedsCallers(t *testing.T) {
	f := &ssa.Function{}
	g := &callgraph.Graph{Nodes: map[*ssa.Function]*callgraph.Node{}}
	a := &analysis{g: g}
	noPanic(t, "no node", func() {
		if a.onlyFromForkChild(f) {
			t.Error("a function the graph does not hold has no fork-child callers")
		}
	})
	g.Nodes[f] = &callgraph.Node{Func: f}
	noPanic(t, "no callers", func() {
		if a.onlyFromForkChild(f) {
			t.Error("a function nothing calls is not fork-child only")
		}
	})
}

func TestIsSysProcAttr(t *testing.T) {
	pkg := types.NewPackage("syscall", "syscall")
	named := func(p *types.Package, name string) *types.Named {
		return types.NewNamed(types.NewTypeName(token.NoPos, p, name, nil), types.NewStruct(nil, nil), nil)
	}
	for _, c := range []struct {
		name string
		t    types.Type
		want bool
	}{
		{"*syscall.SysProcAttr", types.NewPointer(named(pkg, "SysProcAttr")), true},
		{"the value, not the pointer", named(pkg, "SysProcAttr"), false},
		{"an int", types.Typ[types.Int], false},
		{"a pointer to an int", types.NewPointer(types.Typ[types.Int]), false},
		{"another name", types.NewPointer(named(pkg, "Other")), false},
		{"another package", types.NewPointer(named(types.NewPackage("os", "os"), "SysProcAttr")), false},
		{"no package", types.NewPointer(named(nil, "SysProcAttr")), false},
	} {
		noPanic(t, c.name, func() {
			if got := isSysProcAttr(c.t); got != c.want {
				t.Errorf("%s: %v, want %v", c.name, got, c.want)
			}
		})
	}
}

func TestGenerateSysNumRefusesWhatItCannotLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages (the gate1 job runs it)")
	}
	var err error
	noPanic(t, "an absent directory", func() {
		_, err = GenerateSysNum(filepath.Join(t.TempDir(), "absent"), "amd64")
	})
	if err == nil || strings.HasPrefix(err.Error(), "surface: cannot load") {
		t.Errorf("an absent directory: %v", err)
	}
	// a module that does not hold x/sys
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/m\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = GenerateSysNum(dir, "amd64")
	if err == nil || !strings.HasPrefix(err.Error(), "surface: cannot load golang.org/x/sys/unix for amd64") {
		t.Errorf("a module without x/sys: %v", err)
	}
}
