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

package proptools

import (
	"bytes"
	"reflect"

	"testing"

	"github.com/google/blueprint/parser"
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
				s: "abc",
				blank: "",
			}
		`,
		output: []interface{}{
			&struct {
				S     *string
				Blank *string
				Unset *string
			}{
				S:     StringPtr("abc"),
				Blank: StringPtr(""),
				Unset: nil,
			},
		},
	},

	{
		input: `
			m {
				s: "abc",
			}
		`,
		output: []interface{}{
			&struct {
				S string
			}{
				S: "abc",
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
			&struct {
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
			&struct {
				IsGood *bool
				IsBad  *bool
				IsUgly *bool
			}{
				IsGood: BoolPtr(true),
				IsBad:  BoolPtr(false),
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
			&struct {
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
					s: "abc",
				}
			}
		`,
		output: []interface{}{
			&struct {
				Nested struct {
					S string
				}
			}{
				Nested: struct{ S string }{
					S: "abc",
				},
			},
		},
	},

	{
		input: `
			m {
				nested: {
					s: "def",
				}
			}
		`,
		output: []interface{}{
			&struct {
				Nested interface{}
			}{
				Nested: &struct{ S string }{
					S: "def",
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
			&struct {
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
			&struct {
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

	// List of maps
	{
		input: `
			m {
				mapslist: [
					{
						foo: "abc",
						bar: true,
					},
					{
						foo: "def",
						bar: false,
					}
				],
			}
		`,
		output: []interface{}{
			&struct {
				Mapslist []struct {
					Foo string
					Bar bool
				}
			}{
				Mapslist: []struct {
					Foo string
					Bar bool
				}{
					{Foo: "abc", Bar: true},
					{Foo: "def", Bar: false},
				},
			},
		},
	},

	// List of pointers to structs
	{
		input: `
			m {
				mapslist: [
					{
						foo: "abc",
						bar: true,
					},
					{
						foo: "def",
						bar: false,
					}
				],
			}
		`,
		output: []interface{}{
			&struct {
				Mapslist []*struct {
					Foo string
					Bar bool
				}
			}{
				Mapslist: []*struct {
					Foo string
					Bar bool
				}{
					{Foo: "abc", Bar: true},
					{Foo: "def", Bar: false},
				},
			},
		},
	},

	// List of lists
	{
		input: `
			m {
				listoflists: [
					["abc",],
					["def",],
				],
			}
		`,
		output: []interface{}{
			&struct {
				Listoflists [][]string
			}{
				Listoflists: [][]string{
					[]string{"abc"},
					[]string{"def"},
				},
			},
		},
	},

	// Multilevel
	{
		input: `
			m {
				name: "mymodule",
				flag: true,
				settings: ["foo1", "foo2", "foo3",],
				perarch: {
					arm: "32",
					arm64: "64",
				},
				configvars: [
					{ var: "var1", values: ["1.1", "1.2", ], },
					{ var: "var2", values: ["2.1", ], },
				],
            }
        `,
		output: []interface{}{
			&struct {
				Name     string
				Flag     bool
				Settings []string
				Perarch  *struct {
					Arm   string
					Arm64 string
				}
				Configvars []struct {
					Var    string
					Values []string
				}
			}{
				Name:     "mymodule",
				Flag:     true,
				Settings: []string{"foo1", "foo2", "foo3"},
				Perarch: &struct {
					Arm   string
					Arm64 string
				}{Arm: "32", Arm64: "64"},
				Configvars: []struct {
					Var    string
					Values []string
				}{
					{Var: "var1", Values: []string{"1.1", "1.2"}},
					{Var: "var2", Values: []string{"2.1"}},
				},
			},
		},
	},
	// Anonymous struct
	{
		input: `
			m {
				s: "abc",
				nested: {
					s: "def",
				},
			}
		`,
		output: []interface{}{
			&struct {
				EmbeddedStruct
				Nested struct {
					EmbeddedStruct
				}
			}{
				EmbeddedStruct: EmbeddedStruct{
					S: "abc",
				},
				Nested: struct {
					EmbeddedStruct
				}{
					EmbeddedStruct: EmbeddedStruct{
						S: "def",
					},
				},
			},
		},
	},

	// Anonymous interface
	{
		input: `
			m {
				s: "abc",
				nested: {
					s: "def",
				},
			}
		`,
		output: []interface{}{
			&struct {
				EmbeddedInterface
				Nested struct {
					EmbeddedInterface
				}
			}{
				EmbeddedInterface: &struct{ S string }{
					S: "abc",
				},
				Nested: struct {
					EmbeddedInterface
				}{
					EmbeddedInterface: &struct{ S string }{
						S: "def",
					},
				},
			},
		},
	},

	// Anonymous struct with name collision
	{
		input: `
			m {
				s: "abc",
				nested: {
					s: "def",
				},
			}
		`,
		output: []interface{}{
			&struct {
				S string
				EmbeddedStruct
				Nested struct {
					S string
					EmbeddedStruct
				}
			}{
				S: "abc",
				EmbeddedStruct: EmbeddedStruct{
					S: "abc",
				},
				Nested: struct {
					S string
					EmbeddedStruct
				}{
					S: "def",
					EmbeddedStruct: EmbeddedStruct{
						S: "def",
					},
				},
			},
		},
	},

	// Anonymous interface with name collision
	{
		input: `
			m {
				s: "abc",
				nested: {
					s: "def",
				},
			}
		`,
		output: []interface{}{
			&struct {
				S string
				EmbeddedInterface
				Nested struct {
					S string
					EmbeddedInterface
				}
			}{
				S: "abc",
				EmbeddedInterface: &struct{ S string }{
					S: "abc",
				},
				Nested: struct {
					S string
					EmbeddedInterface
				}{
					S: "def",
					EmbeddedInterface: &struct{ S string }{
						S: "def",
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
			struct_value = { name: "foo" }
			m {
				s: string,
				list: list,
				list2: list_with_variable,
				structattr: struct_value,
			}
		`,
		output: []interface{}{
			&struct {
				S          string
				List       []string
				List2      []string
				Structattr struct {
					Name string
				}
			}{
				S:     "def",
				List:  []string{"abc"},
				List2: []string{"def"},
				Structattr: struct {
					Name string
				}{
					Name: "foo",
				},
			},
		},
	},

	// Multiple property structs
	{
		input: `
			m {
				nested: {
					s: "abc",
				}
			}
		`,
		output: []interface{}{
			&struct {
				Nested struct {
					S string
				}
			}{
				Nested: struct{ S string }{
					S: "abc",
				},
			},
			&struct {
				Nested struct {
					S string
				}
			}{
				Nested: struct{ S string }{
					S: "abc",
				},
			},
			&struct {
			}{},
		},
	},

	// Nil pointer to struct
	{
		input: `
			m {
				nested: {
					s: "abc",
				}
			}
		`,
		output: []interface{}{
			&struct {
				Nested *struct {
					S string
				}
			}{
				Nested: &struct{ S string }{
					S: "abc",
				},
			},
		},
		empty: []interface{}{
			&struct {
				Nested *struct {
					S string
				}
			}{},
		},
	},

	// Interface containing nil pointer to struct
	{
		input: `
			m {
				nested: {
					s: "abc",
				}
			}
		`,
		output: []interface{}{
			&struct {
				Nested interface{}
			}{
				Nested: &EmbeddedStruct{
					S: "abc",
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
			&struct {
				String     string
				String_ptr *string
				Bool       bool
				Bool_ptr   *bool
				List       []string
			}{
				String:     "012abc",
				String_ptr: StringPtr("abc"),
				Bool:       true,
				Bool_ptr:   BoolPtr(false),
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
				String_ptr: StringPtr("012"),
				Bool:       true,
				Bool_ptr:   BoolPtr(true),
				List:       []string{"0", "1", "2"},
			},
		},
	},
	// Captitalized property
	{
		input: `
			m {
				CAPITALIZED: "foo",
			}
		`,
		output: []interface{}{
			&struct {
				CAPITALIZED string
			}{
				CAPITALIZED: "foo",
			},
		},
	},
}

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
					output = append(output, CloneEmptyProperties(reflect.ValueOf(p)).Interface())
				}
			}
			_, errs = UnpackProperties(module.Properties, output...)
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
				got := reflect.ValueOf(output[i]).Interface()
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

func BenchmarkUnpackProperties(b *testing.B) {
	run := func(b *testing.B, props []interface{}, input string) {
		b.ReportAllocs()
		b.StopTimer()
		r := bytes.NewBufferString(input)
		file, errs := parser.ParseAndEval("", r, parser.NewScope(nil))
		if len(errs) != 0 {
			b.Errorf("test case: %s", input)
			b.Errorf("unexpected parse errors:")
			for _, err := range errs {
				b.Errorf("  %s", err)
			}
			b.FailNow()
		}

		for i := 0; i < b.N; i++ {
			for _, def := range file.Defs {
				module, ok := def.(*parser.Module)
				if !ok {
					continue
				}

				var output []interface{}
				for _, p := range props {
					output = append(output, CloneProperties(reflect.ValueOf(p)).Interface())
				}

				b.StartTimer()
				_, errs = UnpackProperties(module.Properties, output...)
				b.StopTimer()
				if len(errs) > 0 {
					b.Errorf("unexpected unpack errors:")
					for _, err := range errs {
						b.Errorf("  %s", err)
					}
				}
			}
		}
	}

	b.Run("basic", func(b *testing.B) {
		props := []interface{}{
			&struct {
				Nested struct {
					S string
				}
			}{},
		}
		bp := `
			m {
				nested: {
					s: "abc",
				},
			}
		`
		run(b, props, bp)
	})

	b.Run("interface", func(b *testing.B) {
		props := []interface{}{
			&struct {
				Nested interface{}
			}{
				Nested: (*struct {
					S string
				})(nil),
			},
		}
		bp := `
			m {
				nested: {
					s: "abc",
				},
			}
		`
		run(b, props, bp)
	})

	b.Run("many", func(b *testing.B) {
		props := []interface{}{
			&struct {
				A *string
				B *string
				C *string
				D *string
				E *string
				F *string
				G *string
				H *string
				I *string
				J *string
			}{},
		}
		bp := `
			m {
				a: "a",
				b: "b",
				c: "c",
				d: "d",
				e: "e",
				f: "f",
				g: "g",
				h: "h",
				i: "i",
				j: "j",
			}
		`
		run(b, props, bp)
	})

	b.Run("deep", func(b *testing.B) {
		props := []interface{}{
			&struct {
				Nested struct {
					Nested struct {
						Nested struct {
							Nested struct {
								Nested struct {
									Nested struct {
										Nested struct {
											Nested struct {
												Nested struct {
													Nested struct {
														S string
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}{},
		}
		bp := `
			m {
				nested: { nested: { nested: { nested: { nested: {
					nested: { nested: { nested: { nested: { nested: {
						s: "abc",
					}, }, }, }, },
				}, }, }, }, },
			}
		`
		run(b, props, bp)
	})

	b.Run("mix", func(b *testing.B) {
		props := []interface{}{
			&struct {
				Name     string
				Flag     bool
				Settings []string
				Perarch  *struct {
					Arm   string
					Arm64 string
				}
				Configvars []struct {
					Name   string
					Values []string
				}
			}{},
		}
		bp := `
			m {
				name: "mymodule",
				flag: true,
				settings: ["foo1", "foo2", "foo3",],
				perarch: {
					arm: "32",
					arm64: "64",
				},
				configvars: [
					{ name: "var1", values: ["var1:1", "var1:2", ], },
					{ name: "var2", values: ["var2:1", "var2:2", ], },
				],
            }
        `
		run(b, props, bp)
	})
}
