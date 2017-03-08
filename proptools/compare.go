// Copyright 2017 Google Inc. All rights reserved.
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
)

// DeepCompare is like reflect.deepEqual but it gives an explanation of the first difference that it finds
func DeepCompare(aName string, aItem interface{}, bName string, bItem interface{}) (equal bool, difference string) {
	return deepCompareGeneric(toReflection(aName, aItem), toReflection(bName, bItem), 0)
}

func same() (equal bool, difference string) {
	return true, ""
}

func different(explanation string) (equal bool, difference string) {
	return false, explanation
}

func toReflection(name string, item interface{}) (val namedValue) {
	return namedValue{name, reflect.ValueOf(item)}
}

type namedValue struct {
	name string
	item reflect.Value
}

func getType(value reflect.Value) (result reflect.Type, description string) {
	if !value.IsValid() {
		return nil, "nil"
	}
	return value.Type(), value.Type().String()
}

// tells whether two structs are of the same type
func compareTypes(a namedValue, b namedValue) (equal bool, difference string) {
	aType, aTypeText := getType(a.item)
	bType, bTypeText := getType(b.item)
	if aType == bType {
		return same()
	}
	return different(a.name + " is of type " + aTypeText + " whereas " + b.name + " is of type " + bTypeText)
}

// primary method that compares whether two arbitrary values are equal
func deepCompareGeneric(a namedValue, b namedValue, depth int) (equal bool, difference string) {
	depth++
	if depth > 50 {
		panic(fmt.Sprintf("Detected probable infinite loop comparing %s (%s) and %s (%s). This diff checker doesn't yet support data structures having cycles.",
			a.name,
			a.item,
			b.name,
			b.item))
	}

	equal, difference = compareTypes(a, b)
	if !equal {
		return equal, difference
	}

	switch a.item.Kind() {
	case reflect.Array, reflect.Slice:
		return deepCompareArrays(a, b, depth)
	case reflect.Map:
		return deepCompareMaps(a, b, depth)
	case reflect.Struct:
		return deepCompareStructs(a, b, depth)
	case reflect.Ptr, reflect.Interface:
		return deepComparePointers(a, b, depth)
	case reflect.Int, reflect.String, reflect.Bool:
		return comparePrimitives(a, b, depth)
	default:
		panic(fmt.Sprintf("unrecognized type %v of value %#v\n", a.item.Kind(), a.item))

	}
}

// compares two pointers or interfaces in the same manner as reflect.deepEqual
func deepComparePointers(a namedValue, b namedValue, depth int) (equal bool, difference string) {
	if a.item.IsNil() || b.item.IsNil() {
		if a.item.IsNil() && b.item.IsNil() {
			return same()
		} else {

			if a.item.IsNil() {
				return different(fmt.Sprintf("%s is nil whereas %s = %#v", a.name, b.name, b.item))
			} else {
				return different(fmt.Sprintf("%s is nil whereas %s = %#v", b.name, a.name, a.item))
			}
		}

	}
	return deepCompareGeneric(namedValue{a.name, a.item.Elem()},
		namedValue{b.name, b.item.Elem()},
		depth)
}

// compares two non-struct items
func comparePrimitives(actual namedValue, expected namedValue, depth int) (equal bool, difference string) {
	// TODO can we find a simpler implementation of this method that still works with private fields?
	var a, b interface{}
	if actual.item.Kind() != expected.item.Kind() {
		return different(fmt.Sprintf("%s is of type %s whereas %s is of type %s", actual.name, actual.item.Kind(), expected.name, expected.item.Kind()))
	}
	switch actual.item.Kind() {
	case reflect.String:
		a = actual.item.String()
		b = expected.item.String()
	case reflect.Bool:
		a = actual.item.Bool()
		b = expected.item.Bool()
	case reflect.Int:
		a = actual.item.Int()
		b = expected.item.Int()
	default:
		panic(fmt.Sprintf("unrecognized types, %s (%#v) and %s (%#v)", actual.name, actual.item, expected.name, expected.item))
	}
	if reflect.DeepEqual(a, b) {
		return same()
	} else {
		return different(fmt.Sprintf("%s = %#v whereas %s = %#v", actual.name, actual.item, expected.name, expected.item))
	}
}

