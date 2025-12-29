# RO Test Plan Update Status

**Date**: December 22, 2025
**Status**: ⚠️ **UPDATE NEEDED**

---

## 📋 **Current State of Test Plan**

The test plan `RO_COMPREHENSIVE_TEST_PLAN.md` currently has:

✅ **Unit Test Phase 1-4** - Documented and Phase 1 complete (22 tests)
✅ **Defense-in-Depth Matrix** - Exists but only shows unit test status
✅ **Integration Tests** - Shows "22 tests" but no detailed scenarios
❌ **Integration Phase 1 Plan** - **MISSING**
❌ **Updated Defense Matrix** - **MISSING integration test details**

---

## 🎯 **What Needs to Be Added**

### **1. Integration Test Phase 1 Section** (NEW)

Add comprehensive section documenting **28 integration test scenarios**:

#### **A. Audit Emission Tests** (8 scenarios)
- AE-INT-1: Lifecycle started audit validation
- AE-INT-2: Phase transition audit validation
- AE-INT-3: Completion audit validation
- AE-INT-4: Failure audit validation
- AE-INT-5: Approval requested audit validation
- AE-INT-6: Approval decision audit validation
- AE-INT-7: Rejection audit validation
- AE-INT-8: Timeout audit validation

**Validation Method**: Query Data Storage REST API for persisted events

#### **B. Metrics Validation Tests** (6 scenarios)
- M-INT-1: reconcile_total counter
- M-INT-2: reconcile_duration_seconds histogram
- M-INT-3: phase_transitions_total counter
- M-INT-4: timeouts_total counter
- M-INT-5: status_update_retries_total counter
- M-INT-6: status_update_conflicts_total counter

**Validation Method**: Scrape /metrics endpoint or test registry

#### **C. Timeout Edge Cases** (7 scenarios)
- TO-INT-1: Global timeout exceeded (existing)
- TO-INT-2: Global timeout not exceeded (NEW)
- TO-INT-3: Processing phase timeout (NEW)
- TO-INT-4: Analyzing phase timeout (NEW)
- TO-INT-5: Executing phase timeout (NEW)
- TO-INT-6: Timeout notification created (existing)
- TO-INT-7: Global timeout precedence (NEW - from unit gap)
- TO-INT-8: Terminal phase no-op (NEW - from unit gap)

**Validation Method**: Real time-based validation with Kubernetes API

#### **D. Routing Engine Tests** (5 scenarios)
- RT-INT-1: Consecutive failure blocking (BR-ORCH-042)
- RT-INT-2: Blocked phase expiry transition
- RT-INT-3: Routing blocked audit event
- RT-INT-4: blocked_total metric
- RT-INT-5: blocked_current gauge

**Validation Method**: Manual failed WE CRDs + real routing engine + Redis

#### **E. Notification Creation Tests** (2 scenarios)
- NC-INT-1: Timeout notification CRD creation
- NC-INT-2: Approval notification CRD creation

**Validation Method**: Verify NotificationRequest CRD created (no NT controller)

---

### **2. Updated Defense-in-Depth Matrix**

Need to add Integration Test column details for all scenarios:

