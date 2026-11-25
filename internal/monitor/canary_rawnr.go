//go:build linux && canary_rawnr

package monitor

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// canaryNR is a variable on purpose: the system-call number is not a constant.
var canaryNR uintptr = unix.SYS_GETPID

// canary plants a raw system call whose number the source half cannot read.
func canary() { _, _, _ = syscall.Syscall(canaryNR, 0, 0, 0) }
