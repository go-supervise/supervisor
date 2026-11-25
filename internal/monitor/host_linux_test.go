//go:build linux

package monitor

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/spawn"
)

// liveChild starts a process and holds it as M_p would: by pid and pidfd.
func liveChild(t *testing.T, argv ...string) (*exec.Cmd, *Live) {
	t.Helper()
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		t.Skipf("no %s: %v", argv[0], err)
	}
	fd, err := unix.PidfdOpen(cmd.Process.Pid, 0)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Skipf("pidfd_open: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd, &Live{Child: &spawn.Child{PID: cmd.Process.Pid, PidFD: fd, CgroupFD: -1, LogR: -1, StatusR: -1, Notify: -1}}
}

func testHost(t *testing.T) realHost {
	t.Helper()
	ep, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.Close(ep) })
	return realHost{f: Fds{Trees: -1, Notify: -1}, ep: ep}
}

// A reap acknowledgement for a child that is still running must not stall M_p
// for good: it waits a bounded time, answers ErrChildRunning and releases
// nothing, so the same acknowledgement succeeds once the child has exited.
func TestReapOfALiveChildIsBoundedAndKeepsIt(t *testing.T) {
	cmd, l := liveChild(t, "sleep", "60")
	h := testHost(t)
	start := time.Now()
	if err := h.Reap(l); !errors.Is(err, ErrChildRunning) {
		t.Fatalf("a running child: %v", err)
	}
	if d := time.Since(start); d > 10*reapWaitMs*time.Millisecond {
		t.Fatalf("reap waited %v", d)
	}
	if _, err := unix.FcntlInt(uintptr(l.Child.PidFD), unix.F_GETFD, 0); err != nil {
		t.Fatalf("the pidfd of a live child was released: %v", err)
	}
	_ = cmd.Process.Kill()
	if err := h.Reap(l); err != nil {
		t.Fatalf("the child has exited: %v", err)
	}
	if _, err := unix.FcntlInt(uintptr(l.Child.PidFD), unix.F_GETFD, 0); err == nil {
		t.Fatal("the pidfd of a reaped child is still open")
	}
}

// The subreaper collects the children it inherits: an orphan of a service's
// tree is reaped, not left a zombie, and a live child it collects is marked so
// that its acknowledgement still succeeds.
func TestSweepReapsOrphansAndMarksALiveChildGone(t *testing.T) {
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Skipf("subreaper: %v", err)
	}
	out, err := exec.Command("sh", "-c", "sleep 0.2 & echo $!").Output()
	if err != nil {
		t.Skipf("sh: %v", err)
	}
	orphan, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	_, l := liveChild(t, "sleep", "0.1")
	live := map[uint32]*Live{0: l}
	h := testHost(t)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		h.Sweep(live)
		if unix.Kill(orphan, 0) == unix.ESRCH && l.Gone {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := unix.Kill(orphan, 0); err != unix.ESRCH {
		t.Fatalf("the orphan %d was not reaped: %v", orphan, err)
	}
	if !l.Gone {
		t.Fatal("a live child the sweep collected is not marked gone")
	}
	if err := h.Reap(l); err != nil {
		t.Fatalf("the acknowledgement of a collected child: %v", err)
	}
}
