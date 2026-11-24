package surface

import (
	"encoding/binary"
	"fmt"
	"go/version"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/go-supervise/supervisor/internal/monitor"
)

// Call is one system call a thread of the recorded process tree entered:
// (thread, number, six arguments), plus the path of an openat(AT_FDCWD, ...) or
// open(...), which the filter cannot read.
type Call struct {
	Tid  int
	Nr   int64
	Args [6]uint64
	Path string
}

// Recording is what the ptrace recorder saw (7.1.2 item 2).
type Recording struct {
	Calls    []Call
	Leader   int    // M_p's tgid, which it keeps through the launcher's execve
	Exit     int    // M_p's exit status, when it exited
	Signal   int    // the signal that killed it, 0 when none
	InFlight *Call  // the call that earned a SIGSYS death, when seen
	Last     []Call // the last call of each thread a signal killed
	Sigsys   []Death
}

// Death is one tracee killed by SIGSYS (a refused call): its thread, its
// process (0 when unknown) and the last call it entered.
type Death struct {
	Tid, Group int
	Call       Call
	HasCall    bool
}

// runtimePath reports whether path is one the Go runtime itself opens at
// start-up by a path relative to the working directory: the huge-page size and
// the cgroup it reads once to size GOMAXPROCS (even with containermaxprocs=0,
// which only stops its use of the answer and the periodic re-read).
func runtimePath(path string) bool {
	switch path {
	case hpagePath, "/proc/self/cgroup", "/proc/self/mountinfo":
		return true
	}
	return strings.HasPrefix(path, "/sys/fs/cgroup/") && strings.HasSuffix(path, "/cpu.max")
}

// culprit picks, among the last calls of the threads a SIGSYS killed, the one
// the filter refused: a call outside the allowed set, or a tgkill of another
// process (leader being M_p's own tgid). Threads blocked in futex die with the
// process, and the kernel reports their deaths in no useful order. With no
// refused call among them it answers the first, and nil when there is none.
func culprit(arch string, leader int, last []Call) *Call {
	allowed, names := Allowed(arch), sysNames(arch)
	for i := range last {
		n, ok := names[last[i].Nr]
		if !ok || !allowed[n] || n == "tgkill" && uint32(last[i].Args[0]) != uint32(leader) {
			return &last[i]
		}
	}
	if len(last) == 0 {
		return nil
	}
	return &last[0]
}

const (
	sigSYS              = 31
	hpagePath           = "/sys/kernel/mm/transparent_hugepage/hpage_pmd_size"
	prSetChildSubreaper = 36
	exitFault           = 3 // monitor.ExitFault (a linux test pins the equality)
)

func (r *Recording) name(arch string, nr int64) string {
	if n, ok := sysNames(arch)[nr]; ok {
		return n
	}
	return "syscall#" + strconv.FormatInt(nr, 10)
}

