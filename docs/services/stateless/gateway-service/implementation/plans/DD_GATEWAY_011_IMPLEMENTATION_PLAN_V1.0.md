# DD-GATEWAY-011: Shared Status Ownership - Implementation Plan

**Version**: 1.1
**Filename**: `DD_GATEWAY_011_IMPLEMENTATION_PLAN_V1.0.md`
**Status**: ✅ **COMPLETE**
**Design Decision**: [DD-GATEWAY-011](../../../../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md)
**Service**: Gateway Service
**Confidence**: 95% (Evidence-Based - E2E Tests Passing)
**Actual Effort**: 8 days (APDC cycle: 4 days implementation + 3 days testing + 1 day documentation)

⚠️ **CRITICAL**: Filename version MUST match document version at all times.

---

## 🚨 **CRITICAL: Read This First**

**Before starting implementation, you MUST review these 5 critical pitfalls** (see Critical Pitfalls section):

1. **Insufficient TDD Discipline** → Write ONE test at a time (not batched)
2. **Missing Integration Tests** → Integration tests BEFORE E2E tests
3. **Critical Infrastructure Without Unit Tests** → ≥70% coverage for critical components
4. **Late E2E Discovery** → Follow test pyramid (Unit → Integration → E2E)
5. **No Test Coverage Gates** → Automated CI/CD coverage gates

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.1** | 2025-12-10 | Implementation complete - E2E tests passing | ✅ **CURRENT** |
| **v1.0** | 2025-12-07 | Initial implementation plan created | ⏹️ Superseded |

---

## 🎯 **Business Requirements**

### **Primary Business Requirements**

| BR ID | Description | Success Criteria |
|-------|-------------|------------------|
| **BR-GATEWAY-181** | Move deduplication tracking from spec to status | `status.deduplication` updated, spec immutable |
| **BR-GATEWAY-182** | Move storm aggregation from Redis to status | `status.stormAggregation` updated, Redis not used |
| **BR-GATEWAY-183** | Implement optimistic concurrency for status updates | `retry.RetryOnConflict` pattern used |
| **BR-GATEWAY-184** | Check RR phase for deduplication decisions | Terminal phases allow new RR creation |
| **BR-GATEWAY-185** | Support Redis deprecation | All dedup/storm state in K8s status |

### **Success Metrics**

**Format**: `[Metric]: [Target] - *Justification: [Why this target?]*`

- **Spec Immutability**: 100% - *Justification: K8s best practice - spec should never be updated after creation*
- **Conflict Resolution Success**: >99% - *Justification: `retry.RetryOnConflict` handles transient conflicts*
- **P95 Latency**: <50ms - *Justification: Gateway latency target preserved with 10ms retry backoff*
- **Redis Dependency**: 0 for dedup/storm - *Justification: DD-GATEWAY-011 enables complete Redis removal*

---

## 📅 **Timeline Overview**

### **Phase Breakdown**

| Phase | Duration | Days | Purpose | Key Deliverables |
|-------|----------|------|---------|------------------|
| **ANALYSIS** | 2 hours | Day 0 (pre-work) | Comprehensive context understanding | Analysis document, existing code review |
| **PLAN** | 2 hours | Day 0 (pre-work) | Detailed implementation strategy | This document, TDD phase mapping |
| **DO (Implementation)** | 4 days | Days 1-4 | Controlled TDD execution | Core feature logic, integration |
| **CHECK (Testing)** | 3 days | Days 5-7 | Comprehensive result validation | Test suite (unit/integration/E2E) |
| **PRODUCTION READINESS** | 1 day | Day 8 | Documentation & deployment prep | Runbooks, handoff docs |

### **8-Day Implementation Timeline**

