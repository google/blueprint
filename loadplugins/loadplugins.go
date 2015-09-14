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
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"
)

var (
	output = flag.String("o", "", "output filename")
	pkg    = flag.String("p", "main", "package name")
)

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "error: must pass at least one input")
		os.Exit(1)
	}

	buf := &bytes.Buffer{}

	err := pluginTmpl.Execute(buf, struct {
		Package string
		Plugins []string
	}{
		filepath.Base(*pkg),
		flag.Args(),
	})
	if err != nil {
		panic(err)
	}

	err = ioutil.WriteFile(*output, buf.Bytes(), 0666)
	if err != nil {
		panic(err)
	}
}

var pluginTmpl = template.Must(template.New("pluginloader").Parse(`
package {{.Package}}

import (
{{range .Plugins}}
	_ "{{.}}"
{{end}}
)
`))
