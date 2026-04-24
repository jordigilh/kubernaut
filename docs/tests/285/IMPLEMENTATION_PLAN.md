# Implementation Plan: NetworkPolicies for All Kubernaut Services

**Issue**: #285
**Test Plan**: [TP-285-v1.0](TEST_PLAN.md)
**Branch**: `development/v1.4`
**Created**: 2026-04-04

---

## Overview

This plan implements default-deny NetworkPolicy Helm templates for all 12 Kubernaut services, following strict TDD RED → GREEN → REFACTOR with rigorous checkpoints between phases.

### Services in scope (12 policies)

| # | Service | Pod selector | Directory |
|---|---------|-------------|-----------|
| 1 | Gateway | `app: gateway` | `templates/gateway/` |
| 2 | DataStorage | `app: datastorage` | `templates/datastorage/` |
| 3 | AI Analysis | `app: aianalysis-controller` | `templates/aianalysis/` |
| 4 | HolmesGPT API | `app: holmesgpt-api` | `templates/holmesgpt-api/` |
| 5 | Signal Processing | `app: signalprocessing-controller` | `templates/signalprocessing/` |
| 6 | Remediation Orchestrator | `app: remediationorchestrator-controller` | `templates/remediationorchestrator/` |
| 7 | Workflow Execution | `app: workflowexecution-controller` | `templates/workflowexecution/` |
| 8 | Notification | `app: notification-controller` | `templates/notification/` |
| 9 | Effectiveness Monitor | `app: effectivenessmonitor-controller` | `templates/effectivenessmonitor/` |
| 10 | AuthWebhook | `app: authwebhook` | `templates/authwebhook/` |
| 11 | PostgreSQL | `app: postgresql` | `templates/infrastructure/` |
| 12 | Valkey | `app: valkey` | `templates/infrastructure/` |

---

## Phase 1: TDD RED — Unit Test Scaffolding & Failing Tests

**Goal**: Write all unit tests (UT-NP-285-001 through UT-NP-285-018) that fail because no NetworkPolicy templates exist yet.

### Phase 1.1: Test infrastructure setup

1. Create `test/unit/helm/` directory structure
2. Create `test/unit/helm/helm_suite_test.go` — Ginkgo bootstrap
3. Create `test/unit/helm/networkpolicy_test.go` — test file
4. Implement `helmTemplate()` helper function that shells out to `helm template` with configurable values overrides and returns parsed YAML documents
5. Implement `filterByKind()` helper to extract resources of a given `kind` from rendered output
6. Verify the test infrastructure compiles and can render the existing chart

### Phase 1.2: Global control tests (RED)

Write failing tests for:

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-NP-285-001 | `enabled: false` → zero NetworkPolicy resources | No `networkPolicies` values key exists, but test structure validates zero policies (this one may pass trivially — still write it to lock the contract) |
| UT-NP-285-002 | `enabled: true` → every policy has DNS egress (port 53 UDP/TCP) | No templates exist |
| UT-NP-285-003 | `enabled: true` → every policy has API server egress (configurable CIDR:443) | No templates exist |

### Phase 1.3: Per-service traffic rule tests (RED)

Write failing tests for each service:

| Test ID | Service | Key assertions |
|---------|---------|---------------|
| UT-NP-285-004 | Gateway | podSelector=`gateway`, ingress 8080, egress to DS 8080 |
| UT-NP-285-005 | DataStorage | podSelector=`datastorage`, ingress 8080 from 8 consumers, egress PG 5432 + Valkey 6379 |
| UT-NP-285-006 | AI Analysis | podSelector=`aianalysis-controller`, egress HAPI 8080 + DS 8080 |
| UT-NP-285-007 | HolmesGPT | podSelector=`holmesgpt-api`, ingress 8080 from AA, egress LLM CIDR:443 + DS 8080 |
| UT-NP-285-008 | Signal Processing | podSelector=`signalprocessing-controller`, egress DS 8080 |
| UT-NP-285-009 | Remediation Orchestrator | podSelector=`remediationorchestrator-controller`, egress DS 8080 |
| UT-NP-285-010 | Workflow Execution | podSelector=`workflowexecution-controller`, egress DS 8080 |
| UT-NP-285-011 | Notification | podSelector=`notification-controller`, egress DS 8080 + external 443 |
| UT-NP-285-012 | AuthWebhook | podSelector=`authwebhook`, ingress 9443 from API server CIDR, egress DS 8080 |
| UT-NP-285-013 | Effectiveness Monitor | podSelector=`effectivenessmonitor-controller`, egress Prometheus + AM + DS |
| UT-NP-285-014 | PostgreSQL | podSelector=`postgresql`, ingress 5432 from DS only |
| UT-NP-285-015 | Valkey | podSelector=`valkey`, ingress 6379 from DS only |

