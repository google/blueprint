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
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

// A PackageContext provides a way to create package-scoped Ninja pools,
// rules, and variables.  A Go package should create a single unexported
// package-scoped PackageContext variable that it uses to create all package-
// scoped Ninja object definitions.  This PackageContext object should then be
// passed to all calls to define module- or singleton-specific Ninja
// definitions.  For example:
//
//     package blah
//
//     import (
//         "blueprint"
//     )
//
//     var (
//         pctx = NewPackageContext("path/to/blah")
//
//         myPrivateVar = pctx.StaticVariable("myPrivateVar", "abcdef")
//         MyExportedVar = pctx.StaticVariable("MyExportedVar", "$myPrivateVar 123456!")
//
//         SomeRule = pctx.StaticRule(...)
//     )
//
//     // ...
//
//     func (m *MyModule) GenerateBuildActions(ctx blueprint.Module) {
//         ctx.Build(pctx, blueprint.BuildParams{
//             Rule:    SomeRule,
//             Outputs: []string{"$myPrivateVar"},
//         })
//     }
type PackageContext interface {
	Import(pkgPath string)
	ImportAs(as, pkgPath string)

	StaticVariable(name, value string) Variable
	VariableFunc(name string, f func(config interface{}) (string, error)) Variable
	VariableConfigMethod(name string, method interface{}) Variable

	StaticPool(name string, params PoolParams) Pool
	PoolFunc(name string, f func(interface{}) (PoolParams, error)) Pool

	StaticRule(name string, params RuleParams, argNames ...string) Rule
	RuleFunc(name string, f func(interface{}) (RuleParams, error), argNames ...string) Rule

	AddNinjaFileDeps(deps ...string)

	getScope() *basicScope
}

type packageContext struct {
	fullName      string
	shortName     string
	pkgPath       string
	scope         *basicScope
	ninjaFileDeps []string
}

var _ PackageContext = &packageContext{}

func (p *packageContext) getScope() *basicScope {
	return p.scope
}

var packageContexts = map[string]*packageContext{}

// NewPackageContext creates a PackageContext object for a given package.  The
// pkgPath argument should always be set to the full path used to import the
// package.  This function may only be called from a Go package's init()
// function or as part of a package-scoped variable initialization.
func NewPackageContext(pkgPath string) PackageContext {
	checkCalledFromInit()

	if _, present := packageContexts[pkgPath]; present {
		panic(fmt.Errorf("package %q already has a package context", pkgPath))
	}

	pkgName := pkgPathToName(pkgPath)
	err := validateNinjaName(pkgName)
	if err != nil {
		panic(err)
	}

	i := strings.LastIndex(pkgPath, "/")
	shortName := pkgPath[i+1:]

	p := &packageContext{
		fullName:  pkgName,
		shortName: shortName,
		pkgPath:   pkgPath,
		scope:     newScope(nil),
	}

	packageContexts[pkgPath] = p

	return p
}

var Phony Rule = NewBuiltinRule("phony")

var Console Pool = NewBuiltinPool("console")

var errRuleIsBuiltin = errors.New("the rule is a built-in")
var errPoolIsBuiltin = errors.New("the pool is a built-in")
var errVariableIsArg = errors.New("argument variables have no value")

// checkCalledFromInit panics if a Go package's init function is not on the
// call stack.
func checkCalledFromInit() {
	for skip := 3; ; skip++ {
		_, funcName, ok := callerName(skip)
		if !ok {
			panic("not called from an init func")
		}

		if funcName == "init" || strings.HasPrefix(funcName, "init·") ||
			funcName == "init.ializers" || strings.HasPrefix(funcName, "init.") {
			return
		}
	}
}

// A regex to find a package path within a function name. It finds the shortest string that is
// followed by '.' and doesn't have any '/'s left.
var pkgPathRe = regexp.MustCompile(`^(.*?)\.([^/]+)$`)

