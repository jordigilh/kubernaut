# Test Plan: Monitoring Metadata Label Filtering (Issue #191)

**Feature**: Monitoring metadata label filtering for Prometheus adapter target resource extraction
**Version**: 1.0
**Created**: 2026-02-17
**Author**: AI Assistant
**Status**: Implemented
**Branch**: `feature/demo-scenarios-v1.0`

**Authority**:
- [Issue #191](https://github.com/jordigilh/kubernaut/issues/191): Incorrect target extraction from monitoring metadata labels
- [BR-GATEWAY-184](../../requirements/BR-GATEWAY-184-target-resource-extraction-priority.md): Target Resource Extraction Priority Order

**Cross-References**:
- Testing Strategy: `.cursor/rules/03-testing-strategy.mdc`
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 1. Scope

### In Scope

- **FR-1 priority order**: Validate extraction priority for new/changed candidates (job_name, service with filter) and existing candidates (HPA, PDB, PVC, Deployment, StatefulSet, CronJob, Pod). DaemonSet and Node covered by existing tests in `resource_extraction_test.go` and `resource_extraction_business_test.go` (pre-Issue #191).
- **FR-4 excluded labels**: Validate `job`, `endpoint`, `instance` are not in the candidate list
- **FR-5 monitoring metadata filtering**: Validate `service` label filtering via `LabelFilter` interface
- **FR-6 job_name semantics**: Validate `job_name` (not `job`) identifies Kubernetes Job resources

### Out of Scope

- E2E tests (Kind cluster with real AlertManager firing alerts)
- HAPI prompt changes (none required per BR-GATEWAY-184)
- Signal Processing / AI Analysis / Workflow Execution (consume correct target from CRD)

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit tier**: Covers pure logic (pattern matching, candidate list, filter dispatch). ~100% of unit-testable code.
- **Integration tier**: Covers multi-component wiring (adapter parse -> filter -> signal -> CRD creation). ~87% of integration-testable code.

### Business Outcome Quality Bar

Tests validate **business outcomes** -- behavior, correctness, and data accuracy:

- **Behavior**: Does the pipeline target the correct workload resource for LLM investigation?
- **Correctness**: Are CRD `spec.targetResource` fields populated with the actual affected resource?
- **Accuracy**: Is the fingerprint computed from the correct target (deduplication integrity)?

### Anti-Pattern Compliance

All tests verified against documented anti-patterns (TESTING_GUIDELINES.md):

- **NO time.Sleep()**: All operations are synchronous (`adapter.Parse()` + `server.ProcessSignal()`)
- **NO Skip()**: All tests run unconditionally
- **NO direct audit infrastructure calls**: Tests verify business outcomes, not audit events
- **NO direct metrics method calls**: Tests verify resource extraction, not metrics counters
- **NO HTTP in integration tests**: Direct business logic calls (`adapter.Parse()`, `server.ProcessSignal()`), no `httptest.Server`

---

## 3. Unit Tests

**File**: `test/unit/gateway/adapters/prometheus_adapter_test.go`

### FR-1: Resource Kind Extraction Priority Order

| Test ID  | Scenario | Expected Outcome |
|----------|----------|------------------|
| GW-RE-01 | Alert with `horizontalpodautoscaler` + `pod` labels | Kind=HorizontalPodAutoscaler (HPA takes priority over pod) |
| GW-RE-02 | Alert with `deployment` + `pod` labels | Kind=Deployment |
| GW-RE-03 | Alert with `statefulset` + `pod` labels | Kind=StatefulSet |
| GW-RE-04 | Alert with `poddisruptionbudget` + `pod` labels | Kind=PodDisruptionBudget |
| GW-RE-06 | Alert with `persistentvolumeclaim` label | Kind=PersistentVolumeClaim |
| GW-RE-08 | Alert with `cronjob` + `pod` labels | Kind=CronJob |

### FR-2: Resource Name Extraction

FR-2 requires the resource name to come from the same label that determined the resource kind. This is structurally guaranteed by `extractTargetResource` which returns `(kind, name)` from a single candidate in one return statement. Every GW-RE test implicitly validates this by asserting both `signal.Resource.Kind` and `signal.Resource.Name` from the same alert payload.

### FR-3: Backward Compatibility

| Test ID  | Scenario | Expected Outcome |
|----------|----------|------------------|
| GW-RE-05 | Alert with only `pod` label (no specific resource labels) | Kind=Pod (preserves existing behavior) |

### FR-4: Excluded Scrape Metadata Labels

| Test ID  | Scenario | Expected Outcome |
|----------|----------|------------------|
| GW-RE-12 | Alert with `job` (scrape) + `pod` labels (no `job_name`) | Kind=Pod (job label excluded, falls through to pod) |
| GW-RE-13 | Alert with `endpoint` + `instance` + `pod` labels | Kind=Pod (endpoint/instance excluded, falls through to pod) |

### FR-5: Monitoring Metadata Label Filtering

**Filter unit tests in**: `test/unit/gateway/adapters/label_filter_test.go`

| Test ID  | Scenario | Expected Outcome |
|----------|----------|------------------|
| GW-RE-09 | `service` label with monitoring infrastructure names (10 patterns) | `IsMonitoringMetadata` returns true (table-driven) |
| GW-RE-10 | `service` label with workload names + non-service label keys | `IsMonitoringMetadata` returns false (no false positives) |
| GW-RE-11 | Adapter with filter: `service: kube-prometheus-stack-...` + `pod` | Kind=Pod (service filtered, extraction falls through) |
| GW-RE-14 | Adapter with nil filter: `service: kube-prometheus-stack-...` | Kind=Service (nil filter = no filtering, backward compat) |

### FR-6: job_name Semantics

| Test ID  | Scenario | Expected Outcome |
|----------|----------|------------------|
| GW-RE-07 | Alert with `job_name: data-migration` + `job: kube-state-metrics` + `pod` | Kind=Job, Name=data-migration (job_name used, job ignored) |

---

## 4. Integration Tests

**File**: `test/integration/gateway/adapters_integration_test.go`

All integration tests follow the correct pattern (direct business logic calls, no HTTP):
1. Create adapter with specific configuration
2. Parse payload via `adapter.Parse(ctx, payload)`
3. Verify signal fields (business outcome)
4. For full-pipeline tests: `server.ProcessSignal(ctx, signal)` then verify CRD fields

### IT-GW-184-001: LLM targets crashing pod, not monitoring service (FR-5)

**Business outcome**: When `KubePodCrashLooping` alert includes `service: kube-prometheus-stack-kube-state-metrics`, the pipeline MUST target the actual crashing pod. Selecting the monitoring service would cause the LLM to find it healthy and conclude "self-resolved."

**Assertions**:
- signal.Resource.Kind == "Pod" (not "Service")
- signal.Resource.Name == "payment-api-7f86bb8877-4hv68"
- Fingerprint == SHA256(ns:Pod:payment-api-...) (deduplication accuracy)
- CRD spec.targetResource.Kind == "Pod" (persisted correctly)
- CRD spec.targetResource.Name == "payment-api-7f86bb8877-4hv68"

### IT-GW-184-002: Workload services NOT filtered -- no false positives (FR-5)

**Business outcome**: When alert references a workload's own Service (`service: payment-api`), the filter MUST NOT strip it. False filtering would misidentify the target.

**Assertions**:
- signal.Resource.Kind == "Service" (workload service passes through)
- signal.Resource.Name == "payment-api"

### IT-GW-184-003: Failed K8s Job correctly targeted via job_name (FR-4, FR-6)

**Business outcome**: When `KubeJobFailed` fires, the pipeline MUST target the failed Job from `job_name`, not the Prometheus scrape job from `job`. Using `job` would send the LLM to investigate "kube-state-metrics" -- a healthy component.

**Assertions**:
- signal.Resource.Kind == "Job" (not Pod or scrape job)
- signal.Resource.Name == "data-migration" (not "kube-state-metrics")
- Fingerprint == SHA256(ns:Job:data-migration) (deduplication accuracy)
- CRD spec.targetResource.Kind == "Job"
- CRD spec.targetResource.Name == "data-migration"

### IT-GW-184-004: Scrape metadata does not pollute extraction (FR-4)

**Business outcome**: Prometheus-injected `job`, `endpoint`, `instance` labels MUST NEVER be mistaken for workload resources. The pipeline MUST target the `deployment` label.

**Assertions**:
- signal.Resource.Kind == "Deployment"
- signal.Resource.Name == "api-server" (not kube-state-metrics, http, or 10.244.0.5:8443)
- Fingerprint == SHA256(ns:Deployment:api-server)

---

## 5. Coverage Summary

| Tier | Tests | Estimated Coverage | Files |
|------|-------|--------------------|-------|
| Unit | 14 (GW-RE-01 to GW-RE-14) | ~100% of unit-testable code | prometheus_adapter_test.go, label_filter_test.go |
| Integration | 4 (IT-GW-184-001 to IT-GW-184-004) | ~87% of integration-testable code | adapters_integration_test.go |
