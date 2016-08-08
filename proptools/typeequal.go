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

import "reflect"

// TypeEqual takes two property structs, and returns true if they are of equal type, any embedded
// pointers to structs or interfaces having matching nilitude, and any interface{} values in any
// embedded structs, pointers to structs, or interfaces are also of equal type.
func TypeEqual(s1, s2 interface{}) bool {
	return typeEqual(reflect.ValueOf(s1), reflect.ValueOf(s2))
}

func typeEqual(v1, v2 reflect.Value) bool {
	if v1.Type() != v2.Type() {
		return false
	}

	if v1.Kind() == reflect.Interface {
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.IsNil() {
			return true
		}
		v1 = v1.Elem()
		v2 = v2.Elem()
		if v1.Type() != v2.Type() {
			return false
		}
	}

	if v1.Kind() == reflect.Ptr {
		if v1.Type().Elem().Kind() != reflect.Struct {
			return true
		}
		if v1.IsNil() && !v2.IsNil() {
			return concreteType(v2)
		} else if v2.IsNil() && !v1.IsNil() {
			return concreteType(v1)
		} else if v1.IsNil() && v2.IsNil() {
			return true
		}

		v1 = v1.Elem()
		v2 = v2.Elem()
	}

	if v1.Kind() != reflect.Struct {
		return true
	}

	for i := 0; i < v1.NumField(); i++ {
		v1 := v1.Field(i)
		v2 := v2.Field(i)

		switch kind := v1.Kind(); kind {
		case reflect.Interface, reflect.Ptr, reflect.Struct:
			if !typeEqual(v1, v2) {
				return false
			}
		}
	}

	return true
}

// Returns true if v recursively contains no interfaces
func concreteType(v reflect.Value) bool {
	if v.Kind() == reflect.Interface {
		return false
	}

	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return true
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return true
	}

	for i := 0; i < v.NumField(); i++ {
		v := v.Field(i)

		switch kind := v.Kind(); kind {
		case reflect.Interface, reflect.Ptr, reflect.Struct:
			if !concreteType(v) {
				return false
			}
		}
	}

	return true
}
