// Copyright 2015 Google Inc. All rights reserved.
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

package blueprint

import (
	"reflect"
	"testing"
)

var (
	testModuleA = &moduleInfo{variantName: "testModuleA"}
	testModuleB = &moduleInfo{variantName: "testModuleB"}
	testModuleC = &moduleInfo{variantName: "testModuleC"}
	testModuleD = &moduleInfo{variantName: "testModuleD"}
	testModuleE = &moduleInfo{variantName: "testModuleE"}
	testModuleF = &moduleInfo{variantName: "testModuleF"}
)

var spliceModulesTestCases = []struct {
	in         []*moduleInfo
	replace    *moduleInfo
	with       []*moduleInfo
	out        []*moduleInfo
	reallocate bool
}{
	{
		// Insert at the beginning
		in:         []*moduleInfo{testModuleA, testModuleB, testModuleC},
		replace:    testModuleA,
		with:       []*moduleInfo{testModuleD, testModuleE},
		out:        []*moduleInfo{testModuleD, testModuleE, testModuleB, testModuleC},
		reallocate: true,
	},
	{
		// Insert in the middle
		in:         []*moduleInfo{testModuleA, testModuleB, testModuleC},
		replace:    testModuleB,
		with:       []*moduleInfo{testModuleD, testModuleE},
		out:        []*moduleInfo{testModuleA, testModuleD, testModuleE, testModuleC},
		reallocate: true,
	},
	{
		// Insert at the end
		in:         []*moduleInfo{testModuleA, testModuleB, testModuleC},
		replace:    testModuleC,
		with:       []*moduleInfo{testModuleD, testModuleE},
		out:        []*moduleInfo{testModuleA, testModuleB, testModuleD, testModuleE},
		reallocate: true,
	},
	{
		// Insert over a single element
		in:         []*moduleInfo{testModuleA},
		replace:    testModuleA,
		with:       []*moduleInfo{testModuleD, testModuleE},
		out:        []*moduleInfo{testModuleD, testModuleE},
		reallocate: true,
	},
	{
		// Insert at the beginning without reallocating
		in:         []*moduleInfo{testModuleA, testModuleB, testModuleC, nil}[0:3],
		replace:    testModuleA,
		with:       []*moduleInfo{testModuleD, testModuleE},
		out:        []*moduleInfo{testModuleD, testModuleE, testModuleB, testModuleC},
		reallocate: false,
	},
	{
		// Insert in the middle without reallocating
		in:         []*moduleInfo{testModuleA, testModuleB, testModuleC, nil}[0:3],
		replace:    testModuleB,
		with:       []*moduleInfo{testModuleD, testModuleE},
		out:        []*moduleInfo{testModuleA, testModuleD, testModuleE, testModuleC},
		reallocate: false,
	},
	{
		// Insert at the end without reallocating
		in:         []*moduleInfo{testModuleA, testModuleB, testModuleC, nil}[0:3],
		replace:    testModuleC,
		with:       []*moduleInfo{testModuleD, testModuleE},
		out:        []*moduleInfo{testModuleA, testModuleB, testModuleD, testModuleE},
		reallocate: false,
	},
	{
		// Insert over a single element without reallocating
		in:         []*moduleInfo{testModuleA, nil}[0:1],
		replace:    testModuleA,
		with:       []*moduleInfo{testModuleD, testModuleE},
		out:        []*moduleInfo{testModuleD, testModuleE},
		reallocate: false,
	},
}

func TestSpliceModules(t *testing.T) {
	for _, testCase := range spliceModulesTestCases {
		in := make([]*moduleInfo, len(testCase.in), cap(testCase.in))
		copy(in, testCase.in)
		origIn := in
		got := spliceModules(in, testCase.replace, testCase.with)
		if !reflect.DeepEqual(got, testCase.out) {
			t.Errorf("test case: %v, %v -> %v", testCase.in, testCase.replace, testCase.with)
			t.Errorf("incorrect output:")
			t.Errorf("  expected: %v", testCase.out)
			t.Errorf("       got: %v", got)
		}
		if sameArray(origIn, got) != !testCase.reallocate {
			t.Errorf("test case: %v, %v -> %v", testCase.in, testCase.replace, testCase.with)
			not := ""
			if !testCase.reallocate {
				not = " not"
			}
			t.Errorf("  expected to%s reallocate", not)
		}
	}
}

func sameArray(a, b []*moduleInfo) bool {
	return &a[0:cap(a)][cap(a)-1] == &b[0:cap(b)][cap(b)-1]
}
