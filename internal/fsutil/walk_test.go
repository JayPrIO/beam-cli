package fsutil

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fakeMatcher ignores any path matching one of the given prefixes/names
// for directories, and never ignores files — enough to drive Walk's
// pruning logic in tests without pulling in the real ignore package.
type fakeMatcher struct{ ignoredDirs map[string]bool }

func (f fakeMatcher) IsIgnored(relPath string, isDir bool) bool {
	if !isDir {
		return false
	}
	return f.ignoredDirs[filepath.ToSlash(relPath)]
}

func TestWalkPrunesIgnoredDirectories(t *testing.T) {
	root := t.TempDir()

	mustWriteFile(t, filepath.Join(root, "app.js"), "ok")
	mustWriteFile(t, filepath.Join(root, "src", "lib.js"), "ok")

	nm := filepath.Join(root, "node_modules")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		t.Fatal(err)
	}
	// A file beam should never even need to read, because the walk
	// must prune node_modules/ before descending into it.
	sentinel := filepath.Join(nm, "unreadable.js")
	mustWriteFile(t, sentinel, "junk")
	if err := os.Chmod(sentinel, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sentinel, 0o644) })

	matcher := fakeMatcher{ignoredDirs: map[string]bool{"node_modules": true}}

	entries, err := Walk(root, matcher)
	if err != nil {
		t.Fatalf("Walk returned error (node_modules/ should have been pruned, not read): %v", err)
	}

	var paths []string
	for _, e := range entries {
		paths = append(paths, e.RelPath)
	}
	sort.Strings(paths)

	want := []string{"app.js", "src/lib.js"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Errorf("Walk entries = %v, want %v", paths, want)
	}
}

func TestWalkSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "real.txt"), "content")
	if err := os.Symlink(filepath.Join(root, "real.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlinks unsupported in this environment: %v", err)
	}

	entries, err := Walk(root, fakeMatcher{})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(entries) != 1 || entries[0].RelPath != "real.txt" {
		t.Errorf("expected only real.txt, got %+v", entries)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
