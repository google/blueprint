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
	"strings"
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
	{
		pattern: "c/*/?",
		matches: []string{"c/h/h"},
		deps:    []string{"c", "c/f", "c/g", "c/h"},
	},
	{
		pattern: "c/*/[gh]*",
		matches: []string{"c/g/g.ext", "c/h/h"},
		deps:    []string{"c", "c/f", "c/g", "c/h"},
	},
	{
		pattern: "c/*/[fgh]*",
		matches: []string{"c/f/f.ext", "c/g/g.ext", "c/h/h"},
		deps:    []string{"c", "c/f", "c/g", "c/h"},
	},
	{
		pattern: "c/*/[f-h]*",
		matches: []string{"c/f/f.ext", "c/g/g.ext", "c/h/h"},
		deps:    []string{"c", "c/f", "c/g", "c/h"},
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
		pattern: filepath.Join(pwd, "testdata/glob/c/*/*.ext"),
		matches: []string{
			filepath.Join(pwd, "testdata/glob/c/f/f.ext"),
			filepath.Join(pwd, "testdata/glob/c/g/g.ext"),
		},
		deps: []string{
			filepath.Join(pwd, "testdata/glob/c"),
			filepath.Join(pwd, "testdata/glob/c/f"),
			filepath.Join(pwd, "testdata/glob/c/g"),
			filepath.Join(pwd, "testdata/glob/c/h"),
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
		pattern: filepath.Join(pwd, "testdata/glob/**/*.ext"),
		matches: []string{
			filepath.Join(pwd, "testdata/glob/d.ext"),
			filepath.Join(pwd, "testdata/glob/e.ext"),
			filepath.Join(pwd, "testdata/glob/c/f/f.ext"),
			filepath.Join(pwd, "testdata/glob/c/g/g.ext"),
		},
		deps: []string{
			filepath.Join(pwd, "testdata/glob"),
			filepath.Join(pwd, "testdata/glob/a"),
			filepath.Join(pwd, "testdata/glob/a/a"),
			filepath.Join(pwd, "testdata/glob/a/b"),
			filepath.Join(pwd, "testdata/glob/b"),
			filepath.Join(pwd, "testdata/glob/c"),
			filepath.Join(pwd, "testdata/glob/c/f"),
			filepath.Join(pwd, "testdata/glob/c/g"),
			filepath.Join(pwd, "testdata/glob/c/h"),
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
	{
		pattern: "a**/",
		err:     GlobInvalidRecursiveErr,
	},
	{
		pattern: "**a/",
		err:     GlobInvalidRecursiveErr,
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
		pattern:  filepath.Join(pwd, "testdata/glob/c/*/*.ext"),
		excludes: []string{filepath.Join(pwd, "testdata/glob/c/*/f.ext")},
		matches: []string{
			filepath.Join(pwd, "testdata/glob/c/g/g.ext"),
		},
		deps: []string{
			filepath.Join(pwd, "testdata/glob/c"),
			filepath.Join(pwd, "testdata/glob/c/f"),
			filepath.Join(pwd, "testdata/glob/c/g"),
			filepath.Join(pwd, "testdata/glob/c/h"),
		},
	},
	{
		pattern:  filepath.Join(pwd, "testdata/glob/c/*/*.ext"),
		excludes: []string{filepath.Join(pwd, "testdata/glob/c/f/*.ext")},
		matches: []string{
			filepath.Join(pwd, "testdata/glob/c/g/g.ext"),
		},
		deps: []string{
			filepath.Join(pwd, "testdata/glob/c"),
			filepath.Join(pwd, "testdata/glob/c/f"),
			filepath.Join(pwd, "testdata/glob/c/g"),
			filepath.Join(pwd, "testdata/glob/c/h"),
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
		pattern:  filepath.Join(pwd, "testdata/glob/c/*/*.ext"),
		excludes: []string{filepath.Join(pwd, "testdata/glob/**/f.ext")},
		matches: []string{
			filepath.Join(pwd, "testdata/glob/c/g/g.ext"),
		},
		deps: []string{
			filepath.Join(pwd, "testdata/glob/c"),
			filepath.Join(pwd, "testdata/glob/c/f"),
			filepath.Join(pwd, "testdata/glob/c/g"),
			filepath.Join(pwd, "testdata/glob/c/h"),
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
		mockFiles[filepath.Join(pwd, "testdata/glob", f)] = nil
	}

	mock := MockFs(mockFiles)

	for _, testCase := range globTestCases {
		t.Run(testCase.pattern, func(t *testing.T) {
			testGlob(t, mock, testCase, FollowSymlinks)
		})
	}
}

func TestGlob(t *testing.T) {
	os.Chdir("testdata/glob")
	defer os.Chdir("../..")
	for _, testCase := range globTestCases {
		t.Run(testCase.pattern, func(t *testing.T) {
			testGlob(t, OsFs, testCase, FollowSymlinks)
		})
	}
}

var globEscapeTestCases = []globTestCase{
	{
		pattern: `**/*`,
		matches: []string{`*`, `**/`, `?`, `a/`, `b`, `**/*`, `**/a`, `**/b/`, `**/b/b`, `a/a`},
		deps:    []string{`.`, `**`, `**/b`, `a`},
	},
	{
		pattern: `**/\*`,
		matches: []string{`*`, `**/*`},
		deps:    []string{`.`, `**`, `**/b`, `a`},
	},
	{
		pattern: `\*\*/*`,
		matches: []string{`**/*`, `**/a`, `**/b/`},
		deps:    []string{`.`, `**`},
	},
	{
		pattern: `\*\*/**/*`,
		matches: []string{`**/*`, `**/a`, `**/b/`, `**/b/b`},
		deps:    []string{`.`, `**`, `**/b`},
	},
}

func TestMockGlobEscapes(t *testing.T) {
	files := []string{
		`*`,
		`**/*`,
		`**/a`,
		`**/b/b`,
		`?`,
		`a/a`,
		`b`,
	}

	mockFiles := make(map[string][]byte)

	for _, f := range files {
		mockFiles[f] = nil
	}

	mock := MockFs(mockFiles)

	for _, testCase := range globEscapeTestCases {
		t.Run(testCase.pattern, func(t *testing.T) {
			testGlob(t, mock, testCase, FollowSymlinks)
		})
	}

}

func TestGlobEscapes(t *testing.T) {
	os.Chdir("testdata/escapes")
	defer os.Chdir("../..")
	for _, testCase := range globEscapeTestCases {
		t.Run(testCase.pattern, func(t *testing.T) {
			testGlob(t, OsFs, testCase, FollowSymlinks)
		})
	}

}

var globSymlinkTestCases = []globTestCase{
	{
		pattern: `**/*`,
		matches: []string{"a/", "b/", "c/", "d/", "e", "a/a/", "a/a/a", "b/a/", "b/a/a", "c/a", "d/a"},
		deps:    []string{".", "a", "a/a", "b", "b/a", "c", "d"},
	},
	{
		pattern: `b/**/*`,
		matches: []string{"b/a/", "b/a/a"},
		deps:    []string{"b", "b/a"},
	},
}

func TestMockGlobSymlinks(t *testing.T) {
	files := []string{
		"a/a/a",
		"b -> a",
		"c -> a/a",
		"d -> c",
		"e -> a/a/a",
	}

	mockFiles := make(map[string][]byte)

	for _, f := range files {
		mockFiles[f] = nil
	}

	mock := MockFs(mockFiles)

	for _, testCase := range globSymlinkTestCases {
		t.Run(testCase.pattern, func(t *testing.T) {
			testGlob(t, mock, testCase, FollowSymlinks)
		})
	}
}

func TestGlobSymlinks(t *testing.T) {
	os.Chdir("testdata/symlinks")
	defer os.Chdir("../..")

	for _, testCase := range globSymlinkTestCases {
		t.Run(testCase.pattern, func(t *testing.T) {
			testGlob(t, OsFs, testCase, FollowSymlinks)
		})
	}
}

var globDontFollowSymlinkTestCases = []globTestCase{
	{
		pattern: `**/*`,
		matches: []string{"a/", "b", "c", "d", "e", "a/a/", "a/a/a"},
		deps:    []string{".", "a", "a/a"},
	},
	{
		pattern: `b/**/*`,
		matches: []string{"b/a/", "b/a/a"},
		deps:    []string{"b", "b/a"},
	},
}

func TestMockGlobDontFollowSymlinks(t *testing.T) {
	files := []string{
		"a/a/a",
		"b -> a",
		"c -> a/a",
		"d -> c",
		"e -> a/a/a",
	}

	mockFiles := make(map[string][]byte)

	for _, f := range files {
		mockFiles[f] = nil
	}

	mock := MockFs(mockFiles)

	for _, testCase := range globDontFollowSymlinkTestCases {
		t.Run(testCase.pattern, func(t *testing.T) {
			testGlob(t, mock, testCase, DontFollowSymlinks)
		})
	}
}

func TestGlobDontFollowSymlinks(t *testing.T) {
	os.Chdir("testdata/symlinks")
	defer os.Chdir("../..")

	for _, testCase := range globDontFollowSymlinkTestCases {
		t.Run(testCase.pattern, func(t *testing.T) {
			testGlob(t, OsFs, testCase, DontFollowSymlinks)
		})
	}
}

var globDontFollowDanglingSymlinkTestCases = []globTestCase{
	{
		pattern: `**/*`,
		matches: []string{"a/", "b", "c", "d", "dangling", "e", "f", "a/a/", "a/a/a", "a/a/f"},
		deps:    []string{".", "a", "a/a"},
	},
	{
		pattern: `dangling`,
		matches: []string{"dangling"},
		deps:    []string{"dangling"},
	},
}

func TestMockGlobDontFollowDanglingSymlinks(t *testing.T) {
	files := []string{
		"a/a/a",
		"a/a/f -> ../../f",
		"b -> a",
		"c -> a/a",
		"d -> c",
		"e -> a/a/a",
		"f",
		"dangling -> missing",
	}

	mockFiles := make(map[string][]byte)

	for _, f := range files {
		mockFiles[f] = nil
	}

	mock := MockFs(mockFiles)

	for _, testCase := range globDontFollowDanglingSymlinkTestCases {
		t.Run(testCase.pattern, func(t *testing.T) {
			testGlob(t, mock, testCase, DontFollowSymlinks)
		})
	}
}

func TestGlobDontFollowDanglingSymlinks(t *testing.T) {
	os.Chdir("testdata/dangling")
	defer os.Chdir("../..")

	for _, testCase := range globDontFollowDanglingSymlinkTestCases {
		t.Run(testCase.pattern, func(t *testing.T) {
			testGlob(t, OsFs, testCase, DontFollowSymlinks)
		})
	}
}

func testGlob(t *testing.T, fs FileSystem, testCase globTestCase, follow ShouldFollowSymlinks) {
	t.Helper()
	matches, deps, err := fs.Glob(testCase.pattern, testCase.excludes, follow)
	if err != testCase.err {
		if err == nil {
			t.Fatalf("missing error: %s", testCase.err)
		} else {
			t.Fatalf("error: %s", err)
		}
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

func TestMatch(t *testing.T) {
	testCases := []struct {
		pattern, name string
		match         bool
	}{
		{"a/*", "b/", false},
		{"a/*", "b/a", false},
		{"a/*", "b/b/", false},
		{"a/*", "b/b/c", false},
		{"a/**/*", "b/", false},
		{"a/**/*", "b/a", false},
		{"a/**/*", "b/b/", false},
		{"a/**/*", "b/b/c", false},

		{"a/*", "a/", false},
		{"a/*", "a/a", true},
		{"a/*", "a/b/", false},
		{"a/*", "a/b/c", false},

		{"a/*/", "a/", false},
		{"a/*/", "a/a", false},
		{"a/*/", "a/b/", true},
		{"a/*/", "a/b/c", false},

		{"a/**/*", "a/", false},
		{"a/**/*", "a/a", true},
		{"a/**/*", "a/b/", false},
		{"a/**/*", "a/b/c", true},

		{"a/**/*/", "a/", false},
		{"a/**/*/", "a/a", false},
		{"a/**/*/", "a/b/", true},
		{"a/**/*/", "a/b/c", false},

		{"**/*", "a/", false},
		{"**/*", "a/a", true},
		{"**/*", "a/b/", false},
		{"**/*", "a/b/c", true},

		{"**/*/", "a/", true},
		{"**/*/", "a/a", false},
		{"**/*/", "a/b/", true},
		{"**/*/", "a/b/c", false},

		{`a/\*\*/\*`, `a/**/*`, true},
		{`a/\*\*/\*`, `a/a/*`, false},
		{`a/\*\*/\*`, `a/**/a`, false},
		{`a/\*\*/\*`, `a/a/a`, false},

		{`a/**/\*`, `a/**/*`, true},
		{`a/**/\*`, `a/a/*`, true},
		{`a/**/\*`, `a/**/a`, false},
		{`a/**/\*`, `a/a/a`, false},

		{`a/\*\*/*`, `a/**/*`, true},
		{`a/\*\*/*`, `a/a/*`, false},
		{`a/\*\*/*`, `a/**/a`, true},
		{`a/\*\*/*`, `a/a/a`, false},

		{`*/**/a`, `a/a/a`, true},
		{`*/**/a`, `*/a/a`, true},
		{`*/**/a`, `a/**/a`, true},
		{`*/**/a`, `*/**/a`, true},

		{`\*/\*\*/a`, `a/a/a`, false},
		{`\*/\*\*/a`, `*/a/a`, false},
		{`\*/\*\*/a`, `a/**/a`, false},
		{`\*/\*\*/a`, `*/**/a`, true},

		{`a/?`, `a/?`, true},
		{`a/?`, `a/a`, true},
		{`a/\?`, `a/?`, true},
		{`a/\?`, `a/a`, false},

		{`a/?`, `a/?`, true},
		{`a/?`, `a/a`, true},
		{`a/\?`, `a/?`, true},
		{`a/\?`, `a/a`, false},

		{`a/[a-c]`, `a/b`, true},
		{`a/[abc]`, `a/b`, true},

		{`a/\[abc]`, `a/b`, false},
		{`a/\[abc]`, `a/[abc]`, true},

		{`a/\[abc\]`, `a/b`, false},
		{`a/\[abc\]`, `a/[abc]`, true},

		{`a/?`, `a/?`, true},
		{`a/?`, `a/a`, true},
		{`a/\?`, `a/?`, true},
		{`a/\?`, `a/a`, false},

		{"/a/*", "/a/", false},
		{"/a/*", "/a/a", true},
		{"/a/*", "/a/b/", false},
		{"/a/*", "/a/b/c", false},

		{"/a/*/", "/a/", false},
		{"/a/*/", "/a/a", false},
		{"/a/*/", "/a/b/", true},
		{"/a/*/", "/a/b/c", false},

		{"/a/**/*", "/a/", false},
		{"/a/**/*", "/a/a", true},
		{"/a/**/*", "/a/b/", false},
		{"/a/**/*", "/a/b/c", true},

		{"/**/*", "/a/", false},
		{"/**/*", "/a/a", true},
		{"/**/*", "/a/b/", false},
		{"/**/*", "/a/b/c", true},

		{"/**/*/", "/a/", true},
		{"/**/*/", "/a/a", false},
		{"/**/*/", "/a/b/", true},
		{"/**/*/", "/a/b/c", false},

		{`a`, `/a`, false},
		{`/a`, `a`, false},
		{`*`, `/a`, false},
		{`/*`, `a`, false},
		{`**/*`, `/a`, false},
		{`/**/*`, `a`, false},
	}

	for _, test := range testCases {
		t.Run(test.pattern+","+test.name, func(t *testing.T) {
			match, err := Match(test.pattern, test.name)
			if err != nil {
				t.Fatal(err)
			}
			if match != test.match {
				t.Errorf("want: %v, got %v", test.match, match)
			}
		})
	}

	// Run the same test cases through Glob
	for _, test := range testCases {
		// Glob and Match disagree on matching directories
		if strings.HasSuffix(test.name, "/") || strings.HasSuffix(test.pattern, "/") {
			continue
		}
		t.Run("glob:"+test.pattern+","+test.name, func(t *testing.T) {
			mockFiles := map[string][]byte{
				test.name: nil,
			}

			mock := MockFs(mockFiles)

			matches, _, err := mock.Glob(test.pattern, nil, DontFollowSymlinks)
			t.Log(test.name, test.pattern, matches)
			if err != nil {
				t.Fatal(err)
			}

			match := false
			for _, x := range matches {
				if x == test.name {
					match = true
				}
			}

			if match != test.match {
				t.Errorf("want: %v, got %v", test.match, match)
			}
		})
	}
}
