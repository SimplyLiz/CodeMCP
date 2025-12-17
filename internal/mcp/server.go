package mcp

import (
	"fmt"
	"io"
	"os"

	"ckb/internal/logging"
	"ckb/internal/query"
)

// MCPServer represents the MCP server
type MCPServer struct {
	stdin     io.Reader
	stdout    io.Writer
	logger    *logging.Logger
	version   string
	engine    *query.Engine
	tools     map[string]ToolHandler
	resources map[string]ResourceHandler
}

// NewMCPServer creates a new MCP server
func NewMCPServer(version string, engine *query.Engine, logger *logging.Logger) *MCPServer {
	server := &MCPServer{
		stdin:     os.Stdin,
		stdout:    os.Stdout,
		logger:    logger,
		version:   version,
		engine:    engine,
		tools:     make(map[string]ToolHandler),
		resources: make(map[string]ResourceHandler),
	}

	// Register all tools
	server.RegisterTools()

	return server
}

// Start starts the MCP server and begins processing messages
func (s *MCPServer) Start() error {
	s.logger.Info("MCP server starting", map[string]interface{}{
		"version": s.version,
	})

	// Main message loop
	for {
		msg, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				s.logger.Info("MCP server shutting down (EOF)", nil)
				return nil
			}
			s.logger.Error("Error reading message", map[string]interface{}{
				"error": err.Error(),
			})

			// Try to send error response if we can extract an ID
			if msg != nil && msg.Id != nil {
				_ = s.writeError(msg.Id, ParseError, fmt.Sprintf("Failed to parse message: %v", err))
			}
			continue
		}

		// Process the message
		response := s.handleMessage(msg)

		// Write response if one was generated (notifications don't generate responses)
		if response != nil {
			if err := s.writeMessage(response); err != nil {
				s.logger.Error("Error writing response", map[string]interface{}{
					"error": err.Error(),
				})
			}
		}
	}
}

// SetStdin sets the input stream (for testing)
func (s *MCPServer) SetStdin(r io.Reader) {
	s.stdin = r
}

// SetStdout sets the output stream (for testing)
func (s *MCPServer) SetStdout(w io.Writer) {
	s.stdout = w
}
