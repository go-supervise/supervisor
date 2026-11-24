// Command syscallsurface computes and checks M_p's system-call surface
// (SUPERVISOR.md 7.1.2): `monitor` runs the source half, `symbols` the
// symbol-absence half, and, on Linux, `rgo` regenerates R_go; the hidden
// `launch` and `helper` entry points are the dynamic half's launcher and stub
// helper.
package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/go-supervise/supervisor/internal/surface"
)

var osExit = os.Exit

func report(cmd *cobra.Command, r *surface.Result) error {
	for _, n := range r.Reached {
		fmt.Fprintln(cmd.OutOrStdout(), "reached", n)
	}
	for _, f := range r.Failures {
		fmt.Fprintln(cmd.ErrOrStderr(), f)
	}
	if len(r.Failures) > 0 {
		return fmt.Errorf("%d failure(s)", len(r.Failures))
	}
	return nil
}

// RootCmd builds the command tree.
func RootCmd() *cobra.Command {
	root := &cobra.Command{Use: "syscallsurface", SilenceUsage: true}

	var o surface.Options
	var why []string
	mon := &cobra.Command{
		Use:   "monitor",
		Short: "source half: the system calls reachable from svd-monitor's main",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := surface.SourceHalf(o)
			if err != nil {
				return err
			}
			for _, w := range why {
				fmt.Fprintf(cmd.OutOrStdout(), "why %s: %s\n", w, strings.Join(r.Chains[w], " -> "))
			}
			return report(cmd, r)
		},
	}
	mon.Flags().StringSliceVar(&why, "why", nil, "print one call path reaching each named system call")
	mon.Flags().StringVar(&o.Dir, "dir", ".", "module root")
	mon.Flags().StringVar(&o.Entry, "entry", "./cmd/svd-monitor", "entry package")
	mon.Flags().StringVar(&o.GOARCH, "arch", runtime.GOARCH, "amd64 or arm64")
	mon.Flags().StringSliceVar(&o.Tags, "tags", nil, "build tags")
	root.AddCommand(mon)

	var bin string
	sym := &cobra.Command{
		Use:   "symbols",
		Short: "symbol-absence half: none of the committed symbols may be linked into the binary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := surface.SymbolAbsence(bin)
			if err != nil {
				return err
			}
			return report(cmd, r)
		},
	}
	sym.Flags().StringVar(&bin, "binary", "", "the svd-monitor executable")
	_ = sym.MarkFlagRequired("binary")
	root.AddCommand(sym)

	var snDir, snArch, snOut string
	var snCheck bool
	sn := &cobra.Command{
		Use:   "sysnum",
		Short: "regenerate (or --check) the committed system-call number table of an architecture",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			txt, err := surface.GenerateSysNum(snDir, snArch)
			if err != nil {
				return err
			}
			if snCheck {
				have, err := os.ReadFile(snOut)
				if err != nil {
					return err
				}
				if string(have) != txt {
					return fmt.Errorf("%s is out of date: run `syscallsurface sysnum --arch %s --out %s`", snOut, snArch, snOut)
				}
				return nil
			}
			if snOut == "" {
				_, err = fmt.Fprint(cmd.OutOrStdout(), txt)
				return err
			}
			return os.WriteFile(snOut, []byte(txt), 0o644)
		},
	}
	sn.Flags().StringVar(&snDir, "dir", ".", "module root")
	sn.Flags().StringVar(&snArch, "arch", runtime.GOARCH, "amd64 or arm64")
	sn.Flags().StringVar(&snOut, "out", "", "file to write (default stdout); with --check, the file to compare")
	sn.Flags().BoolVar(&snCheck, "check", false, "fail when --out differs from the regenerated table")
	root.AddCommand(sn)

	addPlatform(root)
	return root
}

func main() {
	if err := RootCmd().Execute(); err != nil {
		osExit(1)
	}
}
