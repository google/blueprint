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

import "fmt"

func AddStringToList(list *List, s string) (modified bool) {
	for _, v := range list.Values {
		if v.Type() != StringType {
			panic(fmt.Errorf("expected string in list, got %s", v.Type()))
		}

		if sv, ok := v.(*String); ok && sv.Value == s {
			// string already exists
			return false
		}
	}

	list.Values = append(list.Values, &String{
		LiteralPos: list.RBracePos,
		Value:      s,
	})

	return true
}

func RemoveStringFromList(list *List, s string) (modified bool) {
	for i, v := range list.Values {
		if v.Type() != StringType {
			panic(fmt.Errorf("expected string in list, got %s", v.Type()))
		}

		if sv, ok := v.(*String); ok && sv.Value == s {
			list.Values = append(list.Values[:i], list.Values[i+1:]...)
			return true
		}
	}

	return false
}
