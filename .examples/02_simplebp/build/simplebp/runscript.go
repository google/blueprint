package simplebp

import (
	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"

	"bytes"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

var (
	// Create a Ninja rule for running a script with arguments. This reuses
	// the package context created in cc.go, available only within the
	// simplebp Go package. The name of the script and the entire arguments
	// string are passed as variables to the rule.
	scriptRule = pctx.StaticRule("script",
		blueprint.RuleParams{
			Command:     "$script $args",
			Description: "RUN  $script",
		},
		"script", "args")
)

// A ScriptModule executes a given script once for each input to produce an
// output. The output name and script arguments are templatized using the values
// of the input.
// TODO: make the outputs available to modules that depend on this module, for
// generating source files.
type ScriptModule struct {
	blueprint.SimpleName
	properties struct {
		Script string
		Inputs []string
		Output string
		Args   string
	}
}

// Factory function for creating ScriptModules.
func NewScript() (blueprint.Module, []interface{}) {
	module := new(ScriptModule)
	properties := &module.properties
	return module, []interface{}{properties, &module.SimpleName.Properties}
}

// A scriptInput is passed to the output and args templates, to make the
// basename, extension, and full name of the input available.
type scriptInput struct {
	Name      string // The full name of the input
	Basename  string // The basename of the input
	Extension string // The extension of the input
}

// The scriptArgs are passed to the output and args templates to produce the
// final output and args strings.
type scriptArgs struct {
	Input  scriptInput // The input to the script, with its basename and extension
	Output string      // The output of the script
}

// GenerateBuildActions takes the script, inputs, output, and args and creates
// Ninja build statements to run the script as prescribed.
func (m *ScriptModule) GenerateBuildActions(ctx blueprint.ModuleContext) {
	// Fetch our config to get the variables set during bootstrapping.
	config := ctx.Config().(*config)

	// Construct the path to the script, relative to the top source dir. If
	// the name of the script starts with "//", assume it is already
	// relative to the top source dir. Otherwise, it is relative to the
	// module dir, and we add the module dir as the prefix.
	var scriptPath string
	if s := m.properties.Script; strings.HasPrefix(s, "//") {
		scriptPath = filepath.Join(config.srcDir, s[2:])
	} else {
		scriptPath = filepath.Join(ctx.ModuleDir(), s)
	}

	// Check that the script exists and is executable. Note that this
	// prevents using scripts that could be created by other modules. This
	// is left as an exercise to the reader. :)
	if stat, err := os.Stat(scriptPath); err != nil {
		ctx.ModuleErrorf("Could not stat %v: %v", scriptPath, err)
		return
	} else if stat.Mode()&0111 == 0 {
		ctx.ModuleErrorf("%s is not an executable", scriptPath)
		return
	}

	// Construct our list of inputs. By default, the sources will just be
	// the names of the files relative to the path of the Blueprints file
	// defining this module. We prefix all the paths with the path of this
	// file (accessible as ctx.ModuleDir()) so that the paths are relative
	// to the top source dir.
	srcs := pathtools.PrefixPaths(m.properties.Inputs, ctx.ModuleDir())

	// Create Go templates from the output and args properties for this module.
	argsTmpl, err := template.New("args").Parse(m.properties.Args)
	if err != nil {
		ctx.ModuleErrorf("Could not parse script args: %v", err)
		return
	}
	outTmpl, err := template.New("out").Parse(m.properties.Output)
	if err != nil {
		ctx.ModuleErrorf("Could not parse output template: %v", err)
		return
	}

	for _, s := range srcs {
		// Initialize the scriptArgs for each input source.
		args := &scriptArgs{
			Input: scriptInput{
				Name:      filepath.Join(config.srcDir, s),
				Basename:  strings.TrimSuffix(s, filepath.Ext(s)),
				Extension: filepath.Ext(s),
			},
		}

		// Execute the output template with the args.
		outBuf := &bytes.Buffer{}
		if err = outTmpl.Execute(outBuf, args); err != nil {
			ctx.ModuleErrorf("Could not generate output: %v", err)
			return
		}

		// Set the result of the output template into args.
		args.Output = filepath.Join(config.buildDir, outBuf.String())

		// Execute the args template.
		argsBuf := &bytes.Buffer{}
		if err = argsTmpl.Execute(argsBuf, args); err != nil {
			ctx.ModuleErrorf("Could not generate args: %v", err)
			return
		}

		// This Ninja build statement has an implicit dependency on the
		// script itself, so any changes to the script will rerun the
		// script on the sources for this module.
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      scriptRule,
			Inputs:    []string{filepath.Join(config.srcDir, s)},
			Outputs:   []string{filepath.Join(config.buildDir, args.Output)},
			Implicits: []string{scriptPath},
			Args: map[string]string{
				"script": scriptPath,
				"args":   argsBuf.String(),
			},
		})
	}
}
