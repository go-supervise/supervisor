//go:build linux

package helper

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/fdlayout"
)

// Every test below drives one error branch of a step through the kernel table
// and asserts the step's specified refusal (SUPERVISOR.md 7.1.1): the step's
// own exit code, and nothing left open.

func TestRealKernelWrappers(t *testing.T) {
	// An empty range closes nothing; bad flags are refused.
	if err := closeRange(1<<30, closeRangeLast, unix.CLOSE_RANGE_CLOEXEC); err != nil {
		t.Errorf("close_range of an empty range: %v", err)
	}
	if err := closeRange(1<<30, closeRangeLast, 1<<20); err != unix.EINVAL {
		t.Errorf("close_range with bad flags = %v, want EINVAL", err)
	}
	// execveat never returns on success: with a bad fd the return is the error.
	argv := []*byte{nil}
	if err := execveat(-1, argv, argv); err != unix.EBADF {
		t.Errorf("execveat(-1) = %v, want EBADF", err)
	}
	if k.getpid() == 0 || k.close == nil {
		t.Error("the real table is incomplete")
	}
}

func TestH1Faults(t *testing.T) {
	cases := []struct {
		name string
		set  func(f *fake)
		step string
	}{
		{"close_range", func(f *fake) { f.failAt["close_range"] = 1 }, "H1 close_range"},
		{"fd0 fstat", func(f *fake) { f.failAt["fstat"] = 1 }, "H1 fd 0"},
		{"fd1 fstat", func(f *fake) { f.failAt["fstat"] = 2 }, "H1 fds 1,2"},
		{"fd2 fstat", func(f *fake) { f.failAt["fstat"] = 3 }, "H1 fds 1,2"},
		{"getsid", func(f *fake) { f.failAt["getsid"] = 1 }, "H1 session"},
		{"tty opens", func(f *fake) { f.tty = 1 }, "H1 ctty"},
		{"tty other error", func(f *fake) { f.tty = 2 }, "H1 ctty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t)
			c.set(f)
			wantStep(t, h1(), c.step, fdlayout.ExitH1)
			f.noLeak(t)
		})
	}
	// Values, not errors: each clause of each condition is held.
	vals := []struct {
		name  string
		fstat func(int, *unix.Stat_t) error
		sid   int
		step  string
	}{
		{"fd0 regular file", func(fd int, st *unix.Stat_t) error {
			if fd == 0 {
				st.Mode, st.Rdev = unix.S_IFREG, unix.Mkdev(1, 3)
			}
			return nil
		}, 42, "H1 fd 0"},
		{"fd0 wrong device", func(fd int, st *unix.Stat_t) error {
			if fd == 0 {
				st.Mode, st.Rdev = unix.S_IFCHR, unix.Mkdev(1, 5)
			}
			return nil
		}, 42, "H1 fd 0"},
		{"fds 1,2 differ by device", func(fd int, st *unix.Stat_t) error {
			switch fd {
			case 0:
				st.Mode, st.Rdev = unix.S_IFCHR, unix.Mkdev(1, 3)
			case 1:
				st.Dev, st.Ino = 5, 9
			case 2:
				st.Dev, st.Ino = 6, 9
			}
			return nil
		}, 42, "H1 fds 1,2"},
		{"fds 1,2 differ by inode", func(fd int, st *unix.Stat_t) error {
			switch fd {
			case 0:
				st.Mode, st.Rdev = unix.S_IFCHR, unix.Mkdev(1, 3)
			case 1:
				st.Dev, st.Ino = 5, 9
			case 2:
				st.Dev, st.Ino = 5, 10
			}
			return nil
		}, 42, "H1 fds 1,2"},
		{"not a session leader", nil, 7, "H1 session"},
	}
	for _, c := range vals {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t)
			if c.fstat != nil {
				k.fstat = c.fstat
			}
			k.getsid = func(int) (int, error) { return c.sid, nil }
			wantStep(t, h1(), c.step, fdlayout.ExitH1)
			f.noLeak(t)
		})
	}
	t.Run("ok", func(t *testing.T) {
		f := newFake(t)
		if err := h1(); err != nil {
			t.Fatal(err)
		}
		if got := f.args["close_range"]; !reflect.DeepEqual(got, []any{uintptr(fdlayout.HelperFD), closeRangeLast, uintptr(unix.CLOSE_RANGE_CLOEXEC)}) {
			t.Errorf("close_range args %v: from k, CLOEXEC, never flag 0", got)
		}
	})
}

