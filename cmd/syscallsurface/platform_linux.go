//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/go-supervise/supervisor/internal/surface"
)

// addPlatform adds the Linux-only commands: the hidden `launch` of the dynamic
// half (installs the filter, then execs the monitor) and `rgo`, which
// regenerates (or --check) R_go from a recording of the reference program.
func addPlatform(root *cobra.Command) {
	root.AddCommand(&cobra.Command{
		Use:    "launch <monitor>",
		Short:  "install the seccomp filter, then exec svd-monitor (dynamic half)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, a []string) error {
			return surface.Launch(runtime.GOARCH, a[0])
		},
	})

	var arch, refprog, stub, out string
	var check bool
	rgo := &cobra.Command{
		Use:   "rgo",
		Short: "regenerate (or --check) R_go, the Go runtime's own system calls, from the reference program",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if arch != runtime.GOARCH {
				return fmt.Errorf("rgo records the reference program and so runs on its own architecture (%s), not %s", runtime.GOARCH, arch)
			}
			rec, err := surface.Record(exec.Command(refprog, stub), arch, 0, time.Minute, nil)
			if err != nil {
				return err
			}
			if rec.Signal != 0 || rec.Exit != 0 {
				return fmt.Errorf("the reference program ended with status %d, signal %d", rec.Exit, rec.Signal)
			}
			txt, err := surface.GenerateRGo(arch, rec.Calls)
			if err != nil {
				return err
			}
			if check {
				have, err := os.ReadFile(out)
				if err != nil {
					return err
				}
				if string(have) != txt {
					return fmt.Errorf("%s is out of date: run `syscallsurface rgo --arch %s --out %s`", out, arch, out)
				}
				return nil
			}
			if out == "" {
				_, err = fmt.Fprint(cmd.OutOrStdout(), txt)
				return err
			}
			return os.WriteFile(out, []byte(txt), 0o644)
		},
	}
	rgo.Flags().StringVar(&arch, "arch", runtime.GOARCH, "amd64 or arm64 (the host's)")
	rgo.Flags().StringVar(&refprog, "refprog", "", "the reference program executable")
	rgo.Flags().StringVar(&stub, "stub", "", "the stub helper executable")
	rgo.Flags().StringVar(&out, "out", "", "file to write (default stdout); with --check, the file to compare")
	rgo.Flags().BoolVar(&check, "check", false, "fail when --out differs from the regenerated list")
	_ = rgo.MarkFlagRequired("refprog")
	_ = rgo.MarkFlagRequired("stub")
	root.AddCommand(rgo)
}
