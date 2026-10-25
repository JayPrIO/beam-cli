package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/kaihiwatari8/beam-cli/internal/config"
	"github.com/kaihiwatari8/beam-cli/internal/registry"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every beamed project registered in iCloud",
		Long: "Reads internal/registry's SQLite index at " +
			"<vaultRoot>/registry.db directly — the same source `beam init --join` " +
			"picks from — and prints each project's id, creator, and last push/pull " +
			"activity across every machine that has synced it.",
		RunE: func(cmd *cobra.Command, args []string) error {
			vaultRoot, err := config.VaultRoot()
			if err != nil {
				return newCmdError(1, err)
			}

			entries, err := registry.List(vaultRoot)
			if err != nil {
				return newCmdError(1, fmt.Errorf("reading registry: %w", err))
			}
			if len(entries) == 0 {
				fmt.Println("no beamed projects registered yet")
				return nil
			}

			for _, e := range entries {
				fmt.Printf("%-24s %s\n", e.ProjectName, e.ProjectID)
				fmt.Printf("  created: %s (%s)\n", formatAgo(&e.CreatedAt), e.CreatedByHost)
				fmt.Printf("  pushed:  %s%s\n", formatAgo(e.LastPushedAt), hostSuffix(e.LastPushedHost))
				fmt.Printf("  pulled:  %s%s\n", formatAgo(e.LastPulledAt), hostSuffix(e.LastPulledHost))
			}
			return nil
		},
	}
	return cmd
}

func formatAgo(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func hostSuffix(host string) string {
	if host == "" {
		return ""
	}
	return " (" + host + ")"
}
