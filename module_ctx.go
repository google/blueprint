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

	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"
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
// specified in the Blueprints file, dynamically added by implementing the
// (deprecated) DynamicDependerModule interface, or dynamically added by a
// BottomUpMutator) using the ModuleContext passed to GenerateBuildActions.
// This ModuleContext is also used to create Ninja build actions and to report
// errors to the user.
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
	// Name returns a string used to uniquely identify each module.  The return
	// value must be unique across all modules.  It is only called once, during
	// initial blueprint parsing.  To change the name later a mutator must call
	// MutatorContext.Rename
	//
	// In most cases, Name should return the contents of a "name:" property from
	// the blueprint file.  An embeddable SimpleName object can be used for this
	// case.
	Name() string

	// GenerateBuildActions is called by the Context that created the Module
	// during its generate phase.  This call should generate all Ninja build
	// actions (rules, pools, and build statements) needed to build the module.
	GenerateBuildActions(ModuleContext)
}

// A DynamicDependerModule is a Module that may add dependencies that do not
// appear in its "deps" property.  Any Module that implements this interface
// will have its DynamicDependencies method called by the Context that created
// it during generate phase.
//
// Deprecated, use a BottomUpMutator instead
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
	ModuleType() string
	Config() interface{}

	ContainsProperty(name string) bool
	Errorf(pos scanner.Position, fmt string, args ...interface{})
	ModuleErrorf(fmt string, args ...interface{})
	PropertyErrorf(property, fmt string, args ...interface{})
	Failed() bool

	// GlobWithDeps returns a list of files and directories that match the
	// specified pattern but do not match any of the patterns in excludes.
	// Any directories will have a '/' suffix.  It also adds efficient
	// dependencies to rerun the primary builder whenever a file matching
	// the pattern as added or removed, without rerunning if a file that
	// does not match the pattern is added to a searched directory.
	GlobWithDeps(pattern string, excludes []string) ([]string, error)

	Fs() pathtools.FileSystem
	AddNinjaFileDeps(deps ...string)

	moduleInfo() *moduleInfo
	error(err error)

	Namespace() Namespace
}

type DynamicDependerModuleContext BottomUpMutatorContext

type ModuleContext interface {
	BaseModuleContext

	OtherModuleName(m Module) string
	OtherModuleDir(m Module) string
	OtherModuleSubDir(m Module) string
	OtherModuleType(m Module) string
	OtherModuleErrorf(m Module, fmt string, args ...interface{})
	OtherModuleDependencyTag(m Module) DependencyTag

	GetDirectDepWithTag(name string, tag DependencyTag) Module
	GetDirectDep(name string) (Module, DependencyTag)

	VisitDirectDeps(visit func(Module))
	VisitDirectDepsIf(pred func(Module) bool, visit func(Module))
	VisitDepsDepthFirst(visit func(Module))
	VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module))
	WalkDeps(visit func(child, parent Module) bool)

	ModuleSubDir() string

	Variable(pctx PackageContext, name, value string)
	Rule(pctx PackageContext, name string, params RuleParams, argNames ...string) Rule
	Build(pctx PackageContext, params BuildParams)

	PrimaryModule() Module
	FinalModule() Module
	VisitAllModuleVariants(visit func(Module))

	GetMissingDependencies() []string
}

var _ BaseModuleContext = (*baseModuleContext)(nil)

type baseModuleContext struct {
	context        *Context
	config         interface{}
	module         *moduleInfo
	errs           []error
	visitingParent *moduleInfo
	visitingDep    depInfo
	ninjaFileDeps  []string
}

func (d *baseModuleContext) moduleInfo() *moduleInfo {
	return d.module
}

func (d *baseModuleContext) ModuleName() string {
	return d.module.Name()
}

