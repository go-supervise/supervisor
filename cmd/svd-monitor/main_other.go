//go:build !linux

// Command svd-monitor exists on Linux only.
package main

func main() { panic("svd-monitor: Linux only") }
