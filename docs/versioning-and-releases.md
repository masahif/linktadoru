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
# 1. After development is complete, merge develop to main
git checkout main
git merge develop

# 2. Create version tag
git tag v1.0.0

# 3. Push tag (release workflow runs automatically)
git push origin v1.0.0
```

### 2.2 Pre-release (Beta)

```bash
# 1. Create beta tag on develop branch
git checkout develop
git tag v1.1.0-beta.1

# 2. Push tag
git push origin v1.1.0-beta.1
```

## 3. Build Artifacts

### 3.1 File Naming Convention

```
linktadoru-[VERSION]-[OS]-[ARCH][SUFFIX]
```

**Examples:**
- `linktadoru-v1.0.0-linux-amd64`
- `linktadoru-v1.0.0-darwin-arm64`
- `linktadoru-v1.0.0-windows-amd64.exe`

### 3.2 Supported Platforms

| OS | Architecture | Example Filename |
|----|--------------|------------------|
| Linux | AMD64 | `linktadoru-v1.0.0-linux-amd64` |
| macOS | ARM64 | `linktadoru-v1.0.0-darwin-arm64` |
| Windows | AMD64 | `linktadoru-v1.0.0-windows-amd64.exe` |

### 3.3 Checksums

Each binary comes with a `.sha256` file:
```bash
# Verification example
sha256sum -c linktadoru-v1.0.0-linux-amd64.sha256
```

## 4. CI/CD Trigger Conditions

### 4.1 CI (Test & Build)

**Triggered on:**
- Push to `main` or `develop` branches
- Pull requests to `main` branch

**Skipped on:**
- Documentation-only changes (`*.md`, `docs/**`, `LICENSE`, `.gitignore`)

### 4.2 Release

**Triggered on:**
- Tag push with `v*` pattern (e.g., `v1.0.0`, `v1.2.3-beta.1`)

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
main (production)
 ↑
develop (development)
 ↑
feature/* (feature branches)
```

### Branch Behavior

| Branch | CI Run | Release | Description |
|--------|--------|---------|-------------|
| `main` | ✅ | Tags only | Stable version |
| `develop` | ✅ | - | Development version |
| `feature/*` | PR only | - | Feature development |

## 7. Manual Local Build

```bash
# Development build
make build

# Release build (with version)
VERSION=1.0.0 BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) make build

# Cross-compile
GOOS=darwin GOARCH=arm64 make build
```

## 8. Hotfixes

For critical bug fixes:

```bash
# 1. Create hotfix branch from main
git checkout main
git checkout -b hotfix/v1.0.1

# 2. Commit fixes
git commit -m "Fix critical bug"

# 3. Merge to main
git checkout main
git merge hotfix/v1.0.1

# 4. Release patch version
git tag v1.0.1
git push origin v1.0.1

# 5. Merge to develop as well
git checkout develop
git merge main
```