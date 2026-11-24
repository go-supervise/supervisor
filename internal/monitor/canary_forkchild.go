//go:build linux && canary_forkchild

package monitor

import "golang.org/x/sys/unix"

// canary plants a forked child (exit signal SIGCHLD, a process of its own)
// that makes a call in the Never list.
func canary() { plantChild(uintptr(unix.SIGCHLD)) }
