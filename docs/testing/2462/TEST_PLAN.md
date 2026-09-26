# Test Plan: Isolated local and Fleet-mode AF E2E coverage

**Test Plan Identifier**: TP-2462-v1.0
**Feature**: Exercise local and Fleet-mode APIFrontend contracts in isolated AF E2E CI lanes
**Version**: 1.4
**Created**: 2026-09-23
**Author**: Kubernaut development team
**Status**: Approved
**Branch**: `fix/2442-workflow-discovery-membership`

---

## 1. Introduction

### 1.1 Purpose

Keep the standalone APIFrontend (AF) E2E baseline in local mode, while proving cluster-attributed AF triage and A2A/SSE contracts against a separate Fleet-enabled AF deployment. A Fleet request with `cluster_id=hub` must resolve through the MCP Gateway's registered `hub` backend, even when the Gateway backend and Fleet AF run in the same physical Kind cluster. The local and Fleet AF Kind clusters remain isolated in separate CI jobs; the suite's default developer mode continues to support running both clusters together.

### 1.2 Objectives and success metrics

1. `E2E (apifrontend)` creates only the standalone local AF Kind cluster and excludes `fleet-mode-af` specs. A separate `E2E (apifrontend-fleet)` job runs only `fleet-mode-af` specs against one hub-only Fleet AF cluster. No third or remote cluster is created.
2. The Fleet cluster exposes exactly one `hub` MCP Gateway registration for hub-only mode; no remote/loopback alias collides with `hub` or silently substitutes for it.
3. Cluster-attributed AF tests prove the resulting RR preserves `cluster_id=hub`, AF uses the registered Gateway path, and an unregistered identity fails closed despite a same-named target.
4. The AF severity, structured-decision, progressive RCA, and concurrent-session fallback contracts listed in Section 7 have passing Fleet-mode E2E coverage.
5. The local AF suite continues to use empty cluster attribution and has no regressions.
6. On failure, must-gather runs against each surviving Kind cluster; the Fleet bundle also collects logs from the Envoy Gateway and Envoy AI Gateway controller namespaces.

### 1.3 Authority and references

- BR-FLEET-054: fleet cluster-scoped access and correlation
- BR-INTEGRATION-054 / BR-INTEGRATION-065: authenticated remote access and multi-cluster signal routing
- BR-AI-056: grounded AF investigation signal identity
- ADR-068: Fleet Federation Architecture
- DD-TEST-014: Fleet E2E Hub-and-Spoke Topology
- DD-TEST-019: Minimal Fleet-Enabled APIFrontend E2E Topology
- Issues #2394, #2461, #2462

## 2. Preflight and spike

### 2.1 Confirmed findings

- `SetupAPIFrontendE2EInfrastructure` already creates the local AF Kind cluster using `CreateKindClusterWithConfig`.
- `SetupFullPipelineInfrastructure` accepts a `FleetProvisioner`, but starts unrelated pipeline services. DD-TEST-019 selects the lean topology: reuse the standalone AF setup/Kind helpers for both clusters and the FMC E2E lane's Fleet-core provisioning sequence for the Fleet cluster.
- The standalone AF/DataStorage helper deploys Redis for its DLQ, not Valkey. The Fleet AF cluster therefore retains the FMC lane's dedicated Valkey, matching FMC's production dependency and avoiding shared cache state.
- `DeployFleetGatewayInfra` and `KubeMCPServerAuthConfig.HubClusterID` support a logical `hub` Gateway registration backed by the same physical cluster's kube-mcp-server.
- The existing full Fleet setup is not hub-only: `provisionFleetCoreInfra` always calls `SetupRemoteClusterForFMC` and enables `AllRegistrationsRemote`.
- Setting `AllRegistrationsRemote=false` with `HubClusterID="hub"` without changing registration generation is unsafe: both `fleetClusterRegistrationIdentity` and `fleetHubRegistrationIdentity` can produce the `hub` name and `hub__` prefix.
- The local AF and full-pipeline Kind configs share host mappings (including 8088, 9190, and 9193). The combined developer path retains its host-port offset. Separate CI jobs can use each Kind config's established host ports without cross-cluster collisions.

