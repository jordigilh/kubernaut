# Test Plan: AgentSession Existence Gate Reads the RemediationRequest Directly (#2674)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2674-v1.0
**Feature**: The `AgentSession` CREATE existence gate (`BR-AA-KA-065.13`) resolves `Spec.RemediationRequestRef` through the manager's direct API reader instead of the manager's cache-backed client, and the `test/integration/authwebhook` harness waits for cache sync before running specs.
**Version**: 1.0
**Created**: 2026-11-19
**Author**: Kubernaut Team
**Status**: Implemented on `work/2674-agentsession-cache-gate`
**Branch**: `work/2674-agentsession-cache-gate`

---

## 1. Introduction

### 1.1 Purpose

`IT-AS-2244-002` flaked once on PR #2668 (CI run 44589878105, attempt 1) with
`remediation request lookup failed for "rr-never-created-…": the cache is not started, can not read objects`
instead of the expected `does not exist`, and passed on retry. Triage (issue #2674) established two
independent defects:

1. **Production window (a) — cache not started.** `cmd/authwebhook/main.go` wired the gate with
   `mgr.GetClient()` (cache-backed). controller-runtime v0.24.1 starts webhook listeners *before*
   caches (`pkg/manager/internal.go:429-446`), and `/readyz` is `healthz.Ping` plus the DataStorage
   gate (`main.go:226-234`), so an admission request can reach the pod while
   `InformerCache.Get` still returns `&cache.ErrCacheNotStarted{}`
   (`pkg/cache/informer_cache.go:77`).
2. **Production window (b) — cache lagging.** Once the cache runs, an informer that has not yet
   observed a just-created `RemediationRequest` reads it as `NotFound`, which the handler maps to the
   same `does not exist` denial as a genuinely dangling reference
   (`pkg/authwebhook/agentsession_handler.go:99-118`).
3. **Harness.** `test/integration/authwebhook/suite_test.go` started the manager in a goroutine and
   never waited for `GetCache().WaitForCacheSync`, while a comment falsely claimed `Manager.Start()`
   blocks until the webhook server is ready.

Both production windows fail closed — no unauthorized admission — but they deny a legitimate
`AgentSession` and record an internal lookup error (or a false "does not exist") as the denial reason
in the audit trail, which misleads operators reconstructing the denial.

### 1.2 Objectives

1. Prove the gate resolves `RemediationRequestRef` from the API server in both cache windows, so a
   real `RemediationRequest` always admits its `AgentSession`.
2. Prove the denial reason for a genuinely missing `RemediationRequest` is unchanged (`does not exist`)
   and that no cache-layer error string reaches the admission response.
3. Prove the integration harness no longer races cache start-up, so `IT-AS-2244-001/002/003` are
   deterministic rather than retry-dependent.
4. Preserve every existing outcome of the gate: allow on a resolvable reference, deny on a dangling
   reference, deny on a cross-namespace reference, no UPDATE/DELETE interception, fail-closed on an
   unmarshal error.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|---|---:|---|
| New cache-window scenarios passing | 100% | `IT-AS-2674-001/002` |
| Existing AgentSession gate behaviour regressions | 0 | `UT-AS-2244-001..007`, `IT-AS-2244-001..004`, `E2E-AS-2244-DENY/ALLOW` |
| In-scope unit business scenarios | >= 90% | §5 matrix; no scenario removed or diluted |
| Wiring points proven | 100% | §4 Wiring Manifest |
| Applicable control objectives assessed | 100% | §6 |

---

## 2. References

### 2.1 Authority

- Issue [#2674](https://github.com/jordigilh/kubernaut/issues/2674) — RCA, file:line evidence, proposed fix.
- Issue [#2667](https://github.com/jordigilh/kubernaut/issues/2667) / PR #2668 — the test-only PR whose CI run surfaced the flake.
- Issue [#2244](https://github.com/jordigilh/kubernaut/issues/2244) — original existence gate.
- [DD-WEBHOOK-001 §Use Case 5](../../architecture/decisions/DD-WEBHOOK-001-crd-webhook-requirements-matrix.md) — `BR-AA-KA-065.13` gate definition (SI-10 at the trust boundary).
- [DD-WORKFLOW-018](../../architecture/decisions/DD-WORKFLOW-018-etcd-single-source-of-truth.md) — existence-gate precedent.
- [DD-AUDIT-003](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) — AuthWebhook audit event controls (FedRAMP AU-2, AU-3).
- controller-runtime v0.24.1 `pkg/manager/internal.go:429-446`, `pkg/cache/informer_cache.go:43-77`.
- AC: every spec carries one scenario ID and one BR/control reference (AGENTS.md).

### 2.2 Control objectives retrieved

| Objective | Applicable controls | Retained from |
|---|---|---|
| Existence validation at the admission trust boundary | FedRAMP SI-10; OWASP ASVS 5.0.0 V2.2.1–V2.3.1 | DD-WEBHOOK-001 Use Case 5; `docs/requirements/BR-KA-016-history-aware-remediation-context.md` control table |
| Denial attribution is accurate and structured | FedRAMP AU-2 (events), AU-3 (structured payload content) | DD-AUDIT-003 "Controls"; `pkg/authwebhook/types.go` `agentsession.denied.create` |
| Fail-closed on lookup failure (no fail-open) | FedRAMP SI-10; SOC 2 CC8.1 (change control, this process) | `pkg/authwebhook/agentsession_handler_test.go` UT-AS-2244-005 |

Change-approval mapping (`CC8.1`) applies to this implementation process only; it is not an audit-event control (AGENTS.md correction, 2026-08-17).

---

## 3. Scope

### 3.1 In scope

- `cmd/authwebhook/main.go`: `agentSessionGateReader` wiring seam (direct API reader).
- `pkg/authwebhook/agentsession_handler.go`: reader type and the doc contracts it states.
- `test/integration/authwebhook/suite_test.go`: `WaitForCacheSync`, corrected comment, direct-reader wiring.
- `test/integration/authwebhook/agentsession_test.go`: handler wired as production is.

### 3.2 Out of scope

- `/readyz` cache-sync gating (deploy-behaviour change) — not required for correctness of this gate.
- Other AuthWebhook handlers (RAR already uses `GetAPIReader`; WFE/RR/NR/RW/ActionType gates untouched).
- CRD or `ValidatingWebhookConfiguration` policy changes; PR #2668 / TP-2584.

---

## 4. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT / wiring test |
|---|---|---|---|
| `agentSessionGateReader` | `registerAuthWebhookWorkflowHandlers` (called from `main`) | `cmd/authwebhook/main.go` (`asHandler := authwebhook.NewAgentSessionHandler(auditStore, agentSessionGateReader(mgr))`) | `IT-AS-2674-001`, `IT-AS-2674-002` (`cmd/authwebhook/agentsession_reader_2674_test.go`) |
| `WaitForCacheSync` before specs | `SynchronizedBeforeSuite` Phase 2 | `test/integration/authwebhook/suite_test.go` | `IT-AS-2244-001/002/003` now deterministic |

`CHECKPOINT W`: the emitters (`NewAgentSessionHandler` + `agentSessionGateReader`) are constructed in
`cmd/authwebhook/main.go`, not just tests; `IT-AS-2674-001/002` drive the same seam production calls.

---

## 5. Scenario matrix

| Scenario ID | Business assertion | Tier / file | BR / control | Pre-fix result |
|---|---|---|---|---|
| `IT-AS-2674-001` | `AgentSession` CREATE is admitted when the `RemediationRequest` exists in etcd and the manager cache has not started (`cache.ErrCacheNotStarted`) | wiring, `cmd/authwebhook/agentsession_reader_2674_test.go` | `BR-AA-KA-065.13`, FedRAMP SI-10 | **Fail** — denied with `the cache is not started, can not read objects` |
| `IT-AS-2674-002` | `AgentSession` CREATE is admitted when the cache lags a just-created `RemediationRequest` (cached read returns `NotFound`), with no `does not exist` in the response | wiring, same file | `BR-AA-KA-065.13`, FedRAMP SI-10, AU-3 | **Fail** — denied with `remediationRequestRef "…" does not exist in namespace "…"` |
| `IT-AS-2244-001` | Admits a reference that resolves, emits `agentsession.admitted.create` | integration, `test/integration/authwebhook/agentsession_test.go` | `BR-AA-KA-065.13` | Pass (regression guard) |
| `IT-AS-2244-002` | Denies a dangling reference with `does not exist`, emits `agentsession.denied.create` | integration, same file | `BR-AA-KA-065.13`, SI-10, AU-2/AU-3 | Pass; previously flaky pre-cache-sync, now deterministic |
| `IT-AS-2244-003` | Denies a same-name reference in another namespace | integration, same file | `BR-AA-KA-065.13`, SI-10 | Pass (regression guard) |
| `IT-AS-2244-004` | UPDATE is not intercepted | integration, same file | `BR-AA-KA-065.13` | Pass (regression guard) |
| `UT-AS-2244-001..007` | nil reader skip, allow, deny-not-found, namespace scoping, fail-closed on lookup error, UPDATE/DELETE pass-through, unmarshal denial | unit, `pkg/authwebhook/agentsession_handler_test.go` | `BR-AA-KA-065.13`, SI-10, AU-3 | Pass (regression guard) |
| `E2E-AS-2244-DENY` / `ALLOW` | Cluster-level journey through the real `ValidatingWebhookConfiguration` | E2E, `test/e2e/authwebhook/06_agentsession_gate_test.go` | `BR-AA-KA-065.13`, SI-10 | Pass (regression guard; not run for this change) |

Scenario coverage for the change: 2 new scenarios, both critical (a false denial of a legitimate
`AgentSession` in either cache window), 2/2 proven. No scenario was removed, merged or diluted.
Line coverage of the touched handler path remains from the existing UT/IT set; the new tests are
wiring proof and are not counted toward business-line percentage.

---

## 6. Control objective → evidence

| Control | Requirement | Evidence in this change |
|---|---|---|
| FedRAMP SI-10 | Validate input at the trust boundary before dependent processing | `IT-AS-2674-001/002`: the boundary check reaches the API server, not a possibly-unstarted/lagging cache; denial on a genuinely absent reference unchanged (`IT-AS-2244-002`) |
| FedRAMP AU-2 | Audit events for admission decisions | `agentsession.admitted.create` / `agentsession.denied.create` unchanged; `IT-AS-2244-001/002` still assert one persisted event per decision |
| FedRAMP AU-3 | Structured, accurate audit record content | `IT-AS-2674-001/002`: no cache-layer error and no false `does not exist` contributes to `denial_reason`; `UT-AS-2244-005` keeps fail-closed lookup errors observable with their true cause |
| OWASP ASVS 5.0.0 V2.2.1–V2.3.1 | Input validation applied to externally supplied references | Same as SI-10 above |
| Fail-closed | A lookup failure never admits | `UT-AS-2244-005` unchanged; switching reader does not introduce a fail-open path |

---

## 7. Risks and mitigations

| Risk | Mitigation |
|---|---|
| `GetAPIReader` adds one API-server GET per `AgentSession` CREATE | Admission volume is low (one per investigation); the sibling RAR handler already does this. No new RBAC — the same SA credential and `get` grant are used. |
| Direct reads bypass any cache-based field index | Not applicable: `RemediationRequestRef` is a plain `ObjectRef{Name, Namespace}` resolved by an exact-name `Get`; no indexer involved (verified in `validateRemediationRequestExists`). |
| Harness `WaitForCacheSync` slows setup | Bounded by a 60 s context timeout; suite already waits for DataStorage health. |
| Other handlers still use the cached client | Out of scope per #2674; their reads are not existence gates on a just-created object. |

---

## 8. Commands

```bash
go build ./...
go test ./cmd/authwebhook/ -count=1
go test ./pkg/authwebhook/ -count=1
make test-integration-authwebhook GINKGO_FOCUS="AgentSession"
golangci-lint run --timeout=5m ./cmd/authwebhook/... ./pkg/authwebhook/...
```