func TestH2Faults(t *testing.T) {
	good, _ := testPlan(ModeService).Marshal()
	cases := []struct {
		name string
		set  func(f *fake)
		step string
	}{
		{"F_GET_SEALS errors with good seals", func(f *fake) { f.failAt["fcntl_seals"] = 1 }, "H2 seals"},
		{"fstat", func(f *fake) { f.failAt["fstat"] = 1 }, "H2 size"},
		{"too large", func(f *fake) { n := int64(MaxPlan + 1); f.planLen = &n }, "H2 size"},
		{"pread", func(f *fake) { f.failAt["pread"] = 2 }, "H2 read"},
		{"short plan", func(f *fake) { n := int64(len(good) + 5); f.planLen = &n }, "H2 read"},
		{"parse", func(f *fake) { f.plan = []byte(`{"mode":"x"}`) }, "H2 parse"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t)
			f.plan = good
			c.set(f)
			_, err := h2()
			wantStep(t, err, c.step, fdlayout.ExitH2)
		})
	}
	t.Run("a seal missing", func(t *testing.T) {
		f := newFake(t)
		f.plan = good
		k.fcntlInt = func(uintptr, int, int) (int, error) { return Seals &^ unix.F_SEAL_GROW, nil }
		_, err := h2()
		wantStep(t, err, "H2 seals", fdlayout.ExitH2)
		_ = f
	})
	t.Run("exactly MaxPlan passes the size check", func(t *testing.T) {
		f := newFake(t)
		n := int64(MaxPlan)
		f.planLen = &n
		f.plan = bytes.Repeat([]byte{' '}, MaxPlan)
		_, err := h2()
		wantStep(t, err, "H2 parse", fdlayout.ExitH2) // past the size check, refused by the parser
	})
	t.Run("ok, read a byte at a time", func(t *testing.T) {
		f := newFake(t)
		f.plan = good
		p, err := h2()
		if err != nil || p.Cwd != "/w" || p.Mode != ModeService {
			t.Fatalf("plan %v err %v", p, err)
		}
	})
}

func TestH3Faults(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(f *fake)
		step string
	}{
		{"write", func(f *fake) { f.failAt["write_file"] = 1 }, "H3 write"},
		{"read back fails", func(f *fake) { f.failAt["read_file"] = 1 }, "H3 read back"},
		{"read back is 1", func(f *fake) { f.files["/proc/self/oom_score_adj"] = []byte("1\n") }, "H3 read back"},
		{"read back is -1000", func(f *fake) { f.files["/proc/self/oom_score_adj"] = []byte("-1000\n") }, "H3 read back"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t)
			c.set(f)
			wantStep(t, h3(), c.step, fdlayout.ExitH3)
		})
	}
	f := newFake(t)
	f.files["/proc/self/oom_score_adj"] = []byte(" 0 \n")
	if err := h3(); err != nil {
		t.Fatal(err)
	}
	if got := f.args["write_file"]; got[0] != "/proc/self/oom_score_adj" || got[1] != "0" {
		t.Errorf("wrote %v", got)
	}
}

