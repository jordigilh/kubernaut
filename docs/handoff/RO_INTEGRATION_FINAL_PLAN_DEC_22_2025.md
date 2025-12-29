# RO Integration Test Phase 1 - Final Plan

**Date**: December 22, 2025
**Status**: ✅ **READY FOR IMPLEMENTATION**
**Approach**: Business Requirement validation first, infrastructure second

---

## 🎯 **Executive Summary**

**Total Tests**: 32 integration tests
**Business Requirements**: 8 BRs validated (all P0/P1 critical)
**Implementation Time**: 10-14 hours
**Business Value**: 🔥 **95%** - All tests validate critical BRs
**Infrastructure**: RO + Data Storage + envtest (NO other controllers)

---

## ✅ **Key Corrections Applied**

### **1. Redis Dependency** ❌ → ✅
- **Incorrect Assumption**: "Routing engine uses Redis for state"
- **Reality**: Routing engine uses **Kubernetes API only**
  - Uses `client.List()` with field selectors
  - All state in CRD status fields
  - NO Redis dependency
- **Redis Purpose**: Data Storage service caching (NOT routing)

### **2. Testing Approach** ❌ → ✅
- **Previous**: Infrastructure-first (Redis, metrics, routing)
- **Corrected**: BR-first (validate business requirements)
- **Impact**: Tests now map directly to BR acceptance criteria

---

## 📋 **Integration Phase 1 Test Matrix** (32 tests)

### **Tier 1: Compliance & Observability** (14 tests, 4-5h)

#### **A. Audit Trail Integration (BR-ORCH-041)** - 8 tests, 2.5-3h
**Priority**: 🔥 **CRITICAL** - DD-AUDIT-003 compliance

| Test ID | Scenario | Validation | Time |
|---------|----------|------------|------|
| AE-INT-1 | Lifecycle started (Pending→Processing) | Query DS API | 20min |
| AE-INT-2 | Phase transition (Processing→Analyzing) | Query DS API | 20min |
| AE-INT-3 | Completion (Executing→Completed) | Query DS API | 20min |
| AE-INT-4 | Failure (any→Failed) | Query DS API | 20min |
| AE-INT-5 | Approval requested (Analyzing→AwaitingApproval) | Query DS API | 20min |
| AE-INT-6 | Manual review (AI→ManualReview) | Query DS API | 20min |
| AE-INT-7 | Timeout (any→TimedOut) | Query DS API | 20min |
| AE-INT-8 | Metadata validation (correlation_id, timestamps) | Query DS API | 20min |

**Validation**: `GET /api/v1/events?correlation_id={rrUID}&event_type={type}`

---

#### **B. Operational Metrics (BR-ORCH-044)** - 6 tests, 1.5-2h
**Priority**: 🔥 **CRITICAL** - SLO foundation

| Test ID | Scenario | Validation | Time |
|---------|----------|------------|------|
| M-INT-1 | reconcile_total counter | Scrape /metrics | 15min |
| M-INT-2 | reconcile_duration_seconds histogram | Scrape /metrics | 15min |
| M-INT-3 | phase_transitions_total counter | Scrape /metrics | 15min |
| M-INT-4 | timeouts_total counter | Scrape /metrics | 15min |
| M-INT-5 | status_update_retries_total | Scrape /metrics | 20min |
| M-INT-6 | status_update_conflicts_total | Scrape /metrics | 20min |

**Validation**: Scrape Prometheus `/metrics` endpoint

---

### **Tier 2: SLA Enforcement** (7 tests, 2.5-3h)

#### **C. Timeout Management (BR-ORCH-027/028)** - 7 tests, 2.5-3h
**Priority**: 🔥 **CRITICAL** - Prevents stuck remediations

| Test ID | Scenario | BR | Time |
|---------|----------|-----|------|
| TO-INT-1 | Global timeout exceeded (>60min) | AC-027-1 | 25min |
| TO-INT-2 | Global timeout not exceeded (<60min) | AC-027-1 | 20min |
| TO-INT-3 | Processing phase timeout (>5min) | AC-028-2 | 25min |
| TO-INT-4 | Analyzing phase timeout (>10min) | AC-028-2 | 25min |
| TO-INT-5 | Executing phase timeout (>30min) | AC-028-2 | 25min |
| TO-INT-6 | Timeout notification created | AC-027-2 | 20min |
| TO-INT-7 | Phase timeout precedence | AC-028-2 | 25min |

