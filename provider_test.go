// Copyright 2020 Google Inc. All rights reserved.
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
	"reflect"
	"strings"
	"testing"
)

type providerTestModule struct {
	SimpleName
	properties struct {
		Deps []string
	}

	mutatorProviderValues              []string
	generateBuildActionsProviderValues []string
}

func newProviderTestModule() (Module, []interface{}) {
	m := &providerTestModule{}
	return m, []interface{}{&m.properties, &m.SimpleName.Properties}
}

type providerTestMutatorInfo struct {
	Values []string
}

type providerTestGenerateBuildActionsInfo struct {
	Value string
}

type providerTestUnsetInfo string

var providerTestMutatorInfoProvider = NewMutatorProvider(&providerTestMutatorInfo{}, "provider_mutator")
var providerTestGenerateBuildActionsInfoProvider = NewProvider(&providerTestGenerateBuildActionsInfo{})
var providerTestUnsetInfoProvider = NewMutatorProvider((providerTestUnsetInfo)(""), "provider_mutator")
var providerTestUnusedMutatorProvider = NewMutatorProvider(&struct{ unused string }{}, "nonexistent_mutator")

func (p *providerTestModule) GenerateBuildActions(ctx ModuleContext) {
	unset := ctx.Provider(providerTestUnsetInfoProvider).(providerTestUnsetInfo)
	if unset != "" {
		panic(fmt.Sprintf("expected zero value for providerTestGenerateBuildActionsInfoProvider before it was set, got %q",
			unset))
	}

	_ = ctx.Provider(providerTestUnusedMutatorProvider)

	ctx.SetProvider(providerTestGenerateBuildActionsInfoProvider, &providerTestGenerateBuildActionsInfo{
		Value: ctx.ModuleName(),
	})

	mp := ctx.Provider(providerTestMutatorInfoProvider).(*providerTestMutatorInfo)
	if mp != nil {
		p.mutatorProviderValues = mp.Values
	}

	ctx.VisitDirectDeps(func(module Module) {
		gbap := ctx.OtherModuleProvider(module, providerTestGenerateBuildActionsInfoProvider).(*providerTestGenerateBuildActionsInfo)
		if gbap != nil {
			p.generateBuildActionsProviderValues = append(p.generateBuildActionsProviderValues, gbap.Value)
		}
	})
}

func providerTestDepsMutator(ctx BottomUpMutatorContext) {
	if p, ok := ctx.Module().(*providerTestModule); ok {
		ctx.AddDependency(ctx.Module(), nil, p.properties.Deps...)
	}
}

func providerTestMutator(ctx BottomUpMutatorContext) {
	values := []string{strings.ToLower(ctx.ModuleName())}

	ctx.VisitDirectDeps(func(module Module) {
		mp := ctx.OtherModuleProvider(module, providerTestMutatorInfoProvider).(*providerTestMutatorInfo)
		if mp != nil {
			values = append(values, mp.Values...)
		}
	})

	ctx.SetProvider(providerTestMutatorInfoProvider, &providerTestMutatorInfo{
		Values: values,
	})
}

func providerTestAfterMutator(ctx BottomUpMutatorContext) {
	_ = ctx.Provider(providerTestMutatorInfoProvider)
}

