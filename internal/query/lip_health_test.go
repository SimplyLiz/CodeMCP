package query

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// startLipHealthDaemon launches a test LIP socket that replies to every
// connection with the supplied indexStatusResp-shaped payload, and returns
// a counter of handled requests. Points LIP_SOCKET at itself.
func startLipHealthDaemon(t *testing.T, mixedModels bool) *int64 {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"indexed_files":           1,
		"pending_embedding_files": 0,
		"last_updated_ms":         nil,
		"mixed_models":            mixedModels,
		"models_in_index":         []string{"model-a"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	dir, err := os.MkdirTemp("/tmp", "lip")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	sockPath := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("listen: %v", err)
	}

	prev := os.Getenv("LIP_SOCKET")
	os.Setenv("LIP_SOCKET", sockPath)

	var reqs int64
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				var lenBuf [4]byte
				if _, err := io.ReadFull(c, lenBuf[:]); err != nil {
					return
				}
				reqLen := binary.BigEndian.Uint32(lenBuf[:])
				if _, err := io.CopyN(io.Discard, c, int64(reqLen)); err != nil {
					return
				}
				atomic.AddInt64(&reqs, 1)
				var out [4]byte
				binary.BigEndian.PutUint32(out[:], uint32(len(payload)))
				_, _ = c.Write(out[:])
				_, _ = c.Write(payload)
			}(conn)
		}
	}()

	t.Cleanup(func() {
		ln.Close()
		os.RemoveAll(dir)
		os.Setenv("LIP_SOCKET", prev)
	})
	return &reqs
}

func TestLipSemanticAvailable_HealthyIndex(t *testing.T) {
	startLipHealthDaemon(t, false)
	e := &Engine{}
	if !e.lipSemanticAvailable() {
		t.Fatal("lipSemanticAvailable = false for healthy single-model index, want true")
	}
}

func TestLipSemanticAvailable_MixedModels(t *testing.T) {
	startLipHealthDaemon(t, true)
	e := &Engine{}
	if e.lipSemanticAvailable() {
		t.Fatal("lipSemanticAvailable = true while MixedModels is set, want false")
	}
}

func TestLipSemanticAvailable_DaemonDown(t *testing.T) {
	// Point at a socket that doesn't exist.
	prev := os.Getenv("LIP_SOCKET")
	os.Setenv("LIP_SOCKET", "/tmp/ckb-lip-nonexistent.sock")
	t.Cleanup(func() { os.Setenv("LIP_SOCKET", prev) })

	e := &Engine{}
	if e.lipSemanticAvailable() {
		t.Fatal("lipSemanticAvailable = true with no daemon, want false")
	}
}

func TestLipSemanticAvailable_CacheWithinTTL(t *testing.T) {
	reqs := startLipHealthDaemon(t, false)
	e := &Engine{}

	for i := 0; i < 5; i++ {
		if !e.lipSemanticAvailable() {
			t.Fatalf("call %d: lipSemanticAvailable = false, want true", i)
		}
	}
	if got := atomic.LoadInt64(reqs); got != 1 {
		t.Fatalf("daemon RPC count = %d, want 1 (TTL cache should suppress subsequent probes)", got)
	}
}

func TestGetDegradationWarnings_LipMixedModels(t *testing.T) {
	startLipHealthDaemon(t, true)
	e := &Engine{}
	// Prime the cache so GetDegradationWarnings has something to read.
	_ = e.lipSemanticAvailable()

	warnings := e.GetDegradationWarnings()
	var found bool
	for _, w := range warnings {
		if w.Code == "lip_mixed_models" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected lip_mixed_models warning, got %+v", warnings)
	}
}

func TestGetDegradationWarnings_NoWarningBeforeFirstProbe(t *testing.T) {
	// Daemon exists and is mixed, but we never call lipSemanticAvailable so
	// the cache has not been populated — we should not emit a warning.
	startLipHealthDaemon(t, true)
	e := &Engine{}

	warnings := e.GetDegradationWarnings()
	for _, w := range warnings {
		if w.Code == "lip_mixed_models" {
			t.Fatalf("lip_mixed_models warning surfaced before first probe: %+v", w)
		}
	}
}
