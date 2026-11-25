// Fixture of the source half: the allowed constructor builds its SysProcAttr
// inside a wrapper struct that leaves as an interface.
package main

import (
	"encoding/asn1"
	"syscall"
)

type holder struct{ A syscall.SysProcAttr }

func configure(der []byte) *syscall.ProcAttr {
	var w holder
	_, _ = asn1.Unmarshal(der, &w)
	return &syscall.ProcAttr{Sys: &w.A}
}

func main() { _ = configure(nil) }
