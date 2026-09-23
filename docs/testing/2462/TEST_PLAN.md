# Test Plan: Isolated local and Fleet-mode AF E2E coverage

**Test Plan Identifier**: TP-2462-v1.0
**Feature**: Exercise local and Fleet-mode APIFrontend contracts in the existing AF E2E job using two isolated Kind clusters
**Version**: 1.2
**Created**: 2026-09-23
**Author**: Kubernaut development team
**Status**: Approved
**Branch**: `fix/2442-workflow-discovery-membership`

---

## 1. Introduction

### 1.1 Purpose

Keep the standalone APIFrontend (AF) E2E baseline in local mode, while proving cluster-attributed AF triage and A2A/SSE contracts against a separate Fleet-enabled AF deployment. A Fleet request with `cluster_id=hub` must resolve through the MCP Gateway's registered `hub` backend, even when the Gateway backend and Fleet AF run in the same physical Kind cluster. The local and Fleet AF Kind clusters remain isolated.

### 1.2 Objectives and success metrics

1. The existing `E2E (apifrontend)` GitHub Actions job creates exactly two isolated Kind clusters: one for local AF and one for Fleet-mode AF. No new workflow lane is added.
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
- The local AF and full-pipeline Kind configs share host mappings (including 8088, 9190, and 9193). The AF lane already supports `AF_E2E_HOST_PORT_OFFSET`; offset the local AF cluster and leave Fleet's established host ports unchanged.

### 2.2 Spike decision

**Question**: Can these AF hub-attributed scenarios run against a Fleet Gateway registration whose backend is in the same physical cluster, while keeping the standalone local AF cluster separate?
**Decision**: **Yes.** The Fleet AF cluster can be a single-cluster hub topology for these hub-target journeys. Existing Fleet E2E retains genuine spoke coverage.
**Implementation caveat**: Use the FMC lane's Fleet-core components with an explicit hub-only registration. Do not create a duplicate `hub`/loopback registration or a remote/spoke Kind cluster.

## 3. Risks and mitigations

| ID | Risk | Impact | Affected tests | Mitigation |
|---|---|---|---|---|
| R1 | Hub-only registration emits duplicate `hub` backend/registration names or prefixes. | Fleet startup fails or AF talks to an unintended backend. | IT-INFRA-AF-FLEET-2462-001/002, E2E setup topology assertion | Test generated manifests and registration identity before cluster startup; emit exactly one `hub` registration. |
| R2 | Cluster-attributed request silently uses local AF/Kubernetes access instead of Gateway. | Wrong-cluster data may appear valid. | E2E-AF-FLEET-2462-001.. | Assert Gateway tool identity and RR cluster ID; include an unregistered-ID/same-name decoy case. |
| R3 | Local AF and Fleet Kind host ports collide. | One cluster fails to start or tests call the wrong endpoint. | Suite setup | Reuse `AF_E2E_HOST_PORT_OFFSET` and existing URL helpers for the local cluster; keep Fleet mappings unchanged. |
| R4 | Fleet Keycloak/Gateway convergence or auth differs from local DEX assumptions. | Fleet AF calls fail before business assertions. | Fleet AF E2E cases | Use the existing Keycloak A2A password-token helper pattern and wait for authenticated Gateway readiness before tests. |
| R5 | Existing full Fleet spoke tests regress. | Remote Fleet coverage is lost while adding the hub-only path. | Existing `test/e2e/fleet` suite | Keep the current full hub-and-spoke setup as the default; hub-only mode must be opt-in and test-infrastructure-only. |
| R6 | Two isolated AF stacks plus Fleet core exceed runner capacity or teardown obscures diagnostics. | CI instability or incomplete failure evidence. | Full AF E2E job | Avoid unrelated pipeline controllers, reuse the AF image set in both clusters, preserve the existing job and 25-minute timeout, and collect must-gather from each surviving cluster before teardown. A local Podman storage failure is host-local evidence and does not justify changing CI without a CI failure. |

## 4. Scope

### 4.1 In scope

- Hub-only Fleet test provisioning using the FMC lane's Keycloak/Gateway/kube-mcp-server/Valkey/FMC components and the existing Kind creation helpers.
- One registered hub backend whose cluster ID is `hub` and whose backend is the Fleet cluster's kube-mcp-server.
- Separate local AF and Fleet AF endpoints, kubeconfigs, Kubernetes clients, and fixture namespaces within the existing AF E2E job.
- Fleet-mode AF A2A/SSE tests for the required triage and RCA contracts.
- Explicit rejection of an unregistered cluster identity when a same-named target is available in the Fleet cluster.

### 4.2 Out of scope

- Adding a GitHub Actions lane or a third Kind cluster.
- Changing production cluster-scope behavior or allowing local fallback for Fleet requests.
- Replacing the existing Fleet suite's genuine remote-spoke setup. It remains authoritative for real hub-to-spoke tests.
- Moving local-only AF health, authentication, RBAC, session lifecycle, audit, or metrics tests to Fleet mode.
- Changing `test/infrastructure/fullpipeline_e2e.go`, which is being modified by another team; the lean Fleet AF setup does not use this file.

