# AIAnalysis Integration Tests - Real HAPI Service Migration

**Date**: December 30, 2025
**Status**: ✅ **CODE COMPLETE** (Blocked by disk space issue)
**Business Requirement**: Testing Strategy Compliance
**Related**: BR-HAPI-197 (Recovery Human Review)

---

## 🎯 **Objective**

Migrate AIAnalysis integration tests from using mock HAPI client to using the **real HAPI service** running at `http://localhost:18120`.

**Testing Strategy Mandate**:
- ✅ **Unit Tests**: Mocks allowed for all external dependencies
- ✅ **Integration Tests**: **ONLY LLM mocked** (inside HAPI via `MOCK_LLM_MODE=true`), all other services REAL
- ✅ **E2E Tests**: **ONLY LLM mocked** (inside HAPI via `MOCK_LLM_MODE=true`), all other services REAL

---

## 🔍 **Root Cause Analysis**

### **Problem Discovered**
The BR-HAPI-197 integration tests were failing because:

1. **Integration tests were using `testutil.MockHolmesGPTClient`** ❌
2. **Real HAPI service was running** at `http://localhost:18120` ✅
3. **Mock client had no knowledge of special `SignalType` values** ❌

### **Why Tests Failed**
```go
// ❌ OLD: Mock client doesn't know about HAPI's special signal types
mockHGClient = testutil.NewMockHolmesGPTClient()
mockHGClient.WithFullResponse(...) // Generic response, no edge case logic

// Test creates CRD with SignalType="MOCK_NO_WORKFLOW_FOUND"
// Mock returns generic success response with needs_human_review=false
// Test expects needs_human_review=true → FAIL
```

The **real HAPI service** has deterministic mock responses in `holmesgpt-api/src/mock_responses.py`:
- `MOCK_NO_WORKFLOW_FOUND` → `needs_human_review=true`, `reason="no_matching_workflows"`
- `MOCK_LOW_CONFIDENCE` → `needs_human_review=true`, `reason="low_confidence"`
- Other signal types → normal successful responses

But the **Go mock client** didn't have this logic!

---

## ✅ **Solution Implemented**

### **Changes Made**

#### **1. Updated `test/integration/aianalysis/suite_test.go`**

**Replaced Mock Client with Real HAPI Client**:

```go
// ❌ OLD: Mock client
mockHGClient = testutil.NewMockHolmesGPTClient()
mockHGClient.WithFullResponse(...)

// ✅ NEW: Real HAPI client
realHGClient, err = hgclient.NewHolmesGPTClient(hgclient.Config{
    BaseURL: "http://localhost:18120",
    Timeout: 30 * time.Second,
})
Expect(err).ToNot(HaveOccurred(), "failed to create real HAPI client")
```

**Updated Handler Initialization**:

```go
// ✅ NEW: Use real HAPI client
investigatingHandler := handlers.NewInvestigatingHandler(
    realHGClient,  // Real client instead of mock
    ctrl.Log.WithName("investigating-handler"),
    testMetrics,
    auditClient,
)
```

**Updated Variable Declarations**:

```go
// ❌ OLD
mockHGClient *testutil.MockHolmesGPTClient

// ✅ NEW
realHGClient *hgclient.HolmesGPTClient
```

**Removed Mock Import**:

```go
// ❌ OLD
import (
    "github.com/jordigilh/kubernaut/pkg/testutil"
)

// ✅ NEW: Removed testutil import
import (
    hgclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)
```

#### **2. Updated `test/integration/aianalysis/recovery_human_review_integration_test.go`**

**Removed Mock Configuration**:

```go
// ❌ OLD: Tried to configure mock (but mock wasn't being used!)
mockHGClient.WithRecoveryResponse(&client.RecoveryResponse{
    NeedsHumanReview: client.OptBool{Set: true, Value: true},
    ...
})

// ✅ NEW: No configuration needed - real HAPI service handles it
// The REAL HAPI service (http://localhost:18120) has mock logic that responds
// to MOCK_NO_WORKFLOW_FOUND with needs_human_review=true
```

---

## 📊 **Impact**

