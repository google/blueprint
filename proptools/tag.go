// Copyright 2019 Google Inc. All rights reserved.
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
	"strings"
)

// HasTag returns true if a StructField has a tag in the form `name:"foo,value"`.
func HasTag(field reflect.StructField, name, value string) bool {
	tag := field.Tag.Get(name)
	for _, entry := range strings.Split(tag, ",") {
		if entry == value {
			return true
		}
	}

	return false
}

// PropertyIndexesWithTag returns the indexes of all properties (in the form used by reflect.Value.FieldByIndex) that
// are tagged with the given key and value, including ones found in embedded structs or pointers to structs.
func PropertyIndexesWithTag(ps interface{}, key, value string) [][]int {
	t := reflect.TypeOf(ps)
	if !isStructPtr(t) {
		panic(fmt.Errorf("type %s is not a pointer to a struct", t))
	}
	t = t.Elem()

	return propertyIndexesWithTag(t, key, value)
}
func propertyIndexesWithTag(t reflect.Type, key, value string) [][]int {
	var indexes [][]int

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		ft := field.Type
		if isStruct(ft) || isStructPtr(ft) {
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			subIndexes := propertyIndexesWithTag(ft, key, value)
			for _, sub := range subIndexes {
				sub = append([]int{i}, sub...)
				indexes = append(indexes, sub)
			}
		} else if HasTag(field, key, value) {
			indexes = append(indexes, field.Index)
		}
	}

	return indexes
}
