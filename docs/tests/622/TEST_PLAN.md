# Test Plan: RR -owide Missing Target Resource Namespace

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-622-v1.0
**Feature**: Add target resource namespace and kind columns to RR CRD printer output
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/v1.2.0-rc4`

---

## 1. Introduction

### 1.1 Purpose

Validates that the RemediationRequest CRD includes additional printer columns for target resource namespace and kind (both spec and status), enabling operators to see the full resource context in `kubectl get rr -owide`.

### 1.2 Objectives

1. **Target columns**: CRD manifest contains `Target NS` and `Target Kind` columns for `.spec.targetResource`
2. **RCA Target columns**: CRD manifest contains `RCA NS` and `RCA Kind` columns for `.status.remediationTarget`
3. **Wide-only**: All new columns use `priority: 1` (visible only with `-owide`)

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/...` |
| CRD/Helm sync | Identical | `diff config/crd/bases/ charts/kubernaut/crds/` |

---

## 2. References

### 2.1 Authority

- Issue #622: RR -owide output missing target resource namespace column
- DD-CRD-003: CRD Manifest Printer Columns

### 2.2 Cross-References

- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `make manifests` fails in worktree | Build blocked | Low | All | Verify `VERSION` file and `controller-gen` binary exist |
| R2 | Helm CRD copy out of sync | Deployment inconsistency | Medium | N/A | `make manifests` copies automatically |

---

## 4. Scope

### 4.1 Features to be Tested

- **CRD printer columns** (`api/remediation/v1alpha1/remediationrequest_types.go`): New kubebuilder markers for Target NS, Target Kind, RCA NS, RCA Kind
- **Generated CRD YAML** (`config/crd/bases/kubernaut.ai_remediationrequests.yaml`): Contains correct `additionalPrinterColumns` entries

### 4.2 Features Not to be Tested

- **kubectl rendering**: Kubernetes API server concern, not testable in unit tests

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% — CRD manifest content validation (established pattern)
- **Integration/E2E**: Skipped (printer columns are a K8s API server rendering concern)

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| Issue #622 | Target NS column in CRD | P0 | Unit | UT-RR-622-001 | Pending |
| Issue #622 | Target Kind column in CRD | P0 | Unit | UT-RR-622-002 | Pending |
| Issue #622 | RCA NS column in CRD | P0 | Unit | UT-RR-622-003 | Pending |
| Issue #622 | RCA Kind column in CRD | P0 | Unit | UT-RR-622-004 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**File**: `test/unit/remediationorchestrator/crd_manifest_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RR-622-001` | CRD manifest contains `Target NS` column with JSONPath `.spec.targetResource.namespace` | Pending |
| `UT-RR-622-002` | CRD manifest contains `Target Kind` column with JSONPath `.spec.targetResource.kind` | Pending |
| `UT-RR-622-003` | CRD manifest contains `RCA NS` column with JSONPath `.status.remediationTarget.namespace` | Pending |
| `UT-RR-622-004` | CRD manifest contains `RCA Kind` column with JSONPath `.status.remediationTarget.kind` | Pending |

### Tier Skip Rationale

- **Integration/E2E**: Printer columns are validated by asserting manifest YAML content. Actual rendering is a Kubernetes API server concern.

---

## 9. Test Cases

### UT-RR-622-001: Target NS column

**Priority**: P0
**Type**: Unit

**Test Steps**:
1. **Given**: The generated CRD YAML at `config/crd/bases/kubernaut.ai_remediationrequests.yaml`
2. **When**: The YAML content is read
3. **Then**: It contains `name: Target NS` and `jsonPath: .spec.targetResource.namespace`

### UT-RR-622-002: Target Kind column

**Test Steps**: Same pattern, asserting `name: Target Kind` and `jsonPath: .spec.targetResource.kind`

### UT-RR-622-003: RCA NS column

**Test Steps**: Same pattern, asserting `name: RCA NS` and `jsonPath: .status.remediationTarget.namespace`

### UT-RR-622-004: RCA Kind column

**Test Steps**: Same pattern, asserting `name: RCA Kind` and `jsonPath: .status.remediationTarget.kind`

---

## 10. Environmental Needs

- **Framework**: Ginkgo/Gomega BDD
- **Location**: `test/unit/remediationorchestrator/`
- **Dependencies**: `make manifests` (controller-gen v0.19.0)

---

## 11. Execution

```bash
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RR-622" -ginkgo.v
```

---

## 12. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
