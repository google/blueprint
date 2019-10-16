package main

import (
	"flag"
	"path/filepath"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"

	"github.com/TKilbourn/simplebp"
)

func main() {
	// Parse commandline flags.
	flag.Parse()

	// The first argument to this binary is the root Blueprints file. The
	// directory for this file is the root source dir.
	srcDir := filepath.Dir(flag.Arg(0))

	// Create a context.
	ctx := blueprint.NewContext()

	// Register each of our module types, pairing the names that will be
	// used in the Blueprints files with the factory function for the
	// module.
	ctx.RegisterModuleType("cc_binary", simplebp.NewCcBinary)
	ctx.RegisterModuleType("cc_shared_lib", simplebp.NewCcSharedLib)
	ctx.RegisterModuleType("run_script", simplebp.NewScript)

	// Create a config with the source and build dirs.
	config := simplebp.NewConfig(srcDir, bootstrap.BuildDir)

	// Do the magic.
	bootstrap.Main(ctx, config)
}
