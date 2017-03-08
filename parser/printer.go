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
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type lineState int

const (
	newlineState lineState = iota
	freshlyIndentedState
	textState
)

type printer struct {
	syntaxTree             *SyntaxTree
	output                 bytes.Buffer
	numIndents             int
	indentSize             int
	lineState              lineState
	commentsVisited        map[*Comment]bool // really Set<Comment>. Does Go implement Set<>?
	permitDanglingComments bool
	lastCharacter          byte
	spaceRequired          bool
}

func NewPrinter(tree *SyntaxTree) *printer {
	return &printer{tree, bytes.Buffer{}, 0, 4, newlineState,
		make(map[*Comment]bool, 0), false, ' ',
		false}
}

func Print(tree *ParseTree) string {
	p := NewPrinter(tree.SyntaxTree)
	return p.PrintTree()
}

func PrintTree(tree *SyntaxTree) string {
	p := NewPrinter(tree)
	return p.PrintTree()
}

func (p *printer) PrintTree() string {
	var prevNode ParseNode
	for _, node := range p.syntaxTree.nodes {
		_, wasModule := prevNode.(*Module)

		// the only case in which isComment should be true is when there is no more code after the module, only comments
		// If there was more code, then this comment should be attached to that other code instead
		comment, isComment := node.(*Comment)

		// if module is followed by something other than a newline, then add a newline
		if wasModule && !isComment {
			preComments := p.syntaxTree.GetComments(node).preComments
			var addNewline = true
			if len(preComments) > 0 {
				if preComments[0].Type == FullLineBlank {
					// already contains a newline comment in front of it
					addNewline = false
				}
			}
			if addNewline {
				p.printNewline()
			}
		}

		p.printNode(node)

		if !(isComment && comment.Type == InlineText) {
			// get an empty line to write on unless this is a comment
			p.getEmptyLine()
		}

		prevNode = node
	}
	p.getEmptyLine()
	if !p.permitDanglingComments {
		p.assertAllCommentsVisited()
	}
	return p.output.String()
}

func (p *printer) AllowDanglingComments() {
	p.permitDanglingComments = true
}

func (p *printer) assertAllCommentsVisited() {
	var printedComments = p.commentsVisited

	for parseNode, commentContainer := range p.syntaxTree.comments {
		for _, comment := range commentContainer.preComments {
			_, contains := printedComments[comment]
			if !contains {
				fmt.Println(fmt.Sprintf("error in printer.go; unvisited comment: %#v attached before %#v at %p\n", comment, parseNode, parseNode))
			}
		}
		for _, comment := range commentContainer.postComments {
			_, contains := printedComments[comment]
			if !contains {
				fmt.Println(fmt.Sprintf("error in printer.go; unvisited comment: %#v attached after %#v at %p\n", comment, parseNode, parseNode))
			}
		}
	}
}

// prints a ParseNode of unknown type, possibly with any comments before or after it
func (p *printer) printNode(node ParseNode) {
	p.beforePrint(node)
	p.printNodeWithoutComments(node)
	p.afterPrint(node)
}

func (p *printer) printNodeWithoutComments(node ParseNode) {
	switch node := node.(type) {
	case *Assignment:
		p.printAssignment(node)
	case *Module:
		p.printModule(node)
	case *String:
		p.printString(strconv.Quote(node.Value))
	case *Token:
		p.printString(node.Value)
	case *Variable:
		p.printNode(node.NameNode)
	case *Operator:
		p.printOperator(node)
	case *Bool:
		var s string
		if node.Value {
			s = "true"
		} else {
			s = "false"
		}
		p.printString(s)
	case *List:
		p.printList(node)
	case *Map:
		p.printMap(node)
	case *MapBody:
		p.printMapBody(node)
	case *Comment:
		p.printComment(node)
	default:
		panic(fmt.Sprintf("Unrecognized type %T for node %#v", node, node))
	}
}

