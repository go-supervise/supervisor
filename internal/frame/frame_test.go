package frame

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
)

func sample() *Frame {
	f := &Frame{UID: 1000, GID: 1000, Groups: []uint32{4, 27}, Caps: 1 << 5, Umask: 022,
		SigState: 1, Nice: -3, IOPrio: 7, TimerSlack: 50000, Personality: 8,
		FDs: []int32{0, 1, 2, 9}, Argv: []string{"/bin/true", "-x"}, Envp: []string{"A=b"},
		Filter: []byte{1, 2, 3}}
	f.Rlimits[7] = [2]uint64{1024, 4096}
	return f
}

func TestRoundTrip(t *testing.T) {
	b, err := sample().Encode()
	if err != nil {
		t.Fatal(err)
	}
	g, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, sample()) {
		t.Fatalf("round trip differs: %+v", g)
	}
}

// The digest is part of the contract between two executables: pin it, so a
// change of the layout string is a visible, reviewed change.
func TestDigestGolden(t *testing.T) {
	const goldenDigest = "81e6e2a164178dcd874d61215a31caa48b9bf98268a1a6368fdc885084c294a7"
	got := hex.EncodeToString(Digest[:])
	if goldenDigest != got {
		t.Fatalf("layout digest changed: %s (update goldenDigest only with the frame layout)", got)
	}
}

func TestRefusals(t *testing.T) {
	good, _ := sample().Encode()
	cases := map[string]struct {
		mut  func([]byte) []byte
		want error
	}{
		"empty":     {func(b []byte) []byte { return nil }, ErrShort},
		"magic":     {func(b []byte) []byte { b[0] ^= 1; return b }, ErrMagic},
		"digest":    {func(b []byte) []byte { b[4] ^= 1; return b }, ErrStageMismatch},
		"truncated": {func(b []byte) []byte { return b[:len(b)-1] }, ErrShort},
		"trailing":  {func(b []byte) []byte { return append(b, 0) }, ErrTrailing},
		"toolarge":  {func(b []byte) []byte { return make([]byte, MaxFrame+1) }, ErrBound},
		"ngroups": {func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[4+32+8:], MaxGroups+1)
			return b
		}, ErrBound},
	}
	cases["magic only"] = struct {
		mut  func([]byte) []byte
		want error
	}{func(b []byte) []byte { return b[:4] }, ErrShort}
	cases["half digest"] = struct {
		mut  func([]byte) []byte
		want error
	}{func(b []byte) []byte { return b[:20] }, ErrShort}
	cases["exactly max, zeros"] = struct {
		mut  func([]byte) []byte
		want error
	}{func(b []byte) []byte { return make([]byte, MaxFrame) }, ErrMagic}
	for name, c := range cases {
		// A panic is an assertion failure here, not a luckily red suite.
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: panic: %v", name, r)
				}
			}()
			_, err := Decode(c.mut(bytes.Clone(good)))
			if !errors.Is(err, c.want) {
				t.Errorf("%s: got %v, want %v", name, err, c.want)
			}
		}()
	}
}

func TestEncodeBounds(t *testing.T) {
	for name, f := range map[string]*Frame{
		"groups": {Groups: make([]uint32, MaxGroups+1)},
		"fds":    {FDs: make([]int32, MaxFDs+1)},
		"argv":   {Argv: make([]string, MaxStrings+1)},
		"envp":   {Envp: make([]string, MaxStrings+1)},
		"string": {Argv: []string{string(make([]byte, MaxString+1))}},
		"filter": {Filter: make([]byte, MaxFrame)},
	} {
		if _, err := f.Encode(); !errors.Is(err, ErrBound) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The total bound: 256 strings of MaxString each fit their own bounds but not
// the frame's; and a frame of exactly MaxFrame bytes is accepted.
func TestEncodeTotalBound(t *testing.T) {
	big := string(make([]byte, MaxString))
	f := &Frame{}
	for i := 0; i < MaxStrings; i++ {
		f.Argv = append(f.Argv, big)
	}
	if _, err := f.Encode(); !errors.Is(err, ErrBound) {
		t.Fatalf("over the total: %v", err)
	}
	// Shrink the last string until the frame is exactly MaxFrame bytes.
	empty, _ := (&Frame{Argv: f.Argv[:MaxStrings-1]}).Encode()
	last := MaxFrame - len(empty) - 4
	if last < 0 || last > MaxString {
		t.Fatalf("arithmetic: %d", last)
	}
	f.Argv[MaxStrings-1] = string(make([]byte, last))
	b, err := f.Encode()
	if err != nil || len(b) != MaxFrame {
		t.Fatalf("exactly max: %d, %v", len(b), err)
	}
	if _, err := Decode(b); err != nil {
		t.Fatalf("decode of a frame of exactly max: %v", err)
	}
}

func FuzzDecode(f *testing.F) {
	good, _ := sample().Encode()
	f.Add(good)
	f.Add([]byte{})
	f.Add(good[:40])
	f.Fuzz(func(t *testing.T, b []byte) {
		g, err := Decode(b)
		if err != nil {
			return
		}
		// An accepted frame re-encodes to exactly the same bytes.
		again, err := g.Encode()
		if err != nil || !bytes.Equal(again, b) {
			t.Fatalf("accepted frame does not round-trip: %v", err)
		}
	})
}
