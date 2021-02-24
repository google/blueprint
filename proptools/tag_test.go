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

package proptools

import (
	"reflect"
	"testing"
)

type testType struct {
	NoTag       string
	EmptyTag    string ``
	OtherTag    string `foo:"bar"`
	MatchingTag string `name:"value"`
	ExtraValues string `name:"foo,value,bar"`
	ExtraTags   string `foo:"bar" name:"value"`
}

func TestHasTag(t *testing.T) {
	tests := []struct {
		field string
		want  bool
	}{
		{
			field: "NoTag",
			want:  false,
		},
		{
			field: "EmptyTag",
			want:  false,
		},
		{
			field: "OtherTag",
			want:  false,
		},
		{
			field: "MatchingTag",
			want:  true,
		},
		{
			field: "ExtraValues",
			want:  true,
		},
		{
			field: "ExtraTags",
			want:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.field, func(t *testing.T) {
			field, _ := reflect.TypeOf(testType{}).FieldByName(test.field)
			if got := HasTag(field, "name", "value"); got != test.want {
				t.Errorf(`HasTag(%q, "name", "value") = %v, want %v`, field.Tag, got, test.want)
			}
		})
	}
}

func BenchmarkHasTag(b *testing.B) {
	tests := []struct {
		field string
	}{
		{
			field: "NoTag",
		},
		{
			field: "EmptyTag",
		},
		{
			field: "OtherTag",
		},
		{
			field: "MatchingTag",
		},
		{
			field: "ExtraValues",
		},
		{
			field: "ExtraTags",
		},
	}
	for _, test := range tests {
		b.Run(test.field, func(b *testing.B) {
			field, _ := reflect.TypeOf(testType{}).FieldByName(test.field)
			for i := 0; i < b.N; i++ {
				HasTag(field, "name", "value")
			}
		})
	}
}

func TestPropertyIndexesWithTag(t *testing.T) {
	tests := []struct {
		name string
		ps   interface{}
		want [][]int
	}{
		{
			name: "none",
			ps: &struct {
				Foo string
			}{},
			want: nil,
		},
		{
			name: "one",
			ps: &struct {
				Foo string `name:"value"`
			}{},
			want: [][]int{{0}},
		},
		{
			name: "two",
			ps: &struct {
				Foo string `name:"value"`
				Bar string `name:"value"`
			}{},
			want: [][]int{{0}, {1}},
		},
		{
			name: "some",
			ps: &struct {
				Foo string `name:"other"`
				Bar string `name:"value"`
			}{},
			want: [][]int{{1}},
		},
		{
			name: "embedded",
			ps: &struct {
				Foo struct {
					Bar string `name:"value"`
				}
			}{},
			want: [][]int{{0, 0}},
		},
		{
			name: "embedded ptr",
			ps: &struct {
				Foo *struct {
					Bar string `name:"value"`
				}
			}{},
			want: [][]int{{0, 0}},
		},
		{
			name: "slice of struct",
			ps: &struct {
				Other int
				Foo   []struct {
					Other int
					Bar   string `name:"value"`
				}
			}{},
			want: [][]int{{1, 1}},
		},
		{
			name: "slice^2 of struct",
			ps: &struct {
				Other int
				Foo   []struct {
					Other int
					Bar   []struct {
						Other int
						Baz   string `name:"value"`
					}
				}
			}{},
			want: [][]int{{1, 1, 1}},
		},
		{
			name: "nil",
			ps: (*struct {
				Foo string `name:"value"`
			})(nil),
			want: [][]int{{0}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := PropertyIndexesWithTag(test.ps, "name", "value"); !reflect.DeepEqual(got, test.want) {
				t.Errorf("PropertyIndexesWithTag() = %v, want %v", got, test.want)
			}
		})
	}
}
