//go:build linux

package monitor

import (
	"errors"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/spawn"
)

// fakeHost records what M_p asks of the system.
type fakeHost struct {
	starts  []spawn.Config
	reaps   []Live
	failing bool
	pid     int
	running int // reaps that find the child still running
	sweeps  int
}

func (h *fakeHost) Start(c *spawn.Config) (*spawn.Child, error) {
	h.starts = append(h.starts, *c)
	if h.failing {
		return nil, errors.New("no")
	}
	return &spawn.Child{PID: h.pid}, nil
}

func (h *fakeHost) Reap(l *Live) error {
	h.reaps = append(h.reaps, *l)
	if h.running > 0 {
		h.running--
		return ErrChildRunning
	}
	return nil
}

func (h *fakeHost) Sweep(live map[uint32]*Live) { h.sweeps++ }

type rig struct {
	t      *testing.T
	fds    Fds
	peer   int // L_p's end of the record socket
	status int // read end of the status pipe
}

func memfd(t *testing.T, b []byte) int {
	t.Helper()
	fd, err := unix.MemfdCreate("cfg", unix.MFD_CLOEXEC)
	if err != nil {
		t.Skipf("memfd_create: %v", err)
	}
	t.Cleanup(func() { unix.Close(fd) })
	for off := 0; off < len(b); {
		n, err := unix.Write(fd, b[off:])
		if err != nil {
			t.Fatal(err)
		}
		off += n
	}
	if _, err := unix.Seek(fd, 0, 0); err != nil {
		t.Fatal(err)
	}
	return fd
}

func newRig(t *testing.T, cfg []byte) *rig {
	t.Helper()
	sp, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	var p [2]int
	if err := unix.Pipe2(p[:], unix.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.Close(sp[0]); unix.Close(sp[1]); unix.Close(p[0]); unix.Close(p[1]) })
	return &rig{t: t, peer: sp[1], status: p[0],
		fds: Fds{Record: sp[0], Status: p[1], Config: memfd(t, cfg), Trees: 100, Notify: 101, DevNull: 102, Helper: 103, Stage: 104}}
}

func (r *rig) send(b []byte) {
	r.t.Helper()
	if _, err := unix.Write(r.peer, b); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) hangUp() { unix.Close(r.peer) }

func (r *rig) statusText() string {
	r.t.Helper()
	if err := unix.SetNonblock(r.status, true); err != nil {
		r.t.Fatal(err)
	}
	buf := make([]byte, 4096)
	n, err := unix.Read(r.status, buf)
	if err != nil {
		return ""
	}
	return string(buf[:n])
}

func baseConfig() *Config {
	return &Config{
		Generation: 7,
		Services:   []Service{{SignalMask: 1 << 15}, {SignalMask: 0}},
		NotifyName: "nn",
		Limits:     [][2]string{{"pids.max", "64"}},
		Plan:       []byte("plan"),
	}
}

func rc(r *[RecordSize + 1]byte) []byte { return r[:RecordSize] }

func TestRunServesTheWholeVocabulary(t *testing.T) {
	r := newRig(t, baseConfig().Marshal())
	h := &fakeHost{pid: 4242}
	r.send(rc(rec(KindSpawn, 0, 1, 0, 1, 7)))
	r.send(rc(rec(KindSignalMain, 0, 1, 15, 2, 7)))
	r.send(rc(rec(KindReapAck, 0, 1, 0, 3, 7)))
	r.send(rc(rec(KindReapAck, 0, 1, 0, 4, 7)))
	r.send(rc(rec(KindSpawn, 0, 2, 0, 5, 6))) // stale generation
	r.send(rc(rec(KindSpawn, 0, 1, 0, 6, 7))) // stale epoch: the next is 2
	r.send(rc(rec(KindKillTree, 1, 0, 0, 7, 7)))
	r.send(rc(rec(KindSignalTree, 0, 1, 15, 8, 7)))
	r.hangUp()
	if got := runBounded(t, r.fds, h); got != ExitOK {
		t.Fatalf("exit %d", got)
	}
	want := "S 0 4242\nD 2 0\nK 0\nN 0\n" +
		"R " + strconv.Itoa(int(StaleGeneration)) + "\n" +
		"R " + strconv.Itoa(int(BadEpoch)) + "\n" +
		"D 4 1\nD 3 0\n"
	if got := r.statusText(); got != want {
		t.Fatalf("status %q, want %q", got, want)
	}
	if len(h.starts) != 1 || len(h.reaps) != 1 {
		t.Fatalf("starts %d reaps %d", len(h.starts), len(h.reaps))
	}
	s := h.starts[0]
	if s.Epoch != "s0e1" || s.NotifyName != "nns0e1" || string(s.Plan) != "plan" || s.Limits["pids.max"] != "64" {
		t.Fatalf("spawn config %+v", s)
	}
	if s.TreesFD != 100 || s.NotifyDirFD != 101 || s.DevNullFD != 102 || s.HelperFD != 103 || s.StageFD != 104 {
		t.Fatalf("spawn fds %+v", s)
	}
	if l := h.reaps[0]; l.Epoch != "s0e1" || l.Notify != "nns0e1" || l.Child.PID != 4242 {
		t.Fatalf("reaped %+v", l)
	}
}

