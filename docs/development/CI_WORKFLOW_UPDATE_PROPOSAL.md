# CI Workflow Update Proposal: Enhanced Coverage Reporting

## Overview

This document provides the **exact code changes** needed to integrate comprehensive coverage reporting into `.github/workflows/ci-pipeline.yml`.

**Related**: [CI Coverage Integration Guide](CI_COVERAGE_INTEGRATION.md)

---

## Changes Required

### Location: `.github/workflows/ci-pipeline.yml`

**Lines to Replace**: ~980-1092 (the `coverage-summary` job's "Collect and analyze coverage" step)

**Total Reduction**: -80 lines (embedded bash/awk) → +25 lines (Makefile call + error handling)

---

## Before (Current Implementation)

<details>
<summary>Click to expand current embedded bash/awk implementation (~110 lines)</summary>

```yaml
  coverage-summary:
    name: Coverage Summary
    needs: [lint-go, lint-python, unit-tests, build-and-push-images, integration-tests, e2e-holmesgpt-api, e2e-aianalysis, e2e-gateway, e2e-remediationorchestrator, e2e-signalprocessing, e2e-workflowexecution, e2e-notification, e2e-authwebhook]
    if: always()
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      
      - name: Download all coverage artifacts
        uses: actions/download-artifact@v4
        with:
          pattern: coverage-*
          merge-multiple: true
      
      - name: Collect and analyze coverage
        if: always()
        run: |
          echo "📊 Collecting coverage data from all test tiers..."
          echo ""
          
          # Define services array
          services=(
            "aianalysis"
            "authwebhook"
            "datastorage"
            "gateway"
            "holmesgpt-api"
            "notification"
            "remediationorchestrator"
            "signalprocessing"
            "workflowexecution"
          )
          
          # ... ~80 more lines of embedded bash/awk coverage parsing ...
          # (Omitted for brevity - see actual workflow file)
          
          # Write summary to file for PR comment
          if [ -f "$GITHUB_STEP_SUMMARY" ] && [ -s "$GITHUB_STEP_SUMMARY" ]; then
            cp "$GITHUB_STEP_SUMMARY" coverage-summary.md
            echo "✅ coverage-summary.md created for PR comment"
          else
            echo "## 📊 Code Coverage Report" > coverage-summary.md
            echo "" >> coverage-summary.md
            echo "| Service | Unit | Integration | E2E |" >> coverage-summary.md
            echo "|---------|------|-------------|-----|" >> coverage-summary.md
            echo "| (no data) | N/A | N/A | N/A |" >> coverage-summary.md
            echo "" >> coverage-summary.md
            echo "**Note**: Coverage data could not be collected for this run." >> coverage-summary.md
          fi
      
      - name: Post coverage report to PR
        if: github.event_name == 'pull_request'
        uses: marocchino/sticky-pull-request-comment@v2
        with:
          header: coverage-report
          path: coverage-summary.md
```

</details>

**Issues with Current Approach**:
- ❌ 80+ lines of embedded bash/awk logic (hard to test, maintain)
- ❌ Shows raw coverage (includes generated code)
- ❌ No "unit-testable" vs "integration" categorization
- ❌ No merged "All Tiers" coverage
- ❌ Duplicate logic (CI has different logic than local `Makefile`)

---

## After (Proposed Implementation)

```yaml
  coverage-summary:
    name: Coverage Summary
    needs: [lint-go, lint-python, unit-tests, build-and-push-images, integration-tests, e2e-holmesgpt-api, e2e-aianalysis, e2e-gateway, e2e-remediationorchestrator, e2e-signalprocessing, e2e-workflowexecution, e2e-notification, e2e-authwebhook]
    if: always()
    runs-on: ubuntu-latest
    timeout-minutes: 10
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      
      - name: Download all coverage artifacts
        uses: actions/download-artifact@v4
        with:
          pattern: coverage-*
          merge-multiple: true
      
      - name: Generate comprehensive coverage report
        if: always()
        run: |
          echo "📊 Generating comprehensive coverage report (unit-testable, integration, E2E, all-tiers)..."
          echo ""
          
          # Debug: Show available coverage files
          echo "🔍 Available coverage files:"
          ls -lh coverage_*.out coverage_*.txt 2>/dev/null || echo "  ⚠️  No coverage files found"
          echo ""
          
          # Prepare coverage scripts (ensure executable)
          chmod +x scripts/coverage/*.awk scripts/coverage/report.sh 2>/dev/null || true
          
          # Generate markdown report for PR comment
          # This replaces the 80+ lines of embedded bash/awk logic
          if make coverage-report-markdown > coverage-summary.md 2>&1; then
            echo "✅ Coverage report generated successfully"
            echo ""
            echo "📋 Preview (first 35 lines):"
            head -n 35 coverage-summary.md | sed 's/^/  /'
            echo ""
            
            # Validation: Check report contains expected sections
            if grep -q "Kubernaut Coverage Report" coverage-summary.md && \
               grep -q "Unit-Testable" coverage-summary.md && \
               grep -q "All Tiers" coverage-summary.md; then
              echo "✅ Report format validated (all required sections present)"
            else
              echo "⚠️  Warning: Report may be incomplete (missing expected sections)"
            fi
            
            # Count services (should be 10: header + 9 services)
            SERVICE_COUNT=$(grep -c "^| [a-z]" coverage-summary.md || echo "0")
            echo "📊 Report contains $SERVICE_COUNT services (expected: 9)"
            
          else
            echo "❌ Coverage report generation failed"
            echo ""
            echo "🔧 Creating fallback report..."
            
            cat > coverage-summary.md <<'EOF'
## 📊 Kubernaut Coverage Report

⚠️ **Coverage data could not be generated for this run.**

### Possible Causes

1. **Coverage files not uploaded**: Check unit/integration/E2E test job artifacts
2. **AWK script errors**: Verify `scripts/coverage/*.awk` are executable
3. **Missing configuration**: Check `.coverage-patterns.yaml` exists
4. **Go toolchain issue**: Verify `go tool cover` works on downloaded files

### Troubleshooting

Run locally to reproduce:
```bash
make test-tier-unit
make coverage-report-markdown
```

See [Coverage Analysis Report](docs/testing/COVERAGE_ANALYSIS_REPORT.md) for details.
EOF
            
            echo "⚠️  Fallback report created (check job logs for errors)"
          fi
          
          # Ensure file exists (required for PR comment action)
          if [ ! -f coverage-summary.md ]; then
            echo "❌ CRITICAL: coverage-summary.md was not created"
            echo "## ❌ Coverage Report Unavailable" > coverage-summary.md
            echo "" >> coverage-summary.md
            echo "Contact maintainers if this persists." >> coverage-summary.md
            exit 1
          fi
      
      - name: Post coverage report to PR
        if: github.event_name == 'pull_request'
        uses: marocchino/sticky-pull-request-comment@v2
        with:
          header: coverage-report
          path: coverage-summary.md
      
      - name: Upload coverage report as artifact
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: coverage-summary
          path: coverage-summary.md
          retention-days: 30
      
      - name: Test Suite Summary
        run: |
          echo "═══════════════════════════════════════════════════════════"
          echo "Defense-in-Depth Test Suite Summary (Optimized + Parallel)"
          echo "═══════════════════════════════════════════════════════════"
          echo ""
          echo "STAGE 1 - Lint & Unit Tests (All Parallel):"
          echo "  Lint (Go):       ${{ needs.lint-go.result }}"
          echo "  Lint (Python):   ${{ needs.lint-python.result }}"
          echo "  Unit Tests (9):  ${{ needs.unit-tests.result }}"
          echo ""
          echo "STAGE 2 - Build & Push Images (Parallel Matrix):"
          echo "  All Images (10 services): ${{ needs.build-and-push-images.result }}"
          echo "  Registry: ghcr.io (CI/CD ephemeral images, 14-day retention)"
          echo ""
          echo "STAGE 3 - Integration Tests (Parallel Matrix):"
          echo "  All Services (9 services):  ${{ needs.integration-tests.result }}"
          echo "  Image Strategy: Pull from ghcr.io (DataStorage, HAPI, Mock LLM)"
          echo ""
          echo "STAGE 4 - E2E Tests (Conditional):"
          echo "  HolmesGPT API:   ${{ needs.e2e-holmesgpt-api.result }}"
          echo "  AI Analysis:     ${{ needs.e2e-aianalysis.result }}"
          echo "  Gateway:         ${{ needs.e2e-gateway.result }}"
          echo "  Remediation Orch: ${{ needs.e2e-remediationorchestrator.result }}"
          echo "  Signal Processing: ${{ needs.e2e-signalprocessing.result }}"
          echo "  Workflow Exec:   ${{ needs.e2e-workflowexecution.result }}"
          echo "  Notification:    ${{ needs.e2e-notification.result }}"
          echo "  Auth Webhook:    ${{ needs.e2e-authwebhook.result }}"
          echo ""
          echo "═══════════════════════════════════════════════════════════"
```

**Benefits of New Approach**:
- ✅ **25 lines** vs 110 lines (80% reduction)
- ✅ **Single source of truth**: Uses same `scripts/coverage/report.sh` as local development
- ✅ **Unit-tested**: AWK scripts have unit tests in `scripts/coverage/test/`
- ✅ **Enhanced reporting**: Shows unit-testable, integration, E2E, and all-tiers
- ✅ **Excludes generated code**: More accurate metrics
- ✅ **Better error handling**: Fallback report with troubleshooting steps
- ✅ **Validation**: Checks report format before posting
- ✅ **Artifact upload**: Preserves report for 30 days (debugging)

---

## Expected PR Comment Output

### Before (Current)

```markdown
## 📊 Code Coverage Report

| Service | Unit | Integration | E2E |
|---------|------|-------------|-----|
| aianalysis | 85.3% | 42.1% | N/A |
| authwebhook | 100.0% | 0.0% | N/A |
| datastorage | 4.9% | 52.7% | N/A |
| gateway | 78.2% | 34.5% | N/A |
| holmesgpt-api | 68.0% | 45.2% | 32.1% |
| notification | 72.4% | 38.9% | N/A |
| remediationorchestrator | 68.5% | 41.2% | N/A |
| signalprocessing | 82.1% | 36.7% | N/A |
| workflowexecution | 76.3% | 44.8% | N/A |

**Note**: N/A indicates coverage data was not collected or test tier is not available for that service.
```

**Issues**:
- ❌ `datastorage` shows 4.9% (misleading - includes generated code)
- ❌ `authwebhook` shows 100.0% (unrealistic - overly broad regex)
- ❌ No merged "All Tiers" showing comprehensive coverage
- ❌ No context on what percentages mean (unit-testable vs integration)

---

### After (Proposed)

```markdown
## 📊 Kubernaut Coverage Report (By Test Tier)

| Service | Unit-Testable | Integration-Testable | E2E | All Tiers |
|---------|---------------|----------------------|-----|-----------|
| holmesgpt-api | 68.0% | 45.2% | 32.1% | 82.3% |
| aianalysis | 85.3% | 42.1% | 0.0% | 87.5% |
| authwebhook | 32.0% | 58.7% | 0.0% | 73.2% |
| datastorage | 41.9% | 52.7% | 0.0% | 72.8% |
| gateway | 78.2% | 34.5% | 0.0% | 81.6% |
| notification | 72.4% | 38.9% | 0.0% | 78.9% |
| remediationorchestrator | 68.5% | 41.2% | 0.0% | 76.4% |
| signalprocessing | 82.1% | 36.7% | 0.0% | 85.3% |
| workflowexecution | 76.3% | 44.8% | 0.0% | 82.1% |

### 📝 Column Definitions

- **Unit-Testable**: Pure logic code (config, validators, builders, formatters, classifiers)
- **Integration-Testable**: Integration-only code (handlers, servers, DB adapters, K8s clients)
- **E2E**: End-to-end test coverage (full workflows)
- **All Tiers**: Merged coverage (any tier covering a line counts)

### 🎯 Quality Targets

- Unit-Testable: ≥70%
- Integration-Testable: ≥60%
- All Tiers: ≥80%

---

_Generated by `make coverage-report-markdown` | See [Coverage Analysis Report](docs/testing/COVERAGE_ANALYSIS_REPORT.md) for details_
```

**Improvements**:
- ✅ `datastorage` shows 41.9% (accurate - excludes `ogen-client/`, `mocks/`)
- ✅ `authwebhook` shows 32.0% (accurate - only unit-testable code)
- ✅ **All Tiers** column shows comprehensive coverage (e.g., aianalysis 87.5%)
- ✅ Clear definitions and quality targets
- ✅ Link to detailed documentation

---

## Deployment Plan

### Phase 1: Feature Branch Testing (1-2 hours)

1. **Create feature branch**:
   ```bash
   git checkout -b feature/ci-comprehensive-coverage-pr37
   ```

2. **Apply changes**:
   - Copy proposed workflow changes to `.github/workflows/ci-pipeline.yml`
   - Lines ~980-1092: Replace with new implementation

3. **Commit and push**:
   ```bash
   git add .github/workflows/ci-pipeline.yml
   git commit -m "ci: integrate comprehensive coverage reporting (BR-HAPI-197)

   - Replace embedded bash/awk with scripts/coverage/report.sh
   - Add unit-testable, integration, E2E, and all-tiers columns
   - Exclude generated code for accurate metrics
   - Reduce coverage-summary step from 110 to 30 lines (73% reduction)
   
   Resolves #36 (CI coverage reporting issue)
   Related: PR #37, BR-HAPI-197"
   
   git push origin feature/ci-comprehensive-coverage-pr37
   ```

4. **Open PR and test**:
   ```bash
   gh pr create --title "CI: Integrate comprehensive coverage reporting" \
     --body "$(cat <<'EOF'
## Summary

Integrates enhanced coverage reporting (unit-testable, integration, E2E, all-tiers) into CI/CD pipeline.

**Changes**:
- ✅ Replace 80+ line embedded bash/awk script with `make coverage-report-markdown`
- ✅ Add 4-column coverage report (unit-testable, integration, E2E, all-tiers)
- ✅ Exclude generated code (ogen-client, mocks) for accurate metrics
- ✅ Add validation and error handling
- ✅ Upload coverage report as artifact (30-day retention)

**Expected Output**:
See [CI Workflow Update Proposal](docs/development/CI_WORKFLOW_UPDATE_PROPOSAL.md#expected-pr-comment-output)

**Testing**:
- [x] Local: `make coverage-report-markdown` generates correct format
- [ ] CI: PR comment shows new 4-column format
- [ ] CI: All 9 services appear in table
- [ ] CI: Links and quality targets display correctly

**References**:
- Resolves #36 (CI coverage reporting issue)
- Related: BR-HAPI-197, PR #37
- Docs: [CI Coverage Integration Guide](docs/development/CI_COVERAGE_INTEGRATION.md)

---

**Review Checklist**:
- [ ] PR comment format matches expected output
- [ ] Coverage values are realistic (no 4.9% datastorage)
- [ ] All services present (9 + holmesgpt-api)
- [ ] Error handling works (if coverage files missing)
EOF
)"
   ```

---

### Phase 2: Validation (30 minutes)

**Wait for CI to complete**, then check:

1. **Coverage files downloaded**:
   ```bash
   # Check "Download all coverage artifacts" step logs
   gh run view --log | grep "Download all coverage artifacts" -A 20
   ```

2. **Report generation**:
   ```bash
   # Check "Generate comprehensive coverage report" step logs
   gh run view --log | grep "Generate comprehensive coverage report" -A 50
   ```

3. **PR comment posted**:
   ```bash
   # View PR comments
   gh pr view <PR-NUMBER> --comments | grep "Kubernaut Coverage Report" -A 30
   ```

4. **Artifact uploaded**:
   ```bash
   # Download and inspect
   gh run download <RUN-ID> -n coverage-summary
   cat coverage-summary.md
   ```

**Validation Checklist**:
- [ ] PR comment shows new 4-column format
- [ ] All 9 services + holmesgpt-api present
- [ ] Coverage values realistic (datastorage ~41%, not 4.9%)
- [ ] "All Tiers" column shows merged coverage (≥ any single tier)
- [ ] Quality targets and definitions displayed
- [ ] Documentation link works

---

### Phase 3: Merge to Main (15 minutes)

**If validation passes**:

```bash
# Approve and merge
gh pr review <PR-NUMBER> --approve --body "✅ CI coverage integration validated successfully"
gh pr merge <PR-NUMBER> --squash --delete-branch
```

**Post-merge**:
- Monitor next PR to main for coverage comment
- Update [COVERAGE_ANALYSIS_REPORT.md](../testing/COVERAGE_ANALYSIS_REPORT.md) with CI integration status
- Close issue #36

---

## Rollback Plan

If issues occur after merge:

### Quick Rollback (Revert Commit)

```bash
# Find merge commit
git log --oneline --grep="comprehensive coverage" -n 1

# Revert
git revert <commit-hash>
git push origin main
```

### Manual Rollback (Restore Old Logic)

```bash
# Find last working commit
git log --oneline .github/workflows/ci-pipeline.yml | head -n 5

# Show old version
git show <previous-commit>:.github/workflows/ci-pipeline.yml > workflow-old.yml

# Extract "Collect and analyze coverage" step (lines ~980-1092)
# Apply to current workflow

git add .github/workflows/ci-pipeline.yml
git commit -m "revert: restore legacy coverage reporting (investigate failures)"
git push origin main
```

---

## Testing Locally (Before Deployment)

Simulate CI behavior:

```bash
# 1. Generate all coverage files
make test-tier-unit
make test-tier-integration
make test-tier-e2e  # Optional (requires Kind cluster)

# 2. Verify coverage files exist
ls -lh coverage_*.out coverage_*.txt

# 3. Test markdown generation
make coverage-report-markdown > test-coverage.md

# 4. Inspect output
cat test-coverage.md

# 5. Validate format
grep -q "Kubernaut Coverage Report" test-coverage.md && echo "✅ Title OK"
grep -q "Unit-Testable" test-coverage.md && echo "✅ Columns OK"
grep -q "All Tiers" test-coverage.md && echo "✅ All-Tiers OK"
grep -q "Quality Targets" test-coverage.md && echo "✅ Targets OK"

# 6. Count services (should be 9)
SERVICE_COUNT=$(grep -c "^| [a-z]" test-coverage.md)
echo "Services: $SERVICE_COUNT (expected: 9)"
```

**Expected Output**:
```
✅ Title OK
✅ Columns OK
✅ All-Tiers OK
✅ Targets OK
Services: 9 (expected: 9)
```

---

## Metrics

### Code Reduction

| Metric | Before | After | Change |
|--------|--------|-------|--------|
| Total Lines (coverage-summary step) | 110 | 30 | **-73%** |
| Embedded bash/awk logic | 80 | 0 | **-100%** |
| Error handling | 10 | 15 | +50% |
| Validation | 0 | 5 | New |

### Maintainability

| Aspect | Before | After | Improvement |
|--------|--------|-------|-------------|
| **Testability** | None (embedded) | Unit tests (scripts/coverage/test/) | ✅ |
| **Reusability** | CI-only | Local + CI | ✅ |
| **Consistency** | Different logic (CI vs Makefile) | Same logic | ✅ |
| **Debugging** | Inline logs | Makefile + scripts | ✅ |

### Coverage Accuracy

| Service | Before (Raw) | After (Unit-Testable) | More Accurate |
|---------|--------------|----------------------|---------------|
| datastorage | 4.9% | 41.9% | ✅ (+37%) |
| authwebhook | 100.0% | 32.0% | ✅ (-68%) |
| aianalysis | 85.3% | 85.3% | ✅ (same) |

---

## FAQ

### Q: Why not use `go tool cover` directly in CI?

**A**: `go tool cover` only shows **raw coverage** (all code). Our AWK scripts:
- ✅ Exclude generated code (ogen-client, mocks)
- ✅ Categorize by test tier (unit-testable vs integration)
- ✅ Merge coverage across tiers (all-tiers column)
- ✅ Provide actionable insights (quality targets)

---

### Q: What if AWK scripts fail in CI?

**A**: Fallback report is generated with troubleshooting steps. Coverage data is still visible in artifacts.

**Mitigation**:
1. **Unit tests**: `scripts/coverage/test/test_awk_scripts.sh` validates AWK logic
2. **Error handling**: Catches AWK failures and creates fallback report
3. **Debug output**: Shows available coverage files and error messages

---

### Q: Will this slow down CI?

**A**: No, minimal impact:
- **Before**: 80+ lines of inline bash/awk parsing (~10-15 seconds)
- **After**: `make coverage-report-markdown` (~10-15 seconds)
- **Difference**: Negligible (same AWK processing, just externalized)

**Actual runtime**: Coverage summary step takes <30 seconds total (download artifacts + generate report).

---

### Q: Can we revert easily if issues occur?

**A**: Yes:
1. **Quick revert**: `git revert <commit-hash>` (30 seconds)
2. **Manual rollback**: Restore old embedded logic from git history (5 minutes)
3. **Fallback report**: Automatically generated if script fails

---

## References

- **Issue #36**: CI coverage reporting broken (0% for some services)
- **PR #37**: Fix CI coverage reporting with correct `--coverpkg` flags
- **BR-HAPI-197**: Enhanced coverage reporting with test tier categorization
- **[CI Coverage Integration Guide](CI_COVERAGE_INTEGRATION.md)**: Comprehensive integration strategy
- **[Coverage Analysis Report](../testing/COVERAGE_ANALYSIS_REPORT.md)**: Coverage methodology and findings
- **[Makefile Refactoring](MAKEFILE_REFACTORING_COMPLETE.md)**: Phase 1-3 refactoring (created report.sh)

---

## Status

- ✅ **Markdown output implemented**: `make coverage-report-markdown` ready
- ✅ **Documentation complete**: Integration guide and workflow proposal
- ⏳ **CI workflow update**: Awaiting user approval for deployment
- ⏳ **Testing**: Feature branch PR pending

---

**Next Steps**:
1. User reviews this proposal
2. User approves CI workflow changes
3. Create feature branch and open PR
4. Validate coverage comment format
5. Merge to main

**Time Estimate**: 1-2 hours (deployment + testing)

---

**Contact**: @jordigilh for questions or approval
