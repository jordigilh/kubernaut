# HTTP Anti-Pattern Triage - Integration Tests

**Date**: January 10, 2026
**Author**: AI Assistant
**Status**: Triage Complete
**Priority**: HIGH - Affects 1 service severely (Gateway)

---

## 📋 Executive Summary

Systematic triage of all service integration tests identified **Gateway as the primary violator** of the HTTP anti-pattern (20/23 tests = 87% violation rate). Other services are clean or have legitimate HTTP usage.

**Key Findings**:
- ✅ **4 services CLEAN**: AIAnalysis, RemediationOrchestrator, WorkflowExecution, HolmesGPT-API
- ⚠️ **3 services with HTTP ANTI-PATTERN**: Gateway (20 tests), SignalProcessing (1 test), Notification (2 tests)
- 📊 **Total violations**: 23 tests need action (20 Gateway + 1 SignalProcessing + 2 Notification)

---

## 🎯 Triage Results by Service

### 📊 Summary Table

| Service | Total Tests | HTTP Tests | Violation Rate | Status | Action Required |
|---------|-------------|------------|----------------|--------|-----------------|
| **Gateway** | 23 | **20** | **87%** | ⚠️ SEVERE | REFACTOR or MOVE TO E2E |
| **SignalProcessing** | 7 | 1 | 14% | ⚠️ ANTI-PATTERN | Refactor (query DB) OR move to E2E |
| **Notification** | 22 | 2 | 9% | ⚠️ ANTI-PATTERN | Move to E2E (TLS = infrastructure) |
| **AIAnalysis** | 10 | 0 | 0% | ✅ CLEAN | None |
| **RemediationOrchestrator** | 14 | 0 | 0% | ✅ CLEAN | None |
| **WorkflowExecution** | 12 | 0 | 0% | ✅ CLEAN | None |
| **HolmesGPT-API** | 11 | 0 | 0% | ✅ CLEAN | None |
| **TOTAL** | **99** | **23** | **23%** | - | **23 tests need action** |

---

## ⚠️ SEVERE: Gateway Service

### Overview

**Violation Rate**: 87% (20 out of 23 integration tests)
**Pattern**: All tests use `httptest.Server` + HTTP webhook calls
**Impact**: Integration test suite is actually E2E tests in wrong tier

### Violating Tests (20 files)

1. `adapter_interaction_test.go` - Adapter pipeline testing via HTTP
2. `audit_errors_integration_test.go` - Audit error scenarios via HTTP
3. `audit_integration_test.go` - Audit emission via HTTP
4. `audit_signal_data_integration_test.go` - Signal data audit via HTTP
5. `cors_test.go` - CORS middleware via HTTP
6. `dd_gateway_011_status_deduplication_test.go` - Deduplication via HTTP
7. `deduplication_edge_cases_test.go` - Dedup edge cases via HTTP
8. `deduplication_state_test.go` - Dedup state management via HTTP
9. `error_classification_test.go` - Error handling via HTTP
10. `error_handling_test.go` - Error responses via HTTP
11. `graceful_shutdown_foundation_test.go` - Shutdown behavior via HTTP
12. `health_integration_test.go` - Health endpoints via HTTP
13. `http_server_test.go` - HTTP server infrastructure (LEGITIMATE)
14. `k8s_api_failure_test.go` - K8s API failures via HTTP
15. `k8s_api_integration_test.go` - K8s API operations via HTTP
16. `k8s_api_interaction_test.go` - K8s API interaction via HTTP
17. `observability_test.go` - Metrics emission via HTTP
18. `prometheus_adapter_integration_test.go` - Prometheus adapter via HTTP
19. `service_resilience_test.go` - Resilience patterns via HTTP
20. `webhook_integration_test.go` - Webhook processing via HTTP

### Common Pattern

```go
// ❌ ANTI-PATTERN: All Gateway integration tests follow this pattern
var _ = Describe("Some Business Logic", func() {
    var testServer *httptest.Server  // ← HTTP server

    BeforeEach(func() {
        gatewayServer := gateway.NewServer(...)
        testServer = httptest.NewServer(gatewayServer.Handler())  // ← HTTP
    })

    It("should do something", func() {
        payload := GeneratePrometheusAlert(...)
        resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)  // ← HTTP call
        Expect(resp.StatusCode).To(Equal(201))  // ← HTTP validation

        // ... verify CRD created (this part is correct)
    })
})
```

### Why This is Wrong

1. **E2E Disguised as Integration**: Full HTTP stack = E2E testing
2. **Slow Execution**: HTTP serialization/deserialization overhead
3. **Wrong Focus**: Tests transport layer, not business logic coordination
4. **Duplicate Coverage**: E2E tests already cover these endpoints (93 E2E test files exist)
5. **Maintenance Burden**: Changes to HTTP layer break integration tests