### 2.2 Spike decision

**Question**: Can these AF hub-attributed scenarios run against a Fleet Gateway registration whose backend is in the same physical cluster, while keeping the standalone local AF cluster separate?
**Decision**: **Yes.** The Fleet AF cluster can be a single-cluster hub topology for these hub-target journeys. Existing Fleet E2E retains genuine spoke coverage.
**Implementation caveat**: Use the FMC lane's Fleet-core components with an explicit hub-only registration. Do not create a duplicate `hub`/loopback registration or a remote/spoke Kind cluster.

## 3. Risks and mitigations

| ID | Risk | Impact | Affected tests | Mitigation |
|---|---|---|---|---|
| R1 | Hub-only registration emits duplicate `hub` backend/registration names or prefixes. | Fleet startup fails or AF talks to an unintended backend. | IT-INFRA-AF-FLEET-2462-001/002, E2E setup topology assertion | Test generated manifests and registration identity before cluster startup; emit exactly one `hub` registration. |
| R2 | Cluster-attributed request silently uses local AF/Kubernetes access instead of Gateway. | Wrong-cluster data may appear valid. | E2E-AF-FLEET-2462-001.. | Assert Gateway tool identity and RR cluster ID; include an unregistered-ID/same-name decoy case. |
| R3 | Local AF and Fleet Kind host ports collide. | One cluster fails to start or tests call the wrong endpoint. | Combined developer setup | Keep `AF_E2E_HOST_PORT_OFFSET` for the combined developer path; separate CI lanes use independent runners and each lane's established host ports. |
| R4 | Fleet Keycloak/Gateway convergence or auth differs from local DEX assumptions. | Fleet AF calls fail before business assertions. | Fleet AF E2E cases | Use the existing Keycloak A2A password-token helper pattern and wait for authenticated Gateway readiness before tests. |
| R5 | Existing full Fleet spoke tests regress. | Remote Fleet coverage is lost while adding the hub-only path. | Existing `test/e2e/fleet` suite | Keep the current full hub-and-spoke setup as the default; hub-only mode must be opt-in and test-infrastructure-only. |
| R6 | One AF mode still exceeds its job budget or teardown obscures diagnostics. | CI instability or incomplete failure evidence. | Local and Fleet AF E2E jobs | Provision only the selected mode in each lane, retain the existing 25-minute GitHub Actions budget initially, emit per-stage setup timing, and collect must-gather from the lane's surviving cluster before teardown. Use CI timing evidence before changing timeouts or adding concurrency. |

## 4. Scope

### 4.1 In scope

- Hub-only Fleet test provisioning using the FMC lane's Keycloak/Gateway/kube-mcp-server/Valkey/FMC components and the existing Kind creation helpers.
- One registered hub backend whose cluster ID is `hub` and whose backend is the Fleet cluster's kube-mcp-server.
- Separate local AF and Fleet AF CI lanes with isolated endpoints, kubeconfigs, Kubernetes clients, and fixture namespaces.
- Fleet-mode AF A2A/SSE tests for the required triage and RCA contracts.
- Explicit rejection of an unregistered cluster identity when a same-named target is available in the Fleet cluster.

### 4.2 Out of scope

- Adding a third Kind cluster or remote/spoke cluster to the AF Fleet lane.
- Changing production cluster-scope behavior or allowing local fallback for Fleet requests.
- Replacing the existing Fleet suite's genuine remote-spoke setup. It remains authoritative for real hub-to-spoke tests.
- Moving local-only AF health, authentication, RBAC, session lifecycle, audit, or metrics tests to Fleet mode.
- Changing `test/infrastructure/fullpipeline_e2e.go`, which is being modified by another team; the lean Fleet AF setup does not use this file.

## 5. Approach and TDD phases

### 5.1 RED

