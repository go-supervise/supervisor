// Fixture of the source half: the allowed constructor steered through a struct
// of the layout of SysProcAttr whose tags differ, which Go converts to it.
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
