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
	Comments []*CommentGroup
}

func (f *File) Pos() scanner.Position {
	return scanner.Position{
		Filename: f.Name,
		Line:     1,
		Column:   1,
		Offset:   0,
	}
}

func (f *File) End() scanner.Position {
	if len(f.Defs) > 0 {
		return f.Defs[len(f.Defs)-1].End()
	}
	return noPos
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
	comments []*CommentGroup
	eval     bool
}

func newParser(r io.Reader, scope *Scope) *parser {
	p := &parser{}
	p.scope = scope
	p.scanner.Init(r)
	p.scanner.Error = func(sc *scanner.Scanner, msg string) {
		p.errorf(msg)
	}
	p.scanner.Mode = scanner.ScanIdents | scanner.ScanInts | scanner.ScanStrings |
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
		if p.tok == scanner.Comment {
			var comments []*Comment
			for p.tok == scanner.Comment {
				lines := strings.Split(p.scanner.TokenText(), "\n")
				if len(comments) > 0 && p.scanner.Position.Line > comments[len(comments)-1].End().Line+1 {
					p.comments = append(p.comments, &CommentGroup{Comments: comments})
					comments = nil
				}
				comments = append(comments, &Comment{lines, p.scanner.Position})
				p.tok = p.scanner.Scan()
			}
			p.comments = append(p.comments, &CommentGroup{Comments: comments})
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

func (p *parser) parseAssignment(name string, namePos scanner.Position,
	assigner string) (assignment *Assignment) {

	assignment = new(Assignment)

	pos := p.scanner.Position
	if !p.accept('=') {
		return
	}
	value := p.parseExpression()

	assignment.Name = name
	assignment.NamePos = namePos
	assignment.Value = value
	assignment.OrigValue = value
	assignment.EqualsPos = pos
	assignment.Assigner = assigner

	if p.scope != nil {
		if assigner == "+=" {
			if old, local := p.scope.Get(assignment.Name); old == nil {
				p.errorf("modified non-existent variable %q with +=", assignment.Name)
			} else if !local {
				p.errorf("modified non-local variable %q with +=", assignment.Name)
			} else if old.Referenced {
				p.errorf("modified variable %q with += after referencing", assignment.Name)
			} else {
				val, err := p.evaluateOperator(old.Value, assignment.Value, '+', assignment.EqualsPos)
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

func (p *parser) parseModule(typ string, typPos scanner.Position) *Module {

	compat := false
	lbracePos := p.scanner.Position
	if p.tok == '{' {
		compat = true
	}

	if !p.accept(p.tok) {
		return nil
	}
	properties := p.parsePropertyList(true, compat)
	rbracePos := p.scanner.Position
	if !compat {
		p.accept(')')
	} else {
		p.accept('}')
	}

	return &Module{
		Type:    typ,
		TypePos: typPos,
		Map: Map{
			Properties: properties,
			LBracePos:  lbracePos,
			RBracePos:  rbracePos,
		},
	}
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
		if compat {
			if !p.accept(':') {
				return
			}
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
	property.NamePos = namePos
	property.Value = value
	property.ColonPos = pos

	return
}

func (p *parser) parseExpression() (value Expression) {
	value = p.parseValue()
	switch p.tok {
	case '+':
		return p.parseOperator(value)
	case '-':
		p.errorf("subtraction not supported: %s", p.scanner.String())
		return value
	default:
		return value
	}
}

func (p *parser) evaluateOperator(value1, value2 Expression, operator rune,
	pos scanner.Position) (*Operator, error) {

	value := value1

	if p.eval {
		e1 := value1.Eval()
		e2 := value2.Eval()
		if e1.Type() != e2.Type() {
			return nil, fmt.Errorf("mismatched type in operator %c: %s != %s", operator,
				e1.Type(), e2.Type())
		}

		value = e1.Copy()

		switch operator {
		case '+':
			switch v := value.(type) {
			case *String:
				v.Value += e2.(*String).Value
			case *Int64:
				v.Value += e2.(*Int64).Value
				v.Token = ""
			case *List:
				v.Values = append(v.Values, e2.(*List).Values...)
			case *Map:
				var err error
				v.Properties, err = p.addMaps(v.Properties, e2.(*Map).Properties, pos)
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

	return &Operator{
		Args:        [2]Expression{value1, value2},
		Operator:    operator,
		OperatorPos: pos,
		Value:       value,
	}, nil
}

func (p *parser) addMaps(map1, map2 []*Property, pos scanner.Position) ([]*Property, error) {
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
		if _, ok := inBoth[prop2.Name]; !ok {
			ret = append(ret, prop2)
		}
	}

	return ret, nil
}

func (p *parser) parseOperator(value1 Expression) *Operator {
	operator := p.tok
	pos := p.scanner.Position
	p.accept(operator)

	value2 := p.parseExpression()

	value, err := p.evaluateOperator(value1, value2, operator, pos)
	if err != nil {
		p.error(err)
		return nil
	}

	return value

}

func (p *parser) parseValue() (value Expression) {
	switch p.tok {
	case scanner.Ident:
		return p.parseVariable()
	case '-', scanner.Int: // Integer might have '-' sign ahead ('+' is only treated as operator now)
		return p.parseIntValue()
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

func (p *parser) parseVariable() Expression {
	var value Expression

	switch text := p.scanner.TokenText(); text {
	case "true", "false":
		value = &Bool{
			LiteralPos: p.scanner.Position,
			Value:      text == "true",
			Token:      text,
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
		} else {
			value = &NotEvaluated{}
		}
		value = &Variable{
			Name:    text,
			NamePos: p.scanner.Position,
			Value:   value,
		}
	}

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
		LiteralPos: p.scanner.Position,
		Value:      str,
	}
	p.accept(scanner.String)
	return value
}

func (p *parser) parseIntValue() *Int64 {
	var str string
	literalPos := p.scanner.Position
	if p.tok == '-' {
		str += string(p.tok)
		p.accept(p.tok)
		if p.tok != scanner.Int {
			p.errorf("expected int; found %s", scanner.TokenString(p.tok))
			return nil
		}
	}
	str += p.scanner.TokenText()
	i, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		p.errorf("couldn't parse int: %s", err)
		return nil
	}

	value := &Int64{
		LiteralPos: literalPos,
		Value:      i,
		Token:      str,
	}
	p.accept(scanner.Int)
	return value
}

func (p *parser) parseListValue() *List {
	lBracePos := p.scanner.Position
	if !p.accept('[') {
		return nil
	}

	var elements []Expression
	for p.tok != ']' {
		element := p.parseExpression()
		elements = append(elements, element)

		if p.tok != ',' {
			// There was no comma, so the list is done.
			break
		}

		p.accept(',')
	}

	rBracePos := p.scanner.Position
	p.accept(']')

	return &List{
		LBracePos: lBracePos,
		RBracePos: rBracePos,
		Values:    elements,
	}
}

func (p *parser) parseMapValue() *Map {
	lBracePos := p.scanner.Position
	if !p.accept('{') {
		return nil
	}

	properties := p.parsePropertyList(false, false)

	rBracePos := p.scanner.Position
	p.accept('}')

	return &Map{
		LBracePos:  lBracePos,
		RBracePos:  rBracePos,
		Properties: properties,
	}
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
	if old, ok := s.vars[assignment.Name]; ok {
		return fmt.Errorf("variable already set, previous assignment: %s", old)
	}

	if old, ok := s.inheritedVars[assignment.Name]; ok {
		return fmt.Errorf("variable already set in inherited scope, previous assignment: %s", old)
	}

	s.vars[assignment.Name] = assignment

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
