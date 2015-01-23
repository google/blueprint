package main

import (
	"blueprint"
	"blueprint/bootstrap"
	"flag"
)

var runAsPrimaryBuilder bool

func init() {
	flag.BoolVar(&runAsPrimaryBuilder, "p", false, "run as a primary builder")
}

type Config bool

func (c Config) GeneratingBootstrapper() bool {
	return bool(c)
}

func main() {
	flag.Parse()

	ctx := blueprint.NewContext()
	if !runAsPrimaryBuilder {
		ctx.SetIgnoreUnknownModuleTypes(true)
	}

	config := Config(!runAsPrimaryBuilder)

	bootstrap.Main(ctx, config)
}
