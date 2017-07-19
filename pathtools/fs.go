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
	"os/user"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"
)

// Based on Andrew Gerrand's "10 things you (probably) dont' know about Go"

var OsFs FileSystem = osFs{}

func NewMockFs(files map[string][]byte) *MockFs {
	workDir := "/cwd"
	fs := &MockFs{
		Clock:   NewClock(time.Unix(2, 2)),
		workDir: workDir,
	}
	fs.root = *fs.newDir()
	fs.MkDirs(workDir)

	for path, bytes := range files {
		dir := filepath.Dir(path)
		fs.MkDirs(dir)
		fs.WriteFile(path, bytes, 0777)
	}

	return fs
}

type FileSystem interface {
	// getting information about files
	Open(name string) (file io.ReadCloser, err error)
	Exists(name string) (exists bool, isDir bool, err error)
	Glob(pattern string, excludes []string) (matches, dirs []string, err error)
	glob(pattern string) (matches []string, err error)
	IsDir(name string) (isDir bool, err error)
	Lstat(path string) (stats os.FileInfo, err error)
	InodeNumber(info os.FileInfo) (number uint64, err error)
	DeviceNumber(info os.FileInfo) (number uint64, err error)
	ReadDir(path string) (contents []os.FileInfo, err error)

	// changing contents of the filesystem
	Rename(oldPath string, newPath string) (err error)
	WriteFile(path string, data []byte, perm os.FileMode) (err error)

	// metadata about the filesystem
	Id() (id string)
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

func (fs osFs) Glob(pattern string, excludes []string) (matches, dirs []string, err error) {
	return startGlob(fs, pattern, excludes)
}

func (osFs) glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

func (osFs) Lstat(path string) (stats os.FileInfo, err error) {
	return os.Lstat(path)
}
func (osFs) InodeNumber(info os.FileInfo) (number uint64, err error) {
	sys := info.Sys()
	linuxStats, ok := sys.(*syscall.Stat_t)
	if ok {
		return linuxStats.Ino, nil
	}
	return 0, fmt.Errorf("%v is not a *syscall.Stat_t", sys)
}
func (osFs) DeviceNumber(info os.FileInfo) (number uint64, err error) {
	sys := info.Sys()
	linuxStats, ok := sys.(*syscall.Stat_t)
	if ok {
		return linuxStats.Dev, nil
	}
	return 0, fmt.Errorf("%v is not a *syscall.Stat_t", sys)
}
func (osFs) ReadDir(path string) (contents []os.FileInfo, err error) {
	return ioutil.ReadDir(path)
}
func (osFs) Rename(oldPath string, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (osFs) WriteFile(path string, data []byte, perm os.FileMode) (err error) {
	return ioutil.WriteFile(path, data, perm)
}

func (osFs) Id() (id string) {
	user, err := user.Current()
	if err != nil {
		return ""
	}
	username := user.Username

	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}

	return username + "@" + hostname
}

type Clock struct {
	time time.Time
}

func NewClock(startTime time.Time) *Clock {
	return &Clock{time: startTime}

}

func (c *Clock) Tick() {
	c.time = c.time.Add(time.Microsecond)
}

func (c *Clock) Time() time.Time {
	return c.time
}

type MockFs struct {
	root mockDir
	id   string

	Clock           *Clock
	workDir         string
	nextInodeNumber uint64
	NumStatCalls    int32
	NumReadDirCalls int32
}

type mockInode struct {
	modTime     time.Time
	sys         interface{}
	inodeNumber uint64
}

func (m mockInode) ModTime() time.Time {
	return m.modTime
}
func (m mockInode) Sys() interface{} {
	return m.sys
}

type mockFile struct {
	bytes []byte

	mockInode
}
type mockDir struct {
	mockInode

	subdirs map[string]*mockDir
	files   map[string]*mockFile
}

func (m *MockFs) abs(path string) (result string) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.workDir, path)
	}
	return filepath.Clean(path)
}

func (m *MockFs) Open(path string) (io.ReadCloser, error) {
	path = m.abs(path)
	parentPath, base := filepath.Split(path)
	parentDir, err := m.getDir(parentPath, false)
	if err != nil {
		return nil, err
	}
	file, exists := parentDir.files[base]
	if exists {
		return struct {
			io.Closer
			*bytes.Reader
		}{
			ioutil.NopCloser(nil),
			bytes.NewReader(file.bytes),
		}, nil
	}

	return nil, &os.PathError{
		Op:   "open",
		Path: path,
		Err:  os.ErrNotExist,
	}
}

