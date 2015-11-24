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
	"reflect"
	"testing"
)

var clonePropertiesTestCases = []struct {
	in  interface{}
	out interface{}
	err error
}{
	// Valid inputs

	{
		// Clone bool
		in: &struct{ B1, B2 bool }{
			B1: true,
			B2: false,
		},
		out: &struct{ B1, B2 bool }{
			B1: true,
			B2: false,
		},
	},
	{
		// Clone strings
		in: &struct{ S string }{
			S: "string1",
		},
		out: &struct{ S string }{
			S: "string1",
		},
	},
	{
		// Clone slice
		in: &struct{ S []string }{
			S: []string{"string1"},
		},
		out: &struct{ S []string }{
			S: []string{"string1"},
		},
	},
	{
		// Clone empty slice
		in: &struct{ S []string }{
			S: []string{},
		},
		out: &struct{ S []string }{
			S: []string{},
		},
	},
	{
		// Clone nil slice
		in:  &struct{ S []string }{},
		out: &struct{ S []string }{},
	},
	{
		// Clone pointer to bool
		in: &struct{ B1, B2 *bool }{
			B1: BoolPtr(true),
			B2: BoolPtr(false),
		},
		out: &struct{ B1, B2 *bool }{
			B1: BoolPtr(true),
			B2: BoolPtr(false),
		},
	},
	{
		// Clone pointer to string
		in: &struct{ S *string }{
			S: StringPtr("string1"),
		},
		out: &struct{ S *string }{
			S: StringPtr("string1"),
		},
	},
	{
		// Clone struct
		in: &struct{ S struct{ S string } }{
			S: struct{ S string }{
				S: "string1",
			},
		},
		out: &struct{ S struct{ S string } }{
			S: struct{ S string }{
				S: "string1",
			},
		},
	},
	{
		// Clone struct pointer
		in: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
		out: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
	},
	{
		// Clone interface
		in: &struct{ S interface{} }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
		out: &struct{ S interface{} }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
	},
	{
		// Clone nested interface
		in: &struct {
			Nested struct{ S interface{} }
		}{
			Nested: struct{ S interface{} }{
				S: &struct{ S string }{
					S: "string1",
				},
			},
		},
		out: &struct {
			Nested struct{ S interface{} }
		}{
			Nested: struct{ S interface{} }{
				S: &struct{ S string }{
					S: "string1",
				},
			},
		},
	}, {
		// Empty struct
		in:  &struct{}{},
		out: &struct{}{},
	},
	{
		// Interface nil
		in: &struct{ S interface{} }{
			S: nil,
		},
		out: &struct{ S interface{} }{
			S: nil,
		},
	},
	{
		// Interface pointer to nil
		in: &struct{ S interface{} }{
			S: (*struct{ S string })(nil),
		},
		out: &struct{ S interface{} }{
			S: (*struct{ S string })(nil),
		},
	},
	{
		// Pointer nil
		in: &struct{ S *struct{} }{
			S: nil,
		},
		out: &struct{ S *struct{} }{
			S: nil,
		},
	},
	{
		// Anonymous struct
		in: &struct {
			EmbeddedStruct
			Nested struct{ EmbeddedStruct }
		}{
			EmbeddedStruct: EmbeddedStruct{
				S: "string1",
			},
			Nested: struct{ EmbeddedStruct }{
				EmbeddedStruct: EmbeddedStruct{
					S: "string2",
				},
			},
		},
		out: &struct {
			EmbeddedStruct
			Nested struct{ EmbeddedStruct }
		}{
			EmbeddedStruct: EmbeddedStruct{
				S: "string1",
			},
			Nested: struct{ EmbeddedStruct }{
				EmbeddedStruct: EmbeddedStruct{
					S: "string2",
				},
			},
		},
	},
	{
		// Anonymous interface
		in: &struct {
			EmbeddedInterface
			Nested struct{ EmbeddedInterface }
		}{
			EmbeddedInterface: &struct{ S string }{
				S: "string1",
			},
			Nested: struct{ EmbeddedInterface }{
				EmbeddedInterface: &struct{ S string }{
					S: "string2",
				},
			},
		},
		out: &struct {
			EmbeddedInterface
			Nested struct{ EmbeddedInterface }
		}{
			EmbeddedInterface: &struct{ S string }{
				S: "string1",
			},
			Nested: struct{ EmbeddedInterface }{
				EmbeddedInterface: &struct{ S string }{
					S: "string2",
				},
			},
		},
	},
}

type EmbeddedStruct struct{ S string }
type EmbeddedInterface interface{}

func TestCloneProperties(t *testing.T) {
	for _, testCase := range clonePropertiesTestCases {
		testString := fmt.Sprintf("%s", testCase.in)

		got := CloneProperties(reflect.ValueOf(testCase.in).Elem()).Interface()

		if !reflect.DeepEqual(testCase.out, got) {
			t.Errorf("test case %s", testString)
			t.Errorf("incorrect output")
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", got)
		}
	}
}

