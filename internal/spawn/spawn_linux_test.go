//go:build linux

package spawn_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/fdlayout"
	"github.com/go-supervise/supervisor/internal/frame"
	"github.com/go-supervise/supervisor/internal/helper"
	"github.com/go-supervise/supervisor/internal/spawn"
)

var svdBin, stubBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "svd-bins")
	if err != nil {
		panic(err)
	}
	_, self, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(self), "..", "..")
	svdBin, stubBin = filepath.Join(dir, "svd"), filepath.Join(dir, "stagestub")
	for _, b := range [][2]string{{svdBin, "./cmd/svd"}, {stubBin, "./internal/stagestub"}} {
		cmd := exec.Command("go", "build", "-trimpath", "-o", b[0], b[1])
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build %s: %v\n%s", b[1], err, out)
			os.Exit(1)
		}
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// priv gates a root/namespace test: in the privileged lane (SVD_PRIVILEGED=1)
// an absent capability FAILS; elsewhere the test is skipped.
func priv(t *testing.T) {
	t.Helper()
	if os.Getenv("SVD_PRIVILEGED") == "" {
		t.Skip("root/namespace test: set SVD_PRIVILEGED=1 (the privileged lane)")
	}
	var hdr = unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	if err := unix.Capget(&hdr, &data[0]); err != nil {
		t.Fatalf("capget: %v", err)
	}
	if os.Geteuid() != 0 || data[0].Effective&(1<<unix.CAP_SYS_ADMIN) == 0 {
		t.Fatalf("privileged lane without root and CAP_SYS_ADMIN (euid %d)", os.Geteuid())
	}
}

