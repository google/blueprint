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

package blueprint

import (
	"io"
	"strings"
	"unicode"
)

const (
	indentWidth    = 4
	maxIndentDepth = 2
	lineWidth      = 80
)

var indentString = strings.Repeat(" ", indentWidth*maxIndentDepth)

type StringWriterWriter interface {
	io.StringWriter
	io.Writer
}

type ninjaWriter struct {
	writer io.StringWriter

	justDidBlankLine bool // true if the last operation was a BlankLine
}

func newNinjaWriter(writer io.StringWriter) *ninjaWriter {
	return &ninjaWriter{
		writer: writer,
	}
}

func (n *ninjaWriter) Comment(comment string) error {
	n.justDidBlankLine = false

	const lineHeaderLen = len("# ")
	const maxLineLen = lineWidth - lineHeaderLen

	var lineStart, lastSplitPoint int
	for i, r := range comment {
		if unicode.IsSpace(r) {
			// We know we can safely split the line here.
			lastSplitPoint = i + 1
		}

		var line string
		var writeLine bool
		switch {
		case r == '\n':
			// Output the line without trimming the left so as to allow comments
			// to contain their own indentation.
			line = strings.TrimRightFunc(comment[lineStart:i], unicode.IsSpace)
			writeLine = true

		case (i-lineStart > maxLineLen) && (lastSplitPoint > lineStart):
			// The line has grown too long and is splittable.  Split it at the
			// last split point.
			line = strings.TrimSpace(comment[lineStart:lastSplitPoint])
			writeLine = true
		}

		if writeLine {
			line = strings.TrimSpace("# "+line) + "\n"
			_, err := n.writer.WriteString(line)
			if err != nil {
				return err
			}
			lineStart = lastSplitPoint
		}
	}

	if lineStart != len(comment) {
		line := strings.TrimSpace(comment[lineStart:])
		_, err := n.writer.WriteString("# ")
		if err != nil {
			return err
		}
		_, err = n.writer.WriteString(line)
		if err != nil {
			return err
		}
		_, err = n.writer.WriteString("\n")
		if err != nil {
			return err
		}
	}

	return nil
}

func (n *ninjaWriter) Pool(name string) error {
	n.justDidBlankLine = false
	return n.writeStatement("pool", name)
}

func (n *ninjaWriter) Rule(name string) error {
	n.justDidBlankLine = false
	return n.writeStatement("rule", name)
}

func (n *ninjaWriter) Build(comment string, rule string, outputs, implicitOuts,
	explicitDeps, implicitDeps, orderOnlyDeps, validations []string) error {

	n.justDidBlankLine = false

	const lineWrapLen = len(" $")
	const maxLineLen = lineWidth - lineWrapLen

	wrapper := ninjaWriterWithWrap{
		ninjaWriter: n,
		maxLineLen:  maxLineLen,
	}

	if comment != "" {
		err := wrapper.Comment(comment)
		if err != nil {
			return err
		}
	}

	wrapper.WriteString("build")

	for _, output := range outputs {
		wrapper.WriteStringWithSpace(output)
	}

	if len(implicitOuts) > 0 {
		wrapper.WriteStringWithSpace("|")

		for _, out := range implicitOuts {
			wrapper.WriteStringWithSpace(out)
		}
	}

	wrapper.WriteString(":")

	wrapper.WriteStringWithSpace(rule)

	for _, dep := range explicitDeps {
		wrapper.WriteStringWithSpace(dep)
	}

	if len(implicitDeps) > 0 {
		wrapper.WriteStringWithSpace("|")

		for _, dep := range implicitDeps {
			wrapper.WriteStringWithSpace(dep)
		}
	}

	if len(orderOnlyDeps) > 0 {
		wrapper.WriteStringWithSpace("||")

		for _, dep := range orderOnlyDeps {
			wrapper.WriteStringWithSpace(dep)
		}
	}

	if len(validations) > 0 {
		wrapper.WriteStringWithSpace("|@")

		for _, dep := range validations {
			wrapper.WriteStringWithSpace(dep)
		}
	}

	return wrapper.Flush()
}

func (n *ninjaWriter) Assign(name, value string) error {
	n.justDidBlankLine = false
	_, err := n.writer.WriteString(name)
	if err != nil {
		return err
	}
	_, err = n.writer.WriteString(" = ")
	if err != nil {
		return err
	}
	_, err = n.writer.WriteString(value)
	if err != nil {
		return err
	}
	_, err = n.writer.WriteString("\n")
	if err != nil {
		return err
	}
	return nil
}

func (n *ninjaWriter) ScopedAssign(name, value string) error {
	n.justDidBlankLine = false
	_, err := n.writer.WriteString(indentString[:indentWidth])
	if err != nil {
		return err
	}
	_, err = n.writer.WriteString(name)
	if err != nil {
		return err
	}
	_, err = n.writer.WriteString(" = ")
	if err != nil {
		return err
	}
	_, err = n.writer.WriteString(value)
	if err != nil {
		return err
	}
	_, err = n.writer.WriteString("\n")
	if err != nil {
		return err
	}
	return nil
}

func (n *ninjaWriter) Default(targets ...string) error {
	n.justDidBlankLine = false

	const lineWrapLen = len(" $")
	const maxLineLen = lineWidth - lineWrapLen

	wrapper := ninjaWriterWithWrap{
		ninjaWriter: n,
		maxLineLen:  maxLineLen,
	}

	wrapper.WriteString("default")

	for _, target := range targets {
		wrapper.WriteString(" " + target)
	}

	return wrapper.Flush()
}

func (n *ninjaWriter) Subninja(file string) error {
	n.justDidBlankLine = false
	return n.writeStatement("subninja", file)
}

func (n *ninjaWriter) BlankLine() (err error) {
	// We don't output multiple blank lines in a row.
	if !n.justDidBlankLine {
		n.justDidBlankLine = true
		_, err = n.writer.WriteString("\n")
	}
	return err
}

func (n *ninjaWriter) writeStatement(directive, name string) error {
	_, err := n.writer.WriteString(directive + " ")
	if err != nil {
		return err
	}
	_, err = n.writer.WriteString(name)
	if err != nil {
		return err
	}
	_, err = n.writer.WriteString("\n")
	if err != nil {
		return err
	}
	return nil
}

type ninjaWriterWithWrap struct {
	*ninjaWriter
	maxLineLen int
	writtenLen int
	err        error
}

func (n *ninjaWriterWithWrap) writeString(s string, space bool) {
	if n.err != nil {
		return
	}

	spaceLen := 0
	if space {
		spaceLen = 1
	}

	if n.writtenLen+len(s)+spaceLen > n.maxLineLen {
		_, n.err = n.writer.WriteString(" $\n")
		if n.err != nil {
			return
		}
		_, n.err = n.writer.WriteString(indentString[:indentWidth*2])
		if n.err != nil {
			return
		}
		n.writtenLen = indentWidth * 2
		s = strings.TrimLeftFunc(s, unicode.IsSpace)
	} else if space {
		_, n.err = n.writer.WriteString(" ")
		if n.err != nil {
			return
		}
		n.writtenLen++
	}

	_, n.err = n.writer.WriteString(s)
	n.writtenLen += len(s)
}

func (n *ninjaWriterWithWrap) WriteString(s string) {
	n.writeString(s, false)
}

func (n *ninjaWriterWithWrap) WriteStringWithSpace(s string) {
	n.writeString(s, true)
}

func (n *ninjaWriterWithWrap) Flush() error {
	if n.err != nil {
		return n.err
	}
	_, err := n.writer.WriteString("\n")
	return err
}
