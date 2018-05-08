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
	"unicode"
)

var noPos scanner.Position

type printer struct {
	defs     []Definition
	comments []*CommentGroup

	curComment int

	pos scanner.Position

	pendingSpace   bool
	pendingNewline int

	output []byte

	indentList []int
	wsBuf      []byte

	skippedComments []*CommentGroup
}

func newPrinter(file *File) *printer {
	return &printer{
		defs:       file.Defs,
		comments:   file.Comments,
		indentList: []int{0},

		// pendingNewLine is initialized to -1 to eat initial spaces if the first token is a comment
		pendingNewline: -1,

		pos: scanner.Position{
			Line: 1,
		},
	}
}

func Print(file *File) ([]byte, error) {
	p := newPrinter(file)

	for _, def := range p.defs {
		p.printDef(def)
	}
	p.flush()
	return p.output, nil
}

func PrintExpression(expression Expression) ([]byte, error) {
	dummyFile := &File{}
	p := newPrinter(dummyFile)
	p.printExpression(expression)
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
	p.printToken(assignment.Name, assignment.NamePos)
	p.requestSpace()
	p.printToken(assignment.Assigner, assignment.EqualsPos)
	p.requestSpace()
	p.printExpression(assignment.OrigValue)
	p.requestNewline()
}

func (p *printer) printModule(module *Module) {
	p.printToken(module.Type, module.TypePos)
	p.printMap(&module.Map)
	p.requestDoubleNewline()
}

func (p *printer) printExpression(value Expression) {
	switch v := value.(type) {
	case *Variable:
		p.printToken(v.Name, v.NamePos)
	case *Operator:
		p.printOperator(v)
	case *Bool:
		var s string
		if v.Value {
			s = "true"
		} else {
			s = "false"
		}
		p.printToken(s, v.LiteralPos)
	case *Int64:
		p.printToken(strconv.FormatInt(v.Value, 10), v.LiteralPos)
	case *String:
		p.printToken(strconv.Quote(v.Value), v.LiteralPos)
	case *List:
		p.printList(v.Values, v.LBracePos, v.RBracePos)
	case *Map:
		p.printMap(v)
	default:
		panic(fmt.Errorf("bad property type: %s", value.Type()))
	}
}

func (p *printer) printList(list []Expression, pos, endPos scanner.Position) {
	p.requestSpace()
	p.printToken("[", pos)
	if len(list) > 1 || pos.Line != endPos.Line {
		p.requestNewline()
		p.indent(p.curIndent() + 4)
		for _, value := range list {
			p.printExpression(value)
			p.printToken(",", noPos)
			p.requestNewline()
		}
		p.unindent(endPos)
	} else {
		for _, value := range list {
			p.printExpression(value)
		}
	}
	p.printToken("]", endPos)
}

func (p *printer) printMap(m *Map) {
	p.requestSpace()
	p.printToken("{", m.LBracePos)
	if len(m.Properties) > 0 || m.LBracePos.Line != m.RBracePos.Line {
		p.requestNewline()
		p.indent(p.curIndent() + 4)
		for _, prop := range m.Properties {
			p.printProperty(prop)
			p.printToken(",", noPos)
			p.requestNewline()
		}
		p.unindent(m.RBracePos)
	}
	p.printToken("}", m.RBracePos)
}

func (p *printer) printOperator(operator *Operator) {
	p.printOperatorInternal(operator, true)
}

func (p *printer) printOperatorInternal(operator *Operator, allowIndent bool) {
	p.printExpression(operator.Args[0])
	p.requestSpace()
	p.printToken(string(operator.Operator), operator.OperatorPos)

	indented := false
	if operator.Args[0].End().Line == operator.Args[1].Pos().Line {
		p.requestSpace()
	} else {
		if allowIndent {
			indented = true
			p.indent(p.curIndent() + 4)
		}
		p.requestNewline()
	}

	if op, isOp := operator.Args[1].(*Operator); isOp {
		p.printOperatorInternal(op, false)
	} else {
		p.printExpression(operator.Args[1])
	}

	if indented {
		p.unindent(p.pos)
	}
}

func (p *printer) printProperty(property *Property) {
	p.printToken(property.Name, property.NamePos)
	p.printToken(":", property.ColonPos)
	p.requestSpace()
	p.printExpression(property.Value)
}

