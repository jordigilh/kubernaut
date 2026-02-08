# SignalProcessing E2E Test Plan

**Version**: 1.1.0
**Created**: 2025-12-21
**Updated**: 2026-02-05
**Status**: ✅ **COMPLETE** - Comprehensive Coverage
**Test Location**: `test/e2e/signalprocessing/`

---

## 📋 Changelog

### Version 1.1.0 (2026-02-05)
- **ADDED**: BR-SP-106 Predictive Signal Mode Classification E2E scenarios (2 tests)
- **TESTS**: 22 E2E test scenarios (was 20)
- **FILE**: `50_predictive_signal_mode_test.go`

### Version 1.0.0 (2025-12-21)
- **INITIAL**: Created E2E test plan documenting existing comprehensive coverage
- **TESTS**: 20 E2E test scenarios covering all critical business requirements
- **COMPLIANCE**: Per TESTING_GUIDELINES.md v2.1.0

---

## 📊 Coverage Summary

### E2E Test Files

| File | Purpose | BR Coverage | Status |
|------|---------|-------------|--------|
| `business_requirements_test.go` | Business value validation | BR-SP-001, 051, 070, 100, 101, 102 | ✅ Complete |
| `50_predictive_signal_mode_test.go` | Predictive signal mode classification | BR-SP-106 | ✅ Complete |
| `suite_test.go` | Kind cluster infrastructure | - | ✅ Complete |

### Test Count by Business Requirement

| BR | Description | E2E Tests | Status |
|----|-------------|-----------|--------|
| **BR-SP-001** | K8s Context Enrichment (Node) | 1 | ✅ |
| **BR-SP-051** | Environment Classification | 4 | ✅ |
| **BR-SP-070** | Priority Assignment | 4 | ✅ |
| **BR-SP-100** | Owner Chain Traversal | 2 | ✅ |
| **BR-SP-101** | Detected Labels (PDB, HPA) | 3 | ✅ |
| **BR-SP-102** | CustomLabels from Rego | 2 | ✅ |
| **BR-SP-090** | Audit Client Wired | 1 | ✅ |
| **DD-METRICS** | Metrics Endpoint | 1 | ✅ |
| **DD-007** | Graceful Shutdown | 1 | ✅ |
| **BR-SP-106** | Predictive Signal Mode Classification | 2 | ✅ |
| **Total** | | **22** | ✅ |

---

## 🧪 Test Scenarios

### BR-SP-001: Node Enrichment

| Test ID | Scenario | Validation |
|---------|----------|------------|
| E2E-NODE-01 | Node enrichment when Pod is scheduled | `status.kubernetesContext.node` populated with real Kind node |

### BR-SP-051: Environment Classification

| Test ID | Scenario | Validation |
|---------|----------|------------|
| E2E-ENV-01 | Production environment from namespace label | `status.environmentClassification.environment = "production"` |
| E2E-ENV-02 | Staging environment from namespace label | `status.environmentClassification.environment = "staging"` |
| E2E-ENV-03 | Development environment from namespace label | `status.environmentClassification.environment = "development"` |
| E2E-ENV-04 | Unknown environment fallback | `status.environmentClassification.environment = "unknown"` |

### BR-SP-070: Priority Assignment

| Test ID | Scenario | Validation |
|---------|----------|------------|
| E2E-PRIO-01 | P0 for production + critical | `status.priorityAssignment.priority = "P0"` |
| E2E-PRIO-02 | P1 for production + warning | `status.priorityAssignment.priority = "P1"` |
| E2E-PRIO-03 | P2 for staging + critical | `status.priorityAssignment.priority = "P2"` |
| E2E-PRIO-04 | P3 for development | `status.priorityAssignment.priority = "P3"` |

### BR-SP-100: Owner Chain Traversal

| Test ID | Scenario | Validation |
|---------|----------|------------|
| E2E-OWNER-01 | Pod → ReplicaSet → Deployment | `status.kubernetesContext.ownerChain` includes all owners |
| E2E-OWNER-02 | StatefulSet owner chain | Full traversal to StatefulSet |

### BR-SP-101: Detected Labels

| Test ID | Scenario | Validation |
|---------|----------|------------|
| E2E-DETECT-01 | PDB detection | `status.kubernetesContext.detectedLabels.hasPDB = true` |
| E2E-DETECT-02 | HPA detection via owner chain | `status.kubernetesContext.detectedLabels.hasHPA = true` |
| E2E-DETECT-03 | NetworkPolicy detection | `status.kubernetesContext.detectedLabels.networkIsolated = true` |

