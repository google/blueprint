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
	"fmt"
	"math"
	"testing"
)

// printer_test.go is allowed to use the parser in its tests because the parser is tested separately (and doesn't use the printer in its tests)

var validPrinterTestCases = []struct {
	description string
	input       string
	output      string
}{
	{
		description: "empty map",
		input: `
foo {}
`,
		output: `
foo {}
`,
	},
	{
		description: "map with '='",
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
		description: "multikey map on one line",
		input: `
IAmConcise{name:"concise",style:"short"}`,
		output: `
IAmConcise {
    name: "concise",
    style: "short",
}
`,
	},
	{
		description: "multiline list",
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
        "jkl;",
        "qwert",
        "uiop",
        "bnm,",
    ],
}
`,
	},
	{
		description: "singleline list with multiple elements",
		input: `
			foo {
				stuff: ["asdf", "jkl;", "qwert", "uiop", "bnm,"]
			}
			`,
		output: `
foo {
    stuff: [
        "asdf",
        "jkl;",
        "qwert",
        "uiop",
        "bnm,",
    ],
}
`,
	},
	{
		description: "singleline list with one element",
		input: `
foo {
    stuff: ["asdf"],
}
`,
		output: `
foo {
    stuff: ["asdf"],
}
`,
	},
	{
		description: "variable assignment",
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
		description: "variable assignment and multiline list",
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
		description: "concatenating lists",
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
		description: "nested structs",
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
		description: "module with trailing comment",
		input: `
fdsa {
} // I'm a comment
`,
		output: `
fdsa {
} // I'm a comment
`,
	},
	{
		description: "commented struct",
		input: `
// comment1
foo /* inline */ {
	// comment2
	isGood: true,  // comment3
	// comment4
	isGood: true,  // comment5
	// comment6
}
`,
		output: `
// comment1
foo /* inline */ {
    // comment2
    isGood: true, // comment3
    // comment4
    isGood: true, // comment5
    // comment6
}
`,
	},
	{
		description: "two structs separated by a space",
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
		description: "several variable assignments",
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
		description: "list with inline comments",
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
        "bootstrap/doc.go", //doc.go
        "bootstrap/config.go", //config.go
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
		description: "extra newlines (1)",
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
		description: "many comments",
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
/* is */
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
/* is */
// A
// Trailing

// Multiline
// Comment
`,
	},
	{
		description: "comments between module type and map body",
		input: `
test // test2

// test3
{
}
`,
		output: `
test { // test2

    // test3

}
`,
	},
	{
		description: "comments separated by multiple spaces",
		input: `
/* if there are multiple spaces between two inline comments */  /* then they are reduced to one space */
`,
		output: `
/* if there are multiple spaces between two inline comments */ /* then they are reduced to one space */
`,
	},
	{
		description: "comment inside a property",
		input: `
smallModule {
    stringProp: /* don't forget me */ "stringVal"
}
`,
		output: `
smallModule {
    stringProp: /* don't forget me */ "stringVal",
}
`,
	},
	{
		description: "extra newlines (2)",
		input: `
// two blank lines after a comment turn into one


// three blank lines after a comment turn into one



// two consecutive comments remain adjacent
// two comments next to each other remain adjacent

myModule {

    // a blank line before a property remains as a blank line

    myProperty: "myValue",

    // a blank line after a property remains as a blank line


    propertyTwo: [ // A blank line remains before the first property

        // End of blank line
        "a",

        // A blank line remains before the second property
        "b",

        "c", // Two blank lines within a list turn into one


        "d",  // Two blank lines after the last property turn into one


    ],
}
`,
		output: `
// two blank lines after a comment turn into one

// three blank lines after a comment turn into one

// two consecutive comments remain adjacent
// two comments next to each other remain adjacent

myModule {

    // a blank line before a property remains as a blank line

    myProperty: "myValue",

    // a blank line after a property remains as a blank line

    propertyTwo: [ // A blank line remains before the first property

        // End of blank line
        "a",

        // A blank line remains before the second property
        "b",

        "c", // Two blank lines within a list turn into one

        "d", // Two blank lines after the last property turn into one

    ],
}
`,
	},
	{
		description: "newlines without comments",
		input: `
		cc_test {
		    name: "linker-unit-tests",

		    cflags: [
			"-g",
			"-Wall",
			"-Wextra",
			"-Wunused",
			"-Werror",
		    ],
		    local_include_dirs: ["../../libc/"],

		    srcs: ["linker_block_allocator_test.cpp"],

		}`,
		output: `
cc_test {
    name: "linker-unit-tests",

    cflags: [
        "-g",
        "-Wall",
        "-Wextra",
        "-Wunused",
        "-Werror",
    ],
    local_include_dirs: ["../../libc/"],

    srcs: ["linker_block_allocator_test.cpp"],

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
		parsed, errs := Parse(fmt.Sprintf("testcase '%s' (printer_test#%v) ", description, i), r, NewScope(nil))
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
