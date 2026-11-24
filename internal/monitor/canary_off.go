//go:build linux && !canary_never && !canary_new && !canary_rawnr && !canary_forkchild && !canary_forkclone

package monitor

// canary is the hook of the negative control of SUPERVISOR.md 7.1.2
// (TestMonitorSyscallSurfaceCatches): it runs on the path of the first
// record, and the canary_* build tags plant one forbidden call in it. In a
// shipped build it is empty.
func canary() {}
