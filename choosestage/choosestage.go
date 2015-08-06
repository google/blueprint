// Copyright 2015 Google Inc. All rights reserved.
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

// Choose which ninja file (stage) to run next
//
// In the common case, this program takes a list of ninja files, compares their
// mtimes against their $file.timestamp mtimes, and picks the last up to date
// ninja file to output. That stage is expected to rebuild the next file in the
// list and call this program again. If none of the ninja files are considered
// dirty, the last stage is output.
//
// One exception is if the current stage's ninja file was rewritten, it will be
// run again.
//
// Another exception is if the source bootstrap file has been updated more
// recently than the first stage, the source file will be copied to the first
// stage, and output. This would be expected with a new source drop via git.
// The timestamp of the first file is not updated so that it can be regenerated
// with any local changes.

package choosestage

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

var (
	outputFile    string
	currentFile   string
	bootstrapFile string
	verbose       bool
)

func init() {
	flag.StringVar(&outputFile, "o", "", "Output file")
	flag.StringVar(&currentFile, "current", "", "Current stage's file")
	flag.StringVar(&bootstrapFile, "bootstrap", "", "Bootstrap file checked into source")
	flag.BoolVar(&verbose, "v", false, "Verbose mode")
}

func compareFiles(a, b string) (bool, error) {
	aData, err := ioutil.ReadFile(a)
	if err != nil {
		return false, err
	}

	bData, err := ioutil.ReadFile(b)
	if err != nil {
		return false, err
	}

	return bytes.Equal(aData, bData), nil
}

// If the source bootstrap reference file is newer, then we may have gotten
// other source updates too. So we need to restart everything with the file
// that was checked in instead of the bootstrap that we last built.
func copyBootstrapIfNecessary(bootstrapFile, filename string) (bool, error) {
	if bootstrapFile == "" {
		return false, nil
	}

	bootstrapStat, err := os.Stat(bootstrapFile)
	if err != nil {
		return false, err
	}

	fileStat, err := os.Stat(filename)
	if err != nil {
		return false, err
	}

	time := fileStat.ModTime()
	if !bootstrapStat.ModTime().After(time) {
		return false, nil
	}

	fmt.Printf("Newer source version of %s. Copying to %s\n", filepath.Base(bootstrapFile), filepath.Base(filename))
	if verbose {
		fmt.Printf("Source: %s\nBuilt:  %s\n", bootstrapStat.ModTime(), time)
	}

	data, err := ioutil.ReadFile(bootstrapFile)
	if err != nil {
		return false, err
	}

	err = ioutil.WriteFile(filename, data, 0666)
	if err != nil {
		return false, err
	}

	// Restore timestamp to force regeneration of the bootstrap.ninja.in
	err = os.Chtimes(filename, time, time)
	return true, err
}

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "Must specify at least one ninja file\n")
		os.Exit(1)
	}

	if outputFile == "" {
		fmt.Fprintf(os.Stderr, "Must specify an output file\n")
		os.Exit(1)
	}

	gotoFile := flag.Arg(0)
	if copied, err := copyBootstrapIfNecessary(bootstrapFile, flag.Arg(0)); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to copy bootstrap ninja file: %s\n", err)
		os.Exit(1)
	} else if !copied {
		for _, fileName := range flag.Args() {
			timestampName := fileName + ".timestamp"

			// If we're currently running this stage, and the build.ninja.in
			// file differs from the current stage file, then it has been rebuilt.
			// Restart the stage.
			if filepath.Clean(currentFile) == filepath.Clean(fileName) {
				if _, err := os.Stat(outputFile); !os.IsNotExist(err) {
					if ok, err := compareFiles(fileName, outputFile); err != nil {
						fmt.Fprintf(os.Stderr, "Failure when comparing files: %s\n", err)
						os.Exit(1)
					} else if !ok {
						fmt.Printf("Stage %s has changed, restarting\n", filepath.Base(fileName))
						gotoFile = fileName
						break
					}
				}
			}

			fileStat, err := os.Stat(fileName)
			if err != nil {
				// Regenerate this stage on error
				break
			}

			timestampStat, err := os.Stat(timestampName)
			if err != nil {
				// This file may not exist. There's no point for
				// the first stage to have one, as it should be
				// a subset of the second stage dependencies,
				// and both will return to the first stage.
				continue
			}

			if verbose {
				fmt.Printf("For %s:\n  file: %s\n  time: %s\n", fileName, fileStat.ModTime(), timestampStat.ModTime())
			}

			// If the timestamp file has a later modification time, that
			// means that this stage needs to be regenerated. Break, so
			// that we run the last found stage.
			if timestampStat.ModTime().After(fileStat.ModTime()) {
				break
			}

			gotoFile = fileName
		}
	}

	fmt.Printf("Choosing %s for next stage\n", filepath.Base(gotoFile))

	data, err := ioutil.ReadFile(gotoFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't read file: %s", err)
		os.Exit(1)
	}

	err = ioutil.WriteFile(outputFile, data, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't write file: %s", err)
		os.Exit(1)
	}
}
