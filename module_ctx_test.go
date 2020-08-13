// Copyright 2019 Google Inc. All rights reserved.
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
	"reflect"
	"strings"
	"testing"
)

type moduleCtxTestModule struct {
	SimpleName
}

func newModuleCtxTestModule() (Module, []interface{}) {
	m := &moduleCtxTestModule{}
	return m, []interface{}{&m.SimpleName.Properties}
}

func (f *moduleCtxTestModule) GenerateBuildActions(ModuleContext) {
}

func noCreateAliasMutator(name string) func(ctx BottomUpMutatorContext) {
	return func(ctx BottomUpMutatorContext) {
		if ctx.ModuleName() == name {
			ctx.CreateVariations("a", "b")
		}
	}
}

func createAliasMutator(name string) func(ctx BottomUpMutatorContext) {
	return func(ctx BottomUpMutatorContext) {
		if ctx.ModuleName() == name {
			ctx.CreateVariations("a", "b")
			ctx.AliasVariation("b")
		}
	}
}

func addVariantDepsMutator(variants []Variation, tag DependencyTag, from, to string) func(ctx BottomUpMutatorContext) {
	return func(ctx BottomUpMutatorContext) {
		if ctx.ModuleName() == from {
			ctx.AddVariationDependencies(variants, tag, to)
		}
	}
}

func TestAliases(t *testing.T) {
	runWithFailures := func(ctx *Context, expectedErr string) {
		t.Helper()
		bp := `
			test {
				name: "foo",
			}

			test {
				name: "bar",
			}
		`

		mockFS := map[string][]byte{
			"Blueprints": []byte(bp),
		}

		ctx.MockFileSystem(mockFS)

		_, errs := ctx.ParseFileList(".", []string{"Blueprints"}, nil)
		if len(errs) > 0 {
			t.Errorf("unexpected parse errors:")
			for _, err := range errs {
				t.Errorf("  %s", err)
			}
		}

		_, errs = ctx.ResolveDependencies(nil)
		if len(errs) > 0 {
			if expectedErr == "" {
				t.Errorf("unexpected dep errors:")
				for _, err := range errs {
					t.Errorf("  %s", err)
				}
			} else {
				for _, err := range errs {
					if strings.Contains(err.Error(), expectedErr) {
						continue
					} else {
						t.Errorf("unexpected dep error: %s", err)
					}
				}
			}
		} else if expectedErr != "" {
			t.Errorf("missing dep error: %s", expectedErr)
		}
	}

	run := func(ctx *Context) {
		t.Helper()
		runWithFailures(ctx, "")
	}

	t.Run("simple", func(t *testing.T) {
		// Creates a module "bar" with variants "a" and "b" and alias "" -> "b".
		// Tests a dependency from "foo" to "bar" variant "b" through alias "".
		ctx := NewContext()
		ctx.RegisterModuleType("test", newModuleCtxTestModule)
		ctx.RegisterBottomUpMutator("1", createAliasMutator("bar"))
		ctx.RegisterBottomUpMutator("2", addVariantDepsMutator(nil, nil, "foo", "bar"))

		run(ctx)

		foo := ctx.moduleGroupFromName("foo", nil).modules[0]
		barB := ctx.moduleGroupFromName("bar", nil).modules[1]

		if g, w := barB.variant.name, "b"; g != w {
			t.Fatalf("expected bar.modules[1] variant to be %q, got %q", w, g)
		}

		if g, w := foo.forwardDeps, []*moduleInfo{barB}; !reflect.DeepEqual(g, w) {
			t.Fatalf("expected foo deps to be %q, got %q", w, g)
		}
	})

	t.Run("chained", func(t *testing.T) {
		// Creates a module "bar" with variants "a_a", "a_b", "b_a" and "b_b" and aliases "" -> "b_b",
		// "a" -> "a_b", and "b" -> "b_b".
		// Tests a dependency from "foo" to "bar" variant "b_b" through alias "".
		ctx := NewContext()
		ctx.RegisterModuleType("test", newModuleCtxTestModule)
		ctx.RegisterBottomUpMutator("1", createAliasMutator("bar"))
		ctx.RegisterBottomUpMutator("2", createAliasMutator("bar"))
		ctx.RegisterBottomUpMutator("3", addVariantDepsMutator(nil, nil, "foo", "bar"))

		run(ctx)

		foo := ctx.moduleGroupFromName("foo", nil).modules[0]
		barBB := ctx.moduleGroupFromName("bar", nil).modules[3]

		if g, w := barBB.variant.name, "b_b"; g != w {
			t.Fatalf("expected bar.modules[3] variant to be %q, got %q", w, g)
		}

		if g, w := foo.forwardDeps, []*moduleInfo{barBB}; !reflect.DeepEqual(g, w) {
			t.Fatalf("expected foo deps to be %q, got %q", w, g)
		}
	})

	t.Run("chained2", func(t *testing.T) {
		// Creates a module "bar" with variants "a_a", "a_b", "b_a" and "b_b" and aliases "" -> "b_b",
		// "a" -> "a_b", and "b" -> "b_b".
		// Tests a dependency from "foo" to "bar" variant "a_b" through alias "a".
		ctx := NewContext()
		ctx.RegisterModuleType("test", newModuleCtxTestModule)
		ctx.RegisterBottomUpMutator("1", createAliasMutator("bar"))
		ctx.RegisterBottomUpMutator("2", createAliasMutator("bar"))
		ctx.RegisterBottomUpMutator("3", addVariantDepsMutator([]Variation{{"1", "a"}}, nil, "foo", "bar"))

		run(ctx)

		foo := ctx.moduleGroupFromName("foo", nil).modules[0]
		barAB := ctx.moduleGroupFromName("bar", nil).modules[1]

		if g, w := barAB.variant.name, "a_b"; g != w {
			t.Fatalf("expected bar.modules[1] variant to be %q, got %q", w, g)
		}

		if g, w := foo.forwardDeps, []*moduleInfo{barAB}; !reflect.DeepEqual(g, w) {
			t.Fatalf("expected foo deps to be %q, got %q", w, g)
		}
	})

	t.Run("removed dangling alias", func(t *testing.T) {
		// Creates a module "bar" with variants "a" and "b" and aliases "" -> "b", then splits the variants into
		// "a_a", "a_b", "b_a" and "b_b" without creating new aliases.
		// Tests a dependency from "foo" to removed "bar" alias "" fails.
		ctx := NewContext()
		ctx.RegisterModuleType("test", newModuleCtxTestModule)
		ctx.RegisterBottomUpMutator("1", createAliasMutator("bar"))
		ctx.RegisterBottomUpMutator("2", noCreateAliasMutator("bar"))
		ctx.RegisterBottomUpMutator("3", addVariantDepsMutator(nil, nil, "foo", "bar"))

		runWithFailures(ctx, `dependency "bar" of "foo" missing variant:`+"\n  \n"+
			"available variants:"+
			"\n  1:a, 2:a\n  1:a, 2:b\n  1:b, 2:a\n  1:b, 2:b")
	})
}

