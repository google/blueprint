package blueprint

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"strings"
)

type pkg struct {
	fullName  string
	shortName string
	pkgPath   string
	scope     *scope
}

var pkgs = map[string]*pkg{}

var pkgRegexp = regexp.MustCompile(`(.*)\.init(·[0-9]+)?`)

var Phony Rule = &builtinRule{
	name_: "phony",
}

var errRuleIsBuiltin = errors.New("the rule is a built-in")
var errVariableIsArg = errors.New("argument variables have no value")

// We make a Ninja-friendly name out of a Go package name by replaceing all the
// '/' characters with '.'.  We assume the results are unique, though this is
// not 100% guaranteed for Go package names that already contain '.' characters.
// Disallowing package names with '.' isn't reasonable since many package names
// contain the name of the hosting site (e.g. "code.google.com").  In practice
// this probably isn't really a problem.
func pkgPathToName(pkgPath string) string {
	return strings.Replace(pkgPath, "/", ".", -1)
}

// callerPackage returns the pkg of the function that called the caller of
// callerPackage.  The caller of callerPackage must have been called from an
// init function of the package or callerPackage will panic.
//
// Looking for the package's init function on the call stack and using that to
// determine its package name is unfortunately dependent upon Go runtime
// implementation details.  However, it allows us to ensure that it's easy to
// determine where a definition in a .ninja file came from.
func callerPackage() *pkg {
	var pc [1]uintptr
	n := runtime.Callers(3, pc[:])
	if n != 1 {
		panic("unable to get caller pc")
	}

	f := runtime.FuncForPC(pc[0])
	callerName := f.Name()

	submatches := pkgRegexp.FindSubmatch([]byte(callerName))
	if submatches == nil {
		println(callerName)
		panic("not called from an init func")
	}

	pkgPath := string(submatches[1])

	pkgName := pkgPathToName(pkgPath)
	err := validateNinjaName(pkgName)
	if err != nil {
		panic(err)
	}

	i := strings.LastIndex(pkgPath, "/")
	shortName := pkgPath[i+1:]

	p, ok := pkgs[pkgPath]
	if !ok {
		p = &pkg{
			fullName:  pkgName,
			shortName: shortName,
			pkgPath:   pkgPath,
			scope:     newScope(nil),
		}
		pkgs[pkgPath] = p
	}

	return p
}

// Import enables access to the exported Ninja pools, rules, and variables that
// are defined at the package scope of another Go package.  Go's visibility
// rules apply to these references - capitalized names indicate that something
// is exported.  It may only be called from a Go package's init() function.  The
// Go package path passed to Import must have already been imported into the Go
// package using a Go import statement.  The imported variables may then be
// accessed from Ninja strings as "${pkg.Variable}", while the imported rules
// can simply be accessed as exported Go variables from the package.  For
// example:
//
//     import (
//         "blueprint"
//         "foo/bar"
//     )
//
//     func init() {
//         blueprint.Import("foo/bar")
//     }
//
//     ...
//
//     func (m *MyModule) GenerateBuildActions(ctx blueprint.Module) {
//         ctx.Build(blueprint.BuildParams{
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
func Import(pkgPath string) {
	callerPkg := callerPackage()

	importPkg, ok := pkgs[pkgPath]
	if !ok {
		panic(fmt.Errorf("package %q has no Blueprints definitions", pkgPath))
	}

	err := callerPkg.scope.AddImport(importPkg.shortName, importPkg.scope)
	if err != nil {
		panic(err)
	}
}

// ImportAs provides the same functionality as Import, but it allows the local
// name that will be used to refer to the package to be specified explicitly.
// It may only be called from a Go package's init() function.
func ImportAs(as, pkgPath string) {
	callerPkg := callerPackage()

	importPkg, ok := pkgs[pkgPath]
	if !ok {
		panic(fmt.Errorf("package %q has no Blueprints definitions", pkgPath))
	}

	err := validateNinjaName(as)
	if err != nil {
		panic(err)
	}

	err = callerPkg.scope.AddImport(as, importPkg.scope)
	if err != nil {
		panic(err)
	}
}

