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
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/scanner"
)

var errTooManyErrors = errors.New("too many errors")

const maxErrors = 1

type ParseError struct {
	Err error
	Pos scanner.Position
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Err)
}

// A ParseTree is a SyntaxTree that knows where (filename and scanner.Position) its tokens came from
// A SyntaxTree is just a tree of nodes that don't know where they came from, as would be created by a programmatic builder
type ParseTree struct {
	FileName        string
	SyntaxTree      *SyntaxTree
	SourcePositions map[ParseNode]scanner.Position // Where the tokens were originally found in the source text
}

func (tree *ParseTree) SourcePosition(parseNode ParseNode) scanner.Position {
	return tree.SourcePositions[parseNode]
}

func (tree *ParseTree) HasSourcePosition(parseNode ParseNode) bool {
	_, found := tree.SourcePositions[parseNode]
	return found
}

// A SyntaxTreeBuilder builds a SyntaxTree.
// It doesn't store details about where (source positions) the tree came from
// To build a tree containing source information, use a ParseTreeBuilder
type SyntaxTreeBuilder struct {
	tree                   *SyntaxTree
	permitDanglingComments bool
	existingNodes          map[ParseNode]bool
}

func NewSyntaxTreeBuilder() *SyntaxTreeBuilder {
	return &SyntaxTreeBuilder{NewSyntaxTree(), false, make(map[ParseNode]bool, 0)}
}
func (s *SyntaxTreeBuilder) Add(parseNode ParseNode) *SyntaxTreeBuilder {
	newTree := NewSyntaxTree()
	newTree.AddNode(parseNode)
	for _, node := range newTree.ListOfAllNodes() {
		_, exists := s.existingNodes[node]
		if exists {
			panic(fmt.Sprintf("Attempted to add node %s which contains as a descendant %s, which already exists in the tree", parseNode, node))
		}
		s.existingNodes[node] = true
	}

	s.tree.AddNode(parseNode)
	return s
}

// It would be really nice to be able to use Generics instead of declaring a new builder method for each type
// The reason we have a new method for each type is to let parser_test.go concisely add comments
func (s *SyntaxTreeBuilder) Module(preComments []*Comment, Type *Token, Map *Map, postComments []*Comment) *Module {
	module := &Module{Type: Type, Map: Map}
	s.AppendPreComments(module, preComments)
	s.AppendPostComments(module, postComments)
	return module
}
func (s *SyntaxTreeBuilder) Token(preComments []*Comment, text string, postComments []*Comment) *Token {
	token := &Token{text}
	s.AppendPreComments(token, preComments)
	s.AppendPostComments(token, postComments)
	return token
}
func (s *SyntaxTreeBuilder) Map(preComments []*Comment, mapBody *MapBody, postComments []*Comment) *Map {
	Map := NewMapWithBody(mapBody)
	s.AppendPreComments(Map, preComments)
	s.AppendPostComments(Map, postComments)
	return Map
}
func (s *SyntaxTreeBuilder) MapBody(preComments []*Comment, properties []*Property, postComments []*Comment) *MapBody {
	MapBody := &MapBody{properties}
	s.AppendPreComments(MapBody, preComments)
	s.AppendPostComments(MapBody, postComments)
	return MapBody
}
func (s *SyntaxTreeBuilder) Prop(preComments []*Comment, name string, value Expression, postComments []*Comment) *Property {
	property := &Property{Name: name, Value: value}
	s.AppendPreComments(property, preComments)
	s.AppendPostComments(property, postComments)
	return property
}
func (s *SyntaxTreeBuilder) Str(preComments []*Comment, text string, postComments []*Comment) *String {
	result := &String{text}
	s.AppendPreComments(result, preComments)
	s.AppendPostComments(result, postComments)
	return result
}
func (s *SyntaxTreeBuilder) Bool(preComments []*Comment, value bool, postComments []*Comment) *Bool {
	result := &Bool{value}
	s.AppendPreComments(result, preComments)
	s.AppendPostComments(result, postComments)
	return result
}
func (s *SyntaxTreeBuilder) List(preComments []*Comment, listBody *ListBody, postComments []*Comment) *List {
	result := ListWithBody(listBody)
	s.AppendPreComments(result, preComments)
	s.AppendPostComments(result, postComments)
	return result
}

func (s *SyntaxTreeBuilder) ListBody(preComments []*Comment, items []Expression, newlineBetweenElements bool,
	postComments []*Comment) *ListBody {
	result := &ListBody{items, newlineBetweenElements}
	s.AppendPreComments(result, preComments)
	s.AppendPostComments(result, postComments)
	return result
}

func (s *SyntaxTreeBuilder) Assignment(preComments []*Comment, variable *Token, value Expression,
	originalValue Expression, assigner *Token, referenced bool, postComments []*Comment) *Assignment {
	result := NewAssignmentFromTokens(variable, value, originalValue, assigner, referenced)
	s.AppendPreComments(result, preComments)
	s.AppendPostComments(result, postComments)
	return result
}
func (s *SyntaxTreeBuilder) Variable(preComments []*Comment, variable *Token, value Expression,
	postComments []*Comment) *Variable {
	result := NewVariable(variable, value)
	s.AppendPreComments(result, preComments)
	s.AppendPostComments(result, postComments)
	return result
}
func (s *SyntaxTreeBuilder) Operator(preComments []*Comment, operator *String, value Expression,
	operands [2]Expression, postComments []*Comment) *Operator {
	result := NewOperatorFromString(operator, operands)
	result.Value = value
	s.AppendPreComments(result, preComments)
	s.AppendPostComments(result, postComments)
	return result
}

