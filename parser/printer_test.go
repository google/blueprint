// Copyright 2014 Google Inc. All rights reserved.
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

package parser

import (
	"bytes"
	"testing"
)

var validPrinterTestCases = []struct {
	input  string
	output string
}{
	{
		input: `
foo {}
`,
		output: `
foo {}
`,
	},
	{
		input: `
foo{name= "abc",}
`,
		output: `
foo {
    name: "abc",
}
`,
	},
	{
		input: `
			foo {
				stuff: ["asdf", "jkl;", "qwert",
					"uiop", "bnm,"]
			}
			`,
		output: `
foo {
    stuff: [
        "asdf",
        "bnm,",
        "jkl;",
        "qwert",
        "uiop",
    ],
}
`,
	},
	{
		input: `
		        var = "asdf"
			foo {
				stuff: ["asdf"] + var,
			}`,
		output: `
var = "asdf"
foo {
    stuff: ["asdf"] + var,
}
`,
	},
	{
		input: `
		        var = "asdf"
			foo {
				stuff: [
				    "asdf"
				] + var,
			}`,
		output: `
var = "asdf"
foo {
    stuff: [
        "asdf",
    ] + var,
}
`,
	},
	{
		input: `
		        var = "asdf"
			foo {
				stuff: ["asdf"] + var + ["qwert"],
			}`,
		output: `
var = "asdf"
foo {
    stuff: ["asdf"] + var + ["qwert"],
}
`,
	},
	{
		input: `
		foo {
			stuff: {
				isGood: true,
				name: "bar"
			}
		}
		`,
		output: `
foo {
    stuff: {
        isGood: true,
        name: "bar",
    },
}
`,
	},
	{
		input: `
// comment1
foo {
	// comment2
	isGood: true,  // comment3
}
`,
		output: `
// comment1
foo {
    // comment2
    isGood: true, // comment3
}
`,
	},
	{
		input: `
foo {
	name: "abc",
}

bar  {
	name: "def",
}
		`,
		output: `
foo {
    name: "abc",
}

bar {
    name: "def",
}
`,
	},
	{
		input: `
foo = "stuff"
bar = foo
baz = foo + bar
baz += foo
`,
		output: `
foo = "stuff"
bar = foo
baz = foo + bar
baz += foo
`,
	},
	{
		input: `
//test
test /* test */ {
    srcs: [
        /*"bootstrap/bootstrap.go",
    "bootstrap/cleanup.go",*/
        "bootstrap/command.go",
        "bootstrap/doc.go", //doc.go
        "bootstrap/config.go", //config.go
    ],
    deps: ["libabc"],
    incs: []
} //test
//test
test2 {
}


//test3
`,
		output: `
//test
test /* test */ {
    srcs: [
        /*"bootstrap/bootstrap.go",
        "bootstrap/cleanup.go",*/
        "bootstrap/command.go",
        "bootstrap/config.go", //config.go
        "bootstrap/doc.go", //doc.go
    ],
    deps: ["libabc"],
    incs: [],
} //test
//test

test2 {
}

//test3
`,
	},
	{
		input: `
// test
module // test

 {
    srcs
   : [
        "src1.c",
        "src2.c",
    ],
//test
}
//test2
`,
		output: `
// test
module { // test

    srcs: [
        "src1.c",
        "src2.c",
    ],
    //test
}

//test2
`,
	},
	{
		input: `
/*test {
    test: true,
}*/

test {
/*test: true,*/
}

// This
/* Is *//* A */ // A
// A

// Multiline
// Comment

test {}

// This
/* Is */
// A
// Trailing

// Multiline
// Comment
`,
		output: `
/*test {
    test: true,
}*/

test {
    /*test: true,*/
}

// This
/* Is */ /* A */ // A
// A

// Multiline
// Comment

test {}

// This
/* Is */
// A
// Trailing

// Multiline
// Comment
`,
	},
	{
		input: `
test // test

// test
{
}
`,
		output: `
test { // test

// test

}
`,
	},
}

func TestPrinter(t *testing.T) {
	for i, testCase := range validPrinterTestCases {

		description := testCase.description

		in := testCase.input[:]
		if in[0] == '\n' {
			in = in[1:]
		} else {
			panic(fmt.Sprintf("A newline at the beginning of testcase input is required, to make the test code easily readable. Offending test case '%s' starts with character: '%s'", description, string(in[0])))
		}

		expected := testCase.output[:]
		if expected[0] == '\n' {
			expected = expected[1:]
		} else {
			panic(fmt.Sprintf("A newline at the beginning of testcase expected output is required, to make the test code easily readable. Offending test case '%v' starts with character: '%s'", description, string(expected[0])))
		}

		r := bytes.NewBufferString(in)
		parsed, errs := Parse(fmt.Sprintf("testcase '%s' (printer_test#%v) ", description, i), r, NewScope(nil), true, false)
		if len(errs) != 0 {
			t.Errorf("input: %s", in)
			t.Error("unexpected errors:")
			for _, err := range errs {
				t.Errorf("  %s", err)
			}
			t.FailNow()
		}

		got := string(PrintTree(parsed.SyntaxTree))

		if got != expected {

			// do a more-explanatory, less pretty print of the parse for debugging
			var treeRepresentation = NewVerbosePrinter(parsed.SyntaxTree).Print()

			var diff = fmt.Sprintf("got.len() = %v whereas expected.len() = %v", len(got), len(expected))
			var line = 0
			var column = 0
			for i := 0; i < int(math.Min(float64(len(got)), float64(len(expected)))); i++ {
				if got[i] != expected[i] {
					diff = fmt.Sprintf("got %#v instead of %#v (at %v, %v)\nMatching portions of strings before mismatch: '%v'",
						string(got[i]), string(expected[i]), line, column, got[:i])
					break
				}
				if got[i] == '\n' {
					line++
					column = 0
				} else {
					column++
				}
			}
			t.Errorf(
				"\ntest input: \n"+
					"%s\n"+
					"parsed: \n%s\n"+
					"expected: \n%s\n"+
					"got     : \n%s\n"+
					"diff    : %v\n",
				in, treeRepresentation, expected, got, diff)

		}
	}
}
