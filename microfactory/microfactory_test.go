// Copyright 2017 Google Inc. All rights reserved.
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

package main

import (
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestSimplePackagePathMap(t *testing.T) {
	t.Parallel()

	var pkgMap pkgPathMapping
	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.Var(&pkgMap, "m", "")
	err := flags.Parse([]string{
		"-m", "android/soong=build/soong/",
		"-m", "github.com/google/blueprint/=build/blueprint",
	})
	if err != nil {
		t.Fatal(err)
	}

	compare := func(got, want interface{}) {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Unexpected values in .pkgs:\nwant: %v\n got: %v",
				want, got)
		}
	}

	wantPkgs := []string{"android/soong", "github.com/google/blueprint"}
	compare(pkgMap.pkgs, wantPkgs)
	compare(pkgMap.paths[wantPkgs[0]], "build/soong")
	compare(pkgMap.paths[wantPkgs[1]], "build/blueprint")

	got, ok, err := pkgMap.Path("android/soong/ui/test")
	if err != nil {
		t.Error("Unexpected error in pkgMap.Path(soong):", err)
	} else if !ok {
		t.Error("Expected a result from pkgMap.Path(soong)")
	} else {
		compare(got, "build/soong/ui/test")
	}

	got, ok, err = pkgMap.Path("github.com/google/blueprint")
	if err != nil {
		t.Error("Unexpected error in pkgMap.Path(blueprint):", err)
	} else if !ok {
		t.Error("Expected a result from pkgMap.Path(blueprint)")
	} else {
		compare(got, "build/blueprint")
	}
}

func TestBadPackagePathMap(t *testing.T) {
	t.Parallel()

	var pkgMap pkgPathMapping
	if _, _, err := pkgMap.Path("testing"); err == nil {
		t.Error("Expected error if no maps are specified")
	}
	if err := pkgMap.Set(""); err == nil {
		t.Error("Expected error with blank argument, but none returned")
	}
	if err := pkgMap.Set("a=a"); err != nil {
		t.Error("Unexpected error: %v", err)
	}
	if err := pkgMap.Set("a=b"); err == nil {
		t.Error("Expected error with duplicate package prefix, but none returned")
	}
	if _, ok, err := pkgMap.Path("testing"); err != nil {
		t.Error("Unexpected error: %v", err)
	} else if ok {
		t.Error("Expected testing to be consider in the stdlib")
	}
}

// TestSingleBuild ensures that just a basic build works.
func TestSingleBuild(t *testing.T) {
	t.Parallel()

	setupDir(t, func(dir string, loadPkg loadPkgFunc) {
		// The output binary
		out := filepath.Join(dir, "out", "test")

		pkg := loadPkg()

		if err := pkg.Compile(filepath.Join(dir, "out"), ""); err != nil {
			t.Fatalf("Got error when compiling:", err)
		}

		if err := pkg.Link(out); err != nil {
			t.Fatal("Got error when linking:", err)
		}

		if _, err := os.Stat(out); err != nil {
			t.Error("Cannot stat output:", err)
		}
	})
}

// testBuildAgain triggers two builds, running the modify function in between
// each build. It verifies that the second build did or did not actually need
// to rebuild anything based on the shouldRebuild argument.
func testBuildAgain(t *testing.T,
	shouldRecompile, shouldRelink bool,
	modify func(dir string, loadPkg loadPkgFunc),
	after func(pkg *GoPackage)) {

	t.Parallel()

	setupDir(t, func(dir string, loadPkg loadPkgFunc) {
		// The output binary
		out := filepath.Join(dir, "out", "test")

		pkg := loadPkg()

		if err := pkg.Compile(filepath.Join(dir, "out"), ""); err != nil {
			t.Fatal("Got error when compiling:", err)
		}

		if err := pkg.Link(out); err != nil {
			t.Fatal("Got error when linking:", err)
		}

		var firstTime time.Time
		if stat, err := os.Stat(out); err == nil {
			firstTime = stat.ModTime()
		} else {
			t.Fatal("Failed to stat output file:", err)
		}

		// mtime on HFS+ (the filesystem on darwin) are stored with 1
		// second granularity, so the timestamp checks will fail unless
		// we wait at least a second. Sleeping 1.1s to be safe.
		if runtime.GOOS == "darwin" {
			time.Sleep(1100 * time.Millisecond)
		}

		modify(dir, loadPkg)

		pkg = loadPkg()

		if err := pkg.Compile(filepath.Join(dir, "out"), ""); err != nil {
			t.Fatal("Got error when compiling:", err)
		}
		if shouldRecompile {
			if !pkg.rebuilt {
				t.Fatal("Package should have recompiled, but was not recompiled.")
			}
		} else {
			if pkg.rebuilt {
				t.Fatal("Package should not have needed to be recompiled, but was recompiled.")
			}
		}

		if err := pkg.Link(out); err != nil {
			t.Fatal("Got error while linking:", err)
		}
		if shouldRelink {
			if !pkg.rebuilt {
				t.Error("Package should have relinked, but was not relinked.")
			}
		} else {
			if pkg.rebuilt {
				t.Error("Package should not have needed to be relinked, but was relinked.")
			}
		}

		if stat, err := os.Stat(out); err == nil {
			if shouldRelink {
				if stat.ModTime() == firstTime {
					t.Error("Output timestamp should be different, but both were", firstTime)
				}
			} else {
				if stat.ModTime() != firstTime {
					t.Error("Output timestamp should be the same.")
					t.Error(" first:", firstTime)
					t.Error("second:", stat.ModTime())
				}
			}
		} else {
			t.Fatal("Failed to stat output file:", err)
		}

		after(pkg)
	})
}

