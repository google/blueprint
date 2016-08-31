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

import (
	"runtime"

	"github.com/google/blueprint"
)

func bootstrapVariable(name, template string, value func() string) blueprint.Variable {
	return pctx.VariableFunc(name, func(config interface{}) (string, error) {
		if c, ok := config.(ConfigInterface); ok && c.GeneratingBootstrapper() {
			return template, nil
		}
		return value(), nil
	})
}

var (
	// These variables are the only configuration needed by the boostrap
	// modules.  For the first bootstrap stage, they are set to the
	// variable name enclosed in "@@" so that their values can be easily
	// replaced in the generated Ninja file.
	srcDir = bootstrapVariable("srcDir", "@@SrcDir@@", func() string {
		return SrcDir
	})
	buildDir = bootstrapVariable("buildDir", "@@BuildDir@@", func() string {
		return BuildDir
	})
	goRoot = bootstrapVariable("goRoot", "@@GoRoot@@", func() string {
		return runtime.GOROOT()
	})
	compileCmd = bootstrapVariable("compileCmd", "@@GoCompile@@", func() string {
		return "$goRoot/pkg/tool/" + runtime.GOOS + "_" + runtime.GOARCH + "/compile"
	})
	linkCmd = bootstrapVariable("linkCmd", "@@GoLink@@", func() string {
		return "$goRoot/pkg/tool/" + runtime.GOOS + "_" + runtime.GOARCH + "/link"
	})
	bootstrapCmd = bootstrapVariable("bootstrapCmd", "@@Bootstrap@@", func() string {
		panic("bootstrapCmd is only available for minibootstrap")
	})
)

type ConfigInterface interface {
	// GeneratingBootstrapper should return true if this build invocation is
	// creating a .minibootstrap/build.ninja file to be used in a build
	// bootstrapping sequence.
	GeneratingBootstrapper() bool
	// GeneratingPrimaryBuilder should return true if this build invocation is
	// creating a .bootstrap/build.ninja file to be used to build the
	// primary builder
	GeneratingPrimaryBuilder() bool
}

type ConfigRemoveAbandonedFiles interface {
	// RemoveAbandonedFiles should return true if files listed in the
	// .ninja_log but not the output build.ninja file should be deleted.
	RemoveAbandonedFiles() bool
}

type ConfigBlueprintToolLocation interface {
	// BlueprintToolLocation can return a path name to install blueprint tools
	// designed for end users (bpfmt, bpmodify, and anything else using
	// blueprint_go_binary).
	BlueprintToolLocation() string
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
