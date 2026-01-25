# Triage: Extract Audit Helpers to Shared Library

**Status**: 📋 **TRIAGE - AWAITING DECISION**
**Date**: January 4, 2026
**Authority**: TS1 feedback from SOC2 implementation planning
**Confidence**: 85%

---

## 🎯 **Context**

During SOC2 test plan creation, audit helper functions were documented following DD-TESTING-001 standards:
- `queryAuditEvents()` - Query Data Storage for audit events using OpenAPI client
- `waitForAuditEvents()` - Poll with `Eventually()` until events appear
- `countEventsByType()` - Deterministic event count validation

**Discovery**: Multiple services have **already implemented similar helpers** locally:
- `test/integration/remediationorchestrator/audit_emission_integration_test.go` (8 usages)
- `test/integration/notification/controller_audit_emission_test.go` (8 usages)
- `test/integration/aianalysis/audit_flow_integration_test.go` (6 usages)
- `test/integration/aianalysis/audit_integration_test.go` (3 usages)

**Question**: Should we extract these to a shared library?

---

## 📊 **Current State Analysis**

### **Existing Implementations**

| Service | Function | Implementation | Lines | Event Category |
|---------|----------|---------------|-------|----------------|
| **Remediation Orchestrator** | `queryAuditEventsOpenAPI()` | Full OpenAPI with error handling | 30 | `orchestration` |
| **Notification** | `queryAuditEvents()` (closure) | OpenAPI + client-side `resource_id` filter | 20 | `notification` |
| **AI Analysis** | `countEventsByType()` | Map-based counting | 7 | `analysis` |
| **AI Analysis** | `queryAuditEventsViaAPI()` | Commented out (unused) | N/A | N/A |

### **Key Differences Between Implementations**

1. **Event Category Binding**:
   - Each service hardcodes its own `event_category` (orchestration, notification, analysis)
   - DD-TESTING-001 requires `event_category` parameter for multi-service usage

2. **Resource Filtering**:
   - Notification service adds client-side `resource_id` filtering (OpenAPI spec gap)
   - Other services don't need this filtering

3. **Error Handling**:
   - Remediation Orchestrator: Verbose with GinkgoWriter logging
   - Notification: Silent error handling (returns nil)

4. **Context Management**:
   - Remediation Orchestrator: Uses `context.Background()` inline
   - Notification: Uses shared `queryCtx` from BeforeEach

---

## 💰 **Cost/Benefit Analysis**

### **Benefits of Shared Library** ✅

| Benefit | Impact | Evidence |
|---------|--------|----------|
| **DD-TESTING-001 Compliance** | HIGH | Centralized implementation ensures all services follow standards |
| **Reduced Duplication** | MEDIUM | 25 usages across 4 files (estimated ~100 LoC duplication) |
| **Easier Maintenance** | HIGH | Single place to update if OpenAPI client changes |
| **Consistent Error Handling** | MEDIUM | Standardized error messages across all services |
| **Reusability for SOC2** | HIGH | New services (Gateway, Workflow Execution) can use immediately |

**Estimated Time Savings**: 2-3 hours per new service (no need to reimplement helpers)

### **Costs of Refactoring** ❌

| Cost | Impact | Evidence |
|------|--------|----------|
| **Refactoring Effort** | MEDIUM | 4-6 hours to extract, test, and migrate existing usages |
| **Risk of Breaking Tests** | LOW | Integration tests are well-isolated, low risk |
| **API Design Complexity** | LOW | Need to support multiple `event_category` values |
| **Delayed SOC2 Work** | MEDIUM | Refactoring is NOT on critical path for 100% RR reconstruction |

**Estimated Effort**: 4-6 hours (0.5-0.75 days)

---

## 🔍 **Recommendation**

### **Option A: Extract to Shared Library (RECOMMENDED)** ⭐

**Rationale**:
1. ✅ **SOC2 Scope**: 5 services need these helpers (Gateway, AI Analysis, Workflow Execution, Orchestrator, Data Storage)
2. ✅ **DD-TESTING-001 Authority**: Shared library ensures ALL services follow validation standards
3. ✅ **Long-Term Savings**: 2-3 hours saved per service × 5 services = 10-15 hours benefit
4. ✅ **Low Risk**: Integration tests are isolated, refactoring is safe
5. ✅ **Future-Proof**: Any OpenAPI client changes only need 1 update

