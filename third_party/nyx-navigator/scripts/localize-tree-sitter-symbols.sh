#!/usr/bin/env bash
#
# Post-process libnavigator.a so its bundled tree-sitter runtime and
# grammar symbols are internal, not externally visible. Consumers that also
# link tree-sitter (e.g. Go projects using github.com/smacker/go-tree-sitter)
# would otherwise trip "duplicate symbol" errors at link time and — worse —
# risk Nyx.Navigator's Rust code binding to the consumer's tree-sitter copy
# if the linker resolved `ts_*` cross-archive. If the two tree-sitter
# versions drift, that route produces silent memory corruption.
#
# Approach: partial-link every .o inside the archive into one combined
# relocatable object so Nyx.Navigator's internal ts_*/tree_sitter_* refs
# resolve within the archive, then mark those symbols local so they no
# longer participate in global symbol resolution. Only the navigator_*
# FFI entry points stay exported.
#
# Requires a C compiler whose linker supports `-r`, an `ar`, and an
# objcopy-style tool. `rust-objcopy` from rustup's llvm-tools-preview
# component works on both Linux (ELF) and macOS (Mach-O).
#
# Usage: localize-tree-sitter-symbols.sh <path/to/libnavigator.a>

set -euo pipefail

ARCHIVE="${1:?usage: $0 <libnavigator.a>}"
case "$ARCHIVE" in
  /*) ARCHIVE_ABS="$ARCHIVE" ;;
  *)  ARCHIVE_ABS="$PWD/$ARCHIVE" ;;
esac

if [[ ! -f "$ARCHIVE_ABS" ]]; then
  echo "error: archive not found: $ARCHIVE_ABS" >&2
  exit 1
fi

pick() {
  for c in "$@"; do
    if command -v "$c" >/dev/null 2>&1; then echo "$c"; return 0; fi
  done
  return 1
}

# `rust-objcopy` ships in the target-specific rustlib bin dir and is not on
# PATH by default; probe it via rustc before falling through to system tools.
OBJCOPY=""
if command -v rustc >/dev/null 2>&1; then
  RUST_BINDIR="$(rustc --print target-libdir 2>/dev/null)/../bin"
  if [[ -x "$RUST_BINDIR/rust-objcopy" ]]; then
    OBJCOPY="$RUST_BINDIR/rust-objcopy"
  fi
fi
if [[ -z "$OBJCOPY" ]]; then
  OBJCOPY="$(pick rust-objcopy llvm-objcopy objcopy)" || {
    echo "error: no objcopy tool found (tried rust-objcopy, llvm-objcopy, objcopy)" >&2
    echo "hint: rustup component add llvm-tools-preview" >&2
    exit 1
  }
fi
CC="$(pick cc clang gcc)"   || { echo "error: no C compiler found" >&2; exit 1; }
AR="$(pick llvm-ar ar)"     || { echo "error: no ar found" >&2; exit 1; }

# Mach-O symbol names carry a leading underscore; ELF does not.
case "$(uname -s)" in
  Darwin) UPREFIX="_" ;;
  *)      UPREFIX=""  ;;
esac

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

cp "$ARCHIVE_ABS" "$WORK/input.a"
(
  cd "$WORK"

  # Partial link (`ld -r`) merges every archive member into a single
  # relocatable object so Nyx.Navigator's internal ts_*/tree_sitter_* refs
  # resolve within the combined object. We feed the archive directly to
  # the linker with a force-load flag rather than `ar x`-extracting first,
  # because Cargo emits multiple `.o` members with identical names (each
  # tree-sitter grammar crate's build.rs produces its own `parser.o` /
  # `scanner.o`) — `ar x` clobbers duplicates on disk, dropping the C
  # parser objects for all but the last grammar. `-force_load` (Mach-O)
  # and `--whole-archive` (ELF) both pull in every member unconditionally,
  # preserving every instance.
  #
  # `-nostdlib` prevents clang/gcc from pulling in CRT or libSystem.
  case "$(uname -s)" in
    Darwin)
      "$CC" -nostdlib -Wl,-r -o combined.o -Wl,-force_load,input.a
      ;;
    *)
      "$CC" -nostdlib -Wl,-r -o combined.o \
        -Wl,--whole-archive input.a -Wl,--no-whole-archive
      ;;
  esac

  # Localize tree-sitter runtime (`ts_*`) and grammar init symbols
  # (`tree_sitter_<lang>`, plus internal `tree_sitter_<lang>_external_scanner_*`
  # helpers). Safe now that the combined object resolved internal refs.
  "$OBJCOPY" \
    --wildcard \
    --localize-symbol="${UPREFIX}ts_*" \
    --localize-symbol="${UPREFIX}tree_sitter_*" \
    combined.o

  # Replace the archive with just the combined, localized object.
  rm -f "$ARCHIVE_ABS"
  "$AR" rcs "$ARCHIVE_ABS" combined.o
)

echo "localized tree-sitter symbols in: $ARCHIVE_ABS"
