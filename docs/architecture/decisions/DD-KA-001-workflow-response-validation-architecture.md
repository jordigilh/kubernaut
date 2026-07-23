# DD-KA-001: Workflow Response Validation Architecture

**Date**: July 14, 2026
**Status**: Approved
**Deciders**: Architecture Team, KubernautAgent Team, Workflow Execution Team
**Version**: 1.1
**Related**: [DD-WORKFLOW-018](./DD-WORKFLOW-018-etcd-single-source-of-truth.md) (Etcd Single Source of Truth, Change 9), DD-WE-006 (Schema Declared Dependencies), Issue #241, Issue #243, Issue #529, Issue #1661, Issue #1711
**Supersedes**: [DD-HAPI-002](./DD-HAPI-002-workflow-parameter-validation.md) (Workflow Response Validation Architecture) — in full

---

## Why a New Document (Clean Cut from DD-HAPI-002)

DD-HAPI-002 accumulated six versions (v1.0 → v1.5) describing an architecture that predates KubernautAgent's Go
rewrite — its implementation sections still show Python (`kubernaut-agent/src/extensions/incident.py`,
`WorkflowResponseValidator`), and its own v1.5 changelog entry already had to point *out* of the document to the
real, current implementation (`cmd/kubernautagent/main.go`). Rather than append a seventh changelog entry describing
yet another architectural change (Issue #1661's collapse to one validation layer, see Decision below) on top of a
document whose body no longer matches the current Go implementation, this is a deliberate clean cut: a new document,
under the `DD-KA-` prefix that matches the current service name (KubernautAgent, not HolmesGPT-API), describing the
current architecture as it actually is today. DD-HAPI-002 is fully superseded, not amended further.

---

## Context

When KubernautAgent (KA)'s LLM returns a workflow recommendation (workflow ID, container image, parameters), the
response must be validated before it is persisted to `AIAnalysis.Status.SelectedWorkflow` and, eventually, executed.
Historically (DD-HAPI-002 v1.0, December 2025) this validation was split across two independent layers: HolmesGPT-API
as the primary validator inside the LLM chat session (enabling self-correction), and the Workflow Engine as a
defense-in-depth re-check before execution. DD-HAPI-002 v1.1 removed the WE layer in favor of a single sole
validator, then Issue #243 re-added an independent WE-side re-check specifically because WE did not want to trust
KA's self-reported validation without an independent verification against its own copy of the schema fetched
directly from Data Storage (DS).

[DD-WORKFLOW-018](./DD-WORKFLOW-018-etcd-single-source-of-truth.md) (Issue #1661) changes the data-flow context this
question is asked in: `WorkflowExecution` no longer makes *any* live call to DS at execution time (Change 8 -- the
full execution snapshot, including declared parameter names, is captured once in
`AIAnalysis.Status.SelectedWorkflow` at selection time and copied verbatim into
`WorkflowExecution.Spec.WorkflowRef` by RemediationOrchestrator). This forces the question DD-HAPI-002 v1.1 and
Issue #243 disagreed about back into the open: **if WE's parameter check is retained, what would it independently
validate against, now that there is no second live fetch to compare against?**

---

## Decision

**KA remains the sole parameter-validation authority.** WE's independent Issue #243 re-check is retired. This is a
deliberate, informed trade-off -- not an oversight, and not simply reverting to DD-HAPI-002 v1.1's original
reasoning without acknowledging what changed since:

- Issue #243 added the WE-side re-check specifically so WE would not have to trust KA's self-report -- it fetched
  its own independent copy of the schema from DS to compare against. That independence had genuine value under the
  *old* data flow, where WE's copy and KA's copy were two separately-fetched reads of a live, mutable DS catalog
  that could theoretically have changed between KA's validation and WE's execution.
- Under DD-WORKFLOW-018's CRD-embedded snapshot, `DeclaredParameterNames` travels with the *same* KA-validated
  snapshot from selection time all the way to execution time -- it is not a second independent fetch, it is the
  exact data KA already validated against, propagated through the CRD chain (`AIAnalysis.Status.SelectedWorkflow`
  → `WorkflowExecution.Spec.WorkflowRef`). A second "independent" check against that same propagated data would not
  catch anything a compromised or buggy KA hadn't already gotten wrong -- it would just re-run the identical
  comparison against the identical data, adding cost without adding a genuinely independent signal.
- The two mechanisms Issue #243 was defending against -- LLM hallucination of undeclared parameters, and drift
  between KA's copy and WE's copy of the schema -- are handled differently now: hallucination is defended against
  by KA's own Step 3b undeclared-parameter stripping (Issue #241, unchanged by this decision), and drift is
  structurally impossible once there is only one snapshot, write-once immutable via a CEL `XValidation` rule on
  `AIAnalysis.Status.SelectedWorkflow` (DD-WORKFLOW-018 Change 8), rather than two independently-fetched reads of a
  live store.
- This mirrors the same "trust the sole validator" reasoning DD-HAPI-002 v1.1 originally used, now with the benefit
  of Issue #241/#243's intervening history as context: the collapse is deliberate and recorded, not a repeat of an
  argument already litigated once without documentation of why the second layer was retired.

**Recommended two-layer alternative was available and explicitly rejected.** A defense-in-depth-preserving option
existed: keep WE's independent re-check, but have it validate directly against `RemediationWorkflow.spec.parameters`
in etcd (bypassing the propagated snapshot entirely) rather than against DS. This was rejected in favor of the
one-layer model per an explicit user decision during Issue #1661 planning, on the basis that KA's Step 3b validation
(existence + type/range/enum + undeclared-key stripping) is already comprehensive, and a second layer validating
against the same source-of-truth CRD KA already validated against would still not be a genuinely independent check
-- it would only be independent if it used a different validation *method*, which was not proposed.

---

## Current Validation Architecture

### Validation Sequence (unchanged from DD-HAPI-002 v1.4, Issue #529 three-phase RCA)

| Phase | Scope | Validation | Max Attempts |
|---|---|---|---|
| **Phase 1: RCA** | LLM investigates, provides `affectedResource` | `affectedResource` format only (kind, name, optional namespace) | 3 |
| **Phase 2: Enrichment** | KA resolves owner chain, detects labels, fetches history | Infrastructure retries (exponential backoff). Fail hard on exhaustion. | 3 per service |
| **Phase 3: Workflow Selection** | LLM receives enrichment, selects workflow | Full workflow validation (existence, image, params, scope, context) | 3 |

### Step Details (Phase 3, unchanged from DD-HAPI-002 v1.3)

1. **Workflow Existence**: `workflow_id` must exist in the DS catalog (hallucination detection)
2. **Container Image / Execution Bundle Consistency**: LLM-provided value (if any) must match the catalog
3. **Parameter Schema Validation**: required/type/length/range/enum checks against the workflow's declared schema
4. **Step 3b -- Undeclared Parameter Stripping** (Issue #241): any parameter key not declared in the workflow's
   schema is silently removed in-place; a workflow with no schema has all parameters stripped

If any validation fails, errors are returned to the LLM for self-correction (max 3 attempts); after exhaustion,
`needs_human_review: true` is set.

### Step 1 (Workflow Existence) Is Unconditional -- It Applies Even When `needs_human_review` Is Already True

Step 1 has no documented exception. If the LLM's response carries a `workflow_id`, that ID **must** resolve against
the DS catalog before it is allowed to appear as structured data anywhere downstream (`AIAnalysis.Status.SelectedWorkflow`
and beyond) -- including when `needs_human_review` was already set to `true` by an earlier signal in the same response
(e.g. `investigation_outcome=inconclusive`, which the parser derives independently of workflow validation). A `workflow_id`
that cannot be resolved is not "a tentative workflow to show the operator" -- it is unvalidated data indistinguishable from
a hallucination, and per the Decision above (KA is the **sole** validator, WE performs no independent re-check), nothing
downstream re-verifies it. Any remediation idea the LLM had belongs in the RCA text, never as a structured `workflow_id`
that skipped Step 1.

This clarifies scope only -- it does not change the Decision or the validation sequence above. It closes a documentation
gap discovered during Issue #1661 triage: KA's `Validate()` short-circuits (skips the catalog-allowlist check) whenever
`HumanReviewNeeded` is already `true`, which is correct for avoiding wasted self-correction retries on a result already
headed to a human, but had left the door open for an unresolved `workflow_id` to survive un-cleared in exactly that
short-circuited path. Tracked as a KA-side implementation gap in
[Issue #1711](https://github.com/jordigilh/kubernaut/issues/1711); the fix mirrors the existing exhaustion-clearing
pattern already used for the other two "invalid workflow must not propagate" paths (self-correction exhaustion in
`validator.go`, and the API-version ambiguity gate exhaustion in `investigator_gates.go`).

### What's New (Issue #1661 / DD-WORKFLOW-018 Change 8)

KA already independently fetches and parses the workflow's schema for the Step 3/3b validation above. As of this
decision, KA **surfaces** the schema-derived fields it already computes -- `Dependencies`, `Resources`,
`DeclaredParameterNames` -- into its response payload, instead of discarding them after validation completes. This
is not new validation logic; it is exposing data the validator already has, so that
`AIAnalysis.Status.SelectedWorkflow` can persist the full execution snapshot KA already verified, for
`RemediationOrchestrator`/`WorkflowExecution` to consume without a second fetch (see DD-WORKFLOW-018 Change 8 for
the CRD-embedding design).

### What's Retired

| Layer | Responsibility | Status |
|---|---|---|
| **KubernautAgent** | Comprehensive workflow response validation with LLM self-correction | ✅ **SOLE VALIDATOR** (unchanged) |
| ~~Workflow Engine (Issue #243 re-check)~~ | ~~Independent re-fetch + re-validation of declared parameter names against DS~~ | ❌ **RETIRED** (this decision) |
| **Tekton / Job runtime** | Runtime K8s state validation (namespace exists, RBAC, image pullable) | ✅ Unchanged -- this was never a parameter-schema check |

---

## Consequences

### Positive

1. Removes a redundant network call and comparison that validated the same data against itself
2. Simplifies WE's execution path further, consistent with DD-WORKFLOW-018's broader goal of zero live DS/etcd/audit
   calls from WE at execution time
3. Makes the single point of validation authority explicit and structurally reinforced (CEL write-once immutability
   on the data KA validates), rather than relying on inter-service trust alone

### Negative / Accepted Risk

1. **No independent verification if KA itself is compromised or has a validation bug.** A bug in KA's Step 3
   validation logic is no longer caught by a second, differently-sourced check. This risk is accepted because: (a)
   KA's validation already runs inside the LLM self-correction loop with fail-closed startup behavior (DD-HAPI-002
   v1.5's fail-closed-startup decision is preserved, not superseded, by this document -- see below), and (b) the
   two-layer alternative would not have been a genuinely independent check under the new CRD-embedded data flow
   (see Decision above) -- it would have validated the same propagated data by the same method, not an
   independent source or method.
2. This is a deliberate, documented trade-off, not an unnoticed regression -- future revisitation should target a
   *methodologically* independent second check (e.g., an OPA/Kyverno policy evaluated against the live
   `RemediationWorkflow` CRD at WE-admission time) if the accepted risk above is reassessed, rather than reverting
   to the retired live-DS-fetch pattern this decision moves away from.

---

## Preserved from DD-HAPI-002 (not superseded in substance, restated here for continuity)

- **Fail-closed startup** (DD-HAPI-002 v1.5, BR-HAPI-433): when DataStorage is configured, KA's workflow validator
  MUST be constructed successfully at startup or KA refuses to start. Implemented in
  `cmd/kubernautagent/main.go`'s `buildWorkflowValidator()`. Unaffected by this decision -- KA remains the sole
  validator and must never silently run with validation disabled when DS is expected to be reachable.
- **Three-phase RCA architecture** (DD-HAPI-002 v1.4, Issue #529): RCA / Enrichment / Workflow Selection remain
  distinct phases with distinct validation scope, as summarized in the table above.
- **Undeclared parameter stripping** (DD-HAPI-002 v1.3, Issue #241): unchanged. This is KA's primary
  defense-in-depth mechanism against LLM hallucination of credentials/arbitrary env vars, and is the reason WE's
  retired Issue #243 layer was a *secondary* check, not the primary one, even while it existed.

---

## Business Requirement

**BR-HAPI-191: Workflow Parameter Validation in Chat Session** -- see
`docs/requirements/BR-HAPI-191-workflow-parameter-validation.md`. Unaffected by this decision; this document changes
*where the number of validation layers is* (one vs. two), not the substance of what KA validates.

---

## Confidence Assessment

**Confidence: 90%**

| Factor | Confidence | Notes |
|---|---|---|
| KA's existing validation is comprehensive (Steps 1-3b) | +40% | Unchanged, proven in production since DD-HAPI-002 v1.3 |
| CRD write-once immutability structurally prevents post-validation tampering | +30% | CEL `XValidation` on `SelectedWorkflow`, DD-WORKFLOW-018 Change 8 |
| Fail-closed startup prevents silent validator bypass | +15% | DD-HAPI-002 v1.5, preserved |
| Retired layer was validating already-propagated data, not independently-sourced data | +5% | Confirmed by tracing WE's Issue #243 call site before retirement |

### Remaining 10% Uncertainty

- No methodologically independent second check remains if KA's validation logic itself has a bug (accepted risk,
  documented above, not mitigated by this decision).
- Long-term monitoring of whether this trade-off needs revisiting is tracked as a follow-up consideration, not a
  blocking gap.

---

## Changelog

| Version | Date | Changes |
|---|---|---|
| 1.1 | 2026-07-23 | **CLARIFICATION**: Added explicit scope note that Step 1 (Workflow Existence) is unconditional, including when `needs_human_review` was already set true by an earlier signal (e.g. `investigation_outcome=inconclusive`). Closes a documentation gap discovered during Issue #1661 triage; the corresponding KA implementation gap (an unresolved `workflow_id` could survive un-cleared through that short-circuit) is tracked in Issue #1711. No change to the Decision or validation sequence. |
| 1.0 | 2026-07-14 | Initial version. Supersedes DD-HAPI-002 in full (clean cut, Go-era terminology). Records the collapse from two parameter-validation layers to one (Issue #1661 / DD-WORKFLOW-018 Change 8-9) as a deliberate, documented trade-off. |
