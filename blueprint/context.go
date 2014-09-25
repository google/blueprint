package blueprint

import (
	"blueprint/parser"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"text/scanner"
	"text/template"
)

var ErrBuildActionsNotReady = errors.New("build actions are not ready")

const maxErrors = 10

// A Context contains all the state needed to parse a set of Blueprints files
// and generate a Ninja file.  The process of generating a Ninja file proceeds
// through a series of four phases.  Each phase corresponds with a some methods
// on the Context object
//
//         Phase                            Methods
//      ------------      -------------------------------------------
//   1. Registration         RegisterModuleType, RegisterSingletonType
//
//   2. Parse                    ParseBlueprintsFiles, Parse
//
//   3. Generate            ResolveDependencies, PrepareBuildActions
//
//   4. Write                           WriteBuildFile
//
// The registration phase prepares the context to process Blueprints files
// containing various types of modules.  The parse phase reads in one or more
// Blueprints files and validates their contents against the module types that
// have been registered.  The generate phase then analyzes the parsed Blueprints
// contents to create an internal representation for the build actions that must
// be performed.  This phase also performs validation of the module dependencies
// and property values defined in the parsed Blueprints files.  Finally, the
// write phase generates the Ninja manifest text based on the generated build
// actions.
type Context struct {
	// set at instantiation
	moduleFactories map[string]ModuleFactory
	modules         map[string]Module
	moduleInfo      map[Module]*moduleInfo
	singletonInfo   map[string]*singletonInfo

	dependenciesReady bool // set to true on a successful ResolveDependencies
	buildActionsReady bool // set to true on a successful PrepareBuildActions

	// set by SetIgnoreUnknownModuleTypes
	ignoreUnknownModuleTypes bool

	// set during PrepareBuildActions
	pkgNames        map[*pkg]string
	globalVariables map[Variable]*ninjaString
	globalPools     map[Pool]*poolDef
	globalRules     map[Rule]*ruleDef

	// set during PrepareBuildActions
	buildDir           *ninjaString // The builddir special Ninja variable
	requiredNinjaMajor int          // For the ninja_required_version variable
	requiredNinjaMinor int          // For the ninja_required_version variable
	requiredNinjaMicro int          // For the ninja_required_version variable

	// set lazily by sortedModuleNames
	cachedSortedModuleNames []string
}

// An Error describes a problem that was encountered that is related to a
// particular location in a Blueprints file.
type Error struct {
	Err error            // the error that occurred
	Pos scanner.Position // the relevant Blueprints file location
}

type localBuildActions struct {
	variables []*localVariable
	rules     []*localRule
	buildDefs []*buildDef
}

type moduleInfo struct {
	// set during Parse
	typeName          string
	relBlueprintsFile string
	pos               scanner.Position
	propertyPos       map[string]scanner.Position
	properties        struct {
		Name    string
		Deps    []string
		Targets map[string][]*parser.Property
	}

	// set during ResolveDependencies
	directDeps []Module

	// set during PrepareBuildActions
	actionDefs localBuildActions
}

type singletonInfo struct {
	// set during RegisterSingletonType
	factory   SingletonFactory
	singleton Singleton

	// set during PrepareBuildActions
	actionDefs localBuildActions
}

type TargetSelector interface {
	SelectTarget() string
}

// Default target selector that simply returns the host OS name
type goosTargetSelector struct {
}

func (g *goosTargetSelector) SelectTarget() string {
	return runtime.GOOS
}

func (e *Error) Error() string {

	return fmt.Sprintf("%s: %s", e.Pos, e.Err)
}

// NewContext creates a new Context object.  The created context initially has
// no module or singleton factories registered, so the RegisterModuleFactory and
// RegisterSingletonFactory methods must be called before it can do anything
// useful.
func NewContext() *Context {
	return &Context{
		moduleFactories: make(map[string]ModuleFactory),
		modules:         make(map[string]Module),
		moduleInfo:      make(map[Module]*moduleInfo),
		singletonInfo:   make(map[string]*singletonInfo),
	}
}

// A ModuleFactory function creates a new Module object.  See the
// Context.RegisterModuleType method for details about how a registered
// ModuleFactory is used by a Context.
type ModuleFactory func() (m Module, propertyStructs []interface{})

// RegisterModuleType associates a module type name (which can appear in a
// Blueprints file) with a Module factory function.  When the given module type
// name is encountered in a Blueprints file during parsing, the Module factory
// is invoked to instantiate a new Module object to handle the build action
// generation for the module.
//
// The module type names given here must be unique for the context.  The factory
// function should be a named function so that its package and name can be
// included in the generated Ninja file for debugging purposes.
//
// The factory function returns two values.  The first is the newly created
// Module object.  The second is a slice of pointers to that Module object's
// properties structs.  Each properties struct is examined when parsing a module
// definition of this type in a Blueprints file.  Exported fields of the
// properties structs are automatically set to the property values specified in
// the Blueprints file.  The properties struct field names determine the name of
// the Blueprints file properties that are used - the Blueprints property name
// matches that of the properties struct field name with the first letter
// converted to lower-case.
//
// The fields of the properties struct must be either []string, a string, or
// bool. The Context will panic if a Module gets instantiated with a properties
// struct containing a field that is not one these supported types.
//
// Any properties that appear in the Blueprints files that are not built-in
// module properties (such as "name" and "deps") and do not have a corresponding
// field in the returned module properties struct result in an error during the
// Context's parse phase.
//
// As an example, the follow code:
//
//   type myModule struct {
//       properties struct {
//           Foo string
//           Bar []string
//       }
//   }
//
//   func NewMyModule() (blueprint.Module, []interface{}) {
//       module := new(myModule)
//       properties := &module.properties
//       return module, []interface{}{properties}
//   }
//
//   func main() {
//       ctx := blueprint.NewContext()
//       ctx.RegisterModuleType("my_module", NewMyModule)
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
func (c *Context) RegisterModuleType(name string, factory ModuleFactory) {
	if _, present := c.moduleFactories[name]; present {
		panic(errors.New("module type name is already registered"))
	}
	c.moduleFactories[name] = factory
}

