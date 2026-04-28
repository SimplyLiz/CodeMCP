package query

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/impact"
)

// startOutgoingImpactDaemon spawns a Unix-socket fake that replies to every
// query_outgoing_impact request with `payload`. Other request types get an
// empty object, which is enough for the handshake-gated paths that never
// reach those code paths in this test. Returns a snapshot closure so tests
// can assert on recorded requests.
func startOutgoingImpactDaemon(t *testing.T, payload map[string]any) func() []map[string]any {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "lip-outgoing")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	sockPath := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("listen: %v", err)
	}
	prev := os.Getenv("LIP_SOCKET")
	os.Setenv("LIP_SOCKET", sockPath)

	var (
		reqsMu sync.Mutex
		reqs   []map[string]any
	)
	reqC := make(chan map[string]any, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for r := range reqC {
			reqsMu.Lock()
			reqs = append(reqs, r)
			reqsMu.Unlock()
		}
	}()

	// Track active connections so cleanup can close them and wait for
	// handler goroutines to exit before closing reqC. Without this, an
	// in-flight handler could send on reqC after close(reqC).
	var (
		connMu      sync.Mutex
		activeConns []net.Conn
		handlersWG  sync.WaitGroup
	)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			connMu.Lock()
			activeConns = append(activeConns, conn)
			connMu.Unlock()
			handlersWG.Add(1)
			go func(c net.Conn) {
				defer handlersWG.Done()
				defer c.Close()
				for {
					var lenBuf [4]byte
					if _, err := io.ReadFull(c, lenBuf[:]); err != nil {
						return
					}
					buf := make([]byte, binary.BigEndian.Uint32(lenBuf[:]))
					if _, err := io.ReadFull(c, buf); err != nil {
						return
					}
					var req map[string]any
					_ = json.Unmarshal(buf, &req)
					reqC <- req

					var resp any = map[string]any{}
					if req["type"] == "query_outgoing_impact" {
						resp = payload
					}
					out, _ := json.Marshal(resp)
					var lb [4]byte
					binary.BigEndian.PutUint32(lb[:], uint32(len(out)))
					_, _ = c.Write(lb[:])
					_, _ = c.Write(out)
				}
			}(conn)
		}
	}()

	t.Cleanup(func() {
		ln.Close()
		connMu.Lock()
		for _, c := range activeConns {
			c.Close()
		}
		connMu.Unlock()
		handlersWG.Wait()
		close(reqC)
		<-done
		os.RemoveAll(dir)
		os.Setenv("LIP_SOCKET", prev)
	})
	return func() []map[string]any {
		reqsMu.Lock()
		defer reqsMu.Unlock()
		out := make([]map[string]any, len(reqs))
		copy(out, reqs)
		return out
	}
}

// newOutgoingTestEngine builds a minimal Engine that skips SCIP/resolver
// lookups: the caller-provided symbolInfo is returned directly, simulating
// a successful resolve. `supported` is the set of LIP message types the
// engine should pretend its daemon advertised.
func newOutgoingTestEngine(t *testing.T, supported []string) *Engine {
	t.Helper()
	e := &Engine{repoRoot: t.TempDir()}
	e.lipSupported = make(map[string]struct{}, len(supported))
	for _, s := range supported {
		e.lipSupported[s] = struct{}{}
	}
	return e
}

// stubOutgoingSymbol is the SymbolInfo we feed the engine via a test-only
// override. Kept at package level so the stub resolveSymbolForImpact can
// find it.
var stubOutgoingSymbol *SymbolInfo

