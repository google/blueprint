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

package bootstrap

import (
	"fmt"
	"go/build"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
)

const bootstrapSubDir = ".bootstrap"
const miniBootstrapSubDir = ".minibootstrap"

var (
	pctx = blueprint.NewPackageContext("github.com/google/blueprint/bootstrap")

	goTestMainCmd   = pctx.StaticVariable("goTestMainCmd", filepath.Join(bootstrapDir, "bin", "gotestmain"))
	goTestRunnerCmd = pctx.StaticVariable("goTestRunnerCmd", filepath.Join(bootstrapDir, "bin", "gotestrunner"))
	pluginGenSrcCmd = pctx.StaticVariable("pluginGenSrcCmd", filepath.Join(bootstrapDir, "bin", "loadplugins"))

	parallelCompile = pctx.StaticVariable("parallelCompile", func() string {
		// Parallel compilation is only supported on >= go1.9
		for _, r := range build.Default.ReleaseTags {
			if r == "go1.9" {
				return fmt.Sprintf("-c %d", runtime.NumCPU())
			}
		}
		return ""
	}())

	compile = pctx.StaticRule("compile",
		blueprint.RuleParams{
			Command: "GOROOT='$goRoot' $compileCmd $parallelCompile -o $out " +
				"-p $pkgPath -complete $incFlags -pack $in",
			CommandDeps: []string{"$compileCmd"},
			Description: "compile $out",
		},
		"pkgPath", "incFlags")

	link = pctx.StaticRule("link",
		blueprint.RuleParams{
			Command:     "GOROOT='$goRoot' $linkCmd -o $out $libDirFlags $in",
			CommandDeps: []string{"$linkCmd"},
			Description: "link $out",
		},
		"libDirFlags")

	goTestMain = pctx.StaticRule("gotestmain",
		blueprint.RuleParams{
			Command:     "$goTestMainCmd -o $out -pkg $pkg $in",
			CommandDeps: []string{"$goTestMainCmd"},
			Description: "gotestmain $out",
		},
		"pkg")

	pluginGenSrc = pctx.StaticRule("pluginGenSrc",
		blueprint.RuleParams{
			Command:     "$pluginGenSrcCmd -o $out -p $pkg $plugins",
			CommandDeps: []string{"$pluginGenSrcCmd"},
			Description: "create $out",
		},
		"pkg", "plugins")

	test = pctx.StaticRule("test",
		blueprint.RuleParams{
			Command:     "$goTestRunnerCmd -p $pkgSrcDir -f $out -- $in -test.short",
			CommandDeps: []string{"$goTestRunnerCmd"},
			Description: "test $pkg",
		},
		"pkg", "pkgSrcDir")

	cp = pctx.StaticRule("cp",
		blueprint.RuleParams{
			Command:     "cp $in $out",
			Description: "cp $out",
		},
		"generator")

	bootstrap = pctx.StaticRule("bootstrap",
		blueprint.RuleParams{
			Command:     "BUILDDIR=$buildDir $bootstrapCmd -i $in",
			CommandDeps: []string{"$bootstrapCmd"},
			Description: "bootstrap $in",
			Generator:   true,
		})

	touch = pctx.StaticRule("touch",
		blueprint.RuleParams{
			Command:     "touch $out",
			Description: "touch $out",
		},
		"depfile", "generator")

	generateBuildNinja = pctx.StaticRule("build.ninja",
		blueprint.RuleParams{
			Command:     "$builder $extra -b $buildDir -n $ninjaBuildDir -d $out.d -o $out $in",
			CommandDeps: []string{"$builder"},
			Description: "$builder $out",
			Depfile:     "$out.d",
			Restat:      true,
		},
		"builder", "extra", "generator")

	// Work around a Ninja issue.  See https://github.com/martine/ninja/pull/634
	phony = pctx.StaticRule("phony",
		blueprint.RuleParams{
			Command:     "# phony $out",
			Description: "phony $out",
			Generator:   true,
		},
		"depfile")

	_ = pctx.VariableFunc("BinDir", func(config interface{}) (string, error) {
		return binDir(), nil
	})

	_ = pctx.VariableFunc("ToolDir", func(config interface{}) (string, error) {
		return toolDir(config), nil
	})

	docsDir = filepath.Join(bootstrapDir, "docs")

	bootstrapDir     = filepath.Join("$buildDir", bootstrapSubDir)
	miniBootstrapDir = filepath.Join("$buildDir", miniBootstrapSubDir)

	minibpFile = filepath.Join(miniBootstrapDir, "minibp")
)

