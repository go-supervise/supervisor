//go:build linux && (canary_forkchild || canary_forkclone)

package monitor

import (
	"time"

	"golang.org/x/sys/unix"
)

// plantChild clones the process with the given flags (no shared memory, so the
// child is a copy), and the child makes a call that is in the Never list and
// exits. The child inherits the seccomp filter: the call kills it, and its
// parent sees nothing.
func plantChild(flags uintptr) {
	pid, _, errno := unix.RawSyscall6(unix.SYS_CLONE, flags, 0, 0, 0, 0, 0)
	if errno != 0 {
		return
	}
	if pid == 0 {
		unix.RawSyscall(unix.SYS_KILL, 1, 0, 0)
		unix.RawSyscall(unix.SYS_EXIT_GROUP, 0, 0, 0)
	}
	time.Sleep(100 * time.Millisecond)
}
