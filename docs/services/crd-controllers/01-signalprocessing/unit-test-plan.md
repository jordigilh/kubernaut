# SignalProcessing Unit Test Plan

**Version**: 1.5.0
**Created**: 2025-12-21
**Updated**: 2025-12-21
**Status**: ✅ **COMPLETE** - Target Exceeded
**Current Coverage**: **75.7%** (combined) ✅ EXCEEDS TARGET
**Target Coverage**: 70%+ (per TESTING_GUIDELINES.md)

---

## 📋 Changelog

### Version 1.6.0 (2026-02-05)
- **PLANNED**: BR-SP-106 Predictive Signal Mode Classification tests
- **TESTS PLANNED**: 9 unit tests (UT-SP-106-001 through UT-SP-106-009), 1 integration test (IT-SP-106-001)
- **NEW FILE**: `test/unit/signalprocessing/signalmode_classifier_test.go`
- **NEW PACKAGE**: `pkg/signalprocessing/classifier/signalmode.go`

### Version 1.5.0 (2025-12-21)
- **IMPLEMENTED**: Detection method unit tests (hasPDB, hasHPA, NetworkIsolated, detectLabels)
- **TESTS ADDED**: 10 new detection tests (CTRL-DETECT-01 through CTRL-DETECT-10)
- **COVERAGE**: Improved 74.8% → **75.7%** (+0.9%)
- **NOTE**: CTRL-DETECT-04 (HPA via owner chain) documented as integration scope

### Version 1.4.0 (2025-12-21)
- **IMPLEMENTED**: Controller reconciliation tests with fake client (ADR-004)
- **TESTS ADDED**: `controller_reconciliation_test.go` - 10 new tests
- **COVERAGE**: Improved 65.3% → **74.8%** (+9.5%)
- **STATUS**: ✅ Target exceeded (74.8% > 70% requirement)

### Version 1.3.0 (2025-12-21)
- **CORRECTION**: Controller code CAN be unit tested with fake client per ADR-004
- **GAP IDENTIFIED**: Controller methods at 0% coverage need unit tests
- **REMOVED**: Redundant `classification_logic_test.go` - classification tested in classifiers

### Version 1.2.0 (2025-12-21) - SUPERSEDED
- ~~Incorrectly claimed controller code requires integration tests~~
- **CORRECTED**: Per ADR-004, fake client is standard for controller unit tests

### Version 1.1.0 (2025-12-21)
- **IMPLEMENTED**: P4 quick wins (config, cache, metrics helpers) +2%
- **IMPLEMENTED**: P1 phase state machine tests +2.4%
- **IMPLEMENTED**: P2 enricher resource types tests +2.6%
- **IMPLEMENTED**: P3 hot reload lifecycle tests +2.8%
- **RESULT**: Coverage improved 59.1% → 65.3% (+6.2%)

### Version 1.0.0 (2025-12-21)
- **INITIAL**: Created unit test plan for SignalProcessing service

---

## 📊 Coverage Summary

### Unit Test Scope (per ADR-004)

Per **ADR-004: Fake Kubernetes Client for Unit Testing**:
- **Fake Client** (`sigs.k8s.io/controller-runtime/pkg/client/fake`) is the standard for unit testing controllers
- Controller reconciliation methods CAN and SHOULD be unit tested with fake client
- Other services (RO, WE, AIAnalysis) already test controllers with fake client

### Current State (per `go tool cover`)

| Package | Current Coverage | Target | Status |
|---------|------------------|--------|--------|
| `pkg/signalprocessing/...` | **83.6%** | 70% | ✅ EXCEEDS |
| `internal/controller/signalprocessing/...` | **~58%** | 70% | ⚠️ Partial (thin wrappers) |
| **Combined** | **75.7%** | 70% | ✅ EXCEEDS |

### Controller Coverage Improvements (ADR-004 Fake Client)

