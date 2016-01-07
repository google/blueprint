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
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"text/scanner"
	"text/template"

	"github.com/google/blueprint/parser"
	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"
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
	moduleFactories     map[string]ModuleFactory
	moduleGroups        map[string]*moduleGroup
	moduleInfo          map[Module]*moduleInfo
	modulesSorted       []*moduleInfo
	singletonInfo       []*singletonInfo
	mutatorInfo         []*mutatorInfo
	earlyMutatorInfo    []*earlyMutatorInfo
	variantMutatorNames []string
	moduleNinjaNames    map[string]*moduleGroup

	dependenciesReady bool // set to true on a successful ResolveDependencies
	buildActionsReady bool // set to true on a successful PrepareBuildActions

	// set by SetIgnoreUnknownModuleTypes
	ignoreUnknownModuleTypes bool

	// set by SetAllowMissingDependencies
	allowMissingDependencies bool

	// set during PrepareBuildActions
	pkgNames        map[*packageContext]string
	globalVariables map[Variable]*ninjaString
	globalPools     map[Pool]*poolDef
	globalRules     map[Rule]*ruleDef

	// set during PrepareBuildActions
	ninjaBuildDir      *ninjaString // The builddir special Ninja variable
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
	name      string
	ninjaName string

	modules []*moduleInfo
}

type moduleInfo struct {
	// set during Parse
	typeName          string
	relBlueprintsFile string
	pos               scanner.Position
	propertyPos       map[string]scanner.Position
	properties        struct {
		Name string
		Deps []string
	}

	variantName       string
	variant           variationMap
	dependencyVariant variationMap

	logicModule      Module
	group            *moduleGroup
	moduleProperties []interface{}

	// set during ResolveDependencies
	directDeps  []*moduleInfo
	missingDeps []string

	// set during updateDependencies
	reverseDeps []*moduleInfo
	depsCount   int

	// used by parallelVisitAllBottomUp
	waitingCount int

	// set during each runMutator
	splitModules []*moduleInfo

	// set during PrepareBuildActions
	actionDefs localBuildActions
}

func (module *moduleInfo) String() string {
	s := fmt.Sprintf("module %q", module.properties.Name)
	if module.variantName != "" {
		s += fmt.Sprintf(" variant %q", module.variantName)
	}
	return s
}

// A Variation is a way that a variant of a module differs from other variants of the same module.
// For example, two variants of the same module might have Variation{"arch","arm"} and
// Variation{"arch","arm64"}
type Variation struct {
	// Mutator is the axis on which this variation applies, i.e. "arch" or "link"
	Mutator string
	// Variation is the name of the variation on the axis, i.e. "arm" or "arm64" for arch, or
	// "shared" or "static" for link.
	Variation string
}

// A variationMap stores a map of Mutator to Variation to specify a variant of a module.
type variationMap map[string]string

func (vm variationMap) clone() variationMap {
	newVm := make(variationMap)
	for k, v := range vm {
		newVm[k] = v
	}

	return newVm
}

// Compare this variationMap to another one.  Returns true if the every entry in this map
// is either the same in the other map or doesn't exist in the other map.
func (vm variationMap) subset(other variationMap) bool {
	for k, v1 := range vm {
		if v2, ok := other[k]; ok && v1 != v2 {
			return false
		}
	}
	return true
}

func (vm variationMap) equal(other variationMap) bool {
	return reflect.DeepEqual(vm, other)
}

type singletonInfo struct {
	// set during RegisterSingletonType
	factory   SingletonFactory
	singleton Singleton
	name      string

	// set during PrepareBuildActions
	actionDefs localBuildActions
}

type mutatorInfo struct {
	// set during RegisterMutator
	topDownMutator  TopDownMutator
	bottomUpMutator BottomUpMutator
	name            string
}

type earlyMutatorInfo struct {
	// set during RegisterEarlyMutator
	mutator EarlyMutator
	name    string
}

func (e *Error) Error() string {

	return fmt.Sprintf("%s: %s", e.Pos, e.Err)
}

