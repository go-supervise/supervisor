// Fixture of the symbol half: a denylisted function that the compiler inlines
// at its one call site, so that no symbol of it is linked (only the
// debug information names it).
package main

import "golang.org/x/sys/unix"

func main() {
	_ = unix.Setuid(0)
}
