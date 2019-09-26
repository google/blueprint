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
	"reflect"
)

type FilterFieldPredicate func(field reflect.StructField, string string) (bool, reflect.StructField)

func filterPropertyStructFields(fields []reflect.StructField, prefix string, predicate FilterFieldPredicate) (filteredFields []reflect.StructField, filtered bool) {
	for _, field := range fields {
		var keep bool
		if keep, field = predicate(field, prefix); !keep {
			filtered = true
			continue
		}

		subPrefix := field.Name
		if prefix != "" {
			subPrefix = prefix + "." + subPrefix
		}

		// Recurse into structs
		switch field.Type.Kind() {
		case reflect.Struct:
			var subFiltered bool
			field.Type, subFiltered = filterPropertyStruct(field.Type, subPrefix, predicate)
			filtered = filtered || subFiltered
			if field.Type == nil {
				continue
			}
		case reflect.Ptr:
			if field.Type.Elem().Kind() == reflect.Struct {
				nestedType, subFiltered := filterPropertyStruct(field.Type.Elem(), subPrefix, predicate)
				filtered = filtered || subFiltered
				if nestedType == nil {
					continue
				}
				field.Type = reflect.PtrTo(nestedType)
			}
		case reflect.Interface:
			panic("Interfaces are not supported in filtered property structs")
		}

		filteredFields = append(filteredFields, field)
	}

	return filteredFields, filtered
}

// FilterPropertyStruct takes a reflect.Type that is either a struct or a pointer to a struct, and returns a
// reflect.Type that only contains the fields in the original type for which predicate returns true, and a bool
// that is true if the new struct type has fewer fields than the original type.  If there are no fields in the
// original type for which predicate returns true it returns nil and true.
func FilterPropertyStruct(prop reflect.Type, predicate FilterFieldPredicate) (filteredProp reflect.Type, filtered bool) {
	return filterPropertyStruct(prop, "", predicate)
}

func filterPropertyStruct(prop reflect.Type, prefix string, predicate FilterFieldPredicate) (filteredProp reflect.Type, filtered bool) {
	var fields []reflect.StructField

	ptr := prop.Kind() == reflect.Ptr
	if ptr {
		prop = prop.Elem()
	}

	for i := 0; i < prop.NumField(); i++ {
		fields = append(fields, prop.Field(i))
	}

	filteredFields, filtered := filterPropertyStructFields(fields, prefix, predicate)

	if len(filteredFields) == 0 {
		return nil, true
	}

	if !filtered {
		if ptr {
			return reflect.PtrTo(prop), false
		}
		return prop, false
	}

	ret := reflect.StructOf(filteredFields)
	if ptr {
		ret = reflect.PtrTo(ret)
	}

	return ret, true
}

// FilterPropertyStructSharded takes a reflect.Type that is either a sturct or a pointer to a struct, and returns a list
// of reflect.Type that only contains the fields in the original type for which predicate returns true, and a bool that
// is true if the new struct type has fewer fields than the original type.  If there are no fields in the original type
// for which predicate returns true it returns nil and true.  Each returned struct type will have a maximum of 10 top
// level fields in it to attempt to avoid hitting the 65535 byte type name length limit in reflect.StructOf
// (reflect.nameFrom: name too long), although the limit can still be reached with a single struct field with many
// fields in it.
func FilterPropertyStructSharded(prop reflect.Type, predicate FilterFieldPredicate) (filteredProp []reflect.Type, filtered bool) {
	var fields []reflect.StructField

	ptr := prop.Kind() == reflect.Ptr
	if ptr {
		prop = prop.Elem()
	}

	for i := 0; i < prop.NumField(); i++ {
		fields = append(fields, prop.Field(i))
	}

	fields, filtered = filterPropertyStructFields(fields, "", predicate)
	if !filtered {
		if ptr {
			return []reflect.Type{reflect.PtrTo(prop)}, false
		}
		return []reflect.Type{prop}, false
	}

	if len(fields) == 0 {
		return nil, true
	}

	shards := shardFields(fields, 10)

	for _, shard := range shards {
		s := reflect.StructOf(shard)
		if ptr {
			s = reflect.PtrTo(s)
		}
		filteredProp = append(filteredProp, s)
	}

	return filteredProp, true
}

func shardFields(fields []reflect.StructField, shardSize int) [][]reflect.StructField {
	ret := make([][]reflect.StructField, 0, (len(fields)+shardSize-1)/shardSize)
	for len(fields) > shardSize {
		ret = append(ret, fields[0:shardSize])
		fields = fields[shardSize:]
	}
	if len(fields) > 0 {
		ret = append(ret, fields)
	}
	return ret
}
