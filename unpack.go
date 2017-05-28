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
			err := &BlueprintError{
				Err: fmt.Errorf("unrecognized property %q", name),
				//Pos: packedProperty.property.ColonPos,
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
		name := namePrefix + propertyDef.Name
		if first, present := propertyMap[name]; present {
			if first.property == propertyDef {
				// We've already added this property.
				continue
			}
			errs = append(errs, &BlueprintError{
				Err: fmt.Errorf("property %q already defined", name),
				//Pos: propertyDef.ColonPos,
			})
			errs = append(errs, &BlueprintError{
				Err: fmt.Errorf("<-- previous definition here"),
				//Pos: first.property.ColonPos,
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

		// In Go 1.7, runtime-created structs are unexported, so it's not
		// possible to create an exported anonymous field with a generated
		// type. So workaround this by special-casing "BlueprintEmbed" to
		// behave like an anonymous field for structure unpacking.
		if field.Name == "BlueprintEmbed" {
			field.Name = ""
			field.Anonymous = true
		}

		if field.PkgPath != "" {
			// This is an unexported field, so just skip it.
			continue
		}

		propertyName := namePrefix + proptools.PropertyNameForField(field.Name)

		if !fieldValue.CanSet() {
			panic(fmt.Errorf("field %s is not settable", propertyName))
		}

		// Get the property value if it was specified.
		packedProperty, propertyIsSet := propertyMap[propertyName]

		origFieldValue := fieldValue

		// To make testing easier we validate the struct field's type regardless
		// of whether or not the property was specified in the parsed string.
		// TODO(ccross): we don't validate types inside nil struct pointers
		// Move type validation to a function that runs on each factory once
		switch kind := fieldValue.Kind(); kind {
		case reflect.Bool, reflect.String, reflect.Struct:
			// Do nothing
		case reflect.Slice:
			elemType := field.Type.Elem()
			if elemType.Kind() != reflect.String {
				panic(fmt.Errorf("field %s is a non-string slice", propertyName))
			}
		case reflect.Interface:
			if fieldValue.IsNil() {
				panic(fmt.Errorf("field %s contains a nil interface", propertyName))
			}
			fieldValue = fieldValue.Elem()
			elemType := fieldValue.Type()
			if elemType.Kind() != reflect.Ptr {
				panic(fmt.Errorf("field %s contains a non-pointer interface", propertyName))
			}
			fallthrough
		case reflect.Ptr:
			switch ptrKind := fieldValue.Type().Elem().Kind(); ptrKind {
			case reflect.Struct:
				if fieldValue.IsNil() && (propertyIsSet || field.Anonymous) {
					// Instantiate nil struct pointers
					// Set into origFieldValue in case it was an interface, in which case
					// fieldValue points to the unsettable pointer inside the interface
					fieldValue = reflect.New(fieldValue.Type().Elem())
					origFieldValue.Set(fieldValue)
				}
				fieldValue = fieldValue.Elem()
			case reflect.Bool, reflect.String:
				// Nothing
			default:
				panic(fmt.Errorf("field %s contains a pointer to %s", propertyName, ptrKind))
			}

		case reflect.Int, reflect.Uint:
			if !proptools.HasTag(field, "blueprint", "mutated") {
				panic(fmt.Errorf(`int field %s must be tagged blueprint:"mutated"`, propertyName))
			}

		default:
			panic(fmt.Errorf("unsupported kind for field %s: %s", propertyName, kind))
		}

		if field.Anonymous && fieldValue.Kind() == reflect.Struct {
			newErrs := unpackStructValue(namePrefix, fieldValue, propertyMap, filterKey, filterValue)
			errs = append(errs, newErrs...)
			continue
		}

		if !propertyIsSet {
			// This property wasn't specified.
			continue
		}

		packedProperty.unpacked = true

		if proptools.HasTag(field, "blueprint", "mutated") {
			errs = append(errs,
				&BlueprintError{
					Err: fmt.Errorf("mutated field %s cannot be set in a Blueprint file", propertyName),
					//Pos: packedProperty.property.ColonPos,
				})
			if len(errs) >= maxErrors {
				return errs
			}
			continue
		}

		if filterKey != "" && !proptools.HasTag(field, filterKey, filterValue) {
			errs = append(errs,
				&BlueprintError{
					Err: fmt.Errorf("filtered field %s cannot be set in a Blueprint file", propertyName),
					//Pos: packedProperty.property.ColonPos,
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
		case reflect.Ptr:
			switch ptrKind := fieldValue.Type().Elem().Kind(); ptrKind {
			case reflect.Bool:
				newValue := reflect.New(fieldValue.Type().Elem())
				newErrs = unpackBool(newValue.Elem(), packedProperty.property)
				fieldValue.Set(newValue)
			case reflect.String:
				newValue := reflect.New(fieldValue.Type().Elem())
				newErrs = unpackString(newValue.Elem(), packedProperty.property)
				fieldValue.Set(newValue)
			default:
				panic(fmt.Errorf("unexpected pointer kind %s", ptrKind))
			}
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
		default:
			panic(fmt.Errorf("unexpected kind %s", kind))
		}
		errs = append(errs, newErrs...)
		if len(errs) >= maxErrors {
			return errs
		}
	}

	return errs
}

func unpackBool(boolValue reflect.Value, property *parser.Property) []error {
	b, ok := property.Value.Eval().(*parser.Bool)
	if !ok {
		return []error{
			fmt.Errorf("%s: can't assign %s value to bool property %q",
				/* property.Value.Pos(), */ 0,
				property.Value.Type(), property.Name),
		}
	}
	boolValue.SetBool(b.Value)
	return nil
}

func unpackString(stringValue reflect.Value,
	property *parser.Property) []error {

	s, ok := property.Value.Eval().(*parser.String)
	if !ok {
		return []error{
			fmt.Errorf("%s: can't assign %s value to string property %q",
				/* property.Value.Pos(), */ 0,
				property.Value.Type(), property.Name),
		}
	}
	stringValue.SetString(s.Value)
	return nil
}

func unpackSlice(sliceValue reflect.Value, property *parser.Property) []error {

	l, ok := property.Value.Eval().(*parser.List)
	if !ok {
		return []error{
			fmt.Errorf("%s: can't assign %s value to list property %q",
				/* property.Value.Pos(), */ 0,
				property.Value.Type(), property.Name),
		}
	}

	list := make([]string, len(l.Values))
	for i, value := range l.Values {
		s, ok := value.Eval().(*parser.String)
		if !ok {
			// The parser should not produce this.
			panic(fmt.Errorf("non-string value %q found in list", value))
		}
		list[i] = s.Value
	}

	sliceValue.Set(reflect.ValueOf(list))
	return nil
}

func unpackStruct(namePrefix string, structValue reflect.Value,
	property *parser.Property, propertyMap map[string]*packedProperty,
	filterKey, filterValue string) []error {

	m, ok := property.Value.Eval().(*parser.MapBody)
	if !ok {
		return []error{
			fmt.Errorf("%s: can't assign %s value to map property %q",
				/* property.Value.Pos(), */ 0,
				property.Value.Type(), property.Name),
		}
	}

	errs := buildPropertyMap(namePrefix, m.Properties, propertyMap)
	if len(errs) > 0 {
		return errs
	}

	return unpackStructValue(namePrefix, structValue, propertyMap, filterKey, filterValue)
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