// NewContext creates a new Context object.  The created context initially has
// no module or singleton factories registered, so the RegisterModuleFactory and
// RegisterSingletonFactory methods must be called before it can do anything
// useful.
func NewContext() *Context {
	ctx := &Context{
		moduleFactories:  make(map[string]ModuleFactory),
		moduleGroups:     make(map[string]*moduleGroup),
		moduleInfo:       make(map[Module]*moduleInfo),
		moduleNinjaNames: make(map[string]*moduleGroup),
	}

	ctx.RegisterBottomUpMutator("blueprint_deps", blueprintDepsMutator)

	return ctx
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
// and invoked exactly once as part of the generate phase.  Each registered
// singleton is invoked in registration order.
//
// The singleton type names given here must be unique for the context.  The
// factory function should be a named function so that its package and name can
// be included in the generated Ninja file for debugging purposes.
func (c *Context) RegisterSingletonType(name string, factory SingletonFactory) {
	for _, s := range c.singletonInfo {
		if s.name == name {
			panic(errors.New("singleton name is already registered"))
		}
	}

	c.singletonInfo = append(c.singletonInfo, &singletonInfo{
		factory:   factory,
		singleton: factory(),
		name:      name,
	})
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
// is invoked in registration order (mixing TopDownMutators and BottomUpMutators)
// once per Module, and is invoked on a module before being invoked on any of its
// dependencies.
//
// The mutator type names given here must be unique to all top down mutators in
// the Context.
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
// Modules into variants.  Each registered mutator is invoked in registration
// order (mixing TopDownMutators and BottomUpMutators) once per Module, and is
// invoked on dependencies before being invoked on dependers.
//
// The mutator type names given here must be unique to all bottom up or early
// mutators in the Context.
func (c *Context) RegisterBottomUpMutator(name string, mutator BottomUpMutator) {
	for _, m := range c.variantMutatorNames {
		if m == name {
			panic(fmt.Errorf("mutator name %s is already registered", name))
		}
	}

	c.mutatorInfo = append(c.mutatorInfo, &mutatorInfo{
		bottomUpMutator: mutator,
		name:            name,
	})

	c.variantMutatorNames = append(c.variantMutatorNames, name)
}

// RegisterEarlyMutator registers a mutator that will be invoked to split
// Modules into multiple variant Modules before any dependencies have been
// created.  Each registered mutator is invoked in registration order once
// per Module (including each variant from previous early mutators).  Module
// order is unpredictable.
//
// In order for dependencies to be satisifed in a later pass, all dependencies
// of a module either must have an identical variant or must have no variations.
//
// The mutator type names given here must be unique to all bottom up or early
// mutators in the Context.
//
// Deprecated, use a BottomUpMutator instead.  The only difference between
// EarlyMutator and BottomUpMutator is that EarlyMutator runs before the
// deprecated DynamicDependencies.
func (c *Context) RegisterEarlyMutator(name string, mutator EarlyMutator) {
	for _, m := range c.variantMutatorNames {
		if m == name {
			panic(fmt.Errorf("mutator name %s is already registered", name))
		}
	}

	c.earlyMutatorInfo = append(c.earlyMutatorInfo, &earlyMutatorInfo{
		mutator: mutator,
		name:    name,
	})

	c.variantMutatorNames = append(c.variantMutatorNames, name)
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

// SetAllowMissingDependencies changes the behavior of Blueprint to ignore
// unresolved dependencies.  If the module's GenerateBuildActions calls
// ModuleContext.GetMissingDependencies Blueprint will not emit any errors
// for missing dependencies.
func (c *Context) SetAllowMissingDependencies(allowMissingDependencies bool) {
	c.allowMissingDependencies = allowMissingDependencies
}

// Parse parses a single Blueprints file from r, creating Module objects for
// each of the module definitions encountered.  If the Blueprints file contains
// an assignment to the "subdirs" variable, then the subdirectories listed are
// searched for Blueprints files returned in the subBlueprints return value.
// If the Blueprints file contains an assignment to the "build" variable, then
// the file listed are returned in the subBlueprints return value.
//
// rootDir specifies the path to the root directory of the source tree, while
// filename specifies the path to the Blueprints file.  These paths are used for
// error reporting and for determining the module's directory.
func (c *Context) parse(rootDir, filename string, r io.Reader,
	scope *parser.Scope) (file *parser.File, subBlueprints []stringAndScope, deps []string,
	errs []error) {

	relBlueprintsFile, err := filepath.Rel(rootDir, filename)
	if err != nil {
		return nil, nil, nil, []error{err}
	}

	scope = parser.NewScope(scope)
	scope.Remove("subdirs")
	scope.Remove("optional_subdirs")
	scope.Remove("build")
	file, errs = parser.ParseAndEval(filename, r, scope)
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
		return nil, nil, nil, errs
	}
	file.Name = relBlueprintsFile

	subdirs, subdirsPos, err := getLocalStringListFromScope(scope, "subdirs")
	if err != nil {
		errs = append(errs, err)
	}

	optionalSubdirs, optionalSubdirsPos, err := getLocalStringListFromScope(scope, "optional_subdirs")
	if err != nil {
		errs = append(errs, err)
	}

	build, buildPos, err := getLocalStringListFromScope(scope, "build")
	if err != nil {
		errs = append(errs, err)
	}

	subBlueprintsName, _, err := getStringFromScope(scope, "subname")

	var blueprints []string

	newBlueprints, newDeps, newErrs := c.findBuildBlueprints(filepath.Dir(filename), build, buildPos)
	blueprints = append(blueprints, newBlueprints...)
	deps = append(deps, newDeps...)
	errs = append(errs, newErrs...)

	newBlueprints, newDeps, newErrs = c.findSubdirBlueprints(filepath.Dir(filename), subdirs, subdirsPos,
		subBlueprintsName, false)
	blueprints = append(blueprints, newBlueprints...)
	deps = append(deps, newDeps...)
	errs = append(errs, newErrs...)

	newBlueprints, newDeps, newErrs = c.findSubdirBlueprints(filepath.Dir(filename), optionalSubdirs,
		optionalSubdirsPos, subBlueprintsName, true)
	blueprints = append(blueprints, newBlueprints...)
	deps = append(deps, newDeps...)
	errs = append(errs, newErrs...)

	subBlueprintsAndScope := make([]stringAndScope, len(blueprints))
	for i, b := range blueprints {
		subBlueprintsAndScope[i] = stringAndScope{b, scope}
	}

	return file, subBlueprintsAndScope, deps, errs
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

	moduleCh := make(chan *moduleInfo)
	errsCh := make(chan []error)
	doneCh := make(chan struct{})
	var numErrs uint32
	var numGoroutines int32

	// handler must be reentrant
	handler := func(file *parser.File) {
		if atomic.LoadUint32(&numErrs) > maxErrors {
			return
		}

		atomic.AddInt32(&numGoroutines, 1)
		go func() {
			for _, def := range file.Defs {
				var module *moduleInfo
				var errs []error
				switch def := def.(type) {
				case *parser.Module:
					module, errs = c.processModuleDef(def, file.Name)
				case *parser.Assignment:
					// Already handled via Scope object
				default:
					panic("unknown definition type")
				}

				if len(errs) > 0 {
					atomic.AddUint32(&numErrs, uint32(len(errs)))
					errsCh <- errs
				} else if module != nil {
					moduleCh <- module
				}
			}
			doneCh <- struct{}{}
		}()
	}

	atomic.AddInt32(&numGoroutines, 1)
	go func() {
		var errs []error
		deps, errs = c.WalkBlueprintsFiles(rootFile, handler)
		if len(errs) > 0 {
			errsCh <- errs
		}
		doneCh <- struct{}{}
	}()

loop:
	for {
		select {
		case newErrs := <-errsCh:
			errs = append(errs, newErrs...)
		case module := <-moduleCh:
			newErrs := c.addModule(module)
			if len(newErrs) > 0 {
				errs = append(errs, newErrs...)
			}
		case <-doneCh:
			n := atomic.AddInt32(&numGoroutines, -1)
			if n == 0 {
				break loop
			}
		}
	}

	return deps, errs
}

type FileHandler func(*parser.File)

// Walk a set of Blueprints files starting with the file at rootFile, calling handler on each.
// When it encounters a Blueprints file with a set of subdirs listed it recursively parses any
// Blueprints files found in those subdirectories.  handler will be called from a goroutine, so
// it must be reentrant.
//
// If no errors are encountered while parsing the files, the list of paths on
// which the future output will depend is returned.  This list will include both
// Blueprints file paths as well as directory paths for cases where wildcard
// subdirs are found.
func (c *Context) WalkBlueprintsFiles(rootFile string, handler FileHandler) (deps []string,
	errs []error) {

	rootDir := filepath.Dir(rootFile)

	blueprintsSet := make(map[string]bool)

	// Channels to receive data back from parseBlueprintsFile goroutines
	blueprintsCh := make(chan stringAndScope)
	errsCh := make(chan []error)
	fileCh := make(chan *parser.File)
	depsCh := make(chan string)

	// Channel to notify main loop that a parseBlueprintsFile goroutine has finished
	doneCh := make(chan struct{})

	// Number of outstanding goroutines to wait for
	count := 0

	startParseBlueprintsFile := func(filename string, scope *parser.Scope) {
		count++
		go func() {
			c.parseBlueprintsFile(filename, scope, rootDir,
				errsCh, fileCh, blueprintsCh, depsCh)
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
		case file := <-fileCh:
			handler(file)
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
	errsCh chan<- []error, fileCh chan<- *parser.File, blueprintsCh chan<- stringAndScope,
	depsCh chan<- string) {

	f, err := os.Open(filename)
	if err != nil {
		errsCh <- []error{err}
		return
	}
	defer func() {
		err = f.Close()
		if err != nil {
			errsCh <- []error{err}
		}
	}()

	file, subBlueprints, deps, errs := c.parse(rootDir, filename, f, scope)
	if len(errs) > 0 {
		errsCh <- errs
	} else {
		fileCh <- file
	}

	for _, b := range subBlueprints {
		blueprintsCh <- b
	}

	for _, d := range deps {
		depsCh <- d
	}
}

func (c *Context) findBuildBlueprints(dir string, build []string,
	buildPos scanner.Position) (blueprints, deps []string, errs []error) {

	for _, file := range build {
		globPattern := filepath.Join(dir, file)
		matches, matchedDirs, err := pathtools.Glob(globPattern)
		if err != nil {
			errs = append(errs, &Error{
				Err: fmt.Errorf("%q: %s", globPattern, err.Error()),
				Pos: buildPos,
			})
			continue
		}

		if len(matches) == 0 {
			errs = append(errs, &Error{
				Err: fmt.Errorf("%q: not found", globPattern),
				Pos: buildPos,
			})
		}

		// Depend on all searched directories so we pick up future changes.
		deps = append(deps, matchedDirs...)

		for _, foundBlueprints := range matches {
			fileInfo, err := os.Stat(foundBlueprints)
			if os.IsNotExist(err) {
				errs = append(errs, &Error{
					Err: fmt.Errorf("%q not found", foundBlueprints),
				})
				continue
			}

			if fileInfo.IsDir() {
				errs = append(errs, &Error{
					Err: fmt.Errorf("%q is a directory", foundBlueprints),
				})
				continue
			}

			blueprints = append(blueprints, foundBlueprints)
		}
	}

	return blueprints, deps, errs
}

func (c *Context) findSubdirBlueprints(dir string, subdirs []string, subdirsPos scanner.Position,
	subBlueprintsName string, optional bool) (blueprints, deps []string, errs []error) {

	for _, subdir := range subdirs {
		globPattern := filepath.Join(dir, subdir)
		matches, matchedDirs, err := pathtools.Glob(globPattern)
		if err != nil {
			errs = append(errs, &Error{
				Err: fmt.Errorf("%q: %s", globPattern, err.Error()),
				Pos: subdirsPos,
			})
			continue
		}

		if len(matches) == 0 && !optional {
			errs = append(errs, &Error{
				Err: fmt.Errorf("%q: not found", globPattern),
				Pos: subdirsPos,
			})
		}

		// Depend on all searched directories so we pick up future changes.
		deps = append(deps, matchedDirs...)

		for _, foundSubdir := range matches {
			fileInfo, subdirStatErr := os.Stat(foundSubdir)
			if subdirStatErr != nil {
				errs = append(errs, subdirStatErr)
				continue
			}

			// Skip files
			if !fileInfo.IsDir() {
				continue
			}

			var subBlueprints string
			if subBlueprintsName != "" {
				subBlueprints = filepath.Join(foundSubdir, subBlueprintsName)
				_, err = os.Stat(subBlueprints)
			}

			if os.IsNotExist(err) || subBlueprints == "" {
				subBlueprints = filepath.Join(foundSubdir, "Blueprints")
				_, err = os.Stat(subBlueprints)
			}

			if os.IsNotExist(err) {
				// There is no Blueprints file in this subdirectory.  We
				// need to add the directory to the list of dependencies
				// so that if someone adds a Blueprints file in the
				// future we'll pick it up.
				deps = append(deps, foundSubdir)
			} else {
				deps = append(deps, subBlueprints)
				blueprints = append(blueprints, subBlueprints)
			}
		}
	}

	return blueprints, deps, errs
}

func getLocalStringListFromScope(scope *parser.Scope, v string) ([]string, scanner.Position, error) {
	if assignment, local := scope.Get(v); assignment == nil || !local {
		return nil, scanner.Position{}, nil
	} else {
		switch assignment.Value.Type {
		case parser.List:
			ret := make([]string, 0, len(assignment.Value.ListValue))

			for _, value := range assignment.Value.ListValue {
				if value.Type != parser.String {
					// The parser should not produce this.
					panic("non-string value found in list")
				}

				ret = append(ret, value.StringValue)
			}

			return ret, assignment.Pos, nil
		case parser.Bool, parser.String:
			return nil, scanner.Position{}, &Error{
				Err: fmt.Errorf("%q must be a list of strings", v),
				Pos: assignment.Pos,
			}
		default:
			panic(fmt.Errorf("unknown value type: %d", assignment.Value.Type))
		}
	}
}

func getStringFromScope(scope *parser.Scope, v string) (string, scanner.Position, error) {
	if assignment, _ := scope.Get(v); assignment == nil {
		return "", scanner.Position{}, nil
	} else {
		switch assignment.Value.Type {
		case parser.String:
			return assignment.Value.StringValue, assignment.Pos, nil
		case parser.Bool, parser.List:
			return "", scanner.Position{}, &Error{
				Err: fmt.Errorf("%q must be a string", v),
				Pos: assignment.Pos,
			}
		default:
			panic(fmt.Errorf("unknown value type: %d", assignment.Value.Type))
		}
	}
}

func (c *Context) createVariations(origModule *moduleInfo, mutatorName string,
	variationNames []string) ([]*moduleInfo, []error) {

	if len(variationNames) == 0 {
		panic(fmt.Errorf("mutator %q passed zero-length variation list for module %q",
			mutatorName, origModule.properties.Name))
	}

	newModules := []*moduleInfo{}

	var errs []error

	for i, variationName := range variationNames {
		typeName := origModule.typeName
		factory, ok := c.moduleFactories[typeName]
		if !ok {
			panic(fmt.Sprintf("unrecognized module type %q during cloning", typeName))
		}

		var newLogicModule Module
		var newProperties []interface{}

		if i == 0 {
			// Reuse the existing module for the first new variant
			// This both saves creating a new module, and causes the insertion in c.moduleInfo below
			// with logicModule as the key to replace the original entry in c.moduleInfo
			newLogicModule = origModule.logicModule
			newProperties = origModule.moduleProperties
		} else {
			props := []interface{}{
				&origModule.properties,
			}
			newLogicModule, newProperties = factory()

			newProperties = append(props, newProperties...)

			if len(newProperties) != len(origModule.moduleProperties) {
				panic("mismatched properties array length in " + origModule.properties.Name)
			}

			for i := range newProperties {
				dst := reflect.ValueOf(newProperties[i]).Elem()
				src := reflect.ValueOf(origModule.moduleProperties[i]).Elem()

				proptools.CopyProperties(dst, src)
			}
		}

		newVariant := origModule.variant.clone()
		newVariant[mutatorName] = variationName

		m := *origModule
		newModule := &m
		newModule.directDeps = append([]*moduleInfo(nil), origModule.directDeps...)
		newModule.logicModule = newLogicModule
		newModule.variant = newVariant
		newModule.dependencyVariant = origModule.dependencyVariant.clone()
		newModule.moduleProperties = newProperties

		if newModule.variantName == "" {
			newModule.variantName = variationName
		} else {
			newModule.variantName += "_" + variationName
		}

		newModules = append(newModules, newModule)

		// Insert the new variant into the global module map.  If this is the first variant then
		// it reuses logicModule from the original module, which causes this to replace the
		// original module in the global module map.
		c.moduleInfo[newModule.logicModule] = newModule

		newErrs := c.convertDepsToVariation(newModule, mutatorName, variationName)
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

func (c *Context) convertDepsToVariation(module *moduleInfo,
	mutatorName, variationName string) (errs []error) {

	for i, dep := range module.directDeps {
		if dep.logicModule == nil {
			var newDep *moduleInfo
			for _, m := range dep.splitModules {
				if m.variant[mutatorName] == variationName {
					newDep = m
					break
				}
			}
			if newDep == nil {
				errs = append(errs, &Error{
					Err: fmt.Errorf("failed to find variation %q for module %q needed by %q",
						variationName, dep.properties.Name, module.properties.Name),
					Pos: module.pos,
				})
				continue
			}
			module.directDeps[i] = newDep
		}
	}

	return errs
}

func (c *Context) prettyPrintVariant(variant variationMap) string {
	names := make([]string, 0, len(variant))
	for _, m := range c.variantMutatorNames {
		if v, ok := variant[m]; ok {
			names = append(names, m+":"+v)
		}
	}

	return strings.Join(names, ", ")
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

	module := &moduleInfo{
		logicModule:       logicModule,
		typeName:          typeName,
		relBlueprintsFile: relBlueprintsFile,
	}

	props := []interface{}{
		&module.properties,
	}
	properties = append(props, properties...)
	module.moduleProperties = properties

	propertyMap, errs := unpackProperties(moduleDef.Properties, properties...)
	if len(errs) > 0 {
		return nil, errs
	}

	module.pos = moduleDef.Type.Pos
	module.propertyPos = make(map[string]scanner.Position)
	for name, propertyDef := range propertyMap {
		module.propertyPos[name] = propertyDef.Pos
	}

	return module, nil
}

func (c *Context) addModule(module *moduleInfo) []error {
	name := module.properties.Name
	c.moduleInfo[module.logicModule] = module

	if group, present := c.moduleGroups[name]; present {
		return []error{
			&Error{
				Err: fmt.Errorf("module %q already defined", name),
				Pos: module.pos,
			},
			&Error{
				Err: fmt.Errorf("<-- previous definition here"),
				Pos: group.modules[0].pos,
			},
		}
	} else {
		ninjaName := toNinjaName(module.properties.Name)

		// The sanitizing in toNinjaName can result in collisions, uniquify the name if it
		// already exists
		for i := 0; c.moduleNinjaNames[ninjaName] != nil; i++ {
			ninjaName = toNinjaName(module.properties.Name) + strconv.Itoa(i)
		}

		group := &moduleGroup{
			name:      module.properties.Name,
			ninjaName: ninjaName,
			modules:   []*moduleInfo{module},
		}
		module.group = group
		c.moduleGroups[name] = group
		c.moduleNinjaNames[ninjaName] = group
	}

	return nil
}

// ResolveDependencies checks that the dependencies specified by all of the
// modules defined in the parsed Blueprints files are valid.  This means that
// the modules depended upon are defined and that no circular dependencies
// exist.
func (c *Context) ResolveDependencies(config interface{}) []error {
	errs := c.runMutators(config)
	if len(errs) > 0 {
		return errs
	}

	c.dependenciesReady = true
	return nil
}

// Default dependencies handling.  If the module implements the (deprecated)
// DynamicDependerModule interface then this set consists of the union of those
// module names listed in its "deps" property, those returned by its
// DynamicDependencies method, and those added by calling AddDependencies or
// AddVariationDependencies on DynamicDependencyModuleContext.  Otherwise it
// is simply those names listed in its "deps" property.
func blueprintDepsMutator(ctx BottomUpMutatorContext) {
	ctx.AddDependency(ctx.Module(), ctx.moduleInfo().properties.Deps...)

	if dynamicDepender, ok := ctx.Module().(DynamicDependerModule); ok {
		func() {
			defer func() {
				if r := recover(); r != nil {
					ctx.error(newPanicErrorf(r, "DynamicDependencies for %s", ctx.moduleInfo()))
				}
			}()
			dynamicDeps := dynamicDepender.DynamicDependencies(ctx)

			if ctx.Failed() {
				return
			}

			ctx.AddDependency(ctx.Module(), dynamicDeps...)
		}()
	}
}

// findMatchingVariant searches the moduleGroup for a module with the same variant as module,
// and returns the matching module, or nil if one is not found.
func (c *Context) findMatchingVariant(module *moduleInfo, group *moduleGroup) *moduleInfo {
	if len(group.modules) == 1 {
		return group.modules[0]
	} else {
		for _, m := range group.modules {
			if m.variant.equal(module.dependencyVariant) {
				return m
			}
		}
	}

	return nil
}

func (c *Context) addDependency(module *moduleInfo, depName string) []error {
	if depName == module.properties.Name {
		return []error{&Error{
			Err: fmt.Errorf("%q depends on itself", depName),
			Pos: module.pos,
		}}
	}

	depInfo, ok := c.moduleGroups[depName]
	if !ok {
		if c.allowMissingDependencies {
			module.missingDeps = append(module.missingDeps, depName)
			return nil
		}
		return []error{&Error{
			Err: fmt.Errorf("%q depends on undefined module %q",
				module.properties.Name, depName),
			Pos: module.pos,
		}}
	}

	for _, m := range module.directDeps {
		if m.group == depInfo {
			return nil
		}
	}

	if m := c.findMatchingVariant(module, depInfo); m != nil {
		module.directDeps = append(module.directDeps, m)
		return nil
	}

	return []error{&Error{
		Err: fmt.Errorf("dependency %q of %q missing variant %q",
			depInfo.modules[0].properties.Name, module.properties.Name,
			c.prettyPrintVariant(module.dependencyVariant)),
		Pos: module.pos,
	}}
}

func (c *Context) findReverseDependency(module *moduleInfo, destName string) (*moduleInfo, []error) {
	if destName == module.properties.Name {
		return nil, []error{&Error{
			Err: fmt.Errorf("%q depends on itself", destName),
			Pos: module.pos,
		}}
	}

	destInfo, ok := c.moduleGroups[destName]
	if !ok {
		return nil, []error{&Error{
			Err: fmt.Errorf("%q has a reverse dependency on undefined module %q",
				module.properties.Name, destName),
			Pos: module.pos,
		}}
	}

	if m := c.findMatchingVariant(module, destInfo); m != nil {
		return m, nil
	}

	return nil, []error{&Error{
		Err: fmt.Errorf("reverse dependency %q of %q missing variant %q",
			destName, module.properties.Name,
			c.prettyPrintVariant(module.dependencyVariant)),
		Pos: module.pos,
	}}
}

func (c *Context) addVariationDependency(module *moduleInfo, variations []Variation,
	depName string, far bool) []error {

	depInfo, ok := c.moduleGroups[depName]
	if !ok {
		if c.allowMissingDependencies {
			module.missingDeps = append(module.missingDeps, depName)
			return nil
		}
		return []error{&Error{
			Err: fmt.Errorf("%q depends on undefined module %q",
				module.properties.Name, depName),
			Pos: module.pos,
		}}
	}

	// We can't just append variant.Variant to module.dependencyVariants.variantName and
	// compare the strings because the result won't be in mutator registration order.
	// Create a new map instead, and then deep compare the maps.
	var newVariant variationMap
	if !far {
		newVariant = module.dependencyVariant.clone()
	} else {
		newVariant = make(variationMap)
	}
	for _, v := range variations {
		newVariant[v.Mutator] = v.Variation
	}

	for _, m := range depInfo.modules {
		var found bool
		if far {
			found = m.variant.subset(newVariant)
		} else {
			found = m.variant.equal(newVariant)
		}
		if found {
			if module == m {
				return []error{&Error{
					Err: fmt.Errorf("%q depends on itself", depName),
					Pos: module.pos,
				}}
			}
			// AddVariationDependency allows adding a dependency on itself, but only if
			// that module is earlier in the module list than this one, since we always
			// run GenerateBuildActions in order for the variants of a module
			if depInfo == module.group && beforeInModuleList(module, m, module.group.modules) {
				return []error{&Error{
					Err: fmt.Errorf("%q depends on later version of itself", depName),
					Pos: module.pos,
				}}
			}
			module.directDeps = append(module.directDeps, m)
			return nil
		}
	}

	return []error{&Error{
		Err: fmt.Errorf("dependency %q of %q missing variant %q",
			depInfo.modules[0].properties.Name, module.properties.Name,
			c.prettyPrintVariant(newVariant)),
		Pos: module.pos,
	}}
}

func (c *Context) parallelVisitAllBottomUp(visit func(group *moduleInfo) bool) {
	doneCh := make(chan *moduleInfo)
	count := 0
	cancel := false

	for _, module := range c.modulesSorted {
		module.waitingCount = module.depsCount
	}

	visitOne := func(module *moduleInfo) {
		count++
		go func() {
			ret := visit(module)
			if ret {
				cancel = true
			}
			doneCh <- module
		}()
	}

	for _, module := range c.modulesSorted {
		if module.waitingCount == 0 {
			visitOne(module)
		}
	}

	for count > 0 {
		select {
		case doneModule := <-doneCh:
			if !cancel {
				for _, parent := range doneModule.reverseDeps {
					parent.waitingCount--
					if parent.waitingCount == 0 {
						visitOne(parent)
					}
				}
			}
			count--
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
	visited := make(map[*moduleInfo]bool)  // modules that were already checked
	checking := make(map[*moduleInfo]bool) // modules actively being checked

	sorted := make([]*moduleInfo, 0, len(c.moduleInfo))

	var check func(group *moduleInfo) []*moduleInfo

	cycleError := func(cycle []*moduleInfo) {
		// We are the "start" of the cycle, so we're responsible
		// for generating the errors.  The cycle list is in
		// reverse order because all the 'check' calls append
		// their own module to the list.
		errs = append(errs, &Error{
			Err: fmt.Errorf("encountered dependency cycle:"),
			Pos: cycle[len(cycle)-1].pos,
		})

		// Iterate backwards through the cycle list.
		curModule := cycle[0]
		for i := len(cycle) - 1; i >= 0; i-- {
			nextModule := cycle[i]
			errs = append(errs, &Error{
				Err: fmt.Errorf("    %q depends on %q",
					curModule.properties.Name,
					nextModule.properties.Name),
				Pos: curModule.pos,
			})
			curModule = nextModule
		}
	}

	check = func(module *moduleInfo) []*moduleInfo {
		visited[module] = true
		checking[module] = true
		defer delete(checking, module)

		deps := make(map[*moduleInfo]bool)

		// Add an implicit dependency ordering on all earlier modules in the same module group
		for _, dep := range module.group.modules {
			if dep == module {
				break
			}
			deps[dep] = true
		}

		for _, dep := range module.directDeps {
			deps[dep] = true
		}

		module.reverseDeps = []*moduleInfo{}
		module.depsCount = len(deps)

		for dep := range deps {
			if checking[dep] {
				// This is a cycle.
				return []*moduleInfo{dep, module}
			}

			if !visited[dep] {
				cycle := check(dep)
				if cycle != nil {
					if cycle[0] == module {
						// We are the "start" of the cycle, so we're responsible
						// for generating the errors.  The cycle list is in
						// reverse order because all the 'check' calls append
						// their own module to the list.
						cycleError(cycle)

						// We can continue processing this module's children to
						// find more cycles.  Since all the modules that were
						// part of the found cycle were marked as visited we
						// won't run into that cycle again.
					} else {
						// We're not the "start" of the cycle, so we just append
						// our module to the list and return it.
						return append(cycle, module)
					}
				}
			}

			dep.reverseDeps = append(dep.reverseDeps, module)
		}

		sorted = append(sorted, module)

		return nil
	}

	for _, module := range c.moduleInfo {
		if !visited[module] {
			cycle := check(module)
			if cycle != nil {
				if cycle[len(cycle)-1] != module {
					panic("inconceivable!")
				}
				cycleError(cycle)
			}
		}
	}

	c.modulesSorted = sorted

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
// by the modules and singletons via the ModuleContext.AddNinjaFileDeps(),
// SingletonContext.AddNinjaFileDeps(), and PackageContext.AddNinjaFileDeps()
// methods.
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

	if c.ninjaBuildDir != nil {
		liveGlobals.addNinjaStringDeps(c.ninjaBuildDir)
	}

	pkgNames, depsPackages := c.makeUniquePackageNames(liveGlobals)

	deps = append(deps, depsPackages...)

	// This will panic if it finds a problem since it's a programming error.
	c.checkForVariableReferenceCycles(liveGlobals.variables, pkgNames)

	c.pkgNames = pkgNames
	c.globalVariables = liveGlobals.variables
	c.globalPools = liveGlobals.pools
	c.globalRules = liveGlobals.rules

	c.buildActionsReady = true

	return deps, nil
}

func (c *Context) runEarlyMutators(config interface{}) (errs []error) {
	for _, mutator := range c.earlyMutatorInfo {
		for _, group := range c.moduleGroups {
			newModules := make([]*moduleInfo, 0, len(group.modules))

			for _, module := range group.modules {
				mctx := &mutatorContext{
					baseModuleContext: baseModuleContext{
						context: c,
						config:  config,
						module:  module,
					},
					name: mutator.name,
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							in := fmt.Sprintf("early mutator %q for %s", mutator.name, module)
							if err, ok := r.(panicError); ok {
								err.addIn(in)
								mctx.error(err)
							} else {
								mctx.error(newPanicErrorf(r, in))
							}
						}
					}()
					mutator.mutator(mctx)
				}()
				if len(mctx.errs) > 0 {
					errs = append(errs, mctx.errs...)
					return errs
				}

				if module.splitModules != nil {
					newModules = append(newModules, module.splitModules...)
				} else {
					newModules = append(newModules, module)
				}
			}

			group.modules = newModules
		}
	}

	errs = c.updateDependencies()
	if len(errs) > 0 {
		return errs
	}

	return nil
}

func (c *Context) runMutators(config interface{}) (errs []error) {
	errs = c.runEarlyMutators(config)
	if len(errs) > 0 {
		return errs
	}

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

	for i := 0; i < len(c.modulesSorted); i++ {
		module := c.modulesSorted[len(c.modulesSorted)-1-i]
		mctx := &mutatorContext{
			baseModuleContext: baseModuleContext{
				context: c,
				config:  config,
				module:  module,
			},
			name: name,
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					in := fmt.Sprintf("top down mutator %q for %s", name, module)
					if err, ok := r.(panicError); ok {
						err.addIn(in)
						mctx.error(err)
					} else {
						mctx.error(newPanicErrorf(r, in))
					}
				}
			}()
			mutator(mctx)
		}()

		if len(mctx.errs) > 0 {
			errs = append(errs, mctx.errs...)
			return errs
		}
	}

	return errs
}

func (c *Context) runBottomUpMutator(config interface{},
	name string, mutator BottomUpMutator) (errs []error) {

	reverseDeps := make(map[*moduleInfo][]*moduleInfo)

	for _, module := range c.modulesSorted {
		newModules := make([]*moduleInfo, 0, 1)

		if module.splitModules != nil {
			panic("split module found in sorted module list")
		}

		mctx := &mutatorContext{
			baseModuleContext: baseModuleContext{
				context: c,
				config:  config,
				module:  module,
			},
			name:        name,
			reverseDeps: reverseDeps,
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					in := fmt.Sprintf("bottom up mutator %q for %s", name, module)
					if err, ok := r.(panicError); ok {
						err.addIn(in)
						mctx.error(err)
					} else {
						mctx.error(newPanicErrorf(r, in))
					}
				}
			}()
			mutator(mctx)
		}()
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

		if module.splitModules != nil {
			newModules = append(newModules, module.splitModules...)
		} else {
			newModules = append(newModules, module)
		}

		module.group.modules = spliceModules(module.group.modules, module, newModules)
	}

	for module, deps := range reverseDeps {
		sort.Sort(moduleSorter(deps))
		module.directDeps = append(module.directDeps, deps...)
	}

	errs = c.updateDependencies()
	if len(errs) > 0 {
		return errs
	}

	return errs
}

func spliceModules(modules []*moduleInfo, origModule *moduleInfo,
	newModules []*moduleInfo) []*moduleInfo {
	for i, m := range modules {
		if m == origModule {
			return spliceModulesAtIndex(modules, i, newModules)
		}
	}

	panic("failed to find original module to splice")
}

func spliceModulesAtIndex(modules []*moduleInfo, i int, newModules []*moduleInfo) []*moduleInfo {
	spliceSize := len(newModules)
	newLen := len(modules) + spliceSize - 1
	var dest []*moduleInfo
	if cap(modules) >= len(modules)-1+len(newModules) {
		// We can fit the splice in the existing capacity, do everything in place
		dest = modules[:newLen]
	} else {
		dest = make([]*moduleInfo, newLen)
		copy(dest, modules[:i])
	}

	// Move the end of the slice over by spliceSize-1
	copy(dest[i+spliceSize:], modules[i+1:])

	// Copy the new modules into the slice
	copy(dest[i:], newModules)

	return dest
}

func (c *Context) initSpecialVariables() {
	c.ninjaBuildDir = nil
	c.requiredNinjaMajor = 1
	c.requiredNinjaMinor = 6
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

	c.parallelVisitAllBottomUp(func(module *moduleInfo) bool {
		// The parent scope of the moduleContext's local scope gets overridden to be that of the
		// calling Go package on a per-call basis.  Since the initial parent scope doesn't matter we
		// just set it to nil.
		prefix := moduleNamespacePrefix(module.group.ninjaName + "_" + module.variantName)
		scope := newLocalScope(nil, prefix)

		mctx := &moduleContext{
			baseModuleContext: baseModuleContext{
				context: c,
				config:  config,
				module:  module,
			},
			scope:              scope,
			handledMissingDeps: module.missingDeps == nil,
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					in := fmt.Sprintf("GenerateBuildActions for %s", module)
					if err, ok := r.(panicError); ok {
						err.addIn(in)
						mctx.error(err)
					} else {
						mctx.error(newPanicErrorf(r, in))
					}
				}
			}()
			mctx.module.logicModule.GenerateBuildActions(mctx)
		}()

		if len(mctx.errs) > 0 {
			errsCh <- mctx.errs
			return true
		}

		if module.missingDeps != nil && !mctx.handledMissingDeps {
			var errs []error
			for _, depName := range module.missingDeps {
				errs = append(errs, &Error{
					Err: fmt.Errorf("%q depends on undefined module %q",
						module.properties.Name, depName),
					Pos: module.pos,
				})
			}
			errsCh <- errs
			return true
		}

		depsCh <- mctx.ninjaFileDeps

		newErrs := c.processLocalBuildActions(&module.actionDefs,
			&mctx.actionDefs, liveGlobals)
		if len(newErrs) > 0 {
			errsCh <- newErrs
			return true
		}
		return false
	})

	cancelCh <- struct{}{}
	<-cancelCh

	return deps, errs
}

