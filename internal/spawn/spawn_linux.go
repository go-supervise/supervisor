//go:build linux

// Package spawn is M_p's spawn path, steps M2-M4 of SUPERVISOR.md 7.1.1;
// C1 is Go's fork child, reached through syscall.ForkExec.
package spawn

import (
	"errors"
	"sort"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/fdlayout"
)

// Config is one spawn. Every fd is owned by the caller and is O_CLOEXEC.
type Config struct {
	TreesFD     int               // dirfd of the cgroup trees/ directory
	Epoch       string            // cgroup directory name
	Limits      map[string]string // cgroup file -> value, written relative to the cgroup fd
	NotifyDirFD int               // dirfd of the 0711 notify directory
	NotifyName  string            // socket name inside it
	DevNullFD   int               // /dev/null, received from the core (T14)
	HelperFD    int               // O_PATH fd of `svd helper` (T17)
	StageFD     int               // O_PATH fd of the drop stage
	Plan        []byte            // the helper.Plan JSON
	Lane        bool              // lane mode: CLONE_NEWIPC | CLONE_NEWNET too
}

// Child is a started helper.
type Child struct {
	PID      int
	PidFD    int
	CgroupFD int
	LogR     int // read end of the log pipe
	StatusR  int // read end of the status pipe
	Notify   int // the bound notify socket
}

// Inject, when non-nil, is called at the start of each of M2, M3 and M4 with
// the step's name; a non-nil result fails that step (TestSpawnFailureLeavesNothing).
var Inject func(step string) error

func inject(step string) error {
	if Inject != nil {
		return Inject(step)
	}
	return nil
}

// ErrSEp is a spawn failure before any child exists (SUPERVISOR.md 3.1 O3).
type ErrSEp struct {
	Step string
	Err  error
}

func (e *ErrSEp) Error() string { return "spawn " + e.Step + ": " + e.Err.Error() }
func (e *ErrSEp) Unwrap() error { return e.Err }

type undo []func()

func (u *undo) push(f func()) { *u = append(*u, f) }
func (u undo) run() {
	for i := len(u) - 1; i >= 0; i-- {
		u[i]()
	}
}