func TestH4Values(t *testing.T) {
	f := newFake(t)
	f.failAt["read_file"] = 1
	wantStep(t, H4(), "H4 read", fdlayout.ExitH4)
	f = newFake(t)
	f.files[SuidDumpablePath] = []byte("1\n")
	wantStep(t, H4(), "H4", fdlayout.ExitH4)
	for _, v := range []string{"0\n", "2\n"} {
		f.files[SuidDumpablePath] = []byte(v)
		if err := H4(); err != nil {
			t.Errorf("suid_dumpable %q: %v", v, err)
		}
	}
}

func TestH5(t *testing.T) {
	f := newFake(t)
	f.failAt["mount"] = 1
	wantStep(t, h5(), "H5 MS_SLAVE", fdlayout.ExitH5)
	f = newFake(t)
	if err := h5(); err != nil {
		t.Fatal(err)
	}
	if got := f.args["mount"]; !reflect.DeepEqual(got, []any{"", "/", "", uintptr(unix.MS_SLAVE | unix.MS_REC), ""}) {
		t.Errorf("mount args %v", got)
	}
}

func TestTmpfsMountFaults(t *testing.T) {
	for _, op := range []string{"fsopen", "fsconfig_set", "fsconfig_create", "fsmount"} {
		t.Run(op, func(t *testing.T) {
			f := newFake(t)
			f.failAt[op] = 1
			fd, err := tmpfsMount("1m", 0)
			if !errors.Is(err, errBoom) || fd != -1 {
				t.Errorf("fd %d err %v", fd, err)
			}
			f.noLeak(t)
		})
	}
	f := newFake(t)
	fd, err := tmpfsMount("", unix.MOUNT_ATTR_NOSUID)
	if err != nil || f.called("fsconfig_set") != 0 || len(f.open) != 1 || !f.open[fd] {
		t.Errorf("empty size: fd %d err %v calls %v open %v", fd, err, f.calls, f.open)
	}
	if got := f.args["fsmount"]; !reflect.DeepEqual(got, []any{unix.FSMOUNT_CLOEXEC, unix.MOUNT_ATTR_NOSUID}) {
		t.Errorf("fsmount args %v", got)
	}
	f = newFake(t)
	tmpfsMount("4k", 0)
	if got := f.args["fsconfig_set"]; !reflect.DeepEqual(got, []any{"size", "4k"}) {
		t.Errorf("size option %v", got)
	}
}

func credPlan() *Plan {
	p := testPlan(ModeService)
	p.CredDir = "/d"
	p.Creds = []Credential{{Name: "a", Source: "/s/a"}, {Name: "b", Source: "/s/b"}}
	return p
}

func TestH7Faults(t *testing.T) {
	for _, c := range []struct {
		op   string
		nth  int
		step string
	}{
		{"openat2", 1, "H7 open"}, {"openat2", 2, "H7 open"}, {"openat2", 3, "H7 dir"},
		{"fsopen", 1, "H7 tmpfs"}, {"fsconfig_set", 1, "H7 tmpfs"}, {"fsconfig_create", 1, "H7 tmpfs"}, {"fsmount", 1, "H7 tmpfs"},
		{"openat", 1, "H7 create"}, {"openat", 2, "H7 create"},
		{"read", 1, "H7 read"}, {"read", 3, "H7 read"},
		{"write", 1, "H7 write"}, {"write_short", 1, "H7 write"},
		{"fchown", 1, "H7 chown"}, {"fchown", 2, "H7 chown"},
		{"mount_setattr", 1, "H7 rdonly"}, {"move_mount", 1, "H7 move_mount"},
	} {
		t.Run(fmt.Sprintf("%s#%d", c.op, c.nth), func(t *testing.T) {
			f := newFake(t)
			f.srcs["/s/a"] = bytes.Repeat([]byte{'a'}, 40000)
			f.srcs["/s/b"] = []byte("bee")
			f.failAt[c.op] = c.nth
			if c.op == "write_short" {
				f.failAt["write_short"] = 1
				f.count["write"] = 0
			}
			wantStep(t, h7(credPlan()), c.step, fdlayout.ExitH7)
			f.noLeak(t) // sources, directory, mount fd and the output file: all closed
		})
	}
	for _, name := range []string{"", "a/b", ".", ".."} {
		t.Run("name "+name, func(t *testing.T) {
			f := newFake(t)
			p := credPlan()
			p.Creds[1].Name = name
			f.srcs["/s/a"], f.srcs["/s/b"] = []byte("x"), []byte("y")
			wantStep(t, h7(p), "H7 name", fdlayout.ExitH7)
			f.noLeak(t)
		})
	}
}

