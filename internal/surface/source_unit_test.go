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
		if (&analysis{}).noBody(&ssa.Function{}) {
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

func TestSourceHalfRefusesAnotherBuildEnvironment(t *testing.T) {
	was, wasEnv := goVersionOf, goEnvOf
	defer func() { goVersionOf, goEnvOf = was, wasEnv }()
	goVersionOf = func(string, []string) (string, error) { return "go1.27.0", nil }
	goEnvOf = func(string, []string, string) (string, error) { return "-race", nil }
	_, err := SourceHalf(Options{Dir: "../..", Entry: fixPkg + "ok", GOARCH: "amd64"})
	if err == nil || !strings.Contains(err.Error(), "GOFLAGS") {
		t.Errorf("a build under extra flags: %v", err)
	}
}

func TestSourceHalfReportsADriverFailure(t *testing.T) {
	was, wasEnv := goVersionOf, goEnvOf
	defer func() { goVersionOf, goEnvOf = was, wasEnv }()
	goVersionOf = func(string, []string) (string, error) { return "go1.27.0", nil }
	goEnvOf = func(string, []string, string) (string, error) { return "", nil }
	_, err := SourceHalf(Options{Dir: filepath.Join(t.TempDir(), "absent"), Entry: fixPkg + "ok", GOARCH: "amd64"})
	if err == nil || strings.HasPrefix(err.Error(), "surface:") {
		t.Errorf("a directory the package driver cannot enter: %v", err)
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

// recursive makes type R struct{ next *R; v extra }, extra nil for none.
func recursive(p *types.Package, name string, extra types.Type) *types.Named {
	n := types.NewNamed(types.NewTypeName(token.NoPos, p, name, nil), nil, nil)
	fs := []*types.Var{types.NewField(token.NoPos, p, "next", types.NewPointer(n), false)}
	if extra != nil {
		fs = append(fs, types.NewField(token.NoPos, p, "v", extra, false))
	}
	n.SetUnderlying(types.NewStruct(fs, nil))
	return n
}

func TestAttrKind(t *testing.T) {
	pkg := types.NewPackage("syscall", "syscall")
	field := func(name string) *types.Var {
		return types.NewField(token.NoPos, pkg, name, types.Typ[types.Int], false)
	}
	attrStruct := types.NewStruct([]*types.Var{field("Chroot"), field("Setsid")}, nil)
	credStruct := types.NewStruct([]*types.Var{field("Uid")}, nil)
	otherStruct := types.NewStruct([]*types.Var{field("Chroot")}, nil)
	named := func(p *types.Package, name string, u types.Type) *types.Named {
		return types.NewNamed(types.NewTypeName(token.NoPos, p, name, nil), u, nil)
	}
	k := attrTypes{attr: attrStruct, cred: credStruct}
	attr := named(pkg, "SysProcAttr", attrStruct)
	mine := types.NewPackage("example.test/m", "m")
	for _, c := range []struct {
		name string
		t    types.Type
		want int
	}{
		{"SysProcAttr", attr, kindAttr},
		{"*SysProcAttr", types.NewPointer(attr), kindAttr},
		{"[]*SysProcAttr", types.NewSlice(types.NewPointer(attr)), kindAttr},
		{"[2]SysProcAttr", types.NewArray(attr, 2), kindAttr},
		{"chan *SysProcAttr", types.NewChan(types.SendRecv, types.NewPointer(attr)), kindAttr},
		{"map[int]*SysProcAttr", types.NewMap(types.Typ[types.Int], types.NewPointer(attr)), kindAttr},
		{"map[*SysProcAttr]int", types.NewMap(types.NewPointer(attr), types.Typ[types.Int]), kindAttr},
		{"a result tuple", types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.Int]), types.NewVar(token.NoPos, nil, "", types.NewPointer(attr))), kindAttr},
		{"Credential", named(pkg, "Credential", credStruct), kindOther},
		{"a defined type of the SysProcAttr struct", named(mine, "B", attrStruct), kindOther},
		{"a pointer to a defined type of the Credential struct", types.NewPointer(named(mine, "C", credStruct)), kindOther},
		{"an unnamed struct of the layout", attrStruct, kindOther},
		{"a defined pointer type", named(mine, "P", types.NewPointer(attr)), kindAttr},
		{"an int", types.Typ[types.Int], kindNone},
		{"a pointer to an int", types.NewPointer(types.Typ[types.Int]), kindNone},
		{"another struct", named(mine, "O", otherStruct), kindNone},
		{"another name in syscall", named(pkg, "Other", otherStruct), kindNone},
		{"another package", named(types.NewPackage("os", "os"), "SysProcAttr", otherStruct), kindNone},
		{"no package", named(nil, "SysProcAttr", otherStruct), kindNone},
		{"an empty tuple", types.NewTuple(), kindNone},
		{"a struct holding a *SysProcAttr (ProcAttr, exec.Cmd)", named(mine, "H", types.NewStruct([]*types.Var{
			types.NewField(token.NoPos, mine, "Sys", types.NewPointer(attr), false)}, nil)), kindAttr},
		{"a struct holding a Credential", named(mine, "HC", types.NewStruct([]*types.Var{
			types.NewField(token.NoPos, mine, "C", named(pkg, "Credential", credStruct), false)}, nil)), kindOther},
		{"a struct holding a slice of a struct holding one", types.NewStruct([]*types.Var{
			types.NewField(token.NoPos, mine, "L", types.NewSlice(named(mine, "H2", types.NewStruct([]*types.Var{
				types.NewField(token.NoPos, mine, "S", attr, false)}, nil))), false)}, nil), kindAttr},
		{"a struct of the layout with other tags", types.NewStruct([]*types.Var{field("Chroot"), field("Setsid")}, []string{`a:"b"`, ""}), kindOther},
		{"a struct holding an int", named(mine, "HI", types.NewStruct([]*types.Var{field("N")}, nil)), kindNone},
		{"a recursive type holding nothing", recursive(mine, "R1", nil), kindNone},
		{"a recursive type holding one", recursive(mine, "R2", attr), kindAttr},
	} {
		noPanic(t, c.name, func() {
			if got := k.kind(c.t); got != c.want {
				t.Errorf("%s: kind %d, want %d", c.name, got, c.want)
			}
		})
	}
	// without the syscall package's structs only the names count
	noPanic(t, "no structs", func() {
		if got := (attrTypes{}).kind(named(mine, "B", attrStruct)); got != kindNone {
			t.Errorf("a layout with no reference struct: kind %d", got)
		}
		if got := (attrTypes{}).kind(attr); got != kindAttr {
			t.Errorf("the name with no reference struct: kind %d", got)
		}
	})
}

func TestNameAllowed(t *testing.T) {
	const fn = "example.test/m/spawn.Start"
	for _, c := range []struct {
		ctors []string
		want  bool
	}{
		{nil, false},
		{[]string{"example.test/m/spawn.Other"}, false},
		{[]string{fn}, true},
		{[]string{"x", fn}, true},
		{[]string{"example.test/m/spawn.Star"}, false}, // an exact name, not a prefix
		{[]string{"example.test/m/spawn."}, true},      // a package
		{[]string{"example.test/m/spa."}, false},
		{[]string{"example.test/m/spawn"}, false},
	} {
		if got := nameAllowed(fn, c.ctors); got != c.want {
			t.Errorf("nameAllowed(%q, %q) = %v, want %v", fn, c.ctors, got, c.want)
		}
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
