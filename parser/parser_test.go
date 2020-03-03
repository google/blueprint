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
	"strconv"
	"strings"
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
								Token:      "true",
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
			num: 4,
		}
		`,
		[]Definition{
			&Module{
				Type:    "foo",
				TypePos: mkpos(3, 2, 3),
				Map: Map{
					LBracePos: mkpos(7, 2, 7),
					RBracePos: mkpos(22, 4, 3),
					Properties: []*Property{
						{
							Name:     "num",
							NamePos:  mkpos(12, 3, 4),
							ColonPos: mkpos(15, 3, 7),
							Value: &Int64{
								LiteralPos: mkpos(17, 3, 9),
								Value:      4,
								Token:      "4",
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

	{
		`
		foo {
			list_of_maps: [
				{
					var: true,
					name: "a",
				},
				{
					var: false,
					name: "b",
				},
			],
		}
`,
		[]Definition{
			&Module{
				Type:    "foo",
				TypePos: mkpos(3, 2, 3),
				Map: Map{
					LBracePos: mkpos(7, 2, 7),
					RBracePos: mkpos(127, 13, 3),
					Properties: []*Property{
						{
							Name:     "list_of_maps",
							NamePos:  mkpos(12, 3, 4),
							ColonPos: mkpos(24, 3, 16),
							Value: &List{
								LBracePos: mkpos(26, 3, 18),
								RBracePos: mkpos(122, 12, 4),
								Values: []Expression{
									&Map{
										LBracePos: mkpos(32, 4, 5),
										RBracePos: mkpos(70, 7, 5),
										Properties: []*Property{
											{
												Name:     "var",
												NamePos:  mkpos(39, 5, 6),
												ColonPos: mkpos(42, 5, 9),
												Value: &Bool{
													LiteralPos: mkpos(44, 5, 11),
													Value:      true,
													Token:      "true",
												},
											},
											{
												Name:     "name",
												NamePos:  mkpos(55, 6, 6),
												ColonPos: mkpos(59, 6, 10),
												Value: &String{
													LiteralPos: mkpos(61, 6, 12),
													Value:      "a",
												},
											},
										},
									},
									&Map{
										LBracePos: mkpos(77, 8, 5),
										RBracePos: mkpos(116, 11, 5),
										Properties: []*Property{
											{
												Name:     "var",
												NamePos:  mkpos(84, 9, 6),
												ColonPos: mkpos(87, 9, 9),
												Value: &Bool{
													LiteralPos: mkpos(89, 9, 11),
													Value:      false,
													Token:      "false",
												},
											},
											{
												Name:     "name",
												NamePos:  mkpos(101, 10, 6),
												ColonPos: mkpos(105, 10, 10),
												Value: &String{
													LiteralPos: mkpos(107, 10, 12),
													Value:      "b",
												},
											},
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
	{
		`
		foo {
			list_of_lists: [
				[ "a", "b" ],
				[ "c", "d" ]
			],
		}
