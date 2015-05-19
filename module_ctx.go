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
	"path/filepath"
	"text/scanner"
)

// A Module handles generating all of the Ninja build actions needed to build a
// single module based on properties defined in a Blueprints file.  Module
// objects are initially created during the parse phase of a Context using one
// of the registered module types (and the associated ModuleFactory function).
// The Module's properties struct is automatically filled in with the property
// values specified in the Blueprints file (see Context.RegisterModuleType for more
// information on this).
//
// A Module can be split into multiple Modules by a Mutator.  All existing
// properties set on the module will be duplicated to the new Module, and then
// modified as necessary by the Mutator.
//
// The Module implementation can access the build configuration as well as any
// modules on which on which it depends (as defined by the "deps" property
// specified in the Blueprints file or dynamically added by implementing the
// DynamicDependerModule interface) using the ModuleContext passed to
// GenerateBuildActions.  This ModuleContext is also used to create Ninja build
// actions and to report errors to the user.
//
// In addition to implementing the GenerateBuildActions method, a Module should
// implement methods that provide dependant modules and singletons information
// they need to generate their build actions.  These methods will only be called
// after GenerateBuildActions is called because the Context calls
// GenerateBuildActions in dependency-order (and singletons are invoked after
// all the Modules).  The set of methods a Module supports will determine how
// dependant Modules interact with it.
//
// For example, consider a Module that is responsible for generating a library
// that other modules can link against.  The library Module might implement the
// following interface:
//
//   type LibraryProducer interface {
//       LibraryFileName() string
//   }
//
//   func IsLibraryProducer(module blueprint.Module) {
//       _, ok := module.(LibraryProducer)
//       return ok
//   }
//
// A binary-producing Module that depends on the library Module could then do:
//
//   func (m *myBinaryModule) GenerateBuildActions(ctx blueprint.ModuleContext) {
//       ...
//       var libraryFiles []string
//       ctx.VisitDepsDepthFirstIf(IsLibraryProducer,
//           func(module blueprint.Module) {
//               libProducer := module.(LibraryProducer)
//               libraryFiles = append(libraryFiles, libProducer.LibraryFileName())
//           })
//       ...
//   }
//
// to build the list of library file names that should be included in its link
// command.
//
// GenerateBuildActions may be called from multiple threads.  It is guaranteed to
// be called after it has finished being called on all dependencies and on all
// variants of that appear earlier in the ModuleContext.VisitAllModuleVariants list.
// Any accesses to global variables or to Module objects that are not dependencies
// or variants of the current Module must be synchronized by the implementation of
// GenerateBuildActions.
type Module interface {
	// GenerateBuildActions is called by the Context that created the Module
	// during its generate phase.  This call should generate all Ninja build
	// actions (rules, pools, and build statements) needed to build the module.
	GenerateBuildActions(ModuleContext)
}

// A DynamicDependerModule is a Module that may add dependencies that do not
// appear in its "deps" property.  Any Module that implements this interface
// will have its DynamicDependencies method called by the Context that created
// it during generate phase.
type DynamicDependerModule interface {
	Module

	// DynamicDependencies is called by the Context that created the
	// DynamicDependerModule during its generate phase.  This call should return
	// the list of module names that the DynamicDependerModule depends on
	// dynamically.  Module names that already appear in the "deps" property may
	// but do not need to be included in the returned list.
	DynamicDependencies(DynamicDependerModuleContext) []string
}

type BaseModuleContext interface {
	ModuleName() string
	ModuleDir() string
	Config() interface{}

	ContainsProperty(name string) bool
	Errorf(pos scanner.Position, fmt string, args ...interface{})
	ModuleErrorf(fmt string, args ...interface{})
	PropertyErrorf(property, fmt string, args ...interface{})
	Failed() bool
}

type DynamicDependerModuleContext interface {
	BaseModuleContext

	AddVariationDependencies([]Variation, ...string)
	AddFarVariationDependencies([]Variation, ...string)
}

type ModuleContext interface {
	BaseModuleContext

	OtherModuleName(m Module) string
	OtherModuleErrorf(m Module, fmt string, args ...interface{})

	VisitDirectDeps(visit func(Module))
	VisitDirectDepsIf(pred func(Module) bool, visit func(Module))
	VisitDepsDepthFirst(visit func(Module))
	VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module))

	ModuleSubDir() string

	Variable(pctx *PackageContext, name, value string)
	Rule(pctx *PackageContext, name string, params RuleParams, argNames ...string) Rule
	Build(pctx *PackageContext, params BuildParams)

	AddNinjaFileDeps(deps ...string)

	PrimaryModule() Module
	FinalModule() Module
	VisitAllModuleVariants(visit func(Module))
}

var _ BaseModuleContext = (*baseModuleContext)(nil)

type baseModuleContext struct {
	context *Context
	config  interface{}
	module  *moduleInfo
	errs    []error
}

func (d *baseModuleContext) ModuleName() string {
	return d.module.properties.Name
}

func (d *baseModuleContext) ContainsProperty(name string) bool {
	_, ok := d.module.propertyPos[name]
	return ok
}

