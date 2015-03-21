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
/* Is */
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
/* Is */
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
}

func TestPrinter(t *testing.T) {
	for _, testCase := range validPrinterTestCases {
		in := testCase.input[1:]
		expected := testCase.output[1:]

		r := bytes.NewBufferString(in)
		file, errs := Parse("", r, NewScope(nil))
		if len(errs) != 0 {
			t.Errorf("test case: %s", in)
			t.Errorf("unexpected errors:")
			for _, err := range errs {
				t.Errorf("  %s", err)
			}
			t.FailNow()
		}

		SortLists(file)

		got, err := Print(file)
		if err != nil {
			t.Errorf("test case: %s", in)
			t.Errorf("unexpected error: %s", err)
			t.FailNow()
		}

		if string(got) != expected {
			t.Errorf("test case: %s", in)
			t.Errorf("  expected: %s", expected)
			t.Errorf("       got: %s", string(got))
		}
	}
}
