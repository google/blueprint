package pathtools

import (
	"path/filepath"
	"strings"
)

// Glob returns the list of files that match the given pattern along with the
// list of directories that were searched to construct the file list.
func Glob(pattern string) (matches, dirs []string, err error) {
	matches, err = filepath.Glob(pattern)
	if err != nil {
		return nil, nil, err
	}

	wildIndices := wildElements(pattern)

	if len(wildIndices) > 0 {
		for _, match := range matches {
			dir := filepath.Dir(match)
			dirElems := strings.Split(dir, string(filepath.Separator))

			for _, index := range wildIndices {
				dirs = append(dirs, strings.Join(dirElems[:index],
					string(filepath.Separator)))
			}
		}
	}

	return
}

func wildElements(pattern string) []int {
	elems := strings.Split(pattern, string(filepath.Separator))

	var result []int
	for i, elem := range elems {
		if isWild(elem) {
			result = append(result, i)
		}
	}
	return result
}

func isWild(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}
