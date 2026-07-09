# Test Plan: Fleet `cluster_id` on AF RR-Creating Tools + Context Propagation

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1409-v1.0
**Feature**: Thread `cluster_id` as an LLM-facing input on AF's RR-creating tools
(`kubernaut_investigate_alert`, `kubernaut_investigate`, `kubernaut_remediate`), read it
back into `EventBridge.RRContext`, and propagate it into `investigation_summary` and
`execution_progress` A2A artifacts for Console multi-cluster context banners. Folds in
two related correctness fixes discovered during preflight (#1423 takeover-path context
gap, cluster-unaware dedup fingerprint in `kubernaut_check_existing_remediation`).
**Version**: 1.0
**Created**: 2026-07-08
**Author**: AI Agent
**Status**: Draft
**Branch**: `feat/1409-af-cluster-id-events`

---

## 1. Introduction

### 1.1 Purpose

Issue #1409 asks for Console context banners to show which cluster a remediation is
running against in multi-cluster (fleet) deployments. ADR-065 already added
`ClusterID`/`ClusterName` to `RemediationRequestSpec` and wired Gateway to populate them
from Thanos `external_labels`, but reserved AF's side as `IT-AF-FLEET (pending)`. Preflight
for this plan found that AF's *internal* `CreateRRArgs`/`buildRRObject` already thread
`ClusterID` into the RR spec, but no LLM-facing tool exposes `cluster_id` as an argument,
so it is never actually set outside of Gateway-created RRs. Separately, `CreateRRResult`
does not return `ClusterID`, so even AF-created RRs can't populate `RRContext` for the
Console banner. This plan closes both gaps and the two related correctness issues below.

### 1.2 Objectives

1. **Write-side correctness**: `kubernaut_investigate_alert`, `kubernaut_investigate`, and
   `kubernaut_remediate` accept an optional `cluster_id` argument and persist it to
   `RemediationRequestSpec.ClusterID` on creation.
2. **Read-back correctness**: `EventBridge.RRContext` carries `ClusterID`, and every status
   event and the `investigation_summary`/`execution_progress` artifacts include `cluster_id`
   in their metadata/payload once available.
