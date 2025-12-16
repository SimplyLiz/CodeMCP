package api

import (
	"encoding/json"
	"net/http"

	"ckb/internal/errors"
)

// ErrorResponse represents an HTTP error response
type ErrorResponse struct {
	Error          string                `json:"error"`
	Code           string                `json:"code"`
	Details        interface{}           `json:"details,omitempty"`
	SuggestedFixes []errors.FixAction    `json:"suggestedFixes,omitempty"`
	Drilldowns     []errors.Drilldown    `json:"drilldowns,omitempty"`
}

// WriteError writes an error response to the HTTP response writer
func WriteError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := ErrorResponse{
		Error: err.Error(),
	}

	// If it's a CkbError, include additional information
	if ckbErr, ok := err.(*errors.CkbError); ok {
		resp.Code = string(ckbErr.Code)
		resp.Details = ckbErr.Details
		resp.SuggestedFixes = ckbErr.SuggestedFixes
		resp.Drilldowns = ckbErr.Drilldowns
	} else {
		resp.Code = "INTERNAL_ERROR"
	}

	json.NewEncoder(w).Encode(resp)
}

// WriteCkbError writes a CkbError with automatic status code mapping
func WriteCkbError(w http.ResponseWriter, err *errors.CkbError) {
	status := MapCkbErrorToStatus(err.Code)
	WriteError(w, err, status)
}

// MapCkbErrorToStatus maps CKB error codes to HTTP status codes
func MapCkbErrorToStatus(code errors.ErrorCode) int {
	switch code {
	case errors.BackendUnavailable:
		return http.StatusServiceUnavailable // 503
	case errors.IndexMissing:
		return http.StatusNotFound // 404
	case errors.IndexStale:
		return http.StatusPreconditionFailed // 412
	case errors.WorkspaceNotReady:
		return http.StatusServiceUnavailable // 503
	case errors.Timeout:
		return http.StatusGatewayTimeout // 504
	case errors.RateLimited:
		return http.StatusTooManyRequests // 429
	case errors.SymbolNotFound:
		return http.StatusNotFound // 404
	case errors.SymbolDeleted:
		return http.StatusGone // 410
	case errors.ScopeInvalid:
		return http.StatusBadRequest // 400
	case errors.AliasCycle:
		return http.StatusUnprocessableEntity // 422
	case errors.AliasChainTooDeep:
		return http.StatusUnprocessableEntity // 422
	case errors.BudgetExceeded:
		return http.StatusRequestEntityTooLarge // 413
	case errors.InternalError:
		return http.StatusInternalServerError // 500
	default:
		return http.StatusInternalServerError // 500
	}
}

// WriteJSON writes a JSON response
func WriteJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// BadRequest writes a 400 Bad Request error
func BadRequest(w http.ResponseWriter, message string) {
	WriteError(w, &errors.CkbError{
		Code:    errors.ScopeInvalid,
		Message: message,
	}, http.StatusBadRequest)
}

// NotFound writes a 404 Not Found error
func NotFound(w http.ResponseWriter, message string) {
	WriteError(w, &errors.CkbError{
		Code:    errors.SymbolNotFound,
		Message: message,
	}, http.StatusNotFound)
}

// InternalError writes a 500 Internal Server Error
func InternalError(w http.ResponseWriter, message string, err error) {
	WriteError(w, &errors.CkbError{
		Code:    errors.InternalError,
		Message: message,
	}, http.StatusInternalServerError)
}
