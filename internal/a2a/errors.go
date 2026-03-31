package a2a

import (
	"fmt"
	"net/http"
)

// A2A-specific JSON-RPC error codes.
const (
	ErrCodeTaskNotFound              = -32001
	ErrCodeTaskNotCancelable         = -32002
	ErrCodePushNotificationNotSupported = -32003
	ErrCodeUnsupportedOperation      = -32004
	ErrCodeContentTypeNotSupported   = -32005
	ErrCodeInvalidAgentResponse      = -32006
	ErrCodeExtendedCardNotConfigured = -32007
	ErrCodeExtensionSupportRequired  = -32008
	ErrCodeVersionNotSupported       = -32009

	// Standard JSON-RPC error codes.
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// A2AError is an error with both JSON-RPC and HTTP representations.
type A2AError struct {
	Code       int
	Message    string
	Data       interface{}
	HTTPStatus int
}

func (e *A2AError) Error() string {
	return e.Message
}

// ToJSONRPC converts the error to a JSON-RPC error object.
func (e *A2AError) ToJSONRPC() *JSONRPCError {
	return &JSONRPCError{
		Code:    e.Code,
		Message: e.Message,
		Data:    e.Data,
	}
}

// httpStatusForCode maps A2A error codes to HTTP status codes.
var httpStatusForCode = map[int]int{
	ErrCodeTaskNotFound:              http.StatusNotFound,
	ErrCodeTaskNotCancelable:         http.StatusConflict,
	ErrCodePushNotificationNotSupported: http.StatusBadRequest,
	ErrCodeUnsupportedOperation:      http.StatusBadRequest,
	ErrCodeContentTypeNotSupported:   http.StatusUnsupportedMediaType,
	ErrCodeInvalidAgentResponse:      http.StatusBadGateway,
	ErrCodeExtendedCardNotConfigured: http.StatusBadRequest,
	ErrCodeExtensionSupportRequired:  http.StatusBadRequest,
	ErrCodeVersionNotSupported:       http.StatusBadRequest,
	ErrCodeParseError:                http.StatusBadRequest,
	ErrCodeInvalidRequest:            http.StatusBadRequest,
	ErrCodeMethodNotFound:            http.StatusNotFound,
	ErrCodeInvalidParams:             http.StatusBadRequest,
	ErrCodeInternalError:             http.StatusInternalServerError,
}

// --- Error Constructors ---

func NewTaskNotFoundError(taskID string) *A2AError {
	return &A2AError{
		Code:       ErrCodeTaskNotFound,
		Message:    fmt.Sprintf("task not found: %s", taskID),
		HTTPStatus: http.StatusNotFound,
	}
}

func NewTaskNotCancelableError(taskID string) *A2AError {
	return &A2AError{
		Code:       ErrCodeTaskNotCancelable,
		Message:    fmt.Sprintf("task cannot be canceled: %s", taskID),
		HTTPStatus: http.StatusConflict,
	}
}

func NewPushNotificationNotSupportedError() *A2AError {
	return &A2AError{
		Code:       ErrCodePushNotificationNotSupported,
		Message:    "push notifications not supported",
		HTTPStatus: http.StatusBadRequest,
	}
}

func NewUnsupportedOperationError(operation string) *A2AError {
	return &A2AError{
		Code:       ErrCodeUnsupportedOperation,
		Message:    fmt.Sprintf("unsupported operation: %s", operation),
		HTTPStatus: http.StatusBadRequest,
	}
}

func NewContentTypeNotSupportedError(contentType string) *A2AError {
	return &A2AError{
		Code:       ErrCodeContentTypeNotSupported,
		Message:    fmt.Sprintf("content type not supported: %s", contentType),
		HTTPStatus: http.StatusUnsupportedMediaType,
	}
}

func NewInvalidAgentResponseError(detail string) *A2AError {
	return &A2AError{
		Code:       ErrCodeInvalidAgentResponse,
		Message:    fmt.Sprintf("invalid agent response: %s", detail),
		HTTPStatus: http.StatusBadGateway,
	}
}

func NewVersionNotSupportedError(version string) *A2AError {
	return &A2AError{
		Code:       ErrCodeVersionNotSupported,
		Message:    fmt.Sprintf("A2A version not supported: %s", version),
		HTTPStatus: http.StatusBadRequest,
	}
}

func NewParseError(detail string) *A2AError {
	return &A2AError{
		Code:       ErrCodeParseError,
		Message:    fmt.Sprintf("parse error: %s", detail),
		HTTPStatus: http.StatusBadRequest,
	}
}

func NewInvalidRequestError(detail string) *A2AError {
	return &A2AError{
		Code:       ErrCodeInvalidRequest,
		Message:    fmt.Sprintf("invalid request: %s", detail),
		HTTPStatus: http.StatusBadRequest,
	}
}

func NewMethodNotFoundError(method string) *A2AError {
	return &A2AError{
		Code:       ErrCodeMethodNotFound,
		Message:    fmt.Sprintf("method not found: %s", method),
		HTTPStatus: http.StatusNotFound,
	}
}

func NewInvalidParamsError(detail string) *A2AError {
	return &A2AError{
		Code:       ErrCodeInvalidParams,
		Message:    fmt.Sprintf("invalid params: %s", detail),
		HTTPStatus: http.StatusBadRequest,
	}
}

func NewInternalError(detail string) *A2AError {
	return &A2AError{
		Code:       ErrCodeInternalError,
		Message:    fmt.Sprintf("internal error: %s", detail),
		HTTPStatus: http.StatusInternalServerError,
	}
}