func (d *baseModuleContext) ModuleType() string {
	return d.module.typeName
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

func (d *baseModuleContext) error(err error) {
	if err != nil {
		d.errs = append(d.errs, err)
	}
}

func (d *baseModuleContext) Errorf(pos scanner.Position,
	format string, args ...interface{}) {

	d.error(&BlueprintError{
		Err: fmt.Errorf(format, args...),
		Pos: pos,
	})
}

func (d *baseModuleContext) ModuleErrorf(format string,
	args ...interface{}) {

	d.error(&ModuleError{
		BlueprintError: BlueprintError{
			Err: fmt.Errorf(format, args...),
			Pos: d.module.pos,
		},
		module: d.module,
	})
}

func (d *baseModuleContext) PropertyErrorf(property, format string,
	args ...interface{}) {

	pos := d.module.propertyPos[property]

	if !pos.IsValid() {
		pos = d.module.pos
	}

	d.error(&PropertyError{
		ModuleError: ModuleError{
			BlueprintError: BlueprintError{
				Err: fmt.Errorf(format, args...),
				Pos: pos,
			},
			module: d.module,
		},
		property: property,
	})
}

func (d *baseModuleContext) Failed() bool {
	return len(d.errs) > 0
}

func (d *baseModuleContext) GlobWithDeps(pattern string,
	excludes []string) ([]string, error) {
	return d.context.glob(pattern, excludes)
}

func (d *baseModuleContext) Fs() pathtools.FileSystem {
	return d.context.fs
}

func (d *baseModuleContext) Namespace() Namespace {
	return d.context.nameInterface.GetNamespace(newNamespaceContext(d.module))
}

var _ ModuleContext = (*moduleContext)(nil)

type moduleContext struct {
	baseModuleContext
	scope              *localScope
	actionDefs         localBuildActions
	handledMissingDeps bool
}

func (m *baseModuleContext) OtherModuleName(logicModule Module) string {
	module := m.context.moduleInfo[logicModule]
	return module.Name()
}

func (m *baseModuleContext) OtherModuleDir(logicModule Module) string {
	module := m.context.moduleInfo[logicModule]
	return filepath.Dir(module.relBlueprintsFile)
}

func (m *baseModuleContext) OtherModuleSubDir(logicModule Module) string {
	module := m.context.moduleInfo[logicModule]
	return module.variantName
}

func (m *baseModuleContext) OtherModuleType(logicModule Module) string {
	module := m.context.moduleInfo[logicModule]
	return module.typeName
}

func (m *baseModuleContext) OtherModuleErrorf(logicModule Module, format string,
	args ...interface{}) {

	module := m.context.moduleInfo[logicModule]
	m.errs = append(m.errs, &ModuleError{
		BlueprintError: BlueprintError{
			Err: fmt.Errorf(format, args...),
			Pos: module.pos,
		},
		module: module,
	})
}

// OtherModuleDependencyTag returns the dependency tag used to depend on a module, or nil if there is no dependency
// on the module.  When called inside a Visit* method with current module being visited, and there are multiple
// dependencies on the module being visited, it returns the dependency tag used for the current dependency.
func (m *baseModuleContext) OtherModuleDependencyTag(logicModule Module) DependencyTag {
	// fast path for calling OtherModuleDependencyTag from inside VisitDirectDeps
	if logicModule == m.visitingDep.module.logicModule {
		return m.visitingDep.tag
	}

	for _, dep := range m.visitingParent.directDeps {
		if dep.module.logicModule == logicModule {
			return dep.tag
		}
	}

	return nil
}

// GetDirectDep returns the Module and DependencyTag for the  direct dependency with the specified
// name, or nil if none exists.  If there are multiple dependencies on the same module it returns
// the first DependencyTag.
func (m *baseModuleContext) GetDirectDep(name string) (Module, DependencyTag) {
	for _, dep := range m.module.directDeps {
		if dep.module.Name() == name {
			return dep.module.logicModule, dep.tag
		}
	}

	return nil, nil
}

// GetDirectDepWithTag returns the Module the direct dependency with the specified name, or nil if
// none exists.  It panics if the dependency does not have the specified tag.
func (m *baseModuleContext) GetDirectDepWithTag(name string, tag DependencyTag) Module {
	var deps []depInfo
	for _, dep := range m.module.directDeps {
		if dep.module.Name() == name {
			if dep.tag == tag {
				return dep.module.logicModule
			}
			deps = append(deps, dep)
		}
	}

	if len(deps) != 0 {
		panic(fmt.Errorf("Unable to find dependency %q with requested tag %#v. Found: %#v", deps[0].module, tag, deps))
	}

	return nil
}

// VisitDirectDeps calls visit for each direct dependency.  If there are multiple direct dependencies on the same module
// visit will be called multiple times on that module and OtherModuleDependencyTag will return a different tag for each.
func (m *baseModuleContext) VisitDirectDeps(visit func(Module)) {
	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitDirectDeps(%s, %s) for dependency %s",
				m.module, funcName(visit), m.visitingDep.module))
		}
	}()

	m.visitingParent = m.module

	for _, dep := range m.module.directDeps {
		m.visitingDep = dep
		visit(dep.module.logicModule)
	}

	m.visitingParent = nil
	m.visitingDep = depInfo{}
}