// Print a single token, including any necessary comments or whitespace between
// this token and the previously printed token
func (p *printer) printToken(s string, pos scanner.Position) {
	newline := p.pendingNewline != 0

	if pos == noPos {
		pos = p.pos
	}

	if newline {
		p.printEndOfLineCommentsBefore(pos)
		p.requestNewlinesForPos(pos)
	}

	p.printInLineCommentsBefore(pos)

	p.flushSpace()

	p.output = append(p.output, s...)

	p.pos = pos
}

// Print any in-line (single line /* */) comments that appear _before_ pos
func (p *printer) printInLineCommentsBefore(pos scanner.Position) {
	for p.curComment < len(p.comments) && p.comments[p.curComment].Pos().Offset < pos.Offset {
		c := p.comments[p.curComment]
		if c.Comments[0].Comment[0][0:2] == "//" || len(c.Comments[0].Comment) > 1 {
			p.skippedComments = append(p.skippedComments, c)
		} else {
			p.printComment(c)
			p.requestSpace()
		}
		p.curComment++
	}
}

// Print any comments, including end of line comments, that appear _before_ the line specified
// by pos
func (p *printer) printEndOfLineCommentsBefore(pos scanner.Position) {
	if len(p.skippedComments) > 0 {
		for _, c := range p.skippedComments {
			p.printComment(c)
		}
		p._requestNewline()
		p.skippedComments = nil
	}
	for p.curComment < len(p.comments) && p.comments[p.curComment].Pos().Line < pos.Line {
		c := p.comments[p.curComment]
		p.printComment(c)
		p._requestNewline()
		p.curComment++
	}
}

// Compare the line numbers of the previous and current positions to determine whether extra
// newlines should be inserted.  A second newline is allowed anywhere requestNewline() is called.
func (p *printer) requestNewlinesForPos(pos scanner.Position) bool {
	if pos.Line > p.pos.Line {
		p._requestNewline()
		if pos.Line > p.pos.Line+1 {
			p.pendingNewline = 2
		}
		return true
	}

	return false
}

func (p *printer) requestSpace() {
	p.pendingSpace = true
}

// Ask for a newline to be inserted before the next token, but do not insert any comments.  Used
// by the comment printers.
func (p *printer) _requestNewline() {
	if p.pendingNewline == 0 {
		p.pendingNewline = 1
	}
}

// Ask for a newline to be inserted before the next token.  Also inserts any end-of line comments
// for the current line
func (p *printer) requestNewline() {
	pos := p.pos
	pos.Line++
	p.printEndOfLineCommentsBefore(pos)
	p._requestNewline()
}

// Ask for two newlines to be inserted before the next token.  Also inserts any end-of line comments
// for the current line
func (p *printer) requestDoubleNewline() {
	p.requestNewline()
	p.pendingNewline = 2
}

// Flush any pending whitespace, ignoring pending spaces if there is a pending newline
func (p *printer) flushSpace() {
	if p.pendingNewline == 1 {
		p.output = append(p.output, '\n')
		p.pad(p.curIndent())
	} else if p.pendingNewline == 2 {
		p.output = append(p.output, "\n\n"...)
		p.pad(p.curIndent())
	} else if p.pendingSpace == true && p.pendingNewline != -1 {
		p.output = append(p.output, ' ')
	}

	p.pendingSpace = false
	p.pendingNewline = 0
}

// Print a single comment, which may be a multi-line comment
func (p *printer) printComment(cg *CommentGroup) {
	for _, comment := range cg.Comments {
		if !p.requestNewlinesForPos(comment.Pos()) {
			p.requestSpace()
		}
		for i, line := range comment.Comment {
			line = strings.TrimRightFunc(line, unicode.IsSpace)
			p.flushSpace()
			if i != 0 {
				lineIndent := strings.IndexFunc(line, func(r rune) bool { return !unicode.IsSpace(r) })
				lineIndent = max(lineIndent, p.curIndent())
				p.pad(lineIndent - p.curIndent())
			}
			p.output = append(p.output, strings.TrimSpace(line)...)
			if i < len(comment.Comment)-1 {
				p._requestNewline()
			}
		}
		p.pos = comment.End()
	}
}

// Print any comments that occur after the last token, and a trailing newline
func (p *printer) flush() {
	for _, c := range p.skippedComments {
		if !p.requestNewlinesForPos(c.Pos()) {
			p.requestSpace()
		}
		p.printComment(c)
	}
	for p.curComment < len(p.comments) {
		p.printComment(p.comments[p.curComment])
		p.curComment++
	}
	p.output = append(p.output, '\n')
}

// Print whitespace to pad from column l to column max
func (p *printer) pad(l int) {
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

func (p *printer) unindent(pos scanner.Position) {
	p.printEndOfLineCommentsBefore(pos)
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
