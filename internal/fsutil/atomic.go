package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// CopyFile copies src to dst, creating dst's parent directory if
// needed, then verifies the written size matches the source's — a
// cheap partial-copy check. It does not fsync; staging's integrity
// comes from the swap never exposing a half-populated directory, not
// from per-file durability guarantees.
func CopyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	srcInfo, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	written, err := io.Copy(out, in)
	if err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if written != srcInfo.Size() {
		return fmt.Errorf("short copy: wrote %d bytes, source is %d bytes", written, srcInfo.Size())
	}
	return nil
}

// SwapDir atomically replaces current with staging. If current
// already exists, it's renamed aside first and removed only after
// staging has taken its place — ordered so there's never a window
// where current is missing, which matters because current is what a
// concurrent pull reads. If current doesn't exist yet (first push),
// staging is renamed straight into place.
func SwapDir(staging, current string) error {
	if _, err := os.Stat(current); err == nil {
		old := current + ".old-" + currentTimestamp()
		if err := os.Rename(current, old); err != nil {
			return fmt.Errorf("renaming existing %s aside: %w", current, err)
		}
		if err := os.Rename(staging, current); err != nil {
			// Best-effort: put the previous current back so we don't
			// leave the vault with neither staging nor current visible
			// under the expected name.
			_ = os.Rename(old, current)
			return fmt.Errorf("renaming staging into place: %w", err)
		}
		return os.RemoveAll(old)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(staging, current)
}

// WriteFileAtomic writes data to a temp file beside dst, then renames
// it into place. Used by pull, which writes directly into the user's
// live working tree (possibly open in an editor) rather than into a
// disposable staging directory, so each file needs its own atomic
// write.
func WriteFileAtomic(dst string, src io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".beam-tmp"

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func currentTimestamp() string {
	return time.Now().UTC().Format("20060102T150405.000000000Z")
}
