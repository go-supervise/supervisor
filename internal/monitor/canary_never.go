//go:build linux && canary_never

package monitor

import "golang.org/x/sys/unix"

// canary plants a call that is in the Never list: kill(getpid(), 0).
func canary() { _ = unix.Kill(unix.Getpid(), 0) }
