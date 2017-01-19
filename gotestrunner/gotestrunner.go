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

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

var (
	chdir = flag.String("p", "", "Change to a path before executing test")
	touch = flag.String("f", "", "Write a file on success")
)

// This will copy the stdout from the test process to our stdout
// unless it only contains "PASS\n".
func handleStdout(stdout io.Reader) {
	reader := bufio.NewReader(stdout)

	// This is intentionally 6 instead of 5 to check for EOF
	buf, _ := reader.Peek(6)
	if bytes.Equal(buf, []byte("PASS\n")) {
		return
	}

	io.Copy(os.Stdout, reader)
}

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "error: must pass at least one test executable")
		os.Exit(1)
	}

	test, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: Failed to locate test binary:", err)
	}

	cmd := exec.Command(test, flag.Args()[1:]...)
	if *chdir != "" {
		cmd.Dir = *chdir

		// GOROOT is commonly a relative path in Android, make it
		// absolute if we're changing directories.
		if absRoot, err := filepath.Abs(runtime.GOROOT()); err == nil {
			os.Setenv("GOROOT", absRoot)
		} else {
			fmt.Fprintln(os.Stderr, "error: Failed to locate GOROOT:", err)
		}
	}

	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	err = cmd.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	handleStdout(stdout)

	if err = cmd.Wait(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			if status, ok := e.Sys().(syscall.WaitStatus); ok && status.Exited() {
				os.Exit(status.ExitStatus())
			} else if status.Signaled() {
				fmt.Fprintf(os.Stderr, "test got signal %s\n", status.Signal())
				os.Exit(1)
			}
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *touch != "" {
		err = ioutil.WriteFile(*touch, []byte{}, 0666)
		if err != nil {
			panic(err)
		}
	}

	os.Exit(0)
}
