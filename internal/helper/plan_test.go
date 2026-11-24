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
	if _, err := ParsePlan([]byte(strings.Repeat(" ", MaxPlan+1))); err == nil {
		t.Error("oversized plan accepted")
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
