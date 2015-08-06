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
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var pwd, _ = os.Getwd()

var globTestCases = []struct {
	pattern  string
	matches  []string
	excludes []string
	dirs     []string
	err      error
}{
	// Current directory tests
	{
		pattern: "*",
		matches: []string{"a", "b", "c", "d.ext", "e.ext"},
		dirs:    []string{"."},
	},
	{
		pattern: "*.ext",
		matches: []string{"d.ext", "e.ext"},
		dirs:    []string{"."},
	},
	{
		pattern: "*/a",
		matches: []string{"a/a", "b/a"},
		dirs:    []string{".", "a", "b", "c"},
	},
	{
		pattern: "*/*/a",
		matches: []string{"a/a/a"},
		dirs:    []string{".", "a", "b", "c", "a/a", "a/b", "c/f", "c/g", "c/h"},
	},
	{
		pattern: "*/a/a",
		matches: []string{"a/a/a"},
		dirs:    []string{".", "a", "b", "c", "a/a"},
	},

	// ./ directory tests
	{
		pattern: "./*",
		matches: []string{"a", "b", "c", "d.ext", "e.ext"},
		dirs:    []string{"."},
	},
	{
		pattern: "./*.ext",
		matches: []string{"d.ext", "e.ext"},
		dirs:    []string{"."},
	},
	{
		pattern: "./*/a",
		matches: []string{"a/a", "b/a"},
		dirs:    []string{".", "a", "b", "c"},
	},
	{
		pattern: "./[ac]/a",
		matches: []string{"a/a"},
		dirs:    []string{".", "a", "c"},
	},

	// subdirectory tests
	{
		pattern: "c/*/*.ext",
		matches: []string{"c/f/f.ext", "c/g/g.ext"},
		dirs:    []string{"c", "c/f", "c/g", "c/h"},
	},
	{
		pattern: "a/*/a",
		matches: []string{"a/a/a"},
		dirs:    []string{"a", "a/a", "a/b"},
	},

	// absolute tests
	{
		pattern: filepath.Join(pwd, "testdata/c/*/*.ext"),
		matches: []string{
			filepath.Join(pwd, "testdata/c/f/f.ext"),
			filepath.Join(pwd, "testdata/c/g/g.ext"),
		},
		dirs: []string{
			filepath.Join(pwd, "testdata/c"),
			filepath.Join(pwd, "testdata/c/f"),
			filepath.Join(pwd, "testdata/c/g"),
			filepath.Join(pwd, "testdata/c/h"),
		},
	},

	// no-wild tests
	{
		pattern: "a",
		matches: []string{"a"},
		dirs:    nil,
	},
	{
		pattern: "a/a",
		matches: []string{"a/a"},
		dirs:    nil,
	},

	// clean tests
	{
		pattern: "./c/*/*.ext",
		matches: []string{"c/f/f.ext", "c/g/g.ext"},
		dirs:    []string{"c", "c/f", "c/g", "c/h"},
	},
	{
		pattern: "c/../c/*/*.ext",
		matches: []string{"c/f/f.ext", "c/g/g.ext"},
		dirs:    []string{"c", "c/f", "c/g", "c/h"},
	},

	// recursive tests
	{
		pattern: "**/a",
		matches: []string{"a", "a/a", "a/a/a", "b/a"},
		dirs:    []string{".", "a", "a/a", "a/b", "b", "c", "c/f", "c/g", "c/h"},
	},
	{
		pattern: "a/**/a",
		matches: []string{"a/a", "a/a/a"},
		dirs:    []string{"a", "a/a", "a/b"},
	},
	{
		pattern: "a/**/*",
		matches: []string{"a/a", "a/b", "a/a/a", "a/b/b"},
		dirs:    []string{"a", "a/a", "a/b"},
	},

	// absolute recursive tests
	{
		pattern: filepath.Join(pwd, "testdata/**/*.ext"),
		matches: []string{
			filepath.Join(pwd, "testdata/d.ext"),
			filepath.Join(pwd, "testdata/e.ext"),
			filepath.Join(pwd, "testdata/c/f/f.ext"),
			filepath.Join(pwd, "testdata/c/g/g.ext"),
		},
		dirs: []string{
			filepath.Join(pwd, "testdata"),
			filepath.Join(pwd, "testdata/a"),
			filepath.Join(pwd, "testdata/a/a"),
			filepath.Join(pwd, "testdata/a/b"),
			filepath.Join(pwd, "testdata/b"),
			filepath.Join(pwd, "testdata/c"),
			filepath.Join(pwd, "testdata/c/f"),
			filepath.Join(pwd, "testdata/c/g"),
			filepath.Join(pwd, "testdata/c/h"),
		},
	},

	// recursive error tests
	{
		pattern: "**/**/*",
		err:     GlobMultipleRecursiveErr,
	},
	{
		pattern: "a/**/**/*",
		err:     GlobMultipleRecursiveErr,
	},
	{
		pattern: "**/a/**/*",
		err:     GlobMultipleRecursiveErr,
	},
	{
		pattern: "**/**/a/*",
		err:     GlobMultipleRecursiveErr,
	},
	{
		pattern: "a/**",
		err:     GlobLastRecursiveErr,
	},
	{
		pattern: "**/**",
		err:     GlobLastRecursiveErr,
	},

	// exclude tests
	{
		pattern:  "*.ext",
		excludes: []string{"d.ext"},
		matches:  []string{"e.ext"},
		dirs:     []string{"."},
	},
	{
		pattern:  "*/*",
		excludes: []string{"a/b"},
		matches:  []string{"a/a", "b/a", "c/c", "c/f", "c/g", "c/h"},
		dirs:     []string{".", "a", "b", "c"},
	},
	{
		pattern:  "*/*",
		excludes: []string{"a/b", "c/c"},
		matches:  []string{"a/a", "b/a", "c/f", "c/g", "c/h"},
		dirs:     []string{".", "a", "b", "c"},
	},
	{
		pattern:  "*/*",
		excludes: []string{"c/*", "*/a"},
		matches:  []string{"a/b"},
		dirs:     []string{".", "a", "b", "c"},
	},
	{
		pattern:  "*/*",
		excludes: []string{"*/*"},
		matches:  nil,
		dirs:     []string{".", "a", "b", "c"},
	},

	// absolute exclude tests
	{
		pattern:  filepath.Join(pwd, "testdata/c/*/*.ext"),
		excludes: []string{filepath.Join(pwd, "testdata/c/*/f.ext")},
		matches: []string{
			filepath.Join(pwd, "testdata/c/g/g.ext"),
		},
		dirs: []string{
			filepath.Join(pwd, "testdata/c"),
			filepath.Join(pwd, "testdata/c/f"),
			filepath.Join(pwd, "testdata/c/g"),
			filepath.Join(pwd, "testdata/c/h"),
		},
	},
	{
		pattern:  filepath.Join(pwd, "testdata/c/*/*.ext"),
		excludes: []string{filepath.Join(pwd, "testdata/c/f/*.ext")},
		matches: []string{
			filepath.Join(pwd, "testdata/c/g/g.ext"),
		},
		dirs: []string{
			filepath.Join(pwd, "testdata/c"),
			filepath.Join(pwd, "testdata/c/f"),
			filepath.Join(pwd, "testdata/c/g"),
			filepath.Join(pwd, "testdata/c/h"),
		},
	},

	// recursive exclude tests
	{
		pattern:  "*.ext",
		excludes: []string{"**/*.ext"},
		matches:  nil,
		dirs:     []string{"."},
	},
	{
		pattern:  "*/*",
		excludes: []string{"**/b"},
		matches:  []string{"a/a", "b/a", "c/c", "c/f", "c/g", "c/h"},
		dirs:     []string{".", "a", "b", "c"},
	},
	{
		pattern:  "*/*",
		excludes: []string{"a/**/*"},
		matches:  []string{"b/a", "c/c", "c/f", "c/g", "c/h"},
		dirs:     []string{".", "a", "b", "c"},
	},
	{
		pattern:  "**/*",
		excludes: []string{"**/*"},
		matches:  nil,
		dirs:     []string{".", "a", "a/a", "a/b", "b", "c", "c/f", "c/g", "c/h"},
	},
	{
		pattern:  "*/*/*",
		excludes: []string{"a/**/a"},
		matches:  []string{"a/b/b", "c/f/f.ext", "c/g/g.ext", "c/h/h"},
		dirs:     []string{".", "a", "b", "c", "a/a", "a/b", "c/f", "c/g", "c/h"},
	},
	{
		pattern:  "*/*/*",
		excludes: []string{"**/a"},
		matches:  []string{"a/b/b", "c/f/f.ext", "c/g/g.ext", "c/h/h"},
		dirs:     []string{".", "a", "b", "c", "a/a", "a/b", "c/f", "c/g", "c/h"},
	},
	{
		pattern:  "c/*/*.ext",
		excludes: []string{"c/**/f.ext"},
		matches:  []string{"c/g/g.ext"},
		dirs:     []string{"c", "c/f", "c/g", "c/h"},
	},

	// absoulte recursive exclude tests
	{
		pattern:  filepath.Join(pwd, "testdata/c/*/*.ext"),
		excludes: []string{filepath.Join(pwd, "testdata/**/f.ext")},
		matches: []string{
			filepath.Join(pwd, "testdata/c/g/g.ext"),
		},
		dirs: []string{
			filepath.Join(pwd, "testdata/c"),
			filepath.Join(pwd, "testdata/c/f"),
			filepath.Join(pwd, "testdata/c/g"),
			filepath.Join(pwd, "testdata/c/h"),
		},
	},

	// clean exclude tests
	{
		pattern:  "./c/*/*.ext",
		excludes: []string{"./c/*/f.ext"},
		matches:  []string{"c/g/g.ext"},
		dirs:     []string{"c", "c/f", "c/g", "c/h"},
	},
	{
		pattern:  "c/*/*.ext",
		excludes: []string{"./c/*/f.ext"},
		matches:  []string{"c/g/g.ext"},
		dirs:     []string{"c", "c/f", "c/g", "c/h"},
	},
	{
		pattern:  "./c/*/*.ext",
		excludes: []string{"c/*/f.ext"},
		matches:  []string{"c/g/g.ext"},
		dirs:     []string{"c", "c/f", "c/g", "c/h"},
	},

	// non-existant non-wild path tests
	{
		pattern: "d/*",
		matches: nil,
		dirs:    []string{"."},
	},
	{
		pattern: "d",
		matches: nil,
		dirs:    []string{"."},
	},
	{
		pattern: "a/d/*",
		matches: nil,
		dirs:    []string{"a"},
	},
	{
		pattern: "a/d",
		matches: nil,
		dirs:    []string{"a"},
	},
	{
		pattern: "a/a/d/*",
		matches: nil,
		dirs:    []string{"a/a"},
	},
	{
		pattern: "a/a/d",
		matches: nil,
		dirs:    []string{"a/a"},
	},
	{
		pattern: "a/d/a/*",
		matches: nil,
		dirs:    []string{"a"},
	},
	{
		pattern: "a/d/a",
		matches: nil,
		dirs:    []string{"a"},
	},
	{
		pattern: "a/d/a/*/a",
		matches: nil,
		dirs:    []string{"a"},
	},
	{
		pattern: "a/d/a/**/a",
		matches: nil,
		dirs:    []string{"a"},
	},

	// recursive exclude error tests
	{
		pattern:  "**/*",
		excludes: []string{"**/**/*"},
		err:      GlobMultipleRecursiveErr,
	},
	{
		pattern:  "**/*",
		excludes: []string{"a/**/**/*"},
		err:      GlobMultipleRecursiveErr,
	},
	{
		pattern:  "**/*",
		excludes: []string{"**/a/**/*"},
		err:      GlobMultipleRecursiveErr,
	},
	{
		pattern:  "**/*",
		excludes: []string{"**/**/a/*"},
		err:      GlobMultipleRecursiveErr,
	},
	{
		pattern:  "**/*",
		excludes: []string{"a/**"},
		err:      GlobLastRecursiveErr,
	},
	{
		pattern:  "**/*",
		excludes: []string{"**/**"},
		err:      GlobLastRecursiveErr,
	},
}

