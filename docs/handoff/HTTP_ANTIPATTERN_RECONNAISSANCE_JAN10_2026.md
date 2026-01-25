# HTTP Anti-Pattern Refactoring - Reconnaissance Summary

**Date**: January 10, 2026
**Status**: ✅ RECONNAISSANCE COMPLETE
**Reference**: `HTTP_ANTIPATTERN_TRIAGE_JAN10_2026.md`

---

## 🔍 Current Test Structure

### Gateway Service

**E2E Tests** (`test/e2e/gateway/`):
- **Count**: 20 tests
- **Numbering**: 02-21 (sequential)
- **Next Available**: 22-36 (for 15 new tests)
- **Files**:
  ```
  02_state_based_deduplication_test.go
  03_k8s_api_rate_limit_test.go
  04_metrics_endpoint_test.go
  05_multi_namespace_isolation_test.go
  06_concurrent_alerts_test.go
  07_health_readiness_test.go
  08_k8s_event_ingestion_test.go
  09_signal_validation_test.go
  10_crd_creation_lifecycle_test.go
  11_fingerprint_stability_test.go
  12_gateway_restart_recovery_test.go
  13_redis_failure_graceful_degradation_test.go
  14_deduplication_ttl_expiration_test.go
  15_audit_trace_validation_test.go
  16_structured_logging_test.go
  17_error_response_codes_test.go
  18_cors_enforcement_test.go
  19_replay_attack_prevention_test.go
  20_security_headers_test.go
  21_crd_lifecycle_test.go
  ```

**Integration Tests** (`test/integration/gateway/`):
- **Count**: 23 tests (excluding suite_test.go)
- **Files from Triage** (20 tests):
  ```
  1.  adapter_interaction_test.go
  2.  audit_errors_integration_test.go
  3.  audit_integration_test.go
  4.  audit_signal_data_integration_test.go
  5.  cors_test.go
  6.  dd_gateway_011_status_deduplication_test.go
  7.  deduplication_edge_cases_test.go
  8.  deduplication_state_test.go
  9.  error_classification_test.go
  10. error_handling_test.go
  11. graceful_shutdown_foundation_test.go
  12. health_integration_test.go
  13. http_server_test.go ← LEGITIMATE (infrastructure)
  14. k8s_api_failure_test.go
  15. k8s_api_integration_test.go
  16. k8s_api_interaction_test.go
  17. observability_test.go
  18. prometheus_adapter_integration_test.go
  19. service_resilience_test.go
  20. webhook_integration_test.go
  ```

**🆕 NEW FINDING: 3 Additional Tests NOT in Triage**:
  ```
  21. priority1_adapter_patterns_test.go
  22. priority1_concurrent_operations_test.go
  23. priority1_edge_cases_test.go
  ```

**Question for Reporting Team**:
- ❓ Are these 3 `priority1_*` tests also HTTP anti-patterns?
- ❓ Should they be included in the refactoring scope?
- ❓ What do they test? (business logic or HTTP stack?)

---

### Notification Service

**E2E Tests** (`test/e2e/notification/`):
- **Count**: 7 tests
- **Numbering**: 01-07 (note: duplicate 04_*)
- **Next Available**: 08-09 (for 2 new TLS tests)
- **Files**:
  ```
  01_notification_lifecycle_audit_test.go
  02_audit_correlation_test.go
  03_file_delivery_validation_test.go
  04_failed_delivery_audit_test.go
  04_metrics_validation_test.go           ← DUPLICATE NUMBER!
  06_multi_channel_fanout_test.go
  07_priority_routing_test.go
  ```

**Integration Tests** (`test/integration/notification/`):
- **Count**: 22 tests (matches triage)
- **TLS Tests to Move** (2 tests):
  ```
  slack_tls_integration_test.go
  tls_failure_scenarios_test.go
  ```

**🆕 FINDING: Duplicate Numbering Issue**:
- ⚠️ Two tests numbered `04_*` in E2E suite
- Should be renumbered before adding new tests
- Proposed fix: Renumber `04_metrics_validation_test.go` → `05_metrics_validation_test.go`
- Then adjust all subsequent numbers (06 → 07, 07 → 08)

---

### SignalProcessing Service

**Integration Tests** (`test/integration/signalprocessing/`):
- **Count**: 7 tests (matches triage)
- **HTTP Anti-Pattern Test** (1 test):
  ```
  audit_integration_test.go
  ```

**E2E Tests** (`test/e2e/signalprocessing/`):
- Status: Not checked (not needed for this refactoring if we refactor to query DB)

---

## 📊 Updated Scope Analysis

### Original Triage Scope
- Gateway: 20 tests
- SignalProcessing: 1 test
- Notification: 2 tests
- **Total**: 23 tests

### Actual Scope (After Reconnaissance)
- Gateway: **23 tests** (20 from triage + 3 priority1_*)
- SignalProcessing: 1 test
- Notification: 2 tests
- **Total**: **26 tests** (if priority1_* are anti-patterns)

### Questions for Reporting Team
1. ❓ Should I include the 3 `priority1_*` Gateway tests in the refactoring scope?
2. ❓ If yes, what category do they fall into (move to E2E vs refactor vs keep)?

---

## 🎯 Proposed E2E Numbering

### Gateway: 15 Tests to Move to E2E

**Destination**: `test/e2e/gateway/`
**Numbering**: 22-36 (continue from existing 21)

