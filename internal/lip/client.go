// Package lip provides a best-effort client for the LIP (Liz Indexing Protocol)
// local socket. All operations degrade silently when LIP is not running —
// callers must never treat LIP unavailability as a fatal error.
package lip

import (
	"encoding/binary"
	"encoding/json"
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

	if _, err := conn.Read(lenBuf); err != nil {
		return nil, nil
	}
	respLen := binary.BigEndian.Uint32(lenBuf)
	if respLen > 4<<20 { // 4 MB cap — embeddings are never this large
		return nil, nil
	}
	respBuf := make([]byte, respLen)
	if _, err := conn.Read(respBuf); err != nil {
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

	if _, err := conn.Read(lenBuf); err != nil {
		return "", false, nil
	}
	respLen := binary.BigEndian.Uint32(lenBuf)
	if respLen > 1<<20 {
		// Sanity cap — ignore malformed responses silently
		return "", false, nil
	}
	respBuf := make([]byte, respLen)
	if _, err := conn.Read(respBuf); err != nil {
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
