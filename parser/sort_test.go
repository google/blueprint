// Copyright 2021 Google Inc. All rights reserved.
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

import "testing"

func Test_numericStringLess(t *testing.T) {
	type args struct {
		a string
		b string
	}
	tests := []struct {
		a, b string
	}{
		{"a", "b"},
		{"aa", "ab"},
		{"aaa", "aba"},

		{"1", "2"},
		{"1", "11"},
		{"2", "11"},
		{"1", "12"},

		{"12", "101"},
		{"11", "102"},

		{"0", "1"},
		{"0", "01"},
		{"1", "02"},
		{"01", "002"},
		{"001", "02"},
	}

	oneTest := func(a, b string, want bool) {
		t.Helper()
		if got := numericStringLess(a, b); got != want {
			t.Errorf("want numericStringLess(%v, %v) = %v, got %v", a, b, want, got)
		}
	}

	for _, tt := range tests {
		t.Run(tt.a+"<"+tt.b, func(t *testing.T) {
			// a should be less than b
			oneTest(tt.a, tt.b, true)
			// b should not be less than a
			oneTest(tt.b, tt.a, false)
			// a should not be less than a
			oneTest(tt.a, tt.a, false)
			// b should not be less than b
			oneTest(tt.b, tt.b, false)

			// The same should be true both strings are prefixed with an "a"
			oneTest("a"+tt.a, "a"+tt.b, true)
			oneTest("a"+tt.b, "a"+tt.a, false)
			oneTest("a"+tt.a, "a"+tt.a, false)
			oneTest("a"+tt.b, "a"+tt.b, false)

			// The same should be true both strings are suffixed with an "a"
			oneTest(tt.a+"a", tt.b+"a", true)
			oneTest(tt.b+"a", tt.a+"a", false)
			oneTest(tt.a+"a", tt.a+"a", false)
			oneTest(tt.b+"a", tt.b+"a", false)

			// The same should be true both strings are suffixed with a "1"
			oneTest(tt.a+"1", tt.b+"1", true)
			oneTest(tt.b+"1", tt.a+"1", false)
			oneTest(tt.a+"1", tt.a+"1", false)
			oneTest(tt.b+"1", tt.b+"1", false)

			// The same should be true both strings are prefixed with a "0"
			oneTest("0"+tt.a, "0"+tt.b, true)
			oneTest("0"+tt.b, "0"+tt.a, false)
			oneTest("0"+tt.a, "0"+tt.a, false)
			oneTest("0"+tt.b, "0"+tt.b, false)

			// The same should be true both strings are suffixed with a "0"
			oneTest(tt.a+"0", tt.b+"0", true)
			oneTest(tt.b+"0", tt.a+"0", false)
			oneTest(tt.a+"0", tt.a+"0", false)
			oneTest(tt.b+"0", tt.b+"0", false)

		})
	}
}
