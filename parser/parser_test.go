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
		}

		if len(file.Comments) == len(testCase.comments) {
			for i := range file.Comments {
				if !reflect.DeepEqual(file.Comments[i], testCase.comments[i]) {
					t.Errorf("test case: %s", testCase.input)
					t.Errorf("incorrect comment %d:", i)
					t.Errorf("  expected: %s", testCase.comments[i])
					t.Errorf("       got: %s", file.Comments[i])
				}
			}
		} else {
			t.Errorf("test case: %s", testCase.input)
			t.Errorf("length mismatch, expected %d comments, got %d",
				len(testCase.comments), len(file.Comments))
		}
	}
}

// TODO: Test error strings