// A SingletonFactory function creates a new Singleton object.  See the
// Context.RegisterSingletonType method for details about how a registered
// SingletonFactory is used by a Context.
type SingletonFactory func() Singleton

// RegisterSingletonType registers a singleton type that will be invoked to
// generate build actions.  Each registered singleton type is instantiated and
// and invoked exactly once as part of the generate phase.
//
// The singleton type names given here must be unique for the context.  The
// factory function should be a named function so that its package and name can
// be included in the generated Ninja file for debugging purposes.
func (c *Context) RegisterSingletonType(name string, factory SingletonFactory) {
	if _, present := c.singletonInfo[name]; present {
		panic(errors.New("singleton name is already registered"))
	}

	c.singletonInfo[name] = &singletonInfo{
		factory:   factory,
		singleton: factory(),
	}
}

func singletonPkgPath(singleton Singleton) string {
	typ := reflect.TypeOf(singleton)
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.PkgPath()
}

func singletonTypeName(singleton Singleton) string {
	typ := reflect.TypeOf(singleton)
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.PkgPath() + "." + typ.Name()
}

// SetIgnoreUnknownModuleTypes sets the behavior of the context in the case
// where it encounters an unknown module type while parsing Blueprints files. By
// default, the context will report unknown module types as an error.  If this
// method is called with ignoreUnknownModuleTypes set to true then the context
// will silently ignore unknown module types.
//
// This method should generally not be used.  It exists to facilitate the
// bootstrapping process.
func (c *Context) SetIgnoreUnknownModuleTypes(ignoreUnknownModuleTypes bool) {
	c.ignoreUnknownModuleTypes = ignoreUnknownModuleTypes
}

// Parse parses a single Blueprints file from r, creating Module objects for
// each of the module definitions encountered.  If the Blueprints file contains
// an assignment to the "subdirs" variable, then the subdirectories listed are
// returned in the subdirs first return value.
//
// rootDir specifies the path to the root directory of the source tree, while
// filename specifies the path to the Blueprints file.  These paths are used for
// error reporting and for determining the module's directory.
//
// This method should probably not be used directly.  It is provided to simplify
// testing.  Instead ParseBlueprintsFiles should be called to parse a set of
// Blueprints files starting from a top-level Blueprints file.
func (c *Context) Parse(rootDir, filename string, r io.Reader) (subdirs []string,
	errs []error) {

	c.dependenciesReady = false

	relBlueprintsFile, err := filepath.Rel(rootDir, filename)
	if err != nil {
		return nil, []error{err}
	}

	defs, errs := parser.Parse(filename, r)
	if len(errs) > 0 {
		for i, err := range errs {
			if parseErr, ok := err.(*parser.ParseError); ok {
				err = &Error{
					Err: parseErr.Err,
					Pos: parseErr.Pos,
				}
				errs[i] = err
			}
		}

		// If there were any parse errors don't bother trying to interpret the
		// result.
		return nil, errs
	}

	for _, def := range defs {
		var newErrs []error
		switch def := def.(type) {
		case *parser.Module:
			newErrs = c.processModuleDef(def, relBlueprintsFile)

		case *parser.Assignment:
			var newSubdirs []string
			newSubdirs, newErrs = c.processAssignment(def)
			if newSubdirs != nil {
				subdirs = newSubdirs
			}

		default:
			panic("unknown definition type")
		}

		if len(newErrs) > 0 {
			errs = append(errs, newErrs...)
			if len(errs) > maxErrors {
				break
			}
		}
	}

	return subdirs, errs
}

// ParseBlueprintsFiles parses a set of Blueprints files starting with the file
// at rootFile.  When it encounters a Blueprints file with a set of subdirs
// listed it recursively parses any Blueprints files found in those
// subdirectories.
//
// If no errors are encountered while parsing the files, the list of paths on
// which the future output will depend is returned.  This list will include both
// Blueprints file paths as well as directory paths for cases where wildcard
// subdirs are found.
func (c *Context) ParseBlueprintsFiles(rootFile string) (deps []string,
	errs []error) {

	rootDir := filepath.Dir(rootFile)

	depsSet := map[string]bool{rootFile: true}
	blueprints := []string{rootFile}

	var file *os.File
	defer func() {
		if file != nil {
			file.Close()
		}
	}()

	var err error

	for i := 0; i < len(blueprints); i++ {
		if len(errs) > maxErrors {
			return
		}

		filename := blueprints[i]
		dir := filepath.Dir(filename)

		file, err = os.Open(filename)
		if err != nil {
			errs = append(errs, &Error{Err: err})
			continue
		}

		subdirs, newErrs := c.Parse(rootDir, filename, file)
		if len(newErrs) > 0 {
			errs = append(errs, newErrs...)
			continue
		}

		err = file.Close()
		if err != nil {
			errs = append(errs, &Error{Err: err})
			continue
		}

		// Add the subdirs to the list of directories to parse Blueprint files
		// from.
		for _, subdir := range subdirs {
			subdir = filepath.Join(dir, subdir)
			dirPart, filePart := filepath.Split(subdir)
			dirPart = filepath.Clean(dirPart)

			if filePart == "*" {
				foundSubdirs, err := listSubdirs(dirPart)
				if err != nil {
					errs = append(errs, &Error{Err: err})
					continue
				}

				for _, foundSubdir := range foundSubdirs {
					subBlueprints := filepath.Join(dirPart, foundSubdir,
						"Blueprints")

					_, err := os.Stat(subBlueprints)
					if os.IsNotExist(err) {
						// There is no Blueprints file in this subdirectory.  We
						// need to add the directory to the list of dependencies
						// so that if someone adds a Blueprints file in the
						// future we'll pick it up.
						depsSet[filepath.Dir(subBlueprints)] = true
					} else if !depsSet[subBlueprints] {
						// We haven't seen this Blueprints file before, so add
						// it to our list.
						depsSet[subBlueprints] = true
						blueprints = append(blueprints, subBlueprints)
					}
				}

				// We now depend on the directory itself because if any new
				// subdirectories get added or removed we need to rebuild the
				// Ninja manifest.
				depsSet[dirPart] = true
			} else {
				subBlueprints := filepath.Join(subdir, "Blueprints")
				if !depsSet[subBlueprints] {
					depsSet[subBlueprints] = true
					blueprints = append(blueprints, subBlueprints)
				}
			}
		}
	}

	for dep := range depsSet {
		deps = append(deps, dep)
	}

	return
}

