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
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
)

const mainSubDir = ".primary"
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
				numCpu := runtime.NumCPU()
				// This will cause us to recompile all go programs if the
				// number of cpus changes. We don't get a lot of benefit from
				// higher values, so cap this to make it cheaper to move trees
				// between machines.
				if numCpu > 8 {
					numCpu = 8
				}
				return fmt.Sprintf("-c %d", numCpu)
			}
		}
		return ""
	}())

	compile = pctx.StaticRule("compile",
		blueprint.RuleParams{
			Command: "GOROOT='$goRoot' $compileCmd $parallelCompile -o $out.tmp " +
				"$debugFlags -p $pkgPath -complete $incFlags -pack $in && " +
				"if cmp --quiet $out.tmp $out; then rm $out.tmp; else mv -f $out.tmp $out; fi",
			CommandDeps: []string{"$compileCmd"},
			Description: "compile $out",
			Restat:      true,
		},
		"pkgPath", "incFlags")

	link = pctx.StaticRule("link",
		blueprint.RuleParams{
			Command: "GOROOT='$goRoot' $linkCmd -o $out.tmp $libDirFlags $in && " +
				"if cmp --quiet $out.tmp $out; then rm $out.tmp; else mv -f $out.tmp $out; fi",
			CommandDeps: []string{"$linkCmd"},
			Description: "link $out",
			Restat:      true,
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
			// TODO: it's kinda ugly that some parameters are computed from
			// environment variables and some from Ninja parameters, but it's probably
			// better to not to touch that while Blueprint and Soong are separate
			// NOTE: The spaces at EOL are important because otherwise Ninja would
			// omit all spaces between the different options.
			Command: `cd "$$(dirname "$builder")" && ` +
				`BUILDER="$$PWD/$$(basename "$builder")" && ` +
				`cd / && ` +
				`env -i "$$BUILDER" ` +
				`    $extra ` +
				`    --top "$$TOP" ` +
				`    --out "$$SOONG_OUTDIR" ` +
				`    --delve_listen "$$SOONG_DELVE" ` +
				`    --delve_path "$$SOONG_DELVE_PATH" ` +
				`    -b "$buildDir" ` +
				`    -n "$ninjaBuildDir" ` +
				`    -d "$out.d" ` +
				`    -globFile "$globFile" ` +
				`    -o "$out" ` +
				`    "$in" `,
			CommandDeps: []string{"$builder"},
			Description: "$builder $out",
			Deps:        blueprint.DepsGCC,
			Depfile:     "$out.d",
			Restat:      true,
		},
		"builder", "extra", "generator", "globFile")

	// Work around a Ninja issue.  See https://github.com/martine/ninja/pull/634
	phony = pctx.StaticRule("phony",
		blueprint.RuleParams{
			Command:     "# phony $out",
			Description: "phony $out",
			Generator:   true,
		},
		"depfile")

	_ = pctx.VariableFunc("BinDir", func(config interface{}) (string, error) {
		return bootstrapBinDir(config), nil
	})

	_ = pctx.VariableFunc("ToolDir", func(config interface{}) (string, error) {
		return toolDir(config), nil
	})

	docsDir = filepath.Join(mainDir, "docs")

	mainDir          = filepath.Join("$buildDir", mainSubDir)
	bootstrapDir     = filepath.Join("$buildDir", bootstrapSubDir)
	miniBootstrapDir = filepath.Join("$buildDir", miniBootstrapSubDir)

	minibpFile = filepath.Join(miniBootstrapDir, "minibp")
)

type GoBinaryTool interface {
	InstallPath() string

	// So that other packages can't implement this interface
	isGoBinary()
}

func bootstrapBinDir(config interface{}) string {
	return filepath.Join(config.(BootstrapConfig).BuildDir(), bootstrapSubDir, "bin")
}

func toolDir(config interface{}) string {
	if c, ok := config.(ConfigBlueprintToolLocation); ok {
		return filepath.Join(c.BlueprintToolLocation())
	}
	return filepath.Join(config.(BootstrapConfig).BuildDir(), "bin")
}