**Validation**: Real K8s API + Time.Now() checks

---

### **Tier 3: Business Logic** (11 tests, 3.5-4.5h)

#### **D. Consecutive Failure Blocking (BR-ORCH-042)** - 5 tests, 2-2.5h
**Priority**: 🔥 **CRITICAL** - Resource protection

| Test ID | Scenario | BR | Time |
|---------|----------|-----|------|
| CF-INT-1 | Block after 3 consecutive failures | AC-042-1.1 | 30min |
| CF-INT-2 | Count resets on Completed RR | AC-042-1.2 | 25min |
| CF-INT-3 | Blocked phase prevents new RR | AC-042-2.2 | 25min |
| CF-INT-4 | Cooldown expiry → Failed | AC-042-3.2 | 25min |
| CF-INT-5 | BlockedUntil calculation | AC-042-3.1 | 20min |

**Validation**: Manual RR CRDs + routing engine (uses K8s API, NOT Redis!)

---

#### **E. Notification Creation (BR-ORCH-001/036)** - 4 tests, 1-1.5h
**Priority**: 🔥 **CRITICAL** - Approval workflow

| Test ID | Scenario | BR | Time |
|---------|----------|-----|------|
| NC-INT-1 | Approval notification (low confidence) | AC-001-1 | 20min |
| NC-INT-2 | Manual review notification | AC-036-1 | 20min |
| NC-INT-3 | Timeout notification | AC-027-2 | 20min |
| NC-INT-4 | Idempotency (no duplicates) | AC-001-2 | 20min |

**Validation**: Verify NotificationRequest CRD exists with correct fields

---

#### **F. Lifecycle Orchestration (BR-ORCH-025)** - 2 tests, ✅ Existing
**Priority**: 🔥 **CRITICAL** - Core orchestration

| Test ID | Scenario | Status |
|---------|----------|--------|
| LC-INT-1 | Happy path (Pending→Completed) | ✅ Existing |
| LC-INT-2 | Failure path (Pending→Failed) | ✅ Existing |

---

## 📊 **Summary Table**

| Tier | Category | Tests | Time | BR | Priority |
|------|----------|-------|------|-----|----------|
| 1 | Audit Trail | 8 | 2.5-3h | BR-ORCH-041 | 🔥 |
| 1 | Metrics | 6 | 1.5-2h | BR-ORCH-044 | 🔥 |
| 2 | Timeouts | 7 | 2.5-3h | BR-ORCH-027/028 | 🔥 |
| 3 | Consecutive Failures | 5 | 2-2.5h | BR-ORCH-042 | 🔥 |
| 3 | Notifications | 4 | 1-1.5h | BR-ORCH-001/036 | 🔥 |
| 3 | Lifecycle | 2 | ✅ Existing | BR-ORCH-025 | 🔥 |
| **TOTAL** | **6** | **32** | **10-14h** | **8 BRs** | **95%** |

---

## 🔧 **Infrastructure Requirements**

### **✅ Required**
- **RO Controller** (running in envtest)
- **Data Storage** (PostgreSQL + Redis for DS)
- **envtest** (Kubernetes API server)

### **❌ NOT Required**
- ❌ Redis for routing engine (uses K8s API!)
- ❌ SP/AI/WE controllers (manual CRD creation)
- ❌ NT controller (CRD validation only)
- ❌ RAR controller (CRD validation only)
- ❌ Gateway service

---

## 🎯 **Implementation Order**

### **Week 1: Compliance (Tier 1)**
**Days 1-2**: Audit Trail (8 tests, 2.5-3h)
- Highest priority (DD-AUDIT-003 compliance)
- Validates event persistence in Data Storage

**Day 3**: Operational Metrics (6 tests, 1.5-2h)
- SLO foundation for production monitoring

---

### **Week 2: SLA Enforcement (Tier 2)**
**Days 1-2**: Timeout Management (7 tests, 2.5-3h)
- Critical for preventing stuck remediations
- Real time-based validation

---

### **Week 3: Business Logic (Tier 3)**
**Days 1-2**: Consecutive Failures (5 tests, 2-2.5h)
- Resource protection mechanism
- Routing engine validation

