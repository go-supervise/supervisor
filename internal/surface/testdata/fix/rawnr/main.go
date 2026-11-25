// Fixture of the source half: a raw system call whose number is a variable.
package main

import "syscall"

var nr uintptr = 39

func main() {
	_, _, _ = syscall.Syscall(nr, 0, 0, 0)
}
