package blueprint

import (
	"blueprint/parser"
	"fmt"
	"reflect"
	"unicode"
	"unicode/utf8"
)

type packedProperty struct {
	property *parser.Property
	unpacked bool
}

func unpackProperties(propertyDefs []*parser.Property,
	propertiesStructs ...interface{}) (errs []error) {

	propertyMap := make(map[string]*packedProperty)
	for _, propertyDef := range propertyDefs {
		name := propertyDef.Name
		if first, present := propertyMap[name]; present {
			errs = append(errs, &Error{
				Err: fmt.Errorf("property %q already defined", name),
				Pos: propertyDef.Pos,
			})
			errs = append(errs, &Error{
				Err: fmt.Errorf("--> previous definition here"),
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

		newErrs := unpackStruct(propertiesValue, propertyMap)
		errs = append(errs, newErrs...)

		if len(errs) >= maxErrors {
			return errs
		}
	}

	// Report any properties that didn't have corresponding struct fields as
	// errors.
	for name, packedProperty := range propertyMap {
		if !packedProperty.unpacked {
			err := &Error{
				Err: fmt.Errorf("unrecognized property %q", name),
				Pos: packedProperty.property.Pos,
			}
			errs = append(errs, err)
		}
	}

	return errs
}

func unpackStruct(structValue reflect.Value,
	propertyMap map[string]*packedProperty) []error {

	structType := structValue.Type()

	var errs []error
	for i := 0; i < structValue.NumField(); i++ {
		fieldValue := structValue.Field(i)
		field := structType.Field(i)

		if !fieldValue.CanSet() {
			panic(fmt.Errorf("field %s is not settable", field.Name))
		}

		// To make testing easier we validate the struct field's type regardless
		// of whether or not the property was specified in the parsed string.
		switch kind := fieldValue.Kind(); kind {
		case reflect.Bool, reflect.String:
			// Do nothing
		case reflect.Slice:
			elemType := field.Type.Elem()
			if elemType.Kind() != reflect.String {
				panic(fmt.Errorf("field %s is a non-string slice", field.Name))
			}
		case reflect.Struct:
			newErrs := unpackStruct(fieldValue, propertyMap)
			errs = append(errs, newErrs...)
			if len(errs) >= maxErrors {
				return errs
			}
			continue // This field doesn't correspond to a specific property.
		default:
			panic(fmt.Errorf("unsupported kind for field %s: %s",
				field.Name, kind))
		}

		// Get the property value if it was specified.
		propertyName := propertyNameForField(field)
		packedProperty, ok := propertyMap[propertyName]
		if !ok {
			// This property wasn't specified.
			continue
		}

		packedProperty.unpacked = true

		var newErrs []error
		switch kind := fieldValue.Kind(); kind {
		case reflect.Bool:
			newErrs = unpackBool(fieldValue, packedProperty.property)
		case reflect.String:
			newErrs = unpackString(fieldValue, packedProperty.property)
		case reflect.Slice:
			newErrs = unpackSlice(fieldValue, packedProperty.property)
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

func propertyNameForField(field reflect.StructField) string {
	r, size := utf8.DecodeRuneInString(field.Name)
	propertyName := string(unicode.ToLower(r))
	if len(field.Name) > size {
		propertyName += field.Name[size:]
	}
	return propertyName
}
