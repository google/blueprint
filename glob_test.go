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

package blueprint

import "testing"

func TestGlobCache(t *testing.T) {
	ctx := NewContext()
	ctx.MockFileSystem(map[string][]byte{
		"Blueprints": nil,
		"a/a":        nil,
		"a/b":        nil,
	})

	// Test a simple glob with no excludes
	matches, err := ctx.glob("a/*", nil)
	if err != nil {
		t.Error("unexpected error", err)
	}
	if len(matches) != 2 || matches[0] != "a/a" || matches[1] != "a/b" {
		t.Error(`expected ["a/a", "a/b"], got`, matches)
	}

	// Test the same glob with an empty excludes array to make sure
	// excludes=nil does not conflict with excludes=[]string{} in the
	// cache.
	matches, err = ctx.glob("a/*", []string{})
	if err != nil {
		t.Error("unexpected error", err)
	}
	if len(matches) != 2 || matches[0] != "a/a" || matches[1] != "a/b" {
		t.Error(`expected ["a/a", "a/b"], got`, matches)
	}

	// Test the same glob with a different excludes array
	matches, err = ctx.glob("a/*", []string{"a/b"})
	if err != nil {
		t.Error("unexpected error", err)
	}
	if len(matches) != 1 || matches[0] != "a/a" {
		t.Error(`expected ["a/a"], got`, matches)
	}
}