func TestProviders(t *testing.T) {
	ctx := NewContext()
	ctx.RegisterModuleType("provider_module", newProviderTestModule)
	ctx.RegisterBottomUpMutator("provider_deps_mutator", providerTestDepsMutator)
	ctx.RegisterBottomUpMutator("provider_mutator", providerTestMutator)
	ctx.RegisterBottomUpMutator("provider_after_mutator", providerTestAfterMutator)

	ctx.MockFileSystem(map[string][]byte{
		"Blueprints": []byte(`
			provider_module {
				name: "A",
				deps: ["B"],
			}
	
			provider_module {
				name: "B",
				deps: ["C", "D"],
			}
	
			provider_module {
				name: "C",
				deps: ["D"],
			}
	
			provider_module {
				name: "D",
			}
		`),
	})

	_, errs := ctx.ParseBlueprintsFiles("Blueprints", nil)
	if len(errs) == 0 {
		_, errs = ctx.ResolveDependencies(nil)
	}
	if len(errs) == 0 {
		_, errs = ctx.PrepareBuildActions(nil)
	}
	if len(errs) > 0 {
		t.Errorf("unexpected errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	aModule := ctx.moduleGroupFromName("A", nil).moduleByVariantName("").logicModule.(*providerTestModule)
	if g, w := aModule.generateBuildActionsProviderValues, []string{"B"}; !reflect.DeepEqual(g, w) {
		t.Errorf("expected A.generateBuildActionsProviderValues %q, got %q", w, g)
	}
	if g, w := aModule.mutatorProviderValues, []string{"a", "b", "c", "d", "d"}; !reflect.DeepEqual(g, w) {
		t.Errorf("expected A.mutatorProviderValues %q, got %q", w, g)
	}

	bModule := ctx.moduleGroupFromName("B", nil).moduleByVariantName("").logicModule.(*providerTestModule)
	if g, w := bModule.generateBuildActionsProviderValues, []string{"C", "D"}; !reflect.DeepEqual(g, w) {
		t.Errorf("expected B.generateBuildActionsProviderValues %q, got %q", w, g)
	}
	if g, w := bModule.mutatorProviderValues, []string{"b", "c", "d", "d"}; !reflect.DeepEqual(g, w) {
		t.Errorf("expected B.mutatorProviderValues %q, got %q", w, g)
	}
}

type invalidProviderUsageMutatorInfo string
type invalidProviderUsageGenerateBuildActionsInfo string

var invalidProviderUsageMutatorInfoProvider = NewMutatorProvider(invalidProviderUsageMutatorInfo(""), "mutator_under_test")
var invalidProviderUsageGenerateBuildActionsInfoProvider = NewProvider(invalidProviderUsageGenerateBuildActionsInfo(""))

type invalidProviderUsageTestModule struct {
	parent *invalidProviderUsageTestModule

	SimpleName
	properties struct {
		Deps []string

		Early_mutator_set_of_mutator_provider       bool
		Late_mutator_set_of_mutator_provider        bool
		Late_build_actions_set_of_mutator_provider  bool
		Early_mutator_set_of_build_actions_provider bool

		Early_mutator_get_of_mutator_provider       bool
		Early_module_get_of_mutator_provider        bool
		Early_mutator_get_of_build_actions_provider bool
		Early_module_get_of_build_actions_provider  bool

		Duplicate_set bool
	}
}

func invalidProviderUsageDepsMutator(ctx BottomUpMutatorContext) {
	if i, ok := ctx.Module().(*invalidProviderUsageTestModule); ok {
		ctx.AddDependency(ctx.Module(), nil, i.properties.Deps...)
	}
}

func invalidProviderUsageParentMutator(ctx TopDownMutatorContext) {
	if i, ok := ctx.Module().(*invalidProviderUsageTestModule); ok {
		ctx.VisitDirectDeps(func(module Module) {
			module.(*invalidProviderUsageTestModule).parent = i
		})
	}
}

func invalidProviderUsageBeforeMutator(ctx BottomUpMutatorContext) {
	if i, ok := ctx.Module().(*invalidProviderUsageTestModule); ok {
		if i.properties.Early_mutator_set_of_mutator_provider {
			// A mutator attempting to set the value of a provider associated with a later mutator.
			ctx.SetProvider(invalidProviderUsageMutatorInfoProvider, invalidProviderUsageMutatorInfo(""))
		}
		if i.properties.Early_mutator_get_of_mutator_provider {
			// A mutator attempting to get the value of a provider associated with a later mutator.
			_ = ctx.Provider(invalidProviderUsageMutatorInfoProvider)
		}
	}
}

func invalidProviderUsageMutatorUnderTest(ctx TopDownMutatorContext) {
	if i, ok := ctx.Module().(*invalidProviderUsageTestModule); ok {
		if i.properties.Early_mutator_set_of_build_actions_provider {
			// A mutator attempting to set the value of a non-mutator provider.
			ctx.SetProvider(invalidProviderUsageGenerateBuildActionsInfoProvider, invalidProviderUsageGenerateBuildActionsInfo(""))
		}
		if i.properties.Early_mutator_get_of_build_actions_provider {
			// A mutator attempting to get the value of a non-mutator provider.
			_ = ctx.Provider(invalidProviderUsageGenerateBuildActionsInfoProvider)
		}
		if i.properties.Early_module_get_of_mutator_provider {
			// A mutator attempting to get the value of a provider associated with this mutator on
			// a module for which this mutator hasn't run.  This is a top down mutator so
			// dependencies haven't run yet.
			ctx.VisitDirectDeps(func(module Module) {
				_ = ctx.OtherModuleProvider(module, invalidProviderUsageMutatorInfoProvider)
			})
		}
	}
}

func invalidProviderUsageAfterMutator(ctx BottomUpMutatorContext) {
	if i, ok := ctx.Module().(*invalidProviderUsageTestModule); ok {
		if i.properties.Late_mutator_set_of_mutator_provider {
			// A mutator trying to set the value of a provider associated with an earlier mutator.
			ctx.SetProvider(invalidProviderUsageMutatorInfoProvider, invalidProviderUsageMutatorInfo(""))
		}
		if i.properties.Late_mutator_set_of_mutator_provider {
			// A mutator trying to set the value of a provider associated with an earlier mutator.
			ctx.SetProvider(invalidProviderUsageMutatorInfoProvider, invalidProviderUsageMutatorInfo(""))
		}
	}
}

func (i *invalidProviderUsageTestModule) GenerateBuildActions(ctx ModuleContext) {
	if i.properties.Late_build_actions_set_of_mutator_provider {
		// A GenerateBuildActions trying to set the value of a provider associated with a mutator.
		ctx.SetProvider(invalidProviderUsageMutatorInfoProvider, invalidProviderUsageMutatorInfo(""))
	}
	if i.properties.Early_module_get_of_build_actions_provider {
		// A GenerateBuildActions trying to get the value of a provider on a module for which
		// GenerateBuildActions hasn't run.
		_ = ctx.OtherModuleProvider(i.parent, invalidProviderUsageGenerateBuildActionsInfoProvider)
	}
	if i.properties.Duplicate_set {
		ctx.SetProvider(invalidProviderUsageGenerateBuildActionsInfoProvider, invalidProviderUsageGenerateBuildActionsInfo(""))
		ctx.SetProvider(invalidProviderUsageGenerateBuildActionsInfoProvider, invalidProviderUsageGenerateBuildActionsInfo(""))
	}
}

func TestInvalidProvidersUsage(t *testing.T) {
	run := func(t *testing.T, module string, prop string, panicMsg string) {
		t.Helper()
		ctx := NewContext()
		ctx.RegisterModuleType("invalid_provider_usage_test_module", func() (Module, []interface{}) {
			m := &invalidProviderUsageTestModule{}
			return m, []interface{}{&m.properties, &m.SimpleName.Properties}
		})
		ctx.RegisterBottomUpMutator("deps", invalidProviderUsageDepsMutator)
		ctx.RegisterBottomUpMutator("before", invalidProviderUsageBeforeMutator)
		ctx.RegisterTopDownMutator("mutator_under_test", invalidProviderUsageMutatorUnderTest)
		ctx.RegisterBottomUpMutator("after", invalidProviderUsageAfterMutator)
		ctx.RegisterTopDownMutator("parent", invalidProviderUsageParentMutator)

		// Don't invalidate the parent pointer and before GenerateBuildActions.
		ctx.skipCloneModulesAfterMutators = true

		var parentBP, moduleUnderTestBP, childBP string

		prop += ": true,"

		switch module {
		case "parent":
			parentBP = prop
		case "module_under_test":
			moduleUnderTestBP = prop
		case "child":
			childBP = prop
		}

		bp := fmt.Sprintf(`
			invalid_provider_usage_test_module {
				name: "parent",
				deps: ["module_under_test"],
				%s
			}
	
			invalid_provider_usage_test_module {
				name: "module_under_test",
				deps: ["child"],
				%s
			}
	
			invalid_provider_usage_test_module {
				name: "child",
				%s
			}

		`,
			parentBP,
			moduleUnderTestBP,
			childBP)

		ctx.MockFileSystem(map[string][]byte{
			"Blueprints": []byte(bp),
		})

		_, errs := ctx.ParseBlueprintsFiles("Blueprints", nil)

		if len(errs) == 0 {
			_, errs = ctx.ResolveDependencies(nil)
		}

		if len(errs) == 0 {
			_, errs = ctx.PrepareBuildActions(nil)
		}

		if len(errs) == 0 {
			t.Fatal("expected an error")
		}

		if len(errs) > 1 {
			t.Errorf("expected a single error, got %d:", len(errs))
			for i, err := range errs {
				t.Errorf("%d:  %s", i, err)
			}
			t.FailNow()
		}

		if panicErr, ok := errs[0].(panicError); ok {
			if panicErr.panic != panicMsg {
				t.Fatalf("expected panic %q, got %q", panicMsg, panicErr.panic)
			}
		} else {
			t.Fatalf("expected a panicError, got %T: %s", errs[0], errs[0].Error())
		}

	}

	tests := []struct {
		prop   string
		module string

		panicMsg string
		skip     string
	}{
		{
			prop:     "early_mutator_set_of_mutator_provider",
			module:   "module_under_test",
			panicMsg: "Can't set value of provider blueprint.invalidProviderUsageMutatorInfo before mutator mutator_under_test started",
		},
		{
			prop:     "late_mutator_set_of_mutator_provider",
			module:   "module_under_test",
			panicMsg: "Can't set value of provider blueprint.invalidProviderUsageMutatorInfo after mutator mutator_under_test finished",
		},
		{
			prop:     "late_build_actions_set_of_mutator_provider",
			module:   "module_under_test",
			panicMsg: "Can't set value of provider blueprint.invalidProviderUsageMutatorInfo after mutator mutator_under_test finished",
		},
		{
			prop:     "early_mutator_set_of_build_actions_provider",
			module:   "module_under_test",
			panicMsg: "Can't set value of provider blueprint.invalidProviderUsageGenerateBuildActionsInfo before GenerateBuildActions started",
		},

		{
			prop:     "early_mutator_get_of_mutator_provider",
			module:   "module_under_test",
			panicMsg: "Can't get value of provider blueprint.invalidProviderUsageMutatorInfo before mutator mutator_under_test finished",
		},
		{
			prop:     "early_module_get_of_mutator_provider",
			module:   "module_under_test",
			panicMsg: "Can't get value of provider blueprint.invalidProviderUsageMutatorInfo before mutator mutator_under_test finished",
		},
		{
			prop:     "early_mutator_get_of_build_actions_provider",
			module:   "module_under_test",
			panicMsg: "Can't get value of provider blueprint.invalidProviderUsageGenerateBuildActionsInfo before GenerateBuildActions finished",
		},
		{
			prop:     "early_module_get_of_build_actions_provider",
			module:   "module_under_test",
			panicMsg: "Can't get value of provider blueprint.invalidProviderUsageGenerateBuildActionsInfo before GenerateBuildActions finished",
		},
		{
			prop:     "duplicate_set",
			module:   "module_under_test",
			panicMsg: "Value of provider blueprint.invalidProviderUsageGenerateBuildActionsInfo is already set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.prop, func(t *testing.T) {
			run(t, tt.module, tt.prop, tt.panicMsg)
		})
	}
}
