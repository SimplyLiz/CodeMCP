//go:build !cartographer

// Package cartographer provides CGo bindings to the Rust Cartographer library.
// This stub is compiled when the 'cartographer' build tag is absent.
// All functions return ErrUnavailable; callers should check Available() first.
package cartographer

import "errors"

// ErrUnavailable is returned by all functions when Cartographer is not compiled in.
var ErrUnavailable = errors.New("cartographer: not compiled in this build (use -tags cartographer)")

// Available reports whether the Cartographer library is linked into this binary.
func Available() bool { return false }

func Version() (string, error)                                             { return "", ErrUnavailable }
func MapProject(_ string) (*ProjectGraph, error)                           { return nil, ErrUnavailable }
func Health(_ string) (*HealthReport, error)                               { return nil, ErrUnavailable }
func CheckLayers(_, _ string) ([]LayerViolation, error)                    { return nil, ErrUnavailable }
func SimulateChange(_, _, _, _ string) (*ImpactAnalysis, error)            { return nil, ErrUnavailable }
func SkeletonMap(_, _ string) (*SkeletonResult, error)                     { return nil, ErrUnavailable }
func GetModuleContext(_ string, _ string, _ uint32) (*ModuleContext, error) { return nil, ErrUnavailable }
func GitChurn(_ string, _ uint32) (map[string]int, error)                  { return nil, ErrUnavailable }
func GitCochange(_ string, _ uint32, _ uint32) ([]CoChangePair, error)     { return nil, ErrUnavailable }
func HiddenCoupling(_ string, _ uint32, _ uint32) ([]CoChangePair, error)  { return nil, ErrUnavailable }
func Semidiff(_, _, _ string) ([]SemidiffFile, error)                                         { return nil, ErrUnavailable }
func RankedSkeleton(_ string, _ []string, _ uint32) (*RankedSkeletonResult, error)            { return nil, ErrUnavailable }
func UnreferencedSymbols(_ string) (*UnreferencedSymbolsResult, error)                        { return nil, ErrUnavailable }
func SearchContent(_, _ string, _ *SearchContentOptions) (*SearchResult, error)               { return nil, ErrUnavailable }
func FindFiles(_, _ string, _ uint32) (*FindResult, error)                                    { return nil, ErrUnavailable }
