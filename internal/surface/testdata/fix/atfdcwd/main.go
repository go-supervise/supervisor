// Fixture of the source half: openat relative to the working directory.
package main

import "golang.org/x/sys/unix"

func main() {
	_, _ = unix.Open("x", 0, 0)
}