### Recommended Actions

**Option A: Move to E2E** (Fastest, preserves tests as-is)
- Move all 20 HTTP tests to `test/e2e/gateway/`
- Rename with sequential numbering (21-40)
- Update suite to use Kind cluster instead of `httptest.Server`
- **Effort**: 2-4 hours
- **Risk**: Low (no test logic changes)

**Option B: Refactor to Direct Calls** (Correct, but more work)
- Refactor tests to call business logic directly:
  ```go
  // ✅ CORRECT: Direct business logic calls
  signal, err := adapter.Transform(payload)
  isDupe, fingerprint, err := dedupService.CheckDuplicate(ctx, signal)
  crd, err := crdManager.CreateRemediationRequest(ctx, signal, fingerprint)
  ```
- Keep 3-5 key integration tests, move rest to E2E
- **Effort**: 8-16 hours
- **Risk**: Medium (test logic changes, may find bugs)

**Option C: Hybrid Approach** (Recommended)
- **Move 15 tests to E2E** (audit, error handling, observability, etc.)
- **Refactor 5 core tests** (adapter integration, dedup, K8s API interaction)
- **Keep 3 as-is** (http_server_test, health, cors - infrastructure tests)
- **Effort**: 4-8 hours
- **Risk**: Low-Medium

### Exception: http_server_test.go

**Status**: ✅ LEGITIMATE (HTTP infrastructure test)

This test validates HTTP server infrastructure (timeouts, limits, server lifecycle), which IS the business logic being tested. Should remain in integration tier per exception clause in guidelines.

---

## ⚠️ ANTI-PATTERN: SignalProcessing Service

### Overview

**Violation Rate**: 14% (1 out of 7 integration tests)
**Status**: ⚠️ ANTI-PATTERN - HTTP for audit verification

### Test: `audit_integration_test.go`

**What it does**:
```go
// Creates SignalProcessing CRD
sp := &signalprocessingv1alpha1.SignalProcessing{...}
k8sClient.Create(ctx, sp)

// Waits for controller to process (business logic)
Eventually(func() string {
    k8sClient.Get(ctx, key, &sp)
    return sp.Status.Phase
}).Should(Equal("Completed"))

// ❌ ANTI-PATTERN: Verifies audit via HTTP query
dsClient, _ := ogenclient.NewClient(dataStorageURL)  // ← HTTP client
resp, _ := dsClient.QueryAuditEvents(ctx, params)    // ← HTTP query
testutil.ValidateAuditEvent(resp.Data[0], ...)
```

**Why this is an anti-pattern**:
1. ❌ **Uses HTTP in Integration Test**: Integration tests should NOT use HTTP
2. ❌ **Workaround for Missing DB Access**: Should have PostgreSQL access like DataStorage tests
3. ❌ **"Exception" is Rationalization**: "It's just for side effects" is weak reasoning

**The Correct Approaches**:

**Option A: Query PostgreSQL Directly** (Recommended)
```go
// ✅ CORRECT: Integration tests SHOULD have DB access
var testDB *sql.DB

BeforeEach(func() {
    testDB, _ = sql.Open("pgx", postgresURL)  // Same DB DataStorage uses
})

It("should emit audit event", func() {
    // Create CRD, wait for completion...

    // ✅ Query PostgreSQL DIRECTLY (no HTTP)
    Eventually(func() int {
        var count int
        testDB.QueryRow(`
            SELECT COUNT(*)
            FROM audit_events
            WHERE correlation_id = $1
              AND event_type = 'signalprocessing.signal.processed'
        `, correlationID).Scan(&count)
        return count
    }, 10*time.Second, 1*time.Second).Should(Equal(1))

    // Validate audit event fields
    var event AuditEvent
    testDB.QueryRow(`
        SELECT event_data
        FROM audit_events
        WHERE correlation_id = $1
    `, correlationID).Scan(&event.EventData)

    Expect(event.EventData["signal_name"]).To(Equal("test-signal"))
})
```

**Option B: Move to E2E**
- If HTTP query is needed, test belongs in E2E tier
- E2E tests validate full stack: Controller → Audit Client → DataStorage HTTP → PostgreSQL
- Simpler than refactoring, but less focused than integration test

**Recommendation**: ⚠️ **Option A: Refactor to query PostgreSQL directly** (Better integration test) OR **Option B: Move to E2E** (Simpler)

---

## ⚠️ ANTI-PATTERN: Notification Service

### Overview

