package monitor

import (
	"encoding/binary"
	"testing"
)

func state() *State {
	return &State{LastSeq: 4, Generation: 7, Services: []Service{
		{LastEpoch: 3, SignalMask: 1<<15 | 1<<1}, {LastEpoch: 0}}}
}

func rec(kind uint8, svc uint32, epoch uint64, sig uint32, seq, gen uint64) *[RecordSize + 1]byte {
	var b [RecordSize + 1]byte
	binary.LittleEndian.PutUint16(b[0:], RecordMagic)
	b[2], b[3] = RecordVersion, kind
	binary.LittleEndian.PutUint32(b[4:], svc)
	binary.LittleEndian.PutUint64(b[8:], epoch)
	binary.LittleEndian.PutUint32(b[16:], sig)
	binary.LittleEndian.PutUint64(b[24:], seq)
	binary.LittleEndian.PutUint64(b[32:], gen)
	return &b
}

func TestDecodeTable(t *testing.T) {
	mut := func(f func(*[RecordSize + 1]byte)) *[RecordSize + 1]byte {
		b := rec(KindSpawn, 0, 4, 0, 5, 7)
		f(b)
		return b
	}
	cases := []struct {
		name  string
		buf   *[RecordSize + 1]byte
		n     int
		trunc bool
		out   Outcome
		why   Reason
	}{
		{"spawn acts", rec(KindSpawn, 0, 4, 0, 5, 7), 64, false, Act, OK},
		{"signal in set", rec(KindSignalMain, 0, 3, 15, 5, 7), 64, false, Act, OK},
		{"hangup", rec(KindSpawn, 0, 4, 0, 5, 7), 0, false, Fault, BadLength},
		{"short", rec(KindSpawn, 0, 4, 0, 5, 7), 63, false, Fault, Truncated},
		{"long", rec(KindSpawn, 0, 4, 0, 5, 7), 65, false, Fault, Truncated},
		{"msg_trunc", rec(KindSpawn, 0, 4, 0, 5, 7), 64, true, Fault, Truncated},
		{"magic", mut(func(b *[65]byte) { b[0] ^= 1 }), 64, false, Fault, BadMagic},
		{"version", mut(func(b *[65]byte) { b[2] = 1 }), 64, false, Fault, BadVersion},
		{"kind 0", rec(0, 0, 4, 0, 5, 7), 64, false, Fault, BadKind},
		{"kind 6", rec(6, 0, 4, 0, 5, 7), 64, false, Fault, BadKind},
		{"flags", mut(func(b *[65]byte) { b[20] = 1 }), 64, false, Fault, BadFlags},
		{"reserved first", mut(func(b *[65]byte) { b[40] = 1 }), 64, false, Fault, BadReserved},
		{"reserved last", mut(func(b *[65]byte) { b[63] = 1 }), 64, false, Fault, BadReserved},
		{"seq replay", rec(KindSpawn, 0, 4, 0, 4, 7), 64, false, Fault, BadSeq},
		{"seq gap", rec(KindSpawn, 0, 4, 0, 6, 7), 64, false, Fault, BadSeq},
		{"newer generation", rec(KindSpawn, 0, 4, 0, 5, 8), 64, false, Fault, NewerGeneration},
		{"stale generation", rec(KindSpawn, 0, 4, 0, 5, 6), 64, false, Refuse, StaleGeneration},
		// T8: a service index valid under g-1 and >= |grant(p)| under g is Refuse, not Fault.
		{"stale generation beats service", rec(KindSpawn, 9, 4, 0, 5, 6), 64, false, Refuse, StaleGeneration},
		{"service out of range", rec(KindSpawn, 2, 4, 0, 5, 7), 64, false, Fault, BadService},
		{"service huge", rec(KindSpawn, 0xffffffff, 4, 0, 5, 7), 64, false, Fault, BadService},
		{"signal outside set", rec(KindSignalMain, 0, 3, 9, 5, 7), 64, false, Fault, BadSignal},
		{"signal 64", rec(KindSignalMain, 0, 3, 64, 5, 7), 64, false, Fault, BadSignal},
		{"signal on spawn", rec(KindSpawn, 0, 4, 1, 5, 7), 64, false, Fault, BadSignal},
		{"spawn wrong epoch", rec(KindSpawn, 0, 3, 0, 5, 7), 64, false, Refuse, BadEpoch},
		{"kill current epoch", rec(KindKillTree, 0, 3, 0, 5, 7), 64, false, Act, OK},
		{"kill stale epoch", rec(KindKillTree, 0, 2, 0, 5, 7), 64, false, Refuse, BadEpoch},
	}
	for _, c := range cases {
		_, out, why := Decode(c.buf, c.n, c.trunc, state())
		if out != c.out || why != c.why {
			t.Errorf("%s: got (%d,%d) want (%d,%d)", c.name, out, why, c.out, c.why)
		}
	}
}

func TestDecodeAllocFree(t *testing.T) {
	b, st := rec(KindSpawn, 0, 4, 0, 5, 7), state()
	if n := testing.AllocsPerRun(100, func() { Decode(b, 64, false, st) }); n != 0 {
		t.Fatalf("decoder allocates %v", n)
	}
}

func FuzzMonitorRecord(f *testing.F) {
	f.Add(rec(KindSpawn, 0, 4, 0, 5, 7)[:], false)
	f.Add(rec(KindSpawn, 9, 4, 0, 5, 6)[:], false) // stale-generation case of T8
	f.Fuzz(func(t *testing.T, raw []byte, trunc bool) {
		var b [RecordSize + 1]byte
		n := copy(b[:], raw)
		st := state()
		r, out, why := Decode(&b, n, trunc, st)
		// Invariants: Act only with Reason OK; Fault/Refuse never OK.
		if (out == Act) != (why == OK) {
			t.Fatalf("outcome %d with reason %d", out, why)
		}
		if out == Act && (r.Seq != st.LastSeq+1 || r.Generation != st.Generation || int(r.Service) >= len(st.Services)) {
			t.Fatalf("acted on %+v", r)
		}
		if out == Refuse && why == StaleGeneration && r.Generation >= st.Generation {
			t.Fatalf("stale refuse for %+v", r)
		}
	})
}
