# Test Plan: Unified Prometheus/AlertManager Monitoring Config

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-463-v1.0
**Feature**: Single `monitoring:` block in Helm values that configures Prometheus/AlertManager for both EffectivenessMonitor and Kubernaut Agent
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.4`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the unified monitoring configuration introduced by Issue #463. The Helm chart currently requires Prometheus/AlertManager to be configured twice — once under `effectivenessmonitor.external.*` and once under `kubernautAgent.prometheus.*` — with different formats. This change introduces a single `monitoring:` top-level block that the chart propagates to both EM and KA, with OCP auto-detection and backward compatibility with legacy keys.

### 1.2 Objectives

1. **Single source of truth**: `monitoring.prometheus.url` and `monitoring.alertManager.url` are the only places operators need to configure monitoring endpoints.
2. **EM propagation**: EM ConfigMap correctly reads from `monitoring.*` with backward-compatible fallback to `effectivenessmonitor.external.*`.
3. **KA propagation**: KA SDK ConfigMap injects Prometheus toolset config from `monitoring.*` with backward-compatible fallback to `kubernautAgent.prometheus.*`.
4. **OCP auto-detection**: When OCP is detected (`.Capabilities.APIVersions.Has "route.openshift.io/v1"`), OCP-specific defaults are applied (Thanos ports, service-serving CA).
5. **TLS volume mounts**: CA bundle mounted into both EM and KA pods when `monitoring.prometheus.tlsCaFile` is set.
6. **OCP RBAC**: Conditional ClusterRoleBindings for `openshift-monitoring` access.
7. **Backward compatibility**: Legacy keys (`effectivenessmonitor.external.*`, `kubernautAgent.prometheus.*`) continue to work.
8. **Deprecation warning**: NOTES.txt emits a deprecation warning when legacy keys are used.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Helm smoke test pass rate | 100% | `scripts/helm-smoke-test.sh` extended tests |
| Template rendering correctness | 100% | All rendering permutations produce valid YAML |
| Backward compatibility | 0 regressions | Existing values produce identical templates |
| OCP auto-detection | Correct | Mocked `.Capabilities.APIVersions` tests |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #463: Unify Prometheus/AlertManager config: single monitoring block for EM and KA
- Issue #452 (closed): TLS CA support (merged in v1.1 — prerequisite)
- Issue #433 (closed): KA Go rewrite (v1.3 — KA config schema is finalized)
- Issue #462: signalAnnotations + anti-confirmation-bias (Prometheus toolset enables disk-pressure investigations)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- Observed failure: jordigilh/kubernaut-demo-scenarios#101, #103 (KA cannot query Prometheus)
- Helm smoke tests: `scripts/helm-smoke-test.sh`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Backward-incompatible removal of legacy keys | Existing deployments break on upgrade | High | UT-MON-463-006, UT-MON-463-007 | Legacy keys become fallback aliases; removal deferred to v1.5 |
| R2 | OCP detection false positive on non-OCP clusters with Route CRD | Wrong Prometheus ports/TLS applied | Low | UT-MON-463-008 | Document: if `route.openshift.io/v1` CRD installed without OCP, disable auto-detection via explicit values |
| R3 | TLS CA bundle path mismatch on OCP | EM/KA cannot reach Prometheus over HTTPS | Medium | UT-MON-463-009 | Validate path in template against OCP-documented CA location |
| R4 | Missing RBAC on OCP — EM/KA cannot query Prometheus | Monitoring silently fails | High | UT-MON-463-010 | ClusterRoleBinding created when monitoring enabled on OCP |
| R5 | Values schema rejects new `monitoring:` block | `helm install` fails validation | Medium | UT-MON-463-001 | Schema updated alongside values |

### 3.1 Risk-to-Test Traceability

- **R1** (backward compat): UT-MON-463-006, UT-MON-463-007
- **R2** (OCP detection): UT-MON-463-008
- **R3** (TLS): UT-MON-463-009
- **R4** (RBAC): UT-MON-463-010

---

## 4. Scope

### 4.1 Features to be Tested

- **Unified `monitoring:` block** (`charts/kubernaut/values.yaml`): New top-level values
- **EM ConfigMap template** (`charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml`): Reads from `monitoring.*` with fallback
- **KA SDK ConfigMap template** (`charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml`): Injects Prometheus toolset from `monitoring.*` with fallback
- **OCP detection logic** (Helm `_helpers.tpl`): `.Capabilities.APIVersions` check
- **TLS volume mounts** (EM + KA Deployment templates): Conditional CA bundle mount
- **OCP RBAC** (EM + KA templates): Conditional ClusterRoleBindings
- **values.schema.json**: Schema for new `monitoring:` block
- **values-ocp.yaml**: Updated OCP overlay
- **NOTES.txt**: Deprecation warning for legacy keys

### 4.2 Features Not to be Tested

- **Prometheus/AlertManager server availability**: Infrastructure concern, not chart logic
- **EM/KA application-level Prometheus querying**: Tested by EM and KA E2E tests respectively
- **NetworkPolicy monitoring egress rules** (#285): Already tested

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Legacy keys as fallback aliases | Zero-downtime upgrade path; operators can migrate at their own pace |
| OCP detection via `.Capabilities.APIVersions` | Standard Helm pattern; no custom flags needed |
| Shared `_helpers.tpl` for monitoring defaults | DRY: both EM and KA templates use same helper |
| Deprecation in NOTES.txt not hard failure | Soft transition; hard failure deferred to v1.5 |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit (Helm smoke tests)**: >=80% of all template rendering paths (enabled/disabled, OCP/vanilla, legacy/new, TLS/no-TLS)
- **Integration**: Deferred — requires real cluster with Prometheus (will be covered by EM and KA CI/CD)
- **E2E**: Deferred — requires OCP cluster for OCP-specific tests

### 5.2 Two-Tier Minimum

Helm chart changes are validated by:
- **Tier 1 (smoke tests)**: `helm template` assertions for all value permutations
- **Tier 2 (lint)**: `helm lint` with strict mode

Integration and E2E are deferred to component-level CI/CD (EM E2E, KA E2E).

### 5.3 Pass/Fail Criteria

**PASS**:
1. All smoke tests pass
2. `helm lint --strict` passes for all value permutations
3. Legacy values produce identical templates to current baseline (no regression)
4. OCP overlay produces correct ports and TLS config

**FAIL**:
1. Any smoke test fails
2. `helm lint` fails
3. Existing deployments would break on upgrade

### 5.5 Suspension & Resumption Criteria

**Suspend**: KA toolset config schema changes after v1.3 finalization
**Resume**: Schema stable

---

## 6. Test Items

### 6.1 Unit-Testable Code (Helm templates)

| File | What is tested | Lines (approx) |
|------|----------------|-----------------|
| `charts/kubernaut/values.yaml` | New `monitoring:` block, defaults | ~20 |
| `charts/kubernaut/templates/_helpers.tpl` | Monitoring helpers, OCP detection | ~30 |
| `charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml` | EM ConfigMap with monitoring values | ~35 |
| `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml` | KA SDK ConfigMap with monitoring values | ~30 |
| `charts/kubernaut/values.schema.json` | Schema for monitoring block | ~30 |
| `charts/kubernaut/values-ocp.yaml` | OCP overlay defaults | ~10 |
| `charts/kubernaut/templates/NOTES.txt` | Deprecation warning | ~10 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-MON-001 | Single monitoring block configures both EM and KA | P0 | Unit | UT-MON-463-001 | Pending |
| BR-MON-001 | EM ConfigMap reads from monitoring.* | P0 | Unit | UT-MON-463-002 | Pending |
| BR-MON-001 | KA SDK ConfigMap reads from monitoring.* | P0 | Unit | UT-MON-463-003 | Pending |
| BR-MON-001 | Monitoring disabled → no Prometheus in either component | P0 | Unit | UT-MON-463-004 | Pending |
| BR-MON-001 | Monitoring enabled → both components configured | P0 | Unit | UT-MON-463-005 | Pending |
| BR-MON-001 | Legacy EM keys still work | P0 | Unit | UT-MON-463-006 | Pending |
| BR-MON-001 | Legacy KA keys still work | P0 | Unit | UT-MON-463-007 | Pending |
| BR-MON-001 | OCP auto-detection applies correct defaults | P0 | Unit | UT-MON-463-008 | Pending |
| BR-MON-001 | TLS CA bundle mounted when configured | P1 | Unit | UT-MON-463-009 | Pending |
| BR-MON-001 | OCP RBAC created when monitoring enabled on OCP | P1 | Unit | UT-MON-463-010 | Pending |
| BR-MON-001 | Monitoring.* overrides legacy keys when both set | P1 | Unit | UT-MON-463-011 | Pending |
| BR-MON-001 | NOTES.txt deprecation warning when legacy keys used | P2 | Unit | UT-MON-463-012 | Pending |
| BR-MON-001 | values.schema.json validates monitoring block | P1 | Unit | UT-MON-463-013 | Pending |

---

## 8. Test Scenarios

### Tier 1: Helm Smoke Tests

**Testable scope**: All template rendering paths via `helm template` with value overrides

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-MON-463-001` | `monitoring.prometheus.enabled: true` + URL → both EM and KA ConfigMaps contain correct Prometheus URL | Pending |
| `UT-MON-463-002` | EM ConfigMap `external.prometheusUrl` populated from `monitoring.prometheus.url` | Pending |
| `UT-MON-463-003` | KA SDK ConfigMap `toolsets.prometheus/metrics.config.prometheus_url` populated from `monitoring.prometheus.url` | Pending |
| `UT-MON-463-004` | `monitoring.prometheus.enabled: false` → EM `prometheusEnabled: false`, KA `toolsets: {}` | Pending |
| `UT-MON-463-005` | `monitoring.prometheus.enabled: true` + `monitoring.alertManager.enabled: true` → both endpoints configured | Pending |
| `UT-MON-463-006` | Legacy `effectivenessmonitor.external.prometheusUrl` still works (backward compat) | Pending |
| `UT-MON-463-007` | Legacy `kubernautAgent.prometheus.url` still works (backward compat) | Pending |
| `UT-MON-463-008` | OCP detected → Prometheus URL defaults to `prometheus-k8s.openshift-monitoring.svc:9091` | Pending |
| `UT-MON-463-009` | `monitoring.prometheus.tlsCaFile` set → volumeMount on both EM and KA Deployments | Pending |
| `UT-MON-463-010` | OCP + monitoring enabled → ClusterRoleBinding for `cluster-monitoring-view` | Pending |
| `UT-MON-463-011` | Both `monitoring.*` and legacy keys set → `monitoring.*` takes precedence | Pending |
| `UT-MON-463-012` | Legacy keys used → NOTES.txt contains deprecation warning | Pending |
| `UT-MON-463-013` | `helm lint --strict` passes with monitoring block | Pending |

