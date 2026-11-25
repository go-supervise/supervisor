package surface

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const fixPkg = "./internal/surface/testdata/fix/"
const fixDir = "internal/surface/testdata/fix/"
const fixPrefix = "github.com/go-supervise/supervisor/" + fixDir + ""

func source(t *testing.T, arch, name string, tags ...string) *Result {
	t.Helper()
	if testing.Short() {
		t.Skip("source half: loads and analyses the whole program (the gate1 job runs it)")
	}
	o := Options{Dir: "../..", Entry: fixPkg + name, GOARCH: arch, Tags: tags}
	// the fixtures that exercise the checks made INSIDE the one function allowed
	// to build a SysProcAttr are that function; every other fixture is not
	switch name {
	case "attr", "attrglobal", "attrloaded", "forkok", "twinattr", "paramattr", "wrapperattr", "cloneparam", "cloneuser", "cloneok", "clonephi", "cloneloop", "clonelookalike":
		o.AttrConstructors = []string{"github.com/go-supervise/supervisor/" + fixDir + name + "."}
	}
	r, err := SourceHalf(o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSourceHalfFixtures(t *testing.T) {
	cases := []struct {
		name    string
		reached []string
		fail    []string
	}{
		// prlimit64 is Go's own start-up call (syscall's init), in every program.
		{"ok", []string{"prlimit64", "write"}, nil},
		{"never", []string{"kill", "prlimit64"}, []string{
			`source: reached system call "kill" at ` + fixDir + `never/main.go:7:15 (never allowed)`,
		}},
		{"unknown", []string{"prlimit64", "sysinfo"}, []string{
			`source: reached system call "sysinfo" at ` + fixDir + `unknown/main.go:8:18 (in none of A_M and R_go)`,
		}},
		{"rawnr", nil, []string{
			`source: non-constant system-call number at ` + fixDir + `rawnr/main.go:9:27 (in github.com/go-supervise/supervisor/internal/surface/testdata/fix/rawnr.main)`,
		}},
		{"atfdcwd", []string{"openat", "prlimit64"}, []string{
			`source: openat with AT_FDCWD at ` + fixDir + `atfdcwd/main.go:7:18`,
		}},
		{"attr", nil, []string{
			`source: SysProcAttr field Setpgid set at ` + fixDir + `attr/main.go:13:37`,
			`source: SysProcAttr is not a composite literal at ` + fixDir + `attr/main.go:15:6`,
			// and the values that arrive from a global or a parameter
			`source: a value of SysProcAttr or Credential (or holding one) comes from *ssa.Global in ` + fixPrefix + `attr.keep at ` + fixDir + `attr/main.go:9:37, not from a literal built there`,
			`source: a value of SysProcAttr or Credential (or holding one) comes from *ssa.Global in ` + fixPrefix + `attr.main at ` + fixDir + `attr/main.go:12:2, not from a literal built there`,
			`source: a value of SysProcAttr or Credential (or holding one) comes from *ssa.Global in ` + fixPrefix + `attr.main at ` + fixDir + `attr/main.go:13:2, not from a literal built there`,
			`source: a value of SysProcAttr or Credential (or holding one) comes from *ssa.Parameter in ` + fixPrefix + `attr.keep at ` + fixDir + `attr/main.go:9:37, not from a literal built there`,
		}},
		{"unsafe", nil, []string{
			`source: github.com/go-supervise/supervisor/internal/surface/testdata/fix/unsafe imports "unsafe"`,
			`source: github.com/go-supervise/supervisor/internal/surface/testdata/fix/unsafe imports unsafe`,
		}},
		{"reflect", nil, []string{
			`source: github.com/go-supervise/supervisor/internal/surface/testdata/fix/reflect imports "reflect"`,
			`source: github.com/go-supervise/supervisor/internal/surface/testdata/fix/reflect imports reflect`,
			`source: unresolved call (reflect.Value).Call is reachable (` + fixDir + `reflect/main.go:7:33)`,
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := source(t, "amd64", c.name)
			if !reflect.DeepEqual(r.Failures, c.fail) {
				t.Fatalf("failures:\n%s\nwant:\n%s", strings.Join(r.Failures, "\n"), strings.Join(c.fail, "\n"))
			}
			if c.reached != nil && !reflect.DeepEqual(r.Reached, c.reached) {
				t.Fatalf("reached %v, want %v", r.Reached, c.reached)
			}
		})
	}
}

// The hardening fixtures: each must make the source half red, naming what it
// refused. (An identity change through the all-threads path, a number that is
// not constant behind it, a function with no body, assembly in the module, and
// a SysProcAttr field set through a package variable or a loaded pointer.)
func TestSourceHalfRefusesWhatItCouldNotSee(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{"setid", []string{`reached system call "setuid" at ` + fixDir + `setid/main.go:`, `(never allowed)`}},
		{"allthreadsnr", []string{`non-constant system-call number at ` + fixDir + `allthreadsnr/main.go:`}},
		{"asm", []string{`has no Go body`, `asm.raw`, `holds the non-Go file raw_amd64.s`}},
		{"attrglobal", []string{`SysProcAttr field Credential set at ` + fixDir + `attrglobal/main.go:`}},
		{"attrloaded", []string{`SysProcAttr field Chroot set at ` + fixDir + `attrloaded/main.go:`}},
		// the type itself is refused outside the one constructor, so no way of
		// writing a field is left to find
		{"instgeneric", []string{`syscall.SysProcAttr is used in `, `instgeneric.configure`, `outside the one function that may build it`}},
		{"aliasattr", []string{`syscall.SysProcAttr is used in `, `aliasattr.main`}},
		{"definedattr", []string{`syscall.Credential, or a type of the layout of it or of SysProcAttr, is used in `, `definedattr.main`}},
		{"reflectset", []string{`reflectset imports reflect`}},
		// a twin that differs by a struct tag is converted by Go all the same,
		// inside the allowed constructor or outside it
		{"twinattr", []string{`syscall.Credential, or a type of the layout of it or of SysProcAttr, is used in `, `twinattr.configure`}},
		{"twinother", []string{`syscall.Credential, or a type of the layout of it or of SysProcAttr, is used in `, `twinother.configure`}},
		// inside the constructor the value must be built there, not received
		{"paramattr", []string{`comes from *ssa.Parameter in `, `paramattr.configure`, `not from a literal built there`}},
		// a type that holds a SysProcAttr is a reference to it
		{"jsonproc", []string{`syscall.SysProcAttr is used in `, `jsonproc.main`}},
		{"gobproc", []string{`syscall.SysProcAttr is used in `, `gobproc.main`}},
		// a function only the runtime calls is reached all the same
		{"finalizer", []string{`reached system call "kill" at `, `(never allowed)`}},
		// every directory-descriptor call, not openat alone
		{"fdcwdother", []string{`unlinkat with AT_FDCWD at ` + fixDir + `fdcwdother/main.go:`}},
		// a wrapper struct that holds the attribute and leaves as an interface
		{"wrapperattr", []string{`SysProcAttr is not a composite literal at `, `is converted to an interface at `}},
		// Cloneflags: a value from the caller, and a constant outside the mask
		{"cloneparam", []string{`Cloneflags set at ` + fixDir + `cloneparam/main.go:`, `that is not a constant within the mask`}},
		{"cloneuser", []string{`Cloneflags set at ` + fixDir + `cloneuser/main.go:`, `that is not a constant within the mask`}},
		// one branch of a conditional is enough
		{"clonephi", []string{`Cloneflags set at ` + fixDir + `clonephi/main.go:`, `that is not a constant within the mask`}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := source(t, "amd64", c.name)
			all := strings.Join(r.Failures, "\n")
			if all == "" {
				t.Fatalf("the source half is green on %s (reached %v)", c.name, r.Reached)
			}
			for _, w := range c.want {
				if !strings.Contains(all, w) {
					t.Errorf("failures lack %q:\n%s", w, all)
				}
			}
		})
	}
}

