// Package sync builds on internal/fsutil's filtered walk to produce
// manifests (per-file size/mtime/hash snapshots), diff two manifests
// into a Plan, and (in push.go / pull.go) execute that plan against the
// vault.
package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/kaihiwatari8/beam-cli/internal/fsutil"
)

// FileState is one path's recorded state: enough to detect whether it
// changed without re-reading the file, plus a hash to confirm a real
// content change when size+mtime alone are ambiguous (e.g. after a
// plain `cp` or git checkout resets mtimes).
type FileState struct {
	Size    int64  `json:"size"`
	ModTime int64  `json:"mtime"`
	SHA256  string `json:"sha256,omitempty"`
}

// Manifest maps a slash-separated relative path to its last-known
// state. It is what gets persisted at .beam/manifest.json after every
// push/pull, and what gets built fresh (via BuildManifest) from a
// filesystem walk to compare against that persisted state.
type Manifest map[string]FileState

// BuildManifest walks root (via fsutil.Walk) and hashes every file to
// produce a complete, authoritative manifest. prev, if non-nil, is the
// last-known-synced manifest: when a path's size and mtime match prev
// exactly, the existing hash is reused instead of re-reading the file,
// since nothing observable about it has changed.
func BuildManifest(root string, ignore fsutil.IgnoreChecker, prev Manifest) (Manifest, error) {
	entries, err := fsutil.Walk(root, ignore)
	if err != nil {
		return nil, err
	}

	m := make(Manifest, len(entries))
	for _, e := range entries {
		if prior, ok := prev[e.RelPath]; ok && prior.Size == e.Size && prior.ModTime == e.ModTime && prior.SHA256 != "" {
			m[e.RelPath] = prior
			continue
		}

		sum, err := hashFile(root, e.RelPath)
		if err != nil {
			return nil, err
		}
		m[e.RelPath] = FileState{Size: e.Size, ModTime: e.ModTime, SHA256: sum}
	}
	return m, nil
}

func hashFile(root, relPath string) (string, error) {
	f, err := os.Open(filepath.Join(root, relPath))
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// LoadManifest reads a persisted manifest from path. A missing file is
// not an error: it just means there's no prior sync to compare against
// (e.g. first push, or a fresh --join).
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Manifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// SaveManifest persists m to path as JSON.
func SaveManifest(path string, m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
