//go:build !navigator

// Package navigator provides CGo bindings to the Rust nyx-navigator library.
// This stub is compiled when the 'navigator' build tag is absent.
// All functions return ErrUnavailable; callers should check Available() first.
package navigator

// Available reports whether the navigator library is linked into this binary.
func Available() bool { return false }

func Version() (string, error)                                  { return "", ErrUnavailable }
func MapProject(_ string) (*ProjectGraph, error)                { return nil, ErrUnavailable }
func Health(_ string) (*HealthReport, error)                    { return nil, ErrUnavailable }
func CheckLayers(_, _ string) ([]LayerViolation, error)         { return nil, ErrUnavailable }
func SimulateChange(_, _, _, _ string) (*ImpactAnalysis, error) { return nil, ErrUnavailable }
func SkeletonMap(_, _ string) (*SkeletonResult, error)          { return nil, ErrUnavailable }
func GetModuleContext(_ string, _ string, _ uint32) (*ModuleContext, error) {
	return nil, ErrUnavailable
}
func GitChurn(_ string, _ uint32) (map[string]int, error)                 { return nil, ErrUnavailable }
func GitCochange(_ string, _ uint32, _ uint32) ([]CoChangePair, error)    { return nil, ErrUnavailable }
func HiddenCoupling(_ string, _ uint32, _ uint32) ([]CoChangePair, error) { return nil, ErrUnavailable }
func Semidiff(_, _, _ string) ([]SemidiffFile, error)                     { return nil, ErrUnavailable }
func RankedSkeleton(_ string, _ []string, _ uint32) (*RankedSkeletonResult, error) {
	return nil, ErrUnavailable
}
func UnreferencedSymbols(_ string) (*UnreferencedSymbolsResult, error) { return nil, ErrUnavailable }
func SearchContent(_, _ string, _ *SearchContentOptions) (*SearchResult, error) {
	return nil, ErrUnavailable
}
func FindFiles(_, _ string, _ uint32, _ *FindOptions) (*FindResult, error) {
	return nil, ErrUnavailable
}
func ReplaceContent(_, _, _ string, _ *ReplaceOptions) (*ReplaceResult, error) {
	return nil, ErrUnavailable
}
func ExtractContent(_, _ string, _ *ExtractOptions) (*ExtractResult, error) {
	return nil, ErrUnavailable
}
func ContextHealth(_ string, _ *ContextHealthOpts) (*ContextHealthReport, error) {
	return nil, ErrUnavailable
}
func BM25Search(_, _ string, _ *BM25Options) (*BM25Result, error) { return nil, ErrUnavailable }
func QueryContext(_, _ string, _ *QueryContextOpts) (*QueryContextResult, error) {
	return nil, ErrUnavailable
}
func ShotgunSurgery(_ string, _, _ uint32) ([]ShotgunSurgeryEntry, error) { return nil, ErrUnavailable }
func Evolution(_ string, _ uint32) (*EvolutionResult, error)              { return nil, ErrUnavailable }
func BlastRadius(_, _ string, _ uint32) (*BlastRadiusResult, error)       { return nil, ErrUnavailable }
func RenderArchitecture(_, _, _ string, _, _ uint32) (*RenderArchitectureResult, error) {
	return nil, ErrUnavailable
}
