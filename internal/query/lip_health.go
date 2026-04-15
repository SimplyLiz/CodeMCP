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
func (e *Engine) startLipSubscriber() {
	ctx, cancel := context.WithCancel(context.Background())
	e.lipSubCancel = cancel
	go lip.Subscribe(ctx, engineLipSubscriber{e: e})
}
