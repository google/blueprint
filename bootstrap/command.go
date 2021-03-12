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
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"runtime/trace"

	"github.com/google/blueprint"
	"github.com/google/blueprint/deptools"
)

var (
	outFile        string
	globFile       string
	depFile        string
	docFile        string
	cpuprofile     string
	memprofile     string
	traceFile      string
	runGoTests     bool
	useValidations bool
	noGC           bool
	emptyNinjaFile bool
	absSrcDir      string

	BuildDir       string
	ModuleListFile string
	NinjaBuildDir  string
	SrcDir         string
)

func init() {
	flag.StringVar(&outFile, "o", "build.ninja", "the Ninja file to output")
	flag.StringVar(&globFile, "globFile", "build-globs.ninja", "the Ninja file of globs to output")
	flag.StringVar(&BuildDir, "b", ".", "the build output directory")
	flag.StringVar(&NinjaBuildDir, "n", "", "the ninja builddir directory")
	flag.StringVar(&depFile, "d", "", "the dependency file to output")
	flag.StringVar(&docFile, "docs", "", "build documentation file to output")
	flag.StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")
	flag.StringVar(&traceFile, "trace", "", "write trace to file")
	flag.StringVar(&memprofile, "memprofile", "", "write memory profile to file")
	flag.BoolVar(&noGC, "nogc", false, "turn off GC for debugging")
	flag.BoolVar(&runGoTests, "t", false, "build and run go tests during bootstrap")
	flag.BoolVar(&useValidations, "use-validations", false, "use validations to depend on go tests")
	flag.StringVar(&ModuleListFile, "l", "", "file that lists filepaths to parse")
	flag.BoolVar(&emptyNinjaFile, "empty-ninja-file", false, "write out a 0-byte ninja file")
}

