package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// MaxMessageSize is the maximum size for a single MCP message (1MB).
// This accommodates large tool responses and batch operations.
const MaxMessageSize = 1024 * 1024

// readMessage reads a JSON-RPC message from the input stream
func (s *MCPServer) readMessage() (*MCPMessage, error) {
	// Lazily initialize the scanner on first use
	if s.scanner == nil {
		s.scanner = bufio.NewScanner(s.stdin)
		// Increase buffer size beyond default 64KB to handle large messages
		s.scanner.Buffer(make([]byte, MaxMessageSize), MaxMessageSize)
	}

	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return nil, fmt.Errorf("error reading from stdin: %w", err)
		}
		return nil, io.EOF
	}

	line := s.scanner.Text()
	s.logger.Debug("Received message", map[string]interface{}{
		"raw": line,
	})

	var msg MCPMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil, fmt.Errorf("error parsing JSON-RPC message: %w", err)
	}

	return &msg, nil
}

// writeMessage writes a JSON-RPC message to the output stream
func (s *MCPServer) writeMessage(msg *MCPMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("error marshaling JSON-RPC message: %w", err)
	}

	s.logger.Debug("Sending message", map[string]interface{}{
		"raw": string(data),
	})

	if _, err := fmt.Fprintf(s.stdout, "%s\n", data); err != nil {
		return fmt.Errorf("error writing to stdout: %w", err)
	}

	return nil
}

// writeError writes an error response
func (s *MCPServer) writeError(id interface{}, code int, message string) error {
	return s.writeMessage(NewErrorMessage(id, code, message, nil))
}

// writeErrorWithData writes an error response with additional data (kept for future use)
var _ = (*MCPServer).writeErrorWithData

func (s *MCPServer) writeErrorWithData(id interface{}, code int, message string, data interface{}) error {
	return s.writeMessage(NewErrorMessage(id, code, message, data))
}

// writeResult writes a successful result response (kept for future use)
var _ = (*MCPServer).writeResult

func (s *MCPServer) writeResult(id interface{}, result interface{}) error {
	return s.writeMessage(NewResultMessage(id, result))
}
