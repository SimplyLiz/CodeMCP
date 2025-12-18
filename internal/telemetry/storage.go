package telemetry

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Storage handles telemetry data persistence
type Storage struct {
	db *sql.DB
}

// NewStorage creates a new telemetry storage instance
func NewStorage(db *sql.DB) *Storage {
	return &Storage{db: db}
}

// InitSchema creates the telemetry tables (v3 migration)
func (s *Storage) InitSchema() error {
	tables := []string{
		// Observed usage aggregates (matched symbols only)
		`CREATE TABLE IF NOT EXISTS observed_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			symbol_id TEXT NOT NULL,
			match_quality TEXT NOT NULL,
			match_confidence REAL NOT NULL,
			period TEXT NOT NULL,
			period_type TEXT NOT NULL,
			call_count INTEGER NOT NULL,
			error_count INTEGER DEFAULT 0,
			service_version TEXT,
			source TEXT NOT NULL,
			ingested_at TEXT NOT NULL,
			UNIQUE(symbol_id, period, source)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_observed_symbol ON observed_usage(symbol_id)`,
		`CREATE INDEX IF NOT EXISTS idx_observed_period ON observed_usage(period)`,
		`CREATE INDEX IF NOT EXISTS idx_observed_quality ON observed_usage(match_quality)`,
		`CREATE INDEX IF NOT EXISTS idx_observed_calls ON observed_usage(call_count DESC)`,

		// Unmatched events (separate table for clean deduplication)
		`CREATE TABLE IF NOT EXISTS observed_unmatched (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			service_name TEXT NOT NULL,
			function_name TEXT NOT NULL,
			namespace TEXT,
			file_path TEXT,
			period TEXT NOT NULL,
			period_type TEXT NOT NULL,
			call_count INTEGER NOT NULL,
			error_count INTEGER DEFAULT 0,
			unmatch_reason TEXT,
			source TEXT NOT NULL,
			ingested_at TEXT NOT NULL,
			UNIQUE(service_name, function_name, COALESCE(namespace, ''), COALESCE(file_path, ''), period, source)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_unmatched_service ON observed_unmatched(service_name)`,
		`CREATE INDEX IF NOT EXISTS idx_unmatched_function ON observed_unmatched(function_name)`,
		`CREATE INDEX IF NOT EXISTS idx_unmatched_period ON observed_unmatched(period)`,

		// Caller breakdown (optional, opt-in)
		`CREATE TABLE IF NOT EXISTS observed_callers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			symbol_id TEXT NOT NULL,
			caller_service TEXT NOT NULL,
			period TEXT NOT NULL,
			call_count INTEGER NOT NULL,
			UNIQUE(symbol_id, caller_service, period)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_callers_symbol ON observed_callers(symbol_id)`,

		// Telemetry sync log
		`CREATE TABLE IF NOT EXISTS telemetry_sync_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			started_at TEXT NOT NULL,
			completed_at TEXT,
			status TEXT NOT NULL,
			events_received INTEGER,
			events_matched_exact INTEGER,
			events_matched_strong INTEGER,
			events_matched_weak INTEGER,
			events_unmatched INTEGER,
			service_versions TEXT,
			coverage_score REAL,
			coverage_level TEXT,
			error TEXT
		)`,

		// Coverage snapshots (for trend tracking)
		`CREATE TABLE IF NOT EXISTS coverage_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_date TEXT NOT NULL,
			attribute_coverage REAL,
			match_coverage REAL,
			service_coverage REAL,
			overall_score REAL,
			overall_level TEXT,
			warnings TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_coverage_date ON coverage_snapshots(snapshot_date)`,
	}

	for _, stmt := range tables {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute schema statement: %w", err)
		}
	}

	return nil
}

// SaveObservedUsage saves or updates an observed usage record
func (s *Storage) SaveObservedUsage(usage *ObservedUsage) error {
	_, err := s.db.Exec(`
		INSERT INTO observed_usage (
			symbol_id, match_quality, match_confidence, period, period_type,
			call_count, error_count, service_version, source, ingested_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(symbol_id, period, source) DO UPDATE SET
			call_count = call_count + excluded.call_count,
			error_count = error_count + excluded.error_count,
			ingested_at = excluded.ingested_at
	`,
		usage.SymbolID, usage.MatchQuality, usage.MatchConfidence,
		usage.Period, usage.PeriodType, usage.CallCount, usage.ErrorCount,
		usage.ServiceVersion, usage.Source, usage.IngestedAt.Format(time.RFC3339),
	)
	return err
}