func (c *Context) generateSingletonBuildActions(config interface{},
	liveGlobals *liveTracker) ([]string, []error) {

	var deps []string
	var errs []error

	for _, info := range c.singletonInfo {
		// The parent scope of the singletonContext's local scope gets overridden to be that of the
		// calling Go package on a per-call basis.  Since the initial parent scope doesn't matter we
		// just set it to nil.
		scope := newLocalScope(nil, singletonNamespacePrefix(info.name))

		sctx := &singletonContext{
			context: c,
			config:  config,
			scope:   scope,
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					in := fmt.Sprintf("GenerateBuildActions for singleton %s", info.name)
					if err, ok := r.(panicError); ok {
						err.addIn(in)
						sctx.error(err)
					} else {
						sctx.error(newPanicErrorf(r, in))
					}
				}
			}()
			info.singleton.GenerateBuildActions(sctx)
		}()

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
		isLive := liveGlobals.RemoveVariableIfLive(v)
		if isLive {
			out.variables = append(out.variables, v)
		}
	}

	for _, r := range in.rules {
		isLive := liveGlobals.RemoveRuleIfLive(r)
		if isLive {
			out.rules = append(out.rules, r)
		}
	}

	return nil
}

func (c *Context) walkDeps(topModule *moduleInfo,
	visit func(Module, Module) bool) {

	visited := make(map[*moduleInfo]bool)
	var visiting *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "WalkDeps(%s, %s) for dependency %s",
				topModule, funcName(visit), visiting))
		}
	}()

	var walk func(module *moduleInfo)
	walk = func(module *moduleInfo) {
		visited[module] = true

		for _, moduleDep := range module.directDeps {
			if !visited[moduleDep] {
				visiting = moduleDep
				if visit(moduleDep.logicModule, module.logicModule) {
					walk(moduleDep)
				}
			}
		}
	}

	walk(topModule)
}

