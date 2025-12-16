package compression

// TruncationReason indicates why data was truncated in a response
type TruncationReason string

const (
	// TruncMaxModules indicates truncation due to module count limit
	TruncMaxModules TruncationReason = "max-modules"

	// TruncMaxSymbols indicates truncation due to symbol count limit per module
	TruncMaxSymbols TruncationReason = "max-symbols"

	// TruncMaxItems indicates truncation due to impact item count limit
	TruncMaxItems TruncationReason = "max-items"

	// TruncMaxRefs indicates truncation due to reference count limit
	TruncMaxRefs TruncationReason = "max-refs"

	// TruncTimeout indicates truncation due to query timeout
	TruncTimeout TruncationReason = "timeout"

	// TruncBudget indicates truncation due to general budget exhaustion
	TruncBudget TruncationReason = "budget-exceeded"

	// TruncNone indicates no truncation occurred
	TruncNone TruncationReason = ""
)

// TruncationInfo tracks information about data that was truncated
type TruncationInfo struct {
	// Reason explains why truncation occurred
	Reason TruncationReason `json:"reason"`

	// OriginalCount is the total number of items before truncation
	OriginalCount int `json:"originalCount"`

	// ReturnedCount is the number of items actually returned
	ReturnedCount int `json:"returnedCount"`

	// DroppedCount is the number of items that were dropped
	DroppedCount int `json:"droppedCount"`
}

// NewTruncationInfo creates a new TruncationInfo with calculated dropped count
func NewTruncationInfo(reason TruncationReason, original, returned int) *TruncationInfo {
	dropped := original - returned
	if dropped < 0 {
		dropped = 0
	}

	return &TruncationInfo{
		Reason:        reason,
		OriginalCount: original,
		ReturnedCount: returned,
		DroppedCount:  dropped,
	}
}

// WasTruncated returns true if any data was dropped
func (t *TruncationInfo) WasTruncated() bool {
	return t != nil && t.DroppedCount > 0
}

// IsEmpty returns true if no truncation info is present
func (t *TruncationInfo) IsEmpty() bool {
	return t == nil || t.Reason == TruncNone
}

// String returns a human-readable description of the truncation
func (t *TruncationInfo) String() string {
	if t == nil || !t.WasTruncated() {
		return "no truncation"
	}

	return string(t.Reason) + ": dropped " + string(rune(t.DroppedCount)) + " of " + string(rune(t.OriginalCount)) + " items"
}
