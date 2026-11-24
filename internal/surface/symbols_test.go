package surface

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAbsenceJudgesASymbolSet(t *testing.T) {
	if _, err := absence(map[string]bool{"runtime.main": true}); err == nil || !strings.Contains(err.Error(), "no main.main") {
		t.Fatalf("a table without main.main: %v", err)
	}
	r, err := absence(map[string]bool{"main.main": true, "fmt.Println": true})
	if err != nil || len(r.Failures) != 0 {
		t.Fatalf("clean set: %v %v", r, err)
	}
	for _, name := range Absent {
		r, err := absence(map[string]bool{"main.main": true, name: true})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"symbols: " + name + " is linked into svd-monitor"}
		if !reflect.DeepEqual(r.Failures, want) {
			t.Fatalf("%s: %v", name, r.Failures)
		}
	}
}

func TestAbsenceReportsEveryLinkedSymbolInListOrder(t *testing.T) {
	r, err := absence(map[string]bool{"main.main": true, Absent[2]: true, Absent[0]: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"symbols: " + Absent[0] + " is linked into svd-monitor",
		"symbols: " + Absent[2] + " is linked into svd-monitor",
	}
	if !reflect.DeepEqual(r.Failures, want) {
		t.Fatalf("%v", r.Failures)
	}
}

func build(t *testing.T, pkg string, flags ...string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a binary (the gate1 job runs it)")
	}
	out := filepath.Join(t.TempDir(), "bin")
	args := append([]string{"build", "-o", out}, flags...)
	cmd := exec.Command("go", append(args, pkg)...)
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b)
	}
	return out
}

func TestSymbolAbsenceOnBinaries(t *testing.T) {
	r, err := SymbolAbsence(build(t, fixPkg+"ok"))
	if err != nil || len(r.Failures) != 0 {
		t.Fatalf("clean binary: %v %v", r, err)
	}
	r, err = SymbolAbsence(build(t, fixPkg+"never"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"symbols: golang.org/x/sys/unix.Kill is linked into svd-monitor"}
	if !reflect.DeepEqual(r.Failures, want) {
		t.Fatalf("failures %v", r.Failures)
	}
}

func TestSymbolAbsenceRefusesWhatItCannotRead(t *testing.T) {
	if _, err := SymbolAbsence(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing file is not a pass")
	}
	txt := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(txt, []byte("not an elf"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SymbolAbsence(txt); err == nil {
		t.Fatal("a non-ELF file is not a pass")
	}
	_, err := SymbolAbsence(build(t, fixPkg+"ok", "-ldflags=-s -w"))
	if err == nil || !strings.Contains(err.Error(), "no symbol table") {
		t.Fatalf("a stripped binary is not a pass: %v", err)
	}
}