// TestRebuildAfterNoChanges ensures that we don't rebuild if nothing
// changes
func TestRebuildAfterNoChanges(t *testing.T) {
	testBuildAgain(t, false, false, func(dir string, loadPkg loadPkgFunc) {}, func(pkg *GoPackage) {})
}

// TestRebuildAfterTimestamp ensures that we don't rebuild because
// timestamps of important files have changed. We should only rebuild if the
// content hashes are different.
func TestRebuildAfterTimestampChange(t *testing.T) {
	testBuildAgain(t, false, false, func(dir string, loadPkg loadPkgFunc) {
		// Ensure that we've spent some amount of time asleep
		time.Sleep(100 * time.Millisecond)

		newTime := time.Now().Local()
		os.Chtimes(filepath.Join(dir, "test.fact"), newTime, newTime)
		os.Chtimes(filepath.Join(dir, "main/main.go"), newTime, newTime)
		os.Chtimes(filepath.Join(dir, "a/a.go"), newTime, newTime)
		os.Chtimes(filepath.Join(dir, "a/b.go"), newTime, newTime)
		os.Chtimes(filepath.Join(dir, "b/a.go"), newTime, newTime)
	}, func(pkg *GoPackage) {})
}

// TestRebuildAfterGoChange ensures that we rebuild after a content change
// to a package's go file.
func TestRebuildAfterGoChange(t *testing.T) {
	testBuildAgain(t, true, true, func(dir string, loadPkg loadPkgFunc) {
		if err := ioutil.WriteFile(filepath.Join(dir, "a", "a.go"), []byte(go_a_a+"\n"), 0666); err != nil {
			t.Fatal("Error writing a/a.go:", err)
		}
	}, func(pkg *GoPackage) {
		if !pkg.directDeps[0].rebuilt {
			t.Fatal("android/soong/a should have rebuilt")
		}
		if !pkg.directDeps[1].rebuilt {
			t.Fatal("android/soong/b should have rebuilt")
		}
	})
}

// TestRebuildAfterMainChange ensures that we don't rebuild any dependencies
// if only the main package's go files are touched.
func TestRebuildAfterMainChange(t *testing.T) {
	testBuildAgain(t, true, true, func(dir string, loadPkg loadPkgFunc) {
		if err := ioutil.WriteFile(filepath.Join(dir, "main", "main.go"), []byte(go_main_main+"\n"), 0666); err != nil {
			t.Fatal("Error writing main/main.go:", err)
		}
	}, func(pkg *GoPackage) {
		if pkg.directDeps[0].rebuilt {
			t.Fatal("android/soong/a should not have rebuilt")
		}
		if pkg.directDeps[1].rebuilt {
			t.Fatal("android/soong/b should not have rebuilt")
		}
	})
}

// TestRebuildAfterRemoveOut ensures that we rebuild if the output file is
// missing, even if everything else doesn't need rebuilding.
func TestRebuildAfterRemoveOut(t *testing.T) {
	testBuildAgain(t, false, true, func(dir string, loadPkg loadPkgFunc) {
		if err := os.Remove(filepath.Join(dir, "out", "test")); err != nil {
			t.Fatal("Failed to remove output:", err)
		}
	}, func(pkg *GoPackage) {})
}