### Tier Skip Rationale

- **Integration**: Requires real Prometheus endpoint — covered by EM/KA component E2E tests
- **E2E**: Requires OCP cluster — covered by OCP CI/CD pipeline

---

## 9. Test Cases

### UT-MON-463-001: Unified monitoring configures both EM and KA

**Priority**: P0
**Type**: Helm smoke test (extend `scripts/helm-smoke-test.sh`)

**Test Steps**:
1. **Given**: `monitoring.prometheus.enabled: true`, `monitoring.prometheus.url: "http://prom:9090"`
2. **When**: `helm template` is run with these values
3. **Then**: EM ConfigMap contains `prometheusUrl: "http://prom:9090"` and `prometheusEnabled: true`; KA SDK ConfigMap contains `prometheus_url: "http://prom:9090"` under `toolsets.prometheus/metrics`

### UT-MON-463-006: Legacy EM keys backward compat

**Priority**: P0
**Type**: Helm smoke test

**Test Steps**:
1. **Given**: Only `effectivenessmonitor.external.prometheusUrl: "http://legacy:9090"` set (no `monitoring.*`)
2. **When**: `helm template` is run
3. **Then**: EM ConfigMap contains `prometheusUrl: "http://legacy:9090"` — existing behavior preserved

### UT-MON-463-008: OCP auto-detection

