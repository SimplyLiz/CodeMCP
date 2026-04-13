package a2a

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func setupTestStore(t *testing.T) *TaskStore {
	t.Helper()
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	store, err := OpenTaskStore(dir, logger)
	if err != nil {
		t.Fatalf("OpenTaskStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestCreateAndGetTask(t *testing.T) {
	store := setupTestStore(t)

	task, err := store.CreateTask("ctx-1", map[string]any{"key": "value"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.ID == "" {
		t.Fatal("task ID should not be empty")
	}
	if task.ContextID != "ctx-1" {
		t.Errorf("contextId = %s, want ctx-1", task.ContextID)
	}
	if task.Status.State != TaskStateSubmitted {
		t.Errorf("state = %s, want %s", task.Status.State, TaskStateSubmitted)
	}

	// Retrieve
	got, err := store.GetTask(task.ID, nil)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got == nil {
		t.Fatal("GetTask returned nil")
	}
	if got.ID != task.ID {
		t.Errorf("ID = %s, want %s", got.ID, task.ID)
	}
	if got.Metadata["key"] != "value" {
		t.Errorf("metadata key = %v, want value", got.Metadata["key"])
	}
}

func TestCreateTaskAutoContextID(t *testing.T) {
	store := setupTestStore(t)

	task, err := store.CreateTask("", nil)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.ContextID == "" {
		t.Error("auto-generated contextId should not be empty")
	}
}

func TestUpdateTaskState(t *testing.T) {
	store := setupTestStore(t)

	task, _ := store.CreateTask("", nil)

	// submitted -> working
	err := store.UpdateTaskState(task.ID, TaskStateWorking, nil)
	if err != nil {
		t.Fatalf("UpdateTaskState to working: %v", err)
	}

	got, _ := store.GetTask(task.ID, nil)
	if got.Status.State != TaskStateWorking {
		t.Errorf("state = %s, want %s", got.Status.State, TaskStateWorking)
	}

	// working -> completed
	msg := &Message{Role: RoleAgent, Parts: []Part{{Text: "done"}}}
	err = store.UpdateTaskState(task.ID, TaskStateCompleted, msg)
	if err != nil {
		t.Fatalf("UpdateTaskState to completed: %v", err)
	}

	got, _ = store.GetTask(task.ID, nil)
	if got.Status.State != TaskStateCompleted {
		t.Errorf("state = %s, want %s", got.Status.State, TaskStateCompleted)
	}
}

func TestUpdateTaskStateInvalidTransition(t *testing.T) {
	store := setupTestStore(t)

	task, _ := store.CreateTask("", nil)

	// submitted -> completed (invalid)
	err := store.UpdateTaskState(task.ID, TaskStateCompleted, nil)
	if err == nil {
		t.Error("expected error for invalid transition submitted -> completed")
	}
}

func TestUpdateTaskStateNotFound(t *testing.T) {
	store := setupTestStore(t)

	err := store.UpdateTaskState("nonexistent", TaskStateWorking, nil)
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestAddAndGetMessages(t *testing.T) {
	store := setupTestStore(t)

	task, _ := store.CreateTask("", nil)

	msg1 := Message{Role: RoleUser, Parts: []Part{{Text: "hello"}}}
	msg2 := Message{Role: RoleAgent, Parts: []Part{{Text: "world"}}}

	if err := store.AddMessage(task.ID, msg1); err != nil {
		t.Fatalf("AddMessage 1: %v", err)
	}
	if err := store.AddMessage(task.ID, msg2); err != nil {
		t.Fatalf("AddMessage 2: %v", err)
	}

	got, _ := store.GetTask(task.ID, nil)
	if len(got.History) != 2 {
		t.Fatalf("history length = %d, want 2", len(got.History))
	}
	if got.History[0].Role != RoleUser {
		t.Errorf("msg[0] role = %s, want %s", got.History[0].Role, RoleUser)
	}
	if got.History[1].Role != RoleAgent {
		t.Errorf("msg[1] role = %s, want %s", got.History[1].Role, RoleAgent)
	}

	// Test history limit
	limit := 1
	got, _ = store.GetTask(task.ID, &limit)
	if len(got.History) != 1 {
		t.Errorf("history with limit=1 length = %d, want 1", len(got.History))
	}
	// Should get the most recent message
	if got.History[0].Role != RoleAgent {
		t.Errorf("limited msg[0] role = %s, want %s (most recent)", got.History[0].Role, RoleAgent)
	}
}

func TestAddAndGetArtifacts(t *testing.T) {
	store := setupTestStore(t)

	task, _ := store.CreateTask("", nil)

	art := Artifact{
		ArtifactID: "art-1",
		Name:       "test-result",
		Parts:      []Part{{Text: `{"data": true}`, MediaType: "application/json"}},
	}
	if err := store.AddArtifact(task.ID, art); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}

	got, _ := store.GetTask(task.ID, nil)
	if len(got.Artifacts) != 1 {
		t.Fatalf("artifacts length = %d, want 1", len(got.Artifacts))
	}
	if got.Artifacts[0].Name != "test-result" {
		t.Errorf("artifact name = %s, want test-result", got.Artifacts[0].Name)
	}
}

func TestListTasks(t *testing.T) {
	store := setupTestStore(t)

	// Create 3 tasks
	t1, _ := store.CreateTask("ctx-a", nil)
	t2, _ := store.CreateTask("ctx-a", nil)
	_, _ = store.CreateTask("ctx-b", nil)

	// List all
	resp, err := store.ListTasks(ListTasksRequest{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if resp.TotalSize != 3 {
		t.Errorf("total = %d, want 3", resp.TotalSize)
	}

	// Filter by context
	resp, _ = store.ListTasks(ListTasksRequest{ContextID: "ctx-a"})
	if resp.TotalSize != 2 {
		t.Errorf("total for ctx-a = %d, want 2", resp.TotalSize)
	}

	// Filter by state
	_ = store.UpdateTaskState(t1.ID, TaskStateWorking, nil)
	state := TaskStateWorking
	resp, _ = store.ListTasks(ListTasksRequest{Status: &state})
	if resp.TotalSize != 1 {
		t.Errorf("total for working = %d, want 1", resp.TotalSize)
	}

	// Pagination
	resp, _ = store.ListTasks(ListTasksRequest{PageSize: 2})
	if len(resp.Tasks) != 2 {
		t.Errorf("page size 2 returned %d tasks", len(resp.Tasks))
	}
	if resp.NextPageToken == "" {
		t.Error("expected nextPageToken for page 1 of 3")
	}

	// Next page
	resp2, _ := store.ListTasks(ListTasksRequest{PageSize: 2, PageToken: resp.NextPageToken})
	if len(resp2.Tasks) != 1 {
		t.Errorf("page 2 returned %d tasks, want 1", len(resp2.Tasks))
	}
	_ = t2 // suppress unused
}

func TestPushNotificationConfig(t *testing.T) {
	store := setupTestStore(t)

	task, _ := store.CreateTask("", nil)

	cfg := PushNotificationConfig{
		URL: "https://example.com/webhook",
		Authentication: &AuthenticationInfo{
			Schemes: []string{"Bearer"},
			Token:   "test-token",
		},
	}

	created, err := store.CreatePushConfig(task.ID, cfg)
	if err != nil {
		t.Fatalf("CreatePushConfig: %v", err)
	}
	if created.ID == "" {
		t.Error("push config ID should not be empty")
	}
	if created.URL != "https://example.com/webhook" {
		t.Errorf("URL = %s", created.URL)
	}

	// Get
	got, err := store.GetPushConfig(created.ID)
	if err != nil {
		t.Fatalf("GetPushConfig: %v", err)
	}
	if got.Authentication.Token != "test-token" {
		t.Errorf("token = %s", got.Authentication.Token)
	}

	// List
	configs, err := store.ListPushConfigs(task.ID)
	if err != nil {
		t.Fatalf("ListPushConfigs: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("configs length = %d, want 1", len(configs))
	}

	// Delete
	err = store.DeletePushConfig(created.ID)
	if err != nil {
		t.Fatalf("DeletePushConfig: %v", err)
	}
	configs, _ = store.ListPushConfigs(task.ID)
	if len(configs) != 0 {
		t.Errorf("configs after delete = %d", len(configs))
	}
}

func TestCleanupOldTasks(t *testing.T) {
	store := setupTestStore(t)

	task, _ := store.CreateTask("", nil)
	_ = store.UpdateTaskState(task.ID, TaskStateWorking, nil)
	_ = store.UpdateTaskState(task.ID, TaskStateCompleted, nil)

	// Cleanup with zero retention (should delete everything completed)
	deleted, err := store.CleanupOldTasks(0 * time.Second)
	if err != nil {
		t.Fatalf("CleanupOldTasks: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	got, _ := store.GetTask(task.ID, nil)
	if got != nil {
		t.Error("task should have been cleaned up")
	}
}

func TestGetTaskNotFound(t *testing.T) {
	store := setupTestStore(t)

	got, err := store.GetTask("nonexistent", nil)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent task")
	}
}