| Scenario ID | Scenario Name | Unit Test | Integration Test | E2E Test | Priority |
|-------------|---------------|-----------|------------------|----------|----------|
| **AUDIT EMISSION** |||||
| AE-7.1 | Lifecycle started | ⚠️ Limited (fire-and-forget) | 🔥 **Phase 1** (DS query) | ❌ N/A | 🔥 CRITICAL |
| AE-7.2 | Phase transition | ⚠️ Limited | 🔥 **Phase 1** (DS query) | ❌ N/A | 🔥 CRITICAL |
| AE-7.3 | Completion | ⚠️ Limited | 🔥 **Phase 1** (DS query) | ⚠️ Phase 2 | 🔥 CRITICAL |
| AE-7.4 | Failure | ⚠️ Limited | 🔥 **Phase 1** (DS query) | ⚠️ Phase 2 | 🔥 CRITICAL |
| AE-7.5 | Approval requested | ⚠️ Limited | 🔥 **Phase 1** (DS query) | ⚠️ Phase 2 | 🔥 CRITICAL |
| AE-7.6 | Approval decision | ⚠️ Limited | 🔥 **Phase 1** (DS query) | ⚠️ Phase 2 | 🔥 CRITICAL |
| AE-7.7 | Rejection | ⚠️ Limited | 🔥 **Phase 1** (DS query) | ⚠️ Phase 2 | 🔥 CRITICAL |
| AE-7.8 | Timeout | ⚠️ Limited | 🔥 **Phase 1** (DS query) | ❌ N/A | 🔥 CRITICAL |
| **METRICS** |||||
| M-1 | reconcile_total | ❌ None | 🔥 **Phase 1** (scrape) | ⚠️ Phase 2 | 🔥 CRITICAL |
| M-2 | reconcile_duration | ❌ None | 🔥 **Phase 1** (scrape) | ⚠️ Phase 2 | 🔥 CRITICAL |
| M-3 | phase_transitions | ❌ None | 🔥 **Phase 1** (scrape) | ⚠️ Phase 2 | 🔥 CRITICAL |
| M-4 | timeouts_total | ❌ None | 🔥 **Phase 1** (scrape) | ⚠️ Phase 2 | 🔥 HIGH |
| M-5 | status_retries | ❌ None | 🔥 **Phase 1** (scrape) | ❌ N/A | ⚠️ HIGH |
| M-6 | status_conflicts | ❌ None | 🔥 **Phase 1** (scrape) | ❌ N/A | ⚠️ HIGH |
| **TIMEOUT EDGE CASES** |||||
| TO-1.2 | Global not exceeded | ❌ None | 🔥 **Phase 1** (real time) | ❌ N/A | ⚠️ HIGH |
| TO-1.3 | Processing timeout | ✅ Unit | 🔥 **Phase 1** (real time) | ⚠️ Phase 2 | 🔥 HIGH |
| TO-1.4 | Analyzing timeout | ✅ Unit | 🔥 **Phase 1** (real time) | ⚠️ Phase 2 | 🔥 HIGH |
| TO-1.5 | Executing timeout | ✅ Unit | 🔥 **Phase 1** (real time) | ⚠️ Phase 2 | 🔥 HIGH |
| TO-1.7 | Global precedence | ✅ Unit | 🔥 **Phase 1** (real time) | ❌ N/A | 🔥 HIGH |
| TO-1.8 | Terminal no-op | ✅ Unit | 🔥 **Phase 1** (real time) | ❌ N/A | ⚠️ HIGH |
| **ROUTING ENGINE** |||||
| RT-1 | Consecutive blocking | ❌ Mocked | 🔥 **Phase 1** (real Redis) | ⚠️ Phase 2 | 🔥 CRITICAL |
| RT-2 | Blocked expiry | ❌ Mocked | 🔥 **Phase 1** (real Redis) | ⚠️ Phase 2 | 🔥 HIGH |
| RT-3 | Routing blocked audit | ❌ Mocked | 🔥 **Phase 1** (DS query) | ⚠️ Phase 2 | 🔥 HIGH |
| RT-4 | blocked_total metric | ❌ None | 🔥 **Phase 1** (scrape) | ⚠️ Phase 2 | ⚠️ HIGH |
| RT-5 | blocked_current gauge | ❌ None | 🔥 **Phase 1** (scrape) | ⚠️ Phase 2 | ⚠️ HIGH |
| **NOTIFICATIONS** |||||
| NC-1 | Timeout notification | ❌ None | 🔥 **Phase 1** (CRD check) | ⚠️ Phase 2 | ⚠️ MEDIUM |
| NC-2 | Approval notification | ❌ None | 🔥 **Phase 1** (CRD check) | ⚠️ Phase 2 | ⚠️ MEDIUM |

**Coverage Summary**:
- **Unit Tests**: 35 scenarios (Phase 1-2 complete)
- **Integration Phase 1**: **28 scenarios** (NEW)
- **E2E Phase 2**: 10+ scenarios (planned)
- **Defense Multiplier**: 2-3x overlap

---

### **3. Updated Test Count Projection Table**

Update the existing table to reflect integration test additions:

| Phase | Unit Tests | Integration Tests | E2E Tests | Total |
|-------|------------|-------------------|-----------|-------|
| **Current (Dec 22)** | 35 (Phase 1-2) | 11 (existing) | 0 (10 skipped) | 46 |
| **After Int Phase 1** | 35 | **28** (+17) | 0 (10 skipped) | **63** |
| **After E2E Phase 2** | 35 | 28 | 10 (enabled) | **73** |
| **Defense Multiplier** | 1x | 2x | 3x | **2-3x overlap** |

---

### **4. Integration Phase 1 Implementation Roadmap**

Add new section after Unit Test Phase 4:

