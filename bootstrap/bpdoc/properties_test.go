// Copyright 2019 Google Inc. All rights reserved.
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

package bpdoc

import (
	"reflect"
	"testing"
)

func TestExcludeByTag(t *testing.T) {
	r := NewReader(pkgFiles)
	ps, err := r.PropertyStruct(pkgPath, "tagTestProps", reflect.ValueOf(tagTestProps{}))
	if err != nil {
		t.Fatal(err)
	}

	ps.ExcludeByTag("tag1", "a")

	expected := []string{"c"}
	actual := []string{}
	for _, p := range ps.Properties {
		actual = append(actual, p.Name)
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("unexpected ExcludeByTag result, expected: %q, actual: %q", expected, actual)
	}
}

func TestIncludeByTag(t *testing.T) {
	r := NewReader(pkgFiles)
	ps, err := r.PropertyStruct(pkgPath, "tagTestProps", reflect.ValueOf(tagTestProps{A: "B"}))
	if err != nil {
		t.Fatal(err)
	}

	ps.IncludeByTag("tag1", "c")

	expected := []string{"b", "c"}
	actual := []string{}
	for _, p := range ps.Properties {
		actual = append(actual, p.Name)
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("unexpected IncludeByTag result, expected: %q, actual: %q", expected, actual)
	}
}
