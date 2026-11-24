//go:build linux

package helper

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/unix"
)

// inPrivateMountNS runs f on a locked thread that has its own mount
// namespace, with propagation made slave first (H5), so nothing it mounts
// reaches the host. The thread is never unlocked: the runtime discards it
// when the goroutine ends, so the namespace dies with it.
func inPrivateMountNS(t *testing.T, f func()) {
	t.Helper()
	if os.Getenv("SVD_PRIVILEGED") == "" {
		t.Skip("needs root with CAP_SYS_ADMIN: set SVD_PRIVILEGED=1 (the privileged CI lane)")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.LockOSThread()
		if err := unix.Unshare(unix.CLONE_NEWNS); err != nil {
			t.Errorf("unshare: %v", err)
			return
		}
		if err := h5(); err != nil {
			t.Errorf("H5: %v", err)
			return
		}
		f()
	}()
	<-done
}

// TestMountByFd is S8: a read-only region is mounted read-only by fd, a later
// open of its path sees that mount, a symlink in the path is refused, and a
// writable region is left writable.
func TestMountByFd(t *testing.T) {
	inPrivateMountNS(t, func() {
		root := t.TempDir()
		for _, d := range []string{"ro", "rw", "real"} {
			if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
				t.Error(err)
				return
			}
		}
		for _, d := range []string{"ro", "rw"} {
			if err := unix.Mount("tmpfs", filepath.Join(root, d), "tmpfs", 0, "size=1m"); err != nil {
				t.Errorf("mount tmpfs: %v", err)
				return
			}
		}
		if err := os.WriteFile(filepath.Join(root, "ro", "f"), []byte("x"), 0o644); err != nil {
			t.Error(err)
			return
		}
		if err := os.Symlink(filepath.Join(root, "ro"), filepath.Join(root, "link")); err != nil {
			t.Error(err)
			return
		}
		ro, rw := filepath.Join(root, "ro"), filepath.Join(root, "rw")

		fds, err := h6(regionPlan(Region{Path: ro, ReadOnly: true}, Region{Path: rw}))
		if err != nil {
			t.Errorf("h6: %v", err)
			return
		}
		defer func() {
			for _, fd := range fds {
				unix.Close(fd)
			}
		}()
		// the read-only region refuses a write by path and through its fd
		if err := os.WriteFile(filepath.Join(ro, "g"), []byte("y"), 0o644); err == nil {
			t.Error("a write to the read-only region by path succeeded")
		}
		if _, err := unix.Openat(fds[0], "f", unix.O_WRONLY, 0); err != unix.EROFS {
			t.Errorf("open for write through the region fd: %v, want EROFS", err)
		}
		if b, err := os.ReadFile(filepath.Join(ro, "f")); err != nil || string(b) != "x" {
			t.Errorf("the read-only region lost its content: %q %v", b, err)
		}
		// the mount flags are the three the plan asked for
		var sf unix.Statfs_t
		if err := unix.Fstatfs(fds[0], &sf); err != nil || sf.Flags&(unix.ST_RDONLY|unix.ST_NOSUID|unix.ST_NODEV) != unix.ST_RDONLY|unix.ST_NOSUID|unix.ST_NODEV {
			t.Errorf("flags %#x, err %v", sf.Flags, err)
		}
		// the writable region is untouched
		if err := os.WriteFile(filepath.Join(rw, "g"), []byte("y"), 0o644); err != nil {
			t.Errorf("a writable region refused a write: %v", err)
		}
		// no new mount appeared beside it, and the region fds name different mounts
		var a, b unix.Statx_t
		if unix.Statx(fds[0], "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &a) != nil || unix.Statx(fds[1], "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &b) != nil || a.Mnt_id == b.Mnt_id {
			t.Errorf("mount ids %d and %d", a.Mnt_id, b.Mnt_id)
		}

		// a symlink in the path is refused, before anything is mounted
		before := mountCount(t)
		if _, err := h6(regionPlan(Region{Path: filepath.Join(root, "link"), ReadOnly: true})); err == nil {
			t.Error("a symlinked region was accepted")
		}
		if got := mountCount(t); got != before {
			t.Errorf("a refused region changed the mount table: %d -> %d", before, got)
		}
		// so is a regular file
		if _, err := h6(regionPlan(Region{Path: filepath.Join(ro, "f")})); err == nil {
			t.Error("a file was accepted as a region")
		}
	})
}

func mountCount(t *testing.T) int {
	t.Helper()
	b, err := os.ReadFile("/proc/thread-self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}
