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
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"text/scanner"
	"text/template"

	"github.com/google/blueprint/parser"
	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"
)

var ErrBuildActionsNotReady = errors.New("build actions are not ready")

const maxErrors = 10
const MockModuleListFile = "bplist"

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
	context.Context

	// set at instantiation
	moduleFactories     map[string]ModuleFactory
	nameInterface       NameInterface
	moduleGroups        []*moduleGroup
	moduleInfo          map[Module]*moduleInfo
	modulesSorted       []*moduleInfo
	preSingletonInfo    []*singletonInfo
	singletonInfo       []*singletonInfo
	mutatorInfo         []*mutatorInfo
	earlyMutatorInfo    []*mutatorInfo
	variantMutatorNames []string

	depsModified uint32 // positive if a mutator modified the dependencies

	dependenciesReady bool // set to true on a successful ResolveDependencies
	buildActionsReady bool // set to true on a successful PrepareBuildActions

	// set by SetIgnoreUnknownModuleTypes
	ignoreUnknownModuleTypes bool

	// set by SetAllowMissingDependencies
	allowMissingDependencies bool

	// set during PrepareBuildActions
	pkgNames        map[*packageContext]string
	liveGlobals     *liveTracker
	globalVariables map[Variable]*ninjaString
	globalPools     map[Pool]*poolDef
	globalRules     map[Rule]*ruleDef

	// set during PrepareBuildActions
	ninjaBuildDir      *ninjaString // The builddir special Ninja variable
	requiredNinjaMajor int          // For the ninja_required_version variable
	requiredNinjaMinor int          // For the ninja_required_version variable
	requiredNinjaMicro int          // For the ninja_required_version variable

	subninjas []string

	// set lazily by sortedModuleGroups
	cachedSortedModuleGroups []*moduleGroup

	globs    map[string]GlobPath
	globLock sync.Mutex

	fs             pathtools.FileSystem
	moduleListFile string
}

// An Error describes a problem that was encountered that is related to a
// particular location in a Blueprints file.
type BlueprintError struct {
	Err error            // the error that occurred
	Pos scanner.Position // the relevant Blueprints file location
}

// A ModuleError describes a problem that was encountered that is related to a
// particular module in a Blueprints file
type ModuleError struct {
	BlueprintError
	module *moduleInfo
}

// A PropertyError describes a problem that was encountered that is related to a
// particular property in a Blueprints file
type PropertyError struct {
	ModuleError
	property string
}

func (e *BlueprintError) Error() string {
	return fmt.Sprintf("%s: %s", e.Pos, e.Err)
}

func (e *ModuleError) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.Pos, e.module, e.Err)
}

func (e *PropertyError) Error() string {
	return fmt.Sprintf("%s: %s: %s: %s", e.Pos, e.module, e.property, e.Err)
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

	namespace Namespace
}

type moduleInfo struct {
	// set during Parse
	typeName          string
	factory           ModuleFactory
	relBlueprintsFile string
	pos               scanner.Position
	propertyPos       map[string]scanner.Position

	variantName       string
	variant           variationMap
	dependencyVariant variationMap

	logicModule Module
	group       *moduleGroup
	properties  []interface{}

	// set during ResolveDependencies
	directDeps  []depInfo
	missingDeps []string

	// set during updateDependencies
	reverseDeps []*moduleInfo
	forwardDeps []*moduleInfo

	// used by parallelVisitAllBottomUp
	waitingCount int

	// set during each runMutator
	splitModules []*moduleInfo

	// set during PrepareBuildActions
	actionDefs localBuildActions
}

type depInfo struct {
	module *moduleInfo
	tag    DependencyTag
}

func (module *moduleInfo) Name() string {
	return module.group.name
}

func (module *moduleInfo) String() string {
	s := fmt.Sprintf("module %q", module.Name())
	if module.variantName != "" {
		s += fmt.Sprintf(" variant %q", module.variantName)
	}
	return s
}

func (module *moduleInfo) namespace() Namespace {
	return module.group.namespace
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
	parallel        bool
}

func newContext() *Context {
	return &Context{
		Context:            context.Background(),
		moduleFactories:    make(map[string]ModuleFactory),
		nameInterface:      NewSimpleNameInterface(),
		moduleInfo:         make(map[Module]*moduleInfo),
		globs:              make(map[string]GlobPath),
		fs:                 pathtools.OsFs,
		ninjaBuildDir:      nil,
		requiredNinjaMajor: 1,
		requiredNinjaMinor: 7,
		requiredNinjaMicro: 0,
	}
}

