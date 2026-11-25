// Fixture of the source half: a directory-descriptor call other than openat,
// given the working directory.
package main

import "golang.org/x/sys/unix"

func main() {
	_ = unix.Unlinkat(unix.AT_FDCWD, "x", 0)
}
