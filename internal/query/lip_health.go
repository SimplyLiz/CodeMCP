package query

import (
	"context"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/lip"
)

// lipSemanticAvailable reports whether LIP semantic operations (rerank,
// semantic search) can be trusted. The flag is maintained by the background
// subscriber started in `startLipSubscriber` — no RPC happens here, so this
// is safe to call on the hot query path. Returns false when:
//
//   - the subscriber has not yet observed its first frame (daemon down at
//     startup, or engine created less than a ping ago),
//   - the most recent health frame reported the daemon unavailable,
//   - the index contains vectors from more than one embedding model, which
//     makes cross-vector cosine similarity mathematically meaningless.
func (e *Engine) lipSemanticAvailable() bool {
	e.lipHealthMu.RLock()
	defer e.lipHealthMu.RUnlock()
	if e.lipHealthCheckedAt.IsZero() {
		return false
	}
	return e.cachedLipAvailable && !e.cachedLipMixed
}

// lipSupports reports whether the connected LIP daemon advertised support
// for a given `type` tag in its handshake. Used to gate calls to v2.0+
// RPCs (ExplainMatch, StreamContext, ...) so we don't dispatch messages
// an older daemon will reject as UnknownMessage.
//
// Returns false when the handshake has not completed yet OR the daemon
// predates `supported_messages` (v1.5). Callers should treat false as
// "fall back to the legacy path" rather than as a hard error — the feature
// is advisory, not guaranteed.
func (e *Engine) lipSupports(msgType string) bool {
	e.lipHealthMu.RLock()
	defer e.lipHealthMu.RUnlock()
	if e.lipSupported == nil {
		return false
	}
	_, ok := e.lipSupported[msgType]
	return ok
}

// engineLipSubscriber is the adapter between `lip.Subscribe` and the
// Engine's cached health flags. A dedicated type (instead of binding methods
// to Engine) keeps the handler interface invisible to the rest of the
// package.
type engineLipSubscriber struct{ e *Engine }

func (s engineLipSubscriber) OnHealth(ev lip.HealthEvent) {
	s.e.lipHealthMu.Lock()
	defer s.e.lipHealthMu.Unlock()
	s.e.lipHealthCheckedAt = time.Now()
	s.e.cachedLipAvailable = ev.Available
	s.e.cachedLipMixed = ev.MixedModels
}

func (s engineLipSubscriber) OnIndexChanged(_ lip.IndexChangedEvent) {
	// Pushes are handled opportunistically: the ping that follows carries the
	// refreshed IndexStatus, so we don't need to re-probe here. This hook
	// exists so future consumers (e.g. cache invalidation keyed on
	// AffectedURIs) can extend it without changing the transport.
}

// startLipSubscriber launches the background subscriber goroutine. It is a
// no-op-safe — if the daemon is absent, Subscribe backs off and retries
// until Close is called. The first health frame lands within `pingInterval`
// of daemon availability.
//
// Before starting the subscriber we probe `Handshake` once: it's the only
// RPC that returns `supported_messages`, which we need to gate v2.0+ calls
// on older daemons. Handshake failures are non-fatal (the daemon is likely
// down; Subscribe will retry), and the resulting empty `lipSupported` map
// makes `lipSupports` return false everywhere — callers then fall back to
// their legacy paths.
func (e *Engine) startLipSubscriber() {
	e.probeHandshake()

	ctx, cancel := context.WithCancel(context.Background())
	e.lipSubCancel = cancel
	go lip.Subscribe(ctx, engineLipSubscriber{e: e})
}

// probeHandshake runs the one-shot handshake and stashes the result on the
// Engine. Split out so tests and re-connect paths can call it.
func (e *Engine) probeHandshake() {
	info, _ := lip.Handshake("ckb")
	if info == nil {
		return
	}
	supported := make(map[string]struct{}, len(info.SupportedMessages))
	for _, m := range info.SupportedMessages {
		supported[m] = struct{}{}
	}

	// Follow up with a cheap IndexStatus probe so callers can distinguish
	// "daemon down" from "daemon up but has no content for this workspace".
	// Failures here are non-fatal — we just leave lipIndexProbed=false and
	// consumers treat that as "unknown, don't warn".
	status, _ := lip.IndexStatus()

	e.lipHealthMu.Lock()
	e.lipSupported = supported
	if status != nil {
		e.lipIndexProbed = true
		e.lipIndexedFiles = status.IndexedFiles
	}
	e.lipHealthMu.Unlock()

	if e.logger != nil {
		files := -1
		if status != nil {
			files = status.IndexedFiles
		}
		e.logger.Info("LIP handshake",
			"daemon_version", info.DaemonVersion,
			"protocol_version", info.ProtocolVersion,
			"supported_count", len(info.SupportedMessages),
			"indexed_files", files,
		)
	}
}

// LIPIndexStatus is a UX-facing snapshot of LIP daemon reachability and
// workspace coverage. Consumers (e.g. `ckb review`, `ckb status`) call this
// after engine startup to decide whether to show a "no LIP index" warning.
//
//   - Reachable=false: daemon not running or handshake didn't complete.
//     Silence is expected; no warning.
//   - Reachable=true, IndexedFiles=0: daemon up, nothing indexed. Semantic
//     enrichment will silently return empty — warn the user and offer the
//     `lip index` command.
//   - Reachable=true, IndexedFiles>0: happy path.
type LIPIndexStatus struct {
	Reachable     bool
	IndexedFiles  int
	DaemonVersion string
}

// LIPStatus returns the cached handshake + IndexStatus snapshot captured at
// engine startup. Cheap — no RPC on the hot path.
func (e *Engine) LIPStatus() LIPIndexStatus {
	e.lipHealthMu.RLock()
	defer e.lipHealthMu.RUnlock()
	return LIPIndexStatus{
		Reachable:    e.lipIndexProbed,
		IndexedFiles: e.lipIndexedFiles,
	}
}
