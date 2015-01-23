package blueprint

import (
	"blueprint/parser"
	"blueprint/proptools"
	"bytes"
	"reflect"
	"testing"
)

var validUnpackTestCases = []struct {
	input  string
	output interface{}
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
	},

	{`
		m {
			stuff: ["asdf", "jkl;", "qwert",
				"uiop", "bnm,"]
		}
		`,
		struct {
			Stuff []string
		}{
			Stuff: []string{"asdf", "jkl;", "qwert", "uiop", "bnm,"},
		},
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
		if len(errs) != 0 {
			t.Errorf("test case: %s", testCase.input)
			t.Errorf("unexpected unpack errors:")
			for _, err := range errs {
				t.Errorf("  %s", err)
			}
			t.FailNow()
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
