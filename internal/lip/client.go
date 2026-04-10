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
