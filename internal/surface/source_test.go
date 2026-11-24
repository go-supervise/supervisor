package surface

import (
	"reflect"
	"strings"
	"testing"
)

const fixPkg = "./internal/surface/testdata/fix/"
const fixDir = "internal/surface/testdata/fix/"

func source(t *testing.T, arch, name string, tags ...string) *Result {
	t.Helper()
	if testing.Short() {
		t.Skip("source half: loads and analyses the whole program (the gate1 job runs it)")
	}
	r, err := SourceHalf(Options{Dir: "../..", Entry: fixPkg + name, GOARCH: arch, Tags: tags})
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
		}},
		{"unsafe", nil, []string{
			`source: github.com/go-supervise/supervisor/internal/surface/testdata/fix/unsafe imports "unsafe"`,
			`source: github.com/go-supervise/supervisor/internal/surface/testdata/fix/unsafe imports unsafe`,
		}},
		{"reflect", nil, []string{
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
