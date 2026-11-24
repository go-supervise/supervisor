package monitor

import (
	"encoding/binary"
	"errors"
)

// ErrConfig is the fixed refusal of a malformed start-up configuration; the
// bytes that caused it are never echoed.
var ErrConfig = errors.New("monitor: malformed start-up configuration")

// Bounds of the start-up configuration (it comes from the core, but M_p checks
// it as it would any input).
const (
	MaxServices = 1024
	// MaxName bounds the notify-name prefix: M_p appends "s<service>e<epoch>"
	// (at most 33 bytes), and a notify name over 61 bytes is refused (7.1.1).
	MaxName   = 24
	MaxLimits = 16
	MaxPlan   = 1 << 20
)

// Config is what the core hands M_p at start, on FDConfig:
//
//	u64 generation | u16 n | n x (u64 lastEpoch, u64 signalMask) |
//	u8 len, notify name | u8 m | m x (u8 len, key, u8 len, value) |
//	u32 len, plan
//
// all little-endian, and nothing after the plan.
type Config struct {
	Generation uint64
	Services   []Service
	NotifyName string
	Limits     [][2]string
	Plan       []byte
}

type reader struct {
	b   []byte
	bad bool
}

func (r *reader) take(n int) []byte {
	if r.bad || n < 0 || n > len(r.b) {
		r.bad = true
		return nil
	}
	p := r.b[:n]
	r.b = r.b[n:]
	return p
}

func (r *reader) u8() int {
	if p := r.take(1); p != nil {
		return int(p[0])
	}
	return 0
}

func (r *reader) str() string { return string(r.take(r.u8())) }

// ParseConfig decodes a configuration, total: it returns ErrConfig for every
// input it does not accept and never panics.
func ParseConfig(b []byte) (*Config, error) {
	r := &reader{b: b}
	c := &Config{}
	if p := r.take(8); p != nil {
		c.Generation = binary.LittleEndian.Uint64(p)
	}
	n := 0
	if p := r.take(2); p != nil {
		n = int(binary.LittleEndian.Uint16(p))
	}
	if n > MaxServices {
		return nil, ErrConfig
	}
	for i := 0; i < n && !r.bad; i++ {
		if p := r.take(16); p != nil {
			c.Services = append(c.Services, Service{
				LastEpoch:  binary.LittleEndian.Uint64(p),
				SignalMask: binary.LittleEndian.Uint64(p[8:]),
			})
		}
	}
	c.NotifyName = r.str()
	m := r.u8()
	if m > MaxLimits {
		return nil, ErrConfig
	}
	for i := 0; i < m && !r.bad; i++ {
		k := r.str()
		c.Limits = append(c.Limits, [2]string{k, r.str()})
	}
	if p := r.take(4); p != nil {
		pl := binary.LittleEndian.Uint32(p)
		if pl > MaxPlan {
			return nil, ErrConfig
		}
		c.Plan = r.take(int(pl))
	}
	if r.bad || len(r.b) != 0 || len(c.NotifyName) == 0 || len(c.NotifyName) > MaxName {
		return nil, ErrConfig
	}
	return c, nil
}

// Marshal is ParseConfig's inverse, for the core and for tests.
func (c *Config) Marshal() []byte {
	b := binary.LittleEndian.AppendUint64(nil, c.Generation)
	b = binary.LittleEndian.AppendUint16(b, uint16(len(c.Services)))
	for _, s := range c.Services {
		b = binary.LittleEndian.AppendUint64(b, s.LastEpoch)
		b = binary.LittleEndian.AppendUint64(b, s.SignalMask)
	}
	b = append(b, byte(len(c.NotifyName)))
	b = append(b, c.NotifyName...)
	b = append(b, byte(len(c.Limits)))
	for _, l := range c.Limits {
		b = append(b, byte(len(l[0])))
		b = append(b, l[0]...)
		b = append(b, byte(len(l[1])))
		b = append(b, l[1]...)
	}
	b = binary.LittleEndian.AppendUint32(b, uint32(len(c.Plan)))
	return append(b, c.Plan...)
}