func TestAnalyzeOutgoingImpact_HappyPath(t *testing.T) {
	payload := map[string]any{
		"type": "outgoing_impact_result",
		"result": map[string]any{
			"target_uri": "lip://local/internal/foo/bar.go#DoWork",
			"direct_items": []map[string]any{
				{
					"file_uri":   "lip://local//repo/internal/foo/bar.go",
					"symbol_uri": "lip://local//repo/internal/foo/bar.go#helper",
					"distance":   1,
					"confidence": 0.95,
				},
				{
					"file_uri":   "lip://local//repo/internal/baz/qux.go",
					"symbol_uri": "lip://local//repo/internal/baz/qux.go#validate",
					"distance":   1,
					"confidence": 0.9,
				},
			},
			"transitive_items": []map[string]any{
				{
					"file_uri":   "lip://local//repo/internal/deep/x.go",
					"symbol_uri": "lip://local//repo/internal/deep/x.go#log",
					"distance":   2,
					"confidence": 0.8,
				},
			},
			"edges_source": "scip_with_tier1_edges",
			"truncated":    false,
			"semantic_items": []map[string]any{
				{
					"file_uri":   "lip://local//repo/internal/sibling/y.go",
					"symbol_uri": "lip://local//repo/internal/sibling/y.go#related",
					"similarity": 0.82,
					"source":     "semantic",
				},
			},
		},
	}
	snap := startOutgoingImpactDaemon(t, payload)

	e := newOutgoingTestEngine(t, []string{"query_outgoing_impact"})
	// Inject a pre-resolved symbol by bypassing resolveSymbolForImpact via
	// the swap hook below; we wrap the method in a way that avoids touching
	// engine_test.go infrastructure.
	withStubResolver(t, &SymbolInfo{
		StableId: "lip://scip-go/gomod/pkg@1/internal/foo.go/DoWork().",
		Name:     "DoWork",
		Kind:     "function",
		Location: &LocationInfo{FileId: "internal/foo/bar.go"},
	})

	resp, err := e.AnalyzeOutgoingImpact(context.Background(), AnalyzeOutgoingImpactOptions{
		SymbolId: "DoWork",
		MinScore: 0.6,
	})
	if err != nil {
		t.Fatalf("AnalyzeOutgoingImpact: %v", err)
	}
	if len(resp.DirectCallees) != 2 {
		t.Errorf("DirectCallees = %d, want 2", len(resp.DirectCallees))
	}
	for _, c := range resp.DirectCallees {
		if c.Kind != string(impact.DirectCallee) {
			t.Errorf("direct callee kind = %q, want direct-callee", c.Kind)
		}
	}
	if len(resp.TransitiveCallees) != 1 {
		t.Errorf("TransitiveCallees = %d, want 1", len(resp.TransitiveCallees))
	}
	if resp.TransitiveCallees[0].Kind != string(impact.TransitiveCallee) {
		t.Errorf("transitive callee kind = %q, want transitive-callee", resp.TransitiveCallees[0].Kind)
	}
	if resp.EdgesSource != "scip_with_tier1_edges" {
		t.Errorf("EdgesSource = %q, want scip_with_tier1_edges", resp.EdgesSource)
	}
	if len(resp.SemanticCallees) != 1 {
		t.Errorf("SemanticCallees = %d, want 1", len(resp.SemanticCallees))
	}
	if resp.SemanticCallees[0].Source != "semantic" {
		t.Errorf("semantic source = %q, want semantic", resp.SemanticCallees[0].Source)
	}
	if resp.Truncated {
		t.Error("Truncated = true unexpectedly")
	}

	reqs := snap()
	var sawOutgoing bool
	for _, r := range reqs {
		if r["type"] == "query_outgoing_impact" {
			sawOutgoing = true
			if r["symbol_uri"] == nil || r["symbol_uri"] == "" {
				t.Errorf("request symbol_uri empty: %+v", r)
			}
		}
	}
	if !sawOutgoing {
		t.Errorf("no query_outgoing_impact request observed; requests=%+v", reqs)
	}
}

