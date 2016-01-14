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

type DocCollector struct {
	pkgFiles map[string][]string // Map of package name to source files, provided by constructor

	mutex   sync.Mutex
	pkgDocs map[string]*doc.Package        // Map of package name to parsed Go AST, protected by mutex
	docs    map[string]*PropertyStructDocs // Map of type name to docs, protected by mutex
}

func NewDocCollector(pkgFiles map[string][]string) *DocCollector {
	return &DocCollector{
		pkgFiles: pkgFiles,
		pkgDocs:  make(map[string]*doc.Package),
		docs:     make(map[string]*PropertyStructDocs),
	}
}

// Return the PropertyStructDocs associated with a property struct type.  The type should be in the
// format <package path>.<type name>
func (dc *DocCollector) Docs(pkg, name string, defaults reflect.Value) (*PropertyStructDocs, error) {
	docs := dc.getDocs(pkg, name)

	if docs == nil {
		pkgDocs, err := dc.packageDocs(pkg)
		if err != nil {
			return nil, err
		}

		for _, t := range pkgDocs.Types {
			if t.Name == name {
				docs, err = newDocs(t)
				if err != nil {
					return nil, err
				}
				docs = dc.putDocs(pkg, name, docs)
			}
		}
	}

	if docs == nil {
		return nil, fmt.Errorf("package %q type %q not found", pkg, name)
	}

	docs = docs.Clone()
	docs.SetDefaults(defaults)

	return docs, nil
}

func (dc *DocCollector) getDocs(pkg, name string) *PropertyStructDocs {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()

	name = pkg + "." + name

	return dc.docs[name]
}

func (dc *DocCollector) putDocs(pkg, name string, docs *PropertyStructDocs) *PropertyStructDocs {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()

	name = pkg + "." + name

	if dc.docs[name] != nil {
		return dc.docs[name]
	} else {
		dc.docs[name] = docs
		return docs
	}
}

type PropertyStructDocs struct {
	Name       string
	Text       string
	Properties []PropertyDocs
}

type PropertyDocs struct {
	Name       string
	OtherNames []string
	Type       string
	Tag        reflect.StructTag
	Text       string
	OtherTexts []string
	Properties []PropertyDocs
	Default    string
}

func (docs *PropertyStructDocs) Clone() *PropertyStructDocs {
	ret := *docs
	ret.Properties = append([]PropertyDocs(nil), ret.Properties...)
	for i, prop := range ret.Properties {
		ret.Properties[i] = prop.Clone()
	}

	return &ret
}

func (docs *PropertyDocs) Clone() PropertyDocs {
	ret := *docs
	ret.Properties = append([]PropertyDocs(nil), ret.Properties...)
	for i, prop := range ret.Properties {
		ret.Properties[i] = prop.Clone()
	}

	return ret
}

func (docs *PropertyDocs) Equal(other PropertyDocs) bool {
	return docs.Name == other.Name && docs.Type == other.Type && docs.Tag == other.Tag &&
		docs.Text == other.Text && docs.Default == other.Default &&
		stringArrayEqual(docs.OtherNames, other.OtherNames) &&
		stringArrayEqual(docs.OtherTexts, other.OtherTexts) &&
		docs.SameSubProperties(other)
}

func (docs *PropertyStructDocs) SetDefaults(defaults reflect.Value) {
	setDefaults(docs.Properties, defaults)
}

