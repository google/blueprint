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
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

func PropertyNameForField(fieldName string) string {
	r, size := utf8.DecodeRuneInString(fieldName)
	propertyName := string(unicode.ToLower(r))
	if len(fieldName) > size {
		propertyName += fieldName[size:]
	}
	return propertyName
}

func FieldNameForProperty(propertyName string) string {
	r, size := utf8.DecodeRuneInString(propertyName)
	fieldName := string(unicode.ToUpper(r))
	if len(propertyName) > size {
		fieldName += propertyName[size:]
	}
	return fieldName
}

func HasTag(field reflect.StructField, name, value string) bool {
	tag := field.Tag.Get(name)
	for _, entry := range strings.Split(tag, ",") {
		if entry == value {
			return true
		}
	}

	return false
}
