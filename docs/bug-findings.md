# Bug Findings

## SCIP TypeScript Index Location Bug

**Date:** 2026-01-06
**Severity:** High (blocks CI for TypeScript projects)
**Status:** Open

### Summary

`ckb index` fails for TypeScript projects because CKB looks for `index.scip` in `.ckb/` but `scip-typescript` outputs it to the project root.

### Reproduction

1. Initialize CKB in a TypeScript project
2. Run `ckb index`
3. Observe the indexer succeeds but CKB reports failure

### CI Log Evidence

```
Detected TypeScript project (from package.json)
Indexer: scip-typescript
Command: scip-typescript index --infer-tsconfig

Generating SCIP index...

.
+ /home/runner/work/LumaVera/LumaVera (1s 530ms)
done /home/runner/work/LumaVera/LumaVera/index.scip

Warning: Indexer completed but index.scip was not created.
Error: Process completed with exit code 1.
```

### Analysis

- CKB correctly detects TypeScript project
- CKB correctly identifies and runs `scip-typescript index --infer-tsconfig`
- `scip-typescript` successfully creates `index.scip` at **project root**
- CKB then checks for `index.scip` in `.ckb/` directory and fails

### Expected Behavior

CKB should either:
1. Pass an output path flag to `scip-typescript` to write directly to `.ckb/index.scip`
2. Move the `index.scip` file from project root to `.ckb/` after indexing completes
3. Look for `index.scip` in the location where `scip-typescript` actually outputs it

### Workaround

Run the indexer manually and move the file:

```yaml
- name: Initialize and Index Repository
  run: |
    ckb init
    scip-typescript index --infer-tsconfig
    mv index.scip .ckb/index.scip
```

### Why Go Projects Work

The `ckb.yml` workflow in CodeMCP uses `scip-go` for Go projects. Either:
- `scip-go` outputs to a different location that CKB expects
- Or the locally-built `./ckb` binary handles this differently than the npm package

### Files to Investigate

- Indexer invocation logic (where CKB runs the indexer command)
- Post-indexing file location check
- Compare Go vs TypeScript indexer handling
