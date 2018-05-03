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

package parser

import (
	"bytes"
	"testing"
)

func TestPatchList(t *testing.T) {
	expectOverlap := func(err error) {
		t.Helper()
		if _, ok := err.(PatchOverlapError); !ok {
			t.Error("missing PatchOverlapError")
		}
	}

	expectOk := func(err error) {
		t.Helper()
		if err != nil {
			t.Error(err)
		}
	}

	in := []byte("abcdefghijklmnopqrstuvwxyz")

	patchlist := PatchList{}
	expectOk(patchlist.Add(0, 3, "ABC"))
	expectOk(patchlist.Add(12, 15, "MNO"))
	expectOk(patchlist.Add(24, 26, "Z"))
	expectOk(patchlist.Add(15, 15, "_"))

	expectOverlap(patchlist.Add(0, 3, "x"))
	expectOverlap(patchlist.Add(12, 13, "x"))
	expectOverlap(patchlist.Add(13, 14, "x"))
	expectOverlap(patchlist.Add(14, 15, "x"))
	expectOverlap(patchlist.Add(11, 13, "x"))
	expectOverlap(patchlist.Add(12, 15, "x"))
	expectOverlap(patchlist.Add(11, 15, "x"))
	expectOverlap(patchlist.Add(15, 15, "x"))

	if t.Failed() {
		return
	}

	buf := new(bytes.Buffer)
	patchlist.Apply(bytes.NewReader(in), buf)
	expected := "ABCdefghijklMNO_pqrstuvwxZ"
	got := buf.String()
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}
