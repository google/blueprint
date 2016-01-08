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
)

type Singleton interface {
	GenerateBuildActions(SingletonContext)
}

type SingletonContext interface {
	Config() interface{}

	ModuleName(module Module) string
	ModuleDir(module Module) string
	ModuleSubDir(module Module) string
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

	VisitAllModules(visit func(Module))
	VisitAllModulesIf(pred func(Module) bool, visit func(Module))
	VisitDepsDepthFirst(module Module, visit func(Module))
	VisitDepsDepthFirstIf(module Module, pred func(Module) bool,
		visit func(Module))

	VisitAllModuleVariants(module Module, visit func(Module))

	PrimaryModule(module Module) Module
	FinalModule(module Module) Module

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

func (s *singletonContext) ModuleName(logicModule Module) string {
	return s.context.ModuleName(logicModule)
}

func (s *singletonContext) ModuleDir(logicModule Module) string {
	return s.context.ModuleDir(logicModule)
}

func (s *singletonContext) ModuleSubDir(logicModule Module) string {
	return s.context.ModuleSubDir(logicModule)
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

func (s *singletonContext) VisitAllModules(visit func(Module)) {
	s.context.VisitAllModules(visit)
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
