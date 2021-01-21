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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/deptools"
)

var GlobMultipleRecursiveErr = errors.New("pattern contains multiple '**'")
var GlobLastRecursiveErr = errors.New("pattern has '**' as last path element")
var GlobInvalidRecursiveErr = errors.New("pattern contains other characters between '**' and path separator")

// Glob returns the list of files and directories that match the given pattern
// but do not match the given exclude patterns, along with the list of
// directories and other dependencies that were searched to construct the file
// list.  The supported glob and exclude patterns are equivalent to
// filepath.Glob, with an extension that recursive glob (** matching zero or
// more complete path entries) is supported. Any directories in the matches
// list will have a '/' suffix.
//
// In general ModuleContext.GlobWithDeps or SingletonContext.GlobWithDeps
// should be used instead, as they will automatically set up dependencies
// to rerun the primary builder when the list of matching files changes.
func Glob(pattern string, excludes []string, follow ShouldFollowSymlinks) (matches, deps []string, err error) {
	return startGlob(OsFs, pattern, excludes, follow)
}

func startGlob(fs FileSystem, pattern string, excludes []string,
	follow ShouldFollowSymlinks) (matches, deps []string, err error) {

	if filepath.Base(pattern) == "**" {
		return nil, nil, GlobLastRecursiveErr
	} else {
		matches, deps, err = glob(fs, pattern, false, follow)
	}

	if err != nil {
		return nil, nil, err
	}

	matches, err = filterExcludes(matches, excludes)
	if err != nil {
		return nil, nil, err
	}

	// If the pattern has wildcards, we added dependencies on the
	// containing directories to know about changes.
	//
	// If the pattern didn't have wildcards, and didn't find matches, the
	// most specific found directories were added.
	//
	// But if it didn't have wildcards, and did find a match, no
	// dependencies were added, so add the match itself to detect when it
	// is removed.
	if !isWild(pattern) {
		deps = append(deps, matches...)
	}

	for i, match := range matches {
		var info os.FileInfo
		if follow == DontFollowSymlinks {
			info, err = fs.Lstat(match)
		} else {
			info, err = fs.Stat(match)
		}
		if err != nil {
			return nil, nil, err
		}

		if info.IsDir() {
			matches[i] = match + "/"
		}
	}

	return matches, deps, nil
}

