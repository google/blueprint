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
	"bytes"
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

type scope interface {
	LookupVariable(name string) (Variable, error)
	IsRuleVisible(rule Rule) bool
	IsPoolVisible(pool Pool) bool
}

func simpleNinjaString(str string) *ninjaString {
	return &ninjaString{
		strings: []string{str},
	}
}

type parseState struct {
	scope       scope
	str         string
	stringStart int
	varStart    int
	result      *ninjaString
}

func (ps *parseState) pushVariable(v Variable) {
	if len(ps.result.variables) == len(ps.result.strings) {
		// Last push was a variable, we need a blank string separator
		ps.result.strings = append(ps.result.strings, "")
	}
	ps.result.variables = append(ps.result.variables, v)
}

func (ps *parseState) pushString(s string) {
	if len(ps.result.strings) != len(ps.result.variables) {
		panic("oops, pushed string after string")
	}
	ps.result.strings = append(ps.result.strings, s)
}

type stateFunc func(*parseState, int, rune) (stateFunc, error)

// parseNinjaString parses an unescaped ninja string (i.e. all $<something>
// occurrences are expected to be variables or $$) and returns a list of the
// variable names that the string references.
func parseNinjaString(scope scope, str string) (*ninjaString, error) {
	// naively pre-allocate slices by counting $ signs
	n := strings.Count(str, "$")
	result := &ninjaString{
		strings:   make([]string, 0, n+1),
		variables: make([]Variable, 0, n),
	}

	parseState := &parseState{
		scope:  scope,
		str:    str,
		result: result,
	}

	state := parseStringState
	var err error
	for i := 0; i < len(str); i++ {
		r := rune(str[i])
		state, err = state(parseState, i, r)
		if err != nil {
			return nil, err
		}
	}

	_, err = state(parseState, len(parseState.str), eof)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func parseStringState(state *parseState, i int, r rune) (stateFunc, error) {
	switch {
	case r == '$':
		state.varStart = i + 1
		return parseDollarStartState, nil

	case r == eof:
		state.pushString(state.str[state.stringStart:i])
		return nil, nil

	default:
		return parseStringState, nil
	}
}

func parseDollarStartState(state *parseState, i int, r rune) (stateFunc, error) {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
		r >= '0' && r <= '9', r == '_', r == '-':
		// The beginning of a of the variable name.  Output the string and
		// keep going.
		state.pushString(state.str[state.stringStart : i-1])
		return parseDollarState, nil

	case r == '$':
		// Just a "$$".  Go back to parseStringState without changing
		// state.stringStart.
		return parseStringState, nil

	case r == '{':
		// This is a bracketted variable name (e.g. "${blah.blah}").  Output
		// the string and keep going.
		state.pushString(state.str[state.stringStart : i-1])
		state.varStart = i + 1
		return parseBracketsState, nil

	case r == eof:
		return nil, fmt.Errorf("unexpected end of string after '$'")

	default:
		// This was some arbitrary character following a dollar sign,
		// which is not allowed.
		return nil, fmt.Errorf("invalid character after '$' at byte "+
			"offset %d", i)
	}
}

func parseDollarState(state *parseState, i int, r rune) (stateFunc, error) {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
		r >= '0' && r <= '9', r == '_', r == '-':
		// A part of the variable name.  Keep going.
		return parseDollarState, nil

	case r == '$':
		// A dollar after the variable name (e.g. "$blah$").  Output the
		// variable we have and start a new one.
		v, err := state.scope.LookupVariable(state.str[state.varStart:i])
		if err != nil {
			return nil, err
		}

		state.pushVariable(v)
		state.varStart = i + 1
		state.stringStart = i

		return parseDollarStartState, nil

	case r == eof:
		// This is the end of the variable name.
		v, err := state.scope.LookupVariable(state.str[state.varStart:i])
		if err != nil {
			return nil, err
		}

		state.pushVariable(v)

		// We always end with a string, even if it's an empty one.
		state.pushString("")

		return nil, nil

	default:
		// We've just gone past the end of the variable name, so record what
		// we have.
		v, err := state.scope.LookupVariable(state.str[state.varStart:i])
		if err != nil {
			return nil, err
		}

		state.pushVariable(v)
		state.stringStart = i
		return parseStringState, nil
	}
}

func parseBracketsState(state *parseState, i int, r rune) (stateFunc, error) {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
		r >= '0' && r <= '9', r == '_', r == '-', r == '.':
		// A part of the variable name.  Keep going.
		return parseBracketsState, nil

	case r == '}':
		if state.varStart == i {
			// The brackets were immediately closed.  That's no good.
			return nil, fmt.Errorf("empty variable name at byte offset %d",
				i)
		}

		// This is the end of the variable name.
		v, err := state.scope.LookupVariable(state.str[state.varStart:i])
		if err != nil {
			return nil, err
		}

		state.pushVariable(v)
		state.stringStart = i + 1
		return parseStringState, nil

	case r == eof:
		return nil, fmt.Errorf("unexpected end of string in variable name")

	default:
		// This character isn't allowed in a variable name.
		return nil, fmt.Errorf("invalid character in variable name at "+
			"byte offset %d", i)
	}
}

func parseNinjaStrings(scope scope, strs []string) ([]*ninjaString,
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

func (n *ninjaString) Value(pkgNames map[*PackageContext]string) string {
	return n.ValueWithEscaper(pkgNames, defaultEscaper)
}

func (n *ninjaString) ValueWithEscaper(pkgNames map[*PackageContext]string,
	escaper *strings.Replacer) string {

	str := escaper.Replace(n.strings[0])
	for i, v := range n.variables {
		str += "${" + v.fullName(pkgNames) + "}"
		str += escaper.Replace(n.strings[i+1])
	}
	return str
}

func (n *ninjaString) Eval(variables map[Variable]*ninjaString) (string, error) {
	str := n.strings[0]
	for i, v := range n.variables {
		variable, ok := variables[v]
		if !ok {
			return "", fmt.Errorf("no such global variable: %s", v)
		}
		value, err := variable.Eval(variables)
		if err != nil {
			return "", err
		}
		str += value + n.strings[i+1]
	}
	return str, nil
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
				"%q at byte offset %d", name, r, i)
		}
	}
	return nil
}

func toNinjaName(name string) string {
	ret := bytes.Buffer{}
	ret.Grow(len(name))
	for _, r := range name {
		valid := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			(r == '_') ||
			(r == '-') ||
			(r == '.')
		if valid {
			ret.WriteRune(r)
		} else {
			ret.WriteRune('_')
		}
	}

	return ret.String()
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
