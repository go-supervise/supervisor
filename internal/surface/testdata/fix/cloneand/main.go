// Fixture of the source half: Cloneflags computed by arithmetic from a value of
// the caller; an expression that mentions an allowed bit is still not a constant.
package main

import "syscall"

func configure(f uintptr) *syscall.ProcAttr {
	return &syscall.ProcAttr{Sys: &syscall.SysProcAttr{Setsid: true, Cloneflags: f & (f | syscall.CLONE_NEWNS)}}
}

func main() { _, _ = syscall.ForkExec("/x", nil, configure(syscall.CLONE_NEWUSER)) }
