# Generating the SCIP index

This file documents how to regenerate the `.scip/index.scip` file for this fixture.

## Toolchain Versions

- scip-go v0.1.26
- Go 1.24.6

## Commands

```bash
cd testdata/fixtures/go
scip-go --output=.scip/index.scip ./...
```

## Verification

After regeneration, verify the index was created:
```bash
ls -la .scip/index.scip
# Should show a file of ~20-25KB
```

## Last Regenerated

2025-12-26

## Notes

- The index.scip file is committed to the repository for deterministic CI builds
- Do not regenerate unless fixture source files change
- After regenerating, update the golden expected/ files with: `go test ./... -run TestGolden -update`
