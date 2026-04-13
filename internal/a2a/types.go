package a2a

import "encoding/json"

// A2A Protocol v0.3 types
// https://github.com/a2aproject/A2A

// --- Enums ---

// TaskState represents the A2A task lifecycle states (SCREAMING_SNAKE_CASE per proto spec).
type TaskState string

const (
	TaskStateSubmitted     TaskState = "TASK_STATE_SUBMITTED"
	TaskStateWorking       TaskState = "TASK_STATE_WORKING"
	TaskStateInputRequired TaskState = "TASK_STATE_INPUT_REQUIRED"
	TaskStateAuthRequired  TaskState = "TASK_STATE_AUTH_REQUIRED"
	TaskStateCompleted     TaskState = "TASK_STATE_COMPLETED"
	TaskStateFailed        TaskState = "TASK_STATE_FAILED"
	TaskStateCanceled      TaskState = "TASK_STATE_CANCELED"
	TaskStateRejected      TaskState = "TASK_STATE_REJECTED"
)

// Role identifies the sender of a message.
type Role string

const (
	RoleUser  Role = "ROLE_USER"
	RoleAgent Role = "ROLE_AGENT"
)

// ProtocolBinding identifies the wire protocol.
type ProtocolBinding string

const (
	BindingJSONRPC  ProtocolBinding = "JSONRPC"
	BindingHTTPJSON ProtocolBinding = "HTTP+JSON"
)

// --- Core Data Model (Layer 1) ---

// Task is the core stateful object in A2A.
type Task struct {
	ID            string                 `json:"id"`
	ContextID     string                 `json:"contextId,omitempty"`
	Status        TaskStatus             `json:"status"`
	History       []Message              `json:"history,omitempty"`
	Artifacts     []Artifact             `json:"artifacts,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	StatusHistory []TaskStatus           `json:"statusHistory,omitempty"`
}

// TaskStatus represents the current state of a task.
type TaskStatus struct {
	State     TaskState `json:"state"`
	Timestamp string    `json:"timestamp"`
	Message   *Message  `json:"message,omitempty"`
}

// Message represents a communication between user and agent.
type Message struct {
	MessageID        string                 `json:"messageId,omitempty"`
	Role             Role                   `json:"role"`
	Parts            []Part                 `json:"parts"`
	TaskID           string                 `json:"taskId,omitempty"`
	ContextID        string                 `json:"contextId,omitempty"`
	ReferenceTaskIDs []string               `json:"referenceTaskIds,omitempty"`
	Extensions       []string               `json:"extensions,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// Part represents a content element within a message or artifact.
type Part struct {
	Text      string                 `json:"text,omitempty"`
	Data      string                 `json:"data,omitempty"` // base64 raw binary
	Filename  string                 `json:"filename,omitempty"`
	MediaType string                 `json:"mediaType,omitempty"`
	URL       string                 `json:"url,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Artifact represents a named output produced by a task.
type Artifact struct {
	ArtifactID string                 `json:"artifactId"`
	Name       string                 `json:"name"`
	Parts      []Part                 `json:"parts"`
	Extensions []string               `json:"extensions,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// --- Agent Card ---

// AgentCard describes an A2A agent's capabilities and endpoint.
type AgentCard struct {
	Name                string                    `json:"name"`
	Description         string                    `json:"description,omitempty"`
	Version             string                    `json:"version,omitempty"`
	Provider            *Provider                 `json:"provider,omitempty"`
	IconURL             string                    `json:"iconUrl,omitempty"`
	DocumentationURL    string                    `json:"documentationUrl,omitempty"`
	SupportedInterfaces []SupportedInterface      `json:"supportedInterfaces"`
	Capabilities        *Capabilities             `json:"capabilities,omitempty"`
	SecuritySchemes     map[string]SecurityScheme `json:"securitySchemes,omitempty"`
	Security            []map[string][]string     `json:"security,omitempty"`
	DefaultInputModes   []string                  `json:"defaultInputModes,omitempty"`
	DefaultOutputModes  []string                  `json:"defaultOutputModes,omitempty"`
	Skills              []Skill                   `json:"skills"`
	Signatures          []json.RawMessage         `json:"signatures,omitempty"`
}

// Provider describes the organization behind an agent.
type Provider struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}

// SupportedInterface describes a protocol endpoint.
type SupportedInterface struct {
	URL             string          `json:"url"`
	ProtocolBinding ProtocolBinding `json:"protocolBinding"`
	ProtocolVersion string          `json:"protocolVersion"`
}

// Capabilities declares optional features the agent supports.
type Capabilities struct {
	Streaming              bool            `json:"streaming,omitempty"`
	PushNotifications      bool            `json:"pushNotifications,omitempty"`
	ExtendedAgentCard      bool            `json:"extendedAgentCard,omitempty"`
	StateTransitionHistory bool            `json:"stateTransitionHistory,omitempty"`
	Extensions             []ExtensionDecl `json:"extensions,omitempty"`
}

// ExtensionDecl declares an extension the agent supports.
type ExtensionDecl struct {
	URI         string `json:"uri"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// SecurityScheme describes an authentication mechanism.
type SecurityScheme struct {
	Type             string      `json:"type"` // apiKey, http, oauth2, openIdConnect, mutualTLS
	Description      string      `json:"description,omitempty"`
	Name             string      `json:"name,omitempty"`         // apiKey: header/query/cookie name
	In               string      `json:"in,omitempty"`           // apiKey: header, query, cookie
	Scheme           string      `json:"scheme,omitempty"`       // http: bearer, basic
	BearerFormat     string      `json:"bearerFormat,omitempty"` // http: JWT, etc.
	Flows            *OAuthFlows `json:"flows,omitempty"`        // oauth2
	OpenIDConnectURL string      `json:"openIdConnectUrl,omitempty"`
}

// OAuthFlows describes OAuth 2.0 flows.
type OAuthFlows struct {
	AuthorizationCode *OAuthFlow `json:"authorizationCode,omitempty"`
	ClientCredentials *OAuthFlow `json:"clientCredentials,omitempty"`
	DeviceCode        *OAuthFlow `json:"deviceCode,omitempty"`
}

// OAuthFlow describes a single OAuth 2.0 flow.
type OAuthFlow struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty"`
	DeviceAuthURL    string            `json:"deviceAuthorizationUrl,omitempty"`
	Scopes           map[string]string `json:"scopes,omitempty"`
}

// Skill describes a capability the agent advertises.
type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
	InputModes  []string `json:"inputModes,omitempty"`
	OutputModes []string `json:"outputModes,omitempty"`
}

