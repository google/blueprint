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
	Config() interface{}

	Name() string

	ModuleName(module Module) string
	ModuleDir(module Module) string
	ModuleSubDir(module Module) string
	ModuleType(module Module) string
	BlueprintFile(module Module) string

	ModuleErrorf(module Module, format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Failed() bool

	Variable(pctx PackageContext, name, value string)
	Rule(pctx PackageContext, name string, params RuleParams, argNames ...string) Rule
	Build(pctx PackageContext, params BuildParams)
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

	VisitAllModules(visit func(Module))
	VisitAllModulesIf(pred func(Module) bool, visit func(Module))
	VisitDepsDepthFirst(module Module, visit func(Module))
	VisitDepsDepthFirstIf(module Module, pred func(Module) bool,
		visit func(Module))

	VisitAllModuleVariants(module Module, visit func(Module))

	PrimaryModule(module Module) Module
	FinalModule(module Module) Module

	AddNinjaFileDeps(deps ...string)

	// GlobWithDeps returns a list of files and directories that match the
	// specified pattern but do not match any of the patterns in excludes.
	// Any directories will have a '/' suffix. It also adds efficient
	// dependencies to rerun the primary builder whenever a file matching
	// the pattern as added or removed, without rerunning if a file that
	// does not match the pattern is added to a searched directory.
	GlobWithDeps(pattern string, excludes []string) ([]string, error)

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
				funcName(visit), visitingModule))
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
