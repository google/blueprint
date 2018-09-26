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
		{"d/f", "f"},

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

func TestFs_IsDir(t *testing.T) {
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
		{"e/missing", false, syscall.ENOTDIR},
		{"dangling/missing", false, os.ErrNotExist},

		{"a/missing/missing", false, os.ErrNotExist},
		{"b/missing/missing", false, os.ErrNotExist},
		{"c/missing/missing", false, os.ErrNotExist},
		{"d/missing/missing", false, os.ErrNotExist},
		{"e/missing/missing", false, syscall.ENOTDIR},
		{"dangling/missing/missing", false, os.ErrNotExist},

		{"c/f/missing", false, syscall.ENOTDIR},
	}

	mock := symlinkMockFs()
	fsList := []FileSystem{mock, OsFs}
	names := []string{"mock", "os"}

	os.Chdir("testdata/dangling")
	defer os.Chdir("../..")

	for i, fs := range fsList {
		t.Run(names[i], func(t *testing.T) {
			for _, test := range testCases {
				t.Run(test.name, func(t *testing.T) {
					got, err := fs.IsDir(test.name)
					checkErr(t, test.err, err)
					if got != test.isDir {
						t.Errorf("want: %v, got %v", test.isDir, got)
					}
				})
			}
		})
	}
}

func TestFs_ListDirsRecursiveFollowSymlinks(t *testing.T) {
	testCases := []struct {
		name string
		dirs []string
		err  error
	}{
		{".", []string{".", "a", "a/a", "b", "b/a", "c", "d"}, nil},

		{"a", []string{"a", "a/a"}, nil},
		{"a/a", []string{"a/a"}, nil},
		{"a/a/a", nil, nil},

		{"b", []string{"b", "b/a"}, nil},
		{"b/a", []string{"b/a"}, nil},
		{"b/a/a", nil, nil},

		{"c", []string{"c"}, nil},
		{"c/a", nil, nil},

		{"d", []string{"d"}, nil},
		{"d/a", nil, nil},

		{"e", nil, nil},

		{"dangling", nil, os.ErrNotExist},

		{"missing", nil, os.ErrNotExist},
	}

	mock := symlinkMockFs()
	fsList := []FileSystem{mock, OsFs}
	names := []string{"mock", "os"}

	os.Chdir("testdata/dangling")
	defer os.Chdir("../..")

	for i, fs := range fsList {
		t.Run(names[i], func(t *testing.T) {

			for _, test := range testCases {
				t.Run(test.name, func(t *testing.T) {
					got, err := fs.ListDirsRecursive(test.name, FollowSymlinks)
					checkErr(t, test.err, err)
					if !reflect.DeepEqual(got, test.dirs) {
						t.Errorf("want: %v, got %v", test.dirs, got)
					}
				})
			}
		})
	}
}

func TestFs_ListDirsRecursiveDontFollowSymlinks(t *testing.T) {
	testCases := []struct {
		name string
		dirs []string
		err  error
	}{
		{".", []string{".", "a", "a/a"}, nil},

		{"a", []string{"a", "a/a"}, nil},
		{"a/a", []string{"a/a"}, nil},
		{"a/a/a", nil, nil},

		{"b", []string{"b", "b/a"}, nil},
		{"b/a", []string{"b/a"}, nil},
		{"b/a/a", nil, nil},

		{"c", []string{"c"}, nil},
		{"c/a", nil, nil},

		{"d", []string{"d"}, nil},
		{"d/a", nil, nil},

		{"e", nil, nil},

		{"dangling", nil, os.ErrNotExist},

		{"missing", nil, os.ErrNotExist},
	}

	mock := symlinkMockFs()
	fsList := []FileSystem{mock, OsFs}
	names := []string{"mock", "os"}

	os.Chdir("testdata/dangling")
	defer os.Chdir("../..")

	for i, fs := range fsList {
		t.Run(names[i], func(t *testing.T) {

			for _, test := range testCases {
				t.Run(test.name, func(t *testing.T) {
					got, err := fs.ListDirsRecursive(test.name, DontFollowSymlinks)
					checkErr(t, test.err, err)
					if !reflect.DeepEqual(got, test.dirs) {
						t.Errorf("want: %v, got %v", test.dirs, got)
					}
				})
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
	fsList := []FileSystem{mock, OsFs}
	names := []string{"mock", "os"}

	os.Chdir("testdata/dangling")
	defer os.Chdir("../..")

	for i, fs := range fsList {
		t.Run(names[i], func(t *testing.T) {
			for _, test := range testCases {
				t.Run(test.pattern, func(t *testing.T) {
					got, err := fs.glob(test.pattern)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, test.files) {
						t.Errorf("want: %v, got %v", test.files, got)
					}
				})
			}
		})
	}
}

func syscallError(err error) error {
	if serr, ok := err.(*os.SyscallError); ok {
		return serr.Err.(syscall.Errno)
	} else if serr, ok := err.(syscall.Errno); ok {
		return serr
	} else {
		return nil
	}
}

func checkErr(t *testing.T, want, got error) {
	t.Helper()
	if (got != nil) != (want != nil) {
		t.Fatalf("want: %v, got %v", want, got)
	}

	if os.IsNotExist(got) == os.IsNotExist(want) {
		return
	}

	if syscallError(got) == syscallError(want) {
		return
	}

	t.Fatalf("want: %v, got %v", want, got)
}
