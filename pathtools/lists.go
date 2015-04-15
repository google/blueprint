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

func ReplaceExtensions(paths []string, extension string) []string {
	result := make([]string, len(paths))
	for i, path := range paths {
		result[i] = ReplaceExtension(path, extension)
	}
	return result
}

func ReplaceExtension(path string, extension string) string {
	dot := strings.LastIndex(path, ".")
	if dot == -1 {
		return path
	}
	return path[:dot+1] + extension
}