func setDefaults(properties []PropertyDocs, defaults reflect.Value) {
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

		if f.Type().Kind() == reflect.Interface {
			f = f.Elem()
		}

		if f.Type().Kind() == reflect.Ptr {
			f = f.Elem()
		}

		if f.Type().Kind() == reflect.Struct {
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

func (docs *PropertyDocs) SameSubProperties(other PropertyDocs) bool {
	if len(docs.Properties) != len(other.Properties) {
		return false
	}

	for i := range docs.Properties {
		if !docs.Properties[i].Equal(other.Properties[i]) {
			return false
		}
	}

	return true
}

func (docs *PropertyStructDocs) GetByName(name string) *PropertyDocs {
	return getByName(name, "", &docs.Properties)
}

func getByName(name string, prefix string, props *[]PropertyDocs) *PropertyDocs {
	for i := range *props {
		if prefix+(*props)[i].Name == name {
			return &(*props)[i]
		} else if strings.HasPrefix(name, prefix+(*props)[i].Name+".") {
			return getByName(name, prefix+(*props)[i].Name+".", &(*props)[i].Properties)
		}
	}
	return nil
}

func (prop *PropertyDocs) Nest(nested *PropertyStructDocs) {
	//prop.Name += "(" + nested.Name + ")"
	//prop.Text += "(" + nested.Text + ")"
	prop.Properties = append(prop.Properties, nested.Properties...)
}

func newDocs(t *doc.Type) (*PropertyStructDocs, error) {
	typeSpec := t.Decl.Specs[0].(*ast.TypeSpec)
	docs := PropertyStructDocs{
		Name: t.Name,
		Text: t.Doc,
	}

	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		return nil, fmt.Errorf("type of %q is not a struct", t.Name)
	}

	var err error
	docs.Properties, err = structProperties(structType)
	if err != nil {
		return nil, err
	}

	return &docs, nil
}

func structProperties(structType *ast.StructType) (props []PropertyDocs, err error) {
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
			var innerProps []PropertyDocs
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
			switch a := f.Type.(type) {
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

			props = append(props, PropertyDocs{
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

func (docs *PropertyStructDocs) ExcludeByTag(key, value string) {
	filterPropsByTag(&docs.Properties, key, value, true)
}

func (docs *PropertyStructDocs) IncludeByTag(key, value string) {
	filterPropsByTag(&docs.Properties, key, value, false)
}

func filterPropsByTag(props *[]PropertyDocs, key, value string, exclude bool) {
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
func (dc *DocCollector) packageDocs(pkg string) (*doc.Package, error) {
	pkgDocs := dc.getPackageDocs(pkg)
	if pkgDocs == nil {
		if files, ok := dc.pkgFiles[pkg]; ok {
			var err error
			pkgAST, err := NewPackageAST(files)
			if err != nil {
				return nil, err
			}
			pkgDocs = doc.New(pkgAST, pkg, doc.AllDecls)
			pkgDocs = dc.putPackageDocs(pkg, pkgDocs)
		} else {
			return nil, fmt.Errorf("unknown package %q", pkg)
		}
	}
	return pkgDocs, nil
}

func (dc *DocCollector) getPackageDocs(pkg string) *doc.Package {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()

	return dc.pkgDocs[pkg]
}

func (dc *DocCollector) putPackageDocs(pkg string, pkgDocs *doc.Package) *doc.Package {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()

	if dc.pkgDocs[pkg] != nil {
		return dc.pkgDocs[pkg]
	} else {
		dc.pkgDocs[pkg] = pkgDocs
		return pkgDocs
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

	docSet := NewDocCollector(pkgFiles)

	var moduleTypeList []*moduleTypeDoc
	for moduleType, propertyStructs := range moduleTypePropertyStructs {
		mtDoc, err := getModuleTypeDoc(docSet, moduleType, propertyStructs)
		if err != nil {
			return err
		}
		removeEmptyPropertyStructs(mtDoc)
		collapseDuplicatePropertyStructs(mtDoc)
		collapseNestedPropertyStructs(mtDoc)
		combineDuplicateProperties(mtDoc)
		moduleTypeList = append(moduleTypeList, mtDoc)
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

func getModuleTypeDoc(docSet *DocCollector, moduleType string,
	propertyStructs []interface{}) (*moduleTypeDoc, error) {
	mtDoc := &moduleTypeDoc{
		Name: moduleType,
		//Text: docSet.ModuleTypeDocs(moduleType),
	}

	for _, s := range propertyStructs {
		v := reflect.ValueOf(s).Elem()
		t := v.Type()

		// Ignore property structs with unexported or unnamed types
		if t.PkgPath() == "" {
			continue
		}
		psDoc, err := docSet.Docs(t.PkgPath(), t.Name(), v)
		if err != nil {
			return nil, err
		}
		psDoc.ExcludeByTag("blueprint", "mutated")

		for nested, nestedValue := range nestedPropertyStructs(v) {
			nestedType := nestedValue.Type()

			// Ignore property structs with unexported or unnamed types
			if nestedType.PkgPath() == "" {
				continue
			}
			nestedDoc, err := docSet.Docs(nestedType.PkgPath(), nestedType.Name(), nestedValue)
			if err != nil {
				return nil, err
			}
			nestedDoc.ExcludeByTag("blueprint", "mutated")
			nestPoint := psDoc.GetByName(nested)
			if nestPoint == nil {
				return nil, fmt.Errorf("nesting point %q not found", nested)
			}

			key, value, err := blueprint.HasFilter(nestPoint.Tag)
			if err != nil {
				return nil, err
			}
			if key != "" {
				nestedDoc.IncludeByTag(key, value)
			}

			nestPoint.Nest(nestedDoc)
		}
		mtDoc.PropertyStructs = append(mtDoc.PropertyStructs, psDoc)
	}

	return mtDoc, nil
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
func removeEmptyPropertyStructs(mtDoc *moduleTypeDoc) {
	for i := 0; i < len(mtDoc.PropertyStructs); i++ {
		if len(mtDoc.PropertyStructs[i].Properties) == 0 {
			mtDoc.PropertyStructs = append(mtDoc.PropertyStructs[:i], mtDoc.PropertyStructs[i+1:]...)
			i--
		}
	}
}

// Squashes duplicates of the same property struct into single entries
func collapseDuplicatePropertyStructs(mtDoc *moduleTypeDoc) {
	var collapsedDocs []*PropertyStructDocs

propertyStructLoop:
	for _, from := range mtDoc.PropertyStructs {
		for _, to := range collapsedDocs {
			if from.Name == to.Name {
				collapseDuplicateProperties(&to.Properties, &from.Properties)
				continue propertyStructLoop
			}
		}
		collapsedDocs = append(collapsedDocs, from)
	}
	mtDoc.PropertyStructs = collapsedDocs
}

func collapseDuplicateProperties(to, from *[]PropertyDocs) {
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
func collapseNestedPropertyStructs(mtDoc *moduleTypeDoc) {
	for _, ps := range mtDoc.PropertyStructs {
		collapseNestedProperties(&ps.Properties)
	}
}

func collapseNestedProperties(p *[]PropertyDocs) {
	var n []PropertyDocs

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

func combineDuplicateProperties(mtDoc *moduleTypeDoc) {
	for _, ps := range mtDoc.PropertyStructs {
		combineDuplicateSubProperties(&ps.Properties)
	}
}

func combineDuplicateSubProperties(p *[]PropertyDocs) {
	var n []PropertyDocs
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

type moduleTypeByName []*moduleTypeDoc

func (l moduleTypeByName) Len() int           { return len(l) }
func (l moduleTypeByName) Less(i, j int) bool { return l[i].Name < l[j].Name }
func (l moduleTypeByName) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

type moduleTypeDoc struct {
	Name            string
	Text            string
	PropertyStructs []*PropertyStructDocs
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