| Day | Phase | Focus | Hours | Key Milestones | Status |
|-----|-------|-------|-------|----------------|--------|
| **Day 0** | ANALYSIS + PLAN | Pre-work | 4h | Analysis complete, Plan approved (this document) | ✅ |
| **Day 1** | DO-RED | Deduplication status tests | 8h | Test framework, failing tests for status.deduplication | ✅ |
| **Day 2** | DO-GREEN | Deduplication implementation | 8h | `status.deduplication` working, spec immutable | ✅ |
| **Day 3** | DO-RED/GREEN | Storm aggregation | 8h | `status.stormAggregation` tests + implementation | ✅ |
| **Day 4** | DO-REFACTOR | Integration + conflict retry | 8h | Full integration with retry logic | ✅ |
| **Day 5** | CHECK | Unit tests | 8h | 70%+ unit test coverage | ✅ |
| **Day 6** | CHECK | Integration tests | 8h | Integration test scenarios | ✅ |
| **Day 7** | CHECK | E2E tests | 8h | Full feature lifecycle, BR validation | ✅ |
| **Day 8** | PRODUCTION | Documentation | 8h | API docs, handoff summary | ✅ |

### **Critical Path Dependencies**

```
Day 1 (Dedup Tests) → Day 2 (Dedup Impl) → Day 3 (Storm)
                                         ↓
Day 4 (Integration) → Days 5-7 (Testing) → Day 8 (Production)
```

### **Pre-requisites** ✅ **ALL MET**

| Pre-requisite | Status | Evidence |
|---------------|--------|----------|
| RO API types ready | ✅ **DONE** | `DeduplicationStatus`, `StormAggregationStatus` added to `api/remediation/v1alpha1/` |
| ADR-049 acknowledged | ✅ **DONE** | Gateway imports from `api/remediation/v1alpha1/` |
| DD-GATEWAY-008 v2.0 | ✅ **DONE** | Async storm aggregation documented |
| Rate limiting removed | ✅ **DONE** | ADR-048 implemented |
| DD-015 timestamp naming | ✅ **DONE** | CRD names now include timestamp (fixed 2025-12-07) |

---

## 📆 **Day-by-Day Implementation Breakdown**

### **Day 0: ANALYSIS + PLAN (Pre-Work) ✅**

**Phase**: ANALYSIS + PLAN
**Duration**: 4 hours
**Status**: ✅ COMPLETE (this document represents Day 0 completion)

**Deliverables**:
- ✅ Analysis document: Files identified, impact assessed
- ✅ Implementation plan (this document v1.0): 8-day timeline
- ✅ Risk assessment: Conflict handling, latency impact
- ✅ Existing code review: 5 files identified
- ✅ BR coverage matrix: 5 primary BRs mapped

---

### **Day 1: Deduplication Status Tests (DO-RED Phase)**

**Phase**: DO-RED
**Duration**: 8 hours
**TDD Focus**: Write failing tests for `status.deduplication`

**⚠️ CRITICAL**: We are **ENHANCING existing code**, not creating from scratch!

**Existing Code to Enhance**:
- ✅ `pkg/gateway/processing/crd_updater.go` (~150 LOC) - Updates `spec.Deduplication.OccurrenceCount`
- ✅ `pkg/gateway/processing/deduplication.go` (~300 LOC) - Reads `spec.Deduplication`
- ✅ `pkg/gateway/processing/crd_creator.go` (~400 LOC) - Sets initial `spec.Deduplication`

**Morning (4 hours): Test Framework Setup + Code Analysis**

1. **Analyze existing implementation** (1 hour)
   - Read `crd_updater.go` - understand current spec update logic
   - Read `deduplication.go` - understand deduplication decision logic
   - Identify status update patterns from RO team (see shared doc)

2. **Create test file** `test/unit/gateway/deduplication_status_test.go` (300-400 LOC)
   - Set up Ginkgo/Gomega test suite
   - Define test fixtures for status-based deduplication
   - Create helper functions for RR status manipulation

**Afternoon (4 hours): Write Failing Tests**

3. **Write failing tests** (strict TDD: ONE test at a time)

