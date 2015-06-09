package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/blueprint"
)

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "No filename supplied\n")
		flag.Usage()
		os.Exit(1)
	}

	ctx := blueprint.NewContext()
	ctx.SetIgnoreUnknownModuleTypes(true)
	blueprints, errs := ctx.FindBlueprintsFiles(flag.Arg(0))
	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "%d errors parsing %s\n", len(errs), flag.Arg(0))
		fmt.Fprintln(os.Stderr, errs)
		os.Exit(1)
	}

	for _, file := range blueprints {
		fmt.Println(file)
	}
}
