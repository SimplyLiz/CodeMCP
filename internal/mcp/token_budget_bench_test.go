package mcp

import (
t"io"
t"log/slog"
	"encoding/json"
	"testing"

)

// BenchmarkToolsListSize measures tools/list payload size per preset.
// Use with benchstat for before/after comparison:
//
//	go test -bench=BenchmarkToolsListSize ./internal/mcp/... -count=5 > before.txt
//	# make changes
//	go test -bench=BenchmarkToolsListSize ./internal/mcp/... -count=5 > after.txt
//	benchstat before.txt after.txt
func BenchmarkToolsListSize(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewMCPServer("test", nil, logger)

	presets := []string{PresetCore, PresetReview, PresetRefactor, PresetFull}

	for _, preset := range presets {
		b.Run(preset, func(b *testing.B) {
			if err := server.SetPreset(preset); err != nil {
				b.Fatalf("SetPreset(%s) failed: %v", preset, err)
			}

			tools := server.GetFilteredTools()

			b.ResetTimer()
			b.ReportAllocs()

			var totalBytes int64
			for i := 0; i < b.N; i++ {
				data, err := json.Marshal(map[string]interface{}{"tools": tools})
				if err != nil {
					b.Fatal(err)
				}
				totalBytes += int64(len(data))
			}

			// Report custom metrics
			avgBytes := float64(totalBytes) / float64(b.N)
			b.ReportMetric(avgBytes, "bytes/op")
			b.ReportMetric(avgBytes/4, "est_tokens/op")
			b.ReportMetric(float64(len(tools)), "tools")
		})
	}
}

// BenchmarkToolsListPaginated measures paginated tools/list payload.
// Compares full page 1 vs multiple pages.
func BenchmarkToolsListPaginated(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewMCPServer("test", nil, logger)

	if err := server.SetPreset(PresetFull); err != nil {
		b.Fatalf("SetPreset failed: %v", err)
	}

	allTools := server.GetFilteredTools()
	toolsetHash := ComputeToolsetHash(allTools)

	scenarios := []struct {
		name     string
		pageSize int
	}{
		{"page_15", 15},
		{"page_20", 20},
		{"page_all", len(allTools)},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			var totalBytes int64
			for i := 0; i < b.N; i++ {
				page, _, err := PaginateTools(allTools, 0, sc.pageSize, PresetFull, toolsetHash)
				if err != nil {
					b.Fatal(err)
				}
				data, err := json.Marshal(map[string]interface{}{"tools": page})
				if err != nil {
					b.Fatal(err)
				}
				totalBytes += int64(len(data))
			}

			avgBytes := float64(totalBytes) / float64(b.N)
			b.ReportMetric(avgBytes, "bytes/op")
			b.ReportMetric(avgBytes/4, "est_tokens/op")
		})
	}
}

// BenchmarkToolSchemaSize measures individual tool schema sizes.
// Helps identify which tools contribute most to token budget.
func BenchmarkToolSchemaSize(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewMCPServer("test", nil, logger)

	tools := server.GetToolDefinitions()

	// Find the largest tools for focused benchmarking
	type toolSize struct {
		name string
		size int
	}
	sizes := make([]toolSize, 0, len(tools))
	for _, tool := range tools {
		data, _ := json.Marshal(tool)
		sizes = append(sizes, toolSize{tool.Name, len(data)})
	}

	// Sort by size descending and take top 5
	for i := 0; i < len(sizes)-1; i++ {
		for j := i + 1; j < len(sizes); j++ {
			if sizes[j].size > sizes[i].size {
				sizes[i], sizes[j] = sizes[j], sizes[i]
			}
		}
	}

	topN := 5
	if len(sizes) < topN {
		topN = len(sizes)
	}

	for _, ts := range sizes[:topN] {
		b.Run(ts.name, func(b *testing.B) {
			// Find the tool
			var tool Tool
			for _, t := range tools {
				if t.Name == ts.name {
					tool = t
					break
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				data, _ := json.Marshal(tool)
				b.ReportMetric(float64(len(data)), "bytes/op")
			}
		})
	}
}
