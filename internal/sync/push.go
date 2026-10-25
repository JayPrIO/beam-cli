package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kaihiwatari8/beam-cli/internal/errs"
	"github.com/kaihiwatari8/beam-cli/internal/fsutil"
)

// PushOptions configures a Push run.
type PushOptions struct {
	RepoRoot   string
	VaultPath  string
	LastSynced Manifest // from .beam/manifest.json; nil/empty on first push
	DryRun     bool
}

// PushResult is returned by Push: the plan it computed (useful for
// --dry-run and --verbose output) and the manifest to persist as the
// new last-known-synced state on success.
type PushResult struct {
	Plan     Plan
	Manifest Manifest
	Report   *errs.Report
}

// Push builds the local manifest, diffs it against LastSynced, and —
// unless DryRun — stages every non-ignored file into a fresh directory
// inside the vault and atomically swaps it in as `current/`. See
// fsutil.SwapDir for the atomicity guarantee.
func Push(opts PushOptions, ignore fsutil.IgnoreChecker) (*PushResult, error) {
	report := &errs.Report{}

	localManifest, err := BuildManifest(opts.RepoRoot, ignore, opts.LastSynced)
	if err != nil {
		report.Fatal = fmt.Errorf("walking repo: %w", err)
		return &PushResult{Report: report}, report.Fatal
	}

	plan := DiffPush(localManifest, opts.LastSynced)

	if opts.DryRun {
		return &PushResult{Plan: plan, Manifest: localManifest, Report: report}, nil
	}

	staging := filepath.Join(opts.VaultPath, stagingDirName())
	if err := fsutil.EnsureDir(staging); err != nil {
		report.Fatal = fmt.Errorf("creating staging dir: %w", err)
		return &PushResult{Report: report}, report.Fatal
	}

	for path := range localManifest {
		src := filepath.Join(opts.RepoRoot, path)
		dst := filepath.Join(staging, path)
		if err := fsutil.CopyFile(src, dst); err != nil {
			// Partial-copy or read failure on one file aborts the whole
			// push: staging is meant to become the new vault state, and
			// a push with a silently-missing file is worse than one
			// that fails loudly and can be retried.
			_ = os.RemoveAll(staging)
			report.Fatal = fmt.Errorf("copying %s: %w", path, err)
			return &PushResult{Report: report}, report.Fatal
		}
	}

	current := filepath.Join(opts.VaultPath, "current")
	if err := fsutil.SwapDir(staging, current); err != nil {
		_ = os.RemoveAll(staging)
		report.Fatal = fmt.Errorf("swapping staged content into place: %w", err)
		return &PushResult{Report: report}, report.Fatal
	}

	return &PushResult{Plan: plan, Manifest: localManifest, Report: report}, nil
}

func stagingDirName() string {
	return ".staging-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}
