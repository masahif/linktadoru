# GitHub Actions Workflows

This project uses the following workflows:

## CI (ci.yml)
**Triggers**: 
- Pull requests to main branch
- Push to develop branch
- Manual execution (workflow_dispatch)

**Purpose**: Daily development quality checks
- Test execution
- Linting
- Security scanning  
- Build verification

## Release (release.yml)
**Triggers**: 
- Push of v* tags (e.g., v1.0.0)

**Purpose**: Final validation and binary build for releases
- Test execution (final confirmation)
- Multi-platform builds (Linux, macOS, Windows)
- Automatic GitHub Release creation
- Release notes generation

## Local Testing (act)

You can test GitHub Actions locally using act:

```bash
# Install act (macOS)
brew install act

# Run CI workflow locally
act -W .github/workflows/ci.yml

# Run specific job only
act -W .github/workflows/ci.yml -j test

# Debug mode
act -W .github/workflows/ci.yml --verbose
```

## Recommended Workflow

1. **Development**: Work on develop branch → CI runs automatically
2. **Pull Request**: Create PR to main → CI runs automatically  
3. **Release**: Create tag → Release workflow runs automatically

This approach avoids unnecessary CI execution on direct pushes to main branch, running complete tests and builds only during releases.