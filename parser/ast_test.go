// Copyright 2017 Google Inc. All rights reserved.
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
	"reflect"
	"testing"
)

// The purpose of this file is to test operations that act on the abstract syntax tree, which are used in parser_test.go
// Currently this only entails testing comments because comments are the only entities that have any noteworthy logic to be tested

func assertEqual(t *testing.T, actual interface{}, expected interface{}, description string) {
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Unexpected %v; expected %#v, got %#v", description, expected, actual)
	}
}

func TestSingleComment(t *testing.T) {
	var tree = NewSyntaxTree()
	var comment = NewFullLineComment("value1")
	tree.AddNode(comment)
	var numNodes = len(tree.nodes)
	if len(tree.nodes) != 1 {
		t.Errorf("Incorrect number of nodes, expected 1, got %v\n", numNodes)
		t.FailNow()
	}
	var firstNode = tree.nodes[0]
	if firstNode != comment {
		t.Errorf("Incorrect node, expected %#v, got %#v\n", comment, firstNode)
		t.FailNow()
	}
}

func TestManyComments(t *testing.T) {
	// make some non-comment nodes
	var one = &String{"1"}
	var two = &String{"2"}
	var firstPlus = &String{"+"}
	var onePlusTwo = &Operator{Args: [2]Expression{one, two}, OperatorToken: *firstPlus}
	var firstIsOnePlusTwo = &Property{Name: "first", Value: onePlusTwo}

	var aList = &List{Values: []Expression{&String{"a"}}}
	var bList = &List{Values: []Expression{&String{"b"}}}
	var aPlusB = &Operator{Args: [2]Expression{aList, bList}, OperatorToken: String{"+"}}
	var secondIsAPlusB = &Property{Name: "second", Value: aPlusB}

	var mapBody = &MapBody{Properties: []*Property{firstIsOnePlusTwo, secondIsAPlusB}}
	var propertyMap = Map{*mapBody}

	var module = &Module{&String{"MyFirstModule"}, propertyMap}

	var tree = NewSyntaxTree()

	// start adding nodes into the tree
	var fileComment1 = NewFullLineComment("file comment 1")
	tree.AddNode(fileComment1)
	var fileComment2 = NewInlineComment("file comment 2")
	tree.AddNode(fileComment2)

	tree.AddNode(module)

	var footerComment1 = NewFullLineComment("footer comment 1")
	tree.AddNode(footerComment1)
	var footerComment2 = NewFullLineComment("footer comment 2")
	tree.AddNode(footerComment2)
	assertEqual(t, len(tree.nodes), 5, "node count")

	var postModuleComment1 = NewInlineComment("module comment")
	tree.GetComments(module.Type).AddPostComment(postModuleComment1)
	var postModuleComment2 = NewInlineComment("module comment")
	tree.GetComments(module.Type).AddPostComment(postModuleComment2)
	assertEqual(t, tree.GetComments(module.Type).postComments, [](*Comment){postModuleComment1, postModuleComment2}, "module comments")

	var mapTopComment = NewFullLineComment("comment at top of map")
	tree.GetComments(mapBody).AddPreComment(mapTopComment)
	assertEqual(t, tree.GetComments(mapBody).preComments, [](*Comment){mapTopComment}, "map top comments")
	var mapBottomComment = NewFullLineComment("comment at bottom of map")
	tree.GetComments(mapBody).AddPostComment(mapBottomComment)
	assertEqual(t, tree.GetComments(mapBody).postComments, [](*Comment){mapBottomComment}, "map bottom comments")

	var preOneComment = NewInlineComment("starting 'one'")
	tree.GetComments(one).AddPreComment(preOneComment)
	assertEqual(t, tree.GetComments(one).preComments, [](*Comment){preOneComment}, "comments before 'one'")
	var postOneComment = NewInlineComment("ending 'one'")
	tree.GetComments(one).AddPostComment(postOneComment)
	assertEqual(t, tree.GetComments(one).preComments, [](*Comment){preOneComment}, "comments after 'one'")

	var prePlusOne = NewInlineComment("starting first '+'")
	tree.GetComments(firstPlus).AddPreComment(prePlusOne)
	assertEqual(t, tree.GetComments(firstPlus).preComments, [](*Comment){prePlusOne}, "comments before first '+'")
	var postPlusOne = NewInlineComment("ending first '+'")
	tree.GetComments(firstPlus).AddPostComment(postPlusOne)
	assertEqual(t, tree.GetComments(firstPlus).postComments, [](*Comment){postPlusOne}, "comments after first '+'")

	var preTwoComment = NewInlineComment("starting 'two'")
	tree.GetComments(two).AddPreComment(preTwoComment)
	assertEqual(t, tree.GetComments(two).preComments, [](*Comment){preTwoComment}, "comments before 'two'")
	var postTwoComment = NewInlineComment("ending 'two'")
	tree.GetComments(two).AddPostComment(postTwoComment)
	assertEqual(t, tree.GetComments(two).postComments, [](*Comment){postTwoComment}, "comments after 'two'")

	// it's getting pretty verbose adding all these comments programatically, so let's just skip to the list and only add a couple comments there
	var preAComment = NewInlineComment("starting 'a'")
	tree.GetComments(aList).AddPreComment(preAComment)
	assertEqual(t, tree.GetComments(aList).preComments, [](*Comment){preAComment}, "comments before \"['a']\"")
	var postAComment = NewInlineComment("ending 'a'")
	tree.GetComments(aList).AddPostComment(postAComment)
	assertEqual(t, tree.GetComments(aList).postComments, [](*Comment){postAComment}, "comments after \"['a']\"")

}

// TODO: Test error cases
