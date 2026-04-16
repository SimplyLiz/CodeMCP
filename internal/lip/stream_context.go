package lip

import (
	"encoding/json"
	"io"
	"net"
	"time"
)

// StreamContextSymbol is one frame of a StreamContext response. The
// embedded `OwnedSymbolInfo` is flattened into the fields we actually
// consume in CKB — the full Rust struct carries many fields we don't
// need (telemetry, relationships, taint) and serialising them through
// `map[string]any` would be wasteful.
type StreamContextSymbol struct {
	URI            string  `json:"uri"`
	DisplayName    string  `json:"display_name"`
	Kind           string  `json:"kind"`
	RelevanceScore float32 `json:"relevance_score"`
	TokenCost      uint32  `json:"token_cost"`
}

// StreamContextResult summarises a completed StreamContext stream.
// `Reason` is one of "token_budget" | "exhausted" | "error".
type StreamContextResult struct {
	Symbols         []StreamContextSymbol
	Reason          string
	Emitted         uint32
	TotalCandidates uint32
	Err             string
}

// StreamContextPosition is the cursor rectangle the daemon ranks around.
// Byte-offset semantics match LIP's `OwnedRange` — 0-based lines and
// chars. Pass a zero-width range at the cursor, or a whole-file range
// (`start_line=0, end_line=lineCount`) for file-level context.
type StreamContextPosition struct {
	StartLine int `json:"start_line"`
	StartChar int `json:"start_char"`
	EndLine   int `json:"end_line"`
	EndChar   int `json:"end_char"`
}

// streamContextMaxFrames caps how many SymbolInfo frames we accept before
// bailing out — defence against a runaway daemon. Large indexes could
// theoretically produce 10k+ candidates; a hard cap of 1024 is well above
// any realistic caller budget and cheap to enforce.
const streamContextMaxFrames = 1024

// StreamContext opens a dedicated connection, sends a `stream_context`
// request, and reads SymbolInfo frames until the daemon writes the
// `end_stream` terminator. Returns (nil, nil) when the daemon is
// unavailable — callers must treat nil as "LIP unavailable" (same contract
// as the rest of the package).
//
// The dedicated connection is intentional: `stream_context` on the
// shared subscriber channel would interleave with IndexStatus pings and
// IndexChanged pushes and complicate parsing. One connection per call is
// fine — the ranking itself dominates latency, and callers shouldn't
// issue this RPC more than a few times per second.
func StreamContext(fileURI string, pos StreamContextPosition, maxTokens uint32, model string) (*StreamContextResult, error) {
	conn, err := net.DialTimeout("unix", SocketPath(), 500*time.Millisecond)
	if err != nil {
		return nil, nil
	}
	defer conn.Close()
	// Overall deadline: the daemon's relevance ranking is heuristic and
	// bounded, but pathological inputs could stall. 10 s is generous; for
	// a token_budget of ~2000 it completes in ~200 ms typically.
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	req := map[string]any{
		"type":            "stream_context",
		"file_uri":        fileURI,
		"cursor_position": pos,
		"max_tokens":      maxTokens,
	}
	if model != "" {
		req["model"] = model
	}
	if err := writeFrame(conn, req); err != nil {
		return nil, nil
	}

	out := &StreamContextResult{Symbols: make([]StreamContextSymbol, 0, 16)}
	for range streamContextMaxFrames + 1 {
		frame, err := readFrame(conn)
		if err != nil {
			if err == io.EOF {
				return out, nil
			}
			return nil, nil
		}
		// ServerResponse { ok: ServerMessage, error: Option<String> }
		inner := frame
		if raw, ok := frame["ok"]; ok && len(raw) > 0 && string(raw) != "null" {
			_ = json.Unmarshal(raw, &inner)
		}
		var kind string
		_ = json.Unmarshal(inner["type"], &kind)

		switch kind {
		case "symbol_info":
			var sym struct {
				SymbolInfo struct {
					URI         string `json:"uri"`
					DisplayName string `json:"display_name"`
					Kind        string `json:"kind"`
				} `json:"symbol_info"`
				RelevanceScore float32 `json:"relevance_score"`
				TokenCost      uint32  `json:"token_cost"`
			}
			if b, ok := marshalInner(inner); ok {
				_ = json.Unmarshal(b, &sym)
			}
			out.Symbols = append(out.Symbols, StreamContextSymbol{
				URI:            sym.SymbolInfo.URI,
				DisplayName:    sym.SymbolInfo.DisplayName,
				Kind:           sym.SymbolInfo.Kind,
				RelevanceScore: sym.RelevanceScore,
				TokenCost:      sym.TokenCost,
			})
		case "end_stream":
			var end struct {
				Reason          string  `json:"reason"`
				Emitted         uint32  `json:"emitted"`
				TotalCandidates uint32  `json:"total_candidates"`
				Error           *string `json:"error"`
			}
			if b, ok := marshalInner(inner); ok {
				_ = json.Unmarshal(b, &end)
			}
			out.Reason = end.Reason
			out.Emitted = end.Emitted
			out.TotalCandidates = end.TotalCandidates
			if end.Error != nil {
				out.Err = *end.Error
			}
			return out, nil
		case "error", "unknown_message":
			// Daemon rejected the request — treat as unavailable.
			return nil, nil
		default:
			// Unknown frame type mid-stream: skip rather than fail hard.
		}
	}
	return out, nil
}
