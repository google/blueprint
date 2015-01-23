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

func SortList(file *File, value Value) {
	for i := 0; i < len(value.ListValue); i++ {
		// Find a set of values on contiguous lines
		line := value.ListValue[i].Pos.Line
		var j int
		for j = i + 1; j < len(value.ListValue); j++ {
			if value.ListValue[j].Pos.Line > line+1 {
				break
			}
			line = value.ListValue[j].Pos.Line
		}

		nextPos := value.EndPos
		if j < len(value.ListValue) {
			nextPos = value.ListValue[j].Pos
		}
		sortSubList(value.ListValue[i:j], nextPos, file)
		i = j - 1
	}
}

func ListIsSorted(value Value) bool {
	for i := 0; i < len(value.ListValue); i++ {
		// Find a set of values on contiguous lines
		line := value.ListValue[i].Pos.Line
		var j int
		for j = i + 1; j < len(value.ListValue); j++ {
			if value.ListValue[j].Pos.Line > line+1 {
				break
			}
			line = value.ListValue[j].Pos.Line
		}

		if !subListIsSorted(value.ListValue[i:j]) {
			return false
		}
		i = j - 1
	}

	return true
}

func sortListsInValue(value Value, file *File) {
	if value.Variable != "" {
		return
	}

	if value.Expression != nil {
		sortListsInValue(value.Expression.Args[0], file)
		sortListsInValue(value.Expression.Args[1], file)
		return
	}

	if value.Type == Map {
		for _, p := range value.MapValue {
			sortListsInValue(p.Value, file)
		}
		return
	} else if value.Type != List {
		return
	}

	SortList(file, value)
}

func sortSubList(values []Value, nextPos scanner.Position, file *File) {
	l := make(elemList, len(values))
	for i, v := range values {
		if v.Type != String {
			panic("list contains non-string element")
		}
		n := nextPos
		if i < len(values)-1 {
			n = values[i+1].Pos
		}
		l[i] = elem{v.StringValue, i, v.Pos, n}
	}

	sort.Sort(l)

	copyValues := append([]Value{}, values...)
	copyComments := append([]Comment{}, file.Comments...)

	curPos := values[0].Pos
	for i, e := range l {
		values[i] = copyValues[e.i]
		values[i].Pos = curPos
		for j, c := range copyComments {
			if c.Pos.Offset > e.pos.Offset && c.Pos.Offset < e.nextPos.Offset {
				file.Comments[j].Pos.Line = curPos.Line
				file.Comments[j].Pos.Offset += values[i].Pos.Offset - e.pos.Offset
			}
		}

		curPos.Offset += e.nextPos.Offset - e.pos.Offset
		curPos.Line++
	}
}

func subListIsSorted(values []Value) bool {
	prev := ""
	for _, v := range values {
		if v.Type != String {
			panic("list contains non-string element")
		}
		if prev > v.StringValue {
			return false
		}
		prev = v.StringValue
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

type commentsByOffset []Comment

func (l commentsByOffset) Len() int {
	return len(l)
}

func (l commentsByOffset) Less(i, j int) bool {
	return l[i].Pos.Offset < l[j].Pos.Offset
}

func (l commentsByOffset) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
