//go:build linux && canary_forkclone

package monitor

import "golang.org/x/sys/unix"

// canary plants a cloned child with another exit signal than SIGCHLD and
// without CLONE_VFORK (a process of its own, reported by the tracer as a
// clone) that makes a call in the Never list.
func canary() { plantChild(uintptr(unix.SIGURG)) }
