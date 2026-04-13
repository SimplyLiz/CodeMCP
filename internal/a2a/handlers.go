package a2a

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// === Shared do* methods (used by both HTTP+JSON and JSON-RPC bindings) ===

// doSendMessage processes a message/send request synchronously.
func (s *Server) doSendMessage(req SendMessageRequest) (*Task, *A2AError) {
	// Parse the skill request from the user message
	skillID, params, err := ParseSkillRequest(req.Message)
	if err != nil {
		return nil, NewInvalidParamsError(err.Error())
	}

	// Validate skill exists
	if s.skills.GetSkill(skillID) == nil {
		return nil, NewInvalidParamsError(fmt.Sprintf("unknown skill: %s", skillID))
	}

	// Determine context ID
	contextID := req.Message.ContextID
	if contextID == "" && req.Metadata != nil {
		if cid, ok := req.Metadata["contextId"].(string); ok {
			contextID = cid
		}
	}

	// Check if this is a follow-up to an existing task
	if req.Message.TaskID != "" {
		return s.doFollowUp(req)
	}

	// Create new task
	task, err := s.store.CreateTask(contextID, req.Metadata)
	if err != nil {
		return nil, NewInternalError(fmt.Sprintf("create task: %v", err))
	}

	// Store the user message
	userMsg := req.Message
	userMsg.MessageID = uuid.New().String()
	if storeErr := s.store.AddMessage(task.ID, userMsg); storeErr != nil {
		s.logger.Warn("Failed to store user message", "error", storeErr.Error())
	}

	// Transition to working
	if storeErr := s.store.UpdateTaskState(task.ID, TaskStateWorking, nil); storeErr != nil {
		return nil, NewInternalError(fmt.Sprintf("transition to working: %v", storeErr))
	}
	s.notify(task.ID, StreamResponse{
		StatusUpdate: &TaskStatusUpdateEvent{
			TaskID:    task.ID,
			ContextID: task.ContextID,
			Status:    TaskStatus{State: TaskStateWorking, Timestamp: nowISO()},
		},
	})

	// Execute the skill via MCP tool handler
	result, toolErr := s.mcpServer.CallTool(skillID, params)

	if toolErr != nil {
		// Task failed
		failMsg := Message{
			MessageID: uuid.New().String(),
			Role:      RoleAgent,
			Parts:     []Part{{Text: fmt.Sprintf("skill execution failed: %v", toolErr), MediaType: "text/plain"}},
		}
		_ = s.store.AddMessage(task.ID, failMsg)
		_ = s.store.UpdateTaskState(task.ID, TaskStateFailed, &failMsg)
		s.notify(task.ID, StreamResponse{
			StatusUpdate: &TaskStatusUpdateEvent{
				TaskID:    task.ID,
				ContextID: task.ContextID,
				Status:    TaskStatus{State: TaskStateFailed, Timestamp: nowISO(), Message: &failMsg},
			},
		})
		return s.getTaskOrError(task.ID)
	}

	// Store artifacts from result
	artifacts := EnvelopeToArtifacts(result, skillID)
	for _, art := range artifacts {
		if storeErr := s.store.AddArtifact(task.ID, art); storeErr != nil {
			s.logger.Warn("Failed to store artifact", "error", storeErr.Error())
		}
		s.notify(task.ID, StreamResponse{
			ArtifactUpdate: &TaskArtifactUpdateEvent{
				TaskID:    task.ID,
				ContextID: task.ContextID,
				Artifact:  art,
			},
		})
	}

	// Store agent response message
	agentMsg := EnvelopeToMessage(result, skillID)
	_ = s.store.AddMessage(task.ID, agentMsg)

	// Mark completed
	_ = s.store.UpdateTaskState(task.ID, TaskStateCompleted, &agentMsg)
	s.notify(task.ID, StreamResponse{
		StatusUpdate: &TaskStatusUpdateEvent{
			TaskID:    task.ID,
			ContextID: task.ContextID,
			Status:    TaskStatus{State: TaskStateCompleted, Timestamp: nowISO(), Message: &agentMsg},
		},
	})

	// Attach index freshness warnings to task metadata
	s.attachIndexWarnings(task.ID)

	// Return final task
	finalTask, getErr := s.store.GetTask(task.ID, nil)
	if getErr != nil {
		return nil, NewInternalError(fmt.Sprintf("get final task: %v", getErr))
	}
	return finalTask, nil
}

