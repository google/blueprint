// Copyright 2014 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

func GlobPatternList(patterns []string, prefix string) (globedList []string, depDirs []string, err error) {
	var (
		matches []string
		deps         []string
	)

	globedList = make([]string, 0)
	depDirs = make([]string, 0)

	for _, pattern := range patterns {
		if isWild(pattern) {
			matches, deps, err = Glob(filepath.Join(prefix, pattern))
			if err != nil {
				return nil, nil, err
			}
			globedList = append(globedList, matches...)
			depDirs = append(depDirs, deps...)
		} else {
			globedList = append(globedList, filepath.Join(prefix, pattern))
		}
	}
	return globedList, depDirs, nil
}
