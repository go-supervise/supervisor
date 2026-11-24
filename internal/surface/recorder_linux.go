//go:build linux

package surface

import (
	"encoding/binary"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ptraceGetSyscallInfo is PTRACE_GET_SYSCALL_INFO; its struct is read by hand
// (struct ptrace_syscall_info: op, arch, ip, sp, then the entry arguments).
const (
	ptraceGetSyscallInfo = 0x420e
	infoEntry            = 1
	infoExit             = 2
	infoSize             = 88
	ptraceEventStop      = 128
	maxPath              = 512
)

// tracee is one traced thread; last is the last call it entered. A refused
// call still gets its exit stop (with an error) before the fatal SIGSYS, so
// "the call in progress" cannot name it: the last entered call does.
type tracee struct{ last *Call }

// Record starts cmd under ptrace and records, losslessly, every system call
// every thread and every descendant enters, from the skipExecs-th execve on
// (the launcher's own execve of svd-monitor is the first; 0 records from the
// start). Each tracee stops at every system-call entry until the recorder
// resumes it, so nothing can be dropped. The calling goroutine must not be
// shared: ptrace requests come from one OS thread, which Record locks.
//
// started, when not nil, is called once the process exists (the caller closes
// its copies of the descriptors the child owns).
func Record(cmd *exec.Cmd, arch string, skipExecs int, timeout time.Duration, started func()) (*Recording, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cmd.SysProcAttr = &syscall.SysProcAttr{Ptrace: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if started != nil {
		started()
	}
	pid := cmd.Process.Pid
	names := sysNames(arch)
	out := &Recording{Leader: pid}
	tr := map[int]*tracee{pid: {}}
	var mu sync.Mutex
	var timedOut bool
	timer := time.AfterFunc(timeout, func() {
		mu.Lock()
		defer mu.Unlock()
		timedOut = true
		for tid := range tr {
			_ = unix.Tgkill(pid, tid, unix.SIGKILL)
		}
	})
	defer timer.Stop()

	var ws unix.WaitStatus
	if _, err := unix.Wait4(pid, &ws, unix.WALL, nil); err != nil {
		return nil, err
	}
	if !ws.Stopped() {
		return nil, fmt.Errorf("surface: the traced process did not stop at its exec (%v)", ws)
	}
	opts := unix.PTRACE_O_TRACESYSGOOD | unix.PTRACE_O_TRACECLONE | unix.PTRACE_O_TRACEFORK |
		unix.PTRACE_O_TRACEVFORK | unix.PTRACE_O_TRACEEXEC | unix.PTRACE_O_EXITKILL
	if err := unix.PtraceSetOptions(pid, opts); err != nil {
		return nil, err
	}
	resume := func(tid, sig int) { _ = unix.PtraceSyscall(tid, sig) }
	resume(pid, 0)

	execs := 0
	recording := skipExecs == 0
	for {
		tid, err := unix.Wait4(-1, &ws, unix.WALL|unix.WNOTHREAD, nil)
		if err == unix.EINTR {
			continue
		}
		if err == unix.ECHILD {
			break
		}
		if err != nil {
			return nil, err
		}
		mu.Lock()
		t := tr[tid]
		if t == nil {
			t = &tracee{}
			tr[tid] = t
		}
		mu.Unlock()
		switch {
		case ws.Exited() || ws.Signaled():
			if ws.Signaled() && t.last != nil { // a group kill reports each thread as signaled
				out.Last = append(out.Last, *t.last)
			}
			if tid == pid {
				if ws.Exited() {
					out.Exit = ws.ExitStatus()
				} else {
					out.Signal = int(ws.Signal())
				}
			}
			mu.Lock()
			delete(tr, tid)
			mu.Unlock()
		case ws.Stopped():
			sig := ws.StopSignal()
			switch {
			case sig == syscall.SIGTRAP|0x80:
				if err := step(tid, t, recording, names, out); err != nil && err != unix.ESRCH {
					return nil, err
				}
				resume(tid, 0)
			case sig == syscall.SIGTRAP && ws.TrapCause() > 0:
				switch ws.TrapCause() {
				case unix.PTRACE_EVENT_EXEC:
					execs++
					if execs >= skipExecs {
						recording = true
					}
				case unix.PTRACE_EVENT_FORK, unix.PTRACE_EVENT_VFORK, unix.PTRACE_EVENT_CLONE:
					if msg, err := unix.PtraceGetEventMsg(tid); err == nil {
						mu.Lock()
						if tr[int(msg)] == nil {
							tr[int(msg)] = &tracee{}
						}
						mu.Unlock()
					}
				}
				resume(tid, 0)
			case sig == syscall.SIGSTOP || int(ws>>16) == ptraceEventStop || sig == syscall.SIGTRAP:
				resume(tid, 0) // a new tracee's first stop, or a group stop
			default:
				resume(tid, int(sig)) // a signal for the tracee (Go's SIGURG preemption)
			}
		}
	}
	out.InFlight = culprit(arch, pid, out.Last)
	mu.Lock()
	to := timedOut
	mu.Unlock()
	if to {
		return out, fmt.Errorf("surface: recording timed out after %v", timeout)
	}
	return out, nil
}

// step handles one system-call stop of a tracee.
func step(tid int, t *tracee, recording bool, names map[int64]string, out *Recording) error {
	var info [infoSize]byte
	_, _, e := unix.Syscall6(unix.SYS_PTRACE, ptraceGetSyscallInfo, uintptr(tid), uintptr(len(info)), uintptr(unsafe.Pointer(&info[0])), 0, 0)
	if e != 0 {
		return e
	}
	if info[0] == infoEntry {
		if !recording {
			return nil
		}
		c := Call{Tid: tid, Nr: int64(binary.LittleEndian.Uint64(info[24:]))}
		for i := range c.Args {
			c.Args[i] = binary.LittleEndian.Uint64(info[32+8*i:])
		}
		switch names[c.Nr] {
		case "openat":
			if uint32(c.Args[0]) == atFdcwd32 {
				c.Path = readString(tid, uintptr(c.Args[1]))
			}
		case "open":
			c.Path = readString(tid, uintptr(c.Args[0]))
		}
		out.Calls = append(out.Calls, c)
		t.last = &c
	}
	return nil
}

// readString reads a NUL-terminated string of the tracee, a word at a time.
func readString(tid int, addr uintptr) string {
	var s []byte
	var w [8]byte
	for len(s) < maxPath {
		if _, err := unix.PtracePeekData(tid, addr+uintptr(len(s)), w[:]); err != nil {
			return string(s)
		}
		for _, b := range w {
			if b == 0 {
				return string(s)
			}
			s = append(s, b)
		}
	}
	return string(s)
}
