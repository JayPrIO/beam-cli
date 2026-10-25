package ignore

import _ "embed"

//go:embed defaults.beamignore
var defaultBeamignore []byte

// DefaultBeamignore returns the contents written to a fresh .beamignore
// file by `beam init`. Kept as an embedded file (rather than a Go string
// constant) so it reads and edits like the gitignore-syntax file it is.
func DefaultBeamignore() []byte {
	out := make([]byte, len(defaultBeamignore))
	copy(out, defaultBeamignore)
	return out
}
