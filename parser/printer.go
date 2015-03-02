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
	"fmt"
	"strconv"
	"strings"
	"text/scanner"
)

var noPos = scanner.Position{}

type whitespace int

const (
	wsDontCare whitespace = iota
	wsBoth
	wsAfter
	wsBefore
	wsCanBreak     // allow extra line breaks or comments
	wsBothCanBreak // wsBoth plus allow extra line breaks or comments
	wsForceBreak
	wsForceDoubleBreak
)

type tokenState struct {
	ws     whitespace
	token  string
	pos    scanner.Position
	indent int
}

type printer struct {
	defs     []Definition
	comments []Comment

	curComment int
	prev       tokenState

	output []byte

	indentList []int
	wsBuf      []byte

	skippedComments []Comment
}

func newPrinter(file *File) *printer {
	return &printer{
		defs:       file.Defs,
		comments:   file.Comments,
		indentList: []int{0},
		prev: tokenState{
			ws: wsCanBreak,
		},
	}
}

func Print(file *File) ([]byte, error) {
	p := newPrinter(file)

	for _, def := range p.defs {
		p.printDef(def)
	}
	p.prev.ws = wsDontCare
	p.flush()
	return p.output, nil
}

func (p *printer) Print() ([]byte, error) {
	for _, def := range p.defs {
		p.printDef(def)
	}
	p.flush()
	return p.output, nil
}

func (p *printer) printDef(def Definition) {
	if assignment, ok := def.(*Assignment); ok {
		p.printAssignment(assignment)
	} else if module, ok := def.(*Module); ok {
		p.printModule(module)
	} else {
		panic("Unknown definition")
	}
}

func (p *printer) printAssignment(assignment *Assignment) {
	p.printToken(assignment.Name.Name, assignment.Name.Pos, wsDontCare)
	p.printToken(assignment.Assigner, assignment.Pos, wsBoth)
	p.printValue(assignment.OrigValue)
	p.prev.ws = wsForceBreak
}

func (p *printer) printModule(module *Module) {
	p.printToken(module.Type.Name, module.Type.Pos, wsDontCare)
	p.printMap(module.Properties, module.LbracePos, module.RbracePos)
	p.prev.ws = wsForceDoubleBreak
}

func (p *printer) printValue(value Value) {
	if value.Variable != "" {
		p.printToken(value.Variable, value.Pos, wsDontCare)
	} else if value.Expression != nil {
		p.printExpression(*value.Expression)
	} else {
		switch value.Type {
		case Bool:
			var s string
			if value.BoolValue {
				s = "true"
			} else {
				s = "false"
			}
			p.printToken(s, value.Pos, wsDontCare)
		case String:
			p.printToken(strconv.Quote(value.StringValue), value.Pos, wsDontCare)
		case List:
			p.printList(value.ListValue, value.Pos, value.EndPos)
		case Map:
			p.printMap(value.MapValue, value.Pos, value.EndPos)
		default:
			panic(fmt.Errorf("bad property type: %d", value.Type))
		}
	}
}

func (p *printer) printList(list []Value, pos, endPos scanner.Position) {
	p.printToken("[", pos, wsBefore)
	if len(list) > 1 || pos.Line != endPos.Line {
		p.prev.ws = wsForceBreak
		p.indent(p.curIndent() + 4)
		for _, value := range list {
			p.printValue(value)
			p.printToken(",", noPos, wsForceBreak)
		}
		p.unindent()
	} else {
		for _, value := range list {
			p.printValue(value)
		}
	}
	p.printToken("]", endPos, wsDontCare)
}

func (p *printer) printMap(list []*Property, pos, endPos scanner.Position) {
	p.printToken("{", pos, wsBefore)
	if len(list) > 0 || pos.Line != endPos.Line {
		p.prev.ws = wsForceBreak
		p.indent(p.curIndent() + 4)
		for _, prop := range list {
			p.printProperty(prop)
			p.printToken(",", noPos, wsForceBreak)
		}
		p.unindent()
	}
	p.printToken("}", endPos, wsDontCare)
}

func (p *printer) printExpression(expression Expression) {
	p.printValue(expression.Args[0])
	p.printToken(string(expression.Operator), expression.Pos, wsBothCanBreak)
	p.printValue(expression.Args[1])
}

func (p *printer) printProperty(property *Property) {
	p.printToken(property.Name.Name, property.Name.Pos, wsDontCare)
	p.printToken(":", property.Pos, wsAfter)
	p.printValue(property.Value)
}