func (s *SyntaxTreeBuilder) AppendPreComment(parseNode ParseNode, comment *Comment) *SyntaxTreeBuilder {
	s.tree.GetComments(parseNode).AppendPreComment(comment)
	return s
}
func (s *SyntaxTreeBuilder) AppendPreComments(parseNode ParseNode, comments []*Comment) *SyntaxTreeBuilder {
	for _, comment := range comments {
		s.AppendPreComment(parseNode, comment)
	}
	return s
}
func (s *SyntaxTreeBuilder) PrependPreComment(parseNode ParseNode, comment *Comment) *SyntaxTreeBuilder {
	s.tree.GetComments(parseNode).PrependPreComment(comment)
	return s
}
func (s *SyntaxTreeBuilder) PrependPreComments(parseNode ParseNode, comments []*Comment) *SyntaxTreeBuilder {
	for i := len(comments) - 1; i >= 0; i-- {
		s.PrependPreComment(parseNode, comments[i])
	}
	return s
}
func (s *SyntaxTreeBuilder) AddCommentsAround(parseNode ParseNode, comments []*Comment) *SyntaxTreeBuilder {
	if len(comments) > 0 {
		var lastComment = comments[len(comments)-1]
		if lastComment.Type != FullLineBlank {
			// the last comment gets attached after the target node (placed on the same line) and the others are placed above on earlier lines
			var preComments = comments[:len(comments)-1]
			s.AppendPreComments(parseNode, preComments)
			s.AppendPostComment(parseNode, lastComment)
		} else {
			// if the last comment was a blank line then don't put that on the same line as the code being commented
			s.AppendPreComments(parseNode, comments)
		}
	}
	return s
}
func (s *SyntaxTreeBuilder) AppendPostComment(parseNode ParseNode, comment *Comment) *SyntaxTreeBuilder {
	s.tree.GetComments(parseNode).AppendPostComment(comment)
	return s
}
func (s *SyntaxTreeBuilder) AppendPostComments(parseNode ParseNode, comments []*Comment) *SyntaxTreeBuilder {
	for _, comment := range comments {
		s.AppendPostComment(parseNode, comment)
	}
	return s
}
func (s *SyntaxTreeBuilder) MoveComments(previousNode ParseNode, newNode ParseNode) *SyntaxTreeBuilder {
	s.tree.MoveComments(previousNode, newNode)
	return s
}
func (s *SyntaxTreeBuilder) AllowDanglingComments() *SyntaxTreeBuilder {
	s.permitDanglingComments = true
	return s
}
func (s *SyntaxTreeBuilder) PullAllCommentsRecursively(parseNode ParseNode) (comments []*Comment) {
	return s.tree.PullAllCommentsRecursively(parseNode)
}
func (s *SyntaxTreeBuilder) confirmNoDanglingComments() *SyntaxTreeBuilder {
	var nodeSet = s.tree.SetOfAllNonCommentNodes()
	for node, comments := range s.tree.comments {
		_, contains := nodeSet[node]
		if !contains {
			var firstComment *Comment
			var locationText = ""
			if len(comments.preComments) > 0 {
				firstComment = comments.preComments[0]
				locationText = "before"
			} else if len(comments.postComments) > 0 {
				firstComment = comments.postComments[0]
				locationText = "after"
			} else {
				continue
			}
			nodeList := s.tree.ListOfAllNodes()
			panic(fmt.Sprintf("Error validating parse tree.\nComment %#v\n\nattached %s %p (%#v) (%s)\n\n"+
				"which is not included in the syntax tree. This means that this comment would not appear in the printed output.\n\n"+
				"All nodes in the tree: %#v .", firstComment, locationText, node, node, node, nodeList))
		}
	}
	return s
}
func (s *SyntaxTreeBuilder) deleteDanglingComments() *SyntaxTreeBuilder {
	var nodeSet = s.tree.SetOfAllNonCommentNodes()
	for node := range s.tree.comments {
		_, contains := nodeSet[node]
		if !contains {
			delete(s.tree.comments, node)
		}
	}
	return s
}
func (s *SyntaxTreeBuilder) confirmNoDuplicateNodes() {
	nodes := make(map[ParseNode]ParseNode)
	for _, rootNode := range s.tree.nodes {
		descendants := s.tree.nodeAndDescendants(rootNode)
		for _, descendant := range descendants {
			prevRoot, exists := nodes[descendant]
			if exists {
				printed := VerbosePrint(&ParseTree{"", s.tree, nil})
				panic(fmt.Sprintf(
					"Internal parser error: node %s(%T), listed as a descendant of\nnode %v(%T)\n"+
						"Was already added to the tree as a descendant of\nnode %v(%T).\n"+
						"This is not permitted because it prevents the ability to attach "+
						"comments to one instance without the other. Entire tree:\n%v",
					descendant, descendant, rootNode, rootNode, prevRoot, prevRoot, printed))
			}
			nodes[descendant] = rootNode
		}
	}
}

