# Test Plan: Helm Policy ConfigMap Ownership Boundary

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2460-v1.0
**Feature**: Prevent Helm upgrades from pruning chart-managed policy ConfigMaps when
`existingConfigMap` is configured, while preserving safe distinct-name externalization.
**Version**: 1.0
**Created**: 2026-09-28
**Author**: Kubernaut development agent
**Status**: Active
**Branch**: `fix/2459-workflow-discovery-audit`

---

## 1. Introduction

### 1.1 Purpose

Issue #2460 exposed an unsafe Helm ownership transition: a release could omit a
chart-managed policy ConfigMap by setting `existingConfigMap` to the same reserved
name, while the controller Deployment continued mounting that identity. Helm then
pruned the ConfigMap from the previous release manifest and wedged the rollout. This
plan verifies the declarative ownership boundary, the safe distinct-name migration,
and the operator recovery path.

### 1.2 Objectives

1. Schema validation rejects same-name externalization for AIAnalysis and
   SignalProcessing before template rendering or resource mutation.
2. Distinct external ConfigMap names remain supported and are mounted by the correct
   controller Deployments.
3. A rejected live upgrade leaves the chart-managed policy ConfigMaps and healthy
   controller rollouts intact.
4. A valid live upgrade preserves pre-created external ConfigMaps, prunes only the
   obsolete chart-managed policy identities, and leaves both controllers healthy.
5. Upgrade and two-phase recovery instructions are documented and generated chart
   references remain synchronized.

### 1.3 Success metrics

| Metric | Target | Measurement |
|---|---:|---|
| Reserved-name structural cases | 2/2 pass | `helm unittest -f 'tests/policies_schema_validation_test.yaml'` |
| Distinct-name structural cases | 5/5 pass | Same focused helm-unittest suite |
| Live invalid-upgrade protection | Pass | `ST-CHART-POLICY-OWNERSHIP-001` |
| Live distinct migration and rollout | Pass | `ST-CHART-POLICY-OWNERSHIP-002` |
| Existing chart regression | 0 new failures | Full Helm unit suite and lint |
| Generated documentation drift | 0 | `make generate-helm-config-docs` + diff check |

---

## 2. References

### 2.1 Authority

- Issue #2460: Helm upgrade can prune policy ConfigMaps and wedge controller rollout
- [BR-PLATFORM-005](../../requirements/BR-PLATFORM-005-helm-chart-operator-security-parity.md)
  — Helm upgrade lifecycle and resource pruning
- [BR-PLATFORM-010](../../requirements/BR-PLATFORM-010-helm-chart-schema-level-input-validation.md)
  — schema-level validation and SI-10 input validation
- [DD-PLATFORM-011](../../architecture/decisions/DD-PLATFORM-011-helm-policy-configmap-ownership-boundary.md)
  — approved ownership boundary

### 2.2 Cross-references

- `charts/kubernaut/values.schema.json`
- `charts/kubernaut/templates/aianalysis/aianalysis.yaml`
- `charts/kubernaut/templates/signalprocessing/signalprocessing.yaml`
- `charts/kubernaut/tests/policies_schema_validation_test.yaml`
- `scripts/helm-smoke-test.sh`
- `charts/kubernaut/README.md`

---

## 3. Risks and mitigations

| ID | Risk | Impact | Affected tests | Mitigation |
|---|---|---|---|---|
| R1 | A `not` schema clause is written without `required`, so it also rejects omitted values or behaves unexpectedly. | Valid installations fail or the guard is ineffective. | IT-HELM-2460-001/002 | Test exact reserved values and retain distinct-name happy paths; validate with `helm lint` and `helm template`. |
| R2 | `helm upgrade --reuse-values` partially retains old policy content while switching to an external name. | The chart appears to accept an ambiguous ownership configuration. | IT-HELM-2460-003/004, ST-CHART-POLICY-OWNERSHIP-002 | Explicitly clear `content` in the migration command and assert the live Deployment volume names. |
| R3 | Helm prunes the old chart-managed ConfigMap before the Deployment is healthy. | Controller rollout fails or remains wedged. | ST-CHART-POLICY-OWNERSHIP-002 | Pre-create distinct external ConfigMaps, use `--wait`, then assert both `rollout status` and mounted names. |
| R4 | The rejected same-name upgrade mutates release state before validation. | Existing policy resources could still be lost. | ST-CHART-POLICY-OWNERSHIP-001 | Assert the Helm command fails and both reserved ConfigMaps retain non-empty data; assert controller rollouts remain healthy. |
| R5 | The full CI image setup is unavailable locally, or Helm 4 server-side apply behavior differs from the targeted run. | Application rollout evidence is incomplete. | ST-* | Preserve the targeted lifecycle evidence and rerun the complete smoke flow in CI/Kind before declaring complete. |