// VisitDirectDepsIf calls pred for each direct dependency, and if pred returns true calls visit.  If there are multiple
// direct dependencies on the same module pred and visit will be called multiple times on that module and
// OtherModuleDependencyTag will return a different tag for each.
func (m *baseModuleContext) VisitDirectDepsIf(pred func(Module) bool, visit func(Module)) {
	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitDirectDepsIf(%s, %s, %s) for dependency %s",
				m.module, funcName(pred), funcName(visit), m.visitingDep.module))
		}
	}()

	m.visitingParent = m.module

	for _, dep := range m.module.directDeps {
		m.visitingDep = dep
		if pred(dep.module.logicModule) {
			visit(dep.module.logicModule)
		}
	}

	m.visitingParent = nil
	m.visitingDep = depInfo{}
}

// VisitDepsDepthFirst calls visit for each transitive dependency, traversing the dependency tree in depth first order.
// visit will only be called once for any given module, even if there are multiple paths through the dependency tree
// to the module or multiple direct dependencies with different tags.  OtherModuleDependencyTag will return the tag for
// the first path found to the module.
func (m *baseModuleContext) VisitDepsDepthFirst(visit func(Module)) {
	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitDepsDepthFirst(%s, %s) for dependency %s",
				m.module, funcName(visit), m.visitingDep.module))
		}
	}()

	m.context.walkDeps(m.module, false, nil, func(dep depInfo, parent *moduleInfo) {
		m.visitingParent = parent
		m.visitingDep = dep
		visit(dep.module.logicModule)
	})

	m.visitingParent = nil
	m.visitingDep = depInfo{}
}

// VisitDepsDepthFirst calls pred for each transitive dependency, and if pred returns true calls visit, traversing the
// dependency tree in depth first order.  visit will only be called once for any given module, even if there are
// multiple paths through the dependency tree to the module or multiple direct dependencies with different tags.
// OtherModuleDependencyTag will return the tag for the first path found to the module.  The return value of pred does
// not affect which branches of the tree are traversed.
func (m *baseModuleContext) VisitDepsDepthFirstIf(pred func(Module) bool,
	visit func(Module)) {

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitDepsDepthFirstIf(%s, %s, %s) for dependency %s",
				m.module, funcName(pred), funcName(visit), m.visitingDep.module))
		}
	}()

	m.context.walkDeps(m.module, false, nil, func(dep depInfo, parent *moduleInfo) {
		if pred(dep.module.logicModule) {
			m.visitingParent = parent
			m.visitingDep = dep
			visit(dep.module.logicModule)
		}
	})

	m.visitingParent = nil
	m.visitingDep = depInfo{}
}

// WalkDeps calls visit for each transitive dependency, traversing the dependency tree in top down order.  visit may be
// called multiple times for the same (child, parent) pair if there are multiple direct dependencies between the
// child and parent with different tags.  OtherModuleDependencyTag will return the tag for the currently visited
// (child, parent) pair.  If visit returns false WalkDeps will not continue recursing down to child.
func (m *baseModuleContext) WalkDeps(visit func(child, parent Module) bool) {
	m.context.walkDeps(m.module, true, func(dep depInfo, parent *moduleInfo) bool {
		m.visitingParent = parent
		m.visitingDep = dep
		return visit(dep.module.logicModule, parent.logicModule)
	}, nil)

	m.visitingParent = nil
	m.visitingDep = depInfo{}
}

func (m *baseModuleContext) AddNinjaFileDeps(deps ...string) {
	m.ninjaFileDeps = append(m.ninjaFileDeps, deps...)
}

func (m *moduleContext) ModuleSubDir() string {
	return m.module.variantName
}

func (m *moduleContext) Variable(pctx PackageContext, name, value string) {
	m.scope.ReparentTo(pctx)

	v, err := m.scope.AddLocalVariable(name, value)
	if err != nil {
		panic(err)
	}

	m.actionDefs.variables = append(m.actionDefs.variables, v)
}

func (m *moduleContext) Rule(pctx PackageContext, name string,
	params RuleParams, argNames ...string) Rule {

	m.scope.ReparentTo(pctx)

	r, err := m.scope.AddLocalRule(name, &params, argNames...)
	if err != nil {
		panic(err)
	}

	m.actionDefs.rules = append(m.actionDefs.rules, r)

	return r
}

