package query

import (
	"context"

	"github.com/SimplyLiz/CodeMCP/internal/testgap"
)

// AnalyzeTestGapsOptions wraps testgap.AnalyzeOptions for the query engine.
type AnalyzeTestGapsOptions struct {
	Target   string
	MinLines int
	Limit    int
}

// AnalyzeTestGaps runs test gap analysis and wraps the result with provenance.
func (e *Engine) AnalyzeTestGaps(ctx context.Context, opts AnalyzeTestGapsOptions) (*testgap.TestGapResult, error) {
	analyzer := testgap.NewAnalyzer(e.repoRoot, e.logger, e.scipAdapter)

	return analyzer.Analyze(ctx, testgap.AnalyzeOptions{
		Target:   opts.Target,
		MinLines: opts.MinLines,
		Limit:    opts.Limit,
	})
}
