package monitor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func sample() *Config {
	return &Config{
		Generation: 0x0102030405060708,
		Services:   []Service{{LastEpoch: 3, SignalMask: 0xff}, {LastEpoch: 9, SignalMask: 1 << 40}},
		NotifyName: "n",
		Limits:     [][2]string{{"memory.max", "1048576"}, {"pids.max", "64"}},
		Plan:       []byte(`{"a":1}`),
	}
}

func TestConfigRoundTrip(t *testing.T) {
	want := sample()
	got, err := ParseConfig(want.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	if got.Generation != want.Generation || got.NotifyName != want.NotifyName || !bytes.Equal(got.Plan, want.Plan) {
		t.Fatalf("got %+v", got)
	}
	if len(got.Services) != 2 || got.Services[0] != want.Services[0] || got.Services[1] != want.Services[1] {
		t.Fatalf("services %+v", got.Services)
	}
	if len(got.Limits) != 2 || got.Limits[0] != want.Limits[0] || got.Limits[1] != want.Limits[1] {
		t.Fatalf("limits %+v", got.Limits)
	}
}

func TestConfigRefusesEveryTruncation(t *testing.T) {
	b := sample().Marshal()
	for i := 0; i < len(b); i++ {
		if _, err := ParseConfig(b[:i]); !errors.Is(err, ErrConfig) {
			t.Fatalf("prefix of %d bytes: %v", i, err)
		}
	}
}

func TestConfigRefusesTrailingBytes(t *testing.T) {
	if _, err := ParseConfig(append(sample().Marshal(), 0)); !errors.Is(err, ErrConfig) {
		t.Fatalf("trailing byte: %v", err)
	}
}

func TestConfigBounds(t *testing.T) {
	ok := func(name string, c *Config) {
		t.Helper()
		got, err := ParseConfig(c.Marshal())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(got.Services) != len(c.Services) || len(got.Limits) != len(c.Limits) || len(got.Plan) != len(c.Plan) || got.NotifyName != c.NotifyName {
			t.Fatalf("%s: shape changed", name)
		}
	}
	bad := func(name string, c *Config) {
		t.Helper()
		if _, err := ParseConfig(c.Marshal()); !errors.Is(err, ErrConfig) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	mk := func(n, m, plan int, name string) *Config {
		c := &Config{NotifyName: name, Services: make([]Service, n), Plan: make([]byte, plan)}
		for i := 0; i < m; i++ {
			c.Limits = append(c.Limits, [2]string{"k", "v"})
		}
		return c
	}
	ok("max services", mk(MaxServices, 0, 0, "n"))
	bad("too many services", mk(MaxServices+1, 0, 0, "n"))
	ok("max limits", mk(0, MaxLimits, 0, "n"))
	bad("too many limits", mk(0, MaxLimits+1, 0, "n"))
	ok("max plan", mk(0, 0, MaxPlan, "n"))
	bad("plan too long", mk(0, 0, MaxPlan+1, "n"))
	ok("max name", mk(0, 0, 0, strings.Repeat("a", MaxName)))
	bad("name too long", mk(0, 0, 0, strings.Repeat("a", MaxName+1)))
	bad("empty name", mk(0, 0, 0, ""))
}

// A plan length that promises more than the input holds is refused without
// allocating what it promises.
func TestConfigPlanLengthLies(t *testing.T) {
	b := (&Config{NotifyName: "n"}).Marshal()
	binary.LittleEndian.PutUint32(b[len(b)-4:], MaxPlan)
	if _, err := ParseConfig(b); !errors.Is(err, ErrConfig) {
		t.Fatalf("lying plan length: %v", err)
	}
	binary.LittleEndian.PutUint32(b[len(b)-4:], 1<<31)
	if _, err := ParseConfig(b); !errors.Is(err, ErrConfig) {
		t.Fatalf("huge plan length: %v", err)
	}
}

func FuzzParseConfig(f *testing.F) {
	f.Add(sample().Marshal())
	f.Add([]byte{})
	f.Add(make([]byte, 11))
	f.Fuzz(func(t *testing.T, b []byte) {
		c, err := ParseConfig(b)
		if err != nil {
			if !errors.Is(err, ErrConfig) {
				t.Fatalf("error %v is not ErrConfig", err)
			}
			return
		}
		if !bytes.Equal(c.Marshal(), b) {
			t.Fatalf("accepted input does not round-trip")
		}
	})
}
