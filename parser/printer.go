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
}

func NewPrinter(tree *SyntaxTree) *printer {
	return &printer{tree, bytes.Buffer{}, 0, 4, newlineState, make(map[*Comment]bool, 0), false}
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
				//fmt.Println("printing newline within PrintTree")
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
	p.printSpace()
	p.printNode(module.Map)
	p.getEmptyLine()
}

func (p *printer) printList(list *List) {
	p.printString("[")
	p.incrementNextIndent()
	var addNewlines = list.NewlineBetweenElements && (len(list.Values) > 0)
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
	/*if len(m.Properties) > 0 {
		p.printNewline()
	}*/
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

// the reason that a MapBody is distinct from a Map is so that comments can be attached before it or after it and still render inside the map, even if the map is empty
func (p *printer) printMapBody(m *MapBody) {
	for _, property := range m.Properties {
		p.getEmptyLine()
		p.beforePrint(property)
		p.printString(property.Name)
		p.printString(":")
		p.printString(" ")
		p.printNode(property.Value)
		p.printString(",")
		p.afterPrint(property)
		p.getEmptyLine()
	}
}

func (p *printer) printOperator(operator *Operator) {
	p.printNode(operator.Args[0])
	p.printString(" ")
	p.printString(operator.OperatorToken.Value)
	p.printString(" ")
	p.printNode(operator.Args[1])
}

//func (p *printer) printPropertyNode(property *Property, includeComma bool) {
//	p.beforePrint(property)
//	p.printString(property.Name)
//	p.printString(":")
//	p.printString(" ")
//	p.printNode(property.Value)
//	if includeComma {
//		p.printString(",")
//	}
//	p.afterPrint(property)
//}

func (p *printer) beforePrint(node ParseNode) {
	var comments = p.syntaxTree.GetComments(node)
	//fmt.Println(fmt.Sprintf("printing %v pre-comments for %v\n", len(comments.preComments), node))
	for _, comment := range comments.preComments {
		//fmt.Println(fmt.Sprintf("printing '%s' pre-comment for %v\n", comment.Text, node))
		p.printComment(comment)
	}
}
func (p *printer) afterPrint(node ParseNode) {
	var comments = p.syntaxTree.GetComments(node)
	for _, comment := range comments.postComments {
		p.printComment(comment)
	}
	//p.printString(fmt.Sprintf("done printing %v", node))
}

func (p *printer) printComment(comment *Comment) {
	switch comment.Type {
	case FullLineText:
		p.printFullLineCommentText(comment.Text)
	case InlineText:
		p.printInterLineCommentText(comment.Text)
	case FullLineBlank:
		//fmt.Println("printing newline comment")
		p.printNewline()
	default:
		panic(fmt.Sprintf("unrecognized comment type %#v", comment))
	}
	p.commentsVisited[comment] = true
}
func (p *printer) printFullLineCommentText(text string) {
	//fmt.Println(fmt.Printf("printing slash slash text '%s'", text))
	p.getSpace()
	p.printString("//" + text)
	p.getEmptyLine()
}

func (p *printer) printInterLineCommentText(text string) {
	//fmt.Println(fmt.Printf("printing slash star text '%s'", text))

	p.getSpace()
	// print each line one at a time and reformat any preceding or trailing spaces
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		// print separators
		if i == 0 {
			p.printString("/*")
		} else {
			//fmt.Println("printing newline within comment")
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
}

func (p *printer) getEmptyLine() {
	//fmt.Println("getting empty line")
	if p.lineState != newlineState && p.lineState != freshlyIndentedState {
		p.printNewline()
	}
}
func (p *printer) getSpace() {
	if p.lineContainsText() {
		p.printSpace()
	}
}
func (p *printer) ensureIndented() {
	if p.lineState == newlineState {
		p.lineState = freshlyIndentedState
		p._print(p.getIndent())
	}
}
func (p *printer) lineContainsText() bool {
	return p.lineState == textState
}

func (p *printer) printNewline() {
	p._print("\n")
	//fmt.Println("printing newline")
	p.lineState = newlineState
}

// Prints some text and doesn't do any formatting other than possibly indenting
func (p *printer) printString(s string) {
	p.ensureIndented()
	p._print(s)
	p.lineState = textState
}

// Prints some text and doesn't do any formatting at all
func (p *printer) _print(s string) {
	p.output.WriteString(s)
}
func (p *printer) printSpace() {
	p.printString(" ")
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
