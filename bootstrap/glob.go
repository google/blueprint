// Copyright 2016 Google Inc. All rights reserved.
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

package bootstrap

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
)

// This file supports globbing source files in Blueprints files.
//
// The build.ninja file needs to be regenerated any time a file matching the glob is added
// or removed.  The naive solution is to have the build.ninja file depend on all the
// traversed directories, but this will cause the regeneration step to run every time a
// non-matching file is added to a traversed directory, including backup files created by
// editors.
//
// The solution implemented here optimizes out regenerations when the directory modifications
// don't match the glob by having the build.ninja file depend on an intermedate file that
// is only updated when a file matching the glob is added or removed.  The intermediate file
// depends on the traversed directories via a depfile.  The depfile is used to avoid build
// errors if a directory is deleted - a direct dependency on the deleted directory would result
// in a build failure with a "missing and no known rule to make it" error.

var (
	globCmd = filepath.Join(miniBootstrapDir, "bpglob")

	// globRule rule traverses directories to produce a list of files that match $glob
	// and writes it to $out if it has changed, and writes the directories to $out.d
	GlobRule = pctx.StaticRule("GlobRule",
		blueprint.RuleParams{
			Command: fmt.Sprintf(`%s -o $out -v %d $excludes "$glob"`,
				globCmd, pathtools.BPGlobArgumentVersion),
			CommandDeps: []string{globCmd},
			Description: "glob $glob",

			Restat:  true,
			Deps:    blueprint.DepsGCC,
			Depfile: "$out.d",
		},
		"glob", "excludes")
)

// GlobFileContext is the subset of ModuleContext and SingletonContext needed by GlobFile
type GlobFileContext interface {
	Build(pctx blueprint.PackageContext, params blueprint.BuildParams)
}

// GlobFile creates a rule to write to fileListFile a list of the files that match the specified
// pattern but do not match any of the patterns specified in excludes.  The file will include
// appropriate dependencies to regenerate the file if and only if the list of matching files has
// changed.
func GlobFile(ctx GlobFileContext, pattern string, excludes []string, fileListFile string) {
	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    GlobRule,
		Outputs: []string{fileListFile},
		Args: map[string]string{
			"glob":     pattern,
			"excludes": joinWithPrefixAndQuote(excludes, "-e "),
		},
	})
}

func joinWithPrefixAndQuote(strs []string, prefix string) string {
	if len(strs) == 0 {
		return ""
	}

	if len(strs) == 1 {
		return prefix + `"` + strs[0] + `"`
	}

	n := len(" ") * (len(strs) - 1)
	for _, s := range strs {
		n += len(prefix) + len(s) + len(`""`)
	}

	ret := make([]byte, 0, n)
	for i, s := range strs {
		if i != 0 {
			ret = append(ret, ' ')
		}
		ret = append(ret, prefix...)
		ret = append(ret, '"')
		ret = append(ret, s...)
		ret = append(ret, '"')
	}
	return string(ret)
}

// globSingleton collects any glob patterns that were seen by Context and writes out rules to
// re-evaluate them whenever the contents of the searched directories change, and retrigger the
// primary builder if the results change.
type globSingleton struct {
	globLister func() []blueprint.GlobPath
	writeRule  bool
}

func globSingletonFactory(ctx *blueprint.Context) func() blueprint.Singleton {
	return func() blueprint.Singleton {
		return &globSingleton{
			globLister: ctx.Globs,
		}
	}
}

func (s *globSingleton) GenerateBuildActions(ctx blueprint.SingletonContext) {
	for _, g := range s.globLister() {
		fileListFile := g.FileListFile(ctx.Config().(BootstrapConfig).BuildDir())

		if s.writeRule {
			// We need to write the file list here so that it has an older modified date
			// than the build.ninja (otherwise we'd run the primary builder twice on
			// every new glob)
			//
			// We don't need to write the depfile because we're guaranteed that ninja
			// will run the command at least once (to record it into the ninja_log), so
			// the depfile will be loaded from that execution.
			err := pathtools.WriteFileIfChanged(absolutePath(fileListFile), g.FileList(), 0666)
			if err != nil {
				panic(fmt.Errorf("error writing %s: %s", fileListFile, err))
			}

			GlobFile(ctx, g.Pattern, g.Excludes, fileListFile)
		} else {
			// Make build.ninja depend on the fileListFile
			ctx.AddNinjaFileDeps(fileListFile)
		}
	}
}

func generateGlobNinjaFile(config interface{}, globLister func() []blueprint.GlobPath) ([]byte, []error) {
	ctx := blueprint.NewContext()
	ctx.RegisterSingletonType("glob", func() blueprint.Singleton {
		return &globSingleton{
			globLister: globLister,
			writeRule:  true,
		}
	})

	extraDeps, errs := ctx.ResolveDependencies(config)
	if len(extraDeps) > 0 {
		return nil, []error{fmt.Errorf("shouldn't have extra deps")}
	}
	if len(errs) > 0 {
		return nil, errs
	}

	extraDeps, errs = ctx.PrepareBuildActions(config)
	if len(extraDeps) > 0 {
		return nil, []error{fmt.Errorf("shouldn't have extra deps")}
	}
	if len(errs) > 0 {
		return nil, errs
	}

	buf := bytes.NewBuffer(nil)
	err := ctx.WriteBuildFile(buf)
	if err != nil {
		return nil, []error{err}
	}

	return buf.Bytes(), nil
}
