package bootstrap

import (
	"blueprint"
	"blueprint/pathtools"
	"fmt"
	"path/filepath"
	"strings"
)

const bootstrapDir = ".bootstrap"

var (
	pctx = blueprint.NewPackageContext("blueprint/bootstrap")

	gcCmd   = pctx.StaticVariable("gcCmd", "$goToolDir/${GoChar}g")
	packCmd = pctx.StaticVariable("packCmd", "$goToolDir/pack")
	linkCmd = pctx.StaticVariable("linkCmd", "$goToolDir/${GoChar}l")

	gc = pctx.StaticRule("gc",
		blueprint.RuleParams{
			Command: "GOROOT='$GoRoot' $gcCmd -o $out -p $pkgPath -complete " +
				"$incFlags $in",
			Description: "${GoChar}g $out",
		},
		"pkgPath", "incFlags")

	pack = pctx.StaticRule("pack",
		blueprint.RuleParams{
			Command:     "GOROOT='$GoRoot' $packCmd grcP $prefix $out $in",
			Description: "pack $out",
		},
		"prefix")

	link = pctx.StaticRule("link",
		blueprint.RuleParams{
			Command:     "GOROOT='$GoRoot' $linkCmd -o $out $libDirFlags $in",
			Description: "${GoChar}l $out",
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
			Command:     "$Bootstrap -i $in",
			Description: "bootstrap $in",
			Generator:   true,
		})

	rebootstrap = pctx.StaticRule("rebootstrap",
		blueprint.RuleParams{
			// Ninja only re-invokes itself once when it regenerates a .ninja
			// file.  For the re-bootstrap process we need that to happen twice,
			// so we invoke ninja ourselves once from this.  Unfortunately this
			// seems to cause "warning: bad deps log signature or version;
			// starting over" messages from Ninja.  This warning can be avoided
			// by having the bootstrap and non-bootstrap build manifests have a
			// different builddir (so they use different log files).
			//
			// This workaround can be avoided entirely by making a simple change
			// to Ninja that would allow it to rebuild the manifest twice rather
			// than just once.
			Command:     "$Bootstrap -i $in && ninja",
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

	binDir     = filepath.Join(bootstrapDir, "bin")
	minibpFile = filepath.Join(binDir, "minibp")
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
		phonyGoTarget(ctx, g.archiveFile, g.properties.Srcs)
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
		objDir      = objDir(ctx)
		archiveFile = filepath.Join(objDir, name+".a")
		aoutFile    = filepath.Join(objDir, "a.out")
		binaryFile  = filepath.Join(binDir, name)
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
			Rule:    link,
			Outputs: []string{aoutFile},
			Inputs:  []string{archiveFile},
			Args:    linkArgs,
		})

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    cp,
			Outputs: []string{binaryFile},
			Inputs:  []string{aoutFile},
		})
	} else {
		phonyGoTarget(ctx, binaryFile, g.properties.Srcs)
	}
}

func buildGoPackage(ctx blueprint.ModuleContext, pkgRoot string,
	pkgPath string, archiveFile string, srcs []string) {

	srcDir := srcDir(ctx)
	srcFiles := pathtools.PrefixPaths(srcs, srcDir)

	objDir := objDir(ctx)
	objFile := filepath.Join(objDir, "_go_.$GoChar")

	var incFlags []string
	var depTargets []string
	ctx.VisitDepsDepthFirstIf(isGoPackageProducer,
		func(module blueprint.Module) {
			dep := module.(goPackageProducer)
			incDir := dep.GoPkgRoot()
			target := dep.GoPackageTarget()
			incFlags = append(incFlags, "-I "+incDir)
			depTargets = append(depTargets, target)
		})

	gcArgs := map[string]string{
		"pkgPath": pkgPath,
	}

	if len(incFlags) > 0 {
		gcArgs["incFlags"] = strings.Join(incFlags, " ")
	}

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      gc,
		Outputs:   []string{objFile},
		Inputs:    srcFiles,
		Implicits: depTargets,
		Args:      gcArgs,
	})

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    pack,
		Outputs: []string{archiveFile},
		Inputs:  []string{objFile},
		Args: map[string]string{
			"prefix": pkgRoot,
		},
	})
}

func phonyGoTarget(ctx blueprint.ModuleContext, target string, srcs []string) {
	var depTargets []string
	ctx.VisitDepsDepthFirstIf(isGoPackageProducer,
		func(module blueprint.Module) {
			dep := module.(goPackageProducer)
			target := dep.GoPackageTarget()
			depTargets = append(depTargets, target)
		})

	moduleDir := ctx.ModuleDir()
	srcs = pathtools.PrefixPaths(srcs, filepath.Join("$SrcDir", moduleDir))

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
	ctx.VisitAllModulesIf(isBootstrapBinaryModule,
		func(module blueprint.Module) {
			binaryModule := module.(*goBinary)
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

	primaryBuilderFile := filepath.Join(binDir, primaryBuilderName)

	// Get the filename of the top-level Blueprints file to pass to minibp.
	// This comes stored in a global variable that's set by Main.
	topLevelBlueprints := filepath.Join("$SrcDir",
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
				Command: fmt.Sprintf("%s %s -d %s -o $out $in",
					primaryBuilderFile, primaryBuilderExtraFlags,
					mainNinjaDepFile),
				Description: fmt.Sprintf("%s $out", primaryBuilderName),
				Depfile:     mainNinjaDepFile,
			})

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      bigbp,
			Outputs:   []string{mainNinjaFile},
			Inputs:    []string{topLevelBlueprints},
			Implicits: []string{primaryBuilderFile},
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
			Implicits: []string{"$Bootstrap", notAFile, bootstrapNinjaFile},
		})

		// Rebuild the bootstrap Ninja file using the minibp that we just built.
		// The checkFile tells minibp to compare the new bootstrap file to the
		// current one.  If the files are the same then minibp sets the new
		// file's mtime to match that of the current one.  If they're different
		// then the new file will have a newer timestamp than the current one
		// and it will trigger a reboostrap by the non-boostrap build manifest.
		minibp := ctx.Rule(pctx, "minibp",
			blueprint.RuleParams{
				Command: fmt.Sprintf("%s -c $checkFile -d $out.d -o $out $in",
					minibpFile),
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
				"checkFile": "$BootstrapManifest",
			},
		})
	} else {
		// We're generating a non-bootstrapper Ninja file, so we need to set it
		// up to depend on the bootstrapper Ninja file.  The build.ninja target
		// also has an implicit dependency on the primary builder, which will
		// have a phony dependency on all its sources.  This will cause any
		// changes to the primary builder's sources to trigger a re-bootstrap
		// operation, which will rebuild the primary builder.
		//
		// On top of that we need to use the depfile generated by the bigbp
		// rule.  We do this by depending on that file and then setting up a
		// phony rule to generate it that uses the depfile.
		ctx.Build(pctx, blueprint.BuildParams{
			Rule:    rebootstrap,
			Outputs: []string{"build.ninja"},
			Inputs:  []string{"$BootstrapManifest"},
			Implicits: []string{
				"$Bootstrap",
				primaryBuilderFile,
				mainNinjaFile,
			},
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
			Outputs: []string{"$BootstrapManifest"},
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

// srcDir returns the path of the directory that all source file paths are
// specified relative to.
func srcDir(ctx blueprint.ModuleContext) string {
	return filepath.Join("$SrcDir", ctx.ModuleDir())
}

// objDir returns the module-specific object directory path.
func objDir(ctx blueprint.ModuleContext) string {
	return filepath.Join(bootstrapDir, ctx.ModuleName(), "obj")
}