| Method | Before | After | Test File |
|--------|--------|-------|-----------|
| `Reconcile` | 28.9% | **55.6%** | `controller_reconciliation_test.go` |
| `reconcilePending` | 0% | **77.8%** | `controller_reconciliation_test.go` |
| `reconcileEnriching` | 57.1% | **67.5%** | `controller_reconciliation_test.go` |
| `reconcileClassifying` | 0% | **covered** | `controller_reconciliation_test.go` |
| `reconcileCategorizing` | 0% | **covered** | `controller_reconciliation_test.go` |
| `hasPDB` | 22.2% | **covered** | `controller_reconciliation_test.go` |
| `hasHPA` | 33.3% | **covered** | `controller_reconciliation_test.go` |
| `hasNetworkPolicy` | 80.0% | **covered** | `controller_reconciliation_test.go` |
| `detectLabels` | 46.7% | **covered** | `controller_reconciliation_test.go` |

> **Note**: Classification wrapper methods (`classifyEnvironment`, `assignPriority`, `classifyBusiness`)
> delegate to `pkg/signalprocessing/classifier/` which is tested at 95%+.
>
> **Note**: CTRL-DETECT-04 (HPA via owner chain) requires full OwnerChainBuilder integration
> and is tested in E2E (BR-SP-101) instead of unit tests.

### Coverage by File