// TestRebuildAfterPartialBuild ensures that even if the build was interrupted
// between the recompile and relink stages, we'll still relink when we run again.
func TestRebuildAfterPartialBuild(t *testing.T) {
	testBuildAgain(t, false, true, func(dir string, loadPkg loadPkgFunc) {
		if err := ioutil.WriteFile(filepath.Join(dir, "main", "main.go"), []byte(go_main_main+"\n"), 0666); err != nil {
			t.Fatal("Error writing main/main.go:", err)
		}

		pkg := loadPkg()

		if err := pkg.Compile(filepath.Join(dir, "out"), ""); err != nil {
			t.Fatal("Got error when compiling:", err)
		}
		if !pkg.rebuilt {
			t.Fatal("Package should have recompiled, but was not recompiled.")
		}
	}, func(pkg *GoPackage) {})
}

// BenchmarkInitialBuild computes how long a clean build takes (for tiny test
// inputs).
func BenchmarkInitialBuild(b *testing.B) {
	for i := 0; i < b.N; i++ {
		setupDir(b, func(dir string, loadPkg loadPkgFunc) {
			pkg := loadPkg()
			if err := pkg.Compile(filepath.Join(dir, "out"), ""); err != nil {
				b.Fatal("Got error when compiling:", err)
			}

			if err := pkg.Link(filepath.Join(dir, "out", "test")); err != nil {
				b.Fatal("Got error when linking:", err)
			}
		})
	}
}

// BenchmarkMinIncrementalBuild computes how long an incremental build that
// doesn't actually need to build anything takes.
func BenchmarkMinIncrementalBuild(b *testing.B) {
	setupDir(b, func(dir string, loadPkg loadPkgFunc) {
		pkg := loadPkg()

		if err := pkg.Compile(filepath.Join(dir, "out"), ""); err != nil {
			b.Fatal("Got error when compiling:", err)
		}

		if err := pkg.Link(filepath.Join(dir, "out", "test")); err != nil {
			b.Fatal("Got error when linking:", err)
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			pkg := loadPkg()

			if err := pkg.Compile(filepath.Join(dir, "out"), ""); err != nil {
				b.Fatal("Got error when compiling:", err)
			}

			if err := pkg.Link(filepath.Join(dir, "out", "test")); err != nil {
				b.Fatal("Got error when linking:", err)
			}

			if pkg.rebuilt {
				b.Fatal("Should not have rebuilt anything")
			}
		}
	})
}

///////////////////////////////////////////////////////
// Templates used to create fake compilable packages //
///////////////////////////////////////////////////////

const go_main_main = `
package main
import (
	"fmt"
	"android/soong/a"
	"android/soong/b"
)
func main() {
	fmt.Println(a.Stdout, b.Stdout)
}
`

const go_a_a = `
package a
import "os"
var Stdout = os.Stdout
`

const go_a_b = `
package a
`

const go_b_a = `
package b
import "android/soong/a"
var Stdout = a.Stdout
`

type T interface {
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
}

type loadPkgFunc func() *GoPackage

func setupDir(t T, test func(dir string, loadPkg loadPkgFunc)) {
	dir, err := ioutil.TempDir("", "test")
	if err != nil {
		t.Fatalf("Error creating temporary directory: %#v", err)
	}
	defer os.RemoveAll(dir)

	writeFile := func(name, contents string) {
		if err := ioutil.WriteFile(filepath.Join(dir, name), []byte(contents), 0666); err != nil {
			t.Fatalf("Error writing %q: %#v", name, err)
		}
	}
	mkdir := func(name string) {
		if err := os.Mkdir(filepath.Join(dir, name), 0777); err != nil {
			t.Fatalf("Error creating %q directory: %#v", name, err)
		}
	}
	mkdir("main")
	mkdir("a")
	mkdir("b")
	writeFile("main/main.go", go_main_main)
	writeFile("a/a.go", go_a_a)
	writeFile("a/b.go", go_a_b)
	writeFile("b/a.go", go_b_a)

	loadPkg := func() *GoPackage {
		pkg := &GoPackage{
			Name: "main",
		}
		pkgMap := &pkgPathMapping{}
		pkgMap.Set("android/soong=" + dir)
		if err := pkg.FindDeps(filepath.Join(dir, "main"), pkgMap); err != nil {
			t.Fatalf("Error finding deps: %v", err)
		}
		return pkg
	}

	test(dir, loadPkg)
}
