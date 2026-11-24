//go:build linux

package spawn

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// sysOps is the table of the kernel calls the spawn path makes. Production
// uses realSys; a test replaces the whole table to drive each error branch of
// M2-M4 and check the rollback.
type sysOps struct {
	mkdirat     func(dirfd int, path string, mode uint32) error
	unlinkat    func(dirfd int, path string, flags int) error
	openat      func(dirfd int, path string, flags int, mode uint32) (int, error)
	close       func(fd int) error
	pipe2       func(p []int, flags int) error
	socket      func(domain, typ, proto int) (int, error)
	bind        func(fd int, sa unix.Sockaddr) error
	write       func(fd int, b []byte) (int, error)
	memfdCreate func(name string, flags int) (int, error)
	fcntlInt    func(fd uintptr, cmd, arg int) (int, error)
	forkExec    func(argv0 string, argv []string, attr *syscall.ProcAttr) (int, error)
}

var realSys = sysOps{
	mkdirat:     unix.Mkdirat,
	unlinkat:    unix.Unlinkat,
	openat:      func(d int, p string, f int, m uint32) (int, error) { return unix.Openat(d, p, f, m) },
	close:       unix.Close,
	pipe2:       unix.Pipe2,
	socket:      unix.Socket,
	bind:        unix.Bind,
	write:       unix.Write,
	memfdCreate: unix.MemfdCreate,
	fcntlInt:    unix.FcntlInt,
	forkExec:    syscall.ForkExec,
}

// sys is the table in use.
var sys = realSys
