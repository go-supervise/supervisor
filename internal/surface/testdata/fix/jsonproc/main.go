// Fixture of the source half: a ProcAttr, which holds a *SysProcAttr, filled by
// a decoder that writes through the type's fields.
package main

import (
	"encoding/json"
	"syscall"
)

func main() {
	var p syscall.ProcAttr
	_ = json.Unmarshal([]byte(`{"Sys":{"Chroot":"/x"}}`), &p)
	_, _ = syscall.ForkExec("/x", nil, &p)
}
