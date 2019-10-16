package simplebp

import (
	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
	"path/filepath"
	"strings"
)

var (
	defaultCFlags = []string{
		"-Wall",
		"-std=c99",
		"-O2",
	}
	defaultCxxFlags = []string{
		"-Wall",
		"-std=c++11",
		"-O2",
	}
	defaultLdFlags = []string{}

	// Create the package context, used as a scope for Ninja statements.
	pctx = blueprint.NewPackageContext("bp/build/simplebp")

	// Create a Ninja rule for compiling C files. The variables in the
	// command and description will be filled in either via static variables
	// (e.g., $cc) or from Ninja build statements (e.g., $cFlags).
	ccRule = pctx.StaticRule("cc",
		blueprint.RuleParams{
			Command:     "$cc -MMD -MF $out.d $cFlags $incPaths -c $in -o $out",
			Depfile:     "$out.d",
			Deps:        blueprint.DepsGCC,
			Description: "CC   $out",
		},
		"cFlags", "incPaths")

	// Create a Ninja rule for compiling C++ files. This should be collapsed
	// into the above rule.
	cxxRule = pctx.StaticRule("cxx",
		blueprint.RuleParams{
			Command:     "$cxx -MMD -MF $out.d $cFlags $incPaths -c $in -o $out",
			Depfile:     "$out.d",
			Deps:        blueprint.DepsGCC,
			Description: "CXX  $out",
		},
		"cFlags", "incPaths")

	// Create a Ninja rule for linking objects. The $ldFlags can be set to
	// determine whether the output is a shared library or a binary.
	linkRule = pctx.StaticRule("link",
		blueprint.RuleParams{
			Command:     "$ld $ldFlags $in -o $out $ldPaths $libs",
			Description: "LINK $out",
		},
		"ldFlags", "ldPaths", "libs")
)

func init() {
	// Create Ninja variables to hold the names of the compilers and linker.
	pctx.StaticVariable("cc", "gcc")
	pctx.StaticVariable("cxx", "g++")
	pctx.StaticVariable("ld", "g++")

	// Create Ninja variables to hold the default flags (defined above) so
	// they don't have to be listed with every Ninja build statement.
	pctx.StaticVariable("defaultCFlags", strings.Join(defaultCFlags, " "))
	pctx.StaticVariable("defaultCxxFlags", strings.Join(defaultCxxFlags, " "))
	pctx.StaticVariable("defaultLdFlags", strings.Join(defaultLdFlags, " "))
}

// BaseProperties are Blueprints properties that apply to all modules in this
// package.
type BaseProperties struct {
	Srcs    []string // The source inputs
	Cflags  []string // The C flags to use while compiling
	Ldflags []string // The linker flags
	Deps    []string // dependencies
}

// A BinaryModule produces a binary from the sources and flags listed in
// properties.
type BinaryModule struct {
	blueprint.SimpleName
	properties BaseProperties // Base properties shared by all modules
	output     string         // The output artifact for the module
}

// SharedLibProperties extend BaseProperties by including a property for
// exporting the include paths that should be inherited by modules that depend
// on the SharedLibModule.
type SharedLibProperties struct {
	blueprint.SimpleName
	BaseProperties
	IncludePaths []string // Paths exported to dependers for include files
}

// A SharedLibModule produces a shared library from the sources and flags listed
// in properties. In addition, it stores intermediate data like the path to the
// output, which allows modules that depend on this one to add the appropriate
// -L flag during linking.
type SharedLibModule struct {
	blueprint.SimpleName
	properties SharedLibProperties
	incPaths   []string // Paths exported to dependers for include files
	output     string   // The output artifact for the module
	outPath    string   // The path exported to dependers for linking
}

// Factory function for creating BinaryModules.
func NewCcBinary() (blueprint.Module, []interface{}) {
	module := new(BinaryModule)
	properties := &module.properties
	return module, []interface{}{properties, &module.SimpleName.Properties}
}

func (f *BinaryModule) DynamicDependencies(ctx blueprint.DynamicDependerModuleContext) []string {
	return f.properties.Deps
}

