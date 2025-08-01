# Contributing to LinkTadoru

First off, thank you for considering contributing to LinkTadoru! It's people like you that make LinkTadoru such a great tool.

## Code of Conduct

This project and everyone participating in it is governed by the [LinkTadoru Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check the existing issues as you might find out that you don't need to create one. When you are creating a bug report, please include as many details as possible:

* **Use a clear and descriptive title** for the issue to identify the problem.
* **Describe the exact steps which reproduce the problem** in as many details as possible.
* **Provide specific examples to demonstrate the steps**.
* **Describe the behavior you observed after following the steps** and point out what exactly is the problem with that behavior.
* **Explain which behavior you expected to see instead and why.**
* **Include screenshots and animated GIFs** if possible.
* **Include your environment details** (OS, Go version, etc.).

### Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. When creating an enhancement suggestion, please include:

* **Use a clear and descriptive title** for the issue to identify the suggestion.
* **Provide a step-by-step description of the suggested enhancement** in as many details as possible.
* **Provide specific examples to demonstrate the steps**.
* **Describe the current behavior** and **explain which behavior you expected to see instead** and why.
* **Explain why this enhancement would be useful** to most LinkTadoru users.

### Pull Requests

* Fill in the required template
* Do not include issue numbers in the PR title
* Follow the Go style guide
* Include thoughtfully-worded, well-structured tests
* Document new code based on the GoDoc format
* End all files with a newline

## Development Process

1. Fork the repo and create your branch from `main`.
2. If you've added code that should be tested, add tests.
3. If you've changed APIs, update the documentation.
4. Ensure the test suite passes.
5. Make sure your code lints.
6. Issue that pull request!

### Prerequisites

* Go 1.23 or higher
* golangci-lint (for linting)
* Make (optional, for using Makefile commands)

### Setting up your development environment

```bash
# Clone your fork
git clone https://github.com/your-username/linktadoru.git
cd linktadoru

# Add upstream remote
git remote add upstream https://github.com/masahif/linktadoru.git

# Install dependencies
go mod download

# Run tests
go test ./...

# Run linter
golangci-lint run

# Build the project
make build
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
go test -cover ./...

# Run tests for a specific package
go test -v ./internal/crawler

# Run benchmarks
make bench
```

### Code Style

* Follow the standard Go formatting guidelines (use `gofmt`)
* Use meaningful variable and function names
* Write clear comments and documentation
* Keep functions small and focused
* Handle errors appropriately

### Commit Messages

* Use the present tense ("Add feature" not "Added feature")
* Use the imperative mood ("Move cursor to..." not "Moves cursor to...")
* Limit the first line to 72 characters or less
* Reference issues and pull requests liberally after the first line

Example:
```
Add concurrent crawling support

- Implement worker pool pattern
- Add rate limiting functionality
- Update documentation

Fixes #123
```

## Project Structure

```
linktadoru/
├── cmd/crawler/        # Main application entry point
├── internal/          # Private application code
│   ├── cmd/          # Command-line interface
│   ├── config/       # Configuration handling
│   ├── crawler/      # Core crawling logic
│   ├── parser/       # HTML parsing
│   └── storage/      # Data persistence
├── .github/          # GitHub specific files
├── docs/             # Documentation
└── examples/         # Example configurations and usage
```

## Testing Guidelines

* Write table-driven tests where appropriate
* Aim for at least 80% code coverage
* Test edge cases and error conditions
* Use mocks for external dependencies
* Keep tests fast and independent

## Documentation

* Add GoDoc comments to all exported types, functions, and methods
* Update README.md if you change user-facing functionality
* Include examples in documentation where helpful
* Keep documentation up to date with code changes

## Release Process

1. All changes go through pull requests
2. Maintainers review and merge PRs
3. Releases are tagged following semantic versioning (v1.2.3)
4. GitHub Actions automatically builds and publishes releases

## Questions?

Feel free to open an issue with your question or reach out to the maintainers.

Thank you for contributing to LinkTadoru!