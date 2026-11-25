// Fixture of the source half: Cloneflags accumulated in a loop, a cycle of
// phis that is not a choice between constants: refused.
package main

import "syscall"

func start(n int) {
	flags := uintptr(syscall.CLONE_NEWNS)
	for i := 0; i < n; i++ {
		flags |= syscall.CLONE_NEWIPC
	}
	_, _ = syscall.ForkExec("/x", nil, &syscall.ProcAttr{Sys: &syscall.SysProcAttr{Setsid: true, Cloneflags: flags}})
}

func main() { start(len("abc")) }
