package a2a

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/envelope"
	"github.com/google/uuid"
)

const (
	sseHeartbeatInterval = 15 * time.Second
)

// handleHTTPMessageStream handles POST /message:stream — SSE streaming send.
func (s *Server) handleHTTPMessageStream(w http.ResponseWriter, r *http.Request) {
	var req SendMessageRequest
	if err := decodeBody(r, &req); err != nil {
		writeA2AError(w, NewParseError(err.Error()))
		return
	}
	s.doStreamingSend(w, r, req)
}

// handleHTTPSubscribeTask handles POST /tasks/{id}:subscribe — SSE subscription.
func (s *Server) handleHTTPSubscribeTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		writeA2AError(w, NewInvalidParamsError("task ID required"))
		return
	}
	s.doStreamSubscribe(w, r, taskID)
}

// doStreamingSend creates a task and streams execution progress via SSE.
func (s *Server) doStreamingSend(w http.ResponseWriter, r *http.Request, req SendMessageRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeA2AError(w, NewInternalError("streaming not supported"))
		return
	}

	// Parse skill request
	skillID, params, err := ParseSkillRequest(req.Message)
	if err != nil {
		writeA2AError(w, NewInvalidParamsError(err.Error()))
		return
	}
	if s.skills.GetSkill(skillID) == nil {
		writeA2AError(w, NewInvalidParamsError(fmt.Sprintf("unknown skill: %s", skillID)))
		return
	}

	// Create task
	contextID := req.Message.ContextID
	task, err := s.store.CreateTask(contextID, req.Metadata)
	if err != nil {
		writeA2AError(w, NewInternalError(fmt.Sprintf("create task: %v", err)))
		return
	}

	// Store user message
	userMsg := req.Message
	userMsg.MessageID = uuid.New().String()
	_ = s.store.AddMessage(task.ID, userMsg)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Send initial task state
	sendSSE(w, flusher, StreamResponse{
		Task: task,
	})

	// Transition to working
	_ = s.store.UpdateTaskState(task.ID, TaskStateWorking, nil)
	sendSSE(w, flusher, StreamResponse{
		StatusUpdate: &TaskStatusUpdateEvent{
			TaskID:    task.ID,
			ContextID: task.ContextID,
			Status:    TaskStatus{State: TaskStateWorking, Timestamp: nowISO()},
		},
	})

	// Execute skill in a goroutine for heartbeat support.
	// Note: MCP tool handlers don't accept context, so we can't cancel the
	// underlying operation. But we stop waiting and mark the task canceled
	// so the client isn't left hanging, and the goroutine drains into a
	// buffered channel (no leak).
	type toolResult struct {
		resp *envelope.Response
		err  error
	}
	resultCh := make(chan toolResult, 1)
	go func() {
		result, toolErr := s.mcpServer.CallTool(skillID, params)
		resultCh <- toolResult{resp: result, err: toolErr}
	}()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	var toolRes toolResult
	waiting := true
	for waiting {
		select {
		case <-r.Context().Done():
			// Client disconnected — cancel the task and stop streaming.
			// The goroutine will complete in the background and write to the
			// buffered channel (no leak), but we don't wait for it.
			cancelMsg := Message{
				MessageID: uuid.New().String(),
				Role:      RoleAgent,
				Parts:     []Part{{Text: "client disconnected", MediaType: "text/plain"}},
			}
			_ = s.store.AddMessage(task.ID, cancelMsg)
			_ = s.store.UpdateTaskState(task.ID, TaskStateCanceled, &cancelMsg)
			s.notify(task.ID, StreamResponse{
				StatusUpdate: &TaskStatusUpdateEvent{
					TaskID:    task.ID,
					ContextID: task.ContextID,
					Status:    TaskStatus{State: TaskStateCanceled, Timestamp: nowISO(), Message: &cancelMsg},
				},
			})
			return
		case <-heartbeat.C:
			sendSSEComment(w, flusher, "heartbeat")
		case toolRes = <-resultCh:
			waiting = false
		}
	}

	if toolRes.err != nil {
		failMsg := Message{
			MessageID: uuid.New().String(),
			Role:      RoleAgent,
			Parts:     []Part{{Text: fmt.Sprintf("skill execution failed: %v", toolRes.err), MediaType: "text/plain"}},
		}
		_ = s.store.AddMessage(task.ID, failMsg)
		_ = s.store.UpdateTaskState(task.ID, TaskStateFailed, &failMsg)
		sendSSE(w, flusher, StreamResponse{
			StatusUpdate: &TaskStatusUpdateEvent{
				TaskID:    task.ID,
				ContextID: task.ContextID,
				Status:    TaskStatus{State: TaskStateFailed, Timestamp: nowISO(), Message: &failMsg},
			},
		})
		finalTask, _ := s.store.GetTask(task.ID, nil)
		if finalTask != nil {
			sendSSE(w, flusher, StreamResponse{Task: finalTask})
		}
		return
	}

	// Emit artifacts
	artifacts := EnvelopeToArtifacts(toolRes.resp, skillID)
	for _, art := range artifacts {
		_ = s.store.AddArtifact(task.ID, art)
		sendSSE(w, flusher, StreamResponse{
			ArtifactUpdate: &TaskArtifactUpdateEvent{
				TaskID:    task.ID,
				ContextID: task.ContextID,
				Artifact:  art,
			},
		})
	}

	// Complete
	agentMsg := EnvelopeToMessage(toolRes.resp, skillID)
	_ = s.store.AddMessage(task.ID, agentMsg)
	_ = s.store.UpdateTaskState(task.ID, TaskStateCompleted, &agentMsg)

	sendSSE(w, flusher, StreamResponse{
		StatusUpdate: &TaskStatusUpdateEvent{
			TaskID:    task.ID,
			ContextID: task.ContextID,
			Status:    TaskStatus{State: TaskStateCompleted, Timestamp: nowISO(), Message: &agentMsg},
		},
	})

	// Send final task object
	finalTask, _ := s.store.GetTask(task.ID, nil)
	if finalTask != nil {
		sendSSE(w, flusher, StreamResponse{Task: finalTask})
	}
}

