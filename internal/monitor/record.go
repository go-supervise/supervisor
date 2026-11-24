// Package monitor is the root monitor M_p's pure part: the decoder of the
// 64-byte record L_p sends (SUPERVISOR.md §3.3) and, on Linux, the spawn steps
// M1-M4 (§7.1.1). The decoder is allocation-free, panic-free by construction
// (fixed offsets into a fixed array, no slicing by a decoded value) and
// evaluates its checks in the order of draft 14 (T8).
package monitor

import "encoding/binary"

const (
	RecordSize    = 64
	RecordMagic   = 0x5653
	RecordVersion = 2
)

// Kinds.
const (
	KindSpawn      = 1
	KindSignalMain = 2
	KindSignalTree = 3
	KindKillTree   = 4
	KindReapAck    = 5
)

// Outcome is the decoder's total, three-valued decision.
type Outcome uint8

const (
	Act Outcome = iota
	Refuse
	Fault
)

// Reason is a fixed code; L_p's bytes are never echoed.
type Reason uint8

const (
	OK Reason = iota
	BadLength
	Truncated
	BadMagic
	BadVersion
	BadKind
	BadFlags
	BadReserved
	BadSeq
	NewerGeneration // Fault: a generation M_p has not been told
	StaleGeneration // Refuse, consumes the seq
	BadService
	BadSignal
	BadEpoch // Refuse
	// ServiceLive is a Refuse of M_p's own loop, not of Decode: a spawn for a
	// service whose previous child has not been acknowledged as reaped.
	ServiceLive
)

// Record is the decoded form.
type Record struct {
	Kind       uint8
	Service    uint32
	Epoch      uint64
	Signal     uint32
	Seq        uint64
	Generation uint64
}

// Service is M_p's own copy of one plan entry's decoder-relevant state.
type Service struct {
	LastEpoch  uint64
	SignalMask uint64 // bit s set: signal s is in the service's plan set (s < 64)
}

// State is what M_p holds: its own copy of the plan, from the core.
type State struct {
	LastSeq    uint64
	Generation uint64
	Services   []Service
}

// Decode evaluates, and stops at the first hit: (1) plan-independent framing
// (length, MSG_TRUNC, magic, version, kind, flags, reserved bytes, seq);
// (2) the generation (newer: Fault; older: Refuse, which consumes the seq);
// (3) plan-dependent checks, only for the current generation (service range,
// signal set); (4) Refuse conditions (a stale epoch); (5) Act.
func Decode(buf *[RecordSize + 1]byte, n int, truncated bool, st *State) (Record, Outcome, Reason) {
	var r Record
	if n == 0 {
		return r, Fault, BadLength // hang-up is handled by the caller; never an empty record
	}
	if truncated || n != RecordSize {
		return r, Fault, Truncated
	}
	b := buf[:RecordSize]
	if binary.LittleEndian.Uint16(b[0:]) != RecordMagic {
		return r, Fault, BadMagic
	}
	if b[2] != RecordVersion {
		return r, Fault, BadVersion
	}
	r.Kind = b[3]
	if r.Kind < KindSpawn || r.Kind > KindReapAck {
		return r, Fault, BadKind
	}
	r.Service = binary.LittleEndian.Uint32(b[4:])
	r.Epoch = binary.LittleEndian.Uint64(b[8:])
	r.Signal = binary.LittleEndian.Uint32(b[16:])
	if binary.LittleEndian.Uint32(b[20:]) != 0 {
		return r, Fault, BadFlags
	}
	r.Seq = binary.LittleEndian.Uint64(b[24:])
	r.Generation = binary.LittleEndian.Uint64(b[32:])
	for _, c := range b[40:RecordSize] {
		if c != 0 {
			return r, Fault, BadReserved
		}
	}
	if r.Seq != st.LastSeq+1 {
		return r, Fault, BadSeq
	}
	if r.Generation > st.Generation {
		return r, Fault, NewerGeneration
	}
	if r.Generation < st.Generation {
		return r, Refuse, StaleGeneration
	}
	if uint64(r.Service) >= uint64(len(st.Services)) {
		return r, Fault, BadService
	}
	sv := &st.Services[r.Service]
	if r.Kind == KindSignalMain || r.Kind == KindSignalTree {
		if r.Signal >= 64 || sv.SignalMask&(1<<r.Signal) == 0 {
			return r, Fault, BadSignal
		}
	} else if r.Signal != 0 {
		return r, Fault, BadSignal
	}
	want := sv.LastEpoch
	if r.Kind == KindSpawn {
		want++
	}
	if r.Epoch != want {
		return r, Refuse, BadEpoch
	}
	return r, Act, OK
}
