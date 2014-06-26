package blueprint

import (
	"fmt"
	"path/filepath"
)

type Singleton interface {
	GenerateBuildActions(SingletonContext)
}

type SingletonContext interface {
	Config() interface{}

	ModuleName(module Module) string
	ModuleDir(module Module) string
	BlueprintFile(module Module) string

	ModuleErrorf(module Module, format string, args ...interface{})
	Errorf(format string, args ...interface{})

	Variable(name, value string)
	Rule(name string, params RuleParams, argNames ...string) Rule
	Build(params BuildParams)
	RequireNinjaVersion(major, minor, micro int)

	// SetBuildDir sets the value of the top-level "builddir" Ninja variable
	// that controls where Ninja stores its build log files.  This value can be
	// set at most one time for a single build.  Setting it multiple times (even
	// across different singletons) will result in a panic.
	SetBuildDir(value string)

	VisitAllModules(visit func(Module))
	VisitAllModulesIf(pred func(Module) bool, visit func(Module))
	VisitDepsDepthFirst(module Module, visit func(Module))
	VisitDepsDepthFirstIf(module Module, pred func(Module) bool,
		visit func(Module))

	AddNinjaFileDeps(deps ...string)
}

var _ SingletonContext = (*singletonContext)(nil)

type singletonContext struct {
	context *Context
	config  interface{}
	scope   *localScope

	ninjaFileDeps []string
	errs          []error

	actionDefs localBuildActions
}

func (s *singletonContext) Config() interface{} {
	return s.config
}

func (s *singletonContext) ModuleName(module Module) string {
	info := s.context.moduleInfo[module]
	return info.properties.Name
}

func (s *singletonContext) ModuleDir(module Module) string {
	info := s.context.moduleInfo[module]
	return filepath.Dir(info.relBlueprintsFile)
}

func (s *singletonContext) BlueprintFile(module Module) string {
	info := s.context.moduleInfo[module]
	return info.relBlueprintsFile
}

func (s *singletonContext) ModuleErrorf(module Module, format string,
	args ...interface{}) {

	info := s.context.moduleInfo[module]
	s.errs = append(s.errs, &Error{
		Err: fmt.Errorf(format, args...),
		Pos: info.pos,
	})
}

func (s *singletonContext) Errorf(format string, args ...interface{}) {
	// TODO: Make this not result in the error being printed as "internal error"
	s.errs = append(s.errs, fmt.Errorf(format, args...))
}

func (s *singletonContext) Variable(name, value string) {
	v, err := s.scope.AddLocalVariable(name, value)
	if err != nil {
		panic(err)
	}

	s.actionDefs.variables = append(s.actionDefs.variables, v)
}

func (s *singletonContext) Rule(name string, params RuleParams,
	argNames ...string) Rule {

	r, err := s.scope.AddLocalRule(name, &params, argNames...)
	if err != nil {
		panic(err)
	}

	s.actionDefs.rules = append(s.actionDefs.rules, r)

	return r
}

func (s *singletonContext) Build(params BuildParams) {
	def, err := parseBuildParams(s.scope, &params)
	if err != nil {
		panic(err)
	}

	s.actionDefs.buildDefs = append(s.actionDefs.buildDefs, def)
}

func (s *singletonContext) RequireNinjaVersion(major, minor, micro int) {
	s.context.requireNinjaVersion(major, minor, micro)
}

func (s *singletonContext) SetBuildDir(value string) {
	ninjaValue, err := parseNinjaString(s.scope, value)
	if err != nil {
		panic(err)
	}

	s.context.setBuildDir(ninjaValue)
}

func (s *singletonContext) VisitAllModules(visit func(Module)) {
	s.context.visitAllModules(visit)
}

func (s *singletonContext) VisitAllModulesIf(pred func(Module) bool,
	visit func(Module)) {

	s.context.visitAllModulesIf(pred, visit)
}

func (s *singletonContext) VisitDepsDepthFirst(module Module,
	visit func(Module)) {

	s.context.visitDepsDepthFirst(module, visit)
}

func (s *singletonContext) VisitDepsDepthFirstIf(module Module,
	pred func(Module) bool, visit func(Module)) {

	s.context.visitDepsDepthFirstIf(module, pred, visit)
}

func (s *singletonContext) AddNinjaFileDeps(deps ...string) {
	s.ninjaFileDeps = append(s.ninjaFileDeps, deps...)
}
