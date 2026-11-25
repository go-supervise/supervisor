// Fixture of the source half: a field outside the allowed set written through
// a SysProcAttr pointer loaded back from another struct.
package main

import "syscall"

type holder struct{ sys *syscall.SysProcAttr }

var keep *holder

func main() {
	h := &holder{sys: &syscall.SysProcAttr{Setsid: true}}
	keep = h
	h.sys.Chroot = "/x"
}