// converts a reflect.Value to a string describing its contents
func printReflectValue(item reflect.Value) (text string) {
	return fmt.Sprintf("%v (%s)", item, item.Type())
}

// compares two maps in the same manner as reflect.DeepEqual
func deepCompareMaps(a namedValue, b namedValue, depth int) (equal bool, difference string) {
	equal, difference = checkForMissingKeys(a, b, depth)
	if !equal {
		return equal, difference
	}
	equal, difference = checkForMissingKeys(b, a, depth)
	if !equal {
		return equal, difference
	}

	aItem := a.item
	bItem := b.item
	aKeys := aItem.MapKeys()
	differences := []string{}
	for _, key := range aKeys {
		keyText := fmt.Sprintf("[%s]", printReflectValue(key))

		equal, difference = deepCompareGeneric(namedValue{a.name + keyText, aItem.MapIndex(key)},
			namedValue{b.name + keyText, bItem.MapIndex(key)},
			depth,
		)
		if !equal {
			differences = append(differences, difference)
		}
	}
	if len(differences) > 0 {
		sort.Strings(differences)
		return false, differences[0]
	}
	return same()
}

// a helper for deepCompareMaps, which checks for keys in a that don't exist in b
func checkForMissingKeys(a namedValue, b namedValue, depth int) (equal bool, difference string) {
	aItem := a.item
	bItem := b.item
	aKeys := aItem.MapKeys()
	bKeys := bItem.MapKeys()
	differences := []string{}
	for _, aKey := range aKeys {
		if !bItem.MapIndex(aKey).IsValid() {
			aValue := aItem.MapIndex(aKey)
			differences = append(differences, fmt.Sprintf(
				"%v contains key %#v (%v, corresponding value = %#v) but %v does not.\n"+
					"%v contains %v keys and %v contains %v keys.",
				a.name, aKey, aKey, aValue, b.name, a.name, len(aKeys), b.name, len(bKeys)))
		}
	}
	// Unfortunately, a Go map doesn't have a deterministic iteration order,
	// but we want a deterministic, concise description of the differences between two maps
	// So, we use the alphabetically first difference
	if len(differences) > 0 {
		sort.Strings(differences)
		return false, differences[0]
	}
	return same()
}

// compares two arrays or slices (this ignores cap(), and only compares elements from indices 0 to len() )
func deepCompareArrays(a namedValue, b namedValue, depth int) (equal bool, difference string) {
	aItem := a.item
	bItem := b.item
	aLen := aItem.Len()
	bLen := bItem.Len()
	sharedLen := min(aLen, bLen)
	for i := 0; i < sharedLen; i++ {
		keyText := fmt.Sprintf("[%v]", i)
		equal, difference = deepCompareGeneric(namedValue{a.name + keyText, aItem.Index(i)},
			namedValue{b.name + keyText, bItem.Index(i)},
			depth,
		)
		if !equal {
			return equal, difference
		}
	}
	if aLen != bLen {
		var mismatchIndex int
		var mismatchName string
		var mismatchValue interface{}
		if aLen < bLen {
			mismatchIndex = aLen
			mismatchName = b.name
			mismatchValue = b.item.Index(mismatchIndex)
		} else {
			mismatchIndex = bLen
			mismatchName = a.name
			mismatchValue = a.item.Index(mismatchIndex)
		}
		return different(fmt.Sprintf("%s.len() = %v whereas %s.len() = %#v.\n\nFirst differing item: %s[%v] = %#v",
			a.name, aLen, b.name, bLen, mismatchName, mismatchIndex, mismatchValue))
	}
	return same()
}

// compares two structs
func deepCompareStructs(a namedValue, b namedValue, depth int) (equal bool, difference string) {
	aItem := a.item
	bItem := b.item
	aCount := aItem.NumField()
	equal, difference = compareTypes(a, b)
	if !equal {
		return equal, difference
	}
	aType := aItem.Type()
	for i := 0; i < aCount; i++ {
		fieldName := aType.Field(i).Name
		equal, difference = deepCompareGeneric(namedValue{a.name + "." + fieldName, aItem.Field(i)},
			namedValue{b.name + "." + fieldName, bItem.Field(i)},
			depth)
		if !equal {
			return equal, difference
		}
	}
	return same()
}

func min(a int, b int) (result int) {
	if a < b {
		return a
	} else {
		return b
	}
}
