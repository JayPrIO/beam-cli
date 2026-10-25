// Package errs defines the shared error/report types used across beam's
// commands so per-file failures can be collected without aborting an
// in-progress push or pull, while still distinguishing them from fatal
// whole-operation errors for exit-code purposes.
package errs

import "fmt"

// FileError records a single per-file failure encountered during a walk,
// copy, or write. These are collected, not fatal: one bad file should
// never abort an entire push or pull.
type FileError struct {
	Path string
	Op   string
	Err  error
}

func (e FileError) Error() string {
	return fmt.Sprintf("%s: %s: %v", e.Op, e.Path, e.Err)
}

// Conflict records a path that changed on both the local and remote side
// since the last known sync point, surfaced by `pull`.
type Conflict struct {
	Path string
}

// Report accumulates the outcome of a push or pull: a Fatal error aborts
// the whole operation, while Errors/Conflicts are collected so the
// operation can finish and report what succeeded alongside what didn't.
type Report struct {
	Errors    []FileError
	Conflicts []Conflict
	Fatal     error
}

func (r *Report) AddError(path, op string, err error) {
	r.Errors = append(r.Errors, FileError{Path: path, Op: op, Err: err})
}

func (r *Report) AddConflict(path string) {
	r.Conflicts = append(r.Conflicts, Conflict{Path: path})
}

func (r *Report) HasErrors() bool {
	return r != nil && len(r.Errors) > 0
}

func (r *Report) HasConflicts() bool {
	return r != nil && len(r.Conflicts) > 0
}

// ExitCode maps a Report (and any command-level error) to beam's exit
// code contract:
//
//	0 success
//	1 fatal error, operation aborted
//	2 completed with partial per-file errors
//	3 pull conflicts detected, user action required
func ExitCode(r *Report, cmdErr error) int {
	if cmdErr != nil && r == nil {
		return 1
	}
	if r == nil {
		if cmdErr != nil {
			return 1
		}
		return 0
	}
	if r.Fatal != nil {
		return 1
	}
	if r.HasConflicts() {
		return 3
	}
	if r.HasErrors() {
		return 2
	}
	return 0
}
