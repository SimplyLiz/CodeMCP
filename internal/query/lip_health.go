package query

import (
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/lip"
)

// lipHealthTTL caps how often we re-probe the LIP daemon for index status.
// IndexStatus is a 200 ms RPC, so we do not want this per-query.
const lipHealthTTL = 60 * time.Second

// lipSemanticAvailable reports whether LIP semantic operations (rerank, semantic
// search) can be trusted. Returns false when the daemon is unavailable OR when
// the index contains vectors from more than one embedding model — cosine
// similarity across different vector spaces is mathematically meaningless, so a
// mixed-model index silently produces garbage rankings.
func (e *Engine) lipSemanticAvailable() bool {
	e.lipHealthMu.RLock()
	fresh := !e.lipHealthCheckedAt.IsZero() && time.Since(e.lipHealthCheckedAt) < lipHealthTTL
	avail, mixed := e.cachedLipAvailable, e.cachedLipMixed
	e.lipHealthMu.RUnlock()
	if fresh {
		return avail && !mixed
	}

	status, _ := lip.IndexStatus()
	e.lipHealthMu.Lock()
	e.lipHealthCheckedAt = time.Now()
	if status == nil {
		e.cachedLipAvailable, e.cachedLipMixed = false, false
	} else {
		e.cachedLipAvailable, e.cachedLipMixed = true, status.MixedModels
	}
	avail, mixed = e.cachedLipAvailable, e.cachedLipMixed
	e.lipHealthMu.Unlock()
	return avail && !mixed
}
