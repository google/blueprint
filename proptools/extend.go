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
)

// AppendProperties appends the values of properties in the property struct src to the property
// struct dst. dst and src must be the same type, and both must be pointers to structs.
//
// The filter function can prevent individual properties from being appended by returning false, or
// abort AppendProperties with an error by returning an error.  Passing nil for filter will append
// all properties.
//
// An error returned by AppendProperties that applies to a specific property will be an
// *ExtendPropertyError, and can have the property name and error extracted from it.
//
// The append operation is defined as appending string and slices of strings normally, replacing
// bool values, and recursing into embedded structs, pointers to structs, and interfaces containing
// pointers to structs.
func AppendProperties(dst interface{}, src interface{}, filter ExtendPropertyFilterFunc) error {
	return extendProperties(dst, src, filter, false)
}

// PrependProperties prepends the values of properties in the property struct src to the property
// struct dst. dst and src must be the same type, and both must be pointers to structs.
//
// The filter function can prevent individual properties from being prepended by returning false, or
// abort PrependProperties with an error by returning an error.  Passing nil for filter will prepend
// all properties.
//
// An error returned by PrependProperties that applies to a specific property will be an
// *ExtendPropertyError, and can have the property name and error extracted from it.
//
// The prepend operation is defined as appending string and slices of strings normally, replacing
// false bool values with true, and recursing into embedded structs, pointers to structs, and
// interfaces containing pointers to structs.
func PrependProperties(dst interface{}, src interface{}, filter ExtendPropertyFilterFunc) error {
	return extendProperties(dst, src, filter, true)
}

// AppendMatchingProperties appends the values of properties in the property struct src to the
// property structs in dst.  dst and src do not have to be the same type, but every property in src
// must be found in at least one property in dst.  dst must be a slice of pointers to structs, and
// src must be a pointer to a struct.
//
// The filter function can prevent individual properties from being appended by returning false, or
// abort AppendProperties with an error by returning an error.  Passing nil for filter will append
// all properties.
//
// An error returned by AppendMatchingProperties that applies to a specific property will be an
// *ExtendPropertyError, and can have the property name and error extracted from it.
//
// The append operation is defined as appending string and slices of strings normally, replacing
// bool values, and recursing into embedded structs, pointers to structs, and interfaces containing
// pointers to structs.
func AppendMatchingProperties(dst []interface{}, src interface{},
	filter ExtendPropertyFilterFunc) error {
	return extendMatchingProperties(dst, src, filter, false)
}

// PrependMatchingProperties prepends the values of properties in the property struct src to the
// property structs in dst.  dst and src do not have to be the same type, but every property in src
// must be found in at least one property in dst.  dst must be a slice of pointers to structs, and
// src must be a pointer to a struct.
//
// The filter function can prevent individual properties from being prepended by returning false, or
// abort PrependProperties with an error by returning an error.  Passing nil for filter will prepend
// all properties.
//
// An error returned by PrependProperties that applies to a specific property will be an
// *ExtendPropertyError, and can have the property name and error extracted from it.
//
// The prepend operation is defined as appending string and slices of strings normally, replacing
// false bool values with true, and recursing into embedded structs, pointers to structs, and
// interfaces containing pointers to structs.
func PrependMatchingProperties(dst []interface{}, src interface{},
	filter ExtendPropertyFilterFunc) error {
	return extendMatchingProperties(dst, src, filter, true)
}

type ExtendPropertyFilterFunc func(property string,
	dstField, srcField reflect.StructField,
	dstValue, srcValue interface{}) (bool, error)

type ExtendPropertyError struct {
	Err      error
	Property string
}

func (e *ExtendPropertyError) Error() string {
	return fmt.Sprintf("can't extend property %q: %s", e.Property, e.Err)
}

func extendPropertyErrorf(property string, format string, a ...interface{}) *ExtendPropertyError {
	return &ExtendPropertyError{
		Err:      fmt.Errorf(format, a...),
		Property: property,
	}
}

func extendProperties(dst interface{}, src interface{}, filter ExtendPropertyFilterFunc,
	prepend bool) error {

	dstValue, err := getStruct(dst)
	if err != nil {
		return err
	}
	srcValue, err := getStruct(src)
	if err != nil {
		return err
	}

	if dstValue.Type() != srcValue.Type() {
		return fmt.Errorf("expected matching types for dst and src, got %T and %T", dst, src)
	}

	dstValues := []reflect.Value{dstValue}

	return extendPropertiesRecursive(dstValues, srcValue, "", filter, true, prepend)
}

func extendMatchingProperties(dst []interface{}, src interface{}, filter ExtendPropertyFilterFunc,
	prepend bool) error {

	dstValues := make([]reflect.Value, len(dst))
	for i := range dst {
		var err error
		dstValues[i], err = getStruct(dst[i])
		if err != nil {
			return err
		}
	}

	srcValue, err := getStruct(src)
	if err != nil {
		return err
	}

	return extendPropertiesRecursive(dstValues, srcValue, "", filter, false, prepend)
}

