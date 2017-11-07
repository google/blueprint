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
	"strings"
	"testing"
)

type Walker interface {
	Walk() bool
}

type fooModule struct {
	SimpleName
	properties struct {
		Deps []string
		Foo  string
	}
}

func newFooModule() (Module, []interface{}) {
	m := &fooModule{}
	return m, []interface{}{&m.properties, &m.SimpleName.Properties}
}

func (f *fooModule) GenerateBuildActions(ModuleContext) {
}

func (f *fooModule) DynamicDependencies(ctx DynamicDependerModuleContext) []string {
	return f.properties.Deps
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
		Deps []string
		Bar  bool
	}
}

func newBarModule() (Module, []interface{}) {
	m := &barModule{}
	return m, []interface{}{&m.properties, &m.SimpleName.Properties}
}

func (b *barModule) DynamicDependencies(ctx DynamicDependerModuleContext) []string {
	return b.properties.Deps
}

func (b *barModule) GenerateBuildActions(ModuleContext) {
}

func (b *barModule) Bar() bool {
	return b.properties.Bar
}

func (b *barModule) Walk() bool {
	return false
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

	_, _, errs := ctx.parseOne(".", "Blueprint", r, nil)
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

// |---B===D       - represents a non-walkable edge
// A               = represents a walkable edge
// |===C---E===G
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
	_, errs := ctx.ParseBlueprintsFiles("Blueprints")
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

	var outputDown string
	var outputUp string
	topModule := ctx.modulesFromName("A")[0]
	ctx.walkDeps(topModule,
		func(dep depInfo, parent *moduleInfo) bool {
			if dep.module.logicModule.(Walker).Walk() {
				outputDown += ctx.ModuleName(dep.module.logicModule)
				return true
			}
			return false
		},
		func(dep depInfo, parent *moduleInfo) {
			if dep.module.logicModule.(Walker).Walk() {
				outputUp += ctx.ModuleName(dep.module.logicModule)
			}
		})
	if outputDown != "CFG" {
		t.Fatalf("unexpected walkDeps behaviour: %s\ndown should be: CFG", outputDown)
	}
	if outputUp != "GFC" {
		t.Fatalf("unexpected walkDeps behaviour: %s\nup should be: GFC", outputUp)
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
	ctx.RegisterBottomUpMutator("deps", blueprintDepsMutator)

	ctx.RegisterModuleType("foo_module", newFooModule)
	ctx.RegisterModuleType("bar_module", newBarModule)
	_, errs := ctx.ParseBlueprintsFiles("Blueprints")
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

	a := ctx.modulesFromName("A")[0].logicModule.(*fooModule)
	b := ctx.modulesFromName("B")[0].logicModule.(*barModule)
	c := ctx.modulesFromName("C")[0].logicModule.(*barModule)
	d := ctx.modulesFromName("D")[0].logicModule.(*fooModule)

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
