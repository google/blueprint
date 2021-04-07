// Copyright 2015 Google Inc. All rights reserved.
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

package blueprint

import (
	"crypto/md5"
	"fmt"
	"sort"
	"strings"

	"github.com/google/blueprint/pathtools"
)

type GlobPath struct {
	pathtools.GlobResult
	Name string
}

func verifyGlob(fileName, pattern string, excludes []string, g GlobPath) {
	if pattern != g.Pattern {
		panic(fmt.Errorf("Mismatched patterns %q and %q for glob file %q", pattern, g.Pattern, fileName))
	}
	if len(excludes) != len(g.Excludes) {
		panic(fmt.Errorf("Mismatched excludes %v and %v for glob file %q", excludes, g.Excludes, fileName))
	}

	for i := range excludes {
		if g.Excludes[i] != excludes[i] {
			panic(fmt.Errorf("Mismatched excludes %v and %v for glob file %q", excludes, g.Excludes, fileName))
		}
	}
}

func (c *Context) glob(pattern string, excludes []string) ([]string, error) {
	fileName := globToFileName(pattern, excludes)

	// Try to get existing glob from the stored results
	c.globLock.Lock()
	g, exists := c.globs[fileName]
	c.globLock.Unlock()

	if exists {
		// Glob has already been done, double check it is identical
		verifyGlob(fileName, pattern, excludes, g)
		// Return a copy so that modifications don't affect the cached value.
		return append([]string(nil), g.Matches...), nil
	}

	// Get a globbed file list
	result, err := c.fs.Glob(pattern, excludes, pathtools.FollowSymlinks)
	if err != nil {
		return nil, err
	}

	// Store the results
	c.globLock.Lock()
	if g, exists = c.globs[fileName]; !exists {
		c.globs[fileName] = GlobPath{result, fileName}
	}
	c.globLock.Unlock()

	if exists {
		// Getting the list raced with another goroutine, throw away the results and use theirs
		verifyGlob(fileName, pattern, excludes, g)
		// Return a copy so that modifications don't affect the cached value.
		return append([]string(nil), g.Matches...), nil
	}

	// Return a copy so that modifications don't affect the cached value.
	return append([]string(nil), result.Matches...), nil
}

func (c *Context) Globs() []GlobPath {
	fileNames := make([]string, 0, len(c.globs))
	for k := range c.globs {
		fileNames = append(fileNames, k)
	}
	sort.Strings(fileNames)

	globs := make([]GlobPath, len(fileNames))
	for i, fileName := range fileNames {
		globs[i] = c.globs[fileName]
	}

	return globs
}

func globToString(pattern string) string {
	ret := ""
	for _, c := range pattern {
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_', c == '-', c == '/':
			ret += string(c)
		default:
			ret += "_"
		}
	}

	return ret
}

func globToFileName(pattern string, excludes []string) string {
	name := globToString(pattern)
	excludeName := ""
	for _, e := range excludes {
		excludeName += "__" + globToString(e)
	}

	// Prevent file names from reaching ninja's path component limit
	if strings.Count(name, "/")+strings.Count(excludeName, "/") > 30 {
		excludeName = fmt.Sprintf("___%x", md5.Sum([]byte(excludeName)))
	}

	return name + excludeName + ".glob"
}
