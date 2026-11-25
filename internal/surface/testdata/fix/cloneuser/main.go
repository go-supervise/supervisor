// Fixture of the source half: the allowed constructor sets a Cloneflags
// constant outside the mask.
package main

import "syscall"

func configure() *syscall.ProcAttr {
	return &syscall.ProcAttr{Sys: &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWUSER}}
}

func main() { _ = configure() }
