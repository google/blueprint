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

package bootstrap

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/blueprint"
)

const logFileName = ".ninja_log"

// removeAbandonedFilesUnder removes any files that appear in the Ninja log, and
// are prefixed with one of the `under` entries, but that are not currently
// build targets, or in `exempt`
func removeAbandonedFilesUnder(ctx *blueprint.Context, config *Config,
	srcDir string, under, exempt []string) error {

	if len(under) == 0 {
		return nil
	}

	ninjaBuildDir, err := ctx.NinjaBuildDir()
	if err != nil {
		return err
	}

	targetRules, err := ctx.AllTargets()
	if err != nil {
		return fmt.Errorf("error determining target list: %s", err)
	}

	replacer := strings.NewReplacer(
		"@@SrcDir@@", srcDir,
		"@@BuildDir@@", BuildDir)
	ninjaBuildDir = replacer.Replace(ninjaBuildDir)
	targets := make(map[string]bool)
	for target := range targetRules {
		replacedTarget := replacer.Replace(target)
		targets[filepath.Clean(replacedTarget)] = true
	}
	for _, target := range exempt {
		replacedTarget := replacer.Replace(target)
		targets[filepath.Clean(replacedTarget)] = true
	}

	filePaths, err := parseNinjaLog(ninjaBuildDir, under)
	if err != nil {
		return err
	}

	for _, filePath := range filePaths {
		isTarget := targets[filePath]
		if !isTarget {
			err = removeFileAndEmptyDirs(absolutePath(filePath))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func parseNinjaLog(ninjaBuildDir string, under []string) ([]string, error) {
	logFilePath := filepath.Join(ninjaBuildDir, logFileName)
	logFile, err := os.Open(logFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer logFile.Close()

	scanner := bufio.NewScanner(logFile)

	// Check that the first line indicates that this is a Ninja log version 5
	const expectedFirstLine = "# ninja log v5"
	if !scanner.Scan() || scanner.Text() != expectedFirstLine {
		return nil, errors.New("unrecognized ninja log format")
	}

	var filePaths []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}

		const fieldSeperator = "\t"
		fields := strings.Split(line, fieldSeperator)

		const precedingFields = 3
		const followingFields = 1

		if len(fields) < precedingFields+followingFields+1 {
			return nil, fmt.Errorf("log entry has too few fields: %q", line)
		}

		start := precedingFields
		end := len(fields) - followingFields
		filePath := strings.Join(fields[start:end], fieldSeperator)

		for _, dir := range under {
			if strings.HasPrefix(filePath, dir) {
				filePaths = append(filePaths, filePath)
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return filePaths, nil
}

func removeFileAndEmptyDirs(path string) error {
	err := os.Remove(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		pathErr := err.(*os.PathError)
		switch pathErr.Err {
		case syscall.ENOTEMPTY, syscall.EEXIST, syscall.ENOTDIR:
			return nil
		}
		return err
	}
	fmt.Printf("removed old ninja-created file %s because it has no rule to generate it\n", path)

	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	for dir := filepath.Dir(path); dir != cwd; dir = filepath.Dir(dir) {
		err = os.Remove(dir)
		if err != nil {
			pathErr := err.(*os.PathError)
			switch pathErr.Err {
			case syscall.ENOTEMPTY, syscall.EEXIST:
				// We've come to a nonempty directory, so we're done.
				return nil
			default:
				return err
			}
		}
	}

	return nil
}