// doStreamSubscribe subscribes to an existing task's events via SSE.
func (s *Server) doStreamSubscribe(w http.ResponseWriter, r *http.Request, taskID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeA2AError(w, NewInternalError("streaming not supported"))
		return
	}

	// Verify task exists
	task, err := s.store.GetTask(taskID, nil)
	if err != nil || task == nil {
		writeA2AError(w, NewTaskNotFoundError(taskID))
		return
	}

	// If task is already terminal, just send the final state
	if IsTerminal(task.Status.State) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		sendSSE(w, flusher, StreamResponse{Task: task})
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Send current task state
	sendSSE(w, flusher, StreamResponse{Task: task})

	// Subscribe to updates
	ch := s.subscribe(taskID)
	defer s.unsubscribe(taskID, ch)

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			sendSSEComment(w, flusher, "heartbeat")
		case event, ok := <-ch:
			if !ok {
				return // channel closed
			}
			sendSSE(w, flusher, event)

			// If this event contains a terminal task state, close the stream
			if event.StatusUpdate != nil && IsTerminal(event.StatusUpdate.Status.State) {
				// Send final task
				finalTask, _ := s.store.GetTask(taskID, nil)
				if finalTask != nil {
					sendSSE(w, flusher, StreamResponse{Task: finalTask})
				}
				return
			}
			if event.Task != nil && IsTerminal(event.Task.Status.State) {
				return
			}
		}
	}
}

// --- SSE helpers ---

func sendSSE(w http.ResponseWriter, flusher http.Flusher, event StreamResponse) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func sendSSEComment(w http.ResponseWriter, flusher http.Flusher, comment string) {
	fmt.Fprintf(w, ": %s\n\n", comment)
	flusher.Flush()
}
