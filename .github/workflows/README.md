# GitHub Actions Workflows

This project uses the following workflows:

## CI (ci.yml)
**Triggers**:
- Pull requests to `main` (documentation-only changes are skipped via `paths-ignore`: `**.md`, `docs/**`, `LICENSE`, `.gitignore`)
- Manual execution (workflow_dispatch)

Note: direct pushes to `main` do not trigger CI. Run it manually when needed: `gh workflow run CI --ref main`.

**Purpose**: Daily development quality checks
- Test execution
- Linting
- Security scanning  
- Build verification

## Release (release.yml)
**Triggers**: 
- Push of `v*` tags (e.g., `v1.0.0`)

**Purpose**: Final validation and binary build for releases
- Test execution (final confirmation)
- Multi-platform builds (Linux amd64/arm64, macOS arm64, Windows amd64)
- Automatic GitHub Release creation
- Release notes generation

## Local Testing (act)

See [docs/github-actions-local-testing.md](../../docs/github-actions-local-testing.md) for running these workflows locally with act.

## Recommended Workflow

1. **Development**: Work on a feature branch
2. **Pull Request**: Create PR to `main` → CI runs automatically
3. **Release**: Push a `v*` tag → Release workflow runs automatically

This approach avoids unnecessary CI execution on direct pushes to `main`, running complete tests and builds only for PRs and releases.