type GoBinaryTool interface {
	InstallPath() string

	// So that other packages can't implement this interface
	isGoBinary()
}

func binDir() string {
	return filepath.Join(BuildDir, bootstrapSubDir, "bin")
}

func toolDir(config interface{}) string {
	if c, ok := config.(ConfigBlueprintToolLocation); ok {
		return filepath.Join(c.BlueprintToolLocation())
	}
	return filepath.Join(BuildDir, "bin")
}

func pluginDeps(ctx blueprint.BottomUpMutatorContext) {
	if pkg, ok := ctx.Module().(*goPackage); ok {
		for _, plugin := range pkg.properties.PluginFor {
			ctx.AddReverseDependency(ctx.Module(), nil, plugin)
		}
	}
}

type goPackageProducer interface {
	GoPkgRoot() string
	GoPackageTarget() string
	GoTestTargets() []string
}

func isGoPackageProducer(module blueprint.Module) bool {
	_, ok := module.(goPackageProducer)
	return ok
}

type goPluginProvider interface {
	GoPkgPath() string
	IsPluginFor(string) bool
}

func isGoPluginFor(name string) func(blueprint.Module) bool {
	return func(module blueprint.Module) bool {
		if plugin, ok := module.(goPluginProvider); ok {
			return plugin.IsPluginFor(name)
		}
		return false
	}
}

func isBootstrapModule(module blueprint.Module) bool {
	_, isPackage := module.(*goPackage)
	_, isBinary := module.(*goBinary)
	return isPackage || isBinary
}

func isBootstrapBinaryModule(module blueprint.Module) bool {
	_, isBinary := module.(*goBinary)
	return isBinary
}

// A goPackage is a module for building Go packages.
type goPackage struct {
	blueprint.SimpleName
	properties struct {
		Deps      []string
		PkgPath   string
		Srcs      []string
		TestSrcs  []string
		PluginFor []string

		Darwin struct {
			Srcs     []string
			TestSrcs []string
		}
		Linux struct {
			Srcs     []string
			TestSrcs []string
		}
	}

	// The root dir in which the package .a file is located.  The full .a file
	// path will be "packageRoot/PkgPath.a"
	pkgRoot string

	// The path of the .a file that is to be built.
	archiveFile string

	// The path of the test result file.
	testResultFile []string

	// The bootstrap Config
	config *Config
}

var _ goPackageProducer = (*goPackage)(nil)

func newGoPackageModuleFactory(config *Config) func() (blueprint.Module, []interface{}) {
	return func() (blueprint.Module, []interface{}) {
		module := &goPackage{
			config: config,
		}
		return module, []interface{}{&module.properties, &module.SimpleName.Properties}
	}
}

func (g *goPackage) DynamicDependencies(ctx blueprint.DynamicDependerModuleContext) []string {
	return g.properties.Deps
}

func (g *goPackage) GoPkgPath() string {
	return g.properties.PkgPath
}

func (g *goPackage) GoPkgRoot() string {
	return g.pkgRoot
}

func (g *goPackage) GoPackageTarget() string {
	return g.archiveFile
}

func (g *goPackage) GoTestTargets() []string {
	return g.testResultFile
}

func (g *goPackage) IsPluginFor(name string) bool {
	for _, plugin := range g.properties.PluginFor {
		if plugin == name {
			return true
		}
	}
	return false
}

