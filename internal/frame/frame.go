// Package frame is the fixed-layout record that `svd helper` (H9) writes into
// a sealed memfd and that the drop stage reads (D2). It is one type so that
// the writer and the reader cannot disagree (SUPERVISOR.md §7.1.1, T17): the
// frame starts with a magic and the digest of the layout below, and a reader
// whose own digest differs refuses with ErrStageMismatch.
package frame

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

// Bounds on every variable-length field (H9: each field length-bounded).
const (
	Magic      = 0x46445653 // "SVDF" little-endian
	MaxGroups  = 64
	MaxFDs     = 64
	MaxStrings = 256
	MaxString  = 4096
	MaxFrame   = 1 << 20
	NRlimits   = 16 // RLIM_NLIMITS on Linux: 0..15
)

// layout is the textual statement of the encoding; Digest hashes it, so
// changing the wire format without changing this string is caught by the
// golden test, and changing this string changes the digest.
const layout = "svd-frame/1 magic:u32 digest:[32] uid:u32 gid:u32 ngroups:u32 groups:u32* caps:u64 " +
	"rlimits:(u64 u64)*16 umask:u32 sigstate:u32 nice:i32 ioprio:u32 timerslack:u64 personality:u32 " +
	"nfds:u32 fds:i32* argv:(u32 len bytes)* envp:(u32 len bytes)* filterlen:u32 filter:bytes"

// Digest is the layout digest compared by the helper and the stage.
var Digest = sha256.Sum256([]byte(layout))

var (
	ErrShort         = errors.New("frame: truncated")
	ErrMagic         = errors.New("frame: bad magic")
	ErrStageMismatch = errors.New("frame: layout digest differs from the stage's own")
	ErrBound         = errors.New("frame: a field exceeds its bound")
	ErrTrailing      = errors.New("frame: trailing bytes")
)

// Frame is the decoded record.
type Frame struct {
	UID, GID    uint32
	Groups      []uint32
	Caps        uint64
	Rlimits     [NRlimits][2]uint64
	Umask       uint32
	SigState    uint32
	Nice        int32
	IOPrio      uint32
	TimerSlack  uint64
	Personality uint32
	FDs         []int32
	Argv, Envp  []string
	Filter      []byte
}

// Encode returns the frame bytes, or ErrBound when a field is out of bounds.
func (f *Frame) Encode() ([]byte, error) {
	if len(f.Groups) > MaxGroups || len(f.FDs) > MaxFDs || len(f.Argv) > MaxStrings ||
		len(f.Envp) > MaxStrings || len(f.Filter) > MaxFrame/2 {
		return nil, ErrBound
	}
	b := binary.LittleEndian.AppendUint32(nil, Magic)
	b = append(b, Digest[:]...)
	b = binary.LittleEndian.AppendUint32(b, f.UID)
	b = binary.LittleEndian.AppendUint32(b, f.GID)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(f.Groups)))
	for _, g := range f.Groups {
		b = binary.LittleEndian.AppendUint32(b, g)
	}
	b = binary.LittleEndian.AppendUint64(b, f.Caps)
	for _, r := range f.Rlimits {
		b = binary.LittleEndian.AppendUint64(b, r[0])
		b = binary.LittleEndian.AppendUint64(b, r[1])
	}
	b = binary.LittleEndian.AppendUint32(b, f.Umask)
	b = binary.LittleEndian.AppendUint32(b, f.SigState)
	b = binary.LittleEndian.AppendUint32(b, uint32(f.Nice))
	b = binary.LittleEndian.AppendUint32(b, f.IOPrio)
	b = binary.LittleEndian.AppendUint64(b, f.TimerSlack)
	b = binary.LittleEndian.AppendUint32(b, f.Personality)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(f.FDs)))
	for _, fd := range f.FDs {
		b = binary.LittleEndian.AppendUint32(b, uint32(fd))
	}
	for _, list := range [][]string{f.Argv, f.Envp} {
		b = binary.LittleEndian.AppendUint32(b, uint32(len(list)))
		for _, s := range list {
			if len(s) > MaxString {
				return nil, ErrBound
			}
			b = binary.LittleEndian.AppendUint32(b, uint32(len(s)))
			b = append(b, s...)
		}
	}
	b = binary.LittleEndian.AppendUint32(b, uint32(len(f.Filter)))
	b = append(b, f.Filter...)
	if len(b) > MaxFrame {
		return nil, ErrBound
	}
	return b, nil
}

type reader struct {
	b   []byte
	err error
}

// fail records the first error only: a later, consequent one never replaces it.
func (r *reader) fail(e error) {
	if r.err == nil {
		r.err = e
	}
}

func (r *reader) take(n int) []byte {
	if n > len(r.b) {
		r.fail(ErrShort)
		return nil
	}
	s := r.b[:n]
	r.b = r.b[n:]
	return s
}

func (r *reader) u32() uint32 {
	if s := r.take(4); s != nil {
		return binary.LittleEndian.Uint32(s)
	}
	return 0
}

func (r *reader) u64() uint64 {
	if s := r.take(8); s != nil {
		return binary.LittleEndian.Uint64(s)
	}
	return 0
}

// count reads a length and refuses one above max before anything is allocated.
func (r *reader) count(max int) int {
	n := r.u32()
	if n > uint32(max) {
		r.fail(ErrBound)
		return 0
	}
	return int(n)
}

func (r *reader) strings() []string {
	n := r.count(MaxStrings)
	out := make([]string, 0, min(n, 16))
	for i := 0; i < n && r.err == nil; i++ {
		l := r.count(MaxString)
		out = append(out, string(r.take(l)))
	}
	return out
}

// Decode parses and bound-checks a frame. It never allocates by a length it
// has not first compared with its bound and with the bytes left.
func Decode(b []byte) (*Frame, error) {
	if len(b) > MaxFrame {
		return nil, ErrBound
	}
	r := &reader{b: b}
	if m := r.u32(); r.err == nil && m != Magic {
		return nil, ErrMagic
	}
	if d := r.take(len(Digest)); r.err == nil && string(d) != string(Digest[:]) {
		return nil, ErrStageMismatch
	}
	f := &Frame{UID: r.u32(), GID: r.u32()}
	for n := r.count(MaxGroups); n > 0 && r.err == nil; n-- {
		f.Groups = append(f.Groups, r.u32())
	}
	f.Caps = r.u64()
	for i := range f.Rlimits {
		f.Rlimits[i] = [2]uint64{r.u64(), r.u64()}
	}
	f.Umask, f.SigState = r.u32(), r.u32()
	f.Nice, f.IOPrio = int32(r.u32()), r.u32()
	f.TimerSlack, f.Personality = r.u64(), r.u32()
	for n := r.count(MaxFDs); n > 0 && r.err == nil; n-- {
		f.FDs = append(f.FDs, int32(r.u32()))
	}
	f.Argv = r.strings()
	f.Envp = r.strings()
	if n := r.count(MaxFrame); r.err == nil {
		f.Filter = append([]byte(nil), r.take(n)...)
	}
	if r.err != nil {
		return nil, r.err
	}
	if len(r.b) != 0 {
		return nil, ErrTrailing
	}
	return f, nil
}
