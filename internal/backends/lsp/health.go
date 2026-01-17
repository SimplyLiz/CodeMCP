package lsp

import (
	"fmt"
	"time"

	"ckb/internal/errors"
)

// handleCrash handles a crashed or unhealthy LSP process
func (s *LspSupervisor) handleCrash(languageId string) {
	s.mu.Lock()
	proc, exists := s.processes[languageId]
	if !exists {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	s.logger.Error("LSP process crashed or unhealthy",
		"languageId", languageId,
		"restartCount", proc.GetRestartCount(),
		"consecutiveFailures", proc.GetConsecutiveFailures(),
	)

	// Mark as dead
	proc.SetState(StateDead)

	// Clear pending queue
	s.clearQueue(languageId)

	// Attempt restart with backoff
	if err := s.restart(languageId); err != nil {
		s.logger.Error("Failed to restart LSP process",
			"languageId", languageId,
			"error", err.Error(),
		)
	}
}

// restart restarts an LSP process with exponential backoff
func (s *LspSupervisor) restart(languageId string) error {
	s.mu.Lock()
	proc, exists := s.processes[languageId]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("no process found for language: %s", languageId)
	}

	// Check if we can restart (backoff check)
	if !proc.CanRestart() {
		s.mu.Unlock()
		waitTime := time.Until(proc.NextRestartAt)
		s.logger.Info("LSP restart delayed due to backoff",
			"languageId", languageId,
			"waitTime", waitTime.String(),
		)
		return errors.NewCkbError(
			errors.BackendUnavailable,
			fmt.Sprintf("LSP server in backoff, retry in %v", waitTime),
			nil,
			nil,
			nil,
		)
	}

	// Increment restart count
	proc.IncrementRestartCount()
	restartCount := proc.GetRestartCount()

	// Compute backoff
	backoff := s.computeBackoff(restartCount)
	proc.SetNextRestartAt(time.Now().Add(backoff))

	// Remove from processes map
	delete(s.processes, languageId)
	s.mu.Unlock()

	// Shutdown old process
	_ = proc.Shutdown()

	s.logger.Info("Restarting LSP server",
		"languageId", languageId,
		"restartCount", restartCount,
		"backoff", backoff.String(),
	)

	// Start new process
	return s.StartServer(languageId)
}

// checkHealth checks if a process is healthy
func (s *LspSupervisor) checkHealth(languageId string) bool {
	proc := s.GetProcess(languageId)
	if proc == nil {
		return false
	}

	// Check process state
	state := proc.GetState()
	if state == StateDead {
		return false
	}

	// Check if process is responsive
	if state == StateReady {
		lastResponse := proc.GetLastResponseTime()
		if !lastResponse.IsZero() && time.Since(lastResponse) > ResponseTimeout {
			s.logger.Warn("LSP process not responding",
				"languageId", languageId,
				"lastResponse", lastResponse,
				"timeSinceResponse", time.Since(lastResponse).String(),
			)
			proc.SetState(StateUnhealthy)
			return false
		}
	}

	// Check consecutive failures
	failures := proc.GetConsecutiveFailures()
	if failures >= MaxConsecutiveFailures {
		s.logger.Warn("LSP process has too many failures",
			"languageId", languageId,
			"consecutiveFailures", failures,
		)
		proc.SetState(StateUnhealthy)
		return false
	}

	// Check if underlying process is alive
	if proc.cmd != nil && proc.cmd.Process != nil {
		// Try to send a signal to check if process exists
		// On Unix, signal 0 checks if process exists without actually sending a signal
		// This is platform-specific but works on macOS/Linux
		// For Windows, we'd need a different approach
		if err := proc.cmd.Process.Signal(nil); err != nil {
			s.logger.Warn("LSP process died unexpectedly",
				"languageId", languageId,
				"error", err.Error(),
			)
			proc.SetState(StateDead)
			return false
		}
	}

	return true
}

// computeBackoff computes exponential backoff duration
func (s *LspSupervisor) computeBackoff(restartCount int) time.Duration {
	// Exponential backoff: base * 2^(restartCount-1)
	// But cap at MaxBackoffMs

	if restartCount <= 0 {
		return time.Duration(BaseBackoffMs) * time.Millisecond
	}

	backoffMs := BaseBackoffMs
	for i := 1; i < restartCount && backoffMs < MaxBackoffMs; i++ {
		backoffMs *= 2
	}

	if backoffMs > MaxBackoffMs {
		backoffMs = MaxBackoffMs
	}

	return time.Duration(backoffMs) * time.Millisecond
}

// HealthCheck performs an active health check on a process
func (s *LspSupervisor) HealthCheck(languageId string) error {
	proc := s.GetProcess(languageId)
	if proc == nil {
		return errors.NewCkbError(
			errors.BackendUnavailable,
			fmt.Sprintf("no LSP server running for language: %s", languageId),
			nil,
			nil,
			nil,
		)
	}

	if !s.checkHealth(languageId) {
		return errors.NewCkbError(
			errors.WorkspaceNotReady,
			fmt.Sprintf("LSP server unhealthy for language: %s", languageId),
			nil,
			errors.GetSuggestedFixes(errors.WorkspaceNotReady),
			nil,
		)
	}

	return nil
}

// ResetFailures resets the failure counter for a process
func (s *LspSupervisor) ResetFailures(languageId string) {
	proc := s.GetProcess(languageId)
	if proc == nil {
		return
	}

	proc.mu.Lock()
	proc.ConsecutiveFailures = 0
	proc.mu.Unlock()

	s.logger.Info("Reset failure counter",
		"languageId", languageId,
	)
}

// ForceRestart forces a restart of an LSP process, bypassing backoff
func (s *LspSupervisor) ForceRestart(languageId string) error {
	s.mu.Lock()
	proc, exists := s.processes[languageId]
	if exists {
		// Reset backoff timer to allow immediate restart
		proc.SetNextRestartAt(time.Now())
		delete(s.processes, languageId)
	}
	s.mu.Unlock()

	if exists {
		_ = proc.Shutdown()
	}

	s.logger.Info("Force restarting LSP server",
		"languageId", languageId,
	)

	return s.StartServer(languageId)
}

// GetHealthStatus returns the health status of all processes
func (s *LspSupervisor) GetHealthStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := make(map[string]interface{})

	for langId, proc := range s.processes {
		healthy := s.checkHealth(langId)

		status[langId] = map[string]interface{}{
			"state":               proc.GetState(),
			"healthy":             healthy,
			"lastResponse":        proc.GetLastResponseTime(),
			"consecutiveFailures": proc.GetConsecutiveFailures(),
			"restartCount":        proc.GetRestartCount(),
			"canRestart":          proc.CanRestart(),
			"nextRestartAt":       proc.NextRestartAt,
		}
	}

	return status
}

// RecoverAll attempts to recover all unhealthy processes
func (s *LspSupervisor) RecoverAll() map[string]error {
	s.mu.RLock()
	languageIds := make([]string, 0, len(s.processes))
	for langId := range s.processes {
		languageIds = append(languageIds, langId)
	}
	s.mu.RUnlock()

	results := make(map[string]error)

	for _, langId := range languageIds {
		if !s.checkHealth(langId) {
			s.logger.Info("Recovering unhealthy LSP server",
				"languageId", langId,
			)

			if err := s.restart(langId); err != nil {
				results[langId] = err
			}
		}
	}

	return results
}
