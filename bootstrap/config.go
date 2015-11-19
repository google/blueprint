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

package bootstrap

var (
	// These variables are the only configuration needed by the boostrap
	// modules.  They are always set to the variable name enclosed in "@@" so
	// that their values can be easily replaced in the generated Ninja file.
	srcDir            = pctx.StaticVariable("srcDir", "@@SrcDir@@")
	buildDir          = pctx.StaticVariable("buildDir", "@@BuildDir@@")
	goRoot            = pctx.StaticVariable("goRoot", "@@GoRoot@@")
	compileCmd        = pctx.StaticVariable("compileCmd", "@@GoCompile@@")
	linkCmd           = pctx.StaticVariable("linkCmd", "@@GoLink@@")
	bootstrapCmd      = pctx.StaticVariable("bootstrapCmd", "@@Bootstrap@@")
	bootstrapManifest = pctx.StaticVariable("bootstrapManifest",
		"@@BootstrapManifest@@")
)

type ConfigInterface interface {
	// GeneratingBootstrapper should return true if this build invocation is
	// creating a build.ninja.in file to be used in a build bootstrapping
	// sequence.
	GeneratingBootstrapper() bool
	// GeneratingPrimaryBuilder should return true if this build invocation is
	// creating a build.ninja.in file to be used to build the primary builder
	GeneratingPrimaryBuilder() bool
}

type ConfigRemoveAbandonedFiles interface {
	// RemoveAbandonedFiles should return true if files listed in the
	// .ninja_log but not the output build.ninja file should be deleted.
	RemoveAbandonedFiles() bool
}

type Stage int

const (
	StageBootstrap Stage = iota
	StagePrimary
	StageMain
)

type Config struct {
	stage Stage

	topLevelBlueprintsFile string

	runGoTests bool
}
