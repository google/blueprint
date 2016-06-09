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
	errs   []error
}{
	{`
		m {
			name: "abc",
			blank: "",
		}
		`,
		[]interface{}{
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
		nil,
	},

	{`
		m {
			name: "abc",
		}
		`,
		[]interface{}{
			struct {
				Name string
			}{
				Name: "abc",
			},
		},
		nil,
	},

	{`
		m {
			isGood: true,
		}
		`,
		[]interface{}{
			struct {
				IsGood bool
			}{
				IsGood: true,
			},
		},
		nil,
	},

	{`
		m {
			isGood: true,
			isBad: false,
		}
		`,
		[]interface{}{
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
		nil,
	},

	{`
		m {
			stuff: ["asdf", "jkl;", "qwert",
				"uiop", "bnm,"],
			empty: []
		}
		`,
		[]interface{}{
			struct {
				Stuff []string
				Empty []string
				Nil   []string
			}{
				Stuff: []string{"asdf", "jkl;", "qwert", "uiop", "bnm,"},
				Empty: []string{},
				Nil:   nil,
			},
		},
		nil,
	},

	{`
		m {
			nested: {
				name: "abc",
			}
		}
		`,
		[]interface{}{
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
		nil,
	},

	{`
		m {
			nested: {
				name: "def",
			}
		}
		`,
		[]interface{}{
			struct {
				Nested interface{}
			}{
				Nested: &struct{ Name string }{
					Name: "def",
				},
			},
		},
		nil,
	},

	{`
		m {
			nested: {
				foo: "abc",
			},
			bar: false,
			baz: ["def", "ghi"],
		}
		`,
		[]interface{}{
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
		nil,
	},

	{`
		m {
			nested: {
				foo: "abc",
			},
			bar: false,
			baz: ["def", "ghi"],
		}
		`,
		[]interface{}{
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
		nil,
	},

	{`
		m {
			nested: {
				foo: "abc",
			},
			bar: false,
			baz: ["def", "ghi"],
		}
		`,
		[]interface{}{
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
		[]error{
			&Error{
				Err: fmt.Errorf("filtered field nested.foo cannot be set in a Blueprint file"),
				Pos: mkpos(27, 4, 8),
			},
		},
	},

	// Anonymous struct
	{`
		m {
			name: "abc",
			nested: {
				name: "def",
			},
		}
		`,
		[]interface{}{
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
		nil,
	},

	// Anonymous interface
	{`
		m {
			name: "abc",
			nested: {
				name: "def",
			},
		}
		`,
		[]interface{}{
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
		nil,
	},

	// Anonymous struct with name collision
	{`
		m {
			name: "abc",
			nested: {
				name: "def",
			},
		}
		`,
		[]interface{}{
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
		nil,
	},

	// Anonymous interface with name collision
	{`
		m {
			name: "abc",
			nested: {
				name: "def",
			},
		}
		`,
		[]interface{}{
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
		nil,
	},

	// Variables
	{`
		list = ["abc"]
		string = "def"
		list_with_variable = [string]
		m {
			name: string,
			list: list,
			list2: list_with_variable,
		}
		`,
		[]interface{}{
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
		nil,
	},

	// Multiple property structs
	{`
		m {
			nested: {
				name: "abc",
			}
		}
		`,
		[]interface{}{
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
		nil,
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

			output := []interface{}{}
			for _, p := range testCase.output {
				output = append(output, proptools.CloneEmptyProperties(reflect.ValueOf(p)).Interface())
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
