# Test Plan: AF Fleet-Aware Prometheus Triage

**Test Plan Identifier**: TP-2394-v1.0
**Feature**: Scope AF severity and signal correlation to the requested fleet cluster
**Version**: 1.0
**Created**: 2026-09-13
**Author**: Kubernaut development team
**Status**: Active
**Branch**: `fix/issue-2390-workflow-snapshot`

## 1. Introduction

### 1.1 Purpose

Issue #2394 closes a fleet isolation gap in AF-created RemediationRequests. AF
queries Prometheus/Thanos fleet-wide, but generic auto-triage previously matched
only namespace/kind/name and could select an equivalent alert from another
cluster. This plan proves that fleet signal and severity correlation is scoped
to the requested `cluster_id`, while preserving hub-local behavior.

### 1.2 Objectives

1. A fleet target selects only alerts attributed to its requested `cluster` label.
2. A fleet target rejects un-attributed or differently attributed alert data.
3. Empty cluster identity preserves local single-cluster triage; local cluster labels are not treated as Gateway identities.
4. Pending and inactive Prometheus rules obey the same fleet cluster boundary.
5. The production AF investigate path carries `cluster_id` into triage and the
   created RR preserves the selected alert and severity.
6. No implementation introduces direct hub-to-spoke Kubernetes API access or
   remote Kubernetes Event consumption.

## 2. References

### 2.1 Authority

