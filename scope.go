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
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A Variable represents a global Ninja variable definition that will be written
// to the output .ninja file.  A variable may contain references to other global
// Ninja variables, but circular variable references are not allowed.
type Variable interface {
	packageContext() *packageContext
	name() string                                        // "foo"
	fullName(pkgNames map[*packageContext]string) string // "pkg.foo" or "path.to.pkg.foo"
	memoizeFullName(pkgNames map[*packageContext]string) // precompute fullName if desired
	value(config interface{}) (ninjaString, error)
	String() string
}

// A Pool represents a Ninja pool that will be written to the output .ninja
// file.
type Pool interface {
	packageContext() *packageContext
	name() string                                        // "foo"
	fullName(pkgNames map[*packageContext]string) string // "pkg.foo" or "path.to.pkg.foo"
	memoizeFullName(pkgNames map[*packageContext]string) // precompute fullName if desired
	def(config interface{}) (*poolDef, error)
	String() string
}

// A Rule represents a Ninja build rule that will be written to the output
// .ninja file.
type Rule interface {
	packageContext() *packageContext
	name() string                                        // "foo"
	fullName(pkgNames map[*packageContext]string) string // "pkg.foo" or "path.to.pkg.foo"
	memoizeFullName(pkgNames map[*packageContext]string) // precompute fullName if desired
	def(config interface{}) (*ruleDef, error)
	scope() *basicScope
	isArg(argName string) bool
	String() string
}

type basicScope struct {
	parent    *basicScope
	variables map[string]Variable
	pools     map[string]Pool
	rules     map[string]Rule
	imports   map[string]*basicScope
}

func newScope(parent *basicScope) *basicScope {
	return &basicScope{
		parent:    parent,
		variables: make(map[string]Variable),
		pools:     make(map[string]Pool),
		rules:     make(map[string]Rule),
		imports:   make(map[string]*basicScope),
	}
}

func makeRuleScope(parent *basicScope, argNames map[string]bool) *basicScope {
	scope := newScope(parent)
	for argName := range argNames {
		_, err := scope.LookupVariable(argName)
		if err != nil {
			arg := &argVariable{argName}
			err = scope.AddVariable(arg)
			if err != nil {
				// This should not happen.  We should have already checked that
				// the name is valid and that the scope doesn't have a variable
				// with this name.
				panic(err)
			}
		}
	}

	// We treat built-in variables like arguments for the purpose of this scope.
	for _, builtin := range builtinRuleArgs {
		arg := &argVariable{builtin}
		err := scope.AddVariable(arg)
		if err != nil {
			panic(err)
		}
	}

	return scope
}

func (s *basicScope) LookupVariable(name string) (Variable, error) {
	dotIndex := strings.IndexRune(name, '.')
	if dotIndex >= 0 {
		// The variable name looks like "pkg.var"
		if dotIndex+1 == len(name) {
			return nil, fmt.Errorf("variable name %q ends with a '.'", name)
		}
		if strings.ContainsRune(name[dotIndex+1:], '.') {
			return nil, fmt.Errorf("variable name %q contains multiple '.' "+
				"characters", name)
		}

		pkgName := name[:dotIndex]
		varName := name[dotIndex+1:]

		first, _ := utf8.DecodeRuneInString(varName)
		if !unicode.IsUpper(first) {
			return nil, fmt.Errorf("cannot refer to unexported name %q", name)
		}

		importedScope, err := s.lookupImportedScope(pkgName)
		if err != nil {
			return nil, err
		}

		v, ok := importedScope.variables[varName]
		if !ok {
			return nil, fmt.Errorf("package %q does not contain variable %q",
				pkgName, varName)
		}

		return v, nil
	} else {
		// The variable name has no package part; just "var"
		for ; s != nil; s = s.parent {
			v, ok := s.variables[name]
			if ok {
				return v, nil
			}
		}
		return nil, fmt.Errorf("undefined variable %q", name)
	}
}

func (s *basicScope) IsRuleVisible(rule Rule) bool {
	_, isBuiltin := rule.(*builtinRule)
	if isBuiltin {
		return true
	}

	name := rule.name()

	for s != nil {
		if s.rules[name] == rule {
			return true
		}

		for _, import_ := range s.imports {
			if import_.rules[name] == rule {
				return true
			}
		}

		s = s.parent
	}

	return false
}

func (s *basicScope) IsPoolVisible(pool Pool) bool {
	_, isBuiltin := pool.(*builtinPool)
	if isBuiltin {
		return true
	}

	name := pool.name()

	for s != nil {
		if s.pools[name] == pool {
			return true
		}

		for _, import_ := range s.imports {
			if import_.pools[name] == pool {
				return true
			}
		}

		s = s.parent
	}

	return false
}

func (s *basicScope) lookupImportedScope(pkgName string) (*basicScope, error) {
	for ; s != nil; s = s.parent {
		importedScope, ok := s.imports[pkgName]
		if ok {
			return importedScope, nil
		}
	}
	return nil, fmt.Errorf("unknown imported package %q (missing call to "+
		"blueprint.Import()?)", pkgName)
}

