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

func bootstrapVariable(name string, value func() string) blueprint.Variable {
	return pctx.VariableFunc(name, func(config interface{}) (string, error) {
		return value(), nil
	})
}

var (
	// These variables are the only configuration needed by the boostrap
	// modules.
	srcDir = bootstrapVariable("srcDir", func() string {
		return SrcDir
	})
	buildDir = bootstrapVariable("buildDir", func() string {
		return BuildDir
	})
	ninjaBuildDir = bootstrapVariable("ninjaBuildDir", func() string {
		return NinjaBuildDir
	})
	goRoot = bootstrapVariable("goRoot", func() string {
		return runtime.GOROOT()
	})
	compileCmd = bootstrapVariable("compileCmd", func() string {
		return "$goRoot/pkg/tool/" + runtime.GOOS + "_" + runtime.GOARCH + "/compile"
	})
	linkCmd = bootstrapVariable("linkCmd", func() string {
		return "$goRoot/pkg/tool/" + runtime.GOOS + "_" + runtime.GOARCH + "/link"
	})
)

type ConfigInterface interface {
	// GeneratingPrimaryBuilder should return true if this build invocation is
	// creating a .bootstrap/build.ninja file to be used to build the
	// primary builder
	GeneratingPrimaryBuilder() bool
}

type ConfigRemoveAbandonedFilesUnder interface {
	// RemoveAbandonedFilesUnder should return a slice of path prefixes that
	// will be cleaned of files that are no longer active targets, but are
	// listed in the .ninja_log.
	RemoveAbandonedFilesUnder() []string
}

type ConfigBlueprintToolLocation interface {
	// BlueprintToolLocation can return a path name to install blueprint tools
	// designed for end users (bpfmt, bpmodify, and anything else using
	// blueprint_go_binary).
	BlueprintToolLocation() string
}

type Stage int

const (
	StagePrimary Stage = iota
	StageMain
)

type Config struct {
	stage Stage

	topLevelBlueprintsFile string

	runGoTests bool
}
