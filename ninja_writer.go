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
	explicitDeps, implicitDeps, orderOnlyDeps, validations []ninjaString,
	pkgNames map[*packageContext]string) error {

	n.justDidBlankLine = false

	const lineWrapLen = len(" $")
	const maxLineLen = lineWidth - lineWrapLen

	wrapper := &ninjaWriterWithWrap{
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
		wrapper.Space()
		output.ValueWithEscaper(wrapper, pkgNames, outputEscaper)
	}

	if len(implicitOuts) > 0 {
		wrapper.WriteStringWithSpace("|")

		for _, out := range implicitOuts {
			wrapper.Space()
			out.ValueWithEscaper(wrapper, pkgNames, outputEscaper)
		}
	}

	wrapper.WriteString(":")

	wrapper.WriteStringWithSpace(rule)

	for _, dep := range explicitDeps {
		wrapper.Space()
		dep.ValueWithEscaper(wrapper, pkgNames, inputEscaper)
	}

	if len(implicitDeps) > 0 {
		wrapper.WriteStringWithSpace("|")

		for _, dep := range implicitDeps {
			wrapper.Space()
			dep.ValueWithEscaper(wrapper, pkgNames, inputEscaper)
		}
	}

	if len(orderOnlyDeps) > 0 {
		wrapper.WriteStringWithSpace("||")

		for _, dep := range orderOnlyDeps {
			wrapper.Space()
			dep.ValueWithEscaper(wrapper, pkgNames, inputEscaper)
		}
	}

	if len(validations) > 0 {
		wrapper.WriteStringWithSpace("|@")

		for _, dep := range validations {
			wrapper.Space()
			dep.ValueWithEscaper(wrapper, pkgNames, inputEscaper)
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

func (n *ninjaWriter) Default(pkgNames map[*packageContext]string, targets ...ninjaString) error {
	n.justDidBlankLine = false

	const lineWrapLen = len(" $")
	const maxLineLen = lineWidth - lineWrapLen

	wrapper := &ninjaWriterWithWrap{
		ninjaWriter: n,
		maxLineLen:  maxLineLen,
	}

	wrapper.WriteString("default")

	for _, target := range targets {
		wrapper.Space()
		target.ValueWithEscaper(wrapper, pkgNames, outputEscaper)
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

// ninjaWriterWithWrap is an io.StringWriter that writes through to a ninjaWriter, but supports
// user-readable line wrapping on boundaries when ninjaWriterWithWrap.Space is called.
// It collects incoming calls to WriteString until either the line length is exceeded, in which case
// it inserts a wrap before the pending strings and then writes them, or the next call to Space, in
// which case it writes out the pending strings.
//
// WriteString never returns an error, all errors are held until Flush is called.  Once an error has
// occurred all writes become noops.
type ninjaWriterWithWrap struct {
	*ninjaWriter
	// pending lists the strings that have been written since the last call to Space.
	pending []string

	// pendingLen accumulates the lengths of the strings in pending.
	pendingLen int

	// lineLen accumulates the number of bytes on the current line.
	lineLen int

	// maxLineLen is the length of the line before wrapping.
	maxLineLen int

	// space is true if the strings in pending should be preceded by a space.
	space bool

	// err holds any error that has occurred to return in Flush.
	err error
}

// WriteString writes the string to buffer, wrapping on a previous Space call if necessary.
// It never returns an error, all errors are held until Flush is called.
func (n *ninjaWriterWithWrap) WriteString(s string) (written int, noError error) {
	// Always return the full length of the string and a nil error.
	// ninjaWriterWithWrap doesn't return errors to the caller, it saves them until Flush()
	written = len(s)

	if n.err != nil {
		return
	}

	const spaceLen = 1
	if !n.space {
		// No space is pending, so a line wrap can't be inserted before this, so just write
		// the string.
		n.lineLen += len(s)
		_, n.err = n.writer.WriteString(s)
	} else if n.lineLen+len(s)+spaceLen > n.maxLineLen {
		// A space is pending, and the pending strings plus the current string would exceed the
		// maximum line length.  Wrap and indent before the pending space and strings, then write
		// the pending and current strings.
		_, n.err = n.writer.WriteString(" $\n")
		if n.err != nil {
			return
		}
		_, n.err = n.writer.WriteString(indentString[:indentWidth*2])
		if n.err != nil {
			return
		}
		n.lineLen = indentWidth*2 + n.pendingLen
		s = strings.TrimLeftFunc(s, unicode.IsSpace)
		n.pending = append(n.pending, s)
		n.writePending()

		n.space = false
	} else {
		// A space is pending but the current string would not reach the maximum line length,
		// add it to the pending list.
		n.pending = append(n.pending, s)
		n.pendingLen += len(s)
		n.lineLen += len(s)
	}

	return
}

// Space inserts a space that is also a possible wrapping point into the string.
func (n *ninjaWriterWithWrap) Space() {
	if n.err != nil {
		return
	}
	if n.space {
		// A space was already pending, and the space plus any strings written after the space did
		// not reach the maxmimum line length, so write out the old space and pending strings.
		_, n.err = n.writer.WriteString(" ")
		n.lineLen++
		n.writePending()
	}
	n.space = true
}

// writePending writes out all the strings stored in pending and resets it.
func (n *ninjaWriterWithWrap) writePending() {
	if n.err != nil {
		return
	}
	for _, pending := range n.pending {
		_, n.err = n.writer.WriteString(pending)
		if n.err != nil {
			return
		}
	}
	// Reset the length of pending back to 0 without reducing its capacity to avoid reallocating
	// the backing array.
	n.pending = n.pending[:0]
	n.pendingLen = 0
}

// WriteStringWithSpace is a helper that calls Space and WriteString.
func (n *ninjaWriterWithWrap) WriteStringWithSpace(s string) {
	n.Space()
	_, _ = n.WriteString(s)
}

// Flush writes out any pending space or strings and then a newline.  It also returns any errors
// that have previously occurred.
func (n *ninjaWriterWithWrap) Flush() error {
	if n.space {
		_, n.err = n.writer.WriteString(" ")
	}
	n.writePending()
	if n.err != nil {
		return n.err
	}
	_, err := n.writer.WriteString("\n")
	return err
}
