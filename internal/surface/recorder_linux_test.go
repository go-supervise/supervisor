//go:build linux

package surface

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// TestRecorderChild is the traced program of TestRecorderReadsCwdRelativeCalls:
// it does nothing unless the parent asks.
func TestRecorderChild(t *testing.T) {
	if os.Getenv("SVD_RECORD_CHILD") == "" {
		return
	}
	_, _ = unix.Openat(unix.AT_FDCWD, "rec-open-rel", unix.O_RDONLY, 0)
	_ = unix.Mkdirat(unix.AT_FDCWD, "rec-mkdir-rel", 0o700)
	if fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY, 0); err == nil {
		_, _ = unix.Openat(fd, "rec-open-fd", unix.O_RDONLY, 0)
	}
	// a path with no NUL inside the limit, and one at an address nobody maps
	long := []byte(strings.Repeat("a", 2*maxPath) + "\x00")
	_, _, _ = unix.Syscall6(unix.SYS_OPENAT, uintptr(unix.AT_FDCWD&0xffffffff), uintptr(unsafe.Pointer(&long[0])), unix.O_RDONLY, 0, 0, 0)
	_, _, _ = unix.Syscall6(unix.SYS_OPENAT, uintptr(unix.AT_FDCWD&0xffffffff), 8, unix.O_RDONLY, 0, 0, 0)
	os.Exit(0)
}

// The recorder marks every call relative to the working directory, with the
// path it names when it could be read whole, and marks none that is given a
// real directory descriptor.
func TestRecorderReadsCwdRelativeCalls(t *testing.T) {
	privileged(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecorderChild$")
	cmd.Env = append(os.Environ(), "SVD_RECORD_CHILD=1")
	cmd.Dir = t.TempDir() // the child makes a directory in its cwd
	rec, err := Record(cmd, runtime.GOARCH, 0, 60*time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := sysNames(runtime.GOARCH)
	var got []Call
	for _, c := range rec.Calls {
		switch names[c.Nr] {
		case "openat", "mkdirat":
			got = append(got, c)
		}
	}
	find := func(name, path string) (Call, bool) {
		for _, c := range got {
			if names[c.Nr] == name && c.Path == path {
				return c, true
			}
		}
		return Call{}, false
	}
	if c, ok := find("openat", "rec-open-rel"); !ok || !c.Cwd {
		t.Errorf("openat(AT_FDCWD, rec-open-rel): %+v, found %v", c, ok)
	}
	if c, ok := find("mkdirat", "rec-mkdir-rel"); !ok || !c.Cwd {
		t.Errorf("mkdirat(AT_FDCWD, rec-mkdir-rel): %+v, found %v", c, ok)
	}
	// a real descriptor: the one openat not relative to the working directory
	// (the Go runtime opens by AT_FDCWD only)
	fdRelative := 0
	for _, c := range got {
		if names[c.Nr] == "openat" && !c.Cwd {
			fdRelative++
			if c.Path != "" {
				t.Errorf("a path was read for a call given a descriptor: %+v", c)
			}
		}
	}
	if fdRelative != 1 {
		t.Errorf("%d openat given a descriptor, want 1: %+v", fdRelative, got)
	}
	// the two the recorder could not read whole are marked and carry no path
	unread := 0
	for _, c := range got {
		if names[c.Nr] == "openat" && c.Cwd && c.Path == "" {
			unread++
		}
	}
	if unread != 2 {
		t.Errorf("%d cwd-relative openat with an unread path, want 2 (too long, unmapped): %+v", unread, got)
	}
}
