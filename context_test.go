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
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/blueprint/parser"
)

type Walker interface {
	Walk() bool
}

func walkDependencyGraph(ctx *Context, topModule *moduleInfo, allowDuplicates bool) (string, string) {
	var outputDown string
	var outputUp string
	ctx.walkDeps(topModule, allowDuplicates,
		func(dep depInfo, parent *moduleInfo) bool {
			outputDown += ctx.ModuleName(dep.module.logicModule)
			if tag, ok := dep.tag.(walkerDepsTag); ok {
				if !tag.follow {
					return false
				}
			}
			if dep.module.logicModule.(Walker).Walk() {
				return true
			}

			return false
		},
		func(dep depInfo, parent *moduleInfo) {
			outputUp += ctx.ModuleName(dep.module.logicModule)
		})
	return outputDown, outputUp
}

type depsProvider interface {
	Deps() []string
	IgnoreDeps() []string
}

type fooModule struct {
	SimpleName
	properties struct {
		Deps         []string
		Ignored_deps []string
		Foo          string
	}
}

func newFooModule() (Module, []interface{}) {
	m := &fooModule{}
	return m, []interface{}{&m.properties, &m.SimpleName.Properties}
}

func (f *fooModule) GenerateBuildActions(ModuleContext) {
}

func (f *fooModule) Deps() []string {
	return f.properties.Deps
}

func (f *fooModule) IgnoreDeps() []string {
	return f.properties.Ignored_deps
}

func (f *fooModule) Foo() string {
	return f.properties.Foo
}

func (f *fooModule) Walk() bool {
	return true
}

type barModule struct {
	SimpleName
	properties struct {
		Deps         []string
		Ignored_deps []string
		Bar          bool
	}
}

func newBarModule() (Module, []interface{}) {
	m := &barModule{}
	return m, []interface{}{&m.properties, &m.SimpleName.Properties}
}

func (b *barModule) Deps() []string {
	return b.properties.Deps
}

func (b *barModule) IgnoreDeps() []string {
	return b.properties.Ignored_deps
}

func (b *barModule) GenerateBuildActions(ModuleContext) {
}

func (b *barModule) Bar() bool {
	return b.properties.Bar
}

func (b *barModule) Walk() bool {
	return false
}

type walkerDepsTag struct {
	BaseDependencyTag
	// True if the dependency should be followed, false otherwise.
	follow bool
}

func depsMutator(mctx BottomUpMutatorContext) {
	if m, ok := mctx.Module().(depsProvider); ok {
		mctx.AddDependency(mctx.Module(), walkerDepsTag{follow: false}, m.IgnoreDeps()...)
		mctx.AddDependency(mctx.Module(), walkerDepsTag{follow: true}, m.Deps()...)
	}
}

