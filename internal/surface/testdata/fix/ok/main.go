// Fixture of the source half: one allowed system call, nothing else.
package main

import "golang.org/x/sys/unix"

func main() {
	_, _ = unix.Write(1, nil)
}
