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

	compile = pctx.StaticRule("compile",
		blueprint.RuleParams{
			Command: "GOROOT='$goRoot' $compileCmd -o $out -p $pkgPath -complete " +
				"$incFlags -pack $in",
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
			Command:     "$builder $extra -b $buildDir -d $out.d -o $out $in",
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

	binDir     = pctx.StaticVariable("BinDir", filepath.Join(bootstrapDir, "bin"))
	minibpFile = filepath.Join("$BinDir", "minibp")

	docsDir = filepath.Join(bootstrapDir, "docs")
	toolDir = pctx.VariableFunc("ToolDir", func(config interface{}) (string, error) {
		if c, ok := config.(ConfigBlueprintToolLocation); ok {
			return c.BlueprintToolLocation(), nil
		}
		return filepath.Join("$buildDir", "bin"), nil
	})

	bootstrapDir     = filepath.Join("$buildDir", bootstrapSubDir)
	miniBootstrapDir = filepath.Join("$buildDir", miniBootstrapSubDir)
)

type bootstrapGoCore interface {
	BuildStage() Stage
	SetBuildStage(Stage)
}

func propagateStageBootstrap(mctx blueprint.TopDownMutatorContext) {
	if mod, ok := mctx.Module().(bootstrapGoCore); ok {
		stage := mod.BuildStage()

		mctx.VisitDirectDeps(func(mod blueprint.Module) {
			if m, ok := mod.(bootstrapGoCore); ok && m.BuildStage() > stage {
				m.SetBuildStage(stage)
			}
		})
	}
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

		// The stage in which this module should be built
		BuildStage Stage `blueprint:"mutated"`
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
		module.properties.BuildStage = StageMain
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

func (g *goPackage) BuildStage() Stage {
	return g.properties.BuildStage
}

func (g *goPackage) SetBuildStage(buildStage Stage) {
	g.properties.BuildStage = buildStage
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

	// We only actually want to build the builder modules if we're running as
	// minibp (i.e. we're generating a bootstrap Ninja file).  This is to break
	// the circular dependence that occurs when the builder requires a new Ninja
	// file to be built, but building a new ninja file requires the builder to
	// be built.
	if g.config.stage == g.BuildStage() {
		if hasPlugins && !buildGoPluginLoader(ctx, g.properties.PkgPath, pluginSrc, g.config.stage) {
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
}

// A goBinary is a module for building executable binaries from Go sources.
type goBinary struct {
	blueprint.SimpleName
	properties struct {
		Deps           []string
		Srcs           []string
		TestSrcs       []string
		PrimaryBuilder bool

		Darwin struct {
			Srcs     []string
			TestSrcs []string
		}
		Linux struct {
			Srcs     []string
			TestSrcs []string
		}

		// The stage in which this module should be built
		BuildStage Stage `blueprint:"mutated"`
	}

	// The bootstrap Config
	config *Config
}

func newGoBinaryModuleFactory(config *Config, buildStage Stage) func() (blueprint.Module, []interface{}) {
	return func() (blueprint.Module, []interface{}) {
		module := &goBinary{
			config: config,
		}
		module.properties.BuildStage = buildStage
		return module, []interface{}{&module.properties, &module.SimpleName.Properties}
	}
}

func (g *goBinary) DynamicDependencies(ctx blueprint.DynamicDependerModuleContext) []string {
	return g.properties.Deps
}

func (g *goBinary) BuildStage() Stage {
	return g.properties.BuildStage
}

func (g *goBinary) SetBuildStage(buildStage Stage) {
	g.properties.BuildStage = buildStage
}

func (g *goBinary) InstallPath() string {
	if g.BuildStage() == StageMain {
		return "$ToolDir"
	}
	return "$BinDir"
}

func (g *goBinary) GenerateBuildActions(ctx blueprint.ModuleContext) {
	var (
		name            = ctx.ModuleName()
		objDir          = moduleObjDir(ctx)
		archiveFile     = filepath.Join(objDir, name+".a")
		testArchiveFile = filepath.Join(testRoot(ctx), name+".a")
		aoutFile        = filepath.Join(objDir, "a.out")
		binaryFile      = filepath.Join(g.InstallPath(), name)
		hasPlugins      = false
		pluginSrc       = ""
		genSrcs         = []string{}
	)

	ctx.VisitDepsDepthFirstIf(isGoPluginFor(name),
		func(module blueprint.Module) { hasPlugins = true })
	if hasPlugins {
		pluginSrc = filepath.Join(moduleGenSrcDir(ctx), "plugin.go")
		genSrcs = append(genSrcs, pluginSrc)
	}

	if g.config.stage == g.BuildStage() {
		var deps []string

		if hasPlugins && !buildGoPluginLoader(ctx, "main", pluginSrc, g.config.stage) {
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
			Rule:    link,
			Outputs: []string{aoutFile},
			Inputs:  []string{archiveFile},
			Args:    linkArgs,
		})

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      cp,
			Outputs:   []string{binaryFile},
			Inputs:    []string{aoutFile},
			OrderOnly: deps,
		})
	}
}

func buildGoPluginLoader(ctx blueprint.ModuleContext, pkgPath, pluginSrc string, stage Stage) bool {
	ret := true
	name := ctx.ModuleName()

	var pluginPaths []string
	ctx.VisitDepsDepthFirstIf(isGoPluginFor(name),
		func(module blueprint.Module) {
			plugin := module.(goPluginProvider)
			pluginPaths = append(pluginPaths, plugin.GoPkgPath())
			if stage == StageBootstrap {
				ctx.OtherModuleErrorf(module, "plugin %q may not be included in core module %q",
					ctx.OtherModuleName(module), name)
				ret = false
			}
		})

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    pluginGenSrc,
		Outputs: []string{pluginSrc},
		Args: map[string]string{
			"pkg":     pkgPath,
			"plugins": strings.Join(pluginPaths, " "),
		},
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
	})

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    link,
		Outputs: []string{testFile},
		Inputs:  []string{testArchive},
		Args: map[string]string{
			"libDirFlags": strings.Join(libDirFlags, " "),
		},
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
			binaryModuleName := ctx.ModuleName(binaryModule)
			installPath := filepath.Join(binaryModule.InstallPath(), binaryModuleName)

			if binaryModule.BuildStage() == StageMain {
				blueprintTools = append(blueprintTools, installPath)
			}
			if binaryModule.properties.PrimaryBuilder {
				primaryBuilders = append(primaryBuilders, binaryModule)
			}
		})

	var extraTestFlags string
	if s.config.runGoTests {
		extraTestFlags = " -t"
	}

	var primaryBuilderName, primaryBuilderExtraFlags string
	switch len(primaryBuilders) {
	case 0:
		// If there's no primary builder module then that means we'll use minibp
		// as the primary builder.  We can trigger its primary builder mode with
		// the -p flag.
		primaryBuilderName = "minibp"
		primaryBuilderExtraFlags = "-p" + extraTestFlags

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
	bootstrapNinjaFileTemplate := filepath.Join(miniBootstrapDir, "build.ninja.in")
	bootstrapNinjaFile := filepath.Join(miniBootstrapDir, "build.ninja")
	docsFile := filepath.Join(docsDir, primaryBuilderName+".html")

	switch s.config.stage {
	case StageBootstrap:
		// We're generating a bootstrapper Ninja file, so we need to set things
		// up to rebuild the build.ninja file using the primary builder.

		// BuildDir must be different between the three stages, otherwise the
		// cleanup process will remove files from the other builds.
		ctx.SetNinjaBuildDir(pctx, miniBootstrapDir)

		// Generate the Ninja file to build the primary builder.
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    generateBuildNinja,
			Outputs: []string{primaryBuilderNinjaFile},
			Inputs:  []string{topLevelBlueprints},
			Args: map[string]string{
				"builder": minibpFile,
				"extra":   "--build-primary" + extraTestFlags,
			},
		})

		// Rebuild the bootstrap Ninja file using the minibp that we just built.
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    generateBuildNinja,
			Outputs: []string{bootstrapNinjaFileTemplate},
			Inputs:  []string{topLevelBlueprints},
			Args: map[string]string{
				"builder": minibpFile,
				"extra":   extraTestFlags,
			},
		})

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    bootstrap,
			Outputs: []string{bootstrapNinjaFile},
			Inputs:  []string{bootstrapNinjaFileTemplate},
		})

	case StagePrimary:
		// We're generating a bootstrapper Ninja file, so we need to set things
		// up to rebuild the build.ninja file using the primary builder.

		// BuildDir must be different between the three stages, otherwise the
		// cleanup process will remove files from the other builds.
		ctx.SetNinjaBuildDir(pctx, bootstrapDir)

		// Add a way to rebuild the primary build.ninja so that globs works
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    generateBuildNinja,
			Outputs: []string{primaryBuilderNinjaFile},
			Inputs:  []string{topLevelBlueprints},
			Args: map[string]string{
				"builder":   minibpFile,
				"extra":     "--build-primary" + extraTestFlags,
				"generator": "true",
			},
		})

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

	case StageMain:
		ctx.SetNinjaBuildDir(pctx, "${buildDir}")

		// Add a way to rebuild the main build.ninja in case it creates rules that
		// it will depend on itself. (In Android, globs with soong_glob)
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    generateBuildNinja,
			Outputs: []string{mainNinjaFile},
			Inputs:  []string{topLevelBlueprints},
			Args: map[string]string{
				"builder":   primaryBuilderFile,
				"extra":     primaryBuilderExtraFlags,
				"generator": "true",
			},
		})

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
