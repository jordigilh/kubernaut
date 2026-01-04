# GitHub Workflows Duplication Triage

**Date**: December 31, 2025
**Issue**: Multiple workflows with duplicate/overlapping intent
**Impact**: Redundant CI runs, slower feedback, resource waste

---

## Current State: 8 Workflow Files

### 🔴 **DUPLICATES - Defense-in-Depth Testing (4 workflows doing same thing!)**

| Workflow File | Size | Name | Triggers | Status |
|---------------|------|------|----------|--------|
| `defense-in-depth-optimized.yml` | 28KB | "Defense-in-Depth (Optimized)" | PR to main (smart paths) | ✅ **KEEP** (Most sophisticated) |
| `defense-in-depth-tests.yml` | 11KB | "Defense-in-Depth Test Suite" | PR + Push to main | ❌ **DELETE** (Duplicate) |
| `test-integration-services.yml` | 10KB | "Defense-in-Depth Test Suite" | PR to main | ❌ **DELETE** (Duplicate) |
| `test-containerized.yml` | 3.8KB | "Containerized Test Suite" | PR to main | ❌ **DELETE** (Duplicate) |

**Evidence from Recent Runs** (Dec 31, 2025):
```
2025-12-31 | Defense-in-Depth Test Suite | failure  ← test-integration-services.yml
2025-12-31 | Defense-in-Depth Test Suite | failure  ← defense-in-depth-tests.yml
2025-12-31 | Defense-in-Depth (Optimized) | failure ← defense-in-depth-optimized.yml
2025-12-31 | Containerized Test Suite | failure     ← test-containerized.yml
```

**Problem**: 4 workflows running on every PR, all testing the same Go services!

---

### ✅ **KEEP - Templates (Reusable Workflows)**

| Workflow File | Purpose | Keep? |
|---------------|---------|-------|
| `e2e-test-template.yml` | Reusable E2E template (`workflow_call`) | ✅ **YES** |
| `integration-test-template.yml` | Reusable integration template (`workflow_call`) | ✅ **YES** |

**Rationale**: These are called by other workflows, not duplicates.

---

### ✅ **KEEP - Service-Specific Workflows**

| Workflow File | Purpose | Keep? |
|---------------|---------|-------|
| `holmesgpt-api-ci.yml` | Python HAPI service (481 unit tests, different stack) | ✅ **YES** |
| `service-maturity-validation.yml` | Service maturity requirements validation | ✅ **YES** |

**Rationale**: Unique purposes, not duplicates.

---

## Recommendation: Delete 3 Duplicate Workflows

### 🗑️ **DELETE THESE:**

1. **`defense-in-depth-tests.yml`** (11KB)
   - **Why**: Older, non-optimized version of defense-in-depth-optimized.yml
   - **Replaced by**: defense-in-depth-optimized.yml (smart path detection)
   - **Runs on**: PR + Push to main
   - **Note**: Same name as test-integration-services.yml causing confusion

2. **`test-integration-services.yml`** (10KB)
   - **Why**: Duplicate of defense-in-depth-optimized.yml
   - **Replaced by**: defense-in-depth-optimized.yml (better smart path detection)
   - **Runs on**: PR to main
   - **Note**: Same name as defense-in-depth-tests.yml causing confusion

3. **`test-containerized.yml`** (3.8KB)
   - **Why**: Redundant containerized testing (covered by defense-in-depth-optimized.yml)
   - **Replaced by**: defense-in-depth-optimized.yml
   - **Runs on**: PR to main
   - **Note**: No clear advantage over optimized workflow

---

### ✅ **KEEP THIS:**

**`defense-in-depth-optimized.yml`** (28KB)
- **Authority**: docs/handoff/TRIAGE_GITHUB_WORKFLOW_OPTIMIZATION_REQUIREMENTS.md
- **Features**:
  - ✅ Smart path detection (Data Storage → ALL, Other → ONLY that service)
  - ✅ 3-stage pipeline (Build/Unit → Integration → E2E)
  - ✅ Parallel execution (8 integration jobs, 8 E2E jobs)
  - ✅ Push to main → Full validation
  - ✅ Most sophisticated and efficient
- **Triggers**: PR to main (Go/Python changes)

---

## Implementation Plan

### Step 1: Delete Duplicate Workflows
```bash
git rm .github/workflows/defense-in-depth-tests.yml
git rm .github/workflows/test-integration-services.yml
git rm .github/workflows/test-containerized.yml
```

### Step 2: Update Submodule Checkout in Remaining Workflows
All workflows need `submodules: true` to fix HolmesGPT dependency:
- ✅ defense-in-depth-optimized.yml (already updated)
- ✅ holmesgpt-api-ci.yml (already updated)
- ⏳ service-maturity-validation.yml (update needed)

### Step 3: Verify No References to Deleted Workflows
Check for any documentation or scripts referencing the deleted workflows.

---

## Benefits After Cleanup

### Before (Current):
- **4 defense-in-depth workflows** running on every PR
- **Redundant testing** (same tests run 4 times)
- **Slower feedback** (parallel resource contention)
- **Confusing names** (2 workflows called "Defense-in-Depth Test Suite")

### After (Proposed):
- **1 defense-in-depth workflow** (optimized)
- **Smart path detection** (only test what changed)
- **Faster feedback** (no redundant runs)
- **Clear naming** (unique workflow names)

---

## Impact Assessment

### CI Runtime Reduction:
- **Before**: 4 workflows × ~30 min = ~120 min of CI time per PR
- **After**: 1 workflow × ~30 min = ~30 min of CI time per PR
- **Savings**: **~75% reduction in CI runtime**

### Resource Savings:
- **Before**: 4 GitHub Actions runners per PR
- **After**: 1 GitHub Actions runner per PR
- **Savings**: **75% reduction in runner usage**

---

## Questions for User

1. **Confirm deletion** of 3 duplicate workflows?
2. **Any special use cases** for the duplicate workflows we should preserve?
3. **Update documentation** referencing these workflows?

---

**Status**: ⏳ **AWAITING USER APPROVAL**
**Next Action**: User confirms deletion → Execute implementation plan

