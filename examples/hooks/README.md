# CKB Git Hooks

Local development hooks for CKB code analysis.

## Available Hooks

### pre-commit

Blocks commits with:
- Complexity violations (cyclomatic > 15, cognitive > 20)
- Critical risk level changes
- Warnings for hotspot modifications

**Installation:**

```bash
cp examples/hooks/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

### pre-push

Validates branch before pushing:
- Analyzes full diff against base branch
- Blocks critical-risk pushes
- Optional dead code detection

**Installation:**

```bash
cp examples/hooks/pre-push .git/hooks/pre-push
chmod +x .git/hooks/pre-push
```

### Pre-commit Framework

For teams using [pre-commit](https://pre-commit.com/):

```bash
pip install pre-commit
cp examples/hooks/.pre-commit-config.yaml .pre-commit-config.yaml
pre-commit install
```

## Configuration

All hooks support environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `CKB_MAX_CYCLOMATIC` | `15` | Maximum cyclomatic complexity |
| `CKB_MAX_COGNITIVE` | `20` | Maximum cognitive complexity |
| `CKB_BLOCK_CRITICAL` | `true` | Block critical risk commits |
| `CKB_WARN_HIGH` | `true` | Warn on high risk commits |
| `CKB_RUN_DEAD_CODE` | `false` | Run dead code check on push |

Example:

```bash
export CKB_MAX_CYCLOMATIC=20
export CKB_BLOCK_CRITICAL=false
git commit -m "message"
```

## Bypassing Hooks

For emergencies, bypass with:

```bash
git commit --no-verify -m "emergency fix"
git push --no-verify
```

## Husky Integration (Node.js)

For JavaScript/TypeScript projects using [Husky](https://typicode.github.io/husky/):

```bash
npm install husky --save-dev
npx husky init
```

Add to `.husky/pre-commit`:

```bash
#!/bin/sh
. "$(dirname "$0")/_/husky.sh"

# CKB complexity check on staged files
FILES=$(git diff --cached --name-only --diff-filter=ACMR | grep -E '\.(ts|tsx|js|jsx)$' || true)

if [ -n "$FILES" ]; then
  for file in $FILES; do
    result=$(npx ckb complexity "$file" --format=json 2>/dev/null || echo '{}')
    cyclo=$(echo "$result" | jq -r '.metrics.cyclomaticComplexity // 0')
    if [ "$cyclo" -gt 15 ]; then
      echo "ERROR: $file has complexity $cyclo (max: 15)"
      exit 1
    fi
  done
fi
```

## lint-staged Integration

For use with [lint-staged](https://github.com/okonet/lint-staged):

```json
{
  "lint-staged": {
    "*.{ts,tsx,js,jsx}": [
      "ckb complexity --format=check"
    ],
    "*.go": [
      "ckb complexity --format=check"
    ]
  }
}
```

## Makefile Integration

Add to your Makefile:

```makefile
.PHONY: hooks
hooks:
	cp examples/hooks/pre-commit .git/hooks/
	cp examples/hooks/pre-push .git/hooks/
	chmod +x .git/hooks/pre-commit .git/hooks/pre-push
	@echo "Hooks installed"

.PHONY: check
check:
	@ckb impact diff --staged --format=markdown
	@ckb complexity . --format=summary
```

## Troubleshooting

### Hook not running

Ensure the hook is executable:

```bash
chmod +x .git/hooks/pre-commit
```

### CKB not found

The hooks check for CKB installation. If not installed:

```bash
npm install -g @tastehub/ckb
```

### Slow analysis

Use `--if-stale` for indexing:

```bash
ckb index --if-stale=24h
```

Or skip hooks temporarily:

```bash
git commit --no-verify -m "message"
```
