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
	"sort"
	"strconv"
	"strings"
	"text/scanner"

	"github.com/google/blueprint/parser"
)

const maxUnpackErrors = 10

type UnpackError struct {
	Err error
	Pos scanner.Position
}

func (e *UnpackError) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Err)
}

// packedProperty helps to track properties usage (`used` will be true)
type packedProperty struct {
	property *parser.Property
	used     bool
}

// unpackContext keeps compound names and their values in a map. It is initialized from
// parsed properties.
type unpackContext struct {
	propertyMap map[string]*packedProperty
	errs        []error
}

// UnpackProperties populates the list of runtime values ("property structs") from the parsed properties.
// If a property a.b.c has a value, a field with the matching name in each runtime value is initialized
// from it. See PropertyNameForField for field and property name matching.
// For instance, if the input contains
//   { foo: "abc", bar: {x: 1},}
// and a runtime value being has been declared as
//   var v struct { Foo string; Bar int }
// then v.Foo will be set to "abc" and v.Bar will be set to 1
// (cf. unpack_test.go for further examples)
//
// The type of a receiving field has to match the property type, i.e., a bool/int/string field
// can be set from a property with bool/int/string value, a struct can be set from a map (only the
// matching fields are set), and an slice can be set from a list.
// If a field of a runtime value has been already set prior to the UnpackProperties, the new value
// is appended to it (see somewhat inappropriately named ExtendBasicType).
// The same property can initialize fields in multiple runtime values. It is an error if any property
// value was not used to initialize at least one field.
func UnpackProperties(properties []*parser.Property, objects ...interface{}) (map[string]*parser.Property, []error) {
	var unpackContext unpackContext
	unpackContext.propertyMap = make(map[string]*packedProperty)
	if !unpackContext.buildPropertyMap("", properties) {
		return nil, unpackContext.errs
	}

	for _, obj := range objects {
		valueObject := reflect.ValueOf(obj)
		if !isStructPtr(valueObject.Type()) {
			panic(fmt.Errorf("properties must be *struct, got %s",
				valueObject.Type()))
		}
		unpackContext.unpackToStruct("", valueObject.Elem())
		if len(unpackContext.errs) >= maxUnpackErrors {
			return nil, unpackContext.errs
		}
	}

	// Gather property map, and collect any unused properties.
	// Avoid reporting subproperties of unused properties.
	result := make(map[string]*parser.Property)
	var unusedNames []string
	for name, v := range unpackContext.propertyMap {
		if v.used {
			result[name] = v.property
		} else {
			unusedNames = append(unusedNames, name)
		}
	}
	if len(unusedNames) == 0 && len(unpackContext.errs) == 0 {
		return result, nil
	}
	return nil, unpackContext.reportUnusedNames(unusedNames)
}

func (ctx *unpackContext) reportUnusedNames(unusedNames []string) []error {
	sort.Strings(unusedNames)
	var lastReported string
	for _, name := range unusedNames {
		// if 'foo' has been reported, ignore 'foo\..*' and 'foo\[.*'
		if lastReported != "" {
			trimmed := strings.TrimPrefix(name, lastReported)
			if trimmed != name && (trimmed[0] == '.' || trimmed[0] == '[') {
				continue
			}
		}
		ctx.errs = append(ctx.errs, &UnpackError{
			fmt.Errorf("unrecognized property %q", name),
			ctx.propertyMap[name].property.ColonPos})
		lastReported = name
	}
	return ctx.errs
}

