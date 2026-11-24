// Fixture of the source half: a field written through the reflect setters.
package main

import (
	"reflect"
	"syscall"
)

func main() {
	pa := &syscall.ProcAttr{Sys: &syscall.SysProcAttr{Setsid: true}}
	reflect.ValueOf(pa).Elem().FieldByName("Sys").Elem().FieldByName("Chroot").SetString("/x")
	_, _ = syscall.ForkExec("/x", nil, pa)
}