### 3.1 Risk-to-test traceability

R1–R4 have direct structural or targeted live coverage. R5 is an infrastructure
gate: the full rollout portion is reported as pending rather than converted into a
skipped test.

---

## 4. Scope

### 4.1 Features to be tested

- AIAnalysis policy reserved identity `aianalysis-policies` and external mount.
- SignalProcessing policy reserved identity `signalprocessing-policy` and external
  mount.
- Schema rejection before rendering for both reserved-name values.
- Live Helm upgrade rejection, ConfigMap preservation, distinct-name migration,
  Helm pruning of the obsolete managed identities, and controller rollout health.
- Operator upgrade and recovery documentation.

### 4.2 Features not to be tested

- Same-name externalization for notification routing or unrelated ConfigMaps; those
  are separate contracts and are not part of Issue #2460's policy scope.
- Automatic validation of an external ConfigMap's existence/content during a
  GitOps-only render; live Kubernetes state is intentionally not a primary guard.
- Same-name takeover via hooks, bridges, or `resource-policy: keep`; rejected by
  DD-PLATFORM-011 unless a future requirement mandates it.
- Go unit tests; no Go production logic changes are introduced.

### 4.3 Design decisions

| Decision | Rationale |
|---|---|
| Declarative schema guard | Works deterministically in Helm CLI and GitOps renderers without `lookup`. |
| Distinct external identity | Avoids switching ownership of one Kubernetes resource identity. |
| Structural + live tests | Structural tests prove render/mount logic; only a real `helm upgrade` proves release-state pruning and rollout behavior. |
| No `resource-policy: keep` fix | Retention alone does not establish correct ownership or mount safety. |

---

## 5. Approach and TDD phases

### 5.1 Coverage policy

This is a Helm chart change. The structural tier uses `helm-unittest`; the
integration/E2E-equivalent tier uses the repository's TAP-producing live Kind smoke
flow. Both tiers are required because static rendering cannot exercise Helm's stored
release manifest diff.

### 5.2 RED

1. Add two schema-failure tests for reserved AIAnalysis and SignalProcessing names.
2. Add live smoke assertions for rejected same-name upgrade and distinct migration.
3. Run the focused structural suite against the current chart to demonstrate the
   reserved-name tests fail before implementation.

### 5.3 GREEN

1. Add the minimal `not` + `required` + `const` schema clauses.
2. Update values/schema descriptions and operator upgrade guidance.
3. Wire the smoke function into Flow A and implement the distinct-name assertions.
4. Run focused structural tests and static chart validation.

### 5.4 REFACTOR

- Consolidate ownership language across `values.yaml`, generated values reference,
  README, DD-PLATFORM-011, and the test plan.
- Keep the smoke assertions explicit and diagnostics useful without adding hooks,
  live lookups, or new production components.
- Confirm generated documentation is reproducible from the schema.

### 5.5 Pass/fail criteria

**PASS** requires all of the following:

1. IT-HELM-2460-001..007 pass.
2. ST-CHART-POLICY-OWNERSHIP-001/002 pass on a real Kind/Kubernetes cluster.
3. `helm lint --strict` and the full Helm unit suite pass.
4. Generated Helm values documentation has no diff after regeneration.
5. No unrelated working-tree files are modified.

**FAIL**: any P0 structural/live case fails, the chart accepts a reserved-name
externalization, a valid migration wedges either controller, or generated docs are
stale.

### 5.6 Suspension and resumption

Suspend only the live tier when Kind cannot be provisioned or Helm/Kubernetes is
unavailable. Resume it in CI or on a clean Kind cluster. Do not mark the live
scenarios passed based solely on `helm template`.

---

## 6. Test items and wiring manifest

### 6.1 Structural test items

