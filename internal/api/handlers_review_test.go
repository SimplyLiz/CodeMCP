package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/query"
)

func TestHandleReviewPR_GET(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/review/pr?baseBranch=main", nil)
	w := httptest.NewRecorder()

	srv.handleReviewPR(w, req)

	// Engine will fail because no git repo, but the handler should return
	// a proper error response, not panic.
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", w.Code)
	}

	// If it returned 500, verify it's a JSON error response
	if w.Code == http.StatusInternalServerError {
		var errResp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
			t.Fatalf("error response not valid JSON: %v", err)
		}
		if _, ok := errResp["error"]; !ok {
			t.Error("error response missing 'error' field")
		}
	}
}

func TestHandleReviewPR_POST(t *testing.T) {
	srv := newTestServer(t)

	body := `{"baseBranch":"main","checks":["breaking","secrets"],"failOnLevel":"none"}`
	req := httptest.NewRequest(http.MethodPost, "/review/pr", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleReviewPR(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", w.Code)
	}
}

func TestHandleReviewPR_POST_PolicyOverrides(t *testing.T) {
	srv := newTestServer(t)

	blockFalse := false
	maxRisk := 0.5
	body := `{"baseBranch":"main","blockBreakingChanges":false,"maxRiskScore":0.5}`
	_ = blockFalse
	_ = maxRisk

	req := httptest.NewRequest(http.MethodPost, "/review/pr", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleReviewPR(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", w.Code)
	}
}

func TestHandleReviewPR_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/review/pr", nil)
	w := httptest.NewRecorder()

	srv.handleReviewPR(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleReviewPR_POST_EmptyBody(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/review/pr", nil)
	w := httptest.NewRecorder()

	srv.handleReviewPR(w, req)

	// Should not panic on nil body — falls through to engine with defaults
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", w.Code)
	}
}

func TestHandleReviewPR_POST_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/review/pr", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleReviewPR(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleReviewPR_GET_WithChecksAndCriticalPaths(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/review/pr?checks=breaking,secrets&criticalPaths=cmd/**,internal/**", nil)
	w := httptest.NewRecorder()

	srv.handleReviewPR(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", w.Code)
	}
}

func TestParseCommaSeparated(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"a", 1},
		{"a,b,c", 3},
		{" a , b , c ", 3},
		{"a,,b", 2}, // empty segments filtered
		{",,,", 0},
	}
	for _, tt := range tests {
		got := parseCommaSeparated(tt.input)
		if len(got) != tt.want {
			t.Errorf("parseCommaSeparated(%q) = %d items, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestDefaultReviewPolicy(t *testing.T) {
	p := query.DefaultReviewPolicy()
	if p.FailOnLevel != "error" {
		t.Errorf("default FailOnLevel = %q, want 'error'", p.FailOnLevel)
	}
	if !p.BlockBreakingChanges {
		t.Error("default BlockBreakingChanges should be true")
	}
	if !p.BlockSecrets {
		t.Error("default BlockSecrets should be true")
	}
}
