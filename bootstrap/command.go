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
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"

	"github.com/google/blueprint"
	"github.com/google/blueprint/deptools"
)

var (
	outFile          string
	depFile          string
	timestampFile    string
	timestampDepFile string
	manifestFile     string
	docFile          string
	cpuprofile       string
	runGoTests       bool

	BuildDir string
)

func init() {
	flag.StringVar(&outFile, "o", "build.ninja.in", "the Ninja file to output")
	flag.StringVar(&BuildDir, "b", ".", "the build output directory")
	flag.StringVar(&depFile, "d", "", "the dependency file to output")
	flag.StringVar(&timestampFile, "timestamp", "", "file to write before the output file")
	flag.StringVar(&timestampDepFile, "timestampdep", "", "the dependency file for the timestamp file")
	flag.StringVar(&manifestFile, "m", "", "the bootstrap manifest file")
	flag.StringVar(&docFile, "docs", "", "build documentation file to output")
	flag.StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")
	flag.BoolVar(&runGoTests, "t", false, "build and run go tests during bootstrap")
}

func Main(ctx *blueprint.Context, config interface{}, extraNinjaFileDeps ...string) {
	if !flag.Parsed() {
		flag.Parse()
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			fatalf("error opening cpuprofile: %s", err)
		}
		pprof.StartCPUProfile(f)
		defer f.Close()
		defer pprof.StopCPUProfile()
	}

	if flag.NArg() != 1 {
		fatalf("no Blueprints file specified")
	}

	stage := StageMain
	if c, ok := config.(ConfigInterface); ok {
		if c.GeneratingBootstrapper() {
			stage = StageBootstrap
		}
		if c.GeneratingPrimaryBuilder() {
			stage = StagePrimary
		}
	}

	bootstrapConfig := &Config{
		stage: stage,
		topLevelBlueprintsFile: flag.Arg(0),
		runGoTests:             runGoTests,
	}

	ctx.RegisterBottomUpMutator("bootstrap_plugin_deps", pluginDeps)
	ctx.RegisterModuleType("bootstrap_go_package", newGoPackageModuleFactory(bootstrapConfig))
	ctx.RegisterModuleType("bootstrap_core_go_binary", newGoBinaryModuleFactory(bootstrapConfig, StageBootstrap))
	ctx.RegisterModuleType("bootstrap_go_binary", newGoBinaryModuleFactory(bootstrapConfig, StagePrimary))
	ctx.RegisterTopDownMutator("bootstrap_stage", propagateStageBootstrap)
	ctx.RegisterSingletonType("bootstrap", newSingletonFactory(bootstrapConfig))

	deps, errs := ctx.ParseBlueprintsFiles(bootstrapConfig.topLevelBlueprintsFile)
	if len(errs) > 0 {
		fatalErrors(errs)
	}

	// Add extra ninja file dependencies
	deps = append(deps, extraNinjaFileDeps...)

	errs = ctx.ResolveDependencies(config)
	if len(errs) > 0 {
		fatalErrors(errs)
	}

	if docFile != "" {
		err := writeDocs(ctx, filepath.Dir(bootstrapConfig.topLevelBlueprintsFile), docFile)
		if err != nil {
			fatalErrors([]error{err})
		}
		return
	}

	extraDeps, errs := ctx.PrepareBuildActions(config)
	if len(errs) > 0 {
		fatalErrors(errs)
	}
	deps = append(deps, extraDeps...)

	buf := bytes.NewBuffer(nil)
	err := ctx.WriteBuildFile(buf)
	if err != nil {
		fatalf("error generating Ninja file contents: %s", err)
	}

	const outFilePermissions = 0666
	if timestampFile != "" {
		err := ioutil.WriteFile(timestampFile, []byte{}, outFilePermissions)
		if err != nil {
			fatalf("error writing %s: %s", timestampFile, err)
		}

		if timestampDepFile != "" {
			err := deptools.WriteDepFile(timestampDepFile, timestampFile, deps)
			if err != nil {
				fatalf("error writing depfile: %s", err)
			}
		}
	}

	err = ioutil.WriteFile(outFile, buf.Bytes(), outFilePermissions)
	if err != nil {
		fatalf("error writing %s: %s", outFile, err)
	}

	if depFile != "" {
		err := deptools.WriteDepFile(depFile, outFile, deps)
		if err != nil {
			fatalf("error writing depfile: %s", err)
		}
		err = deptools.WriteDepFile(depFile+".timestamp", outFile+".timestamp", deps)
		if err != nil {
			fatalf("error writing depfile: %s", err)
		}
	}

	if c, ok := config.(ConfigRemoveAbandonedFiles); !ok || c.RemoveAbandonedFiles() {
		srcDir := filepath.Dir(bootstrapConfig.topLevelBlueprintsFile)
		err := removeAbandonedFiles(ctx, bootstrapConfig, srcDir, manifestFile)
		if err != nil {
			fatalf("error removing abandoned files: %s", err)
		}
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
	fmt.Print("\n")
	os.Exit(1)
}

func fatalErrors(errs []error) {
	red := "\x1b[31m"
	unred := "\x1b[0m"

	for _, err := range errs {
		switch err := err.(type) {
		case *blueprint.Error:
			fmt.Printf("%serror:%s %s\n", red, unred, err.Error())
		default:
			fmt.Printf("%sinternal error:%s %s\n", red, unred, err)
		}
	}
	os.Exit(1)
}