| File | Scope |
|---|---|
| `charts/kubernaut/values.schema.json` | Reserved-name policy ownership constraints |
| `charts/kubernaut/templates/aianalysis/aianalysis.yaml` | External policy volume mount |
| `charts/kubernaut/templates/signalprocessing/signalprocessing.yaml` | External policy volume mount and generated policy identity |
| `charts/kubernaut/tests/policies_schema_validation_test.yaml` | Schema rejection, external mounts, generated identity, and managed-resource omission assertions |

### 6.2 Live test items

| File | Scope |
|---|---|
| `scripts/helm-smoke-test.sh` | Real invalid/valid `helm upgrade`, ConfigMap lifecycle, rollout health |
| Kind cluster | Helm release history and Kubernetes resources |

### 6.3 Wiring manifest

| Component | Production entry point | Wiring location | Proving test |
|---|---|---|---|
| AIAnalysis policy schema guard | `helm template`, `helm install`, `helm upgrade` | `charts/kubernaut/values.schema.json` | IT-HELM-2460-001 |
| SignalProcessing policy schema guard | `helm template`, `helm install`, `helm upgrade` | `charts/kubernaut/values.schema.json` | IT-HELM-2460-002 |
| AIAnalysis external policy mount | Rendered `aianalysis-controller` Deployment | `templates/aianalysis/aianalysis.yaml` | IT-HELM-2460-003, ST-CHART-POLICY-OWNERSHIP-002 |
| SignalProcessing external policy mount | Rendered `signalprocessing-controller` Deployment | `templates/signalprocessing/signalprocessing.yaml` | IT-HELM-2460-004, ST-CHART-POLICY-OWNERSHIP-002 |
| SignalProcessing generated policy identity | Rendered `signalprocessing-config` ConfigMap | `templates/signalprocessing/signalprocessing.yaml` | IT-HELM-2460-005 |
| Upgrade lifecycle protection | Flow A live Helm release | `scripts/helm-smoke-test.sh` → `run_policy_ownership_001` | ST-CHART-POLICY-OWNERSHIP-001/002 |

---

## 7. BR and control coverage matrix

| Requirement/control | Description | Priority | Tier | Test ID | Status |
|---|---|---|---|---|---|
| BR-PLATFORM-010 / SI-10 | Reserved policy ownership input is rejected at schema level | P0 | Structural | IT-HELM-2460-001/002 | Pass |
| BR-PLATFORM-010 | Distinct external policy names remain valid, mount correctly, are reflected in generated config, and omit managed resources | P0 | Structural | IT-HELM-2460-003..007 | Pass |
| BR-PLATFORM-005 FR-9 | Unsafe upgrade cannot prune the still-chart-owned policy ConfigMaps | P0 | Live | ST-CHART-POLICY-OWNERSHIP-001 | Partial local Kind pass; rollout assertion pending |
| BR-PLATFORM-005 FR-9 | Safe distinct-name migration preserves external ConfigMaps and healthy consumers | P0 | Live | ST-CHART-POLICY-OWNERSHIP-002 | Partial local Kind pass; rollout assertion pending |

---

## 8. Test scenarios

### Structural: helm-unittest

| ID | Business outcome | Phase |
|---|---|---|
| `IT-HELM-2460-001` | AIAnalysis cannot reinterpret `aianalysis-policies` as an external ConfigMap | Pass |
| `IT-HELM-2460-002` | SignalProcessing cannot reinterpret `signalprocessing-policy` as an external ConfigMap | Pass |
| `IT-HELM-2460-003` | AIAnalysis accepts and mounts a distinct external ConfigMap name | Pass |
| `IT-HELM-2460-004` | SignalProcessing accepts and mounts a distinct external ConfigMap name | Pass |
| `IT-HELM-2460-005` | SignalProcessing generated config records the distinct external policy identity | Pass |
| `IT-HELM-2460-006` | AIAnalysis omits the chart-managed ConfigMap for distinct externalization | Pass |
| `IT-HELM-2460-007` | SignalProcessing omits the chart-managed ConfigMap for distinct externalization | Pass |

### Live: Kind smoke flow

| ID | Business outcome | Phase |
|---|---|---|
| `ST-CHART-POLICY-OWNERSHIP-001` | A same-name upgrade fails validation and leaves both chart-managed ConfigMaps and controller health intact | Partial local Kind pass; rollout assertion pending |
| `ST-CHART-POLICY-OWNERSHIP-002` | A distinct-name migration preserves external ConfigMaps, prunes only obsolete managed names, updates both mounts, and completes both rollouts | Partial local Kind pass; rollout assertion pending |

