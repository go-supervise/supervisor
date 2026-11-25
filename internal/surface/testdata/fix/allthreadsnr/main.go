// Fixture of the source half: a system call whose number is not a constant,
// issued through the all-threads entry point.
package main

import "syscall"

var nr uintptr = 7

func main() {
	_, _, _ = syscall.AllThreadsSyscall6(nr, 0, 0, 0, 0, 0, 0)
}
