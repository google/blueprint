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

package parser

func AddStringToList(value *Value, s string) (modified bool) {
	if value.Type != List {
		panic("expected list value, got " + value.Type.String())
	}

	for _, v := range value.ListValue {
		if v.Type != String {
			panic("expected string in list, got " + value.Type.String())
		}

		if v.StringValue == s {
			// string already exists
			return false
		}

	}

	value.ListValue = append(value.ListValue, Value{
		Type:        String,
		Pos:         value.EndPos,
		StringValue: s,
	})

	return true
}

func RemoveStringFromList(value *Value, s string) (modified bool) {
	if value.Type != List {
		panic("expected list value, got " + value.Type.String())
	}

	for i, v := range value.ListValue {
		if v.Type != String {
			panic("expected string in list, got " + value.Type.String())
		}

		if v.StringValue == s {
			value.ListValue = append(value.ListValue[:i], value.ListValue[i+1:]...)
			return true
		}

	}

	return false
}
