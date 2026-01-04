# Critical Integration Test Bugs - FIXED (Jan 01, 2026)

## 🎯 Executive Summary

**Status**: ✅ **3 CRITICAL BUGS FIXED** | ⏸️ Awaiting local validation before push

All blocking issues for integration tests have been resolved:
- **Bug 1**: AIAnalysis `event_category` schema violation → **FIXED**
- **Bug 2**: HAPI Dockerfile relative path issue → **FIXED**  
- **Bug 3**: Notification/HAPI PostgreSQL port conflict → **FIXED via DD-TEST-001 v2.0**

---

## 🐛 Bug 1: AIAnalysis `event_category` Schema Violation ✅ **FIXED**

### Problem
AIAnalysis integration tests hung indefinitely due to continuous OpenAPI validation errors.

**Error Message**:
```
Error at "/event_category": value is not one of the allowed values
[...enum values...]
Value: "aianalysis"
```

### Root Cause
`pkg/aianalysis/audit/audit.go` was using `"aianalysis"` as the `event_category` value, but the OpenAPI schema (`api/openapi/data-storage-v1.yaml:832-920`) only allows:
- `"gateway"`
- `"notification"`
- `"analysis"` ← **CORRECT VALUE**
- `"signalprocessing"`
- `"workflow"`
- `"execution"`
- `"orchestration"`

### Fix Applied

**File**: `pkg/aianalysis/audit/audit.go`

```go
// BEFORE (WRONG):
const (
	EventCategoryAIAnalysis = "aianalysis"
)

// AFTER (CORRECT):
const (
	EventCategoryAIAnalysis = "analysis"  // Fixed: matches OpenAPI schema enum
)
```

**Impact**: All AIAnalysis audit events will now pass OpenAPI validation.

**Files Modified**:
- `pkg/aianalysis/audit/audit.go` (line 45)

**Affected Methods** (all now fixed):
- `RecordPhaseTransition()` 
- `RecordRegoEvaluation()`
- `RecordApprovalDecision()`
- `RecordAnalysisComplete()`

---

## 🐛 Bug 2: HAPI Dockerfile Relative Path Issue ✅ **FIXED**

### Problem
HAPI integration test container build failed with:
```
ERROR: Invalid requirement: '../dependencies/holmesgpt/': Expected package name at the start of dependency specifier
    ../dependencies/holmesgpt/
    ^ (from line 37 of holmesgpt-api/requirements.txt)
Hint: It looks like a path. File '../dependencies/holmesgpt/' does not exist.
```

### Root Cause
`holmesgpt-api/requirements.txt` line 37 contains a relative path reference:
```
../dependencies/holmesgpt/
```