func listSubdirs(dir string) ([]string, error) {
	d, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer d.Close()

	infos, err := d.Readdir(-1)
	if err != nil {
		return nil, err
	}

	var subdirs []string
	for _, info := range infos {
		if info.IsDir() {
			subdirs = append(subdirs, info.Name())
		}
	}

	return subdirs, nil
}

func (c *Context) processAssignment(
	assignment *parser.Assignment) (subdirs []string, errs []error) {

	if assignment.Name == "subdirs" {
		switch assignment.Value.Type {
		case parser.List:
			subdirs = make([]string, 0, len(assignment.Value.ListValue))

			for _, value := range assignment.Value.ListValue {
				if value.Type != parser.String {
					// The parser should not produce this.
					panic("non-string value found in list")
				}

				dirPart, filePart := filepath.Split(value.StringValue)
				if (filePart != "*" && strings.ContainsRune(filePart, '*')) ||
					strings.ContainsRune(dirPart, '*') {

					errs = append(errs, &Error{
						Err: fmt.Errorf("subdirs may only wildcard whole " +
							"directories"),
						Pos: value.Pos,
					})

					continue
				}

				subdirs = append(subdirs, value.StringValue)
			}

			if len(errs) > 0 {
				subdirs = nil
			}

			return

		case parser.Bool, parser.String:
			errs = []error{
				&Error{
					Err: fmt.Errorf("subdirs must be a list of strings"),
					Pos: assignment.Pos,
				},
			}

			return

		default:
			panic(fmt.Errorf("unknown value type: %d", assignment.Value.Type))
		}
	}

	return nil, []error{
		&Error{
			Err: fmt.Errorf("only 'subdirs' assignment is supported"),
			Pos: assignment.Pos,
		},
	}
}

func (c *Context) processModuleDef(moduleDef *parser.Module,
	relBlueprintsFile string) []error {

	typeName := moduleDef.Type
	factory, ok := c.moduleFactories[typeName]
	if !ok {
		if c.ignoreUnknownModuleTypes {
			return nil
		}

		return []error{
			&Error{
				Err: fmt.Errorf("unrecognized module type %q", typeName),
				Pos: moduleDef.Pos,
			},
		}
	}

	module, properties := factory()
	info := &moduleInfo{
		typeName:          typeName,
		relBlueprintsFile: relBlueprintsFile,
	}

	properties = append(properties, &info.properties)

	errs := unpackProperties(moduleDef.Properties, properties...)
	if len(errs) > 0 {
		return errs
	}

	var targetName string
	if selector, ok := module.(TargetSelector); ok {
		targetName = selector.SelectTarget()
	} else {
		defaultSelector := goosTargetSelector{}
		targetName = defaultSelector.SelectTarget()
	}

	if targetProperties, ok := info.properties.Targets[targetName]; ok {
		errs = mergeProperties(targetProperties, properties...)
		if len(errs) > 0 {
			return errs
		}
	}

	info.pos = moduleDef.Pos
	info.propertyPos = make(map[string]scanner.Position)
	for _, propertyDef := range moduleDef.Properties {
		info.propertyPos[propertyDef.Name] = propertyDef.Pos
	}

	name := info.properties.Name
	err := validateNinjaName(name)
	if err != nil {
		return []error{
			&Error{
				Err: fmt.Errorf("invalid module name %q: %s", err),
				Pos: info.propertyPos["name"],
			},
		}
	}

	if first, present := c.modules[name]; present {
		errs = append(errs, &Error{
			Err: fmt.Errorf("module %q already defined", name),
			Pos: moduleDef.Pos,
		})
		errs = append(errs, &Error{
			Err: fmt.Errorf("<-- previous definition here"),
			Pos: c.moduleInfo[first].pos,
		})
		if len(errs) > 0 {
			return errs
		}
	}

	c.modules[name] = module
	c.moduleInfo[module] = info

	return nil
}

