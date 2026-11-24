package surface

import (
	"go/version"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func nr(t *testing.T, arch, name string) int64 {
	t.Helper()
	n, ok := SysNum(arch)[name]
	if !ok {
		t.Fatalf("no number for %s on %s", name, arch)
	}
	return n
}

func TestRuntimePath(t *testing.T) {
	for p, want := range map[string]bool{
		hpagePath:                              true,
		"/proc/self/cgroup":                    true,
		"/proc/self/mountinfo":                 true,
		"/sys/fs/cgroup/a/b/cpu.max":           true,
		"/sys/fs/cgroup/cpu.max":               true,
		"/sys/fs/cgroup/a/memory.max":          false,
		"/sys/fs/cgrou/a/cpu.max":              false,
		"/etc/passwd":                          false,
		"":                                     false,
		"/proc/self/environ":                   false,
		"/proc/self/cgroups":                   false,
		"/sys/fs/cgroup/":                      false,
		"/x/sys/fs/cgroup/a/cpu.max":           false,
		"/sys/fs/cgroup/a/cpu.max.extra":       false,
		"/sys/kernel/mm/transparent_hugepage/": false,
	} {
		if got := runtimePath(p); got != want {
			t.Errorf("runtimePath(%q) = %v, want %v", p, got, want)
		}
	}
}

// A SIGSYS in any tracee fails the gate: the fork child inherits the filter,
// and its death is invisible in the leader's status (its parent reads an empty
// error pipe as success).
func TestJudgeFailsOnASigsysInADescendant(t *testing.T) {
	const arch = "arm64"
	const leader = 100
	kill := Call{Tid: 201, Nr: nr(t, arch, "kill")}
	clean := &Recording{Leader: leader}
	if got := Judge(arch, clean, 0); len(got) != 0 {
		t.Fatalf("a clean recording failed: %v", got)
	}
	cases := []struct {
		name string
		d    Death
		want string
	}{
		{"fork child with its call", Death{Tid: 201, Group: 201, Call: kill, HasCall: true},
			`dynamic: a descendant process (pid 201) was killed by SIGSYS at system call "kill"`},
		{"fork child without a call", Death{Tid: 201, Group: 201},
			"dynamic: a descendant process (pid 201) was killed by SIGSYS"},
		{"a process whose group is unknown", Death{Tid: 202},
			"dynamic: a descendant process (pid 0) was killed by SIGSYS"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Judge(arch, &Recording{Leader: leader, Sigsys: []Death{c.d}}, 0)
			if len(got) != 1 || !strings.HasPrefix(got[0], c.want) {
				t.Fatalf("got %v, want one failure starting %q", got, c.want)
			}
		})
	}
	// a thread of the leader's own process is judged by the leader's status
	own := &Recording{Leader: leader, Signal: sigSYS, Sigsys: []Death{{Tid: 103, Group: leader}}}
	if got := Judge(arch, own, 0); len(got) != 1 || !strings.HasPrefix(got[0], "dynamic: svd-monitor killed by SIGSYS") {
		t.Fatalf("a thread of the leader's process was judged twice: %v", got)
	}
	// the same death with the leader alive is a process booked in the leader's
	// group (a clone with another exit signal), and is reported
	alive := &Recording{Leader: leader, Sigsys: []Death{{Tid: 103, Group: leader, Call: kill, HasCall: true}}}
	if got := Judge(arch, alive, 0); len(got) != 1 || !strings.HasPrefix(got[0], "dynamic: a descendant process (pid 100) was killed by SIGSYS at system call") {
		t.Fatalf("a death in the leader's group with the leader alive was dropped: %v", got)
	}
}

