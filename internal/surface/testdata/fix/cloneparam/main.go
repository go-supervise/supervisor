// Fixture of the source half: the allowed constructor takes Cloneflags from
// its caller.
package main

import "syscall"

func configure(flags uintptr) *syscall.ProcAttr {
	return &syscall.ProcAttr{Sys: &syscall.SysProcAttr{Setsid: true, Cloneflags: flags}}
}

func main() { _ = configure(syscall.CLONE_NEWUSER | syscall.CLONE_PARENT) }