// ResolveDependencies checks that the dependencies specified by all of the
// modules defined in the parsed Blueprints files are valid.  This means that
// the modules depended upon are defined and that no circular dependencies
// exist.
//
// The config argument is made available to all of the DynamicDependerModule
// objects via the Config method on the DynamicDependerModuleContext objects
// passed to their DynamicDependencies method.
func (c *Context) ResolveDependencies(config interface{}) []error {
	errs := c.resolveDependencies(config)
	if len(errs) > 0 {
		return errs
	}

	errs = c.checkForDependencyCycles()
	if len(errs) > 0 {
		return errs
	}

	c.dependenciesReady = true
	return nil
}

// moduleDepNames returns the sorted list of dependency names for a given
// module.  If the module implements the DynamicDependerModule interface then
// this set consists of the union of those module names listed in its "deps"
// property and those returned by its DynamicDependencies method.  Otherwise it
// is simply those names listed in its "deps" property.
func (c *Context) moduleDepNames(info *moduleInfo,
	config interface{}) ([]string, []error) {

	depNamesSet := make(map[string]bool)

	for _, depName := range info.properties.Deps {
		depNamesSet[depName] = true
	}

	module := c.modules[info.properties.Name]
	dynamicDepender, ok := module.(DynamicDependerModule)
	if ok {
		ddmctx := &dynamicDependerModuleContext{
			context: c,
			config:  config,
			info:    info,
		}

		dynamicDeps := dynamicDepender.DynamicDependencies(ddmctx)

		if len(ddmctx.errs) > 0 {
			return nil, ddmctx.errs
		}

		for _, depName := range dynamicDeps {
			depNamesSet[depName] = true
		}
	}

	// We need to sort the dependency names to ensure deterministic Ninja file
	// output from one run to the next.
	depNames := make([]string, 0, len(depNamesSet))
	for depName := range depNamesSet {
		depNames = append(depNames, depName)
	}
	sort.Strings(depNames)

	return depNames, nil
}

// resolveDependencies populates the moduleInfo.directDeps list for every
// module.  In doing so it checks for missing dependencies and self-dependant
// modules.
func (c *Context) resolveDependencies(config interface{}) (errs []error) {
	for _, info := range c.moduleInfo {
		depNames, newErrs := c.moduleDepNames(info, config)
		if len(newErrs) > 0 {
			errs = append(errs, newErrs...)
			continue
		}

		info.directDeps = make([]Module, 0, len(depNames))
		depsPos := info.propertyPos["deps"]

		for _, depName := range depNames {
			if depName == info.properties.Name {
				errs = append(errs, &Error{
					Err: fmt.Errorf("%q depends on itself", depName),
					Pos: depsPos,
				})
				continue
			}

			dep, ok := c.modules[depName]
			if !ok {
				errs = append(errs, &Error{
					Err: fmt.Errorf("%q depends on undefined module %q",
						info.properties.Name, depName),
					Pos: depsPos,
				})
				continue
			}

			info.directDeps = append(info.directDeps, dep)
		}
	}

	return
}

// checkForDependencyCycles recursively walks the module dependency graph and
// reports errors when it encounters dependency cycles.  This should only be
// called after resolveDependencies.
func (c *Context) checkForDependencyCycles() (errs []error) {
	visited := make(map[Module]bool)  // modules that were already checked
	checking := make(map[Module]bool) // modules actively being checked

	var check func(m Module) []Module

	check = func(m Module) []Module {
		info := c.moduleInfo[m]

		visited[m] = true
		checking[m] = true
		defer delete(checking, m)

		for _, dep := range info.directDeps {
			if checking[dep] {
				// This is a cycle.
				return []Module{dep, m}
			}

			if !visited[dep] {
				cycle := check(dep)
				if cycle != nil {
					if cycle[0] == m {
						// We are the "start" of the cycle, so we're responsible
						// for generating the errors.  The cycle list is in
						// reverse order because all the 'check' calls append
						// their own module to the list.
						errs = append(errs, &Error{
							Err: fmt.Errorf("encountered dependency cycle:"),
							Pos: info.pos,
						})

						// Iterate backwards through the cycle list.
						curInfo := info
						for i := len(cycle) - 1; i >= 0; i-- {
							nextInfo := c.moduleInfo[cycle[i]]
							errs = append(errs, &Error{
								Err: fmt.Errorf("    %q depends on %q",
									curInfo.properties.Name,
									nextInfo.properties.Name),
								Pos: curInfo.propertyPos["deps"],
							})
							curInfo = nextInfo
						}

						// We can continue processing this module's children to
						// find more cycles.  Since all the modules that were
						// part of the found cycle were marked as visited we
						// won't run into that cycle again.
					} else {
						// We're not the "start" of the cycle, so we just append
						// our module to the list and return it.
						return append(cycle, m)
					}
				}
			}
		}

		return nil
	}

	for _, module := range c.modules {
		if !visited[module] {
			cycle := check(module)
			if cycle != nil {
				panic("inconceivable!")
			}
		}
	}

	return
}

