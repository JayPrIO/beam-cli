package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"

	"github.com/jayprio/beam-cli/internal/config"
	"github.com/jayprio/beam-cli/internal/ignore"
	"github.com/jayprio/beam-cli/internal/registry"
)

// pickSentinel is the NoOptDefVal for --join: it lets `--join` appear
// with no value to mean "show me a picker" while `--join <id>` still
// means "attach to this exact project id".
const pickSentinel = "\x00pick"

func newInitCmd() *cobra.Command {
	var (
		force        bool
		forceIgnore  bool
		join         string
		nameOverride string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Args:  cobra.MaximumNArgs(1), // tolerates `--join <id>`'s stray positional arg; see RunE
		Short: "Initialize this directory as a beam project and provision its iCloud vault",
		Long: "Scans the current directory, writes a default .beamignore (unless one " +
			"already exists), creates .beam/ with a project ID, and provisions a " +
			"matching vault folder in iCloud Drive at " +
			"~/Library/Mobile Documents/com~apple~CloudDocs/beam-vaults/<name>-<id>/. " +
			"Running init again is a no-op unless --force is given. Use --join <id> " +
			"on a second machine to attach to an existing vault instead of creating a " +
			"new one, or just --join (no value) to pick one from a list of every " +
			"project registered in iCloud.",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := os.Getwd()
			if err != nil {
				return newCmdError(1, err)
			}

			if config.Exists(repoRoot) && !force {
				existing, err := config.Load(repoRoot)
				if err != nil {
					return newCmdError(1, fmt.Errorf("reading existing .beam/config.json: %w", err))
				}
				fmt.Printf("already a beam project (project id %s)\nvault: %s\n", existing.ProjectID, existing.VaultPath)
				return nil
			}

			projectName := nameOverride
			if projectName == "" {
				projectName = filepath.Base(repoRoot)
			}

			var vaultPath string
			var projectID string

			if join != "" {
				targetID := join
				if join == pickSentinel {
					// `--join 69b98c3f` (space form) is parsed by pflag
					// as bare --join (NoOptDefVal) plus a positional
					// arg, not as --join taking that value — only
					// --join=<id> does. Accept the positional form too
					// so the space-separated invocation still works as
					// a direct join instead of surprising the user with
					// a picker.
					if len(args) > 0 {
						targetID = args[0]
					} else {
						picked, err := pickProject()
						if err != nil {
							return newCmdError(1, err)
						}
						targetID = picked.ProjectID
					}
				}

				foundPath, meta, err := config.FindVaultByID(targetID)
				if err != nil {
					return newCmdError(1, fmt.Errorf("--join %s: %w", targetID, err))
				}
				vaultPath = foundPath
				projectID = meta.ProjectID
				projectName = meta.ProjectName
			} else {
				id, err := config.NewProjectID()
				if err != nil {
					return newCmdError(1, err)
				}
				projectID = id

				vaultRoot, err := config.VaultRoot()
				if err != nil {
					return newCmdError(1, err)
				}
				vaultPath = config.VaultPath(vaultRoot, projectName, projectID)

				hostname, _ := os.Hostname()
				meta := config.VaultMeta{
					ProjectID:     projectID,
					ProjectName:   projectName,
					CreatedByHost: hostname,
					CreatedAt:     time.Now().UTC(),
				}
				if err := config.ProvisionVault(vaultPath, meta, force); err != nil {
					return newCmdError(1, err)
				}

				// Record this project in the central registry so other
				// machines' `beam init --join` picker can discover it
				// without the user needing to already know the id.
				// Registry writes are best-effort: a failure here must
				// not block init, since --join <id> (directory scan via
				// vault-meta.json) still works without the registry.
				if err := registry.Register(vaultRoot, registry.Entry{
					ProjectID:     projectID,
					ProjectName:   projectName,
					VaultPath:     vaultPath,
					CreatedByHost: meta.CreatedByHost,
					CreatedAt:     meta.CreatedAt,
				}); err != nil {
					fmt.Fprintf(os.Stderr, "warning: couldn't update project registry: %v\n", err)
				}
			}

			ignorePath := filepath.Join(repoRoot, ignore.Filename)
			if _, statErr := os.Stat(ignorePath); statErr != nil {
				if err := os.WriteFile(ignorePath, ignore.DefaultBeamignore(), 0o644); err != nil {
					return newCmdError(1, fmt.Errorf("writing %s: %w", ignore.Filename, err))
				}
			} else if forceIgnore {
				if err := os.WriteFile(ignorePath, ignore.DefaultBeamignore(), 0o644); err != nil {
					return newCmdError(1, fmt.Errorf("rewriting %s: %w", ignore.Filename, err))
				}
			} else {
				fmt.Printf("%s already exists, leaving it untouched (use --force-ignore to overwrite)\n", ignore.Filename)
			}

			project := &config.Project{
				SchemaVersion: 1,
				ProjectName:   projectName,
				ProjectID:     projectID,
				VaultPath:     vaultPath,
				CreatedAt:     time.Now().UTC(),
			}
			if err := project.Save(repoRoot); err != nil {
				return newCmdError(1, fmt.Errorf("writing .beam/config.json: %w", err))
			}

			fmt.Printf("initialized beam project %q\nproject id: %s\nvault:      %s\nignore:     %s\n",
				projectName, projectID, vaultPath, ignorePath)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "re-provision even if this directory is already a beam project")
	cmd.Flags().BoolVar(&forceIgnore, "force-ignore", false, "overwrite an existing .beamignore with beam's defaults")
	cmd.Flags().StringVar(&join, "join", "", "attach to an existing vault by project id; pass with no value to pick from a list")
	cmd.Flags().Lookup("join").NoOptDefVal = pickSentinel
	cmd.Flags().StringVar(&nameOverride, "name", "", "override the project name (defaults to the directory's basename)")

	return cmd
}

// pickProject shows an arrow-key select of every project in the
// central registry (internal/registry) and returns the chosen entry.
func pickProject() (*registry.Entry, error) {
	vaultRoot, err := config.VaultRoot()
	if err != nil {
		return nil, err
	}
	entries, err := registry.List(vaultRoot)
	if err != nil {
		return nil, fmt.Errorf("reading project registry: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no beamed projects found in the registry — pass --join <id> directly, or run `beam init` fresh")
	}

	labels := make([]string, len(entries))
	for i, e := range entries {
		labels[i] = fmt.Sprintf("%-24s %s  (from %s)", e.ProjectName, e.ProjectID, e.CreatedByHost)
	}

	sel := promptui.Select{
		Label: "Select a beamed project to join",
		Items: labels,
	}
	idx, _, err := sel.Run()
	if err != nil {
		return nil, fmt.Errorf("selection cancelled: %w", err)
	}
	return &entries[idx], nil
}
