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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

var appendPropertiesTestCases = []struct {
	in1     interface{}
	in2     interface{}
	out     interface{}
	prepend bool
	filter  ExtendPropertyFilterFunc
	err     error
}{
	// Valid inputs

	{
		// Append bool
		in1: &struct{ B1, B2, B3, B4 bool }{
			B1: true,
			B2: false,
			B3: true,
			B4: false,
		},
		in2: &struct{ B1, B2, B3, B4 bool }{
			B1: true,
			B2: true,
			B3: false,
			B4: false,
		},
		out: &struct{ B1, B2, B3, B4 bool }{
			B1: true,
			B2: true,
			B3: true,
			B4: false,
		},
	},
	{
		// Prepend bool
		in1: &struct{ B1, B2, B3, B4 bool }{
			B1: true,
			B2: false,
			B3: true,
			B4: false,
		},
		in2: &struct{ B1, B2, B3, B4 bool }{
			B1: true,
			B2: true,
			B3: false,
			B4: false,
		},
		out: &struct{ B1, B2, B3, B4 bool }{
			B1: true,
			B2: true,
			B3: true,
			B4: false,
		},
		prepend: true,
	},
	{
		// Append strings
		in1: &struct{ S string }{
			S: "string1",
		},
		in2: &struct{ S string }{
			S: "string2",
		},
		out: &struct{ S string }{
			S: "string1string2",
		},
	},
	{
		// Prepend strings
		in1: &struct{ S string }{
			S: "string1",
		},
		in2: &struct{ S string }{
			S: "string2",
		},
		out: &struct{ S string }{
			S: "string2string1",
		},
		prepend: true,
	},
	{
		// Append pointer to bool
		in1: &struct{ B1, B2, B3, B4, B5, B6, B7, B8, B9 *bool }{
			B1: BoolPtr(true),
			B2: BoolPtr(false),
			B3: nil,
			B4: BoolPtr(true),
			B5: BoolPtr(false),
			B6: nil,
			B7: BoolPtr(true),
			B8: BoolPtr(false),
			B9: nil,
		},
		in2: &struct{ B1, B2, B3, B4, B5, B6, B7, B8, B9 *bool }{
			B1: nil,
			B2: nil,
			B3: nil,
			B4: BoolPtr(true),
			B5: BoolPtr(true),
			B6: BoolPtr(true),
			B7: BoolPtr(false),
			B8: BoolPtr(false),
			B9: BoolPtr(false),
		},
		out: &struct{ B1, B2, B3, B4, B5, B6, B7, B8, B9 *bool }{
			B1: BoolPtr(true),
			B2: BoolPtr(false),
			B3: nil,
			B4: BoolPtr(true),
			B5: BoolPtr(true),
			B6: BoolPtr(true),
			B7: BoolPtr(false),
			B8: BoolPtr(false),
			B9: BoolPtr(false),
		},
	},
	{
		// Prepend pointer to bool
		in1: &struct{ B1, B2, B3, B4, B5, B6, B7, B8, B9 *bool }{
			B1: BoolPtr(true),
			B2: BoolPtr(false),
			B3: nil,
			B4: BoolPtr(true),
			B5: BoolPtr(false),
			B6: nil,
			B7: BoolPtr(true),
			B8: BoolPtr(false),
			B9: nil,
		},
		in2: &struct{ B1, B2, B3, B4, B5, B6, B7, B8, B9 *bool }{
			B1: nil,
			B2: nil,
			B3: nil,
			B4: BoolPtr(true),
			B5: BoolPtr(true),
			B6: BoolPtr(true),
			B7: BoolPtr(false),
			B8: BoolPtr(false),
			B9: BoolPtr(false),
		},
		out: &struct{ B1, B2, B3, B4, B5, B6, B7, B8, B9 *bool }{
			B1: BoolPtr(true),
			B2: BoolPtr(false),
			B3: nil,
			B4: BoolPtr(true),
			B5: BoolPtr(false),
			B6: BoolPtr(true),
			B7: BoolPtr(true),
			B8: BoolPtr(false),
			B9: BoolPtr(false),
		},
		prepend: true,
	},
	{
		// Append pointer to strings
		in1: &struct{ S1, S2, S3, S4 *string }{
			S1: StringPtr("string1"),
			S2: StringPtr("string2"),
		},
		in2: &struct{ S1, S2, S3, S4 *string }{
			S1: StringPtr("string3"),
			S3: StringPtr("string4"),
		},
		out: &struct{ S1, S2, S3, S4 *string }{
			S1: StringPtr("string3"),
			S2: StringPtr("string2"),
			S3: StringPtr("string4"),
			S4: nil,
		},
	},
	{
		// Prepend pointer to strings
		in1: &struct{ S1, S2, S3, S4 *string }{
			S1: StringPtr("string1"),
			S2: StringPtr("string2"),
		},
		in2: &struct{ S1, S2, S3, S4 *string }{
			S1: StringPtr("string3"),
			S3: StringPtr("string4"),
		},
		out: &struct{ S1, S2, S3, S4 *string }{
			S1: StringPtr("string1"),
			S2: StringPtr("string2"),
			S3: StringPtr("string4"),
			S4: nil,
		},
		prepend: true,
	},
	{
		// Append slice
		in1: &struct{ S []string }{
			S: []string{"string1"},
		},
		in2: &struct{ S []string }{
			S: []string{"string2"},
		},
		out: &struct{ S []string }{
			S: []string{"string1", "string2"},
		},
	},
	{
		// Prepend slice
		in1: &struct{ S []string }{
			S: []string{"string1"},
		},
		in2: &struct{ S []string }{
			S: []string{"string2"},
		},
		out: &struct{ S []string }{
			S: []string{"string2", "string1"},
		},
		prepend: true,
	},
	{
		// Append empty slice
		in1: &struct{ S1, S2 []string }{
			S1: []string{"string1"},
			S2: []string{},
		},
		in2: &struct{ S1, S2 []string }{
			S1: []string{},
			S2: []string{"string2"},
		},
		out: &struct{ S1, S2 []string }{
			S1: []string{"string1"},
			S2: []string{"string2"},
		},
	},
	{
		// Prepend empty slice
		in1: &struct{ S1, S2 []string }{
			S1: []string{"string1"},
			S2: []string{},
		},
		in2: &struct{ S1, S2 []string }{
			S1: []string{},
			S2: []string{"string2"},
		},
		out: &struct{ S1, S2 []string }{
			S1: []string{"string1"},
			S2: []string{"string2"},
		},
		prepend: true,
	},
	{
		// Append nil slice
		in1: &struct{ S1, S2, S3 []string }{
			S1: []string{"string1"},
		},
		in2: &struct{ S1, S2, S3 []string }{
			S2: []string{"string2"},
		},
		out: &struct{ S1, S2, S3 []string }{
			S1: []string{"string1"},
			S2: []string{"string2"},
			S3: nil,
		},
	},
	{
		// Prepend nil slice
		in1: &struct{ S1, S2, S3 []string }{
			S1: []string{"string1"},
		},
		in2: &struct{ S1, S2, S3 []string }{
			S2: []string{"string2"},
		},
		out: &struct{ S1, S2, S3 []string }{
			S1: []string{"string1"},
			S2: []string{"string2"},
			S3: nil,
		},
		prepend: true,
	},
	{
		// Append pointer
		in1: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
		in2: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{
				S: "string2",
			},
		},
		out: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{
				S: "string1string2",
			},
		},
	},
	{
		// Prepend pointer
		in1: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
		in2: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{
				S: "string2",
			},
		},
		out: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{
				S: "string2string1",
			},
		},
		prepend: true,
	},
	{
		// Append interface
		in1: &struct{ S interface{} }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
		in2: &struct{ S interface{} }{
			S: &struct{ S string }{
				S: "string2",
			},
		},
		out: &struct{ S interface{} }{
			S: &struct{ S string }{
				S: "string1string2",
			},
		},
	},
	{
		// Prepend interface
		in1: &struct{ S interface{} }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
		in2: &struct{ S interface{} }{
			S: &struct{ S string }{
				S: "string2",
			},
		},
		out: &struct{ S interface{} }{
			S: &struct{ S string }{
				S: "string2string1",
			},
		},
		prepend: true,
	},
	{
		// Unexported field
		in1: &struct{ s string }{
			s: "string1",
		},
		in2: &struct{ s string }{
			s: "string2",
		},
		out: &struct{ s string }{
			s: "string1",
		},
	},
	{
		// Empty struct
		in1: &struct{}{},
		in2: &struct{}{},
		out: &struct{}{},
	},
	{
		// Interface nil
		in1: &struct{ S interface{} }{
			S: nil,
		},
		in2: &struct{ S interface{} }{
			S: nil,
		},
		out: &struct{ S interface{} }{
			S: nil,
		},
	},
	{
		// Pointer nil
		in1: &struct{ S *struct{} }{
			S: nil,
		},
		in2: &struct{ S *struct{} }{
			S: nil,
		},
		out: &struct{ S *struct{} }{
			S: nil,
		},
	},
	{
		// Anonymous struct
		in1: &struct {
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
		in2: &struct {
			EmbeddedStruct
			Nested struct{ EmbeddedStruct }
		}{
			EmbeddedStruct: EmbeddedStruct{
				S: "string3",
			},
			Nested: struct{ EmbeddedStruct }{
				EmbeddedStruct: EmbeddedStruct{
					S: "string4",
				},
			},
		},
		out: &struct {
			EmbeddedStruct
			Nested struct{ EmbeddedStruct }
		}{
			EmbeddedStruct: EmbeddedStruct{
				S: "string1string3",
			},
			Nested: struct{ EmbeddedStruct }{
				EmbeddedStruct: EmbeddedStruct{
					S: "string2string4",
				},
			},
		},
	},
	{
		// Anonymous interface
		in1: &struct {
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
		in2: &struct {
			EmbeddedInterface
			Nested struct{ EmbeddedInterface }
		}{
			EmbeddedInterface: &struct{ S string }{
				S: "string3",
			},
			Nested: struct{ EmbeddedInterface }{
				EmbeddedInterface: &struct{ S string }{
					S: "string4",
				},
			},
		},
		out: &struct {
			EmbeddedInterface
			Nested struct{ EmbeddedInterface }
		}{
			EmbeddedInterface: &struct{ S string }{
				S: "string1string3",
			},
			Nested: struct{ EmbeddedInterface }{
				EmbeddedInterface: &struct{ S string }{
					S: "string2string4",
				},
			},
		},
	},

	// Errors

	{
		// Non-pointer in1
		in1: struct{}{},
		err: errors.New("expected pointer to struct, got struct {}"),
		out: struct{}{},
	},
	{
		// Non-pointer in2
		in1: &struct{}{},
		in2: struct{}{},
		err: errors.New("expected pointer to struct, got struct {}"),
		out: &struct{}{},
	},
	{
		// Non-struct in1
		in1: &[]string{"bad"},
		err: errors.New("expected pointer to struct, got *[]string"),
		out: &[]string{"bad"},
	},
	{
		// Non-struct in2
		in1: &struct{}{},
		in2: &[]string{"bad"},
		err: errors.New("expected pointer to struct, got *[]string"),
		out: &struct{}{},
	},
	{
		// Mismatched types
		in1: &struct{ A string }{
			A: "string1",
		},
		in2: &struct{ B string }{
			B: "string2",
		},
		out: &struct{ A string }{
			A: "string1",
		},
		err: errors.New("expected matching types for dst and src, got *struct { A string } and *struct { B string }"),
	},
	{
		// Unsupported kind
		in1: &struct{ I int }{
			I: 1,
		},
		in2: &struct{ I int }{
			I: 2,
		},
		out: &struct{ I int }{
			I: 1,
		},
		err: extendPropertyErrorf("i", "unsupported kind int"),
	},
	{
		// Interface nilitude mismatch
		in1: &struct{ S interface{} }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
		in2: &struct{ S interface{} }{
			S: nil,
		},
		out: &struct{ S interface{} }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
		err: extendPropertyErrorf("s", "nilitude mismatch"),
	},
	{
		// Interface type mismatch
		in1: &struct{ S interface{} }{
			S: &struct{ A string }{
				A: "string1",
			},
		},
		in2: &struct{ S interface{} }{
			S: &struct{ B string }{
				B: "string2",
			},
		},
		out: &struct{ S interface{} }{
			S: &struct{ A string }{
				A: "string1",
			},
		},
		err: extendPropertyErrorf("s", "mismatched types struct { A string } and struct { B string }"),
	},
	{
		// Interface not a pointer
		in1: &struct{ S interface{} }{
			S: struct{ S string }{
				S: "string1",
			},
		},
		in2: &struct{ S interface{} }{
			S: struct{ S string }{
				S: "string2",
			},
		},
		out: &struct{ S interface{} }{
			S: struct{ S string }{
				S: "string1",
			},
		},
		err: extendPropertyErrorf("s", "interface not a pointer"),
	},
	{
		// Pointer nilitude mismatch
		in1: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
		in2: &struct{ S *struct{ S string } }{
			S: nil,
		},
		out: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{
				S: "string1",
			},
		},
		err: extendPropertyErrorf("s", "nilitude mismatch"),
	},
	{
		// Pointer not a struct
		in1: &struct{ S *[]string }{
			S: &[]string{"string1"},
		},
		in2: &struct{ S *[]string }{
			S: &[]string{"string2"},
		},
		out: &struct{ S *[]string }{
			S: &[]string{"string1"},
		},
		err: extendPropertyErrorf("s", "pointer is a slice"),
	},
	{
		// Error in nested struct
		in1: &struct{ S interface{} }{
			S: &struct{ I int }{
				I: 1,
			},
		},
		in2: &struct{ S interface{} }{
			S: &struct{ I int }{
				I: 2,
			},
		},
		out: &struct{ S interface{} }{
			S: &struct{ I int }{
				I: 1,
			},
		},
		err: extendPropertyErrorf("s.i", "unsupported kind int"),
	},

	// Filters

	{
		// Filter true
		in1: &struct{ S string }{
			S: "string1",
		},
		in2: &struct{ S string }{
			S: "string2",
		},
		out: &struct{ S string }{
			S: "string1string2",
		},
		filter: func(property string,
			dstField, srcField reflect.StructField,
			dstValue, srcValue interface{}) (bool, error) {
			return true, nil
		},
	},
	{
		// Filter false
		in1: &struct{ S string }{
			S: "string1",
		},
		in2: &struct{ S string }{
			S: "string2",
		},
		out: &struct{ S string }{
			S: "string1",
		},
		filter: func(property string,
			dstField, srcField reflect.StructField,
			dstValue, srcValue interface{}) (bool, error) {
			return false, nil
		},
	},
	{
		// Filter check args
		in1: &struct{ S string }{
			S: "string1",
		},
		in2: &struct{ S string }{
			S: "string2",
		},
		out: &struct{ S string }{
			S: "string1string2",
		},
		filter: func(property string,
			dstField, srcField reflect.StructField,
			dstValue, srcValue interface{}) (bool, error) {
			return property == "s" &&
				dstField.Name == "S" && srcField.Name == "S" &&
				dstValue.(string) == "string1" && srcValue.(string) == "string2", nil
		},
	},
	{
		// Filter mutated
		in1: &struct {
			S string `blueprint:"mutated"`
		}{
			S: "string1",
		},
		in2: &struct {
			S string `blueprint:"mutated"`
		}{
			S: "string2",
		},
		out: &struct {
			S string `blueprint:"mutated"`
		}{
			S: "string1",
		},
	},
	{
		// Filter error
		in1: &struct{ S string }{
			S: "string1",
		},
		in2: &struct{ S string }{
			S: "string2",
		},
		out: &struct{ S string }{
			S: "string1",
		},
		filter: func(property string,
			dstField, srcField reflect.StructField,
			dstValue, srcValue interface{}) (bool, error) {
			return true, fmt.Errorf("filter error")
		},
		err: extendPropertyErrorf("s", "filter error"),
	},
}

