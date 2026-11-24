//go:build !linux

// Command refprog exists on Linux only.
package main

func main() { panic("refprog: Linux only") }
