// Command svd: the supervisor. Only the internal `helper` entry point exists
// in this change (SUPERVISOR.md 7.1.1, steps H1-H10).
package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/go-supervise/supervisor/internal/helper"
)

var (
	osExit    = os.Exit
	runHelper = helper.Run
)

// RootCmd builds the command tree.
func RootCmd() *cobra.Command {
	root := &cobra.Command{Use: "svd", SilenceUsage: true}
	root.AddCommand(&cobra.Command{
		Use:    "helper",
		Short:  "internal: the exec-helper (H1-H10), started by the monitor only",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if code := runHelper(cmd.ErrOrStderr()); code != 0 {
				osExit(code)
			}
			return nil
		},
	})
	return root
}

func main() {
	if err := RootCmd().Execute(); err != nil {
		osExit(1)
	}
}