func extendPropertiesRecursive(dstValues []reflect.Value, srcValue reflect.Value,
	prefix string, filter ExtendPropertyFilterFunc, sameTypes, prepend bool) error {

	srcType := srcValue.Type()
	for i := 0; i < srcValue.NumField(); i++ {
		srcField := srcType.Field(i)
		if srcField.PkgPath != "" {
			// The field is not exported so just skip it.
			continue
		}
		if HasTag(srcField, "blueprint", "mutated") {
			continue
		}

		propertyName := prefix + PropertyNameForField(srcField.Name)
		srcFieldValue := srcValue.Field(i)

		found := false
		for _, dstValue := range dstValues {
			dstType := dstValue.Type()
			var dstField reflect.StructField

			if dstType == srcType {
				dstField = dstType.Field(i)
			} else {
				var ok bool
				dstField, ok = dstType.FieldByName(srcField.Name)
				if !ok {
					continue
				}
			}

			found = true

			dstFieldValue := dstValue.FieldByIndex(dstField.Index)

			if srcFieldValue.Kind() != dstFieldValue.Kind() {
				return extendPropertyErrorf(propertyName, "mismatched types %s and %s",
					dstFieldValue.Type(), srcFieldValue.Type())
			}

			switch srcFieldValue.Kind() {
			case reflect.Interface:
				if dstFieldValue.IsNil() != srcFieldValue.IsNil() {
					return extendPropertyErrorf(propertyName, "nilitude mismatch")
				}
				if dstFieldValue.IsNil() {
					continue
				}

				dstFieldValue = dstFieldValue.Elem()
				srcFieldValue = srcFieldValue.Elem()

				if srcFieldValue.Kind() != reflect.Ptr || dstFieldValue.Kind() != reflect.Ptr {
					return extendPropertyErrorf(propertyName, "interface not a pointer")
				}

				fallthrough
			case reflect.Ptr:
				if dstFieldValue.IsNil() != srcFieldValue.IsNil() {
					return extendPropertyErrorf(propertyName, "nilitude mismatch")
				}
				if dstFieldValue.IsNil() {
					continue
				}

				dstFieldValue = dstFieldValue.Elem()
				srcFieldValue = srcFieldValue.Elem()

				if srcFieldValue.Kind() != reflect.Struct || dstFieldValue.Kind() != reflect.Struct {
					return extendPropertyErrorf(propertyName, "pointer not to a struct")
				}

				fallthrough
			case reflect.Struct:
				if sameTypes && dstFieldValue.Type() != srcFieldValue.Type() {
					return extendPropertyErrorf(propertyName, "mismatched types %s and %s",
						dstFieldValue.Type(), srcFieldValue.Type())
				}

				// Recursively extend the struct's fields.
				err := extendPropertiesRecursive([]reflect.Value{dstFieldValue}, srcFieldValue,
					propertyName+".", filter, sameTypes, prepend)
				if err != nil {
					return err
				}
				continue
			case reflect.Bool, reflect.String, reflect.Slice:
				if srcFieldValue.Type() != dstFieldValue.Type() {
					return extendPropertyErrorf(propertyName, "mismatched types %s and %s",
						dstFieldValue.Type(), srcFieldValue.Type())
				}
			default:
				return extendPropertyErrorf(propertyName, "unsupported kind %s",
					srcFieldValue.Kind())
			}

			if filter != nil {
				b, err := filter(propertyName, dstField, srcField,
					dstFieldValue.Interface(), srcFieldValue.Interface())
				if err != nil {
					return &ExtendPropertyError{
						Property: propertyName,
						Err:      err,
					}
				}
				if !b {
					continue
				}
			}

			switch srcFieldValue.Kind() {
			case reflect.Bool:
				if prepend {
					// For prepend, only transition false -> true.
					if srcFieldValue.Bool() {
						dstFieldValue.Set(srcFieldValue)
					}
				} else {
					// For append, replace the original value.
					dstFieldValue.Set(srcFieldValue)
				}
			case reflect.String:
				// Append the extension string.
				if prepend {
					dstFieldValue.SetString(srcFieldValue.String() +
						dstFieldValue.String())
				} else {
					dstFieldValue.SetString(dstFieldValue.String() +
						srcFieldValue.String())
				}
			case reflect.Slice:
				newSlice := reflect.MakeSlice(srcFieldValue.Type(), 0,
					dstFieldValue.Len()+srcFieldValue.Len())
				if prepend {
					newSlice = reflect.AppendSlice(newSlice, srcFieldValue)
					newSlice = reflect.AppendSlice(newSlice, dstFieldValue)
				} else {
					newSlice = reflect.AppendSlice(newSlice, dstFieldValue)
					newSlice = reflect.AppendSlice(newSlice, srcFieldValue)
				}
				dstFieldValue.Set(newSlice)
			}
		}
		if !found {
			return extendPropertyErrorf(propertyName, "failed to find property to extend")
		}
	}

	return nil
}

func getStruct(in interface{}) (reflect.Value, error) {
	value := reflect.ValueOf(in)
	if value.Kind() != reflect.Ptr {
		return reflect.Value{}, fmt.Errorf("expected pointer to struct, got %T", in)
	}
	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("expected pointer to struct, got %T", in)
	}
	return value, nil
}