func (s *SyntaxTreeBuilder) validate() *SyntaxTreeBuilder {
	if s.permitDanglingComments {
		s.deleteDanglingComments()
	} else {
		s.confirmNoDanglingComments()
	}
	s.confirmNoDuplicateNodes()
	return s
}

func (s *SyntaxTreeBuilder) Build() (tree *SyntaxTree) {
	s.validate()
	return s.tree
}

func (s *SyntaxTreeBuilder) ReformatAndBuild() (resultTree *SyntaxTree) {
	s.validate()

	// The parse tree is able to store some formatting information that is supposed to be cleaned up when parsing
	// This method reruns the output through the parser to clean up those kinds of things
	intermediateText := PrintTree(s.tree)

	parseTree, _ := ParseString("intermediate tree", intermediateText, NewScope(nil))
	return parseTree.SyntaxTree
}

// tries to reformat and rebuild the tree, but if that fails, then just returns the existing tree
func (s *SyntaxTreeBuilder) BuildAndAttemptToReformat() (resultTree *SyntaxTree) {
	defer func() {
		if r := recover(); r != nil {
			resultTree = s.Build()
		}
	}()
	return s.ReformatAndBuild()
}

type ParseTreeBuilder struct {
	SyntaxTreeBuilder
	sourcePositions map[ParseNode]scanner.Position
	filename        string
}

func NewParseTreeBuilder(filename string) ParseTreeBuilder {
	return ParseTreeBuilder{*NewSyntaxTreeBuilder(), make(map[ParseNode]scanner.Position, 0), filename}
}
func (p *ParseTreeBuilder) setPosition(parseNode ParseNode, position scanner.Position) {
	if parseNode == nil {
		panic("Internal error in parser.go; cannot call setPosition with a nil ParseNode")
	}
	if _, hasnil := p.sourcePositions[nil]; hasnil {
		panic("have a nil")
	}
	p.sourcePositions[parseNode] = position
}
func (p *ParseTreeBuilder) Add(parseNode ParseNode) *ParseTreeBuilder {
	p.SyntaxTreeBuilder.Add(parseNode)
	return p
}
func (p *ParseTreeBuilder) Module(preComments []*Comment, Type *Token, Map *Map, postComments []*Comment, position scanner.Position) *Module {
	result := p.SyntaxTreeBuilder.Module(preComments, Type, Map, postComments)
	p.setPosition(result, position)
	return result
}
func (p *ParseTreeBuilder) Token(preComments []*Comment, text string, postComments []*Comment, position scanner.Position) *Token {
	result := p.SyntaxTreeBuilder.Token(preComments, text, postComments)
	p.setPosition(result, position)
	return result
}
func (p *ParseTreeBuilder) Map(preComments []*Comment, mapBody *MapBody, postComments []*Comment, position scanner.Position) *Map {
	result := p.SyntaxTreeBuilder.Map(preComments, mapBody, postComments)
	p.setPosition(result, position)
	return result
}
func (p *ParseTreeBuilder) MapBody(preComments []*Comment, properties []*Property, postComments []*Comment, position scanner.Position) *MapBody {
	MapBody := p.SyntaxTreeBuilder.MapBody(preComments, properties, postComments)
	p.setPosition(MapBody, position)
	return MapBody
}
func (p *ParseTreeBuilder) Prop(preComments []*Comment, name string, value Expression, postComments []*Comment, position scanner.Position) *Property {
	property := p.SyntaxTreeBuilder.Prop(preComments, name, value, postComments)
	p.setPosition(property, position)
	return property
}
func (p *ParseTreeBuilder) String(preComments []*Comment, text string, postComments []*Comment, position scanner.Position) *String {
	result := p.SyntaxTreeBuilder.Str(preComments, text, postComments)
	p.setPosition(result, position)
	return result
}
func (p *ParseTreeBuilder) Bool(preComments []*Comment, value bool, postComments []*Comment, position scanner.Position) *Bool {
	result := p.SyntaxTreeBuilder.Bool(preComments, value, postComments)
	p.setPosition(result, position)
	return result
}
func (p *ParseTreeBuilder) List(preComments []*Comment, listBody *ListBody,
	postComments []*Comment, position scanner.Position) *List {
	result := p.SyntaxTreeBuilder.List(preComments, listBody, postComments)
	p.setPosition(result, position)
	return result
}
func (p *ParseTreeBuilder) ListBody(preComments []*Comment, items []Expression, newlineBetweenElements bool,
	postComments []*Comment, position scanner.Position) *ListBody {
	result := p.SyntaxTreeBuilder.ListBody(preComments, items, newlineBetweenElements, postComments)
	p.setPosition(result, position)
	return result
}
func (p *ParseTreeBuilder) Assignment(preComments []*Comment, variable *Token, value Expression,
	originalValue Expression, assigner *Token, referenced bool, postComments []*Comment,
	position scanner.Position) *Assignment {
	result := p.SyntaxTreeBuilder.Assignment(preComments, variable, value, originalValue, assigner, referenced,
		postComments)
	p.setPosition(result, position)
	return result
}
func (p *ParseTreeBuilder) Variable(preComments []*Comment, variable *Token, value Expression, postComments []*Comment,
	position scanner.Position) *Variable {
	result := p.SyntaxTreeBuilder.Variable(preComments, variable, value, postComments)
	p.setPosition(result, position)
	return result
}
func (p *ParseTreeBuilder) Operator(preComments []*Comment, operator *String, value Expression,
	operands [2]Expression, postComments []*Comment, position scanner.Position) *Operator {
	result := p.SyntaxTreeBuilder.Operator(preComments, operator, value, operands, postComments)
	p.setPosition(result, position)
	return result
}
func (p *ParseTreeBuilder) Comment(comment *Comment, position scanner.Position) *Comment {
	p.setPosition(comment, position)
	return comment
}
func (p *ParseTreeBuilder) Pos(offset int, line int, column int) scanner.Position {
	return scanner.Position{Filename: p.filename, Offset: offset, Line: line, Column: column}
}
func (p *ParseTreeBuilder) removeUnusedPositions(allNodes map[ParseNode]bool) {
	for parseNode := range p.sourcePositions {
		_, found := allNodes[parseNode]
		if !found {
			delete(p.sourcePositions, parseNode)
		}
	}
}
func (p *ParseTreeBuilder) Build() *ParseTree {
	syntaxTree := p.SyntaxTreeBuilder.Build()
	allNodes := syntaxTree.SetOfAllNodes()
	// If a caller changed the value of a variable after adding it into this builder, the position might no longer be relevant
	// It's important to remove unused positions because it makes it more convenient to do a difference in a test
	p.removeUnusedPositions(allNodes)

	return &ParseTree{p.filename, syntaxTree, p.sourcePositions}
}

