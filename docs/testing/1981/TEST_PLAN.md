# Test Plan: Pin OPA Policy Hash on RemediationApprovalRequest

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1981-v1.2
**Feature**: Propagate the Rego approval policy's SHA-256 hash (`Evaluator.GetPolicyHash()`) into
`AIAnalysis.Status.ApprovalContext.PolicyEvaluation.PolicyHash` and
`RemediationApprovalRequest.Spec.PolicyEvaluation.PolicyHash`, so an approval decision record can
be attributed to the exact policy bundle revision that produced it. **v1.1 consolidates the two
follow-up gaps originally filed as** [issue #2005](https://github.com/jordigilh/kubernaut/issues/2005)
**into this same PR**: audit-event payload parity (`RemediationApprovalAuditPayload`,
`AIAnalysisRegoEvaluationPayload`) and console/MCP-facing payload parity (`ApprovalPolicyPayload`).
**v1.2 deletes the dead `pkg/remediationapprovalrequest/audit` package** (and its
`RemediationApprovalDecisionPayload` OpenAPI schema) discovered orphaned during v1.1's wiring
verification.
**Version**: 1.2
**Created**: 2026-08-07
**Author**: AI Agent (Cursor)
**Status**: In Progress
**Branch**: `fix/1981-pin-opa-policy-hash-on-rar`

---

## 1. Introduction

### 1.1 Purpose

Approval-gating Rego is evaluated once in AIAnalysis, before the `RemediationApprovalRequest`
(RAR) exists. `RAR.spec.policyEvaluation` stores `policyName`/`matchedRules`/`decision` but no
hash/version, so a policy bundle change during the approval wait window is not detectable or
attributable to a specific bundle revision. This is an audit-attribution gap, not a
dispatch-safety gap: it does not imply re-evaluating policy at dispatch time. Kubernaut
intentionally evaluates approval policy once; re-running it live before dispatch would let an
in-flight policy change silently flip an already-approved decision, which remains out of scope.

### 1.2 Objectives

1. Add `PolicyHash string` to `rego.PolicyResult` ([pkg/aianalysis/rego/evaluator.go](../../../pkg/aianalysis/rego/evaluator.go)),
   populated from `Evaluator.GetPolicyHash()` at each of `Evaluate()`'s return points.
2. Thread the hash through to `AIAnalysis.Status.ApprovalContext.PolicyEvaluation.PolicyHash`
   ([api/aianalysis/v1alpha1/aianalysis_types.go](../../../api/aianalysis/v1alpha1/aianalysis_types.go)).
3. Thread the hash through to `RemediationApprovalRequest.Spec.PolicyEvaluation.PolicyHash`
   ([api/remediation/v1alpha1/remediationapprovalrequest_types.go](../../../api/remediation/v1alpha1/remediationapprovalrequest_types.go)).
4. Zero regression in existing approval-flow and Rego-evaluation test suites.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/aianalysis/... ./pkg/remediationorchestrator/...` |
| Full build | Clean | `go build ./...` |
| Generated-artifact consistency | Zero unreviewed diff | `make generate && make manifests && make sync-embed` produce only the expected additive field diffs |
| Backward compatibility | 0 regressions | Full `pkg/aianalysis`, `pkg/remediationorchestrator`, and `test/integration/aianalysis` suites remain green |

### 1.4 v1.1 Addendum: Consolidated Follow-Up Scope (Issue #2005)

Issue #1981 was deliberately scoped to CRD-level attribution only (Section 1 above). Preflight for
#1981 identified two additional consumers still lacking the hash, filed as
[issue #2005](https://github.com/jordigilh/kubernaut/issues/2005). Per explicit user direction,
both gaps are consolidated into this same PR rather than shipped separately:

1. **Audit-event payload parity**: `AIAnalysisRegoEvaluationPayload` and
   `RemediationApprovalAuditPayload` (`api/openapi/data-storage-v1.yaml`) gain an optional
   `policy_hash` field, so the persisted audit trail (Data Storage) can reconstruct the policy
   bundle revision from the audit event alone, not only from the live CRD.
2. **Console/MCP-facing payload parity**: `ApprovalPolicyPayload`
   (`pkg/apifrontend/tools/approval_event.go`) gains `PolicyHash`, so operators reviewing an
   approval in the console/agent UI can see the pinned hash.

All additions are optional fields (`OptString` in the generated ogen client / `omitempty` in Go),
following the exact precedent already established by `SignalProcessingAuditPayload.PolicyHash` in
the same OpenAPI spec — no new pattern introduced.

**Correction during implementation**: the original #2005 scope named `RemediationApprovalDecisionPayload`
(`pkg/remediationapprovalrequest/audit/`) as the RAR-decision audit payload needing `policy_hash`.
While implementing, wiring verification (CHECKPOINT W) discovered that package has **zero production
callers** — no `cmd/` binary constructs `remediationapprovalrequest/audit.AuditClient`. The actual,
live RAR-decision audit event is `RemediationApprovalAuditPayload` (`webhook.remediationapprovalrequest.decided`),
built by `pkg/authwebhook/audit_payload_builder.go` `BuildRARApprovalAuditPayload()` and wired via
`cmd/authwebhook/main.go` -> `NewRemediationApprovalRequestAuthHandler()` (the ADR-034 v1.7 "Event 1"
webhook-complete audit pattern). `policy_hash` was added to `RemediationApprovalAuditPayload` instead,
so the fix reaches the live audit trail.

**v1.2 — dead code deletion**: per explicit user instruction, the orphaned `pkg/remediationapprovalrequest/audit`
package (`audit.go` + `audit_test.go`, UT-RO-2005-001) and its `RemediationApprovalDecisionPayload`
OpenAPI schema (`api/openapi/data-storage-v1.yaml`, including its `oneOf` discriminator entry) were
deleted rather than left in place. The `ogen` client was regenerated (`make generate`) to drop the
now-unreferenced generated types; `go build ./...`, `go vet ./...`, and the full affected test suites
were re-verified green after deletion. [DD-AUDIT-006](../../architecture/decisions/DD-AUDIT-006-remediation-approval-audit-implementation.md)
and [BR-AUDIT-006](../../requirements/BR-AUDIT-006-remediation-approval-audit-trail.md) were annotated
as superseded/relocated to point at the live `pkg/authwebhook` implementation instead.

---

## 2. References

### 2.1 Authority (governing documents)

- [Issue #1981](https://github.com/jordigilh/kubernaut/issues/1981): Pin OPA policy hash on
  RemediationApprovalRequest for tamper-evident approval decisions
- [Spike comment](https://github.com/jordigilh/kubernaut/issues/1981#issuecomment-5209422073):
  independently re-verified during preflight (see Risks below for the one correction and one gap
  found beyond it)
- **BR-AI-030** ([docs/requirements/02_AI_MACHINE_LEARNING.md](../../requirements/02_AI_MACHINE_LEARNING.md) line 125):
  "MUST maintain policy audit trail for all approval decisions" — primary BR this issue closes a
  gap against
- **BR-AUDIT-006** ([docs/requirements/BR-AUDIT-006-remediation-approval-audit-trail.md](../../requirements/BR-AUDIT-006-remediation-approval-audit-trail.md)):
  RAR audit trail (SOC2 CC8.1/CC6.8) — existing BR this extends
- **BR-AI-059 / BR-AI-076** (#307 precedent, `ApprovalContext` -> RAR spec field mapping)
- **ADR-040** ([docs/architecture/decisions/ADR-040-remediation-approval-request-architecture.md](../../architecture/decisions/ADR-040-remediation-approval-request-architecture.md)):
  RAR spec immutability
- FedRAMP/NIST 800-53 (inherited): CM-3 (configuration change control), SI-7 (software/information
  integrity); SOC2 CC8.1 (user/decision attribution), CC6.8 (non-repudiation)

### 2.2 Cross-References

- Precedent pattern: `pkg/signalprocessing/evaluator/evaluator.go` `EvaluateSeverity()` (sets
  `PolicyHash: e.GetPolicyHash()` on its result at the point of evaluation) and
  `api/signalprocessing/v1alpha1/signalprocessing_types.go` (`PolicyHash` field: plain `+optional`
  string, no CEL/regex validation) — this plan replicates the same pattern, not a new one.
- `pkg/shared/hotreload/file_watcher.go` — source of the two-mutex race this plan's hash-capture
  point minimizes (see Risks).

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|-----------------|------------|
| R1 | `Evaluate()`'s `e.mu`-guarded `compiledQuery` and `GetPolicyHash()`'s separately-`fileWatcher.mu`-guarded `lastHash` can be read at different points in time, so a hot-reload landing between the two reads could record a hash that does not match the policy that actually produced the decision | Low — narrow window, hash only used for after-the-fact attribution, not a safety gate | Low (existing accepted risk class — identical pattern already shipped for SignalProcessing's severity policy) | UT-AI-1981-001 | Capture the hash inside `Evaluate()`'s own return statements (immediately after resolving the query), not later in `populateApprovalContext` — minimizes, does not eliminate, the window; documented as an accepted, precedented risk, consistent with SignalProcessing |
| R2 | The GH issue's spike said the AIAnalysis-side struct is `ApprovalPolicyEvaluation` in `api/aianalysis/v1alpha1/aianalysis_types.go` — this is incorrect; that struct name only exists on the RAR side | Low — would have caused a build error if followed literally | N/A (caught during preflight, before RED) | N/A | Corrected: AIAnalysis-side struct is `PolicyEvaluation` (`api/aianalysis/v1alpha1/aianalysis_types.go`); RAR-side struct is `ApprovalPolicyEvaluation` (`api/remediation/v1alpha1/remediationapprovalrequest_types.go`) |
| R3 | `make manifests`/`make sync-embed` regenerate 8 CRD YAML files (4 locations x 2 CRDs) — a missed regen step would leave the Go type and CRD schema out of sync | Medium — CRD validation would silently accept/reject the new field inconsistently with the Go type | Low (mechanical, single make target) | Manual diff review post-regen (Step green-regen) | Run `make generate && make manifests && make sync-embed` as one sequential step and inspect `git diff --stat` for exactly the 2 new CRD property blocks x 4 locations each, no unrelated diff |
| R4 (out of scope, not blocking) | `RemediationApprovalDecisionPayload`/`AIAnalysisRegoEvaluationPayload` (audit-event OpenAPI schemas) and `pkg/apifrontend/tools/approval_event.go`'s `ApprovalPolicyPayload` (console-facing JSON) do not carry the hash, so full attribution parity across audit events and the console UI is not achieved by this change alone | Low — CRD-level attribution (this issue's explicit scope) is unaffected | Certain (explicit scope decision) | N/A | Documented as a fast-follow issue (todo `followup-issue`); the audit-payload half was already flagged by the original GH spike, the console-payload half was found independently during this plan's preflight |

### 3.1 Risk-to-Test Traceability

R1 is proven acceptable (not eliminated) by UT-AI-1981-001 asserting the hash is captured
consistently with `GetPolicyHash()` within a single `Evaluate()` call under no concurrent
hot-reload — the same coverage depth as the SignalProcessing precedent it mirrors. R2 is a
preflight correction with no residual test impact. R3 is caught by build/test failure (a
sync-embed miss would fail `make sync-embed`'s own copy step or, if silently stale, fail nothing
automatically — hence the explicit manual diff-review mitigation). R4 is explicitly out of scope
and tracked via a follow-up issue rather than silently dropped.

---

## 4. Scope

### 4.1 Features to be Tested

- `rego.PolicyResult.PolicyHash` population across all 3 `Evaluate()` return paths (degraded/no
  query loaded, Rego evaluation error, success).
- `AnalyzingHandler.populateApprovalContext()` copying `result.PolicyHash` into
  `AIAnalysis.Status.ApprovalContext.PolicyEvaluation.PolicyHash`.
- `applyApprovalContext()` (in `ApprovalCreator.Create()`) copying
  `AIAnalysis.Status.ApprovalContext.PolicyEvaluation.PolicyHash` into
  `RemediationApprovalRequest.Spec.PolicyEvaluation.PolicyHash`.

### 4.2 Features Not to be Tested

- Audit-event payload schema changes (out of scope, R4).
- Console/MCP-facing `ApprovalPolicyPayload` hash surfacing (out of scope, R4).
- Re-evaluating policy at dispatch time (explicit non-goal per issue).
- CRD webhook/CEL changes (none needed — additive `+optional` field, whole-spec immutability rule
  unaffected at creation time).

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Hash captured inside `Evaluate()`'s return statements, not in `populateApprovalContext()` | Minimizes the two-mutex race window (R1); mirrors the exact call-site pattern already used by `pkg/signalprocessing/evaluator/evaluator.go` `EvaluateSeverity()` — an existing, accepted pattern, not a new architectural decision, so no new DD-XXX is required |
| Plain `+optional string` field, no CEL/regex validation on hash format | Matches the existing `SignalProcessing.Status.PolicyHash` precedent exactly; keeps the change minimal per GREEN-phase discipline |
| No audit-event or console-payload changes in this issue | Explicit issue scope (CRD-level attribution only); tracked as a fast-follow (R4) instead of silently expanding scope |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: `rego.Evaluator.Evaluate()` hash population — pure logic, 100% of new branches
  (loaded-policy path, degraded/no-policy path).
- **Unit/wiring proof** (ADR-004 fake-K8s-client tier, the same tier the existing #307 precedent
  tests for this exact `ApprovalContext` -> RAR mapping use): `AnalyzingHandler.Handle()` and
  `ApprovalCreator.Create()` propagation, exercised through their real production entry points
  with a real (testdata-backed) Rego evaluator and a fake K8s client.

### 5.2 Two-Tier Minimum

Both wiring points (AIAnalysis status population, RAR spec population) already have production
callers (`internal/controller/aianalysis` phase handlers; `internal/controller/remediationorchestrator`
reconciler) — this plan adds fields to existing, already-wired call chains, not new components, so
no new `cmd/` wiring is required. The wiring proof requirement is satisfied by extending the
existing fake-client-backed tests that already exercise these exact call chains for the sibling
`PolicyName`/`Decision`/`MatchedRules` fields (see Wiring Verification, Section 14).

### 5.4 Pass/Fail Criteria

**PASS**: all 3 new/extended test cases pass; `go build ./...` clean; `make generate && make
manifests && make sync-embed` produce only the expected 2-field-x-4-location CRD diff; the entire
pre-existing `pkg/aianalysis/...`, `pkg/remediationorchestrator/...`, and
`test/integration/aianalysis/...` suites remain green.

**FAIL**: any new or existing test regresses without documented reason; any CRD YAML location
missed by the regen step; `PolicyHash` left unpopulated on any of the 3 propagation hops.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|--------------------|-----------------|
| `pkg/aianalysis/rego/evaluator.go` | `Evaluate()` (3 return points), `PolicyResult` struct | ~10 |
| `pkg/aianalysis/handlers/analyzing.go` | `populateApprovalContext()` | ~1 |
| `pkg/remediationorchestrator/creator/approval.go` | `applyApprovalContext()` | ~1 |
| `api/aianalysis/v1alpha1/aianalysis_types.go` | `PolicyEvaluation` struct (new field) | ~4 |
| `api/remediation/v1alpha1/remediationapprovalrequest_types.go` | `ApprovalPolicyEvaluation` struct (new field) | ~4 |

### 6.2 Generated Artifacts (verified, not hand-tested)

| File | Change |
|------|--------|
| `api/{aianalysis,remediation}/v1alpha1/zz_generated.deepcopy.go` | No diff expected (scalar `*out = *in` already covers new string field) |
| `config/crd/bases/kubernaut.ai_{aianalyses,remediationapprovalrequests}.yaml` + 3 synced copies each | New `policyHash` optional string property under the respective nested schema |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID(s) | Status |
|-------|-------------|----------|------|------------|--------|
| BR-AI-030 | Policy audit trail for approval decisions includes policy version/hash | P1 | Unit | UT-AI-1981-001 | Done |
| BR-AI-030 / BR-AI-059 | AIAnalysis status surfaces policy hash to operator | P1 | Unit (wiring) | UT-AI-1981-002 | Done |
| BR-AUDIT-006 / BR-AI-076 | RAR spec carries policy hash for tamper-evident attribution | P0 | Unit (wiring) | UT-RAR-1981-001 | Done |
| BR-AUDIT-006 (v1.1) | RAR webhook-complete audit event (live production path) carries policy hash for reconstruction from audit trail alone | P1 | Unit (wiring) | UT-AW-2005-001, UT-AW-2005-002 | Done |
| BR-AI-030 (v1.1) | AIAnalysis Rego-evaluation audit event carries policy hash | P1 | Unit (wiring) | UT-AI-2005-001, UT-AI-2005-002 | Done |
| BR-AI-030 (v1.1) | Console/MCP-facing approval payload surfaces policy hash to operator | P2 | Unit (wiring) | UT-AF-2005-001 | Done |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| UT-AI-1981-001 | `Evaluator.Evaluate()` returns a `PolicyResult.PolicyHash` equal to `GetPolicyHash()` for a loaded policy (64-char hex SHA-256), and empty when no policy is loaded (degraded path) | RED/GREEN |

### Tier 2: Unit Tests (Wiring Proof, ADR-004 fake-client tier)

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| UT-AI-1981-002 | `AnalyzingHandler.Handle()` populates `AIAnalysis.Status.ApprovalContext.PolicyEvaluation.PolicyHash` (non-empty for a loaded real policy; empty in degraded/no-policy mode) | RED/GREEN |
| UT-RAR-1981-001 | `ApprovalCreator.Create()` copies `PolicyHash` from `AIAnalysis.Status.ApprovalContext.PolicyEvaluation` onto the created `RemediationApprovalRequest.Spec.PolicyEvaluation`, verified via fake K8s client `Get` after `Create` | RED/GREEN |
| UT-AW-2005-001 / -002 | `BuildRARApprovalAuditPayload()` (live production RAR-decision audit builder, `pkg/authwebhook`) sets `RemediationApprovalAuditPayload.PolicyHash` when `RAR.Spec.PolicyEvaluation.PolicyHash` is pinned, and omits it (unset `OptString`) when `PolicyEvaluation` is nil | RED/GREEN |
| UT-AI-2005-001 / -002 | `AuditClient.RecordRegoEvaluation()` sets `AIAnalysisRegoEvaluationPayload.PolicyHash` when a non-empty hash is passed, and omits it (unset `OptString`) when passed `""` (no policy loaded) | RED/GREEN |
| UT-AF-2005-001 | `MarshalApprovalRequestPayload()` surfaces `RAR.Spec.PolicyEvaluation.PolicyHash` on the console/MCP-facing `ApprovalPolicyPayload.PolicyHash` | RED/GREEN |

### Tier Skip Rationale

- **Integration/E2E**: not required as new tests for this issue. This augments an existing,
  already-proven control objective (SOC2 CC8.1/CC6.8 approval-decision attribution, proven via
  BR-AUDIT-006's existing integration/E2E journeys) with an additional attribute on an existing
  record; it does not introduce a new control objective requiring its own proving journey. The
  existing `test/integration/aianalysis/rego_integration_test.go` and
  `pkg/remediationorchestrator/approval_orchestration_test.go` suites serve as the regression net.

---

## 9. Test Cases (P0/P1 detail)

### UT-AI-1981-001: Evaluator populates PolicyResult.PolicyHash

**BR**: BR-AI-030
**Priority**: P1
**Type**: Unit (Ginkgo)
**File**: `pkg/aianalysis/rego_evaluator_test.go`

**Test Steps**:
1. **Given**: an `Evaluator` started with `StartHotReload()` against a real testdata policy file.
2. **When**: calling `Evaluate(ctx, input)`.
3. **Then**: `result.PolicyHash` is a non-empty 64-character hex string equal to
   `evaluator.GetPolicyHash()`.

**Acceptance Criteria**: hash matches exactly; regex `^[0-9a-f]{64}$`.

### UT-AI-1981-002: AnalyzingHandler surfaces PolicyHash to AIAnalysis status

**BR**: BR-AI-030, BR-AI-059
**Priority**: P1
**Type**: Unit (Ginkgo, wiring proof via real evaluator + fake K8s client tier)
**File**: `pkg/aianalysis/analyzing_handler_test.go`

**Test Steps**:
1. **Given**: an `AnalyzingHandler` wired with a real (testdata-backed) spy evaluator, and an
   `AIAnalysis` that will trigger manual-review-required.
2. **When**: calling `handler.Handle(ctx, analysis)`.
3. **Then**: `analysis.Status.ApprovalContext.PolicyEvaluation.PolicyHash` is non-empty.
4. **Given/When/Then (degraded)**: with `newDegradedSpyEvaluator()` (no policy loaded),
   `PolicyEvaluation.PolicyHash` is empty.

**Acceptance Criteria**: both the loaded and degraded cases behave per `GetPolicyHash()`'s own
contract (non-empty when loaded, empty string when no `fileWatcher`).

### UT-RAR-1981-001: ApprovalCreator propagates PolicyHash to RAR spec

**BR**: BR-AUDIT-006, BR-AI-076
**Priority**: P0
**Type**: Unit (Ginkgo, wiring proof via fake K8s client, ADR-004)
**File**: `pkg/remediationorchestrator/approval_orchestration_test.go`

**Test Steps**:
1. **Given**: an `AIAnalysis` with `Status.ApprovalContext.PolicyEvaluation.PolicyHash` set to a
   fixed 64-char hex value (extends the existing UT-RAR-307-003 fixture).
2. **When**: calling `ac.Create(ctx, rr, ai)`.
3. **Then**: fetching the created RAR via the fake client shows
   `rar.Spec.PolicyEvaluation.PolicyHash` equal to the fixture value.

**Acceptance Criteria**: exact value propagation, no transformation.

---

## 10. Environmental Needs

- **Unit**: Ginkgo/Gomega BDD (mandatory), `go test`, no external infra. Existing testdata Rego
  policy fixtures (`pkg/aianalysis/testdata/policies/*.rego`) reused as-is.

---

## 11. Dependencies & Schedule

No blocking dependencies. Execution order: RED (3 failing test cases, committed to fail against
current types) -> GREEN (evaluator + CRD type + wiring changes, `make generate/manifests/sync-embed`,
full suite to green) -> REFACTOR (duplication/clarity review or explicit N/A) -> final validation.

---

## 12. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/testing/1981/TEST_PLAN.md` |
| Updated Go unit tests | `pkg/aianalysis/rego_evaluator_test.go`, `pkg/aianalysis/analyzing_handler_test.go`, `pkg/remediationorchestrator/approval_orchestration_test.go` |
| Updated Go source | `pkg/aianalysis/rego/evaluator.go`, `pkg/aianalysis/handlers/analyzing.go`, `pkg/remediationorchestrator/creator/approval.go` |
| Updated CRD types | `api/aianalysis/v1alpha1/aianalysis_types.go`, `api/remediation/v1alpha1/remediationapprovalrequest_types.go` |
| Regenerated artifacts | `zz_generated.deepcopy.go` (x2), CRD YAML (x8: 2 CRDs x 4 sync locations) |

---

## 13. Execution

```bash
# Unit
go test ./pkg/aianalysis/... ./pkg/remediationorchestrator/...

# Integration regression net (no new cases, existing suite must stay green)
go test ./test/integration/aianalysis/... -run TestSuite

# Regeneration
make generate && make manifests && make sync-embed

# Full build
go build ./...
```

---

## 14. Wiring Verification (TDD Phase 4)

This plan adds fields to existing, already-wired production call chains — no new components — so
the Wiring Manifest proves the new field reaches production entry points already in use:

| Component | Production Entry Point | Wiring Code Location | Test ID(s) |
|-----------|-------------------------|------------------------|------------|
| `PolicyResult.PolicyHash` | `Evaluator.Evaluate()` | `pkg/aianalysis/rego/evaluator.go` | UT-AI-1981-001 |
| `PolicyEvaluation.PolicyHash` (AIAnalysis) | `AnalyzingHandler.Handle()`, called from `internal/controller/aianalysis` phase handlers, constructed in `cmd/aianalysis/main.go` | `pkg/aianalysis/handlers/analyzing.go` `populateApprovalContext` | UT-AI-1981-002 |
| `ApprovalPolicyEvaluation.PolicyHash` (RAR) | `ApprovalCreator.Create()`, called from `internal/controller/remediationorchestrator/reconciler.go`, constructed in `cmd/remediationorchestrator/main.go` | `pkg/remediationorchestrator/creator/approval.go` `applyApprovalContext` | UT-RAR-1981-001 |
| `RemediationApprovalAuditPayload.PolicyHash` (v1.1) | `RemediationApprovalRequestAuthHandler.Handle()` (mutating admission webhook), registered at `/mutate-remediationapprovalrequest` in `cmd/authwebhook/main.go` | `pkg/authwebhook/audit_payload_builder.go` `BuildRARApprovalAuditPayload` | UT-AW-2005-001, UT-AW-2005-002 |
| `AIAnalysisRegoEvaluationPayload.PolicyHash` (v1.1) | `AnalyzingAuditClientInterface.RecordRegoEvaluation()`, called from `AnalyzingHandler.Handle()` / `handleRegoEvaluationError()`, constructed in `cmd/aianalysis/main.go` | `pkg/aianalysis/audit/audit.go` `RecordRegoEvaluation`, call sites in `pkg/aianalysis/handlers/analyzing.go` | UT-AI-2005-001, UT-AI-2005-002 |
| `ApprovalPolicyPayload.PolicyHash` (v1.1) | `MarshalApprovalRequestPayload()`, called from the apifrontend `kubernaut_watch` MCP tool, constructed in `cmd/apifrontend/main.go` | `pkg/apifrontend/tools/approval_event.go` | UT-AF-2005-001 |

**v1.2**: `pkg/remediationapprovalrequest/audit` (`buildApprovalDecisionPayload`,
`RemediationApprovalDecisionPayload`) — the no-`cmd/`-caller row from v1.1 — was deleted rather
than wired, since `RemediationApprovalAuditPayload` (row above) already covers this control
objective from its live production path.

---

## 15. Existing Tests Requiring Updates

- `pkg/aianalysis/analyzing_handler_test.go` — the "should include policy evaluation details for
  operator visibility" case and the degraded-mode "should inform operator of degraded mode in
  approval context" case both gain a `PolicyHash` assertion (extended, not replaced).
- `pkg/remediationorchestrator/approval_orchestration_test.go` — `UT-RAR-307-003`'s fixture gains
  a `PolicyHash` field and assertion (extended, not replaced).
- `pkg/aianalysis/audit_test.go` — existing `RecordRegoEvaluation()` call sites (v1.1) gain a
  trailing `policyHash` argument (interface signature change); two new `It`s added.
- `pkg/aianalysis/analyzing_handler_test.go` — `noopAnalyzingAuditClient.RecordRegoEvaluation()`
  (v1.1) gains the matching `policyHash` parameter to satisfy the updated interface.
- `pkg/apifrontend/tools/approval_event_test.go` — new `It` added (v1.1); no existing case changed.
- `pkg/authwebhook/coverage_668_test.go` — new `It`s added (v1.1); no existing case changed.
- `pkg/remediationapprovalrequest/audit/audit_test.go` (and `audit.go`) — **deleted** (v1.2); the
  v1.1 `Context` added to this file (UT-RO-2005-001) is removed along with the rest of the
  orphaned package, not carried forward.

No pre-existing case is expected to regress, since the new field is additive and defaults to the
zero value (`""`) everywhere it is not explicitly set.

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-07 | Initial test plan, written before RED phase begins. |
| 1.1 | 2026-08-07 | Consolidated the two follow-up gaps from issue #2005 into this PR (per explicit user request): `AIAnalysisRegoEvaluationPayload.PolicyHash`, `ApprovalPolicyPayload.PolicyHash` (console), and — after a wiring-verification correction — `RemediationApprovalAuditPayload.PolicyHash` (the actual live RAR-decision audit event, `pkg/authwebhook`) rather than the originally-targeted `RemediationApprovalDecisionPayload`, whose owning package was discovered to have no `cmd/` caller. |
| 1.2 | 2026-08-07 | Per explicit user request ("delete the dead code"), removed the orphaned `pkg/remediationapprovalrequest/audit` package (`audit.go`, `audit_test.go`, UT-RO-2005-001) and its `RemediationApprovalDecisionPayload` OpenAPI schema (`oneOf` entry + full definition), regenerated the `ogen` client, and annotated DD-AUDIT-006/BR-AUDIT-006 as superseded/relocated to `pkg/authwebhook`. Removed all now-stale BR Coverage Matrix, Test Scenario, Wiring Manifest, and Existing-Tests rows referencing the deleted package. |
