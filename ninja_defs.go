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
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// A Deps value indicates the dependency file format that Ninja should expect to
// be output by a compiler.
type Deps int

const (
	DepsNone Deps = iota
	DepsGCC
	DepsMSVC
)

func (d Deps) String() string {
	switch d {
	case DepsNone:
		return "none"
	case DepsGCC:
		return "gcc"
	case DepsMSVC:
		return "msvc"
	default:
		panic(fmt.Sprintf("unknown deps value: %d", d))
	}
}

// A PoolParams object contains the set of parameters that make up a Ninja pool
// definition.
type PoolParams struct {
	Comment string // The comment that will appear above the definition.
	Depth   int    // The Ninja pool depth.
}

// A RuleParams object contains the set of parameters that make up a Ninja rule
// definition.
type RuleParams struct {
	// These fields correspond to a Ninja variable of the same name.
	Command        string // The command that Ninja will run for the rule.
	Depfile        string // The dependency file name.
	Deps           Deps   // The format of the dependency file.
	Description    string // The description that Ninja will print for the rule.
	Generator      bool   // Whether the rule generates the Ninja manifest file.
	Pool           Pool   // The Ninja pool to which the rule belongs.
	Restat         bool   // Whether Ninja should re-stat the rule's outputs.
	Rspfile        string // The response file.
	RspfileContent string // The response file content.

	// These fields are used internally in Blueprint
	CommandDeps      []string // Command-specific implicit dependencies to prepend to builds
	CommandOrderOnly []string // Command-specific order-only dependencies to prepend to builds
	Comment          string   // The comment that will appear above the definition.
}

// A BuildParams object contains the set of parameters that make up a Ninja
// build statement.  Each field except for Args corresponds with a part of the
// Ninja build statement.  The Args field contains variable names and values
// that are set within the build statement's scope in the Ninja file.
type BuildParams struct {
	Comment         string            // The comment that will appear above the definition.
	Depfile         string            // The dependency file name.
	Deps            Deps              // The format of the dependency file.
	Description     string            // The description that Ninja will print for the build.
	Rule            Rule              // The rule to invoke.
	Outputs         []string          // The list of explicit output targets.
	ImplicitOutputs []string          // The list of implicit output targets.
	Inputs          []string          // The list of explicit input dependencies.
	Implicits       []string          // The list of implicit input dependencies.
	OrderOnly       []string          // The list of order-only dependencies.
	Args            map[string]string // The variable/value pairs to set.
	Optional        bool              // Skip outputting a default statement
}

// A poolDef describes a pool definition.  It does not include the name of the
// pool.
type poolDef struct {
	Comment string
	Depth   int
}

func parsePoolParams(scope scope, params *PoolParams) (*poolDef,
	error) {

	def := &poolDef{
		Comment: params.Comment,
		Depth:   params.Depth,
	}

	return def, nil
}

func (p *poolDef) WriteTo(nw *ninjaWriter, name string) error {
	if p.Comment != "" {
		err := nw.Comment(p.Comment)
		if err != nil {
			return err
		}
	}

	err := nw.Pool(name)
	if err != nil {
		return err
	}

	return nw.ScopedAssign("depth", strconv.Itoa(p.Depth))
}

// A ruleDef describes a rule definition.  It does not include the name of the
// rule.
type ruleDef struct {
	CommandDeps      []ninjaString
	CommandOrderOnly []ninjaString
	Comment          string
	Pool             Pool
	Variables        map[string]ninjaString
}

