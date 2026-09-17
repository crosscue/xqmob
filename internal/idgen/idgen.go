package idgen

import (
	"crypto/sha256"
	"encoding/hex"
)

// Stable returns a deterministic opaque identifier derived from ordered parts.
// The zero-byte separator makes concatenation unambiguous.
func Stable(prefix string, parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(p))
	}
	sum := h.Sum(nil)
	return prefix + ":" + hex.EncodeToString(sum[:12])
}
