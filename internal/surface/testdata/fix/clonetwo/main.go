// Fixture of the source half: the address of Cloneflags kept and stored
// through twice; one address, two uses.
package main

import "syscall"

func start() {
	a := &syscall.SysProcAttr{Setsid: true}
	p := &a.Cloneflags
	*p = syscall.CLONE_NEWNS
	*p = syscall.CLONE_NEWIPC
	_, _ = syscall.ForkExec("/x", nil, &syscall.ProcAttr{Sys: a})
}

func main() { start() }
