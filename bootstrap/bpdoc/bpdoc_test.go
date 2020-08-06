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
		t.Errorf("expected %d structs, got %d, all entries: %q",
			len(expected), len(allStructs), allStructs)
	}
	for _, e := range expected {
		if _, ok := allStructs[e]; !ok {
			t.Errorf("missing entry %q, all entries: %q", e, allStructs)
		}
	}
}

func TestAllPackages(t *testing.T) {
	packages, err := AllPackages(pkgFiles, moduleTypeNameFactories, moduleTypeNamePropertyStructs)
	if err != nil {
		t.Errorf("expected nil error for AllPackages(%v, %v, %v), got %s", pkgFiles, moduleTypeNameFactories, moduleTypeNamePropertyStructs, err)
	}

	if numPackages := len(packages); numPackages != 1 {
		t.Errorf("Expected %d package, got %d packages %v instead", len(pkgFiles), numPackages, packages)
	}

	pkg := packages[0]

	for _, m := range pkg.ModuleTypes {
		for _, p := range m.PropertyStructs {
			for _, err := range noMutatedProperties(p.Properties) {
				t.Errorf("%s", err)
			}
		}
	}
}

func noMutatedProperties(properties []Property) []error {
	errs := []error{}
	for _, p := range properties {
		if hasTag(p.Tag, "blueprint", "mutated") {
			err := fmt.Errorf("Property %s has `blueprint:\"mutated\" tag but should have been excluded.", p)
			errs = append(errs, err)
		}

		errs = append(errs, noMutatedProperties(p.Properties)...)
	}
	return errs
}
