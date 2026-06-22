#!/usr/bin/env bash
# check-acm-schema-drift.sh — Validates the ACM Search GraphQL schema contract
# across ALL supported upstream release branches (ACM 2.13-2.17 / OCP 4.18-4.22).
#
# Two checks per branch:
#   1. Vendored copy drift: has the vendored branch (release-2.13) been updated?
#   2. Query compatibility: is our adapter's query still valid against each branch's schema?
#      Uses a small Go test program with gqlparser to validate the query.
#
# Exit 0 if all branches pass, exit 1 if any check fails.
# Gracefully skips branches that cannot be fetched (network unavailable).
#
# Usage:
#   ./scripts/check-acm-schema-drift.sh
#   make lint-acm-schema-drift

set -euo pipefail

VENDORED="pkg/fleet/acm/testdata/acm-search-schema.graphqls"
REPO="stolostron/search-v2-api"
FILE_PATH="graph/schema.graphqls"
VENDORED_BRANCH="release-2.13"

# Supported ACM release branches mapped to OCP versions:
#   release-2.13 → OCP 4.18 (compatibility floor, vendored copy source)
#   release-2.14 → OCP 4.19
#   release-2.15 → OCP 4.20
#   release-2.16 → OCP 4.21
#   release-2.17 → OCP 4.22
BRANCHES="release-2.13 release-2.14 release-2.15 release-2.16 release-2.17"

# The exact query our adapter sends (must match acm.SearchQuery in types.go).
QUERY='query($input: [SearchInput]) { searchResult: search(input: $input) { count } }'

if [ ! -f "$VENDORED" ]; then
    echo "ERROR: vendored schema not found at $VENDORED"
    exit 1
fi

TMPDIR_WORK=$(mktemp -d)
trap 'rm -rf "$TMPDIR_WORK"' EXIT

failed=0
checked=0
skipped=0

for branch in $BRANCHES; do
    url="https://raw.githubusercontent.com/${REPO}/${branch}/${FILE_PATH}"
    schema_file="${TMPDIR_WORK}/${branch}.graphqls"

    if ! curl -fsSL "$url" -o "$schema_file" 2>/dev/null; then
        echo "  SKIP  ${branch} (could not fetch)"
        skipped=$((skipped + 1))
        continue
    fi

    checked=$((checked + 1))

    # Check 1: vendored copy drift (only for the branch we vendored from)
    if [ "$branch" = "$VENDORED_BRANCH" ]; then
        if ! diff -q "$VENDORED" "$schema_file" >/dev/null 2>&1; then
            echo "  DRIFT ${branch} (vendored copy is stale)"
            echo ""
            diff -u "$VENDORED" "$schema_file" \
                --label "vendored" --label "upstream (${branch})" || true
            echo ""
            failed=$((failed + 1))
            continue
        fi
    fi

    # Check 2: query compatibility — validate our adapter query against this branch's schema.
    # Uses go run with a small inline program that calls gqlparser.
    validation_output=$(cd "$(dirname "$0")/.." && go run ./scripts/validate-graphql-query \
        -schema "$schema_file" \
        -query "$QUERY" 2>&1) || {
        echo "  FAIL  ${branch} — adapter query is INVALID against this schema"
        echo "        ${validation_output}"
        failed=$((failed + 1))
        continue
    }

    echo "  OK    ${branch}"
done

echo ""
echo "Checked ${checked} branch(es), skipped ${skipped}, failed ${failed}"

if [ "$checked" -eq 0 ]; then
    echo "WARNING: no branches could be fetched — skipping drift check"
    exit 0
fi

if [ "$failed" -gt 0 ]; then
    echo ""
    echo "Action required: one or more ACM release branches failed validation."
    echo "If the vendored copy drifted, update it and re-run the contract test"
    echo "(UT-ACM-054-009). If the query is invalid against a branch, the upstream"
    echo "schema has a breaking change that requires updating the adapter."
    exit 1
fi

echo "ACM Search schema: adapter query is valid against all supported branches"
