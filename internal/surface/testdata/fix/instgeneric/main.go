// Fixture of the source half: a SysProcAttr field written inside an
// instantiated generic function.
package main

import "syscall"

func configure[T any](pa *syscall.ProcAttr, _ T) {
	pa.Sys.Chroot = "/x"
}

func main() {
	pa := &syscall.ProcAttr{Sys: &syscall.SysProcAttr{Setsid: true}}
	configure(pa, 0)
	_, _ = syscall.ForkExec("/x", nil, pa)
}