func Main(ctx *blueprint.Context, config interface{}, extraNinjaFileDeps ...string) {
	if !flag.Parsed() {
		flag.Parse()
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	if noGC {
		debug.SetGCPercent(-1)
	}

	absSrcDir = ctx.SrcDir()

	if cpuprofile != "" {
		f, err := os.Create(absolutePath(cpuprofile))
		if err != nil {
			fatalf("error opening cpuprofile: %s", err)
		}
		pprof.StartCPUProfile(f)
		defer f.Close()
		defer pprof.StopCPUProfile()
	}

	if traceFile != "" {
		f, err := os.Create(absolutePath(traceFile))
		if err != nil {
			fatalf("error opening trace: %s", err)
		}
		trace.Start(f)
		defer f.Close()
		defer trace.Stop()
	}

	if flag.NArg() != 1 {
		fatalf("no Blueprints file specified")
	}

	SrcDir = filepath.Dir(flag.Arg(0))
	if ModuleListFile != "" {
		ctx.SetModuleListFile(ModuleListFile)
		extraNinjaFileDeps = append(extraNinjaFileDeps, ModuleListFile)
	} else {
		fatalf("-l <moduleListFile> is required and must be nonempty")
	}
	filesToParse, err := ctx.ListModulePaths(SrcDir)
	if err != nil {
		fatalf("could not enumerate files: %v\n", err.Error())
	}

	if NinjaBuildDir == "" {
		NinjaBuildDir = BuildDir
	}

	stage := StageMain
	if c, ok := config.(interface{ GeneratingPrimaryBuilder() bool }); ok {
		if c.GeneratingPrimaryBuilder() {
			stage = StagePrimary
		}
	}

	bootstrapConfig := &Config{
		stage: stage,

		topLevelBlueprintsFile: flag.Arg(0),
		emptyNinjaFile:         emptyNinjaFile,
		runGoTests:             runGoTests,
		useValidations:         useValidations,
		moduleListFile:         ModuleListFile,
	}

	ctx.RegisterBottomUpMutator("bootstrap_plugin_deps", pluginDeps)
	ctx.RegisterModuleType("bootstrap_go_package", newGoPackageModuleFactory(bootstrapConfig))
	ctx.RegisterModuleType("bootstrap_go_binary", newGoBinaryModuleFactory(bootstrapConfig, false))
	ctx.RegisterModuleType("blueprint_go_binary", newGoBinaryModuleFactory(bootstrapConfig, true))
	ctx.RegisterSingletonType("bootstrap", newSingletonFactory(bootstrapConfig))

	ctx.RegisterSingletonType("glob", globSingletonFactory(ctx))

	deps, errs := ctx.ParseFileList(filepath.Dir(bootstrapConfig.topLevelBlueprintsFile), filesToParse, config)
	if len(errs) > 0 {
		fatalErrors(errs)
	}

	// Add extra ninja file dependencies
	deps = append(deps, extraNinjaFileDeps...)

	extraDeps, errs := ctx.ResolveDependencies(config)
	if len(errs) > 0 {
		fatalErrors(errs)
	}
	deps = append(deps, extraDeps...)

	if docFile != "" {
		err := writeDocs(ctx, config, absolutePath(docFile))
		if err != nil {
			fatalErrors([]error{err})
		}
		return
	}

	if c, ok := config.(ConfigStopBefore); ok {
		if c.StopBefore() == StopBeforePrepareBuildActions {
			return
		}
	}

	extraDeps, errs = ctx.PrepareBuildActions(config)
	if len(errs) > 0 {
		fatalErrors(errs)
	}
	deps = append(deps, extraDeps...)

	if c, ok := config.(ConfigStopBefore); ok {
		if c.StopBefore() == StopBeforeWriteNinja {
			return
		}
	}

	const outFilePermissions = 0666
	var out io.StringWriter
	var f *os.File
	var buf *bufio.Writer

	if emptyNinjaFile {
		if err := ioutil.WriteFile(absolutePath(outFile), []byte(nil), outFilePermissions); err != nil {
			fatalf("error writing empty Ninja file: %s", err)
		}
	}

	if stage != StageMain || !emptyNinjaFile {
		f, err = os.OpenFile(absolutePath(outFile), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, outFilePermissions)
		if err != nil {
			fatalf("error opening Ninja file: %s", err)
		}
		buf = bufio.NewWriterSize(f, 16*1024*1024)
		out = buf
	} else {
		out = ioutil.Discard.(io.StringWriter)
	}

	if globFile != "" {
		buffer, errs := generateGlobNinjaFile(config, ctx.Globs)
		if len(errs) > 0 {
			fatalErrors(errs)
		}

		err = ioutil.WriteFile(absolutePath(globFile), buffer, outFilePermissions)
		if err != nil {
			fatalf("error writing %s: %s", globFile, err)
		}
	}

	if depFile != "" {
		err := deptools.WriteDepFile(absolutePath(depFile), outFile, deps)
		if err != nil {
			fatalf("error writing depfile: %s", err)
		}
	}

	err = ctx.WriteBuildFile(out)
	if err != nil {
		fatalf("error writing Ninja file contents: %s", err)
	}

	if buf != nil {
		err = buf.Flush()
		if err != nil {
			fatalf("error flushing Ninja file contents: %s", err)
		}
	}

	if f != nil {
		err = f.Close()
		if err != nil {
			fatalf("error closing Ninja file: %s", err)
		}
	}

	if c, ok := config.(ConfigRemoveAbandonedFilesUnder); ok {
		under, except := c.RemoveAbandonedFilesUnder()
		err := removeAbandonedFilesUnder(ctx, SrcDir, BuildDir, under, except)
		if err != nil {
			fatalf("error removing abandoned files: %s", err)
		}
	}

	if memprofile != "" {
		f, err := os.Create(absolutePath(memprofile))
		if err != nil {
			fatalf("error opening memprofile: %s", err)
		}
		defer f.Close()
		pprof.WriteHeapProfile(f)
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
		case *blueprint.BlueprintError,
			*blueprint.ModuleError,
			*blueprint.PropertyError:
			fmt.Printf("%serror:%s %s\n", red, unred, err.Error())
		default:
			fmt.Printf("%sinternal error:%s %s\n", red, unred, err)
		}
	}
	os.Exit(1)
}

func absolutePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(absSrcDir, path)
}