func TestH7Delivers(t *testing.T) {
	f := newFake(t)
	big := bytes.Repeat([]byte("0123456789"), 4000) // 40000 bytes: two reads
	f.srcs["/s/a"], f.srcs["/s/b"] = big, []byte("bee")
	if err := h7(credPlan()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.written["a"], big) || string(f.written["b"]) != "bee" {
		t.Errorf("delivered %d and %d bytes", len(f.written["a"]), len(f.written["b"]))
	}
	f.noLeak(t)
	if got := f.args["fsconfig_set"]; !reflect.DeepEqual(got, []any{"size", "1m"}) {
		t.Errorf("default size %v", got)
	}
	if got := f.args["fsmount"]; !reflect.DeepEqual(got, []any{unix.FSMOUNT_CLOEXEC,
		unix.MOUNT_ATTR_NOSUID | unix.MOUNT_ATTR_NODEV | unix.MOUNT_ATTR_NOEXEC}) {
		t.Errorf("tmpfs attributes %v", got)
	}
	if got := f.args["fchown"]; !reflect.DeepEqual(got, []any{1000, 2000, 1000, 2000}) {
		t.Errorf("owner %v", got)
	}
	if got := f.args["openat"]; !reflect.DeepEqual(got, []any{"a", unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_CLOEXEC, uint32(0o400),
		"b", unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_CLOEXEC, uint32(0o400)}) {
		t.Errorf("create args %v", got)
	}
	rs := uint64(unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS)
	src := uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK)
	want := []any{"/s/a", src, rs, "/s/b", src, rs,
		"/d", uint64(unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC), rs}
	if got := f.args["openat2"]; !reflect.DeepEqual(got, want) {
		t.Errorf("openat2 args %v", got)
	}
	if got := f.args["mount_setattr"]; !reflect.DeepEqual(got, []any{"", uint(unix.AT_EMPTY_PATH), uint64(unix.MOUNT_ATTR_RDONLY)}) {
		t.Errorf("mount_setattr %v", got)
	}
	if got := f.args["move_mount"]; !reflect.DeepEqual(got, []any{unix.MOVE_MOUNT_F_EMPTY_PATH | unix.MOVE_MOUNT_T_EMPTY_PATH}) {
		t.Errorf("move_mount flags %v", got)
	}
	// A custom size is used; no credential means no call at all.
	f = newFake(t)
	f.srcs["/s/a"], f.srcs["/s/b"] = []byte("x"), []byte("y")
	p := credPlan()
	p.CredSize = "4k"
	h7(p)
	if got := f.args["fsconfig_set"]; !reflect.DeepEqual(got, []any{"size", "4k"}) {
		t.Errorf("custom size %v", got)
	}
	f = newFake(t)
	if err := h7(testPlan(ModeService)); err != nil || len(f.calls) != 0 {
		t.Errorf("no credentials: err %v calls %v", err, f.calls)
	}
}

