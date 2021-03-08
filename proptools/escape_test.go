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

package proptools

import (
	"os/exec"
	"testing"
)

type escapeTestCase struct {
	name string
	in   string
	out  string
}

var ninjaEscapeTestCase = []escapeTestCase{
	{
		name: "no escaping",
		in:   `test`,
		out:  `test`,
	},
	{
		name: "leading $",
		in:   `$test`,
		out:  `$$test`,
	},
	{
		name: "trailing $",
		in:   `test$`,
		out:  `test$$`,
	},
	{
		name: "leading and trailing $",
		in:   `$test$`,
		out:  `$$test$$`,
	},
}

var shellEscapeTestCase = []escapeTestCase{
	{
		name: "no escaping",
		in:   `test`,
		out:  `test`,
	},
	{
		name: "leading $",
		in:   `$test`,
		out:  `'$test'`,
	},
	{
		name: "trailing $",
		in:   `test$`,
		out:  `'test$'`,
	},
	{
		name: "leading and trailing $",
		in:   `$test$`,
		out:  `'$test$'`,
	},
	{
		name: "single quote",
		in:   `'`,
		out:  `''\'''`,
	},
	{
		name: "multiple single quote",
		in:   `''`,
		out:  `''\'''\'''`,
	},
	{
		name: "double quote",
		in:   `""`,
		out:  `'""'`,
	},
	{
		name: "ORIGIN",
		in:   `-Wl,--rpath,${ORIGIN}/../bionic-loader-test-libs`,
		out:  `'-Wl,--rpath,${ORIGIN}/../bionic-loader-test-libs'`,
	},
}

var shellEscapeIncludingSpacesTestCase = []escapeTestCase{
	{
		name: "no escaping",
		in:   `test`,
		out:  `test`,
	},
	{
		name: "spacing",
		in:   `arg1 arg2`,
		out:  `'arg1 arg2'`,
	},
	{
		name: "single quote",
		in:   `'arg'`,
		out:  `''\''arg'\'''`,
	},
}

func TestNinjaEscaping(t *testing.T) {
	for _, testCase := range ninjaEscapeTestCase {
		got := NinjaEscape(testCase.in)
		if got != testCase.out {
			t.Errorf("%s: expected `%s` got `%s`", testCase.name, testCase.out, got)
		}
	}
}

func TestShellEscaping(t *testing.T) {
	for _, testCase := range shellEscapeTestCase {
		got := ShellEscape(testCase.in)
		if got != testCase.out {
			t.Errorf("%s: expected `%s` got `%s`", testCase.name, testCase.out, got)
		}
	}
}

func TestShellEscapeIncludingSpaces(t *testing.T) {
	for _, testCase := range shellEscapeIncludingSpacesTestCase {
		got := ShellEscapeIncludingSpaces(testCase.in)
		if got != testCase.out {
			t.Errorf("%s: expected `%s` got `%s`", testCase.name, testCase.out, got)
		}
	}
}

func TestExternalShellEscaping(t *testing.T) {
	if testing.Short() {
		return
	}
	for _, testCase := range shellEscapeTestCase {
		cmd := "echo -n " + ShellEscape(testCase.in)
		got, err := exec.Command("/bin/sh", "-c", cmd).Output()
		if err != nil {
			t.Error(err)
		}
		if string(got) != testCase.in {
			t.Errorf("%s: expected `%s` got `%s`", testCase.name, testCase.in, got)
		}
	}
}

func TestExternalShellEscapeIncludingSpaces(t *testing.T) {
	if testing.Short() {
		return
	}
	for _, testCase := range shellEscapeIncludingSpacesTestCase {
		cmd := "echo -n " + ShellEscapeIncludingSpaces(testCase.in)
		got, err := exec.Command("/bin/sh", "-c", cmd).Output()
		if err != nil {
			t.Error(err)
		}
		if string(got) != testCase.in {
			t.Errorf("%s: expected `%s` got `%s`", testCase.name, testCase.in, got)
		}
	}
}