```go
// test/unit/gateway/deduplication_status_test.go
package gateway

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Deduplication Status (DD-GATEWAY-011)", func() {
    Context("when updating deduplication on duplicate signal", func() {
        It("should update status.deduplication.occurrenceCount (BR-GATEWAY-181)", func() {
            // Setup: Create RR with existing deduplication status
            rr := &remediationv1.RemediationRequest{
                ObjectMeta: metav1.ObjectMeta{Name: "test-rr", Namespace: "kubernaut-system"},
                Status: remediationv1.RemediationRequestStatus{
                    Deduplication: &remediationv1.DeduplicationStatus{
                        OccurrenceCount: 1,
                        FirstSeenAt:     &metav1.Time{Time: time.Now()},
                    },
                },
            }

            // BEHAVIOR: Update deduplication status
            err := updater.UpdateDeduplicationStatus(ctx, rr)

            // CORRECTNESS: Status updated, spec unchanged
            Expect(err).ToNot(HaveOccurred())
            Expect(rr.Status.Deduplication.OccurrenceCount).To(Equal(int32(2)))
            Expect(rr.Status.Deduplication.LastSeenAt).ToNot(BeNil())
        })

        It("should NOT update spec.deduplication (BR-GATEWAY-181)", func() {
            // Verify spec immutability is maintained
        })
    })

    Context("when checking for existing RR by fingerprint", func() {
        It("should return existing RR if non-terminal phase (BR-GATEWAY-184)", func() {
            // Test deduplication decision based on overallPhase
        })

        It("should allow new RR if existing RR is Completed (BR-GATEWAY-184)", func() {
            // Test terminal phase allows new RR
        })
    })
})
```

4. **Run tests → Verify they FAIL (RED phase)**

**EOD Deliverables**:
- ✅ Test framework complete
- ✅ 6-8 failing tests (RED phase)
- ✅ Day 1 EOD report

**Validation Commands**:
```bash
# Verify tests fail (RED phase)
go test ./test/unit/gateway/deduplication_status_test.go -v 2>&1 | grep "FAIL"

# Expected: All tests should FAIL with "not implemented yet"
```

---

### **Day 2: Deduplication Implementation (DO-GREEN Phase)**

**Phase**: DO-GREEN
**Duration**: 8 hours
**TDD Focus**: Minimal implementation to pass tests

**Morning (4 hours): Status Update Implementation**

1. **Modify `crd_updater.go`** - Change from spec to status update

**Before** (remove):
```go
// ❌ Don't update spec
rr.Spec.Deduplication.OccurrenceCount++
err := client.Update(ctx, rr)
```

**After** (implement):
```go
// ✅ Update status.deduplication only
import "k8s.io/client-go/util/retry"

// Gateway-specific retry config (approved by Architecture Team)
var GatewayRetry = wait.Backoff{
    Steps:    3,
    Duration: 10 * time.Millisecond,
    Factor:   2.0,
    Jitter:   0.1,
}

func (u *CRDUpdater) UpdateDeduplicationStatus(ctx context.Context, rr *remediationv1.RemediationRequest) error {
    return retry.RetryOnConflict(GatewayRetry, func() error {
        // Refetch to get latest resourceVersion
        if err := u.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
            return err
        }

        now := metav1.Now()
        if rr.Status.Deduplication == nil {
            rr.Status.Deduplication = &remediationv1.DeduplicationStatus{
                FirstSeenAt:     &now,
                OccurrenceCount: 1,
            }
        } else {
            rr.Status.Deduplication.OccurrenceCount++
        }
        rr.Status.Deduplication.LastSeenAt = &now

        return u.client.Status().Update(ctx, rr)
    })
}
```

2. **Run tests** → Verify they PASS (GREEN phase)

**Afternoon (4 hours): Deduplication Decision Logic**

3. **Modify `deduplication.go`** - Check status.OverallPhase instead of Redis

**Before** (remove):
```go
// ❌ Redis-based deduplication
if redis.Exists("dedup:" + fingerprint) {
    return DUPLICATE
}
```

