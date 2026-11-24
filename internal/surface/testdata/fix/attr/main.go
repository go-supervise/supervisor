// Fixture of the source half: SysProcAttr values, one allowed, one not, and
// one that escapes into a function.
package main

import "syscall"

var sink *syscall.SysProcAttr

func keep(a *syscall.SysProcAttr) { sink = a }

func main() {
	sink = &syscall.SysProcAttr{Setsid: true}
	sink = &syscall.SysProcAttr{Setpgid: true}
	a := &syscall.SysProcAttr{}
	keep(a)
}
