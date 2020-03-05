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
	"text/scanner"
)

func SortLists(file *File) {
	for _, def := range file.Defs {
		if assignment, ok := def.(*Assignment); ok {
			sortListsInValue(assignment.Value, file)
		} else if module, ok := def.(*Module); ok {
			for _, prop := range module.Properties {
				sortListsInValue(prop.Value, file)
			}
		}
	}
	sort.Sort(commentsByOffset(file.Comments))
}

func SortList(file *File, list *List) {
	if !isListOfPrimitives(list.Values) {
		return
	}
	for i := 0; i < len(list.Values); i++ {
		// Find a set of values on contiguous lines
		line := list.Values[i].Pos().Line
		var j int
		for j = i + 1; j < len(list.Values); j++ {
			if list.Values[j].Pos().Line > line+1 {
				break
			}
			line = list.Values[j].Pos().Line
		}

		nextPos := list.End()
		if j < len(list.Values) {
			nextPos = list.Values[j].Pos()
		}
		sortSubList(list.Values[i:j], nextPos, file)
		i = j - 1
	}
}

func ListIsSorted(list *List) bool {
	for i := 0; i < len(list.Values); i++ {
		// Find a set of values on contiguous lines
		line := list.Values[i].Pos().Line
		var j int
		for j = i + 1; j < len(list.Values); j++ {
			if list.Values[j].Pos().Line > line+1 {
				break
			}
			line = list.Values[j].Pos().Line
		}

		if !subListIsSorted(list.Values[i:j]) {
			return false
		}
		i = j - 1
	}

	return true
}

func sortListsInValue(value Expression, file *File) {
	switch v := value.(type) {
	case *Variable:
		// Nothing
	case *Operator:
		sortListsInValue(v.Args[0], file)
		sortListsInValue(v.Args[1], file)
	case *Map:
		for _, p := range v.Properties {
			sortListsInValue(p.Value, file)
		}
	case *List:
		SortList(file, v)
	}
}

func sortSubList(values []Expression, nextPos scanner.Position, file *File) {
	if !isListOfPrimitives(values) {
		return
	}
	l := make(elemList, len(values))
	for i, v := range values {
		s, ok := v.(*String)
		if !ok {
			panic("list contains non-string element")
		}
		n := nextPos
		if i < len(values)-1 {
			n = values[i+1].Pos()
		}
		l[i] = elem{s.Value, i, v.Pos(), n}
	}

	sort.Sort(l)

	copyValues := append([]Expression{}, values...)
	copyComments := make([]*CommentGroup, len(file.Comments))
	for i := range file.Comments {
		cg := *file.Comments[i]
		cg.Comments = make([]*Comment, len(cg.Comments))
		for j := range file.Comments[i].Comments {
			c := *file.Comments[i].Comments[j]
			cg.Comments[j] = &c
		}
		copyComments[i] = &cg
	}

	curPos := values[0].Pos()
	for i, e := range l {
		values[i] = copyValues[e.i]
		values[i].(*String).LiteralPos = curPos
		for j, c := range copyComments {
			if c.Pos().Offset > e.pos.Offset && c.Pos().Offset < e.nextPos.Offset {
				file.Comments[j].Comments[0].Slash.Line = curPos.Line
				file.Comments[j].Comments[0].Slash.Offset += values[i].Pos().Offset - e.pos.Offset
			}
		}

		curPos.Offset += e.nextPos.Offset - e.pos.Offset
		curPos.Line++
	}
}

func subListIsSorted(values []Expression) bool {
	if !isListOfPrimitives(values) {
		return true
	}
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
	s       string
	i       int
	pos     scanner.Position
	nextPos scanner.Position
}

type elemList []elem

func (l elemList) Len() int {
	return len(l)
}

func (l elemList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l elemList) Less(i, j int) bool {
	return l[i].s < l[j].s
}

type commentsByOffset []*CommentGroup

func (l commentsByOffset) Len() int {
	return len(l)
}

func (l commentsByOffset) Less(i, j int) bool {
	return l[i].Pos().Offset < l[j].Pos().Offset
}

func (l commentsByOffset) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func isListOfPrimitives(values []Expression) bool {
	if len(values) == 0 {
		return true
	}
	switch values[0].Type() {
	case BoolType, StringType, Int64Type:
		return true
	default:
		return false
	}
}
