// Fixture of the source half: mkdirat relative to the working directory,
// through a raw system call rather than a wrapper.
package main

import "golang.org/x/sys/unix"

func main() {
	_, _, _ = unix.Syscall(unix.SYS_MKDIRAT, ^uintptr(99), 0, 0o755)
}
