// Copyright 2015 Google Inc. All rights reserved.
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
	"fmt"
	"testing"
)

var typeEqualTestCases = []struct {
	in1 interface{}
	in2 interface{}
	out bool
}{
	{
		// Matching structs
		in1: struct{ S1 string }{},
		in2: struct{ S1 string }{},
		out: true,
	},
	{
		// Mismatching structs
		in1: struct{ S1 string }{},
		in2: struct{ S2 string }{},
		out: false,
	},
	{
		// Matching pointer to struct
		in1: &struct{ S1 string }{},
		in2: &struct{ S1 string }{},
		out: true,
	},
	{
		// Mismatching pointer to struct
		in1: &struct{ S1 string }{},
		in2: &struct{ S2 string }{},
		out: false,
	},
	{
		// Matching embedded structs
		in1: struct{ S struct{ S1 string } }{},
		in2: struct{ S struct{ S1 string } }{},
		out: true,
	},
	{
		// Misatching embedded structs
		in1: struct{ S struct{ S1 string } }{},
		in2: struct{ S struct{ S2 string } }{},
		out: false,
	},
	{
		// Matching embedded pointer to struct
		in1: &struct{ S *struct{ S1 string } }{S: &struct{ S1 string }{}},
		in2: &struct{ S *struct{ S1 string } }{S: &struct{ S1 string }{}},
		out: true,
	},
	{
		// Mismatching embedded pointer to struct
		in1: &struct{ S *struct{ S1 string } }{S: &struct{ S1 string }{}},
		in2: &struct{ S *struct{ S2 string } }{S: &struct{ S2 string }{}},
		out: false,
	},
	{
		// Matching embedded nil pointer to struct
		in1: &struct{ S *struct{ S1 string } }{},
		in2: &struct{ S *struct{ S1 string } }{},
		out: true,
	},
	{
		// Mismatching embedded nil pointer to struct
		in1: &struct{ S *struct{ S1 string } }{},
		in2: &struct{ S *struct{ S2 string } }{},
		out: false,
	},
	{
		// Mismatching nilitude embedded  pointer to struct
		in1: &struct{ S *struct{ S1 string } }{S: &struct{ S1 string }{}},
		in2: &struct{ S *struct{ S1 string } }{},
		out: false,
	},
	{
		// Matching embedded interface to pointer to struct
		in1: &struct{ S interface{} }{S: &struct{ S1 string }{}},
		in2: &struct{ S interface{} }{S: &struct{ S1 string }{}},
		out: true,
	},
	{
		// Mismatching embedded interface to pointer to struct
		in1: &struct{ S interface{} }{S: &struct{ S1 string }{}},
		in2: &struct{ S interface{} }{S: &struct{ S2 string }{}},
		out: false,
	},
	{
		// Matching embedded nil interface to pointer to struct
		in1: &struct{ S interface{} }{},
		in2: &struct{ S interface{} }{},
		out: true,
	},
	{
		// Mismatching nilitude embedded  interface to pointer to struct
		in1: &struct{ S interface{} }{S: &struct{ S1 string }{}},
		in2: &struct{ S interface{} }{},
		out: false,
	},
	{
		// Matching pointer to non-struct
		in1: struct{ S1 *string }{ S1: StringPtr("test1") },
		in2: struct{ S1 *string }{ S1: StringPtr("test2") },
		out: true,
	},
	{
		// Matching nilitude pointer to non-struct
		in1: struct{ S1 *string }{ S1: StringPtr("test1") },
		in2: struct{ S1 *string }{},
		out: true,
	},
	{
		// Mismatching pointer to non-struct
		in1: struct{ S1 *string }{},
		in2: struct{ S2 *string }{},
		out: false,
	},
}

func TestTypeEqualProperties(t *testing.T) {
	for _, testCase := range typeEqualTestCases {
		testString := fmt.Sprintf("%#v, %#v -> %t", testCase.in1, testCase.in2, testCase.out)

		got := TypeEqual(testCase.in1, testCase.in2)

		if got != testCase.out {
			t.Errorf("test case: %s", testString)
			t.Errorf("incorrect output")
			t.Errorf("  expected: %t", testCase.out)
			t.Errorf("       got: %t", got)
		}
	}
}