```markdown
## 🧪 **Integration Test Phase 1: RO + Data Storage + Redis**

**Dependencies**: RO Controller (envtest) + Data Storage (podman) + Redis (podman)
**No Other Controllers Needed**: Can run without SP/AI/WE/NT controllers
**Target**: 28 integration tests
**Business Value**: 🔥 **95%** (compliance + observability + edge cases)

### **Tier 1: Compliance & Observability** (12 tests, 5-6h)
Priority: 🔥 **CRITICAL**

#### **Audit Emission Validation** (8 tests)
- [ ] AE-INT-1: Lifecycle started audit (query DS API)
- [ ] AE-INT-2: Phase transition audit (query DS API)
- [ ] AE-INT-3: Completion audit (query DS API)
- [ ] AE-INT-4: Failure audit (query DS API)
- [ ] AE-INT-5: Approval requested audit (query DS API)
- [ ] AE-INT-6: Approval decision audit (query DS API)
- [ ] AE-INT-7: Rejection audit (query DS API)
- [ ] AE-INT-8: Timeout audit (query DS API)

**Business Requirement**: DD-AUDIT-003 compliance
**Why Integration**: Unit tests failed (fire-and-forget), need to validate persistence
**Validation**: Query `GET /api/v1/events?correlation_id={id}` on Data Storage

#### **Core Metrics Validation** (3 tests)
- [ ] M-INT-1: reconcile_total counter (scrape /metrics)
- [ ] M-INT-2: reconcile_duration_seconds histogram
- [ ] M-INT-3: phase_transitions_total counter

**Business Requirement**: Observability foundation
**Why Integration**: Need real Prometheus metrics collection
**Validation**: Scrape `/metrics` endpoint or use test registry

#### **Timeout Metrics** (1 test)
- [ ] M-INT-4: timeouts_total counter

**Business Requirement**: SLA alerting
**Why Integration**: Real timeout detection + metrics

### **Tier 2: Edge Cases & Blocking** (12 tests, 6-7h)
Priority: 🔥 **HIGH**

#### **Timeout Edge Cases** (7 tests)
- [ ] TO-INT-1: Global timeout exceeded ✅ (existing)
- [ ] TO-INT-2: Global timeout not exceeded (NEW)
- [ ] TO-INT-3: Processing phase timeout (NEW)
- [ ] TO-INT-4: Analyzing phase timeout (NEW)
- [ ] TO-INT-5: Executing phase timeout (NEW)
- [ ] TO-INT-6: Timeout notification created ✅ (existing)
- [ ] TO-INT-7: Global timeout precedence (unit test gap)
- [ ] TO-INT-8: Terminal phase no-op (unit test gap)

**Business Requirement**: BR-ORCH-027, BR-ORCH-028
**Why Integration**: Real time-based validation with Kubernetes API

#### **Routing Engine Blocking** (5 tests)
- [ ] RT-INT-1: Consecutive failure blocking (BR-ORCH-042)
- [ ] RT-INT-2: Blocked phase expiry transition
- [ ] RT-INT-3: Routing blocked audit event
- [ ] RT-INT-4: blocked_total metric
- [ ] RT-INT-5: blocked_current gauge

**Business Requirement**: BR-ORCH-042
**Why Integration**: Real routing engine with Redis state
**Key Insight**: Can manually create failed WE CRDs (no WE controller needed!)

### **Tier 3: Advanced Observability** (4 tests, 3h)
Priority: ⚠️ **MEDIUM**

#### **Retry Metrics** (2 tests)
- [ ] M-INT-5: status_update_retries_total
- [ ] M-INT-6: status_update_conflicts_total

**Business Requirement**: REFACTOR-RO-008
**Why Integration**: Real optimistic concurrency conflicts

#### **Notification Creation** (2 tests)
- [ ] NC-INT-1: Timeout notification CRD creation
- [ ] NC-INT-2: Approval notification CRD creation

**Business Requirement**: BR-ORCH-029, BR-ORCH-030
**Why Integration**: Real CRD creation (no NT controller needed)

### **Implementation Timeline**
- **Week 1**: Audit emission (8 tests)
- **Week 2**: Core metrics + timeout metrics (4 tests)
- **Week 3**: Timeout edge cases (7 tests)
- **Week 4**: Routing engine (5 tests)
- **Week 5**: Retry metrics + notifications (4 tests)

**Total**: 28 tests in 5 weeks (1-2 hours/day avg)

### **Success Criteria**
- ✅ All 28 integration tests passing
- ✅ <60 seconds execution time for full suite
- ✅ DD-AUDIT-003 compliance validated (100% audit paths)
- ✅ Core observability metrics validated
- ✅ BR-ORCH-042 routing blocking validated
- ✅ Defense-in-depth overlap achieved (2-3x)
```

---

## 🎯 **Summary of Changes Needed**

1. **Add**: Integration Test Phase 1 detailed section (28 scenarios)
2. **Update**: Defense-in-Depth Matrix with integration test details
3. **Update**: Test count projection table
4. **Add**: Integration Phase 1 implementation roadmap
5. **Update**: Coverage projection (if applicable)

---

## 📝 **Next Steps**

1. Review this update proposal
2. Approve the structure and content
3. I'll update `RO_COMPREHENSIVE_TEST_PLAN.md` with all sections
4. Result: Complete test plan with full Phase 1 integration coverage

---

**Status**: ⚠️ **AWAITING APPROVAL**
**Question**: Should I proceed to update the comprehensive test plan with these sections?