func TestGlob(t *testing.T) {
	os.Chdir("testdata")
	defer os.Chdir("..")
	for _, testCase := range globTestCases {
		matches, dirs, err := GlobWithExcludes(testCase.pattern, testCase.excludes)
		if err != testCase.err {
			t.Errorf(" pattern: %q", testCase.pattern)
			if testCase.excludes != nil {
				t.Errorf("excludes: %q", testCase.excludes)
			}
			t.Errorf("   error: %s", err)
			continue
		}

		if !reflect.DeepEqual(matches, testCase.matches) {
			t.Errorf("incorrect matches list:")
			t.Errorf(" pattern: %q", testCase.pattern)
			if testCase.excludes != nil {
				t.Errorf("excludes: %q", testCase.excludes)
			}
			t.Errorf("     got: %#v", matches)
			t.Errorf("expected: %#v", testCase.matches)
		}
		if !reflect.DeepEqual(dirs, testCase.dirs) {
			t.Errorf("incorrect dirs list:")
			t.Errorf(" pattern: %q", testCase.pattern)
			if testCase.excludes != nil {
				t.Errorf("excludes: %q", testCase.excludes)
			}
			t.Errorf("     got: %#v", dirs)
			t.Errorf("expected: %#v", testCase.dirs)
		}
	}
}
