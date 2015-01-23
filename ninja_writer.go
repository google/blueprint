package blueprint

import (
	"fmt"
	"io"
	"strings"
	"unicode"
)

const (
	indentWidth = 4
	lineWidth   = 80
)

type ninjaWriter struct {
	writer io.Writer

	justDidBlankLine bool // true if the last operation was a BlankLine
}

func newNinjaWriter(writer io.Writer) *ninjaWriter {
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
			_, err := io.WriteString(n.writer, line)
			if err != nil {
				return err
			}
			lineStart = lastSplitPoint
		}
	}

	if lineStart != len(comment) {
		line := strings.TrimSpace(comment[lineStart:])
		_, err := fmt.Fprintf(n.writer, "# %s\n", line)
		if err != nil {
			return err
		}
	}

	return nil
}

func (n *ninjaWriter) Pool(name string) error {
	n.justDidBlankLine = false
	_, err := fmt.Fprintf(n.writer, "pool %s\n", name)
	return err
}

func (n *ninjaWriter) Rule(name string) error {
	n.justDidBlankLine = false
	_, err := fmt.Fprintf(n.writer, "rule %s\n", name)
	return err
}

func (n *ninjaWriter) Build(rule string, outputs, explicitDeps, implicitDeps,
	orderOnlyDeps []string) error {

	n.justDidBlankLine = false

	const lineWrapLen = len(" $")
	const maxLineLen = lineWidth - lineWrapLen

	line := "build"

	appendWithWrap := func(s string) (err error) {
		if len(line)+len(s) > maxLineLen {
			_, err = fmt.Fprintf(n.writer, "%s $\n", line)
			line = strings.Repeat(" ", indentWidth*2)
			s = strings.TrimLeftFunc(s, unicode.IsSpace)
		}
		line += s
		return
	}

	for _, output := range outputs {
		err := appendWithWrap(" " + output)
		if err != nil {
			return err
		}
	}

	err := appendWithWrap(":")
	if err != nil {
		return err
	}

	err = appendWithWrap(" " + rule)
	if err != nil {
		return err
	}

	for _, dep := range explicitDeps {
		err := appendWithWrap(" " + dep)
		if err != nil {
			return err
		}
	}

	if len(implicitDeps) > 0 {
		err := appendWithWrap(" |")
		if err != nil {
			return err
		}

		for _, dep := range implicitDeps {
			err := appendWithWrap(" " + dep)
			if err != nil {
				return err
			}
		}
	}

	if len(orderOnlyDeps) > 0 {
		err := appendWithWrap(" ||")
		if err != nil {
			return err
		}

		for _, dep := range orderOnlyDeps {
			err := appendWithWrap(" " + dep)
			if err != nil {
				return err
			}
		}
	}

	_, err = fmt.Fprintln(n.writer, line)
	return err
}

func (n *ninjaWriter) Assign(name, value string) error {
	n.justDidBlankLine = false
	_, err := fmt.Fprintf(n.writer, "%s = %s\n", name, value)
	return err
}

func (n *ninjaWriter) ScopedAssign(name, value string) error {
	n.justDidBlankLine = false
	indent := strings.Repeat(" ", indentWidth)
	_, err := fmt.Fprintf(n.writer, "%s%s = %s\n", indent, name, value)
	return err
}

func (n *ninjaWriter) Default(targets ...string) error {
	n.justDidBlankLine = false

	const lineWrapLen = len(" $")
	const maxLineLen = lineWidth - lineWrapLen

	line := "default"

	appendWithWrap := func(s string) (err error) {
		if len(line)+len(s) > maxLineLen {
			_, err = fmt.Fprintf(n.writer, "%s $\n", line)
			line = strings.Repeat(" ", indentWidth*2)
			s = strings.TrimLeftFunc(s, unicode.IsSpace)
		}
		line += s
		return
	}

	for _, target := range targets {
		err := appendWithWrap(" " + target)
		if err != nil {
			return err
		}
	}

	_, err := fmt.Fprintln(n.writer, line)
	return err
}

func (n *ninjaWriter) BlankLine() (err error) {
	// We don't output multiple blank lines in a row.
	if !n.justDidBlankLine {
		n.justDidBlankLine = true
		_, err = io.WriteString(n.writer, "\n")
	}
	return err
}

func writeAssignments(w io.Writer, indent int, assignments ...string) error {
	var maxNameLen int
	for i := 0; i < len(assignments); i += 2 {
		name := assignments[i]
		err := validateNinjaName(name)
		if err != nil {
			return err
		}
		if maxNameLen < len(name) {
			maxNameLen = len(name)
		}
	}

	indentStr := strings.Repeat(" ", indent*indentWidth)
	extraIndentStr := strings.Repeat(" ", (indent+1)*indentWidth)
	replacer := strings.NewReplacer("\n", "$\n"+extraIndentStr)

	for i := 0; i < len(assignments); i += 2 {
		name := assignments[i]
		value := replacer.Replace(assignments[i+1])
		_, err := fmt.Fprintf(w, "%s% *s = %s\n", indentStr, maxNameLen, name,
			value)
		if err != nil {
			return err
		}
	}

	return nil
}