**Proposed Mapping**:
```
22_audit_errors_test.go                   ← audit_errors_integration_test.go
23_audit_emission_test.go                 ← audit_integration_test.go
24_audit_signal_data_test.go              ← audit_signal_data_integration_test.go
25_cors_middleware_test.go                ← cors_test.go
26_error_classification_test.go           ← error_classification_test.go
27_error_responses_test.go                ← error_handling_test.go
28_graceful_shutdown_test.go              ← graceful_shutdown_foundation_test.go
29_k8s_api_failures_test.go               ← k8s_api_failure_test.go
30_observability_metrics_test.go          ← observability_test.go
31_prometheus_adapter_test.go             ← prometheus_adapter_integration_test.go
32_service_resilience_test.go             ← service_resilience_test.go
33_webhook_processing_test.go             ← webhook_integration_test.go
34_deduplication_status_test.go           ← dd_gateway_011_status_deduplication_test.go
35_deduplication_edge_cases_test.go       ← deduplication_edge_cases_test.go
36_deduplication_state_test.go            ← deduplication_state_test.go
```

### Notification: 2 Tests to Move to E2E

**Destination**: `test/e2e/notification/`
**Numbering**: 08-09 (continue from existing 07, after fixing duplicate 04)

**Proposed Mapping**:
```
08_slack_tls_test.go                      ← slack_tls_integration_test.go
09_tls_failure_scenarios_test.go          ← tls_failure_scenarios_test.go
```

**Note**: Should renumber duplicate `04_metrics_validation_test.go` → `05_` first, then shift 06→07, 07→08 before adding new tests.

---

## 🔍 Gateway Business Logic Components (For Refactoring)

**Discovered Components** (for Q6 in questions document):

### 1. SignalAdapter Interface (`pkg/gateway/adapters/adapter.go`)
```go
// SignalAdapter converts source-specific signal formats to NormalizedSignal
type SignalAdapter interface {
    // Name returns the adapter identifier (e.g., "prometheus")
    Name() string

    // Parse converts source-specific raw payload to NormalizedSignal
    // Returns: (signal *types.NormalizedSignal, err error)
    Parse(ctx context.Context, rawPayload []byte) (*types.NormalizedSignal, error)
}
```

**Implementations**:
- `pkg/gateway/adapters/prometheus_adapter.go` - Prometheus alert adapter
- `pkg/gateway/adapters/kubernetes_event_adapter.go` - K8s event adapter

### 2. CRDCreator (`pkg/gateway/processing/crd_creator.go`)
```go
// CRDCreator converts NormalizedSignal to RemediationRequest CRD
type CRDCreator struct {
    k8sClient         k8s.ClientInterface
    logger            logr.Logger
    metrics           *metrics.Metrics
    fallbackNamespace string
    retryConfig       *config.RetrySettings
    clock             Clock
}

// Key Methods:
func NewCRDCreator(...) *CRDCreator
func (c *CRDCreator) CreateRemediationRequest(ctx context.Context, signal *types.NormalizedSignal, deduplicationResult *DeduplicationResult) (*remediationv1alpha1.RemediationRequest, error)
```

### 3. PhaseBasedDeduplicationChecker (`pkg/gateway/processing/phase_checker.go`)
```go
// PhaseBasedDeduplicationChecker checks for existing in-progress RRs by fingerprint
type PhaseBasedDeduplicationChecker struct {
    client client.Client
}

// Key Method:
func NewPhaseBasedDeduplicationChecker(k8sClient client.Client) *PhaseBasedDeduplicationChecker
func (c *PhaseBasedDeduplicationChecker) ShouldDeduplicate(ctx context.Context, namespace, fingerprint string) (bool, *remediationv1alpha1.RemediationRequest, error)
```

**Note**: Deduplication is now K8s CRD status-based (DD-GATEWAY-011), NOT Redis-based

### Proposed Refactoring Pattern (For 5 Core Tests)

**Current HTTP Anti-Pattern**:
```go
// ❌ ANTI-PATTERN: Full HTTP stack
testServer := httptest.NewServer(gatewayServer.Handler())
resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
Expect(resp.StatusCode).To(Equal(201))
```

**Target Direct Call Pattern**:
```go
// ✅ CORRECT: Direct business logic calls
var (
    adapter     adapters.SignalAdapter
    crdCreator  *processing.CRDCreator
    dedupSvc    DeduplicationService  // ← Need to discover
)

BeforeEach(func() {
    adapter = adapters.NewPrometheusAdapter()
    crdCreator = processing.NewCRDCreator(k8sClient, logger, metrics, namespace, retryConfig)
    // dedupSvc = ... (need to discover)
})

It("should process Prometheus alert", func() {
    // Step 1: Parse payload
    signal, err := adapter.Parse(ctx, rawPayload)
    Expect(err).ToNot(HaveOccurred())

    // Step 2: Check deduplication
    dedupResult, err := dedupSvc.CheckDuplicate(ctx, signal)
    Expect(err).ToNot(HaveOccurred())

    // Step 3: Create CRD
    rr, err := crdCreator.CreateRemediationRequest(ctx, signal, dedupResult)
    Expect(err).ToNot(HaveOccurred())
    Expect(rr.Name).To(HavePrefix("rr-"))

    // Step 4: Verify CRD in Kubernetes
    Eventually(func() bool {
        var created remediationv1alpha1.RemediationRequest
        err := k8sClient.Get(ctx, client.ObjectKey{Name: rr.Name, Namespace: rr.Namespace}, &created)
        return err == nil
    }).Should(BeTrue())
})
```

<function_calls>
<invoke name="run_terminal_cmd">
<parameter name="command">cd /Users/jgil/go/src/github.com/jordigilh/kubernaut && find pkg/gateway -type f -name "*.go" | grep -E "(adapter|dedup|manager)" | sort