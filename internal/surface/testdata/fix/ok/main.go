// Fixture of the source half: one allowed system call, nothing else.
package main

import "golang.org/x/sys/unix"

// holder shares a field name with a refused SysProcAttr field; reading it must
// not be taken for one.
type holder struct{ Chroot string }

func main() {
	h := &holder{}
	h.Chroot = "x"
	_, _ = unix.Write(1, []byte(h.Chroot))
}
