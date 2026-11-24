//go:build linux

// Command svd-monitor is M_p, the root monitor of one delegated principal
// (SUPERVISOR.md 7.1.2: its own executable, so that the linker has removed
// what M_p cannot reach). It is started by the core only, with the descriptor
// layout of monitor.StdFds, and has no flags and no command line: a CLI
// framework would put reflection and the whole of fmt's reach in M_p's call
// graph, which the syscall-surface lint bounds.
//
// The two //go:debug lines turn off the runtime's container-aware GOMAXPROCS:
// the runtime still reads /proc/self/cgroup, /proc/self/mountinfo and cpu.max
// once at start-up (the surface gate's recorder admits exactly those paths),
// but no longer re-reads them every second, nor lets them move GOMAXPROCS.
//
//go:debug containermaxprocs=0
//go:debug updatemaxprocs=0
package main

import (
	"syscall"

	"github.com/go-supervise/supervisor/internal/monitor"
)

func main() {
	syscall.Exit(monitor.Run(monitor.StdFds, monitor.NewHost(monitor.StdFds)))
}
