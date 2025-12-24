package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ckb/internal/errors"
)

func TestMapCkbErrorToStatus(t *testing.T) {
	tests := []struct {
		code   errors.ErrorCode
		want   int
	}{
		{errors.BackendUnavailable, http.StatusServiceUnavailable},
		{errors.IndexMissing, http.StatusNotFound},
		{errors.IndexStale, http.StatusPreconditionFailed},
		{errors.WorkspaceNotReady, http.StatusServiceUnavailable},
		{errors.Timeout, http.StatusGatewayTimeout},
		{errors.RateLimited, http.StatusTooManyRequests},
		{errors.SymbolNotFound, http.StatusNotFound},
		{errors.SymbolDeleted, http.StatusGone},
		{errors.ScopeInvalid, http.StatusBadRequest},
		{errors.AliasCycle, http.StatusUnprocessableEntity},
		{errors.AliasChainTooDeep, http.StatusUnprocessableEntity},
		{errors.BudgetExceeded, http.StatusRequestEntityTooLarge},
		{errors.InternalError, http.StatusInternalServerError},
		{"UNKNOWN_CODE", http.StatusInternalServerError}, // default case
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			got := MapCkbErrorToStatus(tt.code)
			if got != tt.want {
				t.Errorf("MapCkbErrorToStatus(%q) = %d, want %d", tt.code, got, tt.want)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	t.Run("writes basic error", func(t *testing.T) {
		w := httptest.NewRecorder()
		err := fmt.Errorf("something went wrong")

		WriteError(w, err, http.StatusInternalServerError)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}

		contentType := w.Header().Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", contentType)
		}

		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp.Error != "something went wrong" {
			t.Errorf("resp.Error = %q, want 'something went wrong'", resp.Error)
		}
		if resp.Code != "INTERNAL_ERROR" {
			t.Errorf("resp.Code = %q, want INTERNAL_ERROR", resp.Code)
		}
	})

	t.Run("writes CkbError with details", func(t *testing.T) {
		w := httptest.NewRecorder()
		ckbErr := &errors.CkbError{
			Code:    errors.SymbolNotFound,
			Message: "symbol not found",
			Details: map[string]string{"symbolId": "test-123"},
		}

		WriteError(w, ckbErr, http.StatusNotFound)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
		}

		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp.Code != "SYMBOL_NOT_FOUND" {
			t.Errorf("resp.Code = %q, want SYMBOL_NOT_FOUND", resp.Code)
		}
		if resp.Details == nil {
			t.Error("resp.Details should not be nil")
		}
	})
}

func TestWriteCkbError(t *testing.T) {
	w := httptest.NewRecorder()
	ckbErr := &errors.CkbError{
		Code:    errors.RateLimited,
		Message: "too many requests",
	}

	WriteCkbError(w, ckbErr)

	// Should automatically map to 429
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Code != "RATE_LIMITED" {
		t.Errorf("resp.Code = %q, want RATE_LIMITED", resp.Code)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"message": "success"}

	WriteJSON(w, data, http.StatusOK)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["message"] != "success" {
		t.Errorf("resp[message] = %q, want success", resp["message"])
	}
}

func TestBadRequest(t *testing.T) {
	w := httptest.NewRecorder()

	BadRequest(w, "invalid query parameter")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// CkbError formats the message with the code prefix
	if resp.Code != "SCOPE_INVALID" {
		t.Errorf("resp.Code = %q, want SCOPE_INVALID", resp.Code)
	}
}

func TestNotFoundHelper(t *testing.T) {
	w := httptest.NewRecorder()

	NotFound(w, "resource not found")

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Code != "SYMBOL_NOT_FOUND" {
		t.Errorf("resp.Code = %q, want SYMBOL_NOT_FOUND", resp.Code)
	}
}

func TestInternalError(t *testing.T) {
	w := httptest.NewRecorder()

	InternalError(w, "database error", fmt.Errorf("connection failed"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Code != "INTERNAL_ERROR" {
		t.Errorf("resp.Code = %q, want INTERNAL_ERROR", resp.Code)
	}
}
