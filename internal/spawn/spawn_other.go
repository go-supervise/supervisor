//go:build !linux

// Package spawn is M_p's spawn path (Linux only).
package spawn

import "errors"

// ErrUnsupported is returned on every OS but Linux.
var ErrUnsupported = errors.New("spawn: unsupported on this OS")

// Config is one spawn.
type Config struct{ Lane bool }

// Child is a started helper.
type Child struct{ PID int }

// Start returns ErrUnsupported.
func Start(*Config) (*Child, error) { return nil, ErrUnsupported }
