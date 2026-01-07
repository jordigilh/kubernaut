# SignalProcessing: Duplicate Metrics Registration Panic Fix

**Date**: December 28, 2025
**Author**: AI Assistant
**Status**: ✅ **RESOLVED** (78/81 tests passing, +78 from baseline)
**Priority**: P0 - CRITICAL (Test suite completely blocked by panic)

---

## Executive Summary

Fixed a duplicate Prometheus metrics registration panic that was preventing ALL SignalProcessing integration tests from running. The test suite now successfully executes **78 of 81 tests** (96% pass rate), up from 0 tests due to the blocking panic.

### Results
- **Before**: 0/81 tests executed (panic in `SynchronizedBeforeSuite`)
- **After**: 78/81 tests passing (3 metrics tests remain failing, but executing)
- **Impact**: Unblocked test suite, enabling further development and validation

---

## Problem Description

### Root Cause
The test suite was creating TWO separate instances of the Prometheus metrics by calling `spmetrics.NewMetrics()` twice:

```go
// Line 445: First call
enricherMetrics := spmetrics.NewMetrics()
k8sEnricher := enricher.NewK8sEnricher(k8sManager.GetClient(), logger, enricherMetrics, 5*time.Second)

// Line 455: Second call
controllerMetrics := spmetrics.NewMetrics()
```

### Why This Caused a Panic
`NewMetrics()` registers 5 metrics to `prometheus.DefaultRegisterer` (the global Prometheus registry):
1. `signalprocessing_processing_total`
2. `signalprocessing_processing_duration_seconds`
3. `signalprocessing_enrichment_total`
4. `signalprocessing_enrichment_duration_seconds`
5. `signalprocessing_enrichment_errors_total`

The second call tried to register the same metrics again, causing:
```
panic: duplicate metrics collector registration attempted
```

---

## Solution Implemented

### Code Changes

**File**: `test/integration/signalprocessing/suite_test.go`

**Changed From**:
```go
// Initialize K8sEnricher (BR-SP-001)
enricherMetrics := spmetrics.NewMetrics()
k8sEnricher := enricher.NewK8sEnricher(k8sManager.GetClient(), logger, enricherMetrics, 5*time.Second)

// Initialize StatusManager
statusManager := spstatus.NewManager(k8sManager.GetClient())

// Initialize Metrics (DD-005)
controllerMetrics := spmetrics.NewMetrics()  // ❌ DUPLICATE!
```

**Changed To**:
```go
// Initialize Metrics (DD-005: Observability)
// CRITICAL: Create metrics instance ONCE and share between enricher + controller
// Multiple calls to NewMetrics() would cause duplicate registration panic
sharedMetrics := spmetrics.NewMetrics() // No args = uses global prometheus.DefaultRegisterer

// Initialize K8sEnricher (BR-SP-001)
// Shares metrics instance with controller to avoid duplicate registration
k8sEnricher := enricher.NewK8sEnricher(k8sManager.GetClient(), logger, sharedMetrics, 5*time.Second)

// Initialize StatusManager
statusManager := spstatus.NewManager(k8sManager.GetClient())
```

**Controller Initialization**:
```go
err = (&signalprocessing.SignalProcessingReconciler{
    // ... other fields ...
    Metrics:            sharedMetrics,     // DD-005: Observability (shared with enricher)
    // ... other fields ...
    K8sEnricher:        k8sEnricher,       // BR-SP-001: K8s context enrichment
}).SetupWithManager(k8sManager)
```

### Pattern Established
✅ **Shared Metrics Instance**: Both controller and enricher use the same `Metrics` instance
✅ **Single Registration**: Metrics are registered to `prometheus.DefaultRegisterer` only once
✅ **Clean Architecture**: Follows AIAnalysis pattern for global registry usage

---

## Validation

### Test Execution Results
```bash
$ make test-integration-signalprocessing
```

**Output**:
```
Ran 81 of 81 Specs in 167.526 seconds
FAIL! -- 78 Passed | 3 Failed | 0 Pending | 0 Skipped
```

### Passing Test Categories
✅ **Core Business Logic**: All SignalProcessing lifecycle tests passing
✅ **Hot-Reload Integration**: ConfigMap change detection working
✅ **Audit Integration**: All audit trace tests passing (with 90s timeouts for DataStorage bug)
✅ **Classification Logic**: Environment, priority, and business classification tests passing
✅ **K8s Integration**: Owner chain, label detection, and Rego engine tests passing

---

## Remaining Work

### 3 Failing Metrics Tests
**Status**: Tests execute but assertions fail (not blocking other tests)

1. **Line 193**: `should emit processing metrics during successful Signal lifecycle`
2. **Line 261**: `should emit enrichment metrics during Pod enrichment`
3. **Line 313**: `should emit error metrics when enrichment encounters missing resources`

### Hypothesis on Remaining Failures
The metrics tests have the `Serial` label and query `prometheus.DefaultGatherer`. The controller registers metrics with `prometheus.DefaultRegisterer` in Process 1. Possible issues:

1. **Timing Issue**: Metrics might not be visible immediately after reconciliation
2. **Registry Synchronization**: Cross-process metric access might have subtle timing issues
3. **Test Isolation**: Parallel test execution might affect metric visibility despite `Serial` label

### Recommended Next Steps
1. Add debug logging to metrics tests to print gathered metrics
2. Verify controller metrics are actually being emitted (check controller logs)
3. Consider adding explicit synchronization between controller and test assertions
4. Investigate if `Serial` tests truly run in Process 1 where controller is active

---

## Technical Context

### Files Modified
- `test/integration/signalprocessing/suite_test.go` (metrics initialization)

### Related Design Decisions
- **DD-005**: Observability Standards (Prometheus metrics)
- **DD-TEST-002**: Integration Test Container Orchestration (parallel execution)
- **BR-SIGNALPROCESSING-OBSERVABILITY-001**: Metrics business requirement

### Pattern Validation
This fix aligns with established patterns:
- **AIAnalysis Pattern**: Single metrics instance shared across components
- **Gateway Pattern**: Global registry for integration tests
- **RemediationOrchestrator Pattern**: `NewMetrics()` uses `prometheus.DefaultRegisterer`

---

## Confidence Assessment

**Confidence**: 95%

**Justification**:
- ✅ Root cause identified through stack trace analysis
- ✅ Fix eliminates duplicate registration panic completely
- ✅ 78/81 tests now passing (from 0/81)
- ✅ Test execution time reasonable (167.5s for 81 tests)
- ✅ No regression in non-metrics tests

**Risks**:
- 3 metrics tests still failing (requires further investigation)
- Metrics cross-process visibility in parallel execution needs validation

---

## References

- **Stack Trace**: `prometheus/registry.go:406` - `duplicate metrics collector registration attempted`
- **Test File**: `test/integration/signalprocessing/metrics_integration_test.go`
- **Metrics Package**: `pkg/signalprocessing/metrics/metrics.go`
- **AIAnalysis Triage**: Documented prometheus.DefaultRegisterer pattern across services

---

## Handoff Notes

**For SignalProcessing Team (@jgil)**:
1. ✅ Test suite unblocked - 78/81 tests passing
2. ⚠️ 3 metrics tests need investigation (not blocking)
3. 📊 Test execution validated on macOS with Podman
4. 🔍 Consider adding metrics visibility debug logging for troubleshooting

**For Future Development**:
- Shared metrics pattern is now established - do not create multiple `Metrics` instances
- Document this pattern in testing guidelines to prevent regression
- Consider creating a helper function to enforce single metrics instance creation