// A spawn with a newer epoch for a service whose child is still live must be
// refused: it would overwrite the live entry and leak its descriptors and
// cgroup. After the reap acknowledgement the next epoch is served.
func TestRunRefusesASpawnForALiveService(t *testing.T) {
	r := newRig(t, baseConfig().Marshal())
	h := &fakeHost{pid: 4242}
	r.send(rc(rec(KindSpawn, 0, 1, 0, 1, 7)))
	r.send(rc(rec(KindSpawn, 0, 2, 0, 2, 7))) // newer epoch, child still live
	r.send(rc(rec(KindReapAck, 0, 1, 0, 3, 7)))
	r.send(rc(rec(KindSpawn, 0, 2, 0, 4, 7))) // the same epoch again: nothing was consumed
	r.hangUp()
	if got := runBounded(t, r.fds, h); got != ExitOK {
		t.Fatalf("exit %d", got)
	}
	want := "S 0 4242\nR " + strconv.Itoa(int(ServiceLive)) + "\nK 0\nS 0 4242\n"
	if got := r.statusText(); got != want {
		t.Fatalf("status %q, want %q", got, want)
	}
	if len(h.starts) != 2 || len(h.reaps) != 1 {
		t.Fatalf("starts %d reaps %d, want 2 and 1", len(h.starts), len(h.reaps))
	}
}

// An acknowledgement for a child that is still running is refused and the child
// stays held (its service cannot be respawned); a later acknowledgement reaps it.
// M_p sweeps the subreaper's children as it serves.
func TestRunKeepsAChildThatIsStillRunning(t *testing.T) {
	r := newRig(t, baseConfig().Marshal())
	h := &fakeHost{pid: 4242, running: 1}
	r.send(rc(rec(KindSpawn, 0, 1, 0, 1, 7)))
	r.send(rc(rec(KindReapAck, 0, 1, 0, 2, 7))) // still running
	r.send(rc(rec(KindSpawn, 0, 2, 0, 3, 7)))   // still held
	r.send(rc(rec(KindReapAck, 0, 1, 0, 4, 7))) // gone now
	r.hangUp()
	if got := runBounded(t, r.fds, h); got != ExitOK {
		t.Fatalf("exit %d", got)
	}
	want := "S 0 4242\nR " + strconv.Itoa(int(ChildRunning)) + "\nR " + strconv.Itoa(int(ServiceLive)) + "\nK 0\n"
	if got := r.statusText(); got != want {
		t.Fatalf("status %q, want %q", got, want)
	}
	if h.sweeps == 0 {
		t.Fatal("the loop never swept the subreaper's children")
	}
}

func TestRunReportsAFailedSpawn(t *testing.T) {
	r := newRig(t, baseConfig().Marshal())
	h := &fakeHost{failing: true}
	r.send(rc(rec(KindSpawn, 1, 1, 0, 1, 7)))
	r.send(rc(rec(KindReapAck, 1, 1, 0, 2, 7)))
	r.hangUp()
	if got := runBounded(t, r.fds, h); got != ExitOK {
		t.Fatalf("exit %d", got)
	}
	if got := r.statusText(); got != "F 1\nN 1\n" {
		t.Fatalf("status %q", got)
	}
	if len(h.reaps) != 0 {
		t.Fatalf("reaped a child that never started")
	}
}

