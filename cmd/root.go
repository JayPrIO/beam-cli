// Package cmd wires beam's cobra command tree (init/push/pull) and
// translates internal/errs.Report outcomes into beam's process exit
// code contract: 0 success, 1 fatal abort, 2 partial per-file errors,
// 3 pull conflicts requiring user action.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var verbose bool

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "beam",
		Short:         "Sync active repository code between Macs via iCloud Drive",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "print per-file add/update/skip/ignore actions")

	root.AddCommand(newInitCmd())
	root.AddCommand(newPushCmd())
	root.AddCommand(newPullCmd())
	root.AddCommand(newListCmd())

	return root
}

// Execute runs the root command and returns the process exit code,
// per the contract above.
func Execute() int {
	root := newRootCmd()
	root.SilenceErrors = true

	if err := root.Execute(); err != nil {
		if ce, ok := err.(*cmdError); ok {
			if ce.err != nil {
				fmt.Fprintln(os.Stderr, "beam:", ce.err)
			}
			return ce.exitCode
		}
		fmt.Fprintln(os.Stderr, "beam:", err)
		return 1
	}
	return 0
}

// cmdError lets a subcommand's RunE return a specific exit code (2 for
// partial per-file errors, 3 for pull conflicts) instead of the
// generic fatal-error code 1 that cobra would otherwise imply.
type cmdError struct {
	exitCode int
	err      error
}

func (e *cmdError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func newCmdError(code int, err error) error {
	return &cmdError{exitCode: code, err: err}
}
