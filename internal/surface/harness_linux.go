//go:build linux

package surface

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

// RunOptions of the dynamic half.
type RunOptions struct {
	Arch     string        // the GOARCH of the binaries (the host's)
	Launcher string        // cmd/syscallsurface: its hidden `launch` installs the filter
	Monitor  string        // svd-monitor, as built (canary tags included)
	Stub     string        // the stub helper, also the stage descriptor
	Dir      string        // scratch directory
	Timeout  time.Duration // per scenario; 0 means a minute
}

// RunResult of one scenario.
type RunResult struct {
	Rec      *Recording
	Status   string
	Failures []string
}

var pidLine = regexp.MustCompile(`(?m)^S (\d+) \d+$`)

func open(path string, flags int) (*os.File, error) {
	fd, err := unix.Open(path, flags|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

// RunScenario runs one fresh svd-monitor under the filter and the recorder
// with the descriptor layout of monitor.StdFds, and judges it.
func RunScenario(o RunOptions, sc Scenario) (*RunResult, error) {
	if o.Timeout == 0 {
		o.Timeout = time.Minute
	}
	dir, err := os.MkdirTemp(o.Dir, "run")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	trees := fmt.Sprintf("/sys/fs/cgroup/svdsurface-%d-%d", os.Getpid(), time.Now().UnixNano())
	if err := os.Mkdir(trees, 0o755); err != nil {
		return nil, fmt.Errorf("surface: cgroup v2 trees directory: %v", err)
	}
	defer removeCgroup(trees)
	notify := filepath.Join(dir, "notify")
	if err := os.Mkdir(notify, 0o711); err != nil {
		return nil, err
	}
	if err := os.Chmod(notify, 0o711); err != nil {
		return nil, err
	}
	cfg := filepath.Join(dir, "config")
	if err := os.WriteFile(cfg, sc.Config.Marshal(), 0o600); err != nil {
		return nil, err
	}

	sp, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	peer, recFile := sp[1], os.NewFile(uintptr(sp[0]), "record")
	defer func() {
		if peer >= 0 {
			unix.Close(peer)
		}
	}()
	statusR, statusW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer statusR.Close()
	errR, errW, err := os.Pipe() // what the launcher or the monitor says on standard error
	if err != nil {
		return nil, err
	}
	defer errR.Close()
	defer errW.Close()

	var extra []*os.File
	own := []*os.File{recFile, statusW, errW}
	add := func(p string, flags int) error {
		f, err := open(p, flags)
		if err != nil {
			return err
		}
		extra = append(extra, f)
		own = append(own, f)
		return nil
	}
	// The order of monitor.StdFds: Record 3, Trees 4, Notify 5, DevNull 6,
	// Helper 7, Stage 8, Config 9, Status 10.
	extra = append(extra, recFile)
	for _, a := range []struct {
		p string
		f int
	}{{trees, unix.O_DIRECTORY | unix.O_RDONLY}, {notify, unix.O_DIRECTORY | unix.O_RDONLY}, {"/dev/null", unix.O_RDWR}} {
		if err := add(a.p, a.f); err != nil {
			return nil, err
		}
	}
	if sc.NoHelper {
		extra = append(extra, nil)
	} else if err := add(o.Stub, unix.O_PATH); err != nil {
		return nil, err
	}
	for _, a := range []struct {
		p string
		f int
	}{{o.Stub, unix.O_PATH}, {cfg, unix.O_RDONLY}} {
		if err := add(a.p, a.f); err != nil {
			return nil, err
		}
	}
	extra = append(extra, statusW)
	closeOwn := func() {
		for _, f := range own {
			f.Close()
		}
	}
	defer closeOwn()

	for _, r := range sc.Records {
		if _, err := unix.Write(peer, r); err != nil {
			return nil, err
		}
	}
	if !sc.KeepOpen {
		unix.Close(peer)
		peer = -1
	}

	cmd := exec.Command(o.Launcher, "launch", o.Monitor)
	cmd.Env = []string{}
	cmd.Stderr = errW
	cmd.ExtraFiles = extra
	rec, err := Record(cmd, o.Arch, 1, o.Timeout, closeOwn)
	if err != nil {
		return nil, err
	}
	res := &RunResult{Rec: rec, Status: readStatus(statusR)}
	res.Failures = Judge(o.Arch, rec, sc.WantExit)
	if msg := readStatus(errR); len(res.Failures) > 0 && msg != "" {
		res.Failures = append(res.Failures, "dynamic: stderr "+strconv.Quote(msg))
	}
	if rec.Signal == 0 {
		got := pidLine.ReplaceAllString(res.Status, "S $1 PID")
		if got != sc.WantStatus {
			res.Failures = append(res.Failures, "dynamic: status "+strconv.Quote(got)+", want "+strconv.Quote(sc.WantStatus))
		}
	}
	return res, nil
}

// readStatus reads what is left of the status pipe once M_p and its
// descendants are gone (every write end is closed, so EOF ends it).
func readStatus(f *os.File) string {
	var out []byte
	buf := make([]byte, 4096)
	_ = f.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		n, err := f.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			return string(out)
		}
	}
}

// removeCgroup removes the trees directory and the epoch cgroups M_p left;
// a cgroup empties asynchronously, so it retries.
func removeCgroup(path string) {
	for i := 0; i < 100; i++ {
		if ents, err := os.ReadDir(path); err == nil {
			for _, e := range ents {
				if e.IsDir() {
					_ = unix.Rmdir(filepath.Join(path, e.Name()))
				}
			}
		}
		if unix.Rmdir(path) == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}