// doFollowUp handles a message sent to an existing task (multi-turn).
func (s *Server) doFollowUp(req SendMessageRequest) (*Task, *A2AError) {
	taskID := req.Message.TaskID
	task, err := s.store.GetTask(taskID, nil)
	if err != nil {
		return nil, NewInternalError(fmt.Sprintf("get task: %v", err))
	}
	if task == nil {
		return nil, NewTaskNotFoundError(taskID)
	}

	// Can only follow up on input-required or auth-required tasks
	if task.Status.State != TaskStateInputRequired && task.Status.State != TaskStateAuthRequired {
		return nil, &A2AError{
			Code:       ErrCodeInvalidRequest,
			Message:    fmt.Sprintf("task %s is in state %s, cannot accept follow-up", taskID, task.Status.State),
			HTTPStatus: http.StatusConflict,
		}
	}

	// Store the follow-up message
	userMsg := req.Message
	userMsg.MessageID = uuid.New().String()
	_ = s.store.AddMessage(taskID, userMsg)

	// Transition back to working
	_ = s.store.UpdateTaskState(taskID, TaskStateWorking, nil)

	// Re-execute the skill with new params
	skillID, params, parseErr := ParseSkillRequest(req.Message)
	if parseErr != nil {
		return nil, NewInvalidParamsError(parseErr.Error())
	}

	result, toolErr := s.mcpServer.CallTool(skillID, params)
	if toolErr != nil {
		failMsg := Message{
			MessageID: uuid.New().String(),
			Role:      RoleAgent,
			Parts:     []Part{{Text: fmt.Sprintf("skill execution failed: %v", toolErr), MediaType: "text/plain"}},
		}
		_ = s.store.AddMessage(taskID, failMsg)
		_ = s.store.UpdateTaskState(taskID, TaskStateFailed, &failMsg)
		return s.getTaskOrError(taskID)
	}

	artifacts := EnvelopeToArtifacts(result, skillID)
	for _, art := range artifacts {
		_ = s.store.AddArtifact(taskID, art)
	}

	agentMsg := EnvelopeToMessage(result, skillID)
	_ = s.store.AddMessage(taskID, agentMsg)
	_ = s.store.UpdateTaskState(taskID, TaskStateCompleted, &agentMsg)

	return s.getTaskOrError(taskID)
}

// doGetTask retrieves a task by ID.
func (s *Server) doGetTask(req GetTaskRequest) (*Task, *A2AError) {
	task, err := s.store.GetTask(req.TaskID, req.HistoryLength)
	if err != nil {
		return nil, NewInternalError(fmt.Sprintf("get task: %v", err))
	}
	if task == nil {
		return nil, NewTaskNotFoundError(req.TaskID)
	}
	return task, nil
}

// doListTasks lists tasks with pagination.
func (s *Server) doListTasks(req ListTasksRequest) (*ListTasksResponse, *A2AError) {
	resp, err := s.store.ListTasks(req)
	if err != nil {
		return nil, NewInternalError(fmt.Sprintf("list tasks: %v", err))
	}
	return resp, nil
}

