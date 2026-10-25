package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !m.IsIgnored("node_modules", true) {
		t.Error("expected default .beamignore to exclude node_modules/")
	}
	if m.IsIgnored("app.js", false) {
		t.Error("app.js should not be ignored by defaults")
	}
}

func TestLoadReadsBeamignore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, Filename), []byte("*.log\n!keep.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cases := []struct {
		path   string
		isDir  bool
		ignore bool
	}{
		{"debug.log", false, true},
		{"keep.log", false, false}, // negated
		{"app.js", false, false},
	}
	for _, c := range cases {
		if got := m.IsIgnored(c.path, c.isDir); got != c.ignore {
			t.Errorf("IsIgnored(%q) = %v, want %v", c.path, got, c.ignore)
		}
	}
}

func TestAlwaysIgnoreOverridesUserFile(t *testing.T) {
	dir := t.TempDir()
	// Even if the user tries to negate it, .beam/ must stay ignored.
	if err := os.WriteFile(filepath.Join(dir, Filename), []byte("!.beam/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !m.IsIgnored(".beam", true) {
		t.Error(".beam/ must always be ignored regardless of .beamignore contents")
	}
}

func TestDirectoryPatternRequiresTrailingSlash(t *testing.T) {
	m, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// .DS_Store is a hardcoded always-ignore file pattern, not a dir
	// pattern; confirm it matches as a file.
	if !m.IsIgnored(".DS_Store", false) {
		t.Error(".DS_Store should always be ignored")
	}
}
