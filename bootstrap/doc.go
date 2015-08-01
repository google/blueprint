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

// The Blueprint bootstrapping mechanism is intended to enable building a source
// tree using a Blueprint-based build system that is embedded (as source) in
// that source tree.  The only prerequisites for performing such a build are:
//
//   1. A Ninja binary
//   2. A script interpreter (e.g. Bash or Python)
//   3. A Go toolchain
//
// The Primary Builder
//
// As part of the bootstrapping process, a binary called the "primary builder"
// is created.  This primary builder is the binary that includes both the core
// Blueprint library and the build logic specific to the source tree.  It is
// used to generate the Ninja file that describes how to build the entire source
// tree.
//
// The primary builder must be a pure Go (i.e. no cgo) module built with the
// module type 'bootstrap_go_binary'.  It should have the 'primaryBuilder'
// module property set to true in its Blueprints file.  If more than one module
// sets primaryBuilder to true the build will fail.
//
// The primary builder main function should look something like:
//
//   package main
//
//   import (
//       "flag"
//       "github.com/google/blueprint"
//       "github.com/google/blueprint/bootstrap"
//       "path/filepath"
//
//       "my/custom/build/logic"
//   )
//
//   func main() {
//       // The primary builder should use the global flag set because the
//       // bootstrap package registers its own flags there.
//       flag.Parse()
//
//       // The top-level Blueprints file is passed as the first argument.
//       srcDir := filepath.Dir(flag.Arg(0))
//
//       // Create the build context.
//       ctx := blueprint.NewContext()
//
//       // Register custom module types
//       ctx.RegisterModuleType("foo", logic.FooModule)
//       ctx.RegisterModuleType("bar", logic.BarModule)
//
//       // Register custom singletons
//       ctx.RegisterSingleton("baz", logic.NewBazSingleton())
//
//       // Create and initialize the custom Config object.
//       config := logic.NewConfig(srcDir)
//
//       // This call never returns
//       bootstrap.Main(ctx, config)
//   }
//
// Required Source Files
//
// There are three files that must be included in the source tree to facilitate
// the build bootstrapping:
//
//   1. The top-level Blueprints file
//   2. The bootstrap Ninja file template
//   3. The bootstrap script
//
// The top-level Blueprints file describes how the entire source tree should be
// built.  It must have a 'subdirs' assignment that includes both the core
// Blueprint library and the custom build logic for the source tree.  It should
// also include (either directly or through a subdirs entry) describe all the
// modules to be built in the source tree.
//
// The bootstrap Ninja file template describes the build actions necessary to
// build the primary builder for the source tree.  This template contains a set
// of placeholder Ninja variable values that get filled in by the bootstrap
// script to create a usable Ninja file.  It can be created by running the
// minibp binary that gets created as part of the standalone Blueprint build.
// Passing minibp the path to the top-level Blueprints file will cause it to
// create a bootstrap Ninja file template named 'build.ninja.in'.
//
// The bootstrap script is a small script (or theoretically a compiled binary)
// that is included in the source tree to begin the bootstrapping process.  It
// is responsible for filling in the bootstrap Ninja file template with some
// basic information about the Go build environemnt and the path to the root
// source directory.  It does this by performing a simple string substitution on
// the template file to produce a usable build.ninja file.
//
// The Bootstrapping Process
//
// A bootstrap-enabled build directory has two states, each with a corresponding
// Ninja file. The states are referred to as the "bootstrap" state and the
// "main" state. Changing the directory to a particular state means replacing
// the build.ninja file with one that will perform the build actions for the
// state.
//
// The bootstrapping process begins with the user running the bootstrap script
// to initialize a new build directory.  The script is run from the build
// directory, and when run with no arguments it copies the source bootstrap
// Ninja file into the build directory as "build.ninja".  It also performs a set
// of string substitutions on the file to configure it for the user's build
// environment. Specifically, the following strings are substituted in the file:
//
//   @@SrcDir@@            - The path to the root source directory (either
//                           absolute or relative to the build dir)
//   @@GoRoot@@            - The path to the root directory of the Go toolchain
//   @@GoCompile@@         - The path to the Go compiler (6g or compile)
//   @@GoLink@@            - The path to the Go linker (6l or link)
//   @@Bootstrap@@         - The path to the bootstrap script
//   @@BootstrapManifest@@ - The path to the source bootstrap Ninja file
//
// Once the script completes the build directory is initialized in the bootstrap
// build state.  In this state, running Ninja may perform the following build
// actions.  Each one but the last can be skipped if its output is determined to
// be up-to-date.
//
// 	- Build the minibp binary
// 	- Run minibp to generate .bootstrap/bootstrap.ninja.in
// 	- Build the primary builder binary
// 	- Run the primary builder to generate .bootstrap/main.ninja.in
// 	- Run the bootstrap script to "copy" .bootstrap/main.ninja.in to build.ninja
//
// The last of these build actions results in transitioning the build directory
// to the main build state.
//
// The main state (potentially) performs the following actions:
//   - Copy .bootstrap/bootstrap.ninja.in to the source bootstrap Ninja location
//   - Run the bootstrap script to "copy" the source bootstrap Ninja file to
//     build.ninja
//   - Build all the non-bootstrap modules defined in Blueprints files
//
// Updating the Bootstrap Ninja File Template
//
// The main purpose of the bootstrap state is to generate the Ninja file for the
// main state.  The one additional thing it does is generate a new bootstrap
// Ninja file template at .bootstrap/bootstrap.ninja.in.  When generating this
// file, minibp will compare the new bootstrap Ninja file contents with the
// original (in the source tree).  If the contents match, the new file will be
// created with a timestamp that matches that of the original, indicating that
// the original file in the source tree is up-to-date.
//
// This is done so that in the main state if the bootstrap Ninja file template
// in the source tree is out of date it can be automatically updated.  Note,
// however, that we can't have the main state generate the new bootstrap Ninja
// file template contents itself, because it may be using an older minibp.
// Recall that minibp is only built during the bootstrap state (to break a
// circular dependence), so if a new bootstrap Ninja file template were
// generated then it could replace a new file (from an updated source tree) with
// one generated using an old minibp.
//
// This scheme ensures that updates to the source tree are always incorporated
// into the build process and that changes that require a new bootstrap Ninja
// file template automatically update the template in the source tree.
package bootstrap
