//go:build linux

package monitor

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/spawn"
)

// bounded runs f and fails the test when it has not returned in a few seconds:
// a loop that no longer ends is a failure with a name, not a suite that hangs.
func bounded(t *testing.T, f func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	select {
	case p := <-done:
		if p != nil { // a crash is a failure that says so, not luck
			t.Fatalf("the code under test crashed: %v", p)
		}
	case <-time.After(5 * time.Second):
		// the stuck goroutine cannot be stopped and would starve the rest of the
		// suite: the failure is reported and the run ends here
		fmt.Fprintf(os.Stderr, "--- FAIL: %s (did not return in 5s)\n", t.Name())
		os.Exit(1)
	}
}

// runBounded is Run, failing instead of hanging.
func runBounded(t *testing.T, f Fds, h Host) int {
	t.Helper()
	var got int
	bounded(t, func() { got = Run(f, h) })
	return got
}

// A reaped child leaves nothing to wait for: exited says so with the error, and
// a running child is not done.
func TestExitedReportsTheStateOfTheChild(t *testing.T) {
	cmd, l := liveChild(t, "sleep", "60")
	if done, err := exited(l.Child.PidFD); done || err != nil {
		t.Fatalf("a running child: done=%v err=%v", done, err)
	}
	_ = cmd.Process.Kill()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if done, _ := exited(l.Child.PidFD); done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// collected by the first call that saw it: a second finds nothing to wait for
	if done, err := exited(l.Child.PidFD); !done || err == nil {
		t.Fatalf("a collected child: done=%v err=%v", done, err)
	}
}

// A descriptor epoll cannot watch ends the wait at once instead of idling for
// the whole bound, and so does an epoll instance that was never made.
func TestAwaitDoesNotWaitOnWhatItCannotWatch(t *testing.T) {
	h := testHost(t)
	for name, host := range map[string]realHost{"bad fd": h, "no epoll": {f: h.f, ep: -1}} {
		_, l := liveChild(t, "sleep", "60")
		fd := l.Child.PidFD
		if name == "bad fd" {
			fd = -1
		}
		start := time.Now()
		host.await(fd)
		if d := time.Since(start); d >= reapWaitMs*time.Millisecond/2 {
			t.Fatalf("%s: await idled %v", name, d)
		}
	}
}

func TestSweepCollectsUntilNothingIsLeft(t *testing.T) {
	cases := []struct {
		name  string
		steps []struct {
			pid int
			err error
		}
		calls int
		gone  bool
	}{
		{"nothing", []struct {
			pid int
			err error
		}{{0, nil}}, 1, false},
		{"no children", []struct {
			pid int
			err error
		}{{-1, unix.ECHILD}}, 1, false},
		{"interrupted, then a held child, then nothing", []struct {
			pid int
			err error
		}{{-1, unix.EINTR}, {5, nil}, {0, nil}}, 3, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := &Live{Child: &spawn.Child{PID: 5}}
			calls := 0
			bounded(t, func() {
				sweep(map[uint32]*Live{0: l}, func() (int, error) {
					if calls >= len(c.steps) {
						t.Error("the sweep went on past its last answer")
						panic("overrun")
					}
					s := c.steps[calls]
					calls++
					return s.pid, s.err
				})
			})
			if calls != c.calls || l.Gone != c.gone {
				t.Fatalf("calls=%d gone=%v, want %d and %v", calls, l.Gone, c.calls, c.gone)
			}
		})
	}
	t.Run("bounded", func(t *testing.T) {
		calls := 0
		sweep(map[uint32]*Live{}, func() (int, error) { calls++; return 9, nil })
		if calls != maxSweep {
			t.Fatalf("%d calls, want the bound %d", calls, maxSweep)
		}
	})
}

// each failing setup call is reported by name, and ends M_p at once
func TestRunReportsEachFailingSetupCall(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name   string
		break_ func()
		want   string
	}{
		{"subreaper", func() { prctl = func(int, uintptr, uintptr, uintptr, uintptr) error { return boom } }, "E subreaper\n"},
		{"epoll create", func() {
			epollCreate = func(int) (int, error) { return -1, boom }
			epollCtl = func(int, int, int, *unix.EpollEvent) error { panic("epoll_ctl on an instance never made") }
		}, "E epoll\n"},
		{"epoll ctl", func() { epollCtl = func(int, int, int, *unix.EpollEvent) error { return boom } }, "E epoll\n"},
		{"epoll wait", func() { epollWait = func(int, []unix.EpollEvent, int) (int, error) { return -1, boom } }, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, cr, ct, w := prctl, epollCreate, epollCtl, epollWait
			t.Cleanup(func() { prctl, epollCreate, epollCtl, epollWait = p, cr, ct, w })
			c.break_()
			r := newRig(t, baseConfig().Marshal())
			if got := runBounded(t, r.fds, &fakeHost{}); got != ExitSetup {
				t.Fatalf("exit %d", got)
			}
			if got := r.statusText(); got != c.want {
				t.Fatalf("status %q, want %q", got, c.want)
			}
		})
	}
}

// an interrupted wait is retried, not an error
func TestRunRetriesAnInterruptedWait(t *testing.T) {
	w := epollWait
	t.Cleanup(func() { epollWait = w })
	n := 0
	epollWait = func(ep int, evs []unix.EpollEvent, ms int) (int, error) {
		if n++; n == 1 {
			return -1, unix.EINTR
		}
		return w(ep, evs, ms)
	}
	r := newRig(t, baseConfig().Marshal())
	r.hangUp()
	if got := runBounded(t, r.fds, &fakeHost{}); got != ExitOK {
		t.Fatalf("exit %d", got)
	}
	if n < 2 {
		t.Fatalf("the wait was called %d time(s)", n)
	}
}
