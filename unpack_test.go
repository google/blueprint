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
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"text/scanner"

	"github.com/google/blueprint/parser"
	"github.com/google/blueprint/proptools"
)

var validUnpackTestCases = []struct {
	input  string
	output []interface{}
	empty  []interface{}
	errs   []error
}{
	{
		input: `
			m {
				name: "abc",
				blank: "",
			}
		`,
		output: []interface{}{
			struct {
				Name  *string
				Blank *string
				Unset *string
			}{
				Name:  proptools.StringPtr("abc"),
				Blank: proptools.StringPtr(""),
				Unset: nil,
			},
		},
	},

	{
		input: `
			m {
				name: "abc",
			}
		`,
		output: []interface{}{
			struct {
				Name string
			}{
				Name: "abc",
			},
		},
	},

	{
		input: `
			m {
				isGood: true,
			}
		`,
		output: []interface{}{
			struct {
				IsGood bool
			}{
				IsGood: true,
			},
		},
	},

	{
		input: `
			m {
				isGood: true,
				isBad: false,
			}
		`,
		output: []interface{}{
			struct {
				IsGood *bool
				IsBad  *bool
				IsUgly *bool
			}{
				IsGood: proptools.BoolPtr(true),
				IsBad:  proptools.BoolPtr(false),
				IsUgly: nil,
			},
		},
	},

	{
		input: `
			m {
				stuff: ["asdf", "jkl;", "qwert",
					"uiop", "bnm,"],
				empty: []
			}
		`,
		output: []interface{}{
			struct {
				Stuff     []string
				Empty     []string
				Nil       []string
				NonString []struct{ S string } `blueprint:"mutated"`
			}{
				Stuff:     []string{"asdf", "jkl;", "qwert", "uiop", "bnm,"},
				Empty:     []string{},
				Nil:       nil,
				NonString: nil,
			},
		},
	},

	{
		input: `
			m {
				nested: {
					name: "abc",
				}
			}
		`,
		output: []interface{}{
			struct {
				Nested struct {
					Name string
				}
			}{
				Nested: struct{ Name string }{
					Name: "abc",
				},
			},
		},
	},

	{
		input: `
			m {
				nested: {
					name: "def",
				}
			}
		`,
		output: []interface{}{
			struct {
				Nested interface{}
			}{
				Nested: &struct{ Name string }{
					Name: "def",
				},
			},
		},
	},

	{
		input: `
			m {
				nested: {
					foo: "abc",
				},
				bar: false,
				baz: ["def", "ghi"],
			}
		`,
		output: []interface{}{
			struct {
				Nested struct {
					Foo string
				}
				Bar bool
				Baz []string
			}{
				Nested: struct{ Foo string }{
					Foo: "abc",
				},
				Bar: false,
				Baz: []string{"def", "ghi"},
			},
		},
	},

	{
		input: `
			m {
				nested: {
					foo: "abc",
				},
				bar: false,
				baz: ["def", "ghi"],
			}
		`,
		output: []interface{}{
			struct {
				Nested struct {
					Foo string `allowNested:"true"`
				} `blueprint:"filter(allowNested:\"true\")"`
				Bar bool
				Baz []string
			}{
				Nested: struct {
					Foo string `allowNested:"true"`
				}{
					Foo: "abc",
				},
				Bar: false,
				Baz: []string{"def", "ghi"},
			},
		},
	},

	{
		input: `
			m {
				nested: {
					foo: "abc",
				},
				bar: false,
				baz: ["def", "ghi"],
			}
		`,
		output: []interface{}{
			struct {
				Nested struct {
					Foo string
				} `blueprint:"filter(allowNested:\"true\")"`
				Bar bool
				Baz []string
			}{
				Nested: struct{ Foo string }{
					Foo: "",
				},
				Bar: false,
				Baz: []string{"def", "ghi"},
			},
		},
		errs: []error{
			&BlueprintError{
				Err: fmt.Errorf("filtered field nested.foo cannot be set in a Blueprint file"),
				Pos: mkpos(30, 4, 9),
			},
		},
	},

	// Anonymous struct
	{
		input: `
			m {
				name: "abc",
				nested: {
					name: "def",
				},
			}
		`,
		output: []interface{}{
			struct {
				EmbeddedStruct
				Nested struct {
					EmbeddedStruct
				}
			}{
				EmbeddedStruct: EmbeddedStruct{
					Name: "abc",
				},
				Nested: struct {
					EmbeddedStruct
				}{
					EmbeddedStruct: EmbeddedStruct{
						Name: "def",
					},
				},
			},
		},
	},

	// Anonymous interface
	{
		input: `
			m {
				name: "abc",
				nested: {
					name: "def",
				},
			}
		`,
		output: []interface{}{
			struct {
				EmbeddedInterface
				Nested struct {
					EmbeddedInterface
				}
			}{
				EmbeddedInterface: &struct{ Name string }{
					Name: "abc",
				},
				Nested: struct {
					EmbeddedInterface
				}{
					EmbeddedInterface: &struct{ Name string }{
						Name: "def",
					},
				},
			},
		},
	},

	// Anonymous struct with name collision
	{
		input: `
			m {
				name: "abc",
				nested: {
					name: "def",
				},
			}
		`,
		output: []interface{}{
			struct {
				Name string
				EmbeddedStruct
				Nested struct {
					Name string
					EmbeddedStruct
				}
			}{
				Name: "abc",
				EmbeddedStruct: EmbeddedStruct{
					Name: "abc",
				},
				Nested: struct {
					Name string
					EmbeddedStruct
				}{
					Name: "def",
					EmbeddedStruct: EmbeddedStruct{
						Name: "def",
					},
				},
			},
		},
	},

	// Anonymous interface with name collision
	{
		input: `
			m {
				name: "abc",
				nested: {
					name: "def",
				},
			}
		`,
		output: []interface{}{
			struct {
				Name string
				EmbeddedInterface
				Nested struct {
					Name string
					EmbeddedInterface
				}
			}{
				Name: "abc",
				EmbeddedInterface: &struct{ Name string }{
					Name: "abc",
				},
				Nested: struct {
					Name string
					EmbeddedInterface
				}{
					Name: "def",
					EmbeddedInterface: &struct{ Name string }{
						Name: "def",
					},
				},
			},
		},
	},

	// Variables
	{
		input: `
			list = ["abc"]
			string = "def"
			list_with_variable = [string]
			m {
				name: string,
				list: list,
				list2: list_with_variable,
			}
		`,
		output: []interface{}{
			struct {
				Name  string
				List  []string
				List2 []string
			}{
				Name:  "def",
				List:  []string{"abc"},
				List2: []string{"def"},
			},
		},
	},

	// Multiple property structs
	{
		input: `
			m {
				nested: {
					name: "abc",
				}
			}
		`,
		output: []interface{}{
			struct {
				Nested struct {
					Name string
				}
			}{
				Nested: struct{ Name string }{
					Name: "abc",
				},
			},
			struct {
				Nested struct {
					Name string
				}
			}{
				Nested: struct{ Name string }{
					Name: "abc",
				},
			},
			struct {
			}{},
		},
	},

	// Nil pointer to struct
	{
		input: `
			m {
				nested: {
					name: "abc",
				}
			}
		`,
		output: []interface{}{
			struct {
				Nested *struct {
					Name string
				}
			}{
				Nested: &struct{ Name string }{
					Name: "abc",
				},
			},
		},
		empty: []interface{}{
			&struct {
				Nested *struct {
					Name string
				}
			}{},
		},
	},

	// Interface containing nil pointer to struct
	{
		input: `
			m {
				nested: {
					name: "abc",
				}
			}
		`,
		output: []interface{}{
			struct {
				Nested interface{}
			}{
				Nested: &EmbeddedStruct{
					Name: "abc",
				},
			},
		},
		empty: []interface{}{
			&struct {
				Nested interface{}
			}{
				Nested: (*EmbeddedStruct)(nil),
			},
		},
	},

	// Factory set properties
	{
		input: `
			m {
				string: "abc",
				string_ptr: "abc",
				bool: false,
				bool_ptr: false,
				list: ["a", "b", "c"],
			}
		`,
		output: []interface{}{
			struct {
				String     string
				String_ptr *string
				Bool       bool
				Bool_ptr   *bool
				List       []string
			}{
				String:     "012abc",
				String_ptr: proptools.StringPtr("abc"),
				Bool:       true,
				Bool_ptr:   proptools.BoolPtr(false),
				List:       []string{"0", "1", "2", "a", "b", "c"},
			},
		},
		empty: []interface{}{
			&struct {
				String     string
				String_ptr *string
				Bool       bool
				Bool_ptr   *bool
				List       []string
			}{
				String:     "012",
				String_ptr: proptools.StringPtr("012"),
				Bool:       true,
				Bool_ptr:   proptools.BoolPtr(true),
				List:       []string{"0", "1", "2"},
			},
		},
	},
}

