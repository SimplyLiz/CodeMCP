// Package lip provides a best-effort client for the LIP (Liz Indexing Protocol)
// local socket. All operations degrade silently when LIP is not running —
// callers must never treat LIP unavailability as a fatal error.
package lip

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"time"
)

// SocketPath returns the path to the LIP Unix domain socket.
// The LIP_SOCKET environment variable overrides the default location.
func SocketPath() string {
	if p := os.Getenv("LIP_SOCKET"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return home + "/.local/share/lip/lip.sock"
}

type annotationGetReq struct {
	Action string `json:"action"`
	URI    string `json:"uri"`
	Key    string `json:"key"`
}

type annotationGetResp struct {
	Value *string `json:"value"`
}

type embeddingGetReq struct {
	Action string `json:"action"`
	URI    string `json:"uri"`
	Model  string `json:"model,omitempty"`
}

type embeddingGetResp struct {
	Vector []float32 `json:"vector"`
	Model  string    `json:"model"`
	Dims   int       `json:"dims"`
}

type embeddingBatchReq struct {
	Action string   `json:"action"`
	URIs   []string `json:"uris"`
	Model  string   `json:"model,omitempty"`
}

type embeddingBatchResp struct {
	Vectors [][]float32 `json:"vectors"`
	Model   string      `json:"model"`
	Dims    int         `json:"dims"`
}

type nearestReq struct {
	Action string `json:"action"`
	URI    string `json:"uri,omitempty"`
	Text   string `json:"text,omitempty"`
	TopK   int    `json:"top_k"`
	Model  string `json:"model,omitempty"`
}

// NearestResult is a single result from a nearest-neighbour query.
type NearestResult struct {
	URI   string  `json:"uri"`
	Score float32 `json:"score"`
}

type nearestResp struct {
	Results []NearestResult `json:"results"`
}

type symbolEmbeddingReq struct {
	Action  string `json:"action"`
	URI     string `json:"uri"`
	Symbol  string `json:"symbol"`
	Context string `json:"context,omitempty"`
	Model   string `json:"model,omitempty"`
}

type indexStatusResp struct {
	IndexedFiles int    `json:"indexed_files"`
	Pending      int    `json:"pending"`
	LastUpdated  string `json:"last_updated"`
}

type fileStatusResp struct {
	Indexed    bool  `json:"indexed"`
	AgeSeconds int64 `json:"age_seconds"`
}

// GetEmbedding requests a quantized embedding vector for the given URI from the
// LIP daemon. model may be empty to use the daemon's default. Returns nil when
// LIP is unavailable or the URI has no embedding — callers must handle nil gracefully.
//
// The returned vector uses TurboQuant-style online VQ: coordinates are pre-rotated
// and scalar-quantized per channel, making dot-product similarity accurate without
// dequantization. Suitable for nearest-neighbour ranking directly as []float32.
func GetEmbedding(lipURI, model string) ([]float32, error) {
	conn, err := net.DialTimeout("unix", SocketPath(), 100*time.Millisecond)
	if err != nil {
		return nil, nil
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(500 * time.Millisecond)) //nolint:errcheck

	payload, _ := json.Marshal(embeddingGetReq{Action: "embedding_get", URI: lipURI, Model: model})
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(payload)))
	if _, err := conn.Write(append(lenBuf, payload...)); err != nil {
		return nil, nil
	}

	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, nil
	}
	respLen := binary.BigEndian.Uint32(lenBuf)
	if respLen > 4<<20 { // 4 MB cap — embeddings are never this large
		return nil, nil
	}
	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respBuf); err != nil {
		return nil, nil
	}

	var resp embeddingGetResp
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return nil, nil
	}
	if len(resp.Vector) == 0 {
		return nil, nil
	}
	return resp.Vector, nil
}

// GetEmbeddingsBatch requests embeddings for multiple URIs in a single round-trip.
// Returns a slice parallel to uris — entries are nil when LIP has no embedding for
// that URI. Returns nil (not an error) when LIP is unavailable.
func GetEmbeddingsBatch(uris []string, model string) ([][]float32, error) {
	if len(uris) == 0 {
		return nil, nil
	}
	conn, err := net.DialTimeout("unix", SocketPath(), 100*time.Millisecond)
	if err != nil {
		return nil, nil
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(time.Duration(len(uris)+1) * 100 * time.Millisecond)) //nolint:errcheck

	payload, _ := json.Marshal(embeddingBatchReq{Action: "embedding_batch", URIs: uris, Model: model})
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(payload)))
	if _, err := conn.Write(append(lenBuf, payload...)); err != nil {
		return nil, nil
	}

	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, nil
	}
	respLen := binary.BigEndian.Uint32(lenBuf)
	if respLen > 64<<20 { // 64 MB cap for batch responses
		return nil, nil
	}
	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respBuf); err != nil {
		return nil, nil
	}

	var resp embeddingBatchResp
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return nil, nil
	}
	// Pad to len(uris) if LIP returns fewer entries (e.g. some URIs unindexed).
	out := make([][]float32, len(uris))
	for i, v := range resp.Vectors {
		if i < len(out) && len(v) > 0 {
			out[i] = v
		}
	}
	return out, nil
}

// NearestByFile returns the top-k files semantically closest to the given file URI.
// Returns nil when LIP is unavailable or the URI has not been indexed.
func NearestByFile(uri string, topK int) ([]NearestResult, error) {
	return nearest(nearestReq{Action: "nearest", URI: uri, TopK: topK})
}

// NearestByText returns the top-k files whose content is semantically closest to
// the given natural-language or code query. Returns nil when LIP is unavailable.
func NearestByText(text string, topK int) ([]NearestResult, error) {
	return nearest(nearestReq{Action: "nearest_by_text", Text: text, TopK: topK})
}

