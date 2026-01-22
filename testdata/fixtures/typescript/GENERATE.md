# Generating the SCIP index

This document describes how to regenerate the SCIP index for the TypeScript fixture.

## Toolchain versions used

- Node.js: 20.10.0 (see .nvmrc)
- TypeScript: 5.3.3
- scip-typescript: 0.4.0

## Prerequisites

1. Install Node.js (version from .nvmrc):
   ```bash
   nvm install
   nvm use
   ```

2. Install dependencies:
   ```bash
   npm install
   ```

## Regenerate index

```bash
cd testdata/fixtures/typescript
npx scip-typescript index --output .scip/index.scip
```

## Last regenerated

2025-12-26

## Notes

- The SCIP index is committed to version control for reproducible tests
- Run index regeneration after modifying any source files in src/
- After regenerating, run golden tests with -update flag:
  ```bash
  go test ./internal/query/... -run TestGolden -update -goldenLang=typescript
  ```