**Proposed Implementation**:
- Location: `pkg/testutil/audit/helpers.go`
- Functions:
  ```go
  // queryAuditEvents queries Data Storage for audit events using OpenAPI client.
  func QueryAuditEvents(
      ctx context.Context,
      client *dsgen.ClientWithResponses,
      correlationID string,
      eventCategory string,
      eventType *string,
  ) ([]dsgen.AuditEvent, error)

  // waitForAuditEvents polls Data Storage until events appear.
  func WaitForAuditEvents(
      ctx context.Context,
      client *dsgen.ClientWithResponses,
      correlationID string,
      eventCategory string,
      eventType string,
      minCount int,
      timeout time.Duration,
  ) []dsgen.AuditEvent

  // countEventsByType counts occurrences of each event type.
  func CountEventsByType(events []dsgen.AuditEvent) map[string]int
  ```

**Effort Breakdown**:
- Create `pkg/testutil/audit/helpers.go`: 1h
- Write unit tests for helpers: 1h
- Migrate Remediation Orchestrator (8 usages): 1h
- Migrate Notification (8 usages): 1h
- Migrate AI Analysis (6 usages): 0.5h
- Validation: 0.5h

**Total**: 5 hours

### **Option B: Keep Local Implementations** ❌

**Rationale**:
- ✅ **Zero Refactoring Cost**: No immediate work needed
- ❌ **Long-Term Cost**: New services must reimplement (2-3h × 5 services = 10-15h)
- ❌ **Inconsistency Risk**: Different error handling across services
- ❌ **Maintenance Burden**: OpenAPI changes require 4 file updates

**Not Recommended**: Short-term savings (5h) outweighed by long-term costs (10-15h)

---

## 📋 **Decision Matrix**

| Criterion | Weight | Option A (Shared) | Option B (Local) | Winner |
|-----------|--------|-------------------|------------------|--------|
| **DD-TESTING-001 Compliance** | ⭐⭐⭐⭐⭐ | Centralized enforcement | Service-specific risk | A |
| **SOC2 Timeline Impact** | ⭐⭐⭐⭐ | +5h upfront, saves 10-15h | 0h upfront, costs 10-15h | A |
| **Maintenance Burden** | ⭐⭐⭐⭐ | 1 file to update | 4-5 files to update | A |
| **Code Quality** | ⭐⭐⭐ | DRY principle | Acceptable duplication | A |
| **Risk** | ⭐⭐⭐ | Low (isolated tests) | Zero (no changes) | B |

**Winner**: **Option A (Shared Library)** - 4 out of 5 criteria favor extraction

---

## 🎯 **Proposed Action**

### **RECOMMEND: Extract to Shared Library**

**Timing**: **After Day 5, Before Day 6 validation** (or parallel to Days 1-5)

**Justification**:
1. SOC2 implementation (Days 1-5) will add 5 NEW services using these helpers
2. Extracting NOW prevents 10-15 hours of duplication
3. Day 6 validation will test the shared library across all services
4. 5-hour investment has 2-3x ROI (10-15 hours saved)

**Critical Path**:
- ❌ **NOT on critical path** for 100% RR reconstruction
- ✅ **Parallel work**: Can be done by second developer while primary works on Days 1-5
- ✅ **Quality improvement**: Ensures DD-TESTING-001 compliance from the start

---

## 🔗 **Related Documents**

- **DD-TESTING-001**: Audit event validation standards (AUTHORITATIVE)
- **SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md**: Test scenarios using these helpers
- **TESTING_GUIDELINES.md**: Anti-patterns and best practices

---

## 📊 **Confidence Assessment**

**Confidence**: 85%

**Justification**:
- ✅ Clear cost/benefit analysis (5h cost, 10-15h benefit)
- ✅ Low risk (integration tests are isolated)
- ✅ Aligns with DD-TESTING-001 authority
- ⚠️ Slight uncertainty on exact migration effort (4-6h estimate)

**Risk Assessment**:
- **Risk**: Breaking existing tests during migration
- **Mitigation**: Run full test suite after each service migration
- **Probability**: LOW (integration tests are well-isolated)

---

**Recommendation**: **Proceed with Option A (Shared Library Extraction)**
**Estimated Effort**: 5 hours (0.625 days)
**ROI**: 2-3x return on investment
**Risk Level**: LOW

---

**Document Status**: 📋 **TRIAGE COMPLETE - AWAITING USER DECISION**
**Next Action**: User approves Option A or B
**Timeline Impact**: +5h if approved, can be parallelized with Days 1-5

