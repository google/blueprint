// Copyright 2016 Google Inc. All rights reserved.
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
	"testing"
)

type visitModule struct {
	SimpleName
	properties struct {
		Visit                 []string
		VisitDepsDepthFirst   string `blueprint:"mutated"`
		VisitDepsDepthFirstIf string `blueprint:"mutated"`
		VisitDirectDeps       string `blueprint:"mutated"`
		VisitDirectDepsIf     string `blueprint:"mutated"`
	}
}

func newVisitModule() (Module, []interface{}) {
	m := &visitModule{}
	return m, []interface{}{&m.properties, &m.SimpleName.Properties}
}

func (f *visitModule) GenerateBuildActions(ModuleContext) {
}

type visitTag struct {
	BaseDependencyTag
}

var visitTagDep visitTag

func visitDepsMutator(ctx BottomUpMutatorContext) {
	if m, ok := ctx.Module().(*visitModule); ok {
		ctx.AddDependency(ctx.Module(), visitTagDep, m.properties.Visit...)
	}
}

func visitMutator(ctx TopDownMutatorContext) {
	if m, ok := ctx.Module().(*visitModule); ok {
		ctx.VisitDepsDepthFirst(func(dep Module) {
			if ctx.OtherModuleDependencyTag(dep) != visitTagDep {
				panic(fmt.Errorf("unexpected dependency tag on %q", ctx.OtherModuleName(dep)))
			}
			m.properties.VisitDepsDepthFirst = m.properties.VisitDepsDepthFirst + ctx.OtherModuleName(dep)
		})
		ctx.VisitDepsDepthFirstIf(func(dep Module) bool {
			return ctx.OtherModuleName(dep) != "B"
		}, func(dep Module) {
			m.properties.VisitDepsDepthFirstIf = m.properties.VisitDepsDepthFirstIf + ctx.OtherModuleName(dep)
		})
		ctx.VisitDirectDeps(func(dep Module) {
			m.properties.VisitDirectDeps = m.properties.VisitDirectDeps + ctx.OtherModuleName(dep)
		})
		ctx.VisitDirectDepsIf(func(dep Module) bool {
			return ctx.OtherModuleName(dep) != "B"
		}, func(dep Module) {
			m.properties.VisitDirectDepsIf = m.properties.VisitDirectDepsIf + ctx.OtherModuleName(dep)
		})
	}
}

// A
// |
// B
// |\
// C \
//  \|
//   D
//   |
//   E
//  / \
//  \ /
//   F
func setupVisitTest(t *testing.T) *Context {
	ctx := NewContext()
	ctx.RegisterModuleType("visit_module", newVisitModule)
	ctx.RegisterBottomUpMutator("visit_deps", visitDepsMutator)
	ctx.RegisterTopDownMutator("visit", visitMutator)

	ctx.MockFileSystem(map[string][]byte{
		"Blueprints": []byte(`
			visit_module {
				name: "A",
				visit: ["B"],
			}
	
			visit_module {
				name: "B",
				visit: ["C", "D"],
			}
	
			visit_module {
				name: "C",
				visit: ["D"],
			}
	
			visit_module {
				name: "D",
				visit: ["E"],
			}
	
			visit_module {
				name: "E",
				visit: ["F", "F"],
			}

			visit_module {
				name: "F",
			}
		`),
	})

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

	return ctx
}

func TestVisit(t *testing.T) {
	ctx := setupVisitTest(t)

	topModule := ctx.moduleGroupFromName("A", nil).modules.firstModule().logicModule.(*visitModule)
	assertString(t, topModule.properties.VisitDepsDepthFirst, "FEDCB")
	assertString(t, topModule.properties.VisitDepsDepthFirstIf, "FEDC")
	assertString(t, topModule.properties.VisitDirectDeps, "B")
	assertString(t, topModule.properties.VisitDirectDepsIf, "")

	eModule := ctx.moduleGroupFromName("E", nil).modules.firstModule().logicModule.(*visitModule)
	assertString(t, eModule.properties.VisitDepsDepthFirst, "F")
	assertString(t, eModule.properties.VisitDepsDepthFirstIf, "F")
	assertString(t, eModule.properties.VisitDirectDeps, "FF")
	assertString(t, eModule.properties.VisitDirectDepsIf, "FF")
}

func assertString(t *testing.T, got, expected string) {
	if got != expected {
		t.Errorf("expected %q got %q", expected, got)
	}
}
