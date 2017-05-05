// Copyright 2016 Google Inc. All rights reserved.
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
	"strings"
)

type commentType int

const (
	FullLineText  = iota // starts with "//"
	InlineText           // starts with "/*"
	FullLineBlank        // just a single newline
	//InlineBlank          // only spaces and tabs - not currently needed but could be added in the future
)

// converts an item to a string, and then adds separators such that the result can be embedded in another string
func readable(item interface{}) (text string) {
	text = fmt.Sprintf("%s", item)
	if strings.Contains(text, " ") {
		text = fmt.Sprintf("(%s)", text)
	}
	return text
}

type Comment struct {
	Text string
	Type commentType
}

func NewFullLineComment(text string) (comment *Comment) {
	return &Comment{text, FullLineText}
}
func NewInlineComment(text string) (comment *Comment) {
	return &Comment{text, InlineText}
}
func NewBlankLine() (comment *Comment) {
	return &Comment{"\n", FullLineBlank}
}

func (c *Comment) IsBpParseNode() {

}

func (c *Comment) Children() []ParseNode {
	return make([]ParseNode, 0)
}

type CommentPair struct {
	preComments  []*Comment
	postComments []*Comment
}

func (c *CommentPair) AppendPreComment(comment *Comment) {
	c.preComments = append(c.preComments, comment)
}
func (c *CommentPair) PrependPreComment(comment *Comment) {
	c.preComments = append([]*Comment{comment}, c.preComments...)
}
func (c *CommentPair) AppendPostComment(comment *Comment) {
	c.postComments = append(c.postComments, comment)
}
func (c *CommentPair) PrependPostComment(comment *Comment) {
	c.postComments = append([]*Comment{comment}, c.postComments...)
}

func (c *CommentPair) PostComments() []*Comment {
	return c.postComments
}

func (c *CommentPair) PreComments() []*Comment {
	return c.preComments
}

// A SyntaxTree is a tree of nodes that don't necessarily know where they came from (filename or position)
// and may not have even been created by parsing a file at all (such as by a programmatic builder in a test)
// To incorporate information about where the tokens came from, use a ParseTree which also stores that information
type SyntaxTree struct {
	nodes    []ParseNode
	comments map[ParseNode](*CommentPair)
}

type ParseNode interface {
	IsBpParseNode() // To ensure that any type that is passed in is intended to be a ParseNode and not just implementing the interface accidentally
	Children() []ParseNode
}

func NewSyntaxTree() *SyntaxTree {
	return &SyntaxTree{
		comments: make(map[ParseNode](*CommentPair)),
	}
}

func (t *SyntaxTree) AddNode(node ParseNode) {
	t.nodes = append(t.nodes, node)
}
func (t *SyntaxTree) GetComments(parseNode ParseNode) (comments *CommentPair) {
	comments = t.GetCommentsIfPresent(parseNode)
	if comments == nil {
		comments = &CommentPair{}
		t.comments[parseNode] = comments
	}
	if comments != nil {
		if _, isAComment := parseNode.(*Comment); isAComment {
			panic(fmt.Sprintf("cannot ask for the comments attached to a comment %s", parseNode))
		}
	}
	return comments
}
func (t *SyntaxTree) GetCommentsIfPresent(parseNode ParseNode) (comments *CommentPair) {
	if parseNode == nil {
		panic("illegal nil value for parseNode")
	}
	comments, ok := t.comments[parseNode]
	if !ok {
		return nil
	}
	return comments
}

func (t *SyntaxTree) MoveComments(oldNode ParseNode, newNode ParseNode) {
	var oldContainer = t.GetComments(oldNode)
	var newContainer = t.GetComments(newNode)
	for _, comment := range oldContainer.preComments {
		newContainer.AppendPreComment(comment)
	}
	for _, comment := range oldContainer.postComments {
		newContainer.AppendPostComment(comment)
	}
	t.RemoveComments(oldNode)
}

// finds all the comments attached to the given node or its descendants, removes those comments, and returns them as a list
func (t *SyntaxTree) PullAllCommentsRecursively(parseNode ParseNode) (comments []*Comment) {
	comments = make([]*Comment, 0)
	return t.pullAllCommentsRecursively(parseNode, comments)
}
func (t *SyntaxTree) pullAllCommentsRecursively(parseNode ParseNode, comments []*Comment) (result []*Comment) {
	container := t.GetComments(parseNode)
	comments = append(comments, container.preComments...)
	for _, child := range parseNode.Children() {
		t.pullAllCommentsRecursively(child, comments)
	}
	comments = append(comments, container.postComments...)
	t.RemoveComments(parseNode)
	return comments
}

