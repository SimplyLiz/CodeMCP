package compliance

import (
	"bufio"
	"os"
	"path/filepath"
)

// ScanFileLines opens a file and calls fn for each line with its 1-based line number.
// Handles open/defer-close lifecycle. Returns on first error or when fn returns false.
// This is the standard pattern for compliance checks that scan files line-by-line.
func ScanFileLines(repoRoot, relPath string, fn func(lineNum int, line string) bool) error {
	f, err := os.Open(filepath.Join(repoRoot, relPath))
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if !fn(lineNum, scanner.Text()) {
			break
		}
	}
	return scanner.Err()
}
