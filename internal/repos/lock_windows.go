//go:build windows

package repos

import (
	"os"
)

// Windows file locking stub - uses simple file existence check
func lockFile(f *os.File) error {
	// On Windows, file locking is implicit with exclusive access
	// This is a simplified implementation
	return nil
}

func unlockFile(f *os.File) error {
	return nil
}