func ParseAndEval(fileName string, r io.Reader, scope *Scope) (tree *ParseTree, errs []error) {
	p := newParser(r, scope, fileName)
	p.eval = true

	tree, errs = p.parse()

	return tree, errs
}

func Parse(fileName string, r io.Reader, scope *Scope) (tree *ParseTree, errs []error) {
	p := newParser(r, scope, fileName)

	tree, errs = p.parse()

	return tree, errs
}
func ParseString(fileName string, text string, scope *Scope) (tree *ParseTree, errs []error) {
	reader := strings.NewReader(text)
	return Parse(text, reader, NewScope(nil))
}

type parser struct {
	fileName                 string
	scanner                  scanner.Scanner
	pos                      scanner.Position
	tok                      rune
	errors                   []error
	scope                    *Scope
	eval                     bool
	builder                  ParseTreeBuilder
	skipUpcomingNewline      bool
	skipCurrentNewline       bool
	allowTwoUpcomingNewlines bool
	allowTwoCurrentNewlines  bool
	pendingNewlines          int
	parsedComments           []*Comment
	ignoredComments          map[*Comment]bool
	pendingComments          []*Comment
}

func newParser(r io.Reader, scope *Scope, fileName string) *parser {
	p := &parser{}
	p.scope = scope
	p.scanner.Init(r)
	p.scanner.Error = func(sc *scanner.Scanner, msg string) {
		p.errorf(msg)
	}
	p.scanner.Mode = scanner.ScanIdents | scanner.ScanStrings |
		scanner.ScanRawStrings | scanner.ScanComments
	p.parsedComments = make([]*Comment, 0)
	p.ignoredComments = make(map[*Comment]bool)
	p.pendingComments = make([]*Comment, 0)
	p.fileName = fileName
	p.scanner.Filename = fileName
	p.next()
	return p
}

func (p *parser) parse() (parseTree *ParseTree, errs []error) {
	// catch errTooManyErrs
	defer func() {
		if r := recover(); r != nil {
			if r == errTooManyErrors {
				errs = p.errors
				return
			}
			fmt.Println(fmt.Sprintf("fatal parse error at %s", p.pos))
			panic(r)
		}
	}()

	p.builder = NewParseTreeBuilder(p.fileName)
loop:
	for {

		switch p.tok {
		case scanner.Ident:

			var ignoreEndingNewline = !p.allowTwoCurrentNewlines

			var preTokenComments = append(p.pendingComments, p.parseComments(ignoreEndingNewline)...)

			p.pendingComments = make([]*Comment, 0)
			ident := p.scanner.TokenText()

			p.accept(scanner.Ident)
			var postTokenComments = p.parseComments(false)

			var rootNode ParseNode
			var leafNode ParseNode
			switch p.tok {
			case '+':
				p.accept('+')
				rootNode = p.parseAssignment(ident, "+=")
				leafNode = rootNode
			case '=':
				rootNode = p.parseAssignment(ident, "=")
				leafNode = rootNode
			case '{', '(':
				var module = p.parseModule(ident)
				rootNode = module
				leafNode = module.Type

				// If the input mistakenly contains any comments requiring newlines before the "{", then move those comments inside the map
				moveCommentsIntoMap := false
				for _, comment := range postTokenComments {
					if comment.Type == FullLineText {
						moveCommentsIntoMap = true
						break
					}
				}
				if moveCommentsIntoMap {
					p.builder.PrependPreComments(module.MapBody, postTokenComments)
					postTokenComments = make([]*Comment, 0)

				}

				p.ignoreNextNewline()
			default:
				p.errorf("expected \"=\" or \"+=\" or \"{\" or \"(\" or Comment, found %s",
					scanner.TokenString(p.tok))
			}
			p.builder.Add(rootNode)
			p.builder.AppendPreComments(rootNode, preTokenComments)
			p.builder.AppendPostComments(leafNode, postTokenComments)
		case scanner.Comment:
			p.pendingComments = append(p.pendingComments, p.parseComment())
		case scanner.EOF:
			break loop
		default:
			p.errorf("expected assignment or module definition, found %s",
				scanner.TokenString(p.tok))
			break loop
		}
	}
	p.accept(scanner.EOF)

	// dump pending comments
	for _, comment := range p.pendingComments {
		p.builder.Add(comment)
	}

	// build+validate tree
	parseTree = p.builder.Build()

	errs = p.errors

	p.validate(parseTree)

	return parseTree, errs

}

