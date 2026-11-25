//go:build linux

package spawn

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/fdlayout"
	"github.com/go-supervise/supervisor/internal/helper"
)

var errBoom = errors.New("injected kernel failure")

// fsys is a whole sysOps table that succeeds by default. It tracks the fds,
// directories and socket names it handed out, so a test asserts both the
// step's refusal and that the rollback left nothing behind (T5).
type fsys struct {
	count   map[string]int
	failAt  map[string]error // "op#n" -> error returned by the n-th call of op
	open    map[int]string
	next    int
	dirs    map[string]bool
	bound   map[string]bool
	written map[string][]byte
	names   map[int]string
	unlink  []string
	chunk   int // bytes accepted per write; 0 = all
	fork    *syscall.ProcAttr
	forkArg []any
	lastDir int
}

func newFsys(t *testing.T) *fsys {
	f := &fsys{count: map[string]int{}, failAt: map[string]error{}, open: map[int]string{}, next: 200,
		dirs: map[string]bool{}, bound: map[string]bool{}, written: map[string][]byte{}, names: map[int]string{}}
	old, oldInject := sys, Inject
	t.Cleanup(func() { sys, Inject = old, oldInject })
	sys = sysOps{
		mkdirat: func(d int, p string, m uint32) error {
			if err := f.hit("mkdirat"); err != nil {
				return err
			}
			f.dirs[p] = true
			return nil
		},
		unlinkat: func(d int, p string, fl int) error {
			f.unlink = append(f.unlink, fmt.Sprintf("%s:%d", p, fl))
			if fl&unix.AT_REMOVEDIR != 0 {
				delete(f.dirs, p)
			} else {
				delete(f.bound, p)
			}
			return nil
		},
		openat: func(d int, p string, fl int, m uint32) (int, error) {
			if err := f.hit("openat"); err != nil {
				return -1, err
			}
			fd := f.alloc(p)
			f.names[fd] = p
			return fd, nil
		},
		close: func(fd int) error {
			if _, ok := f.open[fd]; !ok {
				t.Errorf("close of fd %d that is not open", fd)
			}
			delete(f.open, fd)
			return nil
		},
		pipe2: func(p []int, fl int) error {
			if err := f.hit("pipe2"); err != nil {
				return err
			}
			p[0], p[1] = f.alloc("pipe-r"), f.alloc("pipe-w")
			return nil
		},
		socket: func(a, b, c int) (int, error) {
			if err := f.hit("socket"); err != nil {
				return -1, err
			}
			return f.alloc("socket"), nil
		},
		bind: func(fd int, sa unix.Sockaddr) error {
			if err := f.hit("bind"); err != nil {
				return err
			}
			f.bound[sa.(*unix.SockaddrUnix).Name[strings.LastIndex(sa.(*unix.SockaddrUnix).Name, "/")+1:]] = true
			return nil
		},
		write: func(fd int, b []byte) (int, error) {
			if err := f.hit("write"); err != nil {
				return 0, err
			}
			if f.chunk > 0 && len(b) > f.chunk {
				b = b[:f.chunk]
			}
			f.written[f.open[fd]] = append(f.written[f.open[fd]], b...)
			return len(b), nil
		},
		memfdCreate: func(n string, fl int) (int, error) {
			f.forkArg = append(f.forkArg, "memfd", n, fl)
			if err := f.hit("memfd"); err != nil {
				return -1, err
			}
			return f.alloc("memfd"), nil
		},
		fcntlInt: func(fd uintptr, cmd, arg int) (int, error) {
			f.forkArg = append(f.forkArg, "fcntl", cmd, arg)
			return 0, f.hit("fcntl")
		},
		forkExec: func(a0 string, argv []string, attr *syscall.ProcAttr) (int, error) {
			f.fork = attr
			f.forkArg = append(f.forkArg, "fork", a0, argv)
			if err := f.hit("forkexec"); err != nil {
				return 0, err
			}
			*attr.Sys.PidFD = f.alloc("pidfd")
			return 4242, nil
		},
	}
	return f
}

