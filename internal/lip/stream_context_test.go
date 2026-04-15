package lip

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// startStreamContextDaemon spins up a Unix socket that responds to any
// request with `frames` in order and then closes. Tests inject the full
// frame sequence they want to exercise — the fake is dumb so behaviour
// under malformed input is exercised by the real daemon's tests, not
// CKB's.
func startStreamContextDaemon(t *testing.T, frames []map[string]any) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "lipstream")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	sock := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("listen: %v", err)
	}
	prev := os.Getenv("LIP_SOCKET")
	os.Setenv("LIP_SOCKET", sock)
	t.Cleanup(func() {
		ln.Close()
		os.RemoveAll(dir)
		os.Setenv("LIP_SOCKET", prev)
	})

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Drain the incoming stream_context request.
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var lenBuf [4]byte
		if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
			return
		}
		reqLen := binary.BigEndian.Uint32(lenBuf[:])
		if _, err := io.CopyN(io.Discard, conn, int64(reqLen)); err != nil {
			return
		}
		for _, f := range frames {
			b, _ := json.Marshal(f)
			var out [4]byte
			binary.BigEndian.PutUint32(out[:], uint32(len(b)))
			if _, err := conn.Write(out[:]); err != nil {
				return
			}
			if _, err := conn.Write(b); err != nil {
				return
			}
		}
	}()
}

func TestStreamContext_ParsesSymbolInfoThenEndStream(t *testing.T) {
	startStreamContextDaemon(t, []map[string]any{
		{
			"type": "symbol_info",
			"symbol_info": map[string]any{
				"uri":          "file:///repo/foo.go",
				"display_name": "Foo",
				"kind":         "function",
			},
			"relevance_score": 0.8,
			"token_cost":      120,
		},
		{
			"type": "symbol_info",
			"symbol_info": map[string]any{
				"uri":          "file:///repo/bar.go",
				"display_name": "Bar",
				"kind":         "struct",
			},
			"relevance_score": 0.6,
			"token_cost":      80,
		},
		{
			"type":             "end_stream",
			"reason":           "token_budget",
			"emitted":          2,
			"total_candidates": 17,
		},
	})

	res, err := StreamContext("file:///repo/foo.go", StreamContextPosition{EndLine: 100}, 1024, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res == nil {
		t.Fatal("nil result, want 2 symbols")
	}
	if len(res.Symbols) != 2 {
		t.Fatalf("symbols = %d, want 2", len(res.Symbols))
	}
	if res.Symbols[0].DisplayName != "Foo" || res.Symbols[0].RelevanceScore != 0.8 {
		t.Errorf("symbol[0] = %+v", res.Symbols[0])
	}
	if res.Reason != "token_budget" || res.Emitted != 2 || res.TotalCandidates != 17 {
		t.Errorf("terminator mismatch: %+v", res)
	}
}

func TestStreamContext_DaemonUnavailableReturnsNil(t *testing.T) {
	prev := os.Getenv("LIP_SOCKET")
	os.Setenv("LIP_SOCKET", "/tmp/ckb-lip-stream-nonexistent.sock")
	t.Cleanup(func() { os.Setenv("LIP_SOCKET", prev) })

	res, err := StreamContext("file:///repo/foo.go", StreamContextPosition{}, 1024, "")
	if err != nil {
		t.Fatalf("err = %v, want nil (silent degradation contract)", err)
	}
	if res != nil {
		t.Fatalf("res = %+v, want nil", res)
	}
}

func TestStreamContext_ErrorFrameAborts(t *testing.T) {
	startStreamContextDaemon(t, []map[string]any{
		{
			"type":    "error",
			"message": "cursor out of range",
			"code":    "cursor_out_of_range",
		},
	})
	res, _ := StreamContext("file:///repo/foo.go", StreamContextPosition{EndLine: 9999}, 1024, "")
	if res != nil {
		t.Fatalf("res = %+v, want nil on error frame", res)
	}
}
