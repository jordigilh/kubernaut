# NOTICE: Integration Test Mock Violations - Dependency Services

**Date**: 2025-12-10
**Version**: 1.0
**From**: Development Team (Comprehensive Mock Triage)
**To**: Gateway, DataStorage, Notification Teams
**Status**: 🟡 **GAP IDENTIFIED** - Mock Violations in Integration Tests
**Priority**: HIGH (Gateway), MEDIUM (DataStorage, Notification)

---

## 📋 Summary

**Issue**: Integration tests are mocking dependency services that should be real. Per authoritative testing strategy (03-testing-strategy.mdc), only the **LLM is allowed to be mocked due to costs**. All other internal dependencies must use real services in integration tests.

**Impact**: Integration tier is not validating real cross-service interactions, undermining defense-in-depth testing strategy.

---

## 🎯 Authoritative Testing Policy

### Allowed Mocks
| Mock Type | Allowed In | Rationale |
|-----------|------------|-----------|
| **LLM (HolmesGPT, Claude)** | ✅ All tiers | Cost constraint |
| **External APIs (Slack, etc.)** | ✅ Integration | External service, not our dependency |
| **Service-under-test httptest wrapper** | ✅ Integration | Testing the service's HTTP handler |

### NOT Allowed Mocks (Integration/E2E)
| Mock Type | Status | Rationale |
|-----------|--------|-----------|
| **Data Storage** | ❌ VIOLATION | Internal dependency - use real |
| **Gateway** | ❌ VIOLATION | Internal dependency - use real |
| **Notification** | ❌ VIOLATION | Internal dependency - use real |
| **Embedding Service** | ⚠️ REVIEW | May be external (like LLM) - needs decision |
| **Any internal service** | ❌ VIOLATION | Integration tests validate real interactions |

---

## 🚨 Violations Found

### 🔴 HIGH Priority: Gateway → Data Storage Mock

**File**: `test/integration/gateway/helpers_postgres.go` (lines 124-147)

**Current Code**:
```go
// TODO: Initialize Data Storage service with PostgreSQL
// This will be implemented when Data Storage service is ready
// For now, create a mock server that accepts audit requests

mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // Mock audit trail endpoint
    if r.URL.Path == "/api/v1/audit/events" {
        w.WriteHeader(http.StatusCreated)
        w.Write([]byte(`{"status":"created","event_id":"test-123"}`))
        return
    }
}))
```

**Issue**: Gateway integration tests mock Data Storage audit endpoint
**Impact**:
- Audit integration NOT validated
- Gateway → Data Storage communication NOT tested
- False confidence in audit functionality

**Required Fix**:
```go
// Use real Data Storage service in KIND cluster
// Deploy Data Storage before Gateway tests
dataStorageURL := os.Getenv("DATA_STORAGE_URL")
if dataStorageURL == "" {
    dataStorageURL = "http://datastorage.kubernaut-system.svc.cluster.local:8080"
}
// Use real Data Storage - NO MOCK
```

**Owner**: Gateway Team
**Priority**: HIGH
**Deadline**: Before V1.0 release

---

### 🟡 MEDIUM Priority: DataStorage → Embedding Service Mock

**File**: `test/integration/datastorage/suite_test.go` (lines 470-499)

**Current Code**:
```go
// BR-STORAGE-014: Create mock embedding service for integration tests
embeddingServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // Mock 768-dimensional embedding response
    mockEmbedding := make([]float32, 768)
    // ...
}))
```

**Issue**: DataStorage integration tests mock embedding service
**Impact**: Embedding generation NOT validated with real service

**Decision Required**:
- Is embedding service considered "external" like LLM? (costs, infrastructure)
- If YES: Mock is acceptable (document decision)
- If NO: Use real embedding service in KIND cluster

**Owner**: DataStorage Team
**Priority**: MEDIUM (needs team decision)

---

### 🟡 MEDIUM Priority: Notification → Audit Store Mock

**File**: `test/integration/notification/suite_test.go` (lines 171-173)

**Current Code**:
```go
// Create mock audit store for testing audit emission
// This captures audit events emitted by the controller during reconciliation
testAuditStore = NewTestAuditStore()
```

**Issue**: Uses test audit store instead of real Data Storage
**Impact**: Audit emission to real Data Storage NOT validated at integration tier

**Decision Required**:
- Is this acceptable for test isolation? (capturing events for assertion)
- Should E2E tier validate real Data Storage audit?
- If E2E validates: Mock is acceptable in integration
- If not: Use real Data Storage in integration

**Owner**: Notification Team
**Priority**: MEDIUM (needs team decision)

---

## ✅ Acceptable Mocks (No Action Required)