`,
		[]Definition{
			&Module{
				Type:    "foo",
				TypePos: mkpos(3, 2, 3),
				Map: Map{
					LBracePos: mkpos(7, 2, 7),
					RBracePos: mkpos(72, 7, 3),
					Properties: []*Property{
						{
							Name:     "list_of_lists",
							NamePos:  mkpos(12, 3, 4),
							ColonPos: mkpos(25, 3, 17),
							Value: &List{
								LBracePos: mkpos(27, 3, 19),
								RBracePos: mkpos(67, 6, 4),
								Values: []Expression{
									&List{
										LBracePos: mkpos(33, 4, 5),
										RBracePos: mkpos(44, 4, 16),
										Values: []Expression{
											&String{
												LiteralPos: mkpos(35, 4, 7),
												Value:      "a",
											},
											&String{
												LiteralPos: mkpos(40, 4, 12),
												Value:      "b",
											},
										},
									},
									&List{
										LBracePos: mkpos(51, 5, 5),
										RBracePos: mkpos(62, 5, 16),
										Values: []Expression{
											&String{
												LiteralPos: mkpos(53, 5, 7),
												Value:      "c",
											},
											&String{
												LiteralPos: mkpos(58, 5, 12),
												Value:      "d",
											},
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
		foo {
			stuff: {
				isGood: true,
				name: "bar",
				num: 36,
			}
		}
		`,
		[]Definition{
			&Module{
				Type:    "foo",
				TypePos: mkpos(3, 2, 3),
				Map: Map{
					LBracePos: mkpos(7, 2, 7),
					RBracePos: mkpos(76, 8, 3),
					Properties: []*Property{
						{
							Name:     "stuff",
							NamePos:  mkpos(12, 3, 4),
							ColonPos: mkpos(17, 3, 9),
							Value: &Map{
								LBracePos: mkpos(19, 3, 11),
								RBracePos: mkpos(72, 7, 4),
								Properties: []*Property{
									{
										Name:     "isGood",
										NamePos:  mkpos(25, 4, 5),
										ColonPos: mkpos(31, 4, 11),
										Value: &Bool{
											LiteralPos: mkpos(33, 4, 13),
											Value:      true,
											Token:      "true",
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
									{
										Name:     "num",
										NamePos:  mkpos(60, 6, 5),
										ColonPos: mkpos(63, 6, 8),
										Value: &Int64{
											LiteralPos: mkpos(65, 6, 10),
											Value:      36,
											Token:      "36",
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
								Token:      "true",
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
			num: 4,
		}

		bar {
			name: "def",
			num: -5,
		}
		`,
		[]Definition{
			&Module{
				Type:    "foo",
				TypePos: mkpos(3, 2, 3),
				Map: Map{
					LBracePos: mkpos(7, 2, 7),
					RBracePos: mkpos(38, 5, 3),
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
						{
							Name:     "num",
							NamePos:  mkpos(28, 4, 4),
							ColonPos: mkpos(31, 4, 7),
							Value: &Int64{
								LiteralPos: mkpos(33, 4, 9),
								Value:      4,
								Token:      "4",
							},
						},
					},
				},
			},
			&Module{
				Type:    "bar",
				TypePos: mkpos(43, 7, 3),
				Map: Map{
					LBracePos: mkpos(47, 7, 7),
					RBracePos: mkpos(79, 10, 3),
					Properties: []*Property{
						{
							Name:     "name",
							NamePos:  mkpos(52, 8, 4),
							ColonPos: mkpos(56, 8, 8),
							Value: &String{
								LiteralPos: mkpos(58, 8, 10),
								Value:      "def",
							},
						},
						{
							Name:     "num",
							NamePos:  mkpos(68, 9, 4),
							ColonPos: mkpos(71, 9, 7),
							Value: &Int64{
								LiteralPos: mkpos(73, 9, 9),
								Value:      -5,
								Token:      "-5",
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
		baz = -4 + -5 + 6
		`,
		[]Definition{
			&Assignment{
				Name:      "baz",
				NamePos:   mkpos(3, 2, 3),
				EqualsPos: mkpos(7, 2, 7),
				Value: &Operator{
					OperatorPos: mkpos(12, 2, 12),
					Operator:    '+',
					Value: &Int64{
						LiteralPos: mkpos(9, 2, 9),
						Value:      -3,
					},
					Args: [2]Expression{
						&Int64{
							LiteralPos: mkpos(9, 2, 9),
							Value:      -4,
							Token:      "-4",
						},
						&Operator{
							OperatorPos: mkpos(17, 2, 17),
							Operator:    '+',
							Value: &Int64{
								LiteralPos: mkpos(14, 2, 14),
								Value:      1,
							},
							Args: [2]Expression{
								&Int64{
									LiteralPos: mkpos(14, 2, 14),
									Value:      -5,
									Token:      "-5",
								},
								&Int64{
									LiteralPos: mkpos(19, 2, 19),
									Value:      6,
									Token:      "6",
								},
							},
						},
					},
				},
				OrigValue: &Operator{
					OperatorPos: mkpos(12, 2, 12),
					Operator:    '+',
					Value: &Int64{
						LiteralPos: mkpos(9, 2, 9),
						Value:      -3,
					},
					Args: [2]Expression{
						&Int64{
							LiteralPos: mkpos(9, 2, 9),
							Value:      -4,
							Token:      "-4",
						},
						&Operator{
							OperatorPos: mkpos(17, 2, 17),
							Operator:    '+',
							Value: &Int64{
								LiteralPos: mkpos(14, 2, 14),
								Value:      1,
							},
							Args: [2]Expression{
								&Int64{
									LiteralPos: mkpos(14, 2, 14),
									Value:      -5,
									Token:      "-5",
								},
								&Int64{
									LiteralPos: mkpos(19, 2, 19),
									Value:      6,
									Token:      "6",
								},
							},
						},
					},
				},
				Assigner:   "=",
				Referenced: false,
			},
		},
		nil,
	},

	{`
		foo = 1000000
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
				Value: &Int64{
					LiteralPos: mkpos(9, 2, 9),
					Value:      1000000,
					Token:      "1000000",
				},
				OrigValue: &Int64{
					LiteralPos: mkpos(9, 2, 9),
					Value:      1000000,
					Token:      "1000000",
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
					Value: &Int64{
						LiteralPos: mkpos(9, 2, 9),
						Value:      1000000,
						Token:      "1000000",
					},
				},
				OrigValue: &Variable{
					Name:    "foo",
					NamePos: mkpos(25, 3, 9),
					Value: &Int64{
						LiteralPos: mkpos(9, 2, 9),
						Value:      1000000,
						Token:      "1000000",
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
					Value: &Int64{
						LiteralPos: mkpos(9, 2, 9),
						Value:      2000000,
					},
					Args: [2]Expression{
						&Variable{
							Name:    "foo",
							NamePos: mkpos(37, 4, 9),
							Value: &Int64{
								LiteralPos: mkpos(9, 2, 9),
								Value:      1000000,
								Token:      "1000000",
							},
						},
						&Variable{
							Name:    "bar",
							NamePos: mkpos(43, 4, 15),
							Value: &Variable{
								Name:    "foo",
								NamePos: mkpos(25, 3, 9),
								Value: &Int64{
									LiteralPos: mkpos(9, 2, 9),
									Value:      1000000,
									Token:      "1000000",
								},
							},
						},
					},
				},
				OrigValue: &Operator{
					OperatorPos: mkpos(41, 4, 13),
					Operator:    '+',
					Value: &Int64{
						LiteralPos: mkpos(9, 2, 9),
						Value:      2000000,
					},
					Args: [2]Expression{
						&Variable{
							Name:    "foo",
							NamePos: mkpos(37, 4, 9),
							Value: &Int64{
								LiteralPos: mkpos(9, 2, 9),
								Value:      1000000,
								Token:      "1000000",
							},
						},
						&Variable{
							Name:    "bar",
							NamePos: mkpos(43, 4, 15),
							Value: &Variable{
								Name:    "foo",
								NamePos: mkpos(25, 3, 9),
								Value: &Int64{
									LiteralPos: mkpos(9, 2, 9),
									Value:      1000000,
									Token:      "1000000",
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
								Value: &Int64{
									LiteralPos: mkpos(9, 2, 9),
									Value:      2000000,
								},
								Args: [2]Expression{
									&Variable{
										Name:    "foo",
										NamePos: mkpos(37, 4, 9),
										Value: &Int64{
											LiteralPos: mkpos(9, 2, 9),
											Value:      1000000,
											Token:      "1000000",
										},
									},
									&Variable{
										Name:    "bar",
										NamePos: mkpos(43, 4, 15),
										Value: &Variable{
											Name:    "foo",
											NamePos: mkpos(25, 3, 9),
											Value: &Int64{
												LiteralPos: mkpos(9, 2, 9),
												Value:      1000000,
												Token:      "1000000",
											},
										},
									},
								},
							},
						},
						&Variable{
							Name:    "foo",
							NamePos: mkpos(68, 6, 10),
							Value: &Int64{
								LiteralPos: mkpos(9, 2, 9),
								Value:      1000000,
								Token:      "1000000",
							},
						},
					},
					OperatorPos: mkpos(66, 6, 8),
					Operator:    '+',
					Value: &Int64{
						LiteralPos: mkpos(9, 2, 9),
						Value:      3000000,
					},
				},
				OrigValue: &Variable{
					Name:    "baz",
					NamePos: mkpos(55, 5, 9),
					Value: &Operator{
						OperatorPos: mkpos(41, 4, 13),
						Operator:    '+',
						Value: &Int64{
							LiteralPos: mkpos(9, 2, 9),
							Value:      2000000,
						},
						Args: [2]Expression{
							&Variable{
								Name:    "foo",
								NamePos: mkpos(37, 4, 9),
								Value: &Int64{
									LiteralPos: mkpos(9, 2, 9),
									Value:      1000000,
									Token:      "1000000",
								},
							},
							&Variable{
								Name:    "bar",
								NamePos: mkpos(43, 4, 15),
								Value: &Variable{
									Name:    "foo",
									NamePos: mkpos(25, 3, 9),
									Value: &Int64{
										LiteralPos: mkpos(9, 2, 9),
										Value:      1000000,
										Token:      "1000000",
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
					Value: &Int64{
						LiteralPos: mkpos(9, 2, 9),
						Value:      1000000,
						Token:      "1000000",
					},
				},
				OrigValue: &Variable{
					Name:    "foo",
					NamePos: mkpos(68, 6, 10),
					Value: &Int64{
						LiteralPos: mkpos(9, 2, 9),
						Value:      1000000,
						Token:      "1000000",
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
	for i, testCase := range validParseTestCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
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
						t.Errorf("incorrect definition %d:", i)
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
		})
	}
}

// TODO: Test error strings

func TestParserEndPos(t *testing.T) {
	in := `
		module {
			string: "string",
			stringexp: "string1" + "string2",
			int: -1,
			intexp: -1 + 2,
			list: ["a", "b"],
			listexp: ["c"] + ["d"],
			multilinelist: [
				"e",
				"f",
			],
			map: {
				prop: "abc",
			},
		}
	`

	// Strip each line to make it easier to compute the previous "," from each property
	lines := strings.Split(in, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	in = strings.Join(lines, "\n")

	r := bytes.NewBufferString(in)

	file, errs := ParseAndEval("", r, NewScope(nil))
	if len(errs) != 0 {
		t.Errorf("unexpected errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	mod := file.Defs[0].(*Module)
	modEnd := mkpos(len(in)-1, len(lines)-1, 2)
	if mod.End() != modEnd {
		t.Errorf("expected mod.End() %s, got %s", modEnd, mod.End())
	}

	nextPos := make([]scanner.Position, len(mod.Properties))
	for i := 0; i < len(mod.Properties)-1; i++ {
		nextPos[i] = mod.Properties[i+1].Pos()
	}
	nextPos[len(mod.Properties)-1] = mod.RBracePos

	for i, cur := range mod.Properties {
		endOffset := nextPos[i].Offset - len(",\n")
		endLine := nextPos[i].Line - 1
		endColumn := len(lines[endLine-1]) // scanner.Position.Line is starts at 1
		endPos := mkpos(endOffset, endLine, endColumn)
		if cur.End() != endPos {
			t.Errorf("expected property %s End() %s@%d, got %s@%d", cur.Name, endPos, endPos.Offset, cur.End(), cur.End().Offset)
		}
	}
}

func TestParserNotEvaluated(t *testing.T) {
	// When parsing without evaluation, create variables correctly
	scope := NewScope(nil)
	input := "FOO=abc\n"
	_, errs := Parse("", bytes.NewBufferString(input), scope)
	if errs != nil {
		t.Errorf("unexpected errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}
	assignment, found := scope.Get("FOO")
	if !found {
		t.Fatalf("Expected to find FOO after parsing %s", input)
	}
	if s := assignment.String(); strings.Contains(s, "PANIC") {
		t.Errorf("Attempt to print FOO returned %s", s)
	}
}
