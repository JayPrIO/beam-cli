//go:build darwin

package fsutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// IsDataless reports whether path is an evicted iCloud placeholder. A
// dataless file's real name disappears from the directory listing and
// is replaced by a zero-byte sibling named ".<name>.icloud" — this is
// the detection reachable without linking the FileProvider framework.
func IsDataless(path string) bool {
	dir, name := filepath.Split(path)
	placeholder := filepath.Join(dir, "."+name+".icloud")
	_, err := os.Stat(placeholder)
	return err == nil
}

// MaterializeTimeout bounds how long Materialize waits for iCloud to
// download a file before giving up. 30s matches the plan: long enough
// for a typical source file over a normal connection, short enough
// that one slow/stuck file doesn't hang an entire pull.
const MaterializeTimeout = 30 * time.Second

// Materialize triggers a download of a dataless iCloud file and blocks
// until it's available locally or MaterializeTimeout elapses. It
// shells out to `brctl download` as a best-effort nudge (undocumented
// but commonly relied on), then polls IsDataless — brctl's own exit
// status isn't a reliable download-complete signal, so the poll is the
// actual wait condition.
func Materialize(path string) error {
	if !IsDataless(path) {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), MaterializeTimeout)
	defer cancel()

	// Best-effort: ignore errors (binary may be absent/renamed on some
	// macOS versions), rely on the poll loop below either way.
	_ = exec.CommandContext(ctx, "brctl", "download", path).Run()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !IsDataless(path) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("file not yet downloaded from iCloud after %s — run pull again: %s", MaterializeTimeout, path)
		case <-ticker.C:
		}
	}
}

// IsPermissionDenied is a thin, named wrapper around os.IsPermission
// so call sites in sync/push.go and sync/pull.go can classify an error
// without importing os directly for this one check.
func IsPermissionDenied(err error) bool {
	return os.IsPermission(err) || strings.Contains(err.Error(), "permission denied")
}
