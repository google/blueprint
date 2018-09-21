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
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Based on Andrew Gerrand's "10 things you (probably) dont' know about Go"

var OsFs FileSystem = osFs{}

func MockFs(files map[string][]byte) FileSystem {
	fs := &mockFs{
		files:    make(map[string][]byte, len(files)),
		dirs:     make(map[string]bool),
		symlinks: make(map[string]string),
		all:      []string(nil),
	}

	for f, b := range files {
		if tokens := strings.SplitN(f, "->", 2); len(tokens) == 2 {
			fs.symlinks[strings.TrimSpace(tokens[0])] = strings.TrimSpace(tokens[1])
			continue
		}

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

	for s := range fs.symlinks {
		fs.all = append(fs.all, s)
	}

	sort.Strings(fs.all)

	return fs
}

type FileSystem interface {
	// Open opens a file for reading.  Follows symlinks.
	Open(name string) (io.ReadCloser, error)

	// Exists returns whether the file exists and whether it is a directory.  Follows symlinks.
	Exists(name string) (bool, bool, error)

	Glob(pattern string, excludes []string) (matches, dirs []string, err error)
	glob(pattern string) (matches []string, err error)

	// IsDir returns true if the path points to a directory, false it it points to a file.  Follows symlinks.
	// Returns os.ErrNotExist if the path does not exist or is a symlink to a path that does not exist.
	IsDir(name string) (bool, error)

	// IsSymlink returns true if the path points to a symlink, even if that symlink points to a path that does
	// not exist.  Returns os.ErrNotExist if the path does not exist.
	IsSymlink(name string) (bool, error)

	// Lstat returns info on a file without following symlinks.
	Lstat(name string) (os.FileInfo, error)

	// ListDirsRecursive returns a list of all the directories in a path, following symlinks.
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
		return false, err
	}
	return info.IsDir(), nil
}

func (osFs) IsSymlink(name string) (bool, error) {
	if info, err := os.Lstat(name); err != nil {
		return false, err
	} else {
		return info.Mode()&os.ModeSymlink != 0, nil
	}
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
	// TODO: follow symbolic links
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
	files    map[string][]byte
	dirs     map[string]bool
	symlinks map[string]string
	all      []string
}

func (m *mockFs) followSymlinks(name string) string {
	dir, file := saneSplit(name)
	if dir != "." && dir != "/" {
		dir = m.followSymlinks(dir)
	}
	name = filepath.Join(dir, file)

	for i := 0; i < 255; i++ {
		i++
		if i > 255 {
			panic("symlink loop")
		}
		to, exists := m.symlinks[name]
		if !exists {
			break
		}
		if filepath.IsAbs(to) {
			name = to
		} else {
			name = filepath.Join(dir, to)
		}
	}
	return name
}

func (m *mockFs) Open(name string) (io.ReadCloser, error) {
	name = filepath.Clean(name)
	name = m.followSymlinks(name)
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
	name = m.followSymlinks(name)
	if _, ok := m.files[name]; ok {
		return ok, false, nil
	}
	if _, ok := m.dirs[name]; ok {
		return ok, true, nil
	}
	return false, false, nil
}

func (m *mockFs) IsDir(name string) (bool, error) {
	dir := filepath.Dir(name)
	if dir != "." && dir != "/" {
		isDir, err := m.IsDir(dir)

		if serr, ok := err.(*os.SyscallError); ok && serr.Err == syscall.ENOTDIR {
			isDir = false
		} else if err != nil {
			return false, err
		}

		if !isDir {
			return false, os.NewSyscallError("stat "+name, syscall.ENOTDIR)
		}
	}

	name = filepath.Clean(name)
	name = m.followSymlinks(name)

	if _, ok := m.dirs[name]; ok {
		return true, nil
	}
	if _, ok := m.files[name]; ok {
		return false, nil
	}
	return false, os.ErrNotExist
}

func (m *mockFs) IsSymlink(name string) (bool, error) {
	dir, file := saneSplit(name)
	dir = m.followSymlinks(dir)
	name = filepath.Join(dir, file)

	if _, isSymlink := m.symlinks[name]; isSymlink {
		return true, nil
	}
	if _, isDir := m.dirs[name]; isDir {
		return false, nil
	}
	if _, isFile := m.files[name]; isFile {
		return false, nil
	}
	return false, os.ErrNotExist
}

func (m *mockFs) Glob(pattern string, excludes []string) (matches, dirs []string, err error) {
	return startGlob(m, pattern, excludes)
}

func unescapeGlob(s string) string {
	i := 0
	for i < len(s) {
		if s[i] == '\\' {
			s = s[:i] + s[i+1:]
		} else {
			i++
		}
	}
	return s
}

func (m *mockFs) glob(pattern string) ([]string, error) {
	dir, file := saneSplit(pattern)

	dir = unescapeGlob(dir)
	toDir := m.followSymlinks(dir)

	var matches []string
	for _, f := range m.all {
		fDir, fFile := saneSplit(f)
		if toDir == fDir {
			match, err := filepath.Match(file, fFile)
			if err != nil {
				return nil, err
			}
			if f == "." && f != pattern {
				// filepath.Glob won't return "." unless the pattern was "."
				match = false
			}
			if match {
				matches = append(matches, filepath.Join(dir, fFile))
			}
		}
	}
	return matches, nil
}

type mockStat struct {
	name string
	size int64
	mode os.FileMode
}

func (ms *mockStat) Name() string       { return ms.name }
func (ms *mockStat) IsDir() bool        { return ms.Mode().IsDir() }
func (ms *mockStat) Size() int64        { return ms.size }
func (ms *mockStat) Mode() os.FileMode  { return ms.mode }
func (ms *mockStat) ModTime() time.Time { return time.Time{} }
func (ms *mockStat) Sys() interface{}   { return nil }

func (m *mockFs) Lstat(name string) (os.FileInfo, error) {
	name = filepath.Clean(name)

	ms := mockStat{
		name: name,
		size: int64(len(m.files[name])),
	}

	if isSymlink, err := m.IsSymlink(name); err != nil {
		// IsSymlink handles ErrNotExist
		return nil, err
	} else if isSymlink {
		ms.mode = os.ModeSymlink
	} else if isDir, err := m.IsDir(name); err != nil {
		return nil, err
	} else if isDir {
		ms.mode = os.ModeDir
	} else {
		ms.mode = 0
	}

	return &ms, nil
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
