# Dynamic Toolset Implementation - Checklist with Gateway Learnings

**Status**: Day 1 Complete
**Triage Reference**: `PLAN_TRIAGE_VS_GATEWAY.md`

---

## Critical Gaps to Apply (From Gateway Triage)

### Gap 1: Integration-First Testing Strategy ✅ PLANNED
**Action**: Day 8 Morning - Write 5 critical integration tests BEFORE unit tests
**Status**: Scheduled for Day 8
**Tests**:
1. Service Discovery → ConfigMap Creation
2. Health Check Validation  
3. ConfigMap Reconciliation (drift recovery)
4. Override Preservation
5. Multi-Detector Integration

### Gap 2: Schema Validation Before Testing 📋 PENDING
**Action**: Add to Day 7 (end of day)
**Status**: Will create `design/01-configmap-schema-validation.md`
**Validate**:
- [ ] All toolset YAML fields match HolmesGPT SDK expectations
- [ ] Override preservation format documented
- [ ] Environment variable placeholder syntax validated
- [ ] ConfigMap metadata confirmed

### Gap 3: Handoff/Status Documentation ✅ IN PROGRESS
**Action**: Daily status docs
**Status**: Day 1 complete (`01-day1-complete.md`)
**Schedule**:
- [x] Day 1: 01-day1-complete.md
- [ ] Day 4: 02-day4-midpoint.md
- [ ] Day 7: 03-day7-complete.md
- [ ] Day 12: 00-HANDOFF-SUMMARY.md

### Gap 4: Test Coverage by BR Tracking 📋 PENDING
**Action**: Create BR coverage matrix on Day 9
**Status**: Planned
**File**: `implementation/testing/BR-COVERAGE-MATRIX.md`

### Gap 5: Production Readiness Checklist 📋 PENDING
**Action**: Expand Day 12 with comprehensive checklist
**Status**: Planned
**Items**:
- All 5 detectors tested
- ConfigMap reconciliation with concurrent updates
- Health checks (Redis, K8s unavailability)
- Metrics validation
- Authentication testing
- Performance targets

### Gap 6: Test Infrastructure Pre-Setup 📋 PLANNED
**Action**: Move from Day 10 to Day 7 (end of day)
**Status**: Scheduled
**Setup**: envtest, ConfigMap namespace, Service mocks

### Gap 7: File Organization Strategy 📋 PENDING
**Action**: Add to Day 12
**Status**: Planned
**File**: `implementation/FILE_ORGANIZATION_PLAN.md`

### Gap 8: Error Handling Documentation 📋 PENDING
**Action**: Add to Day 6 (DO-REFACTOR)
**Status**: Planned
**File**: Expand `pkg/toolset/errors.go` with graceful degradation philosophy

---

## Enhancements to Apply

### Enhancement 1: Early-Start Testing Assessment 📋 PENDING
**Action**: Create on Day 7 (end)
**File**: `testing/01-integration-first-rationale.md`

### Enhancement 2: Daily Progress Tracking ✅ IN PROGRESS
**Status**: Doing this (Day 1 complete)

### Enhancement 3: Design Decision Documentation 📋 PLANNED
**Actions**:
- Day 1: `design/01-detector-interface-design.md`
- Day 4: `design/02-discovery-loop-architecture.md`
- Day 6: `design/03-reconciliation-strategy.md`

### Enhancement 5: Metrics Validation 📋 PLANNED
**Action**: Add to Day 8 (after metrics implementation)
**Test**: `curl http://localhost:9090/metrics | grep dynamictoolset`

### Enhancement 9: Performance Benchmarking 📋 PLANNED
**Action**: Add to Day 12
**File**: `implementation/PERFORMANCE_REPORT.md`

### Enhancement 10: Troubleshooting Guide 📋 PLANNED
**Action**: Create on Day 12
**File**: `implementation/TROUBLESHOOTING_GUIDE.md`

---

## Revised Day Schedule with Gaps Applied

### Day 1: ✅ COMPLETE
- [x] Foundation complete
- [x] Status documentation created

### Day 2: Prometheus + Grafana Detectors
- [ ] Implementation
- [ ] DO-REFACTOR: Health validator extraction
- [ ] EOD: Brief status update to day1-complete.md

### Day 3: Jaeger + Elasticsearch Detectors  
- [ ] Implementation
- [ ] DO-REFACTOR: Detector standardization

### Day 4: Custom + Discovery Orchestration
- [ ] Implementation
- [ ] DO-REFACTOR: Discovery pipeline
- [ ] **EOD: Create 02-day4-midpoint.md** ⭐

### Day 5: Generators + ConfigMap Builder
- [ ] Implementation
- [ ] DO-REFACTOR: Generator standardization

### Day 6: Reconciliation Controller
- [ ] Implementation
- [ ] **DO-REFACTOR: Error handling + philosophy doc** ⭐

### Day 7: HTTP Server + REST API
- [ ] Implementation
- [ ] DO-REFACTOR: Middleware standardization
- [ ] **EOD: Schema validation checkpoint** ⭐
- [ ] **EOD: Test infrastructure setup** ⭐
- [ ] **EOD: Create 03-day7-complete.md** ⭐
- [ ] **EOD: Create testing/01-integration-first-rationale.md** ⭐

### Day 8: Tests (Integration-First) ⭐ REVISED
- [ ] **Morning: 5 Critical Integration Tests (4h)** ⭐
- [ ] Afternoon: Unit Tests - Detectors (4h)
- [ ] Metrics validation checkpoint

### Day 9: More Tests
- [ ] Morning: Unit Tests - Generators + Reconciler (4h)
- [ ] Afternoon: Unit Tests - Server + Handlers (4h)
- [ ] **Create BR-COVERAGE-MATRIX.md** ⭐

### Day 10-11: Advanced Integration + E2E
- [ ] Advanced integration tests
- [ ] E2E tests with Kind cluster
- [ ] Documentation

### Day 12: CHECK Phase ⭐ EXPANDED
- [ ] CHECK phase validation
- [ ] **Production readiness checklist** ⭐
- [ ] **File organization plan** ⭐
- [ ] **Performance benchmarking** ⭐
- [ ] **Troubleshooting guide** ⭐
- [ ] **Confidence assessment** ⭐
- [ ] **Create 00-HANDOFF-SUMMARY.md** ⭐

---

## Integration-First Testing Details (Day 8 Morning)

### Test 1: Basic Discovery → ConfigMap (90 min)
```
Setup: envtest + ConfigMap namespace
Deploy: Mock Prometheus service
Action: Run discovery
Verify: ConfigMap created with prometheus-toolset.yaml
```

### Test 2: Health Check Validation (45 min)
```
Setup: Mock service with unhealthy endpoint
Action: Run discovery
Verify: Service skipped, not in ConfigMap
```

### Test 3: ConfigMap Reconciliation (60 min)
```
Setup: Existing ConfigMap
Action: Manually modify/delete
Verify: Reconciler restores desired state
```

### Test 4: Override Preservation (45 min)
```
Setup: ConfigMap with overrides.yaml
Action: Run reconciliation
Verify: overrides.yaml preserved
```

### Test 5: Multi-Detector Integration (30 min)
```
Setup: Prometheus + Grafana services
Action: Run discovery
Verify: Both toolsets in ConfigMap
```

**Total**: 4 hours
**Result**: Architecture validated before detailed unit testing

---

## Status Indicators

- ✅ Applied/Complete
- 📋 Pending/Scheduled
- ⚠️ Needs Attention
- ⭐ Critical Change from Triage

---

**Next Action**: Continue with Day 2 implementation, keeping these enhancements in mind

