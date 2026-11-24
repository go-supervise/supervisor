//go:build linux && canary_new

package monitor

import "golang.org/x/sys/unix"

// canary plants a call in none of A_M, R_go and Never: sysinfo(2).
func canary() {
	var si unix.Sysinfo_t
	_ = unix.Sysinfo(&si)
}
