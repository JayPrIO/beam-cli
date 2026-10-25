// Package registry maintains a central SQLite index of every beamed
// project inside beam-vaults/, so `beam init --join` can offer an
// interactive picker instead of requiring the user to already know a
// project ID. It is the primary source of truth for project discovery
// and sync activity: config.FindVaultByID resolves through it first,
// and push/pull record their activity into it (see RecordPush/
// RecordPull) so `beam list` can show when each machine last synced a
// project, not just who created it.
//
// The index lives at <vaultRoot>/registry.db, inside iCloud Drive, so
// it syncs to every machine without user intervention — but SQLite has
// no awareness of iCloud's file-sync layer, and two Macs writing the
// same .db file near-simultaneously through iCloud could corrupt it
// (classic multi-writer-over-a-sync-service problem, the same class of
// risk documented for concurrent cross-machine push). Every write here
// is therefore structured the same way push's vault swap is: never
// opened for writing in place. A write rebuilds the whole index into a
// private temp copy and atomically renames it over registry.db, so the
// live file other machines are reading/syncing is always either the
// old complete index or the new complete index, never a partial one.
// This does not eliminate the race of two machines writing at the
// literal same instant (last writer wins, like push's own documented
// cross-machine limitation) — it eliminates corruption.
//
// Being the primary source of truth doesn't mean the only one:
// config.FindVaultByID falls back to scanning vault-meta.json files
// (written per-vault, independent of this DB) if registry.db is
// missing or unreadable, so a corrupt/lost registry doesn't strand
// every machine that already knows a project id.
package registry

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// FileName is the registry's filename inside beam-vaults/.
const FileName = "registry.db"

// Entry is one row: a single beamed project, plus the most recent
// push/pull activity recorded against it by any machine.
type Entry struct {
	ProjectID     string
	ProjectName   string
	VaultPath     string
	CreatedByHost string
	CreatedAt     time.Time

	LastPushedAt   *time.Time
	LastPushedHost string
	LastPulledAt   *time.Time
	LastPulledHost string
}

// Path returns <vaultRoot>/registry.db.
func Path(vaultRoot string) string {
	return filepath.Join(vaultRoot, FileName)
}

const schema = `
CREATE TABLE IF NOT EXISTS projects (
	project_id        TEXT PRIMARY KEY,
	project_name      TEXT NOT NULL,
	vault_path        TEXT NOT NULL,
	created_by_host   TEXT NOT NULL,
	created_at        TEXT NOT NULL,
	last_pushed_at    TEXT,
	last_pushed_host  TEXT,
	last_pulled_at    TEXT,
	last_pulled_host  TEXT
);
`

const selectCols = `project_id, project_name, vault_path, created_by_host, created_at,
	last_pushed_at, last_pushed_host, last_pulled_at, last_pulled_host`

// withWrite is the shared plumbing behind every registry mutation:
// copy the live DB (if any) into a private temp file, run fn against
// that temp file, then atomically rename it over the live path. See
// the package doc for why writes never touch registry.db in place.
func withWrite(vaultRoot string, fn func(db *sql.DB) error) error {
	dbPath := Path(vaultRoot)

	if err := os.MkdirAll(vaultRoot, 0o755); err != nil {
		return err
	}

	tmp := dbPath + ".tmp-" + fmt.Sprintf("%d", time.Now().UnixNano())
	if err := copyIfExists(dbPath, tmp); err != nil {
		return err
	}
	defer os.Remove(tmp) // no-op once renamed into place

	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		return err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return fmt.Errorf("applying schema: %w", err)
	}

	if err := fn(db); err != nil {
		db.Close()
		return err
	}

	if err := db.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dbPath)
}

// withRead opens a private, read-only copy of the live DB and passes
// it to fn — the read-side counterpart to withWrite, for the same
// reason: never touch the live iCloud-synced path directly, in case
// it's mid-download from another machine's write.
func withRead(vaultRoot string, fn func(db *sql.DB) error) error {
	dbPath := Path(vaultRoot)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fn(nil)
	}

	tmp := dbPath + ".read-" + fmt.Sprintf("%d", time.Now().UnixNano())
	if err := copyIfExists(dbPath, tmp); err != nil {
		return err
	}
	defer os.Remove(tmp)

	db, err := sql.Open("sqlite", "file:"+tmp+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	return fn(db)
}

