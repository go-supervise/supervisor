//go:build linux

package helper

import (
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/fdlayout"
)

func regionPlan(rs ...Region) *Plan {
	p := testPlan(ModeService)
	p.Regions = rs
	return p
}

var roRegion = Region{Path: "/srv/data", ReadOnly: true}

func TestH6WritableRegionIsOpenedOnceAndNotMounted(t *testing.T) {
	f := newFake(t)
	fds, err := h6(regionPlan(Region{Path: "/srv/rw"}))
	if err != nil || len(fds) != 1 || len(f.open) != 1 || !f.open[fds[0]] {
		t.Fatalf("fds %v err %v open %v", fds, err, f.open)
	}
	want := []any{"/srv/rw", uint64(unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC), uint64(unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS)}
	if got := f.args["openat2"]; !reflect.DeepEqual(got, want) {
		t.Errorf("openat2 %v, want %v", got, want)
	}
	for _, op := range []string{"open_tree", "mount_setattr", "move_mount", "statx", "fstatfs"} {
		if f.called(op) != 0 {
			t.Errorf("a writable region called %s", op)
		}
	}
}

func TestH6ReadOnlyRegionIsMountedByFd(t *testing.T) {
	f := newFake(t)
	fds, err := h6(regionPlan(roRegion))
	if err != nil || len(fds) != 1 {
		t.Fatal(fds, err)
	}
	// Only the region's own fd is left: the tree, the first fd and the parent are closed.
	if len(f.open) != 1 || !f.open[fds[0]] {
		t.Errorf("open %v, region fd %d", f.open, fds[0])
	}
	// The path is resolved once for the region, then its parent, then the name inside the parent.
	wantOpen := []any{
		"/srv/data", uint64(unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC), uint64(unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS),
		"/srv", uint64(unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC), uint64(unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS),
		"data", uint64(unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC), uint64(unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS),
	}
	if got := f.args["openat2"]; !reflect.DeepEqual(got, wantOpen) {
		t.Errorf("openat2 %v\nwant %v", got, wantOpen)
	}
	if got, want := f.args["open_tree"], []any{"", uint(unix.OPEN_TREE_CLONE | unix.OPEN_TREE_CLOEXEC | unix.AT_RECURSIVE | unix.AT_EMPTY_PATH)}; !reflect.DeepEqual(got, want) {
		t.Errorf("open_tree %v, want %v", got, want)
	}
	if got, want := f.args["mount_setattr"], []any{"", uint(unix.AT_EMPTY_PATH | unix.AT_RECURSIVE), uint64(unix.MOUNT_ATTR_RDONLY | unix.MOUNT_ATTR_NOSUID | unix.MOUNT_ATTR_NODEV)}; !reflect.DeepEqual(got, want) {
		t.Errorf("mount_setattr %v, want %v", got, want)
	}
	if got, want := f.args["move_mount"], []any{unix.MOVE_MOUNT_F_EMPTY_PATH | unix.MOVE_MOUNT_T_EMPTY_PATH}; !reflect.DeepEqual(got, want) {
		t.Errorf("move_mount %v, want %v", got, want)
	}
	// Both statx calls ask for the mount id, on the fd itself.
	if got, want := f.args["statx"], []any{f.treeFD, "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, fds[0], "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID}; !reflect.DeepEqual(got, want) {
		t.Errorf("statx %v, want %v", got, want)
	}
	// order: open, clone, restrict, read the id, move, reopen, verify
	want := []string{"openat2", "open_tree", "mount_setattr", "statx", "move_mount:", "move_mount", "openat2", "openat2", "statx", "fstatfs"}
	var got []string
	for _, c := range f.calls {
		if c != "close" {
			got = append(got, c)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls %v, want %v", got, want)
	}
}

func TestH6RootRegionReopensThroughItsOwnParent(t *testing.T) {
	f := newFake(t)
	if _, err := h6(regionPlan(Region{Path: "/", ReadOnly: true})); err != nil {
		t.Fatal(err)
	}
	a := f.args["openat2"]
	if a[3] != "/" || a[6] != "." {
		t.Errorf("reopen of / went through %v then %v", a[3], a[6])
	}
}

func TestH6Refusals(t *testing.T) {
	for _, c := range []struct {
		op   string
		nth  int
		part string
	}{
		{"openat2", 1, "open:"},
		{"open_tree", 1, "open_tree:"},
		{"mount_setattr", 1, "mount_setattr:"},
		{"statx", 1, "statx tree:"},
		{"move_mount", 1, "move_mount:"},
		{"openat2", 2, "reopen parent:"},
		{"openat2", 3, "reopen:"},
		{"statx", 2, "statx reopened:"},
		{"fstatfs", 1, "fstatfs:"},
	} {
		t.Run(c.part, func(t *testing.T) {
			f := newFake(t)
			f.failAt[c.op] = c.nth
			fds, err := h6(regionPlan(roRegion))
			wantStep(t, err, "H6 region 0", fdlayout.ExitH6)
			if fds != nil || !strings.Contains(err.Error(), c.part) {
				t.Errorf("fds %v, err %v, want %q", fds, err, c.part)
			}
			f.noLeak(t)
		})
	}
	t.Run("no mount id", func(t *testing.T) {
		f := newFake(t)
		f.noMask = true
		_, err := h6(regionPlan(roRegion))
		wantStep(t, err, "H6 region 0", fdlayout.ExitH6)
		f.noLeak(t)
	})
	t.Run("another mount", func(t *testing.T) {
		f := newFake(t)
		f.mntDelta = 1
		_, err := h6(regionPlan(roRegion))
		wantStep(t, err, "H6 region 0", fdlayout.ExitH6)
		if !strings.Contains(err.Error(), "not the mount") {
			t.Error(err)
		}
		f.noLeak(t)
	})
	t.Run("not read-only", func(t *testing.T) {
		f := newFake(t)
		f.writable = true
		_, err := h6(regionPlan(roRegion))
		wantStep(t, err, "H6 region 0", fdlayout.ExitH6)
		if !strings.Contains(err.Error(), "not read-only") {
			t.Error(err)
		}
		f.noLeak(t)
	})
	t.Run("a later region fails: the earlier fds are closed", func(t *testing.T) {
		f := newFake(t)
		f.failAt["openat2"] = 2
		fds, err := h6(regionPlan(Region{Path: "/a"}, Region{Path: "/b"}))
		wantStep(t, err, "H6 region 1", fdlayout.ExitH6)
		if fds != nil {
			t.Error(fds)
		}
		f.noLeak(t)
	})
}

func TestH10PlacesRegionFdsForD4(t *testing.T) {
	f := newFake(t)
	f.open[60], f.open[61] = true, true
	p := testPlan(ModeService)
	_ = h10(p, 50, 51, []int{60, 61})
	d := f.args["dup3"]
	// 9 sources are parked (status, ready, frame, cwd, stage, two regions) at >= 64: dup3 args are (old, new, flags)
	if len(d) != 6*3 {
		t.Fatalf("dup3 %v", d)
	}
	news := []any{d[1], d[4], d[7], d[10], d[13], d[16]}
	want := []any{fdlayout.StageStatus, fdlayout.StageReady, fdlayout.StageFrame, fdlayout.StageCwd, fdlayout.StageFirstFree, fdlayout.StageFirstFree + 1}
	if !reflect.DeepEqual(news, want) {
		t.Errorf("targets %v, want %v", news, want)
	}
	if d[12] == d[15] || d[2] != 0 || d[17] != 0 {
		t.Errorf("sources %v", d)
	}
	cr := f.args["close_range"]
	if cr[0] != uintptr(fdlayout.StageFirstFree+2) {
		t.Errorf("close_range from %v", cr[0])
	}
}