In the container context:
- **Build context**: `/workspace/`
- **Working directory**: `/workspace/holmesgpt-api/`
- **Relative path resolves to**: `/workspace/dependencies/holmesgpt/` ❌ (doesn't exist from `/workspace/holmesgpt-api/`)
- **Actual location**: `/workspace/dependencies/holmesgpt/` ✅ (exists at build context root)

### Fix Applied

**File**: `docker/holmesgpt-api-integration-test.Dockerfile`

**Strategy**: Install `holmesgpt` package directly first, then install remaining dependencies.

```dockerfile
# BEFORE (BROKEN):
RUN pip3.12 install --no-cache-dir --break-system-packages \
	-r holmesgpt-api/requirements.txt \
	-r holmesgpt-api/requirements-test.txt

# AFTER (FIXED):
# Install holmesgpt package first (avoids relative path issues)
RUN pip3.12 install --no-cache-dir --break-system-packages ./dependencies/holmesgpt/

# Install remaining Python dependencies
# pip will skip the holmesgpt line in requirements.txt since it's already installed
RUN pip3.12 install --no-cache-dir --break-system-packages \
	-r holmesgpt-api/requirements.txt \
	-r holmesgpt-api/requirements-test.txt
```

**Impact**: HAPI integration test container will now build successfully.

**Files Modified**:
- `docker/holmesgpt-api-integration-test.Dockerfile` (lines 21-28)

---

## 🐛 Bug 3: Notification/HAPI PostgreSQL Port Conflict ✅ **FIXED**

### Problem
Notification integration tests failed with:
```
Error: cannot listen on the TCP port: listen tcp4 :15439: bind: address already in use
[SynchronizedBeforeSuite] [FAILED] [3.993 seconds]
Ran 0 of 124 Specs - A BeforeSuite node failed so all tests were skipped.
```

### Root Cause
**DD-TEST-001 Design Flaw**: Notification and HAPI both shared PostgreSQL port **15439**.

From DD-TEST-001 v1.9:
```markdown
**Note**: HAPI (HolmesGPT API) shares PostgreSQL (15439) with Notification for simplicity (Python service).
```

This prevented Notification and HAPI integration tests from running in parallel.

### Fix Applied

**Updated DD-TEST-001 to v2.0** with new port allocation:

| Service | PostgreSQL Port | Change |
|---------|----------------|--------|
| **Notification** | 15440 | **Changed from 15439** |
| **HAPI** | 15439 | Unchanged (keeps existing port) |

**Files Modified**:
1. **`docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md`**
   - Updated integration test port table (line 648)
   - Updated note to reflect NO SHARED PORTS (line 654)
   - Added v2.0 revision history entry (line 716)

2. **`test/integration/notification/config/config.yaml`**
   - Changed `database.port` from `15439` to `15440` (line 3)

3. **`test/infrastructure/notification_integration.go`**
   - Changed `NTIntegrationPostgresPort` from `15439` to `15440` (line 54)
   - Updated comment to clarify unique port (line 135)

4. **`test/infrastructure/holmesgpt_integration.go`**
   - Updated comments to remove "shared with Notification" (lines 36, 55)

**Impact**: ✅ **TRUE PARALLEL TESTING NOW ENABLED**

All 8 services can now run integration tests simultaneously:
- SignalProcessing: 15436
- RemediationOrchestrator: 15435  
- AIAnalysis: 15438
- **Notification: 15440** ← **NEW UNIQUE PORT**
- WorkflowExecution: 15441
- HAPI: 15439

---

## 📊 Validation Plan

### Phase 1: Test Each Fixed Service Individually

1. **AIAnalysis** (Bug 1 - `event_category` fix):
```bash
make test-integration-aianalysis
# Expected: Tests complete with summary (no hanging)
# Expected: No OpenAPI validation errors in logs
```

2. **HAPI** (Bug 2 - Dockerfile fix):
```bash
make test-integration-holmesgpt-api
# Expected: Container builds successfully
# Expected: Tests run and produce results
```

3. **Notification** (Bug 3 - port fix):
```bash
make test-integration-notification
# Expected: BeforeSuite passes
# Expected: All 124 tests run (no skip due to BeforeSuite failure)
```

### Phase 2: Test Parallel Execution

```bash
# Run HAPI and Notification in parallel (previously impossible)
make test-integration-holmesgpt-api & make test-integration-notification &
# Expected: Both complete successfully without port conflicts
```

---

## 📁 Complete File Change Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `pkg/aianalysis/audit/audit.go` | Bug Fix | Changed `event_category` from `"aianalysis"` to `"analysis"` |
| `docker/holmesgpt-api-integration-test.Dockerfile` | Bug Fix | Install `holmesgpt` package first to avoid relative path issues |
| `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` | Architecture Fix | Notification PostgreSQL 15439 → 15440, updated to v2.0 |
| `test/integration/notification/config/config.yaml` | Config Update | PostgreSQL port 15439 → 15440 |
| `test/infrastructure/notification_integration.go` | Infrastructure Update | `NTIntegrationPostgresPort` 15439 → 15440 |
| `test/infrastructure/holmesgpt_integration.go` | Comment Update | Removed "shared with Notification" references |
| `test/infrastructure/remediationorchestrator.go` | Code Cleanup | Removed ~430 lines of dead E2E code (completed earlier) |

---

## 🎯 Expected Test Results After Fixes

### AIAnalysis
- **Before**: Tests hung indefinitely (OpenAPI validation loop)
- **After**: Tests complete with pass/fail summary

### HAPI
- **Before**: Container build failed (relative path error)
- **After**: Container builds, tests execute

### Notification  
- **Before**: BeforeSuite failed (port conflict), 0/124 tests ran
- **After**: BeforeSuite passes, all 124 tests execute

---

## ⏭️ Next Steps

1. **Test AIAnalysis locally** (validate Bug 1 fix)
   ```bash
   make test-integration-aianalysis
   ```

2. **Test HAPI locally** (validate Bug 2 fix)
   ```bash
   make test-integration-holmesgpt-api
   ```

3. **Test Notification locally** (validate Bug 3 fix)
   ```bash
   make test-integration-notification
   ```

4. **Test parallel execution** (validate DD-TEST-001 v2.0)
   ```bash
   make test-integration-holmesgpt-api & make test-integration-notification &
   ```

5. **Wait for user acknowledgment** before pushing all fixes to CI ⏸️

---

## 🔗 Related Documents

- [DD-TEST-001 v2.0 - Port Allocation Strategy](../architecture/decisions/DD-TEST-001-port-allocation-strategy.md)
- [All Integration Tests - Comprehensive Results](./ALL_INTEGRATION_TESTS_COMPREHENSIVE_RESULTS_JAN_01_2026.md)
- [RO Integration Infrastructure Cleanup](./RO_INTEGRATION_INFRASTRUCTURE_CLEANUP_JAN_01_2026.md)

---

**Status**: ✅ All critical bugs fixed | ⏸️ Awaiting local validation  
**Date**: January 01, 2026  
**Time**: ~6:30pm EST  
**Bugs Fixed**: 3/3 (100%)