func parseRuleParams(scope scope, params *RuleParams) (*ruleDef,
	error) {

	r := &ruleDef{
		Comment:   params.Comment,
		Pool:      params.Pool,
		Variables: make(map[string]ninjaString),
	}

	if params.Command == "" {
		return nil, fmt.Errorf("encountered rule params with no command " +
			"specified")
	}

	if r.Pool != nil && !scope.IsPoolVisible(r.Pool) {
		return nil, fmt.Errorf("Pool %s is not visible in this scope", r.Pool)
	}

	value, err := parseNinjaString(scope, params.Command)
	if err != nil {
		return nil, fmt.Errorf("error parsing Command param: %s", err)
	}
	r.Variables["command"] = value

	if params.Depfile != "" {
		value, err = parseNinjaString(scope, params.Depfile)
		if err != nil {
			return nil, fmt.Errorf("error parsing Depfile param: %s", err)
		}
		r.Variables["depfile"] = value
	}

	if params.Deps != DepsNone {
		r.Variables["deps"] = simpleNinjaString(params.Deps.String())
	}

	if params.Description != "" {
		value, err = parseNinjaString(scope, params.Description)
		if err != nil {
			return nil, fmt.Errorf("error parsing Description param: %s", err)
		}
		r.Variables["description"] = value
	}

	if params.Generator {
		r.Variables["generator"] = simpleNinjaString("true")
	}

	if params.Restat {
		r.Variables["restat"] = simpleNinjaString("true")
	}

	if params.Rspfile != "" {
		value, err = parseNinjaString(scope, params.Rspfile)
		if err != nil {
			return nil, fmt.Errorf("error parsing Rspfile param: %s", err)
		}
		r.Variables["rspfile"] = value
	}

	if params.RspfileContent != "" {
		value, err = parseNinjaString(scope, params.RspfileContent)
		if err != nil {
			return nil, fmt.Errorf("error parsing RspfileContent param: %s",
				err)
		}
		r.Variables["rspfile_content"] = value
	}

	r.CommandDeps, err = parseNinjaStrings(scope, params.CommandDeps)
	if err != nil {
		return nil, fmt.Errorf("error parsing CommandDeps param: %s", err)
	}

	r.CommandOrderOnly, err = parseNinjaStrings(scope, params.CommandOrderOnly)
	if err != nil {
		return nil, fmt.Errorf("error parsing CommandDeps param: %s", err)
	}

	return r, nil
}

func (r *ruleDef) WriteTo(nw *ninjaWriter, name string,
	pkgNames map[*packageContext]string) error {

	if r.Comment != "" {
		err := nw.Comment(r.Comment)
		if err != nil {
			return err
		}
	}

	err := nw.Rule(name)
	if err != nil {
		return err
	}

	if r.Pool != nil {
		err = nw.ScopedAssign("pool", r.Pool.fullName(pkgNames))
		if err != nil {
			return err
		}
	}

	err = writeVariables(nw, r.Variables, pkgNames)
	if err != nil {
		return err
	}

	return nil
}

// A buildDef describes a build target definition.
type buildDef struct {
	Comment         string
	Rule            Rule
	RuleDef         *ruleDef
	Outputs         []ninjaString
	ImplicitOutputs []ninjaString
	Inputs          []ninjaString
	Implicits       []ninjaString
	OrderOnly       []ninjaString
	Args            map[Variable]ninjaString
	Variables       map[string]ninjaString
	Optional        bool
}

