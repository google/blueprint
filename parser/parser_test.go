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
	"reflect"
	"testing"
	"text/scanner"
)

func mkpos(offset, line, column int) scanner.Position {
	return scanner.Position{
		Offset: offset,
		Line:   line,
		Column: column,
	}
}

var validParseTestCases = []struct {
	input    string
	defs     []Definition
	comments []*CommentGroup
}{
	{`
		foo {}
		`,
		[]Definition{
			&Module{
				Type:    "foo",
				TypePos: mkpos(3, 2, 3),
				Map: Map{
					LBracePos: mkpos(7, 2, 7),
					RBracePos: mkpos(8, 2, 8),
				},
			},
		},
		nil,
	},

	{`
		foo {
			name: "abc",
		}
		`,
		[]Definition{
			&Module{
				Type:    "foo",
				TypePos: mkpos(3, 2, 3),
				Map: Map{
					LBracePos: mkpos(7, 2, 7),
					RBracePos: mkpos(27, 4, 3),
					Properties: []*Property{
						{
							Name:     "name",
							NamePos:  mkpos(12, 3, 4),
							ColonPos: mkpos(16, 3, 8),
							Value: &String{
								LiteralPos: mkpos(18, 3, 10),
								Value:      "abc",
							},
						},
					},
				},
			},
		},
		nil,
	},

	{`
		foo {
			isGood: true,
		}
		`,
		[]Definition{
			&Module{
				Type:    "foo",
				TypePos: mkpos(3, 2, 3),
				Map: Map{
					LBracePos: mkpos(7, 2, 7),
					RBracePos: mkpos(28, 4, 3),
					Properties: []*Property{
						{
							Name:     "isGood",
							NamePos:  mkpos(12, 3, 4),
							ColonPos: mkpos(18, 3, 10),
							Value: &Bool{
								LiteralPos: mkpos(20, 3, 12),
								Value:      true,
							},
						},
					},
				},
			},
		},
		nil,
	},

	{`
		foo {
			stuff: ["asdf", "jkl;", "qwert",
				"uiop", "bnm,"]
		}
		`,
		[]Definition{
			&Module{
				Type:    "foo",
				TypePos: mkpos(3, 2, 3),
				Map: Map{
					LBracePos: mkpos(7, 2, 7),
					RBracePos: mkpos(67, 5, 3),
					Properties: []*Property{
						{
							Name:     "stuff",
							NamePos:  mkpos(12, 3, 4),
							ColonPos: mkpos(17, 3, 9),
							Value: &List{
								LBracePos: mkpos(19, 3, 11),
								RBracePos: mkpos(63, 4, 19),
								Values: []Expression{
									&String{
										LiteralPos: mkpos(20, 3, 12),
										Value:      "asdf",
									},
									&String{
										LiteralPos: mkpos(28, 3, 20),
										Value:      "jkl;",
									},
									&String{
										LiteralPos: mkpos(36, 3, 28),
										Value:      "qwert",
									},
									&String{
										LiteralPos: mkpos(49, 4, 5),
										Value:      "uiop",
									},
									&String{
										LiteralPos: mkpos(57, 4, 13),
										Value:      "bnm,",
									},
								},
							},
						},
					},
				},
			},
		},
		nil,
	},

	{`
		foo {
			stuff: {
				isGood: true,
				name: "bar"
			}
		}
		`,
		[]Definition{
			&Module{
				Type:    "foo",
				TypePos: mkpos(3, 2, 3),
				Map: Map{
					LBracePos: mkpos(7, 2, 7),
					RBracePos: mkpos(62, 7, 3),
					Properties: []*Property{
						{
							Name:     "stuff",
							NamePos:  mkpos(12, 3, 4),
							ColonPos: mkpos(17, 3, 9),
							Value: &Map{
								LBracePos: mkpos(19, 3, 11),
								RBracePos: mkpos(58, 6, 4),
								Properties: []*Property{
									{
										Name:     "isGood",
										NamePos:  mkpos(25, 4, 5),
										ColonPos: mkpos(31, 4, 11),
										Value: &Bool{
											LiteralPos: mkpos(33, 4, 13),
											Value:      true,
										},
									},
									{
										Name:     "name",
										NamePos:  mkpos(43, 5, 5),
										ColonPos: mkpos(47, 5, 9),
										Value: &String{
											LiteralPos: mkpos(49, 5, 11),
											Value:      "bar",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		nil,
	},

	{`
		// comment1
		foo /* test */ {
			// comment2
			isGood: true,  // comment3
		}
		`,
		[]Definition{
			&Module{
				Type:    "foo",
				TypePos: mkpos(17, 3, 3),
				Map: Map{
					LBracePos: mkpos(32, 3, 18),
					RBracePos: mkpos(81, 6, 3),
					Properties: []*Property{
						{
							Name:     "isGood",
							NamePos:  mkpos(52, 5, 4),
							ColonPos: mkpos(58, 5, 10),
							Value: &Bool{
								LiteralPos: mkpos(60, 5, 12),
								Value:      true,
							},
						},
					},
				},
			},
		},
		[]*CommentGroup{
			{
				Comments: []*Comment{
					&Comment{
						Comment: []string{"// comment1"},
						Slash:   mkpos(3, 2, 3),
					},
				},
			},
			{
				Comments: []*Comment{
					&Comment{
						Comment: []string{"/* test */"},
						Slash:   mkpos(21, 3, 7),
					},
				},
			},
			{
				Comments: []*Comment{
					&Comment{
						Comment: []string{"// comment2"},
						Slash:   mkpos(37, 4, 4),
					},
				},
			},
			{
				Comments: []*Comment{
					&Comment{
						Comment: []string{"// comment3"},
						Slash:   mkpos(67, 5, 19),
					},
				},
			},
		},
	},

	{`
		foo {
			name: "abc",
		}

		bar {
			name: "def",
		}
		`,
		[]Definition{
			&Module{
				Type:    "foo",
				TypePos: mkpos(3, 2, 3),
				Map: Map{
					LBracePos: mkpos(7, 2, 7),
					RBracePos: mkpos(27, 4, 3),
					Properties: []*Property{
						{
							Name:     "name",
							NamePos:  mkpos(12, 3, 4),
							ColonPos: mkpos(16, 3, 8),
							Value: &String{
								LiteralPos: mkpos(18, 3, 10),
								Value:      "abc",
							},
						},
					},
				},
			},
			&Module{
				Type:    "bar",
				TypePos: mkpos(32, 6, 3),
				Map: Map{
					LBracePos: mkpos(36, 6, 7),
					RBracePos: mkpos(56, 8, 3),
					Properties: []*Property{
						{
							Name:     "name",
							NamePos:  mkpos(41, 7, 4),
							ColonPos: mkpos(45, 7, 8),
							Value: &String{
								LiteralPos: mkpos(47, 7, 10),
								Value:      "def",
							},
						},
					},
				},
			},
		},
		nil,
	},
	{`
		foo = "stuff"
		bar = foo
		baz = foo + bar
		boo = baz
		boo += foo
		`,
		[]Definition{
			&Assignment{
				Name:      "foo",
				NamePos:   mkpos(3, 2, 3),
				EqualsPos: mkpos(7, 2, 7),
				Value: &String{
					LiteralPos: mkpos(9, 2, 9),
					Value:      "stuff",
				},
				OrigValue: &String{
					LiteralPos: mkpos(9, 2, 9),
					Value:      "stuff",
				},
				Assigner:   "=",
				Referenced: true,
			},
			&Assignment{
				Name:      "bar",
				NamePos:   mkpos(19, 3, 3),
				EqualsPos: mkpos(23, 3, 7),
				Value: &Variable{
					Name:    "foo",
					NamePos: mkpos(25, 3, 9),
					Value: &String{
						LiteralPos: mkpos(9, 2, 9),
						Value:      "stuff",
					},
				},
				OrigValue: &Variable{
					Name:    "foo",
					NamePos: mkpos(25, 3, 9),
					Value: &String{
						LiteralPos: mkpos(9, 2, 9),
						Value:      "stuff",
					},
				},
				Assigner:   "=",
				Referenced: true,
			},
			&Assignment{
				Name:      "baz",
				NamePos:   mkpos(31, 4, 3),
				EqualsPos: mkpos(35, 4, 7),
				Value: &Operator{
					OperatorPos: mkpos(41, 4, 13),
					Operator:    '+',
					Value: &String{
						LiteralPos: mkpos(9, 2, 9),
						Value:      "stuffstuff",
					},
					Args: [2]Expression{
						&Variable{
							Name:    "foo",
							NamePos: mkpos(37, 4, 9),
							Value: &String{
								LiteralPos: mkpos(9, 2, 9),
								Value:      "stuff",
							},
						},
						&Variable{
							Name:    "bar",
							NamePos: mkpos(43, 4, 15),
							Value: &Variable{
								Name:    "foo",
								NamePos: mkpos(25, 3, 9),
								Value: &String{
									LiteralPos: mkpos(9, 2, 9),
									Value:      "stuff",
								},
							},
						},
					},
				},
				OrigValue: &Operator{
					OperatorPos: mkpos(41, 4, 13),
					Operator:    '+',
					Value: &String{
						LiteralPos: mkpos(9, 2, 9),
						Value:      "stuffstuff",
					},
					Args: [2]Expression{
						&Variable{
							Name:    "foo",
							NamePos: mkpos(37, 4, 9),
							Value: &String{
								LiteralPos: mkpos(9, 2, 9),
								Value:      "stuff",
							},
						},
						&Variable{
							Name:    "bar",
							NamePos: mkpos(43, 4, 15),
							Value: &Variable{
								Name:    "foo",
								NamePos: mkpos(25, 3, 9),
								Value: &String{
									LiteralPos: mkpos(9, 2, 9),
									Value:      "stuff",
								},
							},
						},
					},
				},
				Assigner:   "=",
				Referenced: true,
			},
			&Assignment{
				Name:      "boo",
				NamePos:   mkpos(49, 5, 3),
				EqualsPos: mkpos(53, 5, 7),
				Value: &Operator{
					Args: [2]Expression{
						&Variable{
							Name:    "baz",
							NamePos: mkpos(55, 5, 9),
							Value: &Operator{
								OperatorPos: mkpos(41, 4, 13),
								Operator:    '+',
								Value: &String{
									LiteralPos: mkpos(9, 2, 9),
									Value:      "stuffstuff",
								},
								Args: [2]Expression{
									&Variable{
										Name:    "foo",
										NamePos: mkpos(37, 4, 9),
										Value: &String{
											LiteralPos: mkpos(9, 2, 9),
											Value:      "stuff",
										},
									},
									&Variable{
										Name:    "bar",
										NamePos: mkpos(43, 4, 15),
										Value: &Variable{
											Name:    "foo",
											NamePos: mkpos(25, 3, 9),
											Value: &String{
												LiteralPos: mkpos(9, 2, 9),
												Value:      "stuff",
											},
										},
									},
								},
							},
						},
						&Variable{
							Name:    "foo",
							NamePos: mkpos(68, 6, 10),
							Value: &String{
								LiteralPos: mkpos(9, 2, 9),
								Value:      "stuff",
							},
						},
					},
					OperatorPos: mkpos(66, 6, 8),
					Operator:    '+',
					Value: &String{
						LiteralPos: mkpos(9, 2, 9),
						Value:      "stuffstuffstuff",
					},
				},
				OrigValue: &Variable{
					Name:    "baz",
					NamePos: mkpos(55, 5, 9),
					Value: &Operator{
						OperatorPos: mkpos(41, 4, 13),
						Operator:    '+',
						Value: &String{
							LiteralPos: mkpos(9, 2, 9),
							Value:      "stuffstuff",
						},
						Args: [2]Expression{
							&Variable{
								Name:    "foo",
								NamePos: mkpos(37, 4, 9),
								Value: &String{
									LiteralPos: mkpos(9, 2, 9),
									Value:      "stuff",
								},
							},
							&Variable{
								Name:    "bar",
								NamePos: mkpos(43, 4, 15),
								Value: &Variable{
									Name:    "foo",
									NamePos: mkpos(25, 3, 9),
									Value: &String{
										LiteralPos: mkpos(9, 2, 9),
										Value:      "stuff",
									},
								},
							},
						},
					},
				},
				Assigner: "=",
			},
			&Assignment{
				Name:      "boo",
				NamePos:   mkpos(61, 6, 3),
				EqualsPos: mkpos(66, 6, 8),
				Value: &Variable{
					Name:    "foo",
					NamePos: mkpos(68, 6, 10),
					Value: &String{
						LiteralPos: mkpos(9, 2, 9),
						Value:      "stuff",
					},
				},
				OrigValue: &Variable{
					Name:    "foo",
					NamePos: mkpos(68, 6, 10),
					Value: &String{
						LiteralPos: mkpos(9, 2, 9),
						Value:      "stuff",
					},
				},
				Assigner: "+=",
			},
		},
		nil,
	},
	{`
		// comment1
		// comment2

		/* comment3
		   comment4 */
		// comment5

		/* comment6 */ /* comment7 */ // comment8
		`,
		nil,
		[]*CommentGroup{
			{
				Comments: []*Comment{
					&Comment{
						Comment: []string{"// comment1"},
						Slash:   mkpos(3, 2, 3),
					},
					&Comment{
						Comment: []string{"// comment2"},
						Slash:   mkpos(17, 3, 3),
					},
				},
			},
			{
				Comments: []*Comment{
					&Comment{
						Comment: []string{"/* comment3", "		   comment4 */"},
						Slash: mkpos(32, 5, 3),
					},
					&Comment{
						Comment: []string{"// comment5"},
						Slash:   mkpos(63, 7, 3),
					},
				},
			},
			{
				Comments: []*Comment{
					&Comment{
						Comment: []string{"/* comment6 */"},
						Slash:   mkpos(78, 9, 3),
					},
					&Comment{
						Comment: []string{"/* comment7 */"},
						Slash:   mkpos(93, 9, 18),
					},
					&Comment{
						Comment: []string{"// comment8"},
						Slash:   mkpos(108, 9, 33),
					},
				},
			},
		},
	},
}

func TestParseValidInput(t *testing.T) {
	for _, testCase := range validParseTestCases {
		r := bytes.NewBufferString(testCase.input)
		file, errs := ParseAndEval("", r, NewScope(nil))
		if len(errs) != 0 {
			t.Errorf("test case: %s", testCase.input)
			t.Errorf("unexpected errors:")
			for _, err := range errs {
				t.Errorf("  %s", err)
			}
			t.FailNow()
		}

		if len(file.Defs) == len(testCase.defs) {
			for i := range file.Defs {
				if !reflect.DeepEqual(file.Defs[i], testCase.defs[i]) {
					t.Errorf("test case: %s", testCase.input)
					t.Errorf("incorrect defintion %d:", i)
					t.Errorf("  expected: %s", testCase.defs[i])
					t.Errorf("       got: %s", file.Defs[i])
				}
			}
		} else {
			t.Errorf("test case: %s", testCase.input)
			t.Errorf("length mismatch, expected %d definitions, got %d",
				len(testCase.defs), len(file.Defs))

type namedItem struct {
	name string
	item reflect.Value
}

// is supposed to tell whether two reflect.Value objects refer to objects that are equal
func compareValues(a reflect.Value, b reflect.Value) (equal bool) {
	equal, _ = deepCompareObjects(namedItem{"a", a}, namedItem{"b", b}, 0)
	return equal
}

func findMatching(item reflect.Value, list []reflect.Value) (match reflect.Value, found bool) {
	for _, x := range list {
		if compareValues(x, item) {
			return x, true
		}

	}
	return item, false
}

func isIn(item reflect.Value, list []reflect.Value) (contained bool) {
	_, found := findMatching(item, list)
	return found
}

// checks if two objects are the same class, returns True if they match
func compareObjectsClasses(actual namedItem, expected namedItem, depth int) (equal bool, diff string) {
	actualType := actual.item.Type()
	expectedType := expected.item.Type()
	if actualType == expectedType {
		return true, ""
	}
	return false, actual.name + " is of type " + actualType.String() + " whereas " + expected.name + " is of type " + expectedType.String()
}

func deepCompareObjects(actual namedItem, expected namedItem, depth int) (equal bool, diff string) {
	depth++
	if depth > 50 {
		panic(fmt.Sprintf("Detected probable infinite loop comparing %s (%s) and %s (%s). This diff checker doesn't yet support data structures having cycles.",
			actual.name,
			actual.item,
			expected.name,
			expected.item))
	}

	equal, diff = compareObjectsClasses(actual, expected, depth)
	if !equal {
		return equal, diff
	}

	switch actual.item.Kind() {
	case reflect.Array, reflect.Slice:
		return deepCompareArrays(actual, expected, depth)
	case reflect.Map:
		return deepCompareMaps(actual, expected, depth)
	case reflect.Struct:
		return deepCompareStructs(actual, expected, depth)
	case reflect.Ptr, reflect.Interface:
		return deepComparePointers(actual, expected, depth)
	default:
		return comparePrimitives(actual, expected, depth)
	}

}

func deepComparePointers(actual namedItem, expected namedItem, depth int) (equal bool, diff string) {
	if actual.item.IsNil() || expected.item.IsNil() {
		if actual.item.IsNil() && expected.item.IsNil() {
			return true, ""
		} else {

			if actual.item.IsNil() {
				return false, fmt.Sprintf("%s is nil whereas %s is %#v", actual.name, expected.name, expected.item)
			} else {
				return false, fmt.Sprintf("%s is nil whereas %s is %#v", expected.name, actual.name, actual.item)
			}
		}

	}
	return deepCompareObjects(namedItem{actual.name, actual.item.Elem()},
		namedItem{expected.name, expected.item.Elem()},
		depth)
}

func comparePrimitives(actual namedItem, expected namedItem, depth int) (equal bool, diff string) {
	// TODO make this less hacky
	var a, b interface{}
	if actual.item.Kind() != expected.item.Kind() {
		return false, fmt.Sprintf("%s is of type %s whereas %s is of type %s", actual.name, actual.item.Kind(), expected.name, expected.item.Kind())
	}
	switch actual.item.Kind() {
	case reflect.String:
		a = actual.item.String()
		b = expected.item.String()
	case reflect.Bool:
		a = actual.item.Bool()
		b = expected.item.Bool()
	case reflect.Int:
		a = actual.item.Int()
		b = expected.item.Int()
	default:
		panic(fmt.Sprintf("unrecognized object types, %s (%#v) and %s (%#v)", actual.name, actual.item, expected.name, expected.item))
	}
	if reflect.DeepEqual(a, b) {
		return true, ""
	} else {
		return false, fmt.Sprintf("%s = %#v whereas %s = %#v", actual.name, actual.item, expected.name, expected.item)
	}

}

func deepCompareMaps(actual namedItem, expected namedItem, depth int) (equal bool, diff string) {
	a := actual.item
	b := expected.item
	aKeys := a.MapKeys()
	bKeys := b.MapKeys()
	for _, aKey := range aKeys {
		if !(isIn(aKey, bKeys)) {
			aValue := a.MapIndex(aKey)
			return false, fmt.Sprintf("%v contains key %s (with value %#v) but %v does not", actual.name, aKey, aValue, expected.name)
		}
	}
	for _, bKey := range bKeys {
		if !(isIn(bKey, aKeys)) {
			bValue := b.MapIndex(bKey)
			return false, fmt.Sprintf("%v contains key %s (with value %#v) but %v does not", expected.name, bKey, bValue, actual.name)
		}
	}
	for _, aKey := range aKeys {
		bKey, _ := findMatching(aKey, bKeys)
		aIndexText := fmt.Sprintf("[%v(%t)]", aKey, bKey)
		bIndexText := fmt.Sprintf("[%v(%t)]", bKey, bKey)

		equal, diff = deepCompareObjects(namedItem{actual.name + aIndexText, a.MapIndex(aKey)},
			namedItem{expected.name + bIndexText, b.MapIndex(bKey)},
			depth,
		)
		if !equal {
			return equal, diff
		}

	}
	return true, ""
}

func deepCompareArrays(actual namedItem, expected namedItem, depth int) (equal bool, diff string) {
	a := actual.item
	b := expected.item
	aLen := a.Len()
	bLen := b.Len()
	sharedLen := int(math.Min(float64(aLen), float64(bLen)))
	for i := 0; i < sharedLen; i++ {
		indexText := fmt.Sprintf("[%v]", i)
		equal, diff = deepCompareObjects(namedItem{actual.name + indexText, a.Index(i)},
			namedItem{expected.name + indexText, b.Index(i)},
			depth,
		)
		if !equal {
			return equal, diff
		}
	}
	if aLen != bLen {
		var mismatchIndex int
		var mismatchName string
		var mismatchValue interface{}
		if aLen < bLen {
			mismatchIndex = aLen
			mismatchName = expected.name
			mismatchValue = expected.item.Index(mismatchIndex)
		} else {
			mismatchIndex = bLen
			mismatchName = actual.name
			mismatchValue = actual.item.Index(mismatchIndex)
		}
		return false, fmt.Sprintf("%s.len() = %v whereas %s.len() = %#v.\n\nFirst differing item: %s[%v] = %s",
			actual.name, aLen, expected.name, bLen, mismatchName, mismatchIndex, mismatchValue)
	}
	return true, ""
}

func deepCompareStructs(actual namedItem, expected namedItem, depth int) (equal bool, diff string) {
	a := actual.item
	b := expected.item
	aCount := a.NumField()
	bCount := b.NumField()
	if aCount != bCount {
		panic(fmt.Sprintf("Internal error: Objects of different classes were sent to deepCompareStructs. It should be caught by deepCompareObjects higher on the stack. "+
			"Objects: %#v, %#v", a, b))
	}
	aType := a.Type()
	for i := 0; i < aCount; i++ {
		fieldName := aType.Field(i).Name
		equal, diff = deepCompareObjects(namedItem{actual.name + "." + fieldName, a.Field(i)},
			namedItem{expected.name + "." + fieldName, b.Field(i)},
			depth)
		if !equal {
			return equal, diff
		}
	}
	return true, ""
}

// deepCompare is much like reflect.DeepEqual, but it does a deep comparison of map keys too (which is slower than a map lookup)
// also, deepCompare gives an explanation of the first difference that it finds
func deepCompare(actual interface{}, expected interface{}) (equal bool, diff string) {
	return deepCompareObjects(namedItem{"actual", reflect.ValueOf(actual)}, namedItem{"expected", reflect.ValueOf(expected)}, 0)
}

// deprecated
func safeDeepCompare(actual interface{}, expected interface{}) (equal bool, diff string) {
	customEqual, diff := deepCompare(actual, expected)
	libraryEqual := reflect.DeepEqual(actual, expected)
	if customEqual != libraryEqual {
		panic(fmt.Sprintf("inconsistent results from deepCompare (%s) (explanation='%s') and from reflect.deepEqual (%s) regarding the equality of \n%s \nand\n%s\n",
			customEqual,
			diff,
			libraryEqual,
			actual,
			expected))
	}
	return customEqual, diff
}

func runValidIndex(t *testing.T, i int) {
	var succeeded = false
	var testCase = validParseTestCases[i]
	defer func() {
		if !succeeded {
			t.Errorf("test case %d failed with input: \n%s\n", i, testCase.input)
		}
	}()
	r := bytes.NewBufferString(testCase.input)
	actualFileParse, errs := Parse("", r, NewScope(nil), true, true)
	actualParse := *actualFileParse.SyntaxTree
	if len(errs) != 0 {
		t.Errorf("test case: %s", testCase.input)
		t.Error("unexpected errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	var correctParse = *testCase.treeProvider()
	// confirm that the actual and expected trees are equivalent
	equal, diff := deepCompare(actualParse, correctParse)

	expectedRepresentation := NewVerbosePrinter(&correctParse).Print()
	actualRepresentation := NewVerbosePrinter(&actualParse).Print()
	if !equal {
		t.Errorf(`
test case: %d with input:
                %s
expected:
%s
got     :
%s
1st diff: %v

`, i, testCase.input, expectedRepresentation, actualRepresentation, diff)
	}
	succeeded = true
}

func TestParseValidInput(t *testing.T) {
	for i := range validParseTestCases {
		runValidIndex(t, i)
	}
}

// TODO: Test error strings
