package backends

// BackendResult represents a raw result from a single backend
type BackendResult struct {
	// BackendID identifies which backend produced this result
	BackendID BackendID

	// Data contains the actual result data (type depends on query)
	Data interface{}

	// Completeness tracks result quality
	Completeness CompletenessInfo

	// DurationMs is how long this backend took to respond
	DurationMs int64

	// Error contains error information if the query failed
	Error error
}

// BackendContribution tracks a backend's contribution to a merged result
type BackendContribution struct {
	// BackendID identifies the backend
	BackendID BackendID

	// ItemCount is how many items this backend contributed
	ItemCount int

	// DurationMs is how long this backend took
	DurationMs int64

	// WasUsed indicates if this backend's data was used in the final result
	WasUsed bool

	// Error contains error message if the backend failed
	Error string
}

// QueryRequest represents a request to query backends
type QueryRequest struct {
	// Type specifies the query type (symbol, references, search, etc.)
	Type QueryType

	// SymbolID for symbol-specific queries
	SymbolID string

	// Query for search queries
	Query string

	// SearchOpts for symbol search
	SearchOpts *SearchOptions

	// RefOpts for reference queries
	RefOpts *RefOptions
}

// QueryType identifies the type of query
type QueryType string

const (
	// QueryTypeSymbol retrieves symbol information
	QueryTypeSymbol QueryType = "symbol"

	// QueryTypeSearch searches for symbols
	QueryTypeSearch QueryType = "search"

	// QueryTypeReferences finds references to a symbol
	QueryTypeReferences QueryType = "references"
)

// QueryResult represents the final merged result from one or more backends
type QueryResult struct {
	// Data contains the merged result data
	Data interface{}

	// Completeness of the merged result
	Completeness CompletenessInfo

	// Contributions tracks which backends were queried and their results
	Contributions []BackendContribution

	// Provenance tracks how the result was assembled
	Provenance Provenance

	// TotalDurationMs is the total time taken (including parallelization)
	TotalDurationMs int64
}

// Provenance tracks how a result was assembled from backends
type Provenance struct {
	// PrimaryBackend is the backend that provided the primary result
	PrimaryBackend BackendID

	// SupplementBackends lists backends that supplemented the result
	SupplementBackends []BackendID

	// MergeMode describes how results were merged
	MergeMode MergeMode

	// MetadataConflicts tracks fields where backends disagreed
	MetadataConflicts []MetadataConflict

	// UnionConflicts tracks conflicts in union merge mode
	UnionConflicts []UnionConflict
}

// MetadataConflict represents a disagreement between backends about metadata
type MetadataConflict struct {
	// Field is the name of the conflicting field
	Field string

	// Values maps backend ID to the value it provided
	Values map[BackendID]interface{}

	// Resolved is the value that was ultimately used
	Resolved interface{}
}

// UnionConflict represents a conflict when merging results in union mode
type UnionConflict struct {
	// Field is the conflicting field
	Field string

	// ItemID identifies the item (e.g., symbol stable ID)
	ItemID string

	// BackendValues maps backend ID to value
	BackendValues map[BackendID]interface{}

	// Resolution describes how the conflict was resolved
	Resolution string
}

// MergeMode specifies how to merge results from multiple backends
type MergeMode string

const (
	// MergeModePreferFirst uses the highest-priority backend's result,
	// supplementing only metadata from equal-or-higher priority backends
	MergeModePreferFirst MergeMode = "prefer-first"

	// MergeModeUnion queries all backends and merges all results,
	// resolving conflicts by backend precedence
	MergeModeUnion MergeMode = "union"
)
