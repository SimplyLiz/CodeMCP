package output

// ImpactKindPriority defines the ordering priority for impact kinds
// Lower numbers have higher priority (sorted first)
var ImpactKindPriority = map[string]int{
	"direct-caller":     1,
	"transitive-caller": 2,
	"type-dependency":   3,
	"test-dependency":   4,
	"unknown":           5,
}

// WarningSeverity defines the ordering priority for warning severities
// Lower numbers have higher priority (sorted first)
var WarningSeverity = map[string]int{
	"error":   1,
	"warning": 2,
	"info":    3,
}

// GetImpactKindPriority returns the priority for a given impact kind
// Unknown kinds get the lowest priority (highest number)
func GetImpactKindPriority(kind string) int {
	if priority, ok := ImpactKindPriority[kind]; ok {
		return priority
	}
	return ImpactKindPriority["unknown"]
}

// GetWarningSeverity returns the priority for a given warning severity
// Unknown severities get the lowest priority (highest number)
func GetWarningSeverity(severity string) int {
	if priority, ok := WarningSeverity[severity]; ok {
		return priority
	}
	return WarningSeverity["info"]
}