func (g *goPackage) GenerateBuildActions(ctx blueprint.ModuleContext) {
	var (
		name       = ctx.ModuleName()
		hasPlugins = false
		pluginSrc  = ""
		genSrcs    = []string{}
	)

	if g.properties.PkgPath == "" {
		ctx.ModuleErrorf("module %s did not specify a valid pkgPath", name)
		return
	}

	g.pkgRoot = packageRoot(ctx)
	g.archiveFile = filepath.Join(g.pkgRoot,
		filepath.FromSlash(g.properties.PkgPath)+".a")

	ctx.VisitDepsDepthFirstIf(isGoPluginFor(name),
		func(module blueprint.Module) { hasPlugins = true })
	if hasPlugins {
		pluginSrc = filepath.Join(moduleGenSrcDir(ctx), "plugin.go")
		genSrcs = append(genSrcs, pluginSrc)
	}

	if hasPlugins && !buildGoPluginLoader(ctx, g.properties.PkgPath, pluginSrc) {
		return
	}

	var srcs, testSrcs []string
	if runtime.GOOS == "darwin" {
		srcs = append(g.properties.Srcs, g.properties.Darwin.Srcs...)
		testSrcs = append(g.properties.TestSrcs, g.properties.Darwin.TestSrcs...)
	} else if runtime.GOOS == "linux" {
		srcs = append(g.properties.Srcs, g.properties.Linux.Srcs...)
		testSrcs = append(g.properties.TestSrcs, g.properties.Linux.TestSrcs...)
	}

	if g.config.runGoTests {
		testArchiveFile := filepath.Join(testRoot(ctx),
			filepath.FromSlash(g.properties.PkgPath)+".a")
		g.testResultFile = buildGoTest(ctx, testRoot(ctx), testArchiveFile,
			g.properties.PkgPath, srcs, genSrcs,
			testSrcs)
	}

	buildGoPackage(ctx, g.pkgRoot, g.properties.PkgPath, g.archiveFile,
		srcs, genSrcs)
}

// A goBinary is a module for building executable binaries from Go sources.
type goBinary struct {
	blueprint.SimpleName
	properties struct {
		Deps           []string
		Srcs           []string
		TestSrcs       []string
		PrimaryBuilder bool
		Default        bool

		Darwin struct {
			Srcs     []string
			TestSrcs []string
		}
		Linux struct {
			Srcs     []string
			TestSrcs []string
		}

		Tool_dir bool `blueprint:mutated`
	}

	installPath string

	// The bootstrap Config
	config *Config
}

var _ GoBinaryTool = (*goBinary)(nil)

func newGoBinaryModuleFactory(config *Config, tooldir bool) func() (blueprint.Module, []interface{}) {
	return func() (blueprint.Module, []interface{}) {
		module := &goBinary{
			config: config,
		}
		module.properties.Tool_dir = tooldir
		return module, []interface{}{&module.properties, &module.SimpleName.Properties}
	}
}

func (g *goBinary) DynamicDependencies(ctx blueprint.DynamicDependerModuleContext) []string {
	return g.properties.Deps
}

func (g *goBinary) isGoBinary() {}
func (g *goBinary) InstallPath() string {
	return g.installPath
}