func (m *moduleContext) Build(pctx PackageContext, params BuildParams) {
	m.scope.ReparentTo(pctx)

	def, err := parseBuildParams(m.scope, &params)
	if err != nil {
		panic(err)
	}

	m.actionDefs.buildDefs = append(m.actionDefs.buildDefs, def)
}

func (m *moduleContext) PrimaryModule() Module {
	return m.module.group.modules[0].logicModule
}

func (m *moduleContext) FinalModule() Module {
	return m.module.group.modules[len(m.module.group.modules)-1].logicModule
}

func (m *moduleContext) VisitAllModuleVariants(visit func(Module)) {
	m.context.visitAllModuleVariants(m.module, visit)
}

func (m *moduleContext) GetMissingDependencies() []string {
	m.handledMissingDeps = true
	return m.module.missingDeps
}

//
// MutatorContext
//

type mutatorContext struct {
	baseModuleContext
	name             string
	reverseDeps      []reverseDep
	rename           []rename
	replace          []replace
	newVariations    []*moduleInfo // new variants of existing modules
	newModules       []*moduleInfo // brand new modules
	defaultVariation *string
}

type baseMutatorContext interface {
	BaseModuleContext

	OtherModuleExists(name string) bool
	Rename(name string)
	Module() Module
}

type EarlyMutatorContext interface {
	baseMutatorContext

	CreateVariations(...string) []Module
	CreateLocalVariations(...string) []Module
}

type TopDownMutatorContext interface {
	baseMutatorContext

	OtherModuleName(m Module) string
	OtherModuleDir(m Module) string
	OtherModuleSubDir(m Module) string
	OtherModuleType(m Module) string
	OtherModuleErrorf(m Module, fmt string, args ...interface{})
	OtherModuleDependencyTag(m Module) DependencyTag

	CreateModule(ModuleFactory, ...interface{})

	GetDirectDepWithTag(name string, tag DependencyTag) Module
	GetDirectDep(name string) (Module, DependencyTag)

	VisitDirectDeps(visit func(Module))
	VisitDirectDepsIf(pred func(Module) bool, visit func(Module))
	VisitDepsDepthFirst(visit func(Module))
	VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module))
	WalkDeps(visit func(Module, Module) bool)
}

type BottomUpMutatorContext interface {
	baseMutatorContext

	AddDependency(module Module, tag DependencyTag, name ...string)
	AddReverseDependency(module Module, tag DependencyTag, name string)
	CreateVariations(...string) []Module
	CreateLocalVariations(...string) []Module

	// SetDependencyVariation sets all dangling dependencies on the current module to point to the variation
	// with given name. This function ignores the default variation set by SetDefaultDependencyVariation.
	SetDependencyVariation(string)

	// SetDefaultDependencyVariation sets the default variation when a dangling reference is detected
	// during the subsequent calls on Create*Variations* functions. To reset, set it to nil.
	SetDefaultDependencyVariation(*string)

	AddVariationDependencies([]Variation, DependencyTag, ...string)
	AddFarVariationDependencies([]Variation, DependencyTag, ...string)
	AddInterVariantDependency(tag DependencyTag, from, to Module)
	ReplaceDependencies(string)
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

// DependencyTag is an interface to an arbitrary object that embeds BaseDependencyTag.  It can be
// used to transfer information on a dependency between the mutator that called AddDependency
// and the GenerateBuildActions method.  Variants created by CreateVariations have a copy of the
// interface (pointing to the same concrete object) from their original module.
type DependencyTag interface {
	dependencyTag(DependencyTag)
}

type BaseDependencyTag struct {
}

func (BaseDependencyTag) dependencyTag(DependencyTag) {
}

var _ DependencyTag = BaseDependencyTag{}

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
	modules, errs := mctx.context.createVariations(mctx.module, mctx.name, mctx.defaultVariation, variationNames)
	if len(errs) > 0 {
		mctx.errs = append(mctx.errs, errs...)
	}

	for i, module := range modules {
		ret = append(ret, module.logicModule)
		if !local {
			module.dependencyVariant[mctx.name] = variationNames[i]
		}
	}

	if mctx.newVariations != nil {
		panic("module already has variations from this mutator")
	}
	mctx.newVariations = modules

	if len(ret) != len(variationNames) {
		panic("oops!")
	}

	return ret
}

// Set all dangling dependencies on the current module to point to the variation
// with given name.
func (mctx *mutatorContext) SetDependencyVariation(variationName string) {
	mctx.context.convertDepsToVariation(mctx.module, mctx.name, variationName, nil)
}