func (p *parser) error(err error) {
	pos := p.scanner.Position
	if !pos.IsValid() {
		pos = p.scanner.Pos()
	}
	err = &ParseError{
		Err: err,
		Pos: pos,
	}
	p.errors = append(p.errors, err)
	if len(p.errors) >= maxErrors {
		panic(errTooManyErrors)
	}
}

func (p *parser) errorf(format string, args ...interface{}) {
	p.error(fmt.Errorf(format, args...))
}

func (p *parser) accept(toks ...rune) bool {
	for _, tok := range toks {
		if p.tok != tok {
			p.errorf("bp parser expected %s, found %s", scanner.TokenString(tok),
				scanner.TokenString(p.tok))
			return false
		}
		p.next()
	}
	return true
}

func (p *parser) next() {
	// save previous location
	p.pos = p.scanner.Position
	var prevEndLine = p.pos.Line + strings.Count(p.scanner.TokenText(), "\n")
	if prevEndLine < 1 {
		prevEndLine = 1
	}
	// advance to the next token
	if p.tok != scanner.EOF {
		p.tok = p.scanner.Scan()
	}
	var currentLine = p.scanner.Line

	// count the number of lines jumped
	p.skipCurrentNewline = p.skipUpcomingNewline
	p.skipUpcomingNewline = false
	p.allowTwoCurrentNewlines = p.allowTwoUpcomingNewlines
	p.allowTwoUpcomingNewlines = false
	p.pendingNewlines = currentLine - prevEndLine
}

// returns a Comment indicating a newline to add to the SyntaxTree, or nil if none is needed
func (p *parser) getPendingNewline() (comment *Comment) {
	count := p.pendingNewlines
	if p.skipCurrentNewline && count > 0 {
		count--
	}
	var found = false
	if p.allowTwoCurrentNewlines && count > 1 {
		p.pendingNewlines = 1
		p.allowTwoCurrentNewlines = false
		p.skipCurrentNewline = false
		found = true
	} else {
		p.pendingNewlines = 0
		p.skipCurrentNewline = false
		p.allowTwoCurrentNewlines = false
		if count > 0 {
			found = true
		}
	}
	if found {
		comment = NewBlankLine()
		p.savePosTo(comment)
		return comment
	}
	return nil
}

func (p *parser) getPendingNewlines() (comments []*Comment) {
	comments = make([]*Comment, 0)
	for {
		comment := p.getPendingNewline()
		if comment != nil {
			comments = append(comments, comment)
		} else {
			break
		}
	}
	return comments
}

func (p *parser) ignorePrevNewline() {
	p.skipCurrentNewline = true
}
func (p *parser) ignoreNextNewline() {
	p.allowTwoUpcomingNewlines = false
	p.skipUpcomingNewline = true
}
func (p *parser) permitTwoNewlines() {
	p.allowTwoUpcomingNewlines = true
	p.skipUpcomingNewline = false
}

func (p *parser) savePosTo(parseNode ParseNode) {
	p.setPosition(parseNode, p.pos)
}

func (p *parser) setPosition(parseNode ParseNode, pos scanner.Position) {
	p.builder.setPosition(parseNode, pos)
}
func (p *parser) sourcePosition(parseNode ParseNode) scanner.Position {
	return p.builder.sourcePositions[parseNode]
}
func (p *parser) hasSourcePosition(parseNode ParseNode) bool {
	_, found := p.builder.sourcePositions[parseNode]
	return found
}