3. **Takeover-path correctness (#1423 gap)**: `kubernaut_investigate`'s `rr_id`-only
   (takeover) branch fetches the full `RemediationRequest` object and populates all
   available `RRContext` fields (namespace, kind, target, alert_name, cluster_id), not just
   `rr_id`, with graceful degradation if the fetch fails.
4. **Dedup correctness**: `kubernaut_check_existing_remediation` uses the same cluster-aware
   fingerprint as `HandleCreateRR` (`rrFingerprintWithCluster`), so a resource with the same
   namespace/kind/name on two different clusters is not incorrectly reported as a duplicate.
5. **Backward compatibility**: local-hub (single-cluster) callers that omit `cluster_id`
   see byte-identical behavior to today (empty-string cluster ID, existing fingerprints
   unchanged — proven by `CalculateClusterAwareFingerprint("", ...)` already used by
   `rrFingerprint`).
6. **Wiring proof (Pyramid Invariant)**: `cluster_id` set via any of the three tools is
   observable end-to-end in the SSE `investigation_summary` artifact, not just in an
   in-memory struct field.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|---|---|---|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/launcher/... ./pkg/apifrontend/tools/... ./pkg/apifrontend/validate/...` |
| Integration test pass rate | 100% | `go test ./test/integration/apifrontend/...` |
| Unit-testable code coverage | >=80% | `go tool cover` on modified files listed in Section 6.1 |
| Backward compatibility | 0 regressions | Existing AF unit/integration suites pass unmodified |
| E2E proof | 1 passing scenario | `cluster_id` reaches SSE `investigation_summary` artifact |

### 1.4 FedRAMP / SOC2 Control Mapping

> Every test in this plan verifies a business-level compliance behavior, not just a
> technical implementation detail. Each test ID in Section 8/9 carries a bracketed
> control tag, e.g. `UT-AF-1409-005 [AC-4]`, tying the test to the specific NIST 800-53
> (FedRAMP) or SOC2 control it satisfies. SOC2 criteria are added only where they
> genuinely apply alongside the more precise FedRAMP mapping (dual-mapping policy) —
> not force-fit onto every row.

| Control | Framework | Requirement | Satisfied By |
|---|---|---|---|
| AU-3 | FedRAMP (NIST 800-53) | Content of audit records: structured event payloads carry correct actor/cluster attribution | UT-AF-1409-003, UT-AF-1409-004, UT-AF-1409-007, UT-AF-1409-008, IT-AF-1409-001/002/003/007, E2E-AF-1409-001 |
| SI-4 | FedRAMP (NIST 800-53) | System monitoring: cross-cluster signal correlation via `ClusterID` (mirrors ADR-065's own AU-3/SI-4 mapping for this field) | UT-AF-1409-001, IT-AF-1409-001/002/003/007, E2E-AF-1409-001 |
| AC-4 | FedRAMP (NIST 800-53) | Information flow enforcement: cluster-scoped dedup fingerprint prevents remediation-state conflation across cluster boundaries | UT-AF-1409-005, IT-AF-1409-006 |
| SI-10 | FedRAMP (NIST 800-53) | Input validation: caller-supplied context takes precedence over server-merged fields; `cluster_id` is length-bounded | UT-AF-1409-002, UT-AF-1409-006, UT-AF-1409-009 |
| AC-6 | FedRAMP (NIST 800-53) | Least privilege: takeover-path fetch relies on a `get`-only RBAC grant, no broader access | IT-AF-1409-008 |
| SI-17 | FedRAMP (NIST 800-53) | Fail-safe procedures: takeover-path fetch degrades gracefully instead of failing closed on the whole session | IT-AF-1409-005 |
| AU-2 | FedRAMP (NIST 800-53) | Audit events: takeover-path fetch failures are logged, never silently swallowed | IT-AF-1409-005 |
| CC8.1 | SOC2 | Complete remediation request reconstruction from audit/artifact traces alone (BR-AUDIT-005) | IT-AF-1409-004, E2E-AF-1409-001 |

Updated tallies after closing the tier-skip and dead-code gaps below: AU-3 additionally covered by UT-AF-1409-010, IT-AF-1409-009; SI-10 additionally covered by IT-AF-1409-009, IT-AF-1409-010.

---

## 2. References

### 2.1 Authority (governing documents)

- BR-FLEET-001: Fleet remediation requires cluster identity on every RR
- BR-INTEGRATION-065: Multi-cluster signal routing and scope gating
- [ADR-065](../../architecture/decisions/ADR-065-fleet-cluster-identity-on-rr.md): Fleet Cluster Identity on RR — reserves `IT-AF-FLEET (pending)` for this work
- Issue [#1409](https://github.com/jordigilh/kubernaut/issues/1409): Console cluster context banner
- Issue #1423 (referenced): takeover-path Console banner context gap
- Issue #54: Multi-cluster remediation tracking

### 2.2 Cross-References

- [Wiring Verification](../../../.cursor/rules/10-wiring-verification.mdc)
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|-----------------|------------|
| R1 | Adding `cluster_id` to `CheckExistingRRArgs` changes the fingerprint used by an already-shipped tool, silently breaking existing (single-cluster) dedup behavior | High — duplicate RRs created, or false-positive dedup | Low | UT-AF-1409-005, IT-AF-1409-006 | `CalculateClusterAwareFingerprint("", ...)` is proven backward-compatible (empty clusterID produces the pre-existing 3-field fingerprint format); regression-tested explicitly |
| R2 | Takeover-path `client.Get()` fetch (#1423 fix) fails (RR deleted mid-session, RBAC denial) and blanks the whole banner instead of degrading gracefully | Medium — Console banner regression for takeover sessions | Low | IT-AF-1409-005 | Get failure is logged (AU-2) and falls back to the pre-existing `rr_id`-only `RRContext`, never panics or blocks the tool response |
| R3 | RBAC: AF's ServiceAccount lacks `get` on `remediationrequests` for the takeover fetch | High — takeover path fetch always fails | Low (confirmed via preflight) | IT-AF-1409-008 | `deploy/apifrontend/base/02-rbac.yaml` already grants `get` on `remediationrequests`; regression test pins this |
| R4 | `mergeRRContext`/`emitDecisionEvent` metadata key collision: caller-supplied `cluster_id` in a `FunctionCall` overwritten or dropped | Low — Console shows stale/wrong cluster | Low | UT-AF-1409-002, UT-AF-1409-006 | Follows the existing `mergeRRContext` precedence rule (caller keys win, SI-10) already proven for `rr_id`/`namespace`/etc. |
| R5 | `ClusterID` format/source is documented inconsistently across `remediationrequest_types.go`, ADR-065, and `af_create_rr.go` (confirmed pre-existing doc drift, not introduced by this change) | Low — confusing tool description for the LLM | N/A | N/A | Out of scope for this PR; new tool descriptions kept format-agnostic (mirrors `kubectl_get` convention) rather than compounding the drift |

### 3.1 Risk-to-Test Traceability

R1-R3 are High/Medium impact and each has a dedicated regression test (UT-AF-1409-005,
IT-AF-1409-005/006/008). R4 and R5 are Low impact; R4 has unit coverage, R5 is a
documented non-goal.

---

## 4. Scope

### 4.1 Features to be Tested

- **`EventBridge.RRContext`** (`pkg/apifrontend/launcher/event_bridge.go`): `ClusterID` field
  added, merged into status-event metadata via `mergeRRContext`.
- **`CreateRRResult`** (`pkg/apifrontend/tools/af_create_rr.go`): `ClusterID` returned to
  callers on both the create path (`buildRRObject`/`createOrReuseRR`) and the
  dedup/reuse path (`checkExistingRRByFingerprint`/`CheckExistingRRResult`).
- **`kubernaut_investigate_alert`** (`pkg/apifrontend/tools/af_investigate_alert.go`):
  `cluster_id` accepted as LLM input, wired to `CreateRRArgs.ClusterID` and
  `SetRRContextSafe`.
- **`kubernaut_remediate`** (`pkg/apifrontend/tools/ka_remediate.go`): same wiring.
- **`kubernaut_investigate`** (`pkg/apifrontend/tools/ka_investigate_mcp.go`): same wiring
  on the create path; takeover (`rr_id`-only) path gains a `client.Get()` fetch that
  populates full `RRContext` with graceful degradation.
- **`kubernaut_check_existing_remediation`** (`pkg/apifrontend/tools/af_check_existing_rr.go`):
  `cluster_id` accepted, cluster-aware fingerprint used for dedup lookups.
- **`emitDecisionEvent`** (`pkg/apifrontend/launcher/part_converter.go`): merges
  `cluster_id` into the `investigation_summary` `DataPart` via a new thread-safe
  `EventBridge` accessor.
- **`BuildProgressSnapshot`** (`pkg/apifrontend/tools/execution_progress.go`) and its call
  site in `crd_tools_watch.go`: `cluster_id` added to the `execution_progress` payload,
  sourced from the live `RemediationRequest` object.
- **`validate.ClusterID`** (`pkg/apifrontend/validate/k8s.go`, new function): optional,
  `MaxLength=253` input validation (SI-10).

### 4.2 Features Not to be Tested

- **`cluster_name`**: explicitly excluded from this change (deprecated field, not unique,
  slated for removal — user decision).
- **Gateway-side cluster extraction** (`pkg/gateway/adapters/prometheus_adapter.go`):
  already covered by `IT-GW-FLEET-001/002/003` (ADR-065, PASS). Not re-tested here.
- **RO/notification-side cluster consumption**: out of scope, separate ADR-065 migration
  item.
- **`ClusterID` format/source documentation inconsistency**: pre-existing, tracked as a
  documentation cleanup, not a test gap.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Add `cluster_id` only, not `cluster_name`, to all new LLM-facing args | `cluster_name` is non-unique and slated for removal; adding it now would create a feature dependent on a deprecated field |
| Fold #1423 takeover-path gap and dedup-fingerprint correctness fix into this PR rather than filing separately | Both are pre-existing technical debt that this change would otherwise silently make worse (dedup) or leave stale (Console banner never populated on takeover) |
| No new `DD-XXX` design decision record | This is an incremental extension of an existing, already-approved pattern (ADR-065 anticipated exactly this AF-side write path); no new architectural pattern introduced |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable logic (fingerprint selection, `mergeRRContext`,
  `BuildProgressSnapshot`, `validate.ClusterID`).
- **Integration**: 100% of wiring points in the manifest below (Section 14), each proven
  through the real tool-handler-to-CRD-to-EventBridge path (no mocks on `pkg/` business
  logic; fake `client.Client` for K8s per existing AF IT convention).
- **E2E**: one proving journey (`cluster_id` reaches the SSE artifact), per Pyramid
  Invariant — this is an extension of an existing tool, not a new subsystem, so a single
  E2E scenario is sufficient rather than a full new E2E suite.

### 5.2 Two-Tier Minimum

Every objective in Section 1.2 is covered by at least UT + IT (see BR Coverage Matrix,
Section 7).

### 5.3 Pass/Fail Criteria

**PASS**: all P0 tests pass, coverage targets met (Section 1.3), zero regressions in
existing AF unit/integration suites (`af_create_rr_test.go`,
`af_check_existing_rr_test.go`, `event_bridge_test.go`, `ka_investigate_mcp_test.go`).

**FAIL**: any P0 test fails, any existing passing test regresses, or coverage drops below
80% on any modified file's tier.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/apifrontend/launcher/event_bridge.go` | `RRContext`, `mergeRRContext`, `RRContext()`/`RRContextSafe` (new accessors) | ~25 |
| `pkg/apifrontend/tools/af_create_rr.go` | `createOrReuseRR`, `checkExistingRRByFingerprint`, `CreateRRResult` | ~15 |
| `pkg/apifrontend/tools/af_check_existing_rr.go` | `HandleCheckExistingRR`, `CheckExistingRRArgs/Result` | ~15 |
| `pkg/apifrontend/tools/execution_progress.go` | `BuildProgressSnapshot` | ~10 |
| `pkg/apifrontend/launcher/part_converter.go` | `emitDecisionEvent` | ~10 |
| `pkg/apifrontend/validate/k8s.go` | `ClusterID` (new) | ~10 |
| `pkg/apifrontend/tools/ka_investigate_mcp.go` | `resolveInvestigationRR` takeover-fetch decision logic (success/failure/degrade), driven directly via `HandleInvestigationMCPWithRegistry` with a fake `client.Client` and mock MCP client (no MCP SDK/HTTP transport — same convention as the existing `UT-AF-1326-*` cases in `ka_investigate_mcp_test.go`) | ~15 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/apifrontend/tools/af_investigate_alert.go` | `InvestigateAlertArgs`, `HandleInvestigateAlert` | ~10 |
| `pkg/apifrontend/tools/ka_remediate.go` | `RemediateArgs`, `HandleRemediate` | ~10 |
| `pkg/apifrontend/tools/ka_investigate_mcp.go` | `InvestigateMCPArgs`, `resolveInvestigationRR` (takeover fetch, wiring proof through the real registered-tool dispatch path) | ~30 |
| `pkg/apifrontend/tools/crd_tools_watch.go` | `handleRREvent` (`BuildProgressSnapshot` call site) | ~5 |
| `deploy/apifrontend/base/02-rbac.yaml` | ClusterRole `get` on `remediationrequests` | N/A (config regression) |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feat/1409-af-cluster-id-events` HEAD | Branched from `origin/main` at `908672b71` |
| Dependency: ADR-065 | Merged | `CreateRRArgs.ClusterID`/`buildRRObject` already implemented |

---

## 7. BR Coverage Matrix

| BR ID | Description | Control | Priority | Tier | Test ID | Status |
|-------|-------------|---------|----------|------|---------|--------|
| BR-FLEET-001 | RRContext structurally carries ClusterID for cross-cluster correlation | SI-4 | P0 | Unit | UT-AF-1409-001 | Passing |
| BR-FLEET-001 | mergeRRContext honors caller-supplied precedence over server-set cluster_id | SI-10 | P0 | Unit | UT-AF-1409-002 | Passing |
| BR-FLEET-001 | CreateRRResult attributes correct cluster provenance on the create path | AU-3 | P0 | Unit | UT-AF-1409-003 | Passing |
| BR-FLEET-001 | CreateRRResult attributes correct cluster provenance on the dedup/reuse path | AU-3 | P1 | Unit | UT-AF-1409-004 | Passing |
| BR-INTEGRATION-065 | Cluster-aware fingerprint enforces information-flow isolation between clusters | AC-4 | P0 | Unit | UT-AF-1409-005 | Passing |
| BR-FLEET-001 | investigation_summary artifact honors caller precedence for cluster_id | SI-10 | P0 | Unit | UT-AF-1409-006 | Passing |
| BR-FLEET-001 | execution_progress payload carries correct cluster attribution | AU-3 | P0 | Unit | UT-AF-1409-007 | Passing |
| BR-FLEET-001 | execution_progress omits cluster_id for local-hub RRs (no false attribution) | AU-3 | P1 | Unit | UT-AF-1409-008 | Passing |
| BR-INTEGRATION-065 | validate.ClusterID enforces length-bounded input validation on LLM-supplied cluster_id | SI-10 | P1 | Unit | UT-AF-1409-009 | Passing |
| BR-AUDIT-005 | emitCreateRRAudit attributes cluster_id in the Detail map for AF-originated fleet RRs | AU-3 | P1 | Unit | UT-AF-1409-010 | Passing |
| BR-FLEET-001 | Takeover path's context-reconstruction decision logic populates full context from a successful fetch | AU-3, CC8.1 | P0 | Unit | UT-AF-1409-011 | Passing |
| BR-FLEET-001 | Takeover path's context-reconstruction decision logic degrades to partial context on fetch failure, without erroring the tool call | SI-17, AU-2 | P1 | Unit | UT-AF-1409-012 | Passing |
| BR-FLEET-001 | kubernaut_investigate_alert correctly attributes and correlates cluster identity end-to-end | AU-3, SI-4 | P0 | Integration | IT-AF-1409-001 | Passing |
| BR-FLEET-001 | kubernaut_remediate correctly attributes and correlates cluster identity end-to-end | AU-3, SI-4 | P0 | Integration | IT-AF-1409-002 | Passing |
| BR-FLEET-001 | kubernaut_investigate (create) correctly attributes and correlates cluster identity end-to-end | AU-3, SI-4 | P0 | Integration | IT-AF-1409-003 | Passing |
| BR-FLEET-001 | Takeover path reconstructs complete remediation context from the RR alone (#1423) | AU-3, CC8.1 | P0 | Integration | IT-AF-1409-004 | Passing |
| BR-FLEET-001 | Takeover path fails safe (degrades, doesn't fail closed) on fetch failure, failure is audited | SI-17, AU-2 | P1 | Integration | IT-AF-1409-005 | Passing |
| BR-INTEGRATION-065 | check_existing_remediation enforces cluster-scoped information-flow isolation in dedup | AC-4 | P0 | Integration | IT-AF-1409-006 | Passing |
| BR-FLEET-001 | execution_progress artifact carries correct cluster attribution end-to-end via watch loop | AU-3, SI-4 | P1 | Integration | IT-AF-1409-007 | Passing |
| BR-INTEGRATION-065 | AF ServiceAccount holds least-privilege (get-only) access for the takeover fetch | AC-6 | P1 | Integration | IT-AF-1409-008 | Passing |
| BR-FLEET-001 | emitDecisionEvent wiring proves cluster_id reaches the investigation_summary DataPart (IT tier, not just E2E) | SI-10, AU-3 | P0 | Integration | IT-AF-1409-009 | Passing |
| BR-INTEGRATION-065 | validate.ClusterID is actually wired into the tool boundary, not dead code | SI-10 | P1 | Integration | IT-AF-1409-010 | Passing |
| BR-FLEET-001 | Cluster identity is reconstructable end-to-end from the SSE-visible artifact alone | AU-3, SI-4, CC8.1 | P0 | E2E | E2E-AF-1409-001 | Passing |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| UT-AF-1409-001 [SI-4] | `RRContext` structurally carries `ClusterID` so downstream status events can support cross-cluster signal correlation, not just single-cluster tracking | Passing |
| UT-AF-1409-002 [SI-10] | `mergeRRContext` merges server-set `cluster_id` into event metadata but never overwrites a caller-supplied value — caller-provided context always takes precedence over merged defaults | Passing |
| UT-AF-1409-003 [AU-3] | `createOrReuseRR`'s create branch attributes the newly created RR's audit-visible cluster identity (`CreateRRResult.ClusterID`) correctly to the cluster the caller specified | Passing |
| UT-AF-1409-004 [AU-3] | `createOrReuseRR`'s existing-RR (dedup) branch attributes cluster identity from the *actual* existing RR object, not the caller's args — preventing a race from misattributing audit provenance to the wrong cluster | Passing |
| UT-AF-1409-005 [AC-4] | Cluster-aware fingerprinting enforces information-flow isolation between clusters: a `cluster_id="a"` lookup never matches state fingerprinted under `cluster_id="b"` for the same namespace/kind/name, preventing remediation-state conflation across cluster boundaries (regression proving R1 is closed) | Passing |
| UT-AF-1409-006 [SI-10] | `emitDecisionEvent` merges `cluster_id` into the `investigation_summary` DataPart from `RRContext`, but never overwrites a caller-supplied `cluster_id` already present in the decision payload — caller precedence enforced at the artifact layer too | Passing |
| UT-AF-1409-007 [AU-3] | `execution_progress` payload carries correct cluster attribution (`cluster_id`) whenever the RR being watched has one, so Console audit/observability views are cluster-attributable | Passing |
| UT-AF-1409-008 [AU-3] | `execution_progress` omits the `cluster_id` key entirely for local-hub RRs, avoiding false cluster attribution (empty-string noise) in audit-visible payloads for single-cluster deployments | Passing |
| UT-AF-1409-009 [SI-10] | `validate.ClusterID` enforces length-bounded input validation on the LLM-supplied `cluster_id`, rejecting oversized values before they reach the RR spec or any downstream event payload | Passing |
| UT-AF-1409-010 [AU-3] | `emitCreateRRAudit` includes `cluster_id` in the audit `Detail` map for both the RR-created and RR-deduplicated events, so AF-originated fleet RRs are cluster-attributable in the audit trail exactly like Gateway-originated ones | Passing |
| UT-AF-1409-011 [AU-3, CC8.1] | `resolveInvestigationRR`'s takeover (`rr_id`-only) branch, driven directly against a fake `client.Client` seeded with a `RemediationRequest`, populates the *complete* context (namespace, kind, target, alert_name, cluster_id) from the fetched object — the pure decision logic, independent of the real MCP/HTTP transport that IT-AF-1409-004 additionally proves is wired | Passing |
| UT-AF-1409-012 [SI-17, AU-2] | `resolveInvestigationRR`'s takeover branch, driven against a fake `client.Client` returning a not-found error, degrades to the partial (`rr_id`-only) context instead of failing the tool call, and logs the fetch failure — the pure decision logic for the fail-safe path IT-AF-1409-005 additionally proves is wired | Passing |

### Tier Skip Rationale

- **Unit tests for `af_investigate_alert.go`/`ka_remediate.go`/`ka_investigate_mcp.go` call-site wiring**: no dedicated UT is written for the `cluster_id` argument -> `CreateRRArgs.ClusterID` -> `SetRRContextSafe` plumbing in these three files. This is a single-field pass-through with no branching logic (zero decision points), so there is no business logic for a UT to exercise beyond what IT-AF-1409-001/002/003 already covers end to end. The wiring is proven at the IT tier instead (Two-Tier Minimum satisfied via IT + the transitively-exercised UT-AF-1409-003/004 for the fields it passes through).
- **Unit tests for `deploy/apifrontend/base/02-rbac.yaml`**: static RBAC YAML has no unit-testable logic; IT-AF-1409-008 is the sole tier for this row (RBAC config assertions are not unit-testable by definition).

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| IT-AF-1409-001 [AU-3, SI-4] | `kubernaut_investigate_alert` with `cluster_id` produces an RR and a status-event stream that are both correctly and consistently attributed to that cluster end-to-end, enabling cross-cluster correlation. Also asserts, via the existing `fakeAuditor` pattern (`pkg/apifrontend/handler/mcp_bridge_integration_test.go`), that the emitted `audit.Event.Detail["cluster_id"]` matches the supplied value — this is the actual IT-tier proof for the audit-Detail-map row (Section 14), replacing the unverified "transitively exercised" claim from the prior revision | Passing |
| IT-AF-1409-002 [AU-3, SI-4] | `kubernaut_remediate` with `cluster_id` produces the same correct end-to-end cluster attribution as IT-AF-1409-001 | Passing |
| IT-AF-1409-003 [AU-3, SI-4] | `kubernaut_investigate`'s create path with `cluster_id` produces the same correct end-to-end cluster attribution as IT-AF-1409-001 | Passing |
| IT-AF-1409-004 [AU-3, CC8.1] | A takeover session (`rr_id` only) reconstructs the *complete* remediation context — namespace, kind, target, alert name, and cluster identity — from the RR alone, proving audit-trail reconstruction is possible without the original creating session (closes the #1423 gap) | Passing |
| IT-AF-1409-005 [SI-17, AU-2] | A takeover session against a deleted/inaccessible RR fails safe: it degrades to a partial (rr_id-only) context instead of failing the whole tool call, and the fetch failure is logged rather than silently swallowed | Passing |
| IT-AF-1409-006 [AC-4] | `kubernaut_check_existing_remediation` enforces cluster-scoped information-flow isolation: a live, non-terminal remediation on one cluster is never reported as covering an identically-named target on a different cluster | Passing |
| IT-AF-1409-007 [AU-3, SI-4] | The watch loop's `execution_progress` artifact is correctly attributed to the live RR's cluster identity end-to-end, not just at the unit level | Passing |
| IT-AF-1409-008 [AC-6] | AF's ServiceAccount holds only `get` (not `list`/`watch`/`delete`) on `remediationrequests` at the RBAC layer, satisfying least-privilege for the new takeover-path fetch | Passing |
| IT-AF-1409-009 [SI-10, AU-3] | `emitDecisionEvent`, driven directly against a real `EventBridge` and a fake `eventqueue.Writer` (no full E2E harness), correctly writes `cluster_id` into the `investigation_summary` DataPart — proving the wiring point itself, independent of the full journey E2E-AF-1409-001 covers | Passing |
| IT-AF-1409-010 [SI-10] | An oversized `cluster_id` (>253 chars) supplied to `kubernaut_investigate_alert` and to `kubernaut_check_existing_remediation` is rejected at the tool boundary before it reaches the RR spec or any fingerprint computation, proving `validate.ClusterID` is actually wired into both validation paths (not dead code) | Passing |

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| E2E-AF-1409-001 [AU-3, SI-4, CC8.1] | A `cluster_id` supplied to `kubernaut_remediate` is fully reconstructable from the SSE-visible `investigation_summary` artifact alone via `RRContext` auto-fill (not LLM-supplied precedence, which UT-AF-1409-006b already covers), proving the complete chain from tool argument to Console-visible, audit-grade artifact (SOC2 CC8.1 reconstruction proof). Verified against a live Kind cluster (`test-e2e-fullpipeline` infra): `Ran 1 of 29 Specs`, `1 Passed \| 0 Failed`. | Passing |

---

## 9. Test Cases (P0 detail)

### UT-AF-1409-005 [AC-4]: Cluster-aware dedup fingerprint enforces information-flow isolation

**BR**: BR-INTEGRATION-065
**Control**: AC-4 (Information flow enforcement, NIST 800-53/FedRAMP)
**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/tools/af_check_existing_rr_test.go`

**Test Steps**:
1. **Given**: two `RemediationRequest`s exist for the same namespace/kind/name, one with
   `spec.clusterID="cluster-a"`, one with `spec.clusterID="cluster-b"`, both non-terminal.
2. **When**: `HandleCheckExistingRR` is called with `args.ClusterID="cluster-a"`.
3. **Then**: it returns the `cluster-a` RR's `rr_id`, never the `cluster-b` RR's.

**Acceptance Criteria**: cluster-aware fingerprint matches only the intended cluster's RR;
omitting `cluster_id` (empty string) preserves today's pre-#1409 single-cluster behavior.

### IT-AF-1409-004 [AU-3, CC8.1]: Takeover path reconstructs complete remediation context

**BR**: BR-FLEET-001
**Control**: AU-3 (Content of audit records); SOC2 CC8.1 (complete reconstruction from traces alone, BR-AUDIT-005)
**Priority**: P0
**Type**: Integration
**File**: `test/integration/apifrontend/ka_investigate_mcp_test.go`

**Test Steps**:
1. **Given**: a `RemediationRequest` exists with `spec.clusterID="cluster-a"`,
   `spec.targetResource={Namespace: "ns1", Kind: "Deployment", Name: "app1"}`, non-terminal.
2. **When**: `kubernaut_investigate` is called with only `rr_id` set (no namespace/kind/name).
3. **Then**: the resulting `EventBridge.RRContext` has `Namespace="ns1"`, `Kind="Deployment"`,
   `Target="app1"`, `ClusterID="cluster-a"` — not just `RRID`.

**Acceptance Criteria**: proves the #1423 Console-banner gap on takeover sessions is closed.

---

## 10. Environmental Needs

- **Framework**: Ginkgo/Gomega BDD (mandatory), matching existing `pkg/apifrontend/tools`
  and `pkg/apifrontend/launcher` suites.
- **Unit**: no external dependencies; `fake.NewClientBuilder()` for any K8s object access.
- **Integration**: `fake.NewClientBuilder()`-backed `client.Client` (existing AF IT
  convention — real business logic, faked K8s API per Mock Strategy in AGENTS.md).
- **E2E**: existing `test/e2e/fullpipeline` harness (already exercises `investigate_alert`
  through a real AF process; extend, don't duplicate).

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None. ADR-065's CRD schema and Gateway-side wiring are already merged.

### 11.2 Execution Order (per-component RED->GREEN cycles, per AGENTS.md's Wiring-First TDD Sequence)

> **Sequencing correction**: an earlier revision of this section described one
> "big-bang" RED phase (write all 20 tests up front) followed by one "big-bang" GREEN
> phase (implement everything). That does not match AGENTS.md's actual Wiring-First TDD
> Sequence, which pairs RED and GREEN *per component/wiring-point*:
>
> ```
> RED: Write IT test calling component through production entry point -> fails
>      Write UT test for component logic -> fails
> GREEN: Wire component in production code -> IT passes
>        Implement component logic -> UT passes
> REFACTOR: Clean up
> ```
>
> REFACTOR is a single shared step at the end, not per-component, because the
> triplicated `RRContext`-from-`CreateRRResult` construction it removes only becomes
> visible once all three create-path call sites already exist from earlier cycles.
> CHECKPOINT W gates the *exit* of the last RED->GREEN cycle — per AGENTS.md, "GREEN is
> NOT complete until both UT and IT pass" — it is not a step performed after REFACTOR.
> REFACTOR only starts once every IT below is green. A second, lightweight re-run of the
> full suite after REFACTOR (Post-Refactor Validation) confirms REFACTOR didn't regress
> the wiring — it is a safety-net check, not the first proof of wiring.

1. **Phase 1 (RED->GREEN cycles)** — ~6h, 12 cycles in dependency order: for each
   component below, write its failing UT/IT first, confirm it fails against current
   code, then write the minimal implementation to turn it green before moving to the
   next component. No sophisticated logic in these cycles — minimal field additions and
   wiring only.
   1. `RRContext.ClusterID` + `mergeRRContext` — UT-AF-1409-001/002 (`event_bridge.go`)
   2. `CreateRRResult.ClusterID` populated on create/dedup branches — UT-AF-1409-003/004 (`af_create_rr.go`)
   3. `cluster_id` in both `emitCreateRRAudit` `Detail` maps — UT-AF-1409-010 (`af_create_rr.go`; missing entirely from the initial draft — code with no RED test)
   4. `CheckExistingRRArgs/Result.ClusterID` + `rrFingerprintWithCluster` switch — UT-AF-1409-005, IT-AF-1409-006 (`af_check_existing_rr.go`)
   5. `cluster_id` arg on `InvestigateAlertArgs`, wired to `CreateRRArgs`/`SetRRContextSafe` — IT-AF-1409-001 (`af_investigate_alert.go`)
   6. `cluster_id` arg on `RemediateArgs`, same wiring — IT-AF-1409-002 (`ka_remediate.go`)
   7. `cluster_id` arg on `InvestigateMCPArgs`, create-path wiring — IT-AF-1409-003 (`ka_investigate_mcp.go`)
   8. Takeover `rr_id`-only branch: `client.Get` + graceful degradation, full `RRContext` — IT-AF-1409-004/005 (`ka_investigate_mcp.go`)
   9. `cluster_id` merged into `investigation_summary` via `emitDecisionEvent` + new `EventBridge` accessor — UT-AF-1409-006, IT-AF-1409-009 (`part_converter.go`; previously skipped straight from UT to E2E, violating the Pyramid Invariant)
   10. `BuildProgressSnapshot` signature extension + `crd_tools_watch.go` call site — UT-AF-1409-007/008, IT-AF-1409-007 (`execution_progress.go`)
   11. `validate.ClusterID` (optional, `MaxLength=253`) wired into `validateCreateRRArgs` and `HandleCheckExistingRR` — UT-AF-1409-009 (unit), IT-AF-1409-010 (proves enforcement at the tool boundary, not just unit-tested in isolation)
   12. RBAC regression pin (expected to pass immediately — locks current state, not new GREEN code) — IT-AF-1409-008

   No new `cmd/` wiring is required for any of these — all three tools are already
   registered in `cmd/apifrontend`; this extends existing registered tools' argument
   schemas.
2. **CHECKPOINT W (GREEN exit gate)** — ~0.25h: after all 12 cycles, confirm every row
   in the Section 14 Wiring Manifest has a passing IT exercising the real
   tool-to-CRD-to-EventBridge path — GREEN is not declared done until this is true,
   including IT-AF-1409-009 (decision artifact) and IT-AF-1409-010 (validation wiring),
   not just the UTs for those two rows.
3. **Phase 3 (REFACTOR)** — ~1.5h: extract a shared "populate RRContext from
   CreateRRResult" helper used by the three call sites (`af_investigate_alert.go`,
   `ka_remediate.go`, `ka_investigate_mcp.go`) to avoid triplicated `SetRRContextSafe`
   construction; apply Go Anti-Pattern Checklist. **Post-Refactor Validation (mandatory
   precondition for calling REFACTOR complete)**: `go build ./...`, re-run every UT/IT
   from Phase 1 and confirm all still pass, `grep` for any stale field/type names left
   behind by the extraction.
4. **Phase 4 (Wiring re-confirmation)** — ~0.5h: re-check the Section 14 manifest after
   REFACTOR to confirm the extraction didn't silently break a wiring path (this is a
   safety-net re-check; the first proof of wiring already happened at CHECKPOINT W above).
5. **Phase 5 (E2E)** — ~1h: extend `test/e2e/fullpipeline` with E2E-AF-1409-001 — the
   full-journey confirmation, distinct from (and downstream of) IT-AF-1409-009's
   wiring-level proof.

**Total estimate**: ~9.5h (~1.2 dev days) — unchanged from the prior revision; this is a
sequencing fix, not a scope change.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|--------------|
| This test plan | `docs/tests/1409/TEST_PLAN.md` | Strategy and test design |
| Unit tests | `pkg/apifrontend/launcher/event_bridge_test.go`, `pkg/apifrontend/tools/af_create_rr_test.go`, `af_check_existing_rr_test.go`, `execution_progress_test.go`, `part_converter_test.go`, `validate/k8s_test.go` | Ginkgo BDD specs |
| Integration tests | `test/integration/apifrontend/*_test.go` | Ginkgo BDD specs |
| E2E test | `test/e2e/fullpipeline/*_test.go` | Ginkgo BDD spec |
| ADR-065 update | `docs/architecture/decisions/ADR-065-fleet-cluster-identity-on-rr.md` | Replace `IT-AF-FLEET (pending)` row with concrete `IT-AF-1409-001..008` |

---

## 13. Execution

```bash
# Unit tests
go test ./pkg/apifrontend/launcher/... ./pkg/apifrontend/tools/... ./pkg/apifrontend/validate/... -ginkgo.v

# Integration tests
go test ./test/integration/apifrontend/... -ginkgo.v -ginkgo.focus="1409"

# E2E
go test ./test/e2e/fullpipeline/... -ginkgo.focus="1409"

# Coverage
go test ./pkg/apifrontend/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Wiring Verification (checked at CHECKPOINT W / GREEN exit; re-confirmed at Phase 4 post-REFACTOR)

| Code Path | Entry Point | Exit Point | Control | Wiring IT | Status |
|-----------|-------------|------------|---------|-----------|--------|
| `cluster_id` arg -> `CreateRRArgs.ClusterID` -> `buildRRObject` | `kubernaut_investigate_alert` | RR `spec.clusterID` | AU-3, SI-4 | IT-AF-1409-001 | Passing |
| `cluster_id` arg -> `CreateRRArgs.ClusterID` -> `buildRRObject` | `kubernaut_remediate` | RR `spec.clusterID` | AU-3, SI-4 | IT-AF-1409-002 | Passing |
| `cluster_id` arg -> `CreateRRArgs.ClusterID` -> `buildRRObject` | `kubernaut_investigate` (create) | RR `spec.clusterID` | AU-3, SI-4 | IT-AF-1409-003 | Passing |
| `CreateRRResult.ClusterID` -> `SetRRContextSafe` | all three create-path tools | `EventBridge.RRContext` | AU-3, SI-4 | IT-AF-1409-001/002/003 | Passing |
| `rr_id`-only takeover (success) -> `client.Get` -> full `RRContext` | `kubernaut_investigate` (takeover) | `EventBridge.RRContext` | AU-3, CC8.1 | IT-AF-1409-004 | Passing |
| `rr_id`-only takeover (fetch failure) -> graceful degradation | `kubernaut_investigate` (takeover) | partial `RRContext` + logged failure | SI-17, AU-2 | IT-AF-1409-005 | Passing |
| `RRContext.ClusterID` -> `emitDecisionEvent` | `kubernaut_present_decision` FunctionCall | `investigation_summary` DataPart (SSE) | SI-10, AU-3 | IT-AF-1409-009 (wiring), E2E-AF-1409-001 (journey) | Passing |
| RR `spec.clusterID` -> `BuildProgressSnapshot` | `handleRREvent` watch loop | `execution_progress` DataPart (SSE) | AU-3, SI-4 | IT-AF-1409-007 | Passing |
| `cluster_id` arg -> cluster-aware fingerprint | `kubernaut_check_existing_remediation` | dedup lookup result | AC-4 | IT-AF-1409-006 | Passing |
| ClusterRole `get` on `remediationrequests` | AF ServiceAccount | takeover `client.Get` call | AC-6 | IT-AF-1409-008 | Passing |
| `args.ClusterID` -> `validate.ClusterID` | `validateCreateRRArgs`, `HandleCheckExistingRR` | rejected/accepted before RR spec or fingerprint | SI-10 | IT-AF-1409-010 | Passing |
| `args.ClusterID` -> `emitCreateRRAudit` Detail map | `createOrReuseRR` (both branches) | `audit.Event.Detail["cluster_id"]` | AU-3 | IT-AF-1409-001 (extended to assert via `fakeAuditor`; see Section 8 note — previously only UT-AF-1409-010, which was an unverified "transitively exercised" claim found and closed on audit) | Passing |

**Unit tests do NOT count as wiring proof.** All 12 rows above are proven by integration
or E2E tests that traverse the real tool-handler-to-CRD-to-EventBridge stack; UT-AF-1409-010
still exists as the unit-level proof of the `Detail` map construction logic itself, but the
row above is no longer proven by UT alone.

**Note on `mergeRRContext`'s own wiring (not a manifest row)**: `mergeRRContext`'s only
production callers are `emitStatusEventWithMeta` and `emitKeepaliveEvent`
(`event_bridge.go:244,291`), both pre-existing and already exercised by AF's existing status-event
test suites. Adding `ClusterID` to its field map is additive to an already-proven wiring path, not
a new wiring point, so UT-AF-1409-001/002 alone is sufficient here without a dedicated new IT.

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|---------------------|---------------------|-------------------|--------|
| `af_check_existing_rr_test.go` (existing cases) | Calls `HandleCheckExistingRR` without `ClusterID` | None — `ClusterID` is `omitempty`; empty string preserves `rrFingerprint("", ...)` equivalence | Backward compatibility, not a behavior change |
| `docs/architecture/decisions/ADR-065-fleet-cluster-identity-on-rr.md` Wiring Manifest (line 119) | `IT-AF-FLEET (pending)` | Replace with `IT-AF-1409-001` (primary AF RR-creation wiring proof) | This plan implements the row ADR-065 reserved |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-07-08 | Initial test plan |
| 1.1 | 2026-07-08 | Added Section 1.4 FedRAMP/SOC2 Control Mapping table; rewrote every test scenario (Sections 7, 8, 9, 14) to carry a bracketed control tag and a business-level compliance description instead of a technical-implementation description, per project methodology (mandatory for all test plans and implementation plans, not just this one) |
| 1.2 | 2026-07-08 | Closed three Pyramid Invariant / TDD gaps found on audit: (1) added UT-AF-1409-010 for the previously-untested `emitCreateRRAudit` Detail-map change; (2) added IT-AF-1409-009 so `emitDecisionEvent` wiring is proven at the IT tier instead of only via E2E; (3) added IT-AF-1409-010 and wired `validate.ClusterID` into `validateCreateRRArgs`/`HandleCheckExistingRR` so it isn't dead code; (4) documented the Tier Skip Rationale for the three call-site tools' UT skip; (5) corrected Section 11.2 so CHECKPOINT W gates GREEN's exit, not a step after REFACTOR |
| 1.3 | 2026-07-08 | Rewrote Section 11.2: replaced the "big-bang" RED-all-then-GREEN-all sequencing with AGENTS.md's actual Wiring-First TDD Sequence (RED->GREEN paired per component, 12 cycles in dependency order, single shared REFACTOR at the end since the triplicated-helper duplication is only visible once all three create-path call sites exist). Retitled Section 14 to clarify the Wiring Manifest is checked twice — first at CHECKPOINT W (GREEN's exit gate), then re-confirmed at the post-REFACTOR phase — not solely "TDD Phase 4" as previously mislabeled. The implementation plan's todos were restructured the same way. |
| 1.4 | 2026-07-08 | Pyramid Invariant re-audit against the real codebase (not just this plan's own claims) found two gaps: (1) the audit-Detail-map manifest row claimed "transitively exercised by IT-AF-1409-001/002/003" but none of those IT descriptions actually asserted on the audit trail — confirmed a `fakeAuditor` capture pattern already exists in `pkg/apifrontend/handler/mcp_bridge_integration_test.go`, so extended IT-AF-1409-001's scope to assert `audit.Event.Detail["cluster_id"]`, closing the row's contradiction with this section's own "unit tests do NOT count as wiring proof" rule; (2) the Wiring Manifest was missing a dedicated row for IT-AF-1409-005 (the fail-safe/degradation branch, distinct from IT-AF-1409-004's happy path) even though it's a listed P1 test — added it. Manifest is now 12 rows, all IT/E2E-proven. Also documented (as a non-gap, verified via grep of `mergeRRContext`'s actual call sites) why `mergeRRContext`'s own field addition doesn't need a new dedicated IT: its callers are pre-existing, already-wired production code. |
| 1.5 | 2026-07-08 | Follow-up Pyramid Invariant audit (triggered by a direct "do you guarantee the pyramid invariant" question) cross-checked all 12 Wiring Manifest rows against Sections 6-9 line by line. Found one documentation inconsistency: `resolveInvestigationRR`'s takeover-fetch rows (IT-AF-1409-004/005) have no UT and, unlike the other two skipped-UT rows (call-site wiring, RBAC YAML), had no documented Tier Skip Rationale explaining why. Added a third Tier Skip Rationale bullet (Section 8) explaining the skip is consistent with the project's Mock Strategy — `client.Get()` against a `RemediationRequest` is Kubernetes-API-backed and therefore fake-client-driven (Integration tier) by definition, with no separable pure-logic branch to extract into a UT without an artificial split. No coverage gap was found (all 12 rows remain IT/E2E-proven); this closes a documentation-completeness gap only. |
| 1.6 | 2026-07-08 | Corrected v1.5's conclusion after re-recall confirmed a stronger, no-exceptions project rule: "100% business logic unit test coverage for all implementations, with no exceptions" — and that this codebase already treats direct-handler-call tests with fake/mock dependencies (including a fake `client.Client`) as legitimate Unit tests, reserving Integration for tests that go through the real MCP SDK/HTTP transport (evidenced by `ka_investigate_mcp_test.go`'s existing `UT-AF-1326-*` cases, which call `HandleInvestigationMCP` directly with a mock MCP client). Reverted the v1.5 Tier Skip Rationale bullet and added UT-AF-1409-011 (success reconstruction) and UT-AF-1409-012 (fail-safe degradation) as genuine Unit tests of `resolveInvestigationRR`'s takeover-fetch decision logic, driven directly (bypassing MCP transport) against a fake `client.Client`. IT-AF-1409-004/005 remain unchanged as the IT-tier wiring proof through the real registered-tool dispatch path. Added a new `RRContext()`/`RRContextSafe` accessor pair (Section 6.1) needed for these UTs (and reused internally by `emitDecisionEvent`) to read back the bridge's context from outside `pkg/apifrontend/launcher`. |
| 1.7 | 2026-07-09 | CHECKPOINT W executed against the real codebase (not just this plan's claims): found that IT-AF-1409-001/002/003/004/005/006 were documented as the Wiring Manifest's proof rows but had never actually been written — only their UT-tier siblings (UT-AF-1409-003/004/005/005b/005c/011/012/013) existed. Closed the gap by adding all six as new `It` specs driven through the real registered-tool dispatch path (`HandleInvestigateAlert`, `HandleRemediate`, `HandleInvestigationMCPWithRegistry` with a mock MCP client, `HandleCheckExistingRR`), asserting on the created RR's `spec.clusterID`, the `audit.Event.Detail["cluster_id"]` via the existing `auditRecorder`, and the A2A status-event metadata via the existing `bridgeQueue` pattern. All 620+ specs in `pkg/apifrontend/tools` and the full `pkg/apifrontend/...` module pass with zero regressions. All 12 Wiring Manifest rows (Section 14) now show `Passing` except the one row whose E2E leg (`E2E-AF-1409-001`) is intentionally deferred to the E2E phase. Also added `IT-AF-1409-008` to `test/infrastructure/rbac_parity_test.go` (Cycle 12), regression-pinning the pre-existing `get` grant on `remediationrequests`. Sections 7 and 8 updated from `Pending` to `Passing` for every test ID now implemented and green. |
| 1.8 | 2026-07-09 | E2E-AF-1409-001 written and run against a live Kind cluster (user explicitly asked "why not live cluster to test? can't you spawn a kind cluster?" — answer: yes, and it surfaced two real bugs). RCA via must-gather on two failed live runs found: (1) a genuine infra issue (`no space left on device` in the Podman VM — resolved via `podman system prune -af --volumes`, unrelated to this change); (2) a pre-existing, unrelated latent bug in `test/services/mock-llm`'s `NextToolCall` chaining (`handlers/gemini.go`): unlike the sibling `RepeatToolCall` path, `NextToolCall` had no "fire once" guard, so `match_last_only` kept re-matching the same scenario and the mock-LLM re-emitted the chained tool call on every ADK reasoning round-trip — an unbounded loop (confirmed in must-gather: same scenario firing every ~0.5s for 60+ seconds). Fixed at the source by adding `response.HasFunctionResponseNamed(contents, name)` (scans *all* conversation history, not just the last turn, unlike the existing `LastContentIsFunctionResponse`) and gating `NextToolCall` on it in `handlers/gemini.go`; this is a mock-LLM infrastructure fix with no production-code impact, verified against the existing `UT-ML-1407-*` regression suite (all still green) before re-running the E2E test. A second must-gather cycle (after the loop fix) found a third, narrower bug: the mock scenario's chained `kubernaut_present_decision` call was (a) incorrectly passing `cluster_id` as a tool argument — `kubernaut_present_decision`'s schema has no such property, so ADK's schema validation rejected the call before its handler ever ran — and (b) missing the schema-required `session_id` field. Both defeated the very RRContext-auto-fill path the test targets. Fixed by removing `cluster_id` from the scenario's `kubernaut_present_decision` arguments (letting `emitDecisionEvent`'s auto-fill populate it from `RRContext`, per cycle 9) and adding a `session_id` placeholder value (uninspected by `HandlePresentDecision`, only schema-validated). Confidence gate: preflight (root-caused via must-gather logs, not guesswork) plus a full mock-llm regression run (`go test ./test/services/mock-llm/...`, all green including OpenAI's unaffected, already-guarded `NextToolCall` path) raised confidence above 90% before the fix was applied, per the monitoring protocol. E2E-AF-1409-001 now passes against a live Kind cluster: `Ran 1 of 29 Specs in 1000.537 seconds` / `1 Passed \| 0 Failed \| 0 Pending \| 28 Skipped`. Sections 7, 8, and 14 updated from `Pending`/`IT Passing / E2E Pending` to `Passing`. ADR-065's Wiring Manifest and Test Plan Reference updated to close out the `IT-AF-FLEET (pending)` placeholder row and reference the full #1409 pyramid. |
