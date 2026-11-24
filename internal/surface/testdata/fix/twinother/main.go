// Fixture of the source half: the same conversion as twinattr, in a function
// that is not the allowed constructor.
package main

import "syscall"

type twin struct {
	Chroot                     string `x:"y"`
	Credential                 *syscall.Credential
	Ptrace                     bool
	Setsid                     bool
	Setpgid                    bool
	Setctty                    bool
	Noctty                     bool
	Ctty                       int
	Foreground                 bool
	Pgid                       int
	Pdeathsig                  syscall.Signal
	Cloneflags                 uintptr
	Unshareflags               uintptr
	UidMappings                []syscall.SysProcIDMap
	GidMappings                []syscall.SysProcIDMap
	GidMappingsEnableSetgroups bool
	AmbientCaps                []uintptr
	UseCgroupFD                bool
	CgroupFD                   int
	PidFD                      *int
}

func configure(t *twin) *syscall.ProcAttr {
	return &syscall.ProcAttr{Sys: (*syscall.SysProcAttr)(t)}
}

func main() { _ = configure(&twin{Chroot: "/x", Ptrace: true}) }
