//go:build linux

package surface

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

// Install installs the filter on every thread of the calling process
// (SECCOMP_FILTER_FLAG_TSYNC). It needs CAP_SYS_ADMIN and does not set
// no_new_privs: the launcher is root, and svd-monitor then runs as in
// production otherwise (7.1.2 item 2).
func Install(p []Insn) error {
	f := make([]unix.SockFilter, len(p))
	for i, in := range p {
		f[i] = unix.SockFilter{Code: in.Code, Jt: in.Jt, Jf: in.Jf, K: in.K}
	}
	prog := unix.SockFprog{Len: uint16(len(f)), Filter: &f[0]}
	_, _, e := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, unix.SECCOMP_FILTER_FLAG_TSYNC, uintptr(unsafe.Pointer(&prog)))
	if e != 0 {
		return e
	}
	return nil
}

// Launch is the test launcher: it installs the filter of its own architecture
// with its own tgid, then execs the monitor, which keeps both.
func Launch(arch, monitorPath string) error {
	p, err := BuildFilter(arch, uint32(unix.Getpid()))
	if err != nil {
		return err
	}
	if err := Install(p); err != nil {
		return err
	}
	return unix.Exec(monitorPath, []string{"svd-monitor"}, []string{})
}