func (ctx *unpackContext) buildPropertyMap(prefix string, properties []*parser.Property) bool {
	nOldErrors := len(ctx.errs)
	for _, property := range properties {
		name := fieldPath(prefix, property.Name)
		if first, present := ctx.propertyMap[name]; present {
			ctx.addError(
				&UnpackError{fmt.Errorf("property %q already defined", name), property.ColonPos})
			if ctx.addError(
				&UnpackError{fmt.Errorf("<-- previous definition here"), first.property.ColonPos}) {
				return false
			}
			continue
		}

		ctx.propertyMap[name] = &packedProperty{property, false}
		switch propValue := property.Value.Eval().(type) {
		case *parser.Map:
			ctx.buildPropertyMap(name, propValue.Properties)
		case *parser.List:
			// If it is a list, unroll it unless its elements are of primitive type
			// (no further mapping will be needed in that case, so we avoid cluttering
			// the map).
			if len(propValue.Values) == 0 {
				continue
			}
			if t := propValue.Values[0].Type(); t == parser.StringType || t == parser.Int64Type || t == parser.BoolType {
				continue
			}

			itemProperties := make([]*parser.Property, len(propValue.Values), len(propValue.Values))
			for i, expr := range propValue.Values {
				itemProperties[i] = &parser.Property{
					Name:     property.Name + "[" + strconv.Itoa(i) + "]",
					NamePos:  property.NamePos,
					ColonPos: property.ColonPos,
					Value:    expr,
				}
			}
			if !ctx.buildPropertyMap(prefix, itemProperties) {
				return false
			}
		}
	}

	return len(ctx.errs) == nOldErrors
}

func fieldPath(prefix, fieldName string) string {
	if prefix == "" {
		return fieldName
	}
	return prefix + "." + fieldName
}

func (ctx *unpackContext) addError(e error) bool {
	ctx.errs = append(ctx.errs, e)
	return len(ctx.errs) < maxUnpackErrors
}

func (ctx *unpackContext) unpackToStruct(namePrefix string, structValue reflect.Value) {
	structType := structValue.Type()

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

		propertyName := fieldPath(namePrefix, PropertyNameForField(field.Name))

		if !fieldValue.CanSet() {
			panic(fmt.Errorf("field %s is not settable", propertyName))
		}

		// Get the property value if it was specified.
		packedProperty, propertyIsSet := ctx.propertyMap[propertyName]

		origFieldValue := fieldValue

		// To make testing easier we validate the struct field's type regardless
		// of whether or not the property was specified in the parsed string.
		// TODO(ccross): we don't validate types inside nil struct pointers
		// Move type validation to a function that runs on each factory once
		switch kind := fieldValue.Kind(); kind {
		case reflect.Bool, reflect.String, reflect.Struct, reflect.Slice:
			// Do nothing
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
			case reflect.Bool, reflect.Int64, reflect.String:
				// Nothing
			default:
				panic(fmt.Errorf("field %s contains a pointer to %s", propertyName, ptrKind))
			}

		case reflect.Int, reflect.Uint:
			if !HasTag(field, "blueprint", "mutated") {
				panic(fmt.Errorf(`int field %s must be tagged blueprint:"mutated"`, propertyName))
			}

		default:
			panic(fmt.Errorf("unsupported kind for field %s: %s", propertyName, kind))
		}

		if field.Anonymous && isStruct(fieldValue.Type()) {
			ctx.unpackToStruct(namePrefix, fieldValue)
			continue
		}

		if !propertyIsSet {
			// This property wasn't specified.
			continue
		}

		packedProperty.used = true
		property := packedProperty.property

		if HasTag(field, "blueprint", "mutated") {
			if !ctx.addError(
				&UnpackError{
					fmt.Errorf("mutated field %s cannot be set in a Blueprint file", propertyName),
					property.ColonPos,
				}) {
				return
			}
			continue
		}

		if isStruct(fieldValue.Type()) {
			ctx.unpackToStruct(propertyName, fieldValue)
			if len(ctx.errs) >= maxUnpackErrors {
				return
			}
		} else if isSlice(fieldValue.Type()) {
			if unpackedValue, ok := ctx.unpackToSlice(propertyName, property, fieldValue.Type()); ok {
				ExtendBasicType(fieldValue, unpackedValue, Append)
			}
			if len(ctx.errs) >= maxUnpackErrors {
				return
			}

		} else {
			unpackedValue, err := propertyToValue(fieldValue.Type(), property)
			if err != nil && !ctx.addError(err) {
				return
			}
			ExtendBasicType(fieldValue, unpackedValue, Append)
		}
	}
}

