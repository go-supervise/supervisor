// Fixture of the source half: Cloneflags built as spawn.Start builds it, by
// choosing between constants inside the mask.
package main

import "syscall"

func start(lane bool) {
	flags := uintptr(syscall.CLONE_NEWNS)
	if lane {
		flags = syscall.CLONE_NEWNS | syscall.CLONE_NEWIPC | syscall.CLONE_NEWNET
	}
	_, _ = syscall.ForkExec("/x", nil, &syscall.ProcAttr{Sys: &syscall.SysProcAttr{Setsid: true, Cloneflags: flags}})
}

func main() { start(len("a") > 0) }