// Register adds or replaces entry's identity/creation fields. Activity
// columns (last pushed/pulled) are preserved if the row already
// existed — Register is about identity, RecordPush/RecordPull are
// about activity, and neither should clobber the other.
func Register(vaultRoot string, entry Entry) error {
	return withWrite(vaultRoot, func(db *sql.DB) error {
		_, err := db.Exec(
			`INSERT INTO projects (project_id, project_name, vault_path, created_by_host, created_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(project_id) DO UPDATE SET
			   project_name=excluded.project_name,
			   vault_path=excluded.vault_path,
			   created_by_host=excluded.created_by_host,
			   created_at=excluded.created_at`,
			entry.ProjectID, entry.ProjectName, entry.VaultPath, entry.CreatedByHost, entry.CreatedAt.Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("writing entry: %w", err)
		}
		return nil
	})
}

// RecordPush updates projectID's last-pushed activity. Called by
// `beam push` after a successful push so the registry reflects live
// fleet sync status, not just creation metadata.
func RecordPush(vaultRoot, projectID, host string, at time.Time) error {
	return withWrite(vaultRoot, func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE projects SET last_pushed_at = ?, last_pushed_host = ? WHERE project_id = ?`,
			at.UTC().Format(time.RFC3339), host, projectID,
		)
		return err
	})
}

// RecordPull updates projectID's last-pulled activity, the pull
// counterpart to RecordPush.
func RecordPull(vaultRoot, projectID, host string, at time.Time) error {
	return withWrite(vaultRoot, func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE projects SET last_pulled_at = ?, last_pulled_host = ? WHERE project_id = ?`,
			at.UTC().Format(time.RFC3339), host, projectID,
		)
		return err
	})
}

// Get looks up a single project by id. It returns an error (not a nil
// entry) when no row matches, so callers can use that to decide
// whether to fall back to a directory scan.
func Get(vaultRoot, projectID string) (*Entry, error) {
	var entry *Entry
	err := withRead(vaultRoot, func(db *sql.DB) error {
		if db == nil {
			return fmt.Errorf("registry not found at %s", Path(vaultRoot))
		}
		row := db.QueryRow(`SELECT `+selectCols+` FROM projects WHERE project_id = ?`, projectID)
		e, err := scanEntry(row)
		if err != nil {
			return err
		}
		entry = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// List returns every registered project, newest first. A missing
// registry.db (nothing has ever registered, or this is the very first
// project on this Mac) returns an empty list, not an error.
func List(vaultRoot string) ([]Entry, error) {
	var entries []Entry
	err := withRead(vaultRoot, func(db *sql.DB) error {
		if db == nil {
			return nil
		}
		rows, err := db.Query(`SELECT ` + selectCols + ` FROM projects ORDER BY created_at DESC`)
		if err != nil {
			return fmt.Errorf("querying registry: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			e, err := scanEntry(rows)
			if err != nil {
				return err
			}
			entries = append(entries, *e)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows, letting Get and
// List share one row-decoding path.
type scanner interface {
	Scan(dest ...any) error
}

func scanEntry(s scanner) (*Entry, error) {
	var e Entry
	var createdAt string
	var lastPushedAt, lastPulledAt, lastPushedHost, lastPulledHost sql.NullString

	if err := s.Scan(&e.ProjectID, &e.ProjectName, &e.VaultPath, &e.CreatedByHost, &createdAt,
		&lastPushedAt, &lastPushedHost, &lastPulledAt, &lastPulledHost); err != nil {
		return nil, err
	}

	e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	e.LastPushedHost = lastPushedHost.String
	e.LastPulledHost = lastPulledHost.String
	if lastPushedAt.Valid {
		if t, err := time.Parse(time.RFC3339, lastPushedAt.String); err == nil {
			e.LastPushedAt = &t
		}
	}
	if lastPulledAt.Valid {
		if t, err := time.Parse(time.RFC3339, lastPulledAt.String); err == nil {
			e.LastPulledAt = &t
		}
	}
	return &e, nil
}

// copyIfExists copies src to dst if src exists; if src doesn't exist,
// dst is left absent so the caller creates a fresh database there.
func copyIfExists(src, dst string) error {
	data, err := os.ReadFile(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