### Phase 1.4: Override and overlay tests (RED)

| Test ID | What it asserts |
|---------|----------------|
| UT-NP-285-016 | OCP overlay: EM Prometheus egress port 9091, AM port 9094 |
| UT-NP-285-017 | `networkPolicies.notification.enabled: false` → Notification policy absent, others present |
| UT-NP-285-018 | Metrics scraping ingress on 9090 from configured monitoring namespace |

### Phase 1.5: Verify RED state

```bash
go test ./test/unit/helm/... -ginkgo.v -count=1
# Expected: All 18 UT tests FAIL (no templates produce NetworkPolicy resources)
# UT-NP-285-001 may pass trivially (zero policies when disabled) — acceptable
```

---

## Checkpoint 1: RED Phase Validation

**Gate criteria**:
- [ ] All 18 unit tests compile and execute
- [ ] Tests fail for the expected reasons (no NetworkPolicy resources rendered)
- [ ] UT-NP-285-001 passes (locking the "disabled = no policies" contract)
- [ ] Test helpers (`helmTemplate`, `filterByKind`) work correctly
- [ ] No anti-patterns: no `time.Sleep()`, no `Skip()`, no `ToNot(BeNil())` existence-only assertions
- [ ] Test IDs match the test plan

**Actions if gate fails**: Fix test infrastructure issues before proceeding to GREEN.

---

## Phase 2: TDD GREEN — Minimal Implementation

**Goal**: Implement the minimum Helm templates and values to make all 18 unit tests pass.

### Phase 2.1: Values schema

1. Add `networkPolicies` block to `charts/kubernaut/values.yaml`:
   ```yaml
   networkPolicies:
     enabled: false
     apiServerCIDR: ""
     monitoring:
       namespace: ""
     externalWebhooks:
       cidr: "0.0.0.0/0"
       port: 443
     externalLLM:
       cidr: "0.0.0.0/0"
       port: 443
   ```
2. Add per-service override keys (e.g., `networkPolicies.gateway.enabled`)
3. Update `values.schema.json` with the new properties

### Phase 2.2: Helper templates

Add to `charts/kubernaut/templates/_helpers.tpl`:

1. `kubernaut.networkpolicy.dnsEgress` — renders DNS egress rule (UDP/TCP 53 to kube-system)
2. `kubernaut.networkpolicy.apiServerEgress` — renders K8s API server egress rule (configurable CIDR:443)
3. `kubernaut.networkpolicy.metricsIngress` — renders metrics scraping ingress (port 9090 from monitoring namespace)

### Phase 2.3: Service NetworkPolicy templates

Create one `networkpolicy.yaml` file per service directory. Each template:
- Wrapped in `{{- if .Values.networkPolicies.enabled }}` and per-service override check
- Uses `podSelector.matchLabels.app` matching the deployment's selector
- Declares `policyTypes: [Ingress, Egress]`
- Includes DNS egress via helper
- Includes API server egress via helper
- Includes service-specific ingress/egress rules per the traffic matrix

Order of implementation (by dependency — DataStorage is hub):

1. **DataStorage** (UT-NP-285-005) — most ingress sources, egress to PG + Valkey
2. **PostgreSQL** (UT-NP-285-014) — ingress from DS only
3. **Valkey** (UT-NP-285-015) — ingress from DS only
4. **Gateway** (UT-NP-285-004) — ingress from external, egress to DS
5. **AI Analysis** (UT-NP-285-006) — egress to HAPI + DS
6. **HolmesGPT API** (UT-NP-285-007) — ingress from AA, egress to LLM + DS
7. **Signal Processing** (UT-NP-285-008) — egress to DS
8. **Remediation Orchestrator** (UT-NP-285-009) — egress to DS
9. **Workflow Execution** (UT-NP-285-010) — egress to DS
10. **Notification** (UT-NP-285-011) — egress to DS + external webhooks
11. **Effectiveness Monitor** (UT-NP-285-013) — egress to Prometheus + AM + DS
12. **AuthWebhook** (UT-NP-285-012) — ingress from API server, egress to DS

### Phase 2.4: OCP overlay

Add to `charts/kubernaut/values-ocp.yaml`:
```yaml
networkPolicies:
  # OCP monitoring uses different ports
  # EM/HAPI Prometheus: 9091 (HTTPS), AlertManager: 9094 (HTTPS)
```

### Phase 2.5: Verify GREEN state

```bash
go test ./test/unit/helm/... -ginkgo.v -count=1
# Expected: All 18 UT tests PASS
helm template test-release charts/kubernaut/ --set networkPolicies.enabled=true | kubectl apply --dry-run=client -f -
# Expected: All resources valid
```

