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
	"blueprint/parser"
	"blueprint/proptools"
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
	moduleFactories    map[string]ModuleFactory
	moduleGroups       map[string]*moduleGroup
	moduleInfo         map[Module]*moduleInfo
	moduleGroupsSorted []*moduleGroup
	singletonInfo      map[string]*singletonInfo
	mutatorInfo        []*mutatorInfo

	dependenciesReady bool // set to true on a successful ResolveDependencies
	buildActionsReady bool // set to true on a successful PrepareBuildActions

	// set by SetIgnoreUnknownModuleTypes
	ignoreUnknownModuleTypes bool

	// set during PrepareBuildActions
	pkgNames        map[*PackageContext]string
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

type moduleGroup struct {
	// set during Parse
	typeName          string
	relBlueprintsFile string
	pos               scanner.Position
	propertyPos       map[string]scanner.Position
	properties        struct {
		Name string
		Deps []string
	}

	modules []*moduleInfo

	// set during updateDependencies
	reverseDeps []*moduleGroup
	depsCount   int

	// set during PrepareBuildActions
	actionDefs localBuildActions

	// used by parallelVisitAllBottomUp
	waitingCount int
}

type moduleInfo struct {
	name             []subName
	logicModule      Module
	group            *moduleGroup
	moduleProperties []interface{}

	// set during ResolveDependencies
	directDeps []*moduleInfo

	// set during each runMutator
	splitModules []*moduleInfo
}

type subName struct {
	mutatorName string
	variantName string
}

func (module *moduleInfo) subName() string {
	names := []string{}
	for _, subName := range module.name {
		if subName.variantName != "" {
			names = append(names, subName.variantName)
		}
	}
	return strings.Join(names, "_")
}

type singletonInfo struct {
	// set during RegisterSingletonType
	factory   SingletonFactory
	singleton Singleton

	// set during PrepareBuildActions
	actionDefs localBuildActions
}

