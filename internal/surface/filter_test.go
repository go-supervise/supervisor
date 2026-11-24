package surface

import (
	"fmt"
	"testing"
)

// seccompData is the part of struct seccomp_data the filter reads.
type seccompData struct {
	nr   uint32
	arch uint32
	arg0 uint32
}

// run interprets a program of the three opcodes BuildFilter emits.
func run(t *testing.T, p []Insn, d seccompData) uint32 {
	t.Helper()
	var acc uint32
	for pc := 0; pc < len(p); pc++ {
		in := p[pc]
		switch in.Code {
		case bpfLdAbsW:
			switch in.K {
			case offNr:
				acc = d.nr
			case offArch:
				acc = d.arch
			case offArg0:
				acc = d.arg0
			default:
				t.Fatalf("pc %d loads offset %d", pc, in.K)
			}
		case bpfJeqK:
			if acc == in.K {
				pc += int(in.Jt)
			} else {
				pc += int(in.Jf)
			}
		case bpfRetK:
			return in.K
		default:
			t.Fatalf("pc %d: opcode %#x", pc, in.Code)
		}
	}
	t.Fatal("program ran off its end")
	return 0
}

func TestBuildFilterRefusesWhatItCannotBuild(t *testing.T) {
	if _, err := BuildFilter("riscv64", 1); err == nil {
		t.Fatal("an unknown architecture")
	}
	if _, ok := AuditArch("amd64"); !ok {
		t.Fatal("amd64")
	}
	if a, _ := AuditArch("arm64"); a != auditArchARM64 {
		t.Fatalf("arm64 %#x", a)
	}
}

func TestFilterDecisions(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		aud, _ := AuditArch(arch)
		const tgid = 4321
		p, err := BuildFilter(arch, tgid)
		if err != nil {
			t.Fatalf("%s: %v", arch, err)
		}
		tab := SysNum(arch)
		allowed := Allowed(arch)
		canon := sysNames(arch)
		allowedNr := map[int64]bool{} // a number is allowed when any of its names is
		for name, nr := range tab {
			if allowed[name] && name != "tgkill" { // tgkill's argument 7 is not our tgid
				allowedNr[nr] = true
			}
		}
		for name, nr := range tab {
			if canon[nr] != name { // a number shared by two names answers once
				continue
			}
			got := run(t, p, seccompData{nr: uint32(nr), arch: aud, arg0: 7})
			want := uint32(RetKillProcess)
			if allowedNr[nr] {
				want = RetAllow
			}
			if got != want {
				t.Errorf("%s: %s (%d) = %#x, want %#x", arch, name, nr, got, want)
			}
		}
		// A foreign architecture dies whatever the number.
		other := uint32(auditArchARM64)
		if arch == "arm64" {
			other = auditArchAMD64
		}
		if got := run(t, p, seccompData{nr: uint32(tab["read"]), arch: other}); got != RetKillProcess {
			t.Errorf("%s: foreign architecture answered %#x", arch, got)
		}
		// A number in no table dies.
		if got := run(t, p, seccompData{nr: 100000, arch: aud}); got != RetKillProcess {
			t.Errorf("%s: unknown number answered %#x", arch, got)
		}
	}
}

func TestFilterTgkillByArgument(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		if !Set(RGo(arch))["tgkill"] {
			t.Skipf("%s: the runtime's list does not hold tgkill", arch)
		}
		aud, _ := AuditArch(arch)
		p, _ := BuildFilter(arch, 99)
		nr := uint32(SysNum(arch)["tgkill"])
		if got := run(t, p, seccompData{nr: nr, arch: aud, arg0: 99}); got != RetAllow {
			t.Errorf("%s: own tgid %#x", arch, got)
		}
		for _, a := range []uint32{98, 100, 0, 1 << 31} {
			if got := run(t, p, seccompData{nr: nr, arch: aud, arg0: a}); got != RetKillProcess {
				t.Errorf("%s: tgid %d answered %#x", arch, a, got)
			}
		}
	}
}

// The runtime's own openat(AT_FDCWD) is the recorder's to judge by path, on
// both architectures: the filter answers only for the call number.
func TestFilterLeavesOpenatAtFdcwdToTheRecorder(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		aud, _ := AuditArch(arch)
		p, _ := BuildFilter(arch, 1)
		nr := uint32(SysNum(arch)["openat"])
		for _, a := range []uint32{atFdcwd32, 5} {
			if got := run(t, p, seccompData{nr: nr, arch: aud, arg0: a}); got != RetAllow {
				t.Errorf("%s: openat(%#x) answered %#x", arch, a, got)
			}
		}
	}
}

func TestFilterNeverAdmitsNeverExceptTgkill(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		aud, _ := AuditArch(arch)
		p, _ := BuildFilter(arch, 1)
		for _, name := range Never {
			nr, ok := SysNum(arch)[name]
			if !ok {
				continue
			}
			if got := run(t, p, seccompData{nr: uint32(nr), arch: aud, arg0: 2}); got != RetKillProcess {
				t.Errorf("%s: %s answered %#x", arch, name, got)
			}
		}
	}
}

func TestFilterJumpsStayInsideTheProgram(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		p, err := BuildFilter(arch, 1)
		if err != nil {
			t.Fatal(err)
		}
		for pc, in := range p {
			if in.Code == bpfJeqK && (pc+1+int(in.Jt) >= len(p) || pc+1+int(in.Jf) >= len(p)) {
				t.Errorf("%s: pc %d jumps past the end", arch, pc)
			}
		}
		if last := p[len(p)-1]; last.Code != bpfRetK {
			t.Errorf("%s: the program does not end in a return", arch)
		}
	}
}

func TestBuildFilterErrorsAreNamed(t *testing.T) {
	_, err := BuildFilter("riscv64", 1)
	if err == nil || err.Error() != `surface: no filter for "riscv64"` {
		t.Fatalf("%v", err)
	}
}

func TestJumpOffset(t *testing.T) {
	for _, c := range []struct {
		from, to int
		want     uint8
		err      string
	}{
		{5, 6, 0, ""},
		{5, 260, 254, ""},
		{5, 261, 255, ""},
		{5, 262, 0, "surface: filter jump of 256 does not fit"},
		{5, 5, 0, "surface: filter jump of -1 does not fit"},
		{5, 4, 0, "surface: filter jump of -2 does not fit"},
	} {
		got, err := jumpOffset(c.from, c.to)
		if got != c.want || (err == nil) != (c.err == "") || err != nil && err.Error() != c.err {
			t.Errorf("jumpOffset(%d, %d) = %d, %v; want %d, %q", c.from, c.to, got, err, c.want, c.err)
		}
	}
}

// A list too long for BPF's 8-bit jumps is refused, not truncated.
func TestBuildFilterRefusesAListTooLongToJumpOver(t *testing.T) {
	saved := AMSteady
	defer func() { AMSteady = saved }()
	never := Set(Never)
	AMSteady = nil
	for name := range SysNum("arm64") {
		if !never[name] {
			AMSteady = append(AMSteady, name)
		}
	}
	_, err := BuildFilter("arm64", 1)
	var d int
	if err == nil || len(AMSteady) < 256 {
		t.Fatalf("%d calls: %v", len(AMSteady), err)
	}
	if n, _ := fmt.Sscanf(err.Error(), "surface: filter jump of %d does not fit", &d); n != 1 || d <= 255 {
		t.Errorf("error %q", err)
	}
}
