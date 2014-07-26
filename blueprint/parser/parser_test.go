package parser

import (
	"bytes"
	"reflect"
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
	input  string
	output []Definition
}{
	{`
		foo {}
		`,
		[]Definition{
			&Module{
				Type: "foo",
				Pos:  mkpos(3, 2, 3),
			},
		},
	},

	{`
		foo {
			name: "abc",
		}
		`,
		[]Definition{
			&Module{
				Type: "foo",
				Pos:  mkpos(3, 2, 3),
				Properties: []*Property{
					{
						Name: "name",
						Pos:  mkpos(12, 3, 4),
						Value: Value{
							Type:        String,
							Pos:         mkpos(18, 3, 10),
							StringValue: "abc",
						},
					},
				},
			},
		},
	},

	{`
		foo {
			isGood: true,
		}
		`,
		[]Definition{
			&Module{
				Type: "foo",
				Pos:  mkpos(3, 2, 3),
				Properties: []*Property{
					{
						Name: "isGood",
						Pos:  mkpos(12, 3, 4),
						Value: Value{
							Type:      Bool,
							Pos:       mkpos(20, 3, 12),
							BoolValue: true,
						},
					},
				},
			},
		},
	},

	{`
		foo {
			stuff: ["asdf", "jkl;", "qwert",
				"uiop", "bnm,"]
		}
		`,
		[]Definition{
			&Module{
				Type: "foo",
				Pos:  mkpos(3, 2, 3),
				Properties: []*Property{
					{
						Name: "stuff",
						Pos:  mkpos(12, 3, 4),
						Value: Value{
							Type: List,
							Pos:  mkpos(19, 3, 11),
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
				Type: "foo",
				Pos:  mkpos(3, 2, 3),
				Properties: []*Property{
					{
						Name: "stuff",
						Pos:  mkpos(12, 3, 4),
						Value: Value{
							Type: Map,
							Pos:  mkpos(19, 3, 11),
							MapValue: []*Property{
								{
									Name: "isGood",
									Pos:  mkpos(25, 4, 5),
									Value: Value{
										Type:      Bool,
										Pos:       mkpos(33, 4, 13),
										BoolValue: true,
									},
								},
								{
									Name: "name",
									Pos:  mkpos(43, 5, 5),
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
	},

	{`
		// comment
		foo {
			// comment
			isGood: true,  // comment
		}
		`,
		[]Definition{
			&Module{
				Type: "foo",
				Pos:  mkpos(16, 3, 3),
				Properties: []*Property{
					{
						Name: "isGood",
						Pos:  mkpos(39, 5, 4),
						Value: Value{
							Type:      Bool,
							Pos:       mkpos(47, 5, 12),
							BoolValue: true,
						},
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
				Type: "foo",
				Pos:  mkpos(3, 2, 3),
				Properties: []*Property{
					{
						Name: "name",
						Pos:  mkpos(12, 3, 4),
						Value: Value{
							Type:        String,
							Pos:         mkpos(18, 3, 10),
							StringValue: "abc",
						},
					},
				},
			},
			&Module{
				Type: "bar",
				Pos:  mkpos(32, 6, 3),
				Properties: []*Property{
					{
						Name: "name",
						Pos:  mkpos(41, 7, 4),
						Value: Value{
							Type:        String,
							Pos:         mkpos(47, 7, 10),
							StringValue: "def",
						},
					},
				},
			},
		},
	},
}

func defListString(defs []Definition) string {
	defStrings := make([]string, len(defs))
	for i, def := range defs {
		defStrings[i] = def.String()
	}

	return strings.Join(defStrings, ", ")
}

func TestParseValidInput(t *testing.T) {
	for _, testCase := range validParseTestCases {
		r := bytes.NewBufferString(testCase.input)
		defs, errs := Parse("", r)
		if len(errs) != 0 {
			t.Errorf("test case: %s", testCase.input)
			t.Errorf("unexpected errors:")
			for _, err := range errs {
				t.Errorf("  %s", err)
			}
			t.FailNow()
		}

		if !reflect.DeepEqual(defs, testCase.output) {
			t.Errorf("test case: %s", testCase.input)
			t.Errorf("incorrect output:")
			t.Errorf("  expected: %s", defListString(testCase.output))
			t.Errorf("       got: %s", defListString(defs))
		}
	}
}

// TODO: Test error strings
