//go:build linux

package helper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/go-supervise/supervisor/internal/fdlayout"
)

func TestH4SuidDumpable(t *testing.T) {
	old := SuidDumpablePath
	defer func() { SuidDumpablePath = old }()
	dir := t.TempDir()
	for val, wantErr := range map[string]bool{"0\n": false, "1\n": true, "2\n": false} {
		p := filepath.Join(dir, "v")
		if err := os.WriteFile(p, []byte(val), 0o600); err != nil {
			t.Fatal(err)
		}
		SuidDumpablePath = p
		err := H4()
		if (err != nil) != wantErr {
			t.Errorf("suid_dumpable %q: err %v", val, err)
		}
		if err != nil {
			se, ok := err.(*stepError)
			if !ok || se.code != fdlayout.ExitH4 || !strings.Contains(err.Error(), "suiddumpable") {
				t.Errorf("suid_dumpable 1: %v", err)
			}
		}
	}
	SuidDumpablePath = filepath.Join(dir, "missing")
	if se, ok := H4().(*stepError); !ok || se.code != fdlayout.ExitH4 {
		t.Errorf("unreadable suid_dumpable must be perm, got %v", se)
	}
}

func TestSealsOK(t *testing.T) {
	for got, want := range map[int]bool{
		Seals:                            true,
		Seals | unix.F_SEAL_EXEC:         true, // the kernel adds it for MFD_NOEXEC_SEAL
		Seals &^ unix.F_SEAL_WRITE:       false,
		Seals &^ unix.F_SEAL_SEAL:        false,
		Seals &^ unix.F_SEAL_GROW:        false,
		Seals &^ unix.F_SEAL_SHRINK:      false,
		0:                                false,
		Seals | unix.F_SEAL_FUTURE_WRITE: false,
	} {
		if SealsOK(got) != want {
			t.Errorf("SealsOK(%#x) = %v", got, !want)
		}
	}
}

func TestExitCodesOutsideRuntimeStatuses(t *testing.T) {
	for _, c := range []int{fdlayout.ExitH1, fdlayout.ExitH2, fdlayout.ExitH3, fdlayout.ExitH4, fdlayout.ExitH5,
		fdlayout.ExitH7, fdlayout.ExitH8, fdlayout.ExitH9, fdlayout.ExitH10} {
		if c == 0 || c == 1 || c == 2 {
			t.Errorf("exit code %d collides with success or the Go runtime's", c)
		}
	}
}
