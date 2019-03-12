// Copyright 2019 Google Inc. All rights reserved.
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

package bpdoc

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

// Handles parsing and low-level processing of Blueprint module source files. Note that most getter
// functions associated with Reader only fill basic information that can be simply extracted from
// AST parsing results. More sophisticated processing is performed in bpdoc.go
type Reader struct {
	pkgFiles map[string][]string // Map of package name to source files, provided by constructor

	mutex  sync.Mutex
	goPkgs map[string]*doc.Package    // Map of package name to parsed Go AST, protected by mutex
	ps     map[string]*PropertyStruct // Map of module type name to property struct, protected by mutex
}

func NewReader(pkgFiles map[string][]string) *Reader {
	return &Reader{
		pkgFiles: pkgFiles,
		goPkgs:   make(map[string]*doc.Package),
		ps:       make(map[string]*PropertyStruct),
	}
}

func (r *Reader) Package(path string) (*Package, error) {
	goPkg, err := r.goPkg(path)
	if err != nil {
		return nil, err
	}

	return &Package{
		Name: goPkg.Name,
		Path: path,
		Text: goPkg.Doc,
	}, nil
}

func (r *Reader) ModuleType(name string, factory reflect.Value) (*ModuleType, error) {
	f := runtime.FuncForPC(factory.Pointer())

	pkgPath, err := funcNameToPkgPath(f.Name())
	if err != nil {
		return nil, err
	}

	factoryName := strings.TrimPrefix(f.Name(), pkgPath+".")

	text, err := r.getModuleTypeDoc(pkgPath, factoryName)
	if err != nil {
		return nil, err
	}

	return &ModuleType{
		Name:    name,
		PkgPath: pkgPath,
		Text:    formatText(text),
	}, nil
}

// Return the PropertyStruct associated with a property struct type.  The type should be in the
// format <package path>.<type name>
func (r *Reader) PropertyStruct(pkgPath, name string, defaults reflect.Value) (*PropertyStruct, error) {
	ps := r.getPropertyStruct(pkgPath, name)

	if ps == nil {
		pkg, err := r.goPkg(pkgPath)
		if err != nil {
			return nil, err
		}

		for _, t := range pkg.Types {
			if t.Name == name {
				ps, err = newPropertyStruct(t)
				if err != nil {
					return nil, err
				}
				ps = r.putPropertyStruct(pkgPath, name, ps)
			}
		}
	}

	if ps == nil {
		return nil, fmt.Errorf("package %q type %q not found", pkgPath, name)
	}

	ps = ps.Clone()
	ps.SetDefaults(defaults)

	return ps, nil
}

func (r *Reader) getModuleTypeDoc(pkgPath, factoryFuncName string) (string, error) {
	goPkg, err := r.goPkg(pkgPath)
	if err != nil {
		return "", err
	}

	for _, fn := range goPkg.Funcs {
		if fn.Name == factoryFuncName {
			return fn.Doc, nil
		}
	}

	// The doc package may associate the method with the type it returns, so iterate through those too
	for _, typ := range goPkg.Types {
		for _, fn := range typ.Funcs {
			if fn.Name == factoryFuncName {
				return fn.Doc, nil
			}
		}
	}

	return "", nil
}

func (r *Reader) getPropertyStruct(pkgPath, name string) *PropertyStruct {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	name = pkgPath + "." + name

	return r.ps[name]
}

func (r *Reader) putPropertyStruct(pkgPath, name string, ps *PropertyStruct) *PropertyStruct {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	name = pkgPath + "." + name

	if r.ps[name] != nil {
		return r.ps[name]
	} else {
		r.ps[name] = ps
		return ps
	}
}

// Package AST generation and storage
func (r *Reader) goPkg(pkgPath string) (*doc.Package, error) {
	pkg := r.getGoPkg(pkgPath)
	if pkg == nil {
		if files, ok := r.pkgFiles[pkgPath]; ok {
			var err error
			pkgAST, err := packageAST(files)
			if err != nil {
				return nil, err
			}
			pkg = doc.New(pkgAST, pkgPath, doc.AllDecls)
			pkg = r.putGoPkg(pkgPath, pkg)
		} else {
			return nil, fmt.Errorf("unknown package %q", pkgPath)
		}
	}
	return pkg, nil
}

func (r *Reader) getGoPkg(pkgPath string) *doc.Package {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.goPkgs[pkgPath]
}

func (r *Reader) putGoPkg(pkgPath string, pkg *doc.Package) *doc.Package {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.goPkgs[pkgPath] != nil {
		return r.goPkgs[pkgPath]
	} else {
		r.goPkgs[pkgPath] = pkg
		return pkg
	}
}

// A regex to find a package path within a function name. It finds the shortest string that is
// followed by '.' and doesn't have any '/'s left.
var pkgPathRe = regexp.MustCompile("^(.*?)\\.[^/]+$")

func funcNameToPkgPath(f string) (string, error) {
	s := pkgPathRe.FindStringSubmatch(f)
	if len(s) < 2 {
		return "", fmt.Errorf("failed to extract package path from %q", f)
	}
	return s[1], nil
}

func packageAST(files []string) (*ast.Package, error) {
	asts := make(map[string]*ast.File)

	fset := token.NewFileSet()
	for _, file := range files {
		ast, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		asts[file] = ast
	}

	pkg, _ := ast.NewPackage(fset, asts, nil, nil)
	return pkg, nil
}
