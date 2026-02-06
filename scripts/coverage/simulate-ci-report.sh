#!/usr/bin/env bash
# scripts/coverage/simulate-ci-report.sh
# Simulate the CI "Generate comprehensive coverage report" step locally using
# artifacts from a real run (e.g. gh run download RUN_ID) or minimal fake artifacts.
# Use this to reproduce why the PR coverage comment shows wrong/missing data.
#
# Usage:
#   ./scripts/coverage/simulate-ci-report.sh [ARTIFACTS_DIR]
#
# Examples:
#   gh run download 123456789   # download all artifacts from run
#   ./scripts/coverage/simulate-ci-report.sh  # uses ./*/coverage-reports from cwd
#
#   ./scripts/coverage/simulate-ci-report.sh _artifacts/ci-coverage  # use this dir
#
#   ./scripts/coverage/simulate-ci-report.sh --fake  # create minimal fake artifacts and run

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

USE_FAKE=false
ARTIFACTS_DIR=""

for arg in "$@"; do
  case "$arg" in
    --fake) USE_FAKE=true ;;
    *)      ARTIFACTS_DIR="$arg" ;;
  esac
done

echo "════════════════════════════════════════════════════════════════"
echo "Simulating CI coverage report generation"
echo "════════════════════════════════════════════════════════════════"
echo ""

# Step 1: Ensure we have coverage-reports/ with *.txt (CI-style one-line summaries)
if [ "$USE_FAKE" = true ]; then
  echo "📁 Creating minimal fake artifacts (service,percent) in coverage-reports/..."
  mkdir -p coverage-reports
  echo "holmesgpt-api,60.37%"   > coverage-reports/unit-holmesgpt-api.txt
  echo "holmesgpt-api,46.86%"   > coverage-reports/integration-holmesgpt-api.txt
  echo "holmesgpt-api,N/A"      > coverage-reports/e2e-holmesgpt-api.txt
  for svc in aianalysis authwebhook datastorage gateway notification remediationorchestrator signalprocessing workflowexecution; do
    echo "$svc,72.5%"           > coverage-reports/unit-$svc.txt
    echo "$svc,55.0%"           > coverage-reports/integration-$svc.txt
    echo "$svc,12.0%"           > coverage-reports/e2e-$svc.txt
  done
  echo "   Done."
  echo ""
elif [ -n "$ARTIFACTS_DIR" ] && [ -d "$ARTIFACTS_DIR" ]; then
  echo "📁 Merging artifacts from $ARTIFACTS_DIR into coverage-reports/..."
  mkdir -p coverage-reports
  for dir in "$ARTIFACTS_DIR"/coverage-unit-* "$ARTIFACTS_DIR"/coverage-integration-* "$ARTIFACTS_DIR"/coverage-e2e-*; do
    [ -d "$dir" ] || continue
    if [ -d "$dir/coverage-reports" ]; then
      for f in "$dir"/coverage-reports/*.txt; do
        [ -f "$f" ] && cp "$f" coverage-reports/
      done
    fi
  done
  echo "   Done."
  echo ""
else
  # Assume cwd has coverage-unit-*, etc. from gh run download
  echo "📁 Merging coverage-* artifact dirs from current directory into coverage-reports/..."
  mkdir -p coverage-reports
  for dir in coverage-unit-* coverage-integration-* coverage-e2e-*; do
    [ -d "$dir" ] || continue
    if [ -d "$dir/coverage-reports" ]; then
      for f in "$dir"/coverage-reports/*.txt; do
        [ -f "$f" ] && cp "$f" coverage-reports/
      done
    fi
  done
  echo "   Done."
  echo ""
fi

echo "🔍 Downloaded coverage artifacts:"
find coverage-reports -name "*.txt" -type f 2>/dev/null | sort || echo "  ⚠️  No coverage-reports directory found"
echo ""

# Step 2: Same reconstruction as CI (creates coverage_${tier}_${service}.txt or .out in repo root)
echo "🔄 Reconstructing coverage files from artifacts (same logic as CI)..."
rm -f coverage_unit_*.txt coverage_integration_*.txt coverage_e2e_*.txt coverage_*.out
if [ -d "coverage-reports" ]; then
  for artifact_file in coverage-reports/*.txt; do
    [ ! -f "$artifact_file" ] && continue
    filename=$(basename "$artifact_file")
    if [[ "$filename" =~ ^(unit|integration|e2e)-(.+)\.txt$ ]]; then
      tier="${BASH_REMATCH[1]}"
      service="${BASH_REMATCH[2]}"
      coverage=$(cut -d',' -f2 "$artifact_file" | head -1 | tr -d '[:space:]')
      echo "  $filename → coverage_${tier}_${service} ($coverage)"
      if [ "$service" = "holmesgpt-api" ] && { [ "$tier" = "unit" ] || [ "$tier" = "integration" ]; }; then
        if [ "$tier" = "integration" ]; then
          echo "TOTAL                                            3523   1872  ${coverage}" > "coverage_integration_holmesgpt-api_python.txt"
        else
          echo "TOTAL                                            3523   1396  ${coverage}" > "coverage_${tier}_${service}.txt"
        fi
      else
        touch "coverage_${tier}_${service}.out"
      fi
    fi
  done
fi
echo ""

echo "🔍 Files report.sh will read (repo root):"
ls -la coverage_unit_*.txt coverage_integration_*.txt coverage_e2e_*.txt coverage_*.out 2>/dev/null || true
echo "  (report.sh expects coverage_unit_holmesgpt-api.txt and coverage_integration_holmesgpt-api_python.txt for HAPI)"
echo ""

# Step 3: Run the same make target as CI
chmod +x scripts/coverage/*.awk scripts/coverage/report.sh 2>/dev/null || true
echo "📊 Running: make coverage-report-markdown"
echo "────────────────────────────────────────────────────────────────"
if make coverage-report-markdown > coverage-summary.md 2>&1; then
  echo "✅ Coverage report generated successfully"
else
  echo "❌ Coverage report generation failed (see coverage-summary.md)"
fi
echo "────────────────────────────────────────────────────────────────"
echo ""
echo "📋 Generated coverage-summary.md (first 50 lines):"
echo ""
head -n 50 coverage-summary.md
echo ""
echo "════════════════════════════════════════════════════════════════"
echo "RC summary:"
echo "  - holmesgpt-api Unit 0.0%%: CI writes only TOTAL line; AWK skips TOTAL and sums only src/ lines → 0.0%%."
echo "  - holmesgpt-api Integration '-': report.sh expects coverage_integration_holmesgpt-api_python.txt; CI creates only .out."
echo "  - Fix: (1) Reconstruct integration Python file (TOTAL line) for HAPI. (2) report.sh fallback: if Python file has only TOTAL, use that %%."
echo "════════════════════════════════════════════════════════════════"
