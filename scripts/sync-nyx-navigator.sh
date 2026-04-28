#!/usr/bin/env bash
#
# Sync the vendored nyx-navigator source tree at third_party/nyx-navigator/
# from an upstream checkout.
#
# Why vendor at all: CKB links libnavigator.a at build time. Shipping the Rust
# source alongside Go keeps reproducible builds (no network fetch, no
# version-skew between Go code and FFI surface) and lets CI build the archive
# from a pinned snapshot.
#
# Why a script: manual file-by-file copies drift silently. This script is the
# only supported path and should be rerun whenever upstream cuts a release
# worth pulling.
#
# Usage:
#   scripts/sync-nyx-navigator.sh <path-to-upstream-cartographer-repo>
#
# Example:
#   scripts/sync-nyx-navigator.sh ../../../Cartographer
#
# After running: inspect `git diff`, then rebuild:
#   make build-navigator
#   go test -tags navigator ./internal/query/...

set -euo pipefail

UPSTREAM="${1:?usage: $0 <path-to-upstream-cartographer-repo>}"
UPSTREAM_NAV="$UPSTREAM/mapper-core/nyx-navigator"

if [[ ! -d "$UPSTREAM_NAV" ]]; then
  echo "error: $UPSTREAM_NAV not found — pass the upstream repo root (the dir containing mapper-core/)" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CKB_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
VENDOR="$CKB_ROOT/third_party/nyx-navigator"

if [[ ! -d "$VENDOR" ]]; then
  echo "error: vendor tree not found: $VENDOR" >&2
  exit 1
fi

echo "syncing $UPSTREAM_NAV → $VENDOR"

# Rsync the Rust source + build config + FFI header. We explicitly list the
# paths to sync rather than mirroring the whole tree so we never pull in build
# artifacts (target/), IDE files, or editor scratch files. If upstream adds new
# top-level items (e.g. a new subcrate), add them here deliberately.
rsync -a --delete "$UPSTREAM_NAV/src/"     "$VENDOR/src/"
rsync -a --delete "$UPSTREAM_NAV/include/" "$VENDOR/include/"
rsync -a --delete "$UPSTREAM_NAV/scripts/" "$VENDOR/scripts/"
cp "$UPSTREAM_NAV/Cargo.toml"    "$VENDOR/Cargo.toml"
cp "$UPSTREAM_NAV/Cargo.lock"    "$VENDOR/Cargo.lock"
cp "$UPSTREAM_NAV/build.rs"      "$VENDOR/build.rs"
cp "$UPSTREAM_NAV/cbindgen.toml" "$VENDOR/cbindgen.toml"

# No local patches known at this time. If a local patch ever becomes
# necessary (e.g. upstream depends on a private crate we can't vendor),
# reapply it here AFTER the rsync and document WHY inline.

echo "done. next steps:"
echo "  1. review: git -C $CKB_ROOT diff third_party/nyx-navigator/"
echo "  2. build:  cd $CKB_ROOT && make build-navigator"
echo "  3. test:   cd $CKB_ROOT && go test -tags navigator ./internal/query/..."
