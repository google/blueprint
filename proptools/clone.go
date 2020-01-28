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
	"fmt"
	"reflect"
	"sync"
)

// CloneProperties takes a reflect.Value of a pointer to a struct and returns a reflect.Value
// of a pointer to a new struct that copies of the values for its fields.  It recursively clones
// struct pointers and interfaces that contain struct pointers.
func CloneProperties(structValue reflect.Value) reflect.Value {
	if !isStructPtr(structValue.Type()) {
		panic(fmt.Errorf("CloneProperties expected *struct, got %s", structValue.Type()))
	}
	result := reflect.New(structValue.Type().Elem())
	copyProperties(result.Elem(), structValue.Elem())
	return result
}

// CopyProperties takes destination and source reflect.Values of a pointer to structs and returns
// copies each field from the source into the destination.  It recursively copies struct pointers
// and interfaces that contain struct pointers.
func CopyProperties(dstValue, srcValue reflect.Value) {
	if !isStructPtr(dstValue.Type()) {
		panic(fmt.Errorf("CopyProperties expected dstValue *struct, got %s", dstValue.Type()))
	}
	if !isStructPtr(srcValue.Type()) {
		panic(fmt.Errorf("CopyProperties expected srcValue *struct, got %s", srcValue.Type()))
	}
	copyProperties(dstValue.Elem(), srcValue.Elem())
}

func copyProperties(dstValue, srcValue reflect.Value) {
	typ := dstValue.Type()
	if srcValue.Type() != typ {
		panic(fmt.Errorf("can't copy mismatching types (%s <- %s)",
			dstValue.Kind(), srcValue.Kind()))
	}

	for i, field := range typeFields(typ) {
		if field.PkgPath != "" {
			panic(fmt.Errorf("can't copy a private field %q", field.Name))
		}

		srcFieldValue := srcValue.Field(i)
		dstFieldValue := dstValue.Field(i)
		dstFieldInterfaceValue := reflect.Value{}
		origDstFieldValue := dstFieldValue

		switch srcFieldValue.Kind() {
		case reflect.Bool, reflect.String, reflect.Int, reflect.Uint:
			dstFieldValue.Set(srcFieldValue)
		case reflect.Struct:
			copyProperties(dstFieldValue, srcFieldValue)
		case reflect.Slice:
			if !srcFieldValue.IsNil() {
				if srcFieldValue != dstFieldValue {
					newSlice := reflect.MakeSlice(field.Type, srcFieldValue.Len(),
						srcFieldValue.Len())
					reflect.Copy(newSlice, srcFieldValue)
					dstFieldValue.Set(newSlice)
				}
			} else {
				dstFieldValue.Set(srcFieldValue)
			}
		case reflect.Interface:
			if srcFieldValue.IsNil() {
				dstFieldValue.Set(srcFieldValue)
				break
			}

			srcFieldValue = srcFieldValue.Elem()

			if !isStructPtr(srcFieldValue.Type()) {
				panic(fmt.Errorf("can't clone field %q: expected interface to contain *struct, found %s",
					field.Name, srcFieldValue.Type()))
			}

			if dstFieldValue.IsNil() || dstFieldValue.Elem().Type() != srcFieldValue.Type() {
				// We can't use the existing destination allocation, so
				// clone a new one.
				newValue := reflect.New(srcFieldValue.Type()).Elem()
				dstFieldValue.Set(newValue)
				dstFieldInterfaceValue = dstFieldValue
				dstFieldValue = newValue
			} else {
				dstFieldValue = dstFieldValue.Elem()
			}
			fallthrough
		case reflect.Ptr:
			if srcFieldValue.IsNil() {
				origDstFieldValue.Set(srcFieldValue)
				break
			}

			switch srcFieldValue.Elem().Kind() {
			case reflect.Struct:
				if !dstFieldValue.IsNil() {
					// Re-use the existing allocation.
					copyProperties(dstFieldValue.Elem(), srcFieldValue.Elem())
					break
				} else {
					newValue := CloneProperties(srcFieldValue)
					if dstFieldInterfaceValue.IsValid() {
						dstFieldInterfaceValue.Set(newValue)
					} else {
						origDstFieldValue.Set(newValue)
					}
				}
			case reflect.Bool, reflect.Int64, reflect.String:
				newValue := reflect.New(srcFieldValue.Elem().Type())
				newValue.Elem().Set(srcFieldValue.Elem())
				origDstFieldValue.Set(newValue)
			default:
				panic(fmt.Errorf("can't clone pointer field %q type %s",
					field.Name, srcFieldValue.Type()))
			}
		default:
			panic(fmt.Errorf("unexpected type for property struct field %q: %s",
				field.Name, srcFieldValue.Type()))
		}
	}
}

// ZeroProperties takes a reflect.Value of a pointer to a struct and replaces all of its fields
// with zero values, recursing into struct, pointer to struct and interface fields.
func ZeroProperties(structValue reflect.Value) {
	if !isStructPtr(structValue.Type()) {
		panic(fmt.Errorf("ZeroProperties expected *struct, got %s", structValue.Type()))
	}
	zeroProperties(structValue.Elem())
}