func (t *SyntaxTree) RemoveComments(parseNode ParseNode) {
	delete(t.comments, parseNode)
}

func (t *SyntaxTree) Nodes() []ParseNode {
	return t.nodes
}

func (t *SyntaxTree) FindAllComments() (comments map[*Comment]bool) { // really Set<Comment>
	comments = make(map[*Comment]bool, 0)
	// add top-level comments
	for _, node := range t.nodes {
		comment, ok := node.(*Comment)
		if ok {
			comments[comment] = true
		}
	}
	// add pre and post comments
	for _, commentContainer := range t.comments {
		for _, comment := range commentContainer.preComments {
			comments[comment] = true
		}
		for _, comment := range commentContainer.postComments {
			comments[comment] = true
		}
	}
	return comments
}

func (t *SyntaxTree) nodeAndDescendants(node ParseNode) (nodes []ParseNode) {
	nodes = make([]ParseNode, 0)
	nodes = append(nodes, node)
	for _, child := range node.Children() {
		if child != nil {
			nodes = append(nodes, t.nodeAndDescendants(child)...)
		} else {
			panic(fmt.Sprintf("Illegal nil node given as child of %#v. All children: %#v", node, node.Children()))
		}
	}
	return nodes
}

func (t *SyntaxTree) ListOfAllNodes() (nodes []ParseNode) {
	nodes = make([]ParseNode, 0)
	for _, node := range t.nodes {
		nodes = append(nodes, t.nodeAndDescendants(node)...)
	}
	return nodes
}

func (t *SyntaxTree) SetOfAllNonCommentNodes() (nodes map[ParseNode]bool) {
	nodes = make(map[ParseNode]bool)
	for _, node := range t.ListOfAllNodes() {
		nodes[node] = true
	}
	return nodes
}

func (t *SyntaxTree) SetOfAllNodes() (nodes map[ParseNode]bool) {
	nodes = t.SetOfAllNonCommentNodes()
	for node := range nodes {
		comments := t.GetCommentsIfPresent(node)
		if comments != nil {
			for _, preComment := range comments.preComments {
				nodes[preComment] = true
			}
			for _, postComment := range comments.postComments {
				nodes[postComment] = true
			}
		}
	}
	return nodes
}

// Definition is an Assignment or a Module at the top level of a Blueprints file
type Definition interface {
	String() string
	definitionTag()
	IsBpParseNode()
	Children() []ParseNode
}

// An Assignment is a variable assignment at the top level of a Blueprints file, scoped to the
// file and and subdirs.
type Assignment struct {
	Name       *Token
	Value      Expression
	OrigValue  Expression
	Assigner   *Token
	Referenced bool
}

func NewAssignment(name string, value Expression, origValue Expression, assigner string, referenced bool) (assignment *Assignment) {
	return &Assignment{&Token{name}, value, origValue, &Token{assigner}, referenced}
}
func NewAssignmentFromTokens(name *Token, value Expression, origValue Expression, assigner *Token, referenced bool) (assignment *Assignment) {
	return &Assignment{name, value, origValue, assigner, referenced}
}

func (a *Assignment) IsBpParseNode() {

}

func (a *Assignment) String() string {
	valueText := fmt.Sprintf("%s", a.Value)
	if strings.Contains(valueText, " ") {
		valueText = fmt.Sprintf("(%s)", valueText)
	}
	return fmt.Sprintf("(%s %s %s) (OrigValue = %s), referenced=%t",
		readable(a.Name), readable(a.Assigner), readable(valueText), readable(a.OrigValue), a.Referenced)
}

func (a *Assignment) Dump() string {
	return a.String()
}

func (a *Assignment) definitionTag() {}

func (a *Assignment) Children() []ParseNode {
	//return []ParseNode{a.Name, a.Assigner, a.Value}
	return []ParseNode{a.Name, a.Assigner, a.OrigValue}
}

// A Module is a module definition at the top level of a Blueprints file
type Module struct {
	Type *Token
	*Map
}