func (g *goBinary) GenerateBuildActions(ctx blueprint.ModuleContext) {
	var (
		name            = ctx.ModuleName()
		objDir          = moduleObjDir(ctx)
		archiveFile     = filepath.Join(objDir, name+".a")
		testArchiveFile = filepath.Join(testRoot(ctx), name+".a")
		aoutFile        = filepath.Join(objDir, "a.out")
		hasPlugins      = false
		pluginSrc       = ""
		genSrcs         = []string{}
	)

	if g.properties.Tool_dir {
		g.installPath = filepath.Join(toolDir(ctx.Config()), name)
	} else {
		g.installPath = filepath.Join(binDir(), name)
	}

	ctx.VisitDepsDepthFirstIf(isGoPluginFor(name),
		func(module blueprint.Module) { hasPlugins = true })
	if hasPlugins {
		pluginSrc = filepath.Join(moduleGenSrcDir(ctx), "plugin.go")
		genSrcs = append(genSrcs, pluginSrc)
	}

	var deps []string

	if hasPlugins && !buildGoPluginLoader(ctx, "main", pluginSrc) {
		return
	}

	var srcs, testSrcs []string
	if runtime.GOOS == "darwin" {
		srcs = append(g.properties.Srcs, g.properties.Darwin.Srcs...)
		testSrcs = append(g.properties.TestSrcs, g.properties.Darwin.TestSrcs...)
	} else if runtime.GOOS == "linux" {
		srcs = append(g.properties.Srcs, g.properties.Linux.Srcs...)
		testSrcs = append(g.properties.TestSrcs, g.properties.Linux.TestSrcs...)
	}

	if g.config.runGoTests {
		deps = buildGoTest(ctx, testRoot(ctx), testArchiveFile,
			name, srcs, genSrcs, testSrcs)
	}

	buildGoPackage(ctx, objDir, name, archiveFile, srcs, genSrcs)

	var libDirFlags []string
	ctx.VisitDepsDepthFirstIf(isGoPackageProducer,
		func(module blueprint.Module) {
			dep := module.(goPackageProducer)
			libDir := dep.GoPkgRoot()
			libDirFlags = append(libDirFlags, "-L "+libDir)
			deps = append(deps, dep.GoTestTargets()...)
		})

	linkArgs := map[string]string{}
	if len(libDirFlags) > 0 {
		linkArgs["libDirFlags"] = strings.Join(libDirFlags, " ")
	}

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:     link,
		Outputs:  []string{aoutFile},
		Inputs:   []string{archiveFile},
		Args:     linkArgs,
		Optional: true,
	})

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      cp,
		Outputs:   []string{g.installPath},
		Inputs:    []string{aoutFile},
		OrderOnly: deps,
		Optional:  !g.properties.Default,
	})
}

func buildGoPluginLoader(ctx blueprint.ModuleContext, pkgPath, pluginSrc string) bool {
	ret := true
	name := ctx.ModuleName()

	var pluginPaths []string
	ctx.VisitDepsDepthFirstIf(isGoPluginFor(name),
		func(module blueprint.Module) {
			plugin := module.(goPluginProvider)
			pluginPaths = append(pluginPaths, plugin.GoPkgPath())
		})

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    pluginGenSrc,
		Outputs: []string{pluginSrc},
		Args: map[string]string{
			"pkg":     pkgPath,
			"plugins": strings.Join(pluginPaths, " "),
		},
		Optional: true,
	})

	return ret
}

func buildGoPackage(ctx blueprint.ModuleContext, pkgRoot string,
	pkgPath string, archiveFile string, srcs []string, genSrcs []string) {

	srcDir := moduleSrcDir(ctx)
	srcFiles := pathtools.PrefixPaths(srcs, srcDir)
	srcFiles = append(srcFiles, genSrcs...)

	var incFlags []string
	var deps []string
	ctx.VisitDepsDepthFirstIf(isGoPackageProducer,
		func(module blueprint.Module) {
			dep := module.(goPackageProducer)
			incDir := dep.GoPkgRoot()
			target := dep.GoPackageTarget()
			incFlags = append(incFlags, "-I "+incDir)
			deps = append(deps, target)
		})

	compileArgs := map[string]string{
		"pkgPath": pkgPath,
	}

	if len(incFlags) > 0 {
		compileArgs["incFlags"] = strings.Join(incFlags, " ")
	}

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      compile,
		Outputs:   []string{archiveFile},
		Inputs:    srcFiles,
		Implicits: deps,
		Args:      compileArgs,
		Optional:  true,
	})
}

