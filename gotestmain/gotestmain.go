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

package gotestmain

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"strings"
	"text/template"
)

var (
	output   = flag.String("o", "", "output filename")
	pkg      = flag.String("pkg", "", "test package")
	exitCode = 0
)

type data struct {
	Package string
	Tests   []string
}

func findTests(srcs []string) (tests []string) {
	for _, src := range srcs {
		f, err := parser.ParseFile(token.NewFileSet(), src, nil, 0)
		if err != nil {
			panic(err)
		}
		for _, obj := range f.Scope.Objects {
			if obj.Kind != ast.Fun || !strings.HasPrefix(obj.Name, "Test") {
				continue
			}
			tests = append(tests, obj.Name)
		}
	}
	return
}

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "error: must pass at least one input")
		exitCode = 1
		return
	}

	buf := &bytes.Buffer{}

	d := data{
		Package: *pkg,
		Tests:   findTests(flag.Args()),
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
	"testing"

	pkg "{{.Package}}"
)

var t = []testing.InternalTest{
{{range .Tests}}
	{"{{.}}", pkg.{{.}}},
{{end}}
}

func matchString(pat, str string) (bool, error) {
	return true, nil
}

func main() {
	testing.Main(matchString, t, nil, nil)
}
`))
