// Package pkg provides HTTP handling and business logic.
package pkg

import "fixture/internal"

// Handler is a type that handles HTTP requests (disambiguation: same name as main.Handler function).
type Handler struct {
	service Service
}

// NewHandler creates a new Handler with the given service.
func NewHandler(svc Service) *Handler {
	return &Handler{service: svc}
}

// Handle processes a request using the service.
// Call chain: Handle -> service.Process (interface call)
func (h *Handler) Handle(input string) string {
	result := h.service.Process(input)
	return internal.FormatOutput(result)
}

// HandleBatch processes multiple inputs.
func (h *Handler) HandleBatch(inputs []string) []string {
	results := make([]string, len(inputs))
	for i, input := range inputs {
		results[i] = h.Handle(input)
	}
	return results
}
