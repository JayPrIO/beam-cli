// Package config manages beam's per-project local state (.beam/) and
// the iCloud vault it points at: the Project record, manifest path
// resolution, and vault provisioning/lookup (vault.go).
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Dir is beam's hidden local state directory at the repo root.
const Dir = ".beam"

// configFile and manifestFile are the two files kept under Dir. They
// are split (rather than one file) so config.json stays small and
// stable while manifest.json is rewritten after every push/pull.
const (
	configFile    = "config.json"
	manifestFile  = "manifest.json"
	lockFile      = "push.lock"
	schemaVersion = 1
)

// Project is the persisted contents of .beam/config.json.
type Project struct {
	SchemaVersion int        `json:"schema_version"`
	ProjectName   string     `json:"project_name"`
	ProjectID     string     `json:"project_id"`
	VaultPath     string     `json:"vault_path"`
	CreatedAt     time.Time  `json:"created_at"`
	LastPushAt    *time.Time `json:"last_push_at,omitempty"`
	LastPullAt    *time.Time `json:"last_pull_at,omitempty"`
}

// Paths resolves the on-disk locations of beam's state files given a
// repo root.
type Paths struct {
	Dir      string
	Config   string
	Manifest string
	Lock     string
}

func ResolvePaths(repoRoot string) Paths {
	dir := filepath.Join(repoRoot, Dir)
	return Paths{
		Dir:      dir,
		Config:   filepath.Join(dir, configFile),
		Manifest: filepath.Join(dir, manifestFile),
		Lock:     filepath.Join(dir, lockFile),
	}
}

// Load reads .beam/config.json. A missing directory/file means this
// repo has not been beam-initialized; callers should treat that as a
// distinct, user-facing error ("run `beam init` first"), not a generic
// I/O failure.
func Load(repoRoot string) (*Project, error) {
	data, err := os.ReadFile(ResolvePaths(repoRoot).Config)
	if err != nil {
		return nil, err
	}
	var p Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Save writes p to .beam/config.json, creating .beam/ if needed.
func (p *Project) Save(repoRoot string) error {
	paths := ResolvePaths(repoRoot)
	if err := os.MkdirAll(paths.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.Config, data, 0o644)
}

// Exists reports whether repoRoot has already been beam-initialized.
func Exists(repoRoot string) bool {
	_, err := os.Stat(ResolvePaths(repoRoot).Config)
	return err == nil
}
