// Copyright 2017 Google Inc. All rights reserved.
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

// This file tests that mutations to the abstract syntax tree work as intended
// In practice, that primarily means checking that comments move with the moved pieces of the tree

var mutationTestCases = []struct {
	input  string
	output string
}{}

func TestMutation(t *testing.T) {
	for _, testCase := range validPrinterTestCases {
		in := testCase.input[1:]
		expected := testCase.output[1:]

		r := bytes.NewBufferString(in)
		parse, errs := Parse("", r, NewScope(nil))
		if len(errs) != 0 {
			t.Errorf("test case: %s", in)
			t.Errorf("unexpected errors:")
			for _, err := range errs {
				t.Errorf("  %s", err)
			}
			t.FailNow()
		}

		got := PrintTree(&parse)

		if string(got) != expected {
			t.Errorf(
				"\ntest case: \n%s\n"+
					"expected: \n%s\n"+
					"got: \n%s\n",
				in, expected, got)
		}
	}
}
