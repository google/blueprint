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
)

func CloneProperties(structValue reflect.Value) reflect.Value {
	result := reflect.New(structValue.Type())
	CopyProperties(result.Elem(), structValue)
	return result
}

func CopyProperties(dstValue, srcValue reflect.Value) {
	typ := dstValue.Type()
	if srcValue.Type() != typ {
		panic(fmt.Errorf("can't copy mismatching types (%s <- %s)",
			dstValue.Kind(), srcValue.Kind()))
	}

	for i := 0; i < srcValue.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			// The field is not exported so just skip it.
			continue
		}

		srcFieldValue := srcValue.Field(i)
		dstFieldValue := dstValue.Field(i)
		dstFieldInterfaceValue := reflect.Value{}

		switch srcFieldValue.Kind() {
		case reflect.Bool, reflect.String, reflect.Int, reflect.Uint:
			dstFieldValue.Set(srcFieldValue)
		case reflect.Struct:
			CopyProperties(dstFieldValue, srcFieldValue)
		case reflect.Slice:
			if !srcFieldValue.IsNil() {
				if field.Type.Elem().Kind() != reflect.String {
					panic(fmt.Errorf("can't copy field %q: slice elements are not strings", field.Name))
				}
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

			if srcFieldValue.Kind() != reflect.Ptr {
				panic(fmt.Errorf("can't clone field %q: interface refers to a non-pointer",
					field.Name))
			}
			if srcFieldValue.Type().Elem().Kind() != reflect.Struct {
				panic(fmt.Errorf("can't clone field %q: interface points to a non-struct",
					field.Name))
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
				dstFieldValue.Set(srcFieldValue)
				break
			}

			srcFieldValue := srcFieldValue.Elem()

			switch srcFieldValue.Kind() {
			case reflect.Struct:
				if !dstFieldValue.IsNil() {
					// Re-use the existing allocation.
					CopyProperties(dstFieldValue.Elem(), srcFieldValue)
					break
				} else {
					newValue := CloneProperties(srcFieldValue)
					if dstFieldInterfaceValue.IsValid() {
						dstFieldInterfaceValue.Set(newValue)
					} else {
						dstFieldValue.Set(newValue)
					}
				}
			case reflect.Bool, reflect.String:
				newValue := reflect.New(srcFieldValue.Type())
				newValue.Elem().Set(srcFieldValue)
				dstFieldValue.Set(newValue)
			default:
				panic(fmt.Errorf("can't clone field %q: points to a %s",
					field.Name, srcFieldValue.Kind()))
			}
		default:
			panic(fmt.Errorf("unexpected kind for property struct field %q: %s",
				field.Name, srcFieldValue.Kind()))
		}
	}
}

func ZeroProperties(structValue reflect.Value) {
	typ := structValue.Type()

	for i := 0; i < structValue.NumField(); i++ {
		field := typ.Field(i)
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
			if fieldValue.Kind() != reflect.Ptr {
				panic(fmt.Errorf("can't zero field %q: interface refers to a non-pointer",
					field.Name))
			}
			if fieldValue.Type().Elem().Kind() != reflect.Struct {
				panic(fmt.Errorf("can't zero field %q: interface points to a non-struct",
					field.Name))
			}
			fallthrough
		case reflect.Ptr:
			switch fieldValue.Type().Elem().Kind() {
			case reflect.Struct:
				if fieldValue.IsNil() {
					break
				}
				ZeroProperties(fieldValue.Elem())
			case reflect.Bool, reflect.String:
				fieldValue.Set(reflect.Zero(fieldValue.Type()))
			default:
				panic(fmt.Errorf("can't zero field %q: points to a %s",
					field.Name, fieldValue.Elem().Kind()))
			}
		case reflect.Struct:
			ZeroProperties(fieldValue)
		default:
			panic(fmt.Errorf("unexpected kind for property struct field %q: %s",
				field.Name, fieldValue.Kind()))
		}
	}
}

func CloneEmptyProperties(structValue reflect.Value) reflect.Value {
	result := reflect.New(structValue.Type())
	cloneEmptyProperties(result.Elem(), structValue)
	return result
}

func cloneEmptyProperties(dstValue, srcValue reflect.Value) {
	typ := srcValue.Type()
	for i := 0; i < srcValue.NumField(); i++ {
		field := typ.Field(i)
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
			if srcFieldValue.Kind() != reflect.Ptr {
				panic(fmt.Errorf("can't clone empty field %q: interface refers to a non-pointer",
					field.Name))
			}
			if srcFieldValue.Type().Elem().Kind() != reflect.Struct {
				panic(fmt.Errorf("can't clone empty field %q: interface points to a non-struct",
					field.Name))
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
				newValue := CloneEmptyProperties(srcFieldValue.Elem())
				if dstFieldInterfaceValue.IsValid() {
					dstFieldInterfaceValue.Set(newValue)
				} else {
					dstFieldValue.Set(newValue)
				}
			case reflect.Bool, reflect.String:
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
