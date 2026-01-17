package lsp

import (
	"fmt"
	"time"
)

// ensureCapacity ensures there's room for a new process by evicting if necessary
func (s *LspSupervisor) ensureCapacity() error {
	// This should be called with s.mu held
	if len(s.processes) < s.maxProcesses {
		return nil // We have capacity
	}

	// Find LRU process to evict
	lruProc := s.findLRUProcess()
	if lruProc == nil {
		return fmt.Errorf("no process available for eviction")
	}

	s.logger.Info("Evicting LSP process to make room",
		"languageId", lruProc.LanguageId,
		"lastResponse", lruProc.GetLastResponseTime(),
		"timeSinceUsed", time.Since(lruProc.GetLastResponseTime()).String(),
	)

	// Shutdown the LRU process
	return s.shutdown(lruProc.LanguageId)
}

// findLRUProcess finds the least recently used process
func (s *LspSupervisor) findLRUProcess() *LspProcess {
	// This should be called with s.mu held

	var lruProc *LspProcess
	var oldestTime time.Time

	for _, proc := range s.processes {
		lastResponse := proc.GetLastResponseTime()

		// Skip processes that have never responded (might be initializing)
		if lastResponse.IsZero() {
			continue
		}

		// Skip unhealthy processes - they'll be cleaned up by health checks
		if !proc.IsHealthy() {
			continue
		}

		// Find the oldest response time
		if lruProc == nil || lastResponse.Before(oldestTime) {
			lruProc = proc
			oldestTime = lastResponse
		}
	}

	// If no healthy process found, evict any process
	if lruProc == nil {
		for _, proc := range s.processes {
			return proc
		}
	}

	return lruProc
}

// shutdown gracefully shuts down a process and removes it from the supervisor
func (s *LspSupervisor) shutdown(languageId string) error {
	// This should be called with s.mu held

	proc, exists := s.processes[languageId]
	if !exists {
		return fmt.Errorf("no process found for language: %s", languageId)
	}

	// Remove from map
	delete(s.processes, languageId)

	// Clear the queue
	go s.clearQueue(languageId)

	// Shutdown the process
	go func() {
		if err := proc.Shutdown(); err != nil {
			s.logger.Error("Error shutting down LSP process",
				"languageId", languageId,
				"error", err.Error(),
			)
		}
	}()

	return nil
}

// EvictProcess evicts a specific process (for testing or manual management)
func (s *LspSupervisor) EvictProcess(languageId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.shutdown(languageId)
}

// EvictIdle evicts all processes that have been idle for longer than the given duration
func (s *LspSupervisor) EvictIdle(idleTimeout time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	evicted := 0

	// Collect processes to evict (can't modify map while iterating)
	toEvict := make([]string, 0)

	for langId, proc := range s.processes {
		lastResponse := proc.GetLastResponseTime()

		// Skip if never used (might be initializing)
		if lastResponse.IsZero() {
			continue
		}

		// Check if idle
		idleTime := now.Sub(lastResponse)
		if idleTime > idleTimeout {
			toEvict = append(toEvict, langId)
		}
	}

	// Evict collected processes
	for _, langId := range toEvict {
		s.logger.Info("Evicting idle LSP process",
			"languageId", langId,
			"idleTimeout", idleTimeout.String(),
		)

		if err := s.shutdown(langId); err != nil {
			s.logger.Error("Failed to evict idle process",
				"languageId", langId,
				"error", err.Error(),
			)
		} else {
			evicted++
		}
	}

	return evicted
}

// GetIdleProcesses returns a list of processes that have been idle for longer than the given duration
func (s *LspSupervisor) GetIdleProcesses(idleTimeout time.Duration) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	idle := make([]string, 0)

	for langId, proc := range s.processes {
		lastResponse := proc.GetLastResponseTime()

		// Skip if never used
		if lastResponse.IsZero() {
			continue
		}

		// Check if idle
		idleTime := now.Sub(lastResponse)
		if idleTime > idleTimeout {
			idle = append(idle, langId)
		}
	}

	return idle
}

// GetProcessCount returns the current number of running processes
func (s *LspSupervisor) GetProcessCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.processes)
}

// GetMaxProcesses returns the maximum allowed number of processes
func (s *LspSupervisor) GetMaxProcesses() int {
	return s.maxProcesses
}

// HasCapacity returns true if there's room for more processes
func (s *LspSupervisor) HasCapacity() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.processes) < s.maxProcesses
}

// GetEvictionCandidates returns a list of processes ordered by LRU (oldest first)
func (s *LspSupervisor) GetEvictionCandidates() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build list of (languageId, lastResponse) pairs
	type candidate struct {
		languageId   string
		lastResponse time.Time
	}

	candidates := make([]candidate, 0, len(s.processes))
	for langId, proc := range s.processes {
		candidates = append(candidates, candidate{
			languageId:   langId,
			lastResponse: proc.GetLastResponseTime(),
		})
	}

	// Sort by lastResponse (oldest first)
	// Simple bubble sort since we expect small number of processes
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			// Put zero times at the end
			if candidates[i].lastResponse.IsZero() {
				candidates[i], candidates[j] = candidates[j], candidates[i]
				continue
			}
			if !candidates[j].lastResponse.IsZero() && candidates[j].lastResponse.Before(candidates[i].lastResponse) {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Extract language IDs
	result := make([]string, len(candidates))
	for i, c := range candidates {
		result[i] = c.languageId
	}

	return result
}

// SetMaxProcesses updates the maximum number of processes (for dynamic adjustment)
func (s *LspSupervisor) SetMaxProcesses(max int) error {
	if max < 1 {
		return fmt.Errorf("max processes must be at least 1")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	oldMax := s.maxProcesses
	s.maxProcesses = max

	s.logger.Info("Updated max processes",
		"oldMax", oldMax,
		"newMax", max,
	)

	// If we're now over capacity, evict excess processes
	for len(s.processes) > s.maxProcesses {
		lruProc := s.findLRUProcess()
		if lruProc == nil {
			break
		}

		s.logger.Info("Evicting excess process",
			"languageId", lruProc.LanguageId,
		)

		if err := s.shutdown(lruProc.LanguageId); err != nil {
			s.logger.Error("Failed to evict excess process",
				"languageId", lruProc.LanguageId,
				"error", err.Error(),
			)
			break
		}
	}

	return nil
}