type mutatorInfo struct {
	// set during RegisterMutator
	topDownMutator  TopDownMutator
	bottomUpMutator BottomUpMutator
	name            string
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
		moduleGroups:    make(map[string]*moduleGroup),
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
// generation for the module.  If a Mutator splits a module into multiple variants,
// the factory is invoked again to create a new Module for each variant.
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
// The factory function may be called from multiple goroutines.  Any accesses
// to global variables must be synchronized.
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

// RegisterTopDownMutator registers a mutator that will be invoked to propagate
// dependency info top-down between Modules.  Each registered mutator
// is invoked once per Module, and is invoked on a module before being invoked
// on any of its dependencies
//
// The mutator type names given here must be unique for the context.
func (c *Context) RegisterTopDownMutator(name string, mutator TopDownMutator) {
	for _, m := range c.mutatorInfo {
		if m.name == name && m.topDownMutator != nil {
			panic(fmt.Errorf("mutator name %s is already registered", name))
		}
	}

	c.mutatorInfo = append(c.mutatorInfo, &mutatorInfo{
		topDownMutator: mutator,
		name:           name,
	})
}

// RegisterBottomUpMutator registers a mutator that will be invoked to split
// Modules into variants.  Each registered mutator is invoked once per Module,
// and is invoked on dependencies before being invoked on dependers.
//
// The mutator type names given here must be unique for the context.
func (c *Context) RegisterBottomUpMutator(name string, mutator BottomUpMutator) {
	for _, m := range c.mutatorInfo {
		if m.name == name && m.bottomUpMutator != nil {
			panic(fmt.Errorf("mutator name %s is already registered", name))
		}
	}

	c.mutatorInfo = append(c.mutatorInfo, &mutatorInfo{
		bottomUpMutator: mutator,
		name:            name,
	})
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
func (c *Context) parse(rootDir, filename string, r io.Reader,
	scope *parser.Scope) (subdirs []string, modules []*moduleInfo, errs []error,
	outScope *parser.Scope) {

	relBlueprintsFile, err := filepath.Rel(rootDir, filename)
	if err != nil {
		return nil, nil, []error{err}, nil
	}

	scope = parser.NewScope(scope)
	scope.Remove("subdirs")
	file, errs := parser.Parse(filename, r, scope)
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
		return nil, nil, errs, nil
	}

	for _, def := range file.Defs {
		var newErrs []error
		var newModule *moduleInfo
		switch def := def.(type) {
		case *parser.Module:
			newModule, newErrs = c.processModuleDef(def, relBlueprintsFile)

		case *parser.Assignment:
			// Already handled via Scope object
		default:
			panic("unknown definition type")
		}

		if len(newErrs) > 0 {
			errs = append(errs, newErrs...)
			if len(errs) > maxErrors {
				break
			}
		} else if newModule != nil {
			modules = append(modules, newModule)
		}
	}

	subdirs, newErrs := c.processSubdirs(scope)
	if len(newErrs) > 0 {
		errs = append(errs, newErrs...)
	}

	return subdirs, modules, errs, scope
}

type stringAndScope struct {
	string
	*parser.Scope
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

	c.dependenciesReady = false

	rootDir := filepath.Dir(rootFile)

	blueprintsSet := make(map[string]bool)

	// Channels to receive data back from parseBlueprintsFile goroutines
	blueprintsCh := make(chan stringAndScope)
	errsCh := make(chan []error)
	modulesCh := make(chan []*moduleInfo)
	depsCh := make(chan string)

	// Channel to notify main loop that a parseBlueprintsFile goroutine has finished
	doneCh := make(chan struct{})

	// Number of outstanding goroutines to wait for
	count := 0

	startParseBlueprintsFile := func(filename string, scope *parser.Scope) {
		count++
		go func() {
			c.parseBlueprintsFile(filename, scope, rootDir,
				errsCh, modulesCh, blueprintsCh, depsCh)
			doneCh <- struct{}{}
		}()
	}

	tooManyErrors := false

	startParseBlueprintsFile(rootFile, nil)

loop:
	for {
		if len(errs) > maxErrors {
			tooManyErrors = true
		}

		select {
		case newErrs := <-errsCh:
			errs = append(errs, newErrs...)
		case dep := <-depsCh:
			deps = append(deps, dep)
		case modules := <-modulesCh:
			newErrs := c.addModules(modules)
			errs = append(errs, newErrs...)
		case blueprint := <-blueprintsCh:
			if tooManyErrors {
				continue
			}
			if blueprintsSet[blueprint.string] {
				continue
			}

			blueprintsSet[blueprint.string] = true
			startParseBlueprintsFile(blueprint.string, blueprint.Scope)
		case <-doneCh:
			count--
			if count == 0 {
				break loop
			}
		}
	}

	return
}

// parseBlueprintFile parses a single Blueprints file, returning any errors through
// errsCh, any defined modules through modulesCh, any sub-Blueprints files through
// blueprintsCh, and any dependencies on Blueprints files or directories through
// depsCh.
func (c *Context) parseBlueprintsFile(filename string, scope *parser.Scope, rootDir string,
	errsCh chan<- []error, modulesCh chan<- []*moduleInfo, blueprintsCh chan<- stringAndScope,
	depsCh chan<- string) {

	dir := filepath.Dir(filename)

	file, err := os.Open(filename)
	if err != nil {
		errsCh <- []error{err}
		return
	}

	subdirs, modules, errs, subScope := c.parse(rootDir, filename, file, scope)
	if len(errs) > 0 {
		errsCh <- errs
	}

	err = file.Close()
	if err != nil {
		errsCh <- []error{err}
	}

	modulesCh <- modules

	for _, subdir := range subdirs {
		subdir = filepath.Join(dir, subdir)

		dirPart, filePart := filepath.Split(subdir)
		dirPart = filepath.Clean(dirPart)

		if filePart == "*" {
			foundSubdirs, err := listSubdirs(dirPart)
			if err != nil {
				errsCh <- []error{err}
				return
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
					depsCh <- filepath.Dir(subBlueprints)
				} else {
					depsCh <- subBlueprints
					blueprintsCh <- stringAndScope{
						subBlueprints,
						subScope,
					}
				}
			}

			// We now depend on the directory itself because if any new
			// subdirectories get added or removed we need to rebuild the
			// Ninja manifest.
			depsCh <- dirPart
		} else {
			subBlueprints := filepath.Join(subdir, "Blueprints")
			depsCh <- subBlueprints
			blueprintsCh <- stringAndScope{
				subBlueprints,
				subScope,
			}

		}
	}
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
		isDotFile := strings.HasPrefix(info.Name(), ".")
		if info.IsDir() && !isDotFile {
			subdirs = append(subdirs, info.Name())
		}
	}

	return subdirs, nil
}

