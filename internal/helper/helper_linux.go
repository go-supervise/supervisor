//go:build linux

package helper

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/fdlayout"
)

// The four seals of H9 (and of the plan memfd, T18).
const Seals = unix.F_SEAL_WRITE | unix.F_SEAL_GROW | unix.F_SEAL_SHRINK | unix.F_SEAL_SEAL

// SealsOK reports whether an F_GET_SEALS result is exactly the four seals.
// MFD_NOEXEC_SEAL makes the kernel add F_SEAL_EXEC (0x20) itself (measured:
// 0x2f), so that bit is not part of the comparison.
func SealsOK(got int) bool { return got&^unix.F_SEAL_EXEC == Seals }

// SuidDumpablePath is read by H4; a variable so a test can point it elsewhere.
var SuidDumpablePath = "/proc/sys/fs/suid_dumpable"

type stepError struct {
	step string
	code int
	err  error
}

func (e *stepError) Error() string { return e.step + ": " + e.err.Error() }

func fail(step string, code int, err error) *stepError {
	return &stepError{step: step, code: code, err: err}
}

// Run executes H1-H10 and, on success, never returns (it execs the stage).
// On failure it writes `perm` on the status pipe, logs the reason on stderr
// and returns the step's fixed exit code.
func Run(stderr io.Writer) int {
	err := runSteps()
	var se *stepError
	if !errors.As(err, &se) {
		se = fail("helper", fdlayout.ExitH10, err)
	}
	_, _ = k.write(fdlayout.StatusW, []byte{fdlayout.StatusPerm})
	fmt.Fprintf(stderr, "svd helper: %v\n", se)
	return se.code
}

// runSteps is run, replaceable by a test that needs a non-step error.
var runSteps = run

func run() error {
	if err := h1(); err != nil {
		return err
	}
	plan, err := h2()
	if err != nil {
		return err
	}
	if err := h3(); err != nil {
		return err
	}
	if plan.Mode != ModeSignal {
		if err := H4(); err != nil {
			return err
		}
		if err := h5(); err != nil {
			return err
		}
		if err := h7(plan); err != nil {
			return err
		}
	}
	cwd, frameFD, err := h9(plan)
	if err != nil {
		return err
	}
	if plan.Mode == ModeLane {
		// The lane's root is emptied last, so every path the plan names
		// (cwd, credentials) was opened while it still existed.
		if err := h8(); err != nil {
			return err
		}
	}
	return h10(plan, cwd, frameFD)
}