// A credential source must be a regular file: a FIFO would block the read, a
// device or directory is not a secret. fstat decides, on the descriptor opened.
func TestH7RefusesASourceThatIsNotARegularFile(t *testing.T) {
	for _, c := range []struct {
		name string
		mode uint32
	}{{"fifo", unix.S_IFIFO}, {"char device", unix.S_IFCHR}, {"directory", unix.S_IFDIR}, {"socket", unix.S_IFSOCK}} {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t)
			f.srcs["/s/a"], f.srcs["/s/b"] = []byte("x"), []byte("y")
			f.srcMode["/s/b"] = c.mode
			wantStep(t, h7(credPlan()), "H7 source", fdlayout.ExitH7)
			f.noLeak(t)
		})
	}
	t.Run("fstat fails", func(t *testing.T) {
		f := newFake(t)
		f.srcs["/s/a"], f.srcs["/s/b"] = []byte("x"), []byte("y")
		f.failAt["fstat"] = 1
		// a failed fstat that still leaves a regular-file mode behind: only the
		// error itself can refuse the source, not the type check after it
		orig := k.fstat
		k.fstat = func(fd int, st *unix.Stat_t) error {
			err := orig(fd, st)
			if err != nil {
				st.Mode = unix.S_IFREG
			}
			return err
		}
		wantStep(t, h7(credPlan()), "H7 source", fdlayout.ExitH7)
		f.noLeak(t)
	})
}

func TestH8(t *testing.T) {
	for _, c := range []struct{ op, step string }{
		{"fsopen", "H8 tmpfs"}, {"fsmount", "H8 tmpfs"}, {"move_mount", "H8 move_mount"},
		{"chdir", "H8 chdir"}, {"pivot_root", "H8 pivot_root"}, {"unmount", "H8 umount"},
	} {
		t.Run(c.op, func(t *testing.T) {
			f := newFake(t)
			f.failAt[c.op] = 1
			wantStep(t, h8(), c.step, fdlayout.ExitH8)
			f.noLeak(t)
		})
	}
	f := newFake(t)
	if err := h8(); err != nil {
		t.Fatal(err)
	}
	var seq []string
	for _, c := range f.calls {
		if strings.Contains(c, ":") {
			seq = append(seq, c)
		}
	}
	want := []string{"move_mount:/proc", "chdir:/proc", "pivot_root:.:.", fmt.Sprintf("unmount:.:%d", unix.MNT_DETACH)}
	if !reflect.DeepEqual(seq, want) {
		t.Errorf("sequence %v, want %v", seq, want)
	}
	if got := f.args["fsmount"]; !reflect.DeepEqual(got, []any{unix.FSMOUNT_CLOEXEC,
		unix.MOUNT_ATTR_NOSUID | unix.MOUNT_ATTR_NODEV | unix.MOUNT_ATTR_NOEXEC | unix.MOUNT_ATTR_RDONLY}) {
		t.Errorf("lane root attributes %v", got)
	}
	if got := f.args["fsconfig_set"]; !reflect.DeepEqual(got, []any{"size", "4k"}) {
		t.Errorf("lane root size %v", got)
	}
	if got := f.args["move_mount"]; !reflect.DeepEqual(got, []any{unix.MOVE_MOUNT_F_EMPTY_PATH}) {
		t.Errorf("move_mount flags %v", got)
	}
}

func TestH9(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(f *fake, p *Plan)
		step string
	}{
		{"cwd", func(f *fake, p *Plan) { f.failAt["openat2"] = 1 }, "H9 cwd"},
		{"encode", func(f *fake, p *Plan) { p.Frame.Groups = make([]uint32, 65) }, "H9 encode"},
		{"memfd", func(f *fake, p *Plan) { f.failAt["memfd"] = 1 }, "H9 memfd"},
		{"write", func(f *fake, p *Plan) { f.failAt["write"] = 1 }, "H9 write"},
		{"short write", func(f *fake, p *Plan) { f.failAt["write_short"] = 1 }, "H9 write"},
		{"seal", func(f *fake, p *Plan) { f.failAt["fcntl_add_seals"] = 1 }, "H9 seal"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t)
			p := testPlan(ModeService)
			c.set(f, p)
			if c.name == "short write" {
				f.count["write"] = 0
			}
			_, _, err := h9(p)
			wantStep(t, err, c.step, fdlayout.ExitH9)
			f.noLeak(t) // neither the cwd fd nor the frame survives a refusal
		})
	}
	f := newFake(t)
	cwd, fr, err := h9(testPlan(ModeService))
	if err != nil || cwd == fr || !f.open[cwd] || !f.open[fr] {
		t.Fatalf("cwd %d frame %d err %v", cwd, fr, err)
	}
	if got := f.args["openat2"]; !reflect.DeepEqual(got, []any{"/w", uint64(unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC),
		uint64(unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS)}) {
		t.Errorf("cwd open %v", got)
	}
	if got := f.args["memfd"]; !reflect.DeepEqual(got, []any{"svd-frame", unix.MFD_CLOEXEC | unix.MFD_ALLOW_SEALING | unix.MFD_NOEXEC_SEAL}) {
		t.Errorf("memfd %v", got)
	}
	if got := f.args["fcntl"]; !reflect.DeepEqual(got, []any{uintptr(fr), unix.F_ADD_SEALS, Seals}) {
		t.Errorf("seal call %v", got)
	}
}