### Tier skip rationale

- **Go unit/integration**: no Go logic or new production component is introduced.
- **Live tier locally**: a targeted real-Helm Kind run proved schema rejection,
  ConfigMap preservation/pruning, Deployment mounts, and generated config. The full
  smoke flow's `--wait` rollout assertions remain pending CI/Kind image setup and
  must pass before this plan can be marked Complete.

---

## 9. Test case details

### IT-HELM-2460-001/002

**Given**: all unrelated required chart values are valid.
**When**: render only `templates/NOTES.txt` with the corresponding
`existingConfigMap` set to its reserved chart-owned name.
**Then**: Helm fails schema validation before template-specific logic runs.

### IT-HELM-2460-003/004

**Given**: a distinct external name such as `aianalysis-policies-external` or
`signalprocessing-policy-external`.
**When**: render the relevant service template with `content` omitted.
**Then**: the chart does not render the chart-managed policy ConfigMap and the
controller Deployment volume references the distinct external name.

### IT-HELM-2460-005

**Given**: SignalProcessing uses `signalprocessing-policy-external`.
**When**: render `signalprocessing-config`.
**Then**: `classifier.regoConfigMapName` records the distinct external identity,
while the policy key remains `policy.rego`.

### IT-HELM-2460-006/007

**Given**: AIAnalysis or SignalProcessing uses a distinct external ConfigMap name.
**When**: render the corresponding service template.
**Then**: the reserved chart-managed policy ConfigMap is absent from the desired
manifest.

### ST-CHART-POLICY-OWNERSHIP-001

1. Start from the production Flow A release with both chart-managed policy
   ConfigMaps present and both controller Deployments healthy.
2. Attempt one upgrade setting both `existingConfigMap` values to their reserved
   names.
3. Expect Helm schema validation failure.
4. Assert both reserved ConfigMaps still contain their required non-empty keys.
5. Assert both controller rollouts remain healthy.

### ST-CHART-POLICY-OWNERSHIP-002

1. Create `aianalysis-policies-external` with `approval.rego` and
   `signalprocessing-policy-external` with `policy.rego` in the release namespace.
2. Upgrade with both policy `content` values empty and both distinct
   `existingConfigMap` names set, using `--wait`.
3. Assert both external ConfigMaps still exist and contain their required keys.
4. Assert the old chart-managed identities are absent after Helm's upgrade diff.
5. Assert the two Deployment volume references use the external names.
6. Assert both controller rollouts complete successfully.

---

## 10. Environment and commands

### Structural

```bash
helm unittest -f 'tests/policies_schema_validation_test.yaml' charts/kubernaut
helm lint --strict charts/kubernaut \
  --set-file aianalysis.policies.content=/path/to/approval.rego \
  --set-file signalprocessing.policies.content=/path/to/policy.rego
```

### Generated documentation

```bash
make generate-helm-config-docs
git diff --exit-code -- docs/generated/helm-values-reference.md
```

### Live

```bash
./scripts/helm-smoke-test.sh --platform kind --image-tag <tag> \
  --chart-path charts/kubernaut/
```

The live command requires a real Kubernetes cluster and is not replaced by a static
render.

---

## 11. Deliverables and completion record

| Deliverable | Location | Status |
|---|---|---|
| Design decision | `docs/architecture/decisions/DD-PLATFORM-011-helm-policy-configmap-ownership-boundary.md` | Approved |
| Test plan | `docs/testing/2460/TEST_PLAN.md` | Approved |
| Schema guard | `charts/kubernaut/values.schema.json` | Implemented; structural tests pass |
| Structural tests | `charts/kubernaut/tests/policies_schema_validation_test.yaml` | 13 focused / 717 full Helm tests pass |
| Live smoke coverage | `scripts/helm-smoke-test.sh` | Implemented; targeted Kind lifecycle pass; full rollout assertion pending CI |
| Upgrade/recovery documentation | `charts/kubernaut/values.yaml`, `charts/kubernaut/README.md` | Implemented |

### Completion confidence

**Current confidence: 94%.** The ownership failure mode, chart resource identities,
schema capability, structural test path, and Helm manifest pruning path are confirmed.
Remaining risk is application rollout health in the full smoke flow, which requires
the CI image setup and is explicitly tracked by R5.
