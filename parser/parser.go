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

type File struct {
	Name     string
	Defs     []Definition
	Comments []Comment
}

func parse(p *parser) (file *File, errs []error) {
	defer func() {
		if r := recover(); r != nil {
			if r == errTooManyErrors {
				errs = p.errors
				return
			}
			panic(r)
		}
	}()

	defs := p.parseDefinitions()
	p.accept(scanner.EOF)
	errs = p.errors
	comments := p.comments

	return &File{
		Name:     p.scanner.Filename,
		Defs:     defs,
		Comments: comments,
	}, errs

}

func ParseAndEval(filename string, r io.Reader, scope *Scope) (file *File, errs []error) {
	p := newParser(r, scope)
	p.eval = true
	p.scanner.Filename = filename

	return parse(p)
}

func Parse(filename string, r io.Reader, scope *Scope) (file *File, errs []error) {
	p := newParser(r, scope)
	p.scanner.Filename = filename

	return parse(p)
}

type parser struct {
	scanner  scanner.Scanner
	tok      rune
	errors   []error
	scope    *Scope
	comments []Comment
	eval     bool
}

func newParser(r io.Reader, scope *Scope) *parser {
	p := &parser{}
	p.scope = scope
	p.scanner.Init(r)
	p.scanner.Error = func(sc *scanner.Scanner, msg string) {
		p.errorf(msg)
	}
	p.scanner.Mode = scanner.ScanIdents | scanner.ScanStrings |
		scanner.ScanRawStrings | scanner.ScanComments
	p.next()
	return p
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
			p.errorf("expected %s, found %s", scanner.TokenString(tok),
				scanner.TokenString(p.tok))
			return false
		}
		p.next()
	}
	return true
}

func (p *parser) next() {
	if p.tok != scanner.EOF {
		p.tok = p.scanner.Scan()
		for p.tok == scanner.Comment {
			lines := strings.Split(p.scanner.TokenText(), "\n")
			p.comments = append(p.comments, Comment{lines, p.scanner.Position})
			p.tok = p.scanner.Scan()
		}
	}
	return
}

func (p *parser) parseDefinitions() (defs []Definition) {
	for {
		switch p.tok {
		case scanner.Ident:
			ident := p.scanner.TokenText()
			pos := p.scanner.Position

			p.accept(scanner.Ident)

			switch p.tok {
			case '+':
				p.accept('+')
				defs = append(defs, p.parseAssignment(ident, pos, "+="))
			case '=':
				defs = append(defs, p.parseAssignment(ident, pos, "="))
			case '{', '(':
				defs = append(defs, p.parseModule(ident, pos))
			default:
				p.errorf("expected \"=\" or \"+=\" or \"{\" or \"(\", found %s",
					scanner.TokenString(p.tok))
			}
		case scanner.EOF:
			return
		default:
			p.errorf("expected assignment or module definition, found %s",
				scanner.TokenString(p.tok))
			return
		}
	}
}

