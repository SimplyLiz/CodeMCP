# Branching and Release Strategy

## Branch Structure

```
main          ← stable, released code
  │
  └── develop ← next release prep
        │
        └── feature/xxx ← individual features
```

### `main`
- Always stable and releasable
- Tagged with version numbers (e.g., `v7.2.0`)
- Protected branch - changes only via PR from `develop`

### `develop`
- Preparation branch for the next release
- Collects features for upcoming version
- Merged to `main` when ready to release

### Feature Branches
- Named `feature/<description>` (e.g., `feature/v7.2-multi-tool-setup`)
- Branch from `develop`, merge back to `develop`
- Deleted after merge

## Release Process

### 1. Prepare Release

```bash
# Create develop branch from main
git checkout main
git checkout -b develop

# Add features (either directly or via feature branches)
git commit -m "feat: Add new feature"

# Update version numbers
# - internal/version/version.go
# - npm/package.json (version + optionalDependencies)

# Update CHANGELOG.md
# - Change "Unreleased" to release date
# - Add new "Unreleased" section for next version
```

### 2. Create PR

```bash
gh pr create --base main --head develop --title "Release vX.Y.Z"
```

### 3. Merge and Tag

```bash
# Merge PR
gh pr merge <PR_NUMBER> --merge

# Create GitHub release (this creates the tag)
gh release create vX.Y.Z --title "vX.Y.Z" --notes "Release notes..."
```

### 4. Automated Publishing

The `release.yml` workflow automatically triggers on tag push:

1. Runs tests
2. Builds binaries for all platforms via GoReleaser
3. Publishes to npm:
   - `@tastehub/ckb` (main package)
   - `@tastehub/ckb-darwin-arm64`
   - `@tastehub/ckb-darwin-x64`
   - `@tastehub/ckb-linux-x64`
   - `@tastehub/ckb-linux-arm64`
   - `@tastehub/ckb-win32-x64`

## Version Numbers

We follow [Semantic Versioning](https://semver.org/):

- **MAJOR** (X.0.0) - Breaking changes
- **MINOR** (0.X.0) - New features, backward compatible
- **PATCH** (0.0.X) - Bug fixes, backward compatible

### Files to Update

| File | Field |
|------|-------|
| `internal/version/version.go` | `Version = "X.Y.Z"` |
| `npm/package.json` | `version` + all `optionalDependencies` |
| `CHANGELOG.md` | Release date and notes |

## Changelog Format

We follow [Keep a Changelog](https://keepachangelog.com/):

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Added
- New features

### Changed
- Changes to existing functionality

### Fixed
- Bug fixes

### Removed
- Removed features
```

## Quick Reference

```bash
# Start new release cycle
git checkout main && git pull
git checkout -b develop

# Add feature
git checkout -b feature/my-feature
# ... make changes ...
git commit -m "feat: Description"
git checkout develop
git merge feature/my-feature
git branch -d feature/my-feature

# Release
vim internal/version/version.go  # bump version
vim npm/package.json             # bump version
vim CHANGELOG.md                 # add release date
git add -A && git commit -m "chore: Bump version to X.Y.Z"
git push -u origin develop
gh pr create --base main --head develop --title "Release vX.Y.Z"
gh pr merge <PR> --merge
gh release create vX.Y.Z --title "vX.Y.Z" --notes-file CHANGELOG.md
```
