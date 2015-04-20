// Copyright 2014 Google Inc. All rights reserved.
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
	"reflect"
	"testing"
)

var ninjaParseTestCases = []struct {
	input string
	vars  []string
	strs  []string
	err   string
}{
	{
		input: "abc def $ghi jkl",
		vars:  []string{"ghi"},
		strs:  []string{"abc def ", " jkl"},
	},
	{
		input: "abc def $ghi$jkl",
		vars:  []string{"ghi", "jkl"},
		strs:  []string{"abc def ", "", ""},
	},
	{
		input: "foo $012_-345xyz_! bar",
		vars:  []string{"012_-345xyz_"},
		strs:  []string{"foo ", "! bar"},
	},
	{
		input: "foo ${012_-345xyz_} bar",
		vars:  []string{"012_-345xyz_"},
		strs:  []string{"foo ", " bar"},
	},
	{
		input: "foo ${012_-345xyz_} bar",
		vars:  []string{"012_-345xyz_"},
		strs:  []string{"foo ", " bar"},
	},
	{
		input: "foo $$ bar",
		vars:  nil,
		strs:  []string{"foo $$ bar"},
	},
	{
		input: "$foo${bar}",
		vars:  []string{"foo", "bar"},
		strs:  []string{"", "", ""},
	},
	{
		input: "$foo$$",
		vars:  []string{"foo"},
		strs:  []string{"", "$$"},
	},
	{
		input: "foo bar",
		vars:  nil,
		strs:  []string{"foo bar"},
	},
	{
		input: "foo $ bar",
		err:   "invalid character after '$' at byte offset 5",
	},
	{
		input: "foo $",
		err:   "unexpected end of string after '$'",
	},
	{
		input: "foo ${} bar",
		err:   "empty variable name at byte offset 6",
	},
	{
		input: "foo ${abc!} bar",
		err:   "invalid character in variable name at byte offset 9",
	},
	{
		input: "foo ${abc",
		err:   "unexpected end of string in variable name",
	},
}

func TestParseNinjaString(t *testing.T) {
	for _, testCase := range ninjaParseTestCases {
		scope := newLocalScope(nil, "namespace")
		expectedVars := []Variable{}
		for _, varName := range testCase.vars {
			v, err := scope.LookupVariable(varName)
			if err != nil {
				v, err = scope.AddLocalVariable(varName, "")
				if err != nil {
					t.Fatalf("error creating scope: %s", err)
				}
			}
			expectedVars = append(expectedVars, v)
		}

		output, err := parseNinjaString(scope, testCase.input)
		if err == nil {
			if !reflect.DeepEqual(output.variables, expectedVars) {
				t.Errorf("incorrect variable list:")
				t.Errorf("     input: %q", testCase.input)
				t.Errorf("  expected: %#v", expectedVars)
				t.Errorf("       got: %#v", output.variables)
			}
			if !reflect.DeepEqual(output.strings, testCase.strs) {
				t.Errorf("incorrect string list:")
				t.Errorf("     input: %q", testCase.input)
				t.Errorf("  expected: %#v", testCase.strs)
				t.Errorf("       got: %#v", output.strings)
			}
		}
		var errStr string
		if err != nil {
			errStr = err.Error()
		}
		if err != nil && err.Error() != testCase.err {
			t.Errorf("unexpected error:")
			t.Errorf("     input: %q", testCase.input)
			t.Errorf("  expected: %q", testCase.err)
			t.Errorf("       got: %q", errStr)
		}
	}
}

func TestParseNinjaStringWithImportedVar(t *testing.T) {
	ImpVar := &staticVariable{name_: "ImpVar"}
	impScope := newScope(nil)
	impScope.AddVariable(ImpVar)
	scope := newScope(nil)
	scope.AddImport("impPkg", impScope)

	input := "abc def ${impPkg.ImpVar} ghi"
	output, err := parseNinjaString(scope, input)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	expect := []Variable{ImpVar}
	if !reflect.DeepEqual(output.variables, expect) {
		t.Errorf("incorrect output:")
		t.Errorf("     input: %q", input)
		t.Errorf("  expected: %#v", expect)
		t.Errorf("       got: %#v", output)
	}
}
