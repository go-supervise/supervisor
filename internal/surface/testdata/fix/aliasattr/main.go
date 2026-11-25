// Fixture of the source half: SysProcAttr reached through a type alias.
package main

import "syscall"

type attr = syscall.SysProcAttr

func main() {
	pa := &syscall.ProcAttr{Sys: &attr{Chroot: "/x"}}
	_, _ = syscall.ForkExec("/x", nil, pa)
}
