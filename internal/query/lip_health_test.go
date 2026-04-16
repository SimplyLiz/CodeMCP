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

// startLipHealthDaemon launches a test LIP socket that speaks the subscriber
// wire format: each connection is handled in a read/write loop — for every
// request frame it writes back an `index_status`-shaped response reflecting
// the supplied `mixedModels` flag. The returned counter records how many
// request frames were served across all connections, so tests can verify
// ping-driven behaviour. Points LIP_SOCKET at the spawned socket.
func startLipHealthDaemon(t *testing.T, mixedModels bool) *int64 {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"type":                    "index_status",
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
				for {
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
					if _, err := c.Write(out[:]); err != nil {
						return
					}
					if _, err := c.Write(payload); err != nil {
						return
					}
				}
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

// waitHealth blocks until the subscriber has populated `lipHealthCheckedAt`
// or the timeout elapses. Returns true when the cache was primed.
func waitHealth(t *testing.T, e *Engine, d time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		e.lipHealthMu.RLock()
		primed := !e.lipHealthCheckedAt.IsZero()
		e.lipHealthMu.RUnlock()
		if primed {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func newSubscribingEngine(t *testing.T) *Engine {
	t.Helper()
	e := &Engine{}
	e.startLipSubscriber()
	t.Cleanup(func() {
		if e.lipSubCancel != nil {
			e.lipSubCancel()
		}
	})
	return e
}

func TestLipSemanticAvailable_HealthyIndex(t *testing.T) {
	startLipHealthDaemon(t, false)
	e := newSubscribingEngine(t)
	if !waitHealth(t, e, 2*time.Second) {
		t.Fatal("subscriber never observed a health frame")
	}
	if !e.lipSemanticAvailable() {
		t.Fatal("lipSemanticAvailable = false for healthy single-model index, want true")
	}
}

func TestLipSemanticAvailable_MixedModels(t *testing.T) {
	startLipHealthDaemon(t, true)
	e := newSubscribingEngine(t)
	if !waitHealth(t, e, 2*time.Second) {
		t.Fatal("subscriber never observed a health frame")
	}
	if e.lipSemanticAvailable() {
		t.Fatal("lipSemanticAvailable = true while MixedModels is set, want false")
	}
}

func TestLipSemanticAvailable_DaemonDown(t *testing.T) {
	prev := os.Getenv("LIP_SOCKET")
	os.Setenv("LIP_SOCKET", "/tmp/ckb-lip-nonexistent.sock")
	t.Cleanup(func() { os.Setenv("LIP_SOCKET", prev) })

	e := newSubscribingEngine(t)
	// Give the subscriber a moment to try-and-fail; it should back off without
	// populating the cache.
	time.Sleep(200 * time.Millisecond)
	if e.lipSemanticAvailable() {
		t.Fatal("lipSemanticAvailable = true with no daemon, want false")
	}
}

func TestLipSubscriber_ReusesSingleConnection(t *testing.T) {
	reqs := startLipHealthDaemon(t, false)
	e := newSubscribingEngine(t)
	if !waitHealth(t, e, 2*time.Second) {
		t.Fatal("subscriber never observed a health frame")
	}
	// Multiple calls to lipSemanticAvailable must NOT drive any new requests
	// — the subscriber owns the connection and the check is lock-free.
	before := atomic.LoadInt64(reqs)
	for i := 0; i < 5; i++ {
		_ = e.lipSemanticAvailable()
	}
	if after := atomic.LoadInt64(reqs); after != before {
		t.Fatalf("lipSemanticAvailable triggered %d extra requests, want 0", after-before)
	}
}

func TestGetDegradationWarnings_LipMixedModels(t *testing.T) {
	startLipHealthDaemon(t, true)
	e := newSubscribingEngine(t)
	if !waitHealth(t, e, 2*time.Second) {
		t.Fatal("subscriber never observed a health frame")
	}

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
	// No daemon running — subscriber can't prime the cache, so no warning
	// should surface regardless of what MixedModels *would* be.
	prev := os.Getenv("LIP_SOCKET")
	os.Setenv("LIP_SOCKET", "/tmp/ckb-lip-nonexistent.sock")
	t.Cleanup(func() { os.Setenv("LIP_SOCKET", prev) })

	e := newSubscribingEngine(t)
	warnings := e.GetDegradationWarnings()
	for _, w := range warnings {
		if w.Code == "lip_mixed_models" {
			t.Fatalf("lip_mixed_models warning surfaced before first probe: %+v", w)
		}
	}
}