---

## Checkpoint 2: GREEN Phase Validation

**Gate criteria**:
- [ ] All 18 unit tests pass
- [ ] `helm template` with `networkPolicies.enabled=true` produces valid YAML (dry-run succeeds)
- [ ] `helm template` with default values produces zero NetworkPolicy resources
- [ ] No lint errors in templates (`helm lint charts/kubernaut/`)
- [ ] Pod selectors exactly match deployment `matchLabels` in every template
- [ ] Traffic matrix fidelity: cross-reference every egress/ingress rule against Issue #285 matrix
- [ ] Values schema updated (`values.schema.json`)

**Due diligence checklist**:
1. For each service, verify the `podSelector.matchLabels.app` value matches the deployment's `spec.selector.matchLabels.app`
2. Verify DNS egress targets `kube-system` namespace (where CoreDNS runs)
3. Verify API server egress uses the configurable CIDR, not a hardcoded value
4. Verify DataStorage ingress allows all 8 consumer services (GW, SP, AA, HAPI, RO, WE, NT, EM, AW — 9 total)
5. Verify PostgreSQL/Valkey ingress is restricted to DataStorage pods only
6. Verify AuthWebhook ingress uses the API server CIDR (same as egress CIDR)
7. Verify external egress (LLM, webhooks) uses configurable CIDR:port

**Actions if gate fails**: Fix template issues, re-run tests until all pass.

---

## Phase 3: TDD REFACTOR — Template Optimization

**Goal**: Improve template quality, reduce duplication, and enhance maintainability without changing behavior.

### Phase 3.1: Template DRY-up

1. Extract common egress patterns into named helpers:
   - `kubernaut.networkpolicy.datastorageEgress` — reused by 9 services
   - `kubernaut.networkpolicy.commonEgress` — DNS + API server + optional metrics ingress
2. Consolidate PostgreSQL + Valkey policies into a single `infrastructure/networkpolicy.yaml` using range/if
3. Add inline documentation comments explaining each rule's purpose

### Phase 3.2: Values organization

1. Group per-service overrides under a consistent pattern:
   ```yaml
   networkPolicies:
     enabled: false
     gateway:
       enabled: true  # inherits from parent .enabled when not set
     # ... each service follows same pattern
   ```
2. Add descriptive comments in `values.yaml` explaining each setting

### Phase 3.3: Verify REFACTOR preserves behavior

```bash
# All tests still pass after refactoring
go test ./test/unit/helm/... -ginkgo.v -count=1
# Template output is semantically identical
helm template test-release charts/kubernaut/ --set networkPolicies.enabled=true > /tmp/after-refactor.yaml
# Diff against pre-refactor output (saved in Phase 2)
diff /tmp/before-refactor.yaml /tmp/after-refactor.yaml
```

---

## Checkpoint 3: REFACTOR Phase Validation

**Gate criteria**:
- [ ] All 18 unit tests still pass
- [ ] `helm template` output is semantically identical to pre-refactor
- [ ] `helm lint` passes
- [ ] No duplicated egress/ingress blocks across templates (DRY)
- [ ] Comments explain non-obvious rules (API server CIDR, monitoring conditional)
- [ ] `values.yaml` is well-documented with inline comments

**Due diligence**:
1. Count unique helper invocations — each common pattern should use a helper, not inline YAML
2. Verify no template uses hardcoded namespaces (all configurable via values)
3. Check that per-service override pattern is consistent across all 12 services

**Actions if gate fails**: Continue refactoring until DRY and lint-clean.

---

## Phase 4: TDD RED — E2E Test Scaffolding

**Goal**: Write E2E tests that fail because Kind cluster doesn't have the chart installed with policies.

### Phase 4.1: E2E test infrastructure

1. Create `test/e2e/helm/` directory structure
2. Create `test/e2e/helm/helm_e2e_suite_test.go` — Ginkgo bootstrap with Kind cluster lifecycle
3. Create `test/e2e/helm/networkpolicy_e2e_test.go` — E2E test file
4. Implement Kind cluster creation with kindnet v0.30 (NetworkPolicy support)
5. Implement Helm install with `networkPolicies.enabled=true` and `apiServerCIDR` auto-detected from cluster

### Phase 4.2: E2E tests (RED)

| Test ID | What it asserts |
|---------|----------------|
| E2E-NP-285-001 | All services reach Running; Gateway responds to HTTP health check; DataStorage can query PostgreSQL |
| E2E-NP-285-002 | A test pod cannot connect to PostgreSQL:5432 (only DataStorage is allowed) |

### Phase 4.3: Verify RED state

```bash
go test ./test/e2e/helm/... -ginkgo.v -timeout 10m
# Expected: Tests fail (no cluster or chart not installed)
```

---

