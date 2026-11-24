//go:build linux

package helper

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// kernelOps is the table of the kernel calls the helper makes. Production
// uses realKernel; a test replaces the whole table, so that every error branch
// of H1-H10 can be driven without privilege and without touching the host.
type kernelOps struct {
	closeRange        func(first, last, flags uintptr) error
	fstat             func(fd int, st *unix.Stat_t) error
	getsid            func(pid int) (int, error)
	getpid            func() int
	open              func(path string, mode int, perm uint32) (int, error)
	close             func(fd int) error
	fcntlInt          func(fd uintptr, cmd, arg int) (int, error)
	pread             func(fd int, b []byte, off int64) (int, error)
	readFile          func(path string) ([]byte, error)
	writeFile         func(path string, b []byte, perm os.FileMode) error
	mount             func(source, target, fstype string, flags uintptr, data string) error
	fsopen            func(name string, flags int) (int, error)
	fsconfigSetString func(fd int, key, value string) error
	fsconfigCreate    func(fd int) error
	fsmount           func(fd, flags, attrs int) (int, error)
	openat2           func(dirfd int, path string, how *unix.OpenHow) (int, error)
	openat            func(dirfd int, path string, flags int, perm uint32) (int, error)
	read              func(fd int, b []byte) (int, error)
	write             func(fd int, b []byte) (int, error)
	fchown            func(fd, uid, gid int) error
	mountSetattr      func(dirfd int, path string, flags uint, attr *unix.MountAttr) error
	moveMount         func(fromFD int, fromPath string, toFD int, toPath string, flags int) error
	chdir             func(path string) error
	pivotRoot         func(newroot, putold string) error
	unmount           func(target string, flags int) error
	memfdCreate       func(name string, flags int) (int, error)
	dup3              func(oldfd, newfd, flags int) error
	exec              func(path string, argv, envp []string) error
	execveat          func(fd int, argv, envp []*byte) error
}

func closeRange(first, last, flags uintptr) error {
	if _, _, e := unix.Syscall(unix.SYS_CLOSE_RANGE, first, last, flags); e != 0 {
		return e
	}
	return nil
}

// execveat never returns on success, so any return is the error.
func execveat(fd int, argv, envp []*byte) error {
	empty, _ := unix.BytePtrFromString("")
	_, _, e := unix.Syscall6(unix.SYS_EXECVEAT, uintptr(fd), uintptr(unsafe.Pointer(empty)),
		uintptr(unsafe.Pointer(&argv[0])), uintptr(unsafe.Pointer(&envp[0])), unix.AT_EMPTY_PATH, 0)
	return e
}

var realKernel = kernelOps{
	closeRange:        closeRange,
	fstat:             unix.Fstat,
	getsid:            unix.Getsid,
	getpid:            os.Getpid,
	open:              unix.Open,
	close:             unix.Close,
	fcntlInt:          unix.FcntlInt,
	pread:             unix.Pread,
	readFile:          os.ReadFile,
	writeFile:         os.WriteFile,
	mount:             unix.Mount,
	fsopen:            unix.Fsopen,
	fsconfigSetString: unix.FsconfigSetString,
	fsconfigCreate:    unix.FsconfigCreate,
	fsmount:           unix.Fsmount,
	openat2:           unix.Openat2,
	openat:            func(d int, p string, f int, m uint32) (int, error) { return unix.Openat(d, p, f, m) },
	read:              unix.Read,
	write:             unix.Write,
	fchown:            unix.Fchown,
	mountSetattr:      unix.MountSetattr,
	moveMount:         unix.MoveMount,
	chdir:             unix.Chdir,
	pivotRoot:         unix.PivotRoot,
	unmount:           unix.Unmount,
	memfdCreate:       unix.MemfdCreate,
	dup3:              unix.Dup3,
	exec:              syscall.Exec,
	execveat:          execveat,
}

// k is the table in use.
var k = realKernel