**Priority**: P0
**Type**: Helm smoke test

**Test Steps**:
1. **Given**: `monitoring.prometheus.enabled: true`, OCP capabilities simulated via `--api-versions route.openshift.io/v1`
2. **When**: `helm template` is run with `--api-versions route.openshift.io/v1`
3. **Then**: Prometheus URL defaults to `https://prometheus-k8s.openshift-monitoring.svc:9091`, TLS CA file auto-configured

---

## 10. Environmental Needs

### 10.1 Helm Smoke Tests

- **Framework**: Bash TAP (existing `scripts/helm-smoke-test.sh` pattern)
- **Tools**: `helm`, `yq`, `grep`
- **Location**: Extended in `scripts/helm-smoke-test.sh`

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Helm | 3.x | Template rendering |
| yq | 4.x | YAML parsing in tests |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| KA Go rewrite (#433) | Code | Merged (v1.3) | KA toolset config schema | N/A — finalized |
| TLS CA support (#452) | Code | Merged (v1.1) | TLS volume mount pattern | N/A — available |

### 11.2 Execution Order

1. **Phase 1**: Values + schema + helpers
2. **Phase 2**: EM template update
3. **Phase 3**: KA template update
4. **Phase 4**: OCP detection + RBAC + TLS
5. **Phase 5**: Smoke tests + deprecation notes

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/463/TEST_PLAN.md` | Strategy and test design |
| Helm smoke tests | `scripts/helm-smoke-test.sh` | Extended smoke test suite |

---

## 13. Execution

```bash
# Full smoke test suite
bash scripts/helm-smoke-test.sh

# Helm lint
helm lint charts/kubernaut/ --strict

# Template with monitoring values
helm template kubernaut charts/kubernaut/ --set monitoring.prometheus.enabled=true --set monitoring.prometheus.url=http://prom:9090

# Template with OCP capabilities
helm template kubernaut charts/kubernaut/ --api-versions route.openshift.io/v1 --set monitoring.prometheus.enabled=true
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `scripts/helm-smoke-test.sh` EM tests | Assert on `effectivenessmonitor.external.*` | Add parallel assertions for `monitoring.*` | New source of truth |
| `scripts/helm-smoke-test.sh` KA tests | Assert on `kubernautAgent.prometheus.*` | Add parallel assertions for `monitoring.*` | New source of truth |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
