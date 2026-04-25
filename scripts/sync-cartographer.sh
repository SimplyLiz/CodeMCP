#!/usr/bin/env bash
#
# Sync the vendored Cartographer source tree at
# third_party/cartographer/mapper-core/cartographer/ from an upstream checkout.
#
# Why vendor at all: CKB links libcartographer.a at build time. Shipping the
# Rust source alongside Go keeps reproducible builds (no network fetch, no
# version-skew between Go code and FFI surface) and lets CI build the archive
# from a pinned snapshot.
#
# Why a script: manual file-by-file copies drift silently — the vendored tree
# was 391 lines behind upstream diagram.rs when the overlays feature landed,
# and nobody noticed until rebuild. This script is the only supported path
# and should be rerun whenever upstream cuts a release worth pulling.
#
# Usage:
#   scripts/sync-cartographer.sh <path-to-upstream-cartographer-checkout>
#
# Example:
#   scripts/sync-cartographer.sh ../../../Cartographer
#
# After running: inspect `git diff`, then rebuild:
#   make build-cartographer
#   go test -tags cartographer ./internal/query/...

set -euo pipefail

UPSTREAM="${1:?usage: $0 <path-to-cartographer-repo>}"
UPSTREAM_CART="$UPSTREAM/mapper-core/cartographer"

if [[ ! -d "$UPSTREAM_CART" ]]; then
  echo "error: $UPSTREAM_CART not found — pass the Cartographer repo root" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CKB_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
VENDOR="$CKB_ROOT/third_party/cartographer/mapper-core/cartographer"

if [[ ! -d "$VENDOR" ]]; then
  echo "error: vendor tree not found: $VENDOR" >&2
  exit 1
fi

echo "syncing $UPSTREAM_CART → $VENDOR"

# Rsync the Rust source + build config + FFI header. We explicitly list the
# paths to sync rather than mirroring the whole tree so we never pull in
# build artifacts (target/), IDE files, or editor scratch files. If upstream
# adds new top-level items (e.g. a new subcrate), add them here deliberately.
rsync -a --delete "$UPSTREAM_CART/src/"     "$VENDOR/src/"
rsync -a --delete "$UPSTREAM_CART/include/" "$VENDOR/include/"
rsync -a --delete "$UPSTREAM_CART/scripts/" "$VENDOR/scripts/"
cp "$UPSTREAM_CART/Cargo.toml"  "$VENDOR/Cargo.toml"
cp "$UPSTREAM_CART/Cargo.lock"  "$VENDOR/Cargo.lock"
cp "$UPSTREAM_CART/build.rs"    "$VENDOR/build.rs"
cp "$UPSTREAM_CART/cbindgen.toml" "$VENDOR/cbindgen.toml"

# No local patches known at this time. If a local patch ever becomes
# necessary (e.g. upstream depends on a private crate we can't vendor),
# reapply it here AFTER the rsync and document WHY inline.

echo "done. next steps:"
echo "  1. review: git -C $CKB_ROOT diff third_party/cartographer/"
echo "  2. build:  cd $CKB_ROOT && make build-cartographer"
echo "  3. test:   cd $CKB_ROOT && go test -tags cartographer ./internal/query/..."
