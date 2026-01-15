package slogutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"ckb/internal/config"
)

// LokiHandler implements slog.Handler and sends logs to Grafana Loki.
// It buffers logs and sends them in batches for efficiency.
type LokiHandler struct {
	endpoint      string
	labels        map[string]string
	batchSize     int
	flushInterval time.Duration
	level         slog.Level

	buffer []lokiEntry
	mu     sync.Mutex
	done   chan struct{}
	wg     sync.WaitGroup
	client *http.Client

	// For WithAttrs/WithGroup support
	attrs  []slog.Attr
	groups []string
}

type lokiEntry struct {
	timestamp time.Time
	line      string
	level     string
}

// lokiPushRequest represents the Loki push API request format
type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// NewLokiHandler creates a handler that pushes logs to Loki.
// baseLabels are static labels applied to all log entries.
func NewLokiHandler(cfg *config.RemoteLogConfig, baseLabels map[string]string, level slog.Level) (*LokiHandler, error) {
	if cfg == nil || cfg.Endpoint == "" {
		return nil, fmt.Errorf("loki endpoint is required")
	}

	// Merge config labels with base labels
	labels := make(map[string]string)
	for k, v := range baseLabels {
		labels[k] = v
	}
	for k, v := range cfg.Labels {
		labels[k] = v
	}

	// Add hostname if not set
	if _, ok := labels["host"]; !ok {
		if hostname, err := os.Hostname(); err == nil {
			labels["host"] = hostname
		}
	}

	// Set defaults
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	flushInterval := 5 * time.Second
	if cfg.FlushInterval != "" {
		if d, err := time.ParseDuration(cfg.FlushInterval); err == nil {
			flushInterval = d
		}
	}

	return &LokiHandler{
		endpoint:      cfg.Endpoint + "/loki/api/v1/push",
		labels:        labels,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		level:         level,
		buffer:        make([]lokiEntry, 0, batchSize),
		done:          make(chan struct{}),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// Start begins the background flush goroutine.
func (h *LokiHandler) Start() {
	h.wg.Add(1)
	go h.flushLoop()
}

// Stop flushes remaining logs and stops the handler.
func (h *LokiHandler) Stop() error {
	close(h.done)
	h.wg.Wait()

	// Final flush
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.buffer) > 0 {
		return h.flushLocked()
	}
	return nil
}

// Enabled implements slog.Handler.
func (h *LokiHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle implements slog.Handler - buffers log and flushes when batch is full.
func (h *LokiHandler) Handle(_ context.Context, r slog.Record) error {
	// Format the log line
	line := h.formatRecord(r)

	h.mu.Lock()
	defer h.mu.Unlock()

	h.buffer = append(h.buffer, lokiEntry{
		timestamp: r.Time,
		line:      line,
		level:     r.Level.String(),
	})

	// Flush if batch is full
	if len(h.buffer) >= h.batchSize {
		return h.flushLocked()
	}

	return nil
}

// WithAttrs implements slog.Handler.
func (h *LokiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Don't copy the handler - it contains mutexes. Create a wrapper instead.
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &lokiHandlerWithContext{
		parent: h,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

// WithGroup implements slog.Handler.
func (h *LokiHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	// Don't copy the handler - it contains mutexes. Create a wrapper instead.
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &lokiHandlerWithContext{
		parent: h,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

// lokiHandlerWithContext wraps LokiHandler to add attrs/groups without copying mutexes.
type lokiHandlerWithContext struct {
	parent *LokiHandler
	attrs  []slog.Attr
	groups []string
}

func (w *lokiHandlerWithContext) Enabled(ctx context.Context, level slog.Level) bool {
	return w.parent.Enabled(ctx, level)
}

func (w *lokiHandlerWithContext) Handle(ctx context.Context, r slog.Record) error {
	// Add our attrs to the record
	for _, attr := range w.attrs {
		r.AddAttrs(attr)
	}
	return w.parent.Handle(ctx, r)
}

func (w *lokiHandlerWithContext) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(w.attrs)+len(attrs))
	copy(newAttrs, w.attrs)
	copy(newAttrs[len(w.attrs):], attrs)
	return &lokiHandlerWithContext{
		parent: w.parent,
		attrs:  newAttrs,
		groups: w.groups,
	}
}

func (w *lokiHandlerWithContext) WithGroup(name string) slog.Handler {
	if name == "" {
		return w
	}
	newGroups := make([]string, len(w.groups)+1)
	copy(newGroups, w.groups)
	newGroups[len(w.groups)] = name
	return &lokiHandlerWithContext{
		parent: w.parent,
		attrs:  w.attrs,
		groups: newGroups,
	}
}

// formatRecord formats a slog.Record into a log line string.
func (h *LokiHandler) formatRecord(r slog.Record) string {
	var buf bytes.Buffer

	// Write level
	buf.WriteString("level=")
	buf.WriteString(r.Level.String())

	// Write message
	buf.WriteString(" msg=")
	buf.WriteString(strconv.Quote(r.Message))

	// Write pre-configured attrs
	for _, attr := range h.attrs {
		buf.WriteByte(' ')
		h.writeAttr(&buf, attr)
	}

	// Write record attrs
	r.Attrs(func(attr slog.Attr) bool {
		buf.WriteByte(' ')
		h.writeAttr(&buf, attr)
		return true
	})

	return buf.String()
}

func (h *LokiHandler) writeAttr(buf *bytes.Buffer, attr slog.Attr) {
	buf.WriteString(attr.Key)
	buf.WriteByte('=')

	switch attr.Value.Kind() {
	case slog.KindString:
		buf.WriteString(strconv.Quote(attr.Value.String()))
	case slog.KindInt64:
		buf.WriteString(strconv.FormatInt(attr.Value.Int64(), 10))
	case slog.KindUint64:
		buf.WriteString(strconv.FormatUint(attr.Value.Uint64(), 10))
	case slog.KindFloat64:
		buf.WriteString(strconv.FormatFloat(attr.Value.Float64(), 'f', -1, 64))
	case slog.KindBool:
		buf.WriteString(strconv.FormatBool(attr.Value.Bool()))
	case slog.KindDuration:
		buf.WriteString(attr.Value.Duration().String())
	case slog.KindTime:
		buf.WriteString(attr.Value.Time().Format(time.RFC3339))
	default:
		buf.WriteString(fmt.Sprintf("%v", attr.Value.Any()))
	}
}

// flushLoop runs in the background and periodically flushes the buffer.
func (h *LokiHandler) flushLoop() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.mu.Lock()
			if len(h.buffer) > 0 {
				_ = h.flushLocked() // Best effort, ignore errors
			}
			h.mu.Unlock()
		case <-h.done:
			return
		}
	}
}

// flushLocked sends buffered logs to Loki. Must be called with h.mu held.
func (h *LokiHandler) flushLocked() error {
	if len(h.buffer) == 0 {
		return nil
	}

	// Group entries by level for better Loki querying
	streams := make(map[string][]lokiEntry)
	for _, entry := range h.buffer {
		streams[entry.level] = append(streams[entry.level], entry)
	}

	// Build request
	req := lokiPushRequest{
		Streams: make([]lokiStream, 0, len(streams)),
	}

	for level, entries := range streams {
		// Copy base labels and add level
		labels := make(map[string]string, len(h.labels)+1)
		for k, v := range h.labels {
			labels[k] = v
		}
		labels["level"] = level

		values := make([][]string, len(entries))
		for i, entry := range entries {
			// Loki expects nanosecond timestamps as strings
			ts := strconv.FormatInt(entry.timestamp.UnixNano(), 10)
			values[i] = []string{ts, entry.line}
		}

		req.Streams = append(req.Streams, lokiStream{
			Stream: labels,
			Values: values,
		})
	}

	// Clear buffer before sending (so we don't lose new logs during send)
	h.buffer = h.buffer[:0]

	// Send to Loki (do this without lock to avoid blocking Handle calls)
	go h.send(req)

	return nil
}

// send sends the request to Loki. Runs in a goroutine.
func (h *LokiHandler) send(req lokiPushRequest) {
	body, err := json.Marshal(req)
	if err != nil {
		return // Best effort, ignore marshal errors
	}

	httpReq, err := http.NewRequest("POST", h.endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return // Best effort, ignore network errors
	}
	defer resp.Body.Close()

	// Log any non-2xx responses for debugging (could add metrics later)
	// For now, we just ignore them to avoid infinite logging loops
}
