# Incremental Indexing: Multi-Language Support

**Status:** Implemented
**Created:** 2025-12-26
**Implemented:** 2025-12-26
**Target:** v7.6

---

## Summary

Incremental indexing is Go-only due to one hardcoded line. The fix is straightforward: make the indexer configurable via a registry pattern.

**Effort:** 4-5 hours
**Languages:** Dart → TypeScript → Python (in that order)

---

## Current State

```go
// internal/incremental/extractor.go:107
cmd := exec.Command("scip-go", "--output", e.indexPath)  // ← The problem
```

The rest of the incremental system is **already language-agnostic**:
- Change detection (git diff) ✅
- Delta extraction from SCIP protobuf ✅
- Database updates ✅
- Callgraph maintenance ✅

---

## Implementation

### 1. Indexer Registry Pattern

Create a registry instead of switch statements:

```go
// internal/project/indexers.go

type IndexerConfig struct {
    Cmd         string   // Base command
    Args        []string // Additional args before output flag
    OutputFlag  string   // Flag for output path (empty if fixed)
    FixedOutput string   // For indexers that ignore output flag (e.g., rust-analyzer)
}

var Indexers = map[Language]IndexerConfig{
    LangGo:         {Cmd: "scip-go", OutputFlag: "--output"},
    LangTypeScript: {Cmd: "scip-typescript", Args: []string{"index", "--infer-tsconfig"}, OutputFlag: "--output"},
    LangJavaScript: {Cmd: "scip-typescript", Args: []string{"index", "--infer-tsconfig"}, OutputFlag: "--output"},
    LangPython:     {Cmd: "scip-python", Args: []string{"index", "."}, OutputFlag: "--output"},
    LangDart:       {Cmd: "dart", Args: []string{"pub", "global", "run", "scip_dart", "./"}, OutputFlag: "--output"},
    LangRust:       {Cmd: "rust-analyzer", Args: []string{"scip", "."}, FixedOutput: "index.scip"},
}

func (c IndexerConfig) BuildCommand(outputPath string) *exec.Cmd {
    args := append([]string{}, c.Args...)
    if c.OutputFlag != "" {
        args = append(args, c.OutputFlag, outputPath)
    }
    return exec.Command(c.Cmd, args...)
}
```

### 2. Graceful Degradation

When indexer is missing, fall back to full reindex instead of failing:

```go
// internal/incremental/indexer.go

func (i *IncrementalIndexer) IndexIncremental(ctx context.Context, since string, lang Language) (*DeltaStats, error) {
    // Check if incremental is available for this language
    config, ok := project.Indexers[lang]
    if !ok {
        return nil, ErrIncrementalNotSupported
    }

    // Check if indexer is installed
    if !i.extractor.IsIndexerAvailable(config) {
        i.logger.Warn("Incremental indexer not found, falling back to full", map[string]interface{}{
            "language": lang,
            "install":  project.GetIndexerInfo(lang).InstallCommand,
        })
        return nil, ErrIndexerNotInstalled
    }

    // ... rest of incremental logic
}
```

CLI output:
```
$ ckb index
⚠ Incremental indexing requires scip-dart
  Install: dart pub global activate scip_dart
  Falling back to full index...
```

### 3. Handle Fixed Output Paths

For rust-analyzer which ignores `--output`:

```go
func (e *SCIPExtractor) RunIndexer(config IndexerConfig) error {
    cmd := config.BuildCommand(e.indexPath)
    cmd.Dir = e.repoRoot

    if err := cmd.Run(); err != nil {
        return err
    }

    // Handle indexers with fixed output paths
    if config.FixedOutput != "" {
        fixedPath := filepath.Join(e.repoRoot, config.FixedOutput)
        if fixedPath != e.indexPath {
            if err := os.Rename(fixedPath, e.indexPath); err != nil {
                return fmt.Errorf("failed to move index: %w", err)
            }
        }
    }

    return nil
}
```

---

## Priority Order

| Order | Language | Rationale |
|-------|----------|-----------|
| 1 | **Dart** | Dogfooding — used daily in TasteHub, reveals bugs |
| 2 | **TypeScript** | Largest user base, most requested |
| 3 | **Python** | Fast indexer, common in AI/ML projects |

Phase 2 (v8.2): Rust
Phase 3 (v8.3): Java/Kotlin

---

## Testing Strategy

### Test Fixtures

Create small repos (10-20 files) for each language:

```
testdata/incremental/
├── dart/
│   ├── pubspec.yaml
│   ├── lib/main.dart
│   └── lib/utils.dart
├── typescript/
│   ├── package.json
│   ├── src/index.ts
│   └── src/utils.ts
└── python/
    ├── pyproject.toml
    ├── src/main.py
    └── src/utils.py
```

### Validation Steps

1. Full index the fixture repo
2. Modify one file (add a function)
3. Run incremental
4. Query DB: verify only that file's symbols updated
5. Verify new function is findable via `searchSymbols`

### CI Integration

```yaml
- name: Test incremental indexing
  run: |
    for lang in dart typescript python; do
      cd testdata/incremental/$lang
      ckb index --force  # Full index
      echo "// change" >> lib/main.*
      ckb index  # Should be incremental
      # Verify delta stats show 1 file changed
    done
```

---

## Files to Change

| File | Change |
|------|--------|
| `internal/project/indexers.go` | New file: registry pattern |
| `internal/incremental/extractor.go` | `RunSCIPGo()` → `RunIndexer(config)` |
| `internal/incremental/indexer.go` | Pass language, add graceful degradation |
| `cmd/ckb/index.go` | Detect language, pass to incremental |
| `testdata/incremental/` | New fixtures for TS/Python/Dart |

---

## Timeline

| Task | Estimate |
|------|----------|
| Create indexer registry | 30 min |
| Refactor extractor | 30 min |
| Add graceful degradation | 30 min |
| Output path handling | 15 min |
| Test fixtures (3 langs) | 1.5 hours |
| Manual testing | 1 hour |
| Docs update | 30 min |
| **Total** | **4.5 hours** |

---

## Success Criteria

- [x] `ckb index` uses incremental for Dart/TS/Python when indexer available
- [x] Missing indexer shows install hint and falls back gracefully
- [x] Delta stats correctly show changed file count
- [x] `searchSymbols` finds symbols from incrementally-indexed files
- [ ] CI tests pass for all three languages (requires test fixtures)
