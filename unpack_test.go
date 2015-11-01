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
	output interface{}
	errs   []error
}{
	{`
		m {
			name: "abc",
		}
		`,
		struct {
			Name string
		}{
			Name: "abc",
		},
		nil,
	},

	{`
		m {
			isGood: true,
		}
		`,
		struct {
			IsGood bool
		}{
			IsGood: true,
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
		struct {
			Stuff []string
			Empty []string
			Nil   []string
		}{
			Stuff: []string{"asdf", "jkl;", "qwert", "uiop", "bnm,"},
			Empty: []string{},
			Nil:   nil,
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
		struct {
			Nested struct {
				Name string
			}
		}{
			Nested: struct{ Name string }{
				Name: "abc",
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
		struct {
			Nested interface{}
		}{
			Nested: &struct{ Name string }{
				Name: "def",
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
		[]error{
			&Error{
				Err: fmt.Errorf("filtered field nested.foo cannot be set in a Blueprint file"),
				Pos: scanner.Position{"", 27, 4, 8},
			},
		},
	},
}

func TestUnpackProperties(t *testing.T) {
	for _, testCase := range validUnpackTestCases {
		r := bytes.NewBufferString(testCase.input)
		file, errs := parser.Parse("", r, nil)
		if len(errs) != 0 {
			t.Errorf("test case: %s", testCase.input)
			t.Errorf("unexpected parse errors:")
			for _, err := range errs {
				t.Errorf("  %s", err)
			}
			t.FailNow()
		}

		module := file.Defs[0].(*parser.Module)
		properties := proptools.CloneProperties(reflect.ValueOf(testCase.output))
		proptools.ZeroProperties(properties.Elem())
		_, errs = unpackProperties(module.Properties, properties.Interface())
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

		output := properties.Elem().Interface()
		if !reflect.DeepEqual(output, testCase.output) {
			t.Errorf("test case: %s", testCase.input)
			t.Errorf("incorrect output:")
			t.Errorf("  expected: %+v", testCase.output)
			t.Errorf("       got: %+v", output)
		}
	}
}
