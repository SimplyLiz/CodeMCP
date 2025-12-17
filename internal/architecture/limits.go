package architecture

import (
	"fmt"
	"time"
)

// ArchitectureLimits defines performance and resource limits for architecture generation
// Section 15.3: Performance targets
// - Architecture (1000 files) < 30s
// - Import scan (1000 files) < 10s
type ArchitectureLimits struct {
	MaxFilesScanned int           // Maximum files to scan across all modules (default 5000)
	MaxModules      int           // Maximum modules to include in response (from budget)
	ScanTimeout     time.Duration // Maximum time for architecture scan (default 30s)
}

// DefaultLimits returns the default architecture limits
func DefaultLimits() *ArchitectureLimits {
	return &ArchitectureLimits{
		MaxFilesScanned: 5000,
		MaxModules:      100, // Generous default; actual limit from budget config
		ScanTimeout:     30 * time.Second,
	}
}

// checkLimits validates that the file count is within acceptable limits (kept for future use)
var _ = (*ArchitectureLimits).checkLimits

func (l *ArchitectureLimits) checkLimits(fileCount int) error {
	if fileCount > l.MaxFilesScanned {
		return fmt.Errorf("file count %d exceeds maximum limit %d", fileCount, l.MaxFilesScanned)
	}
	return nil
}

// checkModuleCount validates that the module count is within acceptable limits
func (l *ArchitectureLimits) checkModuleCount(moduleCount int) error {
	if moduleCount > l.MaxModules {
		return fmt.Errorf("module count %d exceeds maximum limit %d", moduleCount, l.MaxModules)
	}
	return nil
}