func (c *Context) processSubdirs(
	scope *parser.Scope) (subdirs []string, errs []error) {

	if assignment, err := scope.Get("subdirs"); err == nil {
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

	return nil, nil
}

func (c *Context) createVariants(origModule *moduleInfo, mutatorName string,
	variantNames []string) ([]*moduleInfo, []error) {

	newModules := []*moduleInfo{}
	origVariantName := origModule.name
	group := origModule.group

	var errs []error

	for i, variantName := range variantNames {
		typeName := group.typeName
		factory, ok := c.moduleFactories[typeName]
		if !ok {
			panic(fmt.Sprintf("unrecognized module type %q during cloning", typeName))
		}

		var newLogicModule Module
		var newProperties []interface{}

		if i == 0 {
			// Reuse the existing module for the first new variant
			newLogicModule = origModule.logicModule
			newProperties = origModule.moduleProperties
		} else {
			props := []interface{}{
				&group.properties,
			}
			newLogicModule, newProperties = factory()

			newProperties = append(props, newProperties...)

			if len(newProperties) != len(origModule.moduleProperties) {
				panic("mismatched properties array length in " + group.properties.Name)
			}

			for i := range newProperties {
				dst := reflect.ValueOf(newProperties[i]).Elem()
				src := reflect.ValueOf(origModule.moduleProperties[i]).Elem()

				proptools.CopyProperties(dst, src)
			}
		}

		newVariantName := append([]subName(nil), origVariantName...)
		newSubName := subName{
			mutatorName: mutatorName,
			variantName: variantName,
		}
		newVariantName = append(newVariantName, newSubName)

		newModule := &moduleInfo{
			group:            group,
			directDeps:       append([]*moduleInfo(nil), origModule.directDeps...),
			logicModule:      newLogicModule,
			name:             newVariantName,
			moduleProperties: newProperties,
		}

		newModules = append(newModules, newModule)
		c.moduleInfo[newModule.logicModule] = newModule

		newErrs := c.convertDepsToVariant(newModule, newSubName)
		if len(newErrs) > 0 {
			errs = append(errs, newErrs...)
		}
	}

	// Mark original variant as invalid.  Modules that depend on this module will still
	// depend on origModule, but we'll fix it when the mutator is called on them.
	origModule.logicModule = nil
	origModule.splitModules = newModules

	return newModules, errs
}

func (c *Context) convertDepsToVariant(module *moduleInfo, newSubName subName) (errs []error) {

	for i, dep := range module.directDeps {
		if dep.logicModule == nil {
			var newDep *moduleInfo
			for _, m := range dep.splitModules {
				if len(m.name) > 0 && m.name[len(m.name)-1] == newSubName {
					newDep = m
					break
				}
			}
			if newDep == nil {
				errs = append(errs, &Error{
					Err: fmt.Errorf("failed to find variant %q for module %q needed by %q",
						newSubName.variantName, dep.group.properties.Name,
						module.group.properties.Name),
					Pos: module.group.pos,
				})
				continue
			}
			module.directDeps[i] = newDep
		}
	}

	return errs
}

func (c *Context) processModuleDef(moduleDef *parser.Module,
	relBlueprintsFile string) (*moduleInfo, []error) {

	typeName := moduleDef.Type.Name
	factory, ok := c.moduleFactories[typeName]
	if !ok {
		if c.ignoreUnknownModuleTypes {
			return nil, nil
		}

		return nil, []error{
			&Error{
				Err: fmt.Errorf("unrecognized module type %q", typeName),
				Pos: moduleDef.Type.Pos,
			},
		}
	}

	logicModule, properties := factory()
	group := &moduleGroup{
		typeName:          typeName,
		relBlueprintsFile: relBlueprintsFile,
	}

	props := []interface{}{
		&group.properties,
	}
	properties = append(props, properties...)

	propertyMap, errs := unpackProperties(moduleDef.Properties, properties...)
	if len(errs) > 0 {
		return nil, errs
	}

	group.pos = moduleDef.Type.Pos
	group.propertyPos = make(map[string]scanner.Position)
	for name, propertyDef := range propertyMap {
		group.propertyPos[name] = propertyDef.Pos
	}

	name := group.properties.Name
	err := validateNinjaName(name)
	if err != nil {
		return nil, []error{
			&Error{
				Err: fmt.Errorf("invalid module name %q: %s", err),
				Pos: group.propertyPos["name"],
			},
		}
	}

	module := &moduleInfo{
		group:            group,
		logicModule:      logicModule,
		moduleProperties: properties,
	}
	group.modules = []*moduleInfo{module}

	return module, nil
}

func (c *Context) addModules(modules []*moduleInfo) (errs []error) {
	for _, module := range modules {
		name := module.group.properties.Name
		if first, present := c.moduleGroups[name]; present {
			errs = append(errs, []error{
				&Error{
					Err: fmt.Errorf("module %q already defined", name),
					Pos: module.group.pos,
				},
				&Error{
					Err: fmt.Errorf("<-- previous definition here"),
					Pos: first.pos,
				},
			}...)
			continue
		}

		c.moduleGroups[name] = module.group
		c.moduleInfo[module.logicModule] = module
	}

	return errs
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

	errs = c.updateDependencies()
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
func (c *Context) moduleDepNames(group *moduleGroup,
	config interface{}) ([]string, []error) {

	depNamesSet := make(map[string]bool)
	depNames := []string{}

	for _, depName := range group.properties.Deps {
		if !depNamesSet[depName] {
			depNamesSet[depName] = true
			depNames = append(depNames, depName)
		}
	}

	if len(group.modules) != 1 {
		panic("expected a single module during moduleDepNames")
	}
	logicModule := group.modules[0].logicModule
	dynamicDepender, ok := logicModule.(DynamicDependerModule)
	if ok {
		ddmctx := &baseModuleContext{
			context: c,
			config:  config,
			group:   group,
		}

		dynamicDeps := dynamicDepender.DynamicDependencies(ddmctx)

		if len(ddmctx.errs) > 0 {
			return nil, ddmctx.errs
		}

		for _, depName := range dynamicDeps {
			if !depNamesSet[depName] {
				depNamesSet[depName] = true
				depNames = append(depNames, depName)
			}
		}
	}

	return depNames, nil
}

// resolveDependencies populates the moduleGroup.modules[0].directDeps list for every
// module.  In doing so it checks for missing dependencies and self-dependant
// modules.
func (c *Context) resolveDependencies(config interface{}) (errs []error) {
	for _, group := range c.moduleGroups {
		depNames, newErrs := c.moduleDepNames(group, config)
		if len(newErrs) > 0 {
			errs = append(errs, newErrs...)
			continue
		}

		if len(group.modules) != 1 {
			panic("expected a single module in resolveDependencies")
		}
		group.modules[0].directDeps = make([]*moduleInfo, 0, len(depNames))

		for _, depName := range depNames {
			newErrs := c.addDependency(group.modules[0], depName)
			if len(newErrs) > 0 {
				errs = append(errs, newErrs...)
				continue
			}
		}
	}

	return
}

func (c *Context) addDependency(module *moduleInfo, depName string) []error {
	depsPos := module.group.propertyPos["deps"]

	if depName == module.group.properties.Name {
		return []error{&Error{
			Err: fmt.Errorf("%q depends on itself", depName),
			Pos: depsPos,
		}}
	}

	depInfo, ok := c.moduleGroups[depName]
	if !ok {
		return []error{&Error{
			Err: fmt.Errorf("%q depends on undefined module %q",
				module.group.properties.Name, depName),
			Pos: depsPos,
		}}
	}

	if len(depInfo.modules) != 1 {
		panic(fmt.Sprintf("cannot add dependency from %s to %s, it already has multiple variants",
			module.group.properties.Name, depInfo.properties.Name))
	}

	module.directDeps = append(module.directDeps, depInfo.modules[0])

	return nil
}

func (c *Context) parallelVisitAllBottomUp(visit func(group *moduleGroup)) {
	doneCh := make(chan *moduleGroup)
	count := 0

	for _, group := range c.moduleGroupsSorted {
		group.waitingCount = group.depsCount
	}

	visitOne := func(group *moduleGroup) {
		count++
		go func() {
			visit(group)
			doneCh <- group
		}()
	}

	for _, group := range c.moduleGroupsSorted {
		if group.waitingCount == 0 {
			visitOne(group)
		}
	}

loop:
	for {
		select {
		case doneGroup := <-doneCh:
			for _, parent := range doneGroup.reverseDeps {
				parent.waitingCount--
				if parent.waitingCount == 0 {
					visitOne(parent)
				}
			}
			count--
			if count == 0 {
				break loop
			}
		}
	}
}

// updateDependencies recursively walks the module dependency graph and updates
// additional fields based on the dependencies.  It builds a sorted list of modules
// such that dependencies of a module always appear first, and populates reverse
// dependency links and counts of total dependencies.  It also reports errors when
// it encounters dependency cycles.  This should called after resolveDependencies,
// as well as after any mutator pass has called addDependency
func (c *Context) updateDependencies() (errs []error) {
	visited := make(map[*moduleGroup]bool)  // modules that were already checked
	checking := make(map[*moduleGroup]bool) // modules actively being checked

	sorted := make([]*moduleGroup, 0, len(c.moduleGroups))

	var check func(group *moduleGroup) []*moduleGroup

	check = func(group *moduleGroup) []*moduleGroup {
		visited[group] = true
		checking[group] = true
		defer delete(checking, group)

		deps := make(map[*moduleGroup]bool)
		for _, module := range group.modules {
			for _, dep := range module.directDeps {
				deps[dep.group] = true
			}
		}

		group.reverseDeps = []*moduleGroup{}
		group.depsCount = len(deps)

		for dep := range deps {
			if checking[dep] {
				// This is a cycle.
				return []*moduleGroup{dep, group}
			}

			if !visited[dep] {
				cycle := check(dep)
				if cycle != nil {
					if cycle[0] == group {
						// We are the "start" of the cycle, so we're responsible
						// for generating the errors.  The cycle list is in
						// reverse order because all the 'check' calls append
						// their own module to the list.
						errs = append(errs, &Error{
							Err: fmt.Errorf("encountered dependency cycle:"),
							Pos: group.pos,
						})

						// Iterate backwards through the cycle list.
						curGroup := group
						for i := len(cycle) - 1; i >= 0; i-- {
							nextGroup := cycle[i]
							errs = append(errs, &Error{
								Err: fmt.Errorf("    %q depends on %q",
									curGroup.properties.Name,
									nextGroup.properties.Name),
								Pos: curGroup.propertyPos["deps"],
							})
							curGroup = nextGroup
						}

						// We can continue processing this module's children to
						// find more cycles.  Since all the modules that were
						// part of the found cycle were marked as visited we
						// won't run into that cycle again.
					} else {
						// We're not the "start" of the cycle, so we just append
						// our module to the list and return it.
						return append(cycle, group)
					}
				}
			}

			dep.reverseDeps = append(dep.reverseDeps, group)
		}

		sorted = append(sorted, group)

		return nil
	}

	for _, group := range c.moduleGroups {
		if !visited[group] {
			cycle := check(group)
			if cycle != nil {
				panic("inconceivable!")
			}
		}
	}

	c.moduleGroupsSorted = sorted

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

	errs = c.runMutators(config)
	if len(errs) > 0 {
		return nil, errs
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

func (c *Context) runMutators(config interface{}) (errs []error) {
	for _, mutator := range c.mutatorInfo {
		if mutator.topDownMutator != nil {
			errs = c.runTopDownMutator(config, mutator.name, mutator.topDownMutator)
		} else if mutator.bottomUpMutator != nil {
			errs = c.runBottomUpMutator(config, mutator.name, mutator.bottomUpMutator)
		} else {
			panic("no mutator set on " + mutator.name)
		}
		if len(errs) > 0 {
			return errs
		}
	}

	return nil
}

func (c *Context) runTopDownMutator(config interface{},
	name string, mutator TopDownMutator) (errs []error) {

	for i := 0; i < len(c.moduleGroupsSorted); i++ {
		group := c.moduleGroupsSorted[len(c.moduleGroupsSorted)-1-i]
		for _, module := range group.modules {
			mctx := &mutatorContext{
				baseModuleContext: baseModuleContext{
					context: c,
					config:  config,
					group:   group,
				},
				module: module,
				name:   name,
			}

			mutator(mctx)
			if len(mctx.errs) > 0 {
				errs = append(errs, mctx.errs...)
				return errs
			}
		}
	}

	return errs
}

func (c *Context) runBottomUpMutator(config interface{},
	name string, mutator BottomUpMutator) (errs []error) {

	dependenciesModified := false

	for _, group := range c.moduleGroupsSorted {
		newModules := make([]*moduleInfo, 0, len(group.modules))

		for _, module := range group.modules {
			mctx := &mutatorContext{
				baseModuleContext: baseModuleContext{
					context: c,
					config:  config,
					group:   group,
				},
				module: module,
				name:   name,
			}

			mutator(mctx)
			if len(mctx.errs) > 0 {
				errs = append(errs, mctx.errs...)
				return errs
			}

			// Fix up any remaining dependencies on modules that were split into variants
			// by replacing them with the first variant
			for i, dep := range module.directDeps {
				if dep.logicModule == nil {
					module.directDeps[i] = dep.splitModules[0]
				}
			}

			if mctx.dependenciesModified {
				dependenciesModified = true
			}

			if module.splitModules != nil {
				newModules = append(newModules, module.splitModules...)
			} else {
				newModules = append(newModules, module)
			}
		}

		group.modules = newModules
	}

	if dependenciesModified {
		errs = c.updateDependencies()
		if len(errs) > 0 {
			return errs
		}
	}

	return errs
}

func (c *Context) initSpecialVariables() {
	c.buildDir = nil
	c.requiredNinjaMajor = 1
	c.requiredNinjaMinor = 1
	c.requiredNinjaMicro = 0
}

func (c *Context) generateModuleBuildActions(config interface{},
	liveGlobals *liveTracker) ([]string, []error) {

	var deps []string
	var errs []error

	cancelCh := make(chan struct{})
	errsCh := make(chan []error)
	depsCh := make(chan []string)

	go func() {
		for {
			select {
			case <-cancelCh:
				close(cancelCh)
				return
			case newErrs := <-errsCh:
				errs = append(errs, newErrs...)
			case newDeps := <-depsCh:
				deps = append(deps, newDeps...)

			}
		}
	}()

	c.parallelVisitAllBottomUp(func(group *moduleGroup) {
		// The parent scope of the moduleContext's local scope gets overridden to be that of the
		// calling Go package on a per-call basis.  Since the initial parent scope doesn't matter we
		// just set it to nil.
		scope := newLocalScope(nil, moduleNamespacePrefix(group.properties.Name))

		for _, module := range group.modules {
			mctx := &moduleContext{
				baseModuleContext: baseModuleContext{
					context: c,
					config:  config,
					group:   group,
				},
				module: module,
				scope:  scope,
			}

			mctx.module.logicModule.GenerateBuildActions(mctx)

			if len(mctx.errs) > 0 {
				errsCh <- mctx.errs
				break
			}

			depsCh <- mctx.ninjaFileDeps

			newErrs := c.processLocalBuildActions(&group.actionDefs,
				&mctx.actionDefs, liveGlobals)
			if len(newErrs) > 0 {
				errsCh <- newErrs
				break
			}
		}
	})

	cancelCh <- struct{}{}
	<-cancelCh

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

	out.buildDefs = append(out.buildDefs, in.buildDefs...)

	// We use the now-incorrect set of live "globals" to determine which local
	// definitions are live.  As we go through copying those live locals to the
	// moduleGroup we remove them from the live globals set.
	for _, v := range in.variables {
		_, isLive := liveGlobals.variables[v]
		if isLive {
			out.variables = append(out.variables, v)
			delete(liveGlobals.variables, v)
		}
	}

	for _, r := range in.rules {
		_, isLive := liveGlobals.rules[r]
		if isLive {
			out.rules = append(out.rules, r)
			delete(liveGlobals.rules, r)
		}
	}

	return nil
}

func (c *Context) visitDepsDepthFirst(topModule *moduleInfo, visit func(Module)) {
	visited := make(map[*moduleInfo]bool)

	var walk func(module *moduleInfo)
	walk = func(module *moduleInfo) {
		visited[module] = true
		for _, moduleDep := range module.directDeps {
			if !visited[moduleDep] {
				walk(moduleDep)
			}
		}

		if module != topModule {
			visit(module.logicModule)
		}
	}

	walk(topModule)
}

func (c *Context) visitDepsDepthFirstIf(topModule *moduleInfo, pred func(Module) bool,
	visit func(Module)) {

	visited := make(map[*moduleInfo]bool)

	var walk func(module *moduleInfo)
	walk = func(module *moduleInfo) {
		visited[module] = true
		for _, moduleDep := range module.directDeps {
			if !visited[moduleDep] {
				walk(moduleDep)
			}
		}

		if module != topModule {
			if pred(module.logicModule) {
				visit(module.logicModule)
			}
		}
	}

	walk(topModule)
}

func (c *Context) visitDirectDeps(module *moduleInfo, visit func(Module)) {
	for _, dep := range module.directDeps {
		visit(dep.logicModule)
	}
}

func (c *Context) visitDirectDepsIf(module *moduleInfo, pred func(Module) bool,
	visit func(Module)) {

	for _, dep := range module.directDeps {
		if pred(dep.logicModule) {
			visit(dep.logicModule)
		}
	}
}

func (c *Context) sortedModuleNames() []string {
	if c.cachedSortedModuleNames == nil {
		c.cachedSortedModuleNames = make([]string, 0, len(c.moduleGroups))
		for moduleName := range c.moduleGroups {
			c.cachedSortedModuleNames = append(c.cachedSortedModuleNames,
				moduleName)
		}
		sort.Strings(c.cachedSortedModuleNames)
	}

	return c.cachedSortedModuleNames
}

func (c *Context) visitAllModules(visit func(Module)) {
	for _, moduleName := range c.sortedModuleNames() {
		group := c.moduleGroups[moduleName]
		for _, module := range group.modules {
			visit(module.logicModule)
		}
	}
}

func (c *Context) visitAllModulesIf(pred func(Module) bool,
	visit func(Module)) {

	for _, moduleName := range c.sortedModuleNames() {
		group := c.moduleGroups[moduleName]
		for _, module := range group.modules {
			if pred(module.logicModule) {
				visit(module.logicModule)
			}
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
	liveGlobals *liveTracker) map[*PackageContext]string {

	pkgs := make(map[string]*PackageContext)
	pkgNames := make(map[*PackageContext]string)
	longPkgNames := make(map[*PackageContext]bool)

	processPackage := func(pctx *PackageContext) {
		if pctx == nil {
			// This is a built-in rule and has no package.
			return
		}
		if _, ok := pkgNames[pctx]; ok {
			// We've already processed this package.
			return
		}

		otherPkg, present := pkgs[pctx.shortName]
		if present {
			// Short name collision.  Both this package and the one that's
			// already there need to use their full names.  We leave the short
			// name in pkgNames for now so future collisions still get caught.
			longPkgNames[pctx] = true
			longPkgNames[otherPkg] = true
		} else {
			// No collision so far.  Tentatively set the package's name to be
			// its short name.
			pkgNames[pctx] = pctx.shortName
		}
	}

	// We try to give all packages their short name, but when we get collisions
	// we need to use the full unique package name.
	for v, _ := range liveGlobals.variables {
		processPackage(v.packageContext())
	}
	for p, _ := range liveGlobals.pools {
		processPackage(p.packageContext())
	}
	for r, _ := range liveGlobals.rules {
		processPackage(r.packageContext())
	}

	// Add the packages that had collisions using their full unique names.  This
	// will overwrite any short names that were added in the previous step.
	for pctx := range longPkgNames {
		pkgNames[pctx] = pctx.fullName
	}

	return pkgNames
}

func (c *Context) checkForVariableReferenceCycles(
	variables map[Variable]*ninjaString, pkgNames map[*PackageContext]string) {

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

// AllTargets returns a map all the build target names to the rule used to build
// them.  This is the same information that is output by running 'ninja -t
// targets all'.  If this is called before PrepareBuildActions successfully
// completes then ErrbuildActionsNotReady is returned.
func (c *Context) AllTargets() (map[string]string, error) {
	if !c.buildActionsReady {
		return nil, ErrBuildActionsNotReady
	}

	targets := map[string]string{}

	// Collect all the module build targets.
	for _, info := range c.moduleGroups {
		for _, buildDef := range info.actionDefs.buildDefs {
			ruleName := buildDef.Rule.fullName(c.pkgNames)
			for _, output := range buildDef.Outputs {
				outputValue, err := output.Eval(c.globalVariables)
				if err != nil {
					return nil, err
				}
				targets[outputValue] = ruleName
			}
		}
	}

	// Collect all the singleton build targets.
	for _, info := range c.singletonInfo {
		for _, buildDef := range info.actionDefs.buildDefs {
			ruleName := buildDef.Rule.fullName(c.pkgNames)
			for _, output := range buildDef.Outputs {
				outputValue, err := output.Eval(c.globalVariables)
				if err != nil {
					return nil, err
				}
				targets[outputValue] = ruleName
			}
		}
	}

	return targets, nil
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
	fullName(pkgNames map[*PackageContext]string) string
}

type globalEntitySorter struct {
	pkgNames map[*PackageContext]string
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

type moduleGroupSorter []*moduleGroup

func (s moduleGroupSorter) Len() int {
	return len(s)
}

func (s moduleGroupSorter) Less(i, j int) bool {
	iName := s[i].properties.Name
	jName := s[j].properties.Name
	return iName < jName
}

func (s moduleGroupSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (c *Context) writeAllModuleActions(nw *ninjaWriter) error {
	headerTemplate := template.New("moduleHeader")
	_, err := headerTemplate.Parse(moduleHeaderTemplate)
	if err != nil {
		// This is a programming error.
		panic(err)
	}

	infos := make([]*moduleGroup, 0, len(c.moduleGroups))
	for _, info := range c.moduleGroups {
		infos = append(infos, info)
	}
	sort.Sort(moduleGroupSorter(infos))

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
