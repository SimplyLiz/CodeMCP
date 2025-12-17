package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// JsonRpcMessage represents a JSON-RPC 2.0 message
type JsonRpcMessage struct {
	Jsonrpc string      `json:"jsonrpc"`
	Id      *int        `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"`
	Params  interface{} `json:"params,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RpcError   `json:"error,omitempty"`
}

// RpcError represents a JSON-RPC error
type RpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// JSON-RPC error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// sendRequest sends a JSON-RPC request and waits for the response
func (p *LspProcess) sendRequest(method string, params interface{}) (interface{}, error) {
	// Get next message ID
	p.requestsMu.Lock()
	id := p.nextMessageID
	p.nextMessageID++

	// Create response channel
	respChan := make(chan *JsonRpcMessage, 1)
	p.pendingRequests[id] = respChan
	p.requestsMu.Unlock()

	// Build request
	msg := JsonRpcMessage{
		Jsonrpc: "2.0",
		Id:      &id,
		Method:  method,
		Params:  params,
	}

	// Send request
	if err := p.writeMessage(&msg); err != nil {
		p.requestsMu.Lock()
		delete(p.pendingRequests, id)
		p.requestsMu.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response with timeout
	select {
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, fmt.Errorf("LSP error [%d]: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-time.After(30 * time.Second):
		p.requestsMu.Lock()
		delete(p.pendingRequests, id)
		p.requestsMu.Unlock()
		return nil, fmt.Errorf("request timeout")
	case <-p.done:
		return nil, fmt.Errorf("process shutting down")
	}
}

// sendNotification sends a JSON-RPC notification (no response expected)
func (p *LspProcess) sendNotification(method string, params interface{}) error {
	msg := JsonRpcMessage{
		Jsonrpc: "2.0",
		Method:  method,
		Params:  params,
	}

	return p.writeMessage(&msg)
}

// writeMessage writes a JSON-RPC message to the process stdin
func (p *LspProcess) writeMessage(msg *JsonRpcMessage) error {
	if p.stdin == nil {
		return fmt.Errorf("stdin not available")
	}

	// Marshal to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write LSP header + content
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))

	if _, err := p.stdin.Write([]byte(header)); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	if _, err := p.stdin.Write(data); err != nil {
		return fmt.Errorf("failed to write content: %w", err)
	}

	return nil
}

// readLoop continuously reads messages from the LSP server
func (p *LspProcess) readLoop() {
	defer func() {
		p.SetState(StateDead)

		// Clean up pending requests
		p.requestsMu.Lock()
		for _, ch := range p.pendingRequests {
			close(ch)
		}
		p.pendingRequests = make(map[int]chan *JsonRpcMessage)
		p.requestsMu.Unlock()
	}()

	reader := bufio.NewReader(p.stdout)

	for {
		select {
		case <-p.done:
			return
		default:
			// Read message
			msg, err := p.readMessage(reader)
			if err != nil {
				if err == io.EOF {
					return
				}
				// Continue on error, LSP might send malformed messages
				continue
			}

			// Handle message
			p.handleMessage(msg)
		}
	}
}

// readMessage reads a single LSP message (header + content)
func (p *LspProcess) readMessage(reader *bufio.Reader) (*JsonRpcMessage, error) {
	// Read headers
	headers := make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break // End of headers
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	// Get content length
	contentLengthStr, ok := headers["Content-Length"]
	if !ok {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	contentLength, err := strconv.Atoi(contentLengthStr)
	if err != nil {
		return nil, fmt.Errorf("invalid Content-Length: %w", err)
	}

	// Read content
	content := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, content); err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	// Parse JSON
	var msg JsonRpcMessage
	if err := json.Unmarshal(content, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &msg, nil
}

// handleMessage handles an incoming message from the LSP server
func (p *LspProcess) handleMessage(msg *JsonRpcMessage) {
	// If this is a response (has an ID and no method)
	if msg.Id != nil && msg.Method == "" {
		p.requestsMu.Lock()
		respChan, ok := p.pendingRequests[*msg.Id]
		if ok {
			delete(p.pendingRequests, *msg.Id)
		}
		p.requestsMu.Unlock()

		if ok {
			select {
			case respChan <- msg:
			default:
				// Channel full or closed, ignore
			}
		}
		return
	}

	// If this is a request or notification from the server
	if msg.Method != "" {
		// Handle server-to-client notifications/requests
		p.handleServerMessage(msg)
	}
}

// handleServerMessage handles server-initiated messages
func (p *LspProcess) handleServerMessage(msg *JsonRpcMessage) {
	// Handle common server notifications
	switch msg.Method {
	case "window/logMessage":
		// Server sent a log message, we could log it
		// For now, just ignore
	case "textDocument/publishDiagnostics":
		// Server sent diagnostics, we don't need these for our use case
		// Ignore
	case "$/progress":
		// Progress notifications, ignore
	default:
		// Unknown server message, ignore
	}

	// If this was a request (has ID), send empty response
	if msg.Id != nil {
		resp := JsonRpcMessage{
			Jsonrpc: "2.0",
			Id:      msg.Id,
			Result:  nil,
		}
		_ = p.writeMessage(&resp)
	}
}

// stderrLoop reads stderr and logs it
func (p *LspProcess) stderrLoop() {
	if p.stderr == nil {
		return
	}

	buf := make([]byte, 4096)
	for {
		select {
		case <-p.done:
			return
		default:
			n, err := p.stderr.Read(buf)
			if err != nil {
				// EOF or other error - stop reading stderr
				return
			}

			if n > 0 {
				// Could log stderr output here
				// For now, just read and discard
				_ = buf[:n]
			}
		}
	}
}

// formatLspMessage formats a message for debugging
func formatLspMessage(msg *JsonRpcMessage) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(msg)
	return buf.String()
}