// callerName returns the package path and function name of the calling
// function.  The skip argument has the same meaning as the skip argument of
// runtime.Callers.
func callerName(skip int) (pkgPath, funcName string, ok bool) {
	var pc [1]uintptr
	n := runtime.Callers(skip+1, pc[:])
	if n != 1 {
		return "", "", false
	}
	frames := runtime.CallersFrames(pc[:])
	frame, _ := frames.Next()
	f := frame.Function
	s := pkgPathRe.FindStringSubmatch(f)
	if len(s) < 3 {
		panic(fmt.Errorf("failed to extract package path and function name from %q", f))
	}

	return s[1], s[2], true
}

// pkgPathToName makes a Ninja-friendly name out of a Go package name by
// replaceing all the '/' characters with '.'.  We assume the results are
// unique, though this is not 100% guaranteed for Go package names that
// already contain '.' characters. Disallowing package names with '.' isn't
// reasonable since many package names contain the name of the hosting site
// (e.g. "code.google.com").  In practice this probably isn't really a
// problem.
func pkgPathToName(pkgPath string) string {
	return strings.Replace(pkgPath, "/", ".", -1)
}

// Import enables access to the exported Ninja pools, rules, and variables
// that are defined at the package scope of another Go package.  Go's
// visibility rules apply to these references - capitalized names indicate
// that something is exported.  It may only be called from a Go package's
// init() function.  The Go package path passed to Import must have already
// been imported into the Go package using a Go import statement.  The
// imported variables may then be accessed from Ninja strings as
// "${pkg.Variable}", while the imported rules can simply be accessed as
// exported Go variables from the package.  For example:
//
//     import (
//         "blueprint"
//         "foo/bar"
//     )
//
//     var pctx = NewPackagePath("blah")
//
//     func init() {
//         pctx.Import("foo/bar")
//     }
//
//     ...
//
//     func (m *MyModule) GenerateBuildActions(ctx blueprint.Module) {
//         ctx.Build(pctx, blueprint.BuildParams{
//             Rule:    bar.SomeRule,
//             Outputs: []string{"${bar.SomeVariable}"},
//         })
//     }
//
// Note that the local name used to refer to the package in Ninja variable names
// is derived from pkgPath by extracting the last path component.  This differs
// from Go's import declaration, which derives the local name from the package
// clause in the imported package.  By convention these names are made to match,
// but this is not required.
func (p *packageContext) Import(pkgPath string) {
	checkCalledFromInit()
	importPkg, ok := packageContexts[pkgPath]
	if !ok {
		panic(fmt.Errorf("package %q has no context", pkgPath))
	}

	err := p.scope.AddImport(importPkg.shortName, importPkg.scope)
	if err != nil {
		panic(err)
	}
}

// ImportAs provides the same functionality as Import, but it allows the local
// name that will be used to refer to the package to be specified explicitly.
// It may only be called from a Go package's init() function.
func (p *packageContext) ImportAs(as, pkgPath string) {
	checkCalledFromInit()
	importPkg, ok := packageContexts[pkgPath]
	if !ok {
		panic(fmt.Errorf("package %q has no context", pkgPath))
	}

	err := validateNinjaName(as)
	if err != nil {
		panic(err)
	}

	err = p.scope.AddImport(as, importPkg.scope)
	if err != nil {
		panic(err)
	}
}

type staticVariable struct {
	pctx   *packageContext
	name_  string
	value_ string
}

// StaticVariable returns a Variable whose value does not depend on any
// configuration information.  It may only be called during a Go package's
// initialization - either from the init() function or as part of a package-
// scoped variable's initialization.
//
// This function is usually used to initialize a package-scoped Go variable that
// represents a Ninja variable that will be output.  The name argument should
// exactly match the Go variable name, and the value string may reference other
// Ninja variables that are visible within the calling Go package.
func (p *packageContext) StaticVariable(name, value string) Variable {
	checkCalledFromInit()
	err := validateNinjaName(name)
	if err != nil {
		panic(err)
	}

	v := &staticVariable{p, name, value}
	err = p.scope.AddVariable(v)
	if err != nil {
		panic(err)
	}

	return v
}

func (v *staticVariable) packageContext() *packageContext {
	return v.pctx
}

func (v *staticVariable) name() string {
	return v.name_
}

