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
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var GlobMultipleRecursiveErr = errors.New("pattern contains multiple **")
var GlobLastRecursiveErr = errors.New("pattern ** as last path element")

// Glob returns the list of files that match the given pattern along with the
// list of directories that were searched to construct the file list.
// The supported glob patterns are equivalent to filepath.Glob, with an
// extension that recursive glob (** matching zero or more complete path
// entries) is supported.
func Glob(pattern string) (matches, dirs []string, err error) {
	if !isWild(pattern) {
		// If there are no wilds in the pattern, just return whether the file at the pattern
		// exists or not.  Uses filepath.Glob instead of manually statting to get consistent
		// results.
		matches, err = filepath.Glob(filepath.Clean(pattern))
	} else if filepath.Base(pattern) == "**" {
		return nil, nil, GlobLastRecursiveErr
	} else {
		matches, dirs, err = glob(pattern, false)
	}

	return matches, dirs, err
}

// glob is a recursive helper function to handle globbing each level of the pattern individually,
// allowing searched directories to be tracked.  Also handles the recursive glob pattern, **.
func glob(pattern string, hasRecursive bool) (matches, dirs []string, err error) {
	if !isWild(pattern) {
		// If there are no wilds in the pattern, just return whether the file at the pattern
		// exists or not.  Uses filepath.Glob instead of manually statting to get consistent
		// results.
		matches, err = filepath.Glob(filepath.Clean(pattern))
		return matches, dirs, err
	}

	dir, file := saneSplit(pattern)

	if file == "**" {
		if hasRecursive {
			return matches, dirs, GlobMultipleRecursiveErr
		}
		hasRecursive = true
	}

	dirMatches, dirs, err := glob(dir, hasRecursive)
	if err != nil {
		return nil, nil, err
	}

	for _, m := range dirMatches {
		if info, _ := os.Stat(m); info.IsDir() {
			if file == "**" {
				recurseDirs, err := walkAllDirs(m)
				if err != nil {
					return nil, nil, err
				}
				matches = append(matches, recurseDirs...)
			} else {
				dirs = append(dirs, m)
				newMatches, err := filepath.Glob(filepath.Join(m, file))
				if err != nil {
					return nil, nil, err
				}
				matches = append(matches, newMatches...)
			}
		}
	}

	return matches, dirs, nil
}

// Faster version of dir, file := filepath.Dir(path), filepath.File(path) with no allocations
// Similar to filepath.Split, but returns "." if dir is empty and trims trailing slash if dir is
// not "/".  Returns ".", "" if path is "."
func saneSplit(path string) (dir, file string) {
	if path == "." {
		return ".", ""
	}
	dir, file = filepath.Split(path)
	switch dir {
	case "":
		dir = "."
	case "/":
		// Nothing
	default:
		dir = dir[:len(dir)-1]
	}
	return dir, file
}

func isWild(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

// Returns a list of all directories under dir
func walkAllDirs(dir string) (dirs []string, err error) {
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Mode().IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})

	return dirs, err
}

func GlobPatternList(patterns []string, prefix string) (globedList []string, depDirs []string, err error) {
	var (
		matches []string
		deps    []string
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