// mark a particular comment as not relevant and not in need of a exception if it is left out of the generated parse tree
func (p *parser) ignoreComment(comment *Comment) {
	// It'd be great if Golang had an ordered map and then p.parsedComments could be a map and we could do something like p.parsedComments.delete(comment)
	// However, if we tried to use an unordered map for p.parsedComments, then we would iterate over it in a random order and not necessarily show the same error every time for a fixed input
	p.ignoredComments[comment] = true
}
func (p *parser) confirmAllCommentsAttachedOrSkippable(tree *SyntaxTree) {
	var readComments = p.parsedComments
	var savedComments = tree.FindAllComments()
	for _, comment := range readComments {
		_, found := savedComments[comment]
		if !found {
			// Not found in the tree; was that expected?
			_, expected := p.ignoredComments[comment]
			if !expected {
				panic(fmt.Sprintf("comment %s was parsed but not attached to the syntax tree %s", comment, tree))
			}
		}
	}
}
func (p *parser) confirmAllNodesHavePositions(tree *ParseTree) {
	var allNodes = tree.SyntaxTree.ListOfAllNodes()
	var prevNode ParseNode
	for _, node := range allNodes {
		var fileName = p.fileName
		if len(fileName) < 1 {
			fileName = "''"
		}
		if !tree.HasSourcePosition(node) {
			panic(fmt.Sprintf(
				`Internal parser error reading %s.
Failed to get source position for %#v.
There probably needs to be another call to setPosition in parser.go for this case.
Previous node was %s at location %s
`,
				fileName,
				node,
				prevNode,
				tree.SourcePosition(prevNode),
			))
		}
		prevNode = node
	}
}
func (p *parser) validate(tree *ParseTree) {
	p.confirmAllCommentsAttachedOrSkippable(tree.SyntaxTree)
	p.confirmAllNodesHavePositions(tree)
}

func (p *parser) parseComments(ignoreOneEndingNewline bool) (comments []*Comment) {
	comments = make([]*Comment, 0)

	for p.tok == scanner.Comment {

		// add a newline at the beginning if applicable
		comments = append(comments, p.getPendingNewlines()...)

		// add a comment if another one is left
		var comment, err = p.TryParseComment(p.scanner.TokenText())
		if err != nil {
			break
		}
		comments = append(comments, comment)
		p.next()
	}

	if ignoreOneEndingNewline {
		p.ignorePrevNewline()
	}
	comments = append(comments, p.getPendingNewlines()...)

	return comments
}

func (p *parser) parseComment() (comment *Comment) {

	comment = p.getPendingNewline()
	if comment != nil {
		return comment
	}

	tokenText := p.scanner.TokenText()
	comment, err := p.TryParseComment(tokenText)
	if err != nil {
		panic(err)
	}
	p.next()
	return comment
}

func (p *parser) TryParseComment(text string) (comment *Comment, err error) {
	switch {
	case strings.HasPrefix(text, "//"):
		p.ignoreNextNewline()
		comment = NewFullLineComment(strings.Replace(text, "//", "", 1))
	case strings.HasPrefix(text, "/*"):
		text = strings.Replace(text, "/*", "", 1)
		text = strings.Replace(text, "*/", "", 1)
		text = strings.Replace(text, "\t", "", -1)
		p.permitTwoNewlines() // the input text can end the current comment and then have another blank line afterward
		comment = NewInlineComment(text)
	}
	if comment != nil {
		p.savePosTo(comment)
		p.parsedComments = append(p.parsedComments, comment)
		return comment, nil
	}
	return nil, errors.New(fmt.Sprint("Cannot parse comment '", text, "'"))

}

func (p *parser) parseAssignment(name string,
	assigner string) (assignment *Assignment) {

	if !p.accept('=') {
		return nil
	}

	assignment = new(Assignment)

	assignment.Name = &Token{name}
	p.savePosTo(assignment.Name)
	p.savePosTo(assignment)

	value := p.parseExpression()

	assignment.Value = value
	assignment.OrigValue = value
	assignment.Assigner = &Token{assigner}
	p.savePosTo(assignment.Assigner)

	if p.scope != nil {
		if assigner == "+=" {
			if old, local := p.scope.Get(assignment.Name.Value); old == nil {
				p.errorf("modified non-existent variable %q with +=", assignment.Name)
			} else if !local {
				p.errorf("modified non-local variable %q with +=", assignment.Name)
			} else if old.Referenced {
				p.errorf("modified variable %q with += after referencing", assignment.Name)
			} else {
				val, err := p.evaluateOperator(old.Value, assignment.Value, '+')
				if err != nil {
					p.error(err)
				} else {
					old.Value = val
				}
			}
		} else {
			err := p.scope.Add(assignment)
			if err != nil {
				p.error(err)
			}
		}
	}

	return assignment
}

func (p *parser) parseModule(typ string) *Module {

	startPos := p.pos
	includeBrace := false
	if p.tok == '{' {
		includeBrace = true
	}

	if !p.accept(p.tok) {
		return nil
	}
	propertyMap := p.parseMapWithoutBraces(true, includeBrace)

	var endLine = p.scanner.Line

	if includeBrace {
		p.accept('}')
	} else {
		p.accept(')')
	}

	var mod = &Module{
		Type: &Token{typ},
		Map:  propertyMap,
	}
	p.setPosition(mod, startPos)
	p.setPosition(mod.Type, startPos)

	// add any more comments found on the same line as the closing brace
	for p.tok == scanner.Comment && p.scanner.Line == endLine {
		p.builder.AppendPostComment(mod, p.parseComment())
	}

	return mod
}

