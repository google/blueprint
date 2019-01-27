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
	"html/template"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/google/blueprint/proptools"
)

type Reader struct {
	pkgFiles map[string][]string // Map of package name to source files, provided by constructor

	mutex sync.Mutex
	pkgs  map[string]*doc.Package    // Map of package name to parsed Go AST, protected by mutex
	ps    map[string]*PropertyStruct // Map of type name to property struct, protected by mutex
}

func NewReader(pkgFiles map[string][]string) *Reader {
	return &Reader{
		pkgFiles: pkgFiles,
		pkgs:     make(map[string]*doc.Package),
		ps:       make(map[string]*PropertyStruct),
	}
}

func funcToPkgPath(f string) (string, error) {
	p := strings.Split(f, "/")

	pkgPath := strings.Join(p[:len(p)-1], "/")

	p = strings.Split(p[len(p)-1], ".")
	if len(p) < 2 {
		return "", fmt.Errorf("failed to extract package path from %q", f)
	}

	if pkgPath != "" {
		pkgPath += "/"
	}
	pkgPath += p[0]

	return pkgPath, nil
}

// Return the PropertyStruct associated with a property struct type.  The type should be in the
// format <package path>.<type name>
func (r *Reader) PropertyStruct(pkgPath, name string, defaults reflect.Value) (*PropertyStruct, error) {
	ps := r.getPropertyStruct(pkgPath, name)

	if ps == nil {
		pkg, err := r.pkg(pkgPath)
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

type PropertyStruct struct {
	Name       string
	Text       string
	Properties []Property
}

type Property struct {
	Name       string
	OtherNames []string
	Type       string
	Tag        reflect.StructTag
	Text       template.HTML
	OtherTexts []template.HTML
	Properties []Property
	Default    string
}

func (ps *PropertyStruct) Clone() *PropertyStruct {
	ret := *ps
	ret.Properties = append([]Property(nil), ret.Properties...)
	for i, prop := range ret.Properties {
		ret.Properties[i] = prop.Clone()
	}

	return &ret
}

func (p *Property) Clone() Property {
	ret := *p
	ret.Properties = append([]Property(nil), ret.Properties...)
	for i, prop := range ret.Properties {
		ret.Properties[i] = prop.Clone()
	}

	return ret
}

func (p *Property) Equal(other Property) bool {
	return p.Name == other.Name && p.Type == other.Type && p.Tag == other.Tag &&
		p.Text == other.Text && p.Default == other.Default &&
		stringArrayEqual(p.OtherNames, other.OtherNames) &&
		htmlArrayEqual(p.OtherTexts, other.OtherTexts) &&
		p.SameSubProperties(other)
}

func (ps *PropertyStruct) SetDefaults(defaults reflect.Value) {
	setDefaults(ps.Properties, defaults)
}

func setDefaults(properties []Property, defaults reflect.Value) {
	for i := range properties {
		prop := &properties[i]
		fieldName := proptools.FieldNameForProperty(prop.Name)
		f := defaults.FieldByName(fieldName)
		if (f == reflect.Value{}) {
			panic(fmt.Errorf("property %q does not exist in %q", fieldName, defaults.Type()))
		}

		if reflect.DeepEqual(f.Interface(), reflect.Zero(f.Type()).Interface()) {
			continue
		}

		if f.Kind() == reflect.Interface {
			f = f.Elem()
		}

		if f.Kind() == reflect.Ptr {
			if f.IsNil() {
				continue
			}
			f = f.Elem()
		}

		if f.Kind() == reflect.Struct {
			setDefaults(prop.Properties, f)
		} else {
			prop.Default = fmt.Sprintf("%v", f.Interface())
		}
	}
}

func stringArrayEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func htmlArrayEqual(a, b []template.HTML) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func (p *Property) SameSubProperties(other Property) bool {
	if len(p.Properties) != len(other.Properties) {
		return false
	}

	for i := range p.Properties {
		if !p.Properties[i].Equal(other.Properties[i]) {
			return false
		}
	}

	return true
}

func (ps *PropertyStruct) GetByName(name string) *Property {
	return getByName(name, "", &ps.Properties)
}

func getByName(name string, prefix string, props *[]Property) *Property {
	for i := range *props {
		if prefix+(*props)[i].Name == name {
			return &(*props)[i]
		} else if strings.HasPrefix(name, prefix+(*props)[i].Name+".") {
			return getByName(name, prefix+(*props)[i].Name+".", &(*props)[i].Properties)
		}
	}
	return nil
}

func (p *Property) Nest(nested *PropertyStruct) {
	//p.Name += "(" + nested.Name + ")"
	//p.Text += "(" + nested.Text + ")"
	p.Properties = append(p.Properties, nested.Properties...)
}

func newPropertyStruct(t *doc.Type) (*PropertyStruct, error) {
	typeSpec := t.Decl.Specs[0].(*ast.TypeSpec)
	ps := PropertyStruct{
		Name: t.Name,
		Text: t.Doc,
	}

	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		return nil, fmt.Errorf("type of %q is not a struct", t.Name)
	}

	var err error
	ps.Properties, err = structProperties(structType)
	if err != nil {
		return nil, err
	}

	return &ps, nil
}

func structProperties(structType *ast.StructType) (props []Property, err error) {
	for _, f := range structType.Fields.List {
		names := f.Names
		if names == nil {
			// Anonymous fields have no name, use the type as the name
			// TODO: hide the name and make the properties show up in the embedding struct
			if t, ok := f.Type.(*ast.Ident); ok {
				names = append(names, t)
			}
		}
		for _, n := range names {
			var name, typ, tag, text string
			var innerProps []Property
			if n != nil {
				name = proptools.PropertyNameForField(n.Name)
			}
			if f.Doc != nil {
				text = f.Doc.Text()
			}
			if f.Tag != nil {
				tag, err = strconv.Unquote(f.Tag.Value)
				if err != nil {
					return nil, err
				}
			}

			t := f.Type
			if star, ok := t.(*ast.StarExpr); ok {
				t = star.X
			}
			switch a := t.(type) {
			case *ast.ArrayType:
				typ = "list of strings"
			case *ast.InterfaceType:
				typ = "interface"
			case *ast.Ident:
				typ = a.Name
			case *ast.StructType:
				innerProps, err = structProperties(a)
				if err != nil {
					return nil, err
				}
			default:
				typ = fmt.Sprintf("%T", f.Type)
			}

			var html template.HTML

			lines := strings.Split(text, "\n")
			preformatted := false
			for _, line := range lines {
				r, _ := utf8.DecodeRuneInString(line)
				indent := unicode.IsSpace(r)
				if indent && !preformatted {
					html += "<pre>\n"
				} else if !indent && preformatted {
					html += "</pre>\n"
				}
				preformatted = indent
				html += template.HTML(template.HTMLEscapeString(line)) + "\n"
			}
			if preformatted {
				html += "</pre>\n"
			}

			props = append(props, Property{
				Name:       name,
				Type:       typ,
				Tag:        reflect.StructTag(tag),
				Text:       html,
				Properties: innerProps,
			})
		}
	}

	return props, nil
}

func (ps *PropertyStruct) ExcludeByTag(key, value string) {
	filterPropsByTag(&ps.Properties, key, value, true)
}

func (ps *PropertyStruct) IncludeByTag(key, value string) {
	filterPropsByTag(&ps.Properties, key, value, false)
}

func filterPropsByTag(props *[]Property, key, value string, exclude bool) {
	// Create a slice that shares the storage of props but has 0 length.  Appending up to
	// len(props) times to this slice will overwrite the original slice contents
	filtered := (*props)[:0]
	for _, x := range *props {
		tag := x.Tag.Get(key)
		for _, entry := range strings.Split(tag, ",") {
			if (entry == value) == !exclude {
				filtered = append(filtered, x)
			}
		}
	}

	*props = filtered
}

// Package AST generation and storage
func (r *Reader) pkg(pkgPath string) (*doc.Package, error) {
	pkg := r.getPackage(pkgPath)
	if pkg == nil {
		if files, ok := r.pkgFiles[pkgPath]; ok {
			var err error
			pkgAST, err := packageAST(files)
			if err != nil {
				return nil, err
			}
			pkg = doc.New(pkgAST, pkgPath, doc.AllDecls)
			pkg = r.putPackage(pkgPath, pkg)
		} else {
			return nil, fmt.Errorf("unknown package %q", pkgPath)
		}
	}
	return pkg, nil
}

func (r *Reader) getPackage(pkgPath string) *doc.Package {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.pkgs[pkgPath]
}

func (r *Reader) putPackage(pkgPath string, pkg *doc.Package) *doc.Package {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.pkgs[pkgPath] != nil {
		return r.pkgs[pkgPath]
	} else {
		r.pkgs[pkgPath] = pkg
		return pkg
	}
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
