package surface

import (
	"fmt"
	"sort"
)

// Insn is one classic-BPF instruction (struct sock_filter).
type Insn struct {
	Code   uint16
	Jt, Jf uint8
	K      uint32
}

// BPF opcodes and seccomp constants used by the filter.
const (
	bpfLdAbsW = 0x20 // BPF_LD | BPF_W | BPF_ABS
	bpfJeqK   = 0x15 // BPF_JMP | BPF_JEQ | BPF_K
	bpfRetK   = 0x06 // BPF_RET | BPF_K

	// RetKillProcess and RetAllow are SECCOMP_RET_KILL_PROCESS and
	// SECCOMP_RET_ALLOW.
	RetKillProcess = 0x80000000
	RetAllow       = 0x7fff0000

	offNr   = 0  // seccomp_data.nr
	offArch = 4  // seccomp_data.arch
	offArg0 = 16 // seccomp_data.args[0], low 32 bits (little endian)

	auditArchAMD64 = 0xC000003E
	auditArchARM64 = 0xC00000B7

	atFdcwd32 = 0xffffff9c // AT_FDCWD (-100) as the low 32 bits of an argument
)

// AuditArch is the AUDIT_ARCH_* value of a GOARCH the filter supports.
func AuditArch(arch string) (uint32, bool) {
	switch arch {
	case "amd64":
		return auditArchAMD64, true
	case "arm64":
		return auditArchARM64, true
	}
	return 0, false
}

// BuildFilter is the filter of the dynamic half (7.1.2 item 2) for arch: every
// call of Allowed(arch) is allowed, tgkill only when its first argument is
// tgid, openat with AT_FDCWD as its first argument is refused on amd64 (on
// arm64 Go's own runtime issues one, which the recorder attributes by path
// instead), and everything else, a foreign architecture included, answers
// SECCOMP_RET_KILL_PROCESS.
func BuildFilter(arch string, tgid uint32) ([]Insn, error) {
	aud, ok := AuditArch(arch)
	if !ok {
		return nil, fmt.Errorf("surface: no filter for %q", arch)
	}
	tab := SysNum(arch)
	never := Set(Never)
	var plain []int64
	var tgkill, openat int64 = -1, -1
	for name := range Allowed(arch) {
		nr, ok := tab[name]
		if !ok {
			if archSpecific[arch][name] {
				continue
			}
			return nil, fmt.Errorf("surface: %q has no number on %s", name, arch)
		}
		switch {
		case name == "tgkill":
			tgkill = nr
		case never[name]:
			return nil, fmt.Errorf("surface: %q is never allowed", name)
		case name == "openat" && arch == "amd64":
			openat = nr
		default:
			plain = append(plain, nr)
		}
	}
	sort.Slice(plain, func(i, j int) bool { return plain[i] < plain[j] })

	// Layout: arch check (3), load nr, one jeq per plain call, the optional
	// tgkill and openat jeqs, then KILL, ALLOW and the two argument blocks
	// (tgkill: 4 instructions, openat: 4).
	var p []Insn
	p = append(p,
		Insn{Code: bpfLdAbsW, K: offArch},
		Insn{Code: bpfJeqK, Jt: 1, K: aud},
		Insn{Code: bpfRetK, K: RetKillProcess},
		Insn{Code: bpfLdAbsW, K: offNr},
	)
	nJeq := len(plain)
	if tgkill >= 0 {
		nJeq++
	}
	if openat >= 0 {
		nJeq++
	}
	// After the jeqs: [kill] [allow] [tgkill: ld, jeq, kill, allow ...].
	killAt := len(p) + nJeq
	allowAt := killAt + 1
	blockAt := allowAt + 1
	jump := func(from, to int) (uint8, error) {
		d := to - from - 1
		if d < 0 || d > 255 {
			return 0, fmt.Errorf("surface: filter jump of %d does not fit", d)
		}
		return uint8(d), nil
	}
	for _, nr := range plain {
		jt, err := jump(len(p), allowAt)
		if err != nil {
			return nil, err
		}
		p = append(p, Insn{Code: bpfJeqK, Jt: jt, K: uint32(nr)})
	}
	tgAt := blockAt
	opAt := blockAt
	if tgkill >= 0 {
		opAt += 4
	}
	if tgkill >= 0 {
		jt, err := jump(len(p), tgAt)
		if err != nil {
			return nil, err
		}
		p = append(p, Insn{Code: bpfJeqK, Jt: jt, K: uint32(tgkill)})
	}
	if openat >= 0 {
		jt, err := jump(len(p), opAt)
		if err != nil {
			return nil, err
		}
		p = append(p, Insn{Code: bpfJeqK, Jt: jt, K: uint32(openat)})
	}
	if len(p) != killAt {
		return nil, fmt.Errorf("surface: filter layout broken")
	}
	p = append(p, Insn{Code: bpfRetK, K: RetKillProcess}, Insn{Code: bpfRetK, K: RetAllow})
	if tgkill >= 0 { // ld arg0; jeq tgid -> allow, else kill (BPF jumps only forward)
		p = append(p,
			Insn{Code: bpfLdAbsW, K: offArg0},
			Insn{Code: bpfJeqK, Jf: 1, K: tgid},
			Insn{Code: bpfRetK, K: RetAllow},
			Insn{Code: bpfRetK, K: RetKillProcess},
		)
	}
	if openat >= 0 { // ld arg0; jeq AT_FDCWD -> kill (skips the allow)
		p = append(p,
			Insn{Code: bpfLdAbsW, K: offArg0},
			Insn{Code: bpfJeqK, Jt: 1, K: atFdcwd32},
			Insn{Code: bpfRetK, K: RetAllow},
			Insn{Code: bpfRetK, K: RetKillProcess},
		)
	}
	return p, nil
}