// --- Request/Response Types ---

// SendMessageRequest is the request body for message/send.
type SendMessageRequest struct {
	Message       Message                   `json:"message"`
	Configuration *SendMessageConfiguration `json:"configuration,omitempty"`
	Metadata      map[string]interface{}    `json:"metadata,omitempty"`
}

// SendMessageConfiguration controls send behavior.
type SendMessageConfiguration struct {
	ReturnImmediately      bool                    `json:"returnImmediately,omitempty"`
	PushNotificationConfig *PushNotificationConfig `json:"pushNotificationConfig,omitempty"`
}

// SendMessageResponse can be either a Task or a Message.
type SendMessageResponse struct {
	Task    *Task    `json:"task,omitempty"`
	Message *Message `json:"message,omitempty"`
}

// GetTaskRequest is the request for tasks/get.
type GetTaskRequest struct {
	TaskID        string `json:"taskId"`
	HistoryLength *int   `json:"historyLength,omitempty"`
}

// ListTasksRequest is the request for tasks/list.
type ListTasksRequest struct {
	ContextID        string     `json:"contextId,omitempty"`
	Status           *TaskState `json:"status,omitempty"`
	PageSize         int        `json:"pageSize,omitempty"`
	PageToken        string     `json:"pageToken,omitempty"`
	IncludeArtifacts bool       `json:"includeArtifacts,omitempty"`
	HistoryLength    *int       `json:"historyLength,omitempty"`
}

// ListTasksResponse is the response for tasks/list.
type ListTasksResponse struct {
	Tasks         []Task `json:"tasks"`
	TotalSize     int    `json:"totalSize,omitempty"`
	PageSize      int    `json:"pageSize,omitempty"`
	NextPageToken string `json:"nextPageToken,omitempty"`
}

// CancelTaskRequest is the request for tasks/cancel.
type CancelTaskRequest struct {
	TaskID string `json:"taskId"`
}

// SubscribeToTaskRequest is the request for tasks/subscribe.
type SubscribeToTaskRequest struct {
	TaskID        string `json:"taskId"`
	HistoryLength *int   `json:"historyLength,omitempty"`
}

// --- Push Notifications ---

// PushNotificationConfig defines a webhook endpoint for task updates.
type PushNotificationConfig struct {
	ID             string              `json:"id,omitempty"`
	URL            string              `json:"url"`
	TaskID         string              `json:"taskId,omitempty"`
	Authentication *AuthenticationInfo `json:"authentication,omitempty"`
}

// AuthenticationInfo provides credentials for push notification delivery.
type AuthenticationInfo struct {
	Schemes []string `json:"schemes,omitempty"`
	Token   string   `json:"token,omitempty"`
}

// CreatePushNotificationConfigRequest is the request to create a push config.
type CreatePushNotificationConfigRequest struct {
	TaskID         string              `json:"taskId"`
	URL            string              `json:"url"`
	Authentication *AuthenticationInfo `json:"authentication,omitempty"`
}

// --- Streaming ---

// StreamResponse is an SSE event payload containing exactly one field.
type StreamResponse struct {
	Task           *Task                    `json:"task,omitempty"`
	Message        *Message                 `json:"message,omitempty"`
	StatusUpdate   *TaskStatusUpdateEvent   `json:"statusUpdate,omitempty"`
	ArtifactUpdate *TaskArtifactUpdateEvent `json:"artifactUpdate,omitempty"`
}

// TaskStatusUpdateEvent is a streaming status change notification.
type TaskStatusUpdateEvent struct {
	TaskID    string     `json:"taskId"`
	ContextID string     `json:"contextId,omitempty"`
	Status    TaskStatus `json:"status"`
}

// TaskArtifactUpdateEvent is a streaming artifact notification.
type TaskArtifactUpdateEvent struct {
	TaskID    string   `json:"taskId"`
	ContextID string   `json:"contextId,omitempty"`
	Artifact  Artifact `json:"artifact"`
}

// --- JSON-RPC ---

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id,omitempty"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// --- Extended Agent Card ---

// GetExtendedAgentCardRequest is the request for the extended card.
type GetExtendedAgentCardRequest struct{}

// ExtendedSkill extends Skill with full input/output schema.
type ExtendedSkill struct {
	Skill
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// --- Constants ---

const (
	// ProtocolVersion is the A2A protocol version this implementation supports.
	ProtocolVersion = "0.3"

	// WellKnownPath is the discovery endpoint for the agent card.
	WellKnownPath = "/.well-known/agent-card.json"

	// DefaultPageSize is the default number of tasks returned per page.
	DefaultPageSize = 20

	// MaxPageSize is the maximum number of tasks returned per page.
	MaxPageSize = 100
)