func TestAppendProperties(t *testing.T) {
	for _, testCase := range appendPropertiesTestCases {
		testString := fmt.Sprintf("%v, %v -> %v", testCase.in1, testCase.in2, testCase.out)

		got := testCase.in1
		var err error
		var testType string

		if testCase.prepend {
			testType = "prepend"
			err = PrependProperties(got, testCase.in2, testCase.filter)
		} else {
			testType = "append"
			err = AppendProperties(got, testCase.in2, testCase.filter)
		}

		check(t, testType, testString, got, err, testCase.out, testCase.err)
	}
}

var appendMatchingPropertiesTestCases = []struct {
	in1     []interface{}
	in2     interface{}
	out     []interface{}
	prepend bool
	filter  ExtendPropertyFilterFunc
	err     error
}{
	{
		// Append strings
		in1: []interface{}{&struct{ S string }{
			S: "string1",
		}},
		in2: &struct{ S string }{
			S: "string2",
		},
		out: []interface{}{&struct{ S string }{
			S: "string1string2",
		}},
	},
	{
		// Prepend strings
		in1: []interface{}{&struct{ S string }{
			S: "string1",
		}},
		in2: &struct{ S string }{
			S: "string2",
		},
		out: []interface{}{&struct{ S string }{
			S: "string2string1",
		}},
		prepend: true,
	},
	{
		// Append all
		in1: []interface{}{
			&struct{ S, A string }{
				S: "string1",
			},
			&struct{ S, B string }{
				S: "string2",
			},
		},
		in2: &struct{ S string }{
			S: "string3",
		},
		out: []interface{}{
			&struct{ S, A string }{
				S: "string1string3",
			},
			&struct{ S, B string }{
				S: "string2string3",
			},
		},
	},
	{
		// Append some
		in1: []interface{}{
			&struct{ S, A string }{
				S: "string1",
			},
			&struct{ B string }{},
		},
		in2: &struct{ S string }{
			S: "string2",
		},
		out: []interface{}{
			&struct{ S, A string }{
				S: "string1string2",
			},
			&struct{ B string }{},
		},
	},
	{
		// Append mismatched structs
		in1: []interface{}{&struct{ S, A string }{
			S: "string1",
		}},
		in2: &struct{ S string }{
			S: "string2",
		},
		out: []interface{}{&struct{ S, A string }{
			S: "string1string2",
		}},
	},
	{
		// Append mismatched pointer structs
		in1: []interface{}{&struct{ S *struct{ S, A string } }{
			S: &struct{ S, A string }{
				S: "string1",
			},
		}},
		in2: &struct{ S *struct{ S string } }{
			S: &struct{ S string }{
				S: "string2",
			},
		},
		out: []interface{}{&struct{ S *struct{ S, A string } }{
			S: &struct{ S, A string }{
				S: "string1string2",
			},
		}},
	},

	// Errors

	{
		// Non-pointer in1
		in1: []interface{}{struct{}{}},
		err: errors.New("expected pointer to struct, got struct {}"),
		out: []interface{}{struct{}{}},
	},
	{
		// Non-pointer in2
		in1: []interface{}{&struct{}{}},
		in2: struct{}{},
		err: errors.New("expected pointer to struct, got struct {}"),
		out: []interface{}{&struct{}{}},
	},
	{
		// Non-struct in1
		in1: []interface{}{&[]string{"bad"}},
		err: errors.New("expected pointer to struct, got *[]string"),
		out: []interface{}{&[]string{"bad"}},
	},
	{
		// Non-struct in2
		in1: []interface{}{&struct{}{}},
		in2: &[]string{"bad"},
		err: errors.New("expected pointer to struct, got *[]string"),
		out: []interface{}{&struct{}{}},
	},
	{
		// Append none
		in1: []interface{}{
			&struct{ A string }{},
			&struct{ B string }{},
		},
		in2: &struct{ S string }{
			S: "string1",
		},
		out: []interface{}{
			&struct{ A string }{},
			&struct{ B string }{},
		},
		err: extendPropertyErrorf("s", "failed to find property to extend"),
	},
	{
		// Append mismatched kinds
		in1: []interface{}{
			&struct{ S string }{
				S: "string1",
			},
		},
		in2: &struct{ S []string }{
			S: []string{"string2"},
		},
		out: []interface{}{
			&struct{ S string }{
				S: "string1",
			},
		},
		err: extendPropertyErrorf("s", "mismatched types string and []string"),
	},
	{
		// Append mismatched types
		in1: []interface{}{
			&struct{ S []int }{
				S: []int{1},
			},
		},
		in2: &struct{ S []string }{
			S: []string{"string2"},
		},
		out: []interface{}{
			&struct{ S []int }{
				S: []int{1},
			},
		},
		err: extendPropertyErrorf("s", "mismatched types []int and []string"),
	},
}

