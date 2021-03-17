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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/blueprint"
)

func bootstrapVariable(name string, value func(BootstrapConfig) string) blueprint.Variable {
	return pctx.VariableFunc(name, func(config interface{}) (string, error) {
		c, ok := config.(BootstrapConfig)
		if !ok {
			panic(fmt.Sprintf("Bootstrap rules were passed a configuration that does not include theirs, config=%q",
				config))
		}
		return value(c), nil
	})
}

var (
	// These variables are the only configuration needed by the bootstrap
	// modules.
	srcDirVariable = bootstrapVariable("srcDir", func(c BootstrapConfig) string {
		return c.SrcDir()
	})
	buildDirVariable = bootstrapVariable("buildDir", func(c BootstrapConfig) string {
		return c.BuildDir()
	})
	ninjaBuildDirVariable = bootstrapVariable("ninjaBuildDir", func(c BootstrapConfig) string {
		return c.NinjaBuildDir()
	})
	goRootVariable = bootstrapVariable("goRoot", func(c BootstrapConfig) string {
		goroot := runtime.GOROOT()
		// Prefer to omit absolute paths from the ninja file
		if cwd, err := os.Getwd(); err == nil {
			if relpath, err := filepath.Rel(cwd, goroot); err == nil {
				if !strings.HasPrefix(relpath, "../") {
					goroot = relpath
				}
			}
		}
		return goroot
	})
	compileCmdVariable = bootstrapVariable("compileCmd", func(c BootstrapConfig) string {
		return "$goRoot/pkg/tool/" + runtime.GOOS + "_" + runtime.GOARCH + "/compile"
	})
	linkCmdVariable = bootstrapVariable("linkCmd", func(c BootstrapConfig) string {
		return "$goRoot/pkg/tool/" + runtime.GOOS + "_" + runtime.GOARCH + "/link"
	})
	debugFlagsVariable = bootstrapVariable("debugFlags", func(c BootstrapConfig) string {
		if c.DebugCompilation() {
			return "-N -l"
		} else {
			return ""
		}
	})
)

type BootstrapConfig interface {
	// The top-level directory of the source tree
	SrcDir() string

	// The directory where files emitted during bootstrapping are located.
	// Usually NinjaBuildDir() + "/soong".
	BuildDir() string

	// The output directory for the build.
	NinjaBuildDir() string

	// Whether to compile Go code in such a way that it can be debugged
	DebugCompilation() bool
}

type ConfigRemoveAbandonedFilesUnder interface {
	// RemoveAbandonedFilesUnder should return two slices:
	// - a slice of path prefixes that will be cleaned of files that are no
	//   longer active targets, but are listed in the .ninja_log.
	// - a slice of paths that are exempt from cleaning
	RemoveAbandonedFilesUnder(buildDir string) (under, except []string)
}

type ConfigBlueprintToolLocation interface {
	// BlueprintToolLocation can return a path name to install blueprint tools
	// designed for end users (bpfmt, bpmodify, and anything else using
	// blueprint_go_binary).
	BlueprintToolLocation() string
}

type StopBefore int

const (
	StopBeforePrepareBuildActions StopBefore = 1
	StopBeforeWriteNinja          StopBefore = 2
)

type ConfigStopBefore interface {
	StopBefore() StopBefore
}

type Stage int

const (
	StagePrimary Stage = iota
	StageMain
)

type Config struct {
	stage Stage

	topLevelBlueprintsFile string

	emptyNinjaFile bool
	runGoTests     bool
	useValidations bool
	moduleListFile string
}
