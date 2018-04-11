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
	"fmt"
	"io"
	"math"
	"sort"
)

func AddStringToList(list *List, s string) (modified bool) {
	for _, v := range list.Values {
		if v.Type() != StringType {
			panic(fmt.Errorf("expected string in list, got %s", v.Type()))
		}

		if sv, ok := v.(*String); ok && sv.Value == s {
			// string already exists
			return false
		}
	}

	list.Values = append(list.Values, &String{
		LiteralPos: list.RBracePos,
		Value:      s,
	})

	return true
}

func RemoveStringFromList(list *List, s string) (modified bool) {
	for i, v := range list.Values {
		if v.Type() != StringType {
			panic(fmt.Errorf("expected string in list, got %s", v.Type()))
		}

		if sv, ok := v.(*String); ok && sv.Value == s {
			list.Values = append(list.Values[:i], list.Values[i+1:]...)
			return true
		}
	}

	return false
}

// A Patch represents a region of a text buffer to be replaced [Start, End) and its Replacement
type Patch struct {
	Start, End  int
	Replacement string
}

// A PatchList is a list of sorted, non-overlapping Patch objects
type PatchList []Patch

type PatchOverlapError error

// Add adds a Patch to a PatchList.  It returns a PatchOverlapError if the patch cannot be added.
func (list *PatchList) Add(start, end int, replacement string) error {
	patch := Patch{start, end, replacement}
	if patch.Start > patch.End {
		return fmt.Errorf("invalid patch, start %d is after end %d", patch.Start, patch.End)
	}
	for _, p := range *list {
		if (patch.Start >= p.Start && patch.Start < p.End) ||
			(patch.End >= p.Start && patch.End < p.End) ||
			(p.Start >= patch.Start && p.Start < patch.End) ||
			(p.Start == patch.Start && p.End == patch.End) {
			return PatchOverlapError(fmt.Errorf("new patch %d-%d overlaps with existing patch %d-%d",
				patch.Start, patch.End, p.Start, p.End))
		}
	}
	*list = append(*list, patch)
	list.sort()
	return nil
}

func (list *PatchList) sort() {
	sort.SliceStable(*list,
		func(i, j int) bool {
			return (*list)[i].Start < (*list)[j].Start
		})
}

// Apply applies all the Patch objects in PatchList to the data from an input ReaderAt to an output Writer.
func (list *PatchList) Apply(in io.ReaderAt, out io.Writer) error {
	var offset int64
	for _, patch := range *list {
		toWrite := int64(patch.Start) - offset
		written, err := io.Copy(out, io.NewSectionReader(in, offset, toWrite))
		if err != nil {
			return err
		}
		offset += toWrite
		if written != toWrite {
			return fmt.Errorf("unexpected EOF at %d", offset)
		}

		_, err = io.WriteString(out, patch.Replacement)
		if err != nil {
			return err
		}

		offset += int64(patch.End - patch.Start)
	}
	_, err := io.Copy(out, io.NewSectionReader(in, offset, math.MaxInt64-offset))
	return err
}
