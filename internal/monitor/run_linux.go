//go:build linux

package monitor

import (
	"strconv"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/spawn"
)

// Exit codes of M_p.
const (
	ExitOK     = 0
	ExitSetup  = 2 // start-up failed: configuration, prctl, epoll, a receive error
	ExitFault  = 3 // the decoder answered Fault (SUPERVISOR.md 3.3 T8): M_p stops
	maxCfgRead = MaxPlan + 1<<16
)

// Fds are the descriptor numbers the core hands M_p at its start (M1).
type Fds struct{ Record, Status, Config, Trees, Notify, DevNull, Helper, Stage int }

// StdFds is the layout the core uses.
var StdFds = Fds{Record: 3, Trees: 4, Notify: 5, DevNull: 6, Helper: 7, Stage: 8, Config: 9, Status: 10}

// Live is one running spawn M_p holds the parent's ends of.
type Live struct {
	Child  *spawn.Child
	Epoch  string
	Notify string
}

// Host is what M_p asks of the system for a spawn and for its reaping; the
// production host is NewHost, tests substitute their own.
type Host interface {
	Start(c *spawn.Config) (*spawn.Child, error)
	Reap(l *Live) error
}

type realHost struct{ f Fds }

// NewHost is the production Host: spawn.Start, and a waitid on the pidfd.
func NewHost(f Fds) Host { return realHost{f} }

func (realHost) Start(c *spawn.Config) (*spawn.Child, error) { return spawn.Start(c) }

func (h realHost) Reap(l *Live) error {
	var info unix.Siginfo
	var err error = unix.EINTR
	for err == unix.EINTR {
		err = unix.Waitid(unix.P_PIDFD, l.Child.PidFD, &info, unix.WEXITED, nil)
	}
	l.Child.Close()
	_ = unix.Unlinkat(h.f.Trees, l.Epoch, unix.AT_REMOVEDIR)
	_ = unix.Unlinkat(h.f.Notify, l.Notify, 0)
	return err
}

func say(fd int, s string) { _, _ = unix.Write(fd, []byte(s)) }

func readAll(fd int) ([]byte, error) {
	var out []byte
	buf := make([]byte, 4096)
	for len(out) <= maxCfgRead {
		n, err := unix.Read(fd, buf)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return out, nil
		}
		out = append(out, buf[:n]...)
	}
	return nil, ErrConfig
}

// Run is M_p: it reads its configuration, becomes a subreaper, and serves the
// records of L_p on f.Record until the peer hangs up (0) or the decoder says
// Fault (ExitFault). Spawns and reaps go through h; M_p never signals.
func Run(f Fds, h Host) int {
	b, err := readAll(f.Config)
	if err != nil {
		say(f.Status, "E config\n")
		return ExitSetup
	}
	cfg, err := ParseConfig(b)
	if err != nil {
		say(f.Status, "E config\n")
		return ExitSetup
	}
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		say(f.Status, "E subreaper\n")
		return ExitSetup
	}
	ep, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		say(f.Status, "E epoll\n")
		return ExitSetup
	}
	ev := unix.EpollEvent{Events: unix.EPOLLIN | unix.EPOLLRDHUP, Fd: int32(f.Record)}
	if err := unix.EpollCtl(ep, unix.EPOLL_CTL_ADD, f.Record, &ev); err != nil {
		say(f.Status, "E epoll\n")
		return ExitSetup
	}
	limits := map[string]string{}
	for _, l := range cfg.Limits {
		limits[l[0]] = l[1]
	}
	st := &State{Generation: cfg.Generation, Services: cfg.Services}
	live := map[uint32]*Live{}
	var buf [RecordSize + 1]byte
	var evs [1]unix.EpollEvent
	first := true
	for {
		if _, err := unix.EpollWait(ep, evs[:], -1); err != nil {
			if err == unix.EINTR {
				continue
			}
			return ExitSetup
		}
		n, _, flags, _, err := unix.Recvmsg(f.Record, buf[:], nil, unix.MSG_TRUNC)
		if err == unix.EINTR || err == unix.EAGAIN {
			continue
		}
		if err != nil {
			say(f.Status, "E recv\n")
			return ExitSetup
		}
		if n == 0 && evs[0].Events&(unix.EPOLLHUP|unix.EPOLLRDHUP) != 0 {
			return ExitOK
		}
		if first {
			first = false
			canary()
		}
		rec, out, why := Decode(&buf, n, flags&unix.MSG_TRUNC != 0, st)
		if out == Fault {
			say(f.Status, "X "+strconv.Itoa(int(why))+"\n")
			return ExitFault
		}
		st.LastSeq = rec.Seq // Act and Refuse both consume the sequence number
		if out == Refuse {
			say(f.Status, "R "+strconv.Itoa(int(why))+"\n")
			continue
		}
		serve(f, h, cfg, limits, st, live, &rec)
	}
}

func serve(f Fds, h Host, cfg *Config, limits map[string]string, st *State, live map[uint32]*Live, r *Record) {
	svc := strconv.FormatUint(uint64(r.Service), 10)
	switch r.Kind {
	case KindSpawn:
		st.Services[r.Service].LastEpoch = r.Epoch
		epoch := "s" + svc + "e" + strconv.FormatUint(r.Epoch, 10)
		notify := cfg.NotifyName + epoch
		c, err := h.Start(&spawn.Config{
			TreesFD: f.Trees, Epoch: epoch, Limits: limits,
			NotifyDirFD: f.Notify, NotifyName: notify,
			DevNullFD: f.DevNull, HelperFD: f.Helper, StageFD: f.Stage, Plan: cfg.Plan,
		})
		if err != nil {
			say(f.Status, "F "+svc+"\n")
			return
		}
		live[r.Service] = &Live{Child: c, Epoch: epoch, Notify: notify}
		say(f.Status, "S "+svc+" "+strconv.Itoa(c.PID)+"\n")
	case KindReapAck:
		l := live[r.Service]
		if l == nil {
			say(f.Status, "N "+svc+"\n")
			return
		}
		delete(live, r.Service)
		_ = h.Reap(l)
		say(f.Status, "K "+svc+"\n")
	default: // signals and tree kills belong to the signaller, never to M_p (T11)
		say(f.Status, "D "+strconv.Itoa(int(r.Kind))+" "+svc+"\n")
	}
}