func (m *Module) IsAParseNode() {

}

func (m *Module) Copy() *Module {
	ret := *m
	ret.Properties = make([]*Property, len(m.Properties))
	for i := range m.Properties {
		ret.Properties[i] = m.Properties[i].Copy()
	}
	return &ret
}

func (m *Module) String() string {
	if m.Properties != nil {
		propertyStrings := make([]string, len(m.Properties))
		for i, property := range m.Properties {
			propertyStrings[i] = readable(property.String())
		}
		return fmt.Sprintf("%s{%s}", m.Type,
			strings.Join(propertyStrings, ", "))
	}
	return "{}"
}

func (m *Module) Dump() string {
	return m.String()
}

func (m *Module) definitionTag() {}

func (m *Module) Children() []ParseNode {
	return []ParseNode{m.Type, m.Map}
}

// A Property is a name: value pair within a MapBody, which may be a top level Module.
type Property struct {
	Name  string
	Value Expression
}

func (p Property) IsBpParseNode() {

}
func (x *Property) Children() []ParseNode {
	return []ParseNode{x.Value}
}

func (p *Property) Copy() *Property {
	ret := *p
	ret.Value = p.Value.Copy()
	return &ret
}

func (p *Property) String() string {
	return fmt.Sprintf("%s: %s", readable(p.Name), readable(p.Value))
}

// An Expression is a Value in a Property or Assignment.  It can be a literal (String or Bool), a
// MapBody, a List, an Operator that combines two expressions of the same type, or a Variable that
// references and Assignment.
type Expression interface {
	// Copy returns a copy of the Expression that will not affect the original if mutated
	Copy() Expression
	String() string
	// Type returns the underlying Type enum of the Expression if it were to be evalutated
	Type() Type
	// Eval returns an expression that is fully evaluated to a simple type (List, MapBody, String, or
	// Bool).  It will return the same object for every call to Eval().
	Eval() Expression
	IsBpParseNode()
	Children() []ParseNode
}

type Type int

const (
	BoolType Type = iota + 1
	StringType
	ListType
	MapType
)

func (t Type) String() string {
	switch t {
	case BoolType:
		return "bool"
	case StringType:
		return "string"
	case ListType:
		return "list"
	case MapType:
		return "map"
	default:
		panic(fmt.Errorf("Unknown type %d", t))
	}
}

type Operator struct {
	Args          [2]Expression
	OperatorToken *String
	Value         Expression
}

func NewOperator(operator string, args [2]Expression) (result *Operator) {
	return &Operator{args, &String{operator}, nil}
}
func NewOperatorFromString(operator *String, args [2]Expression) (result *Operator) {
	return &Operator{args, operator, nil}
}

func (x *Operator) IsBpParseNode() {

}

func (x *Operator) Copy() Expression {
	ret := *x
	ret.Args[0] = x.Args[0].Copy()
	ret.Args[1] = x.Args[1].Copy()
	return &ret
}

func (x *Operator) Eval() Expression {
	return x.Value.Eval()
}

func (x *Operator) Type() Type {
	return x.Args[0].Type()
}

func (x *Operator) String() string {
	return fmt.Sprintf("%s %s %s = %s",
		readable(x.Args[0].String()), x.OperatorToken, readable(x.Args[1].String()),
		readable(x.Value))
}

func (x *Operator) Dump() string {
	return x.String()
}

func (x *Operator) Children() []ParseNode {
	return []ParseNode{x.Args[0], x.OperatorToken, x.Args[1]}
}

type Variable struct {
	NameNode *Token
	Value    Expression
}

func NewVariable(nameNode *Token, value Expression) *Variable {
	if nameNode == nil {
		panic(fmt.Sprintf("Illegal nil in variable name (%s = %s)", nameNode, value))
	}
	return &Variable{nameNode, value}
}

func (x *Variable) IsBpParseNode() {

}

func (x *Variable) Children() []ParseNode {
	return []ParseNode{x.NameNode} // this value expression isn't a child; the source expression will be elsewhere in the syntax tree
}

func (x *Variable) Copy() Expression {
	ret := *x
	return &ret
}

func (x *Variable) Eval() Expression {
	return x.Value.Eval()
}

func (x *Variable) String() string {
	return readable(x.NameNode.String()) + " = " + readable(x.Value.String())
}

