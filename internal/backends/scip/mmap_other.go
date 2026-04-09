//go:build !unix

package scip

import (
	"fmt"
	"os"
)

// mapFile falls back to os.ReadFile on non-Unix platforms.
func mapFile(path string) (data []byte, cleanup func(), err error) {
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read: %w", err)
	}
	return data, func() {}, nil
}
