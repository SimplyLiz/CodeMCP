package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// fingerprintJSON is the JSON structure stored in symbol_mappings.fingerprint_json
type fingerprintJSON struct {
	QualifiedContainer  string `json:"qualifiedContainer"`
	Name                string `json:"name"`
	Kind                string `json:"kind"`
	Arity               int    `json:"arity,omitempty"`
	SignatureNormalized string `json:"signatureNormalized,omitempty"`
}

// locationJSON is the JSON structure stored in symbol_mappings.location_json
type locationJSON struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"endLine,omitempty"`
	EndColumn int    `json:"endColumn,omitempty"`
}

// QuerySymbols fetches symbols with cursor-based pagination and filtering
func (h *IndexRepoHandle) QuerySymbols(cursor *CursorData, limit int, filters SymbolFilters) ([]IndexSymbol, *CursorData, int, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Build query
	query := `
		SELECT stable_id, fingerprint_json, location_json, rowid
		FROM symbol_mappings
		WHERE state = 'active'
	`
	args := make([]interface{}, 0)
	argIndex := 1

	// Apply cursor (keyset pagination using rowid)
	if cursor != nil && cursor.LastPK != "" {
		query += fmt.Sprintf(" AND rowid > $%d", argIndex)
		args = append(args, cursor.LastPK)
		argIndex++
	}

	// Apply filters
	if filters.Kind != "" {
		query += fmt.Sprintf(` AND json_extract(fingerprint_json, '$.kind') = $%d`, argIndex)
		args = append(args, filters.Kind)
		argIndex++
	}
	if filters.File != "" {
		query += fmt.Sprintf(` AND json_extract(location_json, '$.path') = $%d`, argIndex)
		args = append(args, filters.File)
		argIndex++
	}

	// Note: Language filter would require language column in schema (future enhancement)

	// Order by rowid for stable pagination
	query += " ORDER BY rowid"

	// Limit + 1 to check for more results
	query += fmt.Sprintf(" LIMIT $%d", argIndex)
	args = append(args, limit+1)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("query symbols: %w", err)
	}
	defer func() { _ = rows.Close() }()

	symbols := make([]IndexSymbol, 0, limit)
	var lastRowID string
	count := 0

	for rows.Next() {
		count++
		if count > limit {
			// We have more results
			break
		}

		var stableID string
		var fpJSON, locJSON string
		var rowid int64

		if err := rows.Scan(&stableID, &fpJSON, &locJSON, &rowid); err != nil {
			return nil, nil, 0, fmt.Errorf("scan symbol: %w", err)
		}

		// Parse fingerprint
		var fp fingerprintJSON
		if err := json.Unmarshal([]byte(fpJSON), &fp); err != nil {
			continue // Skip malformed entries
		}

		// Parse location
		var loc locationJSON
		if err := json.Unmarshal([]byte(locJSON), &loc); err != nil {
			continue // Skip malformed entries
		}

		symbol := IndexSymbol{
			ID:           stableID,
			Name:         fp.Name,
			Kind:         fp.Kind,
			FilePath:     loc.Path,
			FileBasename: filepath.Base(loc.Path),
			Line:         loc.Line,
			Column:       loc.Column,
			Signature:    fp.SignatureNormalized,
			Container:    fp.QualifiedContainer,
		}

		symbols = append(symbols, symbol)
		lastRowID = fmt.Sprintf("%d", rowid)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, 0, fmt.Errorf("iterate symbols: %w", err)
	}

	// Build next cursor if there are more results
	var nextCursor *CursorData
	if count > limit {
		nextCursor = &CursorData{
			Entity: "symbol",
			LastPK: lastRowID,
		}
	}

	// Get total count (approximate for performance)
	var total int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM symbol_mappings WHERE state = 'active'`).Scan(&total)

	return symbols, nextCursor, total, nil
}