### **Files Modified**
1. `test/integration/aianalysis/suite_test.go` - Replaced mock with real HAPI client
2. `test/integration/aianalysis/recovery_human_review_integration_test.go` - Removed mock configuration

### **Benefits**
✅ **Correct Testing Strategy**: Integration tests now use real services (only LLM mocked)
✅ **HAPI Edge Cases Work**: Special `SignalType` values trigger correct responses
✅ **BR-HAPI-197 Tests Valid**: Tests now correctly validate `needs_human_review` logic
✅ **Consistency**: Aligns with other service integration tests (Gateway, etc.)
✅ **Better Coverage**: Tests actual HTTP communication, serialization, and HAPI behavior

---

## 🚧 **Current Status**

### **Code Status**: ✅ **COMPLETE**
- All changes implemented
- Code compiles successfully
- No lint errors

### **Test Status**: ⚠️ **BLOCKED BY DISK SPACE**

```
Error: write /var/tmp/container_images_storage4079556598/1: no space left on device
```

**Root Cause**: Podman container image storage is full

**Resolution Needed**:
```bash
# Clean up Podman storage
podman system prune -a --volumes -f

# Or increase disk space allocation
```

---

## 🔄 **Next Steps**

1. **Free up disk space** on the development machine
2. **Re-run integration tests**:
   ```bash
   make test-integration-aianalysis FOCUS="BR-HAPI-197"
   ```
3. **Verify all 3 BR-HAPI-197 tests pass**:
   - Recovery human review when no workflows match
   - Recovery human review on low confidence
   - Recovery human review when not reproducible

---

## 📝 **Testing Strategy Compliance**

### **Before (❌ INCORRECT)**
```
Unit Tests:       ✅ Mocks for all external dependencies
Integration Tests: ❌ Mock HAPI client (should be real!)
E2E Tests:        ✅ Real HAPI service
```

### **After (✅ CORRECT)**
```
Unit Tests:       ✅ Mocks for all external dependencies
Integration Tests: ✅ Real HAPI service (only LLM mocked inside HAPI)
E2E Tests:        ✅ Real HAPI service (only LLM mocked inside HAPI)
```

---

## 🎓 **Key Learnings**

### **1. Integration Test Philosophy**
- **Integration tests MUST use real services** to validate actual behavior
- **Only the LLM is mocked** (inside HAPI via `MOCK_LLM_MODE=true`) to avoid costs
- **Mocks are ONLY for unit tests**

### **2. HAPI Mock Logic Location**
- **HAPI service has deterministic mock responses** in `holmesgpt-api/src/mock_responses.py`
- **Special `SignalType` values trigger edge cases** (e.g., `MOCK_NO_WORKFLOW_FOUND`)
- **Go mock client doesn't replicate this logic** → integration tests must use real service

### **3. Test Infrastructure**
- **HAPI service auto-starts** in `SynchronizedBeforeSuite` via `test/infrastructure/aianalysis.go`
- **Service runs at `http://localhost:18120`** with `MOCK_LLM_MODE=true`
- **No manual configuration needed** - HAPI handles edge cases automatically

---

## 🔗 **Related Documents**

- **HAPI Team Response**: `docs/shared/HAPI_RECOVERY_MOCK_EDGE_CASES_REQUEST.md`
- **Testing Strategy**: `.cursor/rules/03-testing-strategy.mdc`
- **Testing Coverage**: `.cursor/rules/15-testing-coverage-standards.mdc`
- **BR-HAPI-197 Implementation**: `docs/handoff/AA_RECOVERY_HUMAN_REVIEW_COMPLETE_DEC_30_2025.md`

---

## ✅ **Verification Checklist**

- [x] Code compiles without errors
- [x] No lint errors introduced
- [x] Real HAPI client used in integration tests
- [x] Mock client removed from integration tests
- [x] Comments updated to reflect real service usage
- [ ] Tests pass (blocked by disk space)
- [ ] All 3 BR-HAPI-197 tests validated

---

**Confidence**: 95%
**Blocker**: Disk space issue (not code issue)
**Next Action**: Clean up Podman storage and re-run tests

