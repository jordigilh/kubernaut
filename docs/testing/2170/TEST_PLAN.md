# Test Plan: AgentSession CRD Redesign — AA↔KA HTTP Removal

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2170-v1.0
**Feature**: Replace AA↔KA's HTTP submit/poll/result channel with a single, KA-exclusively-written
`AgentSession` CRD watched by AA and AF (`DD-AA-KA-001`, `BR-AA-KA-065`)
**Version**: 1.0
**Created**: 2026-08-18
**Status**: Active (production/UT/IT merged in PR #2189; `pkg/agentclient` + KA HTTP server
removal and the 28-file E2E trigger redesign deferred to follow-up issue #2190)
**Branch**: `feature/2170-agentsession-crd`

---

## 1. Introduction

### 1.1 Purpose

[BR-AA-KA-064](../../requirements/BR-AA-KA-064-session-based-pull-design.md)'s async HTTP
submit/poll design was the confirmed root cause of
[#2080](https://github.com/jordigilh/kubernaut/issues/2080)/
[#2081](https://github.com/jordigilh/kubernaut/issues/2081) (KA's per-session ownership
authorization 404s AA's poll once AF has taken over/correlated a different session, driving the
5-regeneration cap to a permanent `Failed` outcome) and shared its "re-derive a fact from a
secondhand signal" shape with the interactive-detection dual-signal gap behind
[#1713](https://github.com/jordigilh/kubernaut/issues/1713) (AA's `InvestigationSession`-existence
inference racing a concurrent HTTP submit). This test plan documents the finalized test coverage
for [BR-AA-KA-065](../../requirements/BR-AA-KA-065-agentsession-watch-design.md) /
[DD-AA-KA-001](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md), which
replaces both mechanisms with a single `AgentSession` CRD that KA exclusively writes and AA/AF
watch — closing #2080/#2081/#1713 by construction (the failure modes' preconditions no longer
exist in the code), not by tuning caps, backoffs, or timeouts.

This is a **post-implementation, finalized** test plan: all Test IDs below reference tests that
already exist and pass on the feature branch (`go build ./...` clean;
`go test ./pkg/aianalysis/... ./internal/controller/aianalysis/... ./internal/kubernautagent/...
./pkg/apifrontend/tools/...` green as of 2026-08-18). Its purpose is traceability (BR → Test ID →
control objective) for the compliance record, and an honest accounting of what remains deferred.

### 1.2 Objectives

1. **Dispatch/result channel replacement**: Every `BR-AA-KA-065.1`–`.6` sub-requirement (the
   `AgentSession` create/dispatch/status/interactive-detection contract) has passing Unit and
   Integration coverage exercising the real production entry points.
2. **Root-cause closure proven, not just asserted**: #2080/#2081's regeneration-cap failure mode
   and #1713's interactive-detection race each have a named regression test whose passing depends
   on the offending code path no longer existing (`handleSessionLost`, `tryAdoptCorrelatedSession`,
   `checkISMismatchAndCancel`, `HasActiveSession`, `CorrelatedSessionID` — confirmed zero remaining
   references repo-wide).
3. **Three implementation-time gaps closed**: DD-AA-KA-001's 2026-08-17 Amendment (Fresh Interactive
   Start, Terminal-Status-Write Race, KA's MCP-direct Interactive Fallback) each have dedicated
   Unit/Integration coverage, not just a design-doc note.
4. **RBAC least privilege proven, not asserted**: `BR-AA-KA-065.9`/`.10`'s RBAC narrowing
   (AA's `InvestigationSession` read removal, KA's new scoped `AgentSession`/`InvestigationSession`
   grants) has Helm-chart-level test coverage, not just a manual diff review.
5. **Deferred scope is explicit**: `BR-AA-KA-065.8` (HTTP channel removal) is honestly reported as
   **not yet met** — `pkg/agentclient` and KA's HTTP server remain load-bearing for the
   still-HTTP-based 28-file E2E suite — tracked in
   [#2190](https://github.com/jordigilh/kubernaut/issues/2190), not silently dropped from the record.

### 1.3 Success Metrics

| Metric | Target | Measurement | Result |
|--------|--------|-------------|--------|
| Unit test pass rate (touched packages) | 100% | `go test ./pkg/aianalysis/... ./internal/controller/aianalysis/... ./internal/kubernautagent/... ./pkg/apifrontend/tools/...` | **Pass** (2026-08-18) |
| `go build ./...` | Clean | `go build ./...` | **Pass** (2026-08-18) |
| BR-AA-KA-065.1–.7, .9–.12 | 100% have ≥1 passing UT + structural/IT proof | This document's §7 BR Coverage Matrix | **Pass** |
| BR-AA-KA-065.8 (HTTP removal) | `pkg/agentclient` deleted, `go build ./...` clean | Deferred | **Deferred (#2190)** — production dead code (`BuildIncidentRequest`) and the direct-HTTP integration test removed in #2189; the client package, KA's HTTP server, and 28 E2E files remain |
| Backward compatibility | 0 unexplained regressions | Existing suites pass; each removed/replaced test has a documented rationale (§15) | **Pass** |

---

## 2. References

### 2.1 Authority (governing documents)

- [BR-AA-KA-065](../../requirements/BR-AA-KA-065-agentsession-watch-design.md): AgentSession CRD Watch Design for AA-KA-AF Communication (supersedes BR-AA-KA-064)
- [DD-AA-KA-001](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md): Full design, alternatives considered, 2026-08-17 Amendment (3 implementation gaps)
- Issue [#2170](https://github.com/jordigilh/kubernaut/issues/2170): AgentSession CRD + dispatcher (this plan's namesake)
- Issue [#2171](https://github.com/jordigilh/kubernaut/issues/2171): AA-side `AgentSessionGetOrCreator`/handler migration
- Issue [#2172](https://github.com/jordigilh/kubernaut/issues/2172): AF ack-wait migration to `AwaitAgentSessionInteractive`
- Issue [#2080](https://github.com/jordigilh/kubernaut/issues/2080) / [#2081](https://github.com/jordigilh/kubernaut/issues/2081): regeneration-cap root cause this design eliminates
- Issue [#1713](https://github.com/jordigilh/kubernaut/issues/1713): interactive-detection race this design eliminates
- Issue [#2190](https://github.com/jordigilh/kubernaut/issues/2190): follow-up — `pkg/agentclient`/KA HTTP server removal + 28-file E2E trigger redesign (deferred scope from this plan)

### 2.2 Cross-References

- [DD-AUDIT-003](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md): AI Analysis Controller event catalog (updated alongside this plan — §2 below)
- [Test Plan Template](../TEST_PLAN_TEMPLATE.md)
- Related, narrower plans already closed under this same design: [docs/testing/2080/TEST_PLAN.md](../2080/TEST_PLAN.md) (durable backoff — superseded in spirit; regeneration path deleted entirely)

### 2.3 FedRAMP/SOC2 Control-Objective → Test-ID Mapping

> Per `AGENTS.md`'s "How Coverage Is Measured" — Integration/E2E coverage is requirements-based
> (control objective assessed by ≥1 proving test), not line-percentage. This table is the
> control-objective assessment record for this feature.

| Control | Objective | Kubernaut Application in This Feature | Proving Test ID(s) | Status |
|---------|-----------|----------------------------------------|---------------------|--------|
| **SI-4** | System Monitoring | `AgentSessionEventPredicate` ensures AA's reconciler is woken immediately on any KA-written `Interactive`/`SessionID` change, not just terminal `Phase` — no monitoring blind spot between KA's dispatch-accept and AA's next scheduled poll | UT-AA-2030-013, UT-AA-2030-013a | **Pass** |
| **SI-10** | Information Input Validation | `AgentSession.Status.Result` is a curated subset (same SI-10 boundary as `MarshalRCASubset`) — internal workflow/validation/alignment state is never exposed across the AA↔KA trust boundary via Status | UT-AA-KA-065-010 (curated `InvestigationResult` mapping), UT-AA-KA-065-021 (curated `Failed` status, not raw error) | **Pass** |
| **SI-11** | Error Handling | AF's ack-wait surfaces a clear, user-visible timeout — never a silent hang — even when no `AgentSession` exists yet; KA's interactive fallback fails closed with an actionable error rather than a directionless placeholder | UT-AF-AS-WATCH-002, -003, -009; UT-KA-1440-010–012, UT-KA-1818-004; IT-KA-1440-010 | **Pass** |
| **AC-4** | Information Flow Enforcement | AA↔KA dispatch/result flow moves from an ad hoc HTTP call to a single, schema-validated, RBAC-governed CRD — the flow is now enforced by the K8s API server's own admission/RBAC layer, not application-level trust | UT-KA-HELM-002/003/004 (RBAC scoping); structural (CRD schema in `api/agentsession/v1alpha1/`) | **Pass** |
| **AC-6** | Least Privilege | AA's `InvestigationSession` read (Get/List/Watch) grant removed once AA no longer needs it for decisions (narrow write-only grant survives for terminal-phase bookkeeping); KA's `AgentSession` grant scoped to get/list/watch + status-subresource only, never full object write; AA's now-unused `kubernaut-agent-client` RoleBinding removed | UT-KA-HELM-002/003/004; `config/rbac/role.yaml` diff; `charts/kubernaut/templates/aianalysis/aianalysis.yaml` RoleBinding removal | **Pass** |
| **AU-2 / AU-3** | Audit Events / Content of Audit Records | `aianalysis.aiagent.submit`/`.result` events continue to fire at the `AgentSession` Create/Status-read moments (repurposed transport, same audit semantics); `aianalysis.aiagent.session_lost` is retired (zero production callers — its sole trigger, `handleSessionLost`, is deleted) | Structural (grep-confirmed call sites in `investigating.go`); DD-AUDIT-003 event-catalog update (this plan, §2.1) | **Pass** (submit/result); **Retired, not repurposed** (session_lost) |
| **CC7.2** (SOC2) | Monitoring / audit-trail reconstruction | A `correlation_id` query still reconstructs the full signal→analysis→dispatch→result lifecycle; the transport change (HTTP→CRD) does not remove or rename any event in the reconstruction chain | Existing SOC2 reconstruction tests (`audit_flow_integration_test.go`) — unaffected, not re-verified in this pass since no reconstruction-relevant event was removed | **Pass** (no regression) |
| **CC8.1** (SOC2) | Change Management — this project's own authorization/design/test/review workflow for changes to audit-emitting code (not the audit events themselves, per AGENTS.md's 2026-08-17 correction) | This feature followed RED→GREEN→REFACTOR, a documented DD (`DD-AA-KA-001`) with user-approved alternatives, and a scoped PR split (#2189 now / #2190 deferred) rather than an unreviewed direct change | PR #2189 review + this test plan itself as the documentation artifact | **Pass** |

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `AgentSession.Spec` translation loses content the retired `IncidentRequest` HTTP body carried | KA investigates with an incomplete signal picture; silent quality regression | Medium | UT-AA-KA-065-001/002/010–015, UT-AA-KA-065-101–116 | Field-by-field 1:1 mapping tests on both the AA-encode and KA-decode sides, run against the exact same `AIAnalysis.Spec.AnalysisRequest` source the retired `BuildIncidentRequest` read |
| R2 | Two KA replicas double-dispatch the same `AgentSession` | Duplicate LLM cost, conflicting results written to Status | Low | UT-AA-KA-065-022 | Direct concurrency test: two `Dispatcher` instances racing the same object, asserting the underlying investigation runs exactly once |
| R3 | A concurrent `AgentSession.Status` write (CAS conflict) is silently dropped | KA's dispatch-accept/complete/fail transition disappears; AA hangs waiting on a Status field that was never actually written | Medium | UT-AA-KA-065-021, UT-KA-2170-001–006 (indirect: proves the *hook* fires exactly once) | **Coverage gap identified, not closed** — see §15/Known Gaps: no test currently injects a genuine `apierrors.IsConflict` from a fake/envtest client against `status_writer.go`'s `updateStatus` retry loop. The 3-attempt bound (`maxStatusUpdateRetries`) is implemented but its retry-then-give-up behavior under a forced conflict is unverified by any automated test |
| R4 | AA/AF infer `Interactive` from a secondhand signal (IS existence) instead of watching `Status.Interactive`, reintroducing the #1713 race | Interactive upgrade ack silently missed or delayed by a full poll interval | Low (structurally closed — the inferring code is deleted) | UT-AA-2030-013a, UT-AF-AS-WATCH-001–010, `E2E-FP-1390-001` | `AgentSessionEventPredicate` wakes AA immediately on `Interactive`-only changes; AF's `AwaitAgentSessionInteractive` watches `AgentSessionList` directly, never `InvestigationSession.Status.Phase` |
| R5 | KA's MCP-direct interactive fallback creates a directionless placeholder session (Gap 3) | Session has no `AgentSession` to write a result to, no path to AA's escalation pipeline — silent dead end | Low | UT-KA-1440-010–012, UT-KA-1818-001–004, IT-KA-1440-010 | `reattachOrCreateFallback` fails closed with `ErrCodeNoInvestigationAvailable` when no real investigation exists; proven at both UT (unit) and IT (production MCP dispatch path) tiers |
| R6 | `BR-AA-KA-065.8` (HTTP removal) silently reported as done when it isn't | Compliance record overstates actual removal; a future auditor finds `pkg/agentclient` still present and questions the record's accuracy | Low (mitigated by this plan itself) | N/A — process risk | Explicitly tracked as **Deferred (#2190)** in §1.3 and §7, not marked Pass |

### 3.1 Risk-to-Test Traceability

R1–R6 above have their mitigating tests listed inline. **R3 is the one identified risk with no
closing test** — flagged here rather than glossed over, per the project's "ground changes in facts,
don't paper over gaps" directive. Recommendation: a follow-up UT using a fake client wrapped to
return `apierrors.NewConflict(...)` on the first N `Status().Update()` calls, asserting the retry
loop eventually succeeds (or, for the exhausted case, that it logs and returns without panicking).
Not implemented in this pass — filed as a coverage gap, not silently closed.

---

## 4. Scope

### 4.1 Features to be Tested

- **`internal/kubernautagent/agentsession/`** (`mapping.go`, `dispatcher.go`, `status_writer.go`):
  KA's dispatch watcher — Lease-based exactly-once dispatch, Spec→SignalContext mapping,
  curated Status writing, dispatch-time Interactive detection (Gap 1).
- **`pkg/aianalysis/creator/agentsession.go`** + **`pkg/aianalysis/handlers/request_builder.go`**:
  AA's `AgentSessionGetOrCreator` — idempotent Create, lossless Spec population.
  request_builder.go itself now only builds `AgentSessionSpec` after `BuildIncidentRequest` and
  its helpers were deleted as dead code once the HTTP channel had no consumer for their output.
- **`pkg/aianalysis/handlers/investigating.go`**: AA's `InvestigatingHandler` — watch-driven Status
  consumption, audit event emission, `syncKASessionStatus`.
- **`internal/controller/aianalysis/aianalysis_controller.go`**: `AgentSessionEventPredicate` —
  immediate reconcile on `Interactive`/`SessionID` changes (not just terminal `Phase`).
- **`internal/kubernautagent/session/manager.go`, `manager_interactive.go`, `manager_query.go`**:
  `TerminalHook`/`InteractiveUpgradeHook` — single-commit-point terminal/upgrade notification
  (Gap 2).
- **`internal/kubernautagent/mcp/tools/investigate_start.go`, `investigate_autonomous.go`**:
  `reattachOrCreateFallback` fail-closed behavior (Gap 3).
- **`pkg/apifrontend/tools/crd_tools_session.go`**: `AwaitAgentSessionInteractive` — AF's
  `AgentSessionList`-watch ack-wait, replacing `AwaitISPhaseActive`.
- **RBAC**: `charts/kubernaut/templates/aianalysis/aianalysis.yaml`,
  `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml`,
  `charts/kubernaut/templates/apifrontend/apifrontend.yaml`, `config/rbac/role.yaml`.

### 4.2 Features Not to be Tested (this plan)

- **`pkg/agentclient` removal / KA's HTTP server removal**: deferred to
  [#2190](https://github.com/jordigilh/kubernaut/issues/2190) — still load-bearing for the 28-file
  `test/e2e/` suite, which needs its own trigger-mechanism redesign (submit via `AgentSession`
  Create instead of an HTTP POST), not just a delete. Production dead code reachable *only* through
  the retired HTTP path (`BuildIncidentRequest`, its test file, and the direct-HTTP
  `agentclient_integration_test.go`) **was** removed in this pass — see §15.
- **KA pod-restart / Lease-reclaim under real container termination**: the Lease-reclaim *logic*
  (`UT-AA-KA-065-023`) is unit-tested against a simulated stale Lease; a live pod-kill E2E scenario
  is a separate, larger-scope decision tracked as a pending item in this feature's parent task list,
  not resolved in this document.
- **`InvestigationSession` CRD schema itself**: unchanged per `BR-AA-KA-065.10`; out of scope.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Fold production removal + ~11 UT/IT files into PR #2189 now; defer the 28-file E2E trigger redesign to #2190 | Bounds single-PR risk while still capturing most of the CI-cost benefit (E2E doesn't run on every unit-test push); user-approved trade-off given scarce GH Actions runners |
| Keep `RecordAIAgentSubmit`/`RecordAIAgentResult` event *names* and repurpose them for the CRD-create/Status-read moments, rather than renaming | Preserves existing SOC2 audit-trail continuity/dashboards; the event still means "a KA investigation was submitted/resulted," only the transport changed. Documented explicitly in §2's DD-AUDIT-003 update so the repurposing is traceable, not silent |
| Delete `RecordAIAgentSessionLost`/`EventTypeAIAgentSessionLost` outright (2026-08-18), rather than leaving it as inert dead code | `handleSessionLost` (its sole caller) was already deleted; `AuditClientInterface` never declared this method, so no interface contract changed. Confirmed via `go build ./...`, `go vet ./...`, targeted `go test`, and `golangci-lint run` all clean before and after removal |
| Also remove the `aianalysis.aiagent.session_lost` discriminator-mapping entry from `api/openapi/data-storage-v1.yaml` and regenerate `pkg/datastorage/ogen-client` (2026-08-18) | Once the app-level method was gone, leaving the OpenAPI schema still accepting a `session_lost` event type would let the wire contract silently drift from what the code actually emits. The shared `AIAnalysisAIAgentCallPayload` schema itself is untouched (still routes `.call`/`.submit`/`.result`); only the `session_lost` route was removed. Regenerated via `make generate-datastorage-client`; verified with `make test-unit-aianalysis` and `make test-unit-datastorage` (both green) |
| Test-only fix for the namespace-scoping mismatch (integration harness pins test namespace to match KA's per-process watch scope) rather than changing KA's production watch to be cluster-scoped | Matches KA's namespaced `Lease` RBAC (least privilege); avoids widening production RBAC just to satisfy a test topology |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: pure mapping/decision logic — `mapping.go`, `dispatcher.go`'s dispatch decision,
  `request_builder.go`, `TerminalHook`/`InteractiveUpgradeHook` firing logic, predicate logic.
- **Integration**: wiring — real envtest `AgentSession` CRUD/watch, real per-process KA dispatcher
  reconciling against that envtest, real RBAC enforcement (Helm chart tests).
- **E2E**: full-pipeline journeys (`test/e2e/fullpipeline/11_session_upgrade_test.go` for the
  #1713-shaped upgrade journey) — largely **unaffected structurally** by this redesign since AA/KA
  still communicate observably the same way from an external black-box view; the 28 files that
  construct requests via direct HTTP POST against KA are the deferred #2190 scope.

### 5.2 Two-Tier Minimum

Met for every `BR-AA-KA-065.N` except `.8` (deferred, no tier applies yet) — see §7. Most
sub-requirements have 3 tiers (UT + IT wiring proof + E2E journey) since the wiring change here is
inherently cross-service.

### 5.3 Business Outcome Quality Bar

Every Test ID in §7/§8 was selected because it existed in the actual PR #2189 diff with a
business-outcome-shaped `Describe`/`It` string (confirmed via direct file reads, not inferred from
names) — no test ID in this document is aspirational or "to be written."

### 5.4 Pass/Fail Criteria

**PASS** (this plan, as of 2026-08-18):
1. All P0 tests pass — confirmed (`go build ./...`, targeted `go test` run, §1.3).
2. `BR-AA-KA-065.1`–`.7`, `.9`–`.12` each have ≥1 passing Unit test and either an Integration test
   or a structural-deletion proof (grep-confirmed zero remaining references to the retired code
   path).
3. No unexplained regressions — every deleted/rewritten existing test has a documented reason
   (§15).

**Explicitly NOT claimed as PASS**: `BR-AA-KA-065.8` (HTTP removal) — honestly reported as
**Deferred**, not glossed over as done.

### 5.5 Suspension & Resumption Criteria

Not applicable — this is a finalized, post-implementation record, not a plan governing
not-yet-written tests.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/agentsession/mapping.go` | `MapSpecToSignal`, `MapResultToStatus` | ~230 |
| `internal/kubernautagent/agentsession/dispatcher.go` | `Dispatcher.reconcile`, dispatch-time Interactive check | ~530 |
| `pkg/aianalysis/creator/agentsession.go` | `GetOrCreate` | ~90 |
| `pkg/aianalysis/handlers/request_builder.go` | `BuildAgentSessionSpec` | ~200 |
| `internal/controller/aianalysis/aianalysis_controller.go` | `AgentSessionEventPredicate` | ~45 |
| `internal/kubernautagent/session/manager.go`, `manager_interactive.go`, `manager_query.go` | `TerminalHook`/`InteractiveUpgradeHook` invocation sites | ~150 |
| `internal/kubernautagent/mcp/tools/investigate_start.go`, `investigate_autonomous.go` | `reattachOrCreateFallback` | ~120 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/agentsession/status_writer.go` | `updateStatus` (retry-on-conflict against real envtest apiserver) | ~90 |
| `pkg/apifrontend/tools/crd_tools_session.go` | `AwaitAgentSessionInteractive` | ~60 |
| `test/integration/aianalysis/suite_test.go` | Per-process KA + real `AgentSession` CRD wiring | N/A (infra) |
| `charts/kubernaut/templates/{aianalysis,kubernaut-agent,apifrontend}/*.yaml` | RBAC manifests | N/A |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feature/2170-agentsession-crd` HEAD, PR #2189 | Production removal + UT/IT scope |
| Deferred dependency | Issue #2190 | `pkg/agentclient`/KA HTTP server + 28-file E2E redesign |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier(s) | Test ID(s) | Status |
|-------|-------------|----------|---------|------------|--------|
| BR-AA-KA-065.1 | `AgentSession` as single AA-KA dispatch/result channel; idempotent Create | P0 | Unit | UT-AA-KA-065-201, -202, -204, -205, -206, -207, UT-AA-1356-H4-01 | **Pass** |
| BR-AA-KA-065.1 | Same, wiring proof | P0 | Integration | IT-AA-1376-001, IT-AA-1376-002 (real reconcile loop against AgentSession-backed investigation) | **Pass** |
| BR-AA-KA-065.2 | AA Create semantics — lossless 1:1 Spec translation | P0 | Unit | UT-AA-KA-065-001, -002, -010–015 (KA decode side), UT-AA-KA-065-101–116 (AA encode side), UT-AA-KA-065-203 | **Pass** |
| BR-AA-KA-065.3 | KA dispatch exactly-once across replicas (Lease-based) | P0 | Unit | UT-AA-KA-065-020, -022, -023 | **Pass** |
| BR-AA-KA-065.3 | Lease-name DNS-1123 regression (live incident) | P1 | Unit | IT-AA-2170-DISPATCH-LEASE-NAME | **Pass** |
| BR-AA-KA-065.4 | Curated Status writer; no update silently dropped | P0 | Unit | UT-AA-KA-065-021, UT-AA-KA-065-010 | **Pass** (dispatch-accept/complete/fail write paths); **Gap**: CAS-conflict-retry-to-completion path unverified — see §3 R3 |
| BR-AA-KA-065.5 | Interactive detection via `Status.Interactive` (takeover + fresh pre-emptive start), KA-exclusive | P0 | Unit | UT-INTERACTIVE-010-030, UT-AA-KA-065-024, UT-KA-2170-010–013, UT-KA-774-007/008, UT-AA-703-001–004 | **Pass** |
| BR-AA-KA-065.5 | AA reconciles immediately on `Interactive`-only change | P0 | Unit | UT-AA-2030-013a | **Pass** |
| BR-AA-KA-065.6 | AF ack-wait via `AgentSessionList` watch (not named-object, not IS poll) | P0 | Unit | UT-AF-AS-WATCH-001–010 | **Pass** |
| BR-AA-KA-065.6 | AF path integration proof | P1 | Unit (integration-shaped) | UT-AF-1916-002 | **Pass** |
| BR-AA-KA-065.7 | No regeneration-cap failure mode reachable | P0 | Unit | UT-AA-KA-065-207 (#2081 regression) | **Pass** |
| BR-AA-KA-065.7 | Structural closure (offending code deleted) | P0 | Structural | grep-confirmed zero references: `handleSessionLost`, `tryAdoptCorrelatedSession`, `checkISMismatchAndCancel`, `HasActiveSession`, `CorrelatedSessionID` | **Pass** |
| BR-AA-KA-065.8 | HTTP channel removal (`pkg/agentclient`, KA HTTP handlers, OpenAPI spec) | P1 | — | — | **Deferred (#2190)** |
| BR-AA-KA-065.9 | RBAC least privilege (AA/KA/AF scoping) | P0 | Unit (Helm chart) | UT-KA-HELM-002, -003, -004 | **Pass** |
| BR-AA-KA-065.9 | RoleBinding removal (AA no longer binds `kubernaut-agent-client`) | P1 | Manifest | `charts/kubernaut/templates/aianalysis/aianalysis.yaml` diff (RoleBinding deleted) | **Pass** |
| BR-AA-KA-065.10 | `InvestigationSession` unaffected; AA stops reading for decisions | P0 | Unit + Structural | UT-KA-HELM-004; grep-confirmed AA decision-reading functions deleted | **Pass** |
| BR-AA-KA-065.11 | No silent drop on out-of-band terminal completion | P0 | Unit | UT-KA-2170-001–006 (esp. UT-KA-2170-005 race case) | **Pass** |
| BR-AA-KA-065.12 | Interactive fallback fails closed, not placeholder | P0 | Unit | UT-KA-1440-010–012, UT-KA-1818-001–004 | **Pass** |
| BR-AA-KA-065.12 | Same, production MCP dispatch path proof | P0 | Integration | IT-KA-1440-010 | **Pass** |
| — (#2080/#2081 regression) | Concurrent hand-offs never exhaust a regeneration cap that no longer exists | P0 | Unit | UT-AA-KA-065-207 | **Pass** |
| — (#1713 regression) | Interactive-upgrade-ack propagation no longer races AA's own poll | P0 | Unit + E2E | UT-AA-2030-013a; `E2E-FP-1390-001` (90s fast-fail assertion replacing the old 10-min timeout window) | **Pass** |
| — (DD-AA-KA-001 Gap 1) | Fresh interactive start decided at KA's dispatch point, not AA's Create | P0 | Unit + Integration | UT-INTERACTIVE-010-030, UT-AA-KA-065-024; UT-AF-AS-WATCH-003 (AF handles pre-AgentSession-existence race) | **Pass** |
| — (DD-AA-KA-001 Gap 2) | Terminal-status-write race — single commit point | P0 | Unit | UT-KA-2170-001–006 | **Pass** |
| — (DD-AA-KA-001 Gap 3) | KA's MCP-direct interactive fallback fails closed | P0 | Unit + Integration | UT-KA-1440-010–012, UT-KA-1818-001–004; IT-KA-1440-010 | **Pass** |

### Status Legend

- **Pass**: Implemented and passing (confirmed by direct file read + `go test` run, 2026-08-18)
- **Gap**: Implemented but with an identified, undocumented-elsewhere coverage hole (see §3)
- **Deferred (#2190)**: Explicitly out of scope for this plan/PR; tracked in the linked follow-up issue

---

## 8. Test Scenarios by Tier

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/agentsession/`, `pkg/aianalysis/creator/`,
`pkg/aianalysis/handlers/`, `internal/controller/aianalysis/` (predicate only),
`internal/kubernautagent/session/`, `internal/kubernautagent/mcp/tools/`,
`pkg/apifrontend/tools/`.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-AA-KA-065-001/002 | Every incident-payload field on `Spec` maps to `SignalContext`; nil optionals don't panic | Pass |
| UT-AA-KA-065-010–015 | Completed `InvestigationResult` maps every curated field into `AgentSessionResult`, safely handling unknown/absent review reasons | Pass |
| UT-AA-KA-065-020–024 | Dispatch happens exactly once per replica race; failure/interactive/reclaim paths write correct curated Status | Pass |
| UT-AA-KA-065-101–116 | `BuildAgentSessionSpec` losslessly forwards every AA-side field (cluster, signal mode, enrichment, custom labels) | Pass |
| UT-AA-KA-065-201–207 | `GetOrCreate` naming/ownerRef/idempotency/error/concurrency contract | Pass |
| UT-KA-2170-001–013 | `TerminalHook`/`InteractiveUpgradeHook` fire exactly once from the correct single commit point, even under a goroutine race | Pass |
| UT-KA-1440-010–012, UT-KA-1818-001–004 | Interactive fallback session creation fails closed with an actionable error when no real investigation exists | Pass |
| UT-AF-AS-WATCH-001–010 | AF's `AwaitAgentSessionInteractive` correctly watches, times out, filters by RR, and falls back to polling for watch-incapable clients | Pass |
| UT-AA-2030-013/013a/013b/013c | `AgentSessionEventPredicate` passes Interactive/SessionID-only changes, drops no-ops | Pass |

### Tier 2: Integration Tests

**Testable code scope**: `test/integration/aianalysis/` (per-process KA + real envtest
`AgentSession` CRD), `internal/kubernautagent/mcp/tools/*_it_test.go`.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-AA-1376-001/002 | `InvestigationSession` transitions Completed/Failed when the real, AgentSession-backed investigation reaches that outcome (real reconcile loop, CHECKPOINT W) | Pass |
| IT-AA-2030-006 | Schema-rejection retry against a real apiserver (adjacent hardening, same PR) | Pass |
| IT-KA-1440-010 | MCP `action=start` with no prior session fails closed through KA's production dispatch path | Pass |
| IT-AA-2170-DISPATCH-LEASE-NAME | Long `AgentSession` names don't produce an invalid Lease name | Pass |

### Tier 3: E2E Tests

**Testable code scope**: `test/e2e/fullpipeline/11_session_upgrade_test.go` (structurally
unaffected by the CRD-vs-HTTP transport change from a black-box perspective).

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| E2E-FP-1390-001 | Full autonomous→interactive upgrade journey; the interactive-ack assertion now uses a 90s timeout (not the package's 10-minute default) because the #1713 race it used to occasionally hit is structurally gone | Pass |

### Tier Skip Rationale

- **E2E, broader than the one file above**: the remaining 28 E2E files that submit investigation
  requests via a direct HTTP POST against KA are unaffected in *what* they assert (KA still
  produces the same investigation outcome) but need their *trigger mechanism* redesigned to create
  an `AgentSession` instead of calling the HTTP endpoint. Redesigning 28 files' fixtures was judged
  too large to fold into PR #2189 alongside production removal without a second full CI cycle risk;
  deferred to #2190 per explicit user decision. Until #2190 lands, these 28 files continue to
  exercise `pkg/agentclient`/KA's HTTP server, which is why that code cannot yet be deleted (§4.2).

---

## 9. Test Cases (P0 detail)

Per template guidance ("for large plans, provide detailed cases for P0 tests and summarize
P1/P2"), full IEEE 829 case detail is given only for the three implementation-time gaps and the
two named regressions; all other Test IDs are summarized in §7/§8 with direct file:line evidence
already captured during this plan's authoring (see the accompanying research inventory).

### UT-AA-KA-065-207: Concurrent GetOrCreate never exhausts a regeneration cap

**BR**: BR-AA-KA-065.1/.7 (also closes #2081, clone of #2080)
**Priority**: P0
**Type**: Unit
**File**: `pkg/aianalysis/agentsession_creator_test.go`

**Test Steps**:
1. **Given**: An `AIAnalysis` with no existing `AgentSession`.
2. **When**: 10 goroutines concurrently call `GetOrCreate` for the same `AIAnalysis`.
3. **Then**: No caller returns an error; exactly one `AgentSession` exists afterward.

**Expected Results**: `list.Items` has length 1; all 10 `<-results` channel reads are nil.

**Acceptance Criteria**: There is no regeneration-cap concept left in `GetOrCreate` to exhaust —
the old failure mode (`handleSessionLost`'s uncapped-race-against-`tryAdoptCorrelatedSession`) is
eliminated by construction, verified by the fact that this exact race shape (10 concurrent
"hand-offs") no longer has any error path to hit.

### UT-AA-2030-013a: `AgentSessionEventPredicate` reconciles immediately on `Interactive`-only change

**BR**: BR-AA-KA-065.5/.6 (also closes #1713)
**Priority**: P0
**Type**: Unit
**File**: `internal/controller/aianalysis/predicates_test.go`

**Test Steps**:
1. **Given**: An `AgentSession` Update event where only `Status.Interactive` flips `false`→`true`
   (Phase unchanged).
2. **When**: `AgentSessionEventPredicate().Update(event)` is invoked.
3. **Then**: The predicate returns `true` (the controller wakes immediately).

**Acceptance Criteria**: Without this, KA's dispatch acknowledgment (which only changes
`Interactive`/`SessionID`, not `Phase`) would be silently dropped and AA would only learn about it
on its next scheduled poll — the exact multi-minute-hang shape `#1713` reported.

### UT-KA-2170-005: `TerminalHook` race — out-of-band outcome always wins

**BR**: BR-AA-KA-065.11 (DD-AA-KA-001 Amendment Gap 2)
**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/session/terminal_hooks_2170_test.go`

**Test Steps**:
1. **Given**: A session whose investigation goroutine is still running.
2. **When**: `ForceCompleteByRemediationID` commits an out-of-band terminal outcome concurrently
   with the goroutine's own (now-stale) return.
3. **Then**: `TerminalHook` fires exactly once, with the out-of-band outcome — never the stale
   goroutine value.

**Acceptance Criteria**: `calls` has length 1; the cancelled goroutine's own rejected `store.Update`
must never re-fire the hook.

### UT-KA-1818-004: Interactive fallback fails closed with no placeholder

**BR**: BR-AA-KA-065.12 (DD-AA-KA-001 Amendment Gap 3)
**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/mcp/tools/handlestart_robustness_1440_test.go`

**Test Steps**:
1. **Given**: No `user_driving` session and no real RCA anywhere for the remediation request.
2. **When**: `reattachOrCreateFallback` is called.
3. **Then**: It returns without calling `StartInvestigation`, letting `handleStart`'s existing
   `#2100` fail-closed path release the Lease and return `ErrCodeNoInvestigationAvailable`.

**Dependencies**: IT-KA-1440-010 proves the same contract through the real MCP dispatch path.

### IT-KA-1440-010: Fail-closed proven through production MCP dispatch

**BR**: BR-AA-KA-065.12
**Priority**: P0
**Type**: Integration
**File**: `internal/kubernautagent/mcp/tools/handlestart_robustness_1440_it_test.go`

**Test Steps**:
1. **Given**: A real MCP tool-call dispatch path, no prior session for the RR.
2. **When**: `action=start` is invoked.
3. **Then**: The call fails closed with an actionable error through the actual production
   entry point — not just the unit-tested helper in isolation.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (all files above use it; zero `testing.T` business-logic tests
  introduced by this feature).
- **Mocks**: `fake.NewClientBuilder()` (controller-runtime fake client) for K8s API only; no
  external-dependency mocks needed since `AgentSession` is an in-cluster CRD, not an external
  service.
- **Location**: `pkg/aianalysis/`, `internal/kubernautagent/agentsession/`,
  `internal/kubernautagent/session/`, `internal/controller/aianalysis/`, `pkg/apifrontend/tools/`.

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD.
- **Mocks**: ZERO — real envtest apiserver, real per-process KA container.
- **Infrastructure**: `envtest` (one instance per parallel Ginkgo process, per `DD-TEST-010`), a
  per-process KA container watching that same envtest for `AgentSession` dispatch.
- **Location**: `test/integration/aianalysis/`, `internal/kubernautagent/mcp/tools/*_it_test.go`.

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD.
- **Infrastructure**: Kind/real cluster, full service set (AA, KA, AF, mock LLM).
- **Location**: `test/e2e/fullpipeline/`.

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #2190 | Code | Open | `pkg/agentclient`/KA HTTP server cannot be deleted; 28 E2E files still HTTP-based | None needed short-term — HTTP server remains fully functional, just redundant with the new CRD path for AA's traffic |

### 11.2 Execution Order (as actually followed)

1. **Phase 1 (RED)**: `mapping_test.go`, `dispatcher_test.go`, `agentsession_creator_test.go` written first against not-yet-existing production code.
2. **Phase 2 (GREEN)**: `mapping.go`, `dispatcher.go`, `creator/agentsession.go` implemented; wired into `cmd/aianalysis/main.go` and KA's dispatcher startup.
3. **Phase 3 (REFACTOR)**: Dead code removal (`BuildIncidentRequest` + its test file), RBAC narrowing, CRD manifest regeneration (`make manifests`).
4. **Phase 4 (WIRING VERIFICATION)**: IT-AA-1376-001/002, IT-KA-1440-010 confirm the real reconcile/dispatch loops, not just unit-level mocks.
5. **Phase 5 (E2E)**: `E2E-FP-1390-001`'s fast-fail assertion confirmed the #1713 race no longer reproduces; the remaining 28-file E2E redesign deferred to #2190.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/testing/2170/TEST_PLAN.md` | Finalized BR coverage record |
| DD-AUDIT-003 update | `docs/architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md` | Event-catalog correction for the AA↔KA audit events repurposed/retired by this design |
| Unit test suites | `internal/kubernautagent/agentsession/`, `pkg/aianalysis/`, etc. (listed §6.1) | Ginkgo BDD |
| Integration test suites | `test/integration/aianalysis/`, `internal/kubernautagent/mcp/tools/` | Ginkgo BDD |

---

## 13. Execution

```bash
# Unit tests (all touched packages)
go test ./pkg/aianalysis/... ./internal/controller/aianalysis/... \
  ./internal/kubernautagent/agentsession/... ./internal/kubernautagent/session/... \
  ./internal/kubernautagent/mcp/... ./pkg/apifrontend/tools/... -ginkgo.v

# Specific regression by ID
go test ./pkg/aianalysis/... -ginkgo.focus="2081"
go test ./internal/controller/aianalysis/... -ginkgo.focus="UT-AA-2030-013a"

# Integration tests
go test ./test/integration/aianalysis/... -ginkgo.v
go test ./internal/kubernautagent/mcp/tools/... -run TestKubernautAgent -ginkgo.focus="IT-KA-1440"

# Helm RBAC unit tests
make test-helm
```

---

## 14. Wiring Verification

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| `AgentSessionCreator.GetOrCreate` | `InvestigatingHandler.Handle` (AA reconcile) | `AgentSession` object created in-cluster | IT-AA-1376-001/002 | Pass |
| `Dispatcher.reconcile` | KA controller-runtime watch on `AgentSession` | `AgentSession.Status` written | IT-AA-1376-001/002, IT-AA-2170-DISPATCH-LEASE-NAME | Pass |
| `reattachOrCreateFallback` | KA MCP `action=start` tool call | `ErrCodeNoInvestigationAvailable` or seeded session | IT-KA-1440-010 | Pass |
| `AwaitAgentSessionInteractive` | AF MCP tool awaiting session takeover ack | `(bool, error)` returned to AF's ADK tool caller | UT-AF-1916-002 (integration-shaped; no dedicated `*_it_test.go` yet) | Pass (unit-tier only — see note below) |

**Note**: `AwaitAgentSessionInteractive`'s wiring proof currently comes from a unit test
(`status_message_leak_1916_test.go`) that exercises it through AF's production call site with a
fake client, not a dedicated envtest-backed `*_it_test.go`. This satisfies CHECKPOINT W's "has a
production caller" bar but is weaker than the AA/KA-side IT coverage; flagged here for visibility,
not silently upgraded to "Integration" tier without a real envtest behind it.

---

## 15. Existing Tests Requiring Updates / Removed

| Test ID / Location | Prior Assertion | Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `pkg/aianalysis/request_builder_test.go` (deleted) | Tested `BuildIncidentRequest` | File deleted entirely | `BuildIncidentRequest` and its helper `buildEnrichmentResults` are dead code — zero production callers once the HTTP channel had no consumer for their output. All scenarios had equivalent (in some cases stronger) coverage in `request_builder_agentsession_test.go` |
| `pkg/aianalysis/enrichment_cleanup_test.go` | Called `BuildIncidentRequest`, asserted on `ogenclient`-typed `EnrichmentResults.Set`/`.Value` | Calls `BuildAgentSessionSpec`; asserts on the `apiextensionsv1.JSON`-typed raw field via `json.Unmarshal` | Return type changed from the retired ogen client struct to the CRD's raw JSON field |
| `test/integration/aianalysis/agentclient_integration_test.go` (deleted) | Direct-HTTP integration test against KA's endpoint | File deleted entirely | Explicitly named for deletion in DD-AA-KA-001's "What gets deleted, not just added" section — its production code path (`realAgentClient`) has no remaining caller |
| `pkg/apifrontend/tools/await_is_phase_test.go` (deleted) | Tested `AwaitISPhaseActive` polling `InvestigationSession.Status.Phase` | File deleted; replaced by `await_agentsession_interactive_test.go` | `AwaitISPhaseActive` itself is retired per BR-AA-KA-065.6; superseded by `AwaitAgentSessionInteractive` |
| `docs/tests/1937/TEST_PLAN.md` UT-AF-1916-002 | Historical `PASS` record citing `AwaitISPhaseActive`/IS `Phase==Active` trigger | Non-destructive addendum note only — historical PASS record preserved, trigger-fixture swap documented separately | Editing a closed historical record would misrepresent what actually shipped at #1916's close; the addendum explains the later trigger swap without rewriting history |
| `internal/controller/aianalysis/predicates_test.go` | N/A (existing file) | Added UT-AA-2030-013/013a/013b/013c | New predicate behavior (Interactive/SessionID-only wake-up) needed coverage alongside the pre-existing terminal-phase cases |
| `test/integration/aianalysis/suite_test.go` | Constructed a direct-HTTP `agentclient.KubernautAgentClient` (`realAgentClient`) | Removed; `startPerProcessKubernautAgent`'s return values discarded (KA's HTTP endpoint itself still starts, still load-bearing for the deferred `test/e2e/kubernautagent/` suite) | The sole consumer of `realAgentClient` (`agentclient_integration_test.go`) was deleted; AA's only remaining channel to KA in this suite is the `AgentSession` CRD |
| `charts/kubernaut/tests/crd_manifests_split_test.yaml` | `hasDocuments.count: 11`, `lengthEqual.count: 10` | `count: 12`, `count: 11` | New `agentsessions.kubernaut.ai` CRD added a 12th split document |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-18 | Initial finalized test plan, written post-implementation against PR #2189. Documents BR-AA-KA-065.1–.12 coverage, the two closed regressions (#2080/#2081, #1713), the three DD-AA-KA-001 Amendment gaps, and honestly reports BR-AA-KA-065.8 (HTTP removal) as deferred to #2190. Identifies one open coverage gap (CAS-conflict-retry path, §3 R3) rather than papering over it. |
