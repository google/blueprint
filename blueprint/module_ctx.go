package blueprint

import (
	"fmt"
	"path/filepath"
	"text/scanner"
)

// A Module handles generating all of the Ninja build actions needed to build a
// single module that is defined in a Blueprints file.  Module objects are
// created during the parse phase of a Context using one of the registered
// module types (and the associated ModuleFactory function).  The Module's
// properties struct is automatically filled in with the property values
// specified in the Blueprints file (see Context.RegisterModuleType for more
// information on this).
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
type Module interface {
	// GenerateBuildActions is called by the Context that created the Module
	// during its generate phase.  This call should generate all Ninja build
	// actions (rules, pools, and build statements) needed to build the module.
	GenerateBuildActions(ModuleContext)
}

type preGenerateModule interface {
	// PreGenerateBuildActions is called by the Context that created the Module
	// during its generate phase, before calling GenerateBuildActions on
	// any module.  It should not touch any Ninja build actions.
	PreGenerateBuildActions(PreModuleContext)
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

type DynamicDependerModuleContext interface {
	ModuleName() string
	ModuleDir() string
	Config() interface{}

	ContainsProperty(name string) bool
	Errorf(pos scanner.Position, fmt string, args ...interface{})
	ModuleErrorf(fmt string, args ...interface{})
	PropertyErrorf(property, fmt string, args ...interface{})
	Failed() bool
}

type PreModuleContext interface {
	DynamicDependerModuleContext

	OtherModuleName(m Module) string
	OtherModuleErrorf(m Module, fmt string, args ...interface{})

	VisitDepsDepthFirst(visit func(Module))
	VisitDepsDepthFirstIf(pred func(Module) bool, visit func(Module))
}

type ModuleContext interface {
	PreModuleContext

	Variable(pctx *PackageContext, name, value string)
	Rule(pctx *PackageContext, name string, params RuleParams, argNames ...string) Rule
	Build(pctx *PackageContext, params BuildParams)

	AddNinjaFileDeps(deps ...string)
}

var _ DynamicDependerModuleContext = (*dynamicDependerModuleContext)(nil)

type dynamicDependerModuleContext struct {
	context *Context
	config  interface{}
	info    *moduleInfo
	errs    []error
}

func (d *dynamicDependerModuleContext) ModuleName() string {
	return d.info.properties.Name
}

func (d *dynamicDependerModuleContext) ContainsProperty(name string) bool {
	_, ok := d.info.propertyPos[name]
	return ok
}

func (d *dynamicDependerModuleContext) ModuleDir() string {
	return filepath.Dir(d.info.relBlueprintsFile)
}

func (d *dynamicDependerModuleContext) Config() interface{} {
	return d.config
}

func (d *dynamicDependerModuleContext) Errorf(pos scanner.Position,
	format string, args ...interface{}) {

	d.errs = append(d.errs, &Error{
		Err: fmt.Errorf(format, args...),
		Pos: pos,
	})
}

func (d *dynamicDependerModuleContext) ModuleErrorf(format string,
	args ...interface{}) {

	d.errs = append(d.errs, &Error{
		Err: fmt.Errorf(format, args...),
		Pos: d.info.pos,
	})
}

func (d *dynamicDependerModuleContext) PropertyErrorf(property, format string,
	args ...interface{}) {

	pos, ok := d.info.propertyPos[property]
	if !ok {
		panic(fmt.Errorf("property %q was not set for this module", property))
	}

	d.errs = append(d.errs, &Error{
		Err: fmt.Errorf(format, args...),
		Pos: pos,
	})
}

func (d *dynamicDependerModuleContext) Failed() bool {
	return len(d.errs) > 0
}

var _ PreModuleContext = (*preModuleContext)(nil)

type preModuleContext struct {
	dynamicDependerModuleContext
	module Module
}

func (m *preModuleContext) OtherModuleName(module Module) string {
	info := m.context.moduleInfo[module]
	return info.properties.Name
}

func (m *preModuleContext) OtherModuleErrorf(module Module, format string,
	args ...interface{}) {

	info := m.context.moduleInfo[module]
	m.errs = append(m.errs, &Error{
		Err: fmt.Errorf(format, args...),
		Pos: info.pos,
	})
}

func (m *preModuleContext) VisitDepsDepthFirst(visit func(Module)) {
	m.context.visitDepsDepthFirst(m.module, visit)
}

func (m *preModuleContext) VisitDepsDepthFirstIf(pred func(Module) bool,
	visit func(Module)) {

	m.context.visitDepsDepthFirstIf(m.module, pred, visit)
}

var _ ModuleContext = (*moduleContext)(nil)

type moduleContext struct {
	preModuleContext
	scope         *localScope
	ninjaFileDeps []string
	actionDefs    localBuildActions
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