// Judge turns a recording into failures, in a fixed order: how M_p ended, then
// the call checks of the recorder (what a BPF program cannot check). wantExit
// is the exit status the scenario expects.
func Judge(arch string, r *Recording, wantExit int) []string {
	var out []string
	switch {
	case r.Signal == sigSYS && r.InFlight != nil:
		out = append(out, fmt.Sprintf("dynamic: svd-monitor killed by SIGSYS at system call %q (%d)", r.name(arch, r.InFlight.Nr), r.InFlight.Nr))
	case r.Signal == sigSYS:
		out = append(out, "dynamic: svd-monitor killed by SIGSYS")
	case r.Signal != 0:
		out = append(out, fmt.Sprintf("dynamic: svd-monitor killed by signal %d", r.Signal))
	case r.Exit != wantExit:
		out = append(out, fmt.Sprintf("dynamic: svd-monitor exited with status %d, want %d", r.Exit, wantExit))
	}
	startOnly := Set(AMStart)
	for _, l := range [][]string{AMSteady, RGo(arch)} {
		for _, n := range l {
			delete(startOnly, n)
		}
	}
	reported := map[string]bool{}
	fail := func(s string) {
		if !reported[s] {
			reported[s] = true
			out = append(out, s)
		}
	}
	// a refused call in a descendant (the fork child, which inherits the
	// filter) kills only that process, and its parent sees no failure at all
	for _, d := range r.Sigsys {
		// a thread of the leader's own process dies with the leader, whose
		// status is judged above; with the leader alive, the death is of a
		// process the recorder booked in the leader's group (a clone made with
		// another exit signal and without CLONE_VFORK) and must be reported
		if d.Group == r.Leader && r.Signal == sigSYS {
			continue
		}
		if d.HasCall {
			fail(fmt.Sprintf("dynamic: a descendant process (pid %d) was killed by SIGSYS at system call %q (%d)", d.Group, r.name(arch, d.Call.Nr), d.Call.Nr))
		} else {
			fail(fmt.Sprintf("dynamic: a descendant process (pid %d) was killed by SIGSYS", d.Group))
		}
	}
	steady := false
	for _, c := range r.Calls {
		n := r.name(arch, c.Nr)
		switch {
		case n == "recvmsg":
			steady = true
		case steady && startOnly[n]:
			fail(fmt.Sprintf("dynamic: %s after the first recvmsg (a start-up call)", n))
		case steady && n == "prctl" && c.Args[0] == prSetChildSubreaper:
			fail("dynamic: prctl(PR_SET_CHILD_SUBREAPER) after the first recvmsg (a start-up call)")
		}
		switch n {
		case "tgkill":
			if uint32(c.Args[0]) != uint32(r.Leader) {
				fail(fmt.Sprintf("dynamic: tgkill with tgid %d, not svd-monitor's own %d", uint32(c.Args[0]), r.Leader))
			}
		case "openat", "open":
			if c.Path != "" && !runtimePath(c.Path) {
				fail(fmt.Sprintf("dynamic: %s(AT_FDCWD) of %q", n, c.Path))
			}
		}
	}
	return out
}

