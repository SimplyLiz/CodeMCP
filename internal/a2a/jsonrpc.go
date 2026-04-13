package a2a

import (
	"encoding/json"
	"net/http"
)

// handleJSONRPC processes JSON-RPC 2.0 requests for all A2A methods.
func (s *Server) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	var req JSONRPCRequest
	if err := decodeBody(r, &req); err != nil {
		writeJSONRPCError(w, nil, NewParseError(err.Error()))
		return
	}

	if req.JSONRPC != "2.0" {
		writeJSONRPCError(w, req.ID, NewInvalidRequestError("jsonrpc must be \"2.0\""))
		return
	}

	switch req.Method {
	case "message/send":
		s.jsonrpcMessageSend(w, req)
	case "message/sendStream":
		s.jsonrpcMessageSendStream(w, r, req)
	case "tasks/get":
		s.jsonrpcGetTask(w, req)
	case "tasks/list":
		s.jsonrpcListTasks(w, req)
	case "tasks/cancel":
		s.jsonrpcCancelTask(w, req)
	case "tasks/subscribe":
		s.jsonrpcSubscribeTask(w, r, req)
	case "tasks/pushNotificationConfig/set":
		s.jsonrpcCreatePushConfig(w, req)
	case "tasks/pushNotificationConfig/get":
		s.jsonrpcGetPushConfig(w, req)
	case "tasks/pushNotificationConfig/list":
		s.jsonrpcListPushConfigs(w, req)
	case "tasks/pushNotificationConfig/delete":
		s.jsonrpcDeletePushConfig(w, req)
	case "agent/extendedCard":
		s.jsonrpcExtendedCard(w, req)
	default:
		writeJSONRPCError(w, req.ID, NewMethodNotFoundError(req.Method))
	}
}

func (s *Server) jsonrpcMessageSend(w http.ResponseWriter, req JSONRPCRequest) {
	var params SendMessageRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, NewInvalidParamsError(err.Error()))
		return
	}

	task, a2aErr := s.doSendMessage(params)
	if a2aErr != nil {
		writeJSONRPCError(w, req.ID, a2aErr)
		return
	}
	writeJSONRPCResult(w, req.ID, task)
}

func (s *Server) jsonrpcMessageSendStream(w http.ResponseWriter, r *http.Request, req JSONRPCRequest) {
	var params SendMessageRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, NewInvalidParamsError(err.Error()))
		return
	}
	s.doStreamingSend(w, r, params)
}

func (s *Server) jsonrpcGetTask(w http.ResponseWriter, req JSONRPCRequest) {
	var params GetTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, NewInvalidParamsError(err.Error()))
		return
	}

	task, a2aErr := s.doGetTask(params)
	if a2aErr != nil {
		writeJSONRPCError(w, req.ID, a2aErr)
		return
	}
	writeJSONRPCResult(w, req.ID, task)
}

func (s *Server) jsonrpcListTasks(w http.ResponseWriter, req JSONRPCRequest) {
	var params ListTasksRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, NewInvalidParamsError(err.Error()))
		return
	}

	resp, a2aErr := s.doListTasks(params)
	if a2aErr != nil {
		writeJSONRPCError(w, req.ID, a2aErr)
		return
	}
	writeJSONRPCResult(w, req.ID, resp)
}

func (s *Server) jsonrpcCancelTask(w http.ResponseWriter, req JSONRPCRequest) {
	var params CancelTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, NewInvalidParamsError(err.Error()))
		return
	}

	task, a2aErr := s.doCancelTask(params)
	if a2aErr != nil {
		writeJSONRPCError(w, req.ID, a2aErr)
		return
	}
	writeJSONRPCResult(w, req.ID, task)
}

func (s *Server) jsonrpcCreatePushConfig(w http.ResponseWriter, req JSONRPCRequest) {
	var params CreatePushNotificationConfigRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, NewInvalidParamsError(err.Error()))
		return
	}

	cfg := PushNotificationConfig{
		URL:            params.URL,
		Authentication: params.Authentication,
	}
	created, a2aErr := s.doCreatePushConfig(params.TaskID, cfg)
	if a2aErr != nil {
		writeJSONRPCError(w, req.ID, a2aErr)
		return
	}
	writeJSONRPCResult(w, req.ID, created)
}

func (s *Server) jsonrpcGetPushConfig(w http.ResponseWriter, req JSONRPCRequest) {
	var params struct {
		TaskID   string `json:"taskId"`
		ConfigID string `json:"configId"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, NewInvalidParamsError(err.Error()))
		return
	}

	cfg, a2aErr := s.doGetPushConfig(params.ConfigID)
	if a2aErr != nil {
		writeJSONRPCError(w, req.ID, a2aErr)
		return
	}
	writeJSONRPCResult(w, req.ID, cfg)
}

func (s *Server) jsonrpcListPushConfigs(w http.ResponseWriter, req JSONRPCRequest) {
	var params struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, NewInvalidParamsError(err.Error()))
		return
	}

	configs, a2aErr := s.doListPushConfigs(params.TaskID)
	if a2aErr != nil {
		writeJSONRPCError(w, req.ID, a2aErr)
		return
	}
	writeJSONRPCResult(w, req.ID, configs)
}

func (s *Server) jsonrpcDeletePushConfig(w http.ResponseWriter, req JSONRPCRequest) {
	var params struct {
		TaskID   string `json:"taskId"`
		ConfigID string `json:"configId"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, NewInvalidParamsError(err.Error()))
		return
	}

	if a2aErr := s.doDeletePushConfig(params.ConfigID); a2aErr != nil {
		writeJSONRPCError(w, req.ID, a2aErr)
		return
	}
	writeJSONRPCResult(w, req.ID, nil)
}

func (s *Server) jsonrpcExtendedCard(w http.ResponseWriter, req JSONRPCRequest) {
	card := s.buildExtendedAgentCard()
	writeJSONRPCResult(w, req.ID, card)
}

func (s *Server) jsonrpcSubscribeTask(w http.ResponseWriter, r *http.Request, req JSONRPCRequest) {
	var params SubscribeToTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, NewInvalidParamsError(err.Error()))
		return
	}
	s.doStreamSubscribe(w, r, params.TaskID)
}

// --- JSON-RPC response helpers ---

func writeJSONRPCResult(w http.ResponseWriter, id any, result any) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func writeJSONRPCError(w http.ResponseWriter, id any, err *A2AError) {
	status := err.HTTPStatus
	if status == 0 {
		if s, ok := httpStatusForCode[err.Code]; ok {
			status = s
		} else {
			status = http.StatusInternalServerError
		}
	}
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   err.ToJSONRPC(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}
