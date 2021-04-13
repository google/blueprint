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

// bpglob is the command line tool that checks if the list of files matching a glob has
// changed, and only updates the output file list if it has changed.  It is used to optimize
// out build.ninja regenerations when non-matching files are added.  See
// github.com/google/blueprint/bootstrap/glob.go for a longer description.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"github.com/google/blueprint/pathtools"
)

var (
	// flagSet is a flag.FlagSet with flag.ContinueOnError so that we can handle the versionMismatchError
	// error from versionArg.
	flagSet = flag.NewFlagSet("bpglob", flag.ContinueOnError)

	out = flagSet.String("o", "", "file to write list of files that match glob")

	excludes     multiArg
	versionMatch versionArg
)

func init() {
	flagSet.Var(&versionMatch, "v", "version number the command line was generated for")
	flagSet.Var(&excludes, "e", "pattern to exclude from results")
}

// bpglob is executed through the rules in build-globs.ninja to determine whether soong_build
// needs to rerun.  That means when the arguments accepted by bpglob change it will be called
// with the old arguments, then soong_build will rerun and update build-globs.ninja with the new
// arguments.
//
// To avoid having to maintain backwards compatibility with old arguments across the transition,
// a version argument is used to detect the transition in order to stop parsing arguments, touch the
// output file and exit immediately.  Aborting parsing arguments is necessary to handle parsing
// errors that would be fatal, for example the removal of a flag.  The version number in
// pathtools.BPGlobArgumentVersion should be manually incremented when the bpglob argument format
// changes.
//
// If the version argument is not passed then a version mismatch is assumed.

// versionArg checks the argument against pathtools.BPGlobArgumentVersion, returning a
// versionMismatchError error if it does not match.
type versionArg bool

var versionMismatchError = errors.New("version mismatch")

func (v *versionArg) String() string { return "" }

func (v *versionArg) Set(s string) error {
	vers, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("error parsing version argument: %w", err)
	}

	// Force the -o argument to come before the -v argument so that the output file can be
	// updated on error.
	if *out == "" {
		return fmt.Errorf("-o argument must be passed before -v")
	}

	if vers != pathtools.BPGlobArgumentVersion {
		return versionMismatchError
	}

	*v = true

	return nil
}

type multiArg []string

func (m *multiArg) String() string {
	return `""`
}

func (m *multiArg) Set(s string) error {
	*m = append(*m, s)
	return nil
}

func (m *multiArg) Get() interface{} {
	return m
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: bpglob -o out -v version [-e excludes ...] glob")
	flagSet.PrintDefaults()
	os.Exit(2)
}

func main() {
	// Save the command line flag error output to a buffer, the flag package unconditionally
	// writes an error message to the output on error, and we want to hide the error for the
	// version mismatch case.
	flagErrorBuffer := &bytes.Buffer{}
	flagSet.SetOutput(flagErrorBuffer)

	err := flagSet.Parse(os.Args[1:])

	if !versionMatch {
		// A version mismatch error occurs when the arguments written into build-globs.ninja
		// don't match the format expected by the bpglob binary.  This happens during the
		// first incremental build after bpglob is changed.  Handle this case by aborting
		// argument parsing and updating the output file with something that will always cause
		// the primary builder to rerun.
		// This can happen when there is no -v argument or if the -v argument doesn't match
		// pathtools.BPGlobArgumentVersion.
		writeErrorOutput(*out, versionMismatchError)
		os.Exit(0)
	}

	if err != nil {
		os.Stderr.Write(flagErrorBuffer.Bytes())
		fmt.Fprintln(os.Stderr, "error:", err.Error())
		usage()
	}

	if *out == "" {
		fmt.Fprintln(os.Stderr, "error: -o is required")
		usage()
	}

	if flagSet.NArg() != 1 {
		usage()
	}

	_, err = pathtools.GlobWithDepFile(flagSet.Arg(0), *out, *out+".d", excludes)
	if err != nil {
		// Globs here were already run in the primary builder without error.  The only errors here should be if the glob
		// pattern was made invalid by a change in the pathtools glob implementation, in which case the primary builder
		// needs to be rerun anyways.  Update the output file with something that will always cause the primary builder
		// to rerun.
		writeErrorOutput(*out, err)
	}
}

// writeErrorOutput writes an error to the output file with a timestamp to ensure that it is
// considered dirty by ninja.
func writeErrorOutput(path string, globErr error) {
	s := fmt.Sprintf("%s: error: %s\n", time.Now().Format(time.StampNano), globErr.Error())
	err := ioutil.WriteFile(path, []byte(s), 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		os.Exit(1)
	}
}