// glob is a recursive helper function to handle globbing each level of the pattern individually,
// allowing searched directories to be tracked.  Also handles the recursive glob pattern, **.
func glob(fs FileSystem, pattern string, hasRecursive bool,
	follow ShouldFollowSymlinks) (matches, dirs []string, err error) {

	if !isWild(pattern) {
		// If there are no wilds in the pattern, check whether the file exists or not.
		// Uses filepath.Glob instead of manually statting to get consistent results.
		pattern = filepath.Clean(pattern)
		matches, err = fs.glob(pattern)
		if err != nil {
			return matches, dirs, err
		}

		if len(matches) == 0 {
			// Some part of the non-wild pattern didn't exist.  Add the last existing directory
			// as a dependency.
			var matchDirs []string
			for len(matchDirs) == 0 {
				pattern = filepath.Dir(pattern)
				matchDirs, err = fs.glob(pattern)
				if err != nil {
					return matches, dirs, err
				}
			}
			dirs = append(dirs, matchDirs...)
		}
		return matches, dirs, err
	}

	dir, file := saneSplit(pattern)

	if file == "**" {
		if hasRecursive {
			return matches, dirs, GlobMultipleRecursiveErr
		}
		hasRecursive = true
	} else if strings.Contains(file, "**") {
		return matches, dirs, GlobInvalidRecursiveErr
	}

	dirMatches, dirs, err := glob(fs, dir, hasRecursive, follow)
	if err != nil {
		return nil, nil, err
	}

	for _, m := range dirMatches {
		isDir, err := fs.IsDir(m)
		if os.IsNotExist(err) {
			if isSymlink, _ := fs.IsSymlink(m); isSymlink {
				return nil, nil, fmt.Errorf("dangling symlink: %s", m)
			}
		}
		if err != nil {
			return nil, nil, fmt.Errorf("unexpected error after glob: %s", err)
		}

		if isDir {
			if file == "**" {
				recurseDirs, err := fs.ListDirsRecursive(m, follow)
				if err != nil {
					return nil, nil, err
				}
				matches = append(matches, recurseDirs...)
			} else {
				dirs = append(dirs, m)
				newMatches, err := fs.glob(filepath.Join(MatchEscape(m), file))
				if err != nil {
					return nil, nil, err
				}
				if file[0] != '.' {
					newMatches = filterDotFiles(newMatches)
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

// Filters the strings in matches based on the glob patterns in excludes.  Hierarchical (a/*) and
// recursive (**) glob patterns are supported.
func filterExcludes(matches []string, excludes []string) ([]string, error) {
	if len(excludes) == 0 {
		return matches, nil
	}

	var ret []string
matchLoop:
	for _, m := range matches {
		for _, e := range excludes {
			exclude, err := Match(e, m)
			if err != nil {
				return nil, err
			}
			if exclude {
				continue matchLoop
			}
		}
		ret = append(ret, m)
	}

	return ret, nil
}

// filterDotFiles filters out files that start with '.'
func filterDotFiles(matches []string) []string {
	ret := make([]string, 0, len(matches))

	for _, match := range matches {
		_, name := filepath.Split(match)
		if name[0] == '.' {
			continue
		}
		ret = append(ret, match)
	}

	return ret
}

// Match returns true if name matches pattern using the same rules as filepath.Match, but supporting
// recursive globs (**).
func Match(pattern, name string) (bool, error) {
	if filepath.Base(pattern) == "**" {
		return false, GlobLastRecursiveErr
	}

	patternDir := pattern[len(pattern)-1] == '/'
	nameDir := name[len(name)-1] == '/'

	if patternDir != nameDir {
		return false, nil
	}

	if nameDir {
		name = name[:len(name)-1]
		pattern = pattern[:len(pattern)-1]
	}

	for {
		var patternFile, nameFile string
		pattern, patternFile = filepath.Dir(pattern), filepath.Base(pattern)

		if patternFile == "**" {
			if strings.Contains(pattern, "**") {
				return false, GlobMultipleRecursiveErr
			}
			// Test if the any prefix of name matches the part of the pattern before **
			for {
				if name == "." || name == "/" {
					return name == pattern, nil
				}
				if match, err := filepath.Match(pattern, name); err != nil {
					return false, err
				} else if match {
					return true, nil
				}
				name = filepath.Dir(name)
			}
		} else if strings.Contains(patternFile, "**") {
			return false, GlobInvalidRecursiveErr
		}

		name, nameFile = filepath.Dir(name), filepath.Base(name)

		if nameFile == "." && patternFile == "." {
			return true, nil
		} else if nameFile == "/" && patternFile == "/" {
			return true, nil
		} else if nameFile == "." || patternFile == "." || nameFile == "/" || patternFile == "/" {
			return false, nil
		}

		match, err := filepath.Match(patternFile, nameFile)
		if err != nil || !match {
			return match, err
		}
	}
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
			matches, deps, err = Glob(filepath.Join(prefix, pattern), nil, FollowSymlinks)
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

// IsGlob returns true if the pattern contains any glob characters (*, ?, or [).
func IsGlob(pattern string) bool {
	return strings.IndexAny(pattern, "*?[") >= 0
}

// HasGlob returns true if any string in the list contains any glob characters (*, ?, or [).
func HasGlob(in []string) bool {
	for _, s := range in {
		if IsGlob(s) {
			return true
		}
	}

	return false
}

// GlobWithDepFile finds all files and directories that match glob.  Directories
// will have a trailing '/'.  It compares the list of matches against the
// contents of fileListFile, and rewrites fileListFile if it has changed.  It
// also writes all of the the directories it traversed as dependencies on
// fileListFile to depFile.
//
// The format of glob is either path/*.ext for a single directory glob, or
// path/**/*.ext for a recursive glob.
//
// Returns a list of file paths, and an error.
//
// In general ModuleContext.GlobWithDeps or SingletonContext.GlobWithDeps
// should be used instead, as they will automatically set up dependencies
// to rerun the primary builder when the list of matching files changes.
func GlobWithDepFile(glob, fileListFile, depFile string, excludes []string) (files []string, err error) {
	files, deps, err := Glob(glob, excludes, FollowSymlinks)
	if err != nil {
		return nil, err
	}

	fileList := strings.Join(files, "\n") + "\n"

	WriteFileIfChanged(fileListFile, []byte(fileList), 0666)
	deptools.WriteDepFile(depFile, fileListFile, deps)

	return
}

// WriteFileIfChanged wraps ioutil.WriteFile, but only writes the file if
// the files does not already exist with identical contents.  This can be used
// along with ninja restat rules to skip rebuilding downstream rules if no
// changes were made by a rule.
func WriteFileIfChanged(filename string, data []byte, perm os.FileMode) error {
	var isChanged bool

	dir := filepath.Dir(filename)
	err := os.MkdirAll(dir, 0777)
	if err != nil {
		return err
	}

	info, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// The file does not exist yet.
			isChanged = true
		} else {
			return err
		}
	} else {
		if info.Size() != int64(len(data)) {
			isChanged = true
		} else {
			oldData, err := ioutil.ReadFile(filename)
			if err != nil {
				return err
			}

			if len(oldData) != len(data) {
				isChanged = true
			} else {
				for i := range data {
					if oldData[i] != data[i] {
						isChanged = true
						break
					}
				}
			}
		}
	}

	if isChanged {
		err = ioutil.WriteFile(filename, data, perm)
		if err != nil {
			return err
		}
	}

	return nil
}

var matchEscaper = strings.NewReplacer(
	`*`, `\*`,
	`?`, `\?`,
	`[`, `\[`,
	`]`, `\]`,
)

// MatchEscape returns its inputs with characters that would be interpreted by
func MatchEscape(s string) string {
	return matchEscaper.Replace(s)
}