// unpackSlice creates a value of a given slice type from the property which should be a list
func (ctx *unpackContext) unpackToSlice(
	sliceName string, property *parser.Property, sliceType reflect.Type) (reflect.Value, bool) {
	propValueAsList, ok := property.Value.Eval().(*parser.List)
	if !ok {
		ctx.addError(fmt.Errorf("%s: can't assign %s value to list property %q",
			property.Value.Pos(), property.Value.Type(), property.Name))
		return reflect.MakeSlice(sliceType, 0, 0), false
	}
	exprs := propValueAsList.Values
	value := reflect.MakeSlice(sliceType, 0, len(exprs))
	if len(exprs) == 0 {
		return value, true
	}

	// The function to construct an item value depends on the type of list elements.
	var getItemFunc func(*parser.Property, reflect.Type) (reflect.Value, bool)
	switch exprs[0].Type() {
	case parser.BoolType, parser.StringType, parser.Int64Type:
		getItemFunc = func(property *parser.Property, t reflect.Type) (reflect.Value, bool) {
			value, err := propertyToValue(t, property)
			if err != nil {
				ctx.addError(err)
				return value, false
			}
			return value, true
		}
	case parser.ListType:
		getItemFunc = func(property *parser.Property, t reflect.Type) (reflect.Value, bool) {
			return ctx.unpackToSlice(property.Name, property, t)
		}
	case parser.MapType:
		getItemFunc = func(property *parser.Property, t reflect.Type) (reflect.Value, bool) {
			itemValue := reflect.New(t).Elem()
			ctx.unpackToStruct(property.Name, itemValue)
			return itemValue, true
		}
	case parser.NotEvaluatedType:
		getItemFunc = func(property *parser.Property, t reflect.Type) (reflect.Value, bool) {
			return reflect.New(t), false
		}
	default:
		panic(fmt.Errorf("bizarre property expression type: %v", exprs[0].Type()))
	}

	itemProperty := &parser.Property{NamePos: property.NamePos, ColonPos: property.ColonPos}
	elemType := sliceType.Elem()
	isPtr := elemType.Kind() == reflect.Ptr

	for i, expr := range exprs {
		itemProperty.Name = sliceName + "[" + strconv.Itoa(i) + "]"
		itemProperty.Value = expr
		if packedProperty, ok := ctx.propertyMap[itemProperty.Name]; ok {
			packedProperty.used = true
		}
		if isPtr {
			if itemValue, ok := getItemFunc(itemProperty, elemType.Elem()); ok {
				ptrValue := reflect.New(itemValue.Type())
				ptrValue.Elem().Set(itemValue)
				value = reflect.Append(value, ptrValue)
			}
		} else {
			if itemValue, ok := getItemFunc(itemProperty, elemType); ok {
				value = reflect.Append(value, itemValue)
			}
		}
	}
	return value, true
}

// propertyToValue creates a value of a given value type from the property.
func propertyToValue(typ reflect.Type, property *parser.Property) (reflect.Value, error) {
	var value reflect.Value
	var baseType reflect.Type
	isPtr := typ.Kind() == reflect.Ptr
	if isPtr {
		baseType = typ.Elem()
	} else {
		baseType = typ
	}

	switch kind := baseType.Kind(); kind {
	case reflect.Bool:
		b, ok := property.Value.Eval().(*parser.Bool)
		if !ok {
			return value, fmt.Errorf("%s: can't assign %s value to bool property %q",
				property.Value.Pos(), property.Value.Type(), property.Name)
		}
		value = reflect.ValueOf(b.Value)

	case reflect.Int64:
		b, ok := property.Value.Eval().(*parser.Int64)
		if !ok {
			return value, fmt.Errorf("%s: can't assign %s value to int64 property %q",
				property.Value.Pos(), property.Value.Type(), property.Name)
		}
		value = reflect.ValueOf(b.Value)

	case reflect.String:
		s, ok := property.Value.Eval().(*parser.String)
		if !ok {
			return value, fmt.Errorf("%s: can't assign %s value to string property %q",
				property.Value.Pos(), property.Value.Type(), property.Name)
		}
		value = reflect.ValueOf(s.Value)

	default:
		return value, &UnpackError{
			fmt.Errorf("cannot assign %s value %s to %s property %s", property.Value.Type(), property.Value, kind, typ),
			property.NamePos}
	}

	if isPtr {
		ptrValue := reflect.New(value.Type())
		ptrValue.Elem().Set(value)
		return ptrValue, nil
	}
	return value, nil
}
