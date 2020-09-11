// Copyright 2020 Google Inc. All rights reserved.
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

package main

import (
	"strings"
	"testing"

	"github.com/google/blueprint/parser"
)

var testCases = []struct {
	input     string
	output    string
	property  string
	addSet    string
	removeSet string
}{
	{
		`
		cc_foo {
			name: "foo",
		}
		`,
		`
		cc_foo {
			name: "foo",
			deps: ["bar"],
		}
		`,
		"deps",
		"bar",
		"",
	},
	{
		`
		cc_foo {
			name: "foo",
			deps: ["bar"],
		}
		`,
		`
		cc_foo {
			name: "foo",
			deps: [],
		}
		`,
		"deps",
		"",
		"bar",
	},
	{
		`
		cc_foo {
			name: "foo",
		}
		`,
		`
		cc_foo {
			name: "foo",
			arch: {
				arm: {
					deps: [
						"dep2",
						"nested_dep",],
				},
			},
		}
		`,
		"arch.arm.deps",
		"nested_dep,dep2",
		"",
	},
	{
		`
		cc_foo {
			name: "foo",
			arch: {
				arm: {
					deps: [
						"dep2",
						"nested_dep",
					],
				},
			},
		}
		`,
		`
		cc_foo {
			name: "foo",
			arch: {
				arm: {
					deps: [
					],
				},
			},
		}
		`,
		"arch.arm.deps",
		"",
		"nested_dep,dep2",
	},
	{
		`
		cc_foo {
			name: "foo",
			arch: {
				arm: {
					deps: [
						"nested_dep",
						"dep2",
					],
				},
			},
		}
		`,
		`
		cc_foo {
			name: "foo",
			arch: {
				arm: {
					deps: [
						"nested_dep",
						"dep2",
					],
				},
			},
		}
		`,
		"arch.arm.deps",
		"dep2,dep2",
		"",
	},
	{
		`
		cc_foo {
			name: "foo",
			arch: {
				arm: {
					deps: [
						"nested_dep",
						"dep2",
					],
				},
			},
		}
		`,
		`
		cc_foo {
			name: "foo",
			arch: {
				arm: {
					deps: [
						"nested_dep",
						"dep2",
					],
				},
			},
		}
		`,
		"arch.arm.deps",
		"",
		"dep3,dep4",
	},
	{
		`
		cc_foo {
			name: "foo",
		}
		`,
		`
		cc_foo {
			name: "foo",
		}
		`,
		"deps",
		"",
		"bar",
	},
	{
		`
		cc_foo {
			name: "foo",
			arch: {},
		}
		`,
		`
		cc_foo {
			name: "foo",
			arch: {},
		}
		`,
		"arch.arm.deps",
		"",
		"dep3,dep4",
	},
}

func simplifyModuleDefinition(def string) string {
	var result string
	for _, line := range strings.Split(def, "\n") {
		result += strings.TrimSpace(line)
	}
	return result
}

func TestProcessModule(t *testing.T) {
	for i, testCase := range testCases {
		targetedProperty.Set(testCase.property)
		addIdents.Set(testCase.addSet)
		removeIdents.Set(testCase.removeSet)

		inAst, errs := parser.ParseAndEval("", strings.NewReader(testCase.input), parser.NewScope(nil))
		if len(errs) > 0 {
			t.Errorf("test case %d:", i)
			for _, err := range errs {
				t.Errorf("  %s", err)
			}
			t.Errorf("failed to parse:")
			t.Errorf("%+v", testCase)
			continue
		}

		if inModule, ok := inAst.Defs[0].(*parser.Module); !ok {
			t.Errorf("test case %d:", i)
			t.Errorf("  input must only contain a single module definition: %s", testCase.input)
			continue
		} else {
			_, errs := processModule(inModule, "", inAst)
			if len(errs) > 0 {
				t.Errorf("test case %d:", i)
				for _, err := range errs {
					t.Errorf("  %s", err)
				}
			}
			inModuleText, _ := parser.Print(inAst)
			inModuleString := string(inModuleText)
			if simplifyModuleDefinition(inModuleString) != simplifyModuleDefinition(testCase.output) {
				t.Errorf("test case %d:", i)
				t.Errorf("expected module definition:")
				t.Errorf("  %s", testCase.output)
				t.Errorf("actual module definition:")
				t.Errorf("  %s", inModuleString)
			}
		}
	}
}
