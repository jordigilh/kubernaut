# SignalProcessing Integration Test Plan

**Version**: 1.1.0
**Created**: 2025-12-21
**Updated**: 2025-12-21
**Status**: ✅ **COMPLETE** - Comprehensive Coverage
**Test Location**: `test/integration/signalprocessing/`

---

## 📋 Changelog

### Version 1.1.0 (2025-12-21)
- **ADDED**: CTRL-DETECT-04: HPA via owner chain integration test (moved from unit scope)
- **ADDED**: AUDIT-06: Business classification audit trace test
- **TESTS**: Now 28+ integration test scenarios

### Version 1.0.0 (2025-12-21)
- **INITIAL**: Created integration test plan documenting existing comprehensive coverage
- **TESTS**: 6 test files covering audit, metrics, reconciler, Rego, components, and hot reload
- **COMPLIANCE**: Per TESTING_GUIDELINES.md v2.1.0 - all audit traces use OpenAPI client

---

## 📊 Coverage Summary

### Integration Test Files

| File | Purpose | BR Coverage | Status |
|------|---------|-------------|--------|
| `audit_integration_test.go` | Audit trail validation with OpenAPI client | BR-SP-090 | ✅ Complete |
| `metrics_integration_test.go` | Metrics registry validation | DD-METRICS-001 | ✅ Complete |
| `reconciler_integration_test.go` | Controller reconciliation with envtest | BR-SP-001 to BR-SP-081 | ✅ Complete |
| `rego_integration_test.go` | Rego policy evaluation | BR-SP-070 to BR-SP-072 | ✅ Complete |
| `component_integration_test.go` | Cross-component integration | BR-SP-100, BR-SP-101 | ✅ Complete |
| `hot_reloader_test.go` | ConfigMap hot reload | DD-HOTRELOAD-001 | ✅ Complete |

### Per TESTING_GUIDELINES.md Requirements

| Requirement | Status | Implementation |
|-------------|--------|----------------|
| **Audit traces use OpenAPI client** | ✅ | `dsgen.NewClientWithResponses()` |
| **Audit fields validated** | ✅ | `testutil.ValidateAuditEvent()` |
| **Metrics use test registry** | ✅ | `prometheus.NewRegistry()` + `metrics.NewMetrics()` |
| **No `time.Sleep()` before assertions** | ✅ | All use `Eventually()` |
| **No `Skip()` calls** | ✅ | Uses `Fail()` for missing infrastructure |

---

## 🧪 Test Scenarios

### Audit Integration Tests (BR-SP-090)

| Test ID | Scenario | Event Type | Validation |
|---------|----------|------------|------------|
| AUDIT-01 | Signal processing completion | `signalprocessing.signal.processed` | Full field validation |
| AUDIT-02 | Classification decision | `signalprocessing.classification.decision` | Environment, priority |
| AUDIT-03 | Enrichment completion | `signalprocessing.enrichment.completed` | Duration, degraded mode |
| AUDIT-04 | Phase transitions | `signalprocessing.phase.transition` | From/to phases |
| AUDIT-05 | Error handling | `signalprocessing.error.occurred` | Error details |
| **AUDIT-06** | **Business classification** | `signalprocessing.business.classified` | **Criticality, SLA** ⭐ NEW |

### Metrics Integration Tests (DD-METRICS-001)

| Test ID | Metric | Labels | Validation |
|---------|--------|--------|------------|
| METRIC-01 | `signalprocessing_processing_total` | `phase`, `result` | Counter increment |
| METRIC-02 | `signalprocessing_processing_duration_seconds` | `phase` | Histogram observation |
| METRIC-03 | `signalprocessing_enrichment_total` | `result` | Counter increment |
| METRIC-04 | `signalprocessing_enrichment_duration_seconds` | `resource_kind` | Histogram observation |
| METRIC-05 | `signalprocessing_enrichment_errors_total` | `error_type` | Counter increment |
| METRIC-06 | All metrics after reconciliation | all | Post-operation values |

### Reconciler Integration Tests (BR-SP-001 to BR-SP-081)

