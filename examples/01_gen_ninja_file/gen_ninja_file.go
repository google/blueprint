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

package main

import (
	"fmt"
	"os"

	. "github.com/google/blueprint"
)

var (
	pctx = NewPackageContext("example_pctx")

	RuleFoo = pctx.StaticRule("rule_foo", RuleParams{
		Command: "touch foo",
	})
	RuleBar1 = pctx.StaticRule("rule_bar1", RuleParams{
		Command: "touch bar1",
	})
	RuleBar2 = pctx.StaticRule("rule_bar2", RuleParams{
		Command: "touch bar2",
	})
	RuleClean = pctx.StaticRule("rule_clean", RuleParams{
		Command: "rm -f foo bar1 bar2 .ninja_*",
	})
)

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

func (f *fooModule) DynamicDependencies(ctx DynamicDependerModuleContext) []string {
	return f.properties.Deps
}

func (f *fooModule) GenerateBuildActions(ctx ModuleContext) {
	ctx.Build(pctx, BuildParams{
		Rule:    RuleFoo,
		Outputs: []string{"foo"},
		Inputs:  []string{"bar2"},
	})
	ctx.Build(pctx, BuildParams{
		Rule:     RuleClean,
		Outputs:  []string{"clean"},
		Optional: true,
	})
}

type barModule struct {
	SimpleName
	properties struct {
		Bar bool
	}
}

func newBarModule() (Module, []interface{}) {
	m := &barModule{}
	return m, []interface{}{&m.properties, &m.SimpleName.Properties}
}

func (b *barModule) GenerateBuildActions(ctx ModuleContext) {
	ctx.Build(pctx, BuildParams{
		Rule:     RuleBar1,
		Outputs:  []string{"bar1"},
		Optional: true,
	})
	ctx.Build(pctx, BuildParams{
		Rule:     RuleBar2,
		Outputs:  []string{"bar2"},
		Optional: true,
	})
}

func main() {
	// create context and register module types
	ctx := NewContext()
	ctx.RegisterModuleType("foo_module", newFooModule)
	ctx.RegisterModuleType("bar_module", newBarModule)

	// parse blueprint definitions and create modules
	if _, errs := ctx.ParseFileList(".", []string{"blueprints.bp"}); len(errs) > 0 {
		fmt.Printf("error in ParseFileList:\n%v\n", errs)
		os.Exit(1)
	}

	// resolve dependencies and generate build actions
	if _, errs := ctx.ResolveDependencies(nil); len(errs) > 0 {
		fmt.Printf("error in ResolveDependencies:\n%v\n", errs)
		os.Exit(1)
	}
	if _, errs := ctx.PrepareBuildActions(nil); len(errs) > 0 {
		fmt.Printf("error in PrepareBuildActions:\n%v\n", errs)
		os.Exit(1)
	}

	// print all dependencies to make sure they exist
	fmt.Printf("dependencies:\n")
	ctx.VisitAllModules(func(m Module) {
		ctx.VisitDirectDeps(m, func(d Module) {
			fmt.Printf("\t%s -> %s\n", m.Name(), d.Name())
		})
	})

	// write ninja build rules to file
	f, err := os.OpenFile("build/build.ninja",
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		fmt.Printf("error opening build.ninja\n")
		os.Exit(1)
	}
	if err := ctx.WriteBuildFile(f); err != nil {
		fmt.Printf("error writing build.ninja\n")
		os.Exit(1)
	}
	if err := f.Close(); err != nil {
		fmt.Printf("error closing build.ninja\n")
		os.Exit(1)
	}
}