## Checkpoint 4: E2E RED Phase Validation

**Gate criteria**:
- [ ] E2E tests compile
- [ ] Test infrastructure can create Kind cluster
- [ ] Tests fail for expected reasons (chart not installed with policies, or services not ready)
- [ ] No anti-patterns in E2E tests

---

## Phase 5: TDD GREEN — E2E Implementation

**Goal**: Make E2E tests pass by deploying the chart with NetworkPolicies in Kind.

### Phase 5.1: Kind cluster setup

1. Create Kind cluster with kindnet v0.30
2. Install Helm chart with `networkPolicies.enabled=true`, `apiServerCIDR` auto-detected
3. Create prerequisite secrets (PostgreSQL, Valkey, LLM credentials)
4. Wait for all deployments to reach Ready

### Phase 5.2: Traffic validation

1. Verify all pods are Running
2. Execute health checks on Gateway and DataStorage endpoints from within the cluster
3. Create a test pod and verify it cannot connect to PostgreSQL (blocked by policy)

### Phase 5.3: Verify GREEN state

```bash
go test ./test/e2e/helm/... -ginkgo.v -timeout 10m
# Expected: Both E2E tests pass
```

---

## Checkpoint 5: E2E GREEN Phase Validation

**Gate criteria**:
- [ ] Both E2E tests pass
- [ ] All services reach Running with policies enforced
- [ ] Default-deny is verified (unauthorized traffic blocked)
- [ ] No flaky behavior across 3 consecutive runs

---

## Phase 6: TDD REFACTOR — E2E Cleanup

**Goal**: Improve E2E test robustness and cleanup.

### Phase 6.1: Test improvements

1. Add retry logic for transient cluster setup failures
2. Ensure cluster cleanup in `AfterSuite`
3. Add descriptive `By()` annotations for each step

### Phase 6.2: Documentation

1. Update `charts/kubernaut/README.md` with NetworkPolicy section
2. Add traffic matrix documentation in chart README
3. Update `values.yaml` inline comments

### Phase 6.3: Final verification

```bash
# Full test suite
go test ./test/unit/helm/... -ginkgo.v -count=1
go test ./test/e2e/helm/... -ginkgo.v -timeout 10m

# Helm lint
helm lint charts/kubernaut/
helm lint charts/kubernaut/ -f charts/kubernaut/values-ocp.yaml
```

---

## Checkpoint 6: Final Validation

**Gate criteria**:
- [ ] All 18 unit tests pass
- [ ] Both E2E tests pass
- [ ] `helm lint` passes for default and OCP values
- [ ] `helm template` dry-run succeeds
- [ ] Chart README updated with NetworkPolicy documentation
- [ ] No anti-patterns in any test file
- [ ] All test IDs match the test plan
- [ ] Traffic matrix fully covered (cross-reference Issue #285 acceptance criteria)

**Acceptance criteria from Issue #285**:
- [x] Default-deny NetworkPolicy for each Kubernaut service
- [x] Per-service allow rules match the traffic matrix
- [x] Traffic matrix reconciled with #244 (NT RBAC), #463 (conditional Prometheus), #592 (KA conversation ingress)
- [x] DNS egress (port 53) allowed for all services
- [x] K8s API server egress allowed for all services (configurable CIDR)
- [x] External egress (LLM, Slack, PagerDuty, Prometheus) configurable per service
- [x] `networkPolicies.enabled: false` by default (opt-in)
- [x] `helm template` renders correctly with policies enabled and disabled
- [x] No regression: existing deployments without NetworkPolicies are unaffected

---

## Summary

| Phase | Type | Deliverable | Tests |
|-------|------|-------------|-------|
| 1 | TDD RED | Unit test scaffolding + 18 failing tests | UT-NP-285-001 to -018 |
| **CP1** | **Checkpoint** | **RED validation** | |
| 2 | TDD GREEN | 12 NetworkPolicy templates + values + helpers | All 18 UT pass |
| **CP2** | **Checkpoint** | **GREEN validation + traffic matrix audit** | |
| 3 | TDD REFACTOR | DRY templates, helper extraction, docs | All 18 UT pass (same output) |
| **CP3** | **Checkpoint** | **REFACTOR validation** | |
| 4 | TDD RED | E2E test scaffolding + 2 failing tests | E2E-NP-285-001, -002 |
| **CP4** | **Checkpoint** | **E2E RED validation** | |
| 5 | TDD GREEN | Kind cluster setup + policy deployment | Both E2E pass |
| **CP5** | **Checkpoint** | **E2E GREEN validation** | |
| 6 | TDD REFACTOR | E2E cleanup + chart README | All tests pass |
| **CP6** | **Checkpoint** | **Final validation against #285 acceptance criteria** | |