func (d *baseModuleContext) ModuleDir() string {
	return filepath.Dir(d.module.relBlueprintsFile)
}

func (d *baseModuleContext) Config() interface{} {
	return d.config
}

func (d *baseModuleContext) Errorf(pos scanner.Position,
	format string, args ...interface{}) {

	d.errs = append(d.errs, &Error{
		Err: fmt.Errorf(format, args...),
		Pos: pos,
	})
}

func (d *baseModuleContext) ModuleErrorf(format string,
	args ...interface{}) {

	d.errs = append(d.errs, &Error{
		Err: fmt.Errorf(format, args...),
		Pos: d.module.pos,
	})
}

func (d *baseModuleContext) PropertyErrorf(property, format string,
	args ...interface{}) {

	pos, ok := d.module.propertyPos[property]
	if !ok {
		panic(fmt.Errorf("property %q was not set for this module", property))
	}

	d.errs = append(d.errs, &Error{
		Err: fmt.Errorf(format, args...),
		Pos: pos,
	})
}

func (d *baseModuleContext) Failed() bool {
	return len(d.errs) > 0
}

var _ ModuleContext = (*moduleContext)(nil)

type moduleContext struct {
	baseModuleContext
	scope         *localScope
	ninjaFileDeps []string
	actionDefs    localBuildActions
}

func (m *moduleContext) OtherModuleName(logicModule Module) string {
	module := m.context.moduleInfo[logicModule]
	return module.properties.Name
}

func (m *moduleContext) OtherModuleErrorf(logicModule Module, format string,
	args ...interface{}) {

	module := m.context.moduleInfo[logicModule]
	m.errs = append(m.errs, &Error{
		Err: fmt.Errorf(format, args...),
		Pos: module.pos,
	})
}

func (m *moduleContext) VisitDirectDeps(visit func(Module)) {
	m.context.visitDirectDeps(m.module, visit)
}

func (m *moduleContext) VisitDirectDepsIf(pred func(Module) bool, visit func(Module)) {
	m.context.visitDirectDepsIf(m.module, pred, visit)
}

func (m *moduleContext) VisitDepsDepthFirst(visit func(Module)) {
	m.context.visitDepsDepthFirst(m.module, visit)
}

func (m *moduleContext) VisitDepsDepthFirstIf(pred func(Module) bool,
	visit func(Module)) {

	m.context.visitDepsDepthFirstIf(m.module, pred, visit)
}

func (m *moduleContext) ModuleSubDir() string {
	return m.module.variantName
}

func (m *moduleContext) Variable(pctx *PackageContext, name, value string) {
	m.scope.ReparentTo(pctx)

	v, err := m.scope.AddLocalVariable(name, value)
	if err != nil {
		panic(err)
	}

	m.actionDefs.variables = append(m.actionDefs.variables, v)
}

func (m *moduleContext) Rule(pctx *PackageContext, name string,
	params RuleParams, argNames ...string) Rule {

	m.scope.ReparentTo(pctx)

	r, err := m.scope.AddLocalRule(name, &params, argNames...)
	if err != nil {
		panic(err)
	}

	m.actionDefs.rules = append(m.actionDefs.rules, r)

	return r
}

func (m *moduleContext) Build(pctx *PackageContext, params BuildParams) {
	m.scope.ReparentTo(pctx)

	def, err := parseBuildParams(m.scope, &params)
	if err != nil {
		panic(err)
	}

	m.actionDefs.buildDefs = append(m.actionDefs.buildDefs, def)
}

func (m *moduleContext) AddNinjaFileDeps(deps ...string) {
	m.ninjaFileDeps = append(m.ninjaFileDeps, deps...)
}

func (m *moduleContext) PrimaryModule() Module {
	return m.module.group.modules[0].logicModule
}

func (m *moduleContext) FinalModule() Module {
	return m.module.group.modules[len(m.module.group.modules)-1].logicModule
}

func (m *moduleContext) VisitAllModuleVariants(visit func(Module)) {
	for _, module := range m.module.group.modules {
		visit(module.logicModule)
	}
}

//
// DynamicDependerModuleContext
//

type dynamicDependerModuleContext struct {
	baseModuleContext

	module *moduleInfo
}

// AddVariationDependencies adds deps as dependencies of the current module, but uses the variations
// argument to select which variant of the dependency to use.  A variant of the dependency must
// exist that matches the all of the non-local variations of the current module, plus the variations
// argument.
func (mctx *dynamicDependerModuleContext) AddVariationDependencies(variations []Variation,
	deps ...string) {

	for _, dep := range deps {
		errs := mctx.context.addVariationDependency(mctx.module, variations, dep, false)
		if len(errs) > 0 {
			mctx.errs = append(mctx.errs, errs...)
		}
	}
}

