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

	"github.com/google/blueprint/pathtools"
)

type Singleton interface {
	GenerateBuildActions(SingletonContext)
}

type SingletonContext interface {
	// Config returns the config object that was passed to Context.PrepareBuildActions.
	Config() interface{}

	// Name returns the name of the current singleton passed to Context.RegisterSingletonType
	Name() string

	// ModuleName returns the name of the given Module.  See BaseModuleContext.ModuleName for more information.
	ModuleName(module Module) string

	// ModuleDir returns the directory of the given Module.  See BaseModuleContext.ModuleDir for more information.
	ModuleDir(module Module) string

	// ModuleSubDir returns the unique subdirectory name of the given Module.  See ModuleContext.ModuleSubDir for
	// more information.
	ModuleSubDir(module Module) string

	// ModuleType returns the type of the given Module.  See BaseModuleContext.ModuleType for more information.
	ModuleType(module Module) string

	// BlueprintFile returns the path of the Blueprint file that defined the given module.
	BlueprintFile(module Module) string

	// ModuleErrorf reports an error at the line number of the module type in the module definition.
	ModuleErrorf(module Module, format string, args ...interface{})

	// Errorf reports an error at the specified position of the module definition file.
	Errorf(format string, args ...interface{})

	// Failed returns true if any errors have been reported.  In most cases the singleton can continue with generating
	// build rules after an error, allowing it to report additional errors in a single run, but in cases where the error
	// has prevented the singleton from creating necessary data it can return early when Failed returns true.
	Failed() bool

	// Variable creates a new ninja variable scoped to the singleton.  It can be referenced by calls to Rule and Build
	// in the same singleton.
	Variable(pctx PackageContext, name, value string)

	// Rule creates a new ninja rule scoped to the singleton.  It can be referenced by calls to Build in the same
	// singleton.
	Rule(pctx PackageContext, name string, params RuleParams, argNames ...string) Rule

	// Build creates a new ninja build statement.
	Build(pctx PackageContext, params BuildParams)

	// RequireNinjaVersion sets the generated ninja manifest to require at least the specified version of ninja.
	RequireNinjaVersion(major, minor, micro int)

	// SetNinjaBuildDir sets the value of the top-level "builddir" Ninja variable
	// that controls where Ninja stores its build log files.  This value can be
	// set at most one time for a single build, later calls are ignored.
	SetNinjaBuildDir(pctx PackageContext, value string)

	// AddSubninja adds a ninja file to include with subninja. This should likely
	// only ever be used inside bootstrap to handle glob rules.
	AddSubninja(file string)

	// Eval takes a string with embedded ninja variables, and returns a string
	// with all of the variables recursively expanded. Any variables references
	// are expanded in the scope of the PackageContext.
	Eval(pctx PackageContext, ninjaStr string) (string, error)

	// VisitAllModules calls visit for each defined variant of each module in an unspecified order.
	VisitAllModules(visit func(Module))

	// VisitAllModules calls pred for each defined variant of each module in an unspecified order, and if pred returns
	// true calls visit.
	VisitAllModulesIf(pred func(Module) bool, visit func(Module))

	// VisitDirectDeps calls visit for each direct dependency of the Module.  If there are
	// multiple direct dependencies on the same module visit will be called multiple times on
	// that module and OtherModuleDependencyTag will return a different tag for each.
	//
	// The Module passed to the visit function should not be retained outside of the visit
	// function, it may be invalidated by future mutators.
	VisitDirectDeps(module Module, visit func(Module))

	// VisitDirectDepsIf calls pred for each direct dependency of the Module, and if pred
	// returns true calls visit.  If there are multiple direct dependencies on the same module
	// pred and visit will be called multiple times on that module and OtherModuleDependencyTag
	// will return a different tag for each.
	//
	// The Module passed to the visit function should not be retained outside of the visit
	// function, it may be invalidated by future mutators.
	VisitDirectDepsIf(module Module, pred func(Module) bool, visit func(Module))

	// VisitDepsDepthFirst calls visit for each transitive dependency, traversing the dependency tree in depth first
	// order. visit will only be called once for any given module, even if there are multiple paths through the
	// dependency tree to the module or multiple direct dependencies with different tags.
	VisitDepsDepthFirst(module Module, visit func(Module))

	// VisitDepsDepthFirst calls pred for each transitive dependency, and if pred returns true calls visit, traversing
	// the dependency tree in depth first order.  visit will only be called once for any given module, even if there are
	// multiple paths through the dependency tree to the module or multiple direct dependencies with different tags.
	VisitDepsDepthFirstIf(module Module, pred func(Module) bool,
		visit func(Module))

	// VisitAllModuleVariants calls visit for each variant of the given module.
	VisitAllModuleVariants(module Module, visit func(Module))

	// PrimaryModule returns the first variant of the given module.  This can be used to perform
	//	// singleton actions that are only done once for all variants of a module.
	PrimaryModule(module Module) Module

	// FinalModule returns the last variant of the given module.  This can be used to perform
	// singleton actions that are only done once for all variants of a module.
	FinalModule(module Module) Module

	// AddNinjaFileDeps adds dependencies on the specified files to the rule that creates the ninja manifest.  The
	// primary builder will be rerun whenever the specified files are modified.
	AddNinjaFileDeps(deps ...string)

	// GlobWithDeps returns a list of files and directories that match the
	// specified pattern but do not match any of the patterns in excludes.
	// Any directories will have a '/' suffix. It also adds efficient
	// dependencies to rerun the primary builder whenever a file matching
	// the pattern as added or removed, without rerunning if a file that
	// does not match the pattern is added to a searched directory.
	GlobWithDeps(pattern string, excludes []string) ([]string, error)

	// Fs returns a pathtools.Filesystem that can be used to interact with files.  Using the Filesystem interface allows
	// the singleton to be used in build system tests that run against a mock filesystem.
	Fs() pathtools.FileSystem
}

