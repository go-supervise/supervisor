// Fixture of the source half: an identity change through the standard
// library's all-threads path.
package main

import "syscall"

func main() {
	_ = syscall.Setuid(0)
}
