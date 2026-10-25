package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kaihiwatari8/beam-cli/internal/registry"
)

// vaultsDirName is the subfolder beam creates inside iCloud Drive to
// hold every project's vault.
const vaultsDirName = "beam-vaults"

// vaultMetaFile is written inside the vault itself (not just locally)
// so a second machine running `beam init --join <id>` can find and
// validate the vault without any local state to start from.
const vaultMetaFile = "vault-meta.json"

// CurrentDirName is the live, synced tree inside a vault that `pull`
// reads from and `push` atomically swaps into place.
const CurrentDirName = "current"

// VaultMeta is the contents of <vault>/vault-meta.json.
type VaultMeta struct {
	ProjectID     string    `json:"project_id"`
	ProjectName   string    `json:"project_name"`
	CreatedByHost string    `json:"created_by_host"`
	CreatedAt     time.Time `json:"created_at"`
}

// ICloudRoot returns the user's iCloud Drive root
// (~/Library/Mobile Documents/com~apple~CloudDocs). It errors clearly
// if iCloud Drive isn't present/enabled, since every beam command that
// touches the vault depends on it.
func ICloudRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("iCloud Drive not found at %s — enable iCloud Drive for this user before running beam", root)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s exists but is not a directory", root)
	}
	return root, nil
}

// VaultRoot returns <iCloud>/beam-vaults, the parent of every project's
// vault directory.
func VaultRoot() (string, error) {
	icloud, err := ICloudRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(icloud, vaultsDirName), nil
}

var unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// SanitizeProjectName makes name safe to use as a filesystem path
// component: strips path separators and control characters, collapses
// everything else to "-", and caps length so a pathological repo name
// can't produce an unusable vault path.
func SanitizeProjectName(name string) string {
	name = unsafeNameChars.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		name = "project"
	}
	const maxLen = 64
	if len(name) > maxLen {
		name = name[:maxLen]
	}
	return name
}

// VaultPath returns the full path for a project's vault directory,
// e.g. <VaultRoot>/<name>-<id>/.
func VaultPath(vaultRoot, projectName, projectID string) string {
	return filepath.Join(vaultRoot, fmt.Sprintf("%s-%s", SanitizeProjectName(projectName), projectID))
}

// ProvisionVault creates a fresh vault directory and writes its
// vault-meta.json. It refuses to reuse a directory that already exists
// at this exact path unless force is set — with a randomly generated
// ID, a pre-existing directory at the computed path means something
// unexpected (e.g. a prior init's vault survived a deleted local
// .beam/), so silently overwriting it is never the safe default.
func ProvisionVault(vaultPath string, meta VaultMeta, force bool) error {
	if info, err := os.Stat(vaultPath); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s exists and is not a directory", vaultPath)
		}
		if !force {
			return fmt.Errorf("vault directory %s already exists (use --force to reuse it)", vaultPath)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Join(vaultPath, CurrentDirName), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(vaultPath, vaultMetaFile), data, 0o644)
}

// FindVaultByID resolves a project id to its vault path for `beam
// init --join <id>`. The registry (internal/registry, backed by
// registry.db) is the primary source of truth: a single indexed
// lookup, no directory walk. If the registry is missing, unreadable,
// or simply doesn't have the row (e.g. a vault created before the
// registry existed), this falls back to scanning
// VaultRoot/*/vault-meta.json — self-healing against a lost/corrupt
// DB, at the cost of a directory walk only in that degraded case.
func FindVaultByID(id string) (string, *VaultMeta, error) {
	root, err := VaultRoot()
	if err != nil {
		return "", nil, err
	}

	if entry, err := registry.Get(root, id); err == nil {
		return entry.VaultPath, &VaultMeta{
			ProjectID:     entry.ProjectID,
			ProjectName:   entry.ProjectName,
			CreatedByHost: entry.CreatedByHost,
			CreatedAt:     entry.CreatedAt,
		}, nil
	}

	return findVaultByIDScan(root, id)
}

// findVaultByIDScan is FindVaultByID's fallback: a directory walk over
// vault-meta.json files, used only when the registry can't answer.
func findVaultByIDScan(root, id string) (string, *VaultMeta, error) {
	suffix := "-" + id
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, fmt.Errorf("no vaults found at %s", root)
		}
		return "", nil, err
	}

	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		vaultPath := filepath.Join(root, e.Name())
		meta, err := readVaultMeta(vaultPath)
		if err != nil {
			continue
		}
		if meta.ProjectID == id {
			return vaultPath, meta, nil
		}
	}
	return "", nil, fmt.Errorf("no vault found with project id %q under %s", id, root)
}

func readVaultMeta(vaultPath string) (*VaultMeta, error) {
	data, err := os.ReadFile(filepath.Join(vaultPath, vaultMetaFile))
	if err != nil {
		return nil, err
	}
	var meta VaultMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}
