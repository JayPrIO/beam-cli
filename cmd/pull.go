package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/jayprio/beam-cli/internal/config"
	"github.com/jayprio/beam-cli/internal/ignore"
	"github.com/jayprio/beam-cli/internal/registry"
	"github.com/jayprio/beam-cli/internal/sync"
)

func newPullCmd() *cobra.Command {
	var (
		dryRun      bool
		force       bool
		preferLocal bool
	)

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Sync new/updated files down from the iCloud vault",
		Long: "Reads the project's iCloud vault and merges new/updated files into " +
			"the local repo, respecting .beamignore and never deleting local files. " +
			"A path changed on both sides since the last sync is a conflict: by " +
			"default it's left alone and reported, --force lets the vault win.",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := os.Getwd()
			if err != nil {
				return newCmdError(1, err)
			}

			project, err := config.Load(repoRoot)
			if err != nil {
				return newCmdError(1, fmt.Errorf("not a beam project (run `beam init` first): %w", err))
			}

			matcher, err := ignore.Load(repoRoot)
			if err != nil {
				return newCmdError(1, fmt.Errorf("loading .beamignore: %w", err))
			}

			paths := config.ResolvePaths(repoRoot)
			lastSynced, err := sync.LoadManifest(paths.Manifest)
			if err != nil {
				return newCmdError(1, fmt.Errorf("loading last-synced manifest: %w", err))
			}

			conflictMode := sync.ConflictSkip
			if force {
				conflictMode = sync.ConflictForce
			}
			_ = preferLocal // same effective behavior as the default; kept as an explicit, documented alias

			result, pullErr := sync.Pull(sync.PullOptions{
				RepoRoot:     repoRoot,
				VaultPath:    project.VaultPath,
				LastSynced:   lastSynced,
				ConflictMode: conflictMode,
				DryRun:       dryRun,
			}, matcher)

			printPlan(result.Plan, dryRun)

			if pullErr != nil {
				return newCmdError(1, pullErr)
			}

			if dryRun {
				return nil
			}

			if err := sync.SaveManifest(paths.Manifest, result.Manifest); err != nil {
				return newCmdError(1, fmt.Errorf("saving manifest: %w", err))
			}
			now := time.Now().UTC()
			project.LastPullAt = &now
			if err := project.Save(repoRoot); err != nil {
				return newCmdError(1, fmt.Errorf("updating .beam/config.json: %w", err))
			}

			// Best-effort, same reasoning as push: registry activity
			// tracking must never fail an otherwise-successful pull.
			if hostname, hostErr := os.Hostname(); hostErr == nil {
				vaultRoot := filepath.Dir(project.VaultPath)
				if err := registry.RecordPull(vaultRoot, project.ProjectID, hostname, now); err != nil {
					fmt.Fprintf(os.Stderr, "warning: couldn't update project registry: %v\n", err)
				}
			}

			if result.Report.HasConflicts() {
				printConflicts(result.Report)
				return newCmdError(3, fmt.Errorf("%d file(s) skipped due to local changes — use --force to overwrite, or push first", len(result.Report.Conflicts)))
			}
			if result.Report.HasErrors() {
				printFileErrors(result.Report)
				return newCmdError(2, fmt.Errorf("pull completed with %d file error(s)", len(result.Report.Errors)))
			}

			fmt.Println("pull complete")
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what pull would do without writing anything")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite local files on conflict (vault wins)")
	cmd.Flags().BoolVar(&preferLocal, "prefer-local", false, "explicit alias for the default: skip conflicting files, keep local")

	return cmd
}