func pluginDeps(ctx blueprint.BottomUpMutatorContext) {
	if pkg, ok := ctx.Module().(*goPackage); ok {
		if ctx.PrimaryModule() == ctx.Module() {
			for _, plugin := range pkg.properties.PluginFor {
				ctx.AddReverseDependency(ctx.Module(), nil, plugin)
			}
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

func IsBootstrapModule(module blueprint.Module) bool {
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
	if ctx.Module() != ctx.PrimaryModule() {
		return nil
	}
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
	// Allow the primary builder to create multiple variants.  Any variants after the first
	// will copy outputs from the first.
	if ctx.Module() != ctx.PrimaryModule() {
		primary := ctx.PrimaryModule().(*goPackage)
		g.pkgRoot = primary.pkgRoot
		g.archiveFile = primary.archiveFile
		g.testResultFile = primary.testResultFile
		return
	}

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

	g.pkgRoot = packageRoot(ctx, g.config)
	g.archiveFile = filepath.Join(g.pkgRoot,
		filepath.FromSlash(g.properties.PkgPath)+".a")

	ctx.VisitDepsDepthFirstIf(isGoPluginFor(name),
		func(module blueprint.Module) { hasPlugins = true })
	if hasPlugins {
		pluginSrc = filepath.Join(moduleGenSrcDir(ctx, g.config), "plugin.go")
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
		testArchiveFile := filepath.Join(testRoot(ctx, g.config),
			filepath.FromSlash(g.properties.PkgPath)+".a")
		g.testResultFile = buildGoTest(ctx, testRoot(ctx, g.config), testArchiveFile,
			g.properties.PkgPath, srcs, genSrcs,
			testSrcs, g.config.useValidations)
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

		Tool_dir bool `blueprint:"mutated"`
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
	if ctx.Module() != ctx.PrimaryModule() {
		return nil
	}
	return g.properties.Deps
}

func (g *goBinary) isGoBinary() {}
func (g *goBinary) InstallPath() string {
	return g.installPath
}

func (g *goBinary) GenerateBuildActions(ctx blueprint.ModuleContext) {
	// Allow the primary builder to create multiple variants.  Any variants after the first
	// will copy outputs from the first.
	if ctx.Module() != ctx.PrimaryModule() {
		primary := ctx.PrimaryModule().(*goBinary)
		g.installPath = primary.installPath
		return
	}

	var (
		name            = ctx.ModuleName()
		objDir          = moduleObjDir(ctx, g.config)
		archiveFile     = filepath.Join(objDir, name+".a")
		testArchiveFile = filepath.Join(testRoot(ctx, g.config), name+".a")
		aoutFile        = filepath.Join(objDir, "a.out")
		hasPlugins      = false
		pluginSrc       = ""
		genSrcs         = []string{}
	)

	if g.properties.Tool_dir {
		g.installPath = filepath.Join(toolDir(ctx.Config()), name)
	} else {
		g.installPath = filepath.Join(stageDir(g.config), "bin", name)
	}

	ctx.VisitDepsDepthFirstIf(isGoPluginFor(name),
		func(module blueprint.Module) { hasPlugins = true })
	if hasPlugins {
		pluginSrc = filepath.Join(moduleGenSrcDir(ctx, g.config), "plugin.go")
		genSrcs = append(genSrcs, pluginSrc)
	}

	var testDeps []string

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
		testDeps = buildGoTest(ctx, testRoot(ctx, g.config), testArchiveFile,
			name, srcs, genSrcs, testSrcs, g.config.useValidations)
	}

	buildGoPackage(ctx, objDir, "main", archiveFile, srcs, genSrcs)

	var linkDeps []string
	var libDirFlags []string
	ctx.VisitDepsDepthFirstIf(isGoPackageProducer,
		func(module blueprint.Module) {
			dep := module.(goPackageProducer)
			linkDeps = append(linkDeps, dep.GoPackageTarget())
			libDir := dep.GoPkgRoot()
			libDirFlags = append(libDirFlags, "-L "+libDir)
			testDeps = append(testDeps, dep.GoTestTargets()...)
		})

	linkArgs := map[string]string{}
	if len(libDirFlags) > 0 {
		linkArgs["libDirFlags"] = strings.Join(libDirFlags, " ")
	}

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      link,
		Outputs:   []string{aoutFile},
		Inputs:    []string{archiveFile},
		Implicits: linkDeps,
		Args:      linkArgs,
		Optional:  true,
	})

	var orderOnlyDeps, validationDeps []string
	if g.config.useValidations {
		validationDeps = testDeps
	} else {
		orderOnlyDeps = testDeps
	}

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:        cp,
		Outputs:     []string{g.installPath},
		Inputs:      []string{aoutFile},
		OrderOnly:   orderOnlyDeps,
		Validations: validationDeps,
		Optional:    !g.properties.Default,
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
	pkgPath string, srcs, genSrcs, testSrcs []string, useValidations bool) []string {

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

	linkDeps := []string{testPkgArchive}
	libDirFlags := []string{"-L " + testRoot}
	testDeps := []string{}
	ctx.VisitDepsDepthFirstIf(isGoPackageProducer,
		func(module blueprint.Module) {
			dep := module.(goPackageProducer)
			linkDeps = append(linkDeps, dep.GoPackageTarget())
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
		Rule:      link,
		Outputs:   []string{testFile},
		Inputs:    []string{testArchive},
		Implicits: linkDeps,
		Args: map[string]string{
			"libDirFlags": strings.Join(libDirFlags, " "),
		},
		Optional: true,
	})

	var orderOnlyDeps, validationDeps []string
	if useValidations {
		validationDeps = testDeps
	} else {
		orderOnlyDeps = testDeps
	}

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:        test,
		Outputs:     []string{testPassed},
		Inputs:      []string{testFile},
		OrderOnly:   orderOnlyDeps,
		Validations: validationDeps,
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
			if ctx.PrimaryModule(module) == module {
				binaryModule := module.(*goBinary)

				if binaryModule.properties.Tool_dir {
					blueprintTools = append(blueprintTools, binaryModule.InstallPath())
				}
				if binaryModule.properties.PrimaryBuilder {
					primaryBuilders = append(primaryBuilders, binaryModule)
				}
			}
		})

	var extraSharedFlagArray []string
	if s.config.runGoTests {
		extraSharedFlagArray = append(extraSharedFlagArray, "-t")
	}
	if s.config.moduleListFile != "" {
		extraSharedFlagArray = append(extraSharedFlagArray, "-l", s.config.moduleListFile)
	}
	if s.config.emptyNinjaFile {
		extraSharedFlagArray = append(extraSharedFlagArray, "--empty-ninja-file")
	}
	extraSharedFlagString := strings.Join(extraSharedFlagArray, " ")

	var primaryBuilderName, primaryBuilderExtraFlags string
	switch len(primaryBuilders) {
	case 0:
		// If there's no primary builder module then that means we'll use minibp
		// as the primary builder.  We can trigger its primary builder mode with
		// the -p flag.
		primaryBuilderName = "minibp"
		primaryBuilderExtraFlags = "-p " + extraSharedFlagString

	case 1:
		primaryBuilderName = ctx.ModuleName(primaryBuilders[0])
		primaryBuilderExtraFlags = extraSharedFlagString

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
	ctx.SetNinjaBuildDir(pctx, "${ninjaBuildDir}")

	buildDir := ctx.Config().(BootstrapConfig).BuildDir()

	if s.config.stage == StagePrimary {
		mainNinjaFile := filepath.Join("$buildDir", "build.ninja")
		primaryBuilderNinjaGlobFile := absolutePath(filepath.Join(buildDir, bootstrapSubDir, "build-globs.ninja"))

		if _, err := os.Stat(primaryBuilderNinjaGlobFile); os.IsNotExist(err) {
			err = ioutil.WriteFile(primaryBuilderNinjaGlobFile, nil, 0666)
			if err != nil {
				ctx.Errorf("Failed to create empty ninja file: %s", err)
			}
		}

		ctx.AddSubninja(primaryBuilderNinjaGlobFile)

		// Build the main build.ninja
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    generateBuildNinja,
			Outputs: []string{mainNinjaFile},
			Inputs:  []string{topLevelBlueprints},
			Args: map[string]string{
				"builder":  primaryBuilderFile,
				"extra":    primaryBuilderExtraFlags,
				"globFile": primaryBuilderNinjaGlobFile,
			},
		})
	}

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

		// Generate build system docs for the primary builder.  Generating docs reads the source
		// files used to build the primary builder, but that dependency will be picked up through
		// the dependency on the primary builder itself.  There are no dependencies on the
		// Blueprints files, as any relevant changes to the Blueprints files would have caused
		// a rebuild of the primary builder.
		docsFile := filepath.Join(docsDir, primaryBuilderName+".html")
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

		// Add a phony target for building the documentation
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    blueprint.Phony,
			Outputs: []string{"blueprint_docs"},
			Inputs:  []string{docsFile},
		})

		// Add a phony target for building various tools that are part of blueprint
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    blueprint.Phony,
			Outputs: []string{"blueprint_tools"},
			Inputs:  blueprintTools,
		})
	}
}