func (x *Variable) Type() Type { return x.Value.Type() }

func (x *Variable) Name() string {
	return x.NameNode.Value
}

type Map struct {
	*MapBody // the reason that MapBody is distinct from Map is to provide a location to put comments in case the map is empty
}

func NewMap(properties []*Property) *Map {
	return NewMapWithBody(&MapBody{properties})
}
func NewMapWithBody(mapBody *MapBody) *Map {
	return &Map{mapBody}
}

func (x *Map) Children() []ParseNode {
	return []ParseNode{x.MapBody}
}

type MapBody struct {
	Properties []*Property
}

func (x *MapBody) IsBpParseNode() {

}

func (x *MapBody) Children() []ParseNode {
	var children = make([]ParseNode, 0)
	for _, property := range x.Properties {
		children = append(children, property)
	}

	return children
}

func (x *MapBody) Copy() Expression {
	ret := *x
	ret.Properties = make([]*Property, len(x.Properties))
	for i := range x.Properties {
		ret.Properties[i] = x.Properties[i].Copy()
	}
	return &ret
}

func (x *MapBody) Eval() Expression {
	return x
}

func (x *MapBody) String() string {
	propertyStrings := make([]string, len(x.Properties))
	for i, property := range x.Properties {
		propertyStrings[i] = readable(property.String())
	}
	return fmt.Sprintf("{%s}",
		strings.Join(propertyStrings, ", "))
}

func (x *MapBody) Type() Type { return MapType }

type List struct {
	*ListBody // the reason that ListBody is distinct from List is to provide a location to put comments in case the list is empty
}

func NewList(values []Expression, NewlineBewteenElements bool) (list *List) {
	return ListWithBody(&ListBody{Values: values, NewlineBetweenElements: NewlineBewteenElements})
}
func ListWithBody(listBody *ListBody) (list *List) {
	return &List{listBody}
}
func NewEmptyList() (list *List) {
	return NewList(make([]Expression, 0), false)
}

func (x *List) IsBpParseNode() {

}

func (x *List) Children() []ParseNode {
	return []ParseNode{x.ListBody}
}

func (x *List) Copy() Expression {
	ret := NewList(make([]Expression, len(x.Values)), x.NewlineBetweenElements)
	for i := range ret.Values {
		ret.Values[i] = x.Values[i].Copy()
	}
	return ret
}

func (x *List) Eval() Expression {
	return x
}

func (x *List) String() string {
	valueStrings := make([]string, len(x.Values))
	for i, value := range x.Values {
		valueStrings[i] = readable(value.String())
	}
	return fmt.Sprintf("%s",
		strings.Join(valueStrings, ", "))
}

func (x *List) Type() Type { return ListType }

type ListBody struct {
	Values                 []Expression
	NewlineBetweenElements bool
}

func (x *ListBody) IsBpParseNode() {

}

func (x *ListBody) Children() []ParseNode {
	var children = make([]ParseNode, 0)
	for _, val := range x.Values {
		children = append(children, val)
	}
	return children
}

// A String is a literal quoted string whereas a Token is a symbol
type String struct {
	Value string
}

func (x *String) IsBpParseNode() {

}

func (x *String) Children() []ParseNode {
	return make([]ParseNode, 0)
}

func (x *String) Copy() Expression {
	ret := *x
	return &ret
}

func (x *String) Eval() Expression {
	return x
}

func (x *String) String() string {
	return fmt.Sprintf("%q", x.Value)
}

func (x *String) Type() Type {
	return StringType
}

// A Token is a symbol whereas a String is a literal quoted string
type Token struct {
	Value string
}

func (x *Token) IsBpParseNode() {

}

func (x *Token) Children() []ParseNode {
	return make([]ParseNode, 0)
}

func (x *Token) String() string {
	return fmt.Sprintf("%q", x.Value)
}

type Bool struct {
	Value bool
}

func (x *Bool) Children() []ParseNode {
	return make([]ParseNode, 0)
}

func (x *Bool) IsBpParseNode() {

}

func (x *Bool) Copy() Expression {
	ret := *x
	return &ret
}

func (x *Bool) Eval() Expression {
	return x
}

func (x *Bool) String() string {
	return fmt.Sprintf("%t", x.Value)
}

func (x *Bool) Type() Type {
	return BoolType
}