// AddFarVariationDependencies adds deps as dependencies of the current module, but uses the
// variations argument to select which variant of the dependency to use.  A variant of the
// dependency must exist that matches the variations argument, but may also have other variations.
// For any unspecified variation the first variant will be used.
//
// Unlike AddVariationDependencies, the variations of the current module are ignored - the
// depdendency only needs to match the supplied variations.
func (mctx *dynamicDependerModuleContext) AddFarVariationDependencies(variations []Variation,
	deps ...string) {

	for _, dep := range deps {
		errs := mctx.context.addVariationDependency(mctx.module, variations, dep, true)
		if len(errs) > 0 {
			mctx.errs = append(mctx.errs, errs...)
		}
	}
}

//
// MutatorContext
//

type mutatorContext struct {
	baseModuleContext
	name                 string
	dependenciesModified bool
}

type baseMutatorContext interface {
	BaseModuleContext

	Module() Module
}

type EarlyMutatorContext interface {
	baseMutatorContext

	CreateVariations(...string) []Module
	CreateLocalVariations(...string) []Module
}

type TopDownMutatorContext interface {
	baseMutatorContext

	VisitDirectDeps(visit func(Module))
	VisitDirectDepsIf(pred func(Module) bool, visit func(Module))
	VisitDepsDepthFirst(visit func(Module))
	VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module))
}

type BottomUpMutatorContext interface {
	baseMutatorContext

	AddDependency(module Module, name string)
	CreateVariations(...string) []Module
	CreateLocalVariations(...string) []Module
	SetDependencyVariation(string)
}

// A Mutator function is called for each Module, and can use
// MutatorContext.CreateVariations to split a Module into multiple Modules,
// modifying properties on the new modules to differentiate them.  It is called
// after parsing all Blueprint files, but before generating any build rules,
// and is always called on dependencies before being called on the depending module.
//
// The Mutator function should only modify members of properties structs, and not
// members of the module struct itself, to ensure the modified values are copied
// if a second Mutator chooses to split the module a second time.
type TopDownMutator func(mctx TopDownMutatorContext)
type BottomUpMutator func(mctx BottomUpMutatorContext)
type EarlyMutator func(mctx EarlyMutatorContext)

// Split a module into mulitple variants, one for each name in the variationNames
// parameter.  It returns a list of new modules in the same order as the variationNames
// list.
//
// If any of the dependencies of the module being operated on were already split
// by calling CreateVariations with the same name, the dependency will automatically
// be updated to point the matching variant.
//
// If a module is split, and then a module depending on the first module is not split
// when the Mutator is later called on it, the dependency of the depending module will
// automatically be updated to point to the first variant.
func (mctx *mutatorContext) CreateVariations(variationNames ...string) []Module {
	return mctx.createVariations(variationNames, false)
}

// Split a module into mulitple variants, one for each name in the variantNames
// parameter.  It returns a list of new modules in the same order as the variantNames
// list.
//
// Local variations do not affect automatic dependency resolution - dependencies added
// to the split module via deps or DynamicDependerModule must exactly match a variant
// that contains all the non-local variations.
func (mctx *mutatorContext) CreateLocalVariations(variationNames ...string) []Module {
	return mctx.createVariations(variationNames, true)
}

func (mctx *mutatorContext) createVariations(variationNames []string, local bool) []Module {
	ret := []Module{}
	modules, errs := mctx.context.createVariations(mctx.module, mctx.name, variationNames)
	if len(errs) > 0 {
		mctx.errs = append(mctx.errs, errs...)
	}

	for i, module := range modules {
		ret = append(ret, module.logicModule)
		if !local {
			module.dependencyVariant[mctx.name] = variationNames[i]
		}
	}

	if len(ret) != len(variationNames) {
		panic("oops!")
	}

	return ret
}

// Set all dangling dependencies on the current module to point to the variation
// with given name.
func (mctx *mutatorContext) SetDependencyVariation(variationName string) {
	mctx.context.convertDepsToVariation(mctx.module, mctx.name, variationName)
}

func (mctx *mutatorContext) Module() Module {
	return mctx.module.logicModule
}

// Add a dependency to the given module.  The depender can be a specific variant
// of a module, but the dependee must be a module that has no variations.
// Does not affect the ordering of the current mutator pass, but will be ordered
// correctly for all future mutator passes.
func (mctx *mutatorContext) AddDependency(module Module, depName string) {
	errs := mctx.context.addDependency(mctx.context.moduleInfo[module], depName)
	if len(errs) > 0 {
		mctx.errs = append(mctx.errs, errs...)
	}
	mctx.dependenciesModified = true
}

func (mctx *mutatorContext) VisitDirectDeps(visit func(Module)) {
	mctx.context.visitDirectDeps(mctx.module, visit)
}

func (mctx *mutatorContext) VisitDirectDepsIf(pred func(Module) bool, visit func(Module)) {
	mctx.context.visitDirectDepsIf(mctx.module, pred, visit)
}

func (mctx *mutatorContext) VisitDepsDepthFirst(visit func(Module)) {
	mctx.context.visitDepsDepthFirst(mctx.module, visit)
}

func (mctx *mutatorContext) VisitDepsDepthFirstIf(pred func(Module) bool,
	visit func(Module)) {

	mctx.context.visitDepsDepthFirstIf(mctx.module, pred, visit)
}
