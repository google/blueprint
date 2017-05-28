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
	"reflect"
	"testing"
)

// parser_test.go isn't allowed to use the printer in its tests because printer_test.go uses the parser in its tests

var validParseTestCases = []struct {
	input        string
	treeProvider (func() (tree *SyntaxTree))
}{
	{`foo {}
		`,
		func() (tree *SyntaxTree) {
			return TreeWithNodes(
				[]ParseNode{
					&Module{
						Type: &Token{"foo"},
						Map:  NewMap(nil),
					},
				},
			)
		},
	},
	{`foo {
		}
		`,
		func() (tree *SyntaxTree) {
			b := NewBuilder()
			var mod = &Module{
				Type: &Token{"foo"},
				Map:  NewMap(nil),
			}
			b.AddNode(mod)
			b.AppendPostComment(mod.MapBody, NewBlankLine())
			return b.Build()
		},
	},
	{`foo {
			name: "abc",
		}`,
		func() (tree *SyntaxTree) {
			return TreeWithNodes(
				[]ParseNode{
					&Module{
						Type: &Token{"foo"},
						Map: NewMap(
							[]*Property{
								{
									Name: "name",
									Value: &String{
										Value: "abc",
									},
								},
							},
						),
					},
				},
			)
		},
	},

	{`foo {
			isGood: true,
		}
		`,
		func() (tree *SyntaxTree) {
			return TreeWithNodes(
				[]ParseNode{
					&Module{
						Type: &Token{"foo"},
						Map: NewMap(
							[]*Property{
								{
									Name: "isGood",
									Value: &Bool{
										Value: true,
									},
								},
							},
						),
					},
				},
			)
		},
	},
	{`foo {
			stuff: ["asdf", "jkl;", "qwert",
				"uiop", "bnm,",

				"zxcv "]
		}
		`,
		func() (tree *SyntaxTree) {
			bnm := &String{Value: "bnm,"}
			zxcv := &String{Value: "zxcv "}
			b := NewBuilder()
			b.AddNode(
				&Module{
					Type: &Token{"foo"},
					Map: NewMap(
						[]*Property{
							{
								Name: "stuff",
								Value: &List{
									Values: []Expression{
										&String{
											Value: "asdf",
										},
										&String{
											Value: "jkl;",
										},
										&String{
											Value: "qwert",
										},
										&String{
											Value: "uiop",
										},
										bnm,
										zxcv,
									},
									NewlineBetweenElements: true,
								},
							},
						},
					),
				},
			)
			// We have to specify the newline twice in order to get one blank line
			// We didn't need to specify the first newline when it was alone because then it was implied
			// The first newline stops being implied because there can be comments above it
			b.AppendPreComment(zxcv, NewBlankLine())
			b.AppendPreComment(zxcv, NewBlankLine())
			return b.Build()
		},
	},
	{`foo {
			stuff: ["asdf", "jkl;", "qwert", "uiop", "bnm,"]
		}
		`,
		func() (tree *SyntaxTree) {
			return TreeWithNodes(
				[]ParseNode{
					&Module{
						Type: &Token{"foo"},
						Map: NewMap(
							[]*Property{
								{
									Name: "stuff",
									Value: &List{
										Values: []Expression{
											&String{
												Value: "asdf",
											},
											&String{
												Value: "jkl;",
											},
											&String{
												Value: "qwert",
											},
											&String{
												Value: "uiop",
											},
											&String{
												Value: "bnm,",
											},
										},
										NewlineBetweenElements: true,
									},
								},
							},
						),
					},
				},
			)
		},
	},
	{`foo {
			stuff: ["asdf"]
		}
		`,
		func() (tree *SyntaxTree) {
			return TreeWithNodes(
				[]ParseNode{
					&Module{
						Type: &Token{"foo"},
						Map: NewMap(
							[]*Property{
								{
									Name: "stuff",
									Value: &List{
										Values: []Expression{
											&String{
												Value: "asdf",
											},
										},
										NewlineBetweenElements: false,
									},
								},
							},
						),
					},
				},
			)
		},
	},
	{`foo {
			stuff: {
				isGood: true,
				name: "bar"
			}
		}
		`,
		func() (tree *SyntaxTree) {
			return TreeWithNodes(
				[]ParseNode{
					&Module{
						Type: &Token{"foo"},
						Map: NewMap(
							[]*Property{
								{
									Name: "stuff",
									Value: NewMap(
										[]*Property{
											{
												Name: "isGood",
												Value: &Bool{
													Value: true,
												},
											},
											{
												Name: "name",
												Value: &String{
													Value: "bar",
												},
											},
										},
									),
								},
							},
						),
					},
				},
			)
		},
	},
	{`// comment1
		foo /* test */ {
			// comment2
			isGood: true,  // comment3
		} // comment 4
		`,
		func() (tree *SyntaxTree) {
			builder := NewBuilder()
			var val = &Bool{true}
			var key = &String{"isGood"}
			var prop = &Property{Name: key.Value, Value: val}
			var moduleType = &Token{"foo"}
			var propertyMap = NewMap([]*Property{prop})
			var mod = &Module{moduleType, propertyMap}
			builder.AddNode(mod)
			builder.AppendPreComment(mod, NewFullLineComment(" comment1"))

			builder.AppendPostComment(moduleType, NewInlineComment(" test "))
			builder.AppendPreComment(propertyMap.MapBody, NewBlankLine())
			builder.AppendPreComment(propertyMap.MapBody, NewFullLineComment(" comment2"))
			builder.AppendPostComment(prop, NewFullLineComment(" comment3"))
			builder.AppendPostComment(mod, NewFullLineComment(" comment 4"))
			return builder.Build()
		},
	},

	{`foo {
			name: "abc",
		}

		bar {
			name: "def",
		}
		`,
		func() (tree *SyntaxTree) {
			firstModule := &Module{
				Type: &Token{"foo"},
				Map: NewMap(
					[]*Property{
						{
							Name: "name",
							Value: &String{
								Value: "abc",
							},
						},
					},
				),
			}
			secondModule := &Module{
				Type: &Token{"bar"},
				Map: NewMap(
					[]*Property{
						{
							Name: "name",
							Value: &String{
								Value: "def",
							},
						},
					},
				),
			}
			builder := NewBuilder()
			builder.AddNode(firstModule)
			builder.AddNode(secondModule)
			builder.AppendPreComment(secondModule, NewBlankLine())
			return builder.Build()
		},
	},
	{`foo = "stuff"
		bar = foo
		baz = foo + bar
		boo = baz
		boo += foo`,
		func() (tree *SyntaxTree) {
			return TreeWithNodes(
				[]ParseNode{
					&Assignment{
						Name: &Token{"foo"},
						Value: &String{
							Value: "stuff",
						},
						OrigValue: &String{
							Value: "stuff",
						},
						Assigner:   Token{"="},
						Referenced: true,
					},
					&Assignment{
						Name: &Token{"bar"},
						Value: &Variable{
							NameNode: &Token{"foo"},
							Value: &String{
								Value: "stuff",
							},
						},
						OrigValue: &Variable{
							NameNode: &Token{"foo"},
							Value: &String{
								Value: "stuff",
							},
						},
						Assigner:   Token{"="},
						Referenced: true,
					},
					&Assignment{
						Name: &Token{"baz"},
						Value: &Operator{
							OperatorToken: &String{"+"},
							Value: &String{
								Value: "stuffstuff",
							},
							Args: [2]Expression{
								&Variable{
									NameNode: &Token{"foo"},
									Value: &String{
										Value: "stuff",
									},
								},
								&Variable{
									NameNode: &Token{"bar"},
									Value: &Variable{
										NameNode: &Token{"foo"},
										Value: &String{
											Value: "stuff",
										},
									},
								},
							},
						},
						OrigValue: &Operator{
							OperatorToken: &String{"+"},
							Value: &String{
								Value: "stuffstuff",
							},
							Args: [2]Expression{
								&Variable{
									NameNode: &Token{"foo"},
									Value: &String{
										Value: "stuff",
									},
								},
								&Variable{
									NameNode: &Token{"bar"},
									Value: &Variable{
										NameNode: &Token{"foo"},
										Value: &String{
											Value: "stuff",
										},
									},
								},
							},
						},
						Assigner:   Token{"="},
						Referenced: true,
					},
					&Assignment{
						Name: &Token{"boo"},
						Value: &Operator{
							Args: [2]Expression{
								&Variable{
									NameNode: &Token{"baz"},
									Value: &Operator{
										OperatorToken: &String{"+"},
										Value: &String{
											Value: "stuffstuff",
										},
										Args: [2]Expression{
											&Variable{
												NameNode: &Token{"foo"},
												Value: &String{
													Value: "stuff",
												},
											},
											&Variable{
												NameNode: &Token{"bar"},
												Value: &Variable{
													NameNode: &Token{"foo"},
													Value: &String{
														Value: "stuff",
													},
												},
											},
										},
									},
								},
								&Variable{
									NameNode: &Token{"foo"},
									Value: &String{
										Value: "stuff",
									},
								},
							},
							OperatorToken: &String{"+"},
							Value: &String{
								Value: "stuffstuffstuff",
							},
						},
						OrigValue: &Variable{
							NameNode: &Token{"baz"},
							Value: &Operator{
								OperatorToken: &String{"+"},
								Value: &String{
									Value: "stuffstuff",
								},
								Args: [2]Expression{
									&Variable{
										NameNode: &Token{"foo"},
										Value: &String{
											Value: "stuff",
										},
									},
									&Variable{
										NameNode: &Token{"bar"},
										Value: &Variable{
											NameNode: &Token{"foo"},
											Value: &String{
												Value: "stuff",
											},
										},
									},
								},
							},
						},
						Assigner: Token{"="},
					},
					&Assignment{
						Name: &Token{"boo"},
						Value: &Variable{
							NameNode: &Token{"foo"},
							Value: &String{
								Value: "stuff",
							},
						},
						OrigValue: &Variable{
							NameNode: &Token{"foo"},
							Value: &String{
								Value: "stuff",
							},
						},
						Assigner: Token{"+="},
					},
				},
			)
		},
	},
	{`// comment1
		// comment2

		/* comment3
		   comment4 */
		// comment5

		/* comment6 */ /* comment7 */ // comment8
		`,
		func() (tree *SyntaxTree) {
			tree = NewSyntaxTree()
			tree.AddNode(NewFullLineComment(" comment1"))
			tree.AddNode(NewFullLineComment(" comment2"))
			tree.AddNode(NewBlankLine())
			tree.AddNode(NewInlineComment(" comment3\n   comment4 "))
			tree.AddNode(NewBlankLine())
			tree.AddNode(NewFullLineComment(" comment5"))
			tree.AddNode(NewBlankLine())
			tree.AddNode(NewInlineComment(" comment6 "))
			tree.AddNode(NewInlineComment(" comment7 "))
			tree.AddNode(NewFullLineComment(" comment8"))
			return tree
		},
	},
	{
		`first = "one two three"
		`,
		func() (tree *SyntaxTree) {
			b := NewBuilder()
			b.AddNode(&Assignment{Name: &Token{"first"},
				Value:     &String{"one two three"},
				Assigner:  Token{"="},
				OrigValue: &String{"one two three"}, // TODO do we need to keep using OrigValue at all?
			})
			return b.Build()
		},
	},
	{
		`//two comments
		//blank line

		emptyModule {}
		`,
		func() (tree *SyntaxTree) {
			b := NewBuilder()
			mod := &Module{
				Type: &Token{"emptyModule"},
				Map:  NewMap(nil),
			}
			b.AddNode(mod)
			b.AppendPreComment(mod, NewFullLineComment("two comments"))
			b.AppendPreComment(mod, NewFullLineComment("blank line"))
			b.AppendPreComment(mod, NewBlankLine())
			return b.Build()
		},
	},
	{
		`//blank line 1

		emptyModule {
		} //trailing comment
		//trailing comment 2

		emptyModule2 {
		}
		`,
		func() (tree *SyntaxTree) {
			b := NewBuilder()
			mod := &Module{
				Type: &Token{"emptyModule"},
				Map:  NewMap(nil),
			}
			b.AddNode(mod)
			b.AppendPreComment(mod, NewFullLineComment("blank line 1"))
			b.AppendPreComment(mod, NewBlankLine())
			b.AppendPostComment(mod.MapBody, NewBlankLine())
			b.AppendPostComment(mod, NewFullLineComment("trailing comment"))
			mod2 := &Module{
				Type: &Token{"emptyModule2"},
				Map:  NewMap(nil),
			}
			b.AddNode(mod2)
			b.AppendPreComment(mod2, NewFullLineComment("trailing comment 2"))
			b.AppendPreComment(mod2, NewBlankLine())
			b.AppendPostComment(mod2.MapBody, NewBlankLine())
			return b.Build()
		},
	},
	{
		`/*test {
                    test: true,
                }*/

                test {
                /*test: true,*/
                }

                // This

		/* is here */

		anotherModule {}
		`,
		func() (tree *SyntaxTree) {
			b := NewBuilder()
			mod := &Module{
				Type: &Token{"test"},
				Map:  NewMap(nil),
			}
			b.AddNode(mod)
			b.AppendPreComment(mod, NewInlineComment(`test {
                    test: true,
                }`))
			b.AppendPreComment(mod, NewBlankLine())
			b.AppendPreComment(mod, NewBlankLine())
			b.AppendPreComment(mod.MapBody, NewBlankLine())
			b.AppendPreComment(mod.MapBody, NewInlineComment("test: true,"))
			b.AppendPreComment(mod.MapBody, NewBlankLine())
			mod2 := &Module{Type: &Token{"anotherModule"},
				Map: NewMap(nil),
			}
			b.AddNode(mod2)
			b.AppendPreComment(mod2, NewBlankLine())
			b.AppendPreComment(mod2, NewFullLineComment(" This"))
			b.AppendPreComment(mod2, NewBlankLine())
			b.AppendPreComment(mod2, NewInlineComment(" is here "))
			b.AppendPreComment(mod2, NewBlankLine())
			b.AppendPreComment(mod2, NewBlankLine())
			return b.Build()
		},
	},
	{
		`baseList = [
			"libext2fs",
			"libext2_blkid",
		]
		largerList = baseList + ["libc"]`,
		func() (tree *SyntaxTree) {
			var baseList = &List{
				Values: []Expression{
					&String{
						Value: "libext2fs",
					},
					&String{
						Value: "libext2_blkid",
					},
				},
				NewlineBetweenElements: true,
			}
			var largerList = &List{
				Values: []Expression{
					&String{
						Value: "libext2fs",
					},
					&String{
						Value: "libext2_blkid",
					},
					&String{
						Value: "libc",
					},
				},
				NewlineBetweenElements: true,
			}
			var operator = &Operator{
				OperatorToken: &String{"+"},
				Value:         largerList,
				Args: [2]Expression{
					&Variable{
						NameNode: &Token{"baseList"},
						Value:    baseList,
					},
					&List{
						Values: []Expression{
							&String{
								Value: "libc",
							},
						},
					},
				},
			}
			var _ = &Variable{
				NameNode: nil,
				Value:    operator,
			}

			return TreeWithNodes(
				[]ParseNode{
					&Assignment{
						Name:       &Token{"baseList"},
						Value:      baseList,
						OrigValue:  baseList,
						Assigner:   Token{"="},
						Referenced: true,
					},
					&Assignment{
						Name:       &Token{"largerList"},
						Value:      operator,
						OrigValue:  operator,
						Assigner:   Token{"="},
						Referenced: false,
					},
				},
			)
		},
	},
	{
		input: `
myModule {

    // a blank line before a property remains as a blank line

    myProperty: "myValue",

    // a blank line after a property remains as a blank line

}`,
		treeProvider: func() (tree *SyntaxTree) {
			b := NewBuilder()
			prop := &Property{Name: "myProperty", Value: &String{Value: "myValue"}}

			myMap := NewMap([]*Property{prop})
			mod := &Module{
				Type: &Token{"myModule"},
				Map:  myMap,
			}
			b.AddNode(mod)
			b.AppendPreComment(myMap.MapBody, NewBlankLine())
			b.AppendPreComment(myMap.MapBody, NewBlankLine())
			b.AppendPreComment(myMap.MapBody, NewFullLineComment(" a blank line before a property remains as a blank line"))
			b.AppendPreComment(myMap.MapBody, NewBlankLine())
			b.AppendPostComment(prop, NewBlankLine())
			b.AppendPostComment(prop, NewBlankLine())
			b.AppendPostComment(prop, NewFullLineComment(" a blank line after a property remains as a blank line"))
			b.AppendPostComment(prop, NewBlankLine())
			return b.Build()
		},
	},
	{`moduleContainingSpaces {

			propertyOne: "ImFirst",

			propertyTwo: "ImLast",

		}
		`,
		func() (tree *SyntaxTree) {
			b := NewBuilder()

			prop1 := &Property{
				Name: "propertyOne",
				Value: &String{
					Value: "ImFirst",
				},
			}
			prop2 := &Property{
				Name: "propertyTwo",
				Value: &String{
					Value: "ImLast",
				},
			}

			myMap := NewMap([]*Property{prop1, prop2})
			mod := &Module{
				Type: &Token{"moduleContainingSpaces"},
				Map:  myMap,
			}
			mapBody := myMap.MapBody

			b.AddNode(mod)
			b.AppendPreComment(mapBody, NewBlankLine())
			b.AppendPreComment(mapBody, NewBlankLine())

			b.AppendPostComment(prop1, NewBlankLine())
			b.AppendPostComment(prop1, NewBlankLine())

			b.AppendPostComment(mapBody, NewBlankLine())
			//b.AppendPostComment(mapBody, NewBlankLine())
			return b.Build()
		},
	},
}

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
	actualFileParse, errs := ParseAndEval("", r, NewScope(nil))
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