1. Add Ginkgo/Gomega coverage for hub-only registration generation: one `hub` identity, one `hub__` prefix, correct local kube-mcp backend, Authorization forwarding, and no `remote-cluster`, `prod-east`, `prod-west`, or duplicate hub registration.
2. Add BDD coverage for standalone FMC setup prerequisites, Fleet AF/KA config rendering, Keycloak audience/issuer wiring, registry RBAC, Fleet-specific Kind port mappings, and the no-remote-cluster topology assertion.
3. Add Fleet-mode AF E2E assertions for the scenarios in Section 7. Confirm each fails against the current local-only setup or a deliberately missing/unregistered hub registration.
4. Add BDD coverage proving local and Fleet CI lanes diagnose/tear down only their owned cluster, the combined developer mode selects both when Fleet setup was attempted, and Fleet controller namespaces are included only for Fleet-cluster must-gather.

### 5.2 GREEN

1. Reuse the standalone AF Kind creation and deployment helpers for the second cluster. Add only the Keycloak, MCP Gateway, kube-mcp-server, dedicated Valkey, and FMC dependencies from the FMC E2E lane; do not create a spoke cluster or deploy the unrelated full-pipeline controllers.
2. Add an explicit hub-only branch in Gateway registration generation. Preserve the existing full Fleet remote aliases and remote bridge when hub-only is false.
3. Configure the Fleet AF and Kubernaut Agent manifests with the registered hub Gateway, Keycloak OAuth2 credentials, Keycloak JWT validation, and least-privilege registry/FMC RBAC.
4. Add Ginkgo/Gomega coverage for cleanup selection: local lane owns only the local cluster, Fleet lane owns only the Fleet cluster, and the combined developer path continues to collect both when Fleet setup was attempted.
5. Add a `test-e2e-apifrontend-fleet` Make target selecting `fleet-mode-af`, and split the CI matrix into local and Fleet-only entries. Install Helm only for the Fleet AF entry, since it deploys Envoy AI Gateway.
6. Make `SynchronizedBeforeSuite` lane-aware: local CI provisions only standalone AF; Fleet CI provisions only the real hub-only AF/FMC topology; an unset lane preserves today's combined developer behavior. Emit duration records around image builds, Kind setup, fixture/Prometheus provisioning, and readiness waits.
7. Keep the current 25-minute GitHub Actions budget for each lane initially. Do not infer a CI resource issue from local Podman storage failures; use clean-runner timing evidence before changing timeouts or provisioning concurrency.

### 5.3 REFACTOR

- Factor reusable environment/client construction and per-cluster must-gather/teardown into existing helpers where possible.
- Keep hub-only registration rendering deterministic and preserve default full Fleet behavior byte-for-byte where possible.
- Do not modify unrelated files already being edited by other teams.

### 5.4 Verification and rollback

- Run focused Ginkgo BDD coverage for lane ownership/diagnostic selection, plus compile-only E2E checks and Ginkgo dry-runs for both label filters.
- Do not run local live E2E against shared or unisolated Kind infrastructure. Validate the lane setup on CI with current-code images.
- Confirm each CI lane creates and tears down only its named cluster, Fleet setup reports one hub registration and no remote cluster, and timing markers identify the slow setup stage.
- On failure, capture must-gather from the lane's surviving cluster before teardown. The combined developer path still captures both when applicable; full Fleet spoke setup remains unchanged.

## 6. Wiring manifest

