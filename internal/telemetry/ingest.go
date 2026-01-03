package telemetry

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"ckb/internal/config"
)

// IngestHandler handles telemetry ingestion requests
type IngestHandler struct {
	storage *Storage
	mapper  *ServiceMapper
	matcher *Matcher
	config  config.TelemetryConfig
	logger  *slog.Logger
}

// NewIngestHandler creates a new ingestion handler
func NewIngestHandler(storage *Storage, mapper *ServiceMapper, matcher *Matcher, cfg config.TelemetryConfig, logger *slog.Logger) *IngestHandler {
	return &IngestHandler{
		storage: storage,
		mapper:  mapper,
		matcher: matcher,
		config:  cfg,
		logger:  logger,
	}
}

// HandleJSONIngest handles the JSON ingest endpoint (POST /api/v1/ingest/json)
func (h *IngestHandler) HandleJSONIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read body
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse payload
	var payload IngestPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Process the payload
	response := h.ProcessPayload(&payload)

	// Return response
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// ProcessPayload processes an ingest payload and stores the results
func (h *IngestHandler) ProcessPayload(payload *IngestPayload) IngestResponse {
	startTime := time.Now()
	response := IngestResponse{}

	// Create sync log
	syncLog := &SyncLog{
		Source:          payload.Source,
		StartedAt:       startTime,
		Status:          "in_progress",
		ServiceVersions: make(map[string]string),
	}
	logID, _ := h.storage.SaveSyncLog(syncLog)
	syncLog.ID = logID

	// Track matches
	var events []CallAggregate
	var matches []SymbolMatch

	for _, call := range payload.Calls {
		response.Accepted++

		// Record service version if provided
		if payload.ServiceVersion != "" {
			syncLog.ServiceVersions[call.ServiceName] = payload.ServiceVersion
		}

		// Resolve service to repo
		mapResult := h.mapper.Resolve(call.ServiceName)
		if !mapResult.Matched {
			// Save as unmatched
			h.saveUnmatchedEvent(&call, "no_repo_mapping", payload.Source)
			response.Unmatched++
			syncLog.EventsUnmatched++
			continue
		}

		// Match to symbol (if matcher is available)
		var match SymbolMatch
		if h.matcher != nil {
			match = h.matcher.Match(&call)
		} else {
			// No matcher - just record as strong match (assume caller provides good data)
			match = SymbolMatch{
				SymbolID:   fmt.Sprintf("%s:%s:%s", mapResult.RepoID, call.FilePath, call.FunctionName),
				Quality:    MatchStrong,
				Confidence: MatchStrong.Confidence(),
				MatchBasis: []string{"assumed_from_payload"},
			}
		}

		events = append(events, call)
		matches = append(matches, match)

		if match.Quality == MatchUnmatched {
			h.saveUnmatchedEvent(&call, "not_found", payload.Source)
			response.Unmatched++
			syncLog.EventsUnmatched++
			continue
		}

		// Save observed usage
		period := computePeriod(call.PeriodStart, h.config.Aggregation.BucketSize)
		usage := &ObservedUsage{
			SymbolID:        match.SymbolID,
			MatchQuality:    match.Quality,
			MatchConfidence: match.Confidence,
			Period:          period,
			PeriodType:      h.config.Aggregation.BucketSize,
			CallCount:       call.CallCount,
			ErrorCount:      call.ErrorCount,
			ServiceVersion:  payload.ServiceVersion,
			Source:          payload.Source,
			IngestedAt:      time.Now(),
		}

		if err := h.storage.SaveObservedUsage(usage); err != nil {
			h.logger.Error("Failed to save observed usage",
				"error", err.Error(),
				"symbolId", match.SymbolID,
			)
			continue
		}

		response.Matched++

		// Track match quality
		switch match.Quality {
		case MatchExact:
			syncLog.EventsMatchedExact++
		case MatchStrong:
			syncLog.EventsMatchedStrong++
		case MatchWeak:
			syncLog.EventsMatchedWeak++
		}

		// Save callers if enabled
		if h.config.Aggregation.StoreCallers && len(call.Callers) > 0 {
			h.saveCallers(match.SymbolID, call.Callers, period)
		}
	}

	// Compute coverage
	coverage := ComputeCoverage(events, matches, 0) // 0 = no federation context
	response.CoverageScore = coverage.Overall.Score
	response.CoverageLevel = string(coverage.Overall.Level)

	// Update sync log
	completedAt := time.Now()
	syncLog.CompletedAt = &completedAt
	syncLog.Status = "success"
	syncLog.EventsReceived = response.Accepted
	syncLog.CoverageScore = coverage.Overall.Score
	syncLog.CoverageLevel = string(coverage.Overall.Level)
	_ = h.storage.UpdateSyncLog(syncLog)

	return response
}

