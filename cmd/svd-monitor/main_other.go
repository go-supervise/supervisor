//go:build !linux || !go1.27 || go1.28

// Command svd-monitor exists on Linux, built by go 1.27 only (the toolchain
// the syscall-surface gate judges).
package main

func main() { panic("svd-monitor: needs Linux and the go 1.27 toolchain") }
