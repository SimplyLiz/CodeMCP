package storage

import (
	"database/sql"
	"time"
)

// WideResultRecord represents a single tool invocation record
type WideResultRecord struct {
	ID              int64
	ToolName        string
	TotalResults    int
	ReturnedResults int
	TruncatedCount  int
	EstimatedTokens int
	ExecutionMs     int64
	RecordedAt      time.Time
}

// WideResultAggregate represents aggregated stats for a tool
type WideResultAggregate struct {
	ToolName       string  `json:"toolName"`
	QueryCount     int64   `json:"queryCount"`
	TotalResults   int64   `json:"totalResults"`
	TotalReturned  int64   `json:"totalReturned"`
	TotalTruncated int64   `json:"totalTruncated"`
	TotalTokens    int64   `json:"totalTokens"`
	TotalBytes     int64   `json:"totalBytes"`
	TotalMs        int64   `json:"totalMs"`
	AvgTruncation  float64 `json:"avgTruncationRate"`
}

// AvgTruncationRate returns the average truncation rate
func (a *WideResultAggregate) AvgTruncationRate() float64 {
	if a.TotalResults == 0 {
		return 0
	}
	return float64(a.TotalTruncated) / float64(a.TotalResults)
}

// AvgTokens returns the average tokens per query
func (a *WideResultAggregate) AvgTokens() float64 {
	if a.QueryCount == 0 {
		return 0
	}
	return float64(a.TotalTokens) / float64(a.QueryCount)
}

// AvgLatencyMs returns the average latency in milliseconds
func (a *WideResultAggregate) AvgLatencyMs() float64 {
	if a.QueryCount == 0 {
		return 0
	}
	return float64(a.TotalMs) / float64(a.QueryCount)
}

// RecordWideResult persists a wide-result tool invocation to SQLite
func (db *DB) RecordWideResult(toolName string, totalResults, returnedResults, truncatedCount, estimatedTokens, responseBytes int, executionMs int64) error {
	_, err := db.Exec(`
		INSERT INTO wide_result_metrics (
			tool_name, total_results, returned_results, truncated_count,
			estimated_tokens, response_bytes, execution_ms, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, toolName, totalResults, returnedResults, truncatedCount, estimatedTokens, responseBytes, executionMs, time.Now().UTC().Format(time.RFC3339))
	return err
}

// GetWideResultAggregates returns aggregated metrics for all tools within the time window
func (db *DB) GetWideResultAggregates(since time.Time) (map[string]*WideResultAggregate, error) {
	rows, err := db.Query(`
		SELECT
			tool_name,
			COUNT(*) as query_count,
			SUM(total_results) as total_results,
			SUM(returned_results) as total_returned,
			SUM(truncated_count) as total_truncated,
			SUM(estimated_tokens) as total_tokens,
			COALESCE(SUM(response_bytes), 0) as total_bytes,
			SUM(execution_ms) as total_ms
		FROM wide_result_metrics
		WHERE recorded_at >= ?
		GROUP BY tool_name
		ORDER BY query_count DESC
	`, since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*WideResultAggregate)
	for rows.Next() {
		var agg WideResultAggregate
		if err := rows.Scan(
			&agg.ToolName,
			&agg.QueryCount,
			&agg.TotalResults,
			&agg.TotalReturned,
			&agg.TotalTruncated,
			&agg.TotalTokens,
			&agg.TotalBytes,
			&agg.TotalMs,
		); err != nil {
			return nil, err
		}
		agg.AvgTruncation = agg.AvgTruncationRate()
		result[agg.ToolName] = &agg
	}

	return result, rows.Err()
}

// GetWideResultRecords returns recent records, optionally filtered by tool
func (db *DB) GetWideResultRecords(limit int, toolFilter string) ([]WideResultRecord, error) {
	var rows *sql.Rows
	var err error

	if toolFilter != "" {
		rows, err = db.Query(`
			SELECT id, tool_name, total_results, returned_results, truncated_count,
			       estimated_tokens, execution_ms, recorded_at
			FROM wide_result_metrics
			WHERE tool_name = ?
			ORDER BY recorded_at DESC
			LIMIT ?
		`, toolFilter, limit)
	} else {
		rows, err = db.Query(`
			SELECT id, tool_name, total_results, returned_results, truncated_count,
			       estimated_tokens, execution_ms, recorded_at
			FROM wide_result_metrics
			ORDER BY recorded_at DESC
			LIMIT ?
		`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []WideResultRecord
	for rows.Next() {
		var r WideResultRecord
		var recordedAt string
		if err := rows.Scan(
			&r.ID, &r.ToolName, &r.TotalResults, &r.ReturnedResults,
			&r.TruncatedCount, &r.EstimatedTokens, &r.ExecutionMs, &recordedAt,
		); err != nil {
			return nil, err
		}
		r.RecordedAt, _ = time.Parse(time.RFC3339, recordedAt)
		records = append(records, r)
	}

	return records, rows.Err()
}

// CleanupOldMetrics removes metrics older than the retention period
func (db *DB) CleanupOldMetrics(retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention).UTC().Format(time.RFC3339)
	result, err := db.Exec(`
		DELETE FROM wide_result_metrics WHERE recorded_at < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetWideResultStats returns summary statistics for the metrics table
func (db *DB) GetWideResultStats() (totalRecords int64, oldestRecord, newestRecord *time.Time, err error) {
	var oldestStr, newestStr sql.NullString
	err = db.QueryRow(`
		SELECT
			COUNT(*),
			MIN(recorded_at),
			MAX(recorded_at)
		FROM wide_result_metrics
	`).Scan(&totalRecords, &oldestStr, &newestStr)
	if err == sql.ErrNoRows {
		return 0, nil, nil, nil
	}
	if err != nil {
		return 0, nil, nil, err
	}

	if oldestStr.Valid {
		if t, parseErr := time.Parse(time.RFC3339, oldestStr.String); parseErr == nil {
			oldestRecord = &t
		}
	}
	if newestStr.Valid {
		if t, parseErr := time.Parse(time.RFC3339, newestStr.String); parseErr == nil {
			newestRecord = &t
		}
	}
	return
}