func (f *fsys) alloc(what string) int {
	f.next++
	f.open[f.next] = what
	return f.next
}

// hit counts a call of op; failAt["op#n"] fails the n-th. A failure of
// EEXIST/EADDRINUSE is one the code is specified to retry once.
func (f *fsys) hit(op string) error {
	f.count[op]++
	return f.failAt[fmt.Sprintf("%s#%d", op, f.count[op])]
}

func cfg() *Config {
	return &Config{TreesFD: 10, Epoch: "e1", Limits: map[string]string{"pids.max": "9", "memory.max": "1048576"},
		NotifyDirFD: 11, NotifyName: "n1", DevNullFD: 12, HelperFD: 13, StageFD: 14, Plan: []byte("plan-bytes")}
}

func wantSEp(t *testing.T, err error, step string) {
	t.Helper()
	var se *ErrSEp
	if !errors.As(err, &se) || se.Step != step {
		t.Fatalf("got %v, want an ErrSEp at %q", err, step)
	}
	if !errors.Is(err, errBoom) && !strings.HasPrefix(step, "M2 mkdirat") && !strings.HasPrefix(step, "M3 bind") {
		if se.Err == nil {
			t.Errorf("ErrSEp carries no cause")
		}
	}
}

func (f *fsys) nothingLeft(t *testing.T) {
	t.Helper()
	if len(f.open) != 0 || len(f.dirs) != 0 || len(f.bound) != 0 {
		t.Errorf("rollback left fds %v, cgroup dirs %v, sockets %v", f.open, f.dirs, f.bound)
	}
}

func TestStartRefusals(t *testing.T) {
	for _, c := range []struct {
		fail string
		step string
	}{
		{"mkdirat#1", "M2 mkdirat"},
		{"openat#1", "M2 open"},
		{"openat#2", "M2 limit memory.max"}, // names are written in sorted order
		{"write#1", "M2 limit memory.max"},
		{"write#2", "M2 limit pids.max"},
		{"pipe2#1", "M3 log pipe"},
		{"pipe2#2", "M3 status pipe"},
		{"socket#1", "M3 socket"},
		{"bind#1", "M3 bind"},
		{"memfd#1", "M3 plan"},
		{"write#3", "M3 plan"},
		{"fcntl#1", "M3 plan"},
		{"forkexec#1", "M4 ForkExec"},
	} {
		t.Run(c.fail, func(t *testing.T) {
			f := newFsys(t)
			f.failAt[c.fail] = errBoom
			ch, err := Start(cfg())
			if ch != nil {
				t.Fatal("a child was returned with an error")
			}
			wantSEp(t, err, c.step)
			f.nothingLeft(t)
			if !errors.Is(err, errBoom) {
				t.Errorf("cause lost: %v", err)
			}
		})
	}
}

func TestStartInjectedSteps(t *testing.T) {
	for _, step := range []string{"M2", "M3", "M4"} {
		t.Run(step, func(t *testing.T) {
			f := newFsys(t)
			Inject = func(s string) error {
				if s == step {
					return errBoom
				}
				return nil
			}
			_, err := Start(cfg())
			wantSEp(t, err, step)
			f.nothingLeft(t)
			if step == "M2" && f.count["mkdirat"] != 0 {
				t.Error("M2 injected: nothing may be created")
			}
			if step == "M3" && f.count["pipe2"] != 0 {
				t.Error("M3 injected before its first pipe")
			}
			if step == "M4" && f.count["forkexec"] != 0 {
				t.Error("M4 injected: no fork")
			}
		})
	}
}

