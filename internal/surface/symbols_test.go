package surface

import (
	"bytes"
	"debug/dwarf"
	"debug/elf"
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

// A function the compiler inlined everywhere has no symbol, only the debug
// information that names it: the absence is judged on both.
func TestSymbolAbsenceSeesAnInlinedFunction(t *testing.T) {
	bin := build(t, fixPkg+"inlined")
	f, err := elf.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	syms, err := f.Symbols()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range syms {
		if s.Name == "golang.org/x/sys/unix.Setuid" {
			t.Fatal("the compiler kept an out-of-line copy: the fixture inlines nothing")
		}
	}
	r, err := SymbolAbsence(bin)
	if err != nil {
		t.Fatal(err)
	}
	// syscall.Setuid is the callee the inlined body leaves linked, and it
	// pulls the all-threads machinery in with it.
	want := []string{
		"symbols: syscall.AllThreadsSyscall is linked into svd-monitor",
		"symbols: syscall.runtime_doAllThreadsSyscall is linked into svd-monitor",
		"symbols: syscall.Setuid is linked into svd-monitor",
		"symbols: golang.org/x/sys/unix.Setuid is linked into svd-monitor",
	}
	if !reflect.DeepEqual(r.Failures, want) {
		t.Fatalf("failures %v, want %v", r.Failures, want)
	}
}

// Debug information that does not parse is an error, never a shorter list of
// names: a half-read table would pass a binary whose offending subprogram sits
// in the part that was not read.
func TestSymbolAbsenceRefusesDebugInformationItCannotParse(t *testing.T) {
	bin := build(t, fixPkg+"ok", "-ldflags=-compressdwarf=false")
	b, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	f, err := elf.NewFile(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	s := f.Section(".debug_info")
	if s == nil || s.Type == elf.SHT_NOBITS {
		t.Fatal("no .debug_info section")
	}
	// the first unit's header stays, its first entry becomes an abbreviation
	// code that never ends: the reader opens and then fails on the first read
	for i := s.Offset + 12; i < s.Offset+44; i++ {
		b[i] = 0xff
	}
	bad := filepath.Join(t.TempDir(), "bad")
	if err := os.WriteFile(bad, b, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := SymbolAbsence(bad); err == nil {
		t.Fatal("unparsable debug information is not a pass")
	} else if !strings.Contains(err.Error(), "reading debug information") {
		t.Errorf("error does not name the debug information: %v", err)
	}
}

func TestDwarfFunctionsReportsAParseError(t *testing.T) {
	bin := build(t, fixPkg+"ok", "-ldflags=-compressdwarf=false")
	f, err := elf.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	d, err := f.DWARF()
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	if err := dwarfFunctions(d, have); err != nil || !have["main.main"] {
		t.Fatalf("a whole table: %v, main.main %v", err, have["main.main"])
	}
	// units whose DIEs name an abbreviation the table does not hold
	info := []byte{8, 0, 0, 0, 4, 0, 0, 0, 0, 0, 8, 0x7f}
	bad, err := dwarf.New([]byte{0}, nil, nil, info, nil, nil, nil, nil)
	if err != nil {
		t.Skipf("the reader refuses it before reading: %v", err)
	}
	if err := dwarfFunctions(bad, map[string]bool{}); err == nil {
		t.Error("a table that does not parse was read as complete")
	}
}

func TestSymbolAbsenceRefusesWhatItCannotRead(t *testing.T) {
	noPanic(t, "a missing file", func() {
		if _, err := SymbolAbsence(filepath.Join(t.TempDir(), "missing")); !os.IsNotExist(err) {
			t.Errorf("a missing file is not a pass: %v", err)
		}
	})
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
	// symbols kept, debug information dropped: an inlined function would hide
	_, err = SymbolAbsence(build(t, fixPkg+"ok", "-ldflags=-w"))
	if err == nil || !strings.Contains(err.Error(), "no debug information") {
		t.Fatalf("a binary without DWARF is not a pass: %v", err)
	}
}
