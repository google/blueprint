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
	"fmt"
	"sort"
	"strings"

	"github.com/google/blueprint/pathtools"
)

func verifyGlob(key globKey, pattern string, excludes []string, g pathtools.GlobResult) {
	if pattern != g.Pattern {
		panic(fmt.Errorf("Mismatched patterns %q and %q for glob key %q", pattern, g.Pattern, key))
	}
	if len(excludes) != len(g.Excludes) {
		panic(fmt.Errorf("Mismatched excludes %v and %v for glob key %q", excludes, g.Excludes, key))
	}

	for i := range excludes {
		if g.Excludes[i] != excludes[i] {
			panic(fmt.Errorf("Mismatched excludes %v and %v for glob key %q", excludes, g.Excludes, key))
		}
	}
}

func (c *Context) glob(pattern string, excludes []string) ([]string, error) {
	// Sort excludes so that two globs with the same excludes in a different order reuse the same
	// key.  Make a copy first to avoid modifying the caller's version.
	excludes = append([]string(nil), excludes...)
	sort.Strings(excludes)

	key := globToKey(pattern, excludes)

	// Try to get existing glob from the stored results
	c.globLock.Lock()
	g, exists := c.globs[key]
	c.globLock.Unlock()

	if exists {
		// Glob has already been done, double check it is identical
		verifyGlob(key, pattern, excludes, g)
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
	if g, exists = c.globs[key]; !exists {
		c.globs[key] = result
	}
	c.globLock.Unlock()

	if exists {
		// Getting the list raced with another goroutine, throw away the results and use theirs
		verifyGlob(key, pattern, excludes, g)
		// Return a copy so that modifications don't affect the cached value.
		return append([]string(nil), g.Matches...), nil
	}

	// Return a copy so that modifications don't affect the cached value.
	return append([]string(nil), result.Matches...), nil
}

func (c *Context) Globs() pathtools.MultipleGlobResults {
	keys := make([]globKey, 0, len(c.globs))
	for k := range c.globs {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].pattern != keys[j].pattern {
			return keys[i].pattern < keys[j].pattern
		}
		return keys[i].excludes < keys[j].excludes
	})

	globs := make(pathtools.MultipleGlobResults, len(keys))
	for i, key := range keys {
		globs[i] = c.globs[key]
	}

	return globs
}

// globKey combines a pattern and a list of excludes into a hashable struct to be used as a key in
// a map.
type globKey struct {
	pattern  string
	excludes string
}

// globToKey converts a pattern and an excludes list into a globKey struct that is hashable and
// usable as a key in a map.
func globToKey(pattern string, excludes []string) globKey {
	return globKey{pattern, strings.Join(excludes, "|")}
}
