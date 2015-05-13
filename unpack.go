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

package blueprint

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/blueprint/parser"
	"github.com/google/blueprint/proptools"
)

type packedProperty struct {
	property *parser.Property
	unpacked bool
}

func unpackProperties(propertyDefs []*parser.Property,
	propertiesStructs ...interface{}) (map[string]*parser.Property, []error) {

	propertyMap := make(map[string]*packedProperty)
	errs := buildPropertyMap("", propertyDefs, propertyMap)
	if len(errs) > 0 {
		return nil, errs
	}

	for _, properties := range propertiesStructs {
		propertiesValue := reflect.ValueOf(properties)
		if propertiesValue.Kind() != reflect.Ptr {
			panic("properties must be a pointer to a struct")
		}

		propertiesValue = propertiesValue.Elem()
		if propertiesValue.Kind() != reflect.Struct {
			panic("properties must be a pointer to a struct")
		}

		newErrs := unpackStructValue("", propertiesValue, propertyMap, "", "")
		errs = append(errs, newErrs...)

		if len(errs) >= maxErrors {
			return nil, errs
		}
	}

	// Report any properties that didn't have corresponding struct fields as
	// errors.
	result := make(map[string]*parser.Property)
	for name, packedProperty := range propertyMap {
		result[name] = packedProperty.property
		if !packedProperty.unpacked {
			err := &Error{
				Err: fmt.Errorf("unrecognized property %q", name),
				Pos: packedProperty.property.Pos,
			}
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return nil, errs
	}

	return result, nil
}

func buildPropertyMap(namePrefix string, propertyDefs []*parser.Property,
	propertyMap map[string]*packedProperty) (errs []error) {

	for _, propertyDef := range propertyDefs {
		name := namePrefix + propertyDef.Name.Name
		if first, present := propertyMap[name]; present {
			if first.property == propertyDef {
				// We've already added this property.
				continue
			}

			errs = append(errs, &Error{
				Err: fmt.Errorf("property %q already defined", name),
				Pos: propertyDef.Pos,
			})
			errs = append(errs, &Error{
				Err: fmt.Errorf("<-- previous definition here"),
				Pos: first.property.Pos,
			})
			if len(errs) >= maxErrors {
				return errs
			}
			continue
		}

		propertyMap[name] = &packedProperty{
			property: propertyDef,
			unpacked: false,
		}

		// We intentionally do not rescursively add MapValue properties to the
		// property map here.  Instead we add them when we encounter a struct
		// into which they can be unpacked.  We do this so that if we never
		// encounter such a struct then the "unrecognized property" error will
		// be reported only once for the map property and not for each of its
		// sub-properties.
	}

	return
}

func unpackStructValue(namePrefix string, structValue reflect.Value,
	propertyMap map[string]*packedProperty, filterKey, filterValue string) []error {

	structType := structValue.Type()

	var errs []error
	for i := 0; i < structValue.NumField(); i++ {
		fieldValue := structValue.Field(i)
		field := structType.Field(i)

		if field.PkgPath != "" {
			// This is an unexported field, so just skip it.
			continue
		}

		if !fieldValue.CanSet() {
			panic(fmt.Errorf("field %s is not settable", field.Name))
		}

		// To make testing easier we validate the struct field's type regardless
		// of whether or not the property was specified in the parsed string.
		switch kind := fieldValue.Kind(); kind {
		case reflect.Bool, reflect.String, reflect.Struct:
			// Do nothing
		case reflect.Slice:
			elemType := field.Type.Elem()
			if elemType.Kind() != reflect.String {
				panic(fmt.Errorf("field %s is a non-string slice", field.Name))
			}
		case reflect.Interface:
			if fieldValue.IsNil() {
				panic(fmt.Errorf("field %s contains a nil interface",
					field.Name))
			}
			fieldValue = fieldValue.Elem()
			elemType := fieldValue.Type()
			if elemType.Kind() != reflect.Ptr {
				panic(fmt.Errorf("field %s contains a non-pointer interface",
					field.Name))
			}
			fallthrough
		case reflect.Ptr:
			if fieldValue.IsNil() {
				panic(fmt.Errorf("field %s contains a nil pointer",
					field.Name))
			}
			fieldValue = fieldValue.Elem()
			elemType := fieldValue.Type()
			if elemType.Kind() != reflect.Struct {
				panic(fmt.Errorf("field %s contains a non-struct pointer",
					field.Name))
			}

		case reflect.Int, reflect.Uint:
			if !hasTag(field, "blueprint", "mutated") {
				panic(fmt.Errorf(`int field %s must be tagged blueprint:"mutated"`, field.Name))
			}

		default:
			panic(fmt.Errorf("unsupported kind for field %s: %s",
				field.Name, kind))
		}

		// Get the property value if it was specified.
		propertyName := namePrefix + proptools.PropertyNameForField(field.Name)
		packedProperty, ok := propertyMap[propertyName]
		if !ok {
			// This property wasn't specified.
			continue
		}

		packedProperty.unpacked = true

		if hasTag(field, "blueprint", "mutated") {
			errs = append(errs,
				&Error{
					Err: fmt.Errorf("mutated field %s cannot be set in a Blueprint file", propertyName),
					Pos: packedProperty.property.Pos,
				})
			if len(errs) >= maxErrors {
				return errs
			}
			continue
		}

		if filterKey != "" && !hasTag(field, filterKey, filterValue) {
			errs = append(errs,
				&Error{
					Err: fmt.Errorf("filtered field %s cannot be set in a Blueprint file", propertyName),
					Pos: packedProperty.property.Pos,
				})
			if len(errs) >= maxErrors {
				return errs
			}
			continue
		}

		var newErrs []error

		switch kind := fieldValue.Kind(); kind {
		case reflect.Bool:
			newErrs = unpackBool(fieldValue, packedProperty.property)
		case reflect.String:
			newErrs = unpackString(fieldValue, packedProperty.property)
		case reflect.Slice:
			newErrs = unpackSlice(fieldValue, packedProperty.property)
		case reflect.Ptr, reflect.Interface:
			fieldValue = fieldValue.Elem()
			fallthrough
		case reflect.Struct:
			localFilterKey, localFilterValue := filterKey, filterValue
			if k, v, err := HasFilter(field.Tag); err != nil {
				errs = append(errs, err)
				if len(errs) >= maxErrors {
					return errs
				}
			} else if k != "" {
				if filterKey != "" {
					errs = append(errs, fmt.Errorf("nested filter tag not supported on field %q",
						field.Name))
					if len(errs) >= maxErrors {
						return errs
					}
				} else {
					localFilterKey, localFilterValue = k, v
				}
			}
			newErrs = unpackStruct(propertyName+".", fieldValue,
				packedProperty.property, propertyMap, localFilterKey, localFilterValue)
		}
		errs = append(errs, newErrs...)
		if len(errs) >= maxErrors {
			return errs
		}
	}

	return errs
}

func unpackBool(boolValue reflect.Value, property *parser.Property) []error {
	if property.Value.Type != parser.Bool {
		return []error{
			fmt.Errorf("%s: can't assign %s value to %s property %q",
				property.Value.Pos, property.Value.Type, parser.Bool,
				property.Name),
		}
	}
	boolValue.SetBool(property.Value.BoolValue)
	return nil
}

func unpackString(stringValue reflect.Value,
	property *parser.Property) []error {

	if property.Value.Type != parser.String {
		return []error{
			fmt.Errorf("%s: can't assign %s value to %s property %q",
				property.Value.Pos, property.Value.Type, parser.String,
				property.Name),
		}
	}
	stringValue.SetString(property.Value.StringValue)
	return nil
}

func unpackSlice(sliceValue reflect.Value, property *parser.Property) []error {
	if property.Value.Type != parser.List {
		return []error{
			fmt.Errorf("%s: can't assign %s value to %s property %q",
				property.Value.Pos, property.Value.Type, parser.List,
				property.Name),
		}
	}

	var list []string
	for _, value := range property.Value.ListValue {
		if value.Type != parser.String {
			// The parser should not produce this.
			panic("non-string value found in list")
		}
		list = append(list, value.StringValue)
	}

	sliceValue.Set(reflect.ValueOf(list))
	return nil
}

func unpackStruct(namePrefix string, structValue reflect.Value,
	property *parser.Property, propertyMap map[string]*packedProperty,
	filterKey, filterValue string) []error {

	if property.Value.Type != parser.Map {
		return []error{
			fmt.Errorf("%s: can't assign %s value to %s property %q",
				property.Value.Pos, property.Value.Type, parser.Map,
				property.Name),
		}
	}

	errs := buildPropertyMap(namePrefix, property.Value.MapValue, propertyMap)
	if len(errs) > 0 {
		return errs
	}

	return unpackStructValue(namePrefix, structValue, propertyMap, filterKey, filterValue)
}

func hasTag(field reflect.StructField, name, value string) bool {
	tag := field.Tag.Get(name)
	for _, entry := range strings.Split(tag, ",") {
		if entry == value {
			return true
		}
	}

	return false
}

func HasFilter(field reflect.StructTag) (k, v string, err error) {
	tag := field.Get("blueprint")
	for _, entry := range strings.Split(tag, ",") {
		if strings.HasPrefix(entry, "filter") {
			if !strings.HasPrefix(entry, "filter(") || !strings.HasSuffix(entry, ")") {
				return "", "", fmt.Errorf("unexpected format for filter %q: missing ()", entry)
			}
			entry = strings.TrimPrefix(entry, "filter(")
			entry = strings.TrimSuffix(entry, ")")

			s := strings.Split(entry, ":")
			if len(s) != 2 {
				return "", "", fmt.Errorf("unexpected format for filter %q: expected single ':'", entry)
			}
			k = s[0]
			v, err = strconv.Unquote(s[1])
			if err != nil {
				return "", "", fmt.Errorf("unexpected format for filter %q: %s", entry, err.Error())
			}
			return k, v, nil
		}
	}

	return "", "", nil
}
