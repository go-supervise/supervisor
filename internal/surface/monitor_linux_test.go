//go:build linux

package surface

import (
	"debug/buildinfo"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/monitor"
)

// buildHost builds pkg for the host's architecture, with the given flags.
func buildHost(t *testing.T, out, pkg string, flags ...string) {
	t.Helper()
	cmd := exec.Command("go", append(append([]string{"build", "-o", out}, flags...), pkg)...)
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, b)
	}
}

// builtByRelease fails unless the binary was built by the toolchain R_go was
// generated for: what is judged must be what the release toolchain builds.
func builtByRelease(t *testing.T, bin string) {
	t.Helper()
	bi, err := buildinfo.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckToolchain(bi.GoVersion, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}
}

func privileged(t *testing.T) {
	t.Helper()
	if os.Getenv("SVD_PRIVILEGED") == "" {
		t.Skip("root/namespace test: set SVD_PRIVILEGED=1 (the privileged lane)")
	}
	if os.Geteuid() != 0 {
		t.Fatalf("SVD_PRIVILEGED is set but the process is uid %d, not root", os.Geteuid())
	}
}

func monitorSource(t *testing.T, tags ...string) *Result {
	t.Helper()
	r, err := SourceHalf(Options{Dir: "../..", Entry: "./cmd/svd-monitor", GOARCH: runtime.GOARCH, Tags: tags})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func monitorSymbols(t *testing.T, tags ...string) *Result {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "svd-monitor")
	flags := []string(nil)
	if len(tags) > 0 {
		flags = []string{"-tags=" + strings.Join(tags, ",")}
	}
	buildHost(t, bin, "./cmd/svd-monitor", flags...)
	builtByRelease(t, bin)
	r, err := SymbolAbsence(bin)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestMonitorSymbolAbsence is the symbol half on the real monitor (7.1.2 item 3).
func TestMonitorSymbolAbsence(t *testing.T) {
	if testing.Short() {
		t.Skip("builds svd-monitor (the gate1 job runs it)")
	}
	if r := monitorSymbols(t); len(r.Failures) != 0 {
		t.Fatalf("symbols:\n%s", strings.Join(r.Failures, "\n"))
	}
}

// surfaceTools builds the launcher, the monitor (with tags), the stub and the
// reference program into dir.
func surfaceTools(t *testing.T, dir string, tags ...string) RunOptions {
	t.Helper()
	o := RunOptions{Arch: runtime.GOARCH, Dir: dir,
		Launcher: filepath.Join(dir, "syscallsurface"), Monitor: filepath.Join(dir, "svd-monitor"), Stub: filepath.Join(dir, "stub")}
	buildHost(t, o.Launcher, "./cmd/syscallsurface")
	flags := []string(nil)
	if len(tags) > 0 {
		flags = []string{"-tags=" + strings.Join(tags, ",")}
	}
	buildHost(t, o.Monitor, "./cmd/svd-monitor", flags...)
	builtByRelease(t, o.Monitor)
	// Under the recorder every system call stops, so the stub's start-up runs
	// long enough for Go's sysmon to preempt it with a tgkill of its own tgid,
	// which the filter (built for M_p's tgid) answers with SIGSYS. The stub
	// is not what is judged: it does not take that signal.
	buildHost(t, o.Stub, "./internal/surface/stubhelper", "-ldflags=-X=runtime.godebugDefault=asyncpreemptoff=1")
	return o
}

// TestMonitorSyscallSurface is the gate: the source half, the symbol half and,
// in the privileged lane, the dynamic half over every scenario (7.1.2).
func TestMonitorSyscallSurface(t *testing.T) {
	if testing.Short() {
		t.Skip("loads and analyses the whole program (the gate1 job runs it)")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run("source "+arch, func(t *testing.T) {
			r, err := SourceHalf(Options{Dir: "../..", Entry: "./cmd/svd-monitor", GOARCH: arch})
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Failures) != 0 {
				t.Fatalf("source:\n%s", strings.Join(r.Failures, "\n"))
			}
		})
	}
	t.Run("symbols", func(t *testing.T) {
		if r := monitorSymbols(t); len(r.Failures) != 0 {
			t.Fatalf("symbols:\n%s", strings.Join(r.Failures, "\n"))
		}
	})
	t.Run("dynamic", func(t *testing.T) {
		privileged(t)
		o := surfaceTools(t, t.TempDir())
		var calls []Call
		for _, sc := range Scenarios() {
			res, err := RunScenario(o, sc)
			if err != nil {
				t.Fatalf("%s: %v", sc.Name, err)
			}
			if len(res.Failures) != 0 {
				t.Errorf("%s:\n%s", sc.Name, strings.Join(res.Failures, "\n"))
			}
			calls = append(calls, res.Rec.Calls...)
		}
		seen, err := Observed(runtime.GOARCH, calls)
		if err != nil {
			t.Fatal(err)
		}
		// The suite must reach the monitor's vocabulary, or a green run proves
		// nothing about it.
		for _, want := range [][]string{{"recvmsg"}, {"clone3", "clone"}, {"waitid", "wait4"}, {"mkdirat"}, {"unlinkat"},
			{"memfd_create"}, {"bind"}, {"socket"}, {"pipe2"}, {"execve"}, {"epoll_pwait", "epoll_wait"}} {
			ok := false
			for _, n := range want {
				ok = ok || has(seen, n)
			}
			if !ok {
				t.Errorf("no scenario issued any of %v (saw %v)", want, seen)
			}
		}
	})
}

func TestRGoIsCurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and records the reference program")
	}
	privileged(t)
	dir := t.TempDir()
	o := surfaceTools(t, dir)
	ref := filepath.Join(dir, "refprog")
	buildHost(t, ref, "./internal/surface/refprog")
	cmd := exec.Command(o.Launcher, "rgo", "--refprog", ref, "--stub", o.Stub, "--check", "--out", filepath.Join("internal/surface", "rgo_"+runtime.GOARCH+".txt"))
	cmd.Dir = "../.."
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rgo_%s.txt is not current: %v\n%s", runtime.GOARCH, err, b)
	}
}

func TestExitFaultMatchesTheMonitor(t *testing.T) {
	if exitFault != monitor.ExitFault {
		t.Fatalf("exitFault %d, monitor.ExitFault %d", exitFault, monitor.ExitFault)
	}
	if sigSYS != int(unix.SIGSYS) {
		t.Fatalf("sigSYS %d, SIGSYS %d", sigSYS, unix.SIGSYS)
	}
}

// TestMonitorSyscallSurfaceCatches is the negative control: the gate must go
// red on a monitor that plants a forbidden call, with the exact messages.
func TestMonitorSyscallSurfaceCatches(t *testing.T) {
	if testing.Short() {
		t.Skip("loads, builds and runs the planted monitors")
	}
	arch := runtime.GOARCH
	pos := func(tag string) string { return "internal/monitor/" + tag + ".go:" }
	sigsys := func(name string) string {
		return "dynamic: svd-monitor killed by SIGSYS at system call " + strconv.Quote(name) + " (" + strconv.FormatInt(SysNum(arch)[name], 10) + ")"
	}
	// firstFailure checks the one source failure, whose position is pinned by
	// its file and the text around it.
	source1 := func(t *testing.T, tag, prefix, suffix string) {
		t.Helper()
		r := monitorSource(t, tag)
		if len(r.Failures) != 1 || !strings.HasPrefix(r.Failures[0], prefix+pos(tag)) || !strings.HasSuffix(r.Failures[0], suffix) {
			t.Fatalf("source failures %q, want %s%s...%s", r.Failures, prefix, pos(tag), suffix)
		}
	}
	dynamic := func(t *testing.T, tag, want string) {
		t.Helper()
		privileged(t)
		o := surfaceTools(t, t.TempDir(), tag)
		res, err := RunScenario(o, Scenarios()[0])
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Failures) == 0 || res.Failures[0] != want {
			t.Fatalf("dynamic failures %q, want %q first", res.Failures, want)
		}
	}

	t.Run("canary_never", func(t *testing.T) {
		source1(t, "canary_never", `source: reached system call "kill" at `, " (never allowed)")
		if r := monitorSymbols(t, "canary_never"); !reflect.DeepEqual(r.Failures, []string{"symbols: golang.org/x/sys/unix.Kill is linked into svd-monitor"}) {
			t.Fatalf("symbol failures %q", r.Failures)
		}
		dynamic(t, "canary_never", sigsys("kill"))
	})

	t.Run("canary_new", func(t *testing.T) {
		source1(t, "canary_new", `source: reached system call "sysinfo" at `, " (in none of A_M and R_go)")
		if r := monitorSymbols(t, "canary_new"); len(r.Failures) != 0 {
			t.Fatalf("the symbol half must stay green on sysinfo: %q", r.Failures)
		}
		for _, l := range [][]string{AMStart, AMSteady, RGo(arch), Never} {
			if has(l, "sysinfo") {
				t.Fatalf("sysinfo is in a list: %v", l)
			}
		}
		dynamic(t, "canary_new", sigsys("sysinfo"))
	})

	t.Run("canary_rawnr", func(t *testing.T) {
		source1(t, "canary_rawnr", "source: non-constant system-call number at ", " (in github.com/go-supervise/supervisor/internal/monitor.canary)")
	})

	// A forbidden call in a child process, which inherits the filter and dies
	// alone: cloned as a fork (exit signal SIGCHLD) and cloned with another exit
	// signal and without CLONE_VFORK, which the tracer books in the leader's group.
	for _, tag := range []string{"canary_forkchild", "canary_forkclone"} {
		t.Run(tag, func(t *testing.T) {
			privileged(t)
			o := surfaceTools(t, t.TempDir(), tag)
			res, err := RunScenario(o, Scenarios()[0])
			if err != nil {
				t.Fatal(err)
			}
			const want = "dynamic: a descendant process (pid "
			found := false
			for _, f := range res.Failures {
				found = found || (strings.HasPrefix(f, want) && strings.Contains(f, `at system call "kill"`))
			}
			if !found {
				t.Fatalf("dynamic failures %q, want one starting %q naming kill", res.Failures, want)
			}
		})
	}
}
