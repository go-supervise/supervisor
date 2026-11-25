package helper

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-supervise/supervisor/internal/frame"
)

func goodPlan() *Plan {
	return &Plan{Mode: ModeService, Frame: frame.Frame{UID: 1, GID: 2, Argv: []string{"a"}}, Cwd: "/", StageArgv: []string{"svd-drop"}}
}

func TestPlanRoundTrip(t *testing.T) {
	p := goodPlan()
	b, err := p.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	q, err := ParsePlan(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, q) {
		t.Errorf("round trip: %+v != %+v", p, q)
	}
}

func TestPlanRefusals(t *testing.T) {
	for name, mut := range map[string]func(*Plan){
		"mode":         func(p *Plan) { p.Mode = "x" },
		"empty mode":   func(p *Plan) { p.Mode = "" },
		"no stage":     func(p *Plan) { p.StageArgv = nil },
		"no cwd":       func(p *Plan) { p.Cwd = "" },
		"creds no dir": func(p *Plan) { p.Creds = []Credential{{Name: "a", Source: "/s"}} },
		"relative cwd": func(p *Plan) { p.Cwd = "w" },
		"relative dir": func(p *Plan) { p.CredDir = "d"; p.Creds = []Credential{{Name: "a", Source: "/s"}} },
		"relative src": func(p *Plan) { p.CredDir = "/d"; p.Creds = []Credential{{Name: "a", Source: "s"}} },
		"empty src":    func(p *Plan) { p.CredDir = "/d"; p.Creds = []Credential{{Name: "a"}} },
		"signal creds": func(p *Plan) {
			p.Mode, p.CredDir, p.Creds = ModeSignal, "/d", []Credential{{Name: "a", Source: "/s"}}
		},
		"signal dir": func(p *Plan) { p.Mode, p.CredDir = ModeSignal, "/d" },
	} {
		p := goodPlan()
		mut(p)
		if _, err := p.Marshal(); err == nil {
			t.Errorf("%s: Marshal accepted", name)
		}
	}
	if _, err := ParsePlan([]byte("{")); err == nil {
		t.Error("garbage accepted")
	}
	good, _ := goodPlan().Marshal()
	if _, err := ParsePlan(append(good, strings.Repeat(" ", MaxPlan)...)); err == nil {
		t.Error("oversized plan accepted")
	}
	// A type error after the valid fields were read must still refuse.
	if _, err := ParsePlan([]byte(`{"mode":"service","cwd":"/","stage_argv":["x"],"frame":5}`)); err == nil {
		t.Error("ill-typed plan accepted")
	}
	// Well-formed JSON, invalid plan.
	if _, err := ParsePlan([]byte(`{"mode":"x","cwd":"/","stage_argv":["x"]}`)); err == nil {
		t.Error("invalid plan accepted")
	}
	if _, err := ParsePlan([]byte(`{"mode":"lane","cwd":"/","stage_argv":["x"]}`)); err != nil {
		t.Errorf("lane plan refused: %v", err)
	}
	big := goodPlan()
	big.Frame.Argv = make([]string, 0)
	big.StageArgv = []string{strings.Repeat("x", MaxPlan)}
	if _, err := big.Marshal(); err == nil {
		t.Error("oversized Marshal accepted")
	}
}