func TestAppendMatchingProperties(t *testing.T) {
	for _, testCase := range appendMatchingPropertiesTestCases {
		testString := fmt.Sprintf("%s, %s -> %s", p(testCase.in1), p(testCase.in2), p(testCase.out))

		got := testCase.in1
		var err error
		var testType string

		if testCase.prepend {
			testType = "prepend matching"
			err = PrependMatchingProperties(got, testCase.in2, testCase.filter)
		} else {
			testType = "append matching"
			err = AppendMatchingProperties(got, testCase.in2, testCase.filter)
		}

		check(t, testType, testString, got, err, testCase.out, testCase.err)
	}
}

func check(t *testing.T, testType, testString string,
	got interface{}, err error,
	expected interface{}, expectedErr error) {

	printedTestCase := false
	e := func(s string, expected, got interface{}) {
		if !printedTestCase {
			t.Errorf("test case %s: %s", testType, testString)
			printedTestCase = true
		}
		t.Errorf("incorrect %s", s)
		t.Errorf("  expected: %s", p(expected))
		t.Errorf("       got: %s", p(got))
	}

	if err != nil {
		if expectedErr != nil {
			if err.Error() != expectedErr.Error() {
				e("unexpected error", expectedErr.Error(), err.Error())
			}
		} else {
			e("unexpected error", nil, err.Error())
		}
	} else {
		if expectedErr != nil {
			e("missing error", expectedErr, nil)
		}
	}

	if !reflect.DeepEqual(expected, got) {
		e("output:", expected, got)
	}
}

func p(in interface{}) string {
	if v, ok := in.([]interface{}); ok {
		s := make([]string, len(v))
		for i := range v {
			s[i] = fmt.Sprintf("%#v", v[i])
		}
		return "[" + strings.Join(s, ", ") + "]"
	} else {
		return fmt.Sprintf("%#v", in)
	}
}