// PrepareBuildActions generates an internal representation of all the build
// actions that need to be performed.  This process involves invoking the
// GenerateBuildActions method on each of the Module objects created during the
// parse phase and then on each of the registered Singleton objects.
//
// If the ResolveDependencies method has not already been called it is called
// automatically by this method.
//
// The config argument is made available to all of the Module and Singleton
// objects via the Config method on the ModuleContext and SingletonContext
// objects passed to GenerateBuildActions.  It is also passed to the functions
// specified via PoolFunc, RuleFunc, and VariableFunc so that they can compute
// config-specific values.
//
// The returned deps is a list of the ninja files dependencies that were added
// by the modules and singletons via the ModuleContext.AddNinjaFileDeps() and
// SingletonContext.AddNinjaFileDeps() methods.
func (c *Context) PrepareBuildActions(config interface{}) (deps []string, errs []error) {
	c.buildActionsReady = false

	if !c.dependenciesReady {
		errs := c.ResolveDependencies(config)
		if len(errs) > 0 {
			return nil, errs
		}
	}

	liveGlobals := newLiveTracker(config)

	c.initSpecialVariables()

	depsModules, errs := c.generateModuleBuildActions(config, liveGlobals)
	if len(errs) > 0 {
		return nil, errs
	}

	depsSingletons, errs := c.generateSingletonBuildActions(config, liveGlobals)
	if len(errs) > 0 {
		return nil, errs
	}

	deps = append(depsModules, depsSingletons...)

	if c.buildDir != nil {
		liveGlobals.addNinjaStringDeps(c.buildDir)
	}

	pkgNames := c.makeUniquePackageNames(liveGlobals)

	// This will panic if it finds a problem since it's a programming error.
	c.checkForVariableReferenceCycles(liveGlobals.variables, pkgNames)

	c.pkgNames = pkgNames
	c.globalVariables = liveGlobals.variables
	c.globalPools = liveGlobals.pools
	c.globalRules = liveGlobals.rules

	c.buildActionsReady = true

	return deps, nil
}

func (c *Context) initSpecialVariables() {
	c.buildDir = nil
	c.requiredNinjaMajor = 1
	c.requiredNinjaMinor = 1
	c.requiredNinjaMicro = 0
}

func (c *Context) generateModuleBuildActions(config interface{},
	liveGlobals *liveTracker) ([]string, []error) {

	visited := make(map[Module]bool)

	var deps []string
	var errs []error

	var walk func(module Module)
	walk = func(module Module) {
		visited[module] = true

		info := c.moduleInfo[module]
		for _, dep := range info.directDeps {
			if !visited[dep] {
				walk(dep)
				if len(errs) > 0 {
					return
				}
			}
		}

		// The parent scope of the moduleContext's local scope gets overridden to be that of the
		// calling Go package on a per-call basis.  Since the initial parent scope doesn't matter we
		// just set it to nil.
		scope := newLocalScope(nil, moduleNamespacePrefix(info.properties.Name))

		mctx := &moduleContext{
			dynamicDependerModuleContext: dynamicDependerModuleContext{
				context: c,
				config:  config,
				info:    info,
			},
			module: module,
			scope:  scope,
		}

		module.GenerateBuildActions(mctx)

		if len(mctx.errs) > 0 {
			errs = append(errs, mctx.errs...)
			return
		}

		deps = append(deps, mctx.ninjaFileDeps...)

		newErrs := c.processLocalBuildActions(&info.actionDefs,
			&mctx.actionDefs, liveGlobals)
		errs = append(errs, newErrs...)
	}

	for _, module := range c.modules {
		if !visited[module] {
			walk(module)
			if len(errs) > 0 {
				break
			}
		}
	}

	return deps, errs
}

func (c *Context) generateSingletonBuildActions(config interface{},
	liveGlobals *liveTracker) ([]string, []error) {

	var deps []string
	var errs []error

	for name, info := range c.singletonInfo {
		// The parent scope of the singletonContext's local scope gets overridden to be that of the
		// calling Go package on a per-call basis.  Since the initial parent scope doesn't matter we
		// just set it to nil.
		scope := newLocalScope(nil, singletonNamespacePrefix(name))

		sctx := &singletonContext{
			context: c,
			config:  config,
			scope:   scope,
		}

		info.singleton.GenerateBuildActions(sctx)

		if len(sctx.errs) > 0 {
			errs = append(errs, sctx.errs...)
			if len(errs) > maxErrors {
				break
			}
			continue
		}

		deps = append(deps, sctx.ninjaFileDeps...)

		newErrs := c.processLocalBuildActions(&info.actionDefs,
			&sctx.actionDefs, liveGlobals)
		errs = append(errs, newErrs...)
		if len(errs) > maxErrors {
			break
		}
	}

	return deps, errs
}

func (c *Context) processLocalBuildActions(out, in *localBuildActions,
	liveGlobals *liveTracker) []error {

	var errs []error

	// First we go through and add everything referenced by the module's
	// buildDefs to the live globals set.  This will end up adding the live
	// locals to the set as well, but we'll take them out after.
	for _, def := range in.buildDefs {
		err := liveGlobals.AddBuildDefDeps(def)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs
	}

	out.buildDefs = in.buildDefs

	// We use the now-incorrect set of live "globals" to determine which local
	// definitions are live.  As we go through copying those live locals to the
	// moduleInfo we remove them from the live globals set.
	out.variables = nil
	for _, v := range in.variables {
		_, isLive := liveGlobals.variables[v]
		if isLive {
			out.variables = append(out.variables, v)
			delete(liveGlobals.variables, v)
		}
	}

	out.rules = nil
	for _, r := range in.rules {
		_, isLive := liveGlobals.rules[r]
		if isLive {
			out.rules = append(out.rules, r)
			delete(liveGlobals.rules, r)
		}
	}

	return nil
}

func (c *Context) visitDepsDepthFirst(module Module, visit func(Module)) {
	visited := make(map[Module]bool)

	var walk func(m Module)
	walk = func(m Module) {
		info := c.moduleInfo[m]
		visited[m] = true
		for _, dep := range info.directDeps {
			if !visited[dep] {
				walk(dep)
			}
		}
		visit(m)
	}

	info := c.moduleInfo[module]
	for _, dep := range info.directDeps {
		if !visited[dep] {
			walk(dep)
		}
	}
}

