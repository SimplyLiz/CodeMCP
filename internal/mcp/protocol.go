package mcp

// MCPMessage represents a JSON-RPC 2.0 message for MCP
type MCPMessage struct {
	Jsonrpc string      `json:"jsonrpc"`
	Id      interface{} `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"`
	Params  interface{} `json:"params,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCPError represents a JSON-RPC 2.0 error
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error implements the error interface
func (e *MCPError) Error() string {
	return e.Message
}

// Standard JSON-RPC error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// NewErrorMessage creates a new error response message
func NewErrorMessage(id interface{}, code int, message string, data interface{}) *MCPMessage {
	return &MCPMessage{
		Jsonrpc: "2.0",
		Id:      id,
		Error: &MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// NewResultMessage creates a new result response message
func NewResultMessage(id interface{}, result interface{}) *MCPMessage {
	return &MCPMessage{
		Jsonrpc: "2.0",
		Id:      id,
		Result:  result,
	}
}

// NewNotificationMessage creates a new notification message (no id)
func NewNotificationMessage(method string, params interface{}) *MCPMessage {
	return &MCPMessage{
		Jsonrpc: "2.0",
		Method:  method,
		Params:  params,
	}
}

// IsRequest checks if the message is a request
func (m *MCPMessage) IsRequest() bool {
	return m.Method != "" && m.Id != nil
}

// IsNotification checks if the message is a notification
func (m *MCPMessage) IsNotification() bool {
	return m.Method != "" && m.Id == nil
}

// IsResponse checks if the message is a response (must have id and either result or error)
func (m *MCPMessage) IsResponse() bool {
	return m.Id != nil && (m.Result != nil || m.Error != nil)
}