// doCancelTask cancels a running task.
func (s *Server) doCancelTask(req CancelTaskRequest) (*Task, *A2AError) {
	task, err := s.store.GetTask(req.TaskID, nil)
	if err != nil {
		return nil, NewInternalError(fmt.Sprintf("get task: %v", err))
	}
	if task == nil {
		return nil, NewTaskNotFoundError(req.TaskID)
	}

	if !CanCancel(task.Status.State) {
		return nil, NewTaskNotCancelableError(req.TaskID)
	}

	cancelMsg := Message{
		MessageID: uuid.New().String(),
		Role:      RoleAgent,
		Parts:     []Part{{Text: "task canceled by client", MediaType: "text/plain"}},
	}
	if err = s.store.UpdateTaskState(req.TaskID, TaskStateCanceled, &cancelMsg); err != nil {
		return nil, NewInternalError(fmt.Sprintf("cancel task: %v", err))
	}

	s.notify(req.TaskID, StreamResponse{
		StatusUpdate: &TaskStatusUpdateEvent{
			TaskID: req.TaskID,
			Status: TaskStatus{State: TaskStateCanceled, Timestamp: nowISO(), Message: &cancelMsg},
		},
	})

	return s.getTaskOrError(req.TaskID)
}

// doCreatePushConfig creates a push notification config.
func (s *Server) doCreatePushConfig(taskID string, cfg PushNotificationConfig) (*PushNotificationConfig, *A2AError) {
	// Verify task exists
	task, err := s.store.GetTask(taskID, nil)
	if err != nil || task == nil {
		return nil, NewTaskNotFoundError(taskID)
	}

	created, err := s.store.CreatePushConfig(taskID, cfg)
	if err != nil {
		return nil, NewInternalError(fmt.Sprintf("create push config: %v", err))
	}
	return created, nil
}

// doGetPushConfig retrieves a push notification config.
func (s *Server) doGetPushConfig(configID string) (*PushNotificationConfig, *A2AError) {
	cfg, err := s.store.GetPushConfig(configID)
	if err != nil {
		return nil, NewInternalError(fmt.Sprintf("get push config: %v", err))
	}
	if cfg == nil {
		return nil, &A2AError{
			Code:       ErrCodeTaskNotFound,
			Message:    fmt.Sprintf("push config not found: %s", configID),
			HTTPStatus: http.StatusNotFound,
		}
	}
	return cfg, nil
}

// doListPushConfigs lists push notification configs for a task.
func (s *Server) doListPushConfigs(taskID string) ([]PushNotificationConfig, *A2AError) {
	configs, err := s.store.ListPushConfigs(taskID)
	if err != nil {
		return nil, NewInternalError(fmt.Sprintf("list push configs: %v", err))
	}
	return configs, nil
}

// doDeletePushConfig deletes a push notification config.
func (s *Server) doDeletePushConfig(configID string) *A2AError {
	if err := s.store.DeletePushConfig(configID); err != nil {
		return NewInternalError(fmt.Sprintf("delete push config: %v", err))
	}
	return nil
}

// === HTTP+JSON Handlers ===

