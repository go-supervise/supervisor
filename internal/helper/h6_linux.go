//go:build linux

package helper

import (
	"errors"
	"fmt"
	"path"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/fdlayout"
)

// openHow is the resolution of every K6 path (T7): no symlink and no magic
// link anywhere in it. The fd is O_PATH, so nothing is read or executed
// through it.
var openHow = unix.OpenHow{
	Flags:   unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC,
	Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
}

// h6 opens each K6 region once, by the helper, and mounts the read-only ones
// by fd (S8): the path is resolved a single time, and every later step names
// the fd, so a swap of the path between check and use changes nothing. The
// returned fds stay open for D4 (the Landlock ruleset); on a failure every fd
// opened so far is closed.
func h6(p *Plan) (fds []int, err error) {
	defer func() {
		if err != nil {
			for _, fd := range fds {
				k.close(fd)
			}
			fds = nil
		}
	}()
	for i, r := range p.Regions {
		fd, e := regionFD(r)
		if e != nil {
			return fds, fail(fmt.Sprintf("H6 region %d", i), fdlayout.ExitH6, e)
		}
		fds = append(fds, fd)
	}
	return fds, nil
}

func open2(dirfd int, p string) (int, error) {
	how := openHow
	return k.openat2(dirfd, p, &how)
}

// regionFD returns the fd of one region, already mounted read-only and
// verified when the plan says so.
func regionFD(r Region) (int, error) {
	fd, err := open2(unix.AT_FDCWD, r.Path)
	if err != nil {
		return -1, fmt.Errorf("open: %w", err)
	}
	if !r.ReadOnly {
		return fd, nil
	}
	tree, err := k.openTree(fd, "", unix.OPEN_TREE_CLONE|unix.OPEN_TREE_CLOEXEC|unix.AT_RECURSIVE|unix.AT_EMPTY_PATH)
	if err != nil {
		k.close(fd)
		return -1, fmt.Errorf("open_tree: %w", err)
	}
	defer k.close(tree)
	attr := unix.MountAttr{Attr_set: unix.MOUNT_ATTR_RDONLY | unix.MOUNT_ATTR_NOSUID | unix.MOUNT_ATTR_NODEV}
	if err := k.mountSetattr(tree, "", unix.AT_EMPTY_PATH|unix.AT_RECURSIVE, &attr); err != nil {
		k.close(fd)
		return -1, fmt.Errorf("mount_setattr: %w", err)
	}
	var want unix.Statx_t
	if err := k.statx(tree, "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &want); err != nil {
		k.close(fd)
		return -1, fmt.Errorf("statx tree: %w", err)
	}
	if want.Mask&unix.STATX_MNT_ID == 0 {
		k.close(fd)
		return -1, errors.New("statx: no mount id (kernel older than 5.8)")
	}
	if err := k.moveMount(tree, "", fd, "", unix.MOVE_MOUNT_F_EMPTY_PATH|unix.MOVE_MOUNT_T_EMPTY_PATH); err != nil {
		k.close(fd)
		return -1, fmt.Errorf("move_mount: %w", err)
	}
	k.close(fd)
	return reopen(r.Path, want.Mnt_id)
}

// reopen opens the region again through its parent fd, as a later reader
// would, and verifies it is the mount just placed there and is read-only.
func reopen(p string, mnt uint64) (int, error) {
	parent, err := open2(unix.AT_FDCWD, path.Dir(p))
	if err != nil {
		return -1, fmt.Errorf("reopen parent: %w", err)
	}
	defer k.close(parent)
	base := path.Base(p)
	if base == "/" { // the root is its own parent
		base = "."
	}
	fd, err := open2(parent, base)
	if err != nil {
		return -1, fmt.Errorf("reopen: %w", err)
	}
	var st unix.Statx_t
	if err := k.statx(fd, "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &st); err != nil {
		k.close(fd)
		return -1, fmt.Errorf("statx reopened: %w", err)
	}
	if st.Mnt_id != mnt {
		k.close(fd)
		return -1, fmt.Errorf("reopened region is mount %d, not the mount %d just placed", st.Mnt_id, mnt)
	}
	var sf unix.Statfs_t
	if err := k.fstatfs(fd, &sf); err != nil {
		k.close(fd)
		return -1, fmt.Errorf("fstatfs: %w", err)
	}
	if sf.Flags&unix.ST_RDONLY == 0 {
		k.close(fd)
		return -1, errors.New("reopened region is not read-only")
	}
	return fd, nil
}
