package bpdoc

import (
	"fmt"
	"reflect"
	"testing"
)

type propInfo struct {
	name string
	typ  string
}

type parentProps struct {
	A string

	Child *childProps

	Mutated *mutatedProps `blueprint:"mutated"`
}

type childProps struct {
	B int

	Child *grandchildProps
}

type grandchildProps struct {
	C bool
}

type mutatedProps struct {
	D int
}

func TestNestedPropertyStructs(t *testing.T) {
	parent := parentProps{Child: &childProps{Child: &grandchildProps{}}, Mutated: &mutatedProps{}}

	allStructs := nestedPropertyStructs(reflect.ValueOf(parent))

	// mutated shouldn't be found because it's a mutated property.
	expected := []string{"child", "child.child"}
	if len(allStructs) != len(expected) {
		t.Fatalf("expected %d structs, got %d, all entries: %v",
			len(expected), len(allStructs), allStructs)
	}
	got := []string{}
	for _, s := range allStructs {
		got = append(got, s.nestPoint)
	}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("Expected nested properties:\n\t %q,\n but got\n\t %q", expected, got)
	}
}

func TestAllPackages(t *testing.T) {
	packages, err := AllPackages(pkgFiles, moduleTypeNameFactories, moduleTypeNamePropertyStructs)
	if err != nil {
		t.Fatalf("expected nil error for AllPackages(%v, %v, %v), got %s", pkgFiles, moduleTypeNameFactories, moduleTypeNamePropertyStructs, err)
	}

	if numPackages := len(packages); numPackages != 1 {
		t.Errorf("Expected %d package, got %d packages %v instead", len(pkgFiles), numPackages, packages)
	}

	pkg := packages[0]

	expectedProps := map[string][]propInfo{
		"bar": []propInfo{
			propInfo{
				name: "a",
				typ:  "string",
			},
			propInfo{
				name: "nested",
				typ:  "",
			},
			propInfo{
				name: "nested.c",
				typ:  "string",
			},
			propInfo{
				name: "nested_struct",
				typ:  "structToNest",
			},
			propInfo{
				name: "nested_struct.e",
				typ:  "string",
			},
			propInfo{
				name: "struct_has_embed",
				typ:  "StructWithEmbedded",
			},
			propInfo{
				name: "struct_has_embed.nested_in_embedded",
				typ:  "structToNest",
			},
			propInfo{
				name: "struct_has_embed.nested_in_embedded.e",
				typ:  "string",
			},
			propInfo{
				name: "struct_has_embed.f",
				typ:  "string",
			},
			propInfo{
				name: "list_of_ints",
				typ:  "list of int",
			},
			propInfo{
				name: "list_of_nested",
				typ:  "list of structToNest",
			},
			propInfo{
				name: "nested_in_other_embedded",
				typ:  "otherStructToNest",
			},
			propInfo{
				name: "nested_in_other_embedded.g",
				typ:  "string",
			},
			propInfo{
				name: "h",
				typ:  "string",
			},
		},
		"foo": []propInfo{
			propInfo{
				name: "a",
				typ:  "string",
			},
		},
	}

	for _, m := range pkg.ModuleTypes {
		foundProps := []propInfo{}

		for _, p := range m.PropertyStructs {
			nestedProps, errs := findAllProperties("", p.Properties)
			foundProps = append(foundProps, nestedProps...)
			for _, err := range errs {
				t.Errorf("%s", err)
			}
		}
		if wanted, ok := expectedProps[m.Name]; ok {
			if !reflect.DeepEqual(foundProps, wanted) {
				t.Errorf("For %s, expected\n\t %q,\nbut got\n\t %q", m.Name, wanted, foundProps)
			}
		}
	}
}

func findAllProperties(prefix string, properties []Property) ([]propInfo, []error) {
	foundProps := []propInfo{}
	errs := []error{}
	for _, p := range properties {
		prop := propInfo{
			name: prefix + p.Name,
			typ:  p.Type,
		}
		foundProps = append(foundProps, prop)
		if hasTag(p.Tag, "blueprint", "mutated") {
			err := fmt.Errorf("Property %s has `blueprint:\"mutated\" tag but should have been excluded.", p.Name)
			errs = append(errs, err)
		}

		nestedProps, nestedErrs := findAllProperties(prefix+p.Name+".", p.Properties)
		foundProps = append(foundProps, nestedProps...)
		errs = append(errs, nestedErrs...)
	}
	return foundProps, errs
}
