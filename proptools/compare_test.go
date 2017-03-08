// Copyright 2017 Google Inc. All rights reserved.
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

package proptools

import (
	"fmt"
	"strings"
	"testing"
)

type testNode struct {
	Left        *testNode
	Right       *testNode
	Text        string
	privateText string
	privateMap  map[string]string
}

var testCases = []struct {
	a          interface{}
	b          interface{}
	difference string
}{
	// strings
	{
		"",
		"",
		"",
	},
	{
		"one",
		"two",
		`a = "one" whereas b = "two"`,
	},
	// ints
	{
		1,
		1,
		"",
	},
	{
		1,
		2,
		"a = 1 whereas b = 2",
	},
	// bools
	{
		true,
		true,
		"",
	},
	{
		true,
		false,
		"a = true whereas b = false",
	},
	// different types
	{
		"three",
		3,
		"a is of type string whereas b is of type int",
	},
	{
		"something",
		nil,
		"a is of type string whereas b is of type nil",
	},
	// arrays
	{
		[1]int{1},
		[1]int{1},
		"",
	},
	{
		[1]int{1},
		[1]int{2},
		"a[0] = 1 whereas b[0] = 2",
	},
	{
		[1]int{1},
		[2]int{1, 2},
		"a is of type [1]int whereas b is of type [2]int",
	},
	// slices
	{
		[]int{1},
		[]int{1},
		"",
	},
	{
		[]int{1},
		[]int{2},
		"a[0] = 1 whereas b[0] = 2",
	},
	{
		[]int{1},
		[]int{1, 2},
		"a.len() = 1 whereas b.len() = 2.\n\nFirst differing item: b[1] = 2",
	},
	// maps
	{
		map[int]string{0: "zero"},
		map[int]string{0: "zero"},
		"",
	},
	{
		map[int]string{0: "zero", 1: "one"},
		map[int]string{0: "zero"},
		`a contains key 1 (1, corresponding value = "one") but b does not.
a contains 2 keys and b contains 1 keys.`,
	},
	{
		map[int]string{0: "zero", 1: "one"},
		map[int]string{0: "zero", 1: "not one"},
		`a[1 (int)] = "one" whereas b[1 (int)] = "not one"`,
	},
	{
		// a map with several differences, for confirming determinism
		map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6, 7: 7, 8: 8, 9: 9},
		map[int]int{0: 1, 1: 2, 2: 3, 3: 4, 4: 5, 5: 6, 6: 7, 7: 8, 8: 9, 9: 0},
		`a[0 (int)] = 0 whereas b[0 (int)] = 1`,
	},
	// structs
	{
		testNode{Text: "hi"},
		testNode{Text: "hi"},
		"",
	},
	{
		testNode{Text: "text1"},
		testNode{Text: "text2"},
		`a.Text = "text1" whereas b.Text = "text2"`,
	},
	// private fields
	{
		testNode{privateText: "shhh"},
		testNode{privateText: "shhh"},
		"",
	},
	{
		testNode{privateText: "hi"},
		testNode{privateText: "bye"},
		`a.privateText = "hi" whereas b.privateText = "bye"`,
	},
	{
		testNode{privateMap: map[string]string{"a": "b"}},
		testNode{privateMap: map[string]string{"a": "c"}},
		`a.privateMap[a (string)] = "b" whereas b.privateMap[a (string)] = "c"`,
	},
	// pointers
	{
		&testNode{Text: "hello"},
		&testNode{Text: "hello"},
		"",
	},
	{
		&testNode{Text: "hello"},
		testNode{Text: "hello"},
		"a is of type *proptools.testNode whereas b is of type proptools.testNode",
	},
	{
		&testNode{Text: "hello"},
		&testNode{Text: "goodbye"},
		"a.Text = \"hello\" whereas b.Text = \"goodbye\"",
	},
	// nested structs
	{
		testNode{Left: &testNode{Text: "first"}, Right: &testNode{Text: "last"}},
		testNode{Left: &testNode{Text: "first"}, Right: &testNode{Text: "last"}},
		"",
	},
	{
		testNode{Left: &testNode{Text: "first"}, Right: &testNode{Text: "last"}},
		testNode{Left: &testNode{Text: "first"}, Right: &testNode{Text: "middle"}},
		`a.Right.Text = "last" whereas b.Right.Text = "middle"`,
	},
	{
		testNode{Left: &testNode{Text: "first"}, Right: &testNode{Text: "last"}},
		testNode{Left: &testNode{Text: "first"}, Right: nil},

		`b.Right is nil whereas a.Right = &proptools.testNode{Left:(*proptools.testNode)(nil), Right:(*proptools.testNode)(nil), Text:"last", privateText:"", privateMap:map[string]string(nil)}`,
	},
}

func runIndex(t *testing.T, i int) {
	var testCase = testCases[i]
	a := testCase.a
	b := testCase.b
	succeeded := false
	defer func() {
		if !succeeded {
			fmt.Printf("\ncompare_test #%v failed\nException while comparing %#v and %#v\n\n", i, a, b)
		}
	}()

	computedEqual, computedDifference := DeepCompare("a", a, "b", b)
	expectedDifference := testCase.difference
	expectedEqual := testCase.difference == ""

	// it'd be great to just use DeepCompare here, but we can't use that method in the test because it's what we're testing :(
	mismatch := ""
	if computedEqual != expectedEqual {
		mismatch = fmt.Sprintf("got computedEqual %v, expected %v\n", computedEqual, expectedEqual)
	} else if computedDifference != expectedDifference {
		mismatch = fmt.Sprintf("got computedDifference =\n%q\nexpected\n%q\n", computedDifference, expectedDifference)
	}
	if mismatch != "" {
		mismatch = strings.Replace(mismatch, "\n", "\n           ", -1)
		t.Errorf(`
test case: %d
a        : %#v
b        : %#v
error    :
           %s
`, i, a, b, mismatch)
	}
	succeeded = true
}

func TestAll(t *testing.T) {
	for i := range testCases {
		runIndex(t, i)
	}
}