**After** (implement):
```go
// ✅ Status-based deduplication
func (d *Deduplicator) ShouldDeduplicate(ctx context.Context, fingerprint string) (bool, *remediationv1.RemediationRequest, error) {
    rrList := &remediationv1.RemediationRequestList{}
    if err := d.client.List(ctx, rrList,
        client.MatchingLabels{"kubernaut.ai/fingerprint": fingerprint},
    ); err != nil {
        return false, nil, err
    }

    for _, rr := range rrList.Items {
        if !isTerminalPhase(rr.Status.OverallPhase) {
            return true, &rr, nil // Active RR exists → deduplicate
        }
    }
    return false, nil, nil // No active RR → create new
}

func isTerminalPhase(phase string) bool {
    return phase == "Completed" || phase == "Failed" || phase == "Cancelled"
}
```

4. **Run tests** → Verify all deduplication tests PASS

**EOD Deliverables**:
- ✅ `status.deduplication` updates working
- ✅ Spec immutability maintained
- ✅ Phase-based deduplication decisions working
- ✅ All Day 1 tests passing

---

### **Day 3: Storm Aggregation (DO-RED/GREEN Phase)**

**Phase**: DO-RED + DO-GREEN
**Duration**: 8 hours
**TDD Focus**: Storm aggregation in status

**Morning (4 hours): Storm Aggregation Tests (RED)**

1. **Create test file** `test/unit/gateway/storm_aggregation_status_test.go`

```go
var _ = Describe("Storm Aggregation Status (DD-GATEWAY-011)", func() {
    Context("when detecting storm on first alert", func() {
        It("should create RR with status.stormAggregation.aggregatedCount=1 (BR-GATEWAY-182)", func() {
            // Test async storm aggregation per DD-GATEWAY-008 v2.0
        })
    })

    Context("when subsequent alerts arrive", func() {
        It("should increment status.stormAggregation.aggregatedCount (BR-GATEWAY-182)", func() {
            // Test dedup + storm count increment
        })

        It("should set status.stormAggregation.isPartOfStorm=true at threshold (BR-GATEWAY-182)", func() {
            // Test storm threshold detection
        })
    })
})
```

2. **Run tests** → Verify they FAIL (RED phase)

**Afternoon (4 hours): Storm Aggregation Implementation (GREEN)**

3. **Modify `storm_aggregator.go`** - Move from Redis to status

**Before** (remove):
```go
// ❌ Don't store in Redis
redis.Set("storm:"+fingerprint, stormData, TTL)
```

**After** (implement):
```go
// ✅ Store in RR status
func (a *StormAggregator) UpdateStormStatus(ctx context.Context, rr *remediationv1.RemediationRequest, isThresholdReached bool) error {
    return retry.RetryOnConflict(GatewayRetry, func() error {
        if err := a.client.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
            return err
        }

        now := metav1.Now()
        if rr.Status.StormAggregation == nil {
            rr.Status.StormAggregation = &remediationv1.StormAggregationStatus{
                AggregatedCount: 1,
                StormDetectedAt: &now,
            }
        } else {
            rr.Status.StormAggregation.AggregatedCount++
        }

        if isThresholdReached {
            rr.Status.StormAggregation.IsPartOfStorm = true
        }

        return a.client.Status().Update(ctx, rr)
    })
}
```

4. **Run tests** → Verify all storm tests PASS

**EOD Deliverables**:
- ✅ `status.stormAggregation` updates working
- ✅ Async storm aggregation per DD-GATEWAY-008 v2.0
- ✅ All Day 3 tests passing

---

### **Day 4: Integration + Conflict Retry (DO-REFACTOR Phase)**

**Phase**: DO-REFACTOR
**Duration**: 8 hours
**TDD Focus**: Full integration and conflict handling

**Morning (4 hours): Server Integration**

1. **Modify `server.go`** - Wire up new status-based logic