func TestH10(t *testing.T) {
	p := func(m Mode) *Plan { return testPlan(m) }
	// park #1..#5, dup3 #1..#4, close_range: each failure is the step's refusal.
	for n := 1; n <= 5; n++ {
		t.Run(fmt.Sprintf("park#%d", n), func(t *testing.T) {
			f := newFake(t)
			f.failAt["fcntl_dup"] = n
			wantStep(t, h10(p(ModeService), 50, 51, nil), "H10 park", fdlayout.ExitH10)
		})
	}
	for n := 1; n <= 4; n++ {
		t.Run(fmt.Sprintf("dup3#%d", n), func(t *testing.T) {
			f := newFake(t)
			f.failAt["dup3"] = n
			wantStep(t, h10(p(ModeService), 50, 51, nil), "H10 dup3", fdlayout.ExitH10)
			if f.called("exec") != 0 {
				t.Error("exec after a failed dup3")
			}
		})
	}
	t.Run("close_range", func(t *testing.T) {
		f := newFake(t)
		f.failAt["close_range"] = 1
		wantStep(t, h10(p(ModeService), 50, 51, nil), "H10 close_range", fdlayout.ExitH10)
		if f.called("exec") != 0 {
			t.Error("exec after a failed close_range")
		}
	})
	for _, m := range []Mode{ModeService, ModeSignal} {
		t.Run("exec "+string(m), func(t *testing.T) {
			f := newFake(t)
			wantStep(t, h10(p(m), 50, 51, nil), "H10 exec", fdlayout.ExitH10)
			if f.called("execveat") != 0 {
				t.Error("execveat in a mode with /proc")
			}
			if f.called("exec") != 1 {
				t.Fatalf("exec called %d times in mode %s", f.called("exec"), m)
			}
			a := f.args["exec"]
			if a[1].([]string)[0] != "stage" || len(a[2].([]string)) != 0 || a[2].([]string) == nil {
				t.Errorf("exec args %v: argv from the plan, an empty non-nil environment", a)
			}
			// the stage is the 5th parked descriptor
			parked := fmt.Sprintf("/proc/self/fd/%d", 101+4)
			if a[0] != parked {
				t.Errorf("exec path %v, want %s", a[0], parked)
			}
		})
	}
	t.Run("lane execveat", func(t *testing.T) {
		f := newFake(t)
		wantStep(t, h10(p(ModeLane), 50, 51, nil), "H10 execveat", fdlayout.ExitH10)
		if f.called("exec") != 0 {
			t.Error("exec in lane mode: there is no /proc")
		}
		if got := f.args["execveat"]; got[0] != 105 || got[1] != 3 || got[2] != 1 {
			t.Errorf("execveat %v (stage fd, argv+NULL, envp+NULL)", got)
		}
	})
	t.Run("lane argv with a NUL", func(t *testing.T) {
		f := newFake(t)
		pl := p(ModeLane)
		pl.StageArgv = []string{"a\x00b"}
		wantStep(t, h10(pl, 50, 51, nil), "H10 argv", fdlayout.ExitH10)
		if f.called("execveat") != 0 {
			t.Error("execveat with a bad argv")
		}
	})
	t.Run("layout", func(t *testing.T) {
		f := newFake(t)
		h10(p(ModeService), 50, 51, nil)
		wantFcntl := []any{
			uintptr(fdlayout.StatusW), unix.F_DUPFD_CLOEXEC, 64, uintptr(fdlayout.ReadyFD), unix.F_DUPFD_CLOEXEC, 64,
			uintptr(50), unix.F_DUPFD_CLOEXEC, 64, uintptr(51), unix.F_DUPFD_CLOEXEC, 64,
			uintptr(fdlayout.StageFD), unix.F_DUPFD_CLOEXEC, 64}
		// frameFD is the third argument and cwd the fourth of h10(p, cwd, frameFD)
		wantFcntl[6], wantFcntl[9] = uintptr(51), uintptr(50)
		if got := f.args["fcntl"]; !reflect.DeepEqual(got, wantFcntl) {
			t.Errorf("parking %v, want %v", got, wantFcntl)
		}
		wantDup := []any{101, fdlayout.StageStatus, 0, 102, fdlayout.StageReady, 0, 103, fdlayout.StageFrame, 0, 104, fdlayout.StageCwd, 0}
		if got := f.args["dup3"]; !reflect.DeepEqual(got, wantDup) {
			t.Errorf("dup3 %v, want %v", got, wantDup)
		}
		if got := f.args["close_range"]; !reflect.DeepEqual(got, []any{uintptr(fdlayout.StageFirstFree), closeRangeLast, uintptr(unix.CLOSE_RANGE_CLOEXEC)}) {
			t.Errorf("close_range %v", got)
		}
	})
}

