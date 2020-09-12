// Copyright 2017 Google Inc. All rights reserved.
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
	"sort"
)

// This file exposes the logic of locating a module via a query string, to enable
// other projects to override it if desired.
// The default name resolution implementation, SimpleNameInterface,
// just treats the query string as a module name, and does a simple map lookup.

// A ModuleGroup just points to a moduleGroup to allow external packages to refer
// to a moduleGroup but not use it
type ModuleGroup struct {
	*moduleGroup
}

func (h *ModuleGroup) String() string {
	return h.moduleGroup.name
}

// The Namespace interface is just a marker interface for usage by the NameInterface,
// to allow a NameInterface to specify that a certain parameter should be a Namespace.
// In practice, a specific NameInterface will expect to only give and receive structs of
// the same concrete type, but because Go doesn't support generics, we use a marker interface
// for a little bit of clarity, and expect implementers to do typecasting instead.
type Namespace interface {
	namespace(Namespace)
}
type NamespaceMarker struct {
}

func (m *NamespaceMarker) namespace(Namespace) {
}

// A NameInterface tells how to locate modules by name.
// There should only be one name interface per Context, but potentially many namespaces
type NameInterface interface {
	// Gets called when a new module is created
	NewModule(ctx NamespaceContext, group ModuleGroup, module Module) (namespace Namespace, err []error)

	// Finds the module with the given name
	ModuleFromName(moduleName string, namespace Namespace) (group ModuleGroup, found bool)

	// Returns an error indicating that the given module could not be found.
	// The error contains some diagnostic information about where the dependency can be found.
	MissingDependencyError(depender string, dependerNamespace Namespace, depName string) (err error)

	// Rename
	Rename(oldName string, newName string, namespace Namespace) []error

	// Returns all modules in a deterministic order.
	AllModules() []ModuleGroup

	// gets the namespace for a given path
	GetNamespace(ctx NamespaceContext) (namespace Namespace)

	// returns a deterministic, unique, arbitrary string for the given name in the given namespace
	UniqueName(ctx NamespaceContext, name string) (unique string)
}

// A NamespaceContext stores the information given to a NameInterface to enable the NameInterface
// to choose the namespace for any given module
type NamespaceContext interface {
	ModulePath() string
}

type namespaceContextImpl struct {
	modulePath string
}

func newNamespaceContext(moduleInfo *moduleInfo) (ctx NamespaceContext) {
	return &namespaceContextImpl{moduleInfo.pos.Filename}
}

func (ctx *namespaceContextImpl) ModulePath() string {
	return ctx.modulePath
}

// a SimpleNameInterface just stores all modules in a map based on name
type SimpleNameInterface struct {
	modules map[string]ModuleGroup
}

func NewSimpleNameInterface() *SimpleNameInterface {
	return &SimpleNameInterface{
		modules: make(map[string]ModuleGroup),
	}
}

func (s *SimpleNameInterface) NewModule(ctx NamespaceContext, group ModuleGroup, module Module) (namespace Namespace, err []error) {
	name := group.name
	if group, present := s.modules[name]; present {
		return nil, []error{
			// seven characters at the start of the second line to align with the string "error: "
			fmt.Errorf("module %q already defined\n"+
				"       %s <-- previous definition here", name, group.modules.firstModule().pos),
		}
	}

	s.modules[name] = group

	return nil, []error{}
}

func (s *SimpleNameInterface) ModuleFromName(moduleName string, namespace Namespace) (group ModuleGroup, found bool) {
	group, found = s.modules[moduleName]
	return group, found
}

func (s *SimpleNameInterface) Rename(oldName string, newName string, namespace Namespace) (errs []error) {
	existingGroup, exists := s.modules[newName]
	if exists {
		return []error{
			// seven characters at the start of the second line to align with the string "error: "
			fmt.Errorf("renaming module %q to %q conflicts with existing module\n"+
				"       %s <-- existing module defined here",
				oldName, newName, existingGroup.modules.firstModule().pos),
		}
	}

	group, exists := s.modules[oldName]
	if !exists {
		return []error{fmt.Errorf("module %q to renamed to %q doesn't exist", oldName, newName)}
	}
	s.modules[newName] = group
	delete(s.modules, group.name)
	group.name = newName
	return nil
}

func (s *SimpleNameInterface) AllModules() []ModuleGroup {
	groups := make([]ModuleGroup, 0, len(s.modules))
	for _, group := range s.modules {
		groups = append(groups, group)
	}

	duplicateName := ""
	less := func(i, j int) bool {
		if groups[i].name == groups[j].name {
			duplicateName = groups[i].name
		}
		return groups[i].name < groups[j].name
	}
	sort.Slice(groups, less)
	if duplicateName != "" {
		// It is permitted to have two moduleGroup's with the same name, but not within the same
		// Namespace. The SimpleNameInterface should catch this in NewModule, however, so this
		// should never happen.
		panic(fmt.Sprintf("Duplicate moduleGroup name %q", duplicateName))
	}
	return groups
}

func (s *SimpleNameInterface) MissingDependencyError(depender string, dependerNamespace Namespace, dependency string) (err error) {
	return fmt.Errorf("%q depends on undefined module %q", depender, dependency)
}

func (s *SimpleNameInterface) GetNamespace(ctx NamespaceContext) Namespace {
	return nil
}

func (s *SimpleNameInterface) UniqueName(ctx NamespaceContext, name string) (unique string) {
	return name
}