## 5. Approach and TDD phases

### 5.1 RED

1. Add Ginkgo/Gomega coverage for hub-only registration generation: one `hub` identity, one `hub__` prefix, correct local kube-mcp backend, Authorization forwarding, and no `remote-cluster`, `prod-east`, `prod-west`, or duplicate hub registration.
2. Add BDD coverage for standalone FMC setup prerequisites, Fleet AF/KA config rendering, Keycloak audience/issuer wiring, registry RBAC, Fleet-specific Kind port mappings, and the no-remote-cluster topology assertion.
3. Add Fleet-mode AF E2E assertions for the scenarios in Section 7. Confirm each fails against the current local-only setup or a deliberately missing/unregistered hub registration.
4. Add BDD coverage proving failure diagnostics select both clusters when Fleet setup was attempted and include Fleet controller namespaces only in the Fleet cluster's must-gather call.

### 5.2 GREEN

1. Reuse the standalone AF Kind creation and deployment helpers for the second cluster. Add only the Keycloak, MCP Gateway, kube-mcp-server, dedicated Valkey, and FMC dependencies from the FMC E2E lane; do not create a spoke cluster or deploy the unrelated full-pipeline controllers.
2. Add an explicit hub-only branch in Gateway registration generation. Preserve the existing full Fleet remote aliases and remote bridge when hub-only is false.
3. Configure the Fleet AF and Kubernaut Agent manifests with the registered hub Gateway, Keycloak OAuth2 credentials, Keycloak JWT validation, and least-privilege registry/FMC RBAC.
4. Update the existing AF E2E `SynchronizedBeforeSuite` to start local AF and Fleet AF stacks with distinct names/kubeconfigs/endpoints and to distribute both contexts to parallel Ginkgo processes. Build the AF image set once and reuse it in both clusters.
5. Keep the existing AF CI job and 25-minute timeout unchanged. Do not infer a CI resource issue from local Podman storage failures; validate against CI's clean runner environment.
6. Use the local AF port-offset support to avoid collisions. Keep all local test helpers bound to local AF; new Fleet tests use the separate Fleet AF client and Keycloak token.

### 5.3 REFACTOR

- Factor reusable environment/client construction and per-cluster must-gather/teardown into existing helpers where possible.
- Keep hub-only registration rendering deterministic and preserve default full Fleet behavior byte-for-byte where possible.
- Do not modify unrelated files already being edited by other teams.

### 5.4 Verification and rollback

- Run targeted infrastructure BDD tests and compile-only E2E checks first.
- Run focused local AF and Fleet-mode AF scenarios, then the full AF suite.
- Confirm the two cluster names/kubeconfigs, host-port offset, and hub Gateway registration in setup output.
- On failure, capture must-gather from both clusters before teardown. Rollback is limited to the lean Fleet AF setup and Fleet AF specs; the local AF baseline and full Fleet spoke setup remain intact.

## 6. Wiring manifest

| Component | Entry point | Wiring location | Proving test |
|---|---|---|---|
| Local AF environment | Existing AF `SynchronizedBeforeSuite` | `test/e2e/apifrontend/e2e_suite_test.go` → `SetupAPIFrontendE2EInfrastructure` | Existing AF E2E suite |
| Fleet AF environment | Existing AF `SynchronizedBeforeSuite` calls the standalone AF setup with Fleet options | `SetupAPIFrontendFleetE2EInfrastructure` plus `SetupFMCHubOnlyInfrastructure`; no edit to the shared full-pipeline setup file | IT-INFRA-AF-FLEET-2462-001..009 and E2E setup topology assertion |
| Hub Gateway route | Fleet AF A2A call with `cluster_id=hub` | Existing `DeployFleetGatewayInfra` plus hub-only registration renderer | E2E-AF-FLEET-2462-001 |
| Triage/RCA contracts | Fleet AF A2A and A2A SSE endpoints | New Fleet-mode AF E2E spec file(s) | E2E-AF-FLEET-2462-002.. |
| Failure diagnostics | Existing AF `SynchronizedAfterSuite` | `test/e2e/apifrontend/e2e_suite_test.go` selects each surviving cluster and passes Fleet-specific extra namespaces | UT-E2E-AF-FLEET-MUSTGATHER-001/002 |

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

## 8. Approval and confidence

**Approval status**: User approved the two-isolated-cluster design and selected the lean AF-plus-Fleet topology (DD-TEST-019). The existing CI workflow and 25-minute timeout remain unchanged; local Podman storage exhaustion is not assumed to reproduce on CI's clean runner.
**Confidence**: 88%. The standalone AF setup and FMC lane's minimal Fleet component path are confirmed; hub-only Gateway, AF/KA config, RBAC, port-map, and infrastructure BDD tests pass. Local E2E attempts stopped during local image building because this host's Podman storage filled before Kind creation. That is not evidence of a CI issue; live validation must use the existing CI job. Must-gather selection and Fleet-only controller namespace scope have focused BDD coverage.
