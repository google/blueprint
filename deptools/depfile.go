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

package deptools

import (
	"fmt"
	"os"
	"strings"
)

var (
	pathEscaper = strings.NewReplacer(
		`\`, `\\`,
		` `, `\ `,
		`#`, `\#`,
		`*`, `\*`,
		`[`, `\[`,
		`|`, `\|`)
)

// WriteDepFile creates a new gcc-style depfile and populates it with content
// indicating that target depends on deps.
func WriteDepFile(filename, target string, deps []string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	var escapedDeps []string

	for _, dep := range deps {
		escapedDeps = append(escapedDeps, pathEscaper.Replace(dep))
	}

	_, err = fmt.Fprintf(f, "%s: \\\n %s\n", target,
		strings.Join(escapedDeps, " \\\n "))
	if err != nil {
		return err
	}

	return nil
}