| Test ID | Scenario | Phases | Validation |
|---------|----------|--------|------------|
| RECONCILER-01 | Complete phase flow | Pending → Completed | All transitions |
| RECONCILER-02 | Environment classification | Classifying | production/staging/development |
| RECONCILER-03 | Priority assignment | Classifying | P0/P1/P2/P3 |
| RECONCILER-04 | Enrichment context | Enriching | Namespace, Pod, Deployment |
| RECONCILER-05 | Owner chain | Enriching | Full traversal |
| RECONCILER-06 | Detected labels | Enriching | PDB, HPA, NetworkPolicy |
| RECONCILER-07 | Error handling | Failed | Error conditions |
| RECONCILER-08 | Degraded mode | Enriching | Missing target resource |

### Rego Integration Tests (BR-SP-070 to BR-SP-072)

| Test ID | Scenario | Policy | Validation |
|---------|----------|--------|------------|
| REGO-01 | Environment classification | `environment.rego` | Policy evaluation |
| REGO-02 | Priority assignment | `priority.rego` | Signal severity mapping |
| REGO-03 | Custom labels extraction | `customlabels.rego` | Subdomain extraction |
| REGO-04 | Policy hot reload | All policies | ConfigMap update |
| REGO-05 | Policy error handling | Invalid policy | Graceful failure |

### Component Integration Tests (BR-SP-100, BR-SP-101)

| Test ID | Scenario | Components | Validation |
|---------|----------|------------|------------|
| COMPONENT-01 | K8sEnricher with OwnerChainBuilder | Enricher + OwnerChain | Full chain traversal |
| COMPONENT-02 | LabelDetector with K8s resources | Detector + PDB/HPA | Resource detection |
| COMPONENT-03 | Classifier with Rego engine | Classifier + Rego | Policy-based classification |
| **COMPONENT-04** | **HPA detection via owner chain** | **LabelDetector + OwnerChain** | **Pod→RS→Deployment→HPA** ⭐ NEW (CTRL-DETECT-04) |

---

## 🏗️ Infrastructure Requirements

### podman-compose Services

| Service | Image | Port | Purpose |
|---------|-------|------|---------|
| PostgreSQL | `quay.io/jordigilh/pgvector:pg16` | 15432 | Audit persistence |
| Redis | `quay.io/jordigilh/redis:7-alpine` | 16379 | Caching |
| Data Storage | Built locally | 18094 | Audit API |

### Configuration Files

| File | Purpose |
|------|---------|
| `podman-compose.signalprocessing.test.yml` | Infrastructure orchestration |
| `config/config.yaml` | Controller configuration |
| `config/db-secrets.yaml` | Database credentials |
| `config/redis-secrets.yaml` | Redis credentials |

---

## 🚀 Running Integration Tests

```bash
# Start infrastructure
cd test/integration/signalprocessing
podman-compose -f podman-compose.signalprocessing.test.yml up -d

# Wait for services to be ready
./wait-for-infrastructure.sh

# Run integration tests
go test ./test/integration/signalprocessing/... -v

# Run specific test
go test ./test/integration/signalprocessing/... --ginkgo.focus="Audit Integration"

# Cleanup
podman-compose -f podman-compose.signalprocessing.test.yml down -v
```

Or use Make target:
```bash
make test-integration-signalprocessing
```

---

## 📊 Coverage Compliance

### Per TESTING_GUIDELINES.md v2.1.0

| Feature | Test Tier | Implementation | Status |
|---------|-----------|----------------|--------|
| **Metrics recorded** | Integration | Registry inspection | ✅ |
| **Audit fields correct** | Integration | OpenAPI client + `testutil.ValidateAuditEvent()` | ✅ |
| **Phase transitions** | Integration | envtest reconciliation | ✅ |
| **Rego policy evaluation** | Integration | Real policy files | ✅ |
| **Hot reload** | Integration | ConfigMap updates | ✅ |

### Per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md

| Requirement | Status |
|-------------|--------|
| All audit traces tested with OpenAPI client | ✅ |
| `testutil.ValidateAuditEvent()` used | ✅ |
| All metrics tested via registry | ✅ |
| `NewMetricsWithRegistry()` used | ✅ |

---

## 🔗 References

- [TESTING_GUIDELINES.md](../../../development/business-requirements/TESTING_GUIDELINES.md)
- [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](../../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
- [unit-test-plan.md](./unit-test-plan.md)
- [testing-strategy.md](./testing-strategy.md)
- [ADR-004: Fake Kubernetes Client](../../../architecture/decisions/ADR-004-fake-kubernetes-client.md)
- [DD-METRICS-001: Controller Metrics Wiring](../../../architecture/decisions/DD-METRICS-001-controller-metrics-wiring-pattern.md)

