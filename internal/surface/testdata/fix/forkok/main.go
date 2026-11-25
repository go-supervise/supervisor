// Fixture of the source half: Go's fork child behind an allowed SysProcAttr
// is not a finding, while syscall.Open called directly still is.
package main

import "syscall"

func main() {
	_, _ = syscall.ForkExec("/x", nil, &syscall.ProcAttr{Sys: &syscall.SysProcAttr{Setsid: true}})
}
