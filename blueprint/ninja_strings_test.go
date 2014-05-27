package blueprint

import (
	"reflect"
	"testing"
)

var ninjaParseTestCases = []struct {
	input  string
	output []string
	err    string
}{
	{
		input:  "abc def $ghi jkl",
		output: []string{"ghi"},
	},
	{
		input:  "abc def $ghi$jkl",
		output: []string{"ghi", "jkl"},
	},
	{
		input:  "foo $012_-345xyz_! bar",
		output: []string{"012_-345xyz_"},
	},
	{
		input:  "foo ${012_-345xyz_} bar",
		output: []string{"012_-345xyz_"},
	},
	{
		input:  "foo ${012_-345xyz_} bar",
		output: []string{"012_-345xyz_"},
	},
	{
		input:  "foo $$ bar",
		output: nil,
	},
	{
		input: "foo $ bar",
		err:   "invalid character after '$' at byte offset 5",
	},
	{
		input: "foo $",
		err:   "unexpected end of string after '$'",
	},
	{
		input: "foo ${} bar",
		err:   "empty variable name at byte offset 6",
	},
	{
		input: "foo ${abc!} bar",
		err:   "invalid character in variable name at byte offset 9",
	},
	{
		input: "foo ${abc",
		err:   "unexpected end of string in variable name",
	},
}

func TestParseNinjaString(t *testing.T) {
	for _, testCase := range ninjaParseTestCases {
		scope := newLocalScope(nil, "namespace")
		var expectedVars []Variable
		for _, varName := range testCase.output {
			v, err := scope.LookupVariable(varName)
			if err != nil {
				v, err = scope.AddLocalVariable(varName, "")
				if err != nil {
					t.Fatalf("error creating scope: %s", err)
				}
			}
			expectedVars = append(expectedVars, v)
		}

		output, err := parseNinjaString(scope, testCase.input)
		if err == nil && !reflect.DeepEqual(output.variables, expectedVars) {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %q", testCase.input)
			t.Errorf("  expected: %#v", testCase.output)
			t.Errorf("       got: %#v", output)
		}
		var errStr string
		if err != nil {
			errStr = err.Error()
		}
		if err != nil && err.Error() != testCase.err {
			t.Errorf("unexpected error:")
			t.Errorf("     input: %q", testCase.input)
			t.Errorf("  expected: %q", testCase.err)
			t.Errorf("       got: %q", errStr)
		}
	}
}

func TestParseNinjaStringWithImportedVar(t *testing.T) {
	ImpVar := &staticVariable{name_: "ImpVar"}
	impScope := newScope(nil)
	impScope.AddVariable(ImpVar)
	scope := newScope(nil)
	scope.AddImport("impPkg", impScope)

	input := "abc def ${impPkg.ImpVar} ghi"
	output, err := parseNinjaString(scope, input)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	expect := []Variable{ImpVar}
	if !reflect.DeepEqual(output.variables, expect) {
		t.Errorf("incorrect output:")
		t.Errorf("     input: %q", input)
		t.Errorf("  expected: %#v", expect)
		t.Errorf("       got: %#v", output)
	}
}
