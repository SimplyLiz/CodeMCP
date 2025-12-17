package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// readMessage reads a JSON-RPC message from the input stream
func (s *MCPServer) readMessage() (*MCPMessage, error) {
	// Lazily initialize the scanner to ensure it uses the correct reader
	if s.scanner == nil {
		s.scanner = bufio.NewScanner(s.stdin)
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
