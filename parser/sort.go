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

import (
	"sort"
)

func SortLists(tree *ParseTree) {
	for _, def := range tree.SyntaxTree.nodes {
		if assignment, ok := def.(*Assignment); ok {
			sortListsInValue(assignment.Value, tree)
		} else if module, ok := def.(*Module); ok {
			for _, prop := range module.Properties {
				sortListsInValue(prop.Value, tree)
			}
		}
	}
}

func SortList(tree *ParseTree, list *List) {
	for i := 0; i < len(list.Values); i++ {
		// Find a set of values on contiguous lines
		iLine := tree.GetSourcePosition(list.Values[i]).Line
		var j int
		for j = i + 1; j < len(list.Values); j++ {
			jLine := tree.GetSourcePosition(list.Values[j]).Line
			if jLine > iLine+1 {
				break
			}
			iLine = jLine
		}

		sortSubList(list.Values[i:j], tree)
		i = j - 1
	}
}

func ListIsSorted(tree *ParseTree, list *List) bool {
	for i := 0; i < len(list.Values); i++ {
		// Find a set of values on contiguous lines
		iLine := tree.GetSourcePosition(list.Values[i]).Line
		var j int
		for j = i + 1; j < len(list.Values); j++ {
			jLine := tree.GetSourcePosition(list.Values[j]).Line
			if jLine > iLine+1 {
				break
			}
			iLine = jLine
		}

		if !subListIsSorted(list.Values[i:j]) {
			return false
		}
		i = j - 1
	}

	return true
}

func sortListsInValue(value Expression, tree *ParseTree) {
	switch v := value.(type) {
	case *Variable:
		// Nothing
	case *Operator:
		sortListsInValue(v.Args[0], tree)
		sortListsInValue(v.Args[1], tree)
	case *Map:
		for _, p := range v.Properties {
			sortListsInValue(p.Value, tree)
		}
	case *List:
		SortList(tree, v)
	}
}

func sortSubList(values []Expression, file *ParseTree) {
	// make a wrapper list to send into the built-in Sort function
	sortList := make(elemList, len(values))
	for i, v := range values {
		s, ok := v.(*String)
		if !ok {
			panic("list contains non-string element")
		}
		sortList[i] = elem{s.Value, i}
	}

	// call the built-in Sort function
	sort.Sort(sortList)

	// use the positions received by the built-in Sort function to re-order the given list
	clone := append([]Expression{}, values...)
	for i, sortNode := range sortList {
		values[i] = clone[sortNode.index]
	}
}

func subListIsSorted(values []Expression) bool {
	prev := ""
	for _, v := range values {
		s, ok := v.(*String)
		if !ok {
			panic("list contains non-string element")
		}
		if prev > s.Value {
			return false
		}
		prev = s.Value
	}

	return true
}

type elem struct {
	item  string
	index int
}

type elemList []elem

func (l elemList) Len() int {
	return len(l)
}

func (l elemList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l elemList) Less(i, j int) bool {
	return l[i].item < l[j].item
}