func (p *printer) printAssignment(assignment *Assignment) {
	p.printNode(assignment.Name)
	p.printString(" ")
	p.printNode(&assignment.Assigner)
	p.printString(" ")
	p.printNode(assignment.OrigValue)
}

func (p *printer) printModule(module *Module) {
	p.printNode(module.Type)
	p.getSpace()
	p.printNode(module.Map)
}

func (p *printer) printList(list *List) {
	p.printString("[")
	p.incrementNextIndent()
	var addNewlines bool
	var numValues = len(list.Values)
	if numValues == 0 {
		// a list with 0 elements must not have a newline
		addNewlines = false
	} else if numValues == 1 {
		// a list with 1 element may or may not have a newline, so respect the request
		addNewlines = list.NewlineBetweenElements
	} else {
		// a list with 2 or more elements must have a newline
		addNewlines = true
	}
	for i, value := range list.Values {
		p.beforePrint(value)
		if addNewlines {
			p.getEmptyLine()
		}
		p.printNodeWithoutComments(value)
		if addNewlines {
			p.printString(",")
		} else {
			if i < len(list.Values)-1 {
				p.printString(", ")
			}
		}
		p.afterPrint(value)
	}
	p.decrementNextIndent()
	if addNewlines {
		p.getEmptyLine()
	}
	p.printString("]")
}

func (p *printer) printMap(m *Map) {
	p.printString("{")
	p.incrementNextIndent()
	if m.MapBody.Properties == nil {
		panic("nil mapBody")
	}
	p.printNode(m.MapBody)
	p.decrementNextIndent()
	if len(m.MapBody.Properties) > 0 || len(p.syntaxTree.GetComments(m.MapBody).preComments) > 0 || len(p.syntaxTree.GetComments(m.MapBody).postComments) > 0 {
		p.getEmptyLine()
	}
	p.printString("}")
}

// the reason that a MapBody is distinct from a Map is so that comments can be attached before it or after it and still show inside the map, even if the map is empty
func (p *printer) printMapBody(m *MapBody) {
	for _, property := range m.Properties {

		// these calls are unwrapped (instead of just doing printNode) to make sure that the trailing comma (after the property) shows up after any inline comments
		p.getEmptyLine()
		p.beforePrint(property)
		p.printString(property.Name)
		p.printString(":")
		p.printString(" ")

		// these calls are unwrapped (instead of just doing printNode) to make sure that spacing of inline comments inside the property works correctly
		p.beforePrint(property.Value)
		p.getSpace()
		p.printNodeWithoutComments(property.Value)
		p.afterPrint(property.Value)
		p.printString(",")

		p.afterPrint(property)
		p.getEmptyLine()
	}
}

func (p *printer) printOperator(operator *Operator) {
	p.printNode(operator.Args[0])
	p.requireSpace()
	p.printString(operator.OperatorToken.Value)
	p.requireSpace()
	p.printNode(operator.Args[1])
}

func (p *printer) beforePrint(node ParseNode) {
	var comments = p.syntaxTree.GetComments(node)
	for _, comment := range comments.preComments {
		p.printComment(comment)
	}
}
func (p *printer) afterPrint(node ParseNode) {
	var comments = p.syntaxTree.GetComments(node)
	for _, comment := range comments.postComments {
		p.printComment(comment)
	}
}

func (p *printer) printComment(comment *Comment) {
	switch comment.Type {
	case FullLineText:
		p.printFullLineCommentText(comment.Text)
	case InlineText:
		p.printInterLineCommentText(comment.Text)
	case FullLineBlank:
		p.printNewline()
	default:
		panic(fmt.Sprintf("unrecognized comment type %#v", comment))
	}
	p.commentsVisited[comment] = true
}
func (p *printer) printFullLineCommentText(text string) {
	p.getSpace()
	p.printString("//" + text)
	p.getEmptyLine()
}

