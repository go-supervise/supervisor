// Fixture of the source half: a defined type with SysProcAttr's struct, set
// and then converted to it.
package main

import "syscall"

type shadow syscall.SysProcAttr

func main() {
	s := &shadow{Chroot: "/x"}
	pa := &syscall.ProcAttr{Sys: (*syscall.SysProcAttr)(s)}
	_, _ = syscall.ForkExec("/x", nil, pa)
}