| Service | Mock | Reason | Status |
|---------|------|--------|--------|
| AIAnalysis | `MockHolmesGPTClient` | LLM cost constraint | ✅ Approved |
| Notification | `mockSlackServer` | External API (Slack) | ✅ Approved |
| Toolset | `DeployMockService()` | Mock K8s workloads for discovery | ✅ Approved |
| Any | `httptest.NewServer(handler)` | Testing service's own handler | ✅ Approved |

---

## 📊 E2E Tests Status

| Service | Mock Dependencies | Status |
|---------|-------------------|--------|
| AIAnalysis | LLM only | ✅ CLEAN |
| DataStorage | None | ✅ CLEAN |
| Gateway | None | ✅ CLEAN |
| Notification | None | ✅ CLEAN |
| Toolset | Mock K8s workloads (acceptable) | ✅ CLEAN |

---

## 🔧 Implementation Guidance

### For Gateway Team (HIGH Priority)

**Step 1**: Update `test/integration/gateway/helpers_postgres.go`

```go
// SetupDataStorageTestServer - Use REAL Data Storage service
func SetupDataStorageTestServer(ctx context.Context) *DataStorageTestServer {
    // Real Data Storage URL (deployed in KIND cluster before tests)
    dataStorageURL := os.Getenv("DATA_STORAGE_URL")
    if dataStorageURL == "" {
        // Default for KIND cluster
        dataStorageURL = "http://datastorage:8080"
    }

    // Verify Data Storage is available
    healthURL := fmt.Sprintf("%s/healthz", dataStorageURL)
    resp, err := http.Get(healthURL)
    if err != nil || resp.StatusCode != http.StatusOK {
        Skip("Data Storage service not available - skipping integration test")
    }

    return &DataStorageTestServer{
        URL: dataStorageURL,
    }
}
```

**Step 2**: Update test setup to deploy Data Storage before Gateway tests

**Step 3**: Remove mock server code

---

### For DataStorage Team (Decision Required)

**Question**: Is embedding service "external" (like LLM)?

| If External (like LLM) | If Internal |
|------------------------|-------------|
| Mock is acceptable | Use real embedding service |
| Document in ADR | Deploy embedding service in KIND |
| No changes needed | Update suite_test.go |

---

### For Notification Team (Decision Required)

**Question**: Should audit store be real at integration tier?

| If E2E validates audit | If integration must validate |
|------------------------|------------------------------|
| Keep test audit store | Use real Data Storage |
| Acceptable for isolation | Update suite_test.go |
| Document decision | Deploy Data Storage before tests |

---

## 📋 Response Template

Please respond with decisions using this template:

```markdown
## [Team Name] Response

### Decision
- [ ] Will fix (implement real dependency)
- [ ] Acceptable (document rationale)
- [ ] Needs discussion

### Rationale
[Explanation of decision]

### Timeline (if fixing)
- [ ] Before V1.0
- [ ] V1.1
- [ ] Other: ___

### Owner
[Name/Team]
```

---

## 🔗 Related Documentation

| Document | Purpose |
|----------|---------|
| [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc) | Authoritative testing policy |
| [15-testing-coverage-standards.mdc](../../.cursor/rules/15-testing-coverage-standards.mdc) | Coverage enforcement |
| [REQUEST_SHARED_E2E_MIGRATION_LIBRARY.md](./REQUEST_SHARED_E2E_MIGRATION_LIBRARY.md) | E2E test infrastructure |

---

## 📊 Confidence Assessment

**Confidence**: 90%

**Justification**:
- Clear policy violation in Gateway tests (TODO comment acknowledges gap)
- DataStorage and Notification need team decisions (may be acceptable)
- E2E tests are clean

**Remaining Risk**: 10%
- Team decisions may reveal acceptable reasons for current approach

---

**Document Status**: 🟡 **AWAITING TEAM RESPONSES**
**Created**: 2025-12-10
**Response Requested By**: 2025-12-13

---

## 📬 Team Acknowledgments

| Team | Status | Response Date |
|------|--------|---------------|
| Gateway | ⏳ Pending | - |
| DataStorage | ⏳ Pending | - |
| Notification | ✅ [RESPONSE](./RESPONSE_NOTIFICATION_INTEGRATION_MOCK_VIOLATIONS.md) | 2025-12-10 |

### Notification Team Response Summary

**Decision**: ✅ **ACCEPTABLE WITH CLARIFICATION**

**Rationale**: The `TestAuditStore` in `suite_test.go` is used for **Layer 4** testing (controller behavior verification), not Data Storage integration. Real Data Storage is tested in:
- **Layer 3**: `audit_integration_test.go` (real PostgreSQL + real Data Storage)
- **Layer 5**: E2E tests (Kind cluster with `DeployNotificationAuditInfrastructure()`)

**Action Taken**: Fixed Skip() violations in `audit_integration_test.go` - replaced with `Fail()` per TESTING_GUIDELINES.md.


