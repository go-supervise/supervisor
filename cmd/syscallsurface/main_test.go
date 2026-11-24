package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/go-supervise/supervisor/internal/surface"
)

const fix = "./internal/surface/testdata/fix/"

// run drives the command tree as a program would, answering stdout, stderr and
// the error.
func run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	root := RootCmd()
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetErr(&errb)
	err := root.Execute()
	return out.String(), errb.String(), err
}

func slow(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("loads and analyses the whole program (the gate1 job runs it)")
	}
}

func TestMonitorReportsWhatIsReachedAndWhy(t *testing.T) {
	slow(t)
	out, errs, err := run(t, "monitor", "--dir", "../..", "--entry", fix+"ok", "--arch", "amd64", "--why", "write")
	if err != nil || errs != "" {
		t.Fatalf("ok fixture: %v %q", err, errs)
	}
	if !strings.Contains(out, "reached write\n") || !strings.Contains(out, "why write: ") || !strings.Contains(out, " -> ") {
		t.Errorf("stdout %q", out)
	}
}

func TestMonitorFailuresAreReportedAndFail(t *testing.T) {
	slow(t)
	out, errs, err := run(t, "monitor", "--dir", "../..", "--entry", fix+"never", "--arch", "amd64")
	if err == nil || err.Error() != "1 failure(s)" {
		t.Fatalf("error %v", err)
	}
	if !strings.Contains(out, "reached kill\n") || !strings.HasPrefix(errs, `source: reached system call "kill" at `) {
		t.Errorf("stdout %q stderr %q", out, errs)
	}
}

func TestMonitorRefusesWhatItCannotLoad(t *testing.T) {
	slow(t)
	if _, _, err := run(t, "monitor", "--dir", "../..", "--entry", fix+"absent", "--arch", "amd64"); err == nil {
		t.Error("a missing entry package is not a pass")
	}
	if _, _, err := run(t, "monitor", "extra"); err == nil {
		t.Error("an argument is not accepted")
	}
}

func TestSymbolsOnBinaries(t *testing.T) {
	slow(t)
	build := func(name string) string {
		out := filepath.Join(t.TempDir(), "bin")
		cmd := exec.Command("go", "build", "-o", out, fix+name)
		cmd.Dir = "../.."
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, b)
		}
		return out
	}
	if out, errs, err := run(t, "symbols", "--binary", build("ok")); err != nil || out != "" || errs != "" {
		t.Errorf("clean binary: %v %q %q", err, out, errs)
	}
	_, errs, err := run(t, "symbols", "--binary", build("never"))
	if err == nil || err.Error() != "1 failure(s)" || errs != "symbols: golang.org/x/sys/unix.Kill is linked into svd-monitor\nError: 1 failure(s)\n" {
		t.Errorf("never binary: %v %q", err, errs)
	}
	if _, _, err := run(t, "symbols", "--binary", filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("a missing binary is not a pass")
	}
	if _, _, err := run(t, "symbols"); err == nil {
		t.Error("--binary is required")
	}
}

func TestSysnumCheckAndWrite(t *testing.T) {
	committed := "../../internal/surface/sysnum_amd64.txt"
	if _, _, err := run(t, "sysnum", "--dir", "../..", "--arch", "amd64", "--out", committed, "--check"); err != nil {
		t.Fatalf("the committed table is current: %v", err)
	}
	stale := filepath.Join(t.TempDir(), "stale")
	if err := os.WriteFile(stale, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := run(t, "sysnum", "--dir", "../..", "--arch", "amd64", "--out", stale, "--check")
	if want := stale + " is out of date: run `syscallsurface sysnum --arch amd64 --out " + stale + "`"; err == nil || err.Error() != want {
		t.Errorf("stale: %v, want %q", err, want)
	}
	if _, _, err := run(t, "sysnum", "--dir", "../..", "--arch", "amd64", "--out", filepath.Join(t.TempDir(), "missing"), "--check"); err == nil {
		t.Error("--check of a missing file is not a pass")
	}
	// writing, then checking what was written
	written := filepath.Join(t.TempDir(), "w")
	if out, _, err := run(t, "sysnum", "--dir", "../..", "--arch", "amd64", "--out", written); err != nil || out != "" {
		t.Fatalf("write: %v %q", err, out)
	}
	want, _ := os.ReadFile(committed)
	if got, _ := os.ReadFile(written); !bytes.Equal(got, want) {
		t.Error("the written table differs from the committed one")
	}
	// no --out: stdout
	if out, _, err := run(t, "sysnum", "--dir", "../..", "--arch", "amd64"); err != nil || out != string(want) {
		t.Errorf("stdout: %v, %d bytes, want %d", err, len(out), len(want))
	}
	if _, _, err := run(t, "sysnum", "--dir", "../..", "--arch", "nonesuch"); err == nil {
		t.Error("an unknown architecture is not a pass")
	}
}

func TestMainExitsOneOnAnError(t *testing.T) {
	var code []int
	saved := osExit
	osExit = func(c int) { code = append(code, c) }
	defer func() { osExit = saved }()
	// main reads os.Args, so drive it with an argument the tree refuses
	savedArgs := os.Args
	os.Args = []string{"syscallsurface", "no-such-command"}
	defer func() { os.Args = savedArgs }()
	main()
	if len(code) != 1 || code[0] != 1 {
		t.Errorf("exit codes %v, want [1]", code)
	}
	os.Args = []string{"syscallsurface", "--help"}
	code = nil
	main()
	if len(code) != 0 {
		t.Errorf("exit codes %v, want none", code)
	}
}

func TestReportPrintsEveryReachedCallAndFailure(t *testing.T) {
	var out, errb bytes.Buffer
	c := &cobra.Command{}
	c.SetOut(&out)
	c.SetErr(&errb)
	if err := report(c, &surface.Result{Reached: []string{"a", "b"}, Failures: []string{"f1", "f2"}}); err == nil || err.Error() != "2 failure(s)" {
		t.Fatalf("error %v", err)
	}
	if out.String() != "reached a\nreached b\n" || errb.String() != "f1\nf2\n" {
		t.Errorf("stdout %q stderr %q", out.String(), errb.String())
	}
}
