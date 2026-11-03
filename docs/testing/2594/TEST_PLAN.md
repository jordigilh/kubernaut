# Test Plan: Kuadrant MCPServerRegistration v1 Registry Migration

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2594-v1.0  
**Feature**: Update the Kuadrant fleet registry and active fixtures to use `mcp.kuadrant.io/v1`.  
**Version**: 1.0  
**Created**: 2026-11-03  
**Author**: OpenCode  
**Status**: Active  
**Branch**: `openchamber/issue-2594-kuadrant-v1`

---

## 1. Introduction

### 1.1 Purpose

Issue #2594 updates Kubernaut's Kuadrant fleet registry from the removed
`mcp.kuadrant.io/v1alpha1` API to the `mcp.kuadrant.io/v1` API served by Kuadrant MCP
Gateway v1.0.2. This plan proves the production registry's list, watch, probe, readiness,
and filtering behavior while preserving the Envoy AI Gateway registry contract.

### 1.2 Objectives

1. Prove the production Kuadrant GVR is `mcp.kuadrant.io/v1`.
2. Prove managed-label and `spec.state=Disabled` filtering remains unchanged.
3. Prove envtest list/watch/probe/readiness behavior with the v1 CRD fixture.
4. Prove active examples and deployment documentation do not instruct v1alpha1 usage.
5. Record compatibility and distinguish fixture/upstream evidence from live deployment evidence.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Focused unit tests | 100% pass | `go test ./pkg/fleet/registry/...` |
| FMC integration tests | 100% pass when envtest/Valkey prerequisites are available | `go test ./test/integration/fleetmetadatacache/...` |
| Production wiring | 100% of manifest rows proven | Registry startup and FMC probe/cold-start tests |
| Envoy regression | 0 behavior regressions | Existing EAIGW registry tests |
| Active API references | 0 `mcp.kuadrant.io/v1alpha1` references | Scoped repository search |

---

## 2. References

### 2.1 Authority