// saveUnmatchedEvent saves an event that couldn't be matched
func (h *IngestHandler) saveUnmatchedEvent(call *CallAggregate, reason string, source string) {
	if !h.config.Privacy.LogUnmatchedEvents {
		return
	}

	period := computePeriod(call.PeriodStart, h.config.Aggregation.BucketSize)
	event := &UnmatchedEvent{
		ServiceName:   call.ServiceName,
		FunctionName:  call.FunctionName,
		Namespace:     call.Namespace,
		FilePath:      call.FilePath,
		Period:        period,
		PeriodType:    h.config.Aggregation.BucketSize,
		CallCount:     call.CallCount,
		ErrorCount:    call.ErrorCount,
		UnmatchReason: reason,
		Source:        source,
		IngestedAt:    time.Now(),
	}

	if err := h.storage.SaveUnmatchedEvent(event); err != nil {
		h.logger.Error("Failed to save unmatched event",
			"error", err.Error(),
			"serviceName", call.ServiceName,
			"functionName", call.FunctionName,
		)
	}
}

// saveCallers saves caller breakdown data
func (h *IngestHandler) saveCallers(symbolID string, callers []Caller, period string) {
	maxCallers := h.config.Aggregation.MaxCallersPerSymbol
	if maxCallers == 0 {
		maxCallers = 20
	}

	for i, caller := range callers {
		if i >= maxCallers {
			break
		}

		callerName := caller.ServiceName
		if h.config.Privacy.RedactCallerNames {
			// Hash the caller name for privacy
			callerName = hashServiceName(caller.ServiceName)
		}

		oc := &ObservedCaller{
			SymbolID:      symbolID,
			CallerService: callerName,
			Period:        period,
			CallCount:     caller.CallCount,
		}

		if err := h.storage.SaveObservedCaller(oc); err != nil {
			h.logger.Error("Failed to save caller",
				"error", err.Error(),
				"symbolId", symbolID,
			)
		}
	}
}

// computePeriod converts a timestamp to a period string
func computePeriod(t time.Time, bucketSize string) string {
	if t.IsZero() {
		t = time.Now()
	}

	switch bucketSize {
	case "daily":
		return t.Format("2006-01-02")
	case "weekly":
		year, week := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	case "monthly":
		return t.Format("2006-01")
	default:
		return t.Format("2006-01") // Default to monthly
	}
}

// hashServiceName creates a privacy-preserving hash of a service name
func hashServiceName(name string) string {
	// Simple hash for privacy - keeps first 3 chars + hash suffix
	if len(name) <= 3 {
		return name + "_hashed"
	}
	// Use a deterministic but privacy-preserving identifier
	hash := 0
	for _, c := range name {
		hash = hash*31 + int(c)
	}
	return fmt.Sprintf("%s_%x", name[:3], hash&0xFFFF)
}

// OTLPMetric represents a metric from OTLP format
type OTLPMetric struct {
	Name       string
	Attributes map[string]string
	Value      int64
}

// ParseOTLPMetrics parses OTLP metrics into our internal format
func (h *IngestHandler) ParseOTLPMetrics(resourceAttrs map[string]string, metrics []OTLPMetric) []CallAggregate {
	serviceName := resourceAttrs["service.name"]
	serviceVersion := resourceAttrs["service.version"]

	var calls []CallAggregate

	for _, metric := range metrics {
		// Only process call count metrics
		if !isCallCountMetric(metric.Name) {
			continue
		}

		call := CallAggregate{
			ServiceName:    serviceName,
			ServiceVersion: serviceVersion,
			CallCount:      metric.Value,
			PeriodStart:    time.Now(),
			PeriodEnd:      time.Now(),
		}

		// Extract function name from attributes
		for _, key := range h.config.Attributes.FunctionKeys {
			if v, ok := metric.Attributes[key]; ok && v != "" {
				call.FunctionName = v
				break
			}
		}

		// Extract namespace from attributes
		for _, key := range h.config.Attributes.NamespaceKeys {
			if v, ok := metric.Attributes[key]; ok && v != "" {
				call.Namespace = v
				break
			}
		}

		// Extract file path from attributes (resource level, not metric level for cardinality)
		for _, key := range h.config.Attributes.FileKeys {
			if v, ok := resourceAttrs[key]; ok && v != "" {
				call.FilePath = v
				break
			}
		}

		// Extract line number from attributes
		for _, key := range h.config.Attributes.LineKeys {
			if v, ok := metric.Attributes[key]; ok && v != "" {
				fmt.Sscanf(v, "%d", &call.LineNumber)
				break
			}
		}

		// Only add if we have at least a function name
		if call.FunctionName != "" {
			calls = append(calls, call)
		}
	}

	return calls
}

// isCallCountMetric checks if a metric name represents call counts
func isCallCountMetric(name string) bool {
	callCountNames := []string{
		"calls",
		"span.calls",
		"traces.span.count",
		"http.server.request.count",
		"rpc.server.duration_count",
		"grpc.server.duration_count",
	}

	for _, n := range callCountNames {
		if name == n {
			return true
		}
	}
	return false
}
