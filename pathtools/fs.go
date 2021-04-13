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
	"fmt"
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

type ShouldFollowSymlinks bool

const (
	FollowSymlinks     = ShouldFollowSymlinks(true)
	DontFollowSymlinks = ShouldFollowSymlinks(false)
)

var OsFs FileSystem = &osFs{}

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

	fs.dirs["."] = true
	fs.dirs["/"] = true

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

type ReaderAtSeekerCloser interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
}

type FileSystem interface {
	// Open opens a file for reading.  Follows symlinks.
	Open(name string) (ReaderAtSeekerCloser, error)

	// Exists returns whether the file exists and whether it is a directory.  Follows symlinks.
	Exists(name string) (bool, bool, error)

	Glob(pattern string, excludes []string, follow ShouldFollowSymlinks) (GlobResult, error)
	glob(pattern string) (matches []string, err error)

	// IsDir returns true if the path points to a directory, false it it points to a file.  Follows symlinks.
	// Returns os.ErrNotExist if the path does not exist or is a symlink to a path that does not exist.
	IsDir(name string) (bool, error)

	// IsSymlink returns true if the path points to a symlink, even if that symlink points to a path that does
	// not exist.  Returns os.ErrNotExist if the path does not exist.
	IsSymlink(name string) (bool, error)

	// Lstat returns info on a file without following symlinks.
	Lstat(name string) (os.FileInfo, error)

	// Lstat returns info on a file.
	Stat(name string) (os.FileInfo, error)

	// ListDirsRecursive returns a list of all the directories in a path, following symlinks if requested.
	ListDirsRecursive(name string, follow ShouldFollowSymlinks) (dirs []string, err error)

	// ReadDirNames returns a list of everything in a directory.
	ReadDirNames(name string) ([]string, error)

	// Readlink returns the destination of the named symbolic link.
	Readlink(name string) (string, error)
}

// osFs implements FileSystem using the local disk.
type osFs struct {
	srcDir string
}

func NewOsFs(path string) FileSystem {
	return &osFs{srcDir: path}
}

func (fs *osFs) toAbs(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(fs.srcDir, path)
}

func (fs *osFs) removeSrcDirPrefix(path string) string {
	if fs.srcDir == "" {
		return path
	}
	rel, err := filepath.Rel(fs.srcDir, path)
	if err != nil {
		panic(fmt.Errorf("unexpected failure in removeSrcDirPrefix filepath.Rel(%s, %s): %s",
			fs.srcDir, path, err))
	}
	if strings.HasPrefix(rel, "../") {
		panic(fmt.Errorf("unexpected relative path outside directory in removeSrcDirPrefix filepath.Rel(%s, %s): %s",
			fs.srcDir, path, rel))
	}
	return rel
}

func (fs *osFs) removeSrcDirPrefixes(paths []string) []string {
	if fs.srcDir != "" {
		for i, path := range paths {
			paths[i] = fs.removeSrcDirPrefix(path)
		}
	}
	return paths
}

func (fs *osFs) Open(name string) (ReaderAtSeekerCloser, error) {
	return os.Open(fs.toAbs(name))
}

func (fs *osFs) Exists(name string) (bool, bool, error) {
	stat, err := os.Stat(fs.toAbs(name))
	if err == nil {
		return true, stat.IsDir(), nil
	} else if os.IsNotExist(err) {
		return false, false, nil
	} else {
		return false, false, err
	}
}

