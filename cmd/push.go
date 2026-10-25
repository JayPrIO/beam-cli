package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/kaihiwatari8/beam-cli/internal/config"
	"github.com/kaihiwatari8/beam-cli/internal/ignore"
	"github.com/kaihiwatari8/beam-cli/internal/registry"
	"github.com/kaihiwatari8/beam-cli/internal/sync"
)

func newPushCmd() *cobra.Command {
	var dryRun bool
	var force bool

	cmd := &cobra.Command{
		Use:   "push",
		Short: "Copy tracked files into the iCloud vault",
		Long: "Copies files not matched by .beamignore into the project's iCloud " +
			"vault using an atomic stage-and-swap: files are copied into a fresh " +
			"staging directory first, then swapped into place with a directory " +
			"rename, so an interrupted push never leaves the vault in a half-written " +
			"state.",
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

			var lock *config.Lock
			if !dryRun {
				if force {
					_ = os.Remove(paths.Lock)
				}
				lock, err = config.Acquire(paths.Lock)
				if err != nil {
					return newCmdError(1, err)
				}
				defer lock.Release()
			}

			result, pushErr := sync.Push(sync.PushOptions{
				RepoRoot:   repoRoot,
				VaultPath:  project.VaultPath,
				LastSynced: lastSynced,
				DryRun:     dryRun,
			}, matcher)

			printPlan(result.Plan, dryRun)

			if pushErr != nil {
				return newCmdError(1, pushErr)
			}

			if dryRun {
				return nil
			}

			if err := sync.SaveManifest(paths.Manifest, result.Manifest); err != nil {
				return newCmdError(1, fmt.Errorf("saving manifest: %w", err))
			}
			now := time.Now().UTC()
			project.LastPushAt = &now
			if err := project.Save(repoRoot); err != nil {
				return newCmdError(1, fmt.Errorf("updating .beam/config.json: %w", err))
			}

			// Best-effort: the registry is the fleet-wide activity
			// view (`beam list`), but a failure to update it must
			// never fail an otherwise-successful push — the push
			// already landed in the vault.
			if hostname, hostErr := os.Hostname(); hostErr == nil {
				vaultRoot := filepath.Dir(project.VaultPath)
				if err := registry.RecordPush(vaultRoot, project.ProjectID, hostname, now); err != nil {
					fmt.Fprintf(os.Stderr, "warning: couldn't update project registry: %v\n", err)
				}
			}

			if result.Report.HasErrors() {
				printFileErrors(result.Report)
				return newCmdError(2, fmt.Errorf("push completed with %d file error(s)", len(result.Report.Errors)))
			}

			fmt.Println("push complete")
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what push would do without writing anything")
	cmd.Flags().BoolVar(&force, "force", false, "clear a stale push lock before starting (does not override a push actively running)")
	return cmd
}
