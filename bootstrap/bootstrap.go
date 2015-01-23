package bootstrap

import (
	"blueprint"
	"blueprint/pathtools"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const bootstrapDir = ".bootstrap"

var (
	pctx = blueprint.NewPackageContext("blueprint/bootstrap")

	gcCmd   = pctx.StaticVariable("gcCmd", "$goToolDir/${goChar}g")
	linkCmd = pctx.StaticVariable("linkCmd", "$goToolDir/${goChar}l")

	// Ninja only reinvokes itself once when it regenerates a .ninja file. For
	// the re-bootstrap process we need that to happen more than once, so we
	// invoke an additional Ninja process from the rebootstrap rule.
	// Unfortunately this seems to cause "warning: bad deps log signature or
	// version; starting over" messages from Ninja. This warning can be
	// avoided by having the bootstrap and non-bootstrap build manifests have
	// a different builddir (so they use different log files).
	//
	// This workaround can be avoided entirely by making a simple change to
	// Ninja that would allow it to rebuild the manifest multiple times rather
	// than just once.  If the Ninja being used is capable of this, then the
	// workaround we're doing can be disabled by setting the
	// BLUEPRINT_NINJA_HAS_MULTIPASS environment variable to a true value.
	runChildNinja = pctx.VariableFunc("runChildNinja",
		func(config interface{}) (string, error) {
			if ninjaHasMultipass(config) {
				return "", nil
			} else {
				return " && ninja", nil
			}
		})

	gc = pctx.StaticRule("gc",
		blueprint.RuleParams{
			Command: "GOROOT='$goRoot' $gcCmd -o $out -p $pkgPath -complete " +
				"$incFlags -pack $in",
			Description: "${goChar}g $out",
		},
		"pkgPath", "incFlags")

	link = pctx.StaticRule("link",
		blueprint.RuleParams{
			Command:     "GOROOT='$goRoot' $linkCmd -o $out $libDirFlags $in",
			Description: "${goChar}l $out",
		},
		"libDirFlags")

	cp = pctx.StaticRule("cp",
		blueprint.RuleParams{
			Command:     "cp $in $out",
			Description: "cp $out",
		},
		"generator")

	bootstrap = pctx.StaticRule("bootstrap",
		blueprint.RuleParams{
			Command:     "$bootstrapCmd -i $in",
			Description: "bootstrap $in",
			Generator:   true,
		})

	rebootstrap = pctx.StaticRule("rebootstrap",
		blueprint.RuleParams{
			Command:     "$bootstrapCmd -i $in$runChildNinja",
			Description: "re-bootstrap $in",
			Generator:   true,
		})

	// Work around a Ninja issue.  See https://github.com/martine/ninja/pull/634
	phony = pctx.StaticRule("phony",
		blueprint.RuleParams{
			Command:     "# phony $out",
			Description: "phony $out",
			Generator:   true,
		},
		"depfile")

	BinDir     = filepath.Join(bootstrapDir, "bin")
	minibpFile = filepath.Join(BinDir, "minibp")
)

type goPackageProducer interface {
	GoPkgRoot() string
	GoPackageTarget() string
}

func isGoPackageProducer(module blueprint.Module) bool {
	_, ok := module.(goPackageProducer)
	return ok
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

func generatingBootstrapper(config interface{}) bool {
	bootstrapConfig, ok := config.(Config)
	if ok {
		return bootstrapConfig.GeneratingBootstrapper()
	}
	return false
}

// ninjaHasMultipass returns true if Ninja will perform multiple passes
// that can regenerate the build manifest.
func ninjaHasMultipass(config interface{}) bool {
	envString := os.Getenv("BLUEPRINT_NINJA_HAS_MULTIPASS")
	envValue, err := strconv.ParseBool(envString)
	if err != nil {
		return false
	}
	return envValue
}

// A goPackage is a module for building Go packages.
type goPackage struct {
	properties struct {
		PkgPath string
		Srcs    []string
	}

	// The root dir in which the package .a file is located.  The full .a file
	// path will be "packageRoot/PkgPath.a"
	pkgRoot string

	// The path of the .a file that is to be built.
	archiveFile string
}

var _ goPackageProducer = (*goPackage)(nil)

func newGoPackageModule() (blueprint.Module, []interface{}) {
	module := &goPackage{}
	return module, []interface{}{&module.properties}
}

func (g *goPackage) GoPkgRoot() string {
	return g.pkgRoot
}

func (g *goPackage) GoPackageTarget() string {
	return g.archiveFile
}

func (g *goPackage) GenerateBuildActions(ctx blueprint.ModuleContext) {
	name := ctx.ModuleName()

	if g.properties.PkgPath == "" {
		ctx.ModuleErrorf("module %s did not specify a valid pkgPath", name)
		return
	}

	g.pkgRoot = packageRoot(ctx)
	g.archiveFile = filepath.Clean(filepath.Join(g.pkgRoot,
		filepath.FromSlash(g.properties.PkgPath)+".a"))

	// We only actually want to build the builder modules if we're running as
	// minibp (i.e. we're generating a bootstrap Ninja file).  This is to break
	// the circular dependence that occurs when the builder requires a new Ninja
	// file to be built, but building a new ninja file requires the builder to
	// be built.
	if generatingBootstrapper(ctx.Config()) {
		buildGoPackage(ctx, g.pkgRoot, g.properties.PkgPath, g.archiveFile,
			g.properties.Srcs)
	} else {
		phonyGoTarget(ctx, g.archiveFile, g.properties.Srcs, nil)
	}
}

// A goBinary is a module for building executable binaries from Go sources.
type goBinary struct {
	properties struct {
		Srcs           []string
		PrimaryBuilder bool
	}
}

func newGoBinaryModule() (blueprint.Module, []interface{}) {
	module := &goBinary{}
	return module, []interface{}{&module.properties}
}

func (g *goBinary) GenerateBuildActions(ctx blueprint.ModuleContext) {
	var (
		name        = ctx.ModuleName()
		objDir      = moduleObjDir(ctx)
		archiveFile = filepath.Join(objDir, name+".a")
		aoutFile    = filepath.Join(objDir, "a.out")
		binaryFile  = filepath.Join(BinDir, name)
	)

	// We only actually want to build the builder modules if we're running as
	// minibp (i.e. we're generating a bootstrap Ninja file).  This is to break
	// the circular dependence that occurs when the builder requires a new Ninja
	// file to be built, but building a new ninja file requires the builder to
	// be built.
	if generatingBootstrapper(ctx.Config()) {
		buildGoPackage(ctx, objDir, name, archiveFile, g.properties.Srcs)

		var libDirFlags []string
		ctx.VisitDepsDepthFirstIf(isGoPackageProducer,
			func(module blueprint.Module) {
				dep := module.(goPackageProducer)
				libDir := dep.GoPkgRoot()
				libDirFlags = append(libDirFlags, "-L "+libDir)
			})

		linkArgs := map[string]string{}
		if len(libDirFlags) > 0 {
			linkArgs["libDirFlags"] = strings.Join(libDirFlags, " ")
		}

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      link,
			Outputs:   []string{aoutFile},
			Inputs:    []string{archiveFile},
			Implicits: []string{"$linkCmd"},
			Args:      linkArgs,
		})

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    cp,
			Outputs: []string{binaryFile},
			Inputs:  []string{aoutFile},
		})
	} else {
		intermediates := []string{aoutFile, archiveFile}
		phonyGoTarget(ctx, binaryFile, g.properties.Srcs, intermediates)
	}
}

