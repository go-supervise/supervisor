//go:build linux

// Command stubhelper is the stub helper of the dynamic half (7.1.2 item 2):
// the program M_p's fork child executes in place of svd's helper. It writes
// `A` (armed) to the status descriptor of the helper layout and exits, so that
// what M_p spawns under the enforcing filter issues only calls of the Go
// runtime and of that one write. It imports nothing but syscall.
package main

import "syscall"

func main() {
	_, _ = syscall.Write(4, []byte("A"))
	syscall.Exit(0)
}
