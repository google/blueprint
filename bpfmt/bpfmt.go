// Mostly copied from Go's src/cmd/gofmt:
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/google/blueprint/parser"
)

var (
	// main operation modes
	list                = flag.Bool("l", false, "list files whose formatting differs from bpfmt's")
	overwriteSourceFile = flag.Bool("w", false, "write result to (source) file")
	writeToStout        = flag.Bool("o", false, "write result to stdout")
	doDiff              = flag.Bool("d", false, "display diffs instead of rewriting files")
	sortLists           = flag.Bool("s", false, "sort arrays")
)

var (
	exitCode = 0
)

func report(err error) {
	fmt.Fprintln(os.Stderr, err)
	exitCode = 2
}

func usage() {
	usageViolation("")
}

func usageViolation(violation string) {
	fmt.Fprintln(os.Stderr, violation)
	fmt.Fprintln(os.Stderr, "usage: bpfmt [flags] [path ...]")
	flag.PrintDefaults()
	os.Exit(2)
}

func processFile(filename string, out io.Writer) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return processReader(filename, f, out)
}

func processReader(filename string, in io.Reader, out io.Writer) error {
	src, err := ioutil.ReadAll(in)
	if err != nil {
		return err
	}

	r := bytes.NewBuffer(src)

	file, errs := parser.Parse(filename, r, parser.NewScope(nil))
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintln(os.Stderr, err)
		}
		return fmt.Errorf("%d parsing errors", len(errs))
	}

	if *sortLists {
		parser.SortLists(file)
	}

	res, err := parser.Print(file)
	if err != nil {
		return err
	}

	if !bytes.Equal(src, res) {
		// formatting has changed
		if *list {
			fmt.Fprintln(out, filename)
		}
		if *overwriteSourceFile {
			err = ioutil.WriteFile(filename, res, 0644)
			if err != nil {
				return err
			}
		}
		if *doDiff {
			data, err := diff(src, res)
			if err != nil {
				return fmt.Errorf("computing diff: %s", err)
			}
			fmt.Printf("diff %s bpfmt/%s\n", filename, filename)
			out.Write(data)
		}
	}

	if !*list && !*overwriteSourceFile && !*doDiff {
		_, err = out.Write(res)
	}

	return err
}

func walkDir(path string) {
	visitFile := func(path string, f os.FileInfo, err error) error {
		if err == nil && f.Name() == "Blueprints" {
			err = processFile(path, os.Stdout)
		}
		if err != nil {
			report(err)
		}
		return nil
	}

	filepath.Walk(path, visitFile)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if !*writeToStout && !*overwriteSourceFile && !*doDiff && !*list {
		usageViolation("one of -d, -l, -o, or -w is required")
	}

	if flag.NArg() == 0 {
		// file to parse is stdin
		if *overwriteSourceFile {
			fmt.Fprintln(os.Stderr, "error: cannot use -w with standard input")
			os.Exit(2)
		}
		if err := processReader("<standard input>", os.Stdin, os.Stdout); err != nil {
			report(err)
		}
		return
	}

	for i := 0; i < flag.NArg(); i++ {
		path := flag.Arg(i)
		switch dir, err := os.Stat(path); {
		case err != nil:
			report(err)
		case dir.IsDir():
			walkDir(path)
		default:
			if err := processFile(path, os.Stdout); err != nil {
				report(err)
			}
		}
	}

	os.Exit(exitCode)
}

func diff(b1, b2 []byte) (data []byte, err error) {
	f1, err := ioutil.TempFile("", "bpfmt")
	if err != nil {
		return
	}
	defer os.Remove(f1.Name())
	defer f1.Close()

	f2, err := ioutil.TempFile("", "bpfmt")
	if err != nil {
		return
	}
	defer os.Remove(f2.Name())
	defer f2.Close()

	f1.Write(b1)
	f2.Write(b2)

	data, err = exec.Command("diff", "-u", f1.Name(), f2.Name()).CombinedOutput()
	if len(data) > 0 {
		// diff exits with a non-zero status when the files don't match.
		// Ignore that failure as long as we get output.
		err = nil
	}
	return

}