func (v *staticVariable) fullName(pkgNames map[*packageContext]string) string {
	return packageNamespacePrefix(pkgNames[v.pctx]) + v.name_
}

func (v *staticVariable) value(interface{}) (ninjaString, error) {
	ninjaStr, err := parseNinjaString(v.pctx.scope, v.value_)
	if err != nil {
		err = fmt.Errorf("error parsing variable %s value: %s", v, err)
		panic(err)
	}
	return ninjaStr, nil
}

func (v *staticVariable) String() string {
	return v.pctx.pkgPath + "." + v.name_
}

type variableFunc struct {
	pctx   *packageContext
	name_  string
	value_ func(interface{}) (string, error)
}

// VariableFunc returns a Variable whose value is determined by a function that
// takes a config object as input and returns either the variable value or an
// error.  It may only be called during a Go package's initialization - either
// from the init() function or as part of a package-scoped variable's
// initialization.
//
// This function is usually used to initialize a package-scoped Go variable that
// represents a Ninja variable that will be output.  The name argument should
// exactly match the Go variable name, and the value string returned by f may
// reference other Ninja variables that are visible within the calling Go
// package.
func (p *packageContext) VariableFunc(name string,
	f func(config interface{}) (string, error)) Variable {

	checkCalledFromInit()

	err := validateNinjaName(name)
	if err != nil {
		panic(err)
	}

	v := &variableFunc{p, name, f}
	err = p.scope.AddVariable(v)
	if err != nil {
		panic(err)
	}

	return v
}

// VariableConfigMethod returns a Variable whose value is determined by calling
// a method on the config object.  The method must take no arguments and return
// a single string that will be the variable's value.  It may only be called
// during a Go package's initialization - either from the init() function or as
// part of a package-scoped variable's initialization.
//
// This function is usually used to initialize a package-scoped Go variable that
// represents a Ninja variable that will be output.  The name argument should
// exactly match the Go variable name, and the value string returned by method
// may reference other Ninja variables that are visible within the calling Go
// package.
func (p *packageContext) VariableConfigMethod(name string,
	method interface{}) Variable {

	checkCalledFromInit()

	err := validateNinjaName(name)
	if err != nil {
		panic(err)
	}

	methodValue := reflect.ValueOf(method)
	validateVariableMethod(name, methodValue)

	fun := func(config interface{}) (string, error) {
		result := methodValue.Call([]reflect.Value{reflect.ValueOf(config)})
		resultStr := result[0].Interface().(string)
		return resultStr, nil
	}

	v := &variableFunc{p, name, fun}
	err = p.scope.AddVariable(v)
	if err != nil {
		panic(err)
	}

	return v
}

func (v *variableFunc) packageContext() *packageContext {
	return v.pctx
}

func (v *variableFunc) name() string {
	return v.name_
}

func (v *variableFunc) fullName(pkgNames map[*packageContext]string) string {
	return packageNamespacePrefix(pkgNames[v.pctx]) + v.name_
}

func (v *variableFunc) value(config interface{}) (ninjaString, error) {
	value, err := v.value_(config)
	if err != nil {
		return nil, err
	}

	ninjaStr, err := parseNinjaString(v.pctx.scope, value)
	if err != nil {
		err = fmt.Errorf("error parsing variable %s value: %s", v, err)
		panic(err)
	}

	return ninjaStr, nil
}

func (v *variableFunc) String() string {
	return v.pctx.pkgPath + "." + v.name_
}

func validateVariableMethod(name string, methodValue reflect.Value) {
	methodType := methodValue.Type()
	if methodType.Kind() != reflect.Func {
		panic(fmt.Errorf("method given for variable %s is not a function",
			name))
	}
	if n := methodType.NumIn(); n != 1 {
		panic(fmt.Errorf("method for variable %s has %d inputs (should be 1)",
			name, n))
	}
	if n := methodType.NumOut(); n != 1 {
		panic(fmt.Errorf("method for variable %s has %d outputs (should be 1)",
			name, n))
	}
	if kind := methodType.Out(0).Kind(); kind != reflect.String {
		panic(fmt.Errorf("method for variable %s does not return a string",
			name))
	}
}