// NewContext creates a new Context object.  The created context initially has
// no module or singleton factories registered, so the RegisterModuleFactory and
// RegisterSingletonFactory methods must be called before it can do anything
// useful.
func NewContext() *Context {
	ctx := newContext()

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

// RegisterPreSingletonType registers a presingleton type that will be invoked to
// generate build actions before any Blueprint files have been read.  Each registered
// presingleton type is instantiated and invoked exactly once at the beginning of the
// parse phase.  Each registered presingleton is invoked in registration order.
//
// The presingleton type names given here must be unique for the context.  The
// factory function should be a named function so that its package and name can
// be included in the generated Ninja file for debugging purposes.
func (c *Context) RegisterPreSingletonType(name string, factory SingletonFactory) {
	for _, s := range c.preSingletonInfo {
		if s.name == name {
			panic(errors.New("presingleton name is already registered"))
		}
	}

	c.preSingletonInfo = append(c.preSingletonInfo, &singletonInfo{
		factory:   factory,
		singleton: factory(),
		name:      name,
	})
}

func (c *Context) SetNameInterface(i NameInterface) {
	c.nameInterface = i
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

// RegisterTopDownMutator registers a mutator that will be invoked to propagate dependency info
// top-down between Modules.  Each registered mutator is invoked in registration order (mixing
// TopDownMutators and BottomUpMutators) once per Module, and the invocation on any module will
// have returned before it is in invoked on any of its dependencies.
//
// The mutator type names given here must be unique to all top down mutators in
// the Context.
//
// Returns a MutatorHandle, on which Parallel can be called to set the mutator to visit modules in
// parallel while maintaining ordering.
func (c *Context) RegisterTopDownMutator(name string, mutator TopDownMutator) MutatorHandle {
	for _, m := range c.mutatorInfo {
		if m.name == name && m.topDownMutator != nil {
			panic(fmt.Errorf("mutator name %s is already registered", name))
		}
	}

	info := &mutatorInfo{
		topDownMutator: mutator,
		name:           name,
	}

	c.mutatorInfo = append(c.mutatorInfo, info)

	return info
}

// RegisterBottomUpMutator registers a mutator that will be invoked to split Modules into variants.
// Each registered mutator is invoked in registration order (mixing TopDownMutators and
// BottomUpMutators) once per Module, will not be invoked on a module until the invocations on all
// of the modules dependencies have returned.
//
// The mutator type names given here must be unique to all bottom up or early
// mutators in the Context.
//
// Returns a MutatorHandle, on which Parallel can be called to set the mutator to visit modules in
// parallel while maintaining ordering.
func (c *Context) RegisterBottomUpMutator(name string, mutator BottomUpMutator) MutatorHandle {
	for _, m := range c.variantMutatorNames {
		if m == name {
			panic(fmt.Errorf("mutator name %s is already registered", name))
		}
	}

	info := &mutatorInfo{
		bottomUpMutator: mutator,
		name:            name,
	}
	c.mutatorInfo = append(c.mutatorInfo, info)

	c.variantMutatorNames = append(c.variantMutatorNames, name)

	return info
}

type MutatorHandle interface {
	// Set the mutator to visit modules in parallel while maintaining ordering.  Calling any
	// method on the mutator context is thread-safe, but the mutator must handle synchronization
	// for any modifications to global state or any modules outside the one it was invoked on.
	Parallel() MutatorHandle
}

func (mutator *mutatorInfo) Parallel() MutatorHandle {
	mutator.parallel = true
	return mutator
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

	c.earlyMutatorInfo = append(c.earlyMutatorInfo, &mutatorInfo{
		bottomUpMutator: func(mctx BottomUpMutatorContext) {
			mutator(mctx)
		},
		name: name,
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

func (c *Context) SetModuleListFile(listFile string) {
	c.moduleListFile = listFile
}

func (c *Context) ListModulePaths(baseDir string) (paths []string, err error) {
	reader, err := c.fs.Open(c.moduleListFile)
	if err != nil {
		return nil, err
	}
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	text := string(bytes)

	text = strings.Trim(text, "\n")
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = filepath.Join(baseDir, lines[i])
	}

	return lines, nil
}

// a fileParseContext tells the status of parsing a particular file
type fileParseContext struct {
	// name of file
	fileName string

	// scope to use when resolving variables
	Scope *parser.Scope

	// pointer to the one in the parent directory
	parent *fileParseContext

	// is closed once FileHandler has completed for this file
	doneVisiting chan struct{}
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
func (c *Context) ParseBlueprintsFiles(rootFile string) (deps []string, errs []error) {
	baseDir := filepath.Dir(rootFile)
	pathsToParse, err := c.ListModulePaths(baseDir)
	if err != nil {
		return nil, []error{err}
	}
	return c.ParseFileList(baseDir, pathsToParse)
}

func (c *Context) ParseFileList(rootDir string, filePaths []string) (deps []string,
	errs []error) {

	if len(filePaths) < 1 {
		return nil, []error{fmt.Errorf("no paths provided to parse")}
	}

	c.dependenciesReady = false

	moduleCh := make(chan *moduleInfo)
	errsCh := make(chan []error)
	doneCh := make(chan struct{})
	var numErrs uint32
	var numGoroutines int32

	// handler must be reentrant
	handleOneFile := func(file *parser.File) {
		if atomic.LoadUint32(&numErrs) > maxErrors {
			return
		}

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
	}

	atomic.AddInt32(&numGoroutines, 1)
	go func() {
		var errs []error
		deps, errs = c.WalkBlueprintsFiles(rootDir, filePaths, handleOneFile)
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

// WalkBlueprintsFiles walks a set of Blueprints files starting with the given filepaths,
// calling the given file handler on each
//
// When WalkBlueprintsFiles encounters a Blueprints file with a set of subdirs listed,
// it recursively parses any Blueprints files found in those subdirectories.
//
// If any of the file paths is an ancestor directory of any other of file path, the ancestor
// will be parsed and visited first.
//
// the file handler will be called from a goroutine, so it must be reentrant.
//
// If no errors are encountered while parsing the files, the list of paths on
// which the future output will depend is returned.  This list will include both
// Blueprints file paths as well as directory paths for cases where wildcard
// subdirs are found.
//
// visitor will be called asynchronously, and will only be called once visitor for each
// ancestor directory has completed.
//
// WalkBlueprintsFiles will not return until all calls to visitor have returned.
func (c *Context) WalkBlueprintsFiles(rootDir string, filePaths []string,
	visitor FileHandler) (deps []string, errs []error) {

	// make a mapping from ancestors to their descendants to facilitate parsing ancestors first
	descendantsMap, err := findBlueprintDescendants(filePaths)
	if err != nil {
		panic(err.Error())
	}
	blueprintsSet := make(map[string]bool)

	// Channels to receive data back from openAndParse goroutines
	blueprintsCh := make(chan fileParseContext)
	errsCh := make(chan []error)
	depsCh := make(chan string)

	// Channel to notify main loop that a openAndParse goroutine has finished
	doneParsingCh := make(chan fileParseContext)

	// Number of outstanding goroutines to wait for
	activeCount := 0
	var pending []fileParseContext
	tooManyErrors := false

	// Limit concurrent calls to parseBlueprintFiles to 200
	// Darwin has a default limit of 256 open files
	maxActiveCount := 200

	// count the number of pending calls to visitor()
	visitorWaitGroup := sync.WaitGroup{}

	startParseBlueprintsFile := func(blueprint fileParseContext) {
		if blueprintsSet[blueprint.fileName] {
			return
		}
		blueprintsSet[blueprint.fileName] = true
		activeCount++
		deps = append(deps, blueprint.fileName)
		visitorWaitGroup.Add(1)
		go func() {
			file, blueprints, deps, errs := c.openAndParse(blueprint.fileName, blueprint.Scope, rootDir,
				&blueprint)
			if len(errs) > 0 {
				errsCh <- errs
			}
			for _, blueprint := range blueprints {
				blueprintsCh <- blueprint
			}
			for _, dep := range deps {
				depsCh <- dep
			}
			doneParsingCh <- blueprint

			if blueprint.parent != nil && blueprint.parent.doneVisiting != nil {
				// wait for visitor() of parent to complete
				<-blueprint.parent.doneVisiting
			}

			if len(errs) == 0 {
				// process this file
				visitor(file)
			}
			if blueprint.doneVisiting != nil {
				close(blueprint.doneVisiting)
			}
			visitorWaitGroup.Done()
		}()
	}

	foundParseableBlueprint := func(blueprint fileParseContext) {
		if activeCount >= maxActiveCount {
			pending = append(pending, blueprint)
		} else {
			startParseBlueprintsFile(blueprint)
		}
	}

	startParseDescendants := func(blueprint fileParseContext) {
		descendants, hasDescendants := descendantsMap[blueprint.fileName]
		if hasDescendants {
			for _, descendant := range descendants {
				foundParseableBlueprint(fileParseContext{descendant, parser.NewScope(blueprint.Scope), &blueprint, make(chan struct{})})
			}
		}
	}

	// begin parsing any files that have no ancestors
	startParseDescendants(fileParseContext{"", parser.NewScope(nil), nil, nil})

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
		case blueprint := <-blueprintsCh:
			if tooManyErrors {
				continue
			}
			foundParseableBlueprint(blueprint)
		case blueprint := <-doneParsingCh:
			activeCount--
			if !tooManyErrors {
				startParseDescendants(blueprint)
			}
			if activeCount < maxActiveCount && len(pending) > 0 {
				// start to process the next one from the queue
				next := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				startParseBlueprintsFile(next)
			}
			if activeCount == 0 {
				break loop
			}
		}
	}

	sort.Strings(deps)

	// wait for every visitor() to complete
	visitorWaitGroup.Wait()

	return
}

// MockFileSystem causes the Context to replace all reads with accesses to the provided map of
// filenames to contents stored as a byte slice.
func (c *Context) MockFileSystem(files map[string][]byte) {
	// look for a module list file
	_, ok := files[MockModuleListFile]
	if !ok {
		// no module list file specified; find every file named Blueprints
		pathsToParse := []string{}
		for candidate := range files {
			if filepath.Base(candidate) == "Blueprints" {
				pathsToParse = append(pathsToParse, candidate)
			}
		}
		if len(pathsToParse) < 1 {
			panic(fmt.Sprintf("No Blueprints files found in mock filesystem: %v\n", files))
		}
		// put the list of Blueprints files into a list file
		files[MockModuleListFile] = []byte(strings.Join(pathsToParse, "\n"))
	}
	c.SetModuleListFile(MockModuleListFile)

	// mock the filesystem
	c.fs = pathtools.MockFs(files)
}

// openAndParse opens and parses a single Blueprints file, and returns the results
func (c *Context) openAndParse(filename string, scope *parser.Scope, rootDir string,
	parent *fileParseContext) (file *parser.File,
	subBlueprints []fileParseContext, deps []string, errs []error) {

	f, err := c.fs.Open(filename)
	if err != nil {
		// couldn't open the file; see if we can provide a clearer error than "could not open file"
		stats, statErr := c.fs.Lstat(filename)
		if statErr == nil {
			isSymlink := stats.Mode()&os.ModeSymlink != 0
			if isSymlink {
				err = fmt.Errorf("could not open symlink %v : %v", filename, err)
				target, readlinkErr := os.Readlink(filename)
				if readlinkErr == nil {
					_, targetStatsErr := c.fs.Lstat(target)
					if targetStatsErr != nil {
						err = fmt.Errorf("could not open symlink %v; its target (%v) cannot be opened", filename, target)
					}
				}
			} else {
				err = fmt.Errorf("%v exists but could not be opened: %v", filename, err)
			}
		}
		return nil, nil, nil, []error{err}
	}

	func() {
		defer func() {
			err = f.Close()
			if err != nil {
				errs = append(errs, err)
			}
		}()
		file, subBlueprints, errs = c.parseOne(rootDir, filename, f, scope, parent)
	}()

	if len(errs) > 0 {
		return nil, nil, nil, errs
	}

	for _, b := range subBlueprints {
		deps = append(deps, b.fileName)
	}

	return file, subBlueprints, deps, nil
}

// parseOne parses a single Blueprints file from the given reader, creating Module
// objects for each of the module definitions encountered.  If the Blueprints
// file contains an assignment to the "subdirs" variable, then the
// subdirectories listed are searched for Blueprints files returned in the
// subBlueprints return value.  If the Blueprints file contains an assignment
// to the "build" variable, then the file listed are returned in the
// subBlueprints return value.
//
// rootDir specifies the path to the root directory of the source tree, while
// filename specifies the path to the Blueprints file.  These paths are used for
// error reporting and for determining the module's directory.
func (c *Context) parseOne(rootDir, filename string, reader io.Reader,
	scope *parser.Scope, parent *fileParseContext) (file *parser.File, subBlueprints []fileParseContext, errs []error) {

	relBlueprintsFile, err := filepath.Rel(rootDir, filename)
	if err != nil {
		return nil, nil, []error{err}
	}

	scope.Remove("subdirs")
	scope.Remove("optional_subdirs")
	scope.Remove("build")
	file, errs = parser.ParseAndEval(filename, reader, scope)
	if len(errs) > 0 {
		for i, err := range errs {
			if parseErr, ok := err.(*parser.ParseError); ok {
				err = &BlueprintError{
					Err: parseErr.Err,
					Pos: parseErr.Pos,
				}
				errs[i] = err
			}
		}

		// If there were any parse errors don't bother trying to interpret the
		// result.
		return nil, nil, errs
	}
	file.Name = relBlueprintsFile

	build, buildPos, err := getLocalStringListFromScope(scope, "build")
	if err != nil {
		errs = append(errs, err)
	}
	for _, buildEntry := range build {
		if strings.Contains(buildEntry, "/") {
			errs = append(errs, &BlueprintError{
				Err: fmt.Errorf("illegal value %v. The '/' character is not permitted", buildEntry),
				Pos: buildPos,
			})
		}
	}

	subBlueprintsName, _, err := getStringFromScope(scope, "subname")
	if err != nil {
		errs = append(errs, err)
	}

	if subBlueprintsName == "" {
		subBlueprintsName = "Blueprints"
	}

	var blueprints []string

	newBlueprints, newErrs := c.findBuildBlueprints(filepath.Dir(filename), build, buildPos)
	blueprints = append(blueprints, newBlueprints...)
	errs = append(errs, newErrs...)

	subBlueprintsAndScope := make([]fileParseContext, len(blueprints))
	for i, b := range blueprints {
		subBlueprintsAndScope[i] = fileParseContext{b, parser.NewScope(scope), parent, make(chan struct{})}
	}
	return file, subBlueprintsAndScope, errs
}

func (c *Context) findBuildBlueprints(dir string, build []string,
	buildPos scanner.Position) ([]string, []error) {

	var blueprints []string
	var errs []error

	for _, file := range build {
		pattern := filepath.Join(dir, file)
		var matches []string
		var err error

		matches, err = c.glob(pattern, nil)

		if err != nil {
			errs = append(errs, &BlueprintError{
				Err: fmt.Errorf("%q: %s", pattern, err.Error()),
				Pos: buildPos,
			})
			continue
		}

		if len(matches) == 0 {
			errs = append(errs, &BlueprintError{
				Err: fmt.Errorf("%q: not found", pattern),
				Pos: buildPos,
			})
		}

		for _, foundBlueprints := range matches {
			if strings.HasSuffix(foundBlueprints, "/") {
				errs = append(errs, &BlueprintError{
					Err: fmt.Errorf("%q: is a directory", foundBlueprints),
					Pos: buildPos,
				})
			}
			blueprints = append(blueprints, foundBlueprints)
		}
	}

	return blueprints, errs
}

func (c *Context) findSubdirBlueprints(dir string, subdirs []string, subdirsPos scanner.Position,
	subBlueprintsName string, optional bool) ([]string, []error) {

	var blueprints []string
	var errs []error

	for _, subdir := range subdirs {
		pattern := filepath.Join(dir, subdir, subBlueprintsName)
		var matches []string
		var err error

		matches, err = c.glob(pattern, nil)

		if err != nil {
			errs = append(errs, &BlueprintError{
				Err: fmt.Errorf("%q: %s", pattern, err.Error()),
				Pos: subdirsPos,
			})
			continue
		}

		if len(matches) == 0 && !optional {
			errs = append(errs, &BlueprintError{
				Err: fmt.Errorf("%q: not found", pattern),
				Pos: subdirsPos,
			})
		}

		for _, subBlueprints := range matches {
			if strings.HasSuffix(subBlueprints, "/") {
				errs = append(errs, &BlueprintError{
					Err: fmt.Errorf("%q: is a directory", subBlueprints),
					Pos: subdirsPos,
				})
			}
			blueprints = append(blueprints, subBlueprints)
		}
	}

	return blueprints, errs
}

func getLocalStringListFromScope(scope *parser.Scope, v string) ([]string, scanner.Position, error) {
	if assignment, local := scope.Get(v); assignment == nil || !local {
		return nil, scanner.Position{}, nil
	} else {
		switch value := assignment.Value.Eval().(type) {
		case *parser.List:
			ret := make([]string, 0, len(value.Values))

			for _, listValue := range value.Values {
				s, ok := listValue.(*parser.String)
				if !ok {
					// The parser should not produce this.
					panic("non-string value found in list")
				}

				ret = append(ret, s.Value)
			}

			return ret, assignment.EqualsPos, nil
		case *parser.Bool, *parser.String:
			return nil, scanner.Position{}, &BlueprintError{
				Err: fmt.Errorf("%q must be a list of strings", v),
				Pos: assignment.EqualsPos,
			}
		default:
			panic(fmt.Errorf("unknown value type: %d", assignment.Value.Type()))
		}
	}
}

func getStringFromScope(scope *parser.Scope, v string) (string, scanner.Position, error) {
	if assignment, _ := scope.Get(v); assignment == nil {
		return "", scanner.Position{}, nil
	} else {
		switch value := assignment.Value.Eval().(type) {
		case *parser.String:
			return value.Value, assignment.EqualsPos, nil
		case *parser.Bool, *parser.List:
			return "", scanner.Position{}, &BlueprintError{
				Err: fmt.Errorf("%q must be a string", v),
				Pos: assignment.EqualsPos,
			}
		default:
			panic(fmt.Errorf("unknown value type: %d", assignment.Value.Type()))
		}
	}
}

// Clones a build logic module by calling the factory method for its module type, and then cloning
// property values.  Any values stored in the module object that are not stored in properties
// structs will be lost.
func (c *Context) cloneLogicModule(origModule *moduleInfo) (Module, []interface{}) {
	newLogicModule, newProperties := origModule.factory()

	if len(newProperties) != len(origModule.properties) {
		panic("mismatched properties array length in " + origModule.Name())
	}

	for i := range newProperties {
		dst := reflect.ValueOf(newProperties[i]).Elem()
		src := reflect.ValueOf(origModule.properties[i]).Elem()

		proptools.CopyProperties(dst, src)
	}

	return newLogicModule, newProperties
}

func (c *Context) createVariations(origModule *moduleInfo, mutatorName string,
	defaultVariationName *string, variationNames []string) ([]*moduleInfo, []error) {

	if len(variationNames) == 0 {
		panic(fmt.Errorf("mutator %q passed zero-length variation list for module %q",
			mutatorName, origModule.Name()))
	}

	newModules := []*moduleInfo{}

	var errs []error

	for i, variationName := range variationNames {
		var newLogicModule Module
		var newProperties []interface{}

		if i == 0 {
			// Reuse the existing module for the first new variant
			// This both saves creating a new module, and causes the insertion in c.moduleInfo below
			// with logicModule as the key to replace the original entry in c.moduleInfo
			newLogicModule, newProperties = origModule.logicModule, origModule.properties
		} else {
			newLogicModule, newProperties = c.cloneLogicModule(origModule)
		}

		newVariant := origModule.variant.clone()
		newVariant[mutatorName] = variationName

		m := *origModule
		newModule := &m
		newModule.directDeps = append([]depInfo{}, origModule.directDeps...)
		newModule.logicModule = newLogicModule
		newModule.variant = newVariant
		newModule.dependencyVariant = origModule.dependencyVariant.clone()
		newModule.properties = newProperties

		if variationName != "" {
			if newModule.variantName == "" {
				newModule.variantName = variationName
			} else {
				newModule.variantName += "_" + variationName
			}
		}

		newModules = append(newModules, newModule)

		newErrs := c.convertDepsToVariation(newModule, mutatorName, variationName, defaultVariationName)
		if len(newErrs) > 0 {
			errs = append(errs, newErrs...)
		}
	}

	// Mark original variant as invalid.  Modules that depend on this module will still
	// depend on origModule, but we'll fix it when the mutator is called on them.
	origModule.logicModule = nil
	origModule.splitModules = newModules

	atomic.AddUint32(&c.depsModified, 1)

	return newModules, errs
}

func (c *Context) convertDepsToVariation(module *moduleInfo,
	mutatorName, variationName string, defaultVariationName *string) (errs []error) {

	for i, dep := range module.directDeps {
		if dep.module.logicModule == nil {
			var newDep *moduleInfo
			for _, m := range dep.module.splitModules {
				if m.variant[mutatorName] == variationName {
					newDep = m
					break
				}
			}
			if newDep == nil && defaultVariationName != nil {
				// give it a second chance; match with defaultVariationName
				for _, m := range dep.module.splitModules {
					if m.variant[mutatorName] == *defaultVariationName {
						newDep = m
						break
					}
				}
			}
			if newDep == nil {
				errs = append(errs, &BlueprintError{
					Err: fmt.Errorf("failed to find variation %q for module %q needed by %q",
						variationName, dep.module.Name(), module.Name()),
					Pos: module.pos,
				})
				continue
			}
			module.directDeps[i].module = newDep
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

func (c *Context) newModule(factory ModuleFactory) *moduleInfo {
	logicModule, properties := factory()

	module := &moduleInfo{
		logicModule: logicModule,
		factory:     factory,
	}

	module.properties = properties

	return module
}

func (c *Context) processModuleDef(moduleDef *parser.Module,
	relBlueprintsFile string) (*moduleInfo, []error) {

	factory, ok := c.moduleFactories[moduleDef.Type]
	if !ok {
		if c.ignoreUnknownModuleTypes {
			return nil, nil
		}

		return nil, []error{
			&BlueprintError{
				Err: fmt.Errorf("unrecognized module type %q", moduleDef.Type),
				Pos: moduleDef.TypePos,
			},
		}
	}

	module := c.newModule(factory)
	module.typeName = moduleDef.Type

	module.relBlueprintsFile = relBlueprintsFile

	propertyMap, errs := unpackProperties(moduleDef.Properties, module.properties...)
	if len(errs) > 0 {
		return nil, errs
	}

	module.pos = moduleDef.TypePos
	module.propertyPos = make(map[string]scanner.Position)
	for name, propertyDef := range propertyMap {
		module.propertyPos[name] = propertyDef.ColonPos
	}

	return module, nil
}

func (c *Context) addModule(module *moduleInfo) []error {
	name := module.logicModule.Name()
	if name == "" {
		return []error{
			&BlueprintError{
				Err: fmt.Errorf("property 'name' is missing from a module"),
				Pos: module.pos,
			},
		}
	}
	c.moduleInfo[module.logicModule] = module

	group := &moduleGroup{
		name:    name,
		modules: []*moduleInfo{module},
	}
	module.group = group
	namespace, errs := c.nameInterface.NewModule(
		newNamespaceContext(module),
		ModuleGroup{moduleGroup: group},
		module.logicModule)
	if len(errs) > 0 {
		for i := range errs {
			errs[i] = &BlueprintError{Err: errs[i], Pos: module.pos}
		}
		return errs
	}
	group.namespace = namespace

	c.moduleGroups = append(c.moduleGroups, group)

	return nil
}

// ResolveDependencies checks that the dependencies specified by all of the
// modules defined in the parsed Blueprints files are valid.  This means that
// the modules depended upon are defined and that no circular dependencies
// exist.
func (c *Context) ResolveDependencies(config interface{}) (deps []string, errs []error) {
	return c.resolveDependencies(c.Context, config)
}

func (c *Context) resolveDependencies(ctx context.Context, config interface{}) (deps []string, errs []error) {
	pprof.Do(ctx, pprof.Labels("blueprint", "ResolveDependencies"), func(ctx context.Context) {
		c.liveGlobals = newLiveTracker(config)

		deps, errs = c.generateSingletonBuildActions(config, c.preSingletonInfo, c.liveGlobals)
		if len(errs) > 0 {
			return
		}

		errs = c.updateDependencies()
		if len(errs) > 0 {
			return
		}

		var mutatorDeps []string
		mutatorDeps, errs = c.runMutators(ctx, config)
		if len(errs) > 0 {
			return
		}
		deps = append(deps, mutatorDeps...)

		c.cloneModules()

		c.dependenciesReady = true
	})

	if len(errs) > 0 {
		return nil, errs
	}

	return deps, nil
}

// Default dependencies handling.  If the module implements the (deprecated)
// DynamicDependerModule interface then this set consists of the union of those
// module names returned by its DynamicDependencies method and those added by calling
// AddDependencies or AddVariationDependencies on DynamicDependencyModuleContext.
func blueprintDepsMutator(ctx BottomUpMutatorContext) {
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

			ctx.AddDependency(ctx.Module(), nil, dynamicDeps...)
		}()
	}
}

// findMatchingVariant searches the moduleGroup for a module with the same variant as module,
// and returns the matching module, or nil if one is not found.
func (c *Context) findMatchingVariant(module *moduleInfo, possible []*moduleInfo) *moduleInfo {
	if len(possible) == 1 {
		return possible[0]
	} else {
		for _, m := range possible {
			if m.variant.equal(module.dependencyVariant) {
				return m
			}
		}
	}

	return nil
}

func (c *Context) addDependency(module *moduleInfo, tag DependencyTag, depName string) []error {
	if _, ok := tag.(BaseDependencyTag); ok {
		panic("BaseDependencyTag is not allowed to be used directly!")
	}

	if depName == module.Name() {
		return []error{&BlueprintError{
			Err: fmt.Errorf("%q depends on itself", depName),
			Pos: module.pos,
		}}
	}

	possibleDeps := c.modulesFromName(depName, module.namespace())
	if possibleDeps == nil {
		return c.discoveredMissingDependencies(module, depName)
	}

	if m := c.findMatchingVariant(module, possibleDeps); m != nil {
		module.directDeps = append(module.directDeps, depInfo{m, tag})
		atomic.AddUint32(&c.depsModified, 1)
		return nil
	}

	variants := make([]string, len(possibleDeps))
	for i, mod := range possibleDeps {
		variants[i] = c.prettyPrintVariant(mod.variant)
	}
	sort.Strings(variants)

	return []error{&BlueprintError{
		Err: fmt.Errorf("dependency %q of %q missing variant:\n  %s\navailable variants:\n  %s",
			depName, module.Name(),
			c.prettyPrintVariant(module.dependencyVariant),
			strings.Join(variants, "\n  ")),
		Pos: module.pos,
	}}
}

func (c *Context) findReverseDependency(module *moduleInfo, destName string) (*moduleInfo, []error) {
	if destName == module.Name() {
		return nil, []error{&BlueprintError{
			Err: fmt.Errorf("%q depends on itself", destName),
			Pos: module.pos,
		}}
	}

	possibleDeps := c.modulesFromName(destName, module.namespace())
	if possibleDeps == nil {
		return nil, []error{&BlueprintError{
			Err: fmt.Errorf("%q has a reverse dependency on undefined module %q",
				module.Name(), destName),
			Pos: module.pos,
		}}
	}

	if m := c.findMatchingVariant(module, possibleDeps); m != nil {
		return m, nil
	}

	variants := make([]string, len(possibleDeps))
	for i, mod := range possibleDeps {
		variants[i] = c.prettyPrintVariant(mod.variant)
	}
	sort.Strings(variants)

	return nil, []error{&BlueprintError{
		Err: fmt.Errorf("reverse dependency %q of %q missing variant:\n  %s\navailable variants:\n  %s",
			destName, module.Name(),
			c.prettyPrintVariant(module.dependencyVariant),
			strings.Join(variants, "\n  ")),
		Pos: module.pos,
	}}
}

func (c *Context) addVariationDependency(module *moduleInfo, variations []Variation,
	tag DependencyTag, depName string, far bool) []error {
	if _, ok := tag.(BaseDependencyTag); ok {
		panic("BaseDependencyTag is not allowed to be used directly!")
	}

	possibleDeps := c.modulesFromName(depName, module.namespace())
	if possibleDeps == nil {
		return c.discoveredMissingDependencies(module, depName)
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

	for _, m := range possibleDeps {
		var found bool
		if far {
			found = m.variant.subset(newVariant)
		} else {
			found = m.variant.equal(newVariant)
		}
		if found {
			if module == m {
				return []error{&BlueprintError{
					Err: fmt.Errorf("%q depends on itself", depName),
					Pos: module.pos,
				}}
			}
			// AddVariationDependency allows adding a dependency on itself, but only if
			// that module is earlier in the module list than this one, since we always
			// run GenerateBuildActions in order for the variants of a module
			if m.group == module.group && beforeInModuleList(module, m, module.group.modules) {
				return []error{&BlueprintError{
					Err: fmt.Errorf("%q depends on later version of itself", depName),
					Pos: module.pos,
				}}
			}
			module.directDeps = append(module.directDeps, depInfo{m, tag})
			atomic.AddUint32(&c.depsModified, 1)
			return nil
		}
	}

	variants := make([]string, len(possibleDeps))
	for i, mod := range possibleDeps {
		variants[i] = c.prettyPrintVariant(mod.variant)
	}
	sort.Strings(variants)

	return []error{&BlueprintError{
		Err: fmt.Errorf("dependency %q of %q missing variant:\n  %s\navailable variants:\n  %s",
			depName, module.Name(),
			c.prettyPrintVariant(newVariant),
			strings.Join(variants, "\n  ")),
		Pos: module.pos,
	}}
}

func (c *Context) addInterVariantDependency(origModule *moduleInfo, tag DependencyTag,
	from, to Module) {
	if _, ok := tag.(BaseDependencyTag); ok {
		panic("BaseDependencyTag is not allowed to be used directly!")
	}

	var fromInfo, toInfo *moduleInfo
	for _, m := range origModule.splitModules {
		if m.logicModule == from {
			fromInfo = m
		}
		if m.logicModule == to {
			toInfo = m
			if fromInfo != nil {
				panic(fmt.Errorf("%q depends on later version of itself", origModule.Name()))
			}
		}
	}

	if fromInfo == nil || toInfo == nil {
		panic(fmt.Errorf("AddInterVariantDependency called for module %q on invalid variant",
			origModule.Name()))
	}

	fromInfo.directDeps = append(fromInfo.directDeps, depInfo{toInfo, tag})
	atomic.AddUint32(&c.depsModified, 1)
}

// findBlueprintDescendants returns a map linking parent Blueprints files to child Blueprints files
// For example, if paths = []string{"a/b/c/Android.bp", "a/Blueprints"},
// then descendants = {"":[]string{"a/Blueprints"}, "a/Blueprints":[]string{"a/b/c/Android.bp"}}
func findBlueprintDescendants(paths []string) (descendants map[string][]string, err error) {
	// make mapping from dir path to file path
	filesByDir := make(map[string]string, len(paths))
	for _, path := range paths {
		dir := filepath.Dir(path)
		_, alreadyFound := filesByDir[dir]
		if alreadyFound {
			return nil, fmt.Errorf("Found two Blueprint files in directory %v : %v and %v", dir, filesByDir[dir], path)
		}
		filesByDir[dir] = path
	}

	findAncestor := func(childFile string) (ancestor string) {
		prevAncestorDir := filepath.Dir(childFile)
		for {
			ancestorDir := filepath.Dir(prevAncestorDir)
			if ancestorDir == prevAncestorDir {
				// reached the root dir without any matches; assign this as a descendant of ""
				return ""
			}

			ancestorFile, ancestorExists := filesByDir[ancestorDir]
			if ancestorExists {
				return ancestorFile
			}
			prevAncestorDir = ancestorDir
		}
	}
	// generate the descendants map
	descendants = make(map[string][]string, len(filesByDir))
	for _, childFile := range filesByDir {
		ancestorFile := findAncestor(childFile)
		descendants[ancestorFile] = append(descendants[ancestorFile], childFile)
	}
	return descendants, nil
}

type visitOrderer interface {
	// returns the number of modules that this module needs to wait for
	waitCount(module *moduleInfo) int
	// returns the list of modules that are waiting for this module
	propagate(module *moduleInfo) []*moduleInfo
	// visit modules in order
	visit(modules []*moduleInfo, visit func(*moduleInfo) bool)
}

type unorderedVisitorImpl struct{}

func (unorderedVisitorImpl) waitCount(module *moduleInfo) int {
	return 0
}

func (unorderedVisitorImpl) propagate(module *moduleInfo) []*moduleInfo {
	return nil
}

func (unorderedVisitorImpl) visit(modules []*moduleInfo, visit func(*moduleInfo) bool) {
	for _, module := range modules {
		if visit(module) {
			return
		}
	}
}

type bottomUpVisitorImpl struct{}

func (bottomUpVisitorImpl) waitCount(module *moduleInfo) int {
	return len(module.forwardDeps)
}

func (bottomUpVisitorImpl) propagate(module *moduleInfo) []*moduleInfo {
	return module.reverseDeps
}

func (bottomUpVisitorImpl) visit(modules []*moduleInfo, visit func(*moduleInfo) bool) {
	for _, module := range modules {
		if visit(module) {
			return
		}
	}
}

type topDownVisitorImpl struct{}

func (topDownVisitorImpl) waitCount(module *moduleInfo) int {
	return len(module.reverseDeps)
}

func (topDownVisitorImpl) propagate(module *moduleInfo) []*moduleInfo {
	return module.forwardDeps
}

func (topDownVisitorImpl) visit(modules []*moduleInfo, visit func(*moduleInfo) bool) {
	for i := 0; i < len(modules); i++ {
		module := modules[len(modules)-1-i]
		if visit(module) {
			return
		}
	}
}

var (
	bottomUpVisitor bottomUpVisitorImpl
	topDownVisitor  topDownVisitorImpl
)

// Calls visit on each module, guaranteeing that visit is not called on a module until visit on all
// of its dependencies has finished.
func (c *Context) parallelVisit(order visitOrderer, visit func(group *moduleInfo) bool) {
	doneCh := make(chan *moduleInfo)
	cancelCh := make(chan bool)
	count := 0
	cancel := false
	var backlog []*moduleInfo
	const limit = 1000

	for _, module := range c.modulesSorted {
		module.waitingCount = order.waitCount(module)
	}

	visitOne := func(module *moduleInfo) {
		if count < limit {
			count++
			go func() {
				ret := visit(module)
				if ret {
					cancelCh <- true
				}
				doneCh <- module
			}()
		} else {
			backlog = append(backlog, module)
		}
	}

	for _, module := range c.modulesSorted {
		if module.waitingCount == 0 {
			visitOne(module)
		}
	}

	for count > 0 || len(backlog) > 0 {
		select {
		case <-cancelCh:
			cancel = true
			backlog = nil
		case doneModule := <-doneCh:
			count--
			if !cancel {
				for count < limit && len(backlog) > 0 {
					toVisit := backlog[0]
					backlog = backlog[1:]
					visitOne(toVisit)
				}
				for _, module := range order.propagate(doneModule) {
					module.waitingCount--
					if module.waitingCount == 0 {
						visitOne(module)
					}
				}
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
	visited := make(map[*moduleInfo]bool)  // modules that were already checked
	checking := make(map[*moduleInfo]bool) // modules actively being checked

	sorted := make([]*moduleInfo, 0, len(c.moduleInfo))

	var check func(group *moduleInfo) []*moduleInfo

	cycleError := func(cycle []*moduleInfo) {
		// We are the "start" of the cycle, so we're responsible
		// for generating the errors.  The cycle list is in
		// reverse order because all the 'check' calls append
		// their own module to the list.
		errs = append(errs, &BlueprintError{
			Err: fmt.Errorf("encountered dependency cycle:"),
			Pos: cycle[len(cycle)-1].pos,
		})

		// Iterate backwards through the cycle list.
		curModule := cycle[0]
		for i := len(cycle) - 1; i >= 0; i-- {
			nextModule := cycle[i]
			errs = append(errs, &BlueprintError{
				Err: fmt.Errorf("    %q depends on %q",
					curModule.Name(),
					nextModule.Name()),
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
			deps[dep.module] = true
		}

		module.reverseDeps = []*moduleInfo{}
		module.forwardDeps = []*moduleInfo{}

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

			module.forwardDeps = append(module.forwardDeps, dep)
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
	pprof.Do(c.Context, pprof.Labels("blueprint", "PrepareBuildActions"), func(ctx context.Context) {
		c.buildActionsReady = false

		if !c.dependenciesReady {
			var extraDeps []string
			extraDeps, errs = c.resolveDependencies(ctx, config)
			if len(errs) > 0 {
				return
			}
			deps = append(deps, extraDeps...)
		}

		var depsModules []string
		depsModules, errs = c.generateModuleBuildActions(config, c.liveGlobals)
		if len(errs) > 0 {
			return
		}

		var depsSingletons []string
		depsSingletons, errs = c.generateSingletonBuildActions(config, c.singletonInfo, c.liveGlobals)
		if len(errs) > 0 {
			return
		}

		deps = append(deps, depsModules...)
		deps = append(deps, depsSingletons...)

		if c.ninjaBuildDir != nil {
			err := c.liveGlobals.addNinjaStringDeps(c.ninjaBuildDir)
			if err != nil {
				errs = []error{err}
				return
			}
		}

		pkgNames, depsPackages := c.makeUniquePackageNames(c.liveGlobals)

		deps = append(deps, depsPackages...)

		// This will panic if it finds a problem since it's a programming error.
		c.checkForVariableReferenceCycles(c.liveGlobals.variables, pkgNames)

		c.pkgNames = pkgNames
		c.globalVariables = c.liveGlobals.variables
		c.globalPools = c.liveGlobals.pools
		c.globalRules = c.liveGlobals.rules

		c.buildActionsReady = true
	})

	if len(errs) > 0 {
		return nil, errs
	}

	return deps, nil
}

func (c *Context) runMutators(ctx context.Context, config interface{}) (deps []string, errs []error) {
	var mutators []*mutatorInfo

	pprof.Do(ctx, pprof.Labels("blueprint", "runMutators"), func(ctx context.Context) {
		mutators = append(mutators, c.earlyMutatorInfo...)
		mutators = append(mutators, c.mutatorInfo...)

		for _, mutator := range mutators {
			pprof.Do(ctx, pprof.Labels("mutator", mutator.name), func(context.Context) {
				var newDeps []string
				if mutator.topDownMutator != nil {
					newDeps, errs = c.runMutator(config, mutator, topDownMutator)
				} else if mutator.bottomUpMutator != nil {
					newDeps, errs = c.runMutator(config, mutator, bottomUpMutator)
				} else {
					panic("no mutator set on " + mutator.name)
				}
				if len(errs) > 0 {
					return
				}
				deps = append(deps, newDeps...)
			})
			if len(errs) > 0 {
				return
			}
		}
	})

	if len(errs) > 0 {
		return nil, errs
	}

	return deps, nil
}

type mutatorDirection interface {
	run(mutator *mutatorInfo, ctx *mutatorContext)
	orderer() visitOrderer
	fmt.Stringer
}

type bottomUpMutatorImpl struct{}

func (bottomUpMutatorImpl) run(mutator *mutatorInfo, ctx *mutatorContext) {
	mutator.bottomUpMutator(ctx)
}

func (bottomUpMutatorImpl) orderer() visitOrderer {
	return bottomUpVisitor
}

func (bottomUpMutatorImpl) String() string {
	return "bottom up mutator"
}

type topDownMutatorImpl struct{}

func (topDownMutatorImpl) run(mutator *mutatorInfo, ctx *mutatorContext) {
	mutator.topDownMutator(ctx)
}

func (topDownMutatorImpl) orderer() visitOrderer {
	return topDownVisitor
}

func (topDownMutatorImpl) String() string {
	return "top down mutator"
}

var (
	topDownMutator  topDownMutatorImpl
	bottomUpMutator bottomUpMutatorImpl
)

type reverseDep struct {
	module *moduleInfo
	dep    depInfo
}

func (c *Context) runMutator(config interface{}, mutator *mutatorInfo,
	direction mutatorDirection) (deps []string, errs []error) {

	newModuleInfo := make(map[Module]*moduleInfo)
	for k, v := range c.moduleInfo {
		newModuleInfo[k] = v
	}

	type globalStateChange struct {
		reverse    []reverseDep
		rename     []rename
		replace    []replace
		newModules []*moduleInfo
		deps       []string
	}

	reverseDeps := make(map[*moduleInfo][]depInfo)
	var rename []rename
	var replace []replace
	var newModules []*moduleInfo

	errsCh := make(chan []error)
	globalStateCh := make(chan globalStateChange)
	newVariationsCh := make(chan []*moduleInfo)
	done := make(chan bool)

	c.depsModified = 0

	visit := func(module *moduleInfo) bool {
		if module.splitModules != nil {
			panic("split module found in sorted module list")
		}

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
					in := fmt.Sprintf("%s %q for %s", direction, mutator.name, module)
					if err, ok := r.(panicError); ok {
						err.addIn(in)
						mctx.error(err)
					} else {
						mctx.error(newPanicErrorf(r, in))
					}
				}
			}()
			direction.run(mutator, mctx)
		}()

		if len(mctx.errs) > 0 {
			errsCh <- mctx.errs
			return true
		}

		if len(mctx.newVariations) > 0 {
			newVariationsCh <- mctx.newVariations
		}

		if len(mctx.reverseDeps) > 0 || len(mctx.replace) > 0 || len(mctx.rename) > 0 || len(mctx.newModules) > 0 {
			globalStateCh <- globalStateChange{
				reverse:    mctx.reverseDeps,
				replace:    mctx.replace,
				rename:     mctx.rename,
				newModules: mctx.newModules,
				deps:       mctx.ninjaFileDeps,
			}
		}

		return false
	}

	// Process errs and reverseDeps in a single goroutine
	go func() {
		for {
			select {
			case newErrs := <-errsCh:
				errs = append(errs, newErrs...)
			case globalStateChange := <-globalStateCh:
				for _, r := range globalStateChange.reverse {
					reverseDeps[r.module] = append(reverseDeps[r.module], r.dep)
				}
				replace = append(replace, globalStateChange.replace...)
				rename = append(rename, globalStateChange.rename...)
				newModules = append(newModules, globalStateChange.newModules...)
				deps = append(deps, globalStateChange.deps...)
			case newVariations := <-newVariationsCh:
				for _, m := range newVariations {
					newModuleInfo[m.logicModule] = m
				}
			case <-done:
				return
			}
		}
	}()

	if mutator.parallel {
		c.parallelVisit(direction.orderer(), visit)
	} else {
		direction.orderer().visit(c.modulesSorted, visit)
	}

	done <- true

	if len(errs) > 0 {
		return nil, errs
	}

	c.moduleInfo = newModuleInfo

	for _, group := range c.moduleGroups {
		for i := 0; i < len(group.modules); i++ {
			module := group.modules[i]

			// Update module group to contain newly split variants
			if module.splitModules != nil {
				group.modules, i = spliceModules(group.modules, i, module.splitModules)
			}

			// Fix up any remaining dependencies on modules that were split into variants
			// by replacing them with the first variant
			for j, dep := range module.directDeps {
				if dep.module.logicModule == nil {
					module.directDeps[j].module = dep.module.splitModules[0]
				}
			}
		}
	}

	for module, deps := range reverseDeps {
		sort.Sort(depSorter(deps))
		module.directDeps = append(module.directDeps, deps...)
		c.depsModified++
	}

	for _, module := range newModules {
		errs = c.addModule(module)
		if len(errs) > 0 {
			return nil, errs
		}
		atomic.AddUint32(&c.depsModified, 1)
	}

	errs = c.handleRenames(rename)
	if len(errs) > 0 {
		return nil, errs
	}

	errs = c.handleReplacements(replace)
	if len(errs) > 0 {
		return nil, errs
	}

	if c.depsModified > 0 {
		errs = c.updateDependencies()
		if len(errs) > 0 {
			return nil, errs
		}
	}

	return deps, errs
}

// Replaces every build logic module with a clone of itself.  Prevents introducing problems where
// a mutator sets a non-property member variable on a module, which works until a later mutator
// creates variants of that module.
func (c *Context) cloneModules() {
	type update struct {
		orig  Module
		clone *moduleInfo
	}
	ch := make(chan update)
	doneCh := make(chan bool)
	go func() {
		c.parallelVisit(unorderedVisitorImpl{}, func(m *moduleInfo) bool {
			origLogicModule := m.logicModule
			m.logicModule, m.properties = c.cloneLogicModule(m)
			ch <- update{origLogicModule, m}
			return false
		})
		doneCh <- true
	}()

	done := false
	for !done {
		select {
		case <-doneCh:
			done = true
		case update := <-ch:
			delete(c.moduleInfo, update.orig)
			c.moduleInfo[update.clone.logicModule] = update.clone
		}
	}
}

// Removes modules[i] from the list and inserts newModules... where it was located, returning
// the new slice and the index of the last inserted element
func spliceModules(modules []*moduleInfo, i int, newModules []*moduleInfo) ([]*moduleInfo, int) {
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

	return dest, i + spliceSize - 1
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

	c.parallelVisit(bottomUpVisitor, func(module *moduleInfo) bool {

		uniqueName := c.nameInterface.UniqueName(newNamespaceContext(module), module.group.name)
		sanitizedName := toNinjaName(uniqueName)

		prefix := moduleNamespacePrefix(sanitizedName + "_" + module.variantName)

		// The parent scope of the moduleContext's local scope gets overridden to be that of the
		// calling Go package on a per-call basis.  Since the initial parent scope doesn't matter we
		// just set it to nil.
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
				errs = append(errs, c.missingDependencyError(module, depName))
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
	singletons []*singletonInfo, liveGlobals *liveTracker) ([]string, []error) {

	var deps []string
	var errs []error

	for _, info := range singletons {
		// The parent scope of the singletonContext's local scope gets overridden to be that of the
		// calling Go package on a per-call basis.  Since the initial parent scope doesn't matter we
		// just set it to nil.
		scope := newLocalScope(nil, singletonNamespacePrefix(info.name))

		sctx := &singletonContext{
			name:    info.name,
			context: c,
			config:  config,
			scope:   scope,
			globals: liveGlobals,
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

func (c *Context) walkDeps(topModule *moduleInfo, allowDuplicates bool,
	visitDown func(depInfo, *moduleInfo) bool, visitUp func(depInfo, *moduleInfo)) {

	visited := make(map[*moduleInfo]bool)
	var visiting *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "WalkDeps(%s, %s, %s) for dependency %s",
				topModule, funcName(visitDown), funcName(visitUp), visiting))
		}
	}()

	var walk func(module *moduleInfo)
	walk = func(module *moduleInfo) {
		for _, dep := range module.directDeps {
			if allowDuplicates || !visited[dep.module] {
				visiting = dep.module
				recurse := true
				if visitDown != nil {
					recurse = visitDown(dep, module)
				}
				if recurse && !visited[dep.module] {
					walk(dep.module)
				}
				visited[dep.module] = true
				if visitUp != nil {
					visitUp(dep, module)
				}
			}
		}
	}

	walk(topModule)
}

type replace struct {
	from, to *moduleInfo
}

type rename struct {
	group *moduleGroup
	name  string
}

func (c *Context) moduleMatchingVariant(module *moduleInfo, name string) *moduleInfo {
	targets := c.modulesFromName(name, module.namespace())

	if targets == nil {
		return nil
	}

	for _, m := range targets {
		if module.variantName == m.variantName {
			return m
		}
	}

	return nil
}

func (c *Context) handleRenames(renames []rename) []error {
	var errs []error
	for _, rename := range renames {
		group, name := rename.group, rename.name
		if name == group.name || len(group.modules) < 1 {
			continue
		}

		errs = append(errs, c.nameInterface.Rename(group.name, rename.name, group.namespace)...)
	}

	return errs
}

func (c *Context) handleReplacements(replacements []replace) []error {
	var errs []error
	for _, replace := range replacements {
		for _, m := range replace.from.reverseDeps {
			for i, d := range m.directDeps {
				if d.module == replace.from {
					m.directDeps[i].module = replace.to
				}
			}
		}

		atomic.AddUint32(&c.depsModified, 1)
	}

	return errs
}

func (c *Context) discoveredMissingDependencies(module *moduleInfo, depName string) (errs []error) {
	if c.allowMissingDependencies {
		module.missingDeps = append(module.missingDeps, depName)
		return nil
	}
	return []error{c.missingDependencyError(module, depName)}
}

func (c *Context) missingDependencyError(module *moduleInfo, depName string) (errs error) {
	err := c.nameInterface.MissingDependencyError(module.Name(), module.namespace(), depName)

	return &BlueprintError{
		Err: err,
		Pos: module.pos,
	}
}

func (c *Context) modulesFromName(name string, namespace Namespace) []*moduleInfo {
	group, exists := c.nameInterface.ModuleFromName(name, namespace)
	if exists {
		return group.modules
	}
	return nil
}

func (c *Context) sortedModuleGroups() []*moduleGroup {
	if c.cachedSortedModuleGroups == nil {
		unwrap := func(wrappers []ModuleGroup) []*moduleGroup {
			result := make([]*moduleGroup, 0, len(wrappers))
			for _, group := range wrappers {
				result = append(result, group.moduleGroup)
			}
			return result
		}

		c.cachedSortedModuleGroups = unwrap(c.nameInterface.AllModules())
	}

	return c.cachedSortedModuleGroups
}

func (c *Context) visitAllModules(visit func(Module)) {
	var module *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitAllModules(%s) for %s",
				funcName(visit), module))
		}
	}()

	for _, moduleGroup := range c.sortedModuleGroups() {
		for _, module = range moduleGroup.modules {
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

	for _, moduleGroup := range c.sortedModuleGroups() {
		for _, module := range moduleGroup.modules {
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
			for _, output := range append(buildDef.Outputs, buildDef.ImplicitOutputs...) {
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
			for _, output := range append(buildDef.Outputs, buildDef.ImplicitOutputs...) {
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

func (c *Context) ModuleTypeFactories() map[string]ModuleFactory {
	ret := make(map[string]ModuleFactory)
	for k, v := range c.moduleFactories {
		ret[k] = v
	}
	return ret
}

func (c *Context) ModuleName(logicModule Module) string {
	module := c.moduleInfo[logicModule]
	return module.Name()
}

func (c *Context) ModulePath(logicModule Module) string {
	module := c.moduleInfo[logicModule]
	return module.relBlueprintsFile
}

func (c *Context) ModuleDir(logicModule Module) string {
	return filepath.Dir(c.ModulePath(logicModule))
}

func (c *Context) ModuleSubDir(logicModule Module) string {
	module := c.moduleInfo[logicModule]
	return module.variantName
}

func (c *Context) ModuleType(logicModule Module) string {
	module := c.moduleInfo[logicModule]
	return module.typeName
}

func (c *Context) BlueprintFile(logicModule Module) string {
	module := c.moduleInfo[logicModule]
	return module.relBlueprintsFile
}

func (c *Context) ModuleErrorf(logicModule Module, format string,
	args ...interface{}) error {

	module := c.moduleInfo[logicModule]
	return &BlueprintError{
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

func (c *Context) VisitDirectDeps(module Module, visit func(Module)) {
	topModule := c.moduleInfo[module]

	var visiting *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitDirectDeps(%s, %s) for dependency %s",
				topModule, funcName(visit), visiting))
		}
	}()

	for _, dep := range topModule.directDeps {
		visiting = dep.module
		visit(dep.module.logicModule)
	}
}

func (c *Context) VisitDirectDepsIf(module Module, pred func(Module) bool, visit func(Module)) {
	topModule := c.moduleInfo[module]

	var visiting *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitDirectDepsIf(%s, %s, %s) for dependency %s",
				topModule, funcName(pred), funcName(visit), visiting))
		}
	}()

	for _, dep := range topModule.directDeps {
		visiting = dep.module
		if pred(dep.module.logicModule) {
			visit(dep.module.logicModule)
		}
	}
}

func (c *Context) VisitDepsDepthFirst(module Module, visit func(Module)) {
	topModule := c.moduleInfo[module]

	var visiting *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitDepsDepthFirst(%s, %s) for dependency %s",
				topModule, funcName(visit), visiting))
		}
	}()

	c.walkDeps(topModule, false, nil, func(dep depInfo, parent *moduleInfo) {
		visiting = dep.module
		visit(dep.module.logicModule)
	})
}

func (c *Context) VisitDepsDepthFirstIf(module Module, pred func(Module) bool, visit func(Module)) {
	topModule := c.moduleInfo[module]

	var visiting *moduleInfo

	defer func() {
		if r := recover(); r != nil {
			panic(newPanicErrorf(r, "VisitDepsDepthFirstIf(%s, %s, %s) for dependency %s",
				topModule, funcName(pred), funcName(visit), visiting))
		}
	}()

	c.walkDeps(topModule, false, nil, func(dep depInfo, parent *moduleInfo) {
		if pred(dep.module.logicModule) {
			visiting = dep.module
			visit(dep.module.logicModule)
		}
	})
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

// Singletons returns a list of all registered Singletons.
func (c *Context) Singletons() []Singleton {
	var ret []Singleton
	for _, s := range c.singletonInfo {
		ret = append(ret, s.singleton)
	}
	return ret
}

// SingletonName returns the name that the given singleton was registered with.
func (c *Context) SingletonName(singleton Singleton) string {
	for _, s := range c.singletonInfo {
		if s.singleton == singleton {
			return s.name
		}
	}
	return ""
}

// WriteBuildFile writes the Ninja manifeset text for the generated build
// actions to w.  If this is called before PrepareBuildActions successfully
// completes then ErrBuildActionsNotReady is returned.
func (c *Context) WriteBuildFile(w io.Writer) error {
	var err error
	pprof.Do(c.Context, pprof.Labels("blueprint", "WriteBuildFile"), func(ctx context.Context) {
		if !c.buildActionsReady {
			err = ErrBuildActionsNotReady
			return
		}

		nw := newNinjaWriter(w)

		err = c.writeBuildFileHeader(nw)
		if err != nil {
			return
		}

		err = c.writeNinjaRequiredVersion(nw)
		if err != nil {
			return
		}

		err = c.writeSubninjas(nw)
		if err != nil {
			return
		}

		// TODO: Group the globals by package.

		err = c.writeGlobalVariables(nw)
		if err != nil {
			return
		}

		err = c.writeGlobalPools(nw)
		if err != nil {
			return
		}

		err = c.writeBuildDir(nw)
		if err != nil {
			return
		}

		err = c.writeGlobalRules(nw)
		if err != nil {
			return
		}

		err = c.writeAllModuleActions(nw)
		if err != nil {
			return
		}

		err = c.writeAllSingletonActions(nw)
		if err != nil {
			return
		}
	})

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

func (c *Context) writeSubninjas(nw *ninjaWriter) error {
	for _, subninja := range c.subninjas {
		err := nw.Subninja(subninja)
		if err != nil {
			return err
		}
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

type depSorter []depInfo

func (s depSorter) Len() int {
	return len(s)
}

func (s depSorter) Less(i, j int) bool {
	iName := s[i].module.Name()
	jName := s[j].module.Name()
	if iName == jName {
		iName = s[i].module.variantName
		jName = s[j].module.variantName
	}
	return iName < jName
}

func (s depSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type moduleSorter struct {
	modules       []*moduleInfo
	nameInterface NameInterface
}

func (s moduleSorter) Len() int {
	return len(s.modules)
}

func (s moduleSorter) Less(i, j int) bool {
	iMod := s.modules[i]
	jMod := s.modules[j]
	iName := s.nameInterface.UniqueName(newNamespaceContext(iMod), iMod.group.name)
	jName := s.nameInterface.UniqueName(newNamespaceContext(jMod), jMod.group.name)
	if iName == jName {
		iName = s.modules[i].variantName
		jName = s.modules[j].variantName
	}

	if iName == jName {
		panic(fmt.Sprintf("duplicate module name: %s: %#v and %#v\n", iName, iMod, jMod))
	}
	return iName < jName
}

func (s moduleSorter) Swap(i, j int) {
	s.modules[i], s.modules[j] = s.modules[j], s.modules[i]
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
	sort.Sort(moduleSorter{modules, c.nameInterface})

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
		factoryFunc := runtime.FuncForPC(reflect.ValueOf(module.factory).Pointer())
		factoryName := factoryFunc.Name()

		infoMap := map[string]interface{}{
			"name":      module.Name(),
			"typeName":  module.typeName,
			"goFactory": factoryName,
			"pos":       relPos,
			"variant":   module.variantName,
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
Module:  {{.name}}
Variant: {{.variant}}
Type:    {{.typeName}}
Factory: {{.goFactory}}
Defined: {{.pos}}
`

var singletonHeaderTemplate = `# # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # # 
Singleton: {{.name}}
Factory:   {{.goFactory}}
`
