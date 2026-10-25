// Package ignore wraps github.com/sabhiram/go-gitignore to apply
// .beamignore rules (standard gitignore syntax: wildcards, "**", and "!"
// negation) when the walker decides which files and directories are
// eligible to be pushed or pulled.
package ignore

import (
	"os"
	"path/filepath"
	"strings"

	gi "github.com/sabhiram/go-gitignore"
)

// Filename is the name of beam's ignore file at the repo root.
const Filename = ".beamignore"

// alwaysIgnore are hardcoded exclusions applied regardless of what the
// user's .beamignore says: beam's own state directory must never be
// synced, and macOS/AppleDouble noise is never worth syncing either.
var alwaysIgnore = []string{
	".beam/",
	".DS_Store",
	"._*",
}

// Matcher answers whether a repo-relative path should be excluded from a
// push or pull.
type Matcher struct {
	gi *gi.GitIgnore
}

// Load reads <repoRoot>/.beamignore if present, otherwise falls back to
// DefaultBeamignore. The hardcoded alwaysIgnore rules are appended last
// so they take effect even if a user's .beamignore tries to negate them.
func Load(repoRoot string) (*Matcher, error) {
	path := filepath.Join(repoRoot, Filename)

	var lines []string
	content, err := os.ReadFile(path)
	switch {
	case err == nil:
		lines = strings.Split(string(content), "\n")
	case os.IsNotExist(err):
		lines = strings.Split(string(DefaultBeamignore()), "\n")
	default:
		return nil, err
	}

	lines = append(lines, alwaysIgnore...)
	return &Matcher{gi: gi.CompileIgnoreLines(lines...)}, nil
}

// IsIgnored reports whether relPath (slash-separated, relative to the
// repo root) should be excluded. Callers must pass isDir so directory
// patterns like "node_modules/" match correctly; go-gitignore only
// matches a trailing-slash pattern against a path that itself ends in
// "/".
func (m *Matcher) IsIgnored(relPath string, isDir bool) bool {
	relPath = filepath.ToSlash(relPath)
	if isDir && !strings.HasSuffix(relPath, "/") {
		relPath += "/"
	}
	return m.gi.MatchesPath(relPath)
}
