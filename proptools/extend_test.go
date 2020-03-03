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

type appendPropertyTestCase struct {
	in1    interface{}
	in2    interface{}
	out    interface{}
	order  Order // default is Append
	filter ExtendPropertyFilterFunc
	err    error
}

func appendPropertiesTestCases() []appendPropertyTestCase {
	return []appendPropertyTestCase{
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
			order: Prepend,
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
			order: Prepend,
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
			order: Prepend,
		},
		{
			// Append pointer to integer
			in1: &struct{ I1, I2, I3, I4, I5, I6, I7, I8, I9 *int64 }{
				I1: Int64Ptr(55),
				I2: Int64Ptr(-3),
				I3: nil,
				I4: Int64Ptr(100),
				I5: Int64Ptr(33),
				I6: nil,
				I7: Int64Ptr(77),
				I8: Int64Ptr(0),
				I9: nil,
			},
			in2: &struct{ I1, I2, I3, I4, I5, I6, I7, I8, I9 *int64 }{
				I1: nil,
				I2: nil,
				I3: nil,
				I4: Int64Ptr(1),
				I5: Int64Ptr(-2),
				I6: Int64Ptr(8),
				I7: Int64Ptr(9),
				I8: Int64Ptr(10),
				I9: Int64Ptr(11),
			},
			out: &struct{ I1, I2, I3, I4, I5, I6, I7, I8, I9 *int64 }{
				I1: Int64Ptr(55),
				I2: Int64Ptr(-3),
				I3: nil,
				I4: Int64Ptr(1),
				I5: Int64Ptr(-2),
				I6: Int64Ptr(8),
				I7: Int64Ptr(9),
				I8: Int64Ptr(10),
				I9: Int64Ptr(11),
			},
		},
		{
			// Prepend pointer to integer
			in1: &struct{ I1, I2, I3 *int64 }{
				I1: Int64Ptr(55),
				I3: nil,
			},
			in2: &struct{ I1, I2, I3 *int64 }{
				I2: Int64Ptr(33),
			},
			out: &struct{ I1, I2, I3 *int64 }{
				I1: Int64Ptr(55),
				I2: Int64Ptr(33),
				I3: nil,
			},
			order: Prepend,
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
			order: Prepend,
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
			order: Prepend,
		},
		{
			// Replace slice
			in1: &struct{ S []string }{
				S: []string{"string1"},
			},
			in2: &struct{ S []string }{
				S: []string{"string2"},
			},
			out: &struct{ S []string }{
				S: []string{"string2"},
			},
			order: Replace,
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
			order: Prepend,
		},
		{
			// Replace empty slice
			in1: &struct{ S1, S2 []string }{
				S1: []string{"string1"},
				S2: []string{},
			},
			in2: &struct{ S1, S2 []string }{
				S1: []string{},
				S2: []string{"string2"},
			},
			out: &struct{ S1, S2 []string }{
				S1: []string{},
				S2: []string{"string2"},
			},
			order: Replace,
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
			order: Prepend,
		},
		{
			// Replace nil slice
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
			order: Replace,
		},
		{
			// Replace embedded slice
			in1: &struct{ S *struct{ S1 []string } }{
				S: &struct{ S1 []string }{
					S1: []string{"string1"},
				},
			},
			in2: &struct{ S *struct{ S1 []string } }{
				S: &struct{ S1 []string }{
					S1: []string{"string2"},
				},
			},
			out: &struct{ S *struct{ S1 []string } }{
				S: &struct{ S1 []string }{
					S1: []string{"string2"},
				},
			},
			order: Replace,
		},
		{
			// Append slice of structs
			in1: &struct{ S []struct{ F string } }{
				S: []struct{ F string }{
					{F: "foo"}, {F: "bar"},
				},
			},
			in2: &struct{ S []struct{ F string } }{
				S: []struct{ F string }{
					{F: "baz"},
				},
			},
			out: &struct{ S []struct{ F string } }{
				S: []struct{ F string }{
					{F: "foo"}, {F: "bar"}, {F: "baz"},
				},
			},
			order: Append,
		},
		{
			// Prepend slice of structs
			in1: &struct{ S []struct{ F string } }{
				S: []struct{ F string }{
					{F: "foo"}, {F: "bar"},
				},
			},
			in2: &struct{ S []struct{ F string } }{
				S: []struct{ F string }{
					{F: "baz"},
				},
			},
			out: &struct{ S []struct{ F string } }{
				S: []struct{ F string }{
					{F: "baz"}, {F: "foo"}, {F: "bar"},
				},
			},
			order: Prepend,
		},
		{
			// Replace slice of structs
			in1: &struct{ S []struct{ F string } }{
				S: []struct{ F string }{
					{F: "foo"}, {F: "bar"},
				},
			},
			in2: &struct{ S []struct{ F string } }{
				S: []struct{ F string }{
					{F: "baz"},
				},
			},
			out: &struct{ S []struct{ F string } }{
				S: []struct{ F string }{
					{F: "baz"},
				},
			},
			order: Replace,
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
			order: Prepend,
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
			order: Prepend,
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
			// Unexported field
			in1: &struct{ i *int64 }{
				i: Int64Ptr(33),
			},
			in2: &struct{ i *int64 }{
				i: Int64Ptr(5),
			},
			out: &struct{ i *int64 }{
				i: Int64Ptr(33),
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
					I: Int64Ptr(55),
				},
				Nested: struct{ EmbeddedStruct }{
					EmbeddedStruct: EmbeddedStruct{
						S: "string2",
						I: Int64Ptr(-4),
					},
				},
			},
			in2: &struct {
				EmbeddedStruct
				Nested struct{ EmbeddedStruct }
			}{
				EmbeddedStruct: EmbeddedStruct{
					S: "string3",
					I: Int64Ptr(66),
				},
				Nested: struct{ EmbeddedStruct }{
					EmbeddedStruct: EmbeddedStruct{
						S: "string4",
						I: Int64Ptr(-8),
					},
				},
			},
			out: &struct {
				EmbeddedStruct
				Nested struct{ EmbeddedStruct }
			}{
				EmbeddedStruct: EmbeddedStruct{
					S: "string1string3",
					I: Int64Ptr(66),
				},
				Nested: struct{ EmbeddedStruct }{
					EmbeddedStruct: EmbeddedStruct{
						S: "string2string4",
						I: Int64Ptr(-8),
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
				EmbeddedInterface: &struct {
					S string
					I *int64
				}{
					S: "string1",
					I: Int64Ptr(-8),
				},
				Nested: struct{ EmbeddedInterface }{
					EmbeddedInterface: &struct {
						S string
						I *int64
					}{
						S: "string2",
						I: Int64Ptr(55),
					},
				},
			},
			in2: &struct {
				EmbeddedInterface
				Nested struct{ EmbeddedInterface }
			}{
				EmbeddedInterface: &struct {
					S string
					I *int64
				}{
					S: "string3",
					I: Int64Ptr(6),
				},
				Nested: struct{ EmbeddedInterface }{
					EmbeddedInterface: &struct {
						S string
						I *int64
					}{
						S: "string4",
						I: Int64Ptr(6),
					},
				},
			},
			out: &struct {
				EmbeddedInterface
				Nested struct{ EmbeddedInterface }
			}{
				EmbeddedInterface: &struct {
					S string
					I *int64
				}{
					S: "string1string3",
					I: Int64Ptr(6),
				},
				Nested: struct{ EmbeddedInterface }{
					EmbeddedInterface: &struct {
						S string
						I *int64
					}{
						S: "string2string4",
						I: Int64Ptr(6),
					},
				},
			},
		},
		{
			// Nil pointer to a struct
			in1: &struct {
				Nested *struct {
					S string
				}
			}{},
			in2: &struct {
				Nested *struct {
					S string
				}
			}{
				Nested: &struct {
					S string
				}{
					S: "string",
				},
			},
			out: &struct {
				Nested *struct {
					S string
				}
			}{
				Nested: &struct {
					S string
				}{
					S: "string",
				},
			},
		},
		{
			// Nil pointer to a struct in an interface
			in1: &struct {
				Nested interface{}
			}{
				Nested: (*struct{ S string })(nil),
			},
			in2: &struct {
				Nested interface{}
			}{
				Nested: &struct {
					S string
				}{
					S: "string",
				},
			},
			out: &struct {
				Nested interface{}
			}{
				Nested: &struct {
					S string
				}{
					S: "string",
				},
			},
		},
		{
			// Interface src nil
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
		},

		// Errors

		{
			// Non-pointer in1
			in1: struct{}{},
			in2: &struct{}{},
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
			in2: &struct{}{},
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
			// Unsupported kind
			in1: &struct{ I int64 }{
				I: 1,
			},
			in2: &struct{ I int64 }{
				I: 2,
			},
			out: &struct{ I int64 }{
				I: 1,
			},
			err: extendPropertyErrorf("i", "unsupported kind int64"),
		},
		{
			// Interface nilitude mismatch
			in1: &struct{ S interface{} }{
				S: nil,
			},
			in2: &struct{ S interface{} }{
				S: &struct{ S string }{
					S: "string1",
				},
			},
			out: &struct{ S interface{} }{
				S: nil,
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
			// Filter mutated
			in1: &struct {
				S *int64 `blueprint:"mutated"`
			}{
				S: Int64Ptr(4),
			},
			in2: &struct {
				S *int64 `blueprint:"mutated"`
			}{
				S: Int64Ptr(5),
			},
			out: &struct {
				S *int64 `blueprint:"mutated"`
			}{
				S: Int64Ptr(4),
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
}

func TestAppendProperties(t *testing.T) {
	for _, testCase := range appendPropertiesTestCases() {
		testString := fmt.Sprintf("%v, %v -> %v", testCase.in1, testCase.in2, testCase.out)

		got := testCase.in1
		var err error
		var testType string

		switch testCase.order {
		case Append:
			testType = "append"
			err = AppendProperties(got, testCase.in2, testCase.filter)
		case Prepend:
			testType = "prepend"
			err = PrependProperties(got, testCase.in2, testCase.filter)
		case Replace:
			testType = "replace"
			err = ExtendProperties(got, testCase.in2, testCase.filter, OrderReplace)
		}

		check(t, testType, testString, got, err, testCase.out, testCase.err)
	}
}

func TestExtendProperties(t *testing.T) {
	for _, testCase := range appendPropertiesTestCases() {
		testString := fmt.Sprintf("%v, %v -> %v", testCase.in1, testCase.in2, testCase.out)

		got := testCase.in1
		var err error
		var testType string

		order := func(property string,
			dstField, srcField reflect.StructField,
			dstValue, srcValue interface{}) (Order, error) {
			switch testCase.order {
			case Append:
				return Append, nil
			case Prepend:
				return Prepend, nil
			case Replace:
				return Replace, nil
			}
			return Append, errors.New("unknown order")
		}

		switch testCase.order {
		case Append:
			testType = "prepend"
		case Prepend:
			testType = "append"
		case Replace:
			testType = "replace"
		}

		err = ExtendProperties(got, testCase.in2, testCase.filter, order)

		check(t, testType, testString, got, err, testCase.out, testCase.err)
	}
}

type appendMatchingPropertiesTestCase struct {
	in1    []interface{}
	in2    interface{}
	out    []interface{}
	order  Order // default is Append
	filter ExtendPropertyFilterFunc
	err    error
}

func appendMatchingPropertiesTestCases() []appendMatchingPropertiesTestCase {
	return []appendMatchingPropertiesTestCase{
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
			order: Prepend,
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
		{
			// Append through mismatched types
			in1: []interface{}{
				&struct{ B string }{},
				&struct{ S interface{} }{
					S: &struct{ S, A string }{
						S: "string1",
					},
				},
			},
			in2: &struct{ S struct{ S string } }{
				S: struct{ S string }{
					S: "string2",
				},
			},
			out: []interface{}{
				&struct{ B string }{},
				&struct{ S interface{} }{
					S: &struct{ S, A string }{
						S: "string1string2",
					},
				},
			},
		},
		{
			// Append through mismatched types and nil
			in1: []interface{}{
				&struct{ B string }{},
				&struct{ S interface{} }{
					S: (*struct{ S, A string })(nil),
				},
			},
			in2: &struct{ S struct{ S string } }{
				S: struct{ S string }{
					S: "string2",
				},
			},
			out: []interface{}{
				&struct{ B string }{},
				&struct{ S interface{} }{
					S: &struct{ S, A string }{
						S: "string2",
					},
				},
			},
		},
		{
			// Append through multiple matches
			in1: []interface{}{
				&struct {
					S struct{ S, A string }
				}{
					S: struct{ S, A string }{
						S: "string1",
					},
				},
				&struct {
					S struct{ S, B string }
				}{
					S: struct{ S, B string }{
						S: "string2",
					},
				},
			},
			in2: &struct{ S struct{ B string } }{
				S: struct{ B string }{
					B: "string3",
				},
			},
			out: []interface{}{
				&struct {
					S struct{ S, A string }
				}{
					S: struct{ S, A string }{
						S: "string1",
					},
				},
				&struct {
					S struct{ S, B string }
				}{
					S: struct{ S, B string }{
						S: "string2",
						B: "string3",
					},
				},
			},
		},

		// Errors

		{
			// Non-pointer in1
			in1: []interface{}{struct{}{}},
			in2: &struct{}{},
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
			in2: &struct{}{},
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
}

func TestAppendMatchingProperties(t *testing.T) {
	for _, testCase := range appendMatchingPropertiesTestCases() {
		testString := fmt.Sprintf("%s, %s -> %s", p(testCase.in1), p(testCase.in2), p(testCase.out))

		got := testCase.in1
		var err error
		var testType string

		switch testCase.order {
		case Append:
			testType = "append"
			err = AppendMatchingProperties(got, testCase.in2, testCase.filter)
		case Prepend:
			testType = "prepend"
			err = PrependMatchingProperties(got, testCase.in2, testCase.filter)
		case Replace:
			testType = "replace"
			err = ExtendMatchingProperties(got, testCase.in2, testCase.filter, OrderReplace)
		}

		check(t, testType, testString, got, err, testCase.out, testCase.err)
	}
}

func TestExtendMatchingProperties(t *testing.T) {
	for _, testCase := range appendMatchingPropertiesTestCases() {
		testString := fmt.Sprintf("%s, %s -> %s", p(testCase.in1), p(testCase.in2), p(testCase.out))

		got := testCase.in1
		var err error
		var testType string

		order := func(property string,
			dstField, srcField reflect.StructField,
			dstValue, srcValue interface{}) (Order, error) {
			switch testCase.order {
			case Append:
				return Append, nil
			case Prepend:
				return Prepend, nil
			case Replace:
				return Replace, nil
			}
			return Append, errors.New("unknown order")
		}

		switch testCase.order {
		case Append:
			testType = "prepend matching"
		case Prepend:
			testType = "append matching"
		case Replace:
			testType = "replace matching"
		}

		err = ExtendMatchingProperties(got, testCase.in2, testCase.filter, order)

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
