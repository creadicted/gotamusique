package file

import (
	"crypto/sha1"
	"fmt"
)

// IDFromPath returns the sha1 hex digest of the relative path.
func IDFromPath(relPath string) string {
	h := sha1.Sum([]byte(relPath))
	return fmt.Sprintf("%x", h)
}
