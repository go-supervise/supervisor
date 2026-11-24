package helper

import (
	"strings"
	"testing"
)

func TestPlanValidateRegions(t *testing.T) {
	p := &Plan{Mode: ModeService, Cwd: "/w", StageArgv: []string{"stage"}}
	p.Regions = []Region{{Path: "rel"}}
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "not absolute") {
		t.Errorf("relative path: %v", err)
	}
	p.Regions = []Region{{Path: ""}}
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "not absolute") {
		t.Errorf("empty path: %v", err)
	}
	p.Regions = []Region{{Path: "/ok"}, {Path: "/"}}
	if err := p.Validate(); err != nil {
		t.Errorf("absolute paths refused: %v", err)
	}
	p.Regions = make([]Region, MaxRegions+1)
	for i := range p.Regions {
		p.Regions[i].Path = "/x"
	}
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "regions") {
		t.Errorf("too many: %v", err)
	}
	p.Regions = p.Regions[:MaxRegions]
	if err := p.Validate(); err != nil {
		t.Errorf("MaxRegions must be accepted: %v", err)
	}
}
