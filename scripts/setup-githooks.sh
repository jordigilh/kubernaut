#!/bin/bash
# Setup script for anti-pattern detection git hooks
# Per TESTING_GUIDELINES.md and NT_TEST_ANTI_PATTERN_TRIAGE_DEC_17_2025.md

set -e

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔧 Setting up anti-pattern detection git hooks"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Get the git root directory
GIT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")

# Configure git to use .githooks directory
echo "📂 Configuring git hooks path..."
git config core.hooksPath "$GIT_ROOT/.githooks"

# Make hooks executable
echo "🔐 Making hooks executable..."
chmod +x "$GIT_ROOT/.githooks/pre-commit"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Git hooks configured successfully!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📋 Pre-commit hook will now detect:"
echo "   • NULL-TESTING anti-patterns (ToNot(BeNil), ToNot(BeEmpty))"
echo "   • Skip() in integration tests with required infrastructure"
echo "   • time.Sleep() without approved exceptions"
echo ""
echo "📚 References:"
echo "   - docs/development/business-requirements/TESTING_GUIDELINES.md"
echo "   - docs/handoff/NT_TEST_ANTI_PATTERN_TRIAGE_DEC_17_2025.md"
echo "   - .golangci.yml (forbidigo linter rules)"
echo ""
echo "🧪 Test the hook with: git commit (on test files)"
echo ""

