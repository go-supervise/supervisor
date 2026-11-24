// Fixture of the source half: a system call in no list.
package main

import "golang.org/x/sys/unix"

func main() {
	var si unix.Sysinfo_t
	_ = unix.Sysinfo(&si)
}
