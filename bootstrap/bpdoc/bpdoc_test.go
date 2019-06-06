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