func (m *MockFs) Exists(path string) (exists bool, isDir bool, err error) {
	path = m.abs(path)
	parentPath, base := filepath.Split(path)
	parentDir, err := m.getDir(parentPath, false)
	if err != nil {
		return false, false, err
	}
	_, ok := parentDir.subdirs[base]
	if ok {
		return true, true, nil
	}
	_, ok = parentDir.files[base]
	if ok {
		return true, false, nil
	}
	return false, false, nil
}

func (m *MockFs) IsDir(path string) (bool, error) {
	dir, err := m.getDir(m.abs(path), false)
	return dir != nil, err
}

func (m *MockFs) Glob(pattern string, excludes []string) (matches, dirs []string, err error) {
	return startGlob(m, pattern, excludes)
}

func (m *MockFs) find(dir *mockDir, path string) (paths []string) {
	results := []string{}
	for fileName := range dir.files {
		newPath := filepath.Join(path, fileName)
		results = append(results, newPath)
	}
	for dirName, dir := range dir.subdirs {
		newPath := filepath.Join(path, dirName)
		results = append(results, m.find(dir, newPath)...)
	}
	return results
}

func (m *MockFs) all() (paths []string) {
	return m.find(&m.root, "/")
}

func (m *MockFs) glob(pattern string) ([]string, error) {
	isRel := !filepath.IsAbs(pattern)
	absPattern := m.abs(pattern)
	var matches []string
	for _, path := range m.all() {
		match, err := filepath.Match(absPattern, path)
		if err != nil {
			return nil, err
		}
		if match {

			matchingPath := path
			if isRel {
				matchingPath, err = filepath.Rel(m.workDir, path)
				if err != nil {
					return nil, err
				}
			}
			matches = append(matches, matchingPath)
		}
	}
	return matches, nil
}

// a mockFileInfo is for exporting file stats in a way that satisfies the FileInfo interface
type mockFileInfo struct {
	path         string
	size         int64
	modTime      time.Time
	isDir        bool
	inodeNumber  uint64
	deviceNumber uint64
}

func (m *mockFileInfo) Name() string {
	return m.path
}
func (m *mockFileInfo) Size() int64 {
	return m.size
}
func (m *mockFileInfo) Mode() os.FileMode {
	return 0
}
func (m *mockFileInfo) ModTime() time.Time {
	return m.modTime
}
func (m *mockFileInfo) IsDir() bool {
	return m.isDir
}
func (m *mockFileInfo) Sys() interface{} {
	return nil
}
func (m *MockFs) dirToFileInfo(d *mockDir, path string) (info *mockFileInfo) {
	return &mockFileInfo{
		path:         path,
		size:         1,
		modTime:      d.modTime,
		isDir:        true,
		inodeNumber:  d.inodeNumber,
		deviceNumber: 0,
	}

}
func (m *MockFs) fileToFileInfo(f *mockFile, path string) (info *mockFileInfo) {
	return &mockFileInfo{
		path:         path,
		size:         1,
		modTime:      f.modTime,
		isDir:        false,
		inodeNumber:  f.inodeNumber,
		deviceNumber: 0,
	}
}

func (m *MockFs) Lstat(path string) (stats os.FileInfo, err error) {
	atomic.AddInt32(&m.NumStatCalls, 1)

	if path == "/" {
		return m.dirToFileInfo(&m.root, "/"), nil
	}

	parentPath, baseName := filepath.Split(path)
	dir, err := m.getDir(parentPath, false)
	if err != nil {
		return nil, err
	}
	subdir, subdirExists := dir.subdirs[baseName]
	if subdirExists {
		return m.dirToFileInfo(subdir, path), nil
	}
	file, fileExists := dir.files[baseName]
	if fileExists {
		return m.fileToFileInfo(file, path), nil
	}
	return nil, &os.PathError{
		Op:   "stat",
		Path: path,
		Err:  os.ErrNotExist,
	}
}

func (m *MockFs) InodeNumber(info os.FileInfo) (number uint64, err error) {
	mockInfo, ok := info.(*mockFileInfo)
	if ok {
		return mockInfo.inodeNumber, nil
	}
	return 0, fmt.Errorf("%v is not a mockFileInfo", info)
}
func (m *MockFs) DeviceNumber(info os.FileInfo) (number uint64, err error) {
	mockInfo, ok := info.(*mockFileInfo)
	if ok {
		return mockInfo.deviceNumber, nil
	}
	return 0, fmt.Errorf("%v is not a mockFileInfo", info)
}

