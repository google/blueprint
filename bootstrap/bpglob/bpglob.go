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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/google/blueprint/pathtools"
)

var (
	out = flag.String("o", "", "file to write list of files that match glob")

	excludes multiArg
)

func init() {
	flag.Var(&excludes, "e", "pattern to exclude from results")
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
	fmt.Fprintln(os.Stderr, "usage: bpglob -o out glob")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Parse()

	if *out == "" {
		fmt.Fprintln(os.Stderr, "error: -o is required")
		usage()
	}

	if flag.NArg() != 1 {
		usage()
	}

	_, err := pathtools.GlobWithDepFile(flag.Arg(0), *out, *out+".d", excludes)
	if err != nil {
		// Globs here were already run in the primary builder without error.  The only errors here should be if the glob
		// pattern was made invalid by a change in the pathtools glob implementation, in which case the primary builder
		// needs to be rerun anyways.  Update the output file with something that will always cause the primary builder
		// to rerun.
		s := fmt.Sprintf("%s: error: %s\n", time.Now().Format(time.StampNano), err.Error())
		err := ioutil.WriteFile(*out, []byte(s), 0666)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
			os.Exit(1)
		}
	}
}
