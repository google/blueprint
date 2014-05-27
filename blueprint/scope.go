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
	pkg() *pkg
	name() string                             // "foo"
	fullName(pkgNames map[*pkg]string) string // "pkg.foo" or "path/to/pkg.foo"
	value(config Config) (*ninjaString, error)
}

// A Pool represents a Ninja pool that will be written to the output .ninja
// file.
type Pool interface {
	pkg() *pkg
	name() string                             // "foo"
	fullName(pkgNames map[*pkg]string) string // "pkg.foo" or "path/to/pkg.foo"
	def(config Config) (*poolDef, error)
}

// A Rule represents a Ninja build rule that will be written to the output
// .ninja file.
type Rule interface {
	pkg() *pkg
	name() string                             // "foo"
	fullName(pkgNames map[*pkg]string) string // "pkg.foo" or "path/to/pkg.foo"
	def(config Config) (*ruleDef, error)
	scope() *scope
	isArg(argName string) bool
}

type scope struct {
	parent    *scope
	variables map[string]Variable
	pools     map[string]Pool
	rules     map[string]Rule
	imports   map[string]*scope
}

func newScope(parent *scope) *scope {
	return &scope{
		parent:    parent,
		variables: make(map[string]Variable),
		pools:     make(map[string]Pool),
		rules:     make(map[string]Rule),
		imports:   make(map[string]*scope),
	}
}

func makeRuleScope(parent *scope, argNames map[string]bool) *scope {
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

func (s *scope) LookupVariable(name string) (Variable, error) {
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

func (s *scope) lookupImportedScope(pkgName string) (*scope, error) {
	for ; s != nil; s = s.parent {
		importedScope, ok := s.imports[pkgName]
		if ok {
			return importedScope, nil
		}
	}
	return nil, fmt.Errorf("unknown imported package %q (missing call to "+
		"blueprint.Import()?)", pkgName)
}

func (s *scope) AddImport(name string, importedScope *scope) error {
	_, present := s.imports[name]
	if present {
		return fmt.Errorf("import %q is already defined in this scope", name)
	}
	s.imports[name] = importedScope
	return nil
}

func (s *scope) AddVariable(v Variable) error {
	name := v.name()
	_, present := s.variables[name]
	if present {
		return fmt.Errorf("variable %q is already defined in this scope", name)
	}
	s.variables[name] = v
	return nil
}

func (s *scope) AddPool(p Pool) error {
	name := p.name()
	_, present := s.pools[name]
	if present {
		return fmt.Errorf("pool %q is already defined in this scope", name)
	}
	s.pools[name] = p
	return nil
}

func (s *scope) AddRule(r Rule) error {
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
	scope      *scope
}

func newLocalScope(parent *scope, namePrefix string) *localScope {
	return &localScope{
		namePrefix: namePrefix,
		scope:      newScope(parent),
	}
}

func (s *localScope) LookupVariable(name string) (Variable, error) {
	return s.scope.LookupVariable(name)
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
		namePrefix: s.namePrefix,
		name_:      name,
		value_:     ninjaValue,
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
		namePrefix: s.namePrefix,
		name_:      name,
		def_:       def,
		argNames:   argNamesSet,
		scope_:     ruleScope,
	}

	err = s.scope.AddRule(r)
	if err != nil {
		return nil, err
	}

	return r, nil
}

type localVariable struct {
	namePrefix string
	name_      string
	value_     *ninjaString
}

func (l *localVariable) pkg() *pkg {
	return nil
}

func (l *localVariable) name() string {
	return l.name_
}

func (l *localVariable) fullName(pkgNames map[*pkg]string) string {
	return l.namePrefix + l.name_
}

func (l *localVariable) value(Config) (*ninjaString, error) {
	return l.value_, nil
}

type localRule struct {
	namePrefix string
	name_      string
	def_       *ruleDef
	argNames   map[string]bool
	scope_     *scope
}

func (l *localRule) pkg() *pkg {
	return nil
}

func (l *localRule) name() string {
	return l.name_
}

func (l *localRule) fullName(pkgNames map[*pkg]string) string {
	return l.namePrefix + l.name_
}

func (l *localRule) def(Config) (*ruleDef, error) {
	return l.def_, nil
}

func (r *localRule) scope() *scope {
	return r.scope_
}

func (r *localRule) isArg(argName string) bool {
	return r.argNames[argName]
}