func (s *basicScope) AddImport(name string, importedScope *basicScope) error {
	_, present := s.imports[name]
	if present {
		return fmt.Errorf("import %q is already defined in this scope", name)
	}
	s.imports[name] = importedScope
	return nil
}

func (s *basicScope) AddVariable(v Variable) error {
	name := v.name()
	_, present := s.variables[name]
	if present {
		return fmt.Errorf("variable %q is already defined in this scope", name)
	}
	s.variables[name] = v
	return nil
}

func (s *basicScope) AddPool(p Pool) error {
	name := p.name()
	_, present := s.pools[name]
	if present {
		return fmt.Errorf("pool %q is already defined in this scope", name)
	}
	s.pools[name] = p
	return nil
}

func (s *basicScope) AddRule(r Rule) error {
	name := r.name()
	_, present := s.rules[name]
	if present {
		return fmt.Errorf("rule %q is already defined in this scope", name)
	}
	s.rules[name] = r
	return nil
}

type localScope struct {
	namePrefix string
	scope      *basicScope
}

func newLocalScope(parent *basicScope, namePrefix string) *localScope {
	return &localScope{
		namePrefix: namePrefix,
		scope:      newScope(parent),
	}
}

// ReparentTo sets the localScope's parent scope to the scope of the given
// package context.  This allows a ModuleContext and SingletonContext to call
// a function defined in a different Go package and have that function retain
// access to all of the package-scoped variables of its own package.
func (s *localScope) ReparentTo(pctx PackageContext) {
	s.scope.parent = pctx.getScope()
}

func (s *localScope) LookupVariable(name string) (Variable, error) {
	return s.scope.LookupVariable(name)
}

func (s *localScope) IsRuleVisible(rule Rule) bool {
	return s.scope.IsRuleVisible(rule)
}

func (s *localScope) IsPoolVisible(pool Pool) bool {
	return s.scope.IsPoolVisible(pool)
}

func (s *localScope) AddLocalVariable(name, value string) (*localVariable,
	error) {

	err := validateNinjaName(name)
	if err != nil {
		return nil, err
	}

	if strings.ContainsRune(name, '.') {
		return nil, fmt.Errorf("local variable name %q contains '.'", name)
	}

	ninjaValue, err := parseNinjaString(s.scope, value)
	if err != nil {
		return nil, err
	}

	v := &localVariable{
		fullName_: s.namePrefix + name,
		name_:     name,
		value_:    ninjaValue,
	}

	err = s.scope.AddVariable(v)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func (s *localScope) AddLocalRule(name string, params *RuleParams,
	argNames ...string) (*localRule, error) {

	err := validateNinjaName(name)
	if err != nil {
		return nil, err
	}

	err = validateArgNames(argNames)
	if err != nil {
		return nil, fmt.Errorf("invalid argument name: %s", err)
	}

	argNamesSet := make(map[string]bool)
	for _, argName := range argNames {
		argNamesSet[argName] = true
	}

	ruleScope := makeRuleScope(s.scope, argNamesSet)

	def, err := parseRuleParams(ruleScope, params)
	if err != nil {
		return nil, err
	}

	r := &localRule{
		fullName_: s.namePrefix + name,
		name_:     name,
		def_:      def,
		argNames:  argNamesSet,
		scope_:    ruleScope,
	}

	err = s.scope.AddRule(r)
	if err != nil {
		return nil, err
	}

	return r, nil
}

type localVariable struct {
	fullName_ string
	name_     string
	value_    ninjaString
}

func (l *localVariable) packageContext() *packageContext {
	return nil
}

func (l *localVariable) name() string {
	return l.name_
}

func (l *localVariable) fullName(pkgNames map[*packageContext]string) string {
	return l.fullName_
}

func (l *localVariable) memoizeFullName(pkgNames map[*packageContext]string) {
	// Nothing to do, full name is known at initialization.
}

func (l *localVariable) value(interface{}) (ninjaString, error) {
	return l.value_, nil
}

func (l *localVariable) String() string {
	return "<local var>:" + l.fullName_
}

type localRule struct {
	fullName_ string
	name_     string
	def_      *ruleDef
	argNames  map[string]bool
	scope_    *basicScope
}

func (l *localRule) packageContext() *packageContext {
	return nil
}

func (l *localRule) name() string {
	return l.name_
}

func (l *localRule) fullName(pkgNames map[*packageContext]string) string {
	return l.fullName_
}

func (l *localRule) memoizeFullName(pkgNames map[*packageContext]string) {
	// Nothing to do, full name is known at initialization.
}

func (l *localRule) def(interface{}) (*ruleDef, error) {
	return l.def_, nil
}

func (r *localRule) scope() *basicScope {
	return r.scope_
}

func (r *localRule) isArg(argName string) bool {
	return r.argNames[argName]
}

func (r *localRule) String() string {
	return "<local rule>:" + r.fullName_
}