type EmbeddedStruct struct{ Name string }
type EmbeddedInterface interface{}

func TestUnpackProperties(t *testing.T) {
	for _, testCase := range validUnpackTestCases {
		r := bytes.NewBufferString(testCase.input)
		file, errs := parser.ParseAndEval("", r, parser.NewScope(nil))
		if len(errs) != 0 {
			t.Errorf("test case: %s", testCase.input)
			t.Errorf("unexpected parse errors:")
			for _, err := range errs {
				t.Errorf("  %s", err)
			}
			t.FailNow()
		}

		for _, def := range file.Defs {
			module, ok := def.(*parser.Module)
			if !ok {
				continue
			}

			var output []interface{}
			if len(testCase.empty) > 0 {
				output = testCase.empty
			} else {
				for _, p := range testCase.output {
					output = append(output, proptools.CloneEmptyProperties(reflect.ValueOf(p)).Interface())
				}
			}
			_, errs = unpackProperties(module.Properties, output...)
			if len(errs) != 0 && len(testCase.errs) == 0 {
				t.Errorf("test case: %s", testCase.input)
				t.Errorf("unexpected unpack errors:")
				for _, err := range errs {
					t.Errorf("  %s", err)
				}
				t.FailNow()
			} else if !reflect.DeepEqual(errs, testCase.errs) {
				t.Errorf("test case: %s", testCase.input)
				t.Errorf("incorrect errors:")
				t.Errorf("  expected: %+v", testCase.errs)
				t.Errorf("       got: %+v", errs)
			}

			if len(output) != len(testCase.output) {
				t.Fatalf("incorrect number of property structs, expected %d got %d",
					len(testCase.output), len(output))
			}

			for i := range output {
				got := reflect.ValueOf(output[i]).Elem().Interface()
				if !reflect.DeepEqual(got, testCase.output[i]) {
					t.Errorf("test case: %s", testCase.input)
					t.Errorf("incorrect output:")
					t.Errorf("  expected: %+v", testCase.output[i])
					t.Errorf("       got: %+v", got)
				}
			}
		}
	}
}

func mkpos(offset, line, column int) scanner.Position {
	return scanner.Position{
		Offset: offset,
		Line:   line,
		Column: column,
	}
}