func (c *Context) visitDepsDepthFirstIf(module Module, pred func(Module) bool,
	visit func(Module)) {

	visited := make(map[Module]bool)

	var walk func(m Module)
	walk = func(m Module) {
		info := c.moduleInfo[m]
		visited[m] = true
		if pred(m) {
			for _, dep := range info.directDeps {
				if !visited[dep] {
					walk(dep)
				}
			}
			visit(m)
		}
	}

	info := c.moduleInfo[module]
	for _, dep := range info.directDeps {
		if !visited[dep] {
			walk(dep)
		}
	}
}

func (c *Context) sortedModuleNames() []string {
	if c.cachedSortedModuleNames == nil {
		c.cachedSortedModuleNames = make([]string, 0, len(c.modules))
		for moduleName := range c.modules {
			c.cachedSortedModuleNames = append(c.cachedSortedModuleNames,
				moduleName)
		}
		sort.Strings(c.cachedSortedModuleNames)
	}

	return c.cachedSortedModuleNames
}

func (c *Context) visitAllModules(visit func(Module)) {
	for _, moduleName := range c.sortedModuleNames() {
		module := c.modules[moduleName]
		visit(module)
	}
}

func (c *Context) visitAllModulesIf(pred func(Module) bool,
	visit func(Module)) {

	for _, moduleName := range c.sortedModuleNames() {
		module := c.modules[moduleName]
		if pred(module) {
			visit(module)
		}
	}
}

func (c *Context) requireNinjaVersion(major, minor, micro int) {
	if major != 1 {
		panic("ninja version with major version != 1 not supported")
	}
	if c.requiredNinjaMinor < minor {
		c.requiredNinjaMinor = minor
		c.requiredNinjaMicro = micro
	}
	if c.requiredNinjaMinor == minor && c.requiredNinjaMicro < micro {
		c.requiredNinjaMicro = micro
	}
}

func (c *Context) setBuildDir(value *ninjaString) {
	if c.buildDir != nil {
		panic("buildDir set multiple times")
	}
	c.buildDir = value
}

func (c *Context) makeUniquePackageNames(
	liveGlobals *liveTracker) map[*pkg]string {

	pkgs := make(map[string]*pkg)
	pkgNames := make(map[*pkg]string)
	longPkgNames := make(map[*pkg]bool)

	processPackage := func(pkg *pkg) {
		if pkg == nil {
			// This is a built-in rule and has no package.
			return
		}
		if _, ok := pkgNames[pkg]; ok {
			// We've already processed this package.
			return
		}

		otherPkg, present := pkgs[pkg.shortName]
		if present {
			// Short name collision.  Both this package and the one that's
			// already there need to use their full names.  We leave the short
			// name in pkgNames for now so future collisions still get caught.
			longPkgNames[pkg] = true
			longPkgNames[otherPkg] = true
		} else {
			// No collision so far.  Tentatively set the package's name to be
			// its short name.
			pkgNames[pkg] = pkg.shortName
		}
	}

	// We try to give all packages their short name, but when we get collisions
	// we need to use the full unique package name.
	for v, _ := range liveGlobals.variables {
		processPackage(v.pkg())
	}
	for p, _ := range liveGlobals.pools {
		processPackage(p.pkg())
	}
	for r, _ := range liveGlobals.rules {
		processPackage(r.pkg())
	}

	// Add the packages that had collisions using their full unique names.  This
	// will overwrite any short names that were added in the previous step.
	for pkg := range longPkgNames {
		pkgNames[pkg] = pkg.fullName
	}

	return pkgNames
}

func (c *Context) checkForVariableReferenceCycles(
	variables map[Variable]*ninjaString, pkgNames map[*pkg]string) {

	visited := make(map[Variable]bool)  // variables that were already checked
	checking := make(map[Variable]bool) // variables actively being checked

	var check func(v Variable) []Variable

	check = func(v Variable) []Variable {
		visited[v] = true
		checking[v] = true
		defer delete(checking, v)

		value := variables[v]
		for _, dep := range value.variables {
			if checking[dep] {
				// This is a cycle.
				return []Variable{dep, v}
			}

			if !visited[dep] {
				cycle := check(dep)
				if cycle != nil {
					if cycle[0] == v {
						// We are the "start" of the cycle, so we're responsible
						// for generating the errors.  The cycle list is in
						// reverse order because all the 'check' calls append
						// their own module to the list.
						msgs := []string{"detected variable reference cycle:"}

						// Iterate backwards through the cycle list.
						curName := v.fullName(pkgNames)
						curValue := value.Value(pkgNames)
						for i := len(cycle) - 1; i >= 0; i-- {
							next := cycle[i]
							nextName := next.fullName(pkgNames)
							nextValue := variables[next].Value(pkgNames)

							msgs = append(msgs, fmt.Sprintf(
								"    %q depends on %q", curName, nextName))
							msgs = append(msgs, fmt.Sprintf(
								"    [%s = %s]", curName, curValue))

							curName = nextName
							curValue = nextValue
						}

						// Variable reference cycles are a programming error,
						// not the fault of the Blueprint file authors.
						panic(strings.Join(msgs, "\n"))
					} else {
						// We're not the "start" of the cycle, so we just append
						// our module to the list and return it.
						return append(cycle, v)
					}
				}
			}
		}

		return nil
	}

	for v := range variables {
		if !visited[v] {
			cycle := check(v)
			if cycle != nil {
				panic("inconceivable!")
			}
		}
	}
}

