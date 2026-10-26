# Contributing to beam

Thanks for considering a contribution. beam is a small, focused tool —
most useful changes are small too.

## Building and testing

```sh
go build ./...          # build everything
go vet ./...             # static checks
go test ./...             # all tests
go test ./internal/sync   # one package
go test -run TestDiffPull ./internal/sync   # one test
```

No network access or external services are needed to build or run
the test suite — tests use `t.TempDir()`, never the real iCloud Drive.

## Project layout

- `cmd/` — the cobra command tree (`init`/`push`/`pull`/`list`)
- `internal/config` — `.beam/` state, vault path resolution/
  provisioning, the local push lock
- `internal/registry` — the SQLite project index in iCloud Drive
- `internal/ignore` — `.beamignore` parsing
- `internal/fsutil` — ignore-aware directory walk, atomic copy/swap/
  write primitives, macOS iCloud-placeholder handling
- `internal/sync` — manifests, the push/pull diff algorithm, and the
  push/pull orchestration itself
- `internal/errs` — shared error/report types and exit-code mapping

See [README.md](README.md) for how these fit together and the
guarantees (`push`'s atomic stage-and-swap, `pull`'s conflict
handling) that most of the design exists to support.

## Before opening a PR

- `go build ./...`, `go vet ./...`, and `go test ./...` should all
  pass.
- Match the surrounding code's style and comment density — this
  codebase explains *why*, not just *what*, especially around the
  atomicity and conflict-handling guarantees; new code touching those
  areas should keep that up.
- Keep PRs scoped to one change. A one-line fix and a new feature
  don't belong in the same PR.
- If you're changing behavior a design doc comment describes (e.g. in
  `push.go`, `pull.go`, `registry.go`), update the comment too.

## Reporting bugs

Open an issue with: what you ran, what you expected, what happened
instead, and your macOS version. beam only supports macOS (it depends
on iCloud Drive's local path conventions and dataless-file behavior),
so issues from other platforms will be closed as out of scope.

## Reporting security issues

Please don't open a public issue for a security vulnerability — email
the maintainer instead (see the GitHub profile for contact info).
