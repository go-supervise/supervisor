// Fixture of the source half: a field named Cloneflags of another type, set
// from a parameter inside the allowed constructor, is not the attribute's.
package main

import "syscall"

type other struct{ Cloneflags uintptr }

var kept *other

func start(n uintptr) {
	o := &other{}
	o.Cloneflags = n
	kept = o
	_, _ = syscall.ForkExec("/x", nil, &syscall.ProcAttr{Sys: &syscall.SysProcAttr{Setsid: true}})
}

func main() { start(uintptr(len("a"))) }
