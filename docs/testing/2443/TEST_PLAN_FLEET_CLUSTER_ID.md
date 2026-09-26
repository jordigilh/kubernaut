# Test Plan: Fleet E2E Cluster-ID Mode Separation

> Hybrid IEEE 829-2008 + Kubernaut test plan.

| Field | Value |
|-------|-------|
| Test plan identifier | TP-2443-FLEET-CLUSTER-ID-v1 |
| Feature | Enforce explicit cluster identity in Fleet E2E alerts while retaining local-mode behavior |
| Version | 1.0 |
| Created | 2026-09-23 |
| Status | Active |
| Branch | `fix/2442-workflow-discovery-membership` |

---

## 1. Purpose and Objectives

Fleet mode and local mode are mutually exclusive remediation modes. Fleet-mode alerts must carry a registered `cluster_id`; an absent ID must not fall back to local resolution. The fleet E2E fixtures that previously relied on that fallback must use explicit registered IDs. Local no-ID coverage remains in the non-fleet FullPipeline lane.

Objectives:

1. The real KA Fleet journey routes registered hub and spoke targets through the MCP Gateway using `cluster_id=hub` and `cluster_id=remote-cluster`.
2. The fleet reconstruction journey preserves `cluster_id=hub` in both the live RR and DataStorage reconstruction.
3. The organic Prometheus → Alertmanager → Gateway journey creates a hub-scoped RR from an alert carrying the per-alert `cluster=hub` label, without a test-initiated Gateway POST.
4. Existing local-mode FullPipeline coverage proves an un-attributed Alertmanager signal remains local and reconstruction omits the optional cluster ID.
5. Local KA tool-selection coverage remains in a non-fleet mode and proves `kubectl_get_by_name` from live evidence.
6. No production code or shared Prometheus rule-builder behavior changes.

## 2. Authority and References

- BR-INTEGRATION-054: authenticated MCP Gateway access and cluster-scoped reads.
- BR-FLEET-054: fleet cluster-scoped access and correlation.
- BR-AUDIT-005 v2.0 / DD-AUDIT-003: audit reconstruction provenance.
- BR-KA-212: K8s-verified remediation target.
- DD-TEST-014: Fleet E2E hub-and-spoke topology; a fleet hub target uses the registered `hub` Gateway backend.
- DD-FLEET-005 / ADR-068: cluster-transparent tool exposure and Gateway-routed fleet access.
- Issue #1729: KA hub/spoke tool-routing scenario.
- Kubernaut #2309: organic Alertmanager-to-Gateway E2E scenario.

## 3. Risks and Mitigations

| ID | Risk | Impact | Mitigation |
|----|------|--------|------------|
| R1 | Alertmanager omits `commonLabels.cluster` when grouping alerts from multiple clusters. | Gateway cannot resolve the organic hub alert in fleet mode. | Put `cluster=hub` on the Prometheus test metric so it survives as a per-alert label; assert the resulting RR has `Spec.ClusterID == "hub"`. |
| R2 | Hub registration is not ready when the fleet test posts the signal. | False setup failure or local-looking result. | Reuse the existing registered-hub MCP client/readiness and `fmcSyncTimeout` polling before posting. |
| R3 | Local-mode behavior is removed while stale no-ID fleet fixtures are corrected. | Regression in single-cluster mode. | Extend the existing E2E-FP-118-002 local Alertmanager flow to assert empty RR cluster ID and omitted reconstruction field; add one focused local KA tool proof using the registered Mock LLM scenario. |
| R4 | Changes overlap with ongoing work in the shared checkout. | Unrelated edits may be overwritten or tests may race. | Preserve all existing dirty files; only edit files with confirmed ownership. Do not run cluster-mutating E2E checks concurrently. |
| R5 | The organic rule label change leaks into non-fleet Prometheus fixtures. | A non-fleet Gateway could reject a fleet-attributed alert if that metric is later reused. | Do not change the shared Prometheus rule builder. Add the cluster label only to the Fleet E2E's injected metric. |
| R6 | Fleet setup reapplies the local FullPipeline `generic-restart` fixture into the same namespace, where an older immutable workflow version may already exist. | Fleet setup can stop before any Fleet specs run, and changing the shared fixture can affect FullPipeline suites. | Keep the shared generic fixture in local FullPipeline seeds; Fleet seeds only its directly consumed crashloop/OOM fixtures and uses its separately seeded Fleet execution fixture. |

## 4. Scope and Design

### In scope

- Keep E2E-FLEET-017 as registered hub/spoke coverage; assert `resources_get` and target-specific live evidence for both IDs.
- Keep E2E-FLEET-CC81-001 as fleet provenance coverage; the hub case sends `cluster_id=hub` and expects that value in the reconstructed response.
- Keep E2E-FLEET-021 in Fleet mode; add `cluster=hub` to the synthetic metric labels and assert the organically created RR is hub-scoped.
- Preserve no-cluster behavior in local mode using existing FullPipeline coverage and one focused local KA tool-selection proof.
- Keep workflow seed ownership lane-specific: FullPipeline retains its generic/local fixtures, while Fleet seeds only the crashloop and OOM fixtures consumed by Fleet journeys; the Fleet execution-override fixture remains Fleet-owned.
- Add/extend IEEE 829 test-plan traceability without modifying the unrelated `docs/testing/2443/TEST_PLAN.md` (AIAnalysis histogram plan).

