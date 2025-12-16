package git

// Backend represents a generic backend interface
// This will be extended as the backend system evolves
type Backend interface {
	// ID returns the unique identifier for this backend
	ID() string

	// IsAvailable checks if the backend is available and functional
	IsAvailable() bool

	// Capabilities returns a list of capability identifiers this backend supports
	Capabilities() []string
}

// GitBackend extends Backend with Git-specific query capabilities
type GitBackend interface {
	Backend

	// GetFileHistory returns the commit history for a specific file
	GetFileHistory(path string, limit int) (*FileHistory, error)

	// GetFileChurn returns churn metrics for a specific file
	GetFileChurn(path string, since string) (*ChurnMetrics, error)

	// GetHotspots returns the top files with highest churn
	GetHotspots(limit int, since string) ([]ChurnMetrics, error)

	// GetStagedDiff returns statistics for staged changes
	GetStagedDiff() ([]DiffStats, error)

	// GetWorkingTreeDiff returns statistics for working tree changes
	GetWorkingTreeDiff() ([]DiffStats, error)

	// GetUntrackedFiles returns list of untracked files
	GetUntrackedFiles() ([]string, error)
}