func TestRunStopsOnAFault(t *testing.T) {
	cases := []struct {
		name string
		send func(r *rig)
		why  Reason
	}{
		{"bad magic", func(r *rig) { b := rec(KindSpawn, 0, 1, 0, 1, 7); b[0] = 0; r.send(rc(b)) }, BadMagic},
		{"bad version", func(r *rig) { b := rec(KindSpawn, 0, 1, 0, 1, 7); b[2] = 9; r.send(rc(b)) }, BadVersion},
		{"bad kind", func(r *rig) { r.send(rc(rec(0, 0, 1, 0, 1, 7))) }, BadKind},
		{"bad flags", func(r *rig) { b := rec(KindSpawn, 0, 1, 0, 1, 7); b[20] = 1; r.send(rc(b)) }, BadFlags},
		{"bad reserved", func(r *rig) { b := rec(KindSpawn, 0, 1, 0, 1, 7); b[63] = 1; r.send(rc(b)) }, BadReserved},
		{"bad seq", func(r *rig) { r.send(rc(rec(KindSpawn, 0, 1, 0, 2, 7))) }, BadSeq},
		{"newer generation", func(r *rig) { r.send(rc(rec(KindSpawn, 0, 1, 0, 1, 8))) }, NewerGeneration},
		{"bad service", func(r *rig) { r.send(rc(rec(KindSpawn, 2, 1, 0, 1, 7))) }, BadService},
		{"bad signal", func(r *rig) { r.send(rc(rec(KindSignalMain, 0, 0, 14, 1, 7))) }, BadSignal},
		{"short record", func(r *rig) { r.send(rc(rec(KindSpawn, 0, 1, 0, 1, 7))[:63]) }, Truncated},
		{"long record", func(r *rig) { r.send(rec(KindSpawn, 0, 1, 0, 1, 7)[:]) }, Truncated},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, baseConfig().Marshal())
			h := &fakeHost{pid: 1}
			c.send(r)
			r.send(rc(rec(KindSpawn, 0, 1, 0, 1, 7))) // never served after the fault
			if got := runBounded(t, r.fds, h); got != ExitFault {
				t.Fatalf("exit %d", got)
			}
			if got, want := r.statusText(), "X "+strconv.Itoa(int(c.why))+"\n"; got != want {
				t.Fatalf("status %q, want %q", got, want)
			}
			if len(h.starts) != 0 {
				t.Fatalf("served a record after a fault")
			}
		})
	}
}

func TestRunRefusesABadConfiguration(t *testing.T) {
	big := make([]byte, maxCfgRead+1)
	for name, cfg := range map[string][]byte{"empty": nil, "garbage": {1, 2, 3}, "oversized": big} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, cfg)
			if got := runBounded(t, r.fds, &fakeHost{}); got != ExitSetup {
				t.Fatalf("exit %d", got)
			}
			if got := r.statusText(); got != "E config\n" {
				t.Fatalf("status %q", got)
			}
		})
	}
}

func TestRunRefusesAnUnreadableConfigFd(t *testing.T) {
	r := newRig(t, nil)
	r.fds.Config = -1
	if got := runBounded(t, r.fds, &fakeHost{}); got != ExitSetup {
		t.Fatalf("exit %d", got)
	}
	if got := r.statusText(); got != "E config\n" {
		t.Fatalf("status %q", got)
	}
}

func TestRunRefusesARecordFdEpollCannotWatch(t *testing.T) {
	r := newRig(t, baseConfig().Marshal())
	r.fds.Record = -1
	if got := runBounded(t, r.fds, &fakeHost{}); got != ExitSetup {
		t.Fatalf("exit %d", got)
	}
	if got := r.statusText(); got != "E epoll\n" {
		t.Fatalf("status %q", got)
	}
}

func TestRunRefusesARecordFdThatIsNotASocket(t *testing.T) {
	r := newRig(t, baseConfig().Marshal())
	var p [2]int
	if err := unix.Pipe2(p[:], unix.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	defer unix.Close(p[0])
	defer unix.Close(p[1])
	if _, err := unix.Write(p[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	r.fds.Record = p[0]
	if got := runBounded(t, r.fds, &fakeHost{}); got != ExitSetup {
		t.Fatalf("exit %d", got)
	}
	if got := r.statusText(); got != "E recv\n" {
		t.Fatalf("status %q", got)
	}
}

func TestReadAllStopsAtTheEndOfTheInput(t *testing.T) {
	fd := memfd(t, []byte("abc"))
	bounded(t, func() {
		if b, err := readAll(fd); err != nil || string(b) != "abc" {
			t.Errorf("readAll = %q, %v", b, err)
		}
	})
	bounded(t, func() {
		if _, err := readAll(-1); err == nil {
			t.Error("a closed descriptor read without error")
		}
	})
	// readConfig answers with the read's own error, not the parser's
	bounded(t, func() {
		if _, err := readConfig(-1); !errors.Is(err, unix.EBADF) {
			t.Errorf("readConfig(-1) = %v, want EBADF", err)
		}
	})
}

func TestReadAllBound(t *testing.T) {
	exact, over := memfd(t, make([]byte, maxCfgRead)), memfd(t, make([]byte, maxCfgRead+1))
	bounded(t, func() {
		if _, err := readAll(exact); err != nil {
			t.Errorf("a configuration of exactly the bound: %v", err)
		}
		if _, err := readAll(over); !errors.Is(err, ErrConfig) {
			t.Errorf("one byte over: %v", err)
		}
	})
}