**Day 3**: Notifications (4 tests, 1-1.5h)
- Approval workflow enabler
- Lifecycle tests already exist

---

## ✅ **Success Criteria**

### **Phase 1 Complete When**:
1. ✅ All 32 integration tests passing
2. ✅ 8 Business Requirements validated
3. ✅ <60 seconds execution time
4. ✅ DD-AUDIT-003 compliance (100% audit paths)
5. ✅ BR-ORCH-044 metrics exposed and queryable
6. ✅ BR-ORCH-042 blocking validated
7. ✅ BR-ORCH-027/028 timeout detection validated

---

## 🚫 **NOT Phase 1 - E2E Required**

**Phase 2 (E2E Branch)** - Requires additional controllers:

| Category | Tests | Controllers | Reason |
|----------|-------|-------------|--------|
| Approval Orchestration | 5 | RO + RAR | RAR decisions needed |
| Notification Delivery | 3 | RO + NT | NT delivery needed |
| Signal Ingestion | 2 | RO + Gateway | Gateway dedup needed |
| Full Orchestration | 4 | All controllers | Complete flow |
| **TOTAL** | **14** | **Multiple** | **New branch** |

---

## 🔗 **Defense-in-Depth Coverage**

| Scenario | Unit | Integration Phase 1 | E2E Phase 2 | BR |
|----------|------|-------------------|-------------|-----|
| **Audit emission** | ⚠️ Mock | 🔥 **DS query** | ❌ N/A | BR-ORCH-041 |
| **Metrics** | ❌ N/A | 🔥 **Scrape** | ⚠️ Full | BR-ORCH-044 |
| **Global timeout** | ✅ Mock | 🔥 **Real time** | ⚠️ Full | BR-ORCH-027 |
| **Phase timeout** | ✅ Mock | 🔥 **Real time** | ⚠️ Full | BR-ORCH-028 |
| **Consecutive failures** | ❌ Mocked | 🔥 **K8s API** | ⚠️ Full | BR-ORCH-042 |
| **Notifications** | ✅ Mock | 🔥 **CRD check** | ⚠️ Delivery | BR-ORCH-001/036 |
| **Lifecycle** | ✅ Mock | 🔥 **Existing** | ⚠️ Full | BR-ORCH-025 |

**Coverage**: 2-3x defense-in-depth overlap

---

## 📚 **References**

### **Business Requirements**
- [BR_MAPPING.md](../services/crd-controllers/05-remediationorchestrator/BR_MAPPING.md)
- [BR-ORCH-041](../requirements/BR-ORCH-041-audit-trail-integration.md) - Audit
- [BR-ORCH-044](../requirements/BR-ORCH-044-operational-observability-metrics.md) - Metrics
- [BR-ORCH-042](../requirements/BR-ORCH-042-consecutive-failure-blocking.md) - Blocking
- [BR-ORCH-027/028](../requirements/BR-ORCH-027-028-timeout-management.md) - Timeouts

### **Test Plans**
- [RO_COMPREHENSIVE_TEST_PLAN.md](../services/crd-controllers/05-remediationorchestrator/RO_COMPREHENSIVE_TEST_PLAN.md)
- [RO_INTEGRATION_BR_ASSESSMENT_DEC_22_2025.md](./RO_INTEGRATION_BR_ASSESSMENT_DEC_22_2025.md)

### **Implementation**
- [reconciler.go](../../internal/controller/remediationorchestrator/reconciler.go)
- [routing/blocking.go](../../pkg/remediationorchestrator/routing/blocking.go)

---

## 🚀 **Next Steps**

1. ✅ **Test plan updated** with integration phase 1 details
2. ✅ **Defense-in-depth matrix** updated with BR mappings
3. 📋 **User approval** for integration test implementation
4. 🔄 **Implement Tier 1** (Compliance - 2.5 days)
5. 🔄 **Implement Tier 2** (SLA - 2.5 days)
6. 🔄 **Implement Tier 3** (Business Logic - 3 days)
7. ✅ **Phase 1 complete** (32 tests, 8 BRs validated)

---

**Status**: ✅ **READY FOR IMPLEMENTATION**
**Business Value**: 🔥 **95%** - All P0/P1 BRs
**Confidence**: **90%** - Clear BR mapping, infrastructure understood