func TestAnalyzeOutgoingImpact_LipUnsupported(t *testing.T) {
	// No daemon started — lipSupported is empty.
	e := newOutgoingTestEngine(t, nil)
	withStubResolver(t, &SymbolInfo{
		StableId: "some-id",
		Name:     "DoWork",
		Location: &LocationInfo{FileId: "internal/foo/bar.go"},
	})

	resp, err := e.AnalyzeOutgoingImpact(context.Background(), AnalyzeOutgoingImpactOptions{
		SymbolId: "DoWork",
	})
	if err != nil {
		t.Fatalf("AnalyzeOutgoingImpact: %v", err)
	}
	if len(resp.DirectCallees) != 0 || len(resp.TransitiveCallees) != 0 {
		t.Errorf("expected empty callees when LIP unsupported, got direct=%d transitive=%d",
			len(resp.DirectCallees), len(resp.TransitiveCallees))
	}
	if resp.Provenance == nil || len(resp.Provenance.Warnings) == 0 {
		t.Fatal("expected provenance warning when LIP unsupported")
	}
	if !warningsContain(resp.Provenance.Warnings, "query_outgoing_impact") {
		t.Errorf("warning missing 'query_outgoing_impact': %v", resp.Provenance.Warnings)
	}
}

func TestAnalyzeOutgoingImpact_TruncatedPropagates(t *testing.T) {
	payload := map[string]any{
		"type": "outgoing_impact_result",
		"result": map[string]any{
			"target_uri":       "lip://local/foo#bar",
			"direct_items":     []map[string]any{},
			"transitive_items": []map[string]any{},
			"edges_source":     "scip_with_tier1_edges",
			"truncated":        true,
			"semantic_items":   []map[string]any{},
		},
	}
	startOutgoingImpactDaemon(t, payload)

	e := newOutgoingTestEngine(t, []string{"query_outgoing_impact"})
	withStubResolver(t, &SymbolInfo{
		StableId: "x",
		Name:     "bar",
		Location: &LocationInfo{FileId: "foo.go"},
	})

	resp, err := e.AnalyzeOutgoingImpact(context.Background(), AnalyzeOutgoingImpactOptions{
		SymbolId: "bar",
	})
	if err != nil {
		t.Fatalf("AnalyzeOutgoingImpact: %v", err)
	}
	if !resp.Truncated {
		t.Fatal("Truncated not propagated")
	}
	if !warningsContain(resp.Provenance.Warnings, "200-node") {
		t.Errorf("missing truncation warning: %v", resp.Provenance.Warnings)
	}
}

func TestAnalyzeOutgoingImpact_LipReturnsNilResult(t *testing.T) {
	// daemon returns outgoing_impact_result with no result field
	payload := map[string]any{"type": "outgoing_impact_result"}
	startOutgoingImpactDaemon(t, payload)

	e := newOutgoingTestEngine(t, []string{"query_outgoing_impact"})
	withStubResolver(t, &SymbolInfo{
		StableId: "x",
		Name:     "bar",
		Location: &LocationInfo{FileId: "foo.go"},
	})

	resp, err := e.AnalyzeOutgoingImpact(context.Background(), AnalyzeOutgoingImpactOptions{
		SymbolId: "bar",
	})
	if err != nil {
		t.Fatalf("AnalyzeOutgoingImpact: %v", err)
	}
	if len(resp.DirectCallees) != 0 || len(resp.TransitiveCallees) != 0 {
		t.Errorf("expected empty callees on nil result; got direct=%d transitive=%d",
			len(resp.DirectCallees), len(resp.TransitiveCallees))
	}
}

// withStubResolver installs a test hook that short-circuits
// resolveSymbolForImpact to the provided info. Cleanup restores the
// production implementation.
func withStubResolver(t *testing.T, info *SymbolInfo) {
	t.Helper()
	prev := resolveSymbolForImpactHook
	stubOutgoingSymbol = info
	resolveSymbolForImpactHook = func(_ *Engine, _ context.Context, _ string) (*SymbolInfo, CompletenessInfo, []BackendContribution) {
		return stubOutgoingSymbol, CompletenessInfo{Score: 1.0, Reason: "stub"}, nil
	}
	t.Cleanup(func() { resolveSymbolForImpactHook = prev })
}

func warningsContain(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

// Ensure time import used for build green when test file compiles without
// reaching code paths that take a deadline. (keeps go vet quiet)
var _ = time.Second