// An argVariable is a Variable that exists only when it is set by a build
// statement to pass a value to the rule being invoked.  It has no value, so it
// can never be used to create a Ninja assignment statement.  It is inserted
// into the rule's scope, which is used for name lookups within the rule and
// when assigning argument values as part of a build statement.
type argVariable struct {
	name_ string
}

func (v *argVariable) packageContext() *packageContext {
	panic("this should not be called")
}

func (v *argVariable) name() string {
	return v.name_
}

func (v *argVariable) fullName(pkgNames map[*packageContext]string) string {
	return v.name_
}

func (v *argVariable) value(config interface{}) (ninjaString, error) {
	return nil, errVariableIsArg
}

func (v *argVariable) String() string {
	return "<arg>:" + v.name_
}

type staticPool struct {
	pctx   *packageContext
	name_  string
	params PoolParams
}

// StaticPool returns a Pool whose value does not depend on any configuration
// information.  It may only be called during a Go package's initialization -
// either from the init() function or as part of a package-scoped Go variable's
// initialization.
//
// This function is usually used to initialize a package-scoped Go variable that
// represents a Ninja pool that will be output.  The name argument should
// exactly match the Go variable name, and the params fields may reference other
// Ninja variables that are visible within the calling Go package.
func (p *packageContext) StaticPool(name string, params PoolParams) Pool {
	checkCalledFromInit()

	err := validateNinjaName(name)
	if err != nil {
		panic(err)
	}

	pool := &staticPool{p, name, params}
	err = p.scope.AddPool(pool)
	if err != nil {
		panic(err)
	}

	return pool
}

func (p *staticPool) packageContext() *packageContext {
	return p.pctx
}

func (p *staticPool) name() string {
	return p.name_
}

func (p *staticPool) fullName(pkgNames map[*packageContext]string) string {
	return packageNamespacePrefix(pkgNames[p.pctx]) + p.name_
}

func (p *staticPool) def(config interface{}) (*poolDef, error) {
	def, err := parsePoolParams(p.pctx.scope, &p.params)
	if err != nil {
		panic(fmt.Errorf("error parsing PoolParams for %s: %s", p, err))
	}
	return def, nil
}

func (p *staticPool) String() string {
	return p.pctx.pkgPath + "." + p.name_
}

type poolFunc struct {
	pctx       *packageContext
	name_      string
	paramsFunc func(interface{}) (PoolParams, error)
}

// PoolFunc returns a Pool whose value is determined by a function that takes a
// config object as input and returns either the pool parameters or an error. It
// may only be called during a Go package's initialization - either from the
// init() function or as part of a package-scoped variable's initialization.
//
// This function is usually used to initialize a package-scoped Go variable that
// represents a Ninja pool that will be output.  The name argument should
// exactly match the Go variable name, and the string fields of the PoolParams
// returned by f may reference other Ninja variables that are visible within the
// calling Go package.
func (p *packageContext) PoolFunc(name string, f func(interface{}) (PoolParams,
	error)) Pool {

	checkCalledFromInit()

	err := validateNinjaName(name)
	if err != nil {
		panic(err)
	}

	pool := &poolFunc{p, name, f}
	err = p.scope.AddPool(pool)
	if err != nil {
		panic(err)
	}

	return pool
}

func (p *poolFunc) packageContext() *packageContext {
	return p.pctx
}

func (p *poolFunc) name() string {
	return p.name_
}

func (p *poolFunc) fullName(pkgNames map[*packageContext]string) string {
	return packageNamespacePrefix(pkgNames[p.pctx]) + p.name_
}

func (p *poolFunc) def(config interface{}) (*poolDef, error) {
	params, err := p.paramsFunc(config)
	if err != nil {
		return nil, err
	}
	def, err := parsePoolParams(p.pctx.scope, &params)
	if err != nil {
		panic(fmt.Errorf("error parsing PoolParams for %s: %s", p, err))
	}
	return def, nil
}

func (p *poolFunc) String() string {
	return p.pctx.pkgPath + "." + p.name_
}

type builtinPool struct {
	name_ string
}