func TestStartRetriesALeftoverOnce(t *testing.T) {
	// EEXIST on the cgroup: removed once with AT_REMOVEDIR, retried once (T5).
	f := newFsys(t)
	f.failAt["mkdirat#1"] = unix.EEXIST
	ch, err := Start(cfg())
	if err != nil {
		t.Fatal(err)
	}
	if f.count["mkdirat"] != 2 || len(f.unlink) < 1 || f.unlink[0] != fmt.Sprintf("e1:%d", unix.AT_REMOVEDIR) {
		t.Errorf("mkdirat %d, unlinks %v", f.count["mkdirat"], f.unlink)
	}
	ch.Close()
	// A second EEXIST is the refusal.
	f = newFsys(t)
	f.failAt["mkdirat#1"], f.failAt["mkdirat#2"] = unix.EEXIST, unix.EEXIST
	_, err = Start(cfg())
	wantSEp(t, err, "M2 mkdirat")
	if !errors.Is(err, unix.EEXIST) || f.count["mkdirat"] != 2 {
		t.Errorf("err %v after %d mkdirat", err, f.count["mkdirat"])
	}
	f.nothingLeft(t)
	// Another error is not retried.
	f = newFsys(t)
	f.failAt["mkdirat#1"] = unix.EACCES
	Start(cfg())
	if f.count["mkdirat"] != 1 || len(f.unlink) != 0 {
		t.Errorf("EACCES retried: %d mkdirat, unlinks %v", f.count["mkdirat"], f.unlink)
	}

	// EADDRINUSE on the notify socket: its name unlinked (flags 0) once, bound again.
	f = newFsys(t)
	f.failAt["bind#1"] = unix.EADDRINUSE
	ch, err = Start(cfg())
	if err != nil {
		t.Fatal(err)
	}
	if f.count["bind"] != 2 || len(f.unlink) != 1 || f.unlink[0] != "n1:0" {
		t.Errorf("bind %d, unlinks %v", f.count["bind"], f.unlink)
	}
	ch.Close()
	f = newFsys(t)
	f.failAt["bind#1"], f.failAt["bind#2"] = unix.EADDRINUSE, unix.EADDRINUSE
	_, err = Start(cfg())
	wantSEp(t, err, "M3 bind")
	if f.count["bind"] != 2 {
		t.Errorf("%d binds", f.count["bind"])
	}
	f.nothingLeft(t)
	f = newFsys(t)
	f.failAt["bind#1"] = unix.EACCES
	Start(cfg())
	if f.count["bind"] != 1 {
		t.Errorf("EACCES retried: %d binds", f.count["bind"])
	}
}