- BR-FLEET-054: fleet cluster-scoped access and correlation
- BR-INTEGRATION-065: multi-cluster signal routing and scope gating
- BR-AI-056: grounded AF investigation signal identity
- [ADR-065: Fleet Cluster Identity on RR](../../architecture/decisions/ADR-065-fleet-cluster-identity-on-rr.md)
- [ADR-068: Fleet Federation Architecture](../../architecture/decisions/ADR-068-fleet-federation-architecture.md)
- [Issue #2394](https://github.com/jordigilh/kubernaut/issues/2394)

### 2.2 Security Control Objectives

| Control family | Objective proven by this plan |
|----------------|-------------------------------|
| FedRAMP AC-4 | Alert-derived information is not correlated across cluster boundaries. |
| FedRAMP AC-6 | Un-attributed or unauthorized cluster data is not accepted as fleet evidence. |
| FedRAMP SC-7 | Remote Kubernetes access remains behind the MCP Gateway; this change adds no direct network path. |
| FedRAMP SI-4 | Monitoring evidence and RR signal identity retain the target cluster attribution. |
| FedRAMP AU-3 | The created RR contains the selected signal/severity and cluster provenance. |
| OWASP ASVS V4, Access Control | Server-side cluster authorization is enforced by default and cannot be bypassed by matching resource labels alone. |
| OWASP ASVS V5, Validation | The supplied cluster identity is used as an exact server-side attribution constraint; missing attribution fails closed. |

## 3. Risks and Mitigations

| ID | Risk | Affected tests | Mitigation |
|----|------|----------------|------------|
| R1 | Same namespace/kind/name exists in multiple clusters and the wrong alert is selected. | UT-AF-2394-001, IT-AF-2394-006 | Match alert and rule labels against the requested cluster. |
| R2 | Missing `cluster` attribution is treated as valid fleet evidence. | UT-AF-2394-002 | Require an exact cluster label for non-empty `ClusterID`. |
| R3 | Cluster filtering breaks hub-local deployments. | UT-AF-2394-003, existing severity suite | Preserve the empty-`ClusterID` path. |
| R4 | AF receives `cluster_id` but does not pass it to the triager. | IT-AF-2394-006 | Exercise the real registered `kubernaut_investigate` dispatch path. |
| R5 | A fix attempts to bypass fleet network controls for Kubernetes Events. | UT-AF-2390-002, code review, ADR-068 boundary section | Keep Kubernetes Event fallback hub-local and use Prometheus/Thanos for fleet signal grounding. |

## 4. Scope

### 4.1 Features Tested

- `pkg/apifrontend/severity`: alert and rule correlation by fleet cluster.
- `pkg/apifrontend/tools/af_create_rr.go`: propagation of `CreateRRArgs.ClusterID` into triage.
- `pkg/apifrontend/tools/ka_investigate_mcp.go`: production AF investigate dispatch.
- Standalone AF E2E: single-cluster severity scenarios omit Gateway identity and
  preserve local alert correlation without deploying MCP Gateway.
- Full fleet E2E: real AF A2A remediation targeting the Gateway-registered hub
  while a higher-severity alert with identical target labels is attributed to a
  spoke cluster.
- Full fleet E2E: the Gateway rejects an unattributed signal even when its
  target resource exists on the hub, proving no implicit local fallback.

### 4.2 Features Not Tested

- Remote Kubernetes Event reads or watches. These are not part of the current
  MCP Gateway contract and are prohibited as a direct hub-to-spoke path.
- Customer-specific Event exporters. If customers export Event-derived
  telemetry into Prometheus/Thanos, Kubernaut consumes only the resulting alert
  and requires its normal cluster attribution.

## 5. Test Approach

### 5.1 Pyramid and Wiring Manifest

| Component | Production entry point | Wiring location | Test ID |
|-----------|-------------------------|-----------------|---------|
| Cluster-aware alert/rule triage | `Triager.Triage` | `pkg/apifrontend/severity/triage.go` | UT-AF-2394-001..005, UT-AF-2394-006..009 |
| Cluster ID propagation | `HandleCreateRRWithHooks` via `kubernaut_investigate` | `pkg/apifrontend/tools/af_create_rr.go`, `ka_investigate_mcp.go` | IT-AF-2394-006 |
| Local AF cluster-label handling | `Triager.Triage` with fleet disabled | `pkg/apifrontend/severity/triage.go`, `test/infrastructure/apifrontend_prometheus_e2e.go` | UT-AF-2394-003/008/010, UT-INFRA-AF-2394-001 |
| Hub Gateway attribution and severity selection | Real AF A2A `kubernaut_remediate` | `test/e2e/fleet/24_af_hub_cluster_triage_test.go` and `test/infrastructure/fleet_e2e.go` | E2E-FLEET-2394-001 |
| Missing cluster attribution fails closed | Prometheus webhook owner resolution | `cmd/gateway/main.go`, `pkg/gateway/adapters/prometheus_adapter.go` | E2E-FLEET-2394-002 |
| Local-only Event fallback | AF RR creation | `pkg/apifrontend/tools/af_create_rr.go` | UT-AF-2390-001/002 |

### 5.2 Scenario Inventory

| ID | Tier | Business behavior | Controls | BR |
|----|------|-------------------|----------|----|
| UT-AF-2394-001 | UT | Selects the requested cluster's resource alert over an identical higher-severity alert from another cluster. | AC-4, SI-4, ASVS V4 | BR-FLEET-054 |
| UT-AF-2394-002 | UT | Fails closed when only other-cluster or un-attributed alerts exist. | AC-6, ASVS V4/V5 | BR-FLEET-054 |
| UT-AF-2394-003 | UT | Empty cluster identity preserves hub-local alert matching. | SI-4 | BR-INTEGRATION-065 |
| UT-AF-2394-004 | UT | Selects pending rules only when their cluster attribution matches. | AC-4, SI-4, ASVS V4 | BR-FLEET-054 |
| UT-AF-2394-005 | UT | Evaluates inactive rules only when their cluster attribution matches. | AC-4, SI-4, ASVS V4 | BR-FLEET-054 |
| IT-AF-2394-006 | IT | Real AF investigate dispatch carries `cluster_id` into triage and writes the selected signal/severity to the RR. | AU-3, AC-4, SC-7, SI-4, ASVS V4 | BR-FLEET-054, BR-INTEGRATION-065 |
| UT-AF-2394-006 | UT | Fleet triage fails closed when the target cluster ID is empty. | AC-6, ASVS V4/V5 | BR-FLEET-054 |
| UT-AF-2394-007 | UT | Fleet triage rejects rules without exact cluster attribution. | AC-4, AC-6, ASVS V4/V5 | BR-FLEET-054 |
| UT-AF-2394-008 | UT | Local triage ignores an incidental cluster ID and still matches local alerts. | SI-4 | BR-INTEGRATION-065 |
| UT-AF-2394-009 | UT | Fleet hub triage matches `cluster=hub` and excludes spoke collisions. | AC-4, SI-4, ASVS V4 | BR-FLEET-054 |
| UT-AF-2394-010 | UT | Local triage matches pending rules without a Gateway cluster ID. | SI-4 | BR-INTEGRATION-065 |
| UT-INFRA-AF-2394-001 | UT | Standalone AF Prometheus fixtures omit a synthetic Gateway cluster label. | SI-4 | BR-INTEGRATION-065 |
| E2E-FLEET-2394-001 | E2E | Real AF A2A remediation targets a hub-only resource, retains `cluster_id=hub`, and selects its warning alert over an identical-label remote critical collision. | AC-4, SC-7, SI-4, AU-3 | BR-FLEET-054 |
| E2E-FLEET-2394-002 | E2E | Fleet Gateway rejects an unattributed signal and creates no RR even when the same target exists on the hub. | AC-4, AC-6, SI-10, SC-7, ASVS V4/V5 | BR-FLEET-054, BR-INTEGRATION-065 |
| UT-AF-2390-001 | UT | Hub-local investigation preserves a grounded local Kubernetes Event signal. | SI-4, ASVS V5 | BR-AI-056 |
| UT-AF-2390-002 | UT | Fleet investigation does not use a hub-local Kubernetes Event as a remote signal. | AC-4, AC-6, SC-7, ASVS V4 | BR-FLEET-054 |

### 5.3 Pass Criteria

- All listed UT and IT scenarios pass.
- E2E-FLEET-2394-001 passes in the fleet E2E lane.
- E2E-FLEET-2394-002 passes in the fleet E2E lane.
- Existing `pkg/apifrontend/severity` and `pkg/apifrontend/tools` suites have
  zero regressions.
- `go build ./...` succeeds.
- No direct spoke Kubernetes client or Event transport is added.

## 6. Spike Decision

**Question**: Should AF add a remote Kubernetes Event reader for fleet targets?
**Decision**: No.

Fleet infrastructure permits remote Kubernetes access only through the MCP
Gateway. The current fleet MCP contract does not expose Event propagation or
watch semantics to AF signal derivation. Customers may independently convert
Events into Prometheus/Thanos telemetry and AlertManager alerts, but AF consumes
the resulting attributed alert rather than the original Event. A future remote
Event capability would require an explicit MCP contract and separate design
decision.

## 7. Completion Status

Implementation, affected package tests, build, and changed-code lint pass. Live
fleet/full-pipeline validation remains environment-dependent and will run in
CI when the scenario executes.
