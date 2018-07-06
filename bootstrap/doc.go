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

// The Blueprint bootstrapping mechanism is intended to enable building a
// source tree with minimal prebuilts.  The only prerequisites for performing
// such a build are:
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
//   2. The bootstrap script
//   3. The build wrapper script
//
// The top-level Blueprints file describes how the entire source tree should be
// built.  It must have a 'subdirs' assignment that includes both the core
// Blueprint library and the custom build logic for the source tree.  It should
// also include (either directly or through a subdirs entry) describe all the
// modules to be built in the source tree.
//
// The bootstrap script is a small script to setup the build directory, writing
// a couple configuration files (including the path the source directory,
// information about the Go build environment, etc), then copying the build
// wrapper into the build directory.
//
// The Bootstrapping Process
//
// There are three stages to the bootstrapping process, each with a
// corresponding Ninja file. The stages are referred to as the "bootstrap",
// "primary", and "main" stages. Each stage builds the next stage's Ninja file.
//
// The bootstrapping process begins with the user running the bootstrap script
// to initialize a new build directory.  The script is run from the build
// directory, and creates a ".minibootstrap/build.ninja" file that sets a few
// variables then includes blueprint's "bootstrap/build.ninja". It also writes
// out a ".blueprint.bootstrap" file that contains a few variables for later use:
//
//   BLUEPRINT_BOOTSTRAP_VERSION - Used to detect when a user needs to run
//                                 bootstrap.bash again
//
//   SRCDIR         - The path to the source directory
//   BLUEPRINTDIR   - The path to the blueprints directory (includes $SRCDIR)
//   GOROOT         - The path to the root directory of the Go toolchain
//   NINJA_BUILDDIR - The path to store .ninja_log, .ninja_deps
//
// Once the script completes the build directory is initialized and ready to run
// a build. A wrapper script (blueprint.bash by default) has been installed in
// order to run a build. It iterates through the three stages of the build:
//
//      - Runs microfactory.bash to build minibp
//      - Runs the .minibootstrap/build.ninja to build .bootstrap/build.ninja
//      - Runs .bootstrap/build.ninja to build and run the primary builder
//      - Runs build.ninja to build your code
//
// Microfactory takes care of building an up to date version of `minibp` and
// `bpglob` under the .minibootstrap/ directory.
//
// During <builddir>/.minibootstrap/build.ninja, the following actions are
// taken, if necessary:
//
//      - Run minibp to generate .bootstrap/build.ninja (Primary stage)
//      - Includes .minibootstrap/build-globs.ninja, which defines rules to
//        run bpglob during incremental builds. These outputs are listed in
//        the dependency file output by minibp.
//
// During the <builddir>/.bootstrap/build.ninja, the following actions are
// taken, if necessary:
//
//      - Build the primary builder, anything marked `default: true`, and
//        any dependencies.
//      - Run the primary builder to generate build.ninja
//      - Run the primary builder to extract documentation
//      - Includes .bootstrap/build-globs.ninja, which defines rules to run
//        bpglob during incremental builds. These outputs are listed in the
//        dependency file output by the primary builder.
//
// Then the main stage is at <builddir>/build.ninja, and will contain all the
// rules generated by the primary builder. In addition, the bootstrap code
// adds a phony rule "blueprint_tools" that depends on all blueprint_go_binary
// rules (bpfmt, bpmodify, etc).
//
package bootstrap