// Print a single token, including any necessary comments or whitespace between
// this token and the previously printed token
func (p *printer) printToken(s string, pos scanner.Position, ws whitespace) {
	this := tokenState{
		token:  s,
		pos:    pos,
		ws:     ws,
		indent: p.curIndent(),
	}

	if this.pos == noPos {
		this.pos = p.prev.pos
	}

	// Print the previously stored token
	allowLineBreak := false
	lineBreak := 0

	switch p.prev.ws {
	case wsBothCanBreak, wsCanBreak, wsForceBreak, wsForceDoubleBreak:
		allowLineBreak = true
	}

	p.output = append(p.output, p.prev.token...)

	commentIndent := max(p.prev.indent, this.indent)
	p.printComments(this.pos, allowLineBreak, commentIndent)

	switch p.prev.ws {
	case wsForceBreak:
		lineBreak = 1
	case wsForceDoubleBreak:
		lineBreak = 2
	}

	if allowLineBreak && p.prev.pos.IsValid() &&
		pos.Line-p.prev.pos.Line > lineBreak {
		lineBreak = pos.Line - p.prev.pos.Line
	}

	if lineBreak > 0 {
		p.printLineBreak(lineBreak, this.indent)
	} else {
		p.printWhitespace(this.ws)
	}

	p.prev = this
}

func (p *printer) printWhitespace(ws whitespace) {
	if p.prev.ws == wsBoth || ws == wsBoth ||
		p.prev.ws == wsBothCanBreak || ws == wsBothCanBreak ||
		p.prev.ws == wsAfter || ws == wsBefore {
		p.output = append(p.output, ' ')
	}
}

// Pr int all comments that occur before position pos
func (p *printer) printComments(pos scanner.Position, allowLineBreak bool, indent int) {
	if allowLineBreak {
		for _, c := range p.skippedComments {
			p.printComment(c, indent)
		}
		p.skippedComments = []Comment{}
	}
	for p.curComment < len(p.comments) && p.comments[p.curComment].Pos.Offset < pos.Offset {
		if !allowLineBreak && p.comments[p.curComment].Comment[0:2] == "//" {
			p.skippedComments = append(p.skippedComments, p.comments[p.curComment])
		} else {
			p.printComment(p.comments[p.curComment], indent)
		}
		p.curComment++
	}
}

// Print a single comment, which may be a multi-line comment
func (p *printer) printComment(comment Comment, indent int) {
	commentLines := strings.Split(comment.Comment, "\n")
	pos := comment.Pos
	for _, line := range commentLines {
		if p.prev.pos.IsValid() && pos.Line > p.prev.pos.Line {
			// Comment is on the next line
			if p.prev.ws == wsForceDoubleBreak {
				p.printLineBreak(2, indent)
				p.prev.ws = wsForceBreak
			} else {
				p.printLineBreak(pos.Line-p.prev.pos.Line, indent)
			}
		} else if p.prev.pos.IsValid() {
			// Comment is on the current line
			p.printWhitespace(wsBoth)
		}
		p.output = append(p.output, strings.TrimSpace(line)...)
		p.prev.pos = pos
		pos.Line++
	}
	if comment.Comment[0:2] == "//" {
		if p.prev.ws != wsForceDoubleBreak {
			p.prev.ws = wsForceBreak
		}
	} else {
		p.prev.ws = wsBothCanBreak
	}
}

// Print one or two line breaks.  n <= 0 is only valid if forceLineBreak is set,
// n > 2 is collapsed to a single blank line.
func (p *printer) printLineBreak(n, indent int) {
	if n > 2 {
		n = 2
	}

	for i := 0; i < n; i++ {
		p.output = append(p.output, '\n')
	}

	p.pad(0, indent)
}

// Print any comments that occur after the last token, and a trailing newline
func (p *printer) flush() {
	for _, c := range p.skippedComments {
		p.printComment(c, p.curIndent())
	}
	p.printToken("", noPos, wsDontCare)
	for p.curComment < len(p.comments) {
		p.printComment(p.comments[p.curComment], p.curIndent())
		p.curComment++
	}
	p.output = append(p.output, '\n')
}

// Print whitespace to pad from column l to column max
func (p *printer) pad(l, max int) {
	l = max - l
	if l > len(p.wsBuf) {
		p.wsBuf = make([]byte, l)
		for i := range p.wsBuf {
			p.wsBuf[i] = ' '
		}
	}
	p.output = append(p.output, p.wsBuf[0:l]...)
}

func (p *printer) indent(i int) {
	p.indentList = append(p.indentList, i)
}

func (p *printer) unindent() {
	p.indentList = p.indentList[0 : len(p.indentList)-1]
}

func (p *printer) curIndent() int {
	return p.indentList[len(p.indentList)-1]
}

func max(a, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}