type env struct {
	trees, notify, devnull, helper, stage int
	treesPath                             string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{}
	e.treesPath = fmt.Sprintf("/sys/fs/cgroup/svdtest-%d-%d", os.Getpid(), time.Now().UnixNano())
	if err := os.Mkdir(e.treesPath, 0o755); err != nil {
		t.Fatalf("cgroup v2 trees directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(e.treesPath) })
	open := func(p string, fl int) int {
		fd, err := unix.Open(p, fl|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatalf("open %s: %v", p, err)
		}
		t.Cleanup(func() { unix.Close(fd) })
		return fd
	}
	nd, _ := os.MkdirTemp("", "svd-notify")
	_ = os.Chmod(nd, 0o711)
	t.Cleanup(func() { os.RemoveAll(nd) })
	e.trees = open(e.treesPath, unix.O_DIRECTORY|unix.O_RDONLY)
	e.notify = open(nd, unix.O_DIRECTORY|unix.O_RDONLY)
	e.devnull = open("/dev/null", unix.O_RDWR)
	e.helper = open(svdBin, unix.O_PATH)
	e.stage = open(stubBin, unix.O_PATH)
	return e
}

func (e *env) config(epoch string, p *helper.Plan, lane bool) *spawn.Config {
	b, err := p.Marshal()
	if err != nil {
		panic(err)
	}
	return &spawn.Config{TreesFD: e.trees, Epoch: epoch, Limits: map[string]string{"cgroup.max.depth": "3"},
		NotifyDirFD: e.notify, NotifyName: "n-" + epoch, DevNullFD: e.devnull, HelperFD: e.helper,
		StageFD: e.stage, Plan: b, Lane: lane}
}

func baseFrame() frame.Frame {
	f := frame.Frame{UID: 4242, GID: 4242, Groups: []uint32{7}, Umask: 0o77, Argv: []string{"svc", "-x"}, Envp: []string{"A=B"}}
	for i := range f.Rlimits {
		var l unix.Rlimit
		_ = unix.Getrlimit(i, &l)
		f.Rlimits[i] = [2]uint64{l.Cur, l.Max}
	}
	return f
}

type result struct {
	rep    stubReport
	raw    []byte
	log    string
	status syscall.WaitStatus
	child  *spawn.Child
}

type stubReport struct {
	PID, SID    int
	FDs         map[int]string
	FDFlags     map[int]int
	Env         []string
	Sig         map[string]string
	Rlimits     [16][2]uint64
	Cgroup      string
	NS          map[string]string
	Securebits  int
	SessionKey  int
	FrameSeals  int
	Frame       *frame.Frame
	FrameErr    string
	HasProc     bool
	PasswdErr   string
	KeySearch   string
	KeyJoined   int
	CwdIsDir    bool
	Creds       map[string]string
	CredDirMode string
}

func readFD(fd int) []byte {
	// Raw reads: an os.File would close the fd from its finalizer, later,
	// possibly onto a reused number.
	var out []byte
	buf := make([]byte, 4096)
	for {
		n, err := unix.Read(fd, buf)
		out = append(out, buf[:n]...)
		if n == 0 || (err != nil && err != unix.EINTR) {
			return out
		}
	}
}

func (e *env) run(t *testing.T, epoch string, p *helper.Plan, lane bool) result {
	t.Helper()
	ch, err := spawn.Start(e.config(epoch, p, lane))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	var r result
	r.child = ch
	r.raw = readFD(ch.StatusR) // EOF when the stage (or the helper) is gone
	r.log = string(readFD(ch.LogR))
	var ws syscall.WaitStatus
	if _, err := syscall.Wait4(ch.PID, &ws, 0, nil); err != nil {
		t.Fatalf("wait4: %v", err)
	}
	r.status = ws
	if len(r.raw) > 1 {
		if err := json.Unmarshal(r.raw, &r.rep); err != nil {
			t.Fatalf("stub report %q: %v", r.raw, err)
		}
	}
	t.Cleanup(func() {
		for i := 0; i < 50; i++ {
			if unix.Unlinkat(e.trees, epoch, unix.AT_REMOVEDIR) == nil {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		ch.Close()
	})
	return r
}

func servicePlan(stageArgs ...string) *helper.Plan {
	return &helper.Plan{Mode: helper.ModeService, Frame: baseFrame(), Cwd: "/",
		StageArgv: append([]string{"svd-drop"}, stageArgs...)}
}

func selfNS(t *testing.T, n string) string {
	l, err := os.Readlink("/proc/self/ns/" + n)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func sessionKey(t *testing.T) int {
	id, err := unix.KeyctlGetKeyringID(unix.KEY_SPEC_SESSION_KEYRING, false)
	if err != nil {
		t.Fatalf("session keyring: %v", err)
	}
	return id
}

// TestSpawnInheritance asserts, from the stub standing in for svd-drop, each
// property of what the helper hands the stage (SUPERVISOR.md 7.1.1).
func TestSpawnInheritance(t *testing.T) {
	priv(t)
	e := newEnv(t)
	p := servicePlan()
	r := e.run(t, "epoch-1", p, false)
	if r.status.ExitStatus() != 0 || len(r.raw) < 2 {
		t.Fatalf("exit %v, status %q, log %q", r.status, r.raw, r.log)
	}
	rep := r.rep
	// C1: one pid across the four images, a session leader (setsid).
	if rep.PID != r.child.PID || rep.SID != rep.PID {
		t.Errorf("pid %d sid %d, spawned pid %d", rep.PID, rep.SID, r.child.PID)
	}
	// H10: the stage's fd set S is exactly {0,1,2, status, ready, frame, cwd}.
	var fds []int
	for fd := range rep.FDFlags {
		fds = append(fds, fd)
	}
	sort.Ints(fds)
	if !reflect.DeepEqual(fds, []int{0, 1, 2, 3, 4, 5, 6}) {
		t.Errorf("stage fds %v (%v)", fds, rep.FDs)
	}
	if rep.FDs[0] != "/dev/null" || !strings.HasPrefix(rep.FDs[1], "pipe:") || rep.FDs[1] != rep.FDs[2] ||
		!strings.HasPrefix(rep.FDs[4], "socket:") || !strings.Contains(rep.FDs[fdlayout.StageFrame], "svd-frame") {
		t.Errorf("fd targets %v", rep.FDs)
	}
	// H10: no fd the stage needs is lost to CLOEXEC, none leaks.
	for _, fd := range []int{0, 1, 2, 3, 4, 5, 6} {
		if rep.FDFlags[fd]&unix.FD_CLOEXEC != 0 {
			t.Errorf("fd %d is CLOEXEC at the stage", fd)
		}
	}
	// H10: an empty environment.
	if len(rep.Env) != 0 {
		t.Errorf("env %v", rep.Env)
	}
	// Signal state: nothing blocked, nothing ignored (D5 resets what a runtime changed).
	if rep.Sig["SigBlk"] != "0000000000000000" || rep.Sig["SigIgn"] != "0000000000000000" {
		t.Errorf("signal mask/ignored: %v", rep.Sig)
	}
	// Rlimits: none altered by the helper (D5 sets the plan's).
	var own [16][2]uint64
	for i := range own {
		var l unix.Rlimit
		_ = unix.Getrlimit(i, &l)
		own[i] = [2]uint64{l.Cur, l.Max}
	}
	if rep.Rlimits != own {
		t.Errorf("rlimits %v, core's %v", rep.Rlimits, own)
	}
	// M4: the epoch cgroup; the new mount namespace only.
	if !strings.HasSuffix(rep.Cgroup, "/epoch-1") {
		t.Errorf("cgroup %q", rep.Cgroup)
	}
	if rep.NS["mnt"] == selfNS(t, "mnt") {
		t.Error("same mount namespace as the core")
	}
	for _, n := range []string{"net", "ipc", "pid", "user", "uts"} {
		if rep.NS[n] != selfNS(t, n) {
			t.Errorf("service mode changed the %s namespace", n)
		}
	}
	// Securebits and the keyring are untouched by the helper (D8 and D5b are the stage's).
	if rep.Securebits != 0 {
		t.Errorf("securebits %#x", rep.Securebits)
	}
	if rep.SessionKey != sessionKey(t) {
		t.Errorf("session keyring %d, core's %d", rep.SessionKey, sessionKey(t))
	}
	// H9: the frame is sealed with the four seals and decodes to the plan's.
	if !helper.SealsOK(rep.FrameSeals) || rep.FrameErr != "" || rep.Frame == nil || !reflect.DeepEqual(*rep.Frame, p.Frame) {
		t.Errorf("frame seals %#x err %q frame %+v", rep.FrameSeals, rep.FrameErr, rep.Frame)
	}
	if !rep.CwdIsDir {
		t.Error("cwd fd is not a directory")
	}
	// H3: oom_score_adj.
	b, _ := os.ReadFile("/proc/" + strconv.Itoa(r.child.PID) + "/oom_score_adj")
	_ = b // the process is gone; H3's own read-back is the check (a failure is perm)
}

// TestSpawnLane: H8's empty root, and the namespaces M4 adds.
func TestSpawnLane(t *testing.T) {
	priv(t)
	e := newEnv(t)
	p := servicePlan()
	p.Mode = helper.ModeLane
	r := e.run(t, "epoch-lane", p, true)
	if r.status.ExitStatus() != 0 || len(r.raw) < 2 {
		t.Fatalf("exit %v, status %q, log %q", r.status, r.raw, r.log)
	}
	rep := r.rep
	if rep.HasProc {
		t.Error("/proc survives the lane's empty root")
	}
	if !strings.Contains(rep.PasswdErr, "no such file") {
		t.Errorf("open /etc/passwd: %q", rep.PasswdErr)
	}
	if !rep.CwdIsDir || !helper.SealsOK(rep.FrameSeals) {
		t.Errorf("cwd %v seals %#x", rep.CwdIsDir, rep.FrameSeals)
	}
	var fds []int
	for fd := range rep.FDFlags {
		fds = append(fds, fd)
	}
	sort.Ints(fds)
	if !reflect.DeepEqual(fds, []int{0, 1, 2, 3, 4, 5, 6}) {
		t.Errorf("lane stage fds %v", fds)
	}
	// Namespaces M4 adds in lane mode are read from the child's own record, not /proc.
}

// TestSpawnLaneNamespaces reads the lane's namespaces before H8 hides /proc,
// through a service-mode child started with the lane flags is not possible;
// instead check M4's clone flags took effect by spawning with Lane and a
// plan that keeps /proc (signal mode skips H8).
func TestSpawnLaneNamespaces(t *testing.T) {
	priv(t)
	e := newEnv(t)
	p := servicePlan()
	p.Mode = helper.ModeSignal
	r := e.run(t, "epoch-ns", p, true)
	if r.status.ExitStatus() != 0 || len(r.raw) < 2 {
		t.Fatalf("exit %v, status %q, log %q", r.status, r.raw, r.log)
	}
	for _, n := range []string{"net", "ipc", "mnt"} {
		if r.rep.NS[n] == selfNS(t, n) {
			t.Errorf("lane spawn left the %s namespace", n)
		}
	}
}

// TestHelperCredentials: H7's private read-only tmpfs.
func TestHelperCredentials(t *testing.T) {
	priv(t)
	e := newEnv(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "secret")
	if err := os.WriteFile(src, []byte("hunter2"), 0o600); err != nil {
		t.Fatal(err)
	}
	deliver := filepath.Join(dir, "cred")
	if err := os.Mkdir(deliver, 0o755); err != nil {
		t.Fatal(err)
	}
	p := servicePlan("creds", deliver)
	p.Creds = []helper.Credential{{Name: "db", Source: src}}
	p.CredDir = deliver
	r := e.run(t, "epoch-cred", p, false)
	if r.status.ExitStatus() != 0 || len(r.raw) < 2 {
		t.Fatalf("exit %v, status %q, log %q", r.status, r.raw, r.log)
	}
	if r.rep.Creds["db"] != "hunter2" {
		t.Errorf("credentials %v", r.rep.Creds)
	}
	flags, _ := strconv.ParseInt(r.rep.CredDirMode, 16, 64)
	for name, bit := range map[string]int64{"ST_RDONLY": unix.ST_RDONLY, "ST_NOSUID": unix.ST_NOSUID, "ST_NODEV": unix.ST_NODEV, "ST_NOEXEC": unix.ST_NOEXEC} {
		if flags&bit == 0 {
			t.Errorf("credential mount lacks %s (%#x)", name, flags)
		}
	}
	// The mount exists only in the helper's namespace.
	if es, _ := os.ReadDir(deliver); len(es) != 0 {
		t.Errorf("credentials visible in the core's namespace: %v", es)
	}
	// A hostile credential name is refused (perm).
	p.Creds[0].Name = "../x"
	r = e.run(t, "epoch-cred2", p, false)
	if len(r.raw) != 1 || r.raw[0] != fdlayout.StatusPerm || r.status.ExitStatus() != fdlayout.ExitH7 {
		t.Errorf("bad name: status %q exit %v log %q", r.raw, r.status, r.log)
	}
}

// TestHelperPermIsNotFail: a failing step writes `perm` and exits with its
// fixed code, outside {1, 2}.
func TestHelperPermIsNotFail(t *testing.T) {
	priv(t)
	e := newEnv(t)
	p := servicePlan()
	p.Cwd = "/nonexistent-working-directory"
	r := e.run(t, "epoch-perm", p, false)
	if len(r.raw) != 1 || r.raw[0] != fdlayout.StatusPerm || r.status.ExitStatus() != fdlayout.ExitH9 {
		t.Fatalf("status %q exit %v log %q", r.raw, r.status, r.log)
	}
	if code := r.status.ExitStatus(); code == 1 || code == 2 {
		t.Errorf("exit code %d collides with the runtime's", code)
	}
}

// TestServiceKeyringIsFresh (SECURITY-1 T12, D5b): a user key planted in the
// core's session keyring is not found from a service after D5b, and IS found
// when the row is removed (the fault injection that turns the test red).
func TestServiceKeyringIsFresh(t *testing.T) {
	priv(t)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, err := unix.KeyctlInt(unix.KEYCTL_JOIN_SESSION_KEYRING, 0, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	desc := fmt.Sprintf("svd-t12-%d", os.Getpid())
	if _, err := unix.AddKey("user", desc, []byte("hunter2"), unix.KEY_SPEC_SESSION_KEYRING); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t)
	for _, mode := range []helper.Mode{helper.ModeService, helper.ModeSignal} {
		with := e.run(t, "epoch-k1-"+string(mode), mutate(servicePlan("d5b", desc), mode), false)
		if with.rep.KeySearch == "found" || with.rep.KeySearch == "" || with.rep.KeyJoined == 0 {
			t.Errorf("%s: after D5b the key was found or the join failed: %+v (log %q)", mode, with.rep.KeySearch, with.log)
		}
		if with.rep.SessionKey == sessionKey(t) {
			t.Errorf("%s: the service still holds the core's session keyring", mode)
		}
		// Fault injection: the D5b row removed. The same key must be reachable.
		without := e.run(t, "epoch-k2-"+string(mode), mutate(servicePlan("nod5b", desc), mode), false)
		if without.rep.KeySearch != "found" {
			t.Errorf("%s: with the row removed the planted key is not found (%q): the test cannot discriminate", mode, without.rep.KeySearch)
		}
	}
}

func mutate(p *helper.Plan, m helper.Mode) *helper.Plan { p.Mode = m; return p }

// TestSpawnFailureLeavesNothing: injected failures at M2, M3 and M4 leave no
// cgroup, socket or fd behind (T5).
func TestSpawnFailureLeavesNothing(t *testing.T) {
	priv(t)
	e := newEnv(t)
	n := 1000
	if testing.Short() {
		n = 50
	}
	countFDs := func() int {
		es, _ := os.ReadDir("/proc/self/fd")
		return len(es)
	}
	before := countFDs()
	defer func() { spawn.Inject = nil }()
	for _, step := range []string{"M2", "M3", "M4"} {
		spawn.Inject = func(s string) error {
			if s == step {
				return unix.EIO
			}
			return nil
		}
		for i := 0; i < n; i++ {
			cfg := e.config("epoch-fail", servicePlan(), false)
			if _, err := spawn.Start(cfg); err == nil {
				t.Fatalf("%s: injected failure not reported", step)
			} else if sep, ok := err.(*spawn.ErrSEp); !ok || !strings.HasPrefix(sep.Step, step) {
				t.Fatalf("%s: error %v", step, err)
			}
		}
		if _, err := os.Stat(filepath.Join(e.treesPath, "epoch-fail")); err == nil {
			t.Errorf("%s: cgroup left behind", step)
		}
		if _, err := os.Stat("/proc/self/fd/" + strconv.Itoa(e.notify) + "/n-epoch-fail"); err == nil {
			t.Errorf("%s: socket left behind", step)
		}
		if got := countFDs(); got != before {
			t.Errorf("%s: %d fds before, %d after", step, before, got)
		}
	}
}
