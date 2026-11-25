// Fixture of the source half: a ProcAttr, which holds a *SysProcAttr, filled by
// a gob decoder from a stream built here.
package main

import (
	"bytes"
	"encoding/gob"
	"syscall"
)

func main() {
	var p syscall.ProcAttr
	_ = gob.NewDecoder(bytes.NewReader(nil)).Decode(&p)
	_, _ = syscall.ForkExec("/x", nil, &p)
}
