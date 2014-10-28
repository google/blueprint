package bootstrap

import (
	"blueprint"
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const logFileName = ".ninja_log"

// removeAbandonedFiles removes any files that appear in the Ninja log that are
// not currently build targets.
func removeAbandonedFiles(ctx *blueprint.Context, config interface{}) error {
	buildDir := "."
	if generatingBootstrapper(config) {
		buildDir = bootstrapDir
	}

	targets, err := ctx.AllTargets()
	if err != nil {
		fatalf("error determining target list: %s", err)
	}

	filePaths, err := parseNinjaLog(buildDir)
	if err != nil {
		return err
	}

	for _, filePath := range filePaths {
		_, isTarget := targets[filePath]
		if !isTarget {
			err = removeFileAndEmptyDirs(filePath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func parseNinjaLog(buildDir string) ([]string, error) {
	logFilePath := filepath.Join(buildDir, logFileName)
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

		filePaths = append(filePaths, filePath)
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
		return err
	}

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
