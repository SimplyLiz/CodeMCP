package a2a

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	pushMaxRetries     = 3
	pushInitialBackoff = 1 * time.Second
	pushTimeout        = 10 * time.Second
)

// PushManager handles webhook delivery for task push notifications.
type PushManager struct {
	client *http.Client
	store  *TaskStore
	logger *slog.Logger
}

// NewPushManager creates a new push notification manager.
func NewPushManager(store *TaskStore, logger *slog.Logger) *PushManager {
	return &PushManager{
		client: &http.Client{Timeout: pushTimeout},
		store:  store,
		logger: logger,
	}
}

// Notify delivers a stream event to all push notification configs for a task.
func (p *PushManager) Notify(taskID string, event StreamResponse) {
	configs, err := p.store.ListPushConfigs(taskID)
	if err != nil {
		p.logger.Warn("Failed to list push configs", "taskId", taskID, "error", err.Error())
		return
	}

	for _, cfg := range configs {
		go p.deliver(cfg, event)
	}
}

// deliver sends a single webhook with retries.
func (p *PushManager) deliver(cfg PushNotificationConfig, event StreamResponse) {
	body, err := json.Marshal(event)
	if err != nil {
		p.logger.Error("Failed to marshal push notification", "configId", cfg.ID, "error", err.Error())
		return
	}

	backoff := pushInitialBackoff
	var req *http.Request
	var resp *http.Response
	for attempt := 0; attempt <= pushMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		req, err = http.NewRequest("POST", cfg.URL, bytes.NewReader(body))
		if err != nil {
			p.logger.Error("Failed to create push request", "configId", cfg.ID, "error", err.Error())
			return
		}
		req.Header.Set("Content-Type", "application/json")

		// Add authentication
		if cfg.Authentication != nil && cfg.Authentication.Token != "" {
			scheme := "Bearer"
			if len(cfg.Authentication.Schemes) > 0 {
				scheme = cfg.Authentication.Schemes[0]
			}
			req.Header.Set("Authorization", fmt.Sprintf("%s %s", scheme, cfg.Authentication.Token))
		}

		resp, err = p.client.Do(req)
		if err != nil {
			p.logger.Warn("Push notification delivery failed",
				"configId", cfg.ID,
				"url", cfg.URL,
				"attempt", attempt+1,
				"error", err.Error(),
			)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			p.logger.Debug("Push notification delivered",
				"configId", cfg.ID,
				"url", cfg.URL,
				"status", resp.StatusCode,
			)
			return
		}

		p.logger.Warn("Push notification got non-2xx response",
			"configId", cfg.ID,
			"url", cfg.URL,
			"status", resp.StatusCode,
			"attempt", attempt+1,
		)
	}

	p.logger.Error("Push notification delivery exhausted retries",
		"configId", cfg.ID,
		"url", cfg.URL,
		"maxRetries", pushMaxRetries,
	)
}
