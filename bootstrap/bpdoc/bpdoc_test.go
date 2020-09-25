package bpdoc

import (
	"fmt"
	"reflect"
	"testing"
)

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

	expectedProps := map[string][]string{
		"bar": []string{
			"a",
			"nested",
			"nested.c",
			"nested_struct",
			"nested_struct.e",
			"struct_has_embed",
			"struct_has_embed.nested_in_embedded",
			"struct_has_embed.nested_in_embedded.e",
			"struct_has_embed.f",
			"nested_in_other_embedded",
			"nested_in_other_embedded.g",
			"h",
		},
		"foo": []string{
			"a",
		},
	}

	for _, m := range pkg.ModuleTypes {
		foundProps := []string{}
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

func findAllProperties(prefix string, properties []Property) ([]string, []error) {
	foundProps := []string{}
	errs := []error{}
	for _, p := range properties {
		foundProps = append(foundProps, prefix+p.Name)
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