// GenerateBuildActions takes a module created in a Blueprints file and turns it
// into Ninja build statements. The Ninja rules created in the package context
// are used to reduce duplication.
func (m *BinaryModule) GenerateBuildActions(ctx blueprint.ModuleContext) {
	// Fetch our config to get the variables set during bootstrapping.
	config := ctx.Config().(*config)

	// Construct our list of inputs. By default, the sources will just be
	// the names of the files relative to the path of the Blueprints file
	// defining this module. We prefix all the paths with the path of this
	// file (accessible as ctx.ModuleDir()) so that the paths are relative
	// to the top source dir.
	srcs := pathtools.PrefixPaths(m.properties.Srcs, ctx.ModuleDir())
	// Construct the path to the output binary. This matches the name of the
	// module, placed in the build dir with the same directory structure as
	// the module dir relative to the source dir.
	m.output = filepath.Join(config.buildDir, ctx.ModuleDir(), ctx.ModuleName())

	// Use any cflags defined by the module.
	cflags := m.properties.Cflags

	// Gather dependency data. Any modules this binary depends on can export
	// flags and paths that are needed to build the binary.
	deps := new(depsData)
	ctx.VisitDepsDepthFirst(func(module blueprint.Module) {
		gatherDepData(module, ctx, deps)
	})

	// Generate the Ninja build statements to compile the sources and gather
	// the resulting object file paths.
	objs := compileSrcsToObjs(ctx, srcs, cflags, deps.includePaths, config.buildDir)

	// Start with the default ld flags and append any flags defined by the
	// module.
	ldflags := []string{"${defaultLdFlags}"}
	ldflags = append(ldflags, m.properties.Ldflags...)
	for _, linkPath := range deps.linkPaths {
		ldflags = append(ldflags, " -Wl,-rpath="+linkPath)
	}

	// Generate the Ninja build statements to link our objects into the
	// final output.
	compileObjsToOutput(ctx, objs, ldflags, deps.linkPaths, deps.libraryNames, deps.outputPaths, []string{m.output})

	// Create a "phony" rule with the name of this module. This allows a
	// user to build this binary with its module name rather than its
	// complete path in the build dir.
	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      blueprint.Phony,
		Outputs:   []string{ctx.ModuleName()},
		Implicits: []string{m.output},
	})
}

// Factory function for creating SharedLibModules.
func NewCcSharedLib() (blueprint.Module, []interface{}) {
	module := new(SharedLibModule)
	properties := &module.properties
	return module, []interface{}{properties, &module.SimpleName.Properties}
}

func (f *SharedLibModule) DynamicDependencies(ctx blueprint.DynamicDependerModuleContext) []string {
	return f.properties.Deps
}

// Create the Ninja build statements for shared libraries.
func (m *SharedLibModule) GenerateBuildActions(ctx blueprint.ModuleContext) {
	// Fetch our config to get the variables set during bootstrapping.
	config := ctx.Config().(*config)

	// Construct our list of inputs. By default, the sources will just be
	// the names of the files relative to the path of the Blueprints file
	// defining this module. We prefix all the paths with the path of this
	// file (accessible as ctx.ModuleDir()) so that the paths are relative
	// to the top source dir.
	srcs := pathtools.PrefixPaths(m.properties.Srcs, ctx.ModuleDir())
	// Create include paths relative to the top source dir rather than this
	// module's dir.
	m.incPaths = pathtools.PrefixPaths(m.properties.IncludePaths, ctx.ModuleDir())
	// Create the output path relative to the top source dir rather than
	// this module's build dir.
	m.outPath = filepath.Join(config.buildDir, ctx.ModuleDir())
	// Create the name of the resulting shared library, including its path
	// in the build dir.
	m.output = filepath.Join(m.outPath, "lib"+ctx.ModuleName()+".so")

	// Shared libraries use -fPIC as well as any other module-defined flags.
	cflags := []string{"-fPIC"}
	cflags = append(cflags, m.properties.Cflags...)

	// Gather dependency data. Any modules this binary depends on can export
	// flags and paths that are needed to build the binary.
	deps := &depsData{}
	ctx.VisitDepsDepthFirst(func(module blueprint.Module) {
		gatherDepData(module, ctx, deps)
	})

	m.incPaths = append(m.incPaths, deps.includePaths...)

	// Generate the Ninja build statements to compile the sources and gather
	// the resulting object file paths.
	objs := compileSrcsToObjs(ctx, srcs, cflags, m.incPaths, config.buildDir)

	// Start with the default ld flags for shared libraries and append any
	// flags defined by the module.
	ldflags := []string{"${defaultLdFlags}", "-shared"}
	ldflags = append(ldflags, m.properties.Ldflags...)

	// Generate the Ninja build statements to link our objects into the
	// final output.
	compileObjsToOutput(ctx, objs, ldflags, deps.linkPaths, deps.libraryNames, deps.outputPaths, []string{m.output})

	// Create a "phony" rule with the name of this module. This allows a
	// user to build this library with its output name rather than its
	// complete path in the build dir.
	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      blueprint.Phony,
		Outputs:   []string{"lib" + ctx.ModuleName() + ".so"},
		Implicits: []string{m.output},
	})
}

