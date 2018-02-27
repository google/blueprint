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
	"strconv"
	"testing"
)

var pwd, _ = os.Getwd()

type globTestCase struct {
	pattern  string
	matches  []string
	excludes []string
	deps     []string
	err      error
}

var globTestCases = []globTestCase{
	// Current directory tests
	{
		pattern: "*",
		matches: []string{"a/", "b/", "c/", "d.ext", "e.ext"},
		deps:    []string{"."},
	},
	{
		pattern: "*.ext",
		matches: []string{"d.ext", "e.ext"},
		deps:    []string{"."},
	},
	{
		pattern: "*/a",
		matches: []string{"a/a/", "b/a"},
		deps:    []string{".", "a", "b", "c"},
	},
	{
		pattern: "*/*/a",
		matches: []string{"a/a/a"},
		deps:    []string{".", "a", "b", "c", "a/a", "a/b", "c/f", "c/g", "c/h"},
	},
	{
		pattern: "*/a/a",
		matches: []string{"a/a/a"},
		deps:    []string{".", "a", "b", "c", "a/a"},
	},

	// ./ directory tests
	{
		pattern: "./*",
		matches: []string{"a/", "b/", "c/", "d.ext", "e.ext"},
		deps:    []string{"."},
	},
	{
		pattern: "./*.ext",
		matches: []string{"d.ext", "e.ext"},
		deps:    []string{"."},
	},
	{
		pattern: "./*/a",
		matches: []string{"a/a/", "b/a"},
		deps:    []string{".", "a", "b", "c"},
	},
	{
		pattern: "./[ac]/a",
		matches: []string{"a/a/"},
		deps:    []string{".", "a", "c"},
	},

	// subdirectory tests
	{
		pattern: "c/*/*.ext",
		matches: []string{"c/f/f.ext", "c/g/g.ext"},
		deps:    []string{"c", "c/f", "c/g", "c/h"},
	},
	{
		pattern: "a/*/a",
		matches: []string{"a/a/a"},
		deps:    []string{"a", "a/a", "a/b"},
	},

	// absolute tests
	{
		pattern: filepath.Join(pwd, "testdata/c/*/*.ext"),
		matches: []string{
			filepath.Join(pwd, "testdata/c/f/f.ext"),
			filepath.Join(pwd, "testdata/c/g/g.ext"),
		},
		deps: []string{
			filepath.Join(pwd, "testdata/c"),
			filepath.Join(pwd, "testdata/c/f"),
			filepath.Join(pwd, "testdata/c/g"),
			filepath.Join(pwd, "testdata/c/h"),
		},
	},

	// no-wild tests
	{
		pattern: "a",
		matches: []string{"a/"},
		deps:    []string{"a"},
	},
	{
		pattern: "a/a",
		matches: []string{"a/a/"},
		deps:    []string{"a/a"},
	},

	// clean tests
	{
		pattern: "./c/*/*.ext",
		matches: []string{"c/f/f.ext", "c/g/g.ext"},
		deps:    []string{"c", "c/f", "c/g", "c/h"},
	},
	{
		pattern: "c/../c/*/*.ext",
		matches: []string{"c/f/f.ext", "c/g/g.ext"},
		deps:    []string{"c", "c/f", "c/g", "c/h"},
	},

	// recursive tests
	{
		pattern: "**/a",
		matches: []string{"a/", "a/a/", "a/a/a", "b/a"},
		deps:    []string{".", "a", "a/a", "a/b", "b", "c", "c/f", "c/g", "c/h"},
	},
	{
		pattern: "a/**/a",
		matches: []string{"a/a/", "a/a/a"},
		deps:    []string{"a", "a/a", "a/b"},
	},
	{
		pattern: "a/**/*",
		matches: []string{"a/a/", "a/b/", "a/a/a", "a/b/b"},
		deps:    []string{"a", "a/a", "a/b"},
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
		deps: []string{
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
		deps:     []string{"."},
	},
	{
		pattern:  "*/*",
		excludes: []string{"a/b"},
		matches:  []string{"a/a/", "b/a", "c/c", "c/f/", "c/g/", "c/h/"},
		deps:     []string{".", "a", "b", "c"},
	},
	{
		pattern:  "*/*",
		excludes: []string{"a/b", "c/c"},
		matches:  []string{"a/a/", "b/a", "c/f/", "c/g/", "c/h/"},
		deps:     []string{".", "a", "b", "c"},
	},
	{
		pattern:  "*/*",
		excludes: []string{"c/*", "*/a"},
		matches:  []string{"a/b/"},
		deps:     []string{".", "a", "b", "c"},
	},
	{
		pattern:  "*/*",
		excludes: []string{"*/*"},
		matches:  nil,
		deps:     []string{".", "a", "b", "c"},
	},

	// absolute exclude tests
	{
		pattern:  filepath.Join(pwd, "testdata/c/*/*.ext"),
		excludes: []string{filepath.Join(pwd, "testdata/c/*/f.ext")},
		matches: []string{
			filepath.Join(pwd, "testdata/c/g/g.ext"),
		},
		deps: []string{
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
		deps: []string{
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
		deps:     []string{"."},
	},
	{
		pattern:  "*/*",
		excludes: []string{"**/b"},
		matches:  []string{"a/a/", "b/a", "c/c", "c/f/", "c/g/", "c/h/"},
		deps:     []string{".", "a", "b", "c"},
	},
	{
		pattern:  "*/*",
		excludes: []string{"a/**/*"},
		matches:  []string{"b/a", "c/c", "c/f/", "c/g/", "c/h/"},
		deps:     []string{".", "a", "b", "c"},
	},
	{
		pattern:  "**/*",
		excludes: []string{"**/*"},
		matches:  nil,
		deps:     []string{".", "a", "a/a", "a/b", "b", "c", "c/f", "c/g", "c/h"},
	},
	{
		pattern:  "*/*/*",
		excludes: []string{"a/**/a"},
		matches:  []string{"a/b/b", "c/f/f.ext", "c/g/g.ext", "c/h/h"},
		deps:     []string{".", "a", "b", "c", "a/a", "a/b", "c/f", "c/g", "c/h"},
	},
	{
		pattern:  "*/*/*",
		excludes: []string{"**/a"},
		matches:  []string{"a/b/b", "c/f/f.ext", "c/g/g.ext", "c/h/h"},
		deps:     []string{".", "a", "b", "c", "a/a", "a/b", "c/f", "c/g", "c/h"},
	},
	{
		pattern:  "c/*/*.ext",
		excludes: []string{"c/**/f.ext"},
		matches:  []string{"c/g/g.ext"},
		deps:     []string{"c", "c/f", "c/g", "c/h"},
	},

	// absoulte recursive exclude tests
	{
		pattern:  filepath.Join(pwd, "testdata/c/*/*.ext"),
		excludes: []string{filepath.Join(pwd, "testdata/**/f.ext")},
		matches: []string{
			filepath.Join(pwd, "testdata/c/g/g.ext"),
		},
		deps: []string{
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
		deps:     []string{"c", "c/f", "c/g", "c/h"},
	},
	{
		pattern:  "c/*/*.ext",
		excludes: []string{"./c/*/f.ext"},
		matches:  []string{"c/g/g.ext"},
		deps:     []string{"c", "c/f", "c/g", "c/h"},
	},
	{
		pattern:  "./c/*/*.ext",
		excludes: []string{"c/*/f.ext"},
		matches:  []string{"c/g/g.ext"},
		deps:     []string{"c", "c/f", "c/g", "c/h"},
	},

	// non-existant non-wild path tests
	{
		pattern: "d/*",
		matches: nil,
		deps:    []string{"."},
	},
	{
		pattern: "d",
		matches: nil,
		deps:    []string{"."},
	},
	{
		pattern: "a/d/*",
		matches: nil,
		deps:    []string{"a"},
	},
	{
		pattern: "a/d",
		matches: nil,
		deps:    []string{"a"},
	},
	{
		pattern: "a/a/d/*",
		matches: nil,
		deps:    []string{"a/a"},
	},
	{
		pattern: "a/a/d",
		matches: nil,
		deps:    []string{"a/a"},
	},
	{
		pattern: "a/d/a/*",
		matches: nil,
		deps:    []string{"a"},
	},
	{
		pattern: "a/d/a",
		matches: nil,
		deps:    []string{"a"},
	},
	{
		pattern: "a/d/a/*/a",
		matches: nil,
		deps:    []string{"a"},
	},
	{
		pattern: "a/d/a/**/a",
		matches: nil,
		deps:    []string{"a"},
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

	// If names are excluded by default, but referenced explicitly, they should return results
	{
		pattern: ".test/*",
		matches: []string{".test/a"},
		deps:    []string{".test"},
	},
	{
		pattern: ".t*/a",
		matches: []string{".test/a"},
		deps:    []string{".", ".test"},
	},
	{
		pattern: ".*/.*",
		matches: []string{".test/.ing"},
		deps:    []string{".", ".test"},
	},
	{
		pattern: ".t*",
		matches: []string{".test/", ".testing"},
		deps:    []string{"."},
	},
}

func TestMockGlob(t *testing.T) {
	files := []string{
		"a/a/a",
		"a/b/b",
		"b/a",
		"c/c",
		"c/f/f.ext",
		"c/g/g.ext",
		"c/h/h",
		"d.ext",
		"e.ext",
		".test/a",
		".testing",
		".test/.ing",
	}

	mockFiles := make(map[string][]byte)

	for _, f := range files {
		mockFiles[f] = nil
		mockFiles[filepath.Join(pwd, "testdata", f)] = nil
	}

	mock := MockFs(mockFiles)

	for i, testCase := range globTestCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			testGlob(t, mock, testCase)
		})
	}
}

func TestGlob(t *testing.T) {
	os.Chdir("testdata")
	defer os.Chdir("..")
	for i, testCase := range globTestCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			testGlob(t, OsFs, testCase)
		})
	}
}

func testGlob(t *testing.T, fs FileSystem, testCase globTestCase) {
	matches, deps, err := fs.Glob(testCase.pattern, testCase.excludes)
	if err != testCase.err {
		t.Errorf(" pattern: %q", testCase.pattern)
		if testCase.excludes != nil {
			t.Errorf("excludes: %q", testCase.excludes)
		}
		t.Errorf("   error: %s", err)
		return
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
	if !reflect.DeepEqual(deps, testCase.deps) {
		t.Errorf("incorrect deps list:")
		t.Errorf(" pattern: %q", testCase.pattern)
		if testCase.excludes != nil {
			t.Errorf("excludes: %q", testCase.excludes)
		}
		t.Errorf("     got: %#v", deps)
		t.Errorf("expected: %#v", testCase.deps)
	}
}
