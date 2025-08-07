# GitHub Actions Local Testing Guide

## 1. Installing act Tool

`act` is a tool for running GitHub Actions locally.

### Linux/macOS (Homebrew)
```bash
brew install act
```

### Linux (Binary)
```bash
curl -s https://api.github.com/repos/nektos/act/releases/latest \
  | grep "browser_download_url.*linux_amd64" \
  | cut -d '"' -f 4 \
  | wget -qi -
chmod +x act_*
sudo mv act_* /usr/local/bin/act
```

### Windows (Chocolatey)
```bash
choco install act-cli
```

## 2. Local Execution

### Prerequisites
⚠️ **Git repository required**: act must be run within a Git repository. Initialize your project with git init if needed.

```bash
# Initialize Git repository first (if needed)
git init
git add .
git commit -m "Initial commit"
```

### Project Configuration (.actrc)

The `.actrc` file is configured in the project root:

```bash
# Use Ubuntu 22.04 image (better compatibility)
-P ubuntu-latest=catthehacker/ubuntu:act-22.04

# Environment variables
--env GO_VERSION=1.23

# Don't use gitignore
--use-gitignore=false

# Verbose output
--verbose
```

### Basic Usage
```bash
# List workflows
act --list

# Recommended: Run CI workflow (.actrc applied automatically)
act -W .github/workflows/ci.yml

# Run specific job only
act -W .github/workflows/ci.yml -j test

# Dry run (check without execution)
act -W .github/workflows/ci.yml --dryrun

# Simulate PR event
act pull_request -W .github/workflows/ci.yml
```

### Environment Variables
```bash
# Create .env file
echo "GITHUB_TOKEN=your_token" > .env

# Run with .env file
act --env-file .env
```

### Secrets Configuration
```bash
# Create .secrets file
echo "GITHUB_TOKEN=your_token" > .secrets

# Use secrets file
act --secret-file .secrets
```

## 3. Limitations

- Not all GitHub Actions are fully supported
- Docker images required (downloaded on first run)
- Some GitHub-specific features may not work

## 4. Manual CI Execution (workflow_dispatch)

CI strategy has changed - CI no longer runs on direct pushes to main branch. Manual execution is available when needed.

### Using GitHub CLI
```bash
# Manually run CI on main branch
gh workflow run CI --ref main

# Run CI on specific branch
gh workflow run CI --ref feature-branch

# Check run status
gh run list --workflow=CI --limit 5
```

### Using GitHub Web UI
1. Go to GitHub repository page → **Actions** tab
2. Click **CI** in left sidebar
3. Click **Run workflow** button (top right)
4. Select branch and click **Run workflow**

## 5. Recommended Workflow

Current CI strategy (inspired by act project):

1. **Local Testing**: Use act for basic functionality checks
2. **Pull Requests**: Automatic CI execution on PRs to main branch
3. **Manual Execution**: Use workflow_dispatch when needed
4. **Releases**: Release workflow triggered by tag push

### Benefits
- **Efficiency**: Reduce unnecessary CI runs
- **Flexibility**: Manual execution when needed
- **Quality Assurance**: Reliable review through PRs

## 6. Debugging

```bash
# Verbose logging
act --verbose

# Dry run (check without execution)
act --dryrun

# Specify platform
act -P ubuntu-latest=catthehacker/ubuntu:act-latest
```