func buildGoPackage(ctx blueprint.ModuleContext, pkgRoot string,
	pkgPath string, archiveFile string, srcs []string) {

	srcDir := moduleSrcDir(ctx)
	srcFiles := pathtools.PrefixPaths(srcs, srcDir)

	var incFlags []string
	deps := []string{"$gcCmd"}
	ctx.VisitDepsDepthFirstIf(isGoPackageProducer,
		func(module blueprint.Module) {
			dep := module.(goPackageProducer)
			incDir := dep.GoPkgRoot()
			target := dep.GoPackageTarget()
			incFlags = append(incFlags, "-I "+incDir)
			deps = append(deps, target)
		})

	gcArgs := map[string]string{
		"pkgPath": pkgPath,
	}

	if len(incFlags) > 0 {
		gcArgs["incFlags"] = strings.Join(incFlags, " ")
	}

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      gc,
		Outputs:   []string{archiveFile},
		Inputs:    srcFiles,
		Implicits: deps,
		Args:      gcArgs,
	})
}

func phonyGoTarget(ctx blueprint.ModuleContext, target string, srcs []string,
	intermediates []string) {

	var depTargets []string
	ctx.VisitDepsDepthFirstIf(isGoPackageProducer,
		func(module blueprint.Module) {
			dep := module.(goPackageProducer)
			target := dep.GoPackageTarget()
			depTargets = append(depTargets, target)
		})

	moduleDir := ctx.ModuleDir()
	srcs = pathtools.PrefixPaths(srcs, filepath.Join("$srcDir", moduleDir))

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      phony,
		Outputs:   []string{target},
		Inputs:    srcs,
		Implicits: depTargets,
	})

	// If one of the source files gets deleted or renamed that will prevent the
	// re-bootstrapping happening because it depends on the missing source file.
	// To get around this we add a build statement using the built-in phony rule
	// for each source file, which will cause Ninja to treat it as dirty if its
	// missing.
	for _, src := range srcs {
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    blueprint.Phony,
			Outputs: []string{src},
		})
	}

	// If there is no rule to build the intermediate files of a bootstrap go package
	// the cleanup phase of the primary builder will delete the intermediate files,
	// forcing an unnecessary rebuild.  Add phony rules for all of them.
	for _, intermediate := range intermediates {
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    blueprint.Phony,
			Outputs: []string{intermediate},
		})
	}

}

