package deptools

import (
	"fmt"
	"os"
	"strings"
)

// WriteDepFile creates a new gcc-style depfile and populates it with content
// indicating that target depends on deps.
func WriteDepFile(filename, target string, deps []string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%s: \\\n %s\n", target,
		strings.Join(deps, " \\\n "))
	if err != nil {
		return err
	}

	return nil
}
