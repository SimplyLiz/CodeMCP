package mcp

import (
	"fmt"

	"ckb/internal/daemon"
	"ckb/internal/envelope"
	"ckb/internal/logging"
	"ckb/internal/paths"
	"ckb/internal/scheduler"
	"ckb/internal/webhooks"
)

// toolDaemonStatus returns daemon status
func (s *MCPServer) toolDaemonStatus(params map[string]interface{}) (*envelope.Response, error) {
	running, pid, err := daemon.IsRunning()
	if err != nil {
		return nil, fmt.Errorf("failed to check daemon status: %w", err)
	}

	if !running {
		return OperationalResponse(map[string]interface{}{
			"running": false,
			"message": "Daemon is not running. Start with: ckb daemon start",
		}), nil
	}

	// Get daemon info from paths
	info, err := paths.GetDaemonInfo()
	if err != nil {
		//nolint:nilerr // partial status is acceptable
		return OperationalResponse(map[string]interface{}{
			"running": true,
			"pid":     pid,
			"message": "Daemon is running but status details unavailable",
		}), nil
	}

	return OperationalResponse(map[string]interface{}{
		"running":   true,
		"pid":       pid,
		"logFile":   info.LogPath,
		"dbFile":    info.DBPath,
		"pidFile":   info.PIDPath,
		"daemonDir": info.Dir,
		"hint":      "Use 'ckb daemon status' for full status or 'ckb daemon logs' to view logs",
	}), nil
}

// toolListSchedules lists scheduled tasks
func (s *MCPServer) toolListSchedules(params map[string]interface{}) (*envelope.Response, error) {
	// Get daemon directory
	daemonDir, err := paths.GetDaemonDir()
	if err != nil {
		return nil, fmt.Errorf("daemon not configured: %w", err)
	}

	// Create logger
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})

	// Open scheduler store
	sched, err := scheduler.New(daemonDir, logger, scheduler.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to access scheduler: %w", err)
	}

	// Build filter options
	opts := scheduler.ListSchedulesOptions{
		Limit: 20,
	}

	if taskType, ok := params["taskType"].(string); ok {
		opts.TaskType = []scheduler.TaskType{scheduler.TaskType(taskType)}
	}

	if enabled, ok := params["enabled"].(bool); ok {
		opts.Enabled = &enabled
	}

	if limit, ok := params["limit"].(float64); ok {
		opts.Limit = int(limit)
	}

	result, err := sched.ListSchedules(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list schedules: %w", err)
	}

	return OperationalResponse(map[string]interface{}{
		"schedules":  result.Schedules,
		"totalCount": result.TotalCount,
	}), nil
}

// toolRunSchedule runs a scheduled task immediately
func (s *MCPServer) toolRunSchedule(params map[string]interface{}) (*envelope.Response, error) {
	scheduleID, ok := params["scheduleId"].(string)
	if !ok || scheduleID == "" {
		return nil, fmt.Errorf("scheduleId is required")
	}

	// Get daemon directory
	daemonDir, err := paths.GetDaemonDir()
	if err != nil {
		return nil, fmt.Errorf("daemon not configured: %w", err)
	}

	// Create logger
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})

	// Open scheduler
	sched, err := scheduler.New(daemonDir, logger, scheduler.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to access scheduler: %w", err)
	}

	// Run the schedule
	if err := sched.RunNow(scheduleID); err != nil {
		return nil, fmt.Errorf("failed to run schedule: %w", err)
	}

	return OperationalResponse(map[string]interface{}{
		"success":    true,
		"scheduleId": scheduleID,
		"message":    "Schedule triggered successfully",
	}), nil
}

// toolListWebhooks lists configured webhooks
func (s *MCPServer) toolListWebhooks(params map[string]interface{}) (*envelope.Response, error) {
	// Get daemon directory
	daemonDir, err := paths.GetDaemonDir()
	if err != nil {
		return nil, fmt.Errorf("daemon not configured: %w", err)
	}

	// Create logger
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})

	// Open webhook manager
	mgr, err := webhooks.NewManager(daemonDir, logger, webhooks.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to access webhooks: %w", err)
	}

	// List webhooks
	hooks, err := mgr.ListWebhooks()
	if err != nil {
		return nil, fmt.Errorf("failed to list webhooks: %w", err)
	}

	// Convert to summaries
	summaries := make([]webhooks.WebhookSummary, 0, len(hooks))
	for _, h := range hooks {
		summaries = append(summaries, h.ToSummary())
	}

	return OperationalResponse(map[string]interface{}{
		"webhooks":   summaries,
		"totalCount": len(summaries),
	}), nil
}

// toolTestWebhook sends a test event to a webhook
func (s *MCPServer) toolTestWebhook(params map[string]interface{}) (*envelope.Response, error) {
	webhookID, ok := params["webhookId"].(string)
	if !ok || webhookID == "" {
		return nil, fmt.Errorf("webhookId is required")
	}

	// Get daemon directory
	daemonDir, err := paths.GetDaemonDir()
	if err != nil {
		return nil, fmt.Errorf("daemon not configured: %w", err)
	}

	// Create logger
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})

	// Open webhook manager
	mgr, err := webhooks.NewManager(daemonDir, logger, webhooks.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to access webhooks: %w", err)
	}

	// Test the webhook
	if err := mgr.TestWebhook(webhookID); err != nil {
		return nil, fmt.Errorf("failed to test webhook: %w", err)
	}

	return OperationalResponse(map[string]interface{}{
		"success":   true,
		"webhookId": webhookID,
		"message":   "Test webhook queued for delivery",
	}), nil
}

// toolWebhookDeliveries gets delivery history for a webhook
func (s *MCPServer) toolWebhookDeliveries(params map[string]interface{}) (*envelope.Response, error) {
	webhookID, ok := params["webhookId"].(string)
	if !ok || webhookID == "" {
		return nil, fmt.Errorf("webhookId is required")
	}

	// Get daemon directory
	daemonDir, err := paths.GetDaemonDir()
	if err != nil {
		return nil, fmt.Errorf("daemon not configured: %w", err)
	}

	// Create logger
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})

	// Open webhook manager
	mgr, err := webhooks.NewManager(daemonDir, logger, webhooks.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to access webhooks: %w", err)
	}

	// Build filter options
	opts := webhooks.ListDeliveriesOptions{
		WebhookID: webhookID,
		Limit:     20,
	}

	if status, ok := params["status"].(string); ok {
		opts.Status = []webhooks.DeliveryStatus{webhooks.DeliveryStatus(status)}
	}

	if limit, ok := params["limit"].(float64); ok {
		opts.Limit = int(limit)
	}

	result, err := mgr.ListDeliveries(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list deliveries: %w", err)
	}

	return OperationalResponse(map[string]interface{}{
		"deliveries": result.Deliveries,
		"totalCount": result.TotalCount,
	}), nil
}