```go
// In processSignal method
isDuplicate, existingRR, err := s.deduplicator.ShouldDeduplicate(ctx, signal.Fingerprint)
if err != nil {
    return nil, err
}

if isDuplicate {
    // Update existing RR's status.deduplication
    if err := s.crdUpdater.UpdateDeduplicationStatus(ctx, existingRR); err != nil {
        return nil, err
    }

    // Update storm aggregation if applicable
    isThreshold := s.stormDetector.IsThresholdReached(ctx, signal.Fingerprint)
    if err := s.stormAggregator.UpdateStormStatus(ctx, existingRR, isThreshold); err != nil {
        return nil, err
    }

    return NewDuplicateResponse(existingRR), nil
}

// Create new RR (first occurrence)
return s.createRemediationRequestCRD(ctx, signal, start)
```

**Afternoon (4 hours): Conflict Retry Validation**

2. **Create integration test** for conflict scenarios

```go
// test/integration/gateway/conflict_retry_test.go
var _ = Describe("Status Update Conflict Handling (BR-GATEWAY-183)", func() {
    Context("when Gateway and RO update status concurrently", func() {
        It("should retry on conflict and succeed", func() {
            // Simulate concurrent status updates
            // Verify retry.RetryOnConflict handles it
        })
    })
})
```

3. **Validate latency impact**
   - P95 latency <50ms with retry backoff (10ms → 20ms → 40ms)
   - Worst case: 70ms (within budget)

**EOD Deliverables**:
- ✅ Full integration with server.go
- ✅ Conflict retry logic validated
- ✅ Latency impact verified <50ms P95

---

### **Day 5: Unit Tests (CHECK Phase)**

**Phase**: CHECK
**Duration**: 8 hours
**Focus**: Comprehensive unit test coverage

**Target**: ≥70% unit test coverage for modified files

**Test Files**:
- `test/unit/gateway/deduplication_status_test.go` - 15+ tests
- `test/unit/gateway/storm_aggregation_status_test.go` - 10+ tests
- `test/unit/gateway/conflict_retry_test.go` - 8+ tests

**Validation Commands**:
```bash
# Run unit tests with coverage
go test ./test/unit/gateway/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total

# Expected: total coverage ≥70%
```

---

### **Day 6: Integration Tests (CHECK Phase)**

**Phase**: CHECK
**Duration**: 8 hours
**Focus**: Component interaction validation

**Test Scenarios**:
1. Deduplication with real K8s client (envtest)
2. Storm aggregation lifecycle
3. Concurrent status updates
4. Phase transition handling

**Test File**: `test/integration/gateway/dd_gateway_011_integration_test.go`

---

### **Day 7: E2E Tests (CHECK Phase)**

**Phase**: CHECK
**Duration**: 8 hours
**Focus**: End-to-end feature validation

**Critical Paths**:
1. Signal ingestion → Deduplication → Status update
2. Storm detection → Aggregation → RR status
3. Redis removal validation (no Redis calls)

**Test File**: `test/e2e/gateway/dd_gateway_011_e2e_test.go`

---

### **Day 8: Documentation (PRODUCTION Phase)**

**Phase**: PRODUCTION
**Duration**: 8 hours
**Focus**: Finalize documentation and knowledge transfer

**Deliverables**:
- ✅ Update `overview.md` - Add DD-GATEWAY-011 changelog entry
- ✅ Update `BUSINESS_REQUIREMENTS.md` - Mark BR-GATEWAY-181-185 as implemented
- ✅ Update `deduplication.md` - Document status-based approach
- ✅ Handoff summary - Lessons learned, known limitations

---

## 🧪 **TDD Do's and Don'ts - MANDATORY**

### **✅ DO: Strict TDD Discipline**

```go
// ✅ CORRECT: Test WHAT, not HOW
It("should increment occurrence count on duplicate", func() {
    err := updater.UpdateDeduplicationStatus(ctx, rr)
    Expect(err).ToNot(HaveOccurred())
    Expect(rr.Status.Deduplication.OccurrenceCount).To(Equal(int32(2)))
})
```

