//go:build linux

package main

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"golang.org/x/sys/unix"
)

func memfd(t *testing.T, b []byte) int {
	t.Helper()
	fd, err := unix.MemfdCreate("t", unix.MFD_CLOEXEC)
	if err != nil {
		t.Skip("no memfd:", err)
	}
	t.Cleanup(func() { unix.Close(fd) })
	if _, err := unix.Write(fd, b); err != nil {
		t.Fatal(err)
	}
	return fd
}

func TestReadAllWholeFile(t *testing.T) {
	want := bytes.Repeat([]byte("frame"), 1000)
	got, err := readAll(memfd(t, want))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("got %d bytes, %v", len(got), err)
	}
}

func TestReadAllFollowsShortReads(t *testing.T) {
	want := []byte("0123456789")
	fd := memfd(t, want)
	old := pread
	t.Cleanup(func() { pread = old })
	calls := 0
	pread = func(fd int, p []byte, off int64) (int, error) {
		calls++
		return old(fd, p[:min(len(p), 3)], off)
	}
	got, err := readAll(fd)
	if err != nil || !bytes.Equal(got, want) || calls != 4 {
		t.Fatalf("got %q, %v after %d preads", got, err, calls)
	}
}

func TestReadAllRefusals(t *testing.T) {
	boom := errors.New("boom")
	t.Run("fstat", func(t *testing.T) {
		old := fstat
		t.Cleanup(func() { fstat = old })
		fstat = func(int, *unix.Stat_t) error { return boom }
		if b, err := readAll(0); b != nil || !errors.Is(err, boom) {
			t.Fatalf("%q %v", b, err)
		}
	})
	t.Run("pread error", func(t *testing.T) {
		fd := memfd(t, []byte("abc"))
		old := pread
		t.Cleanup(func() { pread = old })
		pread = func(int, []byte, int64) (int, error) { return 0, boom }
		if b, err := readAll(fd); b != nil || !errors.Is(err, boom) {
			t.Fatalf("%q %v", b, err)
		}
	})
	t.Run("short file", func(t *testing.T) {
		fd := memfd(t, []byte("abc"))
		old := pread
		t.Cleanup(func() { pread = old })
		calls := 0
		pread = func(int, []byte, int64) (int, error) {
			if calls++; calls > 2 { // a loop that never ends is a failure, not a hang
				return 0, errors.New("readAll kept reading at EOF")
			}
			return 0, nil
		}
		if b, err := readAll(fd); b != nil || !errors.Is(err, io.ErrUnexpectedEOF) || calls != 1 {
			t.Fatalf("%q %v", b, err)
		}
	})
	t.Run("bad fd", func(t *testing.T) {
		if b, err := readAll(-1); b != nil || err == nil {
			t.Fatalf("%q %v", b, err)
		}
	})
}