func (p *parser) parseAssignment(name string,
	namePos scanner.Position, assigner string) (assignment *Assignment) {

	assignment = new(Assignment)

	pos := p.scanner.Position
	if !p.accept('=') {
		return
	}
	value := p.parseExpression()

	assignment.Name = Ident{name, namePos}
	assignment.Value = value
	assignment.OrigValue = value
	assignment.Pos = pos
	assignment.Assigner = assigner

	if p.scope != nil {
		if assigner == "+=" {
			if old, local := p.scope.Get(assignment.Name.Name); old == nil {
				p.errorf("modified non-existent variable %q with +=", assignment.Name.Name)
			} else if !local {
				p.errorf("modified non-local variable %q with +=", assignment.Name.Name)
			} else if old.Referenced {
				p.errorf("modified variable %q with += after referencing",
					assignment.Name.Name)
			} else {
				val, err := p.evaluateOperator(old.Value, assignment.Value, '+', assignment.Pos)
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

	return
}

func (p *parser) parseModule(typ string,
	typPos scanner.Position) (module *Module) {

	module = new(Module)
	compat := false
	lbracePos := p.scanner.Position
	if p.tok == '{' {
		compat = true
	}

	if !p.accept(p.tok) {
		return
	}
	properties := p.parsePropertyList(true, compat)
	rbracePos := p.scanner.Position
	if !compat {
		p.accept(')')
	} else {
		p.accept('}')
	}

	module.Type = Ident{typ, typPos}
	module.Properties = properties
	module.LbracePos = lbracePos
	module.RbracePos = rbracePos
	return
}

func (p *parser) parsePropertyList(isModule, compat bool) (properties []*Property) {
	for p.tok == scanner.Ident {
		property := p.parseProperty(isModule, compat)
		properties = append(properties, property)

		if p.tok != ',' {
			// There was no comma, so the list is done.
			break
		}

		p.accept(',')
	}

	return
}

func (p *parser) parseProperty(isModule, compat bool) (property *Property) {
	property = new(Property)

	name := p.scanner.TokenText()
	namePos := p.scanner.Position
	p.accept(scanner.Ident)
	pos := p.scanner.Position

	if isModule {
		if compat && p.tok == ':' {
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

	property.Name = Ident{name, namePos}
	property.Value = value
	property.Pos = pos

	return
}

func (p *parser) parseExpression() (value Value) {
	value = p.parseValue()
	switch p.tok {
	case '+':
		return p.parseOperator(value)
	default:
		return value
	}
}

func (p *parser) evaluateOperator(value1, value2 Value, operator rune,
	pos scanner.Position) (Value, error) {

	value := Value{}

	if p.eval {
		if value1.Type != value2.Type {
			return Value{}, fmt.Errorf("mismatched type in operator %c: %s != %s", operator,
				value1.Type, value2.Type)
		}

		value = value1
		value.Variable = ""

		switch operator {
		case '+':
			switch value1.Type {
			case String:
				value.StringValue = value1.StringValue + value2.StringValue
			case List:
				value.ListValue = append([]Value{}, value1.ListValue...)
				value.ListValue = append(value.ListValue, value2.ListValue...)
			case Map:
				var err error
				value.MapValue, err = p.addMaps(value.MapValue, value2.MapValue, pos)
				if err != nil {
					return Value{}, err
				}
			default:
				return Value{}, fmt.Errorf("operator %c not supported on type %s", operator,
					value1.Type)
			}
		default:
			panic("unknown operator " + string(operator))
		}
	}

	value.Expression = &Expression{
		Args:     [2]Value{value1, value2},
		Operator: operator,
		Pos:      pos,
	}

	return value, nil
}

func (p *parser) addMaps(map1, map2 []*Property, pos scanner.Position) ([]*Property, error) {
	ret := make([]*Property, 0, len(map1))

	inMap1 := make(map[string]*Property)
	inMap2 := make(map[string]*Property)
	inBoth := make(map[string]*Property)

	for _, prop1 := range map1 {
		inMap1[prop1.Name.Name] = prop1
	}

	for _, prop2 := range map2 {
		inMap2[prop2.Name.Name] = prop2
		if _, ok := inMap1[prop2.Name.Name]; ok {
			inBoth[prop2.Name.Name] = prop2
		}
	}

	for _, prop1 := range map1 {
		if prop2, ok := inBoth[prop1.Name.Name]; ok {
			var err error
			newProp := *prop1
			newProp.Value, err = p.evaluateOperator(prop1.Value, prop2.Value, '+', pos)
			if err != nil {
				return nil, err
			}
			ret = append(ret, &newProp)
		} else {
			ret = append(ret, prop1)
		}
	}

	for _, prop2 := range map2 {
		if _, ok := inBoth[prop2.Name.Name]; !ok {
			ret = append(ret, prop2)
		}
	}

	return ret, nil
}

func (p *parser) parseOperator(value1 Value) Value {
	operator := p.tok
	pos := p.scanner.Position
	p.accept(operator)

	value2 := p.parseExpression()

	value, err := p.evaluateOperator(value1, value2, operator, pos)
	if err != nil {
		p.error(err)
		return Value{}
	}

	return value
}

func (p *parser) parseValue() (value Value) {
	switch p.tok {
	case scanner.Ident:
		return p.parseVariable()
	case scanner.String:
		return p.parseStringValue()
	case '[':
		return p.parseListValue()
	case '{':
		return p.parseMapValue()
	default:
		p.errorf("expected bool, list, or string value; found %s",
			scanner.TokenString(p.tok))
		return
	}
}

func (p *parser) parseVariable() (value Value) {
	switch text := p.scanner.TokenText(); text {
	case "true":
		value.Type = Bool
		value.BoolValue = true
	case "false":
		value.Type = Bool
		value.BoolValue = false
	default:
		variable := p.scanner.TokenText()
		if p.eval {
			if assignment, local := p.scope.Get(variable); assignment == nil {
				p.errorf("variable %q is not set", variable)
			} else {
				if local {
					assignment.Referenced = true
				}
				value = assignment.Value
			}
		}
		value.Variable = variable
	}
	value.Pos = p.scanner.Position

	p.accept(scanner.Ident)
	return
}

func (p *parser) parseStringValue() (value Value) {
	value.Type = String
	value.Pos = p.scanner.Position
	str, err := strconv.Unquote(p.scanner.TokenText())
	if err != nil {
		p.errorf("couldn't parse string: %s", err)
		return
	}
	value.StringValue = str
	p.accept(scanner.String)
	return
}

func (p *parser) parseListValue() (value Value) {
	value.Type = List
	value.Pos = p.scanner.Position
	if !p.accept('[') {
		return
	}

	var elements []Value
	for p.tok != ']' {
		element := p.parseExpression()
		if p.eval && element.Type != String {
			p.errorf("Expected string in list, found %s", element.String())
			return
		}
		elements = append(elements, element)

		if p.tok != ',' {
			// There was no comma, so the list is done.
			break
		}

		p.accept(',')
	}

	value.ListValue = elements
	value.EndPos = p.scanner.Position

	p.accept(']')
	return
}

func (p *parser) parseMapValue() (value Value) {
	value.Type = Map
	value.Pos = p.scanner.Position
	if !p.accept('{') {
		return
	}

	properties := p.parsePropertyList(false, false)
	value.MapValue = properties

	value.EndPos = p.scanner.Position
	p.accept('}')
	return
}

type Expression struct {
	Args     [2]Value
	Operator rune
	Pos      scanner.Position
}

func (e *Expression) Copy() *Expression {
	ret := *e
	ret.Args[0] = e.Args[0].Copy()
	ret.Args[1] = e.Args[1].Copy()
	return &ret
}

func (e *Expression) String() string {
	return fmt.Sprintf("(%s %c %s)@%d:%s", e.Args[0].String(), e.Operator, e.Args[1].String(),
		e.Pos.Offset, e.Pos)
}

type ValueType int

const (
	Bool ValueType = iota
	String
	List
	Map
)

func (p ValueType) String() string {
	switch p {
	case Bool:
		return "bool"
	case String:
		return "string"
	case List:
		return "list"
	case Map:
		return "map"
	default:
		panic(fmt.Errorf("unknown value type: %d", p))
	}
}

type Definition interface {
	String() string
	definitionTag()
}

type Assignment struct {
	Name       Ident
	Value      Value
	OrigValue  Value
	Pos        scanner.Position
	Assigner   string
	Referenced bool
}

func (a *Assignment) String() string {
	return fmt.Sprintf("%s@%d:%s %s %s", a.Name, a.Pos.Offset, a.Pos, a.Assigner, a.Value)
}

func (a *Assignment) definitionTag() {}

type Module struct {
	Type       Ident
	Properties []*Property
	LbracePos  scanner.Position
	RbracePos  scanner.Position
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
	propertyStrings := make([]string, len(m.Properties))
	for i, property := range m.Properties {
		propertyStrings[i] = property.String()
	}
	return fmt.Sprintf("%s@%d:%s-%d:%s{%s}", m.Type,
		m.LbracePos.Offset, m.LbracePos,
		m.RbracePos.Offset, m.RbracePos,
		strings.Join(propertyStrings, ", "))
}

func (m *Module) definitionTag() {}

type Property struct {
	Name  Ident
	Value Value
	Pos   scanner.Position
}

func (p *Property) Copy() *Property {
	ret := *p
	ret.Value = p.Value.Copy()
	return &ret
}

func (p *Property) String() string {
	return fmt.Sprintf("%s@%d:%s: %s", p.Name, p.Pos.Offset, p.Pos, p.Value)
}

type Ident struct {
	Name string
	Pos  scanner.Position
}

func (i Ident) String() string {
	return fmt.Sprintf("%s@%d:%s", i.Name, i.Pos.Offset, i.Pos)
}

type Value struct {
	Type        ValueType
	BoolValue   bool
	StringValue string
	ListValue   []Value
	MapValue    []*Property
	Expression  *Expression
	Variable    string
	Pos         scanner.Position
	EndPos      scanner.Position
}

func (p Value) Copy() Value {
	ret := p
	if p.MapValue != nil {
		ret.MapValue = make([]*Property, len(p.MapValue))
		for i := range p.MapValue {
			ret.MapValue[i] = p.MapValue[i].Copy()
		}
	}
	if p.ListValue != nil {
		ret.ListValue = make([]Value, len(p.ListValue))
		for i := range p.ListValue {
			ret.ListValue[i] = p.ListValue[i].Copy()
		}
	}
	if p.Expression != nil {
		ret.Expression = p.Expression.Copy()
	}
	return ret
}

func (p Value) String() string {
	var s string
	if p.Variable != "" {
		s += p.Variable + " = "
	}
	if p.Expression != nil {
		s += p.Expression.String()
	}
	switch p.Type {
	case Bool:
		s += fmt.Sprintf("%t@%d:%s", p.BoolValue, p.Pos.Offset, p.Pos)
	case String:
		s += fmt.Sprintf("%q@%d:%s", p.StringValue, p.Pos.Offset, p.Pos)
	case List:
		valueStrings := make([]string, len(p.ListValue))
		for i, value := range p.ListValue {
			valueStrings[i] = value.String()
		}
		s += fmt.Sprintf("@%d:%s-%d:%s[%s]", p.Pos.Offset, p.Pos, p.EndPos.Offset, p.EndPos,
			strings.Join(valueStrings, ", "))
	case Map:
		propertyStrings := make([]string, len(p.MapValue))
		for i, property := range p.MapValue {
			propertyStrings[i] = property.String()
		}
		s += fmt.Sprintf("@%d:%s-%d:%s{%s}", p.Pos.Offset, p.Pos, p.EndPos.Offset, p.EndPos,
			strings.Join(propertyStrings, ", "))
	default:
		panic(fmt.Errorf("bad property type: %d", p.Type))
	}

	return s
}

type Scope struct {
	vars          map[string]*Assignment
	inheritedVars map[string]*Assignment
}

func NewScope(s *Scope) *Scope {
	newScope := &Scope{
		vars:          make(map[string]*Assignment),
		inheritedVars: make(map[string]*Assignment),
	}

	if s != nil {
		for k, v := range s.vars {
			newScope.inheritedVars[k] = v
		}
		for k, v := range s.inheritedVars {
			newScope.inheritedVars[k] = v
		}
	}

	return newScope
}

func (s *Scope) Add(assignment *Assignment) error {
	if old, ok := s.vars[assignment.Name.Name]; ok {
		return fmt.Errorf("variable already set, previous assignment: %s", old)
	}

	if old, ok := s.inheritedVars[assignment.Name.Name]; ok {
		return fmt.Errorf("variable already set in inherited scope, previous assignment: %s", old)
	}

	s.vars[assignment.Name.Name] = assignment

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

type Comment struct {
	Comment []string
	Pos     scanner.Position
}

func (c Comment) String() string {
	l := 0
	for _, comment := range c.Comment {
		l += len(comment) + 1
	}
	buf := make([]byte, 0, l)
	for _, comment := range c.Comment {
		buf = append(buf, comment...)
		buf = append(buf, '\n')
	}

	return string(buf)
}

// Return the text of the comment with // or /* and */ stripped
func (c Comment) Text() string {
	l := 0
	for _, comment := range c.Comment {
		l += len(comment) + 1
	}
	buf := make([]byte, 0, l)

	blockComment := false
	if strings.HasPrefix(c.Comment[0], "/*") {
		blockComment = true
	}

	for i, comment := range c.Comment {
		if blockComment {
			if i == 0 {
				comment = strings.TrimPrefix(comment, "/*")
			}
			if i == len(c.Comment)-1 {
				comment = strings.TrimSuffix(comment, "*/")
			}
		} else {
			comment = strings.TrimPrefix(comment, "//")
		}
		buf = append(buf, comment...)
		buf = append(buf, '\n')
	}

	return string(buf)
}

// Return the line number that the comment ends on
func (c Comment) EndLine() int {
	return c.Pos.Line + len(c.Comment) - 1
}