### **❌ DON'T: Anti-Patterns to Avoid**

```go
// ❌ WRONG: Testing implementation details
Expect(mockClient.StatusUpdateCallCount()).To(Equal(1))

// ❌ WRONG: Weak assertions
Expect(rr.Status.Deduplication).ToNot(BeNil())
```

---

## 📊 **Test File Locations - MANDATORY**

| Test Type | File Location |
|-----------|---------------|
| **Unit Tests** | `test/unit/gateway/deduplication_status_test.go` |
| **Unit Tests** | `test/unit/gateway/storm_aggregation_status_test.go` |
| **Integration Tests** | `test/integration/gateway/dd_gateway_011_integration_test.go` |
| **E2E Tests** | `test/e2e/gateway/dd_gateway_011_e2e_test.go` |

---

## 🎯 **BR Coverage Matrix**

| BR ID | Description | Unit Tests | Integration Tests | E2E Tests | Status |
|-------|-------------|------------|-------------------|-----------|--------|
| **BR-GATEWAY-181** | Deduplication in status | `storm_aggregation_status_test.go` | `dd_gateway_011_status_deduplication_test.go` | `02_state_based_deduplication_test.go` | ✅ |
| **BR-GATEWAY-182** | Storm aggregation in status | `storm_aggregation_status_test.go` | `dd_gateway_011_status_deduplication_test.go` | `02_state_based_deduplication_test.go` | ✅ |
| **BR-GATEWAY-183** | Conflict retry logic | `storm_aggregation_status_test.go` | `dd_gateway_011_status_deduplication_test.go` | - | ✅ |
| **BR-GATEWAY-184** | Phase-based deduplication | `storm_aggregation_status_test.go` | `dd_gateway_011_status_deduplication_test.go` | `02_state_based_deduplication_test.go` | ✅ |
| **BR-GATEWAY-185** | Redis deprecation support | All tests | All tests | All tests | ✅ |

---

## 📊 **Confidence Calculation**

**Overall Confidence**: 90% (Evidence-Based)

| Component | Confidence | Evidence |
|-----------|-----------|----------|
| **Status Update Pattern** | 95% | Industry-standard K8s pattern (Node, Ingress, Argo) |
| **Conflict Retry** | 90% | `retry.RetryOnConflict` battle-tested in K8s ecosystem |
| **API Types** | 100% | RO team provided types, verified import path |
| **Latency Impact** | 85% | 10ms backoff fits within 50ms P95 budget |

**Risk Assessment**:
- **10% Risk**: Conflict rate higher than expected in high-volume scenarios
- **Mitigation**: Monitor `gateway_status_update_retries_total` metric

---

## 🔄 **Rollback Plan**

### **Rollback Triggers**
- P95 latency >100ms (2x budget)
- Conflict rate >5%
- Data loss detected

### **Rollback Procedure**
1. Revert to spec-based deduplication (git revert)
2. Re-enable Redis for storm aggregation
3. Deploy previous version
4. Document rollback reason

---

## 📚 **References**

### **Architecture Documents**
- [DD-GATEWAY-011](../../../../architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md) - Design decision
- [DD-GATEWAY-008 v2.0](../../../../architecture/decisions/DD-GATEWAY-008-storm-aggregation-first-alert-handling.md) - Async storm aggregation
- [ADR-049](../../../../architecture/decisions/ADR-049-remediationrequest-crd-ownership.md) - RO owns RR schema

### **Handoff Documents**
- [NOTICE_SHARED_STATUS_OWNERSHIP_DD_GATEWAY_011](../../../../handoff/NOTICE_SHARED_STATUS_OWNERSHIP_DD_GATEWAY_011.md) - Full context

### **API Types**
- `api/remediation/v1alpha1/remediationrequest_types.go` - `DeduplicationStatus`, `StormAggregationStatus`

---

**Document Status**: ✅ **COMPLETE**
**Last Updated**: 2025-12-10
**Version**: 1.1
**Maintained By**: Gateway Service Team

