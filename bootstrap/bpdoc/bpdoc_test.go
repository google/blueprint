package bpdoc

import (
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
		t.Fatalf("expected no errors, got %s", err)
	}

	if numPackages := len(packages); numPackages != 1 {
		t.Fatalf("Expected 1 package, got %q", numPackages)
	}

	p := packages[0]

	for _, m := range p.ModuleTypes {
		for _, p := range m.PropertyStructs {
			noMutatedProperties(t, p.Properties)
		}
	}

}

func noMutatedProperties(t *testing.T, properties []Property) {
	t.Helper()

	for _, p := range properties {
		if hasTag(p.Tag, "blueprint", "mutated") {
			t.Errorf("Property %s has `blueprint:\"mutated\" tag but should have been excluded.", p)
		}
		noMutatedProperties(t, p.Properties)
	}
}
