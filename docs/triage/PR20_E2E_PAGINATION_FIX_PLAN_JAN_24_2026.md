# PR#20 E2E Pagination Fix Plan - Jan 24, 2026

## 🚨 **Issue Summary**

**Discovered**: 2026-01-24 (PR#20 Run 3/3)
**Root Cause**: E2E tests querying audit events without pagination loops
**Impact**: 13 E2E test files across 7 services
**Symptom**: Tests pass under low load, fail under high load (flaky)

---

## 📊 **Affected Files**

### **By Service**

| Service | Files | Query Helper Pattern |
|---------|-------|---------------------|
| **Notification** | 2 | `queryAuditEvents(dsClient, correlationID)` |
| **AI Analysis** | 1 | `queryAuditEvents(correlationID, eventType)` |
| **Remediation Orchestrator** | 2 | Direct `dsClient.QueryAuditEvents()` calls |
| **Workflow Execution** | 1 | Direct `dsClient.QueryAuditEvents()` calls |
| **Gateway** | 3 | Direct `dsClient.QueryAuditEvents()` calls |
| **Signal Processing** | 1 | `queryAuditEvents(correlationID)` |
| **Data Storage** | 3 | Direct `dsClient.QueryAuditEvents()` calls |

### **Complete File List**

1. `test/e2e/notification/01_notification_lifecycle_audit_test.go`
2. `test/e2e/notification/02_audit_correlation_test.go`
3. `test/e2e/aianalysis/05_audit_trail_test.go`
4. `test/e2e/remediationorchestrator/audit_wiring_e2e_test.go`
5. `test/e2e/remediationorchestrator/gap8_webhook_test.go`
6. `test/e2e/workflowexecution/02_observability_test.go`
7. `test/e2e/gateway/23_audit_emission_test.go`
8. `test/e2e/gateway/24_audit_signal_data_test.go`
9. `test/e2e/gateway/15_audit_trace_validation_test.go`
10. `test/e2e/signalprocessing/business_requirements_test.go`
11. `test/e2e/datastorage/01_happy_path_test.go`
12. `test/e2e/datastorage/22_audit_validation_helper_test.go`
13. `test/e2e/datastorage/03_query_api_timeline_test.go`

---

## ✅ **Fix Strategy**

### **Standard Pagination Pattern**

```go
// queryAllAuditEvents fetches ALL audit events across ALL pages.
// MANDATORY for concurrent test execution (prevents missing events beyond first page).
func queryAllAuditEvents(params ogenclient.QueryAuditEventsParams) ([]ogenclient.AuditEvent, error) {
	var allEvents []ogenclient.AuditEvent
	offset := 0
	limit := 100

	for {
		params.Limit = ogenclient.NewOptInt(limit)
		params.Offset = ogenclient.NewOptInt(offset)

		resp, err := dsClient.QueryAuditEvents(context.Background(), params)
		if err != nil {
			return nil, err
		}

		if resp.Data == nil || len(resp.Data) == 0 {
			break
		}

		allEvents = append(allEvents, resp.Data...)

		if len(resp.Data) < limit {
			break
		}

		offset += limit
	}

	return allEvents, nil
}
```

### **Implementation Approach**

**Option A: Per-File Fix** (RECOMMENDED for E2E)
- Each E2E test file has unique query patterns
- Add pagination loop to existing `queryAuditEvents` helpers
- Preserve existing function signatures for minimal disruption
- Test each service independently

**Option B: Shared Helper** (Better for long-term)
- Create `test/e2e/shared/audit_helpers.go` with pagination helper
- Refactor all E2E tests to use shared helper
- More consistent, but higher risk of breaking tests

**Decision**: Use **Option A** for PR#20 to minimize risk and enable tier-by-tier validation.

---

## 📋 **Execution Plan**

### **Phase 1: Fix E2E Tests (Tier by Tier)**

**Tier 1: Data Storage** (Foundation)
- Files: `01_happy_path_test.go`, `22_audit_validation_helper_test.go`, `03_query_api_timeline_test.go`
- Run: `make test-e2e-datastorage`
- Validate: 3/3 passes

**Tier 2: Gateway** (Signal Ingestion)
- Files: `23_audit_emission_test.go`, `24_audit_signal_data_test.go`, `15_audit_trace_validation_test.go`
- Run: `make test-e2e-gateway`
- Validate: 3/3 passes

**Tier 3: AI Analysis** (RCA Generation)
- Files: `05_audit_trail_test.go`
- Run: `make test-e2e-aianalysis`
- Validate: 3/3 passes

**Tier 4: Remediation Orchestrator** (Orchestration)
- Files: `audit_wiring_e2e_test.go`, `gap8_webhook_test.go`
- Run: `make test-e2e-remediationorchestrator`
- Validate: 3/3 passes

**Tier 5: Workflow Execution** (Execution)
- Files: `02_observability_test.go`
- Run: `make test-e2e-workflowexecution`
- Validate: 3/3 passes

**Tier 6: Notification** (Notification Delivery)
- Files: `01_notification_lifecycle_audit_test.go`, `02_audit_correlation_test.go`
- Run: `make test-e2e-notification`
- Validate: 3/3 passes

**Tier 7: Signal Processing** (Signal Processing)
- Files: `business_requirements_test.go`
- Run: `make test-e2e-signalprocessing`
- Validate: 3/3 passes

### **Phase 2: Integration Tests Validation**

**Already Fixed** (PR#20 earlier commits):
- Notification: ✅ Fixed
- Remediation Orchestrator: ✅ Fixed
- AI Analysis: ✅ Fixed
- Gateway: ✅ Fixed

**Pending Validation**:
- Data Storage: Run `make test-integration-datastorage` (3/3 passes)

### **Phase 3: Commit and Push**

```bash
# Commit all fixes together
git add test/e2e/
git add docs/testing/AUDIT_QUERY_PAGINATION_STANDARDS.md
git add docs/triage/PR20_E2E_PAGINATION_FIX_PLAN_JAN_24_2026.md
git commit -m "fix(e2e): Add pagination to all audit query helpers

- Fix pagination bug in 13 E2E test files across 7 services
- Tests were only querying first page (50-100 events), causing flaky failures under high load
- Add pagination loops to all queryAuditEvents helpers
- Validate each tier passes 3/3 times locally

Fixes: #PR20
Related: docs/testing/AUDIT_QUERY_PAGINATION_STANDARDS.md"

git push origin HEAD
```

---

## 🎯 **Success Criteria**

- ✅ All 13 E2E test files have pagination loops
- ✅ Each tier passes 3/3 times under high load (`-p 12`)
- ✅ Data Storage integration tests pass 3/3 times
- ✅ CI pipeline passes all tests
- ✅ No new flaky test failures in subsequent runs

---

## 📚 **Reference Documentation**

- **Authoritative Standard**: `docs/testing/AUDIT_QUERY_PAGINATION_STANDARDS.md`
- **Integration Test Fixes**: `docs/triage/PR20_AUDIT_QUERY_PAGINATION_BUG_ALL_SERVICES_JAN_24_2026.md`
- **Notification Audit Fix**: `docs/triage/PR20_NT_AUDIT_EMISSION_FLAKY_TESTS_JAN_24_2026.md`

---

## ⏱️ **Estimated Timeline**

- **Phase 1 (E2E Fixes)**: 2-3 hours (13 files × 10-15 min each)
- **Phase 2 (Integration Validation)**: 30 minutes (DS integration tests)
- **Phase 3 (Commit/Push)**: 10 minutes
- **Total**: ~3-4 hours

---

## 🚨 **Risk Assessment**

**Low Risk**:
- Pagination pattern is proven (already fixed 11 integration tests)
- Each tier validated independently before moving to next
- Backup files created (`.pagination-backup`)

**Mitigation**:
- Test each tier 3/3 times before proceeding
- If any tier fails, investigate before continuing
- Can rollback individual files if needed

---

**Status**: 📋 READY FOR EXECUTION
**Priority**: P0 (Blocking CI pipeline)
**Owner**: Platform Team