func (p *parser) parseMapWithoutBraces(isModule, assignerIsColon bool) (result *Map) {
	var property *Property
	result = NewMap(make([]*Property, 0))
	p.savePosTo(result)
	p.savePosTo(result.MapBody)
	var foundAnItem = false
	for (p.tok == scanner.Ident) || (p.tok == scanner.Comment) {
		foundAnItem = true

		p.allowTwoCurrentNewlines = true

		var comments = p.parseComments(false)
		if len(comments) > 0 {
			// If there's exactly one comment and it's a newline, then it's implied and we can skip it
			if len(comments) == 1 && comments[0].Type == FullLineBlank {
				p.ignoreComment(comments[0])
			} else {
				if property == nil {
					p.builder.AppendPreComments(result.MapBody, comments)
				} else {
					p.builder.AppendPostComments(property, comments)
				}
			}
		} else {

			property = p.parseProperty(isModule, assignerIsColon)
			result.MapBody.Properties = append(result.MapBody.Properties, property)

			if p.tok != ',' {
				// There was no comma, so the list is done.
				break
			}

			p.accept(',')
		}

	}

	var ignoreEndingNewline = true
	if !foundAnItem {
		// Normally we ignore one ending newline because a map close-brace is supposed to be on a new line
		// However, if the map is empty, then it's significant
		ignoreEndingNewline = false
	}

	comments := p.parseComments(ignoreEndingNewline)
	p.builder.AppendPostComments(result.MapBody, comments)

	return result
}

func (p *parser) parseMapWithBraces() *Map {
	p.parseComments(false)
	if !p.accept('{') {
		return nil
	}

	var myMap = p.parseMapWithoutBraces(false, false)

	p.accept('}')

	return myMap
}

func (p *parser) parseProperty(isModule, separatorIsColon bool) (property *Property) {
	property = new(Property)

	name := p.scanner.TokenText()
	p.accept(scanner.Ident)
	p.savePosTo(property)

	if isModule {
		if separatorIsColon && p.tok == ':' {
			p.accept(':')
		} else {
			if !p.accept('=') {
				return
			}
		}
	} else {
		if !p.accept(':') {
			return
		}
	}

	value := p.parseExpression()

	property.Name = name
	property.Value = value

	return
}

func (p *parser) parseExpression() (value Expression) {
	value = p.parseValue()
	switch p.tok {
	case '+':
		return p.parseOperator(value)
	default:
		return value
	}
}

func (p *parser) evaluateOperator(value1, value2 Expression, operator rune) (*Operator, error) {

	if value1 == nil || value2 == nil {
		return nil, fmt.Errorf("illegal nil value in operator (%s %c %s)", value1, operator, value2)
	}
	if value1 == nil {
		return nil, errors.New("nil lhs in operator")
	}
	value := value1.Copy()

	if p.eval {
		e1 := value1.Eval()
		e2 := value2.Eval()
		if e1.Type() != e2.Type() {
			return nil, fmt.Errorf("mismatched type in operator %c: %s != %s", operator,
				e1.Type(), e2.Type())
		}

		value = e1.Copy()
		p.copyLocations(e1, value) // assign the same locations to the copied values

		switch operator {
		case '+':
			switch v := value.(type) {
			case *String:
				v.Value += e2.(*String).Value
			case *List:
				v.Values = append(v.Values, e2.(*List).Values...)
			case *MapBody:
				var err error
				v.Properties, err = p.addMaps(v.Properties, e2.(*MapBody).Properties)
				if err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("operator %c not supported on type %s", operator, v.Type())
			}
		default:
			panic("unknown operator " + string(operator))
		}
	}

	var result = &Operator{
		Args:          [2]Expression{value1, value2},
		OperatorToken: &String{string(operator)},
		Value:         value,
	}

	p.setPosition(result.OperatorToken, p.sourcePosition(value1))
	p.setPosition(result.Value, p.sourcePosition(value1))
	p.setPosition(result, p.sourcePosition(value1))

	return result, nil
}

func (p *parser) copyLocations(sourceNode ParseNode, destNode ParseNode) {
	p.setPosition(destNode, p.sourcePosition(sourceNode))
	if len(sourceNode.Children()) != len(destNode.Children()) {
		panic("Internal parser error; cannot copy locations between two objects with different child counts")
	}
	for i, sourceChild := range sourceNode.Children() {
		destChild := destNode.Children()[i]
		p.copyLocations(sourceChild, destChild)
	}
}

func (p *parser) addMaps(map1, map2 []*Property) ([]*Property, error) {
	ret := make([]*Property, 0, len(map1))

	inMap1 := make(map[string]*Property)
	inMap2 := make(map[string]*Property)
	inBoth := make(map[string]*Property)

	for _, prop1 := range map1 {
		inMap1[prop1.Name] = prop1
	}

	for _, prop2 := range map2 {
		inMap2[prop2.Name] = prop2
		if _, ok := inMap1[prop2.Name]; ok {
			inBoth[prop2.Name] = prop2
		}
	}

	for _, prop1 := range map1 {
		if prop2, ok := inBoth[prop1.Name]; ok {
			var err error
			newProp := *prop1
			newProp.Value, err = p.evaluateOperator(prop1.Value, prop2.Value, '+')
			if err != nil {
				return nil, err
			}
			ret = append(ret, &newProp)
		} else {
			ret = append(ret, prop1)
		}
	}

	for _, prop2 := range map2 {
		if _, ok := inBoth[prop2.Name]; !ok {
			ret = append(ret, prop2)
		}
	}

	return ret, nil
}