func TestRunSequenceAndRefusals(t *testing.T) {
	modes := []Mode{ModeService, ModeLane, ModeSignal}
	for _, m := range modes {
		t.Run("sequence "+string(m), func(t *testing.T) {
			f := newFake(t)
			pl := credPlan()
			pl.Mode = m
			if m == ModeSignal {
				pl.Creds, pl.CredDir = nil, "" // a signal-mode plan carries none
			}
			f.srcs["/s/a"], f.srcs["/s/b"] = []byte("x"), []byte("y")
			f.plan, _ = pl.Marshal()
			err := safeRun(t)
			wantStep(t, err, map[Mode]string{ModeService: "H10 exec", ModeLane: "H10 execveat", ModeSignal: "H10 exec"}[m], fdlayout.ExitH10)
			steps := map[string]int{"H3": f.called("write_file"), "H5": f.called("mount"), "H7": f.called("move_mount"), "H9": f.called("memfd")}
			if steps["H3"] != 1 || steps["H9"] != 1 {
				t.Errorf("steps %v", steps)
			}
			wantSkipped := m == ModeSignal
			if (steps["H5"] == 0) != wantSkipped {
				t.Errorf("mode %s: H5 mount calls %d", m, steps["H5"])
			}
			wantMoves := map[Mode]int{ModeService: 1, ModeLane: 2, ModeSignal: 0}[m]
			if steps["H7"] != wantMoves {
				t.Errorf("mode %s: %d move_mount calls, want %d (H7 and, in lane mode, H8)", m, steps["H7"], wantMoves)
			}
			if f.called("read_file") != 1+btoi(m != ModeSignal) { // H3's read back, H4's suid_dumpable
				t.Errorf("mode %s: read_file %d", m, f.called("read_file"))
			}
		})
	}
	// Each step's refusal surfaces from run() with its own exit code.
	for _, c := range []struct {
		name string
		mode Mode
		set  func(f *fake)
		code int
	}{
		{"H1", ModeService, func(f *fake) { f.failAt["close_range"] = 1 }, fdlayout.ExitH1},
		{"H2", ModeService, func(f *fake) { f.failAt["fcntl_seals"] = 1 }, fdlayout.ExitH2},
		{"H3", ModeService, func(f *fake) { f.failAt["write_file"] = 1 }, fdlayout.ExitH3},
		{"H4", ModeService, func(f *fake) { f.files[SuidDumpablePath] = []byte("1") }, fdlayout.ExitH4},
		{"H5", ModeService, func(f *fake) { f.failAt["mount"] = 1 }, fdlayout.ExitH5},
		{"H7", ModeService, func(f *fake) { f.failAt["openat2"] = 1 }, fdlayout.ExitH7},
		{"H8", ModeLane, func(f *fake) { f.failAt["chdir"] = 1 }, fdlayout.ExitH8},
		{"H9", ModeService, func(f *fake) { f.failAt["openat2"] = 4 }, fdlayout.ExitH9}, // after H7's three
		{"H10", ModeService, func(f *fake) {}, fdlayout.ExitH10},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t)
			pl := credPlan()
			pl.Mode = c.mode
			f.srcs["/s/a"], f.srcs["/s/b"] = []byte("x"), []byte("y")
			f.plan, _ = pl.Marshal()
			c.set(f)
			err := safeRun(t)
			se, ok := err.(*stepError)
			if !ok || se.code != c.code || !strings.HasPrefix(se.step, c.name) {
				t.Errorf("got %v, want a %s refusal with exit %d", err, c.name, c.code)
			}
		})
	}
	t.Run("H6", func(t *testing.T) {
		f := newFake(t)
		pl := credPlan()
		pl.Regions = []Region{{Path: "/srv/r", ReadOnly: true}}
		f.srcs["/s/a"], f.srcs["/s/b"] = []byte("x"), []byte("y")
		f.plan, _ = pl.Marshal()
		f.failAt["open_tree"] = 1
		wantStep(t, safeRun(t), "H6 region 0", fdlayout.ExitH6)
		if f.called("move_mount") != 0 || f.called("memfd") != 0 {
			t.Error("H7 ran after a refused H6")
		}
		f.noLeak(t)
	})
	t.Run("signal mode skips H4, H5, H7 and never reads the credentials", func(t *testing.T) {
		f := newFake(t)
		pl := testPlan(ModeSignal)
		pl.Regions = []Region{{Path: "/srv/r", ReadOnly: true}} // H6 is skipped too
		var err error
		if f.plan, err = pl.Marshal(); err != nil {
			t.Fatal(err)
		}
		f.files[SuidDumpablePath] = []byte("1")
		f.failAt["mount"] = 1
		wantStep(t, safeRun(t), "H10 exec", fdlayout.ExitH10)
	})
}

