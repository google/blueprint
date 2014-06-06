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
var checkFile string

// topLevelBlueprintsFile is set by Main as a way to pass this information on to
// the bootstrap build manifest generators.  This information was not passed via
// the Config object so as to allow the caller of Main to use whatever Config
// object it wants.
var topLevelBlueprintsFile string

func init() {
	flag.StringVar(&outFile, "o", "build.ninja.in", "the Ninja file to output")
	flag.StringVar(&depFile, "d", "", "the dependency file to output")
	flag.StringVar(&checkFile, "c", "", "the existing file to check against")
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

	const outFilePermissions = 0666
	err = ioutil.WriteFile(outFile, buf.Bytes(), outFilePermissions)
	if err != nil {
		fatalf("error writing %s: %s", outFile, err)
	}

	if checkFile != "" {
		checkData, err := ioutil.ReadFile(checkFile)
		if err != nil {
			fatalf("error reading %s: %s", checkFile, err)
		}

		matches := buf.Len() == len(checkData)
		if !matches {
			for i, value := range buf.Bytes() {
				if value != checkData[i] {
					matches = false
					break
				}
			}
		}

		if matches {
			// The new file content matches the check-file content, so we set
			// the new file's mtime and atime to match that of the check-file.
			checkFileInfo, err := os.Stat(checkFile)
			if err != nil {
				fatalf("error stat'ing %s: %s", checkFile, err)
			}

			time := checkFileInfo.ModTime()
			err = os.Chtimes(outFile, time, time)
			if err != nil {
				fatalf("error setting timestamps for %s: %s", outFile, err)
			}
		}
	}

	if depFile != "" {
		f, err := os.Create(depFile)
		if err != nil {
			fatalf("error creating depfile: %s", err)
		}

		_, err = fmt.Fprintf(f, "%s: \\\n %s\n", outFile,
			strings.Join(deps, " \\\n "))
		if err != nil {
			fatalf("error writing depfile: %s", err)
		}

		f.Close()
	}

	os.Exit(0)
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
	os.Exit(1)
}

func fatalErrors(errs []error) {
	for _, err := range errs {
		switch err.(type) {
		case *blueprint.Error:
			_, _ = fmt.Printf("%s\n", err.Error())
		default:
			_, _ = fmt.Printf("internal error: %s\n", err)
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
