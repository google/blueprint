package bootstrap

import (
	"blueprint"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

var outFile string
var depFile string
var depTarget string

// topLevelBlueprintsFile is set by Main as a way to pass this information on to
// the bootstrap build manifest generators.  This information was not passed via
// the Config object so as to allow the caller of Main to use whatever Config
// object it wants.
var topLevelBlueprintsFile string

func init() {
	flag.StringVar(&outFile, "o", "build.ninja.in", "the Ninja file to output")
	flag.StringVar(&depFile, "d", "", "the dependency file to output")
	flag.StringVar(&depTarget, "t", "", "the target name for the dependency "+
		"file")
}

func Main(ctx *blueprint.Context, config blueprint.Config) {
	if !flag.Parsed() {
		flag.Parse()
	}

	ctx.RegisterModuleType("bootstrap_go_package", goPackageModule)
	ctx.RegisterModuleType("bootstrap_go_binary", goBinaryModule)
	ctx.RegisterSingleton("bootstrap", newSingleton())

	if flag.NArg() != 1 {
		fatalf("no Blueprints file specified")
	}

	topLevelBlueprintsFile = flag.Arg(0)

	deps, errs := ctx.ParseBlueprintsFiles(topLevelBlueprintsFile)
	if len(errs) > 0 {
		fatalErrors(errs)
	}

	errs = ctx.PrepareBuildActions(config)
	if len(errs) > 0 {
		fatalErrors(errs)
	}

	buf := bytes.NewBuffer(nil)
	err := ctx.WriteBuildFile(buf)
	if err != nil {
		fatalf("error generating Ninja file contents: %s", err)
	}

	err = writeFileIfChanged(outFile, buf.Bytes(), 0666)
	if err != nil {
		fatalf("error writing %s: %s", outFile, err)
	}

	if depFile != "" {
		f, err := os.Create(depFile)
		if err != nil {
			fatalf("error creating depfile: %s", err)
		}

		target := depTarget
		if target == "" {
			target = outFile
		}

		_, err = fmt.Fprintf(f, "%s: \\\n %s\n", target,
			strings.Join(deps, " \\\n "))
		if err != nil {
			fatalf("error writing depfile: %s", err)
		}

		f.Close()
	}

	os.Exit(0)
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func fatalErrors(errs []error) {
	for _, err := range errs {
		switch err.(type) {
		case *blueprint.Error:
			_, _ = fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		default:
			_, _ = fmt.Fprintf(os.Stderr, "internal error: %s\n", err)
		}
	}
	os.Exit(1)
}

func writeFileIfChanged(filename string, data []byte, perm os.FileMode) error {
	var isChanged bool

	info, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// The file does not exist yet.
			isChanged = true
		} else {
			return err
		}
	} else {
		if info.Size() != int64(len(data)) {
			isChanged = true
		} else {
			oldData, err := ioutil.ReadFile(filename)
			if err != nil {
				return err
			}

			if len(oldData) != len(data) {
				isChanged = true
			} else {
				for i := range data {
					if oldData[i] != data[i] {
						isChanged = true
						break
					}
				}
			}
		}
	}

	if isChanged {
		err = ioutil.WriteFile(filename, data, perm)
		if err != nil {
			return err
		}
	}

	return nil
}
