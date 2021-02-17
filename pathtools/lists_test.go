// Copyright 2021 Google Inc. All rights reserved.
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
package pathtools

import (
	"testing"
)

func TestLists_ReplaceExtension(t *testing.T) {

	testCases := []struct {
		from, ext, to string
	}{
		{"1.jpg", "png", "1.png"},
		{"1", "png", "1.png"},
		{"1.", "png", "1.png"},
		{"2.so", "so.1", "2.so.1"},
		{"/out/.test/1.png", "jpg", "/out/.test/1.jpg"},
		{"/out/.test/1", "jpg", "/out/.test/1.jpg"},
	}

	for _, test := range testCases {
		t.Run(test.from, func(t *testing.T) {
			got := ReplaceExtension(test.from, test.ext)
			if got != test.to {
				t.Errorf("ReplaceExtension(%v, %v) = %v; want: %v", test.from, test.ext, got, test.to)
			}
		})
	}
}
