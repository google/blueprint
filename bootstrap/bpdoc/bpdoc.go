package bpdoc

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io/ioutil"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

type Context struct {
	pkgFiles map[string][]string // Map of package name to source files, provided by constructor

	mutex sync.Mutex
	pkgs  map[string]*doc.Package    // Map of package name to parsed Go AST, protected by mutex
	ps    map[string]*PropertyStruct // Map of type name to property struct, protected by mutex
}

func NewContext(pkgFiles map[string][]string) *Context {
	return &Context{
		pkgFiles: pkgFiles,
		pkgs:     make(map[string]*doc.Package),
		ps:       make(map[string]*PropertyStruct),
	}
}

// Return the PropertyStruct associated with a property struct type.  The type should be in the
// format <package path>.<type name>
func (c *Context) PropertyStruct(pkgPath, name string, defaults reflect.Value) (*PropertyStruct, error) {
	ps := c.getPropertyStruct(pkgPath, name)

	if ps == nil {
		pkg, err := c.pkg(pkgPath)
		if err != nil {
			return nil, err
		}

		for _, t := range pkg.Types {
			if t.Name == name {
				ps, err = newPropertyStruct(t)
				if err != nil {
					return nil, err
				}
				ps = c.putPropertyStruct(pkgPath, name, ps)
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

func (c *Context) getPropertyStruct(pkgPath, name string) *PropertyStruct {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	name = pkgPath + "." + name

	return c.ps[name]
}

func (c *Context) putPropertyStruct(pkgPath, name string, ps *PropertyStruct) *PropertyStruct {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	name = pkgPath + "." + name

	if c.ps[name] != nil {
		return c.ps[name]
	} else {
		c.ps[name] = ps
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
	Text       string
	OtherTexts []string
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
		stringArrayEqual(p.OtherTexts, other.OtherTexts) &&
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

			props = append(props, Property{
				Name:       name,
				Type:       typ,
				Tag:        reflect.StructTag(tag),
				Text:       text,
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
func (c *Context) pkg(pkgPath string) (*doc.Package, error) {
	pkg := c.getPackage(pkgPath)
	if pkg == nil {
		if files, ok := c.pkgFiles[pkgPath]; ok {
			var err error
			pkgAST, err := NewPackageAST(files)
			if err != nil {
				return nil, err
			}
			pkg = doc.New(pkgAST, pkgPath, doc.AllDecls)
			pkg = c.putPackage(pkgPath, pkg)
		} else {
			return nil, fmt.Errorf("unknown package %q", pkgPath)
		}
	}
	return pkg, nil
}

func (c *Context) getPackage(pkgPath string) *doc.Package {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.pkgs[pkgPath]
}

func (c *Context) putPackage(pkgPath string, pkg *doc.Package) *doc.Package {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.pkgs[pkgPath] != nil {
		return c.pkgs[pkgPath]
	} else {
		c.pkgs[pkgPath] = pkg
		return pkg
	}
}

func NewPackageAST(files []string) (*ast.Package, error) {
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

func Write(filename string, pkgFiles map[string][]string,
	moduleTypePropertyStructs map[string][]interface{}) error {

	c := NewContext(pkgFiles)

	var moduleTypeList []*moduleType
	for moduleType, propertyStructs := range moduleTypePropertyStructs {
		mt, err := getModuleType(c, moduleType, propertyStructs)
		if err != nil {
			return err
		}
		removeEmptyPropertyStructs(mt)
		collapseDuplicatePropertyStructs(mt)
		collapseNestedPropertyStructs(mt)
		combineDuplicateProperties(mt)
		moduleTypeList = append(moduleTypeList, mt)
	}

	sort.Sort(moduleTypeByName(moduleTypeList))

	buf := &bytes.Buffer{}

	unique := 0

	tmpl, err := template.New("file").Funcs(map[string]interface{}{
		"unique": func() int {
			unique++
			return unique
		}}).Parse(fileTemplate)
	if err != nil {
		return err
	}

	err = tmpl.Execute(buf, moduleTypeList)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filename, buf.Bytes(), 0666)
	if err != nil {
		return err
	}

	return nil
}

func getModuleType(c *Context, moduleTypeName string,
	propertyStructs []interface{}) (*moduleType, error) {
	mt := &moduleType{
		Name: moduleTypeName,
		//Text: c.ModuleTypeDocs(moduleType),
	}

	for _, s := range propertyStructs {
		v := reflect.ValueOf(s).Elem()
		t := v.Type()

		// Ignore property structs with unexported or unnamed types
		if t.PkgPath() == "" {
			continue
		}
		ps, err := c.PropertyStruct(t.PkgPath(), t.Name(), v)
		if err != nil {
			return nil, err
		}
		ps.ExcludeByTag("blueprint", "mutated")

		for nestedName, nestedValue := range nestedPropertyStructs(v) {
			nestedType := nestedValue.Type()

			// Ignore property structs with unexported or unnamed types
			if nestedType.PkgPath() == "" {
				continue
			}
			nested, err := c.PropertyStruct(nestedType.PkgPath(), nestedType.Name(), nestedValue)
			if err != nil {
				return nil, err
			}
			nested.ExcludeByTag("blueprint", "mutated")
			nestPoint := ps.GetByName(nestedName)
			if nestPoint == nil {
				return nil, fmt.Errorf("nesting point %q not found", nestedName)
			}

			key, value, err := blueprint.HasFilter(nestPoint.Tag)
			if err != nil {
				return nil, err
			}
			if key != "" {
				nested.IncludeByTag(key, value)
			}

			nestPoint.Nest(nested)
		}
		mt.PropertyStructs = append(mt.PropertyStructs, ps)
	}

	return mt, nil
}

func nestedPropertyStructs(s reflect.Value) map[string]reflect.Value {
	ret := make(map[string]reflect.Value)
	var walk func(structValue reflect.Value, prefix string)
	walk = func(structValue reflect.Value, prefix string) {
		typ := structValue.Type()
		for i := 0; i < structValue.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath != "" {
				// The field is not exported so just skip it.
				continue
			}

			fieldValue := structValue.Field(i)

			switch fieldValue.Kind() {
			case reflect.Bool, reflect.String, reflect.Slice, reflect.Int, reflect.Uint:
				// Nothing
			case reflect.Struct:
				walk(fieldValue, prefix+proptools.PropertyNameForField(field.Name)+".")
			case reflect.Ptr, reflect.Interface:
				if !fieldValue.IsNil() {
					// We leave the pointer intact and zero out the struct that's
					// pointed to.
					elem := fieldValue.Elem()
					if fieldValue.Kind() == reflect.Interface {
						if elem.Kind() != reflect.Ptr {
							panic(fmt.Errorf("can't get type of field %q: interface "+
								"refers to a non-pointer", field.Name))
						}
						elem = elem.Elem()
					}
					if elem.Kind() == reflect.Struct {
						nestPoint := prefix + proptools.PropertyNameForField(field.Name)
						ret[nestPoint] = elem
						walk(elem, nestPoint+".")
					}
				}
			default:
				panic(fmt.Errorf("unexpected kind for property struct field %q: %s",
					field.Name, fieldValue.Kind()))
			}
		}

	}

	walk(s, "")
	return ret
}

// Remove any property structs that have no exported fields
func removeEmptyPropertyStructs(mt *moduleType) {
	for i := 0; i < len(mt.PropertyStructs); i++ {
		if len(mt.PropertyStructs[i].Properties) == 0 {
			mt.PropertyStructs = append(mt.PropertyStructs[:i], mt.PropertyStructs[i+1:]...)
			i--
		}
	}
}

// Squashes duplicates of the same property struct into single entries
func collapseDuplicatePropertyStructs(mt *moduleType) {
	var collapsed []*PropertyStruct

propertyStructLoop:
	for _, from := range mt.PropertyStructs {
		for _, to := range collapsed {
			if from.Name == to.Name {
				collapseDuplicateProperties(&to.Properties, &from.Properties)
				continue propertyStructLoop
			}
		}
		collapsed = append(collapsed, from)
	}
	mt.PropertyStructs = collapsed
}

func collapseDuplicateProperties(to, from *[]Property) {
propertyLoop:
	for _, f := range *from {
		for i := range *to {
			t := &(*to)[i]
			if f.Name == t.Name {
				collapseDuplicateProperties(&t.Properties, &f.Properties)
				continue propertyLoop
			}
		}
		*to = append(*to, f)
	}
}

// Find all property structs that only contain structs, and move their children up one with
// a prefixed name
func collapseNestedPropertyStructs(mt *moduleType) {
	for _, ps := range mt.PropertyStructs {
		collapseNestedProperties(&ps.Properties)
	}
}

func collapseNestedProperties(p *[]Property) {
	var n []Property

	for _, parent := range *p {
		var containsProperty bool
		for j := range parent.Properties {
			child := &parent.Properties[j]
			if len(child.Properties) > 0 {
				collapseNestedProperties(&child.Properties)
			} else {
				containsProperty = true
			}
		}
		if containsProperty || len(parent.Properties) == 0 {
			n = append(n, parent)
		} else {
			for j := range parent.Properties {
				child := parent.Properties[j]
				child.Name = parent.Name + "." + child.Name
				n = append(n, child)
			}
		}
	}
	*p = n
}

func combineDuplicateProperties(mt *moduleType) {
	for _, ps := range mt.PropertyStructs {
		combineDuplicateSubProperties(&ps.Properties)
	}
}

func combineDuplicateSubProperties(p *[]Property) {
	var n []Property
propertyLoop:
	for _, child := range *p {
		if len(child.Properties) > 0 {
			combineDuplicateSubProperties(&child.Properties)
			for i := range n {
				s := &n[i]
				if s.SameSubProperties(child) {
					s.OtherNames = append(s.OtherNames, child.Name)
					s.OtherTexts = append(s.OtherTexts, child.Text)
					continue propertyLoop
				}
			}
		}
		n = append(n, child)
	}

	*p = n
}

type moduleTypeByName []*moduleType

func (l moduleTypeByName) Len() int           { return len(l) }
func (l moduleTypeByName) Less(i, j int) bool { return l[i].Name < l[j].Name }
func (l moduleTypeByName) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

type moduleType struct {
	Name            string
	Text            string
	PropertyStructs []*PropertyStruct
}

var (
	fileTemplate = `
<html>
<head>
<title>Build Docs</title>
<link rel="stylesheet" href="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.5/css/bootstrap.min.css">
<script src="https://ajax.googleapis.com/ajax/libs/jquery/2.1.4/jquery.min.js"></script>
<script src="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.5/js/bootstrap.min.js"></script>
</head>
<body>
<h1>Build Docs</h1>
<div class="panel-group" id="accordion" role="tablist" aria-multiselectable="true">
  {{range .}}
    {{ $collapseIndex := unique }}
    <div class="panel panel-default">
      <div class="panel-heading" role="tab" id="heading{{$collapseIndex}}">
        <h2 class="panel-title">
          <a class="collapsed" role="button" data-toggle="collapse" data-parent="#accordion" href="#collapse{{$collapseIndex}}" aria-expanded="false" aria-controls="collapse{{$collapseIndex}}">
             {{.Name}}
          </a>
        </h2>
      </div>
    </div>
    <div id="collapse{{$collapseIndex}}" class="panel-collapse collapse" role="tabpanel" aria-labelledby="heading{{$collapseIndex}}">
      <div class="panel-body">
        <p>{{.Text}}</p>
        {{range .PropertyStructs}}
          <p>{{.Text}}</p>
          {{template "properties" .Properties}}
        {{end}}
      </div>
    </div>
  {{end}}
</div>
</body>
</html>

{{define "properties"}}
  <div class="panel-group" id="accordion" role="tablist" aria-multiselectable="true">
    {{range .}}
      {{$collapseIndex := unique}}
      {{if .Properties}}
        <div class="panel panel-default">
          <div class="panel-heading" role="tab" id="heading{{$collapseIndex}}">
            <h4 class="panel-title">
              <a class="collapsed" role="button" data-toggle="collapse" data-parent="#accordion" href="#collapse{{$collapseIndex}}" aria-expanded="false" aria-controls="collapse{{$collapseIndex}}">
                 {{.Name}}{{range .OtherNames}}, {{.}}{{end}}
              </a>
            </h4>
          </div>
        </div>
        <div id="collapse{{$collapseIndex}}" class="panel-collapse collapse" role="tabpanel" aria-labelledby="heading{{$collapseIndex}}">
          <div class="panel-body">
            <p>{{.Text}}</p>
            {{range .OtherTexts}}<p>{{.}}</p>{{end}}
            {{template "properties" .Properties}}
          </div>
        </div>
      {{else}}
        <div>
          <h4>{{.Name}}{{range .OtherNames}}, {{.}}{{end}}</h4>
          <p>{{.Text}}</p>
          {{range .OtherTexts}}<p>{{.}}</p>{{end}}
          <p><i>Type: {{.Type}}</i></p>
          {{if .Default}}<p><i>Default: {{.Default}}</i></p>{{end}}
        </div>
      {{end}}
    {{end}}
  </div>
{{end}}
`
)
