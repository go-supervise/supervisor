// Fixture of the source half: a function the runtime calls and no code calls
// (a finalizer) issues a forbidden system call.
package main

import (
	"runtime"

	"golang.org/x/sys/unix"
)

type T struct{ _ int }

func fin(*T) { _ = unix.Kill(1, 9) }

func main() { runtime.SetFinalizer(&T{}, fin) }
