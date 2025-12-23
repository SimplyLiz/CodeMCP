package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleLanguageQuality(t *testing.T) {
	// Create temp dir and change to it
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	_ = os.Chdir(tmpDir)

	// Create .ckb directory
	_ = os.MkdirAll(filepath.Join(tmpDir, ".ckb"), 0755)

	server := &Server{}
	handler := http.HandlerFunc(server.handleLanguageQuality)

	req := httptest.NewRequest("GET", "/meta/languages", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response LanguageQualityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}
}

func TestHandleLanguageQuality_MethodNotAllowed(t *testing.T) {
	server := &Server{}
	handler := http.HandlerFunc(server.handleLanguageQuality)

	req := httptest.NewRequest("POST", "/meta/languages", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", rec.Code)
	}
}

func TestHandlePythonEnv(t *testing.T) {
	// Create temp dir and change to it
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	_ = os.Chdir(tmpDir)

	server := &Server{}
	handler := http.HandlerFunc(server.handlePythonEnv)

	req := httptest.NewRequest("GET", "/meta/python-env", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response PythonEnvResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}
}

func TestHandlePythonEnv_WithRequirements(t *testing.T) {
	// Create temp dir and change to it
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	_ = os.Chdir(tmpDir)

	// Create requirements.txt
	reqPath := filepath.Join(tmpDir, "requirements.txt")
	_ = os.WriteFile(reqPath, []byte("flask==2.0\n"), 0644)

	server := &Server{}
	handler := http.HandlerFunc(server.handlePythonEnv)

	req := httptest.NewRequest("GET", "/meta/python-env", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response PythonEnvResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.HasRequirements {
		t.Error("Expected HasRequirements to be true")
	}

	// Should have a recommendation since no venv is active
	if response.Recommendation == "" {
		t.Error("Expected recommendation for inactive venv")
	}
}

func TestHandlePythonEnv_MethodNotAllowed(t *testing.T) {
	server := &Server{}
	handler := http.HandlerFunc(server.handlePythonEnv)

	req := httptest.NewRequest("POST", "/meta/python-env", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", rec.Code)
	}
}

func TestHandleTSMonorepo(t *testing.T) {
	// Create temp dir and change to it
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	_ = os.Chdir(tmpDir)

	server := &Server{}
	handler := http.HandlerFunc(server.handleTSMonorepo)

	req := httptest.NewRequest("GET", "/meta/typescript-monorepo", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response TSMonorepoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}

	// Empty dir should not be a monorepo
	if response.IsMonorepo {
		t.Error("Empty dir should not be detected as monorepo")
	}
}

func TestHandleTSMonorepo_WithPnpm(t *testing.T) {
	// Create temp dir and change to it
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	_ = os.Chdir(tmpDir)

	// Create pnpm-workspace.yaml
	workspacePath := filepath.Join(tmpDir, "pnpm-workspace.yaml")
	_ = os.WriteFile(workspacePath, []byte("packages:\n  - packages/*\n"), 0644)

	// Create tsconfig.json
	tsconfigPath := filepath.Join(tmpDir, "tsconfig.json")
	_ = os.WriteFile(tsconfigPath, []byte("{}"), 0644)

	server := &Server{}
	handler := http.HandlerFunc(server.handleTSMonorepo)

	req := httptest.NewRequest("GET", "/meta/typescript-monorepo", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response TSMonorepoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !response.IsMonorepo {
		t.Error("Should be detected as monorepo")
	}

	if response.WorkspaceType != "pnpm" {
		t.Errorf("Expected workspace type 'pnpm', got %s", response.WorkspaceType)
	}

	if !response.HasRootTsconfig {
		t.Error("Should have root tsconfig")
	}
}

func TestHandleTSMonorepo_MethodNotAllowed(t *testing.T) {
	server := &Server{}
	handler := http.HandlerFunc(server.handleTSMonorepo)

	req := httptest.NewRequest("POST", "/meta/typescript-monorepo", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", rec.Code)
	}
}
