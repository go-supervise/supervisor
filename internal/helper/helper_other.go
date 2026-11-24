//go:build !linux

package helper

import "io"

// Run returns ErrUnsupported's exit code on this OS.
func Run(stderr io.Writer) int {
	_, _ = io.WriteString(stderr, ErrUnsupported.Error()+"\n")
	return 3
}