func nearest(req nearestReq) ([]NearestResult, error) {
	conn, err := net.DialTimeout("unix", SocketPath(), 100*time.Millisecond)
	if err != nil {
		return nil, nil
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(500 * time.Millisecond)) //nolint:errcheck

	payload, _ := json.Marshal(req)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(payload)))
	if _, err := conn.Write(append(lenBuf, payload...)); err != nil {
		return nil, nil
	}

	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, nil
	}
	respLen := binary.BigEndian.Uint32(lenBuf)
	if respLen > 4<<20 {
		return nil, nil
	}
	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respBuf); err != nil {
		return nil, nil
	}

	var resp nearestResp
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return nil, nil
	}
	return resp.Results, nil
}

// GetSymbolEmbedding requests an embedding for a specific symbol within a file.
// context should be the symbol's signature and/or leading docstring — LIP uses
// it to anchor the embedding to the symbol rather than the file as a whole.
// Returns nil when LIP is unavailable or has no embedding for the symbol.
func GetSymbolEmbedding(uri, symbol, context, model string) ([]float32, error) {
	conn, err := net.DialTimeout("unix", SocketPath(), 100*time.Millisecond)
	if err != nil {
		return nil, nil
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(500 * time.Millisecond)) //nolint:errcheck

	payload, _ := json.Marshal(symbolEmbeddingReq{
		Action:  "symbol_embedding",
		URI:     uri,
		Symbol:  symbol,
		Context: context,
		Model:   model,
	})
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(payload)))
	if _, err := conn.Write(append(lenBuf, payload...)); err != nil {
		return nil, nil
	}

	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, nil
	}
	respLen := binary.BigEndian.Uint32(lenBuf)
	if respLen > 4<<20 {
		return nil, nil
	}
	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respBuf); err != nil {
		return nil, nil
	}

	var resp embeddingGetResp
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return nil, nil
	}
	if len(resp.Vector) == 0 {
		return nil, nil
	}
	return resp.Vector, nil
}

// IndexStatus returns overall LIP index health — file count, pending, and last
// update time. Returns nil when LIP is unavailable.
func IndexStatus() (*IndexStatusInfo, error) {
	return lipRPC(
		map[string]string{"action": "index_status"},
		200*time.Millisecond,
		4<<10,
		func(r indexStatusResp) *IndexStatusInfo {
			return &IndexStatusInfo{
				IndexedFiles: r.IndexedFiles,
				Pending:      r.Pending,
				LastUpdated:  r.LastUpdated,
			}
		},
	)
}

// IndexStatusInfo is the public view of LIP index health returned by IndexStatus.
type IndexStatusInfo struct {
	IndexedFiles int
	Pending      int
	LastUpdated  string // RFC3339 timestamp or empty
}

// FileStatus returns LIP index status for a single file URI.
// Returns nil when LIP is unavailable or the file is not tracked.
func FileStatus(uri string) (*FileStatusInfo, error) {
	return lipRPC(
		map[string]any{"action": "file_status", "uri": uri},
		200*time.Millisecond,
		4<<10,
		func(r fileStatusResp) *FileStatusInfo {
			return &FileStatusInfo{Indexed: r.Indexed, AgeSeconds: r.AgeSeconds}
		},
	)
}

// FileStatusInfo is the public view of per-file LIP index status.
type FileStatusInfo struct {
	Indexed    bool
	AgeSeconds int64
}

// lipRPC is the shared transport for simple request→response LIP calls.
// T is the JSON response type; U is the public return type.
func lipRPC[T any, U any](req any, timeout time.Duration, maxRespBytes uint32, convert func(T) *U) (*U, error) {
	conn, err := net.DialTimeout("unix", SocketPath(), 100*time.Millisecond)
	if err != nil {
		return nil, nil
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout)) //nolint:errcheck

	payload, _ := json.Marshal(req)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(payload)))
	if _, err := conn.Write(append(lenBuf, payload...)); err != nil {
		return nil, nil
	}

	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, nil
	}
	respLen := binary.BigEndian.Uint32(lenBuf)
	if respLen > maxRespBytes {
		return nil, nil
	}
	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respBuf); err != nil {
		return nil, nil
	}

	var r T
	if err := json.Unmarshal(respBuf, &r); err != nil {
		return nil, nil
	}
	return convert(r), nil
}

// GetAnnotation queries the LIP daemon for an annotation on the given URI and key.
// Returns (value, true, nil) when found, ("", false, nil) when absent or LIP is
// unavailable. The error return is reserved for structural issues (oversized
// response, JSON decode failure after a successful read) but is never fatal.
func GetAnnotation(lipURI, key string) (string, bool, error) {
	conn, err := net.DialTimeout("unix", SocketPath(), 100*time.Millisecond)
	if err != nil {
		// LIP not running — silent degradation
		return "", false, nil
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(200 * time.Millisecond)) //nolint:errcheck

	payload, _ := json.Marshal(annotationGetReq{Action: "annotation_get", URI: lipURI, Key: key})
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(payload)))
	if _, err := conn.Write(append(lenBuf, payload...)); err != nil {
		return "", false, nil
	}

	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return "", false, nil
	}
	respLen := binary.BigEndian.Uint32(lenBuf)
	if respLen > 1<<20 {
		// Sanity cap — ignore malformed responses silently
		return "", false, nil
	}
	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respBuf); err != nil {
		return "", false, nil
	}

	var resp annotationGetResp
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return "", false, nil
	}
	if resp.Value == nil {
		return "", false, nil
	}
	return *resp.Value, true, nil
}
