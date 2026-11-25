// Package surface bounds M_p's system-call surface (SUPERVISOR.md 7.1.2,
// SECURITY-1 T14). It holds the committed lists, the source half
// (reachability over the whole program's call graph), the symbol-absence half,
// and, on Linux, the enforcing seccomp harness with its lossless ptrace
// recorder. All lists name system calls, never numbers: a number is resolved
// per architecture.
package surface

import (
	_ "embed"
	"fmt"
	"go/version"
	"sort"
	"strings"
)

// AMStart is A_M^start: what M_p may issue before its first recvmsg on L_p's
// socket and not after (7.1.2 item 0).
var AMStart = []string{"prctl", "epoll_create1", "prlimit64"}

// AMSteady is A_M^steady, the committed application list, with Go's fork-child
// path (setsid, dup3, execve) and the fork itself (clone3, with clone as Go's
// fallback when the kernel answers ENOSYS).
var AMSteady = []string{
	"recvmsg", "sendmsg", "read", "write", "openat", "mkdirat", "unlinkat",
	"fchownat", "newfstatat", "statx", "close", "fcntl", "pipe2", "socket",
	"bind", "getsockopt", "epoll_ctl", "epoll_pwait", "epoll_wait", "clone3", "clone",
	"waitid", "wait4", "memfd_create", "setsid", "dup3", "execve", "exit",
}

// Never is the list of calls that no list may admit (7.1.2 item 1). tgkill is
// in it and is admitted only by argument, as the runtime's self-directed use.
var Never = []string{
	"kill", "tkill", "tgkill", "rt_sigqueueinfo", "rt_tgsigqueueinfo",
	"pidfd_open", "pidfd_send_signal", "pidfd_getfd", "ptrace",
	"process_vm_readv", "process_vm_writev",
	"setuid", "setgid", "setreuid", "setregid", "setresuid", "setresgid",
	"setfsuid", "setfsgid", "setgroups", "capset",
	"mount", "fsopen", "fsconfig", "fsmount", "fspick", "open_tree",
	"move_mount", "mount_setattr", "pivot_root", "unshare", "setns",
	"init_module", "finit_module", "delete_module", "bpf", "keyctl",
	"perf_event_open", "kexec_load", "kexec_file_load", "reboot", "chroot",
}

// ForkChildGated is the set of calls that Go's fork child contains behind
// SysProcAttr fields (Credential, Ptrace, Chroot, Unshareflags, Pdeathsig,
// UidMappings, ...). The source half exempts those call sites, and only
// inside syscall's forkAndExecInChild and ID-mapping writers, because it
// instead refuses every SysProcAttr that sets a field outside AllowedAttr
// (7.1.2 item 1).
var ForkChildGated = []string{
	"kill", "ptrace", "setgroups", "setresuid", "setresgid", "setuid", "setgid",
	"setpgid", "chroot", "unshare", "mount", "capget", "capset", "chdir",
	"ioctl", "getpid", "getppid", "prctl",
}

// AllowedAttr are the only SysProcAttr fields M_p may set.
var AllowedAttr = []string{"Setsid", "Cloneflags", "UseCgroupFD", "CgroupFD", "PidFD"}

// AllowedCloneflags is the mask the one SysProcAttr M_p builds may hold in
// Cloneflags: CLONE_NEWNS, CLONE_NEWIPC and CLONE_NEWNET. The low byte (the
// exit signal) and every other bit (CLONE_NEWUSER, CLONE_PARENT, CLONE_VM, ...)
// change what the dynamic judge assumes, so a value outside it is refused.
const AllowedCloneflags uint64 = 0x00020000 | 0x08000000 | 0x40000000