type singleton struct{}

func newSingleton() blueprint.Singleton {
	return &singleton{}
}

func (s *singleton) GenerateBuildActions(ctx blueprint.SingletonContext) {
	// Find the module that's marked as the "primary builder", which means it's
	// creating the binary that we'll use to generate the non-bootstrap
	// build.ninja file.
	var primaryBuilders []*goBinary
	var allGoBinaries []string
	ctx.VisitAllModulesIf(isBootstrapBinaryModule,
		func(module blueprint.Module) {
			binaryModule := module.(*goBinary)
			binaryModuleName := ctx.ModuleName(binaryModule)
			binaryModulePath := filepath.Join(BinDir, binaryModuleName)
			allGoBinaries = append(allGoBinaries, binaryModulePath)
			if binaryModule.properties.PrimaryBuilder {
				primaryBuilders = append(primaryBuilders, binaryModule)
			}
		})

	var primaryBuilderName, primaryBuilderExtraFlags string
	switch len(primaryBuilders) {
	case 0:
		// If there's no primary builder module then that means we'll use minibp
		// as the primary builder.  We can trigger its primary builder mode with
		// the -p flag.
		primaryBuilderName = "minibp"
		primaryBuilderExtraFlags = "-p"

	case 1:
		primaryBuilderName = ctx.ModuleName(primaryBuilders[0])

	default:
		ctx.Errorf("multiple primary builder modules present:")
		for _, primaryBuilder := range primaryBuilders {
			ctx.ModuleErrorf(primaryBuilder, "<-- module %s",
				ctx.ModuleName(primaryBuilder))
		}
		return
	}

	primaryBuilderFile := filepath.Join(BinDir, primaryBuilderName)

	// Get the filename of the top-level Blueprints file to pass to minibp.
	// This comes stored in a global variable that's set by Main.
	topLevelBlueprints := filepath.Join("$srcDir",
		filepath.Base(topLevelBlueprintsFile))

	mainNinjaFile := filepath.Join(bootstrapDir, "main.ninja.in")
	mainNinjaDepFile := mainNinjaFile + ".d"
	bootstrapNinjaFile := filepath.Join(bootstrapDir, "bootstrap.ninja.in")

	if generatingBootstrapper(ctx.Config()) {
		// We're generating a bootstrapper Ninja file, so we need to set things
		// up to rebuild the build.ninja file using the primary builder.

		// Because the non-bootstrap build.ninja file manually re-invokes Ninja,
		// its builddir must be different than that of the bootstrap build.ninja
		// file.  Otherwise we occasionally get "warning: bad deps log signature
		// or version; starting over" messages from Ninja, presumably because
		// two Ninja processes try to write to the same log concurrently.
		ctx.SetBuildDir(pctx, bootstrapDir)

		// We generate the depfile here that includes the dependencies for all
		// the Blueprints files that contribute to generating the big build
		// manifest (build.ninja file).  This depfile will be used by the non-
		// bootstrap build manifest to determine whether it should trigger a re-
		// bootstrap.  Because the re-bootstrap rule's output is "build.ninja"
		// we need to force the depfile to have that as its "make target"
		// (recall that depfiles use a subset of the Makefile syntax).
		bigbp := ctx.Rule(pctx, "bigbp",
			blueprint.RuleParams{
				Command: fmt.Sprintf("%s %s -d %s -m $bootstrapManifest "+
					"-o $out $in", primaryBuilderFile,
					primaryBuilderExtraFlags, mainNinjaDepFile),
				Description: fmt.Sprintf("%s $out", primaryBuilderName),
				Depfile:     mainNinjaDepFile,
			})

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      bigbp,
			Outputs:   []string{mainNinjaFile},
			Inputs:    []string{topLevelBlueprints},
			Implicits: allGoBinaries,
		})

		// When the current build.ninja file is a bootstrapper, we always want
		// to have it replace itself with a non-bootstrapper build.ninja.  To
		// accomplish that we depend on a file that should never exist and
		// "build" it using Ninja's built-in phony rule.
		//
		// We also need to add an implicit dependency on bootstrapNinjaFile so
		// that it gets generated as part of the bootstrap process.
		notAFile := filepath.Join(bootstrapDir, "notAFile")
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    blueprint.Phony,
			Outputs: []string{notAFile},
		})

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      bootstrap,
			Outputs:   []string{"build.ninja"},
			Inputs:    []string{mainNinjaFile},
			Implicits: []string{"$bootstrapCmd", notAFile, bootstrapNinjaFile},
		})

		// Rebuild the bootstrap Ninja file using the minibp that we just built.
		// The checkFile tells minibp to compare the new bootstrap file to the
		// current one.  If the files are the same then minibp sets the new
		// file's mtime to match that of the current one.  If they're different
		// then the new file will have a newer timestamp than the current one
		// and it will trigger a reboostrap by the non-boostrap build manifest.
		minibp := ctx.Rule(pctx, "minibp",
			blueprint.RuleParams{
				Command: fmt.Sprintf("%s -c $checkFile -m $bootstrapManifest "+
					"-d $out.d -o $out $in", minibpFile),
				Description: "minibp $out",
				Generator:   true,
				Depfile:     "$out.d",
			},
			"checkFile")

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      minibp,
			Outputs:   []string{bootstrapNinjaFile},
			Inputs:    []string{topLevelBlueprints},
			Implicits: []string{minibpFile},
			Args: map[string]string{
				"checkFile": "$bootstrapManifest",
			},
		})
	} else {
		// We're generating a non-bootstrapper Ninja file, so we need to set it
		// up to depend on the bootstrapper Ninja file.  The build.ninja target
		// also has an implicit dependency on the primary builder and all other
		// bootstrap go binaries, which will have phony dependencies on all of
		// their sources.  This will cause any changes to a bootstrap binary's
		// sources to trigger a re-bootstrap operation, which will rebuild the
		// binary.
		//
		// On top of that we need to use the depfile generated by the bigbp
		// rule.  We do this by depending on that file and then setting up a
		// phony rule to generate it that uses the depfile.
		buildNinjaDeps := []string{"$bootstrapCmd", mainNinjaFile}
		buildNinjaDeps = append(buildNinjaDeps, allGoBinaries...)
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      rebootstrap,
			Outputs:   []string{"build.ninja"},
			Inputs:    []string{"$bootstrapManifest"},
			Implicits: buildNinjaDeps,
		})

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    phony,
			Outputs: []string{mainNinjaFile},
			Inputs:  []string{topLevelBlueprints},
			Args: map[string]string{
				"depfile": mainNinjaDepFile,
			},
		})

		// If the bootstrap Ninja invocation caused a new bootstrapNinjaFile to be
		// generated then that means we need to rebootstrap using it instead of
		// the current bootstrap manifest.  We enable the Ninja "generator"
		// behavior so that Ninja doesn't invoke this build just because it's
		// missing a command line log entry for the bootstrap manifest.
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    cp,
			Outputs: []string{"$bootstrapManifest"},
			Inputs:  []string{bootstrapNinjaFile},
			Args: map[string]string{
				"generator": "true",
			},
		})

		if primaryBuilderName == "minibp" {
			// This is a standalone Blueprint build, so we copy the minibp
			// binary to the "bin" directory to make it easier to find.
			finalMinibp := filepath.Join("bin", primaryBuilderName)
			ctx.Build(pctx, blueprint.BuildParams{
				Rule:    cp,
				Inputs:  []string{primaryBuilderFile},
				Outputs: []string{finalMinibp},
			})
		}
	}
}

// packageRoot returns the module-specific package root directory path.  This
// directory is where the final package .a files are output and where dependant
// modules search for this package via -I arguments.
func packageRoot(ctx blueprint.ModuleContext) string {
	return filepath.Join(bootstrapDir, ctx.ModuleName(), "pkg")
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