func parseBuildParams(scope scope, params *BuildParams) (*buildDef,
	error) {

	comment := params.Comment
	rule := params.Rule

	b := &buildDef{
		Comment: comment,
		Rule:    rule,
	}

	setVariable := func(name string, value ninjaString) {
		if b.Variables == nil {
			b.Variables = make(map[string]ninjaString)
		}
		b.Variables[name] = value
	}

	if !scope.IsRuleVisible(rule) {
		return nil, fmt.Errorf("Rule %s is not visible in this scope", rule)
	}

	if len(params.Outputs) == 0 {
		return nil, errors.New("Outputs param has no elements")
	}

	var err error
	b.Outputs, err = parseNinjaStrings(scope, params.Outputs)
	if err != nil {
		return nil, fmt.Errorf("error parsing Outputs param: %s", err)
	}

	b.ImplicitOutputs, err = parseNinjaStrings(scope, params.ImplicitOutputs)
	if err != nil {
		return nil, fmt.Errorf("error parsing ImplicitOutputs param: %s", err)
	}

	b.Inputs, err = parseNinjaStrings(scope, params.Inputs)
	if err != nil {
		return nil, fmt.Errorf("error parsing Inputs param: %s", err)
	}

	b.Implicits, err = parseNinjaStrings(scope, params.Implicits)
	if err != nil {
		return nil, fmt.Errorf("error parsing Implicits param: %s", err)
	}

	b.OrderOnly, err = parseNinjaStrings(scope, params.OrderOnly)
	if err != nil {
		return nil, fmt.Errorf("error parsing OrderOnly param: %s", err)
	}

	b.Optional = params.Optional

	if params.Depfile != "" {
		value, err := parseNinjaString(scope, params.Depfile)
		if err != nil {
			return nil, fmt.Errorf("error parsing Depfile param: %s", err)
		}
		setVariable("depfile", value)
	}

	if params.Deps != DepsNone {
		setVariable("deps", simpleNinjaString(params.Deps.String()))
	}

	if params.Description != "" {
		value, err := parseNinjaString(scope, params.Description)
		if err != nil {
			return nil, fmt.Errorf("error parsing Description param: %s", err)
		}
		setVariable("description", value)
	}

	argNameScope := rule.scope()

	if len(params.Args) > 0 {
		b.Args = make(map[Variable]ninjaString)
		for name, value := range params.Args {
			if !rule.isArg(name) {
				return nil, fmt.Errorf("unknown argument %q", name)
			}

			argVar, err := argNameScope.LookupVariable(name)
			if err != nil {
				// This shouldn't happen.
				return nil, fmt.Errorf("argument lookup error: %s", err)
			}

			ninjaValue, err := parseNinjaString(scope, value)
			if err != nil {
				return nil, fmt.Errorf("error parsing variable %q: %s", name,
					err)
			}

			b.Args[argVar] = ninjaValue
		}
	}

	return b, nil
}

func (b *buildDef) WriteTo(nw *ninjaWriter, pkgNames map[*packageContext]string) error {
	var (
		comment       = b.Comment
		rule          = b.Rule.fullName(pkgNames)
		outputs       = valueList(b.Outputs, pkgNames, outputEscaper)
		implicitOuts  = valueList(b.ImplicitOutputs, pkgNames, outputEscaper)
		explicitDeps  = valueList(b.Inputs, pkgNames, inputEscaper)
		implicitDeps  = valueList(b.Implicits, pkgNames, inputEscaper)
		orderOnlyDeps = valueList(b.OrderOnly, pkgNames, inputEscaper)
	)

	if b.RuleDef != nil {
		implicitDeps = append(valueList(b.RuleDef.CommandDeps, pkgNames, inputEscaper), implicitDeps...)
		orderOnlyDeps = append(valueList(b.RuleDef.CommandOrderOnly, pkgNames, inputEscaper), orderOnlyDeps...)
	}

	err := nw.Build(comment, rule, outputs, implicitOuts, explicitDeps, implicitDeps, orderOnlyDeps)
	if err != nil {
		return err
	}

	args := make(map[string]string)

	for argVar, value := range b.Args {
		args[argVar.fullName(pkgNames)] = value.Value(pkgNames)
	}

	err = writeVariables(nw, b.Variables, pkgNames)
	if err != nil {
		return err
	}

	var keys []string
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		err = nw.ScopedAssign(name, args[name])
		if err != nil {
			return err
		}
	}

	if !b.Optional {
		err = nw.Default(outputs...)
		if err != nil {
			return err
		}
	}

	return nw.BlankLine()
}

func valueList(list []ninjaString, pkgNames map[*packageContext]string,
	escaper *strings.Replacer) []string {

	result := make([]string, len(list))
	for i, ninjaStr := range list {
		result[i] = ninjaStr.ValueWithEscaper(pkgNames, escaper)
	}
	return result
}

func writeVariables(nw *ninjaWriter, variables map[string]ninjaString,
	pkgNames map[*packageContext]string) error {
	var keys []string
	for k := range variables {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		err := nw.ScopedAssign(name, variables[name].Value(pkgNames))
		if err != nil {
			return err
		}
	}
	return nil
}