func zeroProperties(structValue reflect.Value) {
	typ := structValue.Type()

	for i, field := range typeFields(typ) {
		if field.PkgPath != "" {
			// The field is not exported so just skip it.
			continue
		}

		fieldValue := structValue.Field(i)

		switch fieldValue.Kind() {
		case reflect.Bool, reflect.String, reflect.Slice, reflect.Int, reflect.Uint:
			fieldValue.Set(reflect.Zero(fieldValue.Type()))
		case reflect.Interface:
			if fieldValue.IsNil() {
				break
			}

			// We leave the pointer intact and zero out the struct that's
			// pointed to.
			fieldValue = fieldValue.Elem()
			if !isStructPtr(fieldValue.Type()) {
				panic(fmt.Errorf("can't zero field %q: expected interface to contain *struct, found %s",
					field.Name, fieldValue.Type()))
			}
			fallthrough
		case reflect.Ptr:
			switch fieldValue.Type().Elem().Kind() {
			case reflect.Struct:
				if fieldValue.IsNil() {
					break
				}
				zeroProperties(fieldValue.Elem())
			case reflect.Bool, reflect.Int64, reflect.String:
				fieldValue.Set(reflect.Zero(fieldValue.Type()))
			default:
				panic(fmt.Errorf("can't zero field %q: points to a %s",
					field.Name, fieldValue.Elem().Kind()))
			}
		case reflect.Struct:
			zeroProperties(fieldValue)
		default:
			panic(fmt.Errorf("unexpected kind for property struct field %q: %s",
				field.Name, fieldValue.Kind()))
		}
	}
}

// CloneEmptyProperties takes a reflect.Value of a pointer to a struct and returns a reflect.Value
// of a pointer to a new struct that has the zero values for its fields.  It recursively clones
// struct pointers and interfaces that contain struct pointers.
func CloneEmptyProperties(structValue reflect.Value) reflect.Value {
	if !isStructPtr(structValue.Type()) {
		panic(fmt.Errorf("CloneEmptyProperties expected *struct, got %s", structValue.Type()))
	}
	result := reflect.New(structValue.Type().Elem())
	cloneEmptyProperties(result.Elem(), structValue.Elem())
	return result
}

func cloneEmptyProperties(dstValue, srcValue reflect.Value) {
	typ := srcValue.Type()
	for i, field := range typeFields(typ) {
		if field.PkgPath != "" {
			// The field is not exported so just skip it.
			continue
		}

		srcFieldValue := srcValue.Field(i)
		dstFieldValue := dstValue.Field(i)
		dstFieldInterfaceValue := reflect.Value{}

		switch srcFieldValue.Kind() {
		case reflect.Bool, reflect.String, reflect.Slice, reflect.Int, reflect.Uint:
			// Nothing
		case reflect.Struct:
			cloneEmptyProperties(dstFieldValue, srcFieldValue)
		case reflect.Interface:
			if srcFieldValue.IsNil() {
				break
			}

			srcFieldValue = srcFieldValue.Elem()
			if !isStructPtr(srcFieldValue.Type()) {
				panic(fmt.Errorf("can't clone empty field %q: expected interface to contain *struct, found %s",
					field.Name, srcFieldValue.Type()))
			}

			newValue := reflect.New(srcFieldValue.Type()).Elem()
			dstFieldValue.Set(newValue)
			dstFieldInterfaceValue = dstFieldValue
			dstFieldValue = newValue
			fallthrough
		case reflect.Ptr:
			switch srcFieldValue.Type().Elem().Kind() {
			case reflect.Struct:
				if srcFieldValue.IsNil() {
					break
				}
				newValue := CloneEmptyProperties(srcFieldValue)
				if dstFieldInterfaceValue.IsValid() {
					dstFieldInterfaceValue.Set(newValue)
				} else {
					dstFieldValue.Set(newValue)
				}
			case reflect.Bool, reflect.Int64, reflect.String:
				// Nothing
			default:
				panic(fmt.Errorf("can't clone empty field %q: points to a %s",
					field.Name, srcFieldValue.Elem().Kind()))
			}

		default:
			panic(fmt.Errorf("unexpected kind for property struct field %q: %s",
				field.Name, srcFieldValue.Kind()))
		}
	}
}

var typeFieldCache sync.Map

func typeFields(typ reflect.Type) []reflect.StructField {
	// reflect.Type.Field allocates a []int{} to hold the index every time it is called, which ends up
	// being a significant portion of the GC pressure.  It can't reuse the same one in case a caller
	// modifies the backing array through the slice.  Since we don't modify it, cache the result
	// locally to reduce allocations.

	// Fast path
	if typeFields, ok := typeFieldCache.Load(typ); ok {
		return typeFields.([]reflect.StructField)
	}

	// Slow path
	typeFields := make([]reflect.StructField, typ.NumField())

	for i := range typeFields {
		typeFields[i] = typ.Field(i)
	}

	typeFieldCache.Store(typ, typeFields)

	return typeFields
}