func (mctx *mutatorContext) SetDefaultDependencyVariation(variationName *string) {
	mctx.defaultVariation = variationName
}

func (mctx *mutatorContext) Module() Module {
	return mctx.module.logicModule
}

// Add a dependency to the given module.
// Does not affect the ordering of the current mutator pass, but will be ordered
// correctly for all future mutator passes.
func (mctx *mutatorContext) AddDependency(module Module, tag DependencyTag, deps ...string) {
	for _, dep := range deps {
		modInfo := mctx.context.moduleInfo[module]
		errs := mctx.context.addDependency(modInfo, tag, dep)
		if len(errs) > 0 {
			mctx.errs = append(mctx.errs, errs...)
		}
	}
}

// Add a dependency from the destination to the given module.
// Does not affect the ordering of the current mutator pass, but will be ordered
// correctly for all future mutator passes.  All reverse dependencies for a destination module are
// collected until the end of the mutator pass, sorted by name, and then appended to the destination
// module's dependency list.
func (mctx *mutatorContext) AddReverseDependency(module Module, tag DependencyTag, destName string) {
	if _, ok := tag.(BaseDependencyTag); ok {
		panic("BaseDependencyTag is not allowed to be used directly!")
	}

	destModule, errs := mctx.context.findReverseDependency(mctx.context.moduleInfo[module], destName)
	if len(errs) > 0 {
		mctx.errs = append(mctx.errs, errs...)
		return
	}

	mctx.reverseDeps = append(mctx.reverseDeps, reverseDep{
		destModule,
		depInfo{mctx.context.moduleInfo[module], tag},
	})
}

// AddVariationDependencies adds deps as dependencies of the current module, but uses the variations
// argument to select which variant of the dependency to use.  A variant of the dependency must
// exist that matches the all of the non-local variations of the current module, plus the variations
// argument.
func (mctx *mutatorContext) AddVariationDependencies(variations []Variation, tag DependencyTag,
	deps ...string) {

	for _, dep := range deps {
		errs := mctx.context.addVariationDependency(mctx.module, variations, tag, dep, false)
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
func (mctx *mutatorContext) AddFarVariationDependencies(variations []Variation, tag DependencyTag,
	deps ...string) {

	for _, dep := range deps {
		errs := mctx.context.addVariationDependency(mctx.module, variations, tag, dep, true)
		if len(errs) > 0 {
			mctx.errs = append(mctx.errs, errs...)
		}
	}
}

func (mctx *mutatorContext) AddInterVariantDependency(tag DependencyTag, from, to Module) {
	mctx.context.addInterVariantDependency(mctx.module, tag, from, to)
}

// ReplaceDependencies replaces all dependencies on the identical variant of the module with the
// specified name with the current variant of this module.  Replacements don't take effect until
// after the mutator pass is finished.
func (mctx *mutatorContext) ReplaceDependencies(name string) {
	target := mctx.context.moduleMatchingVariant(mctx.module, name)

	if target == nil {
		panic(fmt.Errorf("ReplaceDependencies could not find identical variant %q for module %q",
			mctx.module.variantName, name))
	}

	mctx.replace = append(mctx.replace, replace{target, mctx.module})
}

func (mctx *mutatorContext) OtherModuleExists(name string) bool {
	_, exists := mctx.context.nameInterface.ModuleFromName(name, mctx.module.namespace())
	return exists
}

// Rename all variants of a module.  The new name is not visible to calls to ModuleName,
// AddDependency or OtherModuleName until after this mutator pass is complete.
func (mctx *mutatorContext) Rename(name string) {
	mctx.rename = append(mctx.rename, rename{mctx.module.group, name})
}

// Create a new module by calling the factory method for the specified moduleType, and apply
// the specified property structs to it as if the properties were set in a blueprint file.
func (mctx *mutatorContext) CreateModule(factory ModuleFactory, props ...interface{}) {
	module := mctx.context.newModule(factory)

	module.relBlueprintsFile = mctx.module.relBlueprintsFile
	module.pos = mctx.module.pos

	for _, p := range props {
		err := proptools.AppendMatchingProperties(module.properties, p, nil)
		if err != nil {
			panic(err)
		}
	}

	mctx.newModules = append(mctx.newModules, module)
}

// SimpleName is an embeddable object to implement the ModuleContext.Name method using a property
// called "name".  Modules that embed it must also add SimpleName.Properties to their property
// structure list.
type SimpleName struct {
	Properties struct {
		Name string
	}
}

func (s *SimpleName) Name() string {
	return s.Properties.Name
}
