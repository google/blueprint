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
	"strings"
	"unicode"

	"github.com/google/blueprint/parser"
)

var (
	// main operation modes
	list            = flag.Bool("l", false, "list files that would be modified by bpmodify")
	write           = flag.Bool("w", false, "write result to (source) file instead of stdout")
	doDiff          = flag.Bool("d", false, "display diffs instead of rewriting files")
	sortLists       = flag.Bool("s", false, "sort touched lists, even if they were unsorted")
	parameter       = flag.String("parameter", "deps", "name of parameter to modify on each module")
	targetedModules = new(identSet)
	addIdents       = new(identSet)
	removeIdents    = new(identSet)
)

func init() {
	flag.Var(targetedModules, "m", "comma or whitespace separated list of modules on which to operate")
	flag.Var(addIdents, "a", "comma or whitespace separated list of identifiers to add")
	flag.Var(removeIdents, "r", "comma or whitespace separated list of identifiers to remove")
	flag.Usage = usage
}

var (
	exitCode = 0
)

func report(err error) {
	fmt.Fprintln(os.Stderr, err)
	exitCode = 2
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] [path ...]\n", os.Args[0])
	flag.PrintDefaults()
}

// If in == nil, the source is the contents of the file with the given filename.
func processFile(filename string, in io.Reader, out io.Writer) error {
	if in == nil {
		f, err := os.Open(filename)
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}

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

	modified, errs := findModules(file)
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintln(os.Stderr, err)
		}
		fmt.Fprintln(os.Stderr, "continuing...")
	}

	if modified {
		res, err := parser.Print(file)
		if err != nil {
			return err
		}

		if *list {
			fmt.Fprintln(out, filename)
		}
		if *write {
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

		if !*list && !*write && !*doDiff {
			_, err = out.Write(res)
		}
	}

	return err
}

func findModules(file *parser.File) (modified bool, errs []error) {

	for _, def := range file.Defs {
		if module, ok := def.(*parser.Module); ok {
			for _, prop := range module.Properties {
				if prop.Name == "name" && prop.Value.Type() == parser.StringType {
					if targetedModule(prop.Value.Eval().(*parser.String).Value) {
						m, newErrs := processModule(module, prop.Name, file)
						errs = append(errs, newErrs...)
						modified = modified || m
					}
				}
			}
		}
	}

	return modified, errs
}

func processModule(module *parser.Module, moduleName string,
	file *parser.File) (modified bool, errs []error) {

	for _, prop := range module.Properties {
		if prop.Name == *parameter {
			modified, errs = processParameter(prop.Value, *parameter, moduleName, file)
			return
		}
	}

	prop := parser.Property{Name: *parameter, Value: &parser.List{}}
	modified, errs = processParameter(prop.Value, *parameter, moduleName, file)

	if modified {
		module.Properties = append(module.Properties, &prop)
	}

	return modified, errs
}

func processParameter(value parser.Expression, paramName, moduleName string,
	file *parser.File) (modified bool, errs []error) {
	if _, ok := value.(*parser.Variable); ok {
		return false, []error{fmt.Errorf("parameter %s in module %s is a variable, unsupported",
			paramName, moduleName)}
	}

	if _, ok := value.(*parser.Operator); ok {
		return false, []error{fmt.Errorf("parameter %s in module %s is an expression, unsupported",
			paramName, moduleName)}
	}

	list, ok := value.(*parser.List)
	if !ok {
		return false, []error{fmt.Errorf("expected parameter %s in module %s to be list, found %s",
			paramName, moduleName, value.Type().String())}
	}

	wasSorted := parser.ListIsSorted(list)

	for _, a := range addIdents.idents {
		m := parser.AddStringToList(list, a)
		modified = modified || m
	}

	for _, r := range removeIdents.idents {
		m := parser.RemoveStringFromList(list, r)
		modified = modified || m
	}

	if (wasSorted || *sortLists) && modified {
		parser.SortList(file, list)
	}

	return modified, nil
}

func targetedModule(name string) bool {
	if targetedModules.all {
		return true
	}
	for _, m := range targetedModules.idents {
		if m == name {
			return true
		}
	}

	return false
}

func visitFile(path string, f os.FileInfo, err error) error {
	if err == nil && f.Name() == "Blueprints" {
		err = processFile(path, nil, os.Stdout)
	}
	if err != nil {
		report(err)
	}
	return nil
}

func walkDir(path string) {
	filepath.Walk(path, visitFile)
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			report(fmt.Errorf("error: %s", err))
		}
		os.Exit(exitCode)
	}()

	flag.Parse()

	if flag.NArg() == 0 {
		if *write {
			report(fmt.Errorf("error: cannot use -w with standard input"))
			return
		}
		if err := processFile("<standard input>", os.Stdin, os.Stdout); err != nil {
			report(err)
		}
		return
	}

	if len(targetedModules.idents) == 0 {
		report(fmt.Errorf("-m parameter is required"))
		return
	}

	if len(addIdents.idents) == 0 && len(removeIdents.idents) == 0 {
		report(fmt.Errorf("-a or -r parameter is required"))
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
			if err := processFile(path, nil, os.Stdout); err != nil {
				report(err)
			}
		}
	}
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

	data, err = exec.Command("diff", "-uw", f1.Name(), f2.Name()).CombinedOutput()
	if len(data) > 0 {
		// diff exits with a non-zero status when the files don't match.
		// Ignore that failure as long as we get output.
		err = nil
	}
	return

}

type identSet struct {
	idents []string
	all    bool
}

func (m *identSet) String() string {
	return strings.Join(m.idents, ",")
}

func (m *identSet) Set(s string) error {
	m.idents = strings.FieldsFunc(s, func(c rune) bool {
		return unicode.IsSpace(c) || c == ','
	})
	if len(m.idents) == 1 && m.idents[0] == "*" {
		m.all = true
	}
	return nil
}

func (m *identSet) Get() interface{} {
	return m.idents
}
