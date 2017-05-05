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
	"testing"
	"text/scanner"

	"github.com/google/blueprint/proptools"
	"io/ioutil"
	"os"
	"path/filepath"
)

// parser_test.go isn't allowed to use the printer in its tests because printer_test.go uses the parser in its tests

// Assuming that the two trees have equivalent structure, this function returns a map that translates non-comment nodes in <actual> to <expected>
// The behavior is undefined if the two trees don't have equal structure between their non-comment nodes
func getNodeMappingWithoutComments(expected *SyntaxTree, actual *SyntaxTree) (remappings map[ParseNode]ParseNode) {
	actualNodes := actual.ListOfAllNodes()
	expectedNodes := expected.ListOfAllNodes()
	remappings = make(map[ParseNode]ParseNode, 0)
	if len(actualNodes) != len(expectedNodes) {
		panic(fmt.Errorf("Illegal usage of getNodeMappingWithoutComments. getNodeMappingWithoutComments may only be executed against two trees with equivalent structure between their non-comment nodes. "+
			"Actual number of nodes: %v nodes in tree %v and %v nodes in tree %v .",
			len(actualNodes), actualNodes, len(expectedNodes), expectedNodes))
	}
	count := 0
	for i, actualNode := range actualNodes {
		count++
		analogue := expectedNodes[i]
		if actualNode != nil && analogue != nil {
			_, exists := remappings[actualNode]
			if exists {
				panic(fmt.Sprintf("node %s(%#v) already found in remapping", actualNode, actualNode))
			}
			remappings[actualNode] = analogue
		} else {
			panic(fmt.Sprintf("why do the nodes actual=%#v and expected=%#v have a nil (index %v)?", actualNode, expectedNodes, i))
		}
	}
	if count != len(remappings) {
		panic(fmt.Sprintf("i length mismatch, got %v, expected %v", len(remappings), count))
	}
	if len(remappings) != len(actualNodes) {
		panic(fmt.Sprintf("length mismatch, got %v, expected %v", len(remappings), len(actualNodes)))
	}
	return remappings
}

// Assuming that the two trees have equivalent structure, this function returns a map that translates nodes in <actual> to <expected>
// This includes attached comments
// The behavior is undefined if the two trees don't have equal structure between their non-comment nodes
func getNodeMappingWithComments(expected *SyntaxTree, actual *SyntaxTree, existingRemappings map[ParseNode]ParseNode) map[ParseNode]ParseNode {
	remappings := existingRemappings
	for _, actualNode := range actual.ListOfAllNodes() {
		expectedNode := existingRemappings[actualNode]
		actualComments := actual.GetCommentsIfPresent(actualNode)
		expectedComments := expected.GetCommentsIfPresent(expectedNode)
		if actualComments != nil && expectedComments != nil {
			if len(actualComments.preComments) != len(expectedComments.preComments) {
				panic(fmt.Sprintf(
					"Illegal usage of getNodeMappingWithComments. Expected node %s has %v "+
						"pre-comments whereas actual node %s has %v",
					expectedNode, len(expectedComments.preComments),
					actualNode, len(actualComments.preComments)))
			}
			for i := range actualComments.preComments {
				remappings[actualComments.preComments[i]] = expectedComments.preComments[i]
			}
			if len(actualComments.postComments) != len(expectedComments.postComments) {
				panic(fmt.Sprintf(
					"Illegal usage of getNodeMappingWithComments. Expected node %s has %v "+
						"post-comments whereas actual node %s has %v",
					expectedNode, len(expectedComments.postComments),
					actualNode, len(actualComments.postComments)))
			}
			for i := range actualComments.postComments {
				remappings[actualComments.postComments[i]] = expectedComments.postComments[i]
			}
		}
	}
	return remappings
}

// Tells whether the attached comments are equivalent between the two trees, when applying the given remapping
func compareComments(expected *ParseTree, actual *ParseTree,
	actualToExpected map[ParseNode]ParseNode) (equal bool, difference string) {
	// Construct a new map of comments, whose values are the same as <actual>, but whose keys are the corresponding keys in <expected>
	// The reason we replace the keys is so proptools.DeepCompare can know which keys we want to compare against which others

	analogousActualComments := make(map[ParseNode](*CommentPair), 0)
	for actualKey, actualValue := range actual.SyntaxTree.comments {
		expectedKey := actualToExpected[actualKey]
		if actualKey != nil && expectedKey != nil { // we don't attach comments to a nil node
			analogousActualComments[expectedKey] = actualValue
		}
	}

	// now compare the comments
	return proptools.DeepCompare(
		"expected.SyntaxTree.Comments", expected.SyntaxTree.comments,
		"analogous actual.SyntaxTree.Comments", analogousActualComments)
}