func (s *Server) handleHTTPMessageSend(w http.ResponseWriter, r *http.Request) {
	var req SendMessageRequest
	if err := decodeBody(r, &req); err != nil {
		writeA2AError(w, NewParseError(err.Error()))
		return
	}

	task, a2aErr := s.doSendMessage(req)
	if a2aErr != nil {
		writeA2AError(w, a2aErr)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleHTTPGetTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		writeA2AError(w, NewInvalidParamsError("task ID required"))
		return
	}

	var historyLength *int
	if hl := r.URL.Query().Get("historyLength"); hl != "" {
		if v, err := strconv.Atoi(hl); err == nil {
			historyLength = &v
		}
	}

	task, a2aErr := s.doGetTask(GetTaskRequest{TaskID: taskID, HistoryLength: historyLength})
	if a2aErr != nil {
		writeA2AError(w, a2aErr)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleHTTPListTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := ListTasksRequest{
		ContextID:        q.Get("contextId"),
		PageToken:        q.Get("pageToken"),
		IncludeArtifacts: q.Get("includeArtifacts") == "true",
	}
	if ps := q.Get("pageSize"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil {
			req.PageSize = v
		}
	}
	if status := q.Get("status"); status != "" {
		s := TaskState(status)
		req.Status = &s
	}
	if hl := q.Get("historyLength"); hl != "" {
		if v, err := strconv.Atoi(hl); err == nil {
			req.HistoryLength = &v
		}
	}

	resp, a2aErr := s.doListTasks(req)
	if a2aErr != nil {
		writeA2AError(w, a2aErr)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHTTPCancelTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		writeA2AError(w, NewInvalidParamsError("task ID required"))
		return
	}

	task, a2aErr := s.doCancelTask(CancelTaskRequest{TaskID: taskID})
	if a2aErr != nil {
		writeA2AError(w, a2aErr)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleHTTPCreatePushConfig(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	var cfg PushNotificationConfig
	if err := decodeBody(r, &cfg); err != nil {
		writeA2AError(w, NewParseError(err.Error()))
		return
	}

	created, a2aErr := s.doCreatePushConfig(taskID, cfg)
	if a2aErr != nil {
		writeA2AError(w, a2aErr)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleHTTPGetPushConfig(w http.ResponseWriter, r *http.Request) {
	configID := r.PathValue("configId")
	cfg, a2aErr := s.doGetPushConfig(configID)
	if a2aErr != nil {
		writeA2AError(w, a2aErr)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleHTTPListPushConfigs(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	configs, a2aErr := s.doListPushConfigs(taskID)
	if a2aErr != nil {
		writeA2AError(w, a2aErr)
		return
	}
	writeJSON(w, http.StatusOK, configs)
}

func (s *Server) handleHTTPDeletePushConfig(w http.ResponseWriter, r *http.Request) {
	configID := r.PathValue("configId")
	if a2aErr := s.doDeletePushConfig(configID); a2aErr != nil {
		writeA2AError(w, a2aErr)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// === Helpers ===

func decodeBody(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if err = json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// getTaskOrError wraps store.GetTask with A2A error conversion.
func (s *Server) getTaskOrError(taskID string) (*Task, *A2AError) {
	task, err := s.store.GetTask(taskID, nil)
	if err != nil {
		return nil, NewInternalError(fmt.Sprintf("get task: %v", err))
	}
	if task == nil {
		return nil, NewTaskNotFoundError(taskID)
	}
	return task, nil
}

// attachIndexWarnings checks index freshness and adds warnings to task metadata.
// This surfaces reindex hints to consuming agents so they know results may be stale.
func (s *Server) attachIndexWarnings(taskID string) {
	indexStatus := s.getIndexStatus()

	initialized, _ := indexStatus["initialized"].(bool)
	fresh, _ := indexStatus["fresh"].(bool)

	if initialized && fresh {
		return
	}

	var warnings []string
	if !initialized {
		warnings = append(warnings, "CKB index not initialized — results are limited to git-based features. Run 'ckb init && ckb index' or use the 'reindex' skill.")
	} else if !fresh {
		if commitsBehind, ok := indexStatus["commitsBehind"].(int); ok && commitsBehind > 0 {
			warnings = append(warnings, fmt.Sprintf("CKB index is %d commit(s) behind — results may be stale. Use the 'reindex' skill to refresh.", commitsBehind))
		} else {
			warnings = append(warnings, "CKB index may be stale. Use the 'reindex' skill to refresh.")
		}
	}

	if len(warnings) == 0 {
		return
	}

	// Store warnings as task metadata
	task, err := s.store.GetTask(taskID, nil)
	if err != nil || task == nil {
		return
	}

	meta := task.Metadata
	if meta == nil {
		meta = make(map[string]any)
	}
	meta["indexWarnings"] = warnings
	meta["indexStatus"] = indexStatus

	// Update metadata in the store (best-effort, non-critical)
	_ = s.store.UpdateTaskMetadata(taskID, meta)
}