- [BR-INTEGRATION-065](../../requirements/BR-INTEGRATION-065-mcp-gateway-fleet-integration.md) — fleet registry integration, if present in the repository
- [BR-FLEET-003](../../requirements/BR-FLEET-003-cluster-scoped-workflow-targeting.md) — fleet registration labels and registry source
- Issue [#2594](https://github.com/jordigilh/kubernaut/issues/2594)
- Kuadrant MCP Gateway v1.0.2 CRD: `mcp.kuadrant.io/v1` served/storage; `v1alpha1` served=false/storage=false

### 2.2 Cross-References

- `docs/testing/TEST_PLAN_TEMPLATE.md`
- `pkg/fleet/registry/kuadrant_registry.go`
- `test/integration/fleetmetadatacache/registry_probe_it_test.go`
- `test/integration/fleetmetadatacache/kuadrant_registry_coldstart_it_test.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Production GVR remains v1alpha1 | Registry cannot list/watch Kuadrant v1.0.2 registrations | High | UT-REG-KUA-2594-001, IT-FMC-2594-001 | Contract assertion plus focused registry and FMC tests |
| R2 | Fixture API version differs from the production GVR | Envtest passes for the wrong resource or fails at startup | Medium | IT-FMC-2594-001, IT-FMC-2594-002 | Update external CRD and all active v1 registration fixtures together |
| R3 | Filtering/readiness behavior regresses during migration | Unmanaged or disabled clusters enter the fleet; readiness lies | Medium | UT-REG-KUA existing scenarios, IT-FMC-2594-002 | Retain and rerun existing managed/disabled/probe/readiness coverage |
| R4 | Older Kuadrant installation serves only v1alpha1 | Upgrade causes an operational discovery gap | High | Documentation review and static search | Document upgrade prerequisite; do not claim dual-version support |
| R5 | Live Kuadrant deployment is unavailable | No production qualification evidence | Medium | Handoff evidence | Explicitly label upstream CRD and fixture/envtest evidence; do not imply live validation |

### 3.1 Risk-to-Test Traceability

R1–R3 are covered by the registry unit and FMC integration scenarios. R4 is covered by
the compatibility documentation review. R5 is handled as an explicit evidence limitation,
not hidden as a passing test.

---

## 4. Scope

### 4.1 Features to be Tested

- `pkg/fleet/registry/kuadrant_registry.go`: GVR, informer start, authoritative probe,
  readiness, managed-label filtering, disabled-state filtering, event handling.
- `config/crd/external/mcp.kuadrant.io_mcpserverregistrations.yaml`: envtest v1 CRD.
- FMC integration fixtures and active fleet E2E registration manifests.
- Existing EAIGW registry tests as a regression boundary.

### 4.2 Features Not to be Tested

- Live Kuadrant/OCP qualification: no live deployment is available in this checkout.
- Kuadrant controller behavior or CRD conversion behavior: owned by Kuadrant.
- Historical spike manifests tied to releases that predate the v1 contract, unless they
  are active runnable examples.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Use v1 only | Kuadrant MCP Gateway v1.0.2 serves v1 and explicitly does not serve v1alpha1 |
| Keep one centralized GVR | All Kuadrant list/watch/probe paths already consume `MCPServerRegistrationGVR` |
| Do not add fallback discovery | Older-version support is not established and fallback would obscure an unsupported deployment |
| Preserve historical spike evidence | Historical documents describe the release/API they tested and are not active production instructions |

---

## 5. Approach

### 5.1 Coverage Policy

This localized change uses behavior-focused coverage rather than whole-repository line
coverage. Unit scenarios cover pure filtering/extraction and the GVR contract. Integration
scenarios cover envtest list/watch/probe/readiness wiring. Full E2E is deferred because no
live Kuadrant deployment is available.

### 5.2 Two-Tier Minimum

- Unit: registry GVR contract and existing filtering/lifecycle assertions.
- Integration: FMC envtest cold-start and probe scenarios using the v1 CRD.

### 5.3 Pass/Fail Criteria

PASS requires all focused unit tests, available FMC integration tests, the production build,
and lint to pass; no active v1alpha1 references remain; and compatibility documentation is
explicit. Live qualification is reported separately as unavailable, not treated as passed.

FAIL includes any focused regression, any active v1alpha1 instruction, or an unqualified
claim of live deployment validation.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods |
|------|-------------------|
| `pkg/fleet/registry/kuadrant_registry.go` | `MCPServerRegistrationGVR`, `trackableClusterInfo`, `extractKuadrantClusterInfo`, `isKuadrantDisabled` |
| `pkg/fleet/registry/kuadrant_registry_lifecycle_test.go` | Existing lifecycle, filtering, event, and probe scenarios |

### 6.2 Integration-Testable Code

| File | Functions/Methods |
|------|-------------------|
| `pkg/fleet/registry/kuadrant_registry.go` | `Start`, `Probe`, `Ready`, informer list/watch wiring |
| `test/integration/fleetmetadatacache/suite_test.go` | envtest loads external v1 CRD |
| `test/integration/fleetmetadatacache/registry_probe_it_test.go` | production registry refresh and removal |
| `test/integration/fleetmetadatacache/kuadrant_registry_coldstart_it_test.go` | synchronous v1 cold-start discovery |

### 6.3 Wiring Manifest

| Component | Production entry point | Wiring location | IT proof |
|---|---|---|---|
| `KuadrantRegistry` v1 GVR | `NewKuadrantRegistry` selected by `GatewayKuadrant` | `pkg/fleet/registry/factory.go` and service startup callers | FMC registry probe and cold-start scenarios |
| v1 envtest CRD | FMC envtest API server | `test/integration/fleetmetadatacache/suite_test.go` | FMC integration suite |

---

## 7. BR Coverage Matrix

| BR/Issue | Description | Priority | Tier | Test ID | Status |
|----------|-------------|----------|------|---------|--------|
| #2594 / BR-INTEGRATION-065 | Production registry uses Kuadrant v1 GVR | P0 | Unit | UT-REG-KUA-2594-001 | RED |
| #2594 / BR-INTEGRATION-065 | v1 registration is discovered after API changes | P0 | Integration | IT-FMC-2594-001 | Pending |
| #2594 / BR-INTEGRATION-065 | v1 registrations are synchronously discovered at cold start | P0 | Integration | IT-FMC-2594-002 | Pending |
| BR-FLEET-003 | Managed labels remain the cluster classification source | P1 | Unit/Integration | Existing UT-REG-KUA-004..006; FMC scenarios | Pending |

---

## 8. Test Scenarios

| Test ID | Scenario | Expected result |
|---------|----------|-----------------|
| UT-REG-KUA-2594-001 | Inspect the production Kuadrant GVR | Group is `mcp.kuadrant.io`, version is `v1`, resource is `mcpserverregistrations` |
| UT-REG-KUA-004..006 | Add/update registration with managed, unmanaged, enabled, disabled, and absent state | Only managed non-disabled registrations are tracked |
| IT-FMC-2594-001 | Start registry, create v1 managed registration, probe, delete, probe | Registration appears then disappears; readiness remains healthy |
| IT-FMC-2594-002 | Start registry with pre-existing v1 managed registrations | All registrations are present when `Start` returns |
| IT-FMC-2594-003 | Make authoritative API unavailable during probe | Probe fails and readiness becomes false |
| REG-EAIGW-2594-001 | Run existing EAIGW registry tests | EAIGW behavior and v1alpha1 Backend contract are unchanged |

---

## 9. Environment & Evidence

- Focused unit tests use the repository's Ginkgo/Gomega suite and dynamic fake client.
- FMC integration tests use controller-runtime envtest and Valkey; they may be unavailable
  if required local binaries/containers are absent.
- Upstream CRD evidence is the Kuadrant MCP Gateway v1.0.2 tagged manifest.
- No live Kuadrant deployment was tested for this change. Any failure to run live validation
  is reported as an evidence gap, not converted into a fixture claim.

---

## 10. TDD Phase Tracking

| Phase | Exit criterion | Status |
|-------|----------------|--------|
| RED | Contract test fails against v1alpha1 implementation | In progress |
| GREEN | Production GVR and active fixtures use v1; focused UT/IT pass | Pending |
| REFACTOR | Active docs/examples aligned; compatibility wording and evidence boundaries reviewed | Pending |
