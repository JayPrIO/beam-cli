package config

import (
	"crypto/rand"
	"encoding/hex"
)

// NewProjectID returns an 8-hex-char random nonce. It is intentionally
// not derived from the project name or path: two unrelated repos can
// share a basename on different machines, and deriving the ID from the
// path would make `beam init` non-idempotent across machines that
// mount the same repo at different absolute paths. A second machine
// instead attaches to an existing vault explicitly via `beam init
// --join <id>` (see vault.go).
func NewProjectID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