func (p *parser) parseOperator(value1 Expression) *Operator {
	operator := p.tok
	var pos = p.pos
	p.accept(operator)

	value2 := p.parseExpression()

	value, err := p.evaluateOperator(value1, value2, operator)
	if err != nil {
		p.error(err)
		return nil
	}
	p.setPosition(value, pos)
	p.setPosition(value.OperatorToken, pos)

	return value
}

func (p *parser) parseValue() (value Expression) {
	var comments = p.parseComments(false)
	switch p.tok {
	case scanner.Ident:
		value = p.parseVariable()
	case scanner.String:
		value = p.parseStringValue()
	case '[':
		value = p.parseListValue()
	case '{':
		value = p.parseMapWithBraces()
	default:
		p.errorf("expected bool, list, or string value; found %s",
			scanner.TokenString(p.tok))
		return
	}
	p.builder.AppendPreComments(value, comments)
	return value
}

func (p *parser) parseVariable() Expression {
	var value Expression

	switch text := p.scanner.TokenText(); text {
	case "true", "false":
		value = &Bool{
			Value: text == "true",
		}
	default:
		if p.eval {
			if assignment, local := p.scope.Get(text); assignment == nil {
				p.errorf("variable %q is not set", text)
			} else {
				if local {
					assignment.Referenced = true
				}
				value = assignment.Value
			}
		}
		var token = &Token{text}
		p.savePosTo(token)
		if value != nil {
			p.savePosTo(value)
		}
		value = NewVariable(token, value)
	}
	p.savePosTo(value)
	p.accept(scanner.Ident)
	return value
}

func (p *parser) parseStringValue() *String {
	str, err := strconv.Unquote(p.scanner.TokenText())
	if err != nil {
		p.errorf("couldn't parse string: %s", err)
		return nil
	}

	value := &String{
		Value: str,
	}
	p.savePosTo(value)
	p.accept(scanner.String)
	return value
}

func (p *parser) parseListValue() *List {
	if !p.accept('[') {
		return nil
	}
	var startPos = p.scanner.Pos()

	var elements []Expression

	var element Expression
	var comments []*Comment
	for {
		p.allowTwoCurrentNewlines = true
		comments = p.parseComments(false)
		// if there's exactly one comment and it's a newline, then we can skip it because it's implied
		if len(comments) == 1 && comments[0].Type == FullLineBlank {
			p.ignoreComment(comments[0])
			comments = comments[:0]
		}
		if p.tok == ']' {
			break
		}
		element = p.parseExpression()
		if p.eval && element.Type() != StringType {
			p.errorf("Expected string in list, found %s", element.Type().String())
			return nil
		}
		p.builder.AppendPreComments(element, comments)
		elements = append(elements, element)

		p.builder.AppendPostComments(element, p.parseComments(false))
		comments = nil

		if p.tok != ',' {
			// There was no comma, so the list is done.
			break
		}

		p.accept(',')
	}
	var endPos = p.scanner.Pos()
	p.accept(']')

	var list = NewList(elements, (len(elements) > 1 || endPos.Line > startPos.Line))
	p.setPosition(list, startPos)
	p.setPosition(list.ListBody, startPos)
	if element != nil {
		// in case of inline comments attached to the last list element
		p.builder.AppendPostComments(element, comments)
	} else {
		// in case of comments inside an empty list
		p.builder.AppendPostComments(list.ListBody, comments)
	}
	return list
}

type Scope struct {
	vars          map[string]*Assignment
	inheritedVars map[string]*Assignment
}

func NewScope(parent *Scope) *Scope {
	newScope := &Scope{
		vars:          make(map[string]*Assignment),
		inheritedVars: make(map[string]*Assignment),
	}

	if parent != nil {
		for k, v := range parent.vars {
			newScope.inheritedVars[k] = v
		}
		for k, v := range parent.inheritedVars {
			newScope.inheritedVars[k] = v
		}
	}

	return newScope
}

func (s *Scope) Add(assignment *Assignment) error {
	if old, ok := s.vars[assignment.Name.Value]; ok {
		return fmt.Errorf("variable already set, previous assignment: %s", old)
	}

	if old, ok := s.inheritedVars[assignment.Name.Value]; ok {
		return fmt.Errorf("variable already set in inherited scope, previous assignment: %s", old)
	}

	s.vars[assignment.Name.Value] = assignment

	return nil
}

func (s *Scope) Remove(name string) {
	delete(s.vars, name)
	delete(s.inheritedVars, name)
}

func (s *Scope) Get(name string) (*Assignment, bool) {
	if a, ok := s.vars[name]; ok {
		return a, true
	}

	if a, ok := s.inheritedVars[name]; ok {
		return a, false
	}

	return nil, false
}

func (s *Scope) String() string {
	vars := []string{}

	for k := range s.vars {
		vars = append(vars, k)
	}
	for k := range s.inheritedVars {
		vars = append(vars, k)
	}

	sort.Strings(vars)

	ret := []string{}
	for _, v := range vars {
		if assignment, ok := s.vars[v]; ok {
			ret = append(ret, assignment.String())
		} else {
			ret = append(ret, s.inheritedVars[v].String())
		}
	}

	return strings.Join(ret, "\n")
}