// Start runs M2, M3 and M4. On failure everything it created is removed.
func Start(c *Config) (ch *Child, err error) {
	var rb undo
	defer func() {
		if err != nil {
			rb.run()
		}
	}()
	sep := func(step string, e error) (*Child, error) { return nil, &ErrSEp{Step: step, Err: e} }

	// M2: the epoch cgroup, a leaf; limits written relative to its fd.
	if e := inject("M2"); e != nil {
		return sep("M2", e)
	}
	mk := func() error { return sys.mkdirat(c.TreesFD, c.Epoch, 0o755) }
	e := mk()
	if errors.Is(e, unix.EEXIST) { // a leftover of svd's own (T5): remove once, retry once
		_ = sys.unlinkat(c.TreesFD, c.Epoch, unix.AT_REMOVEDIR)
		e = mk()
	}
	if e != nil {
		return sep("M2 mkdirat", e)
	}
	rb.push(func() { _ = sys.unlinkat(c.TreesFD, c.Epoch, unix.AT_REMOVEDIR) })
	cg, e := sys.openat(c.TreesFD, c.Epoch, unix.O_DIRECTORY|unix.O_PATH|unix.O_CLOEXEC, 0)
	if e != nil {
		return sep("M2 open", e)
	}
	rb.push(func() { sys.close(cg) })
	names := make([]string, 0, len(c.Limits))
	for k := range c.Limits {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if e := writeAt(cg, k, c.Limits[k]); e != nil {
			return sep("M2 limit "+k, e)
		}
	}

	// M3: pipes, notify socket, plan memfd. All O_CLOEXEC.
	if e := inject("M3"); e != nil {
		return sep("M3", e)
	}
	var logP, stP [2]int
	if e := sys.pipe2(logP[:], unix.O_CLOEXEC); e != nil {
		return sep("M3 log pipe", e)
	}
	rb.push(func() { sys.close(logP[0]); sys.close(logP[1]) })
	if e := sys.pipe2(stP[:], unix.O_CLOEXEC); e != nil {
		return sep("M3 status pipe", e)
	}
	rb.push(func() { sys.close(stP[0]); sys.close(stP[1]) })
	sock, e := sys.socket(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if e != nil {
		return sep("M3 socket", e)
	}
	rb.push(func() { sys.close(sock) })
	sa := &unix.SockaddrUnix{Name: "/proc/self/fd/" + strconv.Itoa(c.NotifyDirFD) + "/" + c.NotifyName}
	bind := func() error { return sys.bind(sock, sa) }
	if e = bind(); errors.Is(e, unix.EADDRINUSE) {
		_ = sys.unlinkat(c.NotifyDirFD, c.NotifyName, 0)
		e = bind()
	}
	if e != nil {
		return sep("M3 bind", e)
	}
	rb.push(func() { _ = sys.unlinkat(c.NotifyDirFD, c.NotifyName, 0) })
	plan, e := sealedMemfd("svd-plan", c.Plan)
	if e != nil {
		return sep("M3 plan", e)
	}
	defer sys.close(plan) // the child holds its own copy after the fork

	// M4: ForkExec into the new mount namespace and the epoch cgroup.
	if e := inject("M4"); e != nil {
		return sep("M4", e)
	}
	flags := uintptr(unix.CLONE_NEWNS)
	if c.Lane {
		flags |= unix.CLONE_NEWIPC | unix.CLONE_NEWNET
	}
	files := make([]uintptr, fdlayout.NFiles)
	files[fdlayout.DevNull] = uintptr(c.DevNullFD)
	files[fdlayout.LogW], files[fdlayout.LogW2] = uintptr(logP[1]), uintptr(logP[1])
	files[fdlayout.Plan] = uintptr(plan)
	files[fdlayout.StatusW] = uintptr(stP[1])
	files[fdlayout.ReadyFD] = uintptr(sock)
	files[fdlayout.StageFD] = uintptr(c.StageFD)
	files[fdlayout.HelperFD] = uintptr(c.HelperFD)
	var pidfd int
	pid, e := sys.forkExec("/proc/self/fd/"+strconv.Itoa(fdlayout.HelperFD), []string{"svd", "helper"}, &syscall.ProcAttr{
		Env:   []string{},
		Files: files,
		Sys: &syscall.SysProcAttr{
			Setsid:      true,
			Cloneflags:  flags,
			UseCgroupFD: true,
			CgroupFD:    cg,
			PidFD:       &pidfd,
		},
	})
	if e != nil {
		return sep("M4 ForkExec", e)
	}
	// The child's ends are its own now; the parent keeps the read ends.
	sys.close(logP[1])
	sys.close(stP[1])
	return &Child{PID: pid, PidFD: pidfd, CgroupFD: cg, LogR: logP[0], StatusR: stP[0], Notify: sock}, nil
}

func writeAt(dirfd int, name, val string) error {
	fd, err := sys.openat(dirfd, name, unix.O_WRONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer sys.close(fd)
	_, err = sys.write(fd, []byte(val))
	return err
}

// sealedMemfd is the plan memfd of M3 (T18): written in full, then sealed
// with the four seals of H9.
func sealedMemfd(name string, b []byte) (int, error) {
	fd, err := sys.memfdCreate(name, unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING|unix.MFD_NOEXEC_SEAL)
	if err != nil {
		return -1, err
	}
	for off := 0; off < len(b); {
		n, err := sys.write(fd, b[off:])
		if err != nil {
			sys.close(fd)
			return -1, err
		}
		off += n
	}
	if _, err := sys.fcntlInt(uintptr(fd), unix.F_ADD_SEALS, fdlayout.Seals); err != nil {
		sys.close(fd)
		return -1, &sealError{err}
	}
	return fd, nil
}

// sealError wraps a failed F_ADD_SEALS; it is a type of its own so that M_p
// does not link fmt (and with it os) for one message.
type sealError struct{ err error }

func (e *sealError) Error() string { return "seal: " + e.err.Error() }
func (e *sealError) Unwrap() error { return e.err }

// Close releases the parent's fds of a child.
func (c *Child) Close() {
	for _, fd := range []int{c.PidFD, c.CgroupFD, c.LogR, c.StatusR, c.Notify} {
		sys.close(fd)
	}
}