// depsData holds all of the values exported by dependencies.
type depsData struct {
	includePaths []string
	linkPaths    []string
	libraryNames []string
	outputPaths  []string
}

// Gather all the exported data from the module and add it to deps.
func gatherDepData(module blueprint.Module, ctx blueprint.ModuleContext, deps *depsData) {
	// At this time, only SharedLibModules can be used as dependencies.
	libModule, ok := module.(*SharedLibModule)
	if !ok {
		// TODO: report an error
		return
	}
	deps.includePaths = append(deps.includePaths, libModule.incPaths...)
	deps.linkPaths = append(deps.linkPaths, libModule.outPath)
	deps.libraryNames = append(deps.libraryNames, ctx.OtherModuleName(module))
	deps.outputPaths = append(deps.outputPaths, libModule.output)
}

// Generate the Ninja build statements for a list of sources and return the list
// of object files that will result.
func compileSrcsToObjs(ctx blueprint.ModuleContext, srcs []string, flags []string, includePaths []string, buildDir string) []string {
	config := ctx.Config().(*config)
	incPathFlags := make([]string, len(includePaths))
	for i, path := range includePaths {
		incPathFlags[i] = "-I" + config.srcDir + "/" + path
	}
	incStr := strings.Join(incPathFlags, " ")

	// Generate a Ninja build statement for each source file.
	objs := make([]string, len(srcs))
	for i, s := range srcs {
		// The rule and cflags for each source depend on the extension
		// of the file, to determine whether it's a C or C++ file.
		var rule blueprint.Rule
		var cflags []string
		switch filepath.Ext(s) {
		case ".c":
			rule = ccRule
			cflags = append(flags, "${defaultCFlags}")
		case ".cpp", ".cc", ".cxx":
			rule = cxxRule
			cflags = append(flags, "${defaultCxxFlags}")
		default:
			ctx.ModuleErrorf("unknown extension for %v", s)
			continue
		}
		flagStr := strings.Join(cflags, " ")

		// The object file has the same name as the source file, but
		// with its extension replaced by ".o".
		objs[i] = filepath.Join(buildDir, pathtools.ReplaceExtension(s, "o"))
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    rule,
			Inputs:  []string{filepath.Join(config.srcDir, s)},
			Outputs: []string{filepath.Join(config.buildDir, objs[i])},
			Args: map[string]string{
				"cFlags":   flagStr,
				"incPaths": incStr,
			},
		})
	}
	return objs
}

// Generate the Ninja build statement to link the list of objects into the
// resulting output file.
func compileObjsToOutput(ctx blueprint.ModuleContext, objs []string, flags []string, linkPaths []string, libNames []string, libOutputs []string, out []string) {
	flagStr := strings.Join(flags, " ")

	linkPathFlags := make([]string, len(linkPaths))
	for i, path := range linkPaths {
		linkPathFlags[i] = "-L" + path
	}
	linkPathStr := strings.Join(linkPathFlags, " ")

	libNameFlags := make([]string, len(libNames))
	for i, name := range libNames {
		libNameFlags[i] = "-l" + name
	}
	libNameStr := strings.Join(libNameFlags, " ")

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      linkRule,
		Inputs:    objs,
		Outputs:   out,
		Implicits: libOutputs,
		Args: map[string]string{
			"ldFlags": flagStr,
			"ldPaths": linkPathStr,
			"libs":    libNameStr,
		},
	})
}
