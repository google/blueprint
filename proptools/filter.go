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
	"strconv"
)

type FilterFieldPredicate func(field reflect.StructField, string string) (bool, reflect.StructField)

type cantFitPanic struct {
	field reflect.StructField
	size  int
}

func (x cantFitPanic) Error() string {
	return fmt.Sprintf("Can't fit field %s %s %s size %d into %d",
		x.field.Name, x.field.Type.String(), strconv.Quote(string(x.field.Tag)),
		fieldToTypeNameSize(x.field, true)+2, x.size)
}

// All runtime created structs will have a name that starts with "struct {" and ends with "}"
const emptyStructTypeNameSize = len("struct {}")

func filterPropertyStructFields(fields []reflect.StructField, prefix string, maxTypeNameSize int,
	predicate FilterFieldPredicate) (filteredFieldsShards [][]reflect.StructField, filtered bool) {

	structNameSize := emptyStructTypeNameSize

	var filteredFields []reflect.StructField

	appendAndShardIfNameFull := func(field reflect.StructField) {
		fieldTypeNameSize := fieldToTypeNameSize(field, true)
		// Every field will have a space before it and either a semicolon or space after it.
		fieldTypeNameSize += 2

		if maxTypeNameSize > 0 && structNameSize+fieldTypeNameSize > maxTypeNameSize {
			if len(filteredFields) == 0 {
				if isStruct(field.Type) || isStructPtr(field.Type) {
					// An error fitting the nested struct should have been caught when recursing
					// into the nested struct.
					panic(fmt.Errorf("Shouldn't happen: can't fit nested struct %q (%d) into %d",
						field.Type.String(), len(field.Type.String()), maxTypeNameSize-structNameSize))
				}
				panic(cantFitPanic{field, maxTypeNameSize - structNameSize})

			}
			filteredFieldsShards = append(filteredFieldsShards, filteredFields)
			filteredFields = nil
			structNameSize = emptyStructTypeNameSize
		}

		filteredFields = append(filteredFields, field)
		structNameSize += fieldTypeNameSize
	}

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

		ptrToStruct := false
		if isStructPtr(field.Type) {
			ptrToStruct = true
		}

		// Recurse into structs
		if ptrToStruct || isStruct(field.Type) {
			subMaxTypeNameSize := maxTypeNameSize
			if maxTypeNameSize > 0 {
				// In the worst case where only this nested struct will fit in the outer struct, the
				// outer struct will contribute struct{}, the name and tag of the field that contains
				// the nested struct, and one space before and after the field.
				subMaxTypeNameSize -= emptyStructTypeNameSize + fieldToTypeNameSize(field, false) + 2
			}
			typ := field.Type
			if ptrToStruct {
				subMaxTypeNameSize -= len("*")
				typ = typ.Elem()
			}
			nestedTypes, subFiltered := filterPropertyStruct(typ, subPrefix, subMaxTypeNameSize, predicate)
			filtered = filtered || subFiltered
			if nestedTypes == nil {
				continue
			}

			for _, nestedType := range nestedTypes {
				if ptrToStruct {
					nestedType = reflect.PtrTo(nestedType)
				}
				field.Type = nestedType
				appendAndShardIfNameFull(field)
			}
		} else {
			appendAndShardIfNameFull(field)
		}
	}

	if len(filteredFields) > 0 {
		filteredFieldsShards = append(filteredFieldsShards, filteredFields)
	}

	return filteredFieldsShards, filtered
}

func fieldToTypeNameSize(field reflect.StructField, withType bool) int {
	nameSize := len(field.Name)
	nameSize += len(" ")
	if withType {
		nameSize += len(field.Type.String())
	}
	if field.Tag != "" {
		nameSize += len(" ")
		nameSize += len(strconv.Quote(string(field.Tag)))
	}
	return nameSize
}

// FilterPropertyStruct takes a reflect.Type that is either a struct or a pointer to a struct, and returns a
// reflect.Type that only contains the fields in the original type for which predicate returns true, and a bool
// that is true if the new struct type has fewer fields than the original type.  If there are no fields in the
// original type for which predicate returns true it returns nil and true.
func FilterPropertyStruct(prop reflect.Type, predicate FilterFieldPredicate) (filteredProp reflect.Type, filtered bool) {
	filteredFieldsShards, filtered := filterPropertyStruct(prop, "", -1, predicate)
	switch len(filteredFieldsShards) {
	case 0:
		return nil, filtered
	case 1:
		return filteredFieldsShards[0], filtered
	default:
		panic("filterPropertyStruct should only return 1 struct if maxNameSize < 0")
	}
}

func filterPropertyStruct(prop reflect.Type, prefix string, maxNameSize int,
	predicate FilterFieldPredicate) (filteredProp []reflect.Type, filtered bool) {

	var fields []reflect.StructField

	ptr := prop.Kind() == reflect.Ptr
	if ptr {
		prop = prop.Elem()
	}

	for i := 0; i < prop.NumField(); i++ {
		fields = append(fields, prop.Field(i))
	}

	filteredFieldsShards, filtered := filterPropertyStructFields(fields, prefix, maxNameSize, predicate)

	if len(filteredFieldsShards) == 0 {
		return nil, true
	}

	// If the predicate selected all fields in the structure then it is generally better to reuse the
	// original type as it avoids the footprint of creating another type. Also, if the original type
	// is a named type then it will reduce the size of any structs the caller may create that include
	// fields of this type. However, the original type should only be reused if it does not exceed
	// maxNameSize. That is, of course, more likely for an anonymous type than a named one but this
	// treats them the same.
	if !filtered && (maxNameSize < 0 || len(prop.String()) < maxNameSize) {
		if ptr {
			return []reflect.Type{reflect.PtrTo(prop)}, false
		}
		return []reflect.Type{prop}, false
	}

	var ret []reflect.Type
	for _, filteredFields := range filteredFieldsShards {
		p := reflect.StructOf(filteredFields)
		if ptr {
			p = reflect.PtrTo(p)
		}
		ret = append(ret, p)
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
func FilterPropertyStructSharded(prop reflect.Type, maxTypeNameSize int, predicate FilterFieldPredicate) (filteredProp []reflect.Type, filtered bool) {
	return filterPropertyStruct(prop, "", maxTypeNameSize, predicate)
}
