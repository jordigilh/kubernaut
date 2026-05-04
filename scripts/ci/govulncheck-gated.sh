#!/bin/bash
# govulncheck-gated.sh — Security gate that accepts only known unfixable vulnerabilities
#
# Runs govulncheck and fails if any NEW vulnerability is found that isn't in
# the allow-list (.govulncheck-ignore.yaml). This re-hardens the security gate
# while explicitly accepting upstream vulns with no available fix.
#
# Usage: IGNORE_FILE=.govulncheck-ignore.yaml ./scripts/ci/govulncheck-gated.sh
#
# Exit codes:
#   0 — Only known/accepted vulnerabilities found (or none)
#   1 — NEW actionable vulnerability detected (blocks CI)

set -uo pipefail

IGNORE_FILE="${IGNORE_FILE:-.govulncheck-ignore.yaml}"

if [ ! -f "$IGNORE_FILE" ]; then
  echo "⚠️  No ignore file found at $IGNORE_FILE — running govulncheck without filtering"
  exec govulncheck ./...
fi

# Extract allowed vuln IDs from the YAML (top-level keys matching GO-XXXX-XXXX)
ALLOWED_VULNS=$(grep -E '^GO-[0-9]+-[0-9]+:' "$IGNORE_FILE" | sed 's/://' | tr '\n' '|' | sed 's/|$//')

echo "🔍 Running govulncheck with gated security policy..."
echo "   Accepted vulns (no upstream fix): ${ALLOWED_VULNS}"
echo ""

# Run govulncheck and capture output + exit code
VULN_OUTPUT=$(govulncheck ./... 2>&1) || VULN_EXIT=$?
VULN_EXIT=${VULN_EXIT:-0}

if [ "$VULN_EXIT" -eq 0 ]; then
  echo "✅ No vulnerabilities found"
  exit 0
fi

if [ "$VULN_EXIT" -ne 3 ]; then
  echo "❌ govulncheck failed with unexpected exit code: $VULN_EXIT"
  echo "$VULN_OUTPUT"
  exit 1
fi

# Exit code 3 means vulnerabilities found — extract IDs
FOUND_VULNS=$(echo "$VULN_OUTPUT" | grep -oE 'GO-[0-9]+-[0-9]+' | sort -u)

if [ -z "$FOUND_VULNS" ]; then
  echo "⚠️  govulncheck reported vulns but couldn't parse IDs"
  echo "$VULN_OUTPUT"
  exit 1
fi

# Filter out allowed vulns
NEW_VULNS=$(echo "$FOUND_VULNS" | grep -vE "^($ALLOWED_VULNS)$" || true)

if [ -z "$NEW_VULNS" ]; then
  echo "✅ All found vulnerabilities are in the accepted list (no upstream fix available):"
  echo "$FOUND_VULNS" | sort -u | sed 's/^/   ✓ /'
  exit 0
fi

echo "❌ NEW vulnerabilities detected (not in allow-list):"
echo "$NEW_VULNS" | sed 's/^/   ✗ /'
echo ""
echo "Accepted (no fix available):"
echo "$FOUND_VULNS" | grep -E "^($ALLOWED_VULNS)$" | sed 's/^/   ✓ /' || true
echo ""
echo "Action required: fix the dependency, or add to $IGNORE_FILE with justification."
echo ""
echo "--- Full govulncheck output ---"
echo "$VULN_OUTPUT"
exit 1
