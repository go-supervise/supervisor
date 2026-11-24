//go:build linux

// Command refprog is the reference program R_go is generated from (7.1.2 item
// 0). It imports nothing of svd, so that regenerating the list cannot launder
// a call of the monitor into the runtime's list; it exercises the scheduler,
// the garbage collector, timers, the netpoller, os/signal and
// syscall.ForkExec of a stub (its first argument), and initialises os and fmt.
// It never sends a signal: kill is in the never-allowed list. It carries the
// same runtime settings as svd-monitor (cmd/svd-monitor/main.go), so that R_go
// is the list of the monitor's own runtime.
//
//go:debug containermaxprocs=0
//go:debug updatemaxprocs=0
package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: refprog <stub>")
		os.Exit(2)
	}
	runtime.GOMAXPROCS(2)

	// The scheduler and its preemption: more runnable goroutines than Ps, each
	// running past the 10 ms of sysmon's time slice.
	var wg sync.WaitGroup
	stop := time.Now().Add(120 * time.Millisecond)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := 0
			for time.Now().Before(stop) {
				n++
			}
			_ = n
		}()
	}
	wg.Wait()

	// The garbage collector and the scavenger.
	var keep [][]byte
	for i := 0; i < 32; i++ {
		keep = append(keep, make([]byte, 1<<20))
	}
	runtime.GC()
	keep = nil
	runtime.GC()
	debug.FreeOSMemory()

	// Timers.
	t := time.NewTimer(5 * time.Millisecond)
	tk := time.NewTicker(2 * time.Millisecond)
	<-t.C
	<-tk.C
	tk.Stop()
	time.Sleep(3 * time.Millisecond)

	// The netpoller.
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	go func() {
		time.Sleep(2 * time.Millisecond)
		_, _ = w.Write([]byte("x"))
	}()
	buf := make([]byte, 1)
	_ = r.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = r.Read(buf)
	_ = r.Close()
	_ = w.Close()

	// os/signal: handlers installed and removed, no signal sent.
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1, syscall.SIGHUP)
	signal.Stop(c)

	// syscall.ForkExec of a stub, reaped.
	pid, err := syscall.ForkExec(os.Args[1], []string{os.Args[1]}, &syscall.ProcAttr{Files: []uintptr{0, 1, 2}})
	if err != nil {
		panic(err)
	}
	var ws syscall.WaitStatus
	if _, err := syscall.Wait4(pid, &ws, 0, nil); err != nil {
		panic(err)
	}
	fmt.Fprintln(os.Stderr, "refprog: done", len(keep))
}
