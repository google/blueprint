package parser

import (
	"errors"
	"fmt"
	"io"
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

func Parse(filename string, r io.Reader) (defs []Definition, errs []error) {
	p := newParser(r)
	p.scanner.Filename = filename

	defer func() {
		if r := recover(); r != nil {
			if r == errTooManyErrors {
				defs = nil
				errs = p.errors
				return
			}
			panic(r)
		}
	}()

	defs = p.parseDefinitions()
	p.accept(scanner.EOF)
	errs = p.errors

	return
}

type parser struct {
	scanner scanner.Scanner
	tok     rune
	errors  []error
}

func newParser(r io.Reader) *parser {
	p := &parser{}
	p.scanner.Init(r)
	p.scanner.Error = func(sc *scanner.Scanner, msg string) {
		p.errorf(msg)
	}
	p.scanner.Mode = scanner.ScanIdents | scanner.ScanStrings |
		scanner.ScanComments | scanner.SkipComments
	p.next()
	return p
}

func (p *parser) errorf(format string, args ...interface{}) {
	pos := p.scanner.Position
	if !pos.IsValid() {
		pos = p.scanner.Pos()
	}
	err := &ParseError{
		Err: fmt.Errorf(format, args...),
		Pos: pos,
	}
	p.errors = append(p.errors, err)
	if len(p.errors) >= maxErrors {
		panic(errTooManyErrors)
	}
}

func (p *parser) accept(toks ...rune) bool {
	for _, tok := range toks {
		if p.tok != tok {
			p.errorf("expected %s, found %s", scanner.TokenString(tok),
				scanner.TokenString(p.tok))
			return false
		}
		if p.tok != scanner.EOF {
			p.tok = p.scanner.Scan()
		}
	}
	return true
}

func (p *parser) next() {
	if p.tok != scanner.EOF {
		p.tok = p.scanner.Scan()
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
			case '=':
				defs = append(defs, p.parseAssignment(ident, pos))
			case '{':
				defs = append(defs, p.parseModule(ident, pos))
			default:
				p.errorf("expected \"=\" or \"{\", found %s",
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
	pos scanner.Position) (assignment *Assignment) {

	assignment = new(Assignment)

	if !p.accept('=') {
		return
	}
	value := p.parseValue()

	assignment.Name = name
	assignment.Value = value
	assignment.Pos = pos

	return
}

func (p *parser) parseModule(typ string,
	pos scanner.Position) (module *Module) {

	module = new(Module)

	if !p.accept('{') {
		return
	}
	properties := p.parsePropertyList()
	p.accept('}')

	module.Type = typ
	module.Properties = properties
	module.Pos = pos
	return
}

func (p *parser) parsePropertyList() (properties []*Property) {
	for p.tok == scanner.Ident {
		property := p.parseProperty()
		properties = append(properties, property)

		if p.tok != ',' {
			// There was no comma, so the list is done.
			break
		}

		p.accept(',')
	}

	return
}

func (p *parser) parseProperty() (property *Property) {
	property = new(Property)

	name := p.scanner.TokenText()
	pos := p.scanner.Position
	if !p.accept(scanner.Ident, ':') {
		return
	}
	value := p.parseValue()

	property.Name = name
	property.Value = value
	property.Pos = pos

	return
}

func (p *parser) parseValue() (value Value) {
	switch p.tok {
	case scanner.Ident:
		return p.parseBoolValue()
	case scanner.String:
		return p.parseStringValue()
	case '[':
		return p.parseListValue()
	default:
		p.errorf("expected bool, list, or string value; found %s",
			scanner.TokenString(p.tok))
		return
	}
}

func (p *parser) parseBoolValue() (value Value) {
	value.Type = Bool
	value.Pos = p.scanner.Position
	switch text := p.scanner.TokenText(); text {
	case "true":
		value.BoolValue = true
	case "false":
		value.BoolValue = false
	default:
		p.errorf("expected true or false; found %q", text)
		return
	}
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
	for p.tok == scanner.String {
		elements = append(elements, p.parseStringValue())

		if p.tok != ',' {
			// There was no comma, so the list is done.
			break
		}

		p.accept(',')
	}

	value.ListValue = elements

	p.accept(']')
	return
}

type ValueType int

const (
	Bool ValueType = iota
	String
	List
)

func (p ValueType) String() string {
	switch p {
	case Bool:
		return "bool"
	case String:
		return "string"
	case List:
		return "list"
	default:
		panic(fmt.Errorf("unknown value type: %d", p))
	}
}

type Definition interface {
	String() string
	definitionTag()
}

type Assignment struct {
	Name  string
	Value Value
	Pos   scanner.Position
}

func (a *Assignment) String() string {
	return fmt.Sprintf("%s@%d:%s: %s", a.Name, a.Pos.Offset, a.Pos, a.Value)
}

func (a *Assignment) definitionTag() {}

type Module struct {
	Type       string
	Properties []*Property
	Pos        scanner.Position
}

func (m *Module) String() string {
	propertyStrings := make([]string, len(m.Properties))
	for i, property := range m.Properties {
		propertyStrings[i] = property.String()
	}
	return fmt.Sprintf("%s@%d:%s{%s}", m.Type, m.Pos.Offset, m.Pos,
		strings.Join(propertyStrings, ", "))
}

func (m *Module) definitionTag() {}

type Property struct {
	Name  string
	Value Value
	Pos   scanner.Position
}

func (p *Property) String() string {
	return fmt.Sprintf("%s@%d:%s: %s", p.Name, p.Pos.Offset, p.Pos, p.Value)
}

type Value struct {
	Type        ValueType
	BoolValue   bool
	StringValue string
	ListValue   []Value
	Pos         scanner.Position
}

func (p Value) String() string {
	switch p.Type {
	case Bool:
		return fmt.Sprintf("%t@%d:%s", p.BoolValue, p.Pos.Offset, p.Pos)
	case String:
		return fmt.Sprintf("%q@%d:%s", p.StringValue, p.Pos.Offset, p.Pos)
	case List:
		valueStrings := make([]string, len(p.ListValue))
		for i, value := range p.ListValue {
			valueStrings[i] = value.String()
		}
		return fmt.Sprintf("@%d:%s[%s]", p.Pos.Offset, p.Pos,
			strings.Join(valueStrings, ", "))
	default:
		panic(fmt.Errorf("bad property type: %d", p.Type))
	}
}