func (p *printer) printInterLineCommentText(text string) {
	p.getSpace()
	// print each line one at a time and reformat any preceding or trailing spaces
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		// print separators
		if i == 0 {
			p.printString("/*")
		} else {
			p.printNewline()
		}

		if i != 0 {
			// if it's not the first line, then remove any preceding spaces before the indent
			for j := 0; i < p.getIndentLength() && strings.HasPrefix(line, " "); j++ {
				line = line[1:]
			}
		}
		if i != len(lines)-1 {
			// remove trailing spaces on a line other than the last
			for strings.HasSuffix(line, " ") {
				line = line[:len(line)-1]
			}
		}

		// print the line and indent it correctly
		p.printString(line)
	}
	p.printString("*/")
	p.requireSpace()
}

func (p *printer) getEmptyLine() {
	if p.lineState != newlineState {
		p.printNewline()
	}
}
func (p *printer) getSpace() {
	if p.lineContainsText() && !p.lastCharacterEquals(' ') {
		p._print(" ")
	}
}
func (p *printer) requireSpace() {
	p.spaceRequired = true
}
func (p *printer) lastCharacterEquals(matcher byte) (matches bool) {
	return p.lastCharacter == matcher
}
func (p *printer) ensureIndented() {
	if p.lineState == newlineState {
		p._print(p.getIndent())
		p.lineState = freshlyIndentedState
	}
}
func (p *printer) lineContainsText() bool {
	return p.lineState == textState
}

func (p *printer) printNewline() {
	p._print("\n")
	p.lineState = newlineState
}

// Prints some text and doesn't do any formatting other than possibly indenting
func (p *printer) printString(s string) {
	if len(s) > 0 {
		p.ensureIndented()
		if p.spaceRequired {
			p.getSpace()
			p.spaceRequired = false
		}
		p._print(s)
		p.lineState = textState
	}
}

// Prints some text and doesn't do any formatting at all
func (p *printer) _print(s string) {
	p.output.WriteString(s)
	if len(s) > 0 {
		p.lastCharacter = s[len(s)-1]
	}
}

func (p *printer) incrementNextIndent() {
	p.numIndents += 1
}
func (p *printer) decrementNextIndent() {
	p.numIndents -= 1
}
func (p *printer) getIndentLength() int {
	return p.numIndents * p.indentSize
}
func (p *printer) getIndent() string {
	return strings.Repeat(" ", p.getIndentLength())
}

// a VerbosePrinter prints a syntax tree and is intended for debugging purposes
type VerbosePrinter struct {
	syntaxTree *SyntaxTree
	output     bytes.Buffer
	indent     int
}

func VerbosePrint(tree *SyntaxTree) string {
	return NewVerbosePrinter(tree).Print()
}

func NewVerbosePrinter(tree *SyntaxTree) (p *VerbosePrinter) {
	return &VerbosePrinter{tree, bytes.Buffer{}, 0}
}

func (p *VerbosePrinter) Print() string {
	for _, node := range p.syntaxTree.nodes {
		p.printNode(node)
	}
	return p.output.String()
}

func (p *VerbosePrinter) printNode(parseNode ParseNode) {
	comments := p.syntaxTree.GetComments(parseNode)

	p.printComments("Pre-comments", comments.preComments)

	p.printIndent()
	p.printString(fmt.Sprintf(
		`Node %T@%p
`, parseNode, parseNode))

	p.indent++
	for _, child := range parseNode.Children() {
		p.printNode(child)
	}
	p.indent--

	p.printComments("Post-comments", comments.postComments)
}

func (p *VerbosePrinter) printComments(description string, comments []*Comment) {
	if len(comments) > 0 {
		p.printIndent()
		p.printString(fmt.Sprintf("%s: [", description))
		for _, comment := range comments {
			p.printString(fmt.Sprintf("%#v,", comment.Text))
		}
		p.printString("]\n")
	}
}
func (p *VerbosePrinter) printString(text string) {
	p.output.WriteString(text)
}
func (p *VerbosePrinter) printIndent() {
	p.printString(strings.Repeat(" ", p.indent*2))
}