func TestStartSuccessShape(t *testing.T) {
	for _, lane := range []bool{false, true} {
		t.Run(fmt.Sprint("lane=", lane), func(t *testing.T) {
			f := newFsys(t)
			f.chunk = 3 // the plan is written in pieces: the loop must advance
			c := cfg()
			c.Lane = lane
			ch, err := Start(c)
			if err != nil {
				t.Fatal(err)
			}
			if ch.PID != 4242 || ch.PidFD == 0 {
				t.Errorf("child %+v", ch)
			}
			// What the parent keeps: cgroup fd, log and status read ends, the socket, the pidfd.
			if len(f.open) != 5 {
				t.Errorf("open after success: %v (the plan and the child's ends must be closed)", f.open)
			}
			for _, fd := range []int{ch.PidFD, ch.CgroupFD, ch.LogR, ch.StatusR, ch.Notify} {
				if _, ok := f.open[fd]; !ok {
					t.Errorf("fd %d of the child is not open", fd)
				}
			}
			if f.open[ch.LogR] != "pipe-r" || f.open[ch.StatusR] != "pipe-r" || f.open[ch.Notify] != "socket" {
				t.Errorf("the parent kept the wrong ends: %v", f.open)
			}
			// the sealed plan memfd holds the plan in full, sealed with the four seals
			if got := f.written["memfd"]; !bytes.Equal(got, c.Plan) {
				t.Errorf("plan memfd holds %q", got)
			}
			if !reflect.DeepEqual(f.forkArg[:6], []any{"memfd", "svd-plan", unix.MFD_CLOEXEC | unix.MFD_ALLOW_SEALING | unix.MFD_NOEXEC_SEAL,
				"fcntl", unix.F_ADD_SEALS, helper.Seals}) {
				t.Errorf("memfd/seal calls %v", f.forkArg[:6])
			}
			fa := f.fork
			wantFlags := uintptr(unix.CLONE_NEWNS)
			if lane {
				wantFlags |= unix.CLONE_NEWIPC | unix.CLONE_NEWNET
			}
			s := fa.Sys
			if s.Cloneflags != wantFlags || !s.Setsid || !s.UseCgroupFD || s.CgroupFD != ch.CgroupFD {
				t.Errorf("SysProcAttr %+v, want flags %#x", *s, wantFlags)
			}
			if fa.Env == nil || len(fa.Env) != 0 || len(fa.Files) != fdlayout.NFiles {
				t.Errorf("env %v files %d", fa.Env, len(fa.Files))
			}
			if !reflect.DeepEqual(f.forkArg[6:], []any{"fork", "/proc/self/fd/7", []string{"svd", "helper"}}) {
				t.Errorf("fork call %v", f.forkArg[6:])
			}
			// Files: /dev/null, the log pipe twice, the plan, status, readiness, stage, helper.
			wantFiles := []uintptr{12, uintptr(fa.Files[1]), uintptr(fa.Files[1]), uintptr(fa.Files[3]), uintptr(fa.Files[4]), uintptr(ch.Notify), 14, 13}
			if !reflect.DeepEqual(fa.Files, wantFiles) || fa.Files[1] == uintptr(ch.LogR) || fa.Files[4] == uintptr(ch.StatusR) {
				t.Errorf("Files %v", fa.Files)
			}
			if f.written["memfd"] == nil || f.written["memfd"][0] != 'p' {
				t.Error("plan not written")
			}
			ch.Close()
			if len(f.open) != 0 {
				t.Errorf("Child.Close left %v", f.open)
			}
		})
	}
}

func TestLimitsAreWrittenRelativeToTheCgroupFD(t *testing.T) {
	f := newFsys(t)
	ch, err := Start(cfg())
	if err != nil {
		t.Fatal(err)
	}
	if string(f.written["memory.max"]) != "1048576" || string(f.written["pids.max"]) != "9" {
		t.Errorf("limits written: %v", f.written)
	}
	ch.Close()
}

func TestWriteAtAndSealedMemfd(t *testing.T) {
	f := newFsys(t)
	f.failAt["openat#1"] = errBoom
	if err := writeAt(10, "x", "v"); !errors.Is(err, errBoom) {
		t.Errorf("open failure: %v", err)
	}
	f = newFsys(t)
	f.failAt["write#1"] = errBoom
	if err := writeAt(10, "x", "v"); !errors.Is(err, errBoom) || len(f.open) != 0 {
		t.Errorf("write failure: %v, open %v", err, f.open)
	}
	f = newFsys(t)
	f.failAt["memfd#1"] = errBoom
	if fd, err := sealedMemfd("n", []byte("abc")); fd != -1 || !errors.Is(err, errBoom) {
		t.Errorf("memfd failure: %d %v", fd, err)
	}
	f = newFsys(t)
	f.failAt["write#1"] = errBoom
	if fd, err := sealedMemfd("n", []byte("abc")); fd != -1 || !errors.Is(err, errBoom) || len(f.open) != 0 {
		t.Errorf("memfd write failure: %d %v, open %v", fd, err, f.open)
	}
	f = newFsys(t)
	f.failAt["fcntl#1"] = errBoom
	if fd, err := sealedMemfd("n", []byte("abc")); fd != -1 || !errors.Is(err, errBoom) || !strings.HasPrefix(err.Error(), "seal:") || len(f.open) != 0 {
		t.Errorf("seal failure: %d %v, open %v", fd, err, f.open)
	}
}
