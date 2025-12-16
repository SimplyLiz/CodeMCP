package lsp

import (
	"bufio"
	"io"
	"os/exec"
	"sync"
	"time"
)

// LspProcessState represents the state of an LSP server process
type LspProcessState string

const (
	// StateStarting indicates the process is being spawned
	StateStarting LspProcessState = "starting"
	// StateInitializing indicates the process is initializing (sent initialize request)
	StateInitializing LspProcessState = "initializing"
	// StateReady indicates the process is ready to handle requests
	StateReady LspProcessState = "ready"
	// StateUnhealthy indicates the process is not responding properly
	StateUnhealthy LspProcessState = "unhealthy"
	// StateDead indicates the process has terminated
	StateDead LspProcessState = "dead"
)

// LspProcess represents a running LSP server process
type LspProcess struct {
	// LanguageId is the language this process handles (typescript, dart, go, python, etc.)
	LanguageId string

	// WorkspaceRoot is the workspace directory
	WorkspaceRoot string

	// State is the current process state
	State LspProcessState

	// LastResponseTime tracks when we last got a successful response
	LastResponseTime time.Time

	// ConsecutiveFailures counts failed requests in a row
	ConsecutiveFailures int

	// RestartCount tracks how many times this process has been restarted
	RestartCount int

	// NextRestartAt indicates when this process can be restarted (for backoff)
	NextRestartAt time.Time

	// cmd is the underlying process
	cmd *exec.Cmd

	// stdin is the input stream to the process
	stdin io.WriteCloser

	// stdout is the output stream from the process
	stdout io.ReadCloser

	// stderr is the error stream from the process
	stderr io.ReadCloser

	// reader wraps stdout for reading responses
	reader *bufio.Reader

	// mu protects access to process state
	mu sync.RWMutex

	// nextMessageID tracks JSON-RPC message IDs
	nextMessageID int

	// pendingRequests maps message IDs to response channels
	pendingRequests map[int]chan *JsonRpcMessage

	// requestsMu protects pendingRequests
	requestsMu sync.RWMutex

	// done signals when the process should shut down
	done chan struct{}

	// capabilities stores the server's capabilities after initialization
	capabilities map[string]interface{}
}

// NewLspProcess creates a new LSP process (but doesn't start it yet)
func NewLspProcess(languageId, workspaceRoot string) *LspProcess {
	return &LspProcess{
		LanguageId:      languageId,
		WorkspaceRoot:   workspaceRoot,
		State:           StateStarting,
		pendingRequests: make(map[int]chan *JsonRpcMessage),
		done:            make(chan struct{}),
		capabilities:    make(map[string]interface{}),
	}
}

// GetState returns the current state (thread-safe)
func (p *LspProcess) GetState() LspProcessState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.State
}

// SetState sets the current state (thread-safe)
func (p *LspProcess) SetState(state LspProcessState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.State = state
}

// IsHealthy returns true if the process is ready to handle requests
func (p *LspProcess) IsHealthy() bool {
	state := p.GetState()
	return state == StateReady
}

// IsDead returns true if the process has terminated
func (p *LspProcess) IsDead() bool {
	state := p.GetState()
	return state == StateDead
}

// RecordSuccess records a successful request
func (p *LspProcess) RecordSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.LastResponseTime = time.Now()
	p.ConsecutiveFailures = 0
}

// RecordFailure records a failed request
func (p *LspProcess) RecordFailure() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ConsecutiveFailures++
}

// GetConsecutiveFailures returns the failure count
func (p *LspProcess) GetConsecutiveFailures() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ConsecutiveFailures
}

// CanRestart returns true if enough time has passed for restart
func (p *LspProcess) CanRestart() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return time.Now().After(p.NextRestartAt)
}

// IncrementRestartCount increments the restart counter
func (p *LspProcess) IncrementRestartCount() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.RestartCount++
}

// GetRestartCount returns the restart count
func (p *LspProcess) GetRestartCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.RestartCount
}

// SetNextRestartAt sets when the process can next be restarted
func (p *LspProcess) SetNextRestartAt(t time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.NextRestartAt = t
}

// GetLastResponseTime returns the last successful response time
func (p *LspProcess) GetLastResponseTime() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.LastResponseTime
}

// GetCapabilities returns the server capabilities
func (p *LspProcess) GetCapabilities() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.capabilities
}

// SetCapabilities sets the server capabilities
func (p *LspProcess) SetCapabilities(caps map[string]interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.capabilities = caps
}

// Shutdown gracefully shuts down the LSP process
func (p *LspProcess) Shutdown() error {
	// Signal shutdown
	close(p.done)

	// Send shutdown request
	if p.stdin != nil {
		p.sendNotification("shutdown", nil)
		p.sendNotification("exit", nil)
	}

	// Close streams
	if p.stdin != nil {
		p.stdin.Close()
	}
	if p.stdout != nil {
		p.stdout.Close()
	}
	if p.stderr != nil {
		p.stderr.Close()
	}

	// Kill process if it's still running
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}

	p.SetState(StateDead)
	return nil
}
