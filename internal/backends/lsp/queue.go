package lsp

import (
	"fmt"
	"time"

	"ckb/internal/errors"
)

// enqueue adds a request to the language's request queue
func (s *LspSupervisor) enqueue(languageId string, req *LspRequest) error {
	s.queuesMu.RLock()
	queue, exists := s.queues[languageId]
	s.queuesMu.RUnlock()

	if !exists {
		// Queue doesn't exist, create it
		s.queuesMu.Lock()
		queue, exists = s.queues[languageId]
		if !exists {
			queue = make(chan *LspRequest, s.queueSize)
			s.queues[languageId] = queue

			// Start queue processor
			s.wg.Add(1)
			go s.processQueue(languageId)
		}
		s.queuesMu.Unlock()
	}

	// Try to enqueue with timeout
	select {
	case queue <- req:
		return nil
	case <-time.After(time.Duration(s.maxQueueWaitMs) * time.Millisecond):
		return errors.NewCkbError(
			errors.RateLimited,
			fmt.Sprintf("LSP queue full for language: %s", languageId),
			nil,
			errors.GetSuggestedFixes(errors.RateLimited),
			nil,
		)
	case <-req.Context.Done():
		return errors.NewCkbError(
			errors.Timeout,
			"Request cancelled before enqueuing",
			req.Context.Err(),
			nil,
			nil,
		)
	}
}

// processQueue processes requests from the queue for a specific language
func (s *LspSupervisor) processQueue(languageId string) {
	defer s.wg.Done()

	s.queuesMu.RLock()
	queue := s.queues[languageId]
	s.queuesMu.RUnlock()

	for {
		select {
		case req, ok := <-queue:
			if !ok {
				// Queue closed, exit
				return
			}

			// Check if request was cancelled
			select {
			case <-req.Context.Done():
				// Request cancelled, send error response
				req.Response <- &LspResponse{
					Error: errors.NewCkbError(
						errors.Timeout,
						"Request cancelled",
						req.Context.Err(),
						nil,
						nil,
					),
				}
				continue
			default:
			}

			// Execute request
			resp := s.executeRequest(languageId, req)

			// Send response
			select {
			case req.Response <- resp:
			default:
				// Response channel full or closed, log and continue
				s.logger.Warn("Failed to send LSP response", map[string]interface{}{
					"languageId": languageId,
					"method":     req.Method,
				})
			}

		case <-s.done:
			// Supervisor shutting down
			return
		}
	}
}

// getQueueSize returns the current queue size for a language
func (s *LspSupervisor) getQueueSize(languageId string) int {
	s.queuesMu.RLock()
	defer s.queuesMu.RUnlock()

	queue, exists := s.queues[languageId]
	if !exists {
		return 0
	}

	return len(queue)
}

// waitForSlot waits for a queue slot to become available
func (s *LspSupervisor) waitForSlot(languageId string, maxWaitMs int) bool {
	s.queuesMu.RLock()
	queue, exists := s.queues[languageId]
	s.queuesMu.RUnlock()

	if !exists {
		return true // No queue yet, slot available
	}

	// Check current queue size
	if len(queue) < s.queueSize {
		return true // Slot available
	}

	// Wait for a slot with timeout
	timeout := time.After(time.Duration(maxWaitMs) * time.Millisecond)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if len(queue) < s.queueSize {
				return true
			}
		case <-timeout:
			return false
		}
	}
}

// clearQueue clears all pending requests for a language
func (s *LspSupervisor) clearQueue(languageId string) {
	s.queuesMu.RLock()
	queue, exists := s.queues[languageId]
	s.queuesMu.RUnlock()

	if !exists {
		return
	}

	// Drain the queue
	drained := 0
	for {
		select {
		case req := <-queue:
			// Send error response
			req.Response <- &LspResponse{
				Error: errors.NewCkbError(
					errors.BackendUnavailable,
					"LSP server restarting",
					nil,
					nil,
					nil,
				),
			}
			drained++
		default:
			// Queue empty
			if drained > 0 {
				s.logger.Info("Cleared LSP queue", map[string]interface{}{
					"languageId": languageId,
					"count":      drained,
				})
			}
			return
		}
	}
}

// GetQueueStats returns statistics about the queues
func (s *LspSupervisor) GetQueueStats() map[string]interface{} {
	s.queuesMu.RLock()
	defer s.queuesMu.RUnlock()

	stats := make(map[string]interface{})

	for langId, queue := range s.queues {
		stats[langId] = map[string]interface{}{
			"size":     len(queue),
			"capacity": s.queueSize,
		}
	}

	return stats
}

// RejectFast determines if a request should be rejected immediately due to queue pressure
func (s *LspSupervisor) RejectFast(languageId string) bool {
	queueSize := s.getQueueSize(languageId)

	// Reject if queue is nearly full (>80% capacity)
	threshold := int(float64(s.queueSize) * 0.8)
	return queueSize > threshold
}

// GetInFlightCount returns the number of in-flight requests for a language
func (s *LspSupervisor) GetInFlightCount(languageId string) int {
	// For now, this is approximated by queue size
	// In a more sophisticated implementation, we'd track active requests separately
	return s.getQueueSize(languageId)
}

// WaitForQueue waits for the queue to drain below a threshold
func (s *LspSupervisor) WaitForQueue(languageId string, threshold int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if s.getQueueSize(languageId) <= threshold {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}

	return false
}