var _ SingletonContext = (*singletonContext)(nil)

type singletonContext struct {
	name    string
	context *Context
	config  interface{}
	scope   *localScope
	globals *liveTracker

	ninjaFileDeps []string
	errs          []error

	actionDefs localBuildActions
}

func (s *singletonContext) Config() interface{} {
	return s.config
}

func (s *singletonContext) Name() string {
	return s.name
}

func (s *singletonContext) ModuleName(logicModule Module) string {
	return s.context.ModuleName(logicModule)
}

func (s *singletonContext) ModuleDir(logicModule Module) string {
	return s.context.ModuleDir(logicModule)
}

func (s *singletonContext) ModuleSubDir(logicModule Module) string {
	return s.context.ModuleSubDir(logicModule)
}

func (s *singletonContext) ModuleType(logicModule Module) string {
	return s.context.ModuleType(logicModule)
}

func (s *singletonContext) BlueprintFile(logicModule Module) string {
	return s.context.BlueprintFile(logicModule)
}

func (s *singletonContext) error(err error) {
	if err != nil {
		s.errs = append(s.errs, err)
	}
}

func (s *singletonContext) ModuleErrorf(logicModule Module, format string,
	args ...interface{}) {

	s.error(s.context.ModuleErrorf(logicModule, format, args...))
}

func (s *singletonContext) Errorf(format string, args ...interface{}) {
	// TODO: Make this not result in the error being printed as "internal error"
	s.error(fmt.Errorf(format, args...))
}

func (s *singletonContext) Failed() bool {
	return len(s.errs) > 0
}

func (s *singletonContext) Variable(pctx PackageContext, name, value string) {
	s.scope.ReparentTo(pctx)

	v, err := s.scope.AddLocalVariable(name, value)
	if err != nil {
		panic(err)
	}

	s.actionDefs.variables = append(s.actionDefs.variables, v)
}

func (s *singletonContext) Rule(pctx PackageContext, name string,
	params RuleParams, argNames ...string) Rule {

	s.scope.ReparentTo(pctx)

	r, err := s.scope.AddLocalRule(name, &params, argNames...)
	if err != nil {
		panic(err)
	}

	s.actionDefs.rules = append(s.actionDefs.rules, r)

	return r
}

func (s *singletonContext) Build(pctx PackageContext, params BuildParams) {
	s.scope.ReparentTo(pctx)

	def, err := parseBuildParams(s.scope, &params)
	if err != nil {
		panic(err)
	}

	s.actionDefs.buildDefs = append(s.actionDefs.buildDefs, def)
}

func (s *singletonContext) Eval(pctx PackageContext, str string) (string, error) {
	s.scope.ReparentTo(pctx)

	ninjaStr, err := parseNinjaString(s.scope, str)
	if err != nil {
		return "", err
	}

	err = s.globals.addNinjaStringDeps(ninjaStr)
	if err != nil {
		return "", err
	}

	return ninjaStr.Eval(s.globals.variables)
}

func (s *singletonContext) RequireNinjaVersion(major, minor, micro int) {
	s.context.requireNinjaVersion(major, minor, micro)
}

func (s *singletonContext) SetNinjaBuildDir(pctx PackageContext, value string) {
	s.scope.ReparentTo(pctx)

	ninjaValue, err := parseNinjaString(s.scope, value)
	if err != nil {
		panic(err)
	}

	s.context.setNinjaBuildDir(ninjaValue)
}

func (s *singletonContext) AddSubninja(file string) {
	s.context.subninjas = append(s.context.subninjas, file)
}

func (s *singletonContext) VisitAllModules(visit func(Module)) {
	var visitingModule Module
	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitAllModules(%s) for module %s",
				funcName(visit), s.context.moduleInfo[visitingModule]))
		}
	}()

	s.context.VisitAllModules(func(m Module) {
		visitingModule = m
		visit(m)
	})
}

func (s *singletonContext) VisitAllModulesIf(pred func(Module) bool,
	visit func(Module)) {

	s.context.VisitAllModulesIf(pred, visit)
}

func (s *singletonContext) VisitDirectDeps(module Module, visit func(Module)) {
	s.context.VisitDirectDeps(module, visit)
}

func (s *singletonContext) VisitDirectDepsIf(module Module, pred func(Module) bool, visit func(Module)) {
	s.context.VisitDirectDepsIf(module, pred, visit)
}

func (s *singletonContext) VisitDepsDepthFirst(module Module,
	visit func(Module)) {

	s.context.VisitDepsDepthFirst(module, visit)
}

func (s *singletonContext) VisitDepsDepthFirstIf(module Module,
	pred func(Module) bool, visit func(Module)) {

	s.context.VisitDepsDepthFirstIf(module, pred, visit)
}

func (s *singletonContext) PrimaryModule(module Module) Module {
	return s.context.PrimaryModule(module)
}

func (s *singletonContext) FinalModule(module Module) Module {
	return s.context.FinalModule(module)
}

func (s *singletonContext) VisitAllModuleVariants(module Module, visit func(Module)) {
	s.context.VisitAllModuleVariants(module, visit)
}

func (s *singletonContext) AddNinjaFileDeps(deps ...string) {
	s.ninjaFileDeps = append(s.ninjaFileDeps, deps...)
}

func (s *singletonContext) GlobWithDeps(pattern string,
	excludes []string) ([]string, error) {
	return s.context.glob(pattern, excludes)
}

func (s *singletonContext) Fs() pathtools.FileSystem {
	return s.context.fs
}
