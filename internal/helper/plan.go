// Package helper is `svd helper`: steps H1-H10 of SUPERVISOR.md 7.1.1.
// It never changes an id, a capability or a per-thread attribute.
package helper

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-supervise/supervisor/internal/frame"
)

// ErrUnsupported is returned on every OS but Linux.
var ErrUnsupported = errors.New("helper: unsupported on this OS")

// Mode is the spawn mode.
type Mode string

// The three modes of 7.1.1 "Modes".
const (
	ModeService Mode = "service"
	ModeLane    Mode = "lane"
	ModeSignal  Mode = "signal"
)

// Credential is one credential source to deliver (H7).
type Credential struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// Plan is the core's plan entry as M_p writes it into the sealed memfd.
// It is trusted (written by M_p from the core's plan).
type Plan struct {
	Mode      Mode         `json:"mode"`
	Frame     frame.Frame  `json:"frame"`
	Cwd       string       `json:"cwd"`
	Creds     []Credential `json:"creds,omitempty"`
	CredDir   string       `json:"cred_dir,omitempty"`
	CredSize  string       `json:"cred_size,omitempty"`
	StageArgv []string     `json:"stage_argv"`
}

// MaxPlan bounds the plan entry.
const MaxPlan = 1 << 20

// Validate refuses a plan the helper cannot honour.
func (p *Plan) Validate() error {
	switch p.Mode {
	case ModeService, ModeLane, ModeSignal:
	default:
		return fmt.Errorf("helper: unknown mode %q", p.Mode)
	}
	if len(p.StageArgv) == 0 {
		return errors.New("helper: empty stage argv")
	}
	if p.Cwd == "" {
		return errors.New("helper: empty cwd")
	}
	if len(p.Creds) > 0 && p.CredDir == "" {
		return errors.New("helper: credentials without a delivery directory")
	}
	return nil
}

// Marshal encodes a plan.
func (p *Plan) Marshal() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxPlan {
		return nil, errors.New("helper: plan too large")
	}
	return b, nil
}

// ParsePlan decodes and validates a plan.
func ParsePlan(b []byte) (*Plan, error) {
	if len(b) > MaxPlan {
		return nil, errors.New("helper: plan too large")
	}
	var p Plan
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}