func TestCulprit(t *testing.T) {
	const arch = "arm64"
	const leader = 100
	futex := Call{Tid: 1, Nr: nr(t, arch, "futex")}
	kill := Call{Tid: 2, Nr: nr(t, arch, "kill")}
	tgOwn := Call{Tid: 3, Nr: nr(t, arch, "tgkill"), Args: [6]uint64{leader}}
	tgOther := Call{Tid: 4, Nr: nr(t, arch, "tgkill"), Args: [6]uint64{leader + 1}}
	// a high bit in the argument must not make another tgid look like the leader's
	tgHigh := Call{Tid: 5, Nr: nr(t, arch, "tgkill"), Args: [6]uint64{1<<32 | leader}}
	unknown := Call{Tid: 6, Nr: 99999}
	for _, c := range []struct {
		name string
		last []Call
		want *Call
	}{
		{"none", nil, nil},
		{"a refused call after an allowed one", []Call{futex, kill}, &kill},
		{"a refused call before an allowed one", []Call{kill, futex}, &kill},
		{"all allowed answers the first", []Call{futex, tgOwn}, &futex},
		{"a tgkill of another process", []Call{futex, tgOther}, &tgOther},
		{"a tgkill of the leader is allowed", []Call{tgOwn, futex}, &tgOwn},
		{"a tgid is compared on 32 bits", []Call{futex, tgHigh}, &futex},
		{"a number with no name is refused", []Call{futex, unknown}, &unknown},
	} {
		var got *Call
		noPanic(t, c.name, func() { got = culprit(arch, leader, c.last) })
		switch {
		case got == nil && c.want == nil:
		case got == nil || c.want == nil || got.Tid != c.want.Tid:
			t.Errorf("%s: culprit = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestRecordingName(t *testing.T) {
	r := &Recording{}
	if got := r.name("arm64", nr(t, "arm64", "kill")); got != "kill" {
		t.Errorf("name = %q", got)
	}
	if got := r.name("arm64", 99999); got != "syscall#99999" {
		t.Errorf("name = %q", got)
	}
}

func TestJudgeHowTheProcessEnded(t *testing.T) {
	const arch = "arm64"
	kill := &Call{Nr: nr(t, arch, "kill")}
	for _, c := range []struct {
		name string
		r    *Recording
		want int
		out  []string
	}{
		{"clean", &Recording{}, 0, nil},
		{"wanted status", &Recording{Exit: 3}, 3, nil},
		{"wrong status", &Recording{Exit: 1}, 0, []string{"dynamic: svd-monitor exited with status 1, want 0"}},
		{"sigsys named", &Recording{Signal: sigSYS, InFlight: kill}, 0,
			[]string{`dynamic: svd-monitor killed by SIGSYS at system call "kill" (` + itoa(kill.Nr) + `)`}},
		{"sigsys unnamed", &Recording{Signal: sigSYS}, 0, []string{"dynamic: svd-monitor killed by SIGSYS"}},
		{"other signal", &Recording{Signal: 9}, 0, []string{"dynamic: svd-monitor killed by signal 9"}},
	} {
		if got := Judge(arch, c.r, c.want); !reflect.DeepEqual(got, c.out) {
			t.Errorf("%s: %q, want %q", c.name, got, c.out)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestJudgeCallChecks(t *testing.T) {
	const arch = "arm64"
	// The committed R_go holds the start-up calls (the Go runtime issues them
	// too), so pin one that does not.
	saved := rgoARM64
	rgoARM64 = "futex\n"
	t.Cleanup(func() { rgoARM64 = saved })
	call := func(name string, args ...uint64) Call {
		c := Call{Nr: nr(t, arch, name)}
		copy(c.Args[:], args)
		return c
	}
	withPath := func(name, path string) Call { c := call(name); c.Path = path; return c }
	for _, c := range []struct {
		name  string
		calls []Call
		out   []string
	}{
		{"start-up calls before the first recvmsg", []Call{call("prctl", prSetChildSubreaper), call("epoll_create1"), call("prlimit64"), call("recvmsg")}, nil},
		{"start-up call after it", []Call{call("recvmsg"), call("epoll_create1")},
			[]string{"dynamic: epoll_create1 after the first recvmsg (a start-up call)"}},
		{"a start-up call also in the steady list stays allowed", []Call{call("recvmsg"), call("read")}, nil},
		{"reported once", []Call{call("recvmsg"), call("epoll_create1"), call("epoll_create1")},
			[]string{"dynamic: epoll_create1 after the first recvmsg (a start-up call)"}},
		{"tgkill of its own tgid", []Call{call("tgkill", 77)}, nil},
		{"tgkill of another", []Call{call("tgkill", 78)},
			[]string{"dynamic: tgkill with tgid 78, not svd-monitor's own 77"}},
		{"tgid compared on 32 bits", []Call{call("tgkill", 1<<32|77)}, nil},
		{"runtime path", []Call{withPath("openat", hpagePath)}, nil},
		{"open path", nil, []string{`dynamic: open(AT_FDCWD) of "/etc/passwd"`}},
		{"openat path", []Call{withPath("openat", "/etc/passwd")}, []string{`dynamic: openat(AT_FDCWD) of "/etc/passwd"`}},
		{"openat without a path", []Call{call("openat")}, nil},
	} {
		r := &Recording{Leader: 77, Calls: c.calls}
		if c.name == "open path" {
			// open has no number on arm64: judge it on amd64
			r.Calls = []Call{{Nr: nr(t, "amd64", "open"), Path: "/etc/passwd"}}
			if got := Judge("amd64", r, 0); !reflect.DeepEqual(got, c.out) {
				t.Errorf("%s: %q, want %q", c.name, got, c.out)
			}
			continue
		}
		if got := Judge(arch, r, 0); !reflect.DeepEqual(got, c.out) {
			t.Errorf("%s: %q, want %q", c.name, got, c.out)
		}
	}
}

func TestJudgeSubreaperIsStartUpOnly(t *testing.T) {
	const arch = "arm64" // R_go holds prctl, so only its argument tells
	call := func(name string, args ...uint64) Call {
		c := Call{Nr: nr(t, arch, name)}
		copy(c.Args[:], args)
		return c
	}
	r := &Recording{Calls: []Call{call("prctl", prSetChildSubreaper), call("recvmsg"), call("prctl", 15)}}
	if got := Judge(arch, r, 0); got != nil {
		t.Errorf("allowed prctl: %q", got)
	}
	r = &Recording{Calls: []Call{call("recvmsg"), call("prctl", prSetChildSubreaper)}}
	want := []string{"dynamic: prctl(PR_SET_CHILD_SUBREAPER) after the first recvmsg (a start-up call)"}
	if got := Judge(arch, r, 0); !reflect.DeepEqual(got, want) {
		t.Errorf("%q, want %q", got, want)
	}
}

func TestObserved(t *testing.T) {
	got, err := Observed("arm64", []Call{{Nr: nr(t, "arm64", "write")}, {Nr: nr(t, "arm64", "read")}, {Nr: nr(t, "arm64", "write")}})
	if err != nil || !reflect.DeepEqual(got, []string{"read", "write"}) {
		t.Errorf("Observed = %q, %v", got, err)
	}
	if got, err := Observed("arm64", nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("Observed(nil) = %#v, %v", got, err)
	}
	_, err = Observed("arm64", []Call{{Nr: 99999}})
	if err == nil || err.Error() != "surface: system call number 99999 has no name on arm64" {
		t.Errorf("unnamed number: %v", err)
	}
}

func TestGenerateRGo(t *testing.T) {
	const arch = "arm64"
	out, err := GenerateRGo(arch, []Call{{Nr: nr(t, arch, "write")}, {Nr: nr(t, arch, "tgkill")}, {Nr: nr(t, arch, "futex")}})
	if err != nil {
		t.Fatal(err)
	}
	want := "# R_go for arm64: generated by `syscallsurface rgo` from internal/surface/refprog,\n# once per toolchain; do not edit.\n# toolchain: " + version.Lang(runtime.Version()) + "\nfutex\nrestart_syscall\ntgkill\nwrite\n"
	if out != want {
		t.Errorf("GenerateRGo = %q, want %q", out, want)
	}
	// restart_syscall already there is not doubled
	out, err = GenerateRGo(arch, []Call{{Nr: nr(t, arch, "restart_syscall")}})
	if err != nil || strings.Count(out, "restart_syscall") != 1 {
		t.Errorf("GenerateRGo = %q, %v", out, err)
	}
	_, err = GenerateRGo(arch, []Call{{Nr: nr(t, arch, "kill")}})
	if err == nil || err.Error() != "surface: the reference program issued kill, which is never allowed (R_go and Never share only tgkill)" {
		t.Errorf("a Never call: %v", err)
	}
	if _, err = GenerateRGo(arch, []Call{{Nr: 99999}}); err == nil {
		t.Error("an unnamed number was accepted")
	}
}

func TestScenariosAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Scenarios() {
		if s.Name == "" || seen[s.Name] || s.Config == nil || s.WantStatus == "" {
			t.Errorf("scenario %+v is malformed or repeated", s.Name)
		}
		seen[s.Name] = true
		if strings.HasPrefix(s.Name, "fault: ") && (!s.KeepOpen || s.WantExit != exitFault) {
			t.Errorf("%s: a fault must keep L_p open and exit %d", s.Name, exitFault)
		}
	}
	if len(seen) != 14 {
		t.Errorf("%d scenarios, want 14", len(seen))
	}
}
