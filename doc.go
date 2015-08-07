// Copyright 2015 Google Inc. All rights reserved.
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

// Blueprint is a meta-build system that reads in Blueprints files that describe
// modules that need to be built, and produces a Ninja
// (http://martine.github.io/ninja/) manifest describing the commands that need
// to be run and their dependencies.  Where most build systems use built-in
// rules or a domain-specific language to describe the logic how modules are
// converted to build rules, Blueprint delegates this to per-project build logic
// written in Go.  For large, heterogenous projects this allows the inherent
// complexity of the build logic to be maintained in a high-level language,
// while still allowing simple changes to individual modules by modifying easy
// to understand Blueprints files.
//
// Blueprint uses a bootstrapping process to allow the code for Blueprint,
// the code for the build logic, and the code for the project being compiled
// to all live in the project.  Dependencies between the layers are fully
// tracked - a change to the logic code will cause the logic to be recompiled,
// regenerate the project build manifest, and run modified project rules.  A
// change to Blueprint itself will cause Blueprint to rebuild, and then rebuild
// the logic, etc.
//
// A Blueprints file is a list of modules in a pseudo-python data format, where
// the module type looks like a function call, and the properties of the module
// look like optional arguments.  For example, a simple module might look like:
//
//   cc_library(
//       name = "cmd",
//       srcs = [
//           "main.c",
//       ],
//       deps = [
//           "libc",
//       ],
//   )
//
//   subdirs = ["subdir1", "subdir2"]
//
// The modules from the top level Blueprints file and recursively through any
// subdirectories listed by the "subdirs" variable are read by Blueprint, and
// their properties are stored into property structs by module type.  Once
// all modules are read, Blueprint calls any registered Mutators, in
// registration order.  Mutators can visit each module top-down or bottom-up,
// and modify them as necessary.  Common modifications include setting
// properties on modules to propagate information down from dependers to
// dependees (for example, telling a module what kinds of parents depend on it),
// or splitting a module into multiple variants (for example, one per
// architecture being compiled).  After all Mutators have run, each module is
// asked to generate build rules based on property values, and then singletons
// can generate any build rules from the output of all modules.
//
// The per-project build logic defines a top level command, referred to in the
// documentation as the "primary builder".  This command is responsible for
// registering the module types needed for the project, as well as any
// singletons or mutators, and then calling into Blueprint with the path of the
// root Blueprint file.
package blueprint
