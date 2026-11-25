// Fixture of the source half: a system call no list may admit.
package main

import "golang.org/x/sys/unix"

func main() {
	_ = unix.Kill(1, 0)
}
