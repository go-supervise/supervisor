// Fixture of the source half: the address of Cloneflags handed to a function
// that stores a value of the caller through it.
package main

import "syscall"

func set(p *uintptr, v uintptr) { *p = v }

func configure(f uintptr) *syscall.ProcAttr {
	a := &syscall.SysProcAttr{Setsid: true}
	set(&a.Cloneflags, f)
	return &syscall.ProcAttr{Sys: a}
}

func main() { _, _ = syscall.ForkExec("/x", nil, configure(syscall.CLONE_NEWUSER)) }
