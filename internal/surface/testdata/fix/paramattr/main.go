// Fixture of the source half: the allowed constructor takes its SysProcAttr
// from its caller instead of building it.
package main

import "syscall"

func configure(a *syscall.SysProcAttr) *syscall.ProcAttr {
	return &syscall.ProcAttr{Sys: a}
}

func main() { _ = configure(nil) }