type innerPanicError error

func (c *Context) visitDepsDepthFirst(topModule *moduleInfo, visit func(Module)) {
	visited := make(map[*moduleInfo]bool)
	var visiting *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitDepsDepthFirst(%s, %s) for dependency %s",
				topModule, funcName(visit), visiting))
		}
	}()

	var walk func(module *moduleInfo)
	walk = func(module *moduleInfo) {
		visited[module] = true
		for _, moduleDep := range module.directDeps {
			if !visited[moduleDep] {
				walk(moduleDep)
			}
		}

		if module != topModule {
			visiting = module
			visit(module.logicModule)
		}
	}

	walk(topModule)
}

func (c *Context) visitDepsDepthFirstIf(topModule *moduleInfo, pred func(Module) bool,
	visit func(Module)) {

	visited := make(map[*moduleInfo]bool)
	var visiting *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitDepsDepthFirstIf(%s, %s, %s) for dependency %s",
				topModule, funcName(pred), funcName(visit), visiting))
		}
	}()

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
				visiting = module
				visit(module.logicModule)
			}
		}
	}

	walk(topModule)
}

func (c *Context) visitDirectDeps(module *moduleInfo, visit func(Module)) {
	var dep *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitDirectDeps(%s, %s) for dependency %s",
				module, funcName(visit), dep))
		}
	}()

	for _, dep = range module.directDeps {
		visit(dep.logicModule)
	}
}

