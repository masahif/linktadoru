#!/bin/bash
# Install Git hooks for the project

set -e

HOOKS_DIR=".git/hooks"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "📦 Installing Git hooks for linktadoru project..."

# Create pre-commit hook
cat > "$PROJECT_ROOT/$HOOKS_DIR/pre-commit" << 'EOF'
#!/bin/bash
# pre-commit hook to run code quality checks

set -e

echo "🔍 Running pre-commit checks..."

# Check if make is available
if ! command -v make &> /dev/null; then
    echo "❌ make is not available. Skipping pre-commit checks."
    exit 0
fi

# Run make check (fmt, vet, lint, cyclo, test)
echo "📋 Running make check..."
if ! make check; then
    echo ""
    echo "❌ Pre-commit checks failed!"
    echo "Please fix the issues above and try committing again."
    echo ""
    echo "To run checks manually:"
    echo "  make check"
    echo ""
    echo "To skip this hook temporarily:"
    echo "  git commit --no-verify"
    echo ""
    exit 1
fi

echo "✅ All pre-commit checks passed!"
exit 0
EOF

# Make hooks executable
chmod +x "$PROJECT_ROOT/$HOOKS_DIR/pre-commit"

echo "✅ Git hooks installed successfully!"
echo ""
echo "The following hooks are now active:"
echo "  - pre-commit: Runs 'make check' before each commit"
echo ""
echo "To skip hooks temporarily, use: git commit --no-verify"