### BR-SP-102: CustomLabels from Rego

| Test ID | Scenario | Validation |
|---------|----------|------------|
| E2E-CUSTOM-01 | Constraint labels from Rego | `customLabels["constraint"]` populated |
| E2E-CUSTOM-02 | Team labels from Rego | `customLabels["team"]` populated |

### BR-SP-106: Predictive Signal Mode Classification (ADR-054)

| Test ID | Scenario | Validation |
|---------|----------|------------|
| E2E-SP-106-001 | PredictedOOMKill classified as predictive, normalized to OOMKilled | `status.signalMode = "predictive"`, `status.signalType = "OOMKilled"`, `status.originalSignalType = "PredictedOOMKill"` |
| E2E-SP-106-002 | Standard OOMKilled defaults to reactive mode | `status.signalMode = "reactive"`, `status.signalType = "OOMKilled"`, `status.originalSignalType = ""` |

**Test File**: `50_predictive_signal_mode_test.go`
**Status**: ✅ Passed
**References**: [BR-SP-106](../../../requirements/BR-SP-106-predictive-signal-mode-classification.md), [ADR-054](../../../architecture/decisions/ADR-054-predictive-signal-mode-classification.md)

### V1.0 Maturity: Controller Wiring

| Test ID | Scenario | Validation |
|---------|----------|------------|
| E2E-AUDIT-WIRE | Audit client wired in controller | Audit events appear in Data Storage |
| E2E-METRICS-WIRE | Metrics endpoint accessible | `/metrics` returns 200 with expected metrics |
| E2E-SHUTDOWN | Graceful shutdown flushes audit | Audit events persisted before pod termination |

---

## 🏗️ Infrastructure Requirements

### Kind Cluster

| Component | Purpose |
|-----------|---------|
| Kind cluster | Real K8s environment |
| Controller deployment | Actual SP controller binary |
| Data Storage | Audit trail persistence |
| PostgreSQL | Database for Data Storage |
| Redis | Caching for Data Storage |

### Configuration

| ConfigMap | Purpose |
|-----------|---------|
| `environment.rego` | Environment classification policy |
| `priority.rego` | Priority assignment policy |
| `business.rego` | Business classification policy |
| `customlabels.rego` | Custom label extraction |
| `predictive-signal-mappings.yaml` | Predictive signal mode type mappings (BR-SP-106) |

---

## 🚀 Running E2E Tests

```bash
# Create Kind cluster (if not exists)
kind create cluster --name signalprocessing-e2e --kubeconfig ~/.kube/signalprocessing-e2e-config

# Set KUBECONFIG
export KUBECONFIG=~/.kube/signalprocessing-e2e-config

# Deploy infrastructure
kubectl apply -k test/infrastructure/signalprocessing/

# Run E2E tests
make test-e2e-signalprocessing

# Cleanup
kind delete cluster --name signalprocessing-e2e
rm -f ~/.kube/signalprocessing-e2e-config
```

---

## 📊 Coverage Compliance

### Per TESTING_GUIDELINES.md v2.1.0

| Requirement | Status | Implementation |
|-------------|--------|----------------|
| **Audit client wired** | ✅ | E2E-AUDIT-WIRE test |
| **Metrics endpoint accessible** | ✅ | E2E-METRICS-WIRE test |
| **EventRecorder emits** | ✅ | Kubernetes events verified |
| **Graceful shutdown** | ✅ | E2E-SHUTDOWN test |
| **Health probes accessible** | ✅ | `/healthz` and `/readyz` |

### Per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md

| Feature | Test Tier | Status |
|---------|-----------|--------|
| Metrics on `/metrics` | E2E | ✅ |
| Audit client wired | E2E | ✅ |
| EventRecorder emits | E2E | ✅ |
| Graceful shutdown flush | E2E | ✅ |
| Health probes | E2E | ✅ |

---

## 🔄 Known Flaky Tests

| Test ID | Issue | Mitigation |
|---------|-------|------------|
| E2E-OWNER-01 | Timing sensitivity | `FlakeAttempts(3)` |
| E2E-DETECT-02 | HPA via owner chain | `FlakeAttempts(3)` |

---

## 🔗 References

- [TESTING_GUIDELINES.md](../../../development/business-requirements/TESTING_GUIDELINES.md)
- [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](../../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
- [unit-test-plan.md](./unit-test-plan.md)
- [integration-test-plan.md](./integration-test-plan.md)
- [testing-strategy.md](./testing-strategy.md)
- [E2E_COVERAGE_COLLECTION.md](../../../development/testing/E2E_COVERAGE_COLLECTION.md)

