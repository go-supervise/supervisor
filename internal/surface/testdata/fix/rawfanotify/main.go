// Fixture of the source half: fanotify_mark relative to the working directory,
// whose directory descriptor is its fourth argument, through a raw system call.
package main

import "golang.org/x/sys/unix"

func main() {
	_, _, _ = unix.Syscall6(unix.SYS_FANOTIFY_MARK, 3, 1, 1, ^uintptr(99), 0, 0)
}
