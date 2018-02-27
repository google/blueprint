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
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"text/template"
)

var (
	output   = flag.String("o", "", "output filename")
	pkg      = flag.String("pkg", "", "test package")
	exitCode = 0
)

type data struct {
	Package                 string
	Tests                   []string
	HasMain                 bool
	MainStartTakesInterface bool
}

func findTests(srcs []string) (tests []string, hasMain bool) {
	for _, src := range srcs {
		f, err := parser.ParseFile(token.NewFileSet(), src, nil, 0)
		if err != nil {
			panic(err)
		}
		for _, obj := range f.Scope.Objects {
			if obj.Kind != ast.Fun || !strings.HasPrefix(obj.Name, "Test") {
				continue
			}
			if obj.Name == "TestMain" {
				hasMain = true
			} else {
				tests = append(tests, obj.Name)
			}
		}
	}
	sort.Strings(tests)
	return
}

// Returns true for go1.8+, where testing.MainStart takes an interface instead of a function
// as its first argument.
func mainStartTakesInterface() bool {
	return reflect.TypeOf(testing.MainStart).In(0).Kind() == reflect.Interface
}

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "error: must pass at least one input")
		exitCode = 1
		return
	}

	buf := &bytes.Buffer{}

	tests, hasMain := findTests(flag.Args())

	d := data{
		Package:                 *pkg,
		Tests:                   tests,
		HasMain:                 hasMain,
		MainStartTakesInterface: mainStartTakesInterface(),
	}

	err := testMainTmpl.Execute(buf, d)
	if err != nil {
		panic(err)
	}

	err = ioutil.WriteFile(*output, buf.Bytes(), 0666)
	if err != nil {
		panic(err)
	}
}

var testMainTmpl = template.Must(template.New("testMain").Parse(`
package main

import (
	"io"
{{if not .HasMain}}
	"os"
{{end}}
	"regexp"
	"testing"

	pkg "{{.Package}}"
)

var t = []testing.InternalTest{
{{range .Tests}}
	{"{{.}}", pkg.{{.}}},
{{end}}
}

var matchPat string
var matchRe *regexp.Regexp

type matchString struct{}

func MatchString(pat, str string) (result bool, err error) {
	if matchRe == nil || matchPat != pat {
		matchPat = pat
		matchRe, err = regexp.Compile(matchPat)
		if err != nil {
			return
		}
	}
	return matchRe.MatchString(str), nil
}

func (matchString) MatchString(pat, str string) (bool, error) {
	return MatchString(pat, str)
}

func (matchString) StartCPUProfile(w io.Writer) error {
	panic("shouldn't get here")
}

func (matchString) StopCPUProfile() {
}

func (matchString) WriteHeapProfile(w io.Writer) error {
    panic("shouldn't get here")
}

func (matchString) WriteProfileTo(string, io.Writer, int) error {
    panic("shouldn't get here")
}

func (matchString) ImportPath() string {
	return "{{.Package}}"
}

func (matchString) StartTestLog(io.Writer) {
	panic("shouldn't get here")
}

func (matchString) StopTestLog() error {
	panic("shouldn't get here")
}

func main() {
{{if .MainStartTakesInterface}}
	m := testing.MainStart(matchString{}, t, nil, nil)
{{else}}
	m := testing.MainStart(MatchString, t, nil, nil)
{{end}}
{{if .HasMain}}
	pkg.TestMain(m)
{{else}}
	os.Exit(m.Run())
{{end}}
}
`))