func (p *builtinPool) packageContext() *packageContext {
	return nil
}

func (p *builtinPool) name() string {
	return p.name_
}

func (p *builtinPool) fullName(pkgNames map[*packageContext]string) string {
	return p.name_
}

func (p *builtinPool) def(config interface{}) (*poolDef, error) {
	return nil, errPoolIsBuiltin
}

// NewBuiltinPool returns a Pool object that refers to a pool name created outside of Blueprint
func NewBuiltinPool(name string) Pool {
	return &builtinPool{
		name_: name,
	}
}

func (p *builtinPool) String() string {
	return "<builtin>:" + p.name_
}

type staticRule struct {
	pctx       *packageContext
	name_      string
	params     RuleParams
	argNames   map[string]bool
	scope_     *basicScope
	sync.Mutex // protects scope_ during lazy creation
}

// StaticRule returns a Rule whose value does not depend on any configuration
// information.  It may only be called during a Go package's initialization -
// either from the init() function or as part of a package-scoped Go variable's
// initialization.
//
// This function is usually used to initialize a package-scoped Go variable that
// represents a Ninja rule that will be output.  The name argument should
// exactly match the Go variable name, and the params fields may reference other
// Ninja variables that are visible within the calling Go package.
//
// The argNames arguments list Ninja variables that may be overridden by Ninja
// build statements that invoke the rule.  These arguments may be referenced in
// any of the string fields of params.  Arguments can shadow package-scoped
// variables defined within the caller's Go package, but they may not shadow
// those defined in another package.  Shadowing a package-scoped variable
// results in the package-scoped variable's value being used for build
// statements that do not override the argument.  For argument names that do not
// shadow package-scoped variables the default value is an empty string.
func (p *packageContext) StaticRule(name string, params RuleParams,
	argNames ...string) Rule {

	checkCalledFromInit()

	err := validateNinjaName(name)
	if err != nil {
		panic(err)
	}

	err = validateArgNames(argNames)
	if err != nil {
		panic(fmt.Errorf("invalid argument name: %s", err))
	}

	argNamesSet := make(map[string]bool)
	for _, argName := range argNames {
		argNamesSet[argName] = true
	}

	ruleScope := (*basicScope)(nil) // This will get created lazily

	r := &staticRule{
		pctx:     p,
		name_:    name,
		params:   params,
		argNames: argNamesSet,
		scope_:   ruleScope,
	}
	err = p.scope.AddRule(r)
	if err != nil {
		panic(err)
	}

	return r
}

func (r *staticRule) packageContext() *packageContext {
	return r.pctx
}

func (r *staticRule) name() string {
	return r.name_
}

func (r *staticRule) fullName(pkgNames map[*packageContext]string) string {
	return packageNamespacePrefix(pkgNames[r.pctx]) + r.name_
}

func (r *staticRule) def(interface{}) (*ruleDef, error) {
	def, err := parseRuleParams(r.scope(), &r.params)
	if err != nil {
		panic(fmt.Errorf("error parsing RuleParams for %s: %s", r, err))
	}
	return def, nil
}

func (r *staticRule) scope() *basicScope {
	// We lazily create the scope so that all the package-scoped variables get
	// declared before the args are created.  Otherwise we could incorrectly
	// shadow a package-scoped variable with an arg variable.
	r.Lock()
	defer r.Unlock()

	if r.scope_ == nil {
		r.scope_ = makeRuleScope(r.pctx.scope, r.argNames)
	}
	return r.scope_
}

func (r *staticRule) isArg(argName string) bool {
	return r.argNames[argName]
}

func (r *staticRule) String() string {
	return r.pctx.pkgPath + "." + r.name_
}

type ruleFunc struct {
	pctx       *packageContext
	name_      string
	paramsFunc func(interface{}) (RuleParams, error)
	argNames   map[string]bool
	scope_     *basicScope
	sync.Mutex // protects scope_ during lazy creation
}