// Absent is the committed list of symbols svd-monitor must not link (7.1.2
// item 3).
var Absent = []string{
	"syscall.Kill", "syscall.Setresuid", "syscall.Setgroups",
	"syscall.AllThreadsSyscall", "syscall.PtraceAttach", "syscall.Mount",
	"syscall.AllThreadsSyscall6", "syscall.runtime_doAllThreadsSyscall",
	"syscall.Setuid", "syscall.Setgid", "syscall.Setreuid", "syscall.Setregid",
	"syscall.Setresgid", "golang.org/x/sys/unix.Setuid", "golang.org/x/sys/unix.Setgid",
	"golang.org/x/sys/unix.Setreuid", "golang.org/x/sys/unix.Setregid",
	"golang.org/x/sys/unix.Setresgid",
	"golang.org/x/sys/unix.Kill", "golang.org/x/sys/unix.Tgkill",
	"golang.org/x/sys/unix.PidfdOpen", "golang.org/x/sys/unix.PidfdSendSignal",
	"golang.org/x/sys/unix.PidfdGetfd", "golang.org/x/sys/unix.Mount",
	"golang.org/x/sys/unix.MoveMount", "golang.org/x/sys/unix.Fsopen",
	"golang.org/x/sys/unix.Fsmount", "golang.org/x/sys/unix.OpenTree",
	"golang.org/x/sys/unix.MountSetattr", "golang.org/x/sys/unix.PivotRoot",
	"golang.org/x/sys/unix.Unshare", "golang.org/x/sys/unix.Setns",
	"golang.org/x/sys/unix.Setresuid", "golang.org/x/sys/unix.Setgroups",
	"golang.org/x/sys/unix.Capset", "golang.org/x/sys/unix.KeyctlInt",
	"golang.org/x/sys/unix.KeyctlString", "golang.org/x/sys/unix.PtraceAttach",
	"golang.org/x/sys/unix.InitModule", "golang.org/x/sys/unix.Reboot",
	"golang.org/x/sys/unix.Chroot", "os.(*Process).Signal",
}

//go:embed rgo_amd64.txt
var rgoAMD64 string

//go:embed rgo_arm64.txt
var rgoARM64 string

// RGo is R_go for an architecture: the Go runtime's own calls, generated from
// a reference program that imports nothing of svd (cmd/syscallsurface rgo).
func RGo(arch string) []string {
	switch arch {
	case "amd64":
		return parseList(rgoAMD64)
	case "arm64":
		return parseList(rgoARM64)
	}
	return nil
}

// toolchainPrefix starts the header line of an rgo_*.txt that names the
// toolchain (language version, "go1.27") its list was generated by.
const toolchainPrefix = "# toolchain: "

// RGoToolchain is the toolchain an architecture's R_go was generated by, or ""
// when the committed table carries no stamp.
func RGoToolchain(arch string) string {
	var s string
	switch arch {
	case "amd64":
		s = rgoAMD64
	case "arm64":
		s = rgoARM64
	}
	for _, l := range strings.Split(s, "\n") {
		if v, ok := strings.CutPrefix(l, toolchainPrefix); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// CheckToolchain refuses a judged build made by any toolchain but the one the
// R_go table of arch was generated for: the runtime's own calls differ from one
// toolchain to the next, so a verdict on another one would judge nothing. built
// is a full version such as "go1.27.4".
func CheckToolchain(built, arch string) error {
	want := RGoToolchain(arch)
	if want == "" {
		return fmt.Errorf("surface: rgo_%s.txt carries no toolchain stamp", arch)
	}
	if got := version.Lang(built); got != want {
		return fmt.Errorf("surface: built by %q (%s), but R_go for %s is generated for %s: build with that toolchain", built, got, arch, want)
	}
	return nil
}

func parseList(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out
}

// Allowed is A_M^start u A_M^steady u R_go for arch, as a set.
func Allowed(arch string) map[string]bool {
	m := map[string]bool{}
	for _, l := range [][]string{AMStart, AMSteady, RGo(arch)} {
		for _, n := range l {
			m[n] = true
		}
	}
	return m
}

// Set returns the members of a list as a set.
func Set(l []string) map[string]bool {
	m := make(map[string]bool, len(l))
	for _, n := range l {
		m[n] = true
	}
	return m
}