var cloneEmptyPropertiesTestCases = []struct {
	in  interface{}
	out interface{}
	err error
}{
	// Valid inputs

	{
		// Clone bool
		in: &struct{ B1, B2 bool }{
			B1: true,
			B2: false,
		},
		out: &struct{ B1, B2 bool }{},
	},
	{
		// Clone strings
		in: &struct{ S string }{
			S: "string1",
		},
		out: &struct{ S string }{},
	},
	{
		// Clone slice
		in: &struct{ S []string }{
			S: []string{"string1"},
		},
		out: &struct{ S []string }{},
	},
	{
		// Clone empty slice
		in: &struct{ S []string }{
			S: []string{},
		},
		out: &struct{ S []string }{},
	},
	{
		// Clone nil slice
		in:  &struct{ S []string }{},
		out: &struct{ S []string }{},
	},
	{
		// Clone pointer to bool
		in: &struct{ B1, B2 *bool }{
			B1: BoolPtr(true),
			B2: BoolPtr(false),
		},
		out: &struct{ B1, B2 *bool }{},
	},
	{
		// Clone pointer to string
		in: &struct{ S *string }{
			S: StringPtr("string1"),
		},
		out: &struct{ S *string }{},
	},
	{
		// Clone struct
		in: &struct{ S struct{ S string } }{
			S: struct{ S string }{
				S: "string1",
			},
		},
		out: &struct{ S struct{ S string } }{
			S: struct{ S string }{},
		},
	},
	{
		// Clone struct pointer
		in: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
		out: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{},
		},
	},
	{
		// Clone interface
		in: &struct{ S interface{} }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
		out: &struct{ S interface{} }{
			S: &struct{ S string }{},
		},
	},
	{
		// Clone nested interface
		in: &struct {
			Nested struct{ S interface{} }
		}{
			Nested: struct{ S interface{} }{
				S: &struct{ S string }{
					S: "string1",
				},
			},
		},
		out: &struct {
			Nested struct{ S interface{} }
		}{
			Nested: struct{ S interface{} }{
				S: &struct{ S string }{},
			},
		},
	},
	{
		// Empty struct
		in:  &struct{}{},
		out: &struct{}{},
	},
	{
		// Interface nil
		in: &struct{ S interface{} }{
			S: nil,
		},
		out: &struct{ S interface{} }{},
	},
	{
		// Interface pointer to nil
		in: &struct{ S interface{} }{
			S: (*struct{ S string })(nil),
		},
		out: &struct{ S interface{} }{
			S: (*struct{ S string })(nil),
		},
	},
	{
		// Pointer nil
		in: &struct{ S *struct{} }{
			S: nil,
		},
		out: &struct{ S *struct{} }{},
	},
	{
		// Anonymous struct
		in: &struct {
			EmbeddedStruct
			Nested struct{ EmbeddedStruct }
		}{
			EmbeddedStruct: EmbeddedStruct{
				S: "string1",
			},
			Nested: struct{ EmbeddedStruct }{
				EmbeddedStruct: EmbeddedStruct{
					S: "string2",
				},
			},
		},
		out: &struct {
			EmbeddedStruct
			Nested struct{ EmbeddedStruct }
		}{
			EmbeddedStruct: EmbeddedStruct{},
			Nested: struct{ EmbeddedStruct }{
				EmbeddedStruct: EmbeddedStruct{},
			},
		},
	},
	{
		// Anonymous interface
		in: &struct {
			EmbeddedInterface
			Nested struct{ EmbeddedInterface }
		}{
			EmbeddedInterface: &struct{ S string }{
				S: "string1",
			},
			Nested: struct{ EmbeddedInterface }{
				EmbeddedInterface: &struct{ S string }{
					S: "string2",
				},
			},
		},
		out: &struct {
			EmbeddedInterface
			Nested struct{ EmbeddedInterface }
		}{
			EmbeddedInterface: &struct{ S string }{},
			Nested: struct{ EmbeddedInterface }{
				EmbeddedInterface: &struct{ S string }{},
			},
		},
	},
}

func TestCloneEmptyProperties(t *testing.T) {
	for _, testCase := range cloneEmptyPropertiesTestCases {
		testString := fmt.Sprintf("%#v", testCase.in)

		got := CloneEmptyProperties(reflect.ValueOf(testCase.in).Elem()).Interface()

		if !reflect.DeepEqual(testCase.out, got) {
			t.Errorf("test case %s", testString)
			t.Errorf("incorrect output")
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", got)
		}
	}
}

func TestZeroProperties(t *testing.T) {
	for _, testCase := range cloneEmptyPropertiesTestCases {
		testString := fmt.Sprintf("%#v", testCase.in)

		got := CloneProperties(reflect.ValueOf(testCase.in).Elem()).Interface()
		ZeroProperties(reflect.ValueOf(got).Elem())

		if !reflect.DeepEqual(testCase.out, got) {
			t.Errorf("test case %s", testString)
			t.Errorf("incorrect output")
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", got)
		}
	}
}