func buildGoTest(ctx blueprint.ModuleContext, testRoot, testPkgArchive,
	pkgPath string, srcs, genSrcs, testSrcs []string) []string {

	if len(testSrcs) == 0 {
		return nil
	}

	srcDir := moduleSrcDir(ctx)
	testFiles := pathtools.PrefixPaths(testSrcs, srcDir)

	mainFile := filepath.Join(testRoot, "test.go")
	testArchive := filepath.Join(testRoot, "test.a")
	testFile := filepath.Join(testRoot, "test")
	testPassed := filepath.Join(testRoot, "test.passed")

	buildGoPackage(ctx, testRoot, pkgPath, testPkgArchive,
		append(srcs, testSrcs...), genSrcs)

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    goTestMain,
		Outputs: []string{mainFile},
		Inputs:  testFiles,
		Args: map[string]string{
			"pkg": pkgPath,
		},
		Optional: true,
	})

	libDirFlags := []string{"-L " + testRoot}
	testDeps := []string{}
	ctx.VisitDepsDepthFirstIf(isGoPackageProducer,
		func(module blueprint.Module) {
			dep := module.(goPackageProducer)
			libDir := dep.GoPkgRoot()
			libDirFlags = append(libDirFlags, "-L "+libDir)
			testDeps = append(testDeps, dep.GoTestTargets()...)
		})

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      compile,
		Outputs:   []string{testArchive},
		Inputs:    []string{mainFile},
		Implicits: []string{testPkgArchive},
		Args: map[string]string{
			"pkgPath":  "main",
			"incFlags": "-I " + testRoot,
		},
		Optional: true,
	})

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    link,
		Outputs: []string{testFile},
		Inputs:  []string{testArchive},
		Args: map[string]string{
			"libDirFlags": strings.Join(libDirFlags, " "),
		},
		Optional: true,
	})

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      test,
		Outputs:   []string{testPassed},
		Inputs:    []string{testFile},
		OrderOnly: testDeps,
		Args: map[string]string{
			"pkg":       pkgPath,
			"pkgSrcDir": filepath.Dir(testFiles[0]),
		},
		Optional: true,
	})

	return []string{testPassed}
}

type singleton struct {
	// The bootstrap Config
	config *Config
}

func newSingletonFactory(config *Config) func() blueprint.Singleton {
	return func() blueprint.Singleton {
		return &singleton{
			config: config,
		}
	}
}

