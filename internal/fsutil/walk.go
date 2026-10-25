// Package fsutil holds filesystem-level primitives shared by push and
// pull: a filtered, ignore-aware directory walk, and (in atomic.go /
// icloud_darwin.go) the atomic staging/swap and iCloud-placeholder
// helpers. Kept independent of internal/sync so it can be exercised
// against a plain temp directory with no knowledge of manifests or
// vaults.
package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Entry is one file discovered by Walk. It carries only filesystem
// facts; internal/sync layers manifest/hash bookkeeping on top of this.
type Entry struct {
	RelPath string // slash-separated, relative to root
	Size    int64
	ModTime int64 // unix nanoseconds
}

// IgnoreChecker is satisfied by internal/ignore.Matcher. Declared here
// (rather than imported) to keep fsutil free of a dependency on the
// ignore package's gitignore backend.
type IgnoreChecker interface {
	IsIgnored(relPath string, isDir bool) bool
}

// Walk traverses root and returns every non-ignored regular file as an
// Entry. Ignored directories are pruned entirely — the walk never
// descends into them — both so push never stats a 100k-file
// node_modules tree, and so a deliberately-unreadable file inside an
// ignored directory can't fail the walk.
func Walk(root string, ignore IgnoreChecker) ([]Entry, error) {
	var entries []Entry

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		if d.IsDir() {
			if ignore.IsIgnored(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip symlinks: beam copies real file content, and a symlink
		// into an ignored tree (or outside the repo) is not something
		// push should try to resolve and sync.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		if ignore.IsIgnored(rel, false) {
			return nil
		}

		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}

		entries = append(entries, Entry{
			RelPath: filepath.ToSlash(rel),
			Size:    info.Size(),
			ModTime: info.ModTime().UnixNano(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// EnsureDir is a small helper used by both push (staging) and pull
// (destination parent dirs) to create a directory if it doesn't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
