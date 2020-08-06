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
	"html/template"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/blueprint/proptools"
)

//
// Utility functions for PropertyStruct and Property
//

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
				Text:       formatText(text),
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
		if hasTag(x.Tag, key, value) == !exclude {
			filterPropsByTag(&x.Properties, key, value, exclude)
			filtered = append(filtered, x)
		}
	}

	*props = filtered
}

func hasTag(tag reflect.StructTag, key, value string) bool {
	for _, entry := range strings.Split(tag.Get(key), ",") {
		if entry == value {
			return true
		}
	}
	return false
}

func formatText(text string) template.HTML {
	var html template.HTML
	lines := strings.Split(text, "\n")
	preformatted := false
	for _, line := range lines {
		r, _ := utf8.DecodeRuneInString(line)
		indent := unicode.IsSpace(r)
		if indent && !preformatted {
			html += "<pre>\n\n"
			preformatted = true
		} else if !indent && line != "" && preformatted {
			html += "</pre>\n"
			preformatted = false
		}
		html += template.HTML(template.HTMLEscapeString(line)) + "\n"
	}
	if preformatted {
		html += "</pre>\n"
	}
	return html
}