// Tells whether the source positions are equivalent between the two trees, when applying the given remapping
func compareSourcePositions(expected *ParseTree, actual *ParseTree,
	actualToExpected map[ParseNode]ParseNode) (equal bool, difference string) {
	// Construct a new map of source positions, whose values are the same as <actual>, but whose keys are the corresponding keys in <expected>
	// The reason we replace the keys is so proptools.DeepCompare can know which keys we want to compare against which others

	analogousSourcePositions := make(map[ParseNode]scanner.Position)
	for actualKey, actualValue := range actual.SourcePositions {
		expectedKey := actualToExpected[actualKey]
		if actualKey != nil && expectedKey != nil { // we don't save the position of a nil node
			analogousSourcePositions[expectedKey] = actualValue
		}
	}

	// now compare the source positions
	return proptools.DeepCompare(
		"expected.SourcePositions", expected.SourcePositions,
		"analogous actual.SourcePositions", analogousSourcePositions)
}

func compareParseTrees(expected *ParseTree, actual *ParseTree) (equal bool, difference string) {
	// first compare all nodes other than attached comments
	equal, difference = proptools.DeepCompare(
		"expected.SyntaxTree.Nodes", expected.SyntaxTree.nodes,
		"actual.SyntaxTree.Nodes", actual.SyntaxTree.nodes)
	if !equal {
		return equal, difference
	}

	// now that we've confirmed that the non-comment nodes have equivalent structure, we can compute a node mapping
	remappings := getNodeMappingWithoutComments(expected.SyntaxTree, actual.SyntaxTree)

	// use the node mapping to compare the comments
	equal, difference = compareComments(expected, actual, remappings)
	if !equal {
		return equal, difference
	}

	remappings = getNodeMappingWithComments(expected.SyntaxTree, actual.SyntaxTree, remappings)

	return compareSourcePositions(expected, actual, remappings)
}

func runValidTestCase(t *testing.T, testCase parserTestCase) {
	var succeeded = false
	defer func() {
		if !succeeded {
			t.Errorf("test case %s failed with input: \n%s\n", testCase.name, testCase.input)
		}
	}()
	r := bytes.NewBufferString(testCase.input)
	actualFileParse, errs := ParseAndEval(testCase.name, r, NewScope(nil))
	actualParse := actualFileParse
	if len(errs) != 0 {
		t.Errorf("test case: %s", testCase.input)
		t.Error("unexpected errors:")
		for _, err := range errs {
			t.Errorf("  %s", err)
		}
		t.FailNow()
	}

	correctParse := testCase.treeProvider(testCase.name)
	// TODO update all the test cases to specify SourcePositions, and then just use that

	// confirm that the actual and expected trees are equivalent
	equal, difference := compareParseTrees(correctParse, actualParse)

	expectedRepresentation := VerbosePrint(correctParse)
	actualRepresentation := VerbosePrint(actualParse)
	if equal {
		succeeded = true
	} else {
		messageTemplate := `
test case: %s
with input:
                %s
expected:
%s
got     :
%s
1st diff: %v

`
		t.Errorf(messageTemplate, testCase.name, testCase.input, expectedRepresentation, actualRepresentation, difference)
	}
}

// this function runs the tests
func TestParseValidInput(t *testing.T) {
	testNames := make(map[string]bool, len(parserTestCases))
	for i := range parserTestCases {
		testCase := parserTestCases[i]
		name := testCase.name
		if _, found := testNames[name]; found {
			t.Fatalf("Duplicate test case name %s", name)
		}
		testNames[name] = true
		runValidTestCase(t, testCase)
	}
}

// TODO: Test error strings

// This function confirms that the test cases in parser_test_cases.go can be autogenerated from parser_test_inputs.go
// This is checked so users don't accidentally edit parser_test_cases.go directly
func TestAutogeneratedTestCasesMatch(t *testing.T) {
	testCasesPath := "parser_test_cases.go"
	actualBytes, err := ioutil.ReadFile(testCasesPath)
	actualText := string(actualBytes)
	if err != nil {
		t.Errorf("failed to open %s", testCasesPath)
	}
	generatedText := generateParserTestCasesDotGo()
	generatedBytes := []byte(generatedText)
	if actualText != generatedText {
		// save generated file and tell the user to copy it
		generatedPath := filepath.Join(os.TempDir(), "parser_test_cases.go.generated")
		ioutil.WriteFile(generatedPath, generatedBytes, os.FileMode(0777))
		separator := "\n\n\n***\n\n\n"
		t.Errorf("%sparser_test_cases.go DOES NOT MATCH THE RESULT GENERATED BY parser_test_generator.go .\n"+
			"Do `diff %q %q` to inspect the proposed changes\n"+
			"Do `cp %q %q`   to accept  the proposed changes%s",
			separator, testCasesPath, generatedPath, generatedPath, testCasesPath, separator)
	}
}
