//go:build linux

// Command stagestub stands in for svd-drop in tests: it records what the
// helper's exec handed it, as JSON on the status fd (3), and exits 0.
//
// argv: "d5b <desc>" joins a fresh session keyring first (the D5b row) and
// then searches for the user key <desc>; "nod5b <desc>" only searches.
package main

import (
	"encoding/json"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/fdlayout"
	"github.com/go-supervise/supervisor/internal/frame"
	"github.com/go-supervise/supervisor/internal/keyring"
)

// Report is what the stub saw.
type Report struct {
	PID, SID    int
	FDs         map[int]string // fd -> readlink target, or "" without /proc
	FDFlags     map[int]int    // fd -> F_GETFD
	Env         []string
	Sig         map[string]string // SigBlk, SigIgn, SigCgt from /proc/self/status
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
	KeySearch   string // "found", or the errno text
	KeyJoined   int
	CwdIsDir    bool
	Creds       map[string]string // credential dir listing: name -> content
	CredDirMode string
}

func init() { runtime.LockOSThread() }

func main() {
	r := Report{FDs: map[int]string{}, FDFlags: map[int]int{}, Sig: map[string]string{}, NS: map[string]string{}}
	r.PID, r.SID = os.Getpid(), 0
	r.SID, _ = unix.Getsid(0)
	r.Env = os.Environ()
	if _, err := os.Stat("/proc/self"); err == nil {
		r.HasProc = true
	}
	for fd := 0; fd < 256; fd++ {
		fl, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if err != nil {
			continue
		}
		r.FDFlags[fd] = fl
		if r.HasProc {
			r.FDs[fd], _ = os.Readlink("/proc/self/fd/" + strconv.Itoa(fd))
		}
	}
	if r.HasProc {
		if b, err := os.ReadFile("/proc/self/status"); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if k, v, ok := strings.Cut(l, ":"); ok && (k == "SigBlk" || k == "SigIgn" || k == "SigCgt") {
					r.Sig[k] = strings.TrimSpace(v)
				}
			}
		}
		b, _ := os.ReadFile("/proc/self/cgroup")
		r.Cgroup = strings.TrimSpace(string(b))
		for _, n := range []string{"mnt", "net", "ipc", "pid", "uts", "user", "cgroup"} {
			r.NS[n], _ = os.Readlink("/proc/self/ns/" + n)
		}
	}
	for i := 0; i < 16; i++ {
		var l unix.Rlimit
		_ = unix.Getrlimit(i, &l)
		r.Rlimits[i] = [2]uint64{l.Cur, l.Max}
	}
	r.Securebits, _ = unix.PrctlRetInt(unix.PR_GET_SECUREBITS, 0, 0, 0, 0)
	r.SessionKey, _ = unix.KeyctlGetKeyringID(unix.KEY_SPEC_SESSION_KEYRING, false)
	if _, err := os.Open("/etc/passwd"); err != nil {
		r.PasswdErr = err.Error()
	}
	var st unix.Stat_t
	r.CwdIsDir = unix.Fstat(fdlayout.StageCwd, &st) == nil && st.Mode&unix.S_IFMT == unix.S_IFDIR
	r.FrameSeals, _ = unix.FcntlInt(fdlayout.StageFrame, unix.F_GET_SEALS, 0)
	if buf, err := readAll(fdlayout.StageFrame); err == nil {
		f, derr := frame.Decode(buf)
		if derr != nil {
			r.FrameErr = derr.Error()
		}
		r.Frame = f
	}
	if len(os.Args) >= 3 {
		if os.Args[1] == "d5b" {
			r.KeyJoined, _ = keyring.JoinFresh()
		}
		if os.Args[1] == "d5b" || os.Args[1] == "nod5b" {
			if _, err := keyring.Search(os.Args[2]); err != nil {
				r.KeySearch = err.Error()
			} else {
				r.KeySearch = "found"
			}
			r.SessionKey, _ = unix.KeyctlGetKeyringID(unix.KEY_SPEC_SESSION_KEYRING, false)
		}
		if os.Args[1] == "creds" {
			r.Creds = map[string]string{}
			if es, err := os.ReadDir(os.Args[2]); err == nil {
				for _, e := range es {
					b, _ := os.ReadFile(os.Args[2] + "/" + e.Name())
					r.Creds[e.Name()] = string(b)
				}
			}
			var sfs unix.Statfs_t
			if unix.Statfs(os.Args[2], &sfs) == nil {
				r.CredDirMode = strconv.FormatInt(int64(sfs.Flags), 16)
			}
		}
	}
	b, _ := json.Marshal(&r)
	_, _ = unix.Write(fdlayout.StageStatus, b)
}

// The two kernel calls readAll makes, replaceable by a test.
var (
	fstat = unix.Fstat
	pread = unix.Pread
)

func readAll(fd int) ([]byte, error) {
	var st unix.Stat_t
	if err := fstat(fd, &st); err != nil {
		return nil, err
	}
	buf := make([]byte, st.Size)
	for off := 0; off < len(buf); {
		n, err := pread(fd, buf[off:], int64(off))
		if err != nil {
			return nil, err
		}
		if n == 0 { // the file is shorter than its size said
			return nil, io.ErrUnexpectedEOF
		}
		off += n
	}
	return buf, nil
}
