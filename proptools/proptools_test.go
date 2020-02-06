// Copyright 2020 Google Inc. All rights reserved.
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

import "testing"

func TestPropertyNameForField(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "short",
			input: "S",
			want:  "s",
		},
		{
			name:  "long",
			input: "String",
			want:  "string",
		},
		{
			name:  "uppercase",
			input: "STRING",
			want:  "STRING",
		},
		{
			name:  "mixed",
			input: "StRiNg",
			want:  "stRiNg",
		},
		{
			name:  "underscore",
			input: "Under_score",
			want:  "under_score",
		},
		{
			name:  "uppercase underscore",
			input: "UNDER_SCORE",
			want:  "UNDER_SCORE",
		},
		{
			name:  "x86",
			input: "X86",
			want:  "x86",
		},
		{
			name:  "x86_64",
			input: "X86_64",
			want:  "x86_64",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PropertyNameForField(tt.input); got != tt.want {
				t.Errorf("PropertyNameForField(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFieldNameForProperty(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "short lowercase",
			input: "s",
			want:  "S",
		},
		{
			name:  "short uppercase",
			input: "S",
			want:  "S",
		},
		{
			name:  "long lowercase",
			input: "string",
			want:  "String",
		},
		{
			name:  "long uppercase",
			input: "STRING",
			want:  "STRING",
		},
		{
			name:  "mixed",
			input: "StRiNg",
			want:  "StRiNg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FieldNameForProperty(tt.input); got != tt.want {
				t.Errorf("FieldNameForProperty(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
