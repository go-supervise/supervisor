//go:build !linux

// Command stubhelper exists on Linux only.
package main

func main() { panic("stubhelper: Linux only") }