func (s *singleton) GenerateBuildActions(ctx blueprint.SingletonContext) {
	// Find the module that's marked as the "primary builder", which means it's
	// creating the binary that we'll use to generate the non-bootstrap
	// build.ninja file.
	var primaryBuilders []*goBinary
	// blueprintTools contains blueprint go binaries that will be built in StageMain
	var blueprintTools []string
	ctx.VisitAllModulesIf(isBootstrapBinaryModule,
		func(module blueprint.Module) {
			binaryModule := module.(*goBinary)

			if binaryModule.properties.Tool_dir {
				blueprintTools = append(blueprintTools, binaryModule.InstallPath())
			}
			if binaryModule.properties.PrimaryBuilder {
				primaryBuilders = append(primaryBuilders, binaryModule)
			}
		})

	var extraTestFlags string
	if s.config.runGoTests {
		extraTestFlags = "-t"
	}

	var primaryBuilderName, primaryBuilderExtraFlags string
	switch len(primaryBuilders) {
	case 0:
		// If there's no primary builder module then that means we'll use minibp
		// as the primary builder.  We can trigger its primary builder mode with
		// the -p flag.
		primaryBuilderName = "minibp"
		primaryBuilderExtraFlags = "-p " + extraTestFlags

	case 1:
		primaryBuilderName = ctx.ModuleName(primaryBuilders[0])
		primaryBuilderExtraFlags = extraTestFlags

	default:
		ctx.Errorf("multiple primary builder modules present:")
		for _, primaryBuilder := range primaryBuilders {
			ctx.ModuleErrorf(primaryBuilder, "<-- module %s",
				ctx.ModuleName(primaryBuilder))
		}
		return
	}

	primaryBuilderFile := filepath.Join("$BinDir", primaryBuilderName)

	// Get the filename of the top-level Blueprints file to pass to minibp.
	topLevelBlueprints := filepath.Join("$srcDir",
		filepath.Base(s.config.topLevelBlueprintsFile))

	mainNinjaFile := filepath.Join("$buildDir", "build.ninja")
	primaryBuilderNinjaFile := filepath.Join(bootstrapDir, "build.ninja")
	docsFile := filepath.Join(docsDir, primaryBuilderName+".html")

	ctx.SetNinjaBuildDir(pctx, "${ninjaBuildDir}")

	// Build the main build.ninja
	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    generateBuildNinja,
		Outputs: []string{mainNinjaFile},
		Inputs:  []string{topLevelBlueprints},
		Args: map[string]string{
			"builder": primaryBuilderFile,
			"extra":   primaryBuilderExtraFlags,
		},
	})

	// Generate build system docs for the primary builder.  Generating docs reads the source
	// files used to build the primary builder, but that dependency will be picked up through
	// the dependency on the primary builder itself.  There are no dependencies on the
	// Blueprints files, as any relevant changes to the Blueprints files would have caused
	// a rebuild of the primary builder.
	bigbpDocs := ctx.Rule(pctx, "bigbpDocs",
		blueprint.RuleParams{
			Command: fmt.Sprintf("%s %s -b $buildDir --docs $out %s", primaryBuilderFile,
				primaryBuilderExtraFlags, topLevelBlueprints),
			CommandDeps: []string{primaryBuilderFile},
			Description: fmt.Sprintf("%s docs $out", primaryBuilderName),
		})

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    bigbpDocs,
		Outputs: []string{docsFile},
	})

	// Add a way to rebuild the primary build.ninja so that globs works
	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    generateBuildNinja,
		Outputs: []string{primaryBuilderNinjaFile},
		Inputs:  []string{topLevelBlueprints},
		Args: map[string]string{
			"builder": minibpFile,
			"extra":   extraTestFlags,
		},
	})

	if s.config.stage == StageMain {
		if primaryBuilderName == "minibp" {
			// This is a standalone Blueprint build, so we copy the minibp
			// binary to the "bin" directory to make it easier to find.
			finalMinibp := filepath.Join("$buildDir", "bin", primaryBuilderName)
			ctx.Build(pctx, blueprint.BuildParams{
				Rule:    cp,
				Inputs:  []string{primaryBuilderFile},
				Outputs: []string{finalMinibp},
			})
		}

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    blueprint.Phony,
			Outputs: []string{"blueprint_tools"},
			Inputs:  blueprintTools,
		})
	}
}

// packageRoot returns the module-specific package root directory path.  This
// directory is where the final package .a files are output and where dependant
// modules search for this package via -I arguments.
func packageRoot(ctx blueprint.ModuleContext) string {
	return filepath.Join(bootstrapDir, ctx.ModuleName(), "pkg")
}

// testRoot returns the module-specific package root directory path used for
// building tests. The .a files generated here will include everything from
// packageRoot, plus the test-only code.
func testRoot(ctx blueprint.ModuleContext) string {
	return filepath.Join(bootstrapDir, ctx.ModuleName(), "test")
}

// moduleSrcDir returns the path of the directory that all source file paths are
// specified relative to.
func moduleSrcDir(ctx blueprint.ModuleContext) string {
	return filepath.Join("$srcDir", ctx.ModuleDir())
}

// moduleObjDir returns the module-specific object directory path.
func moduleObjDir(ctx blueprint.ModuleContext) string {
	return filepath.Join(bootstrapDir, ctx.ModuleName(), "obj")
}

// moduleGenSrcDir returns the module-specific generated sources path.
func moduleGenSrcDir(ctx blueprint.ModuleContext) string {
	return filepath.Join(bootstrapDir, ctx.ModuleName(), "gen")
}
