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
	"testing"
)

type Walker interface {
	Walk() bool
}

type fooModule struct {
	properties struct {
		Foo string
	}
}

func newFooModule() (Module, []interface{}) {
	m := &fooModule{}
	return m, []interface{}{&m.properties}
}

func (f *fooModule) GenerateBuildActions(ModuleContext) {
}

func (f *fooModule) Foo() string {
	return f.properties.Foo
}

func (f *fooModule) Walk() bool {
	return true
}

type barModule struct {
	properties struct {
		Bar bool
	}
}

func newBarModule() (Module, []interface{}) {
	m := &barModule{}
	return m, []interface{}{&m.properties}
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

	_, _, _, errs := ctx.parse(".", "Blueprint", r, nil)
	if len(errs) > 0 {
		t.Errorf("unexpected parse errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	errs = ctx.ResolveDependencies(nil)
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
	ctx.RegisterModuleType("foo_module", newFooModule)
	ctx.RegisterModuleType("bar_module", newBarModule)
	ctx.ParseBlueprintsFiles("context_test_Blueprints")
	ctx.ResolveDependencies(nil)

	var output string
	topModule := ctx.moduleGroups["A"].modules[0]
	ctx.walkDeps(topModule,
		func(module, parent Module) bool {
			if module.(Walker).Walk() {
				output += ctx.ModuleName(module)
				return true
			}
			return false
		})
	if output != "CFG" {
		t.Fatalf("unexpected walkDeps behaviour: %s\nshould be: CFG", output)
	}
}
