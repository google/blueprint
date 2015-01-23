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
	wsNone whitespace = iota
	wsBoth
	wsAfter
	wsBefore
	wsMaybe
)

type printer struct {
	defs     []Definition
	comments []Comment

	curComment int
	prev       scanner.Position

	ws whitespace

	output []byte

	indentList     []int
	wsBuf          []byte
	forceLineBreak int
}

func newPrinter(file *File) *printer {
	return &printer{
		defs:       file.Defs,
		comments:   file.Comments,
		indentList: []int{0},
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
	p.printToken(assignment.Name.Name, assignment.Name.Pos, wsMaybe)
	p.printToken("=", assignment.Pos, wsBoth)
	p.printValue(assignment.Value)
}

func (p *printer) printModule(module *Module) {
	p.printToken(module.Type.Name, module.Type.Pos, wsBoth)
	p.printMap(module.Properties, module.LbracePos, module.RbracePos, true)
	p.forceLineBreak = 2
}

func (p *printer) printValue(value Value) {
	if value.Variable != "" {
		p.printToken(value.Variable, value.Pos, wsMaybe)
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
			p.printToken(s, value.Pos, wsMaybe)
		case String:
			p.printToken(strconv.Quote(value.StringValue), value.Pos, wsMaybe)
		case List:
			p.printList(value.ListValue, value.Pos, value.EndPos)
		case Map:
			p.printMap(value.MapValue, value.Pos, value.EndPos, false)
		default:
			panic(fmt.Errorf("bad property type: %d", value.Type))
		}
	}
}

func (p *printer) printList(list []Value, pos, endPos scanner.Position) {
	p.printToken("[", pos, wsBefore)
	if len(list) > 1 || pos.Line != endPos.Line {
		p.forceLineBreak = 1
		p.indent(p.curIndent() + 4)
		for _, value := range list {
			p.printValue(value)
			p.printToken(",", noPos, wsAfter)
			p.forceLineBreak = 1
		}
		p.unindent()
	} else {
		for _, value := range list {
			p.printValue(value)
		}
	}
	p.printToken("]", endPos, wsAfter)
}

func (p *printer) printMap(list []*Property, pos, endPos scanner.Position, isModule bool) {
	if isModule {
		p.printToken("(", pos, wsNone)
	} else {
		p.printToken("{", pos, wsBefore)
	}
	if len(list) > 0 || pos.Line != endPos.Line {
		p.forceLineBreak = 1
		p.indent(p.curIndent() + 4)
		for _, prop := range list {
			p.printProperty(prop, isModule)
			p.printToken(",", noPos, wsAfter)
			p.forceLineBreak = 1
		}
		p.unindent()
	}
	if isModule {
		p.printToken(")", endPos, wsAfter)
	} else {
		p.printToken("}", endPos, wsAfter)
	}
}

func (p *printer) printExpression(expression Expression) {
	p.printValue(expression.Args[0])
	p.printToken(string(expression.Operator), expression.Pos, wsBoth)
	p.printValue(expression.Args[1])
}

func (p *printer) printProperty(property *Property, isModule bool) {
	p.printToken(property.Name.Name, property.Name.Pos, wsMaybe)
	if isModule {
		p.printToken("=", property.Pos, wsBoth)
	} else {
		p.printToken(":", property.Pos, wsAfter)
	}
	p.printValue(property.Value)
}

// Print a single token, including any necessary comments or whitespace between
// this token and the previously printed token
func (p *printer) printToken(s string, pos scanner.Position, ws whitespace) {
	p.printComments(pos, false)
	if p.forceLineBreak > 0 || p.prev.Line != 0 && pos.Line > p.prev.Line {
		p.printLineBreak(pos.Line - p.prev.Line)
	} else {
		p.printWhitespace(ws)
	}
	p.output = append(p.output, s...)
	p.ws = ws
	if pos != noPos {
		p.prev = pos
	}
}

// Print all comments that occur before position pos
func (p *printer) printComments(pos scanner.Position, flush bool) {
	for p.curComment < len(p.comments) && p.comments[p.curComment].Pos.Offset < pos.Offset {
		p.printComment(p.comments[p.curComment])
		p.curComment++
	}
}

// Print a single comment, which may be a multi-line comment
func (p *printer) printComment(comment Comment) {
	commentLines := strings.Split(comment.Comment, "\n")
	pos := comment.Pos
	for _, line := range commentLines {
		if p.prev.Line != 0 && pos.Line > p.prev.Line {
			// Comment is on the next line
			p.printLineBreak(pos.Line - p.prev.Line)
		} else {
			// Comment is on the current line
			p.printWhitespace(wsBoth)
		}
		p.output = append(p.output, strings.TrimSpace(line)...)
		p.prev = pos
		pos.Line++
	}
	p.ws = wsBoth
}

// Print one or two line breaks.  n <= 0 is only valid if forceLineBreak is set,
// n > 2 is collapsed to a single blank line.
func (p *printer) printLineBreak(n int) {
	if n > 2 {
		n = 2
	}

	if p.forceLineBreak > n {
		if p.forceLineBreak == 0 {
			panic("unexpected 0 line break")
		}
		n = p.forceLineBreak
	}

	for i := 0; i < n; i++ {
		p.output = append(p.output, '\n')
	}

	p.pad(0, p.curIndent())
	p.forceLineBreak = 0
	p.ws = wsNone
}

// Print any necessary whitespace before the next token, based on the current
// ws value and the previous ws value.
func (p *printer) printWhitespace(ws whitespace) {
	if (ws == wsBefore || ws == wsBoth) && p.ws != wsNone ||
		ws == wsMaybe && (p.ws == wsMaybe || p.ws == wsAfter || p.ws == wsBoth) {

		p.output = append(p.output, ' ')
	}
	p.ws = ws
}

// Print any comments that occur after the last token, and a trailing newline
func (p *printer) flush() {
	for p.curComment < len(p.comments) {
		p.printComment(p.comments[p.curComment])
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