// WriteBuildFile writes the Ninja manifeset text for the generated build
// actions to w.  If this is called before PrepareBuildActions successfully
// completes then ErrBuildActionsNotReady is returned.
func (c *Context) WriteBuildFile(w io.Writer) error {
	if !c.buildActionsReady {
		return ErrBuildActionsNotReady
	}

	nw := newNinjaWriter(w)

	err := c.writeBuildFileHeader(nw)
	if err != nil {
		return err
	}

	err = c.writeNinjaRequiredVersion(nw)
	if err != nil {
		return err
	}

	// TODO: Group the globals by package.

	err = c.writeGlobalVariables(nw)
	if err != nil {
		return err
	}

	err = c.writeGlobalPools(nw)
	if err != nil {
		return err
	}

	err = c.writeBuildDir(nw)
	if err != nil {
		return err
	}

	err = c.writeGlobalRules(nw)
	if err != nil {
		return err
	}

	err = c.writeAllModuleActions(nw)
	if err != nil {
		return err
	}

	err = c.writeAllSingletonActions(nw)
	if err != nil {
		return err
	}

	return nil
}

type pkgAssociation struct {
	PkgName string
	PkgPath string
}

type pkgAssociationSorter struct {
	pkgs []pkgAssociation
}

func (s *pkgAssociationSorter) Len() int {
	return len(s.pkgs)
}

func (s *pkgAssociationSorter) Less(i, j int) bool {
	iName := s.pkgs[i].PkgName
	jName := s.pkgs[j].PkgName
	return iName < jName
}

func (s *pkgAssociationSorter) Swap(i, j int) {
	s.pkgs[i], s.pkgs[j] = s.pkgs[j], s.pkgs[i]
}

func (c *Context) writeBuildFileHeader(nw *ninjaWriter) error {
	headerTemplate := template.New("fileHeader")
	_, err := headerTemplate.Parse(fileHeaderTemplate)
	if err != nil {
		// This is a programming error.
		panic(err)
	}

	var pkgs []pkgAssociation
	maxNameLen := 0
	for pkg, name := range c.pkgNames {
		pkgs = append(pkgs, pkgAssociation{
			PkgName: name,
			PkgPath: pkg.pkgPath,
		})
		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
	}

	for i := range pkgs {
		pkgs[i].PkgName += strings.Repeat(" ", maxNameLen-len(pkgs[i].PkgName))
	}

	sort.Sort(&pkgAssociationSorter{pkgs})

	params := map[string]interface{}{
		"Pkgs": pkgs,
	}

	buf := bytes.NewBuffer(nil)
	err = headerTemplate.Execute(buf, params)
	if err != nil {
		return err
	}

	return nw.Comment(buf.String())
}

func (c *Context) writeNinjaRequiredVersion(nw *ninjaWriter) error {
	value := fmt.Sprintf("%d.%d.%d", c.requiredNinjaMajor, c.requiredNinjaMinor,
		c.requiredNinjaMicro)

	err := nw.Assign("ninja_required_version", value)
	if err != nil {
		return err
	}

	return nw.BlankLine()
}

func (c *Context) writeBuildDir(nw *ninjaWriter) error {
	if c.buildDir != nil {
		err := nw.Assign("builddir", c.buildDir.Value(c.pkgNames))
		if err != nil {
			return err
		}

		err = nw.BlankLine()
		if err != nil {
			return err
		}
	}
	return nil
}

type globalEntity interface {
	fullName(pkgNames map[*pkg]string) string
}

type globalEntitySorter struct {
	pkgNames map[*pkg]string
	entities []globalEntity
}

func (s *globalEntitySorter) Len() int {
	return len(s.entities)
}

func (s *globalEntitySorter) Less(i, j int) bool {
	iName := s.entities[i].fullName(s.pkgNames)
	jName := s.entities[j].fullName(s.pkgNames)
	return iName < jName
}

func (s *globalEntitySorter) Swap(i, j int) {
	s.entities[i], s.entities[j] = s.entities[j], s.entities[i]
}

func (c *Context) writeGlobalVariables(nw *ninjaWriter) error {
	visited := make(map[Variable]bool)

	var walk func(v Variable) error
	walk = func(v Variable) error {
		visited[v] = true

		// First visit variables on which this variable depends.
		value := c.globalVariables[v]
		for _, dep := range value.variables {
			if !visited[dep] {
				err := walk(dep)
				if err != nil {
					return err
				}
			}
		}

		err := nw.Assign(v.fullName(c.pkgNames), value.Value(c.pkgNames))
		if err != nil {
			return err
		}

		err = nw.BlankLine()
		if err != nil {
			return err
		}

		return nil
	}

	globalVariables := make([]globalEntity, 0, len(c.globalVariables))
	for variable := range c.globalVariables {
		globalVariables = append(globalVariables, variable)
	}

	sort.Sort(&globalEntitySorter{c.pkgNames, globalVariables})

	for _, entity := range globalVariables {
		v := entity.(Variable)
		if !visited[v] {
			err := walk(v)
			if err != nil {
				return nil
			}
		}
	}

	return nil
}

