# E2E Test Plan: apiVersion Validation Gate with CRD Kind Collision

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1044-E2E-v1.0
**Feature**: End-to-end validation of `apiVersionValidationGate` with real CRD kind collision in Kind cluster
**Version**: 1.0
**Created**: 2026-05-07
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/1044-apiversion-validation-gate`
**Parent**: [TP-1044-v1.0](TEST_PLAN.md) (unit + integration tiers)

---

## 1. Introduction

### 1.1 Purpose

This E2E test plan validates the `apiVersionValidationGate` in a production-representative
environment: a Kind cluster with two CRDs sharing the same Kind name (`TestWidget`) in
different API groups, a mock LLM that consistently omits `api_version`, and the full
Kubernaut Agent pipeline from signal intake through investigation to human review routing.

This plan is a **companion** to [TP-1044-v1.0](TEST_PLAN.md) which covers unit and
integration tiers. Together they provide >= 80% coverage per tier.

### 1.2 Rationale

Unit and integration tests validate gate mechanics with fake REST mappers and mock LLM
clients. E2E tests close the remaining confidence gap by exercising:

- Real Kubernetes API server discovering CRDs via the REST mapper
- Real `ResourcesFor()` returning multiple GVRs for the same kind
- Full investigation pipeline: signal → enrichment → RCA → gate → human review
- In-cluster mock LLM deployed as a Kubernetes service

### 1.3 Design Decision: Test the Exhaustion Path

The mock LLM scenario system returns the same response regardless of conversation turn
(it matches on signal keywords, not conversation state). The gate retry naturally receives
a response without `api_version`, exercising the **exhaustion path** — the security-critical
behavior that prevents incorrect RBAC grants by setting `HumanReviewNeeded=true`.

The **success path** (LLM provides `api_version` on retry) is covered by UT-KA-1044-001
and IT-KA-1044-001 in the parent test plan.

### 1.4 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| E2E test pass rate | 100% | `make test-e2e-kubernautagent` with `-ginkgo.focus="1044"` |
| Race detector | 0 races | `go test -race` |
| CRD installation | Both CRDs discoverable | `kubectl get crd testwidgets.alpha.kubernaut-test.ai` |

---

## 2. References

### 2.1 Authoritative Documents

- [Issue #1044](https://github.com/jordigilh/kubernaut/issues/1044) — apiVersion omission regression
- [TP-1044-v1.0](TEST_PLAN.md) — Parent test plan (unit + integration tiers)
- BR-AI-1044 — apiVersion validation gate for ambiguous kinds
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) — Per-tier >=80%
- [ANTI_PATTERN_DETECTION.md](../../testing/ANTI_PATTERN_DETECTION.md) — Forbidden test patterns

### 2.2 Infrastructure Files

| File | Role |
|------|------|
| `test/infrastructure/kubernautagent.go` | `createAmbiguousKindCRDs()`, enrichment fixtures, RBAC, workflow seeding |
| `test/services/mock-llm/scenarios/scenario_ambiguous_kind.go` | Mock LLM scenario (no `APIVersion`) |
| `test/services/mock-llm/scenarios/registry_default.go` | Scenario registration |
| `test/e2e/kubernautagent/apiversion_gate_e2e_test.go` | E2E test file |

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Mitigation |
|----|------|--------|-------------|------------|
| R1 | CRDs not installed before KA starts | REST mapper misses ambiguity | Low | CRD install in Phase 5.5 (before Phase 6 KA deploy) |
| R2 | Missing RBAC for test CRD groups | Mapper discovery fails silently | Medium | ClusterRole grants `get`/`list`/`watch` on both groups |
| R3 | Mock LLM image doesn't include new scenario | Scenario detection falls to default | None | Image built from source in Phase 5 |
| R4 | Kind cluster timeout on CRD readiness | Flaky test | Low | `kubectl wait --for=condition=Established` with 30s timeout |

---

## 4. Scope

### 4.1 Features to be Tested

- CRD kind collision detection via real REST mapper in Kind cluster
- Gate exhaustion → `HumanReviewNeeded=true` through full pipeline
- Audit trail includes gate-related events
- Warning message in response references ambiguous kind

### 4.2 Features Not to be Tested

- Gate success path (LLM provides `api_version` on retry) — covered by UT/IT in parent plan
- Prompt template changes — follow-up
- Real LLM behavioral validation — production only

---

## 5. Test Infrastructure

### 5.1 CRD Fixtures

Two minimal CRDs with Kind `TestWidget` in different API groups:

| CRD | API Group | Kind | Scope |
|-----|-----------|------|-------|
| `testwidgets.alpha.kubernaut-test.ai` | `alpha.kubernaut-test.ai` | `TestWidget` | Namespaced |
| `testwidgets.beta.kubernaut-test.ai` | `beta.kubernaut-test.ai` | `TestWidget` | Namespaced |

Installed via `createAmbiguousKindCRDs()` in Phase 5.5 of `SetupKubernautAgentInfrastructure`.

### 5.2 RBAC

ClusterRole `kubernaut-agent-testwidget-reader` grants `get`/`list`/`watch` on `testwidgets`
in both `alpha.kubernaut-test.ai` and `beta.kubernaut-test.ai`, bound to `kubernaut-agent-sa`.

### 5.3 Mock LLM Scenario

Scenario `ambiguous_kind` matches keyword `mock_ambiguous_kind` and returns:
- `ResourceKind: "TestWidget"` (ambiguous — exists in 2 groups)
- `APIVersion: ""` (intentionally empty — triggers gate)
- `Confidence: 0.85`, `InvestigationOutcome: "actionable"`

### 5.4 Enrichment Fixture

A `TestWidget` CR instance in `alpha.kubernaut-test.ai/v1` in the `default` namespace,
so re-enrichment has a resource to resolve.

---

## 6. Test Design Specification

### 6.1 E2E Tests

**Test file**: `test/e2e/kubernautagent/apiversion_gate_e2e_test.go`

| Test ID | Description | BR | Category |
|---------|-------------|-----|----------|
| E2E-KA-1044-001 | Full pipeline: Pod signal, RCA targets ambiguous `TestWidget` without `api_version`, gate exhaustion → `HumanReviewNeeded=true` | BR-AI-1044 | Security |
| E2E-KA-1044-002 | Full pipeline: `TestWidget` signal directly, same gate exhaustion behavior | BR-AI-1044 | Security |

### 6.2 E2E-KA-1044-001: Pod signal, ambiguous RCA target

**BR**: BR-AI-1044 AC3
**Type**: E2E
**Category**: Security (gate exhaustion prevents wrong RBAC)
**Priority**: P0

**Preconditions**:
- Kind cluster with both `TestWidget` CRDs installed
- Mock LLM deployed with `ambiguous_kind` scenario
- KA deployed with REST mapper aware of both CRD groups

**Steps**:
1. **Given**: Signal `{ResourceKind: "Pod", ResourceName: "test-pod", Namespace: "default", SignalName: "MOCK_AMBIGUOUS_KIND"}`
2. **When**: `sessionClient.Investigate(ctx, req)` is called
3. **Then**: Pipeline completes with human review

**Expected Result**:
- `NeedsHumanReview == true`
- `HumanReviewReason` indicates gate exhaustion (e.g., `"rca_incomplete"`)
- Response includes warning text mentioning the ambiguous kind

### 6.3 E2E-KA-1044-002: Ambiguous kind as signal resource

**BR**: BR-AI-1044 AC3
**Type**: E2E
**Category**: Security
**Priority**: P1

**Preconditions**: Same as E2E-KA-1044-001

**Steps**:
1. **Given**: Signal `{ResourceKind: "TestWidget", ResourceName: "test-widget-instance", Namespace: "default", SignalName: "MOCK_AMBIGUOUS_KIND"}`
2. **When**: `sessionClient.Investigate(ctx, req)` is called
3. **Then**: Pipeline completes with human review

**Expected Result**:
- `NeedsHumanReview == true` (gate fires on the RCA target kind)

---

## 7. BR Coverage Matrix (E2E Tier)

| BR ID | AC | Description | Test ID | Status |
|-------|----|-------------|---------|--------|
| BR-AI-1044 | AC3 | Gate exhaustion → `HumanReviewNeeded=true` (full pipeline) | E2E-KA-1044-001 | Pending |
| BR-AI-1044 | AC3 | Gate exhaustion with ambiguous signal kind | E2E-KA-1044-002 | Pending |

---

## 8. Execution

```bash
# Full KA E2E suite (includes #1044 tests)
make test-e2e-kubernautagent

# Focused run (#1044 only)
cd test/e2e/kubernautagent && go test -v -timeout=15m -ginkgo.v -ginkgo.focus="1044" .
```

---

## 9. Sign-off

| Role | Name | Date | Status |
|------|------|------|--------|
| Author | AI Assistant | 2026-05-07 | Draft |
| Technical Review | | | Pending |

---

## 10. Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-07 | AI Assistant | Initial E2E test plan for CRD kind collision scenario |
