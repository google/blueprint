package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/blueprint"
)

// TODO: implement, need to set a variable in the top-level scope
// This would let you start from a directory that isn't the root directory
//var subname = flag.String("subname", "Android.bp", "Default subname")

var name = flag.String("name", "", "Only print blueprints with this file name")
var without = flag.String("without", "", "Only print blueprints without this file in the same directory")

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
		if *name != "" && filepath.Base(file) != *name {
			continue
		}
		if *without != "" {
			_, err := os.Stat(filepath.Join(filepath.Dir(file), *without))
			if !os.IsNotExist(err) {
				continue
			}
		}
		fmt.Println(file)
	}
}