func (m *MockFs) ReadDir(path string) (contents []os.FileInfo, err error) {
	atomic.AddInt32(&m.NumReadDirCalls, 1)

	results := []os.FileInfo{}
	dir, err := m.getDir(path, false)
	if err != nil {
		return nil, err
	}
	for name, subdir := range dir.subdirs {
		dirInfo := m.dirToFileInfo(subdir, name)
		results = append(results, dirInfo)
	}
	for name, file := range dir.files {
		info := m.fileToFileInfo(file, name)
		results = append(results, info)
	}
	return results, nil
}

func (m *MockFs) Rename(sourcePath string, destPath string) error {
	// validate source parent exists
	sourcePath = m.abs(sourcePath)
	sourceParentPath := filepath.Dir(sourcePath)
	sourceParentDir, err := m.getDir(sourceParentPath, false)
	if err != nil {
		return err
	}
	if sourceParentDir == nil {
		return &os.PathError{
			Op:   "move",
			Path: sourcePath,
			Err:  os.ErrNotExist,
		}
	}

	// validate dest parent exists
	destPath = m.abs(destPath)
	destParentPath := filepath.Dir(destPath)
	destParentDir, err := m.getDir(destParentPath, false)
	if err != nil {
		return err
	}
	if destParentDir == nil {
		return &os.PathError{
			Op:   "move",
			Path: destParentPath,
			Err:  os.ErrNotExist,
		}
	}

	// now do the move
	sourceBase := filepath.Base(sourcePath)
	destBase := filepath.Base(destPath)
	_, isDir, err := m.Exists(sourcePath)
	if isDir {
		destParentDir.subdirs[destBase] = sourceParentDir.subdirs[sourceBase]
		delete(sourceParentDir.subdirs, sourceBase)

	} else {
		destParentDir.files[destBase] = sourceParentDir.files[sourceBase]
		delete(destParentDir.files, sourceBase)
	}

	return nil
}

func (m *MockFs) newInodeNumber() uint64 {
	result := m.nextInodeNumber
	m.nextInodeNumber++
	return result
}
func (m *MockFs) WriteFile(filePath string, data []byte, perm os.FileMode) error {
	filePath = m.abs(filePath)
	parentPath := filepath.Dir(filePath)
	parentDir, err := m.getDir(parentPath, false)
	if err != nil || parentDir == nil {
		return &os.PathError{
			Op:   "write",
			Path: parentPath,
			Err:  os.ErrNotExist,
		}
	}

	baseName := filepath.Base(filePath)
	_, exists := parentDir.files[baseName]
	if !exists {
		parentDir.modTime = m.Clock.Time()
		parentDir.files[baseName] = m.newFile()
	}
	file := parentDir.files[baseName]
	file.bytes = data
	file.modTime = m.Clock.Time()
	return nil
}

func (m *MockFs) newFile() *mockFile {
	newFile := &mockFile{}
	newFile.inodeNumber = m.newInodeNumber()
	newFile.modTime = m.Clock.Time()
	return newFile
}

func (m *MockFs) newDir() *mockDir {
	result := &mockDir{
		subdirs: make(map[string]*mockDir, 0),
		files:   make(map[string]*mockFile, 0),
	}
	result.inodeNumber = m.newInodeNumber()
	result.modTime = m.Clock.Time()
	return result
}
func (m *MockFs) MkDirs(path string) {
	m.getDir(m.abs(path), true)
}
func (m *MockFs) getDir(path string, createIfMissing bool) (dir *mockDir, err error) {
	cleanedPath := filepath.Clean(path)
	if cleanedPath == "/" {
		return &m.root, nil
	}

	parentPath, leaf := filepath.Split(cleanedPath)
	if len(parentPath) >= len(path) {
		return &m.root, nil
	}
	parent, err := m.getDir(parentPath, createIfMissing)
	if err != nil {
		return nil, err
	}
	childDir, dirExists := parent.subdirs[leaf]
	if !dirExists {
		if createIfMissing {
			// confirm that a file with the same name doesn't already exist
			_, fileExists := parent.files[leaf]
			if fileExists {
				return nil, &os.PathError{
					Op:   "mkdir",
					Path: path,
					Err:  os.ErrExist,
				}
			}
			// create this directory
			childDir = m.newDir()
			parent.subdirs[leaf] = childDir
		} else {
			return nil, &os.PathError{
				Op:   "stat",
				Path: path,
				Err:  os.ErrExist,
			}
		}
	}
	return childDir, nil

}

func (m *MockFs) Id() (id string) {
	return m.id
}