// Observed is the set of names the calls entered, sorted; a number with no
// name on arch is an error, never dropped.
func Observed(arch string, calls []Call) ([]string, error) {
	set := map[string]bool{}
	names := sysNames(arch)
	for _, c := range calls {
		n, ok := names[c.Nr]
		if !ok {
			return nil, fmt.Errorf("surface: system call number %d has no name on %s", c.Nr, arch)
		}
		set[n] = true
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

// GenerateRGo renders R_go for arch from the recording of the reference
// program, and refuses it when it admits a Never call other than tgkill
// (7.1.2 item 0).
func GenerateRGo(arch string, calls []Call) (string, error) {
	names, err := Observed(arch, calls)
	if err != nil {
		return "", err
	}
	// restart_syscall is issued by the kernel, never by a program: an
	// interrupted nanosleep or futex wait resumes through it, so a recording of
	// one particular run cannot be relied on to hold it.
	if !Set(names)["restart_syscall"] {
		names = append(names, "restart_syscall")
		sort.Strings(names)
	}
	never := Set(Never)
	for _, n := range names {
		if never[n] && n != "tgkill" {
			return "", fmt.Errorf("surface: the reference program issued %s, which is never allowed (R_go and Never share only tgkill)", n)
		}
	}
	return "# R_go for " + arch + ": generated by `syscallsurface rgo` from internal/surface/refprog,\n# once per toolchain; do not edit.\n" + toolchainPrefix + version.Lang(runtime.Version()) + "\n" + strings.Join(names, "\n") + "\n", nil
}

// Scenario is one fresh run of svd-monitor: its start-up configuration, the
// records the core would send, and what must come back.
type Scenario struct {
	Name       string
	Config     *monitor.Config
	Records    [][]byte
	NoHelper   bool   // helper descriptor closed: the spawn fails in Go's fork child
	KeepOpen   bool   // L_p stays open: M_p ends by itself (a hang-up would race an empty record)
	WantExit   int    // M_p's exit status
	WantStatus string // M_p's status lines, "PID" standing for the child's pid
}

// rec encodes a record of L_p (the layout of monitor.Decode).
func rec(kind uint8, svc uint32, epoch uint64, sig uint32, seq, gen uint64) []byte {
	b := make([]byte, monitor.RecordSize)
	binary.LittleEndian.PutUint16(b[0:], monitor.RecordMagic)
	b[2], b[3] = monitor.RecordVersion, kind
	binary.LittleEndian.PutUint32(b[4:], svc)
	binary.LittleEndian.PutUint64(b[8:], epoch)
	binary.LittleEndian.PutUint32(b[16:], sig)
	binary.LittleEndian.PutUint64(b[24:], seq)
	binary.LittleEndian.PutUint64(b[32:], gen)
	return b
}

func baseConfig() *monitor.Config {
	return &monitor.Config{
		Generation: 7,
		Services:   []monitor.Service{{SignalMask: 1 << 15}, {SignalMask: 0}},
		NotifyName: "nn",
		Limits:     [][2]string{{"cgroup.max.depth", "3"}},
		Plan:       []byte("plan"),
	}
}

func reason(r monitor.Reason) string { return "X " + strconv.Itoa(int(r)) + "\n" }

// Scenarios is the suite M_p is recorded under: every record kind, every
// Refuse path and every Fault path of the decoder, a spawn that fails in the
// fork child (Go's wait4), and the whole spawn and reap path.
func Scenarios() []Scenario {
	const k, g = monitor.KindSpawn, 7
	s := []Scenario{{
		Name:   "vocabulary",
		Config: baseConfig(),
		Records: [][]byte{
			rec(monitor.KindSpawn, 0, 1, 0, 1, g),
			rec(monitor.KindSignalMain, 0, 1, 15, 2, g),
			rec(monitor.KindReapAck, 0, 1, 0, 3, g),
			rec(monitor.KindReapAck, 0, 1, 0, 4, g),
			rec(monitor.KindSpawn, 0, 2, 0, 5, g-1), // stale generation
			rec(monitor.KindSpawn, 0, 1, 0, 6, g),   // stale epoch
			rec(monitor.KindKillTree, 1, 0, 0, 7, g),
			rec(monitor.KindSignalTree, 0, 1, 15, 8, g),
		},
		WantStatus: "S 0 PID\nD 2 0\nK 0\nN 0\nR " + strconv.Itoa(int(monitor.StaleGeneration)) + "\nR " + strconv.Itoa(int(monitor.BadEpoch)) + "\nD 4 1\nD 3 0\n",
	}, {
		Name:       "spawn fails in the fork child",
		Config:     baseConfig(),
		NoHelper:   true,
		Records:    [][]byte{rec(monitor.KindSpawn, 1, 1, 0, 1, g), rec(monitor.KindReapAck, 1, 1, 0, 2, g)},
		WantStatus: "F 1\nN 1\n",
	}}
	mut := func(f func(b []byte)) []byte { b := rec(k, 0, 1, 0, 1, g); f(b); return b }
	faults := []struct {
		name string
		rec  []byte
		why  monitor.Reason
	}{
		{"empty record", []byte{}, monitor.BadLength},
		{"bad magic", mut(func(b []byte) { b[0] = 0 }), monitor.BadMagic},
		{"bad version", mut(func(b []byte) { b[2] = 9 }), monitor.BadVersion},
		{"bad kind", rec(0, 0, 1, 0, 1, g), monitor.BadKind},
		{"bad flags", mut(func(b []byte) { b[20] = 1 }), monitor.BadFlags},
		{"bad reserved", mut(func(b []byte) { b[63] = 1 }), monitor.BadReserved},
		{"bad seq", rec(k, 0, 1, 0, 2, g), monitor.BadSeq},
		{"newer generation", rec(k, 0, 1, 0, 1, g+1), monitor.NewerGeneration},
		{"bad service", rec(k, 2, 1, 0, 1, g), monitor.BadService},
		{"bad signal", rec(monitor.KindSignalMain, 0, 0, 14, 1, g), monitor.BadSignal},
		{"short record", rec(k, 0, 1, 0, 1, g)[:63], monitor.Truncated},
		{"long record", append(rec(k, 0, 1, 0, 1, g), 0), monitor.Truncated},
	}
	for _, f := range faults {
		s = append(s, Scenario{Name: "fault: " + f.name, Config: baseConfig(), Records: [][]byte{f.rec},
			KeepOpen: true, WantExit: exitFault, WantStatus: reason(f.why)})
	}
	return s
}
