package blueprint

import (
	"fmt"
	"strconv"
	"strings"
)

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

type PoolParams struct {
	Comment string
	Depth   int
}

type RuleParams struct {
	Comment        string
	Command        string
	Depfile        string
	Deps           Deps
	Description    string
	Generator      bool
	Pool           Pool
	Restat         bool
	Rspfile        string
	RspfileContent string
}

type BuildParams struct {
	Rule      Rule
	Outputs   []string
	Inputs    []string
	Implicits []string
	OrderOnly []string
	Args      map[string]string
}

// A poolDef describes a pool definition.  It does not include the name of the
// pool.
type poolDef struct {
	Comment string
	Depth   int
}

func parsePoolParams(scope variableLookup, params *PoolParams) (*poolDef,
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
	Comment   string
	Pool      Pool
	Variables map[string]*ninjaString
}

func parseRuleParams(scope variableLookup, params *RuleParams) (*ruleDef,
	error) {

	r := &ruleDef{
		Comment:   params.Comment,
		Pool:      params.Pool,
		Variables: make(map[string]*ninjaString),
	}

	if params.Command == "" {
		return nil, fmt.Errorf("encountered rule params with no command " +
			"specified")
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

	return r, nil
}

func (r *ruleDef) WriteTo(nw *ninjaWriter, name string,
	pkgNames map[*pkg]string) error {

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

	for name, value := range r.Variables {
		err = nw.ScopedAssign(name, value.Value(pkgNames))
		if err != nil {
			return err
		}
	}

	return nil
}

// A buildDef describes a build target definition.
type buildDef struct {
	Rule      Rule
	Outputs   []*ninjaString
	Inputs    []*ninjaString
	Implicits []*ninjaString
	OrderOnly []*ninjaString
	Args      map[Variable]*ninjaString
}

func parseBuildParams(scope variableLookup, params *BuildParams) (*buildDef,
	error) {

	rule := params.Rule

	b := &buildDef{
		Rule: rule,
	}

	var err error
	b.Outputs, err = parseNinjaStrings(scope, params.Outputs)
	if err != nil {
		return nil, fmt.Errorf("error parsing Outputs param: %s", err)
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

	argNameScope := rule.scope()

	if len(params.Args) > 0 {
		b.Args = make(map[Variable]*ninjaString)
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

func (b *buildDef) WriteTo(nw *ninjaWriter, pkgNames map[*pkg]string) error {
	var (
		rule          = b.Rule.fullName(pkgNames)
		outputs       = valueList(b.Outputs, pkgNames, outputEscaper)
		explicitDeps  = valueList(b.Inputs, pkgNames, inputEscaper)
		implicitDeps  = valueList(b.Implicits, pkgNames, inputEscaper)
		orderOnlyDeps = valueList(b.OrderOnly, pkgNames, inputEscaper)
	)
	err := nw.Build(rule, outputs, explicitDeps, implicitDeps, orderOnlyDeps)
	if err != nil {
		return err
	}

	for argVar, value := range b.Args {
		name := argVar.fullName(pkgNames)
		err = nw.ScopedAssign(name, value.Value(pkgNames))
		if err != nil {
			return err
		}
	}

	return nil
}

func valueList(list []*ninjaString, pkgNames map[*pkg]string,
	escaper *strings.Replacer) []string {

	result := make([]string, len(list))
	for i, ninjaStr := range list {
		result[i] = ninjaStr.ValueWithEscaper(pkgNames, escaper)
	}
	return result
}
