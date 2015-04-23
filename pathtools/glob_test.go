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
	pattern string
	matches []string
	dirs    []string
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
}

func TestGlob(t *testing.T) {
	os.Chdir("testdata")
	defer os.Chdir("..")
	for _, testCase := range globTestCases {
		matches, dirs, err := Glob(testCase.pattern)
		if err != nil {
			t.Errorf(" pattern: %q", testCase.pattern)
			t.Errorf("   error: %s", err.Error())
			continue
		}

		if !reflect.DeepEqual(matches, testCase.matches) {
			t.Errorf("incorrect matches list:")
			t.Errorf(" pattern: %q", testCase.pattern)
			t.Errorf("     got: %#v", matches)
			t.Errorf("expected: %#v", testCase.matches)
		}
		if !reflect.DeepEqual(dirs, testCase.dirs) {
			t.Errorf("incorrect dirs list:")
			t.Errorf(" pattern: %q", testCase.pattern)
			t.Errorf("     got: %#v", dirs)
			t.Errorf("expected: %#v", testCase.dirs)
		}
	}
}
