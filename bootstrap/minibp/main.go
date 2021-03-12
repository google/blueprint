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

package main

import (
	"flag"
	"path/filepath"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
)

var runAsPrimaryBuilder bool
var buildPrimaryBuilder bool

func init() {
	flag.BoolVar(&runAsPrimaryBuilder, "p", false, "run as a primary builder")
}

type Config struct {
	buildDir      string
	ninjaBuildDir string
}

func (c Config) GeneratingPrimaryBuilder() bool {
	return true
}

func (c Config) SrcDir() string {
	return "."
}

func (c Config) BuildDir() string {
	return c.buildDir
}

func (c Config) NinjaBuildDir() string {
	return c.ninjaBuildDir
}

func (c Config) RemoveAbandonedFilesUnder() (under, exempt []string) {
	under = []string{filepath.Join(bootstrap.BuildDir, ".bootstrap")}
	exempt = []string{filepath.Join(bootstrap.BuildDir, ".bootstrap", "build.ninja")}
	return
}

func main() {
	flag.Parse()

	ctx := blueprint.NewContext()
	if !runAsPrimaryBuilder {
		ctx.SetIgnoreUnknownModuleTypes(true)
	}

	config := Config{
		buildDir:      bootstrap.BuildDir,
		ninjaBuildDir: bootstrap.NinjaBuildDir,
	}

	bootstrap.Main(ctx, config)
}