// GetSymbol fetches a single symbol by ID
func (h *IndexRepoHandle) GetSymbol(symbolID string) (*IndexSymbol, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var fpJSON, locJSON string
	err := h.db.QueryRow(`
		SELECT fingerprint_json, location_json
		FROM symbol_mappings
		WHERE stable_id = $1 AND state = 'active'
	`, symbolID).Scan(&fpJSON, &locJSON)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get symbol: %w", err)
	}

	var fp fingerprintJSON
	if err := json.Unmarshal([]byte(fpJSON), &fp); err != nil {
		return nil, fmt.Errorf("parse fingerprint: %w", err)
	}

	var loc locationJSON
	if err := json.Unmarshal([]byte(locJSON), &loc); err != nil {
		return nil, fmt.Errorf("parse location: %w", err)
	}

	return &IndexSymbol{
		ID:           symbolID,
		Name:         fp.Name,
		Kind:         fp.Kind,
		FilePath:     loc.Path,
		FileBasename: filepath.Base(loc.Path),
		Line:         loc.Line,
		Column:       loc.Column,
		Signature:    fp.SignatureNormalized,
		Container:    fp.QualifiedContainer,
	}, nil
}

// BatchGetSymbols fetches multiple symbols by ID
func (h *IndexRepoHandle) BatchGetSymbols(ids []string) ([]IndexSymbol, []string, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(ids) == 0 {
		return []IndexSymbol{}, []string{}, nil
	}

	// Build query with placeholders
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT stable_id, fingerprint_json, location_json
		FROM symbol_mappings
		WHERE stable_id IN (%s) AND state = 'active'
	`, strings.Join(placeholders, ","))

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("batch get symbols: %w", err)
	}
	defer func() { _ = rows.Close() }()

	found := make(map[string]bool)
	symbols := make([]IndexSymbol, 0, len(ids))

	for rows.Next() {
		var stableID, fpJSON, locJSON string
		if err := rows.Scan(&stableID, &fpJSON, &locJSON); err != nil {
			continue
		}

		var fp fingerprintJSON
		if err := json.Unmarshal([]byte(fpJSON), &fp); err != nil {
			continue
		}

		var loc locationJSON
		if err := json.Unmarshal([]byte(locJSON), &loc); err != nil {
			continue
		}

		symbols = append(symbols, IndexSymbol{
			ID:           stableID,
			Name:         fp.Name,
			Kind:         fp.Kind,
			FilePath:     loc.Path,
			FileBasename: filepath.Base(loc.Path),
			Line:         loc.Line,
			Column:       loc.Column,
			Signature:    fp.SignatureNormalized,
			Container:    fp.QualifiedContainer,
		})
		found[stableID] = true
	}

	// Find not found IDs
	notFound := make([]string, 0)
	for _, id := range ids {
		if !found[id] {
			notFound = append(notFound, id)
		}
	}

	return symbols, notFound, nil
}

// QueryFiles fetches files with cursor-based pagination
func (h *IndexRepoHandle) QueryFiles(cursor *CursorData, limit int) ([]IndexFile, *CursorData, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	query := `SELECT path, hash, symbol_count FROM indexed_files`
	args := make([]interface{}, 0)
	argIndex := 1

	// Apply cursor
	if cursor != nil && cursor.LastPK != "" {
		query += fmt.Sprintf(" WHERE path > $%d", argIndex)
		args = append(args, cursor.LastPK)
		argIndex++
	}

	query += " ORDER BY path"
	query += fmt.Sprintf(" LIMIT $%d", argIndex)
	args = append(args, limit+1)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("query files: %w", err)
	}
	defer func() { _ = rows.Close() }()

	files := make([]IndexFile, 0, limit)
	var lastPath string
	count := 0

	for rows.Next() {
		count++
		if count > limit {
			break
		}

		var path string
		var hash sql.NullString
		var symbolCount int

		if err := rows.Scan(&path, &hash, &symbolCount); err != nil {
			continue
		}

		file := IndexFile{
			Path:        path,
			Basename:    filepath.Base(path),
			SymbolCount: symbolCount,
		}
		if hash.Valid {
			file.Hash = hash.String
		}

		// Detect language from extension
		file.Language = detectLanguage(path)

		files = append(files, file)
		lastPath = path
	}

	var nextCursor *CursorData
	if count > limit {
		nextCursor = &CursorData{
			Entity: "file",
			LastPK: lastPath,
		}
	}

	return files, nextCursor, nil
}

// QueryCallgraph fetches call edges with cursor-based pagination and filtering
func (h *IndexRepoHandle) QueryCallgraph(cursor *CursorData, limit int, filters CallgraphFilters) ([]IndexCallEdge, *CursorData, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	query := `SELECT caller_id, callee_id, caller_file, call_line, call_col, call_end_col, rowid FROM callgraph WHERE 1=1`
	args := make([]interface{}, 0)
	argIndex := 1

	// Apply cursor
	if cursor != nil && cursor.LastPK != "" {
		query += fmt.Sprintf(" AND rowid > $%d", argIndex)
		args = append(args, cursor.LastPK)
		argIndex++
	}

	// Apply filters
	if filters.CallerID != "" {
		query += fmt.Sprintf(" AND caller_id = $%d", argIndex)
		args = append(args, filters.CallerID)
		argIndex++
	}
	if filters.CalleeID != "" {
		query += fmt.Sprintf(" AND callee_id = $%d", argIndex)
		args = append(args, filters.CalleeID)
		argIndex++
	}
	if filters.CallerFile != "" {
		query += fmt.Sprintf(" AND caller_file = $%d", argIndex)
		args = append(args, filters.CallerFile)
		argIndex++
	}

	query += " ORDER BY rowid"
	query += fmt.Sprintf(" LIMIT $%d", argIndex)
	args = append(args, limit+1)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("query callgraph: %w", err)
	}
	defer func() { _ = rows.Close() }()

	edges := make([]IndexCallEdge, 0, limit)
	var lastRowID string
	count := 0

	for rows.Next() {
		count++
		if count > limit {
			break
		}

		var callerID, calleeID sql.NullString
		var callerFile string
		var callLine, callCol int
		var endCol sql.NullInt64
		var rowid int64

		if err := rows.Scan(&callerID, &calleeID, &callerFile, &callLine, &callCol, &endCol, &rowid); err != nil {
			continue
		}

		edge := IndexCallEdge{
			CalleeID:   calleeID.String,
			CallerFile: callerFile,
			CallLine:   callLine,
			CallCol:    callCol,
			Language:   detectLanguage(callerFile),
		}
		if callerID.Valid {
			edge.CallerID = callerID.String
		}
		if endCol.Valid {
			edge.EndCol = int(endCol.Int64)
		}

		edges = append(edges, edge)
		lastRowID = fmt.Sprintf("%d", rowid)
	}

	var nextCursor *CursorData
	if count > limit {
		nextCursor = &CursorData{
			Entity: "callgraph",
			LastPK: lastRowID,
		}
	}

	return edges, nextCursor, nil
}

// QueryRefs queries file dependencies as a proxy for references
// Note: Full reference support requires SCIP index loading (Phase 2)
func (h *IndexRepoHandle) QueryRefs(cursor *CursorData, limit int, filters RefFilters) ([]IndexRef, *CursorData, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// For Phase 1, we return call edges formatted as refs
	// This gives us "call" kind references only
	query := `
		SELECT caller_file, callee_id, call_line, call_col, call_end_col, rowid
		FROM callgraph
		WHERE 1=1
	`
	args := make([]interface{}, 0)
	argIndex := 1

	// Apply cursor
	if cursor != nil && cursor.LastPK != "" {
		query += fmt.Sprintf(" AND rowid > $%d", argIndex)
		args = append(args, cursor.LastPK)
		argIndex++
	}

	// Apply filters
	if filters.FromFile != "" {
		query += fmt.Sprintf(" AND caller_file = $%d", argIndex)
		args = append(args, filters.FromFile)
		argIndex++
	}
	if filters.ToSymbolID != "" {
		query += fmt.Sprintf(" AND callee_id = $%d", argIndex)
		args = append(args, filters.ToSymbolID)
		argIndex++
	}

	query += " ORDER BY rowid"
	query += fmt.Sprintf(" LIMIT $%d", argIndex)
	args = append(args, limit+1)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("query refs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	refs := make([]IndexRef, 0, limit)
	var lastRowID string
	count := 0

	for rows.Next() {
		count++
		if count > limit {
			break
		}

		var fromFile, toSymbolID string
		var line, col int
		var endCol sql.NullInt64
		var rowid int64

		if err := rows.Scan(&fromFile, &toSymbolID, &line, &col, &endCol, &rowid); err != nil {
			continue
		}

		ref := IndexRef{
			FromFile:   fromFile,
			ToSymbolID: toSymbolID,
			Line:       line,
			Col:        col,
			Kind:       "call", // All callgraph entries are calls
			Language:   detectLanguage(fromFile),
		}
		if endCol.Valid {
			ref.EndCol = int(endCol.Int64)
		}

		refs = append(refs, ref)
		lastRowID = fmt.Sprintf("%d", rowid)
	}

	var nextCursor *CursorData
	if count > limit {
		nextCursor = &CursorData{
			Entity: "ref",
			LastPK: lastRowID,
		}
	}

	return refs, nextCursor, nil
}

// SearchSymbols searches symbols by name using LIKE
func (h *IndexRepoHandle) SearchSymbols(query string, limit int, filters SymbolFilters) ([]IndexSymbol, bool, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Prepare search pattern
	pattern := "%" + query + "%"

	sql := `
		SELECT stable_id, fingerprint_json, location_json
		FROM symbol_mappings
		WHERE state = 'active'
		AND json_extract(fingerprint_json, '$.name') LIKE $1
	`
	args := []interface{}{pattern}
	argIndex := 2

	// Apply filters
	if filters.Kind != "" {
		sql += fmt.Sprintf(` AND json_extract(fingerprint_json, '$.kind') = $%d`, argIndex)
		args = append(args, filters.Kind)
		argIndex++
	}
	if filters.File != "" {
		sql += fmt.Sprintf(` AND json_extract(location_json, '$.path') = $%d`, argIndex)
		args = append(args, filters.File)
		argIndex++
	}

	sql += fmt.Sprintf(" LIMIT $%d", argIndex)
	args = append(args, limit+1)

	rows, err := h.db.Query(sql, args...)
	if err != nil {
		return nil, false, fmt.Errorf("search symbols: %w", err)
	}
	defer func() { _ = rows.Close() }()

	symbols := make([]IndexSymbol, 0, limit)
	count := 0

	for rows.Next() {
		count++
		if count > limit {
			break
		}

		var stableID, fpJSON, locJSON string
		if err := rows.Scan(&stableID, &fpJSON, &locJSON); err != nil {
			continue
		}

		var fp fingerprintJSON
		if err := json.Unmarshal([]byte(fpJSON), &fp); err != nil {
			continue
		}

		var loc locationJSON
		if err := json.Unmarshal([]byte(locJSON), &loc); err != nil {
			continue
		}

		symbols = append(symbols, IndexSymbol{
			ID:           stableID,
			Name:         fp.Name,
			Kind:         fp.Kind,
			FilePath:     loc.Path,
			FileBasename: filepath.Base(loc.Path),
			Line:         loc.Line,
			Column:       loc.Column,
			Signature:    fp.SignatureNormalized,
			Container:    fp.QualifiedContainer,
		})
	}

	return symbols, count > limit, nil
}

// SearchFiles searches files by path using LIKE
func (h *IndexRepoHandle) SearchFiles(query string, limit int) ([]IndexFile, bool, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	pattern := "%" + query + "%"

	rows, err := h.db.Query(`
		SELECT path, hash, symbol_count
		FROM indexed_files
		WHERE path LIKE $1
		ORDER BY path
		LIMIT $2
	`, pattern, limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("search files: %w", err)
	}
	defer func() { _ = rows.Close() }()

	files := make([]IndexFile, 0, limit)
	count := 0

	for rows.Next() {
		count++
		if count > limit {
			break
		}

		var path string
		var hash sql.NullString
		var symbolCount int

		if err := rows.Scan(&path, &hash, &symbolCount); err != nil {
			continue
		}

		file := IndexFile{
			Path:        path,
			Basename:    filepath.Base(path),
			SymbolCount: symbolCount,
			Language:    detectLanguage(path),
		}
		if hash.Valid {
			file.Hash = hash.String
		}

		files = append(files, file)
	}

	return files, count > limit, nil
}

// detectLanguage detects language from file extension
func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".dart":
		return "dart"
	default:
		return ""
	}
}