func (c *Context) visitDirectDepsIf(module *moduleInfo, pred func(Module) bool,
	visit func(Module)) {

	var dep *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitDirectDepsIf(%s, %s, %s) for dependency %s",
				module, funcName(pred), funcName(visit), dep))
		}
	}()

	for _, dep = range module.directDeps {
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
	var module *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitAllModules(%s) for %s",
				funcName(visit), module))
		}
	}()

	for _, moduleName := range c.sortedModuleNames() {
		group := c.moduleGroups[moduleName]
		for _, module = range group.modules {
			visit(module.logicModule)
		}
	}
}

func (c *Context) visitAllModulesIf(pred func(Module) bool,
	visit func(Module)) {

	var module *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitAllModulesIf(%s, %s) for %s",
				funcName(pred), funcName(visit), module))
		}
	}()

	for _, moduleName := range c.sortedModuleNames() {
		group := c.moduleGroups[moduleName]
		for _, module := range group.modules {
			if pred(module.logicModule) {
				visit(module.logicModule)
			}
		}
	}
}

func (c *Context) visitAllModuleVariants(module *moduleInfo,
	visit func(Module)) {

	var variant *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitAllModuleVariants(%s, %s) for %s",
				module, funcName(visit), variant))
		}
	}()

	for _, variant = range module.group.modules {
		visit(variant.logicModule)
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

func (c *Context) setNinjaBuildDir(value *ninjaString) {
	if c.ninjaBuildDir == nil {
		c.ninjaBuildDir = value
	}
}

func (c *Context) makeUniquePackageNames(
	liveGlobals *liveTracker) (map[*packageContext]string, []string) {

	pkgs := make(map[string]*packageContext)
	pkgNames := make(map[*packageContext]string)
	longPkgNames := make(map[*packageContext]bool)

	processPackage := func(pctx *packageContext) {
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
			pkgs[pctx.shortName] = pctx
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

	// Create deps list from calls to PackageContext.AddNinjaFileDeps
	deps := []string{}
	for _, pkg := range pkgs {
		deps = append(deps, pkg.ninjaFileDeps...)
	}

	return pkgNames, deps
}

func (c *Context) checkForVariableReferenceCycles(
	variables map[Variable]*ninjaString, pkgNames map[*packageContext]string) {

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
	for _, module := range c.moduleInfo {
		for _, buildDef := range module.actionDefs.buildDefs {
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

func (c *Context) NinjaBuildDir() (string, error) {
	if c.ninjaBuildDir != nil {
		return c.ninjaBuildDir.Eval(c.globalVariables)
	} else {
		return "", nil
	}
}

// ModuleTypePropertyStructs returns a mapping from module type name to a list of pointers to
// property structs returned by the factory for that module type.
func (c *Context) ModuleTypePropertyStructs() map[string][]interface{} {
	ret := make(map[string][]interface{})
	for moduleType, factory := range c.moduleFactories {
		_, ret[moduleType] = factory()
	}

	return ret
}

func (c *Context) ModuleName(logicModule Module) string {
	module := c.moduleInfo[logicModule]
	return module.properties.Name
}

func (c *Context) ModuleDir(logicModule Module) string {
	module := c.moduleInfo[logicModule]
	return filepath.Dir(module.relBlueprintsFile)
}

func (c *Context) ModuleSubDir(logicModule Module) string {
	module := c.moduleInfo[logicModule]
	return module.variantName
}

func (c *Context) BlueprintFile(logicModule Module) string {
	module := c.moduleInfo[logicModule]
	return module.relBlueprintsFile
}

func (c *Context) ModuleErrorf(logicModule Module, format string,
	args ...interface{}) error {

	module := c.moduleInfo[logicModule]
	return &Error{
		Err: fmt.Errorf(format, args...),
		Pos: module.pos,
	}
}

func (c *Context) VisitAllModules(visit func(Module)) {
	c.visitAllModules(visit)
}

func (c *Context) VisitAllModulesIf(pred func(Module) bool,
	visit func(Module)) {

	c.visitAllModulesIf(pred, visit)
}

func (c *Context) VisitDepsDepthFirst(module Module,
	visit func(Module)) {

	c.visitDepsDepthFirst(c.moduleInfo[module], visit)
}

func (c *Context) VisitDepsDepthFirstIf(module Module,
	pred func(Module) bool, visit func(Module)) {

	c.visitDepsDepthFirstIf(c.moduleInfo[module], pred, visit)
}

func (c *Context) PrimaryModule(module Module) Module {
	return c.moduleInfo[module].group.modules[0].logicModule
}

func (c *Context) FinalModule(module Module) Module {
	modules := c.moduleInfo[module].group.modules
	return modules[len(modules)-1].logicModule
}

func (c *Context) VisitAllModuleVariants(module Module,
	visit func(Module)) {

	c.visitAllModuleVariants(c.moduleInfo[module], visit)
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
	if c.ninjaBuildDir != nil {
		err := nw.Assign("builddir", c.ninjaBuildDir.Value(c.pkgNames))
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
	fullName(pkgNames map[*packageContext]string) string
}

type globalEntitySorter struct {
	pkgNames map[*packageContext]string
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

type moduleSorter []*moduleInfo

func (s moduleSorter) Len() int {
	return len(s)
}

func (s moduleSorter) Less(i, j int) bool {
	iName := s[i].properties.Name
	jName := s[j].properties.Name
	if iName == jName {
		iName = s[i].variantName
		jName = s[j].variantName
	}
	return iName < jName
}

func (s moduleSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (c *Context) writeAllModuleActions(nw *ninjaWriter) error {
	headerTemplate := template.New("moduleHeader")
	_, err := headerTemplate.Parse(moduleHeaderTemplate)
	if err != nil {
		// This is a programming error.
		panic(err)
	}

	modules := make([]*moduleInfo, 0, len(c.moduleInfo))
	for _, module := range c.moduleInfo {
		modules = append(modules, module)
	}
	sort.Sort(moduleSorter(modules))

	buf := bytes.NewBuffer(nil)

	for _, module := range modules {
		if len(module.actionDefs.variables)+len(module.actionDefs.rules)+len(module.actionDefs.buildDefs) == 0 {
			continue
		}

		buf.Reset()

		// In order to make the bootstrap build manifest independent of the
		// build dir we need to output the Blueprints file locations in the
		// comments as paths relative to the source directory.
		relPos := module.pos
		relPos.Filename = module.relBlueprintsFile

		// Get the name and location of the factory function for the module.
		factory := c.moduleFactories[module.typeName]
		factoryFunc := runtime.FuncForPC(reflect.ValueOf(factory).Pointer())
		factoryName := factoryFunc.Name()

		infoMap := map[string]interface{}{
			"properties": module.properties,
			"typeName":   module.typeName,
			"goFactory":  factoryName,
			"pos":        relPos,
			"variant":    module.variantName,
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

		err = c.writeLocalBuildActions(nw, &module.actionDefs)
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

	for _, info := range c.singletonInfo {
		if len(info.actionDefs.variables)+len(info.actionDefs.rules)+len(info.actionDefs.buildDefs) == 0 {
			continue
		}

		// Get the name of the factory function for the module.
		factory := info.factory
		factoryFunc := runtime.FuncForPC(reflect.ValueOf(factory).Pointer())
		factoryName := factoryFunc.Name()

		buf.Reset()
		infoMap := map[string]interface{}{
			"name":      info.name,
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

func beforeInModuleList(a, b *moduleInfo, list []*moduleInfo) bool {
	found := false
	if a == b {
		return false
	}
	for _, l := range list {
		if l == a {
			found = true
		} else if l == b {
			return found
		}
	}

	missing := a
	if found {
		missing = b
	}
	panic(fmt.Errorf("element %v not found in list %v", missing, list))
}

type panicError struct {
	panic interface{}
	stack []byte
	in    string
}

func newPanicErrorf(panic interface{}, in string, a ...interface{}) error {
	buf := make([]byte, 4096)
	count := runtime.Stack(buf, false)
	return panicError{
		panic: panic,
		in:    fmt.Sprintf(in, a...),
		stack: buf[:count],
	}
}

func (p panicError) Error() string {
	return fmt.Sprintf("panic in %s\n%s\n%s\n", p.in, p.panic, p.stack)
}

func (p *panicError) addIn(in string) {
	p.in += " in " + in
}

func funcName(f interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
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
Variant: {{.variant}}
Type:    {{.typeName}}
Factory: {{.goFactory}}
Defined: {{.pos}}
`

var singletonHeaderTemplate = `# # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # 
Singleton: {{.name}}
Factory:   {{.goFactory}}
`