func (c *Context) writeGlobalPools(nw *ninjaWriter) error {
	globalPools := make([]globalEntity, 0, len(c.globalPools))
	for pool := range c.globalPools {
		globalPools = append(globalPools, pool)
	}

	sort.Sort(&globalEntitySorter{c.pkgNames, globalPools})

	for _, entity := range globalPools {
		pool := entity.(Pool)
		name := pool.fullName(c.pkgNames)
		def := c.globalPools[pool]
		err := def.WriteTo(nw, name)
		if err != nil {
			return err
		}

		err = nw.BlankLine()
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Context) writeGlobalRules(nw *ninjaWriter) error {
	globalRules := make([]globalEntity, 0, len(c.globalRules))
	for rule := range c.globalRules {
		globalRules = append(globalRules, rule)
	}

	sort.Sort(&globalEntitySorter{c.pkgNames, globalRules})

	for _, entity := range globalRules {
		rule := entity.(Rule)
		name := rule.fullName(c.pkgNames)
		def := c.globalRules[rule]
		err := def.WriteTo(nw, name, c.pkgNames)
		if err != nil {
			return err
		}

		err = nw.BlankLine()
		if err != nil {
			return err
		}
	}

	return nil
}

type moduleInfoSorter []*moduleInfo

func (s moduleInfoSorter) Len() int {
	return len(s)
}

func (s moduleInfoSorter) Less(i, j int) bool {
	iName := s[i].properties.Name
	jName := s[j].properties.Name
	return iName < jName
}

func (s moduleInfoSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (c *Context) writeAllModuleActions(nw *ninjaWriter) error {
	headerTemplate := template.New("moduleHeader")
	_, err := headerTemplate.Parse(moduleHeaderTemplate)
	if err != nil {
		// This is a programming error.
		panic(err)
	}

	infos := make([]*moduleInfo, 0, len(c.moduleInfo))
	for _, info := range c.moduleInfo {
		infos = append(infos, info)
	}
	sort.Sort(moduleInfoSorter(infos))

	buf := bytes.NewBuffer(nil)

	for _, info := range infos {
		buf.Reset()

		// In order to make the bootstrap build manifest independent of the
		// build dir we need to output the Blueprints file locations in the
		// comments as paths relative to the source directory.
		relPos := info.pos
		relPos.Filename = info.relBlueprintsFile

		// Get the name and location of the factory function for the module.
		factory := c.moduleFactories[info.typeName]
		factoryFunc := runtime.FuncForPC(reflect.ValueOf(factory).Pointer())
		factoryName := factoryFunc.Name()

		infoMap := map[string]interface{}{
			"properties": info.properties,
			"typeName":   info.typeName,
			"goFactory":  factoryName,
			"pos":        relPos,
		}
		err = headerTemplate.Execute(buf, infoMap)
		if err != nil {
			return err
		}

		err = nw.Comment(buf.String())
		if err != nil {
			return err
		}

		err = nw.BlankLine()
		if err != nil {
			return err
		}

		err = c.writeLocalBuildActions(nw, &info.actionDefs)
		if err != nil {
			return err
		}

		err = nw.BlankLine()
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Context) writeAllSingletonActions(nw *ninjaWriter) error {
	headerTemplate := template.New("singletonHeader")
	_, err := headerTemplate.Parse(singletonHeaderTemplate)
	if err != nil {
		// This is a programming error.
		panic(err)
	}

	buf := bytes.NewBuffer(nil)

	singletonNames := make([]string, 0, len(c.singletonInfo))
	for name := range c.singletonInfo {
		singletonNames = append(singletonNames, name)
	}
	sort.Strings(singletonNames)

	for _, name := range singletonNames {
		info := c.singletonInfo[name]

		// Get the name of the factory function for the module.
		factory := info.factory
		factoryFunc := runtime.FuncForPC(reflect.ValueOf(factory).Pointer())
		factoryName := factoryFunc.Name()

		buf.Reset()
		infoMap := map[string]interface{}{
			"name":      name,
			"goFactory": factoryName,
		}
		err = headerTemplate.Execute(buf, infoMap)
		if err != nil {
			return err
		}

		err = nw.Comment(buf.String())
		if err != nil {
			return err
		}

		err = nw.BlankLine()
		if err != nil {
			return err
		}

		err = c.writeLocalBuildActions(nw, &info.actionDefs)
		if err != nil {
			return err
		}

		err = nw.BlankLine()
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Context) writeLocalBuildActions(nw *ninjaWriter,
	defs *localBuildActions) error {

	// Write the local variable assignments.
	for _, v := range defs.variables {
		// A localVariable doesn't need the package names or config to
		// determine its name or value.
		name := v.fullName(nil)
		value, err := v.value(nil)
		if err != nil {
			panic(err)
		}
		err = nw.Assign(name, value.Value(c.pkgNames))
		if err != nil {
			return err
		}
	}

	if len(defs.variables) > 0 {
		err := nw.BlankLine()
		if err != nil {
			return err
		}
	}

	// Write the local rules.
	for _, r := range defs.rules {
		// A localRule doesn't need the package names or config to determine
		// its name or definition.
		name := r.fullName(nil)
		def, err := r.def(nil)
		if err != nil {
			panic(err)
		}

		err = def.WriteTo(nw, name, c.pkgNames)
		if err != nil {
			return err
		}

		err = nw.BlankLine()
		if err != nil {
			return err
		}
	}

	// Write the build definitions.
	for _, buildDef := range defs.buildDefs {
		err := buildDef.WriteTo(nw, c.pkgNames)
		if err != nil {
			return err
		}

		if len(buildDef.Args) > 0 {
			err = nw.BlankLine()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

var fileHeaderTemplate = `******************************************************************************
***            This file is generated and should not be edited             ***
******************************************************************************
{{if .Pkgs}}
This file contains variables, rules, and pools with name prefixes indicating
they were generated by the following Go packages:
{{range .Pkgs}}
    {{.PkgName}} [from Go package {{.PkgPath}}]{{end}}{{end}}

`

var moduleHeaderTemplate = `# # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # 
Module:  {{.properties.Name}}
Type:    {{.typeName}}
Factory: {{.goFactory}}
Defined: {{.pos}}
`

var singletonHeaderTemplate = `# # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # 
Singleton: {{.name}}
Factory:   {{.goFactory}}
`