// A dependency whose module path has no dot in its first element (a local
// replacement) is not the standard library: its assembly and its function with
// no body are refused like any other module's.
func TestSourceHalfRefusesAssemblyInADotlessModule(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages (the gate1 job runs it)")
	}
	ver, err := exec.Command("go", "list", "-m", "-f", "{{.Version}}", "golang.org/x/sys").Output()
	if err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile("../../go.sum")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		"go.mod":               "module example.test/g\n\ngo 1.25.0\n\nrequire (\n\tevil v0.0.0\n\tgolang.org/x/sys " + strings.TrimSpace(string(ver)) + "\n)\n\nreplace evil => ./evil\n",
		"go.sum":               string(sum),
		"main.go":              "package main\n\nimport \"evil/raw\"\n\nfunc main() { _ = raw.Raw(1, 2, 3) }\n",
		"evil/go.mod":          "module evil\n\ngo 1.25.0\n",
		"evil/raw/raw.go":      "package raw\n\nfunc Raw(nr, a1, a2 uintptr) uintptr\n",
		"evil/raw/raw_amd64.s": "#include \"textflag.h\"\n\nTEXT ·Raw(SB), NOSPLIT, $0-32\n\tMOVQ nr+0(FP), AX\n\tMOVQ a1+8(FP), DI\n\tMOVQ a2+16(FP), SI\n\tSYSCALL\n\tMOVQ AX, ret+24(FP)\n\tRET\n",
	} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r, err := SourceHalf(Options{Dir: dir, Entry: ".", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(r.Failures, "\n")
	for _, w := range []string{`evil/raw holds the non-Go file raw_amd64.s`, `reached function evil/raw.Raw has no Go body`} {
		if !strings.Contains(all, w) {
			t.Errorf("failures lack %q:\n%s", w, all)
		}
	}
}

// Go's fork child behind an allowed SysProcAttr is exempt; the same fixture on
// both architectures, whose system-call tables differ.
func TestSourceHalfForkChildIsGatedByTheAttr(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			r := source(t, arch, "forkok")
			if len(r.Failures) != 0 {
				t.Fatalf("failures:\n%s", strings.Join(r.Failures, "\n"))
			}
			for _, n := range []string{"clone3", "execve", "setsid", "dup3"} {
				if !has(r.Reached, n) {
					t.Errorf("%s not reached: %v", n, r.Reached)
				}
			}
			for _, n := range ForkChildGated {
				if has(r.Reached, n) {
					t.Errorf("gated call %s reported as reached", n)
				}
			}
		})
	}
}

