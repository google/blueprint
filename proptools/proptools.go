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
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PropertyNameForField converts the name of a field in property struct to the property name that
// might appear in a Blueprints file.  Since the property struct fields must always be exported
// to be accessed with reflection and the canonical Blueprints style is lowercased names, it
// lower cases the first rune in the field name unless the field name contains an uppercase rune
// after the first rune (which is always uppercase), and no lowercase runes.
func PropertyNameForField(fieldName string) string {
	r, size := utf8.DecodeRuneInString(fieldName)
	propertyName := string(unicode.ToLower(r))
	if size == len(fieldName) {
		return propertyName
	}
	if strings.IndexFunc(fieldName[size:], unicode.IsLower) == -1 &&
		strings.IndexFunc(fieldName[size:], unicode.IsUpper) != -1 {
		return fieldName
	}
	if len(fieldName) > size {
		propertyName += fieldName[size:]
	}
	return propertyName
}

// FieldNameForProperty converts the name of a property that might appear in a Blueprints file to
// the name of a field in property struct by uppercasing the first rune.
func FieldNameForProperty(propertyName string) string {
	r, size := utf8.DecodeRuneInString(propertyName)
	fieldName := string(unicode.ToUpper(r))
	if len(propertyName) > size {
		fieldName += propertyName[size:]
	}
	return fieldName
}

// BoolPtr returns a pointer to a new bool containing the given value.
func BoolPtr(b bool) *bool {
	return &b
}

// Int64Ptr returns a pointer to a new int64 containing the given value.
func Int64Ptr(i int64) *int64 {
	b := int64(i)
	return &(b)
}

// StringPtr returns a pointer to a new string containing the given value.
func StringPtr(s string) *string {
	return &s
}

// BoolDefault takes a pointer to a bool and returns the value pointed to by the pointer if it is non-nil,
// or def if the pointer is nil.
func BoolDefault(b *bool, def bool) bool {
	if b != nil {
		return *b
	}
	return def
}

// Bool takes a pointer to a bool and returns true iff the pointer is non-nil and points to a true
// value.
func Bool(b *bool) bool {
	return BoolDefault(b, false)
}

// String takes a pointer to a string and returns the value of the string if the pointer is non-nil,
// or def if the pointer is nil.
func StringDefault(s *string, def string) string {
	if s != nil {
		return *s
	}
	return def
}

// String takes a pointer to a string and returns the value of the string if the pointer is non-nil,
// or an empty string.
func String(s *string) string {
	return StringDefault(s, "")
}

// IntDefault takes a pointer to an int64 and returns the value pointed to by the pointer cast to int
// if it is non-nil, or def if the pointer is nil.
func IntDefault(i *int64, def int) int {
	if i != nil {
		return int(*i)
	}
	return def
}

// Int takes a pointer to an int64 and returns the value pointed to by the pointer cast to int
// if it is non-nil, or 0 if the pointer is nil.
func Int(i *int64) int {
	return IntDefault(i, 0)
}

func isStruct(t reflect.Type) bool {
	return t.Kind() == reflect.Struct
}

func isStructPtr(t reflect.Type) bool {
	return t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct
}

func isSlice(t reflect.Type) bool {
	return t.Kind() == reflect.Slice
}

func isSliceOfStruct(t reflect.Type) bool {
	return isSlice(t) && isStruct(t.Elem())
}
