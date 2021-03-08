// Copyright 2016 Google Inc. All rights reserved.
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

import "strings"

// NinjaEscapeList takes a slice of strings that may contain characters that are meaningful to ninja
// ($), and escapes each string so they will be passed to bash.  It is not necessary on input,
// output, or dependency names, those are handled by ModuleContext.Build.  It is generally required
// on strings from properties in Blueprint files that are used as Args to ModuleContext.Build.  A
// new slice containing the escaped strings is returned.
func NinjaEscapeList(slice []string) []string {
	slice = append([]string(nil), slice...)
	for i, s := range slice {
		slice[i] = NinjaEscape(s)
	}
	return slice
}

// NinjaEscapeList takes a string that may contain characters that are meaningful to ninja
// ($), and escapes it so it will be passed to bash.  It is not necessary on input,
// output, or dependency names, those are handled by ModuleContext.Build.  It is generally required
// on strings from properties in Blueprint files that are used as Args to ModuleContext.Build.  A
// new slice containing the escaped strings is returned.
func NinjaEscape(s string) string {
	return ninjaEscaper.Replace(s)
}

var ninjaEscaper = strings.NewReplacer(
	"$", "$$")

// ShellEscapeList takes a slice of strings that may contain characters that are meaningful to bash and
// escapes them if necessary by wrapping them in single quotes, and replacing internal single quotes with
// '\'' (one single quote to end the quoting, a shell-escaped single quote to insert a real single
// quote, and then a single quote to restarting quoting.  A new slice containing the escaped strings
// is returned.
func ShellEscapeList(slice []string) []string {
	slice = append([]string(nil), slice...)

	for i, s := range slice {
		slice[i] = ShellEscape(s)
	}
	return slice

}

func shellUnsafeChar(r rune) bool {
	switch {
	case 'A' <= r && r <= 'Z',
		'a' <= r && r <= 'z',
		'0' <= r && r <= '9',
		r == '_',
		r == '+',
		r == '-',
		r == '=',
		r == '.',
		r == ',',
		r == '/':
		return false
	default:
		return true
	}
}

// ShellEscape takes string that may contain characters that are meaningful to bash and
// escapes it if necessary by wrapping it in single quotes, and replacing internal single quotes with
// '\'' (one single quote to end the quoting, a shell-escaped single quote to insert a real single
// quote, and then a single quote to restarting quoting.
func ShellEscape(s string) string {
	shellUnsafeCharNotSpace := func(r rune) bool {
		return r != ' ' && shellUnsafeChar(r)
	}

	if strings.IndexFunc(s, shellUnsafeCharNotSpace) == -1 {
		// No escaping necessary
		return s
	}

	return `'` + singleQuoteReplacer.Replace(s) + `'`
}

// ShellEscapeIncludingSpaces escapes the input `s` in a similar way to ShellEscape except that
// this treats spaces as meaningful characters.
func ShellEscapeIncludingSpaces(s string) string {
	if strings.IndexFunc(s, shellUnsafeChar) == -1 {
		// No escaping necessary
		return s
	}

	return `'` + singleQuoteReplacer.Replace(s) + `'`
}

func NinjaAndShellEscapeList(slice []string) []string {
	return ShellEscapeList(NinjaEscapeList(slice))
}

func NinjaAndShellEscape(s string) string {
	return ShellEscape(NinjaEscape(s))
}

var singleQuoteReplacer = strings.NewReplacer(`'`, `'\''`)
