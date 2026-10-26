# beam

Sync active repository code between Macs (e.g. a Mac Studio and a
MacBook) via iCloud Drive — no node_modules, vendor, build artifacts,
or other heavy dependency folders, filtered out by a `.beamignore`
file.

iCloud Drive is the transport: beam writes into a folder under
`~/Library/Mobile Documents/com~apple~CloudDocs/beam-vaults/` and lets
iCloud sync it to your other Macs. There's no server, no network
protocol of beam's own.

## Install

```sh
brew tap jayprio/beam
brew install beam
```

Installs a prebuilt binary — no Go toolchain required. See
[jayprio/homebrew-beam](https://github.com/jayprio/homebrew-beam).

## Quick start

**First machine:**

```sh
cd /path/to/project
beam init
beam push
```

`init` prints the project's id (e.g. `a6213d80`) — you'll need it on
the second machine.

**Second machine:**

```sh
mkdir -p /path/to/project && cd /path/to/project
beam init --join a6213d80   # or just `beam init --join` for an arrow-key picker
beam pull
```

From then on, `beam push` / `beam pull` on either machine.

## Commands

### `beam init`

Scans the current directory, writes a default `.beamignore` (unless
one already exists), creates `.beam/` with a project id, and
provisions a matching vault folder in iCloud Drive at
`beam-vaults/<name>-<id>/`. Re-running `init` is a no-op unless
`--force` is given.

| flag | effect |
| --- | --- |
| `--join <id>` | attach to an existing vault instead of creating a new one |
| `--join` (no value) | pick a project from an arrow-key list of everything registered in iCloud |
| `--name <name>` | override the project name (defaults to the directory's basename) |
| `--force` | re-provision even if this directory is already a beam project |
| `--force-ignore` | overwrite an existing `.beamignore` with beam's defaults |

### `beam push`

Copies every file not matched by `.beamignore` into the vault, using
an atomic stage-and-swap: files are copied into a fresh staging
directory first, then swapped into place with a single directory
rename — so if `push` is interrupted, the vault is never left half
written.

| flag | effect |
| --- | --- |
| `--dry-run` | show what push would do, write nothing |
| `--force` | clear a stale push lock before starting (doesn't override an actively-running push) |
| `--verbose` | print every add/update action, not just the summary |

### `beam pull`

Reads the vault and merges new/updated files into the local repo.
Pull never deletes local files, and always respects `.beamignore`.

A file changed on **both** sides since the last sync is a conflict:
by default it's left alone (and reported), so nothing gets
overwritten silently.

| flag | effect |
| --- | --- |
| `--dry-run` | show what pull would do, write nothing |
| `--force` | overwrite local files on conflict — vault wins |
| `--prefer-local` | explicit alias for the default: skip conflicts, keep local |
| `--verbose` | print every add/update action, not just the summary |

### `beam list`

Lists every project registered in iCloud — name, id, who created it,
and when each machine last pushed/pulled it.

## How it works

- **`.beamignore`** — standard gitignore syntax (wildcards, `**`,
  `!` negation). Defaults to excluding `.git/`, `node_modules/`,
  `.venv/`, `vendor/`, `build/`, `dist/`, `__pycache__/`, and similar.
  `.beam/` and `.DS_Store`/AppleDouble files are always excluded,
  regardless of `.beamignore`'s contents.
- **`.beam/`** (local, per-machine, never synced) — `config.json`
  (project id, vault path), `manifest.json` (last-known-synced file
  state, used to diff on the next push/pull), `push.lock` (advisory
  lock held during a push).
- **The registry** — a small SQLite index at
  `beam-vaults/registry.db`, inside iCloud Drive, so it syncs to every
  machine automatically. It's what powers `beam list` and the
  `--join` picker, and tracks each project's last push/pull activity.
  Every write rebuilds the index into a temp copy and atomically
  renames it over the live file, so a concurrent iCloud sync can't
  corrupt it. If it's ever missing or unreadable, `--join <id>` falls
  back to scanning each vault's own `vault-meta.json`.
- **Conflicts** are never auto-merged — beam syncs files, it doesn't
  merge them. A conflicting file is left alone until you resolve it
  (edit, then push or pull again) or explicitly choose `--force`.

## Exit codes

| code | meaning |
| --- | --- |
| `0` | success |
| `1` | fatal error, operation aborted |
| `2` | completed, but with per-file errors |
| `3` | pull found conflicts, left unresolved |

## Known limitations

- Two machines pushing at the exact same moment isn't coordinated —
  there's no distributed lock over iCloud, only a local advisory lock
  per machine. Last push wins.
- No automatic merge on conflicting files; you resolve them by hand.
- macOS only (relies on iCloud Drive's local path and dataless-file
  behavior).

## Building from source

```sh
go build ./...
go test ./...
```
