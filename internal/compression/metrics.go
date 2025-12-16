package compression

// CompressionMetrics tracks statistics about compression operations
type CompressionMetrics struct {
	// InputCount is the number of items before compression
	InputCount int `json:"inputCount"`

	// OutputCount is the number of items after compression
	OutputCount int `json:"outputCount"`

	// CompressionRatio is the ratio of output to input (output/input)
	// A ratio of 1.0 means no compression, 0.5 means 50% compression
	CompressionRatio float64 `json:"compressionRatio"`

	// Truncations is a list of all truncation operations that occurred
	Truncations []TruncationInfo `json:"truncations,omitempty"`
}

// ComputeMetrics calculates compression metrics from input/output counts and truncations
func ComputeMetrics(input, output int, truncations []TruncationInfo) *CompressionMetrics {
	ratio := 0.0
	if input > 0 {
		ratio = float64(output) / float64(input)
	}

	return &CompressionMetrics{
		InputCount:       input,
		OutputCount:      output,
		CompressionRatio: ratio,
		Truncations:      truncations,
	}
}

// NewMetrics creates a new CompressionMetrics instance
func NewMetrics(input, output int) *CompressionMetrics {
	return ComputeMetrics(input, output, nil)
}

// AddTruncation adds a truncation info to the metrics
func (m *CompressionMetrics) AddTruncation(truncation *TruncationInfo) {
	if truncation != nil && truncation.WasTruncated() {
		if m.Truncations == nil {
			m.Truncations = []TruncationInfo{}
		}
		m.Truncations = append(m.Truncations, *truncation)
	}
}

// WasTruncated returns true if any truncation occurred
func (m *CompressionMetrics) WasTruncated() bool {
	return len(m.Truncations) > 0
}

// TotalDropped returns the total number of items dropped across all truncations
func (m *CompressionMetrics) TotalDropped() int {
	total := 0
	for _, t := range m.Truncations {
		total += t.DroppedCount
	}
	return total
}

// CompressionPercentage returns the compression percentage (0-100)
// 0% means no compression, 100% means everything was dropped
func (m *CompressionMetrics) CompressionPercentage() float64 {
	if m.InputCount == 0 {
		return 0.0
	}
	dropped := m.InputCount - m.OutputCount
	return (float64(dropped) / float64(m.InputCount)) * 100.0
}

// GetTruncationByReason finds a truncation by reason, returns nil if not found
func (m *CompressionMetrics) GetTruncationByReason(reason TruncationReason) *TruncationInfo {
	for i := range m.Truncations {
		if m.Truncations[i].Reason == reason {
			return &m.Truncations[i]
		}
	}
	return nil
}

// HasTruncationReason checks if a specific truncation reason occurred
func (m *CompressionMetrics) HasTruncationReason(reason TruncationReason) bool {
	return m.GetTruncationByReason(reason) != nil
}