### Out of scope

- Changing Gateway's fail-closed fleet resolver or restoring implicit local fallback in fleet mode.
- Changing the shared `fleetInteractiveBridgeGroundingRule()` / `DeployPrometheus()` configuration; CocoIndex call graph shows this is rendered by several E2E suites.
- Changing other lanes' existing work or cleaning up their in-progress diffs.

## 5. Test Inventory and Acceptance

| Test ID | Lane | Expected behavior |
|---------|------|-------------------|
| E2E-FLEET-017 | Fleet | Registered hub and spoke alerts each carry their explicit cluster ID; KA uses `resources_get` and returns evidence from the selected cluster. |
| E2E-FLEET-CC81-001 | Fleet | Hub RR and reconstructed response both preserve `cluster_id=hub`. |
| E2E-FLEET-021 | Fleet | A Prometheus rule fires from the injected metric, Alertmanager forwards it without a direct Gateway POST, and the created RR has `cluster_id=hub`. |
| E2E-FP-118-002 | FullPipeline/local | The un-attributed Alertmanager signal produces a local RR with empty `Spec.ClusterID`; reconstructing that RR leaves `ClusterID.Set == false`. |
| E2E-FP-1729-001 | FullPipeline/local | The default Mock LLM local path uses `kubectl_get_by_name`; the RCA contains the live marker evidence and the local RR has no cluster ID. |
| UT-WORKFLOW-004-004 | Infrastructure | Fleet and local FullPipeline use distinct seed lists; Fleet excludes shared generic/local fixtures while FullPipeline retains its existing inventory. |

The Fleet E2E-FLEET-021 rule itself remains shared, but the unique synthetic metric is only injected by that fleet test. The E2E must verify RR attribution so Prometheus-to-Alertmanager label propagation is checked end to end.

## 6. TDD Plan

| Phase | Work | Estimate | Exit criteria |
|-------|------|----------|---------------|
| RED | Add the E2E-FLEET-021 RR cluster assertion and metric label; extend the local FullPipeline assertions and focused local KA tool case; add a BDD assertion for lane-owned workflow seed inventories. | 1–2 hours | The tests express hub attribution, local no-ID preservation, the local tool-selection contract, and fixture ownership. |
| GREEN | Reuse the existing Fleet hub registration and default Mock LLM scenario; select only the workflow fixtures consumed by each lane. No production behavior changes. | 1–2 hours | All focused scenarios pass in their correct mode and a pre-existing local fixture is not reapplied by Fleet setup. |
| REFACTOR | Remove stale Fleet comments/empty-ID branch from the Fleet-only test helper, retain local Mock LLM behavior for local mode, update test-plan traceability. | 30–45 minutes | No Fleet spec can accidentally exercise an implicit local fallback; local coverage remains explicit. |
| Validation | Compile, run the focused infrastructure test, then run Fleet and FullPipeline focused E2Es sequentially on an isolated or coordinated runner. | 2–4 hours | All focused tests pass, with no other-lane regressions. |

No new production component is introduced, so no production wiring-manifest row is required. The E2Es traverse the existing Gateway, controllers, KA, and DataStorage paths.

## 7. Verification and Environment

1. Run `UT-WORKFLOW-004-004` and compile changed test packages with `go test -run '^$'`.
2. Run E2E-FLEET-017, E2E-FLEET-CC81-001, and E2E-FLEET-021 sequentially using the existing Fleet topology.
3. Run E2E-FP-118-002 and E2E-FP-1729-001 sequentially using the existing local FullPipeline topology.
4. Run `go build ./...`, `golangci-lint run --timeout=5m`, and affected tests.

Because this checkout is shared, do not run full E2E lanes, delete Kind clusters, prune images, or overwrite existing dirty files until a dedicated runner or coordinated test window is available.

### 7.1 Shared-Worktree Handoff

At preflight, `test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go` and
`test/e2e/fullpipeline/03_ka_target_resource_test.go` already contained active
staging-environment edits from the FullPipeline lane. The FullPipeline team
approved the narrow local assertions in `01_full_remediation_lifecycle_test.go`;
those edits preserve the existing staging changes. The local tool proof is in a
new focused file (`21_ka_local_tool_routing_test.go`) rather than the active
`03_ka_target_resource_test.go`. The shared
`test/infrastructure/prometheus_alertmanager_e2e.go` builder remains
unchanged; E2E-FLEET-021 supplies the hub identity on its test-only metric
input. Full E2E execution remains coordinated with the teams using those lanes.

## 8. Spike Decision and Confidence

**No separate architecture spike is needed.** DD-TEST-014 already registers the hub backend; the default Mock LLM registry already registers the dynamic KA tool-call scenario; FullPipeline runs with fleet disabled. The focused RED tests will validate those existing routes. If the local tool scenario is not selected in FullPipeline, stop and time-box a test-fixture spike before adding workarounds.

**Preflight confidence: 95%.** The fleet-mode contract, current test edits, Mock LLM registration, and call-graph blast radius are verified. Remaining risk is limited to E2E fixture timing and shared-runner coordination.
