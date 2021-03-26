// Copyright 2019 Google Inc. All rights reserved.
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
	"reflect"
	"strings"
	"testing"
)

type Named struct {
	A *string `keep:"true"`
	B *string
}

type NamedAllFiltered struct {
	A *string
}

type NamedNoneFiltered struct {
	A *string `keep:"true"`
}

func TestFilterPropertyStruct(t *testing.T) {
	tests := []struct {
		name     string
		in       interface{}
		out      interface{}
		filtered bool
	}{
		// Property tests
		{
			name: "basic",
			in: &struct {
				A *string `keep:"true"`
				B *string
			}{},
			out: &struct {
				A *string
			}{},
			filtered: true,
		},
		{
			name: "all filtered",
			in: &struct {
				A *string
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "none filtered",
			in: &struct {
				A *string `keep:"true"`
			}{},
			out: &struct {
				A *string `keep:"true"`
			}{},
			filtered: false,
		},

		// Sub-struct tests
		{
			name: "substruct",
			in: &struct {
				A struct {
					A *string `keep:"true"`
					B *string
				} `keep:"true"`
			}{},
			out: &struct {
				A struct {
					A *string
				}
			}{},
			filtered: true,
		},
		{
			name: "substruct all filtered",
			in: &struct {
				A struct {
					A *string
				} `keep:"true"`
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "substruct none filtered",
			in: &struct {
				A struct {
					A *string `keep:"true"`
				} `keep:"true"`
			}{},
			out: &struct {
				A struct {
					A *string `keep:"true"`
				} `keep:"true"`
			}{},
			filtered: false,
		},

		// Named sub-struct tests
		{
			name: "named substruct",
			in: &struct {
				A Named `keep:"true"`
			}{},
			out: &struct {
				A struct {
					A *string
				}
			}{},
			filtered: true,
		},
		{
			name: "substruct all filtered",
			in: &struct {
				A NamedAllFiltered `keep:"true"`
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "substruct none filtered",
			in: &struct {
				A NamedNoneFiltered `keep:"true"`
			}{},
			out: &struct {
				A NamedNoneFiltered `keep:"true"`
			}{},
			filtered: false,
		},

		// Pointer to sub-struct tests
		{
			name: "pointer substruct",
			in: &struct {
				A *struct {
					A *string `keep:"true"`
					B *string
				} `keep:"true"`
			}{},
			out: &struct {
				A *struct {
					A *string
				}
			}{},
			filtered: true,
		},
		{
			name: "pointer substruct all filtered",
			in: &struct {
				A *struct {
					A *string
				} `keep:"true"`
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "pointer substruct none filtered",
			in: &struct {
				A *struct {
					A *string `keep:"true"`
				} `keep:"true"`
			}{},
			out: &struct {
				A *struct {
					A *string `keep:"true"`
				} `keep:"true"`
			}{},
			filtered: false,
		},

		// Pointer to named sub-struct tests
		{
			name: "pointer named substruct",
			in: &struct {
				A *Named `keep:"true"`
			}{},
			out: &struct {
				A *struct {
					A *string
				}
			}{},
			filtered: true,
		},
		{
			name: "pointer substruct all filtered",
			in: &struct {
				A *NamedAllFiltered `keep:"true"`
			}{},
			out:      nil,
			filtered: true,
		},
		{
			name: "pointer substruct none filtered",
			in: &struct {
				A *NamedNoneFiltered `keep:"true"`
			}{},
			out: &struct {
				A *NamedNoneFiltered `keep:"true"`
			}{},
			filtered: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, filtered := FilterPropertyStruct(reflect.TypeOf(test.in),
				func(field reflect.StructField, prefix string) (bool, reflect.StructField) {
					if HasTag(field, "keep", "true") {
						field.Tag = ""
						return true, field
					}
					return false, field
				})
			if filtered != test.filtered {
				t.Errorf("expected filtered %v, got %v", test.filtered, filtered)
			}
			expected := reflect.TypeOf(test.out)
			if out != expected {
				t.Errorf("expected type %v, got %v", expected, out)
			}
		})
	}
}

func TestFilterPropertyStructSharded(t *testing.T) {
	type KeepAllWithAReallyLongNameThatExceedsTheMaxNameSize struct {
		A *string `keep:"true"`
		B *string `keep:"true"`
		C *string `keep:"true"`
	}

	tests := []struct {
		name        string
		maxNameSize int
		in          interface{}
		out         []interface{}
		filtered    bool
	}{
		// Property tests
		{
			name:        "basic",
			maxNameSize: 20,
			in: &struct {
				A *string `keep:"true"`
				B *string `keep:"true"`
				C *string
			}{},
			out: []interface{}{
				&struct {
					A *string
				}{},
				&struct {
					B *string
				}{},
			},
			filtered: true,
		},
		{
			name:        "anonymous where all match but still needs sharding",
			maxNameSize: 20,
			in: &struct {
				A *string `keep:"true"`
				B *string `keep:"true"`
				C *string `keep:"true"`
			}{},
			out: []interface{}{
				&struct {
					A *string
				}{},
				&struct {
					B *string
				}{},
				&struct {
					C *string
				}{},
			},
			filtered: true,
		},
		{
			name:        "named where all match but still needs sharding",
			maxNameSize: 20,
			in:          &KeepAllWithAReallyLongNameThatExceedsTheMaxNameSize{},
			out: []interface{}{
				&struct {
					A *string
				}{},
				&struct {
					B *string
				}{},
				&struct {
					C *string
				}{},
			},
			filtered: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, filtered := filterPropertyStruct(reflect.TypeOf(test.in), "", test.maxNameSize,
				func(field reflect.StructField, prefix string) (bool, reflect.StructField) {
					if HasTag(field, "keep", "true") {
						field.Tag = ""
						return true, field
					}
					return false, field
				})
			if filtered != test.filtered {
				t.Errorf("expected filtered %v, got %v", test.filtered, filtered)
			}
			var expected []reflect.Type
			for _, t := range test.out {
				expected = append(expected, reflect.TypeOf(t))
			}
			if !reflect.DeepEqual(out, expected) {
				t.Errorf("expected type %v, got %v", expected, out)
			}
		})
	}
}

func Test_fieldToTypeNameSize(t *testing.T) {
	tests := []struct {
		name  string
		field reflect.StructField
	}{
		{
			name: "string",
			field: reflect.StructField{
				Name: "Foo",
				Type: reflect.TypeOf(""),
			},
		},
		{
			name: "string pointer",
			field: reflect.StructField{
				Name: "Foo",
				Type: reflect.TypeOf(StringPtr("")),
			},
		},
		{
			name: "anonymous struct",
			field: reflect.StructField{
				Name: "Foo",
				Type: reflect.TypeOf(struct{ foo string }{}),
			},
		}, {
			name: "anonymous struct pointer",
			field: reflect.StructField{
				Name: "Foo",
				Type: reflect.TypeOf(&struct{ foo string }{}),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			typeName := reflect.StructOf([]reflect.StructField{test.field}).String()
			typeName = strings.TrimPrefix(typeName, "struct { ")
			typeName = strings.TrimSuffix(typeName, " }")
			if g, w := fieldToTypeNameSize(test.field, true), len(typeName); g != w {
				t.Errorf("want fieldToTypeNameSize(..., true) = %v, got %v", w, g)
			}
			if g, w := fieldToTypeNameSize(test.field, false), len(typeName)-len(test.field.Type.String()); g != w {
				t.Errorf("want fieldToTypeNameSize(..., false) = %v, got %v", w, g)
			}
		})
	}
}

func Test_filterPropertyStructFields(t *testing.T) {
	type args struct {
	}
	tests := []struct {
		name            string
		maxTypeNameSize int
		in              interface{}
		out             []interface{}
	}{
		{
			name:            "empty",
			maxTypeNameSize: -1,
			in:              struct{}{},
			out:             nil,
		},
		{
			name:            "one",
			maxTypeNameSize: -1,
			in: struct {
				A *string
			}{},
			out: []interface{}{
				struct {
					A *string
				}{},
			},
		},
		{
			name:            "two",
			maxTypeNameSize: 20,
			in: struct {
				A *string
				B *string
			}{},
			out: []interface{}{
				struct {
					A *string
				}{},
				struct {
					B *string
				}{},
			},
		},
		{
			name:            "nested",
			maxTypeNameSize: 36,
			in: struct {
				AAAAA struct {
					A string
				}
				BBBBB struct {
					B string
				}
			}{},
			out: []interface{}{
				struct {
					AAAAA struct {
						A string
					}
				}{},
				struct {
					BBBBB struct {
						B string
					}
				}{},
			},
		},
		{
			name:            "nested pointer",
			maxTypeNameSize: 37,
			in: struct {
				AAAAA *struct {
					A string
				}
				BBBBB *struct {
					B string
				}
			}{},
			out: []interface{}{
				struct {
					AAAAA *struct {
						A string
					}
				}{},
				struct {
					BBBBB *struct {
						B string
					}
				}{},
			},
		},
		{
			name:            "doubly nested",
			maxTypeNameSize: 49,
			in: struct {
				AAAAA struct {
					A struct {
						A string
					}
				}
				BBBBB struct {
					B struct {
						B string
					}
				}
			}{},
			out: []interface{}{
				struct {
					AAAAA struct {
						A struct {
							A string
						}
					}
				}{},
				struct {
					BBBBB struct {
						B struct {
							B string
						}
					}
				}{},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inType := reflect.TypeOf(test.in)
			var in []reflect.StructField
			for i := 0; i < inType.NumField(); i++ {
				in = append(in, inType.Field(i))
			}

			keep := func(field reflect.StructField, string string) (bool, reflect.StructField) {
				return true, field
			}

			// Test that maxTypeNameSize is the
			if test.maxTypeNameSize > 0 {
				correctPanic := false
				func() {
					defer func() {
						if r := recover(); r != nil {
							if _, ok := r.(cantFitPanic); ok {
								correctPanic = true
							} else {
								panic(r)
							}
						}
					}()

					_, _ = filterPropertyStructFields(in, "", test.maxTypeNameSize-1, keep)
				}()

				if !correctPanic {
					t.Errorf("filterPropertyStructFields() with size-1 should produce cantFitPanic")
				}
			}

			filteredFieldsShards, _ := filterPropertyStructFields(in, "", test.maxTypeNameSize, keep)

			var out []interface{}
			for _, filteredFields := range filteredFieldsShards {
				typ := reflect.StructOf(filteredFields)
				if test.maxTypeNameSize > 0 && len(typ.String()) > test.maxTypeNameSize {
					t.Errorf("out %q expected size <= %d, got %d",
						typ.String(), test.maxTypeNameSize, len(typ.String()))
				}
				out = append(out, reflect.Zero(typ).Interface())
			}

			if g, w := out, test.out; !reflect.DeepEqual(g, w) {
				t.Errorf("filterPropertyStructFields() want %v, got %v", w, g)
			}
		})
	}
}
