package sync

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jayprio/beam-cli/internal/errs"
	"github.com/jayprio/beam-cli/internal/fsutil"
)

// ConflictMode controls what Pull does with a path that changed on
// both sides since the last known sync.
type ConflictMode int

const (
	// ConflictSkip leaves the local file untouched and records the
	// conflict in the report. Default behavior.
	ConflictSkip ConflictMode = iota
	// ConflictForce overwrites the local file with the vault's version.
	ConflictForce
)

// PullOptions configures a Pull run.
type PullOptions struct {
	RepoRoot     string
	VaultPath    string
	LastSynced   Manifest
	ConflictMode ConflictMode
	DryRun       bool
}

// PullResult mirrors PushResult: the computed plan and, on success,
// the manifest to persist as the new last-known-synced state.
type PullResult struct {
	Plan     Plan
	Manifest Manifest
	Report   *errs.Report
}

// Pull reads the vault's current/ tree, diffs it against the local
// working tree and the last-known-synced manifest (DiffPull's
// three-way comparison), then — unless DryRun — materializes every
// add/update into the local repo with a per-file atomic write.
// Conflicts are never auto-merged: ConflictSkip (default) leaves the
// local file alone, ConflictForce lets the vault win.
//
// Pull never deletes local files: a path present locally but absent
// from the vault manifest is left untouched, by construction, since
// the plan is built by iterating the vault's manifest, not the
// local tree's.
func Pull(opts PullOptions, ignore fsutil.IgnoreChecker) (*PullResult, error) {
	report := &errs.Report{}

	currentDir := filepath.Join(opts.VaultPath, "current")
	vaultManifest, err := BuildManifest(currentDir, ignore, nil)
	if err != nil {
		report.Fatal = fmt.Errorf("walking vault: %w", err)
		return &PullResult{Report: report}, report.Fatal
	}

	localManifest, err := BuildManifest(opts.RepoRoot, ignore, opts.LastSynced)
	if err != nil {
		report.Fatal = fmt.Errorf("walking repo: %w", err)
		return &PullResult{Report: report}, report.Fatal
	}

	plan := DiffPull(vaultManifest, localManifest, opts.LastSynced)

	if opts.DryRun {
		return &PullResult{Plan: plan, Manifest: localManifest, Report: report}, nil
	}

	resultManifest := make(Manifest, len(localManifest))
	for k, v := range localManifest {
		resultManifest[k] = v
	}

	for _, entry := range plan {
		switch entry.Action {
		case ActionConflict:
			report.AddConflict(entry.Path)
			if opts.ConflictMode != ConflictForce {
				continue
			}
			fallthrough
		case ActionAdd, ActionUpdate:
			if err := materialize(currentDir, opts.RepoRoot, entry.Path); err != nil {
				report.AddError(entry.Path, "pull-write", err)
				continue
			}
			resultManifest[entry.Path] = vaultManifest[entry.Path]
		case ActionSkip:
			// nothing to do
		}
	}

	return &PullResult{Plan: plan, Manifest: resultManifest, Report: report}, nil
}

func materialize(vaultCurrent, repoRoot, relPath string) error {
	src := filepath.Join(vaultCurrent, relPath)
	dst := filepath.Join(repoRoot, relPath)

	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	return fsutil.WriteFileAtomic(dst, f, info.Mode().Perm())
}