// safeRun is run() with a panic turned into a failed assertion: a step that
// ignores a refusal must be reported as that, not as a crash of the binary.
func safeRun(t *testing.T) (err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("run() panicked: a refusal was ignored and the next step used its zero value: %v", r)
			err = errors.New("panicked")
		}
	}()
	return run()
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestRunWritesPermAndLogs(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		code int
		log  string
	}{
		{"a step refusal", fail("H4", fdlayout.ExitH4, errors.New("why")), fdlayout.ExitH4, "svd helper: H4: why\n"},
		{"a foreign error", errors.New("odd"), fdlayout.ExitH10, "svd helper: helper: odd\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(t)
			old := runSteps
			runSteps = func() error { return c.err }
			defer func() { runSteps = old }()
			var out bytes.Buffer
			if code := Run(&out); code != c.code {
				t.Errorf("exit %d, want %d", code, c.code)
			}
			if out.String() != c.log {
				t.Errorf("log %q, want %q", out.String(), c.log)
			}
			if got := f.args["write"]; !reflect.DeepEqual(got, []any{fdlayout.StatusW, string(rune(fdlayout.StatusPerm))}) {
				t.Errorf("status pipe write %v: want one perm byte on fd %d", got, fdlayout.StatusW)
			}
		})
	}
}