type staticVariable struct {
	pkg_   *pkg
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
func StaticVariable(name, value string) Variable {
	err := validateNinjaName(name)
	if err != nil {
		panic(err)
	}

	pkg := callerPackage()

	v := &staticVariable{pkg, name, value}
	err = pkg.scope.AddVariable(v)
	if err != nil {
		panic(err)
	}

	return v
}

func (v *staticVariable) pkg() *pkg {
	return v.pkg_
}

func (v *staticVariable) name() string {
	return v.name_
}

func (v *staticVariable) fullName(pkgNames map[*pkg]string) string {
	return packageNamespacePrefix(pkgNames[v.pkg_]) + v.name_
}

func (v *staticVariable) value(interface{}) (*ninjaString, error) {
	ninjaStr, err := parseNinjaString(v.pkg_.scope, v.value_)
	if err != nil {
		err = fmt.Errorf("error parsing variable %s.%s value: %s",
			v.pkg_.pkgPath, v.name_, err)
		panic(err)
	}
	return ninjaStr, nil
}

type variableFunc struct {
	pkg_   *pkg
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
func VariableFunc(name string, f func(config interface{}) (string,
	error)) Variable {

	err := validateNinjaName(name)
	if err != nil {
		panic(err)
	}

	pkg := callerPackage()

	v := &variableFunc{pkg, name, f}
	err = pkg.scope.AddVariable(v)
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
func VariableConfigMethod(name string, method interface{}) Variable {
	err := validateNinjaName(name)
	if err != nil {
		panic(err)
	}

	pkg := callerPackage()

	methodValue := reflect.ValueOf(method)
	validateVariableMethod(name, methodValue)

	fun := func(config interface{}) (string, error) {
		result := methodValue.Call([]reflect.Value{reflect.ValueOf(config)})
		resultStr := result[0].Interface().(string)
		return resultStr, nil
	}

	v := &variableFunc{pkg, name, fun}
	err = pkg.scope.AddVariable(v)
	if err != nil {
		panic(err)
	}

	return v
}

func (v *variableFunc) pkg() *pkg {
	return v.pkg_
}

func (v *variableFunc) name() string {
	return v.name_
}

func (v *variableFunc) fullName(pkgNames map[*pkg]string) string {
	return packageNamespacePrefix(pkgNames[v.pkg_]) + v.name_
}

func (v *variableFunc) value(config interface{}) (*ninjaString, error) {
	value, err := v.value_(config)
	if err != nil {
		return nil, err
	}

	ninjaStr, err := parseNinjaString(v.pkg_.scope, value)
	if err != nil {
		err = fmt.Errorf("error parsing variable %s.%s value: %s",
			v.pkg_.pkgPath, v.name_, err)
		panic(err)
	}

	return ninjaStr, nil
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

func (v *argVariable) pkg() *pkg {
	panic("this should not be called")
}

func (v *argVariable) name() string {
	return v.name_
}

func (v *argVariable) fullName(pkgNames map[*pkg]string) string {
	return v.name_
}

func (v *argVariable) value(config interface{}) (*ninjaString, error) {
	return nil, errVariableIsArg
}

type staticPool struct {
	pkg_   *pkg
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
func StaticPool(name string, params PoolParams) Pool {
	err := validateNinjaName(name)
	if err != nil {
		panic(err)
	}

	pkg := callerPackage()

	p := &staticPool{pkg, name, params}
	err = pkg.scope.AddPool(p)
	if err != nil {
		panic(err)
	}

	return p
}

func (p *staticPool) pkg() *pkg {
	return p.pkg_
}

func (p *staticPool) name() string {
	return p.name_
}

func (p *staticPool) fullName(pkgNames map[*pkg]string) string {
	return packageNamespacePrefix(pkgNames[p.pkg_]) + p.name_
}

func (p *staticPool) def(config interface{}) (*poolDef, error) {
	def, err := parsePoolParams(p.pkg_.scope, &p.params)
	if err != nil {
		panic(fmt.Errorf("error parsing PoolParams for %s.%s: %s",
			p.pkg_.pkgPath, p.name_, err))
	}
	return def, nil
}

type poolFunc struct {
	pkg_       *pkg
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
func PoolFunc(name string, f func(interface{}) (PoolParams, error)) Pool {
	err := validateNinjaName(name)
	if err != nil {
		panic(err)
	}

	pkg := callerPackage()

	p := &poolFunc{pkg, name, f}
	err = pkg.scope.AddPool(p)
	if err != nil {
		panic(err)
	}

	return p
}

func (p *poolFunc) pkg() *pkg {
	return p.pkg_
}

func (p *poolFunc) name() string {
	return p.name_
}

func (p *poolFunc) fullName(pkgNames map[*pkg]string) string {
	return packageNamespacePrefix(pkgNames[p.pkg_]) + p.name_
}

func (p *poolFunc) def(config interface{}) (*poolDef, error) {
	params, err := p.paramsFunc(config)
	if err != nil {
		return nil, err
	}
	def, err := parsePoolParams(p.pkg_.scope, &params)
	if err != nil {
		panic(fmt.Errorf("error parsing PoolParams for %s.%s: %s",
			p.pkg_.pkgPath, p.name_, err))
	}
	return def, nil
}

type staticRule struct {
	pkg_     *pkg
	name_    string
	params   RuleParams
	argNames map[string]bool
	scope_   *scope
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
func StaticRule(name string, params RuleParams, argNames ...string) Rule {
	pkg := callerPackage()

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

	ruleScope := (*scope)(nil) // This will get created lazily

	r := &staticRule{pkg, name, params, argNamesSet, ruleScope}
	err = pkg.scope.AddRule(r)
	if err != nil {
		panic(err)
	}

	return r
}

func (r *staticRule) pkg() *pkg {
	return r.pkg_
}

func (r *staticRule) name() string {
	return r.name_
}

func (r *staticRule) fullName(pkgNames map[*pkg]string) string {
	return packageNamespacePrefix(pkgNames[r.pkg_]) + r.name_
}

func (r *staticRule) def(interface{}) (*ruleDef, error) {
	def, err := parseRuleParams(r.scope(), &r.params)
	if err != nil {
		panic(fmt.Errorf("error parsing RuleParams for %s.%s: %s",
			r.pkg_.pkgPath, r.name_, err))
	}
	return def, nil
}

func (r *staticRule) scope() *scope {
	// We lazily create the scope so that all the global variables get declared
	// before the args are created.  Otherwise we could incorrectly shadow a
	// global variable with an arg variable.
	if r.scope_ == nil {
		r.scope_ = makeRuleScope(r.pkg_.scope, r.argNames)
	}
	return r.scope_
}

func (r *staticRule) isArg(argName string) bool {
	return r.argNames[argName]
}

type ruleFunc struct {
	pkg_       *pkg
	name_      string
	paramsFunc func(interface{}) (RuleParams, error)
	argNames   map[string]bool
	scope_     *scope
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
func RuleFunc(name string, f func(interface{}) (RuleParams, error),
	argNames ...string) Rule {

	pkg := callerPackage()

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

	ruleScope := (*scope)(nil) // This will get created lazily

	r := &ruleFunc{pkg, name, f, argNamesSet, ruleScope}
	err = pkg.scope.AddRule(r)
	if err != nil {
		panic(err)
	}

	return r
}

func (r *ruleFunc) pkg() *pkg {
	return r.pkg_
}

func (r *ruleFunc) name() string {
	return r.name_
}

func (r *ruleFunc) fullName(pkgNames map[*pkg]string) string {
	return packageNamespacePrefix(pkgNames[r.pkg_]) + r.name_
}

func (r *ruleFunc) def(config interface{}) (*ruleDef, error) {
	params, err := r.paramsFunc(config)
	if err != nil {
		return nil, err
	}
	def, err := parseRuleParams(r.scope(), &params)
	if err != nil {
		panic(fmt.Errorf("error parsing RuleParams for %s.%s: %s",
			r.pkg_.pkgPath, r.name_, err))
	}
	return def, nil
}

func (r *ruleFunc) scope() *scope {
	// We lazily create the scope so that all the global variables get declared
	// before the args are created.  Otherwise we could incorrectly shadow a
	// global variable with an arg variable.
	if r.scope_ == nil {
		r.scope_ = makeRuleScope(r.pkg_.scope, r.argNames)
	}
	return r.scope_
}

func (r *ruleFunc) isArg(argName string) bool {
	return r.argNames[argName]
}

type builtinRule struct {
	name_  string
	scope_ *scope
}

func (r *builtinRule) pkg() *pkg {
	return nil
}

func (r *builtinRule) name() string {
	return r.name_
}

func (r *builtinRule) fullName(pkgNames map[*pkg]string) string {
	return r.name_
}

func (r *builtinRule) def(config interface{}) (*ruleDef, error) {
	return nil, errRuleIsBuiltin
}

func (r *builtinRule) scope() *scope {
	if r.scope_ == nil {
		r.scope_ = makeRuleScope(nil, nil)
	}
	return r.scope_
}

func (r *builtinRule) isArg(argName string) bool {
	return false
}

// A ModuleType represents a type of module that can be defined in a Blueprints
// file.  In order for it to be used when interpreting Blueprints files, a
// ModuleType must first be registered with a Context object via the
// Context.RegisterModuleType method.
type ModuleType interface {
	pkg() *pkg
	name() string
	new() (m Module, properties interface{})
}

type moduleTypeFunc struct {
	pkg_  *pkg
	name_ string
	new_  func() (Module, interface{})
}

// MakeModuleType returns a new ModuleType object that will instantiate new
// Module objects with the given new function.  MakeModuleType may only be
// called during a Go package's initialization - either from the init() function
// or as part of a package-scoped variable's initialization.
//
// This function is usually used to initialize a package-scoped Go ModuleType
// variable that can then be passed to Context.RegisterModuleType.  The name
// argument should exactly match the Go variable name.  Note that this name is
// different than the one passed to Context.RegisterModuleType.  This name is
// used to identify the Go object in error messages, making it easier to
// identify problematic build logic code.  The name passed to
// Context.RegisterModuleType is the name that appear in Blueprints files to
// instantiate modules of this type.
//
// The new function passed to MakeModuleType returns two values.  The first is
// the newly created Module object.  The second is a pointer to that Module
// object's properties struct.  This properties struct is examined when parsing
// a module definition of this type in a Blueprints file.  Exported fields of
// the properties struct are automatically set to the property values specified
// in the Blueprints file.  The properties struct field names determine the name
// of the Blueprints file properties that are used - the Blueprints property
// name matches that of the properties struct field name with the first letter
// converted to lower-case.
//
// The fields of the properties struct must either []string, a string, or bool.
// The Context will panic if a Module gets instantiated with a properties struct
// containing a field that is not one these supported types.
//
// Any properties that appear in the Blueprints files that are not built-in
// module properties (such as "name" and "deps") and do not have a corresponding
// field in the returned module properties struct result in an error during the
// Context's parse phase.
//
// As an example, the follow code:
//
//   var MyModuleType = blueprint.MakeModuleType("MyModuleType", newMyModule)
//
//   type myModule struct {
//       properties struct {
//           Foo string
//           Bar []string
//       }
//   }
//
//   func newMyModule() (blueprint.Module, interface{}) {
//       module := new(myModule)
//       properties := &module.properties
//       return module, properties
//   }
//
//   func main() {
//       ctx := blueprint.NewContext()
//       ctx.RegisterModuleType("my_module", MyModuleType)
//       // ...
//   }
//
// would support parsing a module defined in a Blueprints file as follows:
//
//   my_module {
//       name: "myName",
//       foo:  "my foo string",
//       bar:  ["my", "bar", "strings"],
//   }
//
func MakeModuleType(name string,
	new func() (m Module, properties interface{})) ModuleType {

	pkg := callerPackage()
	return &moduleTypeFunc{pkg, name, new}
}

func (m *moduleTypeFunc) pkg() *pkg {
	return m.pkg_
}

func (m *moduleTypeFunc) name() string {
	return m.pkg_.pkgPath + "." + m.name_
}

func (m *moduleTypeFunc) new() (Module, interface{}) {
	return m.new_()
}