**Violation Rate**: 9% (2 out of 22 integration tests)
**Status**: ⚠️ ANTI-PATTERN - TLS testing in wrong tier

### Tests

1. **`slack_tls_integration_test.go`** - TLS certificate validation
2. **`tls_failure_scenarios_test.go`** - TLS handshake failures

**What they do**:
```go
// ❌ ANTI-PATTERN: Creates full HTTP/TLS stack in integration test
tlsServer := httptest.NewTLSServer(...)

// Tests TLS certificate validation behavior
client := &http.Client{
    Transport: &http.Transport{
        TLSClientConfig: &tls.Config{...},
    },
}

// Verifies TLS handshake succeeds/fails correctly
resp, err := client.Post(tlsServer.URL, ...)
```

**Why this is an anti-pattern**:
1. ❌ **TLS = Infrastructure**: Infrastructure validation belongs in E2E tier
2. ❌ **Requires Full HTTP/TLS Stack**: That's literally the definition of E2E (full stack)
3. ❌ **"Exception" is Rationalization**: "TLS IS the business logic" is weak reasoning
4. ❌ **Wrong Tier**: Integration tests should test component coordination, not infrastructure

**The Correct Approach**:

**Move to E2E Tier**
```bash
# Move TLS tests to E2E
git mv test/integration/notification/slack_tls_integration_test.go \
       test/e2e/notification/XX_slack_tls_test.go

git mv test/integration/notification/tls_failure_scenarios_test.go \
       test/e2e/notification/YY_tls_failure_scenarios_test.go
```

**Why E2E is Correct**:
- ✅ E2E tests validate full infrastructure (including TLS)
- ✅ E2E tests can test certificate validation just as well
- ✅ TLS handshake requires real HTTP stack (E2E scope)
- ✅ Separates business logic tests (integration) from infrastructure tests (E2E)

**Recommendation**: ⚠️ **Move to E2E tier** (TLS infrastructure = E2E scope)

---

## ✅ CLEAN: 4 Services

### AIAnalysis (10 tests, 0 HTTP)

**Status**: ✅ CLEAN
**Pattern**: All tests use direct K8s API calls + envtest
**Example**: `test/integration/aianalysis/reconciliation_integration_test.go`

```go
// ✅ CORRECT: Direct K8s API, no HTTP
aianalysis := &aianalysisv1alpha1.AIAnalysis{...}
k8sClient.Create(ctx, aianalysis)

Eventually(func() string {
    k8sClient.Get(ctx, key, &aianalysis)
    return aianalysis.Status.Phase
}).Should(Equal("Completed"))

// Verifies audit via DataStorage HTTP (legitimate side effect check)
```

### RemediationOrchestrator (14 tests, 0 HTTP)

**Status**: ✅ CLEAN
**Pattern**: All tests use direct K8s API calls + envtest
**Example**: `test/integration/remediationorchestrator/reconciliation_integration_test.go`

```go
// ✅ CORRECT: Direct K8s API, no HTTP
rr := &remediationv1alpha1.RemediationRequest{...}
k8sClient.Create(ctx, rr)

Eventually(func() string {
    k8sClient.Get(ctx, key, &rr)
    return rr.Status.Phase
}).Should(Equal("WorkflowExecutionCreated"))
```

### WorkflowExecution (12 tests, 0 HTTP)

**Status**: ✅ CLEAN
**Pattern**: All tests use direct K8s API calls + envtest
**Example**: `test/integration/workflowexecution/reconciliation_integration_test.go`

```go
// ✅ CORRECT: Direct K8s API, no HTTP
we := &workflowv1alpha1.WorkflowExecution{...}
k8sClient.Create(ctx, we)

Eventually(func() string {
    k8sClient.Get(ctx, key, &we)
    return we.Status.Phase
}).Should(Equal("Completed"))
```

### HolmesGPT-API (11 tests, 0 HTTP - Python)

**Status**: ✅ CLEAN
**Pattern**: All tests use direct service calls, no FastAPI TestClient
**Example**: `holmesgpt-api/tests/integration/test_audit_store.py`

```python
# ✅ CORRECT: Direct business logic calls
audit_store = BufferedStore(...)
await audit_store.store_event(event)

# Verifies PostgreSQL directly (no HTTP)
result = await db.fetch_one("SELECT * FROM audit_events WHERE ...")
assert result is not None
```

---

## 📊 Impact Analysis

### Test Distribution

