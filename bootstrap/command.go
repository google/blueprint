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
	outFile      string
	depFile      string
	checkFile    string
	manifestFile string
	cpuprofile   string
)

// topLevelBlueprintsFile is set by Main as a way to pass this information on to
// the bootstrap build manifest generators.  This information was not passed via
// the config object so as to allow the caller of Main to use whatever Config
// object it wants.
var topLevelBlueprintsFile string

func init() {
	flag.StringVar(&outFile, "o", "build.ninja.in", "the Ninja file to output")
	flag.StringVar(&depFile, "d", "", "the dependency file to output")
	flag.StringVar(&checkFile, "c", "", "the existing file to check against")
	flag.StringVar(&manifestFile, "m", "", "the bootstrap manifest file")
	flag.StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")
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

	ctx.RegisterModuleType("bootstrap_go_package", newGoPackageModule)
	ctx.RegisterModuleType("bootstrap_go_binary", newGoBinaryModule)
	ctx.RegisterSingletonType("bootstrap", newSingleton)

	if flag.NArg() != 1 {
		fatalf("no Blueprints file specified")
	}

	topLevelBlueprintsFile = flag.Arg(0)

	deps, errs := ctx.ParseBlueprintsFiles(topLevelBlueprintsFile)
	if len(errs) > 0 {
		fatalErrors(errs)
	}

	// Add extra ninja file dependencies
	deps = append(deps, extraNinjaFileDeps...)

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
	err = ioutil.WriteFile(outFile, buf.Bytes(), outFilePermissions)
	if err != nil {
		fatalf("error writing %s: %s", outFile, err)
	}

	if checkFile != "" {
		checkData, err := ioutil.ReadFile(checkFile)
		if err != nil {
			fatalf("error reading %s: %s", checkFile, err)
		}

		matches := buf.Len() == len(checkData)
		if matches {
			for i, value := range buf.Bytes() {
				if value != checkData[i] {
					matches = false
					break
				}
			}
		}

		if matches {
			// The new file content matches the check-file content, so we set
			// the new file's mtime and atime to match that of the check-file.
			checkFileInfo, err := os.Stat(checkFile)
			if err != nil {
				fatalf("error stat'ing %s: %s", checkFile, err)
			}

			time := checkFileInfo.ModTime()
			err = os.Chtimes(outFile, time, time)
			if err != nil {
				fatalf("error setting timestamps for %s: %s", outFile, err)
			}
		}
	}

	if depFile != "" {
		err := deptools.WriteDepFile(depFile, outFile, deps)
		if err != nil {
			fatalf("error writing depfile: %s", err)
		}
	}

	srcDir := filepath.Dir(topLevelBlueprintsFile)
	err = removeAbandonedFiles(ctx, config, srcDir, manifestFile)
	if err != nil {
		fatalf("error removing abandoned files: %s", err)
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
	os.Exit(1)
}

func fatalErrors(errs []error) {
	for _, err := range errs {
		switch err.(type) {
		case *blueprint.Error:
			_, _ = fmt.Printf("%s\n", err.Error())
		default:
			_, _ = fmt.Printf("internal error: %s\n", err)
		}
	}
	os.Exit(1)
}
