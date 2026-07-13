#!/usr/bin/env bash
#
# Smoke test for scripts/localize-tree-sitter-symbols.sh.
#
# Builds a small static archive that mirrors libcode_cartographer.a's symbol
# shape — a tree-sitter runtime object, a grammar object, and a wrapper
# object that references them and exposes codecartographer_* entry points —
# runs the script, and asserts ts_*/tree_sitter_* are no longer global
# while codecartographer_* still is.

set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/../localize-tree-sitter-symbols.sh"

CC="${CC:-cc}"
AR="${AR:-ar}"
NM="${NM:-nm}"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
cd "$WORK"

cat > runtime.c <<'EOF'
int ts_parser_new(void) { return 42; }
int ts_tree_root_node(int x) { return x + 1; }
EOF

cat > grammar.c <<'EOF'
int tree_sitter_rust(void) { return 7; }
EOF

cat > wrapper.c <<'EOF'
extern int ts_parser_new(void);
extern int tree_sitter_rust(void);
int codecartographer_version(void) { return ts_parser_new() + tree_sitter_rust(); }
int codecartographer_render_architecture(void) { return 0; }
EOF

"$CC" -c -fPIC runtime.c -o runtime.o
"$CC" -c -fPIC grammar.c -o grammar.o
"$CC" -c -fPIC wrapper.c -o wrapper.o
"$AR" rcs libfixture.a runtime.o grammar.o wrapper.o

# Mach-O prepends an underscore to C symbol names; ELF does not.
case "$(uname -s)" in
  Darwin) U=_ ;;
  *)      U=  ;;
esac

fail() { echo "FAIL: $*" >&2; exit 1; }

# Pre-condition: baseline archive exposes ts_* and tree_sitter_* as globals.
"$NM" -g runtime.o | grep -qE " T ${U}ts_parser_new\$" \
  || fail "baseline: ${U}ts_parser_new should be global in runtime.o"
"$NM" -g grammar.o | grep -qE " T ${U}tree_sitter_rust\$" \
  || fail "baseline: ${U}tree_sitter_rust should be global in grammar.o"

"$SCRIPT" libfixture.a >/dev/null

# After localization: archive should contain exactly combined.o.
rm -f runtime.o grammar.o wrapper.o
"$AR" x libfixture.a
[[ -f combined.o ]] || fail "expected combined.o inside archive after localization"

GLOBAL_TS="$("$NM" -g combined.o | grep -cE " T ${U}ts_" || true)"
GLOBAL_TSL="$("$NM" -g combined.o | grep -cE " T ${U}tree_sitter_" || true)"
GLOBAL_CARTO="$("$NM" -g combined.o | grep -cE " T ${U}codecartographer_" || true)"

[[ "$GLOBAL_TS" -eq 0 ]]     || fail "ts_* still global ($GLOBAL_TS)"
[[ "$GLOBAL_TSL" -eq 0 ]]    || fail "tree_sitter_* still global ($GLOBAL_TSL)"
[[ "$GLOBAL_CARTO" -ge 2 ]]  || fail "codecartographer_* lost exports (got $GLOBAL_CARTO, want >= 2)"

# And the localized symbols should still be present as local (t), i.e. the
# definitions weren't stripped — just made invisible to the global resolver.
LOCAL_TS="$("$NM" combined.o | grep -cE " t ${U}ts_" || true)"
LOCAL_TSL="$("$NM" combined.o | grep -cE " t ${U}tree_sitter_" || true)"
[[ "$LOCAL_TS" -ge 1 ]]  || fail "ts_* definitions missing post-localization"
[[ "$LOCAL_TSL" -ge 1 ]] || fail "tree_sitter_* definitions missing post-localization"

echo "PASS: ts_* and tree_sitter_* localized; codecartographer_* still exported ($GLOBAL_CARTO symbols)"