// SaveUnmatchedEvent saves an unmatched telemetry event
func (s *Storage) SaveUnmatchedEvent(event *UnmatchedEvent) error {
	_, err := s.db.Exec(`
		INSERT INTO observed_unmatched (
			service_name, function_name, namespace, file_path, period, period_type,
			call_count, error_count, unmatch_reason, source, ingested_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(service_name, function_name, COALESCE(namespace, ''), COALESCE(file_path, ''), period, source) DO UPDATE SET
			call_count = call_count + excluded.call_count,
			error_count = error_count + excluded.error_count,
			ingested_at = excluded.ingested_at
	`,
		event.ServiceName, event.FunctionName, event.Namespace, event.FilePath,
		event.Period, event.PeriodType, event.CallCount, event.ErrorCount,
		event.UnmatchReason, event.Source, event.IngestedAt.Format(time.RFC3339),
	)
	return err
}

// SaveObservedCaller saves a caller breakdown record
func (s *Storage) SaveObservedCaller(caller *ObservedCaller) error {
	_, err := s.db.Exec(`
		INSERT INTO observed_callers (symbol_id, caller_service, period, call_count)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(symbol_id, caller_service, period) DO UPDATE SET
			call_count = call_count + excluded.call_count
	`,
		caller.SymbolID, caller.CallerService, caller.Period, caller.CallCount,
	)
	return err
}

// GetObservedUsage retrieves usage data for a symbol
func (s *Storage) GetObservedUsage(symbolID string, periodFilter string) ([]ObservedUsage, error) {
	query := `
		SELECT symbol_id, match_quality, match_confidence, period, period_type,
		       call_count, error_count, service_version, source, ingested_at
		FROM observed_usage
		WHERE symbol_id = ?
	`
	args := []interface{}{symbolID}

	if periodFilter != "" {
		query += " AND period >= ?"
		args = append(args, periodFilter)
	}

	query += " ORDER BY period DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usages []ObservedUsage
	for rows.Next() {
		var u ObservedUsage
		var ingestedAt string
		err := rows.Scan(
			&u.SymbolID, &u.MatchQuality, &u.MatchConfidence,
			&u.Period, &u.PeriodType, &u.CallCount, &u.ErrorCount,
			&u.ServiceVersion, &u.Source, &ingestedAt,
		)
		if err != nil {
			return nil, err
		}
		u.IngestedAt, _ = time.Parse(time.RFC3339, ingestedAt)
		usages = append(usages, u)
	}

	return usages, rows.Err()
}

