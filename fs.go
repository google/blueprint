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

package blueprint

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
)

// Based on Andrew Gerrand's "10 things you (probably) dont' know about Go"

var fs fileSystem = osFS{}

type fileSystem interface {
	Open(name string) (io.ReadCloser, error)
	Exists(name string) (bool, bool, error)
}

// osFS implements fileSystem using the local disk.
type osFS struct{}

func (osFS) Open(name string) (io.ReadCloser, error) { return os.Open(name) }
func (osFS) Exists(name string) (bool, bool, error) {
	stat, err := os.Stat(name)
	if err == nil {
		return true, stat.IsDir(), nil
	} else if os.IsNotExist(err) {
		return false, false, nil
	} else {
		return false, false, err
	}
}

type mockFS struct {
	files map[string][]byte
}

func (m mockFS) Open(name string) (io.ReadCloser, error) {
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

func (m mockFS) Exists(name string) (bool, bool, error) {
	_, ok := m.files[name]
	return ok, false, nil
}
