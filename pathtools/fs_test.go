// Copyright 2018 Google Inc. All rights reserved.
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
	"syscall"
	"testing"
)

func symlinkMockFs() *mockFs {
	files := []string{
		"a/a/a",
		"a/a/f -> ../../f",
		"b -> a",
		"c -> a/a",
		"d -> c",
		"e -> a/a/a",
		"dangling -> missing",
		"f",
	}

	mockFiles := make(map[string][]byte)

	for _, f := range files {
		mockFiles[f] = nil
		mockFiles[filepath.Join(pwd, "testdata", f)] = nil
	}

	return MockFs(mockFiles).(*mockFs)
}

func TestMockFs_followSymlinks(t *testing.T) {

	testCases := []struct {
		from, to string
	}{
		{".", "."},
		{"/", "/"},

		{"a", "a"},
		{"a/a", "a/a"},
		{"a/a/a", "a/a/a"},
		{"a/a/f", "f"},

		{"b", "a"},
		{"b/a", "a/a"},
		{"b/a/a", "a/a/a"},
		{"b/a/f", "f"},

		{"c/a", "a/a/a"},
		{"c/f", "f"},

		{"d/a", "a/a/a"},
		{"c/f", "f"},

		{"e", "a/a/a"},

		{"f", "f"},

		{"dangling", "missing"},

		{"a/missing", "a/missing"},
		{"b/missing", "a/missing"},
		{"c/missing", "a/a/missing"},
		{"d/missing", "a/a/missing"},
		{"e/missing", "a/a/a/missing"},
		{"dangling/missing", "missing/missing"},

		{"a/missing/missing", "a/missing/missing"},
		{"b/missing/missing", "a/missing/missing"},
		{"c/missing/missing", "a/a/missing/missing"},
		{"d/missing/missing", "a/a/missing/missing"},
		{"e/missing/missing", "a/a/a/missing/missing"},
		{"dangling/missing/missing", "missing/missing/missing"},
	}

	mock := symlinkMockFs()

	for _, test := range testCases {
		t.Run(test.from, func(t *testing.T) {
			got := mock.followSymlinks(test.from)
			if got != test.to {
				t.Errorf("want: %v, got %v", test.to, got)
			}
		})
	}
}

func TestMockFs_IsDir(t *testing.T) {
	testCases := []struct {
		name  string
		isDir bool
		err   error
	}{
		{"a", true, nil},
		{"a/a", true, nil},
		{"a/a/a", false, nil},
		{"a/a/f", false, nil},

		{"b", true, nil},
		{"b/a", true, nil},
		{"b/a/a", false, nil},
		{"b/a/f", false, nil},

		{"c", true, nil},
		{"c/a", false, nil},
		{"c/f", false, nil},

		{"d", true, nil},
		{"d/a", false, nil},
		{"d/f", false, nil},

		{"e", false, nil},

		{"f", false, nil},

		{"dangling", false, os.ErrNotExist},

		{"a/missing", false, os.ErrNotExist},
		{"b/missing", false, os.ErrNotExist},
		{"c/missing", false, os.ErrNotExist},
		{"d/missing", false, os.ErrNotExist},
		{"e/missing", false, os.NewSyscallError("stat e/missing", syscall.ENOTDIR)},
		{"dangling/missing", false, os.ErrNotExist},

		{"a/missing/missing", false, os.ErrNotExist},
		{"b/missing/missing", false, os.ErrNotExist},
		{"c/missing/missing", false, os.ErrNotExist},
		{"d/missing/missing", false, os.ErrNotExist},
		{"e/missing/missing", false, os.NewSyscallError("stat e/missing/missing", syscall.ENOTDIR)},
		{"dangling/missing/missing", false, os.ErrNotExist},

		{"c/f/missing", false, os.NewSyscallError("stat c/f/missing", syscall.ENOTDIR)},
	}

	mock := symlinkMockFs()

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			got, err := mock.IsDir(test.name)
			if !reflect.DeepEqual(err, test.err) {
				t.Errorf("want: %v, got %v", test.err, err)
			}
			if got != test.isDir {
				t.Errorf("want: %v, got %v", test.isDir, got)
			}
		})
	}
}

func TestMockFs_glob(t *testing.T) {
	testCases := []struct {
		pattern string
		files   []string
	}{
		{"*", []string{"a", "b", "c", "d", "dangling", "e", "f"}},
		{"./*", []string{"a", "b", "c", "d", "dangling", "e", "f"}},
		{"a", []string{"a"}},
		{"a/a", []string{"a/a"}},
		{"a/*", []string{"a/a"}},
		{"a/a/a", []string{"a/a/a"}},
		{"a/a/f", []string{"a/a/f"}},
		{"a/a/*", []string{"a/a/a", "a/a/f"}},

		{"b", []string{"b"}},
		{"b/a", []string{"b/a"}},
		{"b/*", []string{"b/a"}},
		{"b/a/a", []string{"b/a/a"}},
		{"b/a/f", []string{"b/a/f"}},
		{"b/a/*", []string{"b/a/a", "b/a/f"}},

		{"c", []string{"c"}},
		{"c/a", []string{"c/a"}},
		{"c/f", []string{"c/f"}},
		{"c/*", []string{"c/a", "c/f"}},

		{"d", []string{"d"}},
		{"d/a", []string{"d/a"}},
		{"d/f", []string{"d/f"}},
		{"d/*", []string{"d/a", "d/f"}},

		{"e", []string{"e"}},

		{"dangling", []string{"dangling"}},

		{"missing", nil},
	}

	mock := symlinkMockFs()

	for _, test := range testCases {
		t.Run(test.pattern, func(t *testing.T) {
			got, err := mock.glob(test.pattern)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.files) {
				t.Errorf("want: %v, got %v", test.files, got)
			}
		})
	}
}