```
Total Integration Tests: 99
├─ ✅ CLEAN: 76 tests (77%)
│  ├─ AIAnalysis: 10
│  ├─ RemediationOrchestrator: 14
│  ├─ WorkflowExecution: 12
│  ├─ HolmesGPT-API: 11
│  ├─ SignalProcessing: 6 (non-HTTP)
│  ├─ Notification: 20 (non-HTTP)
│  └─ Gateway: 3 (non-HTTP)
│
└─ ⚠️ ANTI-PATTERN: 23 tests (23%)
   ├─ Gateway: 20 (HTTP testing in wrong tier)
   ├─ SignalProcessing: 1 (HTTP for audit verification)
   └─ Notification: 2 (TLS infrastructure)
```

### Refactoring Effort Estimate

| Action | Tests Affected | Estimated Hours | Priority |
|--------|---------------|-----------------|----------|
| **Gateway - Option C (Hybrid)** | 20 | 4-8 hours | HIGH |
| **SignalProcessing - Refactor to Query DB** | 1 | 1 hour | MEDIUM |
| **SignalProcessing - Move to E2E** | 1 | 30 min | MEDIUM |
| **Notification - Move to E2E** | 2 | 30 min | LOW |
| **TOTAL (All Services)** | 23 | 6-10 hours | - |

**Recommended Path**:
1. Gateway Option C (Hybrid) - 4-8 hours
2. SignalProcessing - Refactor to query PostgreSQL - 1 hour
3. Notification - Move to E2E - 30 min

---

## 🎯 Recommendations

### Immediate Actions (This Sprint)

1. **Gateway Service**: Execute Option C (Hybrid) - 4-8 hours
   - Week 1: Move 15 HTTP tests to E2E (2 hours)
   - Week 1: Refactor 5 core tests to direct calls (4 hours)
   - Week 1: Verify 3 infrastructure tests remain (30 min)
   - Week 1: Run full test suite to validate (30 min)

2. **SignalProcessing Service**: Refactor audit test - 1 hour
   - Add PostgreSQL connection to integration suite
   - Replace HTTP audit queries with direct DB queries
   - Update `audit_integration_test.go` to use `testDB.QueryRow(...)`

3. **Notification Service**: Move TLS tests to E2E - 30 min
   - Move `slack_tls_integration_test.go` → `test/e2e/notification/`
   - Move `tls_failure_scenarios_test.go` → `test/e2e/notification/`
   - Renumber E2E tests sequentially

2. **Documentation**: ✅ COMPLETE
   - Added comprehensive anti-pattern section to `TESTING_GUIDELINES.md`
   - Includes Gateway as example
   - Includes refactoring guidance

### Long-Term Actions

1. **CI Enforcement**: Add linter check for HTTP in integration tests
   ```bash
   # Detect HTTP anti-pattern (warn, not block)
   if grep -r "httptest\|http\.Post\|http\.Get" test/integration --include="*_test.go" | \
      grep -v "http_server_test\|tls.*test\|audit_integration_test" | \
      grep -v "^Binary"; then
       echo "⚠️  WARNING: HTTP detected in integration tests"
       echo "   See: TESTING_GUIDELINES.md#anti-pattern-http-testing"
   fi
   ```

2. **Team Training**: Share findings in team meeting
   - Show Gateway example
   - Explain integration vs E2E distinction
   - Demo correct pattern (DataStorage refactoring)

3. **Future Reviews**: Add to PR checklist
   - [ ] Integration tests use direct business logic calls (no HTTP)
   - [ ] HTTP tests are in E2E or performance tier
   - [ ] Exceptions (TLS, audit verification) are documented

---

## 📚 References

**Documentation**:
- `docs/development/business-requirements/TESTING_GUIDELINES.md` (Section: HTTP Anti-Pattern)
- `docs/handoff/DS_E2E_MIGRATION_COMPLETE_JAN10_2026.md` (DataStorage refactoring example)

**Success Stories**:
- **DataStorage**: Refactored 12 HTTP tests → 9 moved to E2E, 1 moved to performance, 2 to direct calls
- **SignalProcessing**: Clean from start (audit verification is legitimate)
- **AIAnalysis**: Clean from start (direct K8s API calls)

**Anti-Pattern Examples**:
- **Gateway**: 20 tests using `httptest.Server` (87% violation rate)

---

## ✅ Triage Complete

**Date**: January 10, 2026
**Total Services Analyzed**: 7
**Total Tests Analyzed**: 99
**Violations Found**: 23 (Gateway: 20, SignalProcessing: 1, Notification: 2)
**Estimated Fix Time**: 6-10 hours total

**Next Steps**:
1. ✅ Documentation updated (`TESTING_GUIDELINES.md`)
2. ⏳ Gateway refactoring (Option C - 4-8 hours)
3. ⏳ SignalProcessing refactoring (query DB directly - 1 hour)
4. ⏳ Notification migration (move to E2E - 30 min)
5. ⏳ CI enforcement (optional, low priority)