// h1: close_range(k, ~0, CLOEXEC); fd 0 is /dev/null, fds 1 and 2 are the
// log pipe, getsid(0) == getpid(), no controlling terminal.
func h1() error {
	if e := k.closeRange(fdlayout.HelperFD, ^uintptr(0)>>32, unix.CLOSE_RANGE_CLOEXEC); e != nil {
		return fail("H1 close_range", fdlayout.ExitH1, e)
	}
	var st, s1, s2 unix.Stat_t
	if err := k.fstat(0, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFCHR || st.Rdev != unix.Mkdev(1, 3) {
		return fail("H1 fd 0", fdlayout.ExitH1, errors.New("fd 0 is not /dev/null"))
	}
	if k.fstat(1, &s1) != nil || k.fstat(2, &s2) != nil || s1.Dev != s2.Dev || s1.Ino != s2.Ino {
		return fail("H1 fds 1,2", fdlayout.ExitH1, errors.New("fds 1 and 2 are not the same log pipe"))
	}
	if sid, err := k.getsid(0); err != nil || sid != k.getpid() {
		return fail("H1 session", fdlayout.ExitH1, errors.New("not a session leader"))
	}
	if fd, err := k.open("/dev/tty", unix.O_RDWR|unix.O_CLOEXEC, 0); err != unix.ENXIO {
		if err == nil {
			k.close(fd)
		}
		return fail("H1 ctty", fdlayout.ExitH1, errors.New("a controlling terminal exists"))
	}
	return nil
}

// h2: the plan from fd 3, a memfd carrying exactly the four seals.
func h2() (*Plan, error) {
	seals, err := k.fcntlInt(fdlayout.Plan, unix.F_GET_SEALS, 0)
	if err != nil || !SealsOK(seals) {
		return nil, fail("H2 seals", fdlayout.ExitH2, fmt.Errorf("plan seals %#x: %v", seals, err))
	}
	var st unix.Stat_t
	if err := k.fstat(fdlayout.Plan, &st); err != nil || st.Size > MaxPlan {
		return nil, fail("H2 size", fdlayout.ExitH2, errors.New("plan size"))
	}
	buf := make([]byte, st.Size)
	for off := 0; off < len(buf); {
		n, err := k.pread(fdlayout.Plan, buf[off:], int64(off))
		if err != nil || n == 0 {
			return nil, fail("H2 read", fdlayout.ExitH2, errors.New("short plan"))
		}
		off += n
	}
	p, err := ParsePlan(buf)
	if err != nil {
		return nil, fail("H2 parse", fdlayout.ExitH2, err)
	}
	return p, nil
}

// h3: oom_score_adj := 0, read back (S1).
func h3() error {
	const path = "/proc/self/oom_score_adj"
	if err := k.writeFile(path, []byte("0"), 0); err != nil {
		return fail("H3 write", fdlayout.ExitH3, err)
	}
	b, err := k.readFile(path)
	if err != nil || strings.TrimSpace(string(b)) != "0" {
		return fail("H3 read back", fdlayout.ExitH3, fmt.Errorf("oom_score_adj %q: %v", b, err))
	}
	return nil
}

// H4 refuses fs.suid_dumpable = 1 (ErrIsolationUnavailable{suiddumpable, linux}).
func H4() error {
	b, err := k.readFile(SuidDumpablePath)
	if err != nil {
		return fail("H4 read", fdlayout.ExitH4, err)
	}
	if strings.TrimSpace(string(b)) == "1" {
		return fail("H4", fdlayout.ExitH4, errors.New("fs.suid_dumpable is 1: ErrIsolationUnavailable{suiddumpable, linux}"))
	}
	return nil
}

// h5: MS_SLAVE | MS_REC on / before any other mount (S11).
func h5() error {
	if err := k.mount("", "/", "", unix.MS_SLAVE|unix.MS_REC, ""); err != nil {
		return fail("H5 MS_SLAVE", fdlayout.ExitH5, err)
	}
	return nil
}

// tmpfsMount builds a detached tmpfs with the given size option and attributes.
func tmpfsMount(size string, attrs int) (int, error) {
	fs, err := k.fsopen("tmpfs", unix.FSOPEN_CLOEXEC)
	if err != nil {
		return -1, err
	}
	defer k.close(fs)
	if size != "" {
		if err := k.fsconfigSetString(fs, "size", size); err != nil {
			return -1, err
		}
	}
	if err := k.fsconfigCreate(fs); err != nil {
		return -1, err
	}
	return k.fsmount(fs, unix.FSMOUNT_CLOEXEC, attrs)
}

// h7: credentials onto a private read-only tmpfs (T3). Sources and the
// delivery directory are opened with openat2(RESOLVE_NO_SYMLINKS).
func h7(p *Plan) error {
	if len(p.Creds) == 0 {
		return nil
	}
	size := p.CredSize
	if size == "" {
		size = "1m"
	}
	how := &unix.OpenHow{Flags: unix.O_CLOEXEC, Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS}
	var srcs []int
	defer func() {
		for _, fd := range srcs {
			k.close(fd)
		}
	}()
	for _, c := range p.Creds {
		if c.Name == "" || strings.ContainsRune(c.Name, '/') || c.Name == "." || c.Name == ".." {
			return fail("H7 name", fdlayout.ExitH7, fmt.Errorf("bad credential name %q", c.Name))
		}
		h := *how
		h.Flags = unix.O_RDONLY | unix.O_CLOEXEC
		fd, err := k.openat2(unix.AT_FDCWD, c.Source, &h)
		if err != nil {
			return fail("H7 open", fdlayout.ExitH7, err)
		}
		srcs = append(srcs, fd)
	}
	h := *how
	h.Flags = unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC
	dir, err := k.openat2(unix.AT_FDCWD, p.CredDir, &h)
	if err != nil {
		return fail("H7 dir", fdlayout.ExitH7, err)
	}
	defer k.close(dir)
	mfd, err := tmpfsMount(size, unix.MOUNT_ATTR_NOSUID|unix.MOUNT_ATTR_NODEV|unix.MOUNT_ATTR_NOEXEC)
	if err != nil {
		return fail("H7 tmpfs", fdlayout.ExitH7, err)
	}
	defer k.close(mfd)
	buf := make([]byte, 32<<10)
	defer clear(buf)
	for i, c := range p.Creds {
		if err := deliver(mfd, srcs[i], c.Name, int(p.Frame.UID), int(p.Frame.GID), buf); err != nil {
			return err
		}
	}
	if err := k.mountSetattr(mfd, "", unix.AT_EMPTY_PATH, &unix.MountAttr{Attr_set: unix.MOUNT_ATTR_RDONLY}); err != nil {
		return fail("H7 rdonly", fdlayout.ExitH7, err)
	}
	if err := k.moveMount(mfd, "", dir, "", unix.MOVE_MOUNT_F_EMPTY_PATH|unix.MOVE_MOUNT_T_EMPTY_PATH); err != nil {
		return fail("H7 move_mount", fdlayout.ExitH7, err)
	}
	return nil
}

// deliver copies one credential into the tmpfs as a 0400 file owned by u:g.
// A read error is a refusal: a truncated credential must never be delivered.
func deliver(mfd, src int, name string, uid, gid int, buf []byte) error {
	out, err := k.openat(mfd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o400)
	if err != nil {
		return fail("H7 create", fdlayout.ExitH7, err)
	}
	defer k.close(out)
	for {
		n, rerr := k.read(src, buf)
		if n > 0 {
			if w, werr := k.write(out, buf[:n]); werr != nil || w != n {
				return fail("H7 write", fdlayout.ExitH7, fmt.Errorf("short or failed write: %d of %d: %v", w, n, werr))
			}
		}
		if rerr != nil {
			return fail("H7 read", fdlayout.ExitH7, rerr)
		}
		if n == 0 {
			break
		}
	}
	if err := k.fchown(out, uid, gid); err != nil {
		return fail("H7 chown", fdlayout.ExitH7, err)
	}
	return nil
}

// h8: the lane's empty root (T9): an empty read-only tmpfs becomes /, and
// the old root is detached.
func h8() error {
	mfd, err := tmpfsMount("4k", unix.MOUNT_ATTR_NOSUID|unix.MOUNT_ATTR_NODEV|unix.MOUNT_ATTR_NOEXEC|unix.MOUNT_ATTR_RDONLY)
	if err != nil {
		return fail("H8 tmpfs", fdlayout.ExitH8, err)
	}
	defer k.close(mfd)
	// pivot_root needs a mount of this namespace that is not the root mount
	// (measured: stacking over "/" gives EBUSY). /proc is the one directory
	// the helper already relies on; the old root, /proc included, is
	// detached below.
	if err := k.moveMount(mfd, "", unix.AT_FDCWD, "/proc", unix.MOVE_MOUNT_F_EMPTY_PATH); err != nil {
		return fail("H8 move_mount", fdlayout.ExitH8, err)
	}
	if err := k.chdir("/proc"); err != nil {
		return fail("H8 chdir", fdlayout.ExitH8, err)
	}
	if err := k.pivotRoot(".", "."); err != nil {
		return fail("H8 pivot_root", fdlayout.ExitH8, err)
	}
	if err := k.unmount(".", unix.MNT_DETACH); err != nil {
		return fail("H8 umount", fdlayout.ExitH8, err)
	}
	return nil
}

// h9: the working directory as an O_PATH fd, and the sealed frame.
func h9(p *Plan) (cwd, frameFD int, err error) {
	cwd, err = k.open(p.Cwd, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return 0, 0, fail("H9 cwd", fdlayout.ExitH9, err)
	}
	f := p.Frame
	b, err := f.Encode()
	if err != nil {
		k.close(cwd)
		return 0, 0, fail("H9 encode", fdlayout.ExitH9, err)
	}
	frameFD, err = k.memfdCreate("svd-frame", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING|unix.MFD_NOEXEC_SEAL)
	if err != nil {
		k.close(cwd)
		return 0, 0, fail("H9 memfd", fdlayout.ExitH9, err)
	}
	if n, err := k.write(frameFD, b); err != nil || n != len(b) {
		k.close(cwd)
		k.close(frameFD)
		return 0, 0, fail("H9 write", fdlayout.ExitH9, fmt.Errorf("wrote %d of %d: %v", n, len(b), err))
	}
	if _, err := k.fcntlInt(uintptr(frameFD), unix.F_ADD_SEALS, Seals); err != nil {
		k.close(cwd)
		k.close(frameFD)
		return 0, 0, fail("H9 seal", fdlayout.ExitH9, err)
	}
	return cwd, frameFD, nil
}

// h10: lay out the stage's descriptors and exec the stage.
func h10(p *Plan, cwd, frameFD int) error {
	const high = 64
	// Park every source above the target range, then dup3 into place.
	srcs := [...]int{fdlayout.StatusW, fdlayout.ReadyFD, frameFD, cwd, fdlayout.StageFD}
	var parked [len(srcs)]int
	for i, s := range srcs {
		fd, err := k.fcntlInt(uintptr(s), unix.F_DUPFD_CLOEXEC, high)
		if err != nil {
			return fail("H10 park", fdlayout.ExitH10, err)
		}
		parked[i] = fd
	}
	for i, dst := range [...]int{fdlayout.StageStatus, fdlayout.StageReady, fdlayout.StageFrame, fdlayout.StageCwd} {
		if err := k.dup3(parked[i], dst, 0); err != nil {
			return fail("H10 dup3", fdlayout.ExitH10, err)
		}
	}
	stage := parked[4]
	if e := k.closeRange(fdlayout.StageFirstFree, ^uintptr(0)>>32, unix.CLOSE_RANGE_CLOEXEC); e != nil {
		return fail("H10 close_range", fdlayout.ExitH10, e)
	}
	// The stage and the parked copies are CLOEXEC; the stage fd is still
	// needed until the exec itself, and is closed by it.
	if p.Mode != ModeLane {
		err := k.exec("/proc/self/fd/"+strconv.Itoa(stage), p.StageArgv, []string{})
		return fail("H10 exec", fdlayout.ExitH10, err)
	}
	argv, err := syscall.SlicePtrFromStrings(p.StageArgv)
	if err != nil {
		return fail("H10 argv", fdlayout.ExitH10, err)
	}
	envp, _ := syscall.SlicePtrFromStrings([]string{})
	return fail("H10 execveat", fdlayout.ExitH10, k.execveat(stage, argv, envp))
}
