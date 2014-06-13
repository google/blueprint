package blueprint

import (
	"fmt"
	"strings"
)

const eof = -1

var (
	defaultEscaper = strings.NewReplacer(
		"\n", "$\n")
	inputEscaper = strings.NewReplacer(
		"\n", "$\n",
		" ", "$ ")
	outputEscaper = strings.NewReplacer(
		"\n", "$\n",
		" ", "$ ",
		":", "$:")
)

type ninjaString struct {
	strings   []string
	variables []Variable
}

type variableLookup interface {
	LookupVariable(name string) (Variable, error)
}

func simpleNinjaString(str string) *ninjaString {
	return &ninjaString{
		strings: []string{str},
	}
}

// parseNinjaString parses an unescaped ninja string (i.e. all $<something>
// occurrences are expected to be variables or $$) and returns a list of the
// variable names that the string references.
func parseNinjaString(scope variableLookup, str string) (*ninjaString, error) {
	type stateFunc func(int, rune) (stateFunc, error)
	var (
		stringState      stateFunc
		dollarStartState stateFunc
		dollarState      stateFunc
		bracketsState    stateFunc
	)

	var stringStart, varStart int
	var result ninjaString

	stringState = func(i int, r rune) (stateFunc, error) {
		switch {
		case r == '$':
			varStart = i + 1
			return dollarStartState, nil

		case r == eof:
			result.strings = append(result.strings, str[stringStart:i])
			return nil, nil

		default:
			return stringState, nil
		}
	}

	dollarStartState = func(i int, r rune) (stateFunc, error) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '_', r == '-':
			// The beginning of a of the variable name.  Output the string and
			// keep going.
			result.strings = append(result.strings, str[stringStart:i-1])
			return dollarState, nil

		case r == '$':
			// Just a "$$".  Go back to stringState without changing
			// stringStart.
			return stringState, nil

		case r == '{':
			// This is a bracketted variable name (e.g. "${blah.blah}").  Output
			// the string and keep going.
			result.strings = append(result.strings, str[stringStart:i-1])
			varStart = i + 1
			return bracketsState, nil

		case r == eof:
			return nil, fmt.Errorf("unexpected end of string after '$'")

		default:
			// This was some arbitrary character following a dollar sign,
			// which is not allowed.
			return nil, fmt.Errorf("invalid character after '$' at byte "+
				"offset %d", i)
		}
	}

	dollarState = func(i int, r rune) (stateFunc, error) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '_', r == '-':
			// A part of the variable name.  Keep going.
			return dollarState, nil

		case r == '$':
			// A dollar after the variable name (e.g. "$blah$").  Output the
			// variable we have and start a new one.
			v, err := scope.LookupVariable(str[varStart:i])
			if err != nil {
				return nil, err
			}

			result.variables = append(result.variables, v)
			varStart = i + 1
			return dollarState, nil

		case r == eof:
			// This is the end of the variable name.
			v, err := scope.LookupVariable(str[varStart:i])
			if err != nil {
				return nil, err
			}

			result.variables = append(result.variables, v)

			// We always end with a string, even if it's an empty one.
			result.strings = append(result.strings, "")

			return nil, nil

		default:
			// We've just gone past the end of the variable name, so record what
			// we have.
			v, err := scope.LookupVariable(str[varStart:i])
			if err != nil {
				return nil, err
			}

			result.variables = append(result.variables, v)
			stringStart = i
			return stringState, nil
		}
	}

	bracketsState = func(i int, r rune) (stateFunc, error) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '_', r == '-', r == '.':
			// A part of the variable name.  Keep going.
			return bracketsState, nil

		case r == '}':
			if varStart == i {
				// The brackets were immediately closed.  That's no good.
				return nil, fmt.Errorf("empty variable name at byte offset %d",
					i)
			}

			// This is the end of the variable name.
			v, err := scope.LookupVariable(str[varStart:i])
			if err != nil {
				return nil, err
			}

			result.variables = append(result.variables, v)
			stringStart = i + 1
			return stringState, nil

		case r == eof:
			return nil, fmt.Errorf("unexpected end of string in variable name")

		default:
			// This character isn't allowed in a variable name.
			return nil, fmt.Errorf("invalid character in variable name at "+
				"byte offset %d", i)
		}
	}

	state := stringState
	var err error
	for i, r := range str {
		state, err = state(i, r)
		if err != nil {
			return nil, err
		}
	}

	_, err = state(len(str), eof)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func parseNinjaStrings(scope variableLookup, strs []string) ([]*ninjaString,
	error) {

	if len(strs) == 0 {
		return nil, nil
	}
	result := make([]*ninjaString, len(strs))
	for i, str := range strs {
		ninjaStr, err := parseNinjaString(scope, str)
		if err != nil {
			return nil, fmt.Errorf("error parsing element %d: %s", i, err)
		}
		result[i] = ninjaStr
	}
	return result, nil
}

func (n *ninjaString) Value(pkgNames map[*pkg]string) string {
	return n.ValueWithEscaper(pkgNames, defaultEscaper)
}

func (n *ninjaString) ValueWithEscaper(pkgNames map[*pkg]string,
	escaper *strings.Replacer) string {

	str := escaper.Replace(n.strings[0])
	for i, v := range n.variables {
		str += "${" + v.fullName(pkgNames) + "}"
		str += escaper.Replace(n.strings[i+1])
	}
	return str
}

func validateNinjaName(name string) error {
	for i, r := range name {
		valid := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			(r == '_') ||
			(r == '-') ||
			(r == '.')
		if !valid {
			return fmt.Errorf("%q contains an invalid Ninja name character "+
				"at byte offset %d", name, i)
		}
	}
	return nil
}

var builtinRuleArgs = []string{"out", "in"}

func validateArgName(argName string) error {
	err := validateNinjaName(argName)
	if err != nil {
		return err
	}

	// We only allow globals within the rule's package to be used as rule
	// arguments.  A global in another package can always be mirrored into
	// the rule's package by defining a new variable, so this doesn't limit
	// what's possible.  This limitation prevents situations where a Build
	// invocation in another package must use the rule-defining package's
	// import name for a 3rd package in order to set the rule's arguments.
	if strings.ContainsRune(argName, '.') {
		return fmt.Errorf("%q contains a '.' character", argName)
	}

	for _, builtin := range builtinRuleArgs {
		if argName == builtin {
			return fmt.Errorf("%q conflicts with Ninja built-in", argName)
		}
	}

	return nil
}

func validateArgNames(argNames []string) error {
	for _, argName := range argNames {
		err := validateArgName(argName)
		if err != nil {
			return err
		}
	}

	return nil
}