func (fs *osFs) IsDir(name string) (bool, error) {
	info, err := os.Stat(fs.toAbs(name))
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func (fs *osFs) IsSymlink(name string) (bool, error) {
	if info, err := os.Lstat(fs.toAbs(name)); err != nil {
		return false, err
	} else {
		return info.Mode()&os.ModeSymlink != 0, nil
	}
}

func (fs *osFs) Glob(pattern string, excludes []string, follow ShouldFollowSymlinks) (GlobResult, error) {
	return startGlob(fs, pattern, excludes, follow)
}

func (fs *osFs) glob(pattern string) ([]string, error) {
	paths, err := filepath.Glob(fs.toAbs(pattern))
	fs.removeSrcDirPrefixes(paths)
	return paths, err
}

func (fs *osFs) Lstat(path string) (stats os.FileInfo, err error) {
	return os.Lstat(fs.toAbs(path))
}

func (fs *osFs) Stat(path string) (stats os.FileInfo, err error) {
	return os.Stat(fs.toAbs(path))
}

// Returns a list of all directories under dir
func (fs *osFs) ListDirsRecursive(name string, follow ShouldFollowSymlinks) (dirs []string, err error) {
	return listDirsRecursive(fs, name, follow)
}

func (fs *osFs) ReadDirNames(name string) ([]string, error) {
	dir, err := os.Open(fs.toAbs(name))
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	contents, err := dir.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	sort.Strings(contents)
	return contents, nil
}

func (fs *osFs) Readlink(name string) (string, error) {
	return os.Readlink(fs.toAbs(name))
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

func (m *mockFs) Open(name string) (ReaderAtSeekerCloser, error) {
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

func (m *mockFs) Glob(pattern string, excludes []string, follow ShouldFollowSymlinks) (GlobResult, error) {
	return startGlob(m, pattern, excludes, follow)
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
			if (f == "." || f == "/") && f != pattern {
				// filepath.Glob won't return "." or "/" unless the pattern was "." or "/"
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
	dir, file := saneSplit(name)
	dir = m.followSymlinks(dir)
	name = filepath.Join(dir, file)

	ms := mockStat{
		name: file,
	}

	if symlink, isSymlink := m.symlinks[name]; isSymlink {
		ms.mode = os.ModeSymlink
		ms.size = int64(len(symlink))
	} else if _, isDir := m.dirs[name]; isDir {
		ms.mode = os.ModeDir
	} else if _, isFile := m.files[name]; isFile {
		ms.mode = 0
		ms.size = int64(len(m.files[name]))
	} else {
		return nil, os.ErrNotExist
	}

	return &ms, nil
}

func (m *mockFs) Stat(name string) (os.FileInfo, error) {
	name = filepath.Clean(name)
	origName := name
	name = m.followSymlinks(name)

	ms := mockStat{
		name: filepath.Base(origName),
		size: int64(len(m.files[name])),
	}

	if _, isDir := m.dirs[name]; isDir {
		ms.mode = os.ModeDir
	} else if _, isFile := m.files[name]; isFile {
		ms.mode = 0
		ms.size = int64(len(m.files[name]))
	} else {
		return nil, os.ErrNotExist
	}

	return &ms, nil
}

func (m *mockFs) ReadDirNames(name string) ([]string, error) {
	name = filepath.Clean(name)
	name = m.followSymlinks(name)

	exists, isDir, err := m.Exists(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, os.ErrNotExist
	}
	if !isDir {
		return nil, os.NewSyscallError("readdir", syscall.ENOTDIR)
	}

	var ret []string
	for _, f := range m.all {
		dir, file := saneSplit(f)
		if dir == name && len(file) > 0 && file[0] != '.' {
			ret = append(ret, file)
		}
	}
	return ret, nil
}

func (m *mockFs) ListDirsRecursive(name string, follow ShouldFollowSymlinks) ([]string, error) {
	return listDirsRecursive(m, name, follow)
}

func (m *mockFs) Readlink(name string) (string, error) {
	dir, file := saneSplit(name)
	dir = m.followSymlinks(dir)

	origName := name
	name = filepath.Join(dir, file)

	if dest, isSymlink := m.symlinks[name]; isSymlink {
		return dest, nil
	}

	if exists, _, err := m.Exists(name); err != nil {
		return "", err
	} else if !exists {
		return "", os.ErrNotExist
	} else {
		return "", os.NewSyscallError("readlink: "+origName, syscall.EINVAL)
	}
}

func listDirsRecursive(fs FileSystem, name string, follow ShouldFollowSymlinks) ([]string, error) {
	name = filepath.Clean(name)

	isDir, err := fs.IsDir(name)
	if err != nil {
		return nil, err
	}

	if !isDir {
		return nil, nil
	}

	dirs := []string{name}

	subDirs, err := listDirsRecursiveRelative(fs, name, follow, 0)
	if err != nil {
		return nil, err
	}

	for _, d := range subDirs {
		dirs = append(dirs, filepath.Join(name, d))
	}

	return dirs, nil
}

func listDirsRecursiveRelative(fs FileSystem, name string, follow ShouldFollowSymlinks, depth int) ([]string, error) {
	depth++
	if depth > 255 {
		return nil, fmt.Errorf("too many symlinks")
	}
	contents, err := fs.ReadDirNames(name)
	if err != nil {
		return nil, err
	}

	var dirs []string
	for _, f := range contents {
		if f[0] == '.' {
			continue
		}
		f = filepath.Join(name, f)
		var info os.FileInfo
		if follow == DontFollowSymlinks {
			info, err = fs.Lstat(f)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				continue
			}
		} else {
			info, err = fs.Stat(f)
			if err != nil {
				continue
			}
		}
		if info.IsDir() {
			dirs = append(dirs, f)
			subDirs, err := listDirsRecursiveRelative(fs, f, follow, depth)
			if err != nil {
				return nil, err
			}
			for _, s := range subDirs {
				dirs = append(dirs, filepath.Join(f, s))
			}
		}
	}

	for i, d := range dirs {
		rel, err := filepath.Rel(name, d)
		if err != nil {
			return nil, err
		}
		dirs[i] = rel
	}

	return dirs, nil
}