| File | Current | Target | Status |
|------|---------|--------|--------|
| **pkg/signalprocessing/** | | | |
| `audit/client.go` | 80%+ | 70% | ✅ |
| `cache/cache.go` | 100% | 70% | ✅ |
| `classifier/business.go` | 95%+ | 70% | ✅ |
| `classifier/environment.go` | 88%+ | 70% | ✅ |
| `classifier/priority.go` | 80%+ | 70% | ✅ |
| `classifier/signalmode.go` | 0% | 70% | ⏸️ Planned (BR-SP-106) |
| `classifier/helpers.go` | 0% | 70% | ⚠️ (unused) |
| `conditions.go` | 100% | 70% | ✅ |
| `config/config.go` | 100% | 70% | ✅ |
| `detection/labels.go` | 85%+ | 70% | ✅ |
| `enricher/degraded.go` | 80%+ | 70% | ✅ |
| `enricher/k8s_enricher.go` | 75%+ | 70% | ✅ |
| `metrics/metrics.go` | 100% | 70% | ✅ |
| `ownerchain/builder.go` | 95%+ | 70% | ✅ |
| `rego/engine.go` | 90%+ | 70% | ✅ |
| **internal/controller/** | | | |
| `signalprocessing_controller.go` | **~7%** | 70% | ❌ NEEDS TESTS |

### Gap Analysis: Controller Unit Tests

Per **ADR-004**, controller methods should be unit tested with fake client. Current gaps:

```
reconcilePending        0.0%  ← Needs fake client tests
reconcileClassifying    0.0%  ← Needs fake client tests
reconcileCategorizing   0.0%  ← Needs fake client tests
enrichDeployment        0.0%  ← Needs fake client tests
enrichStatefulSet       0.0%  ← Needs fake client tests
enrichDaemonSet         0.0%  ← Needs fake client tests
enrichService           0.0%  ← Needs fake client tests
```

**Reference Pattern**: See `test/unit/remediationorchestrator/controller_test.go` for the correct approach.

---

## ✅ Existing Unit Tests

### Test File: `audit_client_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| BR-SP-090 | RecordSignalProcessed - completed signal with all classification data | ✅ |
| BR-SP-090 | RecordSignalProcessed - failed signal with failure outcome | ✅ |
| BR-SP-090 | RecordPhaseTransition - phase transition with from/to phases | ✅ |
| BR-SP-090 | RecordClassificationDecision - classification with all decisions | ✅ |
| BR-SP-090 | RecordEnrichmentComplete - enrichment with duration and context | ✅ |
| BR-SP-090 | RecordError - error with phase and error message | ✅ |
| Edge Cases | Handle nil classification fields gracefully | ✅ |
| Edge Cases | Handle empty owner chain | ✅ |
| Error Handling | Handle store error gracefully (fire-and-forget) | ✅ |

### Test File: `backoff_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| BR-SP-111 | Backoff calculation - 30s first attempt | ✅ |
| BR-SP-111 | Backoff calculation - 1m second attempt | ✅ |
| BR-SP-111 | Backoff calculation - 2m third attempt | ✅ |
| BR-SP-111 | Backoff calculation - cap at 5m | ✅ |
| BR-SP-111 | Custom config - conservative multiplier | ✅ |
| BR-SP-111 | Custom config - aggressive multiplier | ✅ |
| BR-SP-111 | Transient error detection - timeout errors | ✅ |
| BR-SP-111 | Transient error detection - context canceled | ✅ |
| BR-SP-111 | Jitter anti-thundering herd | ✅ |

### Test File: `business_classifier_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| BR-SP-002 | Classify payment service correctly | ✅ |
| BR-SP-002 | Classify API gateway via Rego | ✅ |
| BR-SP-002 | Classify background job correctly | ✅ |
| BR-SP-002 | Classify via team label | ✅ |
| BR-SP-002 | Classify via namespace pattern | ✅ |
| BR-SP-002 | Apply custom Rego business rules | ✅ |

### Test File: `cache_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| Cache | NewTTLCache creates cache with TTL | ✅ |
| Cache | Get/Set operations | ✅ |
| Cache | Delete operation | ✅ |
| Cache | Clear operation | ✅ |
| Cache | TTL expiration | ✅ |

### Test File: `conditions_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| BR-SP-006 | SetCondition - add new condition | ✅ |
| BR-SP-006 | SetCondition - update existing condition | ✅ |
| BR-SP-006 | GetCondition - returns correct condition | ✅ |
| BR-SP-006 | IsConditionTrue - true/false cases | ✅ |
| BR-SP-006 | SetEnrichmentComplete - sets condition correctly | ✅ |
| BR-SP-006 | SetClassificationComplete - sets condition correctly | ✅ |
| BR-SP-006 | SetCategorizationComplete - sets condition correctly | ✅ |
| BR-SP-006 | SetProcessingComplete - sets condition correctly | ✅ |

### Test File: `config_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| Config | Validate - valid config passes | ✅ |
| Config | Validate - invalid config fails | ✅ |

### Test File: `degraded_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| BR-SP-001 | BuildDegradedContext from signal labels | ✅ |
| BR-SP-001 | ValidateContextSize - within limits | ✅ |
| BR-SP-001 | validateLabels - valid labels | ✅ |

### Test File: `enricher_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| BR-SP-001 | NewK8sEnricher creates enricher | ✅ |
| BR-SP-001 | Enrich - Pod signal | ✅ |
| BR-SP-001 | Enrich - Deployment signal | ✅ |
| BR-SP-001 | Enrich - StatefulSet signal | ✅ |
| BR-SP-001 | Enrich - Service signal | ✅ |
| BR-SP-001 | Enrich - Node signal | ✅ |

### Test File: `environment_classifier_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| BR-SP-050 | Classify production environment | ✅ |
| BR-SP-050 | Classify staging environment | ✅ |
| BR-SP-050 | Classify development environment | ✅ |
| BR-SP-050 | Classify via namespace labels | ✅ |
| BR-SP-071 | Handle context cancellation gracefully | ✅ |

### Test File: `label_detector_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| BR-SP-101 | DetectLabels - detect GitOps | ✅ |
| BR-SP-101 | DetectLabels - detect PDB | ✅ |
| BR-SP-101 | DetectLabels - detect HPA | ✅ |
| BR-SP-101 | DetectLabels - detect NetworkPolicy | ✅ |
| BR-SP-101 | DetectLabels - detect Helm managed | ✅ |
| BR-SP-101 | DetectLabels - detect Service Mesh | ✅ |
| BR-SP-101 | DetectLabels - detect Stateful | ✅ |

### Test File: `metrics_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| BR-SP-008 | NewMetrics registers all metrics | ✅ |
| BR-SP-008 | IncrementProcessingTotal | ✅ |
| BR-SP-008 | RecordEnrichmentError | ✅ |
| BR-SP-008 | ObserveProcessingDuration | ✅ |

### Test File: `ownerchain_builder_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| BR-SP-100 | Build - Pod with ReplicaSet owner | ✅ |
| BR-SP-100 | Build - Pod with Deployment owner chain | ✅ |
| BR-SP-100 | Build - StatefulSet owner | ✅ |
| BR-SP-100 | Build - cluster-scoped resources | ✅ |
| BR-SP-100 | getControllerOwner - returns controller ref | ✅ |

### Test File: `priority_engine_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| BR-SP-051 | Assign - critical severity to P1 | ✅ |
| BR-SP-051 | Assign - high severity to P2 | ✅ |
| BR-SP-051 | Assign - via Rego policy | ✅ |
| BR-SP-051 | buildRegoInput - correct structure | ✅ |

### Test File: `rego_engine_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| Rego | NewEngine creates engine | ✅ |
| Rego | LoadPolicy - valid policy | ✅ |
| Rego | LoadPolicy - invalid policy fails | ✅ |
| Rego | EvaluatePolicy - returns result | ✅ |
| Rego | validateAndSanitize - valid input | ✅ |
| Rego | hasReservedPrefix detection | ✅ |

### Test File: `rego_security_wrapper_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| Security | Prevent unsafe input injection | ✅ |
| Security | Prevent policy override attempts | ✅ |

### Test File: `controller_error_handling_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| BR-SP-111 | isTransientError - identifies transient errors | ✅ |

### Test File: `controller_shutdown_test.go`

| BR/ID | Scenario | Status |
|-------|----------|--------|
| DD-007 | Graceful shutdown flushes audit store | ✅ |

---

## ❌ Missing Unit Tests (0% Coverage)

### Priority 1: Controller Functions (High Business Impact)

| Function | File | Lines | Priority | New Test Count |
|----------|------|-------|----------|----------------|
| `reconcilePending` | controller.go | 215 | P1 | 3 |
| `reconcileClassifying` | controller.go | 457 | P1 | 3 |
| `reconcileCategorizing` | controller.go | 511 | P1 | 3 |
| `classifyEnvironment` | controller.go | 779 | P1 | 2 |
| `assignPriority` | controller.go | 816 | P1 | 2 |
| `classifyBusiness` | controller.go | 892 | P1 | 2 |

### Priority 2: Enricher Functions (Medium Business Impact)

| Function | File | Lines | Priority | New Test Count |
|----------|------|-------|----------|----------------|
| `enrichDaemonSetSignal` | k8s_enricher.go | 253 | P2 | 2 |
| `enrichReplicaSetSignal` | k8s_enricher.go | 283 | P2 | 2 |
| `buildOwnerChain` (enricher) | k8s_enricher.go | 372 | P2 | 2 |
| `getDaemonSet` | k8s_enricher.go | 468 | P2 | 1 |
| `getReplicaSet` | k8s_enricher.go | 477 | P2 | 1 |
| `convertPodDetails` | k8s_enricher.go | 495 | P2 | 2 |
| `convertDaemonSetDetails` | k8s_enricher.go | 581 | P2 | 1 |
| `convertReplicaSetDetails` | k8s_enricher.go | 592 | P2 | 1 |

### Priority 3: Hot Reload & Lifecycle (Medium Impact)

| Function | File | Lines | Priority | New Test Count |
|----------|------|-------|----------|----------------|
| `StartHotReload` (rego) | engine.go | 294 | P3 | 2 |
| `Stop` (rego) | engine.go | 320 | P3 | 1 |
| `GetPolicyHash` (rego) | engine.go | 328 | P3 | 1 |
| `StartHotReload` (environment) | environment.go | 328 | P3 | 2 |
| `Stop` (environment) | environment.go | 362 | P3 | 1 |
| `ReloadConfigMap` | environment.go | 315 | P3 | 2 |
| `StartHotReload` (priority) | priority.go | 207 | P3 | 2 |
| `Stop` (priority) | priority.go | 240 | P3 | 1 |

### Priority 4: Config & Helpers (Low Impact)

| Function | File | Lines | Priority | New Test Count |
|----------|------|-------|----------|----------------|
| `DefaultControllerConfig` | config.go | 73 | P4 | 1 |
| `LoadFromFile` | config.go | 83 | P4 | 2 |
| `extractConfidence` | helpers.go | 24 | P4 | 2 |
| `Len` (cache) | cache.go | 92 | P4 | 1 |
| `NewMetricsWithRegistry` | metrics.go | 67 | P4 | 1 |

---

## 📝 New Test Scenarios

### P1: Controller Reconciliation Tests

```go
// test/unit/signalprocessing/controller_reconciliation_test.go

var _ = Describe("Controller Reconciliation", func() {
    Describe("reconcilePending", func() {
        Context("Happy Path", func() {
            It("CTRL-PEND-01: should transition to Enriching phase", func() {})
            It("CTRL-PEND-02: should record phase transition audit", func() {})
        })
        Context("Error Handling", func() {
            It("CTRL-PEND-03: should handle missing signal gracefully", func() {})
        })
    })

    Describe("reconcileClassifying", func() {
        Context("Happy Path", func() {
            It("CTRL-CLASS-01: should classify environment via Rego", func() {})
            It("CTRL-CLASS-02: should classify priority via Rego", func() {})
            It("CTRL-CLASS-03: should transition to Categorizing phase", func() {})
        })
    })

    Describe("reconcileCategorizing", func() {
        Context("Happy Path", func() {
            It("CTRL-CAT-01: should classify business context", func() {})
            It("CTRL-CAT-02: should transition to Completed phase", func() {})
            It("CTRL-CAT-03: should emit completion audit event", func() {})
        })
    })
})
```

### P2: Enricher Resource Type Tests

```go
// test/unit/signalprocessing/enricher_resource_types_test.go

var _ = Describe("K8sEnricher Resource Types", func() {
    Describe("enrichDaemonSetSignal", func() {
        It("ENRICH-DS-01: should enrich DaemonSet with node selector", func() {})
        It("ENRICH-DS-02: should handle missing DaemonSet gracefully", func() {})
    })

    Describe("enrichReplicaSetSignal", func() {
        It("ENRICH-RS-01: should enrich ReplicaSet with replicas", func() {})
        It("ENRICH-RS-02: should build owner chain to Deployment", func() {})
    })

    Describe("buildOwnerChain (enricher)", func() {
        It("ENRICH-OC-01: should traverse full ownership chain", func() {})
        It("ENRICH-OC-02: should handle circular references safely", func() {})
    })

    Describe("convertPodDetails", func() {
        It("ENRICH-POD-01: should convert all pod fields", func() {})
        It("ENRICH-POD-02: should handle nil containers gracefully", func() {})
    })

    Describe("convertDaemonSetDetails", func() {
        It("ENRICH-DSD-01: should convert DaemonSet to ResourceDetails", func() {})
    })

    Describe("convertReplicaSetDetails", func() {
        It("ENRICH-RSD-01: should convert ReplicaSet to ResourceDetails", func() {})
    })
})
```

### P3: Hot Reload Lifecycle Tests

```go
// test/unit/signalprocessing/hot_reload_lifecycle_test.go

var _ = Describe("Hot Reload Lifecycle", func() {
    Describe("Rego Engine Hot Reload", func() {
        It("HR-REGO-01: StartHotReload should watch policy file", func() {})
        It("HR-REGO-02: should reload policy on file change", func() {})
        It("HR-REGO-03: Stop should cleanup watchers", func() {})
        It("HR-REGO-04: GetPolicyHash should return current hash", func() {})
    })

    Describe("Environment Classifier Hot Reload", func() {
        It("HR-ENV-01: StartHotReload should watch policy", func() {})
        It("HR-ENV-02: ReloadConfigMap should update mappings", func() {})
        It("HR-ENV-03: Stop should cleanup resources", func() {})
    })

    Describe("Priority Engine Hot Reload", func() {
        It("HR-PRI-01: StartHotReload should watch policy", func() {})
        It("HR-PRI-02: Stop should cleanup watchers", func() {})
    })
})
```

### P4: Config & Helpers Tests

```go
// test/unit/signalprocessing/config_helpers_test.go

var _ = Describe("Config & Helpers", func() {
    Describe("DefaultControllerConfig", func() {
        It("CFG-01: should return config with default values", func() {})
    })

    Describe("LoadFromFile", func() {
        It("CFG-02: should load config from YAML file", func() {})
        It("CFG-03: should return error for missing file", func() {})
    })

    Describe("extractConfidence", func() {
        It("HELP-01: should extract confidence from result map", func() {})
        It("HELP-02: should return default for missing confidence", func() {})
    })

    Describe("cache.Len", func() {
        It("CACHE-LEN-01: should return correct item count", func() {})
    })

    Describe("NewMetricsWithRegistry", func() {
        It("METRICS-REG-01: should register metrics to custom registry", func() {})
    })
})
```

---

## 📈 Coverage Improvement Estimate

| Priority | New Tests | Estimated Coverage Gain |
|----------|-----------|------------------------|
| P1: Controller | 15 | +15% |
| P2: Enricher | 12 | +8% |
| P3: Hot Reload | 10 | +5% |
| P4: Config/Helpers | 7 | +2% |
| **Total** | **44** | **+30%** |

**Projected Coverage**: 59.1% + 30% = **~89%** (exceeds 70% target)

---

## 🔄 Implementation Order

### Phase 1: Quick Wins (P4 - 1 day)
- [ ] `config_helpers_test.go` - 7 tests
- Expected gain: +2%

### Phase 2: Core Business Logic (P1 - 2 days)
- [ ] `controller_reconciliation_test.go` - 15 tests
- Expected gain: +15%

### Phase 3: Enricher Coverage (P2 - 1 day)
- [ ] `enricher_resource_types_test.go` - 12 tests
- Expected gain: +8%

### Phase 4: Lifecycle (P3 - 1 day)
- [ ] `hot_reload_lifecycle_test.go` - 10 tests
- Expected gain: +5%

---

## BR-SP-106: Predictive Signal Mode Classification

**Test File**: `test/unit/signalprocessing/signalmode_classifier_test.go` (new)

| Test ID | Scenario | Status |
|---------|----------|--------|
| UT-SP-106-001 | Classify PredictedOOMKill as predictive + normalize to OOMKilled | ✅ Passed |
| UT-SP-106-002 | Classify OOMKilled as reactive (unchanged) | ✅ Passed |
| UT-SP-106-003 | Classify unmapped type as reactive (default) | ✅ Passed |
| UT-SP-106-004 | Preserve OriginalSignalType for predictive signals | ✅ Passed |
| UT-SP-106-005 | Empty/nil signal type handling | ✅ Passed |
| UT-SP-106-006 | Config loading from YAML file | ✅ Passed |
| UT-SP-106-007 | Hot-reload config change | ✅ Passed |

**Integration Test**: `test/integration/signalprocessing/`

| Test ID | Scenario | Status |
|---------|----------|--------|
| IT-SP-106-001 | Enrichment pipeline sets SignalMode + normalized SignalType in SP status | ✅ Passed |

**Audit Test**: `test/unit/signalprocessing/audit_client_test.go` (extend)

| Test ID | Scenario | Status |
|---------|----------|--------|
| UT-SP-106-008 | RecordClassificationDecision includes signal_mode in payload | ✅ Passed |
| UT-SP-106-009 | RecordSignalProcessed includes signal_mode + original_signal_type in payload | ✅ Passed |

**References**: [BR-SP-106](../../../requirements/BR-SP-106-predictive-signal-mode-classification.md), [ADR-054](../../../architecture/decisions/ADR-054-predictive-signal-mode-classification.md)

---

## ✅ Acceptance Criteria

1. **Coverage**: Unit test coverage ≥ 70% (target: 89%)
2. **All Tests Pass**: `make test-unit-signalprocessing` exits 0
3. **No Flaky Tests**: All tests deterministic (no `time.Sleep()`)
4. **BR Mapping**: All tests map to business requirements
5. **Error Paths**: All error handling paths covered

---

## 📚 References

- [TESTING_GUIDELINES.md](../../../development/business-requirements/TESTING_GUIDELINES.md)
- [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc)
- [WorkflowExecution testing-strategy.md](../03-workflowexecution/testing-strategy.md) (reference)