func stageDir(config *Config) string {
	if config.stage == StageMain {
		return mainDir
	} else {
		return bootstrapDir
	}
}

// packageRoot returns the module-specific package root directory path.  This
// directory is where the final package .a files are output and where dependant
// modules search for this package via -I arguments.
func packageRoot(ctx blueprint.ModuleContext, config *Config) string {
	return filepath.Join(stageDir(config), ctx.ModuleName(), "pkg")
}

// testRoot returns the module-specific package root directory path used for
// building tests. The .a files generated here will include everything from
// packageRoot, plus the test-only code.
func testRoot(ctx blueprint.ModuleContext, config *Config) string {
	return filepath.Join(stageDir(config), ctx.ModuleName(), "test")
}

// moduleSrcDir returns the path of the directory that all source file paths are
// specified relative to.
func moduleSrcDir(ctx blueprint.ModuleContext) string {
	return filepath.Join("$srcDir", ctx.ModuleDir())
}

// moduleObjDir returns the module-specific object directory path.
func moduleObjDir(ctx blueprint.ModuleContext, config *Config) string {
	return filepath.Join(stageDir(config), ctx.ModuleName(), "obj")
}

// moduleGenSrcDir returns the module-specific generated sources path.
func moduleGenSrcDir(ctx blueprint.ModuleContext, config *Config) string {
	return filepath.Join(stageDir(config), ctx.ModuleName(), "gen")
}
