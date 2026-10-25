package config

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
)

// Lock is a simple local advisory lock: it stops two concurrent `beam
// push` runs on the *same machine* from racing on the vault's
// current/ directory. It cannot and does not attempt to coordinate
// across machines — there's no distributed lock over iCloud, so
// concurrent pushes from two Macs at once is a documented limitation
// (see PullOptions.ConflictMode and the plan's §5 notes), not
// something this type solves.
type Lock struct {
	path string
}

// Acquire takes the lock at path, a PID file under .beam/. If an
// existing lock file names a PID that's no longer running (the
// process that held it crashed instead of releasing it), the stale
// lock is reclaimed automatically.
func Acquire(path string) (*Lock, error) {
	if data, err := os.ReadFile(path); err == nil {
		if pid, parseErr := strconv.Atoi(string(data)); parseErr == nil && processAlive(pid) {
			return nil, fmt.Errorf("another beam push is already running (pid %d, lock: %s)", pid, path)
		}
		// Stale lock from a dead process: fall through and reclaim it.
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		return nil, err
	}
	return &Lock{path: path}, nil
}

// Release removes the lock file.
func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	return os.Remove(l.path)
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 performs no-op existence/permission checks without
	// actually sending a signal.
	return proc.Signal(syscall.Signal(0)) == nil
}
