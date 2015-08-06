package bootstrap

import (
	"fmt"
	"path/filepath"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap/bpdoc"
	"github.com/google/blueprint/pathtools"
)

func writeDocs(ctx *blueprint.Context, srcDir, filename string) error {
	// Find the module that's marked as the "primary builder", which means it's
	// creating the binary that we'll use to generate the non-bootstrap
	// build.ninja file.
	var primaryBuilders []*goBinary
	var minibp *goBinary
	ctx.VisitAllModulesIf(isBootstrapBinaryModule,
		func(module blueprint.Module) {
			binaryModule := module.(*goBinary)
			if binaryModule.properties.PrimaryBuilder {
				primaryBuilders = append(primaryBuilders, binaryModule)
			}
			if ctx.ModuleName(binaryModule) == "minibp" {
				minibp = binaryModule
			}
		})

	if minibp == nil {
		panic("missing minibp")
	}

	var primaryBuilder *goBinary
	switch len(primaryBuilders) {
	case 0:
		// If there's no primary builder module then that means we'll use minibp
		// as the primary builder.
		primaryBuilder = minibp

	case 1:
		primaryBuilder = primaryBuilders[0]

	default:
		return fmt.Errorf("multiple primary builder modules present")
	}

	pkgFiles := make(map[string][]string)
	ctx.VisitDepsDepthFirst(primaryBuilder, func(module blueprint.Module) {
		switch m := module.(type) {
		case (*goPackage):
			pkgFiles[m.properties.PkgPath] = pathtools.PrefixPaths(m.properties.Srcs,
				filepath.Join(srcDir, ctx.ModuleDir(m)))
		default:
			panic(fmt.Errorf("unknown dependency type %T", module))
		}
	})

	return bpdoc.Write(filename, pkgFiles, ctx.ModuleTypePropertyStructs())
}
