# Test Plan: NetworkPolicies for All Kubernaut Services

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-285-v1.0
**Feature**: Add NetworkPolicy Helm templates for all Kubernaut services with default-deny posture
**Version**: 1.0
**Created**: 2026-04-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.4`

> **Partial Deprecation Notice**: References to "KA conversation ingress" and #592 feature-gated NetworkPolicy rules in this test plan are **stale**. The #592 conversational RAR API and its NetworkPolicy ingress rule were removed (PR #867). NetworkPolicy tests for the Kubernaut Agent should only cover the investigation traffic path (port 8443 from aianalysis-controller) and metrics ingress.

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the NetworkPolicy Helm chart templates introduced by Issue #285. The templates enforce a default-deny network posture for every Kubernaut service, with explicit allow rules matching the documented traffic matrix. Tests verify that templates render correctly under all configuration permutations (enabled/disabled, per-service overrides, OCP overlays) and that traffic flows correctly when policies are enforced in a Kind cluster with kindnet v0.30.

### 1.2 Objectives

1. **Template Correctness**: All 12 NetworkPolicy templates render valid Kubernetes manifests for every permutation of `networkPolicies.*` values.
2. **Default-Deny Enforcement**: When enabled, each service receives both Ingress and Egress policyTypes with only the documented allow rules.
3. **Opt-In Safety**: With `networkPolicies.enabled: false` (default), zero NetworkPolicy resources are rendered.
4. **Traffic Matrix Fidelity**: Allow rules match the documented inter-service communication matrix (ports, selectors, CIDRs).
5. **OCP Compatibility**: OpenShift overlay values (`values-ocp.yaml`) produce correct monitoring egress ports (9091/9094 + TLS).
6. **No Regression**: Existing deployments without NetworkPolicies are unaffected.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `helm template` assertions via Ginkgo |
| E2E test pass rate | 100% | Kind cluster with kindnet v0.30 |
| Unit-testable code coverage | >=80% | All template rendering paths exercised |
| Backward compatibility | 0 regressions | Existing `helm template` output unchanged when `networkPolicies.enabled: false` |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-GATEWAY-050**: NetworkPolicy for Gateway service (expanded to all services by #285)
- **BR-NOT-104**: Multi-channel notification delivery (PagerDuty, Teams egress)
- Issue #285: Enhancement: Add NetworkPolicies to Helm chart for all Kubernaut services
- Issue #592: Conversational RAR (future KA ingress — feature-gated placeholder)
- Issue #463: Unified monitoring config (conditional Prometheus/AlertManager egress)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Traffic Matrix — Issue #285](https://github.com/jordigilh/kubernaut/issues/285)
- [Operator-side tracking — kubernaut-operator#1](https://github.com/jordigilh/kubernaut-operator/issues/1)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Incorrect pod selectors break inter-service traffic | Services cannot communicate, notifications fail | Medium | UT-NP-285-004 through -015, E2E-NP-285-001 | Verify selectors match deployment `matchLabels` exactly |
| R2 | DNS egress rule omission blocks all service resolution | All services fail to resolve any hostname | High | UT-NP-285-002, E2E-NP-285-001 | Dedicated test for DNS port 53 UDP/TCP in every policy |
| R3 | K8s API server CIDR misconfiguration | Controllers cannot watch CRDs | High | UT-NP-285-003, E2E-NP-285-001 | Configurable `apiServerCIDR` with auto-detection fallback |
| R4 | OCP monitoring port mismatch (9090 vs 9091) | EM/HAPI cannot reach Prometheus on OCP | Medium | UT-NP-285-016 | Dedicated OCP overlay test with `values-ocp.yaml` |
| R5 | Policies rendered when disabled | Unexpected network restrictions in clusters without CNI enforcement | Low | UT-NP-285-001 | Explicit test: `enabled: false` produces zero NetworkPolicy resources |
| R6 | AuthWebhook ingress from API server blocked | Admission webhooks fail, CRD operations break | High | UT-NP-285-012, E2E-NP-285-001 | API server CIDR egress covers webhook callback path |

### 3.1 Risk-to-Test Traceability

- **R1** (selector mismatch): UT-NP-285-004 through -015 validate each service's podSelector
- **R2** (DNS): UT-NP-285-002 validates DNS egress in every policy
- **R3** (API server): UT-NP-285-003 validates configurable CIDR
- **R4** (OCP ports): UT-NP-285-016 validates OCP overlay rendering
- **R5** (disabled rendering): UT-NP-285-001 validates zero resources when disabled
- **R6** (AuthWebhook): UT-NP-285-012 validates API server ingress rule

---

## 4. Scope

### 4.1 Features to be Tested

- **NetworkPolicy templates** (`charts/kubernaut/templates/*/networkpolicy.yaml`): Per-service default-deny policies with explicit allow rules
- **Helm values schema** (`charts/kubernaut/values.yaml`, `values.schema.json`): `networkPolicies.*` configuration block
- **Helper templates** (`charts/kubernaut/templates/_helpers.tpl`): Shared NetworkPolicy helpers (DNS egress, API server egress)
- **OCP overlay** (`charts/kubernaut/values-ocp.yaml`): OpenShift-specific monitoring ports

### 4.2 Features Not to be Tested

- **Operator-managed NetworkPolicies**: Tracked in kubernaut-operator#1 (separate repo)
- **CNI plugin behavior**: NetworkPolicy enforcement depends on the CNI — we test with kindnet v0.30 only
- **External LLM egress CIDR accuracy**: Provider-specific CIDRs are user-configured, not chart-controlled

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| One `networkpolicy.yaml` per service directory | Follows existing chart pattern of one file per resource type per service |
| `helm template` + YAML parsing for unit tests | Fast, deterministic, no cluster required — validates template rendering logic |
| Kind + kindnet v0.30 for E2E | kindnet supports NetworkPolicy since v0.30; no Calico dependency needed |
| Feature-gated conditional rules for #592/#463 | Allows shipping NetworkPolicies without blocking on unreleased features |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit (Tier 1)**: >=80% of template rendering paths — all 12 service policies x enabled/disabled x per-service overrides x OCP overlay
- **E2E (Tier 3)**: Smoke-level validation that services communicate correctly with policies enforced in Kind

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests**: `helm template` rendering correctness (fast feedback, no cluster)
- **E2E tests**: Traffic flow validation in Kind cluster with enforced policies

### 5.3 Tier Skip Rationale

- **Integration (Tier 2)**: Skipped. Helm template rendering is pure YAML generation — there is no Go business logic or I/O boundary to integration-test. Template correctness is validated by unit tests (`helm template` + YAML assertions); runtime correctness is validated by E2E tests. This is consistent with standard Helm chart testing practice.

### 5.4 Business Outcome Quality Bar

Tests validate **business outcomes**:
- "When NetworkPolicies are enabled, only documented traffic flows are allowed"
- "When NetworkPolicies are disabled, the chart produces no restrictions"
- "Services communicate correctly under policy enforcement"

### 5.5 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Template rendering covers >=80% of configuration paths
4. No regressions in existing `helm template` output when `networkPolicies.enabled: false`
5. Kind E2E: all services reach `Running` and inter-service calls succeed with policies enforced

**FAIL** — any of the following:

1. Any P0 test fails
2. `helm template` produces invalid YAML for any configuration permutation
3. Existing deployments break when upgrading with `networkPolicies.enabled: false`
4. Services cannot communicate in Kind with policies enforced

### 5.6 Suspension & Resumption Criteria

**Suspend testing when**:

- Helm binary unavailable or incompatible version
- Kind cluster cannot be provisioned
- Code does not compile; templates have syntax errors

**Resume testing when**:

- Blocking condition resolved
- Templates render without syntax errors

---

## 6. Test Items

### 6.1 Unit-Testable Code (Helm template rendering)

| File | What is tested | Approx scope |
|------|---------------|--------------|
| `charts/kubernaut/templates/gateway/networkpolicy.yaml` | Gateway NetworkPolicy rendering | ~40 lines |
| `charts/kubernaut/templates/datastorage/networkpolicy.yaml` | DataStorage NetworkPolicy rendering | ~50 lines |
| `charts/kubernaut/templates/aianalysis/networkpolicy.yaml` | AI Analysis NetworkPolicy rendering | ~40 lines |
| `charts/kubernaut/templates/kubernaut-agent/networkpolicy.yaml` | Kubernaut Agent NetworkPolicy rendering | ~50 lines |
| `charts/kubernaut/templates/signalprocessing/networkpolicy.yaml` | Signal Processing NetworkPolicy rendering | ~35 lines |
| `charts/kubernaut/templates/remediationorchestrator/networkpolicy.yaml` | Remediation Orchestrator NetworkPolicy rendering | ~40 lines |
| `charts/kubernaut/templates/workflowexecution/networkpolicy.yaml` | Workflow Execution NetworkPolicy rendering | ~40 lines |
| `charts/kubernaut/templates/notification/networkpolicy.yaml` | Notification NetworkPolicy rendering | ~40 lines |
| `charts/kubernaut/templates/effectivenessmonitor/networkpolicy.yaml` | Effectiveness Monitor NetworkPolicy rendering | ~50 lines |
| `charts/kubernaut/templates/authwebhook/networkpolicy.yaml` | AuthWebhook NetworkPolicy rendering | ~40 lines |
| `charts/kubernaut/templates/infrastructure/networkpolicy.yaml` | PostgreSQL + Valkey NetworkPolicy rendering | ~60 lines |
| `charts/kubernaut/templates/_helpers.tpl` | Shared helpers (DNS egress, API server egress) | ~30 lines added |
| `charts/kubernaut/values.yaml` | `networkPolicies.*` defaults | ~30 lines added |

### 6.2 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.4` HEAD | Current branch |
| Helm | v4.1.1 | Available locally |
| Kind | v0.30+ | kindnet NetworkPolicy support |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-GATEWAY-050 | Default-deny NetworkPolicy for all services | P0 | Unit | UT-NP-285-001 | Pending |
| BR-GATEWAY-050 | DNS egress allowed for all services | P0 | Unit | UT-NP-285-002 | Pending |
| BR-GATEWAY-050 | K8s API server egress allowed (configurable CIDR) | P0 | Unit | UT-NP-285-003 | Pending |
| BR-GATEWAY-050 | Gateway: ingress from AlertManager, egress to DataStorage | P0 | Unit | UT-NP-285-004 | Pending |
| BR-GATEWAY-050 | DataStorage: ingress from consumers, egress to PostgreSQL/Valkey | P0 | Unit | UT-NP-285-005 | Pending |
| BR-GATEWAY-050 | AI Analysis: egress to KA + DataStorage | P0 | Unit | UT-NP-285-006 | Pending |
| BR-GATEWAY-050 | KA: ingress from AI Analysis, egress to LLM + DataStorage | P0 | Unit | UT-NP-285-007 | Pending |
| BR-GATEWAY-050 | Signal Processing: egress to DataStorage | P0 | Unit | UT-NP-285-008 | Pending |
| BR-GATEWAY-050 | Remediation Orchestrator: egress to DataStorage | P0 | Unit | UT-NP-285-009 | Pending |
| BR-GATEWAY-050 | Workflow Execution: egress to DataStorage + K8s API (Jobs) | P0 | Unit | UT-NP-285-010 | Pending |
| BR-GATEWAY-050 | Notification: egress to DataStorage + external webhooks | P0 | Unit | UT-NP-285-011 | Pending |
| BR-GATEWAY-050 | AuthWebhook: ingress from API server, egress to DataStorage | P0 | Unit | UT-NP-285-012 | Pending |
| BR-GATEWAY-050 | Effectiveness Monitor: egress to Prometheus + AlertManager + DS | P0 | Unit | UT-NP-285-013 | Pending |
| BR-GATEWAY-050 | PostgreSQL: ingress from DataStorage only | P1 | Unit | UT-NP-285-014 | Pending |
| BR-GATEWAY-050 | Valkey: ingress from DataStorage only | P1 | Unit | UT-NP-285-015 | Pending |
| BR-GATEWAY-050 | OCP overlay: monitoring ports 9091/9094 for EM/HAPI | P1 | Unit | UT-NP-285-016 | Pending |
| BR-GATEWAY-050 | Per-service override disables individual policy | P1 | Unit | UT-NP-285-017 | Pending |
| BR-GATEWAY-050 | Metrics scraping ingress from monitoring namespace | P1 | Unit | UT-NP-285-018 | Pending |
| BR-GATEWAY-050 | Services communicate correctly with policies enforced | P0 | E2E | E2E-NP-285-001 | Pending |
| BR-GATEWAY-050 | Default-deny blocks unauthorized traffic | P1 | E2E | E2E-NP-285-002 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-NP-285-{SEQUENCE}`

- **TIER**: `UT` (Unit), `E2E` (End-to-End)
- **NP**: NetworkPolicy domain
- **285**: Issue number
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: All `networkpolicy.yaml` templates + `_helpers.tpl` NetworkPolicy helpers + `values.yaml` defaults

**Test tool**: `helm template` with YAML parsing + Ginkgo/Gomega assertions

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NP-285-001` | Zero NetworkPolicy resources when `networkPolicies.enabled: false` (default) | Pending |
| `UT-NP-285-002` | Every service policy includes DNS egress (UDP/TCP 53) when enabled | Pending |
| `UT-NP-285-003` | Every service policy includes K8s API server egress (configurable CIDR, port 443) | Pending |
| `UT-NP-285-004` | Gateway: ingress on 8080 from AlertManager pods, egress to DataStorage on 8080 | Pending |
| `UT-NP-285-005` | DataStorage: ingress on 8080 from GW/SP/AA/HAPI/RO/WE/NT/EM/AW pods, egress to PostgreSQL 5432 + Valkey 6379 | Pending |
| `UT-NP-285-006` | AI Analysis: egress to KA on 8080 + DataStorage on 8080 | Pending |
| `UT-NP-285-007` | KA: ingress on 8080 from AI Analysis, egress to LLM (configurable CIDR:443) + DataStorage 8080 | Pending |
| `UT-NP-285-008` | Signal Processing: egress to DataStorage on 8080 only | Pending |
| `UT-NP-285-009` | Remediation Orchestrator: egress to DataStorage on 8080 only | Pending |
| `UT-NP-285-010` | Workflow Execution: egress to DataStorage on 8080 | Pending |
| `UT-NP-285-011` | Notification: egress to DataStorage 8080 + external webhooks (0.0.0.0/0:443) | Pending |
| `UT-NP-285-012` | AuthWebhook: ingress on 9443 from API server CIDR, egress to DataStorage 8080 | Pending |
| `UT-NP-285-013` | Effectiveness Monitor: egress to Prometheus (configurable URL/port) + AlertManager + DataStorage | Pending |
| `UT-NP-285-014` | PostgreSQL: ingress on 5432 from DataStorage pods only (when `postgresql.enabled`) | Pending |
| `UT-NP-285-015` | Valkey: ingress on 6379 from DataStorage pods only (when `valkey.enabled`) | Pending |
| `UT-NP-285-016` | OCP overlay: EM/HAPI Prometheus egress uses port 9091, AlertManager uses 9094 | Pending |
| `UT-NP-285-017` | Per-service `networkPolicies.<service>.enabled: false` skips that service's policy | Pending |
| `UT-NP-285-018` | Metrics scraping: ingress on 9090 from monitoring namespace (when configured) | Pending |

### Tier 3: E2E Tests

**Testable code scope**: Full Helm chart deployed in Kind cluster with kindnet v0.30 NetworkPolicy enforcement

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-NP-285-001` | All services reach Running and inter-service communication succeeds with policies enforced | Pending |
| `E2E-NP-285-002` | Unauthorized cross-service traffic is blocked by default-deny | Pending |

### Tier Skip Rationale

- **Integration (Tier 2)**: Helm templates are pure YAML rendering — no Go business logic or I/O boundary to integration-test. Template correctness is fully validated by `helm template` unit tests; runtime enforcement is validated by E2E tests.

---

## 9. Test Cases

### UT-NP-285-001: Zero policies when disabled

**BR**: BR-GATEWAY-050
**Priority**: P0
**Type**: Unit
**File**: `test/unit/helm/networkpolicy_test.go`

**Preconditions**:
- Helm v4+ available
- Chart renders without errors

**Test Steps**:
1. **Given**: Default `values.yaml` (no `networkPolicies` key or `networkPolicies.enabled: false`)
2. **When**: `helm template` is executed
3. **Then**: Output contains zero `kind: NetworkPolicy` resources

**Acceptance Criteria**:
- **Behavior**: Chart renders all existing resources unchanged
- **Correctness**: No NetworkPolicy resources in output
- **Accuracy**: Existing resource count is preserved

### UT-NP-285-002: DNS egress in every policy

**BR**: BR-GATEWAY-050
**Priority**: P0
**Type**: Unit
**File**: `test/unit/helm/networkpolicy_test.go`

**Test Steps**:
1. **Given**: `networkPolicies.enabled: true`
2. **When**: `helm template` is executed
3. **Then**: Every NetworkPolicy egress block includes port 53 (UDP and TCP) to `kube-system` namespace

**Acceptance Criteria**:
- **Behavior**: All services can resolve DNS
- **Correctness**: Both UDP and TCP port 53 present
- **Accuracy**: Namespace selector targets `kube-system` (where CoreDNS runs)

### UT-NP-285-003: K8s API server egress

**BR**: BR-GATEWAY-050
**Priority**: P0
**Type**: Unit
**File**: `test/unit/helm/networkpolicy_test.go`

**Test Steps**:
1. **Given**: `networkPolicies.enabled: true`, `networkPolicies.apiServerCIDR: "10.96.0.1/32"`
2. **When**: `helm template` is executed
3. **Then**: Every NetworkPolicy egress block includes CIDR `10.96.0.1/32` on port 443

**Acceptance Criteria**:
- **Correctness**: Exact CIDR from values is rendered
- **Accuracy**: Port 443 TCP

### UT-NP-285-004: Gateway traffic rules

**BR**: BR-GATEWAY-050
**Priority**: P0
**Type**: Unit
**File**: `test/unit/helm/networkpolicy_test.go`

**Test Steps**:
1. **Given**: `networkPolicies.enabled: true`
2. **When**: `helm template` is executed
3. **Then**: Gateway policy has:
   - `podSelector: { matchLabels: { app: gateway } }`
   - Ingress: port 8080 from pods with alertmanager label (configurable)
   - Egress: port 8080 to `app: datastorage` pods
   - Egress: port 9090 (metrics) — internal only

### UT-NP-285-005 through UT-NP-285-015: Per-service traffic rules

**Priority**: P0 (005-013), P1 (014-015)
**Pattern**: Each test validates the podSelector, ingress rules, and egress rules for one service against the documented traffic matrix. The test structure mirrors UT-NP-285-004.

### UT-NP-285-016: OCP overlay monitoring ports

**BR**: BR-GATEWAY-050
**Priority**: P1
**Type**: Unit
**File**: `test/unit/helm/networkpolicy_test.go`

**Test Steps**:
1. **Given**: `networkPolicies.enabled: true` + `values-ocp.yaml` overlay
2. **When**: `helm template` with `-f values-ocp.yaml`
3. **Then**: EM egress to Prometheus uses port 9091, AlertManager uses port 9094

### UT-NP-285-017: Per-service disable

**BR**: BR-GATEWAY-050
**Priority**: P1
**Type**: Unit
**File**: `test/unit/helm/networkpolicy_test.go`

**Test Steps**:
1. **Given**: `networkPolicies.enabled: true`, `networkPolicies.notification.enabled: false`
2. **When**: `helm template` is executed
3. **Then**: All services except Notification have NetworkPolicy; Notification has none

### UT-NP-285-018: Metrics scraping ingress

**BR**: BR-GATEWAY-050
**Priority**: P1
**Type**: Unit
**File**: `test/unit/helm/networkpolicy_test.go`

**Test Steps**:
1. **Given**: `networkPolicies.enabled: true`, `networkPolicies.monitoring.namespace: monitoring`
2. **When**: `helm template` is executed
3. **Then**: Each service with a metrics port (9090) has ingress from `monitoring` namespace on that port

### E2E-NP-285-001: Services communicate with policies

**BR**: BR-GATEWAY-050
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/helm/networkpolicy_e2e_test.go`

**Test Steps**:
1. **Given**: Kind cluster with kindnet v0.30, Helm chart installed with `networkPolicies.enabled: true`
2. **When**: All services are deployed and reach Running
3. **Then**: Gateway can receive AlertManager webhooks; DataStorage can reach PostgreSQL; controllers reconcile CRDs successfully

### E2E-NP-285-002: Unauthorized traffic blocked

**BR**: BR-GATEWAY-050
**Priority**: P1
**Type**: E2E
**File**: `test/e2e/helm/networkpolicy_e2e_test.go`

**Test Steps**:
1. **Given**: Kind cluster with policies enforced
2. **When**: A test pod in the namespace attempts to connect to PostgreSQL on port 5432
3. **Then**: Connection is refused/timed out (only DataStorage pods are allowed)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Tools**: `helm template` CLI for rendering, Go `gopkg.in/yaml.v3` or `k8s.io/apimachinery/pkg/util/yaml` for parsing
- **Mocks**: None — pure template rendering
- **Location**: `test/unit/helm/`

### 10.2 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster with kindnet v0.30 (built-in NetworkPolicy support)
- **Tools**: `helm`, `kubectl`, Kind
- **Location**: `test/e2e/helm/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.24 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| Helm | v4.x | Template rendering |
| Kind | v0.30+ | E2E cluster with kindnet NetworkPolicy |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Helm v4+ | Tool | Available | UT tests cannot run | N/A |
| Kind v0.30+ | Tool | Available | E2E tests cannot run | Skip E2E tier |
| #244 (FileWatcher) | Code | Closed | NT RBAC alignment | N/A — already merged |

### 11.2 Execution Order

1. **Phase 1**: Unit tests for global controls (enabled/disabled, DNS, API server)
2. **Phase 2**: Unit tests for per-service policies (traffic matrix)
3. **Phase 3**: Unit tests for overlays and overrides (OCP, per-service disable, metrics)
4. **Phase 4**: E2E tests in Kind cluster

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/285/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/helm/networkpolicy_test.go` | `helm template` rendering tests |
| E2E test suite | `test/e2e/helm/networkpolicy_e2e_test.go` | Kind cluster traffic validation |

---

## 13. Execution

```bash
# Unit tests (helm template rendering)
go test ./test/unit/helm/... -ginkgo.v

# E2E tests (Kind cluster)
go test ./test/e2e/helm/... -ginkgo.v -timeout 10m

# Specific test by ID
go test ./test/unit/helm/... -ginkgo.focus="UT-NP-285-001"
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | N/A | N/A | NetworkPolicies are additive; no existing tests affected |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-04 | Initial test plan |
