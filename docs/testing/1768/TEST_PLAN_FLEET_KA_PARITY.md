# Test Plan: KubernautAgent Fleet E2E Parity Gaps

**Test Plan Identifier**: TP-1768-KA-FLEET-PARITY-v1.0
**Feature**: Keep KubernautAgent E2E scenarios in local mode while proving the fleet-relevant subset in the Fleet E2E lane
**Version**: 1.0
**Created**: 2026-09-22
**Author**: Kubernaut development team
**Status**: Draft
**Branch**: `fix/2442-workflow-discovery-membership`

---

## 1. Introduction

### 1.1 Purpose

`test/e2e/kubernautagent` remains the local-mode baseline for KubernautAgent. The Fleet E2E
lane should prove the subset of those journeys whose behavior depends on a registered cluster
identity, MCP Gateway routing, or real remote-cluster data. This plan records which outcomes
already have Fleet proofs and defines focused additions for the remaining fleet-specific gaps;
it does not propose moving or duplicating the full local KA suite.

### 1.2 Objectives

1. Preserve all local-mode KA E2E scenarios and their current execution lane.
2. Prove real KA workflow discovery selects workflows compatible with the target fleet cluster,
   not merely that cluster classification reaches AIAnalysis.
3. Prove interactive KA turns for a hub target use the registered `hub` Gateway backend, as the
   existing remote-spoke interactive journey proves for `remote-cluster`.
4. Cover hub-backed HPA/PDB detection and multiple fleet tool calls where those scenarios are
   relevant to cluster-scoped routing.
5. Require each Fleet E2E to distinguish the intended cluster with live evidence and fail if a
   wrong-cluster or local-shortcut result could pass.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Local KA E2E regressions | 0 | Existing `test/e2e/kubernautagent` lane remains local and passes |
| Fleet-specific KA E2E gaps in scope | 0 | All P0 scenarios below pass in `test/e2e/fleet` |
| Wrong-cluster false positives | 0 | Tests assert cluster-specific live evidence or selected workflow identity |
| Fleet E2E buildability | Pass | `go test -run '^$' ./test/e2e/fleet` |

---

## 2. References

### 2.1 Authority

