//go:build linux

package helper

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/fdlayout"
	"github.com/go-supervise/supervisor/internal/frame"
)

var errBoom = errors.New("injected kernel failure")

// fake is a whole kernelOps table that succeeds by default, records every call
// and tracks the descriptors it handed out, so a test can drive one error
// branch of H1-H10 without privilege and assert what the step left behind.
type fake struct {
	t       *testing.T
	calls   []string
	count   map[string]int
	failAt  map[string]int // op -> 1-based call number that fails
	open    map[int]bool
	next    int
	plan    []byte
	planLen *int64 // fstat size of the plan fd, when set
	tty     int    // /dev/tty: 0 ENXIO, 1 opens, 2 EACCES
	files   map[string][]byte
	written map[string][]byte // by file name created with openat
	outName map[int]string
	srcs    map[string][]byte // credential sources by path
	srcByFD map[int][]byte
	srcOff  map[int]int
	args    map[string][]any // last arguments of an op
}

func newFake(t *testing.T) *fake {
	f := &fake{t: t, count: map[string]int{}, failAt: map[string]int{}, open: map[int]bool{}, next: 100,
		files:   map[string][]byte{"/proc/self/oom_score_adj": []byte("0\n"), SuidDumpablePath: []byte("0\n")},
		written: map[string][]byte{}, outName: map[int]string{}, srcs: map[string][]byte{},
		srcByFD: map[int][]byte{}, srcOff: map[int]int{}, args: map[string][]any{}}
	old := k
	t.Cleanup(func() { k = old })
	k = kernelOps{
		closeRange: func(a, b, c uintptr) error { f.args["close_range"] = []any{a, b, c}; return f.hit("close_range") },
		fstat: func(fd int, st *unix.Stat_t) error {
			if err := f.hit("fstat"); err != nil {
				return err
			}
			switch fd {
			case 0:
				st.Mode, st.Rdev = unix.S_IFCHR, unix.Mkdev(1, 3)
			case 1, 2:
				st.Dev, st.Ino = 5, 9
			case fdlayout.Plan:
				st.Size = int64(len(f.plan))
				if f.planLen != nil {
					st.Size = *f.planLen
				}
			}
			return nil
		},
		getsid: func(int) (int, error) { return 42, f.hit("getsid") },
		getpid: func() int { return 42 },
		open: func(path string, mode int, perm uint32) (int, error) {
			f.args["open"] = []any{path, mode}
			if path == "/dev/tty" {
				switch f.tty {
				case 1:
					return f.alloc(), nil // a controlling terminal exists
				case 2:
					return -1, unix.EACCES
				}
				return -1, unix.ENXIO
			}
			if err := f.hit("open"); err != nil {
				return -1, err
			}
			return f.alloc(), nil
		},
		close: func(fd int) error {
			f.calls = append(f.calls, "close")
			if !f.open[fd] {
				t.Errorf("close of fd %d that is not open", fd)
			}
			delete(f.open, fd)
			return nil
		},
		fcntlInt: func(fd uintptr, cmd, arg int) (int, error) {
			f.args["fcntl"] = append(f.args["fcntl"], fd, cmd, arg)
			switch cmd {
			case unix.F_GET_SEALS:
				return Seals | unix.F_SEAL_EXEC, f.hit("fcntl_seals")
			case unix.F_DUPFD_CLOEXEC:
				if err := f.hit("fcntl_dup"); err != nil {
					return -1, err
				}
				return f.alloc(), nil
			}
			return 0, f.hit("fcntl_add_seals")
		},
		pread: func(fd int, b []byte, off int64) (int, error) {
			if err := f.hit("pread"); err != nil {
				return 0, err
			}
			if int(off) >= len(f.plan) {
				return 0, nil
			}
			return copy(b[:1], f.plan[off:]), nil // one byte per call: the loop must advance
		},
		readFile: func(path string) ([]byte, error) {
			if err := f.hit("read_file"); err != nil {
				return nil, err
			}
			b, ok := f.files[path]
			if !ok {
				return nil, os.ErrNotExist
			}
			return b, nil
		},
		writeFile: func(path string, b []byte, perm os.FileMode) error {
			f.args["write_file"] = []any{path, string(b)}
			return f.hit("write_file")
		},
		mount: func(s, tg, ty string, fl uintptr, d string) error {
			f.args["mount"] = []any{s, tg, ty, fl, d}
			return f.hit("mount")
		},
		fsopen: func(n string, fl int) (int, error) {
			f.args["fsopen"] = []any{n, fl}
			if err := f.hit("fsopen"); err != nil {
				return -1, err
			}
			return f.alloc(), nil
		},
		fsconfigSetString: func(fd int, key, v string) error {
			f.args["fsconfig_set"] = []any{key, v}
			return f.hit("fsconfig_set")
		},
		fsconfigCreate: func(int) error { return f.hit("fsconfig_create") },
		fsmount: func(fd, fl, attrs int) (int, error) {
			f.args["fsmount"] = []any{fl, attrs}
			if err := f.hit("fsmount"); err != nil {
				return -1, err
			}
			return f.alloc(), nil
		},
		openat2: func(dirfd int, path string, how *unix.OpenHow) (int, error) {
			f.args["openat2"] = append(f.args["openat2"], path, how.Flags, how.Resolve)
			if err := f.hit("openat2"); err != nil {
				return -1, err
			}
			fd := f.alloc()
			if b, ok := f.srcs[path]; ok {
				f.srcByFD[fd] = b
			}
			return fd, nil
		},
		openat: func(dirfd int, path string, fl int, perm uint32) (int, error) {
			f.args["openat"] = append(f.args["openat"], path, fl, perm)
			if err := f.hit("openat"); err != nil {
				return -1, err
			}
			fd := f.alloc()
			f.outName[fd] = path
			return fd, nil
		},
		read: func(fd int, b []byte) (int, error) {
			if err := f.hit("read"); err != nil {
				return 0, err
			}
			src := f.srcByFD[fd]
			n := copy(b, src[f.srcOff[fd]:])
			f.srcOff[fd] += n
			return n, nil
		},
		write: func(fd int, b []byte) (int, error) {
			f.args["write"] = []any{fd, string(b)}
			if err := f.hit("write"); err != nil {
				return 0, err
			}
			if f.failAt["write_short"] == f.count["write"] {
				return len(b) - 1, nil
			}
			if name, ok := f.outName[fd]; ok {
				f.written[name] = append(f.written[name], b...)
			}
			return len(b), nil
		},
		fchown: func(fd, u, g int) error {
			f.args["fchown"] = append(f.args["fchown"], u, g)
			return f.hit("fchown")
		},
		mountSetattr: func(d int, p string, fl uint, a *unix.MountAttr) error {
			f.args["mount_setattr"] = []any{p, fl, a.Attr_set}
			return f.hit("mount_setattr")
		},
		moveMount: func(a int, ap string, b int, bp string, fl int) error {
			f.calls = append(f.calls, "move_mount:"+bp)
			f.args["move_mount"] = append(f.args["move_mount"], fl)
			return f.hit("move_mount")
		},
		chdir:     func(p string) error { f.calls = append(f.calls, "chdir:"+p); return f.hit("chdir") },
		pivotRoot: func(n, o string) error { f.calls = append(f.calls, "pivot_root:"+n+":"+o); return f.hit("pivot_root") },
		unmount: func(tg string, fl int) error {
			f.calls = append(f.calls, fmt.Sprintf("unmount:%s:%d", tg, fl))
			return f.hit("unmount")
		},
		memfdCreate: func(n string, fl int) (int, error) {
			f.args["memfd"] = []any{n, fl}
			if err := f.hit("memfd"); err != nil {
				return -1, err
			}
			return f.alloc(), nil
		},
		dup3: func(o, n, fl int) error {
			f.args["dup3"] = append(f.args["dup3"], o, n, fl)
			return f.hit("dup3")
		},
		exec: func(path string, argv, envp []string) error {
			f.args["exec"] = []any{path, argv, envp}
			f.calls = append(f.calls, "exec")
			f.hit("exec")
			return errBoom
		},
		execveat: func(fd int, argv, envp []*byte) error {
			f.args["execveat"] = []any{fd, len(argv), len(envp)}
			f.calls = append(f.calls, "execveat")
			f.hit("execveat")
			return errBoom
		},
	}
	return f
}

func (f *fake) alloc() int {
	f.next++
	f.open[f.next] = true
	return f.next
}

// hit counts a call of op and fails it when it is the one the test chose.
func (f *fake) hit(op string) error {
	f.count[op]++
	f.calls = append(f.calls, op)
	if f.failAt[op] == f.count[op] {
		return errBoom
	}
	return nil
}

func (f *fake) called(op string) int { return f.count[op] }

func (f *fake) noLeak(t *testing.T) {
	t.Helper()
	if len(f.open) != 0 {
		t.Errorf("descriptors left open: %v", f.open)
	}
}

func wantStep(t *testing.T, err error, step string, code int) {
	t.Helper()
	se, ok := err.(*stepError)
	if !ok {
		t.Fatalf("want step %q (exit %d), got %v", step, code, err)
	}
	if se.step != step || se.code != code {
		t.Errorf("got step %q exit %d (%v), want %q exit %d", se.step, se.code, se.err, step, code)
	}
}

func testPlan(mode Mode) *Plan {
	return &Plan{Mode: mode, Cwd: "/w", StageArgv: []string{"stage", "x"}, Frame: frame.Frame{UID: 1000, GID: 2000}}
}