// GetObservedCallers retrieves caller breakdown for a symbol
func (s *Storage) GetObservedCallers(symbolID string, limit int) ([]ObservedCaller, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(`
		SELECT symbol_id, caller_service, period, call_count
		FROM observed_callers
		WHERE symbol_id = ?
		ORDER BY call_count DESC
		LIMIT ?
	`, symbolID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var callers []ObservedCaller
	for rows.Next() {
		var c ObservedCaller
		err := rows.Scan(&c.SymbolID, &c.CallerService, &c.Period, &c.CallCount)
		if err != nil {
			return nil, err
		}
		callers = append(callers, c)
	}

	return callers, rows.Err()
}

// GetSymbolsWithZeroCalls returns symbols with no observed calls in the given period
func (s *Storage) GetSymbolsWithZeroCalls(minPeriod string) ([]string, error) {
	// Get all symbols we have telemetry for, but with zero calls
	rows, err := s.db.Query(`
		SELECT DISTINCT symbol_id
		FROM observed_usage
		WHERE period >= ?
		GROUP BY symbol_id
		HAVING SUM(call_count) = 0
	`, minPeriod)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var symbols []string
	for rows.Next() {
		var symbolID string
		if err := rows.Scan(&symbolID); err != nil {
			return nil, err
		}
		symbols = append(symbols, symbolID)
	}

	return symbols, rows.Err()
}

// GetTotalCallCount returns the total call count for a symbol across all periods
func (s *Storage) GetTotalCallCount(symbolID string) (int64, error) {
	var total sql.NullInt64
	err := s.db.QueryRow(`
		SELECT SUM(call_count) FROM observed_usage WHERE symbol_id = ?
	`, symbolID).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total.Int64, nil
}

// SaveSyncLog saves a sync log entry and returns its ID
func (s *Storage) SaveSyncLog(log *SyncLog) (int64, error) {
	versionsJSON := "{}"
	if log.ServiceVersions != nil {
		b, _ := json.Marshal(log.ServiceVersions)
		versionsJSON = string(b)
	}

	var completedAt interface{}
	if log.CompletedAt != nil {
		completedAt = log.CompletedAt.Format(time.RFC3339)
	}

	result, err := s.db.Exec(`
		INSERT INTO telemetry_sync_log (
			source, started_at, completed_at, status, events_received,
			events_matched_exact, events_matched_strong, events_matched_weak,
			events_unmatched, service_versions, coverage_score, coverage_level, error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		log.Source, log.StartedAt.Format(time.RFC3339), completedAt, log.Status,
		log.EventsReceived, log.EventsMatchedExact, log.EventsMatchedStrong,
		log.EventsMatchedWeak, log.EventsUnmatched, versionsJSON,
		log.CoverageScore, log.CoverageLevel, log.Error,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateSyncLog updates an existing sync log entry
func (s *Storage) UpdateSyncLog(log *SyncLog) error {
	versionsJSON := "{}"
	if log.ServiceVersions != nil {
		b, _ := json.Marshal(log.ServiceVersions)
		versionsJSON = string(b)
	}

	var completedAt interface{}
	if log.CompletedAt != nil {
		completedAt = log.CompletedAt.Format(time.RFC3339)
	}

	_, err := s.db.Exec(`
		UPDATE telemetry_sync_log SET
			completed_at = ?, status = ?, events_received = ?,
			events_matched_exact = ?, events_matched_strong = ?,
			events_matched_weak = ?, events_unmatched = ?,
			service_versions = ?, coverage_score = ?, coverage_level = ?, error = ?
		WHERE id = ?
	`,
		completedAt, log.Status, log.EventsReceived,
		log.EventsMatchedExact, log.EventsMatchedStrong,
		log.EventsMatchedWeak, log.EventsUnmatched, versionsJSON,
		log.CoverageScore, log.CoverageLevel, log.Error, log.ID,
	)
	return err
}

// GetLastSyncLog returns the most recent sync log entry
func (s *Storage) GetLastSyncLog() (*SyncLog, error) {
	var log SyncLog
	var startedAt, completedAt, versionsJSON sql.NullString
	var errorStr sql.NullString

	err := s.db.QueryRow(`
		SELECT id, source, started_at, completed_at, status,
		       events_received, events_matched_exact, events_matched_strong,
		       events_matched_weak, events_unmatched, service_versions,
		       coverage_score, coverage_level, error
		FROM telemetry_sync_log
		ORDER BY started_at DESC
		LIMIT 1
	`).Scan(
		&log.ID, &log.Source, &startedAt, &completedAt, &log.Status,
		&log.EventsReceived, &log.EventsMatchedExact, &log.EventsMatchedStrong,
		&log.EventsMatchedWeak, &log.EventsUnmatched, &versionsJSON,
		&log.CoverageScore, &log.CoverageLevel, &errorStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if startedAt.Valid {
		log.StartedAt, _ = time.Parse(time.RFC3339, startedAt.String)
	}
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339, completedAt.String)
		log.CompletedAt = &t
	}
	if versionsJSON.Valid && versionsJSON.String != "" {
		_ = json.Unmarshal([]byte(versionsJSON.String), &log.ServiceVersions)
	}
	if errorStr.Valid {
		log.Error = errorStr.String
	}

	return &log, nil
}

// SaveCoverageSnapshot saves a coverage snapshot
func (s *Storage) SaveCoverageSnapshot(snapshot *CoverageSnapshot) error {
	warningsJSON := "[]"
	if snapshot.Warnings != nil {
		b, _ := json.Marshal(snapshot.Warnings)
		warningsJSON = string(b)
	}

	_, err := s.db.Exec(`
		INSERT INTO coverage_snapshots (
			snapshot_date, attribute_coverage, match_coverage,
			service_coverage, overall_score, overall_level, warnings
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		snapshot.SnapshotDate.Format(time.RFC3339),
		snapshot.AttributeCoverage, snapshot.MatchCoverage,
		snapshot.ServiceCoverage, snapshot.OverallScore,
		snapshot.OverallLevel, warningsJSON,
	)
	return err
}

// GetCoverageSnapshots returns coverage snapshots for trend analysis
func (s *Storage) GetCoverageSnapshots(days int) ([]CoverageSnapshot, error) {
	since := time.Now().AddDate(0, 0, -days).Format(time.RFC3339)

	rows, err := s.db.Query(`
		SELECT id, snapshot_date, attribute_coverage, match_coverage,
		       service_coverage, overall_score, overall_level, warnings
		FROM coverage_snapshots
		WHERE snapshot_date >= ?
		ORDER BY snapshot_date ASC
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []CoverageSnapshot
	for rows.Next() {
		var snap CoverageSnapshot
		var dateStr, warningsJSON string
		err := rows.Scan(
			&snap.ID, &dateStr, &snap.AttributeCoverage, &snap.MatchCoverage,
			&snap.ServiceCoverage, &snap.OverallScore, &snap.OverallLevel, &warningsJSON,
		)
		if err != nil {
			return nil, err
		}
		snap.SnapshotDate, _ = time.Parse(time.RFC3339, dateStr)
		if warningsJSON != "" && warningsJSON != "[]" {
			_ = json.Unmarshal([]byte(warningsJSON), &snap.Warnings)
		}
		snapshots = append(snapshots, snap)
	}

	return snapshots, rows.Err()
}

// GetUnmappedServices returns services that couldn't be mapped to repos
func (s *Storage) GetUnmappedServices(limit int) ([]string, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(`
		SELECT DISTINCT service_name
		FROM observed_unmatched
		WHERE unmatch_reason = 'no_repo_mapping'
		ORDER BY service_name
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []string
	for rows.Next() {
		var svc string
		if err := rows.Scan(&svc); err != nil {
			return nil, err
		}
		services = append(services, svc)
	}

	return services, rows.Err()
}

// GetEventsLast24h returns count of events received in the last 24 hours
func (s *Storage) GetEventsLast24h() (int64, error) {
	since := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)

	var count sql.NullInt64
	err := s.db.QueryRow(`
		SELECT SUM(events_received)
		FROM telemetry_sync_log
		WHERE started_at >= ?
	`, since).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count.Int64, nil
}

// GetActiveSourcesLast24h returns sources that reported in the last 24 hours
func (s *Storage) GetActiveSourcesLast24h() ([]string, error) {
	since := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)

	rows, err := s.db.Query(`
		SELECT DISTINCT source
		FROM telemetry_sync_log
		WHERE started_at >= ? AND status = 'success'
		ORDER BY source
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sources []string
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			return nil, err
		}
		sources = append(sources, src)
	}

	return sources, rows.Err()
}

// GetObservationWindowDays returns the number of days of telemetry data available
func (s *Storage) GetObservationWindowDays() (int, error) {
	var minPeriod, maxPeriod sql.NullString
	err := s.db.QueryRow(`
		SELECT MIN(period), MAX(period) FROM observed_usage
	`).Scan(&minPeriod, &maxPeriod)
	if err != nil || !minPeriod.Valid || !maxPeriod.Valid {
		return 0, err
	}

	// Parse periods (format: "2024-12" or "2024-W51")
	min, err := parsePeriod(minPeriod.String)
	if err != nil {
		return 0, nil
	}
	max, err := parsePeriod(maxPeriod.String)
	if err != nil {
		return 0, nil
	}

	return int(max.Sub(min).Hours() / 24), nil
}

// parsePeriod parses a period string into a time.Time
func parsePeriod(period string) (time.Time, error) {
	// Try monthly format "2024-12"
	if t, err := time.Parse("2006-01", period); err == nil {
		return t, nil
	}
	// Try weekly format "2024-W51"
	if len(period) >= 8 && period[4] == '-' && period[5] == 'W' {
		year := period[:4]
		week := period[6:]
		// Approximate to first day of that week
		t, err := time.Parse("2006", year)
		if err == nil {
			weekNum := 0
			fmt.Sscanf(week, "%d", &weekNum)
			return t.AddDate(0, 0, (weekNum-1)*7), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid period format: %s", period)
}

// CleanupOldData removes telemetry data older than retentionDays
func (s *Storage) CleanupOldData(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Format("2006-01") // monthly format

	tables := []string{
		"DELETE FROM observed_usage WHERE period < ?",
		"DELETE FROM observed_unmatched WHERE period < ?",
		"DELETE FROM observed_callers WHERE period < ?",
	}

	for _, stmt := range tables {
		if _, err := s.db.Exec(stmt, cutoff); err != nil {
			return err
		}
	}

	return nil
}

// GetMatchStats returns match quality distribution
func (s *Storage) GetMatchStats() (exact, strong, weak, unmatched int, err error) {
	err = s.db.QueryRow(`
		SELECT
			(SELECT COUNT(DISTINCT symbol_id) FROM observed_usage WHERE match_quality = 'exact'),
			(SELECT COUNT(DISTINCT symbol_id) FROM observed_usage WHERE match_quality = 'strong'),
			(SELECT COUNT(DISTINCT symbol_id) FROM observed_usage WHERE match_quality = 'weak'),
			(SELECT COUNT(DISTINCT service_name || ':' || function_name) FROM observed_unmatched)
	`).Scan(&exact, &strong, &weak, &unmatched)
	return
}