- BR-FLEET-054: fleet cluster-scoped access and correlation
- BR-INTEGRATION-054: remote access through authenticated MCP Gateway
- BR-FLEET-003: cluster-scoped workflow targeting
- [ADR-068: Fleet Federation Architecture](../../architecture/decisions/ADR-068-fleet-federation-architecture.md)
- [DD-FLEET-005: Cluster-Transparent Tool Exposure](../../architecture/decisions/DD-FLEET-005-cluster-transparent-tool-exposure.md)
- [DD-TEST-014: Fleet E2E Hub-and-Spoke Topology](../../architecture/decisions/DD-TEST-014-fleet-e2e-remote-cluster-only-topology.md)
- [Issue #1511 test plan](../../tests/1511/TEST_PLAN.md)
- [Issue #1768 Track 1 test plan](TEST_PLAN.md)
- [Issue #1768 Track 2 test plan](TEST_PLAN_TRACK2.md)
- [DD-KA-017 three-step discovery test plan](../DD-KA-017/TEST_PLAN.md)

### 2.2 Related E2E Scenarios

- Local KA: `test/e2e/kubernautagent/three_step_discovery_test.go`,
  `detected_labels_e2e_test.go`, `interactive_discovery_e2e_test.go`, and
  `parallel_tools_e2e_test.go`.
- Fleet: `test/e2e/fleet/13_cluster_scoped_workflow_targeting_test.go`,
  `17_ka_real_fleet_investigation_test.go`,
  `18_af_ka_interactive_fleet_bridge_test.go`, and
  `23_af_consent_parity_test.go`.

---

## 3. Current Coverage Audit

| Local KA scenario | Existing Fleet proof | Assessment |
|---|---|---|
| `E2E-KA-017-001-001` / `001b`: OOM and CrashLoop three-step workflow discovery | `E2E-FLEET-2365-001..003` exercises remote discovery/selection/execution through AF; `E2E-FLEET-1511-001` proves cluster classification reaches AIAnalysis | **Partial.** The consent journeys exercise remote workflow discovery, but `1511-001` explicitly stops before proving KA's catalog selects the cluster-compatible workflow. Add the selection proof below. |
| `E2E-KA-056-003`: PDB/HPA detection | `E2E-FLEET-017` checks remote HPA/PDB labels through `resources_get` | **Covered for a spoke; hub parity missing.** The Fleet lane's new hub case currently proves an autonomous hub read, not hub HPA/PDB detection. |
| `E2E-KA-INT-007` and interactive discovery journeys | `E2E-FLEET-018` proves an AF `kubernaut_message` turn reaches a live remote target; `E2E-FLEET-2365-001..003` proves remote workflow consent and completion | **Covered for a spoke; hub interactive routing missing.** The interactive bridge is a distinct `RunInteractiveTurn` path and needs an explicit hub-registration case. |
| `E2E-KA-970-001`: parallel tool calls | No Fleet E2E asserts multiple one-turn tool results through a cluster overlay | **Gap.** Add a result-completeness case proving all calls retain the same explicit cluster identity; do not assert a latency threshold. |
| `E2E-KA-1507-001`: Alertmanager and node-proxy tools | No direct Fleet counterpart | **Not currently a fleet parity target.** The local test uses the suite's Alertmanager and Kubelet `nodes/proxy` path, not a cluster-attributed MCP Gateway read. Keep it local unless a fleet contract for these tools is defined. |
| KA auth, session lifecycle, snapshots, audit, observability, and local input-validation scenarios | No one-for-one Fleet copy | **Intentionally local.** These tests do not establish a fleet cluster-routing behavior and should remain in the local KA lane. |

### Existing Fleet-Specific Coverage

- `E2E-FLEET-017`: real Helm-deployed KA autonomous investigation via Gateway. The current
  worktree updates this journey to use `cluster_id=hub` for the hub case and asserts
  `resources_get`; the spoke case remains. Live Fleet-lane execution is still required.
- `E2E-FLEET-018`: AF A2A interactive bridge to a genuine remote-spoke target.
- `E2E-FLEET-2365-001..003`: phase-transition consent and autonomous discover-select-watch
  journeys against a remote target.
- `E2E-FLEET-1511-001`: cluster classification propagation through SP/RO into AIAnalysis;
  its comments explicitly defer actual KA catalog filtering to lower-tier coverage.
- `E2E-FLEET-2394-001/002`: hub severity attribution and fail-closed Gateway handling are
  tracked separately from KA interactive and catalog-selection coverage.

---

## 4. Risks and Mitigations

| ID | Risk | Impact | Probability | Affected tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Cluster classification reaches AIAnalysis but KA still selects a workflow for another cluster | A fleet target can receive an incompatible remediation workflow | Medium | E2E-FLEET-1511-002/003 | Include matching and mismatching workflow classifications and assert the actual selected workflow name. |
| R2 | Hub interactive turns use a local or spoke tool path | Hub reads may bypass the Gateway or return plausible data from the wrong backend | Medium | E2E-FLEET-018-HUB-001 | Give hub and spoke same-named targets distinct live evidence; assert Gateway tool identity and hub evidence. |
| R3 | HPA/PDB detection silently reads hub-local data for a spoke or skips the hub Gateway path | Detected labels can be incomplete or attributed to the wrong cluster | Low | E2E-FLEET-056-HUB-001 | Create the labels on the hub target, verify hub Gateway visibility, and assert persisted HPA/PDB flags. |
| R4 | Parallel Gateway calls mix results or lose cluster context | RCA may combine evidence across clusters | Low | E2E-FLEET-970-001 | Use distinct target evidence and assert every requested result; do not gate on wall-clock latency. |
| R5 | Fleet fixture readiness/cache convergence makes tests flaky | Intermittent false failures during setup | Medium | All Fleet E2E | Use unique fixture names, FMC-aware eventual polling, and the existing authenticated Gateway helper. |

### 4.1 Risk-to-Test Traceability

R1 and R2 are P0 because they protect workflow authorization and cluster isolation. R3 and R4
are P1 parity tests for supported KA capabilities. The Fleet suite's existing full-pipeline
bootstrap, Keycloak authentication, separate remote Kind cluster, and Gateway topology are
reused; no additional infrastructure pattern is proposed.

---

## 5. Scope

### 5.1 Features to be Tested

- KA catalog-based workflow selection when the RemediationRequest has a non-hub `ClusterID`.
- `RunInteractiveTurn` for a hub target attributed as `cluster_id=hub` and routed through the
  registered hub MCP Gateway backend.
- Hub HPA/PDB label detection through the same Gateway registration.
- Multiple fleet overlay tool results in one investigation turn, with each result attributed to
  the same explicit cluster identity.

### 5.2 Features Not to be Tested

- Porting all `test/e2e/kubernautagent` test cases to the Fleet lane.
- Local auth, session ownership, audit snapshot, observability, and generic interactive lifecycle
  tests that do not vary by cluster identity.
- Alertmanager and Kubelet `nodes/proxy` coverage from `E2E-KA-1507-001`; these currently have
  no cluster-attributed Gateway contract in the source scenario.
- Adding direct hub-to-spoke Kubernetes API or Event access.

### 5.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Keep `test/e2e/kubernautagent` in local mode | It remains the fast, isolated service behavior baseline. |
| Add only fleet-relevant counterparts to `test/e2e/fleet` | The Fleet lane owns Gateway, Keycloak, hub registration, and real spoke isolation; duplicating all local cases would add cost without validating cluster routing. |
| Use live target evidence and selected workflow identity | Tool invocation alone does not prove the call reached the right cluster or that the right workflow was exposed. |
| Avoid hard latency assertions for parallel calls | Shared CI hardware is noisy; the contract is complete, correctly attributed results, not a wall-clock threshold. |

---

## 6. Approach

### 6.1 Test Strategy

This is test-only parity work against already-wired Fleet behavior. Reuse existing unit and
integration tests for pure catalog filtering, tool routing, and interactive overlay wiring. Add
Fleet E2E journeys only where the production deployment topology or real cluster identity is
the unique proving value. All Fleet E2E cases use the real deployed services, Keycloak, MCP
Gateway, and separate Kind clusters; do not mock internal business logic.

### 6.2 Pass/Fail Criteria

**PASS**:

1. `test/e2e/kubernautagent` remains local-mode and its existing suite is unchanged by this plan.
2. P0 Fleet scenarios 1511-002/003 and 018-HUB-001 pass in the Fleet E2E lane.
3. Hub HPA/PDB and parallel-call parity tests pass.
4. Every positive Fleet test proves live evidence or selected-workflow identity from the intended
   registered cluster; a local fallback or wrong-cluster result fails the assertion.
5. Existing unit/integration coverage, `go build ./...`, and changed-code lint remain green.

**FAIL**: a P0 journey selects a mismatched-cluster workflow, omits live hub/spoke evidence, or
completes using a local-only shortcut where a registered Gateway route is required.

---

## 7. BR Coverage Matrix and Scenario Inventory

| ID | Priority | Tier | Business outcome | Controls | BR | Target file |
|----|----------|------|------------------|----------|----|-------------|
| E2E-FLEET-1511-002 | P0 | E2E | A remote OOM investigation selects the production-classified workflow and not a same-action-type workflow classified for another cluster. | AC-4, AC-6, SC-7 | BR-FLEET-003, BR-FLEET-054 | `test/e2e/fleet/13_cluster_scoped_workflow_targeting_test.go` |
| E2E-FLEET-1511-003 | P0 | E2E | A remote CrashLoop investigation performs the same cluster-compatible workflow selection for a second signal/workflow family. | AC-4, AC-6, SC-7 | BR-FLEET-003, BR-FLEET-054 | `test/e2e/fleet/13_cluster_scoped_workflow_targeting_test.go` |
| E2E-FLEET-018-HUB-001 | P0 | E2E | A real AF `kubernaut_message` turn continues a hub-scoped KA session through `hub__resources_get` and returns hub-only evidence despite a same-named spoke decoy. | AC-4, AC-6, AU-3, SC-7 | BR-FLEET-054, BR-INTEGRATION-054 | `test/e2e/fleet/18_af_ka_interactive_fleet_bridge_test.go` |
| E2E-FLEET-056-HUB-001 | P1 | E2E | A hub-targeted KA investigation detects HPA/PDB labels through the registered hub Gateway backend and persists those labels in AIAnalysis. | AC-4, SI-4, SC-7 | BR-INTEGRATION-1489, BR-FLEET-054 | `test/e2e/fleet/17_ka_real_fleet_investigation_test.go` |
| E2E-FLEET-970-001 | P1 | E2E | One KA investigation returns evidence from multiple concurrent tool calls routed through the same explicitly attributed spoke cluster. | AC-4, SC-7, SI-4 | BR-PERFORMANCE-970, BR-FLEET-054 | `test/e2e/fleet/17_ka_real_fleet_investigation_test.go` or a focused companion file |

### 7.1 Detailed P0 Test Cases

#### E2E-FLEET-1511-002/003: Cluster-Compatible Workflow Selection

**Preconditions**:

- Fleet E2E has a real hub registration and a genuinely separate `remote-cluster` spoke.
- AuthWebhook/DataStorage contain two otherwise comparable workflows for the target action type:
  one classified for the spoke's `production` classification and one for a different
  classification.
- Both workflows are created before the signal; use unique names and IDs per run.

**Steps**:

1. Create a managed target on the remote cluster and post a signal with
   `cluster_id=remote-cluster`.
2. Let SP classify the cluster and let the real KA binary run the catalog-backed three-step
   discovery flow.
3. Wait for AIAnalysis completion and inspect the selected workflow.
4. Repeat with the second source scenario (OOMKilled and CrashLoopBackOff).

**Expected results**:

- AIAnalysis carries the expected `production` cluster classification.
- The selected workflow is the matching-classification workflow for the signal.
- The mismatched workflow is not selected. The mock scenario must make the mismatch observable
  if cluster filtering is absent; do not rely only on `1511-001`'s classification propagation
  assertion.
- The result is based on the real Fleet-deployed KA and its real catalog cache, not a direct
  catalog-unit call.

#### E2E-FLEET-018-HUB-001: Interactive Hub Gateway Routing

**Preconditions**:

- Fleet E2E AF/KA binaries, Keycloak A2A auth, hub Gateway registration, and spoke Gateway
  registration are ready.
- A same-named marker Deployment exists on hub and spoke with distinct memory-limit evidence.

**Steps**:

1. Use the real AF A2A endpoint to create/investigate an RR with `cluster_id=hub`.
2. Continue the session through `kubernaut_message` and make the real KA interactive turn read
   the marker.
3. Inspect the A2A artifact and relevant AIAnalysis/session output.

**Expected results**:

- The interactive turn uses the Gateway-provided `hub__resources_get` tool.
- Hub evidence appears; spoke-decoy evidence does not.
- No empty-ID local shortcut is used for the Fleet-mode hub target.

### 7.2 Wiring Manifest

| Component | Production entry point | Wiring location | E2E proof |
|-----------|------------------------|-----------------|-----------|
| KA fleet catalog discovery | Gateway signal -> RO -> AIAnalysis -> KA investigator/catalog | `test/e2e/fleet/13_cluster_scoped_workflow_targeting_test.go`, KA catalog wiring in `cmd/kubernautagent` | E2E-FLEET-1511-002/003 |
| Fleet overlay for interactive turns | AF A2A `kubernaut_message` -> KA `InvestigateTool.handleMessage` -> `RunInteractiveTurn` | `test/e2e/fleet/18_af_ka_interactive_fleet_bridge_test.go` | E2E-FLEET-018-HUB-001 |
| Hub detected-label reads | KA autonomous investigation -> registered hub MCP Gateway backend | `test/e2e/fleet/17_ka_real_fleet_investigation_test.go` | E2E-FLEET-056-HUB-001 |
| Parallel fleet tool calls | KA investigator multi-tool dispatch -> remote Gateway registration | Fleet test suite and mock-LLM scenario fixture | E2E-FLEET-970-001 |

---

## 8. TDD Plan and Execution Order

| Phase | Work | Estimate | Exit criteria |
|-------|------|----------|---------------|
| Discovery | Confirm current AIAnalysis status exposes selected workflow identity and determine a deterministic mismatch-sensitive mock scenario. | 1–2 hours | Test oracle distinguishes filtered from unfiltered catalog results. |
| RED | Add P0 fleet E2E specs and fixtures first; verify they fail when workflow classification or hub tool routing is intentionally misconfigured in a test fixture. | 2–4 hours | Failures identify wrong workflow or wrong cluster evidence, not setup flakiness. |
| GREEN | Reuse existing Fleet topology, authenticated clients, MCP helper, namespaces, and mock-LLM conventions. Only add production changes if a real defect is demonstrated. | 2–4 hours | Focused P0 Fleet E2E cases pass. |
| REFACTOR | Remove duplicated fixture setup, keep unique resources/IDs, improve failure messages, and document exact BR/test traceability. | 1 hour | No new types/components solely for test plumbing; changed-code lint clean. |
| Wiring verification | Verify tests traverse real Gateway webhook, deployed KA, and real target cluster; confirm no in-process test-only caller is substituting for production dispatch. | 1 hour | Wiring Manifest rows have passing E2E evidence. |
| Validation | Build, focused unit/integration packages, then focused Fleet E2E specs and regression suite. | 1–3 hours | All required checks pass; full Fleet lane may be run separately due resource cost. |

No production component or architecture change is planned. If the new end-to-end journey
reveals a production routing/design defect requiring a new pattern, stop and raise a separate
design decision before changing production behavior.

---

## 9. Environment and Execution

- **Local KA lane**: remains the existing local-mode `test/e2e/kubernautagent` suite.
- **Fleet lane**: `test/e2e/fleet`, with real Keycloak, Envoy AI Gateway, hub registration,
  separate remote Kind cluster, and deployed Kubernaut services.
- **Resource profile**: reuse the Fleet suite's documented primary and remote cluster footprint;
  do not create an additional cluster topology.
- **Focused Fleet command**:

  ```bash
  FLEET_E2E=true ginkgo -v ./test/e2e/fleet/... \
    --focus='E2E-FLEET-1511-00[23]|E2E-FLEET-018-HUB-001|E2E-FLEET-056-HUB-001|E2E-FLEET-970-001'
  ```

- Run `go test -run '^$' ./test/e2e/fleet` before provisioning the Fleet topology to catch
  compile errors cheaply.
- Use unique target names, alert fingerprints, namespaces, and workflow names to remain safe
  under parallel Ginkgo execution.
- Do not add hard wall-clock thresholds to E2E assertions.

---

## 10. Deliverables and Approval Gate

| Deliverable | Location | State |
|-------------|----------|-------|
| This test plan and coverage matrix | `docs/testing/1768/TEST_PLAN_FLEET_KA_PARITY.md` | Draft |
| Local-mode KA baseline | `test/e2e/kubernautagent/` | Existing; retain unchanged |
| Fleet parity E2E scenarios | `test/e2e/fleet/` | P0/P1 additions pending plan approval |
| Fleet mock-LLM selectors/fixtures | `test/infrastructure/shared_e2e.go`, `test/services/mock-llm/scenarios/` | Add only when required by approved scenarios |

Implementation of the P0/P1 planned scenarios should begin after review/approval of this plan.

---

## 11. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-09-22 | Initial Fleet-vs-local KA E2E coverage audit and parity plan. |