| Component | Entry point | Wiring location | Proving test |
|---|---|---|---|
| Local AF environment | `E2E (apifrontend)` with `AF_E2E_LANE=local` | `test/e2e/apifrontend/e2e_suite_test.go` → `SetupAPIFrontendE2EInfrastructure` | Local AF E2E specs; lane ownership BDD |
| Fleet AF environment | `E2E (apifrontend-fleet)` with `AF_E2E_LANE=fleet` | `test-e2e-apifrontend-fleet` → `SetupAPIFrontendFleetE2EInfrastructure` plus `SetupFMCHubOnlyInfrastructure`; no edit to shared full-pipeline setup | IT-INFRA-AF-FLEET-2462-001..009; E2E Fleet specs; lane ownership BDD |
| CI label separation | Existing `apifrontend` matrix row plus new `apifrontend-fleet` row | `.github/workflows/ci-pipeline.yml` sets `GINKGO_LABEL` and `AF_E2E_LANE` per row | Ginkgo dry-runs prove local excludes `fleet-mode-af` and Fleet selects it |
| Hub Gateway route | Fleet AF A2A call with `cluster_id=hub` | Existing `DeployFleetGatewayInfra` plus hub-only registration renderer | E2E-AF-FLEET-2462-001 |
| Triage/RCA contracts | Fleet AF A2A and A2A SSE endpoints | New Fleet-mode AF E2E spec file(s) | E2E-AF-FLEET-2462-002.. |
| Failure diagnostics | Lane-aware AF `SynchronizedAfterSuite` | `test/e2e/apifrontend/e2e_suite_test.go` selects the owning cluster and passes Fleet-specific extra namespaces only for Fleet | UT-E2E-AF-FLEET-MUSTGATHER-001/002 and UT-E2E-AF-FLEET-LANE-001/002 |
| Shared image lifecycle | Combined developer `SynchronizedBeforeSuite` | `BuildAPIFrontendE2EImages` retains images across its local then Fleet cluster imports | UT-INFRA-AF-FLEET-2462-010/011 |

## 7. Scenario inventory

| Proposed ID | Existing local scenario | Fleet-mode contract |
|---|---|---|
| E2E-AF-FLEET-2462-001 | `TC-E2E-SEV-01..06` | Preserve firing, pending, inactive-rule/live-data, no-data, fail-closed/no-correlation, and user-hint triage outcomes for `cluster_id=hub`; filter cluster-labeled alert collisions. |
| E2E-AF-FLEET-2462-002 | `E2E-AF-1395-001`, `1396-001/002` | Structured decision >512 characters, RCA fields, and workflow options arrive intact over Fleet AF SSE. |
| E2E-AF-FLEET-2462-003 | `E2E-AF-1407-001..003`, `1408-001` | `early_rca` carries severity/confidence; `investigation_summary` has the expected DataPart and schema version. |
| E2E-AF-FLEET-2462-004 | `E2E-AF-1922-001` | Concurrent `session_active` rejection still returns a renderable summary with non-empty causal chain. |
| E2E-AF-FLEET-2462-005 | New negative contract | An unregistered cluster ID fails closed despite a same-named Fleet-cluster object; no implicit local fallback. |

### 7.1 Failure diagnostic collection

| Scenario ID | Contract |
|---|---|
| UT-E2E-AF-FLEET-MUSTGATHER-001 | Always include the local cluster; include the separate Fleet cluster when Fleet setup was attempted. |
| UT-E2E-AF-FLEET-MUSTGATHER-002 | Request `envoy-gateway-system` and `envoy-ai-gateway-system` logs only for the Fleet cluster's must-gather run. |
| UT-E2E-AF-FLEET-LANE-001 | Local CI lane diagnostics and teardown include only the standalone local AF cluster. |
| UT-E2E-AF-FLEET-LANE-002 | Fleet CI lane diagnostics and teardown include only the Fleet AF cluster, and include no cluster if setup failed before creation. |

### 7.2 Shared CI image lifecycle

| Scenario ID | Contract |
|---|---|
| UT-INFRA-AF-FLEET-2462-010 | Ordinary single-cluster Kind image loads prune the Podman source copy. |
| UT-INFRA-AF-FLEET-2462-011 | The combined developer path retains shared images so the second Fleet cluster can load them before pruning. |

## 8. Approval and confidence

**Approval status**: User approved the 2026-09-26 CI-lane amendment to DD-TEST-019. Each lane keeps the existing 25-minute GitHub Actions budget initially; the 18-minute Ginkgo default remains unchanged pending isolated timing evidence. Local Podman storage exhaustion is not treated as CI evidence.
**Confidence**: 90%. The standalone AF setup, FMC lane's minimal Fleet path, labels, cleanup ownership, and coverage output conventions are confirmed in code. The implementation is validated with targeted BDD and dry-run checks before CI; live validation must use the isolated CI jobs and current-code images.
