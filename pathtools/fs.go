// Copyright 2016 Google Inc. All rights reserved.
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
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Based on Andrew Gerrand's "10 things you (probably) dont' know about Go"

var OsFs FileSystem = osFs{}

func MockFs(files map[string][]byte) FileSystem {
	fs := &mockFs{
		files: make(map[string][]byte, len(files)),
		dirs:  make(map[string]bool),
		all:   []string(nil),
	}

	for f, b := range files {
		fs.files[filepath.Clean(f)] = b
		dir := filepath.Dir(f)
		for dir != "." && dir != "/" {
			fs.dirs[dir] = true
			dir = filepath.Dir(dir)
		}
		fs.dirs[dir] = true
	}

	for f := range fs.files {
		fs.all = append(fs.all, f)
	}

	for d := range fs.dirs {
		fs.all = append(fs.all, d)
	}

	sort.Strings(fs.all)

	return fs
}

type FileSystem interface {
	Open(name string) (io.ReadCloser, error)
	Exists(name string) (bool, bool, error)
	Glob(pattern string, excludes []string) (matches, dirs []string, err error)
	glob(pattern string) (matches []string, err error)
	IsDir(name string) (bool, error)
	Lstat(name string) (os.FileInfo, error)
	ListDirsRecursive(name string) (dirs []string, err error)
}

// osFs implements FileSystem using the local disk.
type osFs struct{}

func (osFs) Open(name string) (io.ReadCloser, error) { return os.Open(name) }
func (osFs) Exists(name string) (bool, bool, error) {
	stat, err := os.Stat(name)
	if err == nil {
		return true, stat.IsDir(), nil
	} else if os.IsNotExist(err) {
		return false, false, nil
	} else {
		return false, false, err
	}
}

func (osFs) IsDir(name string) (bool, error) {
	info, err := os.Stat(name)
	if err != nil {
		return false, fmt.Errorf("unexpected error after glob: %s", err)
	}
	return info.IsDir(), nil
}

func (fs osFs) Glob(pattern string, excludes []string) (matches, dirs []string, err error) {
	return startGlob(fs, pattern, excludes)
}

func (osFs) glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

func (osFs) Lstat(path string) (stats os.FileInfo, err error) {
	return os.Lstat(path)
}

// Returns a list of all directories under dir
func (osFs) ListDirsRecursive(name string) (dirs []string, err error) {
	err = filepath.Walk(name, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Mode().IsDir() {
			name := info.Name()
			if name[0] == '.' && name != "." {
				return filepath.SkipDir
			}

			dirs = append(dirs, path)
		}
		return nil
	})

	return dirs, err
}

type mockFs struct {
	files map[string][]byte
	dirs  map[string]bool
	all   []string
}

func (m *mockFs) Open(name string) (io.ReadCloser, error) {
	if f, ok := m.files[name]; ok {
		return struct {
			io.Closer
			*bytes.Reader
		}{
			ioutil.NopCloser(nil),
			bytes.NewReader(f),
		}, nil
	}

	return nil, &os.PathError{
		Op:   "open",
		Path: name,
		Err:  os.ErrNotExist,
	}
}

func (m *mockFs) Exists(name string) (bool, bool, error) {
	name = filepath.Clean(name)
	if _, ok := m.files[name]; ok {
		return ok, false, nil
	}
	if _, ok := m.dirs[name]; ok {
		return ok, true, nil
	}
	return false, false, nil
}

func (m *mockFs) IsDir(name string) (bool, error) {
	return m.dirs[filepath.Clean(name)], nil
}

func (m *mockFs) Glob(pattern string, excludes []string) (matches, dirs []string, err error) {
	return startGlob(m, pattern, excludes)
}

func (m *mockFs) glob(pattern string) ([]string, error) {
	var matches []string
	for _, f := range m.all {
		match, err := filepath.Match(pattern, f)
		if err != nil {
			return nil, err
		}
		if f == "." && f != pattern {
			// filepath.Glob won't return "." unless the pattern was "."
			match = false
		}
		if match {
			matches = append(matches, f)
		}
	}
	return matches, nil
}

func (m *mockFs) Lstat(path string) (stats os.FileInfo, err error) {
	return nil, errors.New("Lstat is not yet implemented in MockFs")
}

func (m *mockFs) ListDirsRecursive(name string) (dirs []string, err error) {
	name = filepath.Clean(name)
	dirs = append(dirs, name)
	if name == "." {
		name = ""
	} else if name != "/" {
		name = name + "/"
	}
	for _, f := range m.all {
		if _, isDir := m.dirs[f]; isDir && filepath.Base(f)[0] != '.' {
			if strings.HasPrefix(f, name) &&
				strings.HasPrefix(f, "/") == strings.HasPrefix(name, "/") {
				dirs = append(dirs, f)
			}
		}
	}

	return dirs, nil
}