func TestContextParse(t *testing.T) {
	ctx := NewContext()
	ctx.RegisterModuleType("foo_module", newFooModule)
	ctx.RegisterModuleType("bar_module", newBarModule)

	r := bytes.NewBufferString(`
		foo_module {
	        name: "MyFooModule",
			deps: ["MyBarModule"],
		}

		bar_module {
	        name: "MyBarModule",
		}
	`)

	_, _, errs := ctx.parseOne(".", "Blueprint", r, parser.NewScope(nil), nil)
	if len(errs) > 0 {
		t.Errorf("unexpected parse errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	_, errs = ctx.ResolveDependencies(nil)
	if len(errs) > 0 {
		t.Errorf("unexpected dep errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}
}

// |===B---D       - represents a non-walkable edge
// A               = represents a walkable edge
// |===C===E---G
//     |       |   A should not be visited because it's the root node.
//     |===F===|   B, D and E should not be walked.
func TestWalkDeps(t *testing.T) {
	ctx := NewContext()
	ctx.MockFileSystem(map[string][]byte{
		"Blueprints": []byte(`
			foo_module {
			    name: "A",
			    deps: ["B", "C"],
			}
			
			bar_module {
			    name: "B",
			    deps: ["D"],
			}
			
			foo_module {
			    name: "C",
			    deps: ["E", "F"],
			}
			
			foo_module {
			    name: "D",
			}
			
			bar_module {
			    name: "E",
			    deps: ["G"],
			}
			
			foo_module {
			    name: "F",
			    deps: ["G"],
			}
			
			foo_module {
			    name: "G",
			}
		`),
	})

	ctx.RegisterModuleType("foo_module", newFooModule)
	ctx.RegisterModuleType("bar_module", newBarModule)
	ctx.RegisterBottomUpMutator("deps", depsMutator)
	_, errs := ctx.ParseBlueprintsFiles("Blueprints", nil)
	if len(errs) > 0 {
		t.Errorf("unexpected parse errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	_, errs = ctx.ResolveDependencies(nil)
	if len(errs) > 0 {
		t.Errorf("unexpected dep errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	topModule := ctx.moduleGroupFromName("A", nil).modules[0]
	outputDown, outputUp := walkDependencyGraph(ctx, topModule, false)
	if outputDown != "BCEFG" {
		t.Errorf("unexpected walkDeps behaviour: %s\ndown should be: BCEFG", outputDown)
	}
	if outputUp != "BEGFC" {
		t.Errorf("unexpected walkDeps behaviour: %s\nup should be: BEGFC", outputUp)
	}
}

// |===B---D           - represents a non-walkable edge
// A                   = represents a walkable edge
// |===C===E===\       A should not be visited because it's the root node.
//     |       |       B, D should not be walked.
//     |===F===G===H   G should be visited multiple times
//         \===/       H should only be visited once
func TestWalkDepsDuplicates(t *testing.T) {
	ctx := NewContext()
	ctx.MockFileSystem(map[string][]byte{
		"Blueprints": []byte(`
			foo_module {
			    name: "A",
			    deps: ["B", "C"],
			}

			bar_module {
			    name: "B",
			    deps: ["D"],
			}

			foo_module {
			    name: "C",
			    deps: ["E", "F"],
			}

			foo_module {
			    name: "D",
			}

			foo_module {
			    name: "E",
			    deps: ["G"],
			}

			foo_module {
			    name: "F",
			    deps: ["G", "G"],
			}

			foo_module {
			    name: "G",
				deps: ["H"],
			}

			foo_module {
			    name: "H",
			}
		`),
	})

	ctx.RegisterModuleType("foo_module", newFooModule)
	ctx.RegisterModuleType("bar_module", newBarModule)
	ctx.RegisterBottomUpMutator("deps", depsMutator)
	_, errs := ctx.ParseBlueprintsFiles("Blueprints", nil)
	if len(errs) > 0 {
		t.Errorf("unexpected parse errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	_, errs = ctx.ResolveDependencies(nil)
	if len(errs) > 0 {
		t.Errorf("unexpected dep errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	topModule := ctx.moduleGroupFromName("A", nil).modules[0]
	outputDown, outputUp := walkDependencyGraph(ctx, topModule, true)
	if outputDown != "BCEGHFGG" {
		t.Errorf("unexpected walkDeps behaviour: %s\ndown should be: BCEGHFGG", outputDown)
	}
	if outputUp != "BHGEGGFC" {
		t.Errorf("unexpected walkDeps behaviour: %s\nup should be: BHGEGGFC", outputUp)
	}
}

//                     - represents a non-walkable edge
// A                   = represents a walkable edge
// |===B-------\       A should not be visited because it's the root node.
//     |       |       B -> D should not be walked.
//     |===C===D===E   B -> C -> D -> E should be walked
func TestWalkDepsDuplicates_IgnoreFirstPath(t *testing.T) {
	ctx := NewContext()
	ctx.MockFileSystem(map[string][]byte{
		"Blueprints": []byte(`
			foo_module {
			    name: "A",
			    deps: ["B"],
			}

			foo_module {
			    name: "B",
			    deps: ["C"],
			    ignored_deps: ["D"],
			}

			foo_module {
			    name: "C",
			    deps: ["D"],
			}

			foo_module {
			    name: "D",
			    deps: ["E"],
			}

			foo_module {
			    name: "E",
			}
		`),
	})

	ctx.RegisterModuleType("foo_module", newFooModule)
	ctx.RegisterModuleType("bar_module", newBarModule)
	ctx.RegisterBottomUpMutator("deps", depsMutator)
	_, errs := ctx.ParseBlueprintsFiles("Blueprints", nil)
	if len(errs) > 0 {
		t.Errorf("unexpected parse errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	_, errs = ctx.ResolveDependencies(nil)
	if len(errs) > 0 {
		t.Errorf("unexpected dep errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	topModule := ctx.moduleGroupFromName("A", nil).modules[0]
	outputDown, outputUp := walkDependencyGraph(ctx, topModule, true)
	expectedDown := "BDCDE"
	if outputDown != expectedDown {
		t.Errorf("unexpected walkDeps behaviour: %s\ndown should be: %s", outputDown, expectedDown)
	}
	expectedUp := "DEDCB"
	if outputUp != expectedUp {
		t.Errorf("unexpected walkDeps behaviour: %s\nup should be: %s", outputUp, expectedUp)
	}
}

func TestCreateModule(t *testing.T) {
	ctx := newContext()
	ctx.MockFileSystem(map[string][]byte{
		"Blueprints": []byte(`
			foo_module {
			    name: "A",
			    deps: ["B", "C"],
			}
		`),
	})

	ctx.RegisterTopDownMutator("create", createTestMutator)
	ctx.RegisterBottomUpMutator("deps", depsMutator)

	ctx.RegisterModuleType("foo_module", newFooModule)
	ctx.RegisterModuleType("bar_module", newBarModule)
	_, errs := ctx.ParseBlueprintsFiles("Blueprints", nil)
	if len(errs) > 0 {
		t.Errorf("unexpected parse errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	_, errs = ctx.ResolveDependencies(nil)
	if len(errs) > 0 {
		t.Errorf("unexpected dep errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	a := ctx.moduleGroupFromName("A", nil).modules[0].logicModule.(*fooModule)
	b := ctx.moduleGroupFromName("B", nil).modules[0].logicModule.(*barModule)
	c := ctx.moduleGroupFromName("C", nil).modules[0].logicModule.(*barModule)
	d := ctx.moduleGroupFromName("D", nil).modules[0].logicModule.(*fooModule)

	checkDeps := func(m Module, expected string) {
		var deps []string
		ctx.VisitDirectDeps(m, func(m Module) {
			deps = append(deps, ctx.ModuleName(m))
		})
		got := strings.Join(deps, ",")
		if got != expected {
			t.Errorf("unexpected %q dependencies, got %q expected %q",
				ctx.ModuleName(m), got, expected)
		}
	}

	checkDeps(a, "B,C")
	checkDeps(b, "D")
	checkDeps(c, "D")
	checkDeps(d, "")
}

func createTestMutator(ctx TopDownMutatorContext) {
	type props struct {
		Name string
		Deps []string
	}

	ctx.CreateModule(newBarModule, &props{
		Name: "B",
		Deps: []string{"D"},
	})

	ctx.CreateModule(newBarModule, &props{
		Name: "C",
		Deps: []string{"D"},
	})

	ctx.CreateModule(newFooModule, &props{
		Name: "D",
	})
}

func TestWalkFileOrder(t *testing.T) {
	// Run the test once to see how long it normally takes
	start := time.Now()
	doTestWalkFileOrder(t, time.Duration(0))
	duration := time.Since(start)

	// Run the test again, but put enough of a sleep into each visitor to detect ordering
	// problems if they exist
	doTestWalkFileOrder(t, duration)
}

// test that WalkBlueprintsFiles calls asyncVisitor in the right order
func doTestWalkFileOrder(t *testing.T, sleepDuration time.Duration) {
	// setup mock context
	ctx := newContext()
	mockFiles := map[string][]byte{
		"Blueprints": []byte(`
			sample_module {
			    name: "a",
			}
		`),
		"dir1/Blueprints": []byte(`
			sample_module {
			    name: "b",
			}
		`),
		"dir1/dir2/Blueprints": []byte(`
			sample_module {
			    name: "c",
			}
		`),
	}
	ctx.MockFileSystem(mockFiles)

	// prepare to monitor the visit order
	visitOrder := []string{}
	visitLock := sync.Mutex{}
	correctVisitOrder := []string{"Blueprints", "dir1/Blueprints", "dir1/dir2/Blueprints"}

	// sleep longer when processing the earlier files
	chooseSleepDuration := func(fileName string) (duration time.Duration) {
		duration = time.Duration(0)
		for i := len(correctVisitOrder) - 1; i >= 0; i-- {
			if fileName == correctVisitOrder[i] {
				return duration
			}
			duration = duration + sleepDuration
		}
		panic("unrecognized file name " + fileName)
	}

	visitor := func(file *parser.File) {
		time.Sleep(chooseSleepDuration(file.Name))
		visitLock.Lock()
		defer visitLock.Unlock()
		visitOrder = append(visitOrder, file.Name)
	}
	keys := []string{"Blueprints", "dir1/Blueprints", "dir1/dir2/Blueprints"}

	// visit the blueprints files
	ctx.WalkBlueprintsFiles(".", keys, visitor)

	// check the order
	if !reflect.DeepEqual(visitOrder, correctVisitOrder) {
		t.Errorf("Incorrect visit order; expected %v, got %v", correctVisitOrder, visitOrder)
	}
}

// test that WalkBlueprintsFiles reports syntax errors
func TestWalkingWithSyntaxError(t *testing.T) {
	// setup mock context
	ctx := newContext()
	mockFiles := map[string][]byte{
		"Blueprints": []byte(`
			sample_module {
			    name: "a" "b",
			}
		`),
		"dir1/Blueprints": []byte(`
			sample_module {
			    name: "b",
		`),
		"dir1/dir2/Blueprints": []byte(`
			sample_module {
			    name: "c",
			}
		`),
	}
	ctx.MockFileSystem(mockFiles)

	keys := []string{"Blueprints", "dir1/Blueprints", "dir1/dir2/Blueprints"}

	// visit the blueprints files
	_, errs := ctx.WalkBlueprintsFiles(".", keys, func(file *parser.File) {})

	expectedErrs := []error{
		errors.New(`Blueprints:3:18: expected "}", found String`),
		errors.New(`dir1/Blueprints:4:3: expected "}", found EOF`),
	}
	if fmt.Sprintf("%s", expectedErrs) != fmt.Sprintf("%s", errs) {
		t.Errorf("Incorrect errors; expected:\n%s\ngot:\n%s", expectedErrs, errs)
	}

}

func TestParseFailsForModuleWithoutName(t *testing.T) {
	ctx := NewContext()
	ctx.MockFileSystem(map[string][]byte{
		"Blueprints": []byte(`
			foo_module {
			    name: "A",
			}
			
			bar_module {
			    deps: ["A"],
			}
		`),
	})
	ctx.RegisterModuleType("foo_module", newFooModule)
	ctx.RegisterModuleType("bar_module", newBarModule)

	_, errs := ctx.ParseBlueprintsFiles("Blueprints", nil)

	expectedErrs := []error{
		errors.New(`Blueprints:6:4: property 'name' is missing from a module`),
	}
	if fmt.Sprintf("%s", expectedErrs) != fmt.Sprintf("%s", errs) {
		t.Errorf("Incorrect errors; expected:\n%s\ngot:\n%s", expectedErrs, errs)
	}
}
