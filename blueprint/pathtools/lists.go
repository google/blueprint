package pathtools

import (
	"path/filepath"
)

// PrefixPaths returns a list of paths consisting of prefix joined with each
// element of paths.  The resulting paths are "clean" in the filepath.Clean
// sense.
func PrefixPaths(paths []string, prefix string) []string {
	result := make([]string, len(paths))
	for i, path := range paths {
		result[i] = filepath.Join(prefix, path)
	}
	return result
}