// Cloneflags built as spawn.Start builds it (constants inside the mask, one of
// them under a condition) is not a finding.
func TestSourceHalfAcceptsCloneflagsInsideTheMask(t *testing.T) {
	// a loop makes a cycle of phis; another type's Cloneflags field is not the
	// attribute's
	for _, name := range []string{"cloneok", "cloneloop", "clonelookalike"} {
		t.Run(name, func(t *testing.T) {
			r := source(t, "amd64", name)
			if len(r.Failures) != 0 {
				t.Fatalf("failures:\n%s", strings.Join(r.Failures, "\n"))
			}
		})
	}
}

// A sink that names no function of the program guards nothing: refused.
func TestSourceHalfRefusesASinkThatResolvesNowhere(t *testing.T) {
	const stale = "internal/runtime/syscall.Syscall6"
	sinks[stale] = true
	t.Cleanup(func() { delete(sinks, stale) })
	r := source(t, "amd64", "ok")
	want := "source: the sink " + stale + " names no function of the analysed program"
	if !has(r.Failures, want) {
		t.Fatalf("failures:\n%s\nwant %q", strings.Join(r.Failures, "\n"), want)
	}
}

// The module's root package is the module's own: a call reached from it is
// attributed to its source line, not to the library below.
func TestSourceHalfAttributesToTheModuleRootPackage(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages (the gate1 job runs it)")
	}
	ver, err := exec.Command("go", "list", "-m", "-f", "{{.Version}}", "golang.org/x/sys").Output()
	if err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile("../../go.sum")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		"go.mod":  "module example.test/root\n\ngo 1.25.0\n\nrequire golang.org/x/sys " + strings.TrimSpace(string(ver)) + "\n",
		"go.sum":  string(sum),
		"main.go": "package main\n\nimport \"golang.org/x/sys/unix\"\n\nfunc main() { _ = unix.Kill(1, 9) }\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r, err := SourceHalf(Options{Dir: dir, Entry: ".", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	want := `source: reached system call "kill" at main.go:5:28 (never allowed)`
	if !has(r.Failures, want) {
		t.Fatalf("failures:\n%s\nwant %q", strings.Join(r.Failures, "\n"), want)
	}
}

func TestInModule(t *testing.T) {
	for _, c := range []struct {
		path, module string
		want         bool
	}{
		{"example.test/m", "example.test/m", true},
		{"example.test/m/x", "example.test/m", true},
		{"example.test/mx", "example.test/m", false},
		{"example.test", "example.test/m", false},
		{"syscall", "", false},
	} {
		if got := inModule(c.path, c.module); got != c.want {
			t.Errorf("inModule(%q, %q) = %v", c.path, c.module, got)
		}
	}
}

func TestSourceHalfNeverOnArm64(t *testing.T) {
	r := source(t, "arm64", "never")
	if len(r.Failures) != 1 || !strings.Contains(r.Failures[0], `"kill"`) {
		t.Fatalf("failures %v", r.Failures)
	}
}

func has(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