func expectedErrors(t *testing.T, errs []error, expectedMessages ...string) {
	t.Helper()
	if len(errs) != len(expectedMessages) {
		t.Errorf("expected %d error, found: %q", len(expectedMessages), errs)
	} else {
		for i, expected := range expectedMessages {
			err := errs[i]
			if err.Error() != expected {
				t.Errorf("expected error %q found %q", expected, err)
			}
		}
	}
}

func TestCheckBlueprintSyntax(t *testing.T) {
	factories := map[string]ModuleFactory{
		"test": newModuleCtxTestModule,
	}

	t.Run("valid", func(t *testing.T) {
		errs := CheckBlueprintSyntax(factories, "path/Blueprint", `
test {
	name: "test",
}
`)
		expectedErrors(t, errs)
	})

	t.Run("syntax error", func(t *testing.T) {
		errs := CheckBlueprintSyntax(factories, "path/Blueprint", `
test {
	name: "test",

`)

		expectedErrors(t, errs, `path/Blueprint:5:1: expected "}", found EOF`)
	})

	t.Run("unknown module type", func(t *testing.T) {
		errs := CheckBlueprintSyntax(factories, "path/Blueprint", `
test2 {
	name: "test",
}
`)

		expectedErrors(t, errs, `path/Blueprint:2:1: unrecognized module type "test2"`)
	})

	t.Run("unknown property name", func(t *testing.T) {
		errs := CheckBlueprintSyntax(factories, "path/Blueprint", `
test {
	nam: "test",
}
`)

		expectedErrors(t, errs, `path/Blueprint:3:5: unrecognized property "nam"`)
	})

	t.Run("invalid property type", func(t *testing.T) {
		errs := CheckBlueprintSyntax(factories, "path/Blueprint", `
test {
	name: false,
}
`)

		expectedErrors(t, errs, `path/Blueprint:3:8: can't assign bool value to string property "name"`)
	})

	t.Run("multiple failures", func(t *testing.T) {
		errs := CheckBlueprintSyntax(factories, "path/Blueprint", `
test {
	name: false,
}

test2 {
	name: false,
}
`)

		expectedErrors(t, errs,
			`path/Blueprint:3:8: can't assign bool value to string property "name"`,
			`path/Blueprint:6:1: unrecognized module type "test2"`,
		)
	})
}
