# Versioning and Release Guide

## 1. Versioning Strategy

This project follows [Semantic Versioning](https://semver.org/):

- **MAJOR.MINOR.PATCH** (e.g., `1.2.3`)
- **Pre-releases**: `1.2.3-beta.1`, `1.2.3-alpha.1`, `1.2.3-rc.1`

### Version Types

| Version | Description | Example |
|---------|-------------|----------|
| Major | Breaking changes | `1.0.0` → `2.0.0` |
| Minor | New features (backward compatible) | `1.0.0` → `1.1.0` |
| Patch | Bug fixes | `1.0.0` → `1.0.1` |
| Pre-release | Beta/RC versions | `1.1.0-beta.1` |

## 2. Release Process

### 2.1 Regular Release

```bash
# 1. Make sure main is green (all PRs merged, CI passing)
git checkout main
git pull

# 2. Create version tag
git tag v1.0.0

# 3. Push tag (release workflow runs automatically)
git push origin v1.0.0
```

### 2.2 Pre-release (Beta)

Pre-release tags trigger the same release workflow:

```bash
git tag v1.1.0-beta.1
git push origin v1.1.0-beta.1
```

## 3. Build Artifacts

### 3.1 File Naming Convention

Release binaries are named without a version suffix (the version is embedded in the binary and shown by `--version`):

```
linktadoru-[OS]-[ARCH][.exe]
```

### 3.2 Supported Platforms

| OS | Architecture | Filename |
|----|--------------|----------|
| Linux | AMD64 | `linktadoru-linux-amd64` |
| Linux | ARM64 | `linktadoru-linux-arm64` |
| macOS | ARM64 | `linktadoru-darwin-arm64` |
| Windows | AMD64 | `linktadoru-windows-amd64.exe` |

## 4. CI/CD Trigger Conditions

See [.github/workflows/README.md](../.github/workflows/README.md) for the authoritative description of the workflows. In short: CI runs on pull requests to `main` (documentation-only changes are skipped) and on manual dispatch; the release workflow runs on `v*` tag pushes.

## 5. Version Information Embedding

Version information is automatically embedded during build:

```go
// cmd/crawler/main.go
var (
    Version   = "dev"      // Replaced with actual version at release
    BuildTime = "unknown"  // Replaced with build time
)
```

### Verification
```bash
./linktadoru --version
# Example output: linktadoru version 1.0.0 (built 2023-12-01T10:00:00Z)
```

## 6. Branch Strategy

```
main (stable, release tags)
 ↑
feature/* (feature branches, merged via PR)
```

| Branch | CI Run | Release |
|--------|--------|---------|
| `main` | Manual dispatch only | Tags only |
| `feature/*` | On PR to `main` | - |

## 7. Manual Local Build

The Makefile derives `VERSION` from `git describe`; override it on the make command line (an environment variable will not override it):

```bash
# Development build
make build

# Release build (with explicit version)
make build VERSION=1.0.0 BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Cross-compile
GOOS=darwin GOARCH=arm64 make build
```

## 8. Hotfixes

For critical bug fixes:

```bash
# 1. Create hotfix branch from main
git checkout main
git checkout -b hotfix/v1.0.1

# 2. Commit fixes and open a PR to main
git commit -m "Fix critical bug"

# 3. After the PR is merged, tag the patch release
git checkout main
git pull
git tag v1.0.1
git push origin v1.0.1
```
