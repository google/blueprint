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
	comments []Comment
}{
	{`
		foo {}
		`,
		[]Definition{
			&Module{
				Type:      Ident{"foo", mkpos(3, 2, 3)},
				LbracePos: mkpos(7, 2, 7),
				RbracePos: mkpos(8, 2, 8),
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
				Type:      Ident{"foo", mkpos(3, 2, 3)},
				LbracePos: mkpos(7, 2, 7),
				RbracePos: mkpos(27, 4, 3),
				Properties: []*Property{
					{
						Name: Ident{"name", mkpos(12, 3, 4)},
						Pos:  mkpos(16, 3, 8),
						Value: Value{
							Type:        String,
							Pos:         mkpos(18, 3, 10),
							StringValue: "abc",
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
				Type:      Ident{"foo", mkpos(3, 2, 3)},
				LbracePos: mkpos(7, 2, 7),
				RbracePos: mkpos(28, 4, 3),
				Properties: []*Property{
					{
						Name: Ident{"isGood", mkpos(12, 3, 4)},
						Pos:  mkpos(18, 3, 10),
						Value: Value{
							Type:      Bool,
							Pos:       mkpos(20, 3, 12),
							BoolValue: true,
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
				Type:      Ident{"foo", mkpos(3, 2, 3)},
				LbracePos: mkpos(7, 2, 7),
				RbracePos: mkpos(67, 5, 3),
				Properties: []*Property{
					{
						Name: Ident{"stuff", mkpos(12, 3, 4)},
						Pos:  mkpos(17, 3, 9),
						Value: Value{
							Type:   List,
							Pos:    mkpos(19, 3, 11),
							EndPos: mkpos(63, 4, 19),
							ListValue: []Value{
								Value{
									Type:        String,
									Pos:         mkpos(20, 3, 12),
									StringValue: "asdf",
								},
								Value{
									Type:        String,
									Pos:         mkpos(28, 3, 20),
									StringValue: "jkl;",
								},
								Value{
									Type:        String,
									Pos:         mkpos(36, 3, 28),
									StringValue: "qwert",
								},
								Value{
									Type:        String,
									Pos:         mkpos(49, 4, 5),
									StringValue: "uiop",
								},
								Value{
									Type:        String,
									Pos:         mkpos(57, 4, 13),
									StringValue: "bnm,",
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
				Type:      Ident{"foo", mkpos(3, 2, 3)},
				LbracePos: mkpos(7, 2, 7),
				RbracePos: mkpos(62, 7, 3),
				Properties: []*Property{
					{
						Name: Ident{"stuff", mkpos(12, 3, 4)},
						Pos:  mkpos(17, 3, 9),
						Value: Value{
							Type:   Map,
							Pos:    mkpos(19, 3, 11),
							EndPos: mkpos(58, 6, 4),
							MapValue: []*Property{
								{
									Name: Ident{"isGood", mkpos(25, 4, 5)},
									Pos:  mkpos(31, 4, 11),
									Value: Value{
										Type:      Bool,
										Pos:       mkpos(33, 4, 13),
										BoolValue: true,
									},
								},
								{
									Name: Ident{"name", mkpos(43, 5, 5)},
									Pos:  mkpos(47, 5, 9),
									Value: Value{
										Type:        String,
										Pos:         mkpos(49, 5, 11),
										StringValue: "bar",
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
		foo {
			// comment2
			isGood: true,  // comment3
		}
		`,
		[]Definition{
			&Module{
				Type:      Ident{"foo", mkpos(17, 3, 3)},
				LbracePos: mkpos(21, 3, 7),
				RbracePos: mkpos(70, 6, 3),
				Properties: []*Property{
					{
						Name: Ident{"isGood", mkpos(41, 5, 4)},
						Pos:  mkpos(47, 5, 10),
						Value: Value{
							Type:      Bool,
							Pos:       mkpos(49, 5, 12),
							BoolValue: true,
						},
					},
				},
			},
		},
		[]Comment{
			Comment{
				Comment: []string{"// comment1"},
				Pos:     mkpos(3, 2, 3),
			},
			Comment{
				Comment: []string{"// comment2"},
				Pos:     mkpos(26, 4, 4),
			},
			Comment{
				Comment: []string{"// comment3"},
				Pos:     mkpos(56, 5, 19),
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
				Type:      Ident{"foo", mkpos(3, 2, 3)},
				LbracePos: mkpos(7, 2, 7),
				RbracePos: mkpos(27, 4, 3),
				Properties: []*Property{
					{
						Name: Ident{"name", mkpos(12, 3, 4)},
						Pos:  mkpos(16, 3, 8),
						Value: Value{
							Type:        String,
							Pos:         mkpos(18, 3, 10),
							StringValue: "abc",
						},
					},
				},
			},
			&Module{
				Type:      Ident{"bar", mkpos(32, 6, 3)},
				LbracePos: mkpos(36, 6, 7),
				RbracePos: mkpos(56, 8, 3),
				Properties: []*Property{
					{
						Name: Ident{"name", mkpos(41, 7, 4)},
						Pos:  mkpos(45, 7, 8),
						Value: Value{
							Type:        String,
							Pos:         mkpos(47, 7, 10),
							StringValue: "def",
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
				Name: Ident{"foo", mkpos(3, 2, 3)},
				Pos:  mkpos(7, 2, 7),
				Value: Value{
					Type:        String,
					Pos:         mkpos(9, 2, 9),
					StringValue: "stuff",
				},
				OrigValue: Value{
					Type:        String,
					Pos:         mkpos(9, 2, 9),
					StringValue: "stuff",
				},
				Assigner:   "=",
				Referenced: true,
			},
			&Assignment{
				Name: Ident{"bar", mkpos(19, 3, 3)},
				Pos:  mkpos(23, 3, 7),
				Value: Value{
					Type:        String,
					Pos:         mkpos(25, 3, 9),
					StringValue: "stuff",
					Variable:    "foo",
				},
				OrigValue: Value{
					Type:        String,
					Pos:         mkpos(25, 3, 9),
					StringValue: "stuff",
					Variable:    "foo",
				},
				Assigner:   "=",
				Referenced: true,
			},
			&Assignment{
				Name: Ident{"baz", mkpos(31, 4, 3)},
				Pos:  mkpos(35, 4, 7),
				Value: Value{
					Type:        String,
					Pos:         mkpos(37, 4, 9),
					StringValue: "stuffstuff",
					Expression: &Expression{
						Args: [2]Value{
							{
								Type:        String,
								Pos:         mkpos(37, 4, 9),
								StringValue: "stuff",
								Variable:    "foo",
							},
							{
								Type:        String,
								Pos:         mkpos(43, 4, 15),
								StringValue: "stuff",
								Variable:    "bar",
							},
						},
						Operator: '+',
						Pos:      mkpos(41, 4, 13),
					},
				},
				OrigValue: Value{
					Type:        String,
					Pos:         mkpos(37, 4, 9),
					StringValue: "stuffstuff",
					Expression: &Expression{
						Args: [2]Value{
							{
								Type:        String,
								Pos:         mkpos(37, 4, 9),
								StringValue: "stuff",
								Variable:    "foo",
							},
							{
								Type:        String,
								Pos:         mkpos(43, 4, 15),
								StringValue: "stuff",
								Variable:    "bar",
							},
						},
						Operator: '+',
						Pos:      mkpos(41, 4, 13),
					},
				},
				Assigner:   "=",
				Referenced: true,
			},
			&Assignment{
				Name: Ident{"boo", mkpos(49, 5, 3)},
				Pos:  mkpos(53, 5, 7),
				Value: Value{
					Type:        String,
					Pos:         mkpos(55, 5, 9),
					StringValue: "stuffstuffstuff",
					Expression: &Expression{
						Args: [2]Value{
							{
								Type:        String,
								Pos:         mkpos(55, 5, 9),
								StringValue: "stuffstuff",
								Variable:    "baz",
								Expression: &Expression{
									Args: [2]Value{
										{
											Type:        String,
											Pos:         mkpos(37, 4, 9),
											StringValue: "stuff",
											Variable:    "foo",
										},
										{
											Type:        String,
											Pos:         mkpos(43, 4, 15),
											StringValue: "stuff",
											Variable:    "bar",
										},
									},
									Operator: '+',
									Pos:      mkpos(41, 4, 13),
								},
							},
							{
								Variable:    "foo",
								Type:        String,
								Pos:         mkpos(68, 6, 10),
								StringValue: "stuff",
							},
						},
						Pos:      mkpos(66, 6, 8),
						Operator: '+',
					},
				},
				OrigValue: Value{
					Type:        String,
					Pos:         mkpos(55, 5, 9),
					StringValue: "stuffstuff",
					Variable:    "baz",
					Expression: &Expression{
						Args: [2]Value{
							{
								Type:        String,
								Pos:         mkpos(37, 4, 9),
								StringValue: "stuff",
								Variable:    "foo",
							},
							{
								Type:        String,
								Pos:         mkpos(43, 4, 15),
								StringValue: "stuff",
								Variable:    "bar",
							},
						},
						Operator: '+',
						Pos:      mkpos(41, 4, 13),
					},
				},
				Assigner: "=",
			},
			&Assignment{
				Name: Ident{"boo", mkpos(61, 6, 3)},
				Pos:  mkpos(66, 6, 8),
				Value: Value{
					Type:        String,
					Pos:         mkpos(68, 6, 10),
					StringValue: "stuff",
					Variable:    "foo",
				},
				OrigValue: Value{
					Type:        String,
					Pos:         mkpos(68, 6, 10),
					StringValue: "stuff",
					Variable:    "foo",
				},
				Assigner: "+=",
			},
		},
		nil,
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
				if !reflect.DeepEqual(file.Comments, testCase.comments) {
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