// RuleFunc returns a Rule whose value is determined by a function that takes a
// config object as input and returns either the rule parameters or an error. It
// may only be called during a Go package's initialization - either from the
// init() function or as part of a package-scoped variable's initialization.
//
// This function is usually used to initialize a package-scoped Go variable that
// represents a Ninja rule that will be output.  The name argument should
// exactly match the Go variable name, and the string fields of the RuleParams
// returned by f may reference other Ninja variables that are visible within the
// calling Go package.
//
// The argNames arguments list Ninja variables that may be overridden by Ninja
// build statements that invoke the rule.  These arguments may be referenced in
// any of the string fields of the RuleParams returned by f.  Arguments can
// shadow package-scoped variables defined within the caller's Go package, but
// they may not shadow those defined in another package.  Shadowing a package-
// scoped variable results in the package-scoped variable's value being used for
// build statements that do not override the argument.  For argument names that
// do not shadow package-scoped variables the default value is an empty string.
func (p *packageContext) RuleFunc(name string, f func(interface{}) (RuleParams,
	error), argNames ...string) Rule {

	checkCalledFromInit()

	err := validateNinjaName(name)
	if err != nil {
		panic(err)
	}

	err = validateArgNames(argNames)
	if err != nil {
		panic(fmt.Errorf("invalid argument name: %s", err))
	}

	argNamesSet := make(map[string]bool)
	for _, argName := range argNames {
		argNamesSet[argName] = true
	}

	ruleScope := (*basicScope)(nil) // This will get created lazily

	rule := &ruleFunc{
		pctx:       p,
		name_:      name,
		paramsFunc: f,
		argNames:   argNamesSet,
		scope_:     ruleScope,
	}
	err = p.scope.AddRule(rule)
	if err != nil {
		panic(err)
	}

	return rule
}

func (r *ruleFunc) packageContext() *packageContext {
	return r.pctx
}

func (r *ruleFunc) name() string {
	return r.name_
}

func (r *ruleFunc) fullName(pkgNames map[*packageContext]string) string {
	return packageNamespacePrefix(pkgNames[r.pctx]) + r.name_
}

func (r *ruleFunc) def(config interface{}) (*ruleDef, error) {
	params, err := r.paramsFunc(config)
	if err != nil {
		return nil, err
	}
	def, err := parseRuleParams(r.scope(), &params)
	if err != nil {
		panic(fmt.Errorf("error parsing RuleParams for %s: %s", r, err))
	}
	return def, nil
}

func (r *ruleFunc) scope() *basicScope {
	// We lazily create the scope so that all the global variables get declared
	// before the args are created.  Otherwise we could incorrectly shadow a
	// global variable with an arg variable.
	r.Lock()
	defer r.Unlock()

	if r.scope_ == nil {
		r.scope_ = makeRuleScope(r.pctx.scope, r.argNames)
	}
	return r.scope_
}

func (r *ruleFunc) isArg(argName string) bool {
	return r.argNames[argName]
}

func (r *ruleFunc) String() string {
	return r.pctx.pkgPath + "." + r.name_
}

type builtinRule struct {
	name_      string
	scope_     *basicScope
	sync.Mutex // protects scope_ during lazy creation
}

func (r *builtinRule) packageContext() *packageContext {
	return nil
}

func (r *builtinRule) name() string {
	return r.name_
}

func (r *builtinRule) fullName(pkgNames map[*packageContext]string) string {
	return r.name_
}

func (r *builtinRule) def(config interface{}) (*ruleDef, error) {
	return nil, errRuleIsBuiltin
}

func (r *builtinRule) scope() *basicScope {
	r.Lock()
	defer r.Unlock()

	if r.scope_ == nil {
		r.scope_ = makeRuleScope(nil, nil)
	}
	return r.scope_
}

func (r *builtinRule) isArg(argName string) bool {
	return false
}

func (r *builtinRule) String() string {
	return "<builtin>:" + r.name_
}

// NewBuiltinRule returns a Rule object that refers to a rule that was created outside of Blueprint
func NewBuiltinRule(name string) Rule {
	return &builtinRule{
		name_: name,
	}
}

func (p *packageContext) AddNinjaFileDeps(deps ...string) {
	p.ninjaFileDeps = append(p.ninjaFileDeps, deps...)
}
