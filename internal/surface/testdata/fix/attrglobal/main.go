// Fixture of the source half: a SysProcAttr kept in a package variable, with
// a field outside the allowed set written to it afterwards.
package main

import "syscall"

var attr syscall.SysProcAttr

func main() {
	attr.Setsid = true
	attr.Credential = &syscall.Credential{Uid: 1}
}
