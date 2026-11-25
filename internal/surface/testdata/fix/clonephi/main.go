// Fixture of the source half: one branch of a conditional adds a Cloneflags
// constant outside the mask.
package main

import "syscall"

func start(lane bool) {
	flags := uintptr(syscall.CLONE_NEWNS)
	if lane {
		flags |= syscall.CLONE_NEWUSER
	}
	_, _ = syscall.ForkExec("/x", nil, &syscall.ProcAttr{Sys: &syscall.SysProcAttr{Setsid: true, Cloneflags: flags}})
}

func main() { start(len("a") > 0) }
