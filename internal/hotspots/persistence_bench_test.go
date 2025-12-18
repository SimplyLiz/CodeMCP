package hotspots

import (
	"testing"
	"time"
)

// =============================================================================
// v6.0 Hotspot Benchmarks
// =============================================================================
// Cheap tools: P95 < 300ms
// Heavy tools: P95 < 2000ms
// =============================================================================

func BenchmarkCalculateTrend(b *testing.B) {
	snapshots := make([]HotspotSnapshot, 30)
	for i := 0; i < 30; i++ {
		snapshots[i] = HotspotSnapshot{
			SnapshotDate: time.Now().AddDate(0, 0, -i),
			Score:        0.5 + float64(i)*0.01,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateTrend(snapshots)
	}
}

func BenchmarkCalculateInstability(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateInstability(5, 10)
	}
}

func BenchmarkComputeCompositeScore(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeCompositeScore(0.6, 0.4, 0.5)
	}
}

func BenchmarkNormalizeChurnScore(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NormalizeChurnScore(15, 45, 3)
	}
}

func BenchmarkNormalizeCouplingScore(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NormalizeCouplingScore(5, 10)
	}
}

func BenchmarkNormalizeComplexityScore(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NormalizeComplexityScore(12.5, 18.2)
	}
}

// BenchmarkHotspotScoringPipeline simulates full hotspot scoring for multiple items
func BenchmarkHotspotScoringPipeline(b *testing.B) {
	items := make([]struct {
		commits30d, commits90d, authors30d int
		afferent, efferent                 int
		cyclomatic, cognitive              float64
	}, 100)

	for i := 0; i < 100; i++ {
		items[i] = struct {
			commits30d, commits90d, authors30d int
			afferent, efferent                 int
			cyclomatic, cognitive              float64
		}{
			commits30d: 10 + i%20,
			commits90d: 30 + i%40,
			authors30d: 2 + i%5,
			afferent:   3 + i%10,
			efferent:   5 + i%15,
			cyclomatic: 8.0 + float64(i%10),
			cognitive:  12.0 + float64(i%15),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, item := range items {
			churnScore := NormalizeChurnScore(item.commits30d, item.commits90d, item.authors30d)
			couplingScore := NormalizeCouplingScore(item.afferent, item.efferent)
			complexityScore := NormalizeComplexityScore(item.cyclomatic, item.cognitive)
			ComputeCompositeScore(churnScore, couplingScore, complexityScore)
		}
	}
}

// BenchmarkTrendAnalysisPipeline simulates trend analysis for multiple files
func BenchmarkTrendAnalysisPipeline(b *testing.B) {
	// Create snapshots for 50 files, 10 snapshots each
	files := make([][]HotspotSnapshot, 50)
	for f := 0; f < 50; f++ {
		files[f] = make([]HotspotSnapshot, 10)
		for s := 0; s < 10; s++ {
			files[f][s] = HotspotSnapshot{
				SnapshotDate: time.Now().AddDate(0, 0, -s*7),
				Score:        0.5 + float64(s)*0.02,
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, snapshots := range files {
			CalculateTrend(snapshots)
		}
	}
}
