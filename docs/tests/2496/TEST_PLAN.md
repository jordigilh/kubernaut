# Test Plan: Failed Workflow Identity and Repeat-Selection Reasoning

**Test Plan Identifier**: TP-2496-v1.6<br>
**Feature**: Preserve prior workflow identity/confidence and require auditable justification for same-workflow repeats<br>
**Version**: 1.6 — synchronized final pre-implementation contract, live projection, pyramid/control proof and existing-E2E regression matrix<br>
**Latest execution revision**: 1.60 — scoped audit repairs and user-approved parallel history journeys in progress; CI runtime RCA active<br>
**Created**: 2026-10-09<br>
**Status**: Approved — implementation and wiring-first TDD in progress; full GA gates outstanding<br>
**Baseline**: `8a907bb57d3530da32de9b3b8250b53587ed6f41`, branch `issue-2496-history-aware-rca`<br>
**Implementation plan**: [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)
**Isolated spike**: [SPIKE_REPORT.md](SPIKE_REPORT.md)

This follows the [hybrid IEEE 829 template](../../testing/TEST_PLAN_TEMPLATE.md). Where that older template differs, [AGENTS.md](../../../AGENTS.md) is authoritative: ≥90% of defined in-scope business scenarios with 100% of critical scenarios proven by passing UTs, 100% IT wiring/control assessment, proving E2E journeys for all applicable control objectives, ≥80% merged structural coverage, and CHECKPOINT W during GREEN, not after REFACTOR. Raw line/branch coverage remains diagnostic rather than a literal business-logic target. The current approach confidence is 95%; this is not implementation or GA evidence.

**Current execution evidence:** the local-only `PRODUCTION_IMPLEMENTATION_EVIDENCE.md`
ledger (intentionally excluded from Git at the user's request) tracks production
checks separately from the historical spikes below. Phase
isolation and history-rendering focused checks passed. Authenticated history
IT-DS-2496-001/002 passed through Make's default all-CPU runtime. Remaining
controller, selection/correction, frozen-handoff, persistence, live projection and
E2E rows are not marked complete by those passes.

## 1. Introduction and success metrics

### Purpose

Prove that a subsequent AIAnalysis receives the identity and actual selection confidence/effectiveness of prior workflows, distinguishes the **same workflow ID** from another workflow even when names/action types/versions overlap, requires auditable justification for every same-ID repeat with a completed valid prior assessment, and preserves configured confidence/review and no-workflow behavior. The current RCA/selection result reaches the Console through the KA MCP event/decision payload and AF/A2A artifact; audit/DS reads are tested only for past-remediation/history retrieval and are not a live-result dependency. The agent assesses suitability; trusted code enforces the explanation's structure, not its truth. Assertions are about operator/system outcomes and data provenance, not private call counts or wording snapshots alone.

**Scope clarification (2026-10-09)**: The user's intent is improved context and auditability, not a separate semantic retry judge. This revision removes the former J-A review/capture-acknowledgement and actual-completion reconstruction assumptions. The isolated spike supports test-local structural composition but found no inspected Phase 3 trusted citation-resolution contract; new evidence-reference gates are not approved or required by this test plan. Prototype results do not satisfy any production UT/IT/E2E release gate below.

**Expanded scope under spike (2026-10-09)**: The user subsequently authorized a vertical spike covering recurrence-aware RCA, structured frozen evidence handoff/current-prior comparison, actual selection/correction dispatch and persisted status/audit. The eventual enhancement may include these in the same PR. Issue [#2498](https://github.com/jordigilh/kubernaut/issues/2498) is incorporated into this plan: Phase 1 resource-context/history tools are optional, while Phase 3 exposes only workflow-discovery and required result-submission tooling. A future runtime reviewer is tracked separately in [#2497](https://github.com/jordigilh/kubernaut/issues/2497), not included here. Kubernetes MCP access for discovery/selection remains prohibited by the user's constraint.

**Incorporated #2498 phase-tool contract**: The expanded matrix must prove both sides of the boundary: Phase 1 resource-context/history tools are advertised as optional and RCA completes without calling them; Phase 3 exposes only workflow-discovery and required result-submission tooling, with no Kubernetes/context-history tools. A registered but off-phase tool must not execute if forced before dispatch. Preserve authorized RCA/fleet-overlay and catalog/submission paths, use a server-bound phase rather than model arguments, and make rejection observable. The forced-call spike found an existing dispatcher gap; implementation of the minimal guard is part of this combined #2496/#2498 slice. Rejected RED attempts and unmigrated/missing-asset harness failures do not count as production contract or persistence proof.

**Expanded RED review result**: [UPSTREAM_RED_REVIEW.md](UPSTREAM_RED_REVIEW.md) records the independently reviewed fresh Luna checkpoint. It **requires changes**: exact-value mapping tests are stronger and envtest/schema prerequisites now work, but repeat acceptance, frozen observation handoff, authenticated DS persistence and production-mapper admission are not adequately tested. No expanded scenario is counted as a passed production release gate on this basis.

**Foundational history-slice evidence**: [HISTORY_SLICE_REPORT.md](HISTORY_SLICE_REPORT.md) separately records six passing representative prototype specs, actual DS/KA/first-provider request wiring, build/lint and reproducible generation. It does not turn the matrix below into a complete expanded release plan. Current-version metadata emission, full fleet/interactive RCA, frozen evidence, repeat justification and new status/audit persistence remain open; fixture-seeded workflow name/confidence must not be counted as proof of normal metadata production. Legacy AA recovery is excluded, not an outstanding gate.

**Current compatibility scope (user-directed, 2026-10-09)**: Target one v1.6 producer/history contract; v1.5 deployments, old-record recovery/backfill, version fallbacks and mixed v1.5/v1.6 rollout are not supported/tested by this enhancement. The user reports no known production deployments; this is not verified inventory. Keep current-version missing/null/malformed/zero handling, legitimately pending effectiveness, provenance, minimization and execution-safety tests. Do not delete audit records/migrations or weaken those tests to remove compatibility work.

**Old-record logging requirement (user-directed, 2026-10-09)**: Old records are retained without new selection-metadata enrichment/backfill. History preserves existing recorded information and leaves missing fields unknown. Test a bounded structured operational diagnostic when such records are encountered: query correlation/scope, affected count, missing field names and an explicit indication that no enrichment/backfill was attempted, with no parameter/secret payloads. Missing fields alone cannot establish record age/version; use an incomplete-metadata reason when provenance is unavailable. This is defensive read/log behavior, not support for an old deployment or an alternate AA recovery query. Ordinary logs do not substitute for correlated audit reconstruction evidence.

### Objectives

1. Preserve v1.6-emitted prior identity, selection confidence and effectiveness without fabricating missing values or disclosing prior parameters; label existing timestamps honestly. No old-deployment recovery/backfill requirement.
2. Present a chronological, globally numbered history across both tiers.
3. Keep every otherwise-eligible candidate discoverable; enforce same-ID repeat reasoning only at acceptance.
4. Persist required justification through real AgentSession/AA/DS boundaries, including low-confidence and partial human-review context, and reconstruct past-remediation/history context by correlation; separately prove current live Console delivery through KA MCP/A2A.
5. Reject absent/incomplete same-ID repeat justification; provide failure-aware reasoning instructions and preserve existing confidence/safety/approval gates without asserting that structure proves semantic correctness.
6. Reach manual review with **no WorkflowExecution** when the agent declines every candidate or repeat validation exhausts.
7. Enforce strict phase-scoped tool availability: optional resource-context/history tools in Phase 1, discovery-only tooling in Phase 3, and no off-phase execution before the registered tool is invoked.

| Metric | Target | Evidence |
|---|---|---|
| Changed business-logic UT | All in-scope reachable business/control behavior passes; narrowly justified defensive exclusions per user clarification in §17 | Ginkgo/Gomega, honest raw structural coverage and source-attributed branch review; no artificial tests to force literal 100% |
| IT wiring | 100% manifest rows have passing production-dispatch tests | Manifest in implementation plan, real HTTP/controller/database paths |
| Applicable FedRAMP control objectives | 100% assessed at IT | Matrix in §2.2 |
| Applicable SOC2/FedRAMP control objectives | At least one passing E2E journey per objective | Matrix and correlated journey evidence |
| ASVS objectives in scope | Business-level proving UT/IT/E2E cases | Explicit versioned requirements below |
| All tiers merged | ≥80% line coverage, deduplicated | `scripts/coverage/coverage_report.py` |
| Current-version regression | Zero newly introduced failures | Existing #2490/#1802, discovery, validation, no-workflow, approval and routing cases; no v1.5 compatibility matrix |

## 2. References and control traceability

### 2.1 Governing documents

- [Issue #2496](https://github.com/jordigilh/kubernaut/issues/2496).
- [Issue #2498](https://github.com/jordigilh/kubernaut/issues/2498), incorporated into this implementation and PR scope.
- [AGENTS.md](../../../AGENTS.md), especially preflight approval, TDD, pyramid invariant, checkpoints A/B/C/W/DD and audit-control corrections.
- [DD-KA-016](../../architecture/decisions/DD-KA-016-remediation-history-context.md); BR-KA-016 extension must be formalized before RED.
- [ADR-055](../../architecture/decisions/ADR-055-llm-driven-context-enrichment.md) for automatic Phase 2 enrichment and the optional Phase 1 resource-context role.
- [DD-KA-017](../../architecture/decisions/DD-KA-017-three-step-workflow-discovery-integration.md) for the Phase 3 workflow-discovery protocol and its strict separation from RCA/context tools.
- [BR-KA-197](../../requirements/BR-KA-197-needs-human-review-field.md), [BR-KA-200](../../requirements/BR-KA-200-resolved-stale-signals.md), [BR-KA-191](../../requirements/BR-KA-191-workflow-parameter-validation.md), [BR-KA-211](../../requirements/BR-KA-211-llm-input-sanitization.md).
- [BR-AI-088](../../requirements/BR-AI-088-configurable-confidence-thresholds.md) and BR-AI-003: existing current-confidence review and Rego approval behavior, not new repeat-specific thresholds.
- BR-INS-001/002, BR-AUDIT-005, BR-ORCH-042.5 and BR-FLEET-004 as cited by the implementation plan.
- [DD-AUDIT-003](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md), [ADR-034](../../architecture/decisions/ADR-034-unified-audit-table-design.md), DD-KA-001 / DD-KA-017 / DD-KA-005 and [DD-KA-2496](../../architecture/decisions/DD-KA-2496-history-aware-rca-and-repeat-selection.md).
- [Integration/E2E real-business-components policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md): real Kubernaut logic/storage/controllers; provider test endpoints are external fixtures, not replacements for business validation.
- **OWASP ASVS v5.0.0**, pinned official [Validation and Business Logic](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x11-V2-Validation-and-Business-Logic.md) and [Security Logging and Error Handling](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x25-V16-Security-Logging-and-Error-Handling.md). Do not mix unversioned v4 V5/V7 IDs with these v5 requirements.

### 2.2 Control objective → observable proof

Mappings are scoped evidence for this feature, not a claim of whole-system certification. SOC2 **CC7.2** is monitoring/investigative reconstruction; **CC8.1** governs approval/testing/review of this code change and is evidenced by the development record, not by relabeling a workflow-selection audit event.

| Objective and exact applicable controls | Business-level assertion | UT proof | IT proof | E2E proof |
|---|---|---|---|---|
| Trusted input structure: FedRAMP **SI-10**; ASVS **v5.0.0-2.2.1 / 2.2.2** | Malformed, missing, blank, oversized or wrong-type justification cannot authorize an in-scope same-workflow repeat; validation occurs in trusted KA code. | UT-KA-2496-004/005 | IT-KA-2496-002/003 | E2E-FP-2496-003/007 |
| Contextual consistency: **SI-10**; ASVS **2.2.3** | Identity refers to the actual historical selection; confidence comes from selected-workflow audit, not RCA; unknown/SpecDrift is not zero-effectiveness failure. | UT-DS-2496-001/003; UT-KA-2496-002/003 | IT-DS-2496-001/002; IT-KA-2496-002 | E2E-FP-2496-001/002 |
| Required sequence cannot be bypassed: **SI-10**; ASVS **2.3.1** | Initial, text/parse-recovered and self-corrected selections all satisfy membership and structural repeat checks before acceptance; decline remains possible. Existing configured confidence/review gates remain independent. | UT-KA-2496-006/007; UT-AA-2496-002 | IT-KA-2496-003; IT-AA-2496-004 | E2E-FP-2496-003/004/006 |
| Attributable audit / reconstruction: **AU-2 / AU-3**, SOC2 **CC7.2**; ASVS **16.2.1 / 16.2.4 / 16.3.3** | For explicit past-remediation/history retrieval, correlation queries recover prior attempts, selected workflow confidence/effectiveness, current justification and acceptance/rejection, with UUID, actor, action and outcome. Validation bypass attempts are recorded. | UT-KA-2496-009; UT-AA-2496-001 | IT-KA-2496-005; IT-AA-2496-002 | E2E-FP-2496-001/003/004 |
| Live current-result delivery: **AC-4 / SI-10**; ASVS **2.2.1 / 16.2.4** | The current safe workflow/repeat projection reaches the Console from the KA MCP complete/event or interactive decision payload through AF/A2A without an audit/DS read; past-remediation retrieval remains a separate path. | UT-AF-2496-001 | IT-AF-2496-001/002 | E2E-FP-2496-001/003/007 |
| Honest timeline: **AU-3**, **CC7.2**; ASVS **16.2.1**, timestamp representation part of **16.2.2** | Numbered oldest-first attempts label existing workflow-created/recorded timestamps honestly, without claiming they are verified execution completion. | UT-DS-2496-002; UT-KA-2496-001 | IT-DS-2496-001; IT-KA-2496-001 | E2E-FP-2496-001 |
| Information-flow / least privilege: **AC-4 / AC-6**; ASVS **2.2.3** | A same-name/hash/UUID fixture on another target/origin cluster cannot contribute prior identity/confidence to this attempt; no new arbitrary audit/K8s read permission is granted. | UT-DS-2496-003 | IT-DS-2496-002 | E2E-FP-2496-005 |
| Phase-scoped least privilege: FedRAMP **AC-6 / SI-10**; ASVS **2.2.1 / 2.2.3** | Phase 1 may optionally call resource context; Phase 3 exposes and executes only discovery/submission tools; forced off-phase calls are blocked before external execution. | UT-KA-2498-001/002 | IT-KA-2498-001/002 | E2E-FP-2498-001 |
| Minimized and injection-safe new evidence: **AC-4 / SI-10**; ASVS **16.2.5 / 16.4.1** for audit privacy/log encoding | Prior parameter sentinel never reaches model context/new history projection; new identity/reason fields are sanitized before prompt use and safely encoded in persisted audit JSON. | UT-KA-2496-008/009 | IT-DS-2496-001; IT-KA-2496-005/006 | E2E-FP-2496-001/005 |
| Secure degradation and observable failure: **SI-10 / AU-3**; ASVS **16.3.4 / 16.5.2 / 16.5.3** | Failed history acquisition or exhausted structural correction produces observable error/review, not a fabricated successful validation. A below-floor current-confidence repeat reaches review with its explanation preserved and no automatic execution. Existing audit-delivery errors remain observable. | UT-KA-2496-005/006; UT-DS-2496-004; UT-AA-2496-002 | IT-DS-2496-003; IT-KA-2496-005; IT-AA-2496-003/004 | E2E-FP-2496-003/006 |

**Scope limits**: Prompt-injection defense supports SI-10/AC-4 but ASVS 16.4.1 specifically proves **log encoding**, not generic LLM safety. Timestamp tests prove UTC/explicit-offset representation; infrastructure clock synchronization is an existing operational prerequisite, not established by these unit tests. AU-9/AU-11/SC-8 remain inherited storage/retention/TLS controls; this change must not weaken them, but it introduces no new cryptographic, retention or transport implementation.

## 3. Risks and mitigations

Supplementary deployment proof **IT-PLATFORM-2496-001** (BR-KA-016/DD-KA-2496):
the existing Helm schema and KA ConfigMap must materialize the independent
65,536-byte default, honor a positive operator override, and reject explicit
zero/negative values. IT-KA-2496-008 proves the rendered configuration's runtime
boundary through the production investigator builder. Observation-privacy cases
under UT-KA-2496-008 additionally cover Secret context before metadata removal,
safe credential redaction/encoding and complete multi-document YAML capture.
Supplementary UT-KA-2496-008/010 cases exercise nested/array Secret context,
mandatory packet safety with a customized tool-output pipeline, malformed-input
and non-finite serialization failures, whole-packet (not observation-only) budget,
25-attempt completeness/deep-copy, concurrent correlation isolation and cleanup.

Further malformed-YAML cases cover a broken Secret resource and a valid Secret
followed by a malformed document: no raw fallback or partial packet may reach
selection. IT-KA-2496-008 distinguishes observation overflow from complete-packet
overflow and source serialization failure via observable production diagnostics;
every failure requests review before any selection-provider call.

| Risk | Impact | Priority / mitigation cases |
|---|---|---|
| Same action/name mistaken for same workflow; shared names conflate IDs, versions or action types | Wrong candidate discouraged or repeat guard missed | P0: UT-KA-2496-003; IT-KA-2496-002; approval of identity semantics |
| Unknown or SpecDrift becomes proven zero failure | Unjustified manual review / suppressed useful retry | P0: UT-KA-2496-002; IT-DS-2496-001 |
| New validator works only on first submit | Unsafe retry bypass | P0: UT-KA-2496-007; IT-KA-2496-003 |
| Three non-empty strings mistaken for genuine evidence | Structural validation overstated as proof of reasoning | State the auditability guarantee explicitly; UT-KA-2496-006 proves instructions/outcome handling, not arbitrary narrative adequacy. Evidence-grounding quality is characterized by the spike/rehearsal, not a new CI semantic gate. |
| Field pruned by CRD or typed audit mapping | Operators cannot reconstruct repeat selection | P0: UT-AA-2496-001; IT-AA-2496-001/002; IT-KA-2496-005 |
| Producer/projection borrows another workflow, RR or origin cluster's metadata | Wrong historical evidence / disclosure | P0: IT-RO-2496-002; IT-DS-2496-002; E2E-FP-2496-005; no AA recovery/WFE execution-cluster join |
| Existing history query or new rendering silently truncates | Incomplete history / expensive investigations | P0: UT-DS-2496-004; IT-DS-2496-003; existing #2490 resource guards; no new metadata query or N+1 fan-out |
| Timestamp/order changes alter RO blocking windows | Safety/availability regression | P0: IT-RO-2496-001; existing #1802/#2490/BR-ORCH-042 tests; keep routing/tier semantics unchanged |
| Live MCP/A2A projection or historical audit/typed-field loss | The current Console card is incomplete, or past-remediation retrieval loses operator context | P0: IT-AF-2496-001/002; IT-KA-2496-005; IT-AA-2496-001/002; retain inherited observable delivery behavior, with no new synchronous acknowledgement gate or live DS audit lookup |
| Historical/RCA confidence substituted for current selection score | Configured review/approval bypass | P0: UT-AA-2496-002; IT-AA-2496-004; E2E-FP-2496-006 |
| New prompt text leaks prior parameters or injects instructions | External disclosure / altered selection | P0: UT-KA-2496-008/009; IT-KA-2496-006; E2E-FP-2496-005 |
| Phase 3 gains RCA/Kubernetes/context access or RCA incorrectly requires context call | Least-privilege violation / brittle investigation | P0: UT-KA-2498-001/002; IT-KA-2498-001/002; E2E-FP-2498-001 |

## 4. Scope and design decisions

Concrete contracts approved on 2026-10-10: independent positive
`ai.investigation.maxEvidenceHandoffBytes` (default 65,536 bytes); non-executable
sibling `selection_assessment` / AA `selectionAssessment`; NoExecution does not
establish a completed valid workflow assessment. Frozen-handoff supplementary
proofs: **IT-KA-2496-007** pre-transform sanitized capture and whole-packet delivery;
**IT-KA-2496-008** config-to-production budget/review; **IT-KA-2496-009** autonomous/
interactive and concurrent-request isolation. Unit **UT-KA-2496-010** covers freeze,
digest, deep-copy, unknowns and budget failure. Existing E2E gates are unchanged.

Supplementary **UT-KA-2496-010** verifies the exact complete-wire byte boundary
(digest and timestamp included), digest independence from runtime metadata and
input immutability. An unparseable target API group/version must remain explicitly
unknown without changing the existing API-version gate or downstream input.
**IT-AA-2496-001** exercises real KA mapping, AgentSession admission, AA
reconciliation and persisted status across successful, explicit-zero,
partial-review and rejected outcomes; selected metadata must reject edits/removal
after `selectedAt` while admitting identical resubmits.

Test real DS history projection, both tiers, KA context/selection/validation, AgentSession result mapping, AA admission/status/audit, existing no-WE/manual-review dispatch, strict phase-scoped tooling, and correlated lifecycle journeys. Do not test EM scoring internals, new retry policy, catalog exclusion or interactive operator selection as part of this issue.

Cases reflect the revised **H-B/J-B** proposal: a single v1.6 RO selection-metadata contract with no legacy AA recovery; structured justification, agent-owned suitability and existing confidence controls. The user selected the no-compatibility scope; the overall implementation plan and proposed field/matching contract still require approval. No separate reviewer, evidence-reference registry, higher repeat threshold, automatic score penalty, WFE completion join or synchronous audit-capture gate is included. The former draft-only IT-KA-2496-004 semantic-review scenario is removed, not turned into a skipped spec. Compatibility cases are removed from the plan, not deferred with skipped tests.

### Exact live Console projection

The autonomous KA complete-event path and interactive `present_decision` path must preserve the same additive, allow-listed projection:

```json
{
  "selected_workflow": {
    "workflow_id": "catalog-uuid",
    "workflow_name": "catalog-name",
    "workflow_selection_confidence": 0.84,
    "repeat_justification": {
      "evidence_basis": "...",
      "prior_outcome_relevance": "...",
      "expected_mechanism": "..."
    },
    "repeat_justification_required": true,
    "prior_workflow_match": true,
    "repeat_validation_outcome": "accepted",
    "handoff_digest": "sha256:..."
  }
}
```

`rca.confidence` remains RCA confidence and must not populate `workflow_selection_confidence`. No parameters, raw audit records, secrets, prior RR identifiers or full frozen handoff may cross this live boundary. No-workflow results omit `selected_workflow`; malformed/oversized data follows the existing observable fallback.

## 5. Approach and pass/fail/suspension

- **UT proves logic**: pure projection, missing-vs-zero semantics, matching, chronology, parser/schema, bounded explanation and trusted result handling. Ginkgo/Gomega only.
- **IT proves wiring**: real authenticated HTTP middleware/router/handler/DB chain; real investigator/client/catalog/enrichment path; envtest AA/RO reconciliation and persisted status. Use external LLM test endpoints, not mocked Kubernaut validators/controllers.
- **E2E proves journey**: Kind deployed DS/KA/AA/RO and applicable WFE/EM/Notification components; drive real RR/AgentSession/controller flow. External Mock LLM deterministically selects/declines/revises; real platform code makes acceptance, persistence and routing decisions.
- Every BR in scope has UT + IT, and every control objective in §2.2 has a complete E2E journey. No tier is skipped.
- Fixtures use unique RR IDs, resource names/namespaces/clusters; HTTP test endpoints bind `:0`. No shared mutable per-investigation history or validator state.

### Pyramid invariant enforcement

The plan will not accept a UT-only feature as implemented:

- **UT:** proves real business logic—identity/repeat predicates, unknown-vs-zero semantics, bounded justification validation, chronological history, sanitizer behavior, explicit confidence provenance and safe projection.
- **IT:** proves every Wiring Manifest production boundary—RO/DS producer and HTTP path, KA enrichment/selection/correction, phase dispatch, KA MCP/AF/A2A projection, AgentSession/AA status and typed audit persistence. Internal mocks are prohibited; only external providers, databases and Kubernetes APIs are substituted according to `AGENTS.md`.
- **E2E:** proves complete business/control journeys—first selection, different workflow, justified repeat, rejected repeat/manual review, no-workflow/no-WE, target isolation, current-confidence review, live Console delivery and #2498 phase isolation.

Each scenario is mapped to a BR and, where applicable, FedRAMP SI-10/AC-4/AC-6/AU-2/AU-3, SOC 2 CC7.2, and pinned OWASP ASVS v5.0.0 validation/logging objectives in §2.2. A passing unit or serialization test cannot mark the corresponding integration or end-to-end control row complete.

**PASS**: all specified P0 scenarios pass, wiring/control matrices are complete, coverage targets meet §1, generated schemas admit/preserve the data, and regressions pass. **FAIL**: a candidate disappears because of failed history; a required justification is structurally missing/incomplete but accepted; a required field is lost; configured confidence/review gates are bypassed; a no-workflow/rejected result creates WE; history is wrong-scope/incomplete; any required objective lacks runtime proof. A structurally complete explanation is not rejected by a new narrative-quality oracle.

**Suspend** for disabled Engram, unapproved BR/DD/implementation contract, unsafe provenance ambiguity, broken baseline/build, unavailable infrastructure, or cascading failure. Escalate with evidence/options; do not replace business components with mocks or weaken assertions to claim success. Resume only after the specific blocker is resolved. Runtime IT/E2E not run means **blocked/not executed**, not PASS. An isolated spike/prototype is not release evidence.

## 6. Test items and version identification

| Tier | Existing seams to extend |
|---|---|
| UT | `pkg/datastorage/remediation_history_logic_test.go`, `remediation_history_handler_test.go`, `remediation_history_repository_test.go`; `internal/kubernautagent/{enrichment,prompt,parser,investigator,agentsession,audit}` tests; AA response/audit tests |
| IT | `test/integration/datastorage/remediation_history_integration_test.go` and query-fix tests; `test/integration/kubernautagent/{enrichment,investigator,audit}`; `test/integration/aianalysis/reconciliation_test.go`; `test/integration/remediationorchestrator` |
| E2E | Existing `test/e2e/kubernautagent/three_step_discovery_test.go` / audit harness; `test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go`, `12_problem_resolved_no_we_test.go`, `13_escalation_lifecycle_test.go` harness patterns |
| Schemas | `api/openapi/data-storage-v1.yaml`, generated DS client/embedded specs; AA type/deepcopy/CRD copies; shared typed justification |

Checkpoint B discovery of these existing seams was performed before this plan. New scenario files are allowed only where extending the existing suite would be clearer; do not create a duplicate harness. Re-identify implementation branch SHA after approval; #2490 must remain in the baseline.

## 7. BR coverage matrix

All rows are **Specified**; execution status is tracked in the production evidence
ledger. A passing subset is not completion of an entire requirement/control row.

| BR / contract | Unit | Integration | E2E |
|---|---|---|---|
| BR-KA-016 + #2496 history fidelity | UT-DS-2496-001/002/003; UT-KA-2496-001/003 | IT-DS-2496-001/002; IT-KA-2496-001 | E2E-FP-2496-001/002/005 |
| BR-INS-001/002 evidence interpretation | UT-KA-2496-002 | IT-DS-2496-001; IT-KA-2496-002 | E2E-FP-2496-001/002 |
| BR-KA-016 + #2496 conditional repeat acceptance | UT-KA-2496-003/004/005/006/007 | IT-KA-2496-002/003 | E2E-FP-2496-001/003 |
| BR-AI-088 / BR-AI-003 existing confidence/review controls | UT-AA-2496-002 | IT-AA-2496-004 | E2E-FP-2496-006 |
| BR-KA-197 / BR-KA-200 safe decline | UT-KA-2496-006; AA existing outcome UT extensions | IT-AA-2496-003; IT-RO-2496-001 | E2E-FP-2496-003/004/006 |
| BR-KA-191 existing validation/membership safety | UT-KA-2496-007; existing catalog/parameter UT | IT-KA-2496-002/003 | E2E-FP-2496-003 |
| BR-KA-211 / DD-KA-005 minimized context | UT-KA-2496-008/009 | IT-KA-2496-006; IT-DS-2496-001 | E2E-FP-2496-001/005 |
| BR-AUDIT-005 required decision reconstruction | UT-KA-2496-009; UT-AA-2496-001 | IT-KA-2496-005; IT-AA-2496-002; IT-RO-2496-002 | E2E-FP-2496-001/003/004/006 |
| BR-ORCH-042.5 / BR-FLEET-004 regression constraints | UT-DS-2496-003 plus existing routing UT | IT-RO-2496-001; IT-DS-2496-002 | E2E-FP-2496-005; existing routing E2E |
| BR-KA-017 / DD-KA-017 + #2498 phase isolation | UT-KA-2498-001/002 | IT-KA-2498-001/002 | E2E-FP-2498-001 |

## 8. Test scenarios

### Unit scenarios — logic and schema

| ID | Business outcome and essential assertions |
|---|---|
| UT-DS-2496-001 | Both tiers preserve the mandatory stored workflow ID and catalog-authoritative name/selection confidence from the v1.6 RO metadata contract. A fixture with RCA `0.99`, selected workflow `0.94`, score `0`, resolved `false` yields prior selection `0.94`; missing/null/malformed identity cannot become a fabricated value or fallback match, valid `0` stays present. Old records are not rewritten/enriched; incomplete historical identity metadata is identified for bounded logging without guessing age/version and cannot establish a same-workflow repeat. Empty history, known zero and pending effectiveness do not trigger that identity-gap diagnostic. Never source approval-string/provider/RCA confidence or query old AA events as fallback. |
| UT-DS-2496-002 | Existing workflow-created/recorded timestamps retain their instant and are not presented as actual execution completion. UTC/offset normalization preserves the instant. Routing's legacy timestamp is not silently repurposed or its windows changed. |
| UT-DS-2496-003 | Metadata binding rejects mismatched RR/workflow/provenance and handles ambiguity deterministically; no wrong-target/cluster record can become prior selection context. |
| UT-DS-2496-004 | Current-version metadata projection errors and existing history query overflow produce observable failure rather than fabricated/partial successful history. Existing #2490 result/depth/time guards remain; no compatibility metadata query is introduced. |
| UT-KA-2496-001 | Older Tier 2 + newer Tier 1 entries render in global oldest-first attempt order, with stable numbering/ties and confidence `0.94 → 0.90 → 0.84` separated from effectiveness. No mutation of source slices or accidental effectiveness-trend reversal. |
| UT-KA-2496-002 | Table-driven known-ineffective-attempt predicate: known zero + explicit unresolved + valid completed assessment in the existing RO→WFE→EM flow qualifies for failure-aware history guidance after one attempt; nil score, nil resolution, resolved signal, pending assessment and SpecDrift do not. This predicate does not decide whether repeat justification is required. Do not rely on a synthetic workflow-created `outcome=Success` absent from production. Recurrence with previous resolution is only durability evidence; zero effectiveness does not establish the failure cause. |
| UT-KA-2496-003 | Exact mandatory workflow-ID matching and mandatory catalog-authoritative names distinguish workflows in the same action family, including shared names across versions/action types. Missing ID or name invalidates a selection; there is no name-only, revision-only, or action-type fallback and no action-family ban. |
| UT-KA-2496-004 | First selection and a different workflow ID need no justification. A same-ID workflow with any completed valid prior assessment—including successful/remediated and ineffective outcomes—needs all three parts; pending, invalid, unknown and SpecDrift history does not trigger the requirement. Valid justification does not remove the workflow from discovery or change its confidence. Schema requiredness is conditional at the trusted runtime boundary. |
| UT-KA-2496-005 | Missing/null/blank/partial/wrong-type/unknown-fields-in-place-of-required-parts/oversized objects cannot authorize a repeat. Nested and flat parsing preserve valid objects. Structural validation errors are observable; no default pass. |
| UT-KA-2496-006 | Actual prompt explains that symptom/RCA fit alone is not a same-ID repeat justification, asks for evidence basis/prior-outcome relevance/expected mechanism and acknowledges unknown causes. Structurally complete rationale is not judged by another model or keyword filter. Explicit no-workflow remains terminal and needs no justification. Tests prove application instructions/handling, not correctness of arbitrary reasoning. |
| UT-KA-2496-007 | Initial submit, text fallback, parse-recovery and successive correction proposals all obey membership/parameters/repeat checks. A corrected different workflow is not incorrectly bound to the previous proposal's review result. Cancellation/budget exhaustion never authorizes a retry. |
| UT-KA-2496-008 | New workflow/name/reason fields cannot inject instructions into actual Phase 3 context. Prior parameters/bundle sentinel is absent even when source audits contain it. New free-text evidence uses existing credential/injection sanitization, with observable failure. |
| UT-KA-2496-009 | AgentSession and typed audit projection preserve justification, server-bound prior links and validation outcome; JSON/log metacharacters cannot forge events. Sensitive sentinel is absent from newly projected explanation/context. AgentSession unit cases separately prove accepted/below-floor/rejected/no-workflow mapping, independent unknown/zero confidence and false/unevaluated repeat state; rejected invalid UTF-8 or oversized optional parts are omitted, never truncated, while accepted execution snapshots remain intact. |
| UT-KA-2496-010 | The frozen evidence packet is target-scoped and preserves the target's cluster ID, API group/version, kind, namespace/name, target-resolution provenance, and observed/recorded timestamps when available. Cross-cluster or cross-API-group history cannot be treated as target evidence; unavailable target values remain unknown. Existing correlated tool/audit records provide source reconstruction without duplicating source metadata in every evidence item. |
| UT-KA-2496-011 | The shared live selection DTO preserves explicit zero/false, omits unknown/non-finite/out-of-range confidence without borrowing RCA confidence, and deep-copies selected/rejected context. Invalid identity or blank/invalid UTF-8/oversized text cannot manufacture an executable selection or apparently valid truncated reasoning. Enforce the budget again after credential sanitization; `ToMap()` returns JSON-shaped values without private execution/prior-link contents. Test-only coverage completion for the approved AC-4/SI-10 privacy contract; no new production behavior. |
| UT-AF-2496-001 | The live autonomous result projection maps the bounded KA MCP `complete` payload into the AF/A2A `investigation_summary` artifact without a DS audit read. Safe workflow/repeat fields are preserved; malformed or oversized structured data follows the existing observable artifact fallback. |
| UT-AF-2496-002 | The interactive `kubernaut_present_decision` projection preserves the same bounded repeat-justification object in its A2A `DataPart`, while existing server-grounding and cluster-attribution rules remain authoritative. Saved malformed/unserializable selection context and ADK read/write failures cannot revive stale workflow context; transport failures are observable, caller presentation data stays unchanged, and presenting reasoning does not grant execution consent. |
| UT-AA-2496-001 | AA maps valid justification into typed `SelectedWorkflow` and audit payload on success, low-confidence and partial human-review preservation paths, without altering selection confidence/immutable snapshot. First/different selection remains valid without the optional field. No-workflow clears/omits selection rather than retaining stale repeat state. |
| UT-AA-2496-002 | Configured low-confidence floor and later Rego threshold use the current selected-workflow score, not historical selection/RCA confidence. Boundary/override cases preserve existing review and auto-approval semantics; a repeat explanation does not bypass either gate. |
| UT-KA-2498-001 | Phase 1 tool definitions include both resource-context/history tools as optional capabilities; Phase 3 definitions include only workflow-discovery and required result-submission tooling (plus existing common tooling), and exclude Kubernetes/resource-context/history tools. |
| UT-KA-2498-002 | A Phase 1 investigation can submit RCA without calling a resource-context/history tool; no mandatory-call validation or synthetic tool invocation occurs. |

### Integration scenarios — production wiring

| ID | Real entry path and observable business output |
|---|---|
| IT-DS-2496-001 | Authenticated history HTTP route → real PostgreSQL audit projection → response parsed by generated client. Both tiers return actual identity/selection confidence/effectiveness, safe missing-vs-zero semantics and no parameter sentinel from v1.6 RO records. An additional old-record fixture lacks new metadata: existing data remains visible, fields stay unknown, source audit JSON/hash is unchanged, and the production read path emits the bounded correlated diagnostic that enrichment/backfill was skipped. Capture real logger output and verify both parameter sentinel key and value absent. No old-record AA recovery/backfill path. Existing timestamp semantics remain unchanged. |
| IT-DS-2496-002 | Same hash/name/correlation fixtures on other targets/origin clusters: only correctly bound RO metadata is returned. A legitimate different execution cluster must not be confused with origin provenance. Exercise HTTP query scoping, not just repository helpers; no AA recovery or new WFE join. |
| IT-DS-2496-003 | Real route + failed/cancelled existing history query or overflow: non-success history response/error, never partial `200` or fabricated empty-success history. Show the existing #2490 protections still work; no additional compatibility query. |
| IT-KA-2496-001 | Autonomous investigator → real DSAdapter/Enricher → actual provider request contains chronological, numbered history with stable workflow identity and distinct prior selection confidence. Use a capturing external provider endpoint, not a stub prompt builder. |
| IT-KA-2496-002 | Real catalog discovery returns the previously failed candidate and a different workflow in the same action family. First/different selections pass without justification; the repeated candidate requires the approved contract. |
| IT-KA-2496-003 | Through the production investigator, first response/text/parse-recovery/self-correction cannot bypass required justification. Valid revised repeat or different workflow can complete; exhaustion cannot produce an executable accepted selection. |
| IT-KA-2496-005 | Selection result → actual KA typed audit mapper → DS ingestion/query for compliance and past-remediation/history retrieval. Raw persisted JSON and generated client both contain the justification/validation/prior links with complete envelope metadata, proving no DTO drop; this path is not used to deliver the current live Console result. |
| IT-AF-2496-001 | Autonomous KA investigation → terminal MCP `complete` payload → AF bridge → A2A `investigation_summary` artifact. The safe workflow/repeat projection reaches the Console without any DS audit read; malformed/oversized structured data follows the existing observable artifact fallback. |
| IT-AF-2496-002 | Interactive KA decision function arguments → AF `kubernaut_present_decision` conversion → A2A artifact. The current decision card contains the safe workflow/repeat projection and does not query audit/history data as a transport. |
| IT-AF-2496-003 | The documented `investigation_summary.v1` schema accepts the additive safe projection, and the real `EmitArtifact` gob/JSON boundary preserves valid bounded values while dropping unsafe structured data to the existing text fallback. |
| IT-KA-2496-006 | Audit-derived injection/secret fixtures → real enrichment/prompt → provider request; adversarial text is sanitized, parameters are absent, and sanitization failure is observable. No mocked internal sanitizer. |
| IT-AA-2496-001 | Envtest AA reconciler consumes completed real AgentSession results on successful, low-confidence and partial-review paths; admitted status read-back preserves `repeatJustification` and selected confidence where a selection is retained. CRD pruning/immutability behavior is exercised, not inferred from Go serialization. |
| IT-AA-2496-002 | AA production completion/review handling → typed audit → real DS query preserves required justification and current selected-workflow confidence/context; actor/UUID/action/outcome/correlation are present. |
| IT-AA-2496-003 | A non-empty catalog deliberately declined via no-workflow, and an invalid repeated proposal, produce the correct human-review/outcome classification, not self-resolution or a stale selected workflow. |
| IT-AA-2496-004 | Real AA reconciliation and Rego evaluation use configured lowConfidenceFloor/confidenceThreshold for current selections. Below-floor repeats preserve explanation for operators without progressing automatically; cases between the two controls follow existing approval semantics, and higher-confidence repeats still require normal approval. Exercise configuration-to-handler wiring, not a hard-coded threshold test. |
| IT-RO-2496-001 | RO controller handles AA no-workflow/rejected-repeat outcome: operator manual-review evidence and no WE. Separately, approved selection still respects existing ineffective-chain/approval constraints. |
| IT-RO-2496-002 | v1.6 RO production workflow-creation dispatch copies workflow ID/name/selection confidence from the actual AA snapshot into the typed audit. Query the stored DS record and subsequent history to prove the current-version metadata producer is wired and RCA confidence is not substituted. Missing/malformed selection metadata must not borrow an older AA record or manufacture a score. |
| IT-KA-2498-001 | Production investigator phase dispatch exposes only the configured Phase 1/Phase 3 allow-lists: RCA includes optional resource context; Phase 3 includes discovery/result submission but no Kubernetes/resource-context/history tools. |
| IT-KA-2498-002 | A forced Phase 3 call to a registered RCA/Kubernetes/context tool is rejected before registry/external execution using trusted server phase state; the rejection is observable and permitted catalog/submission paths remain functional. |

### E2E scenarios — journeys, not canned response validation

| ID | Journey and required final evidence |
|---|---|
| E2E-FP-2496-001 | A real first RR selects workflow A at `0.94`; WFE completes and real EM records known zero/unresolved. A later RR retrieves A's past-remediation identity/confidence/effectiveness with honestly labeled dates, selects A with a structurally complete repeat explanation, persists it through AgentSession → AA → DS, and delivers the current safe result through KA MCP → AF/A2A. A remains in actual discovery results; no prior parameters reach the provider. Query history/audit only for the past-remediation journey, not as the live Console transport. This proves auditable selection, not semantic truth; existing RO policy can still block execution independently. |
| E2E-FP-2496-002 | After A's ineffective attempt, a later RR selects workflow B (same action family allowed) without repeat justification. B is not falsely treated as A, and its normal approval/execution path remains available. |
| E2E-FP-2496-003 | Provider proposes A again with missing/incomplete justification and fails bounded correction. The actual platform routes manual review/no WE. Query persisted failure/structural-validation evidence, not just a warning string. No semantic judge or RCA-keyword blacklist is part of this journey. |
| E2E-FP-2496-004 | All catalog candidates remain present, but provider calls `submit_result_no_workflow` as unsuitable. Actual AA has `selectedWorkflow=null`, `no_matching_workflows` review reason; RO creates no WE and the configured operator notification path is observable. |
| E2E-FP-2496-005 | Another target/origin cluster with identical names/spec/identity and malicious/sensitive history does not contaminate this RR's decision. Different origin/execution clusters do not widen AA selection-history provenance. Correlated raw audits are safely encoded/minimized. |
| E2E-FP-2496-006 | Provider supplies a structurally complete repeat explanation but current selection confidence is below the configured floor, despite high prior selection/RCA confidence. Real AA/RO reach review/no WE with explanation preserved for the operator. A companion approval-path case proves the configured Rego threshold remains independent and unbypassed. |
| E2E-FP-2496-006a | The companion supplies current selection confidence `0.75`, independent RCA confidence `0.99`, and complete repeat reasoning in a production environment. Deployed AA/Rego require an actual RemediationApprovalRequest and operator notification; no WE exists before approval. The operator then approves normally, with all subsequent RO safety controls unchanged. This environment-policy journey does not alone prove an operator-overridden numeric threshold. |
| E2E-FP-2496-007 | A first RR selects workflow A, WFE completes and EM records a successful/remediated outcome, then the fault is intentionally reintroduced. A later RR selects the same `workflow_id`; despite the prior success, a structurally complete explanation of current evidence, prior-outcome relevance and expected mechanism is required and persisted. A different workflow ID remains eligible without that explanation. |
| E2E-FP-2498-001 | Given an autonomous investigation, when Phase 1 runs without a resource-context call and Phase 3 attempts only discovery/submission, then RCA completes, Phase 3 receives no Kubernetes/context-history tool, and any forced off-phase call cannot reach an external tool. |

For E2E-FP-2496-001, generate prior lifecycle records through actual controllers/audit emitters, not direct SQL seeding alone. Keep assessment controls/configuration realistic and the target signal demonstrably unresolved; do not fabricate the aggregate score to satisfy a fixture. UT/IT may seed authoritative records to isolate data-contract cases. Deterministic provider behavior proves application enforcement, not general real-LLM quality.

Extend that journey with a supplementary immutable historical-record fixture missing the new selection metadata. When the deployed history path encounters it, existing evidence remains available, unavailable fields stay unknown and correlated operational logs explain that enrichment/backfill was not attempted without disclosing seeded secrets. This fixture proves safe reading/observability only; it neither replaces the current-version producer journey nor requires deploying an older binary.

## 9. Detailed P0 test cases

### Shared rehearsal fixture

Use a target/cluster-scoped chain with two prior completed workflow-A attempts: selection confidence `0.94`, then `0.90`; effectiveness `0`; `signal_resolved=false`. Current RCA confidence may be `0.99`, deliberately distinct. The third proposal may be `0.84`; use a separate below-floor current score for the review journey. Provide another workflow B with the same action type and include a name/version-collision fixture where the IDs differ. Include a prior-parameter sentinel in source storage, never in allowed history/model input. One old attempt falls into Tier 2 to prove global order. Add a companion successful/remediated workflow-A recurrence fixture to prove that repeat justification is outcome-agnostic. Where actual thresholds independently block RR3 execution, assert selection/audit and routing-block evidence rather than weakening RO thresholds. Do not pretend zero effectiveness identifies the prior failure mechanism.

| Case | BR / priority | Given → When → Then / measurable expected outcome |
|---|---|---|
| IT-DS-2496-001 | BR-KA-016; BR-AUDIT-005; P0 | Given real scoped RO/AA/EM records with distinct selected/RCA confidence, when the authenticated HTTP endpoint is called, then both raw JSON and ogen client return actual selected confidence/identity, complete disjoint tiers, unchanged timestamp semantics and no prior-parameter sentinel. |
| IT-KA-2496-001 | BR-KA-016; BR-KA-211; P0 | Given the shared fixture and a real DS endpoint, when a production investigator reaches Phase 3, then the external provider receives attempts in chronological order with unique stable numbers, `workflow_selection_confidence` explicitly named, zero effectiveness/unresolved shown, and failed-strategy guidance that leaves retry/no-workflow available. |
| IT-KA-2496-002 | BR-KA-016; BR-KA-191; P0 | Given A failed and B shares its action, when discovery runs and B is selected without a justification, then A and B were both offered, B passes the normal validator, and no action-family ban/repeat requirement is invented. |
| IT-KA-2496-003 | BR-KA-016; BR-KA-191; P0 | Given a repeated A proposal missing required explanation parts, when production correction runs, then an accepted result requires structurally complete justification; exhaustion instead reaches review. A complete explanation is not passed through another semantic judge. |
| IT-AA-2496-001/002 | BR-KA-016; BR-AUDIT-005; P0 | Given an accepted or below-floor repeat result, when AgentSession completion drives AA reconciliation/auditing, then admitted status and queried raw DS records retain required explanation and selected confidence/context with server-bound prior links. No mapper loss or schema pruning. |
| IT-AA-2496-004 | BR-AI-088; BR-AI-003; P0 | Given configured review/approval controls and high prior/RCA confidence, when a repeat arrives below the current-selection floor, then the real handler routes review with explanation preserved; an above-floor selection still follows normal Rego approval. |
| E2E-FP-2496-004 | BR-KA-197/200; BR-AUDIT-005; P0 | Given unsuitable but discoverable candidates, when the provider explicitly declines, then actual terminal AA/RO state requires review, selection is null, no WE exists for the RR, and correlated audit/notification evidence explains the outcome. |
| E2E-FP-2496-005 / E2E-FLEET-2496-005 | BR-KA-016; BR-FLEET-004; P0 | Given a producer-generated prior lifecycle and colliding valid historical fixtures on another origin or target, when this RR runs, then only its bound history may establish a repeat; private fixture contents stay absent from current audit/live context and source rows remain unchanged. Positive controls must prove both distractors are completed Full/known-zero/unresolved history in their own scopes. User-approved relocation to `test/e2e/fleet/history_selection_2496_test.go` uses registered hub/spoke origins, preserves the full isolation contract and adds a genuinely remote completed Job/patch plus unchanged hub-target proof. Canonical fullpipeline stays standalone; existing fleet regressions remain separate gates. |
| E2E-FP-2496-006 | BR-AI-088; BR-AI-003; BR-AUDIT-005; P0 | Given a structurally valid repeat below the configured current-confidence floor, when AA/RO process it, then manual review preserves justification and no WE is created; high historical/RCA confidence cannot substitute for the current score. |

## 10. Environmental needs

- Go **1.26.6**, Ginkgo v2/Gomega, repository-pinned ogen/controller-gen, existing suite configuration.
- UT: external-dependency fixtures only; real production projection/parser/prompt/validator logic. No standard `testing.T` business cases.
- IT: real PostgreSQL/DS, Redis as required by existing harness, envtest API server/admitted CRDs, real informer/catalog and HTTP provider-client path. Unique fixture names and random endpoint ports.
- E2E: isolated Kind harness with deployed compatible DS/KA/AA/RO, WFE/EM and configured notification service; deterministic external Mock LLM test endpoint and real observability inputs for ineffective history.
- No live customer cluster/logins used. No shared cluster/resource cleanup beyond resources owned by the suite. Do not prune broad container images/volumes to unblock tests without permission.

## 11. Dependencies and schedule

| Dependency | Gate |
|---|---|
| Revised H-B/J-B proposal and proposed field/identity/failure semantics | Overall implementation-plan approval and BR/DD formalization before implementation |
| #2490 merged history behavior | Keep in baseline; regress complete chain, scoping, resource limits and SpecDrift |
| Explicit v1.6 RO selection metadata | Producer/consumer dispatch IT proves provenance and safe absent-vs-zero semantics; no legacy AA recovery/backfill or WFE join |
| Generated schemas/CRDs | Update authoritative sources and all generated copies before runtime contract tests |
| Real IT/E2E infrastructure | Must actually execute for GREEN/release evidence; cannot replace with business mocks |

Order: approve contract → write production-dispatch IT and logic UT (RED) → minimal code/wiring and both tiers PASS (GREEN / CHECKPOINT W) → concrete refactor and mandatory build/compile checks → proving E2E journeys → full readiness audit. Effort estimate and per-slice durations are in the implementation plan.

## 12. Deliverables

- This formal test plan and linked implementation/preflight/spike record.
- Requirement/DD changes after approval; control-mapped Ginkgo scenarios and provider fixture updates in existing suites.
- Generated API/CRD artifacts and consistency evidence, wiring checklist, raw/typed audit reconstruction evidence and coverage report.
- Execution results with exact commands, baseline SHA, pass/fail/blocked status, and updated confidence. No claim of completion based only on compiling tests.

## 13. Execution commands and evidence

Already executed **baseline only**: focused prompt/parser/enrichment test command in the implementation plan, three packages PASS.

After approval/implementation:

```sh
go build ./...
go test ./... -run=^$ -timeout=30s
golangci-lint run --timeout=5m
make test
make test-integration-datastorage
make test-integration-kubernautagent
go test ./test/integration/aianalysis/... -ginkgo.focus='2496'
go test ./test/integration/remediationorchestrator/... -ginkgo.focus='2496'
make test-e2e-kubernautagent
make test-e2e-fullpipeline
make test-e2e-fleet
make lint-test-patterns
make lint-business-integration
make lint-tdd-compliance
```

Direct suite commands require the existing suite's configured infrastructure/envtest assets. Use suite focus/labels for initial RED/GREEN feedback without adding skipped tests; then run affected regression suites. Communicate before long IT/E2E (~10-minute KA / ~30-minute full-pipeline harness) and before commits. Generation must be checked with repository tooling (`make generate`, plus the existing CRD-manifest/copy workflow); do not hand-maintain generated outputs or treat documentation changes as a clean `gen-diff` code baseline.

## 14. Wiring verification

Use the manifest in implementation-plan §5.2. For each row record the production caller (prefer Engram symbol references), actual production-dispatch IT ID and PASS evidence. Include request-local history/validation state, future RO metadata producer, existing configured confidence gates and typed audit mapper/CRD admission; isolated validator UT cannot substitute. GREEN remains incomplete until every retained row passes CHECKPOINT W.

## 15. Existing E2E regression matrix

This matrix corrects the earlier broad wording “existing E2E suites remain regression gates.” It names the current scenarios that must be edited, the scenarios that must be rerun without changing their business assertions, and representative suites outside the changed contract. New `E2E-FP-2496-*` and `E2E-FP-2498-*` journeys supplement these cases; they do not replace or weaken them.

### 15.1 Discovery evidence and limitation

The requested Engram codegraph blast-radius queries were attempted against the planned production seams (`emitWorkflowCreatedAudit`, `HandleGetRemediationHistoryContext`, `RenderWorkflowSelection`, `processToolCalls`, `executeTool`, `MarshalRCASubset`, `emitInvestigationSummaryArtifact`, and `buildSelectedWorkflowMap`). The initial parallel request returned `RemoteProtocolError`/`ConnectError`; a bounded retry recovered, but every result reported the same low-quality graph-wide counts (`82,430` total calls, `43,464` unresolved, `22,049` ambiguous). Usable resolved production chains were: `RenderWorkflowSelection` ← `runWorkflowSelection` ← `runWorkflowDiscoveryPhase`/`RunWorkflowDiscoveryFromRCA`; `processToolCalls` ← `runLoopTurn` ← `runLLMLoop`; `executeTool` ← `processToolCalls`; `MarshalRCASubset` ← `emitCompleteEvent` ← `runInvestigation`; `emitInvestigationSummaryArtifact` ← `runBlockingInvestigation` ← `HandleInvestigationMCPWithRegistry`; and `buildSelectedWorkflowMap` ← `MapInvestigationResultToAgentSessionResult` ← `writeTerminalStatus`. No resolved callers were returned for the RO producer or history handler. The graph is production-call oriented and does not resolve network-driven E2E callers, so this matrix is still **not** claimed to be graph-complete; it was reconciled against the live test source with bounded exact searches and reads.

### 15.2 Must update or extend before the corresponding RED/ GREEN gate

These are existing tests whose fixtures or assertions touch a changed wire/projection contract. Existing assertions listed in the treatment column remain mandatory.

| Existing file | Existing scenario IDs | Required regression treatment |
|---|---|---|
| `test/e2e/datastorage/25_remediation_history_api_test.go` | `E2E-DS-016-001`, `E2E-DS-016-002`, `E2E-DS-016-003`, `E2E-DS-016-004` | Extend the real HTTP/DB fixtures for current-version `workflow_id`, catalog-authoritative `workflow_name` and `workflow_selection_confidence`; keep selected confidence distinct from RCA/effectiveness confidence, preserve `SpecDrift`/known-zero semantics and oldest-first ordering, and prove empty history does not look like missing metadata. Add an incomplete/old-record fixture only for bounded “no enrichment/backfill” diagnostics; do not change immutable source rows. Keep `E2E-DS-016-005` unchanged for invalid query validation. |
| `test/e2e/kubernautagent/three_step_discovery_test.go` | `E2E-KA-2459-001`, `E2E-KA-017-001-001b` | Extend the selected-workflow decoder and correlated discovery assertions to require the actual ID and name plus explicitly named selection confidence. Keep top-level `incidentResp.Confidence` as RCA confidence, retain catalog membership and minimized `get_workflow` audit payload checks, and keep the parameter/execution-bundle sentinels absent. |
| `test/e2e/kubernautagent/interactive_discovery_e2e_test.go` | `E2E-KA-DISC-001`, `E2E-KA-DISC-004`, `E2E-KA-DISC-005` | Extend successful discovery/selection result checks for the additive safe projection and mandatory ID/name; retain alternative parameter propagation, Lease release, and RCA-target GVK behavior. `E2E-KA-DISC-005` must continue proving discovery follows the RCA-resolved target rather than the original alert target. |
| `test/e2e/kubernautagent/audit_pipeline_test.go` | `E2E-KA-2478-001`, `E2E-KA-2478-002`, `E2E-KA-433-AP-001`, `E2E-KA-433-AP-002` | Extend completed-selection/response-complete reconstruction to assert safe selection metadata and, when present, repeat validation outcome/justification without raw parameters. Preserve correlation ID, actor attribution, event action/outcome/UUID and the ranked-family/generic-fallback assertions. |
| `test/e2e/kubernautagent/param_validation_e2e_test.go` | `E2E-KA-1170-001`, consolidated `E2E-KA-1170-002`, `E2E-KA-1170-003` | Retain the two-attempt invalid-parameter → corrected-parameter proof and sequential timestamped validation history. Extend the production scenario/fixture so repeat-justification checks run on the corrected path; a corrected different workflow must not inherit a prior repeat requirement, while an invalid same-ID repeat must not pass merely because parameters were corrected. |
| `test/e2e/apifrontend/progressive_flow_e2e_test.go` | `E2E-AF-1408-001` | Extend the real `investigation_summary` artifact assertion for the bounded allow-listed workflow/repeat projection and its safe fallback behavior. Preserve schema/version, valid `DataPart`, and `summary` assertions. Do not add a DS/audit lookup to the live AF path. |
| `test/e2e/apifrontend/structured_decision_e2e_test.go` | `E2E-AF-1395-001`, `E2E-AF-1396-001`, `E2E-AF-1396-002` | Extend the interactive decision payload decoder for the same bounded selected-workflow/repeat projection when a selection exists. Preserve >512-byte JSON integrity, authoritative severity/target/RCA fields, all three human-review options, parameter visibility rules, and no hidden automated choice. |
| `test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go` | `E2E-FP-118-001`, `E2E-FP-118-002` | Keep every existing SP/AA/RO/WFE/notification/status/audit/reconstruction assertion. Extend the real v1.6 selected-workflow status and correlated workflow-created/selection audit checks for ID/name/selection confidence provenance; do not replace RCA confidence, alter execution safety, or weaken completion evidence. |
| `test/e2e/fullpipeline/15_af_a2a_interactive_streaming_test.go` | `E2E-FP-1189-005` | Extend the live SSE turn-2/decision artifact checks for the safe current projection, while retaining visible RCA, discovery/selection turns, execution-progress `completed_at`, terminal stream state, real WFE completion and `TARGET_RESOURCE_*` assertions. |

### 15.3 Rerun unchanged as mandatory regression gates

“Unchanged” means the source assertions are not relaxed or rewritten to accommodate the feature. If an additive field changes a decoder, update only the decoder while preserving the listed business assertion; otherwise rerun the existing file as-is.

| Existing file | Existing scenario IDs | Non-negotiable invariant |
|---|---|---|
| `test/e2e/kubernautagent/incident_analysis_test.go` | `E2E-KA-001`, `E2E-KA-002`, `E2E-KA-003`, `E2E-KA-004`, `E2E-KA-005`, `E2E-KA-006` | No workflow remains human review with `SelectedWorkflow=nil`; low confidence is returned to AA rather than enforced by KA; exhausted correction remains review with complete validation history; normal selection/response structure still works. |
| `test/e2e/kubernautagent/adversarial_parity_e2e_test.go` | `E2E-KA-433-ADV-010`, `E2E-KA-433-ADV-011`, `E2E-KA-433-ADV-012`, `E2E-KA-433-ADV-013`, `E2E-KA-433-ADV-015`, `E2E-KA-1039-001`, `E2E-KA-1039-002` | Preserve selected-workflow presence/identity, low-confidence output, incident correlation, valid no-workflow review reason, and deleted-resource continuation. No additive field may turn a no-workflow or deleted-resource result into execution. |
| `test/e2e/kubernautagent/interactive_discovery_e2e_test.go` | `E2E-KA-DISC-002`, `E2E-KA-DISC-003` | `complete_no_action` remains terminal and releases the Lease; `select_workflow` without prior discovery remains rejected with the prerequisite error. |
| `test/e2e/kubernautagent/workflow_catalog_test.go` | `E2E-KA-030`–`E2E-KA-044` (implemented IDs in the file) | Catalog search, confidence scoring, empty results, filters, top-K and no-match behavior remain intact; do not introduce candidate filtering or an action-family ban. |
| `test/e2e/kubernautagent/empty_tool_result_2482_e2e_test.go` | `E2E-KA-2482-001` | An empty Kubernetes tool result still completes the AgentSession path safely; optional Phase 1 context tools must not become mandatory. |
| `test/e2e/kubernautagent/audit_pipeline_test.go` | `E2E-KA-045`–`E2E-KA-048`, `E2E-KA-1111-001`–`E2E-KA-1111-003`, `E2E-KA-433-AP-003`, `E2E-KA-1401-001`, `E2E-KA-1401-002` | Existing audit event presence, correlation, actor attribution and security-event behavior remain valid; extra fields must not drop or forge the audit envelope. |
| `test/e2e/apifrontend/mcp_fullpath_test.go` | `TC-E2E-MCP-FULL-02+04`, `TC-E2E-MCP-FULL-03`, `TC-E2E-MCP-FULL-05`, `TC-E2E-MCP-FULL-06` | MCP lifecycle, catalog response, exact AF domain-tool count (23 unless the AF public contract is intentionally changed), and unknown-tool rejection remain unchanged. KA internal phase tools must not silently alter the AF public tool list. |
| `test/e2e/apifrontend/discover_workflows_test.go` | `E2E-AF-WP-001`–`E2E-AF-WP-006` | AF discovery/select HTTP/MCP response, RBAC, metrics and invalid-RR behavior remain valid; current result delivery is not replaced by a DS read. |
| `test/e2e/apifrontend/ka_integration_test.go` | `TC-E2E-KA-01`, `TC-E2E-KA-02`, `TC-E2E-KA-05`, `TC-E2E-DS-02`–`TC-E2E-DS-05`, `TC-E2E-KA-06`, `TC-E2E-KA-07`, `TC-E2E-MULTI-01` | AF→KA, explicit history/audit tools, present-decision formatting, and cross-service routing remain available. History/audit calls are retrieval paths, not live-result transport. |
| `test/e2e/apifrontend/fleet_mode_e2e_test.go` | `E2E-AF-FLEET-2462-001`, `001a`–`001f`, `002`, `003`, `004`, `005` | Hub severity, remote-cluster collision filtering, SSE content, concurrent-session status and unregistered-cluster fail-closed behavior remain intact. |
| `test/e2e/fullpipeline/05_mcp_interactive_lifecycle_test.go` | `FP-MCP-001`, `FP-MCP-002`, `FP-MCP-003a`, `FP-MCP-005`, `FP-MCP-005c`, `FP-MCP-006`, `FP-MCP-008`, `FP-MCP-009`, `E2E-FP-1452-001` | Interactive start/message/discovery/selection/no-action, Lease/session lifecycle and session-ID forwarding remain valid. |
| `test/e2e/fullpipeline/06_af_audit_trace_test.go` | `E2E-FP-AF-001` | Required AF audit event categories, correlation-independent envelope and category assertions remain valid. |
| `test/e2e/fullpipeline/07_af_a2a_autonomous_test.go` | `E2E-FP-1189-002` | Autonomous AF→RR→full pipeline still completes and creates no InvestigationSession. |
| `test/e2e/fullpipeline/08_af_a2a_interactive_test.go` | `E2E-FP-2390-001` | Four-turn interactive discovery/selection still creates the selected workflow, preserves dependencies/parameters/target injection and completes the WFE. |
| `test/e2e/fullpipeline/12_problem_resolved_no_we_test.go` | `E2E-FP-1430-001` | `problem_resolved` remains `WorkflowNotNeeded`/`NoActionRequired`, with `SelectedWorkflow=nil` and no WorkflowExecution. |
| `test/e2e/fullpipeline/13_escalation_lifecycle_test.go` | `E2E-FP-1456-001` | Operator escalation remains `ManualReviewRequired`, sets the existing review reason and creates no WorkflowExecution. |
| `test/e2e/fullpipeline/17_af_a2a_full_interactive_remediation_test.go` | `E2E-FP-1853-002` | Authorized full-remediation mode still auto-chains through discovery/selection/watch without a manual pause and completes. |
| `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go` | `E2E-FP-1899-001`, `E2E-FP-1899-002`, `E2E-FP-1912-001`, `E2E-FP-1918-001` | Same-turn phase transitions remain blocked unless the existing consent/mode rules authorize them; terminal-tool no-reinvocation and non-actionable no-WE behavior remain unchanged. The new KA server-side off-phase test supplements, not replaces, these AF consent tests. |
| `test/e2e/fullpipeline/03_ka_target_resource_test.go`, `16_standalone_execution_cluster_id_test.go` | `E2E-FP-496-001/002/003`, `E2E-FP-2390-005` | KA-owned target identity, target parameter injection and execution-cluster attribution remain authoritative; history scope must not widen across clusters. |

### 15.4 Unaffected by the changed contract (full suite still remains a final gate)

These representative scenarios do not consume the changed history/selection/result projection or phase-dispatch seams. They do not need #2496-specific assertion edits, but they remain part of the normal repository regression run:

| Existing file | Existing scenario IDs / scope |
|---|---|
| `test/e2e/kubernautagent/jwt_e2e_test.go` | `E2E-KA-JWT-001`–`E2E-KA-JWT-005` authentication and token expiry |
| `test/e2e/apifrontend/session_crd_test.go` | `TC-E2E-SESS-001`–`TC-E2E-SESS-004` AF controller/CRD startup and TTL lifecycle |
| `test/e2e/apifrontend/alert_prioritization_e2e_test.go` | `E2E-AF-1412-001`, `E2E-AF-1412-002` alert prioritization |
| `test/e2e/datastorage/05_soc2_compliance_test.go` | `BR-SOC2-001`–`BR-SOC2-005` hash-chain, immutability, legal hold and signing |
| `test/e2e/fullpipeline/04_interactive_regression_test.go` | `E2E-KA-HARM-001` autonomous path has no interactive artifacts/Lease |
| `test/e2e/fullpipeline/14_crashloop_real_fix_test.go` | `E2E-FP-1542-001` real CrashLoop fix execution |

All other E2E files are “unaffected” only in the narrow contract sense; `make test-e2e-kubernautagent` and `make test-e2e-fullpipeline` remain required final regression gates. A test that fails because a generated additive field changes a decoder moves into §15.2; its pre-existing business assertion must not be removed.

## 16. Changelog and current execution status

| Version | Date | Change |
|---|---|---|
| 1.0 | 2026-10-09 | Draft pre-implementation plan, business scenarios, pyramid/wiring and versioned control objective coverage. |
| 1.1 | 2026-10-09 | Aligned with clarified auditability/agent-owned reasoning scope: removed semantic-review and capture-acknowledgement gates, deferred WFE completion reconstruction, added explicit RO metadata and configured confidence/human-review preservation coverage. |
| 1.2 | 2026-10-09 | User permits a v1.6 break from v1.5: remove legacy AA recovery/backfill and mixed-version compatibility; select the direct H-B producer/history scope while retaining safe missing/malformed current-version data and all applicable control coverage. User also requires logs when old records remain unenriched; extend existing history UT/IT/E2E cases for bounded diagnostics, unknown semantics, immutable source preservation and no secret disclosure. Expanded evidence-handoff scenarios still need a separate full revision. |
| 1.5 | 2026-10-09 | Synchronized the final pre-implementation plan with the 95% approach-confidence assessment, exact live `selected_workflow` projection, strict fail-closed handoff contract, pyramid invariant and BR/FedRAMP/SOC 2/OWASP ASVS proof obligations. |
| 1.6 | 2026-10-09 | Consolidated the existing-E2E regression matrix with exact files/scenario IDs, separated required assertion updates from unchanged rerun gates, and recorded the unavailable Engram graph-backend limitation. |
| 1.7 | 2026-10-10 | Approved implementation is in progress. Added genuine producer-lifecycle supplementary journeys, approval-band provider fixtures, target/origin positive controls and independent durable/live reconstruction assertions. Preserve canonical assertions and distinguish standalone provenance proof from remote execution proof. |
| 1.8 | 2026-10-10 | User approved moving the supplemental isolation case to the existing real fleet harness, preserving all assertions and canonical standalone topology. Shared independent audit/live validators avoid drift between suites. DS Full-zero fixture supplies zero assessed components, retaining the zero assertion; all six focused deployed history cases now pass, while the complete DS gate retains 12 DB/DLQ timeout failures and needs a full rerun. |
| 1.9 | 2026-10-10 | Added external-provider RED→GREEN fixture and compile-verified deployed phase-isolation scenario. Post-refactor Mock Make gate passes all 348 specs. DS full retry and focused diagnostics remain failed; retained fixed-correlation history explains the diagnostic count mismatch, not prior DB-write timeouts. Preserve immutable history and all canonical assertions. |
| 1.10 | 2026-10-10 | E2E-FP-2498-001 passes deployed with genuine permitted Job/512Mi patch. Focused fullpipeline is one pass/seven failures: six new telemetry fixtures use rejected backdated samples; one new audit validator misclassifies AuthWebhook user principals as service actors. Test-only repairs preserve scoring, safety and canonical assertions. KA Make passes 3,219 specs including 21 live-projection boundary cases; stock Helm coverage is not attributable because no GOCOVERDIR is configured. |
| 1.11 | 2026-10-10 | Corrected E2E-FP-2496-001/007 pass deployed (2/2 focused) with real zero/success assessments, repeat acceptance and live/audit reconstruction. User-approved temporary instrumentation yields attributable coverage from 11 services without old-counter loss. Ten added AgentSession mapping cases pass in the complete KA Make gate (3,229 specs); canonical fullpipeline/AF/fleet and merged coverage remain pending. |
| 1.12 | 2026-10-10 | Six additional UT-KA-2496-010 observation-isolation, sanitizer-collision and deterministic chronology cases pass with no production change. Complete KA Make now passes 3,235 specs / 35 suites, zero failed/pending/skipped; build and scoped lint pass. Canonical deployed gates and final coverage/control review remain incomplete. |
| 1.13 | 2026-10-10 | Four AF persisted-state/error-boundary characterization cases pass without production changes. Complete AF Make passes 2,725 BDD specs / 25 suites (22 BDD), zero failed/pending/skipped, 80.7% service composite; scoped lint is clean. Canonical fullpipeline is in progress, with no deployed/merged gate claim. |
| 1.14 | 2026-10-11 | Canonical fullpipeline fails 16/29, all 45 specs, zero pending/skipped; failed output, frozen provenance and attributable eleven-service coverage are archived. Prepare test-host URL isolation corrections and test-only new-history repairs: immediate OOM rather than transient 40Mi warm-up, real WFE cooldown wait for different-ID execution, existing Completed/no-workflow review semantics. Preserve zero-score, review/notification, catalog, live/audit and canonical assertions; diagnose AF/no-RR independently. Latest KA unit gate passes 3,236 specs / 35 suites with the lifecycle-sweep characterization. |
| 1.15 | 2026-10-11 | Isolated runtime diagnostic passes MCP lifecycle and fails AF consent plus new-history setup (1/3 selected, zero pending). Actual AF response uses a prior run's namespace: the retained Mock pod predates the newly applied namespace ConfigMap, which the fixture loads at startup. Prepare a normal isolated Mock rollout, without forced deletion or production-policy changes. The new history fixture's whole-object Deployment update races controller status; replace it with a spec merge patch while retaining every scoring/history/Job assertion. Ten additional KA boundary cases cover request-scope clearing, optional reasoning, nil results, malformed optional context, deterministic history ties, compact signal resolution and serializer failure. The first unit run fails only a newly introduced expectation about existing actionable routing; correcting that characterization yields a complete Make pass of 3,246 specs / 35 suites, zero failed/pending/skipped. Build and scoped lint pass; no production repair was required. |
| 1.16 | 2026-10-11 | Normal isolated Mock reload and spec merge patches pass the same three-spec deployed diagnostic (3/3, zero failed/pending, 42 focus-excluded), including actual consent-gated execution and producer/history/live/audit reconstruction. Frozen eleven-service coverage/provenance are archived and all 22 copied coverage files match mounted originals. Five additional AA boundary cases pass in complete Make (476 BDD specs, two pre-existing skips); seven AF complete-event/privacy cases pass in complete Make (2,732 BDD specs, zero skipped). Scoped lint passes with no production change. Updated changed-block characterization covers 1,156/1,205 statements; it is not full business-logic or merged release coverage. Prepare complete canonical fullpipeline without diagnostic logging, focus, skip or process override. |
| 1.17 | 2026-10-11 | Complete KA/AF Make retry passes 5,993 BDD specs (3,260 KA / 2,733 AF), zero failed/pending/skipped, with unchanged source hashes and clean scoped lint. Three new malformed-actionable expectations were corrected to preserve existing conservative routing; forged-context and unconditional repeat-validation checks remain. Defensive JSON/prior-assessment tests increase changed-block characterization to 1,169/1,205 statements, not complete business-logic or merged coverage. Further operator-policy withholding tests are in verification. Audit catalog amendments map changed KA/AA/RO payloads to their control objectives without sweeping unrelated historical mappings. Unfocused canonical fullpipeline rerun uses fresh ID fullpipeline-canonical-02; outcome pending. |
| 1.18 | 2026-10-11 | Operator-policy KA Make passes 3,266 specs; build/scoped lint pass and changed-block characterization reaches 1,175/1,205 statements (30 unexercised; not a release coverage gate). Canonical-02 fails 36/9 across all 45 specs, zero pending/skipped, suite timeout; frozen provenance and eleven-service coverage are archived and independently matched against 22 mounted originals. All nine failed specs are classified in handoff §22: two retained-name collisions, two unresolved AF journeys, default-50 pagination omitting a catalog record at rank 61/64, a rejected-reasoning audit serialization defect, an incorrect low-confidence RR phase expectation, an unresolved approval/execution wait and a suite-timeout interruption. User approves fresh named DS/fullpipeline clusters and new-fullpipeline-only instrumentation, retaining every old resource; also chart-matching AF fixture RBAC with namespace-scoped Leases and authenticated positive/negative proof. Extend UT-KA-2496-009 and IT-KA-2496-005 to rejected absent reasoning/optional context (AU-3/CC7.2); add IT-AF-2496-004 allowed/denied and real LIST proof (AC-6). Full KA RED reproduces two serializer failures; the narrow JSON-shaped sanitized projection fix passes all 3,268 KA specs. Authenticated DS RED reproduces the same serializer defect; ingestion GREEN is still pending. AF RBAC specs are not yet exercised because a test-host :9090 metrics collision fails BeforeSuite; isolate that fixture listener without changing production or stopping unrelated services. |
| 1.19 | 2026-10-11 | Authenticated DS audit GREEN passes 7/7 selected cases, including absent rejected reasoning. Approved AF fixture grants pass 22/22 allowed/denied and actual LIST cases after 11-failure RED; production/chart RBAC unchanged, Leases namespace-scoped. Complete AF integration Make passes all 217 specs across three suites, zero failed/pending/skipped. Isolated KA rebuild preserves the original fourteen images and every older counter/inode/ledger; aliases unchanged services under a new tag. Approved fresh DS canonical run fails BeforeSuite with 0/143 specs because a local-only image was incorrectly pulled as a registry artifact, leaving ImagePullBackOff; no DB-timeout resolution claim. Runtime-only local-image loading and guarded reuse preserve the new cluster. Canonical-02 GitOps provider/admitted state proves explicit workflow score zero correctly triggers low-confidence review. UT-MOCK-2496-012 captures the real generated ConfigMap at the external kubectl boundary; UT-MOCK-2496-013 proves explicit score/known-zero/omitted/unrelated boundaries before correcting only the external fixture's named score override. Complete Mock Make GREEN passes 352 specs/two suites. Cross-namespace AF had no RR-producing event and no scoped fixture rule; include its unique target name in existing firing-alert setup and create the namespace in its spec, preserving real crash, ADR-057 namespace split, no-IS and fail-closed severity policy. Validator pagination, exact failed-AA terminal phase and genuine twelve-minute approval execution wait are corrected narrowly. Whole-suite fullpipeline Make budget is 70m without changing per-journey safety deadlines or process selection. Fresh fullpipeline/fleet and release coverage gates remain outstanding. |

| 1.20 | 2026-10-11 | Complete local Make retry passes 11,524 specs across 114 suites with immutable source; preserve its preceding one-failure run from an untouched AA 200ms shutdown test. Mock-only image rebuild succeeds, preserving thirteen audited service IDs and every original image/counter/inode/ledger. Fresh DS runs all 143 specs: 119 pass, 24 fail, zero pending/skipped. Complete remote-only DS host-DSN/fallback isolation; DB-write/DLQ cancellation remains unresolved. Fresh fullpipeline canonical-03 runs all 45 specs: 40 pass, five fail, zero pending/skipped, no timeout. Independently verified eleven-service coverage/provenance are archived. Cross-namespace spec redundantly creates a namespace already created by SynchronizedBeforeSuite; restore strict GET/managed-label proof. Four history fixtures fail on .30: raw EM audits record transient health .75 during an OOM restart, although final CRD health is zero. Add real capacity readiness for the 64Mi-demand workload, retaining actual OOM, zero assertions and 512Mi positive recovery; cached-image capability proof passes in an owned diagnostic Pod. No EM/DS scoring or history changes. Engram public semantic route returns fresh results. User clarifies reasonable business/control coverage, not artificial literal 100%: document defensible exclusions and keep real gaps explicit. Canonical fixture GREEN and remaining deployed gates remain outstanding. |
| 1.21 | 2026-10-11 | DS endpoint retry runs 136/143: six pass, 130 fail, seven BeforeAll-derived skips. The failure is predominantly strict TLS rejection, not proof of a production API regression: the generic host CA path depends only on the shared kubeconfig directory, so a subsequent fullpipeline suite overwrote DS client trust. Read-only comparison proves DS still serves its correct Secret certificate; OpenSSL chain and Python chain/hostname verification both pass against DS's unchanged cluster CA. Remote-only DS client fixture gets a unique host CA path, copied from its existing ConfigMap; shared host CA and server Secret remain byte-identical. No TLS bypass, certificate rotation, production/configuration or process change. Complete canonical trust-isolated DS retry remains pending; DB cancellations and preserved fixed-correlation contamination must be assessed separately. |
| 1.22 | 2026-10-11 | Strict trust-isolated canonical DS executes 136/143: 123 pass, 13 fail, seven BeforeAll-derived skips, 344.536s outer. TLS/authenticated allowed and denied paths work; DB query/write cancellations and SOC2 setup still fail. All 98 read-only samples have provenance: active LockManager contention, no reported blocking transactions, substantial CPU throttling under the unchanged 0.5-CPU PostgreSQL fixture. Idle unchanged predecessor query: 198.083ms planning, 0.860ms execution. All expected indexes exist; 633 audit rows and only two fixed-correlation rows rule out large retained-dataset size as an explanation for this run. Resource-only diagnostic in a fresh DS cluster proposed, not approved/implemented; do not label this flaky or alter production/CI policy. Final complete Helm Make passes 91 suites/721 tests. Next fullpipeline uses a fresh name/never-used coverage ID and an isolated host CA path without resource changes. |
| 1.23 | 2026-10-11 | User approves increasing limits for the measured concurrency spike. Two emitted-Deployment BDD tests fail on the actual 500m values (RED); test-only inline manifests change PostgreSQL CPU limit to 4 and DS to 2, retaining memory bounds and all requests. Complete infrastructure package passes 237/237 without pending/skips (supplement, not canonical Make substitute). Build and scoped new-diff lint pass. Production/Helm/CI process settings and application deadlines are untouched; REFACTOR N/A for plain data values. Fresh DS capacity canonical gate remains pending; no flakiness claim. |
| 1.24 | 2026-10-11 | Fresh `issue2496-ds-clean-04` complete `make test-e2e-datastorage` passes **143/143**, zero failures/pending/skips, 399.546s suite / 426.791s outer. Strict TLS, authenticated allowed/denied SAR and SOC2/history cases remain intact. Applied Deployments confirm PostgreSQL 4 CPU / DS 2 CPU, unchanged memory/requests. The 124 read-only samples record 43 active LockManager waits, no blocking transactions, 27.956s additional aggregated PostgreSQL throttling and none for DS; prior failing run recorded 389.160s. This supports the fixture burst-capacity correction, not a flaky-test or CI-executed claim. Frozen source, all old counters/mount inode, fourteen original images and old remote ledger remain unchanged. Verified local/remote archive SHA-256 `5b809c2fcfaf077350b20e57729bc6154aeb5ea7cc6c5663800b2e1e66e848ac`. Fresh fullpipeline-04 is next; remaining AF/fleet/control/merged gates stay open. |
| 1.25 | 2026-10-11 | Complete fresh fullpipeline-04 runs 45/45: **44 pass / one fail / zero pending/skips**, 3453.123s suite / 3594.520s outer. Both workload-readiness and namespace-ownership corrections pass. The remaining rejected-repeat test expects RR Failed, but retained state/logs prove Completed + ManualReviewRequired, delivered notification and no WE within two seconds. Existing BR-ORCH-036 v5.0 and `IT-RO-550-001/002` deliberately route by selected-workflow presence, independently of AA phase. Test-only correction preserves Failed AA, rejected non-executable assessment and Failed RR for low-confidence admitted selections; additionally asserts manual-review flag and no WE after terminal completion. No RO/AA/production/safety/deadline change. Eleven deployed profiles are attributable and every preservation check passes. Verified archive SHA-256 `90c73b2c8d16fd606b067d1078f1609a92ced55acda32f106711496f5d310d41`. Canonical-05 GREEN remains required; no release/publication claim. |
| 1.26 | 2026-10-11 | Complete fresh `fullpipeline-clean-canonical-05` passes **45/45**, zero failed/pending/skipped, 3275.265s suite / 3415.542s outer, exit 0. The selection-presence terminal correction passes without production or safety-policy changes. All eleven deployed profiles and their mounted/copied counters are independently hash-verified; every source/image/CA/counter/inode/remote-ledger preservation check passes. Verified archive SHA-256 `d4199307a8d06dfebe097324cfa3166d36fd04153a65c8eec3c913a50ef63446`. Preserve failed predecessors. Complete affected IT, deployed AF local/fleet lanes, real registered-origin fleet execution and final merged/control/wiring evidence remain required before publication. |
| 1.27 | 2026-10-11 | Complete affected IT batch `it-complete-canonical-01` exits 2 after 513.941s: standalone DS 182/182 passes, but six DS-backed suites cannot start because the verification overlay moved host bindings without moving mounted YAML database/Redis endpoints. All eight preservation checks pass; archive SHA-256 `564634bd4b71ec5d1bd051dcee334d64b82761c57891267ec94e5d024d3fa0a8`. Correct only the remote-runtime helper to remap YAML and suite-owned metrics listeners, preserve processes/deadlines, and require a fresh unused identity. Local dry-run verifies all six endpoint/metrics triples and unchanged repository sources; canonical runtime GREEN remains pending. Extend the existing investigator suite with planned `IT-KA-2496-001/006`: real authenticated DS → real enrichment/freeze → actual HTTP provider, both tiers/global chronology, independent confidence/zero/unresolved/unknown-cause and historical credential/parameter omission. Compile-only passes; these new cases still need execution. |
| 1.28 | 2026-10-11 | Complete `it-complete-canonical-02` exits 2 after 564.120s. AA passes 76/76 across four suites and RO passes 150/150. Standalone DS passes 169/182, with nine graceful-shutdown and four history/audit cases failing on database/HTTP timeouts; cause not established. Four KA DS-backed suites fail BeforeSuite: full logs prove YAML metrics used API+10000, colliding with the bootstrap's derived health listener. Earlier dry-run verified the assumed mapping, not the actual Go MetricsPort. Archive and all four retained profiles independently hash-verified, SHA-256 `bc6660d2d6aa83697b82fc212539d0eb98693652d0801476e9b77419d4799413`; all eight preservation checks pass. Runtime-only RED captures four YAML/Go MetricsPort mismatches; GREEN matches all six suites' actual PostgreSQL/Redis/metrics values, reserves independent health listeners, and syntax-validates IT plus both AF runners. Fresh IT-03 +6000 bindings are unused; complete canonical retry will capture read-only DS activity/blocking PIDs without raw query text, process/capacity/deadline/assertion changes. New real DS-to-provider cases remain runtime-unproven. |
| 1.29 | 2026-10-11 | Complete `it-complete-canonical-03` exits 2 after 498.051s. DS 182/182 and AA 76/76 pass; KA executes all suites, with investigator 151/174 and all other KA suites passing. RO executes 0/150 after a health bind collision. Archive/profiles/activity/config hashes independently verified: SHA-256 `83285137f69629d732dbb0ec876aadea579de00767d86fced3f69f0e990be6ef`; eight preservation checks pass. Attribution is to our changes/verification setup, not an origin/main or flakiness claim. RO setup/bootstrap/executable are identical to origin/main; remote-only +6000 moved health 28140→34140 into the unchanged Linux ephemeral range 32768–60999. The competing socket owner was not captured. Guard RED now rejects that range overlap before writes; GREEN checks all TCP sockets and unused +4200 bindings outside it, without changing kernel policy. Investigator failures classify as fourteen incomplete catalog-name fixtures, six tool fixtures stopped by the new phase guard, and three history fixture/packet-decoder mismatches. Correct only fixtures: catalog-authoritative names preserve schemas/allowlists; use real phase-eligible reader names without broadening policy; bind the historical target/producer metadata explicitly; decode the actual fenced-JSON handoff. All business/control assertions remain. Compile/scoped lint/diff checks pass, runtime GREEN pending. User expects the normal automated CI matrix to provide final proof, not permanent manual per-lane verification; temporary remote isolation remains outside CI/production. |

| 1.30 | 2026-10-11 | User keeps the existing 45-minute fullpipeline CI job limit and requires all known issues fixed before commit/push/new PR; draft publication is not authorized. Existing fixture/isolation corrections compile, with no production or CI policy changes. The first source-sync attempt is blocked by an unrelated remote fleet run before any source transfer; retain its preflight directory and do not interfere with that run. Recheck confirms +4200 IT bindings are unused/outside the unchanged ephemeral range, but remote idle is not established. Further fixture review finds IT-KA-970-003 can pass cancellation without executing its phase-denied synthetic tools: strengthen it to require all four tools actually started before proving cleanup, preserving the 100ms context deadline and every existing cancellation assertion. This RED assertion remains unexecuted until a safe complete IT batch; do not claim a runtime failure/pass from source inspection. Complete local Make unit regression is active with immutable Go hashes and no process override; it cannot replace IT GREEN. |

| 1.31 | 2026-10-11 | Complete local `make test` regression passes **11,524 specs / 114 suites**, exit 0, 269.450s, zero failed/pending and the same two pre-existing kubeconfig-dependent skips. Independently verify all 3,204 captured Go source hashes and all twelve profiles; archived full log/result/profiles SHA-256 `82ef461dfcdfb583c11ff9ff26f7f565c2e3b9a546975f6ad7133bbd2d4a9e9f`, artifact `issue2496-fixture-contract-unit-regression-f2jor9i7`. Scoped new-diff investigator lint passes zero issues. No process/focus/skip override or source mutation. This confirms local unit regressions, not the failed complete IT gate, the new cancellation RED or unrun deployed AF/fleet journeys. |

| 1.32 | 2026-10-11 | Passive process-exit notification completes and a fresh global idle/all-TCP preflight passes. Non-deleting sync `it-fixture-contract-source-15` verifies all 5,910 files; manifest SHA-256 `70a874d292c266ae6ca34342154ccc2448118da4c9dcc3b1bba3d644ec5b4cdf`. All 5,005 old counters, mount inode, fourteen original image IDs and remote ledger remain unchanged; ledger is excluded from transfer. One-use IT-04 runtime applies +4200, with every host/derived health reservation outside unchanged 32768–60999 ephemeral range, actual Go/YAML metrics consistent and no process/deadline/capacity/production-policy change. Complete four-target canonical batch is active, shell `sh_12d43cda8001UTtK2f0IvT4TZv`; no result yet. It validates IT-03 corrections and executes the new cancellation count assertion for real RED. Repository-wide new-diff lint passes zero issues; fresh complete-unit profiles reproduce 1,169/1,199 changed production statements without source/denominator changes. User reconfirms fixing all known issues before logical commits/push. No publication or unrun AF/fleet pass is claimed. |

| 1.33 | 2026-10-11 | Complete IT-04 finishes exit 2 / 503.209s: DS 182/182, AA 76/76 and RO 150/150 pass; KA has four failures across investigator/catalog, with no pending/skipped cases. Independently verified archive SHA-256 `b8bfb2261122ab2be0751b36b3b035238fd7a2b05ae52f79b657d002f0ee9f01`, four profiles/activity capture and all nine preservation checks; failed-batch profiles remain excluded from canonical-success coverage. Twenty-one of the original 23 investigator failures pass. The two provider cases now reach the real frozen packet but fail exact microsecond timestamps; the unchanged pinned ogen RFC3339 codec emits whole seconds. Seed whole-second fixtures, retaining exact equality, ordering, zero/unknown and privacy assertions. Cancellation IT-KA-970-003 supplies actual RED (0 tools started, expected 4); replace only denied synthetic names with existing allowed RCA readers, retaining the 100ms deadline, execution count, cancellation flag and cleanup assertion. User explicitly approves test-only catalog IT-KA-1677-DISC-005 readiness repair: wait for both owned Active/Disabled status updates, retain 5s/100ms timing and all filtering assertions, no production catalog change. Compilation, repository new-diff lint and diff checks pass; runtime GREEN still pending. Fresh remote global-idle/all-TCP preflight finds +4400 unused/outside unchanged ephemeral range and IT-05/sync16 identities absent. |

| 1.34 | 2026-10-12 | Complete IT-05 finishes exit 2 / 506.653s: DS 168/182, AA 76/76, RO 150/150, investigator 171/174, catalog 47/47; all other KA suites pass, zero pending/skipped. Independently verified archive `ea116004c08310d4d1495e180f60de82073e454793b299548279c65a4eddd384`, four profiles, activity capture and nine preservation checks in `issue2496-it05-failure-diagnosis-j1t18y3_`. Activity evidence has 824 blocked-row samples (727 relation / 97 advisory), including ALTER blocked by DELETE for 56.766s; do not conflate row samples with distinct queries or claim a specific DELETE caller. Latest full local Make fails two unchanged SP stale-cache fixtures, 11,524 specs / 114 suites / 254.040s; 3,204 hashes and twelve profiles independently verified, archive `a0297e485839916bb85b78b148d0b0ee4d29f1b86f14d33e560dd2c4450bdb25`. Both failed-run profile sets remain excluded from successful coverage. User approves SP timestamps after fake-API setup, preserving assertions and 30s production safety valve. Provider tests now fail unknown-vs-false: seed the existing alert_resolution.alert_resolved field on typed alert events; no codec or production history change. User rejects Serial and dependency omission, then explicitly approves one private retention database in the existing instance and cancellation's 100ms window after four readers start, keeping real DS services and adding authenticated persisted-event proof. See §16.5. SSH DNS is blocked; no gate is active. |

| 1.35 | 2026-10-12 | Test-only private retention DB and four-start cancellation/persisted-audit candidates compile and pass integration-tagged new-diff lint; repository build/all-package compile/new-diff lint/diff checks pass. All 2,997 protected non-test files/settings unchanged. Latest complete unit Make exit 2 / 283.563s, 11,524 specs/114 suites; SP stale-cache fixes pass, one untouched UT-SP-2467-001 fails Rego evaluation's unchanged 100ms guard. Policy preparation happens before that guard; exact cause is not proved. Evaluator, test and chart policy match origin/main byte-for-byte. Twelve profiles/3,204 Go hashes independently verified; archive `af0a59a74ecb25ad6ea5aa5c5d7d0d29ad70c48014a372abfe0c71c91cf6f5ec`, failed profiles excluded. User confirms restored SSH; read-only preflight proves idle, existing CREATEDB/superuser role, unchanged max_connections=200, all 834 +4500 reservations unused/outside ephemeral range and fresh identities absent. Idle-only sync17 verifies 5,910 files, manifest `1fd41d3182b5f7985ad2ec0c90c4b3f99658ceb2a25ae9ebf2d497931ecb6198`, old counters/images/inode/ledger preserved. Fresh IT-06 applies matching Go/YAML bindings, RO health32640, no image rebuild. Complete four-target IT active `sh_130321d7a0010SK0LqJGLdVxI5`; unchanged complete local Make reproduction active `sh_130350be7001jq02KGb7z2GZBg`. No result yet, no timeout/process/focus/skip override; no publication. |

| 1.36 | 2026-10-12 | IT-06 completes exit 2 / 503.054s, DS 171/182, AA 73/76, RO 150/150, investigator 174/174 and catalog 47/47; all other KA suites pass, zero pending/skipped. Independent archive `b43e5d765052375f6b3c6a4252073e3a223da5f29ebd66df269634b1aa2d09f5`, four profiles, activity capture and all nine preservation checks verified in `issue2496-it06-failure-diagnosis-b8h3bibk`. Investigator proves all four readers active before the unchanged 100ms cancellation window, cleanup, authenticated persisted cancellation and exact history/privacy assertions. Separate read-only proof confirms the retained private retention DB has real audit/retention tables and five audit triggers in the same PostgreSQL instance. Shared DB activity still has 233 blocked-row samples, including a DROP waiting ~54.79s on DELETE; raw SQL/caller is unknown, and the separate partition group still drops shared partitions. AA RED is namespace AlreadyExists/deleting: randSuffix uses seconds plus literal 000000. User explicitly approves private DB isolation for that partition group and UUID namespace suffixes. Full unchanged unit reproduction completes exit 2 / 256.306s, 11,524 specs/114 suites: Rego failure does not reproduce, but the untouched sleep-only shutdown simulation times out (configured 200ms, observed 250ms). Independent twelve profiles/3,204 source hashes verified, archive `8265a25d5663514d43ad2767130a0021acd70529ec4525a4a8c9188d4a955dc8`; failed profiles excluded. User asks for the existing middle-proxy approach rather than approving the proposed sleep-to-channel simulation edit. No gate remains active; no production/deadline/process/capacity edits or publication. |

| 1.38 | 2026-10-12 | User approves verified embedded db.DB argument and real proxy-gated audit-shutdown replacement. Partition group gets its own database in the same instance, original connection settings/parallelism and all assertions retained, both sequential connections verify private identity, shared test partitions remain absent; AA suffixes use UUIDs. SP test uses real BufferedAuditStore/generated client/forwarding request gate, producer cancellation, held-delivery wait, unchanged 200ms/10ms completion assertion and checked cleanup; not controller/SIGTERM/real DB proof. Integration compilation/new-diff lint, repository build/compile/new-diff lint/diff checks pass. Initial typecheck logs retained. Protection accounting corrected: prior 2,997 population includes 1,354 tests; only approved SP test in that population changes, 2,996 others/all 1,643 non-test hashes unchanged, no old hashes rewritten. Complete local make test passes 11,524 specs/114 suites, exit 0/284.799s, zero failures/pending and two unchanged kubeconfig-dependent skips. Independently verified twelve profiles/3,204 Go hashes, archive `fca27c55291d6e2e0b1541a25481f9ed2059dc47c9831cc8a3ef987a0835a5c9`. Idle-only sync18 verifies 5,910 files, manifest `2ca334ec5bcb1a51f07bf34054953c6137122ea9c6a23c29dc8a024798757683`, older counters/images/inode/ledger preserved; local ledger excluded. Fresh all-TCP preflight finds all 834 +4600 reservations unused/outside unchanged 32768–60999; IT07/helper/artifact identities absent, existing CREATEDB role/max_connections=200 unchanged. One-use overlay rechecks isolation and matching Go/YAML ports; RO health 32740. Complete four-target IT-07 ACTIVE `sh_130666d8d001KujEIpdWHtV6C0`, no result or process/focus/skip/deadline/capacity/image override. No publication. |

| 1.39 | 2026-10-12 | Complete IT-07 finishes exit 2 / 508.521s: DS 182/182, AA 76/76, RO 150/150, investigator 174/174, catalog 47/47 and all other KA suites pass except MCP 102/103; zero pending/skipped. Independently verify archive `dc36baf90269620a5809d8b525864778f2a37b4613175acf4d2fe596b9e9841a`, four profiles, activity capture and all nine preservation checks in `issue2496-it07-failure-diagnosis-jrh_ei3r`; failed-batch profiles remain excluded from canonical-success coverage. Shared DS capture has zero blocked-row samples; a long-running DELETE remains visible but its raw SQL/caller is unknown. Read-only proof `issue2496-it07-private-database-proof-5dbmj4ed` confirms distinct retention/partition private databases in the same PG instance, each with real tables/five triggers; shared action_history has no 2029 test partitions. Remaining failure is untouched IT-KA-UX-04 at interactive_ux_test.go:186: session ID appears in expiration callbacks after cancel was sent. Test checks transport err only, not MCP IsError/status. Source starts its unchanged 300ms inactivity timer before real DS reconstruction; cancel resolves the real Lease driver before StopTracking, and Stop cannot retract an already-fired callback. Exact runtime ordering/cause is not captured; no production flaw or fixture cause is yet proved. No retry-as-fix or deadline increase. Complete unit remains PASS with fresh source-attributed coverage 1169/1199 (97.50%), same 30 defensive exclusions. No background gate remains active. |

| 1.41 | 2026-10-12 | User explicitly approves §16.8 MCP accepted-status/identity/timing/expiry diagnostics. Only IT-KA-UX-04 changes, unchanged real DS/Lease and 300ms/500ms windows. Integration-tagged compilation/new-diff lint, repository build/compile/new-diff lint and diff checks pass; all 2,997 current protected-file hashes and original guard manifest unchanged, diagnostic source `db16fdb88e66dcfae8f058ed02bf0f3db4cd12385c8ed297a0623497fcbd2c1c`. Fresh preflight rejects +1800/+1700/+800/+500 socket collisions; +300 has 834 unused reservations outside unchanged 32768–60999 and was not used by any earlier IT overlay. Idle-only non-deleting sync19 verifies 5,910 files, manifest `0931f0fd59162225461e48dbd1820fc0515d5036b0d4c3cf2555459492b98994`, archive `fd834123a7ba6fc13866d305b497ac425e15d0ffa383c2f1e909898b4ce80f73`; 5,005 older counters/inode/fourteen image IDs/remote ledger preserved, local ledger excluded. One-use IT08 helper rechecks actual bindings, RO health28440. Complete four-target IT08 ACTIVE `sh_130846a7e0016jXATc5iRQMBxx`; unchanged full-unit source refresh ACTIVE `sh_130846a980012k93qlhOHJpbNs`. No results yet, no process/focus/skip/deadline/capacity override or image build. No claim diagnostics fix IT07 or establish its root cause. |

| 1.42 | 2026-10-12 | Local full-unit source refresh COMPLETES exit 2 / 311.897s, 11,524 specs/114 suites, one failure: untouched UT-EM-254-006a at reconciler_partial_scope_test.go:144 returned zero instead of exact 5s requeue; scenario took 39.254s. Original fixture stamps EA creation five seconds before now, BEFORE building fake API/reconciler, while production partial grace uses real time.Since against unchanged 30s window after hash/health. Scenario and production paths identical to origin/main; exact time spent before versus inside reconciliation is not captured. Existing ReconcilerDeps.Clock controls only validity checks, not partial-grace time.Since; no clock change is proposed/applied. Archive `8a06f973fc1c2b362f3dbcf97f81a96cb08fc931e880e3ad264dafc78ed7b26d`, all 3,204 Go hashes/twelve profiles independently verified; failed profile set excluded. Parallel Make interleaves suite headings: corrected proof attributes failure by its explicit assertion source path, not the misleading preceding SignalProcessing heading; raw archive/initial proof preserved. Complete IT08 remains ACTIVE; no Go or remote changes. No blind retry, timing weakening or root-cause conclusion. |

| 1.43 | 2026-10-12 | Complete IT08 finishes PASS exit 0 / 500.511s, 913 specs/24 suites, zero failures/pending/skips: DS182/182, AA76/76, RO150/150, investigator174/174, MCP103/103, catalog47/47 and every other KA suite PASS. Archive `6ab6fa78488c9e0295f0304ef96ccf753bebae5cf20ecfb782c2ace2168e36a5`, four profiles/activity/all nine preservation checks independently verified in `issue2496-it08-canonical-proof-f1dmgl77`; successful profiles eligible for final attributable merge. DS capture zero blocked-row samples. MCP diagnostics require accepted started/cancelled and matching identity: start297.550689ms, before cancel297.685231ms, cancel10.091791ms/total307.800291ms, final808.392092ms; all expiration snapshots empty. Start elapsed includes work before the internal inactivity timer is armed, so do not subtract these figures from 300ms or infer IT07 ordering. The new assertions prove this execution's intended cancellation contract, not a repair or cause of the old failure. Latest unit refresh still failed the untouched EM grace fixture; no active operations, no source edit/rerun authorized by that new failure. |

| 1.45 | 2026-10-12 | User explicitly approves measuring stages in only UT-EM-254-006a. Scheme/fake-client/reconciler setup, persisted pre-dispatch EA age, reconciliation/result/phase/reason and observation-read timings added; original creation timestamp/30s grace/exact5s assertion retained, after-grace companion byte-identical. Unit compilation/new-diff lint, repository build/compile/new-diff lint and diff checks PASS, all2,997 current protected files and old manifest unchanged; diagnostic SHA `1c3a3a81af1fdbae85dec95a95f801251c99ddc5bd95dacd404168fd82972842`, evidence `issue2496-em-approved-stage-diagnosis-wi7t_fbe`. Initial own QF1008 embedded-selector lint retained and corrected via promoted Format method, no suppression. Complete unchanged `make test` ACTIVE `sh_130a174ab001cB3W4vbOycPIzx`, label `em-original-window-stage-diagnosis`, no result yet or process/focus/skip/deadline/capacity override. Remote idle, no new sync/image build/deployment. Diagnostics are not a repair or retroactive root cause. |

| 1.46 | 2026-10-12 | Complete unit timing invocation finishes exit2/290.143s, 11,244 executed specs/104 BDD suites, two failures: EM006a and DS1048QEM2-002. Archive `f3a952fd840467da86d074ffc4f37ab392e33d31077be5d67ab4a2c865bf7176`, 3,204 Go hashes/twelve profiles verified and excluded; original proof/archive preserved, v2 adds explicit scenario/source-path attribution. EM construction38.878418s, pre-dispatch EA age44.196060s versus30s grace, reconciliation80.68ms, resulting Completed/Partial: setup consumed the grace in this measured reproduction, not a production failure or retrospective proof of every earlier failure. DS rejects1,048,686-byte payload against1,048,576-byte limit at4.983s into5s context; returns at5.010414s with timed_out=true, errors0/events0/nil error. Source cancellation exits and existing timeout/cancellation specs deliberately allow nil error; this execution does not prove the permanent-failure two-pass branch completed. Exact internal time cost unknown. Ten DS BDD suites/280 specs did not execute because the unchanged dedicated Make target lacks --keep-going; pinned Ginkgo skips later suites on prior failure and prints discovered len(suites) in its "ran" line. No user skip/process/deadline override. Test-only alternatives in§16.10 require explicit approval. No active gate, source repair, new sync/build/deployment, commit/push/PR. |

| 1.48 | 2026-10-12 | User re-asks dismissed questions and explicitly approves EM Readiness-first fixture and DS Separate contracts. Only EM006a/DS1048QEM2-002 repaired. EM recent fixture created after dependencies with same5s age/30s grace/exact5s assertion; real hash/health activity via existing metrics, stored nonterminal outcome, after-grace companion unchanged. DS removes ONLYthisscenario's5s deadline, requires specific two-pass error/no timeout/zero writes/identical retained ID+payload; every other deadline/cancellation case byte-identical. Build/affected+all-package compilation/affected+repository new-diff lint/diff checks PASS; all3,206 protected current Go/config hashes/three original guard manifests/shared EM builder unchanged. Source hashes EM `47acb13c0b9fcbcd80fae961ef7033070a36357924add832a0eed7113e1b5842`, DS `d9ef3a83764123c4b9ad9e7cb66554c95c8a499b6f1cb762434352f2481fc272`, evidence `issue2496-approved-em-dlq-repairs-f0f4nge2`. No production clock/status/timeout/dependency/canonical gate change. Complete unchanged unit Make ACTIVE `sh_130d5dff9001kx7yTQPwrVYyMq`, label approved-em-dlq-contract-repairs, no result yet. Setup cost hypothesis not declared fixed; prior failed profiles remain excluded. |

| 1.49 | 2026-10-12 | Full unit invocation finishes exit2/319.768s, all11,524 specs/114BDD suites executed. One failure is our NEW metric series-count assertion, not the repaired grace contract: stored age6.501237s, Reconcile313.674ms returns exact5s, stored Assessing/uncompleted; CollectAndCount(curried CounterVec) got2 (hash+health) versus1 because CurryWith does not filter collection, per client_golang v1.23.2 counter.go309–313. DLQ validation completes9.870879s, specific2passerror, TimedOutfalse/zero writes/same retained Redis entry allPASS. Archive `ccd056514935c89088cb9f262f9d00b6f55ad9e54189d0f75c6548962dc11aff`, all3,204 Go/twelve profiles verified/excluded. User says carry on; correct only our assertion by summing individual labeled counter values for each component over existing success/error/skipped results, still require exactlyone assessment per component. Before/counter correction snapshot `issue2496-em-metric-assertion-correction-3_vqavev`, all3,207 other current Go/config files/four old guards protected. No production/test-context/canonical deadline/process/dependency change, no active operation; validation/fresh full unit gate pending. |

| 1.50 | 2026-10-12 | Own metric observation corrected using per-component/outcome counter values, not collected series cardinality. Snapshot `issue2496-em-metric-assertion-correction-3_vqavev`: only metric loop changes, all3,207 other Go/config files/four original guard manifests unchanged, all readiness/original/companion assertions and DS repair preserved. Build/affected+repository compilation/new-diff lint/diff PASS, source `8afc3e6749dc6b72b221d326be2498955e0b1fb76f73ffc4c8a4f83635f2161d`. Complete unchanged local unit Make ACTIVE `sh_1316fb117001ZuFxSnBN4rOAME`, label em-metric-values-contract-green; no result yet, no process/focus/skip/deadline/dependency/capacity change or remote sync/image/deployment. Prior failed archive/profiles preserved/excluded. |

| 1.51 | 2026-10-12 | Complete current unit Make PASS0/264.778s, all11,524 specs/114BDD suites, zero failed/pending, two unchanged kubeconfig skips. Archive `a67b995a3f2aab3a576b56994bfa704e42fb658fdaf3e571a8dfa7b9871a3dfe`, all3,204 source hashes/twelve profiles/3,207 protected files/four original guard manifests independently verified in issue2496-em-metric-values-contract-green-8wtflm0b. EM age5.540057s/exact5srequeue/Assessing/uncompleted/hash1health1 and DS8.077502s/specific2passerror/TimedOutfalse/zeroDBwrites/same retained entry PASS. Current source-attributed characterization1169/1199(97.50%), identical production hashes and same30 uncovered defensive statements; reportSHA `4d1665c6525067417a01be44b697872cda29ae750e116d49960d1cea3cca705f`. Failed sets remain excluded. Read-only AF preflight os0exvps shows identities absent/+700candidate outside32768–60999, not assigned/reserved; allTCP/idle checks must repeat immediately, later fleet cannot reuse local's retained ports. Engram strict-fresh semantic search resource-busy admission timeout reported/no stale results or alternate route. No active operation, sync20/AFlocal next, no new instrumentation/imagebuild. |

| 1.52 | 2026-10-12 | Immediate AF-local preflight z6kt48fw proves globalremoteidle/freshabsent identities and twelve unused+700bindings outside unchanged32768–60999. Idle-only non-deleting sync20 verifies5,910files, manifest `15f3aad798fcba61d530f94408ed03bf9d408c63bfbc013a9640d0e883341abb`, archive `0149aecdf209f460ed3d1448683c9683b6680a6095fc15272da34265e2112723`, staging issue2496-verify-me7vko36; all5,005oldercounters/mountinode/fourteenimages/remoteledger preserved and localledger excluded. One-use AF local helper SHA `ed29e4215840bcca58f272002a53216d25097d551decd4673294ac481a6249e4` rechecks allTCP/globalidle, actualKind bindings match declared offsets, retainedCAs unchanged/uniquefreshCApath. Complete normal AF local Make ACTIVE `sh_13177c99d001cE49LjHjkWzJ3x`, cluster issue2496-af-local-clean-01, artifact e2e-af-local-complete-canonical-01, canonical !fleet-mode-af lane label; no result yet. No focus/skip/process/deadline override, rebuild or deployedcoverage/stalebinaryprofile/cleanup. No furtherremote mutation/sync/build whileactive. |

| 1.53 | 2026-10-12 | AF local Make finishes exit2/808.500s,194passed/12failed/18normal-lane-filtered/0pending; suite781.012s. Wrapper exits1 because terminal deployment inventory unavailable. Archive `7956ad3403b5adb658d504e337d8d18f871dd59a42afc75179580f69e4c4568a`, one runner profile/raw twelve assertion blocks independently verified in issue2496-af-local01-failure-diagnosis-zp532wju; failed profile excluded. Eight source/oldcounter/inode/image/oldcluster/CA/run-image checks pass, but the NEW failed cluster was deleted. Our KEEP_CLUSTER/PRESERVE_E2E_CLUSTER settings are not read by AF teardown; actual AF_E2E_SKIP_TEARDOWN guard omitted. Blank IMAGE_REGISTRY (needed for local artifact loading) selects DeleteCluster's local-deletion path. Manifest no_cleanup=true was contradicted by runtime output. Both logical local/Fleet names equal, explaining must-gather scope failure. Several other cases show Mock LLM EOF; cause still unknown. Named own/tmp diagnostic recovery and runner correction decisions requested; no active operation, recreation/cleanup/prodchange/deadline increase or retry. |

| 1.54 | 2026-10-12 | User approves named read-only diagnostic recovery and runner-only containment. Two DNS attempts fail before any remote action; user reconnects VPN and recovery succeeds. Private archive `667a4222d252b25931681f7f692cd8da604baf3090f7e173405c533d77f99ff6`, 114mustgather+72node-exportfiles copied into owned rootartifact/localprivate directory, originals hashes/inodes/mtimes unchanged. Saved AF/Mock descriptions Running/Ready/0restarts/no recordedOOM; not live/restored state.105AFEOFs matched to132stream openings, elapsed60.689–95.025s/median76.984s; MocksourceWriteTimeout60s, no recorded artificialsecond-turn delay, exactprovider stage cost/CPUcause stillUNKNOWN. Runner corrected actualAF_E2E_SKIP_TEARDOWN flag/distinctlogicalnames+kubeconfigs/freshcluster+oldercontainer checks. Sandbox executes actual copiedscope/teardownguard with externaldelete trap: bothlanesREDthenGREEN; no SSH/Make/Kind/livestate operation. RunnerSHA `1cad892a9dc6e14df1f4a41d6fc6210b0d07a0bea847304e68e477d93e8119bd`, originaledaf...retained; all3,204Go/Make/dependencies/otherhelpers unchanged. No testgate resumed. Skip accounting CORRECTED:11normal lane exclusions+7unexecuted ordered followups afterpriorfailure, not18laneexclusions; v2proof preservesrawarchive/oldproof. RemainingEOF/behavior stillblockpublication; no capacity/timeout/production change authorized. |

| 1.55 | 2026-10-12 | User explicitly approves controlled provider probe: unchanged prebuiltMock image/original200mCPU/256Mi limit/60sHTTPbudget, onevs40diagnostic clients, existingmetrics+cgroupcounters only; noAFcluster recreation/rebuild/coverage/logginginstrumentation/productionchange/canonicaloverride. Fresh remote-globalidle/absenthelper+container+artifacts/allTCPunusedport8257outside unchanged32768–60999 verified. Diagnostic payload28,741bytes uses actualstaticAFprompt plus18observedAFtoolnames in oneGemini declaration block, noFunctionResponse/submit-with-workflow; parameter schemas synthetic and originaloverride/config/body/SDK not replayed. PayloadSHA `ee5699981a373070a468ae2f9de1e9e199f3d2c1b4ea9313f7284c1801e6321a`, helperSHA `15df17d2bb8bd0d799d7a386274e0a3dc41de0db6456fd0c64acc54afd6a4aa9`; inputartifact issue2496-af-provider-probe-input-_re03pub. Probe ACTIVE `sh_1320b9b64001Gu5UX5ZeoebuC8`, owncontainer issue2496-af-provider-probe-01, no result yet. Containerretained/pullnever/loopbackonly; diagnostics cannot replace Make or authorize fixes after nonreproduction. No canonical gate/sync/build active. |

| 1.56 | 2026-10-12 | Controlled provider probe completes1/1+40/40validresponses, noEOF. Single0.975s,40clients36.309–44.607s; original200m CPU447/448throttledperiods,8.939sCPUusage/80.956saggregate throttledtime. Seven oldercontainer/cluster/image/counter/inode/CA/ledger checksPASS. Remoteowncontainer retained; resultSHA `1effd922e738526224a086bbcc38f90cf4483c418126647f34d4ea958fd6b765`, four existingmetrics hashes independently verified in issue2496-af-provider-probe01-verified-n43p72ad. Throttling proves diagnosticquota contention, NOT originalAFEOFcause. Staticprompt/observednames but syntheticparameter schemas/nooriginalconfig/body/SDK/conversation replay; nofix/capacity/deadline/canonicalretry authorized bynegative result. Noactiveoperation; actualfullschema/transport/config attribution stillneeded. |

| 1.57 | 2026-10-12 | User approves Use realAFrequest andcarryon. Existing NewRootAgent/tool+dynamicinstruction/model/ADK/GenAIv1.71.0 builders emit exactlyone capturedHTTPcall/26realfullschemas/58,061bytes, no tool/cluster operation; all3,204Go hashes unchanged. RequestSHA `07d3738a7db37258c96591b77925b90df96d9a7529f676adac253958fb54c433`, capturear2xqibs. Freshownedprobe02 on8258 reuses frozenprobe01 experimentcode with ONLYidentities/input substitutions, sameimage200m256Mi/60sbudget/1vs40. Completes41/41validresponses/noEOF,1.003ssingle/34.672–48.423s40clients,484/486periodsthrottled. ResultSHA `2f4fe6b08c975c657ffe0dedb5c7fe9dbdae1d4e2d8af83b803817d839ab2eb3`, fourmetrics/sevenpreservationguards verifiedadmwukml, bothprovidercontainers retained. Originalconfig/conversation/identity/mixedKAload/networkcontext not replayed. SavedAFstreampeak40; post-detectionMocklogs show180Gemini+31OpenAI but cannot recover exactprovider overlap. No capacity/deadline/sourcefix or canonicalretry inferred from negative result. Noactiveoperation; actualfull-workload passive evidence proposal pendingexplicitdecision. |

**Current status**: complete **unit11,524/114** and **IT-08 913/24** GREEN. Both isolatedprovider probes, including realAFrequest, finishwithoutEOF; throttling ismeasured, originalcauseunresolved. **No operation active; no canonicalrerun.** Corrected runner/privateevidence retained. FailedAFlocal12/orderedunexecuted7 plusAFfleet/hubspokefleet/finalwiring-control/normalCI blockpublication; rawchanged97.50%/widerDS77.21% remainhonest, notreleaseproof.

Latest complete fresh fullpipeline `fullpipeline-clean-canonical-05` is **45 passed / zero failed/pending/skipped**. The BR-ORCH-036 selection-presence expectation, real cooldown/Job/patch, review/no-execution and independent live/audit reconstruction assertions pass. Preserve all earlier fullpipeline/DS failures. The user-approved CPU-limit-only E2E fixture correction passes emitted-manifest RED/GREEN, **237/237** infrastructure specs and the fresh complete canonical DS gate: **143/143**, zero failures/pending/skips. Strict TLS, authenticated SAR and SOC2/history assertions remain intact; production/Helm/process/deadline policy is unchanged. This is not a flaky-test or CI-executed claim. Final Helm Make passes **91 suites / 721 tests**. Current source-attributed changed-block UT characterization is **1,169/1,199 statements**. Per the user's clarification, assess meaningful reachable business/control behavior and document defensible exclusions, not artificial tests to force literal 100%. Eleven services now have passing attributable deployed coverage. Final merged coverage, complete affected IT, registered-origin fleet isolation and canonical AF/fleet gates still prevent release-readiness claims.

Latest completed affected **IT-08 is GREEN** across the complete unchanged four-target batch. IT07's failure remains archived with its cause unknown; accepted-cancellation diagnostics prove the strengthened contract in IT08 without retroactive causal claims. The latest unit gate is still failed. No integration capacity change is authorized by the E2E CPU-limit decision above, and no CI result exists for these uncommitted changes.

Publication remains blocked. The user explicitly requires known issues resolved
before logical commits, rebase, push and a new PR; do not interpret the CI/CD
preference as permission to publish a failing draft. Keep fullpipeline's CI job
limit at 45 minutes despite the documented 57-minute remote passing run. The
cancellation execution-count and persisted-event assertions pass in IT-06; this
does not turn the failed complete batch GREEN. Unrelated remote runs are not this
task's to stop, inspect internally or repair.

### 16.1 Exact production-caller verification (runtime gates still open)

Engram's current-source exact symbol/reference navigation verifies the following
production callers. This is source-level wiring evidence, **not** CHECKPOINT W
completion or a substitute for the complete canonical IT/E2E gates above.

| Boundary / objective | Actual production caller and dispatch | Required proving scenarios |
|---|---|---|
| Independent packet budget; SI-10 / AC-4 | `cmd/kubernautagent/bootstrap.go:buildInvestigationRunner` calls `buildInvestigator`, whose `Pipeline.MaxEvidenceHandoffBytes` comes from `AI.Investigation`, not the tool summarizer. | IT-PLATFORM-2496-001; IT-KA-2496-008 |
| Same frozen packet across entry points; AC-4 / SI-10 | `investigator.go:runWorkflowDiscoveryPhase` and `investigator_discovery.go:RunWorkflowDiscoveryFromRCA` both call `investigator_workflow_selection.go:runWorkflowSelection`; it freezes before any Phase 3 provider call and binds acceptance to that sanitized snapshot. | IT-KA-2496-003/004/008 |
| Repeat acceptance on initial/recovered/corrected selection; SI-10 | `completeWorkflowSelection` sends both first and parse-recovered results through `selfCorrectWorkflowSelection`, which calls `SetRepeatSelectionContext`; `validator.go:validateSelectionIdentity` calls `validateRepeatSelection` after catalog-authoritative identity/name resolution. | IT-KA-2496-006/007 |
| Off-phase denial before side effects; AC-6 / AU-3 | `investigator_loop.go:runLoopTurn` calls `processToolCalls`; both that batch/sentinel path and `investigator_tools.go:executeTool` check `toolAllowedInPhase` before registry/overlay dispatch. `emitToolCallAudit` uses the same trusted phase decision. | IT-KA-2498-001/002 |
| Normal history metadata producer; AU-3 / CC7.2 | `internal/controller/remediationorchestrator/reconciler.go:registerWorkflowPhaseHandlers` binds `emitWorkflowCreatedAudit` into `WFECreationCallbacks`, not a test-only emitter. | IT-RO-2496-002 |
| Authenticated stored-history reader; AC-4 / AU-3 | `pkg/datastorage/server/server_routes.go:registerAuditRoutes` registers `HandleGetRemediationHistoryContext` on `/remediation-history/context`; the existing repository/adapter supplies the two scoped tiers. | IT-DS-2496-001/002/003; E2E-DS-2496-001/002 |
| AgentSession result → admitted AA status; AU-3 / SI-10 | `agentsession/dispatcher.go:OnTerminal` calls `status_writer.go:writeTerminalStatus`, which invokes `MapInvestigationResultToAgentSessionResult`. AA's `handleSessionCompleted` invokes `ProcessAgentSessionResult`; that stores the non-executable assessment separately and `stampWorkflowSnapshot` invokes `stampSelectionMetadata`. | IT-AA-2496-001/002/004 |
| Live current selection without a DS read; AC-4 / SI-10 | `session/manager_events.go:emitCompleteEvent` calls `MarshalRCASubset`; AF `processBridgeEvent` calls `captureCompleteEventRCA`, and `ka_investigate_mcp.go:runBlockingInvestigation` emits the summary artifact. `recordInvestigateGroundingState` / `recordDiscoverWorkflowsCheckpoint` restore only trusted KA selection context. | IT-AF-2496-001/002; E2E-FP-2496-001–007 |
| One safe projection for live and durable paths; AU-3 / CC7.2 | `LiveSelectionFromResult` has production callers in the MCP discovery result, complete-event payload and `ResultToAuditJSON` → `marshalAuditResponseData`. AA audit uses `LiveSelectionContext.SafeCopy`; trusted prior links are added only at persistence. | IT-KA-2496-005; IT-AA-2496-002; correlated E2E audit reconstruction |

The three repository methodology heuristic targets also fail on unchanged HEAD
using identical scripts. Current/baseline counts are null flags 76/76, literal
assertions 1,754/1,712, orphan-type flags 15/15, no-BR flags 261/263 and direct-mock
flags 12/12. Each of the 42 additional literal flags asserts a BR-backed real
boundary output, and every new test file carries a BR. Do not suppress the checks,
equate these heuristics with exact production references, or claim they pass.

All older clusters/counters/images/history, failed artifacts and provenance remain retained. Do not change process counts, relax assertions, delete history or treat focused diagnostics as canonical gates. The excluded ledger is local-only and unsynchronized. No commit, rebase, push or PR before verification completes. A fresh, fully contextualized post-publication audit subagent and actionable-finding fixes, validation, commits and pushes remain required.

### 16.2 Current-source coverage attribution checkpoint

The passing fullpipeline-05 archive merges with the source-hash-verified complete
unit run and prior complete AF integration gate: **1,169/1,199 changed production
statements (97.50%)**; deployed E2E covers **879/1,199 (73.31%)**. This is measured
coverage, not a final business/control/Wiring Manifest completion claim. Remaining
successful affected IT profiles still need to join the immutable merge. No profile
from the failed IT batch or failed fullpipeline predecessors is accepted as
canonical success evidence.

The independent AF compiled-file inventory originally missed an E2E-only handler.
Inspection of the unchanged image binary proves `-tags=e2e,fips140v1.0`, Linux/amd64,
CGO disabled and its exact image/binary hashes. Regenerate that inventory with
the actual tags and verify every included repository file and build input against
the original serial snapshot; zero mismatches. The strict merge then passes,
without changing a production file, binary, coverage pattern or hash guard.

## 17. Reasonable coverage assessment (user clarification, 2026-10-11)

### Approved DS verification capacity correction (2026-10-11)

The user authorizes increasing fixture limits for the measured concurrency spike.
This is limited to inline **DS E2E test deployment manifests** in
`test/infrastructure/datastorage.go`, serving BR-DS-006. Production/Helm limits,
memory limits, CPU/memory requests, process selection, timeouts, and every old
cluster/data artifact remain unchanged. A fresh isolated DS cluster proves the
result without deleting retained data. Do not classify a successful retry as
flakiness or claim CI has run before the PR exists.

- **RED**: `UT-INFRA-DS-2496-CPU-001/002` capture the emitted Kubernetes Deployment
  through the existing external kubectl stub and require PostgreSQL 4 CPU / DS
  2 CPU limits while retaining requests and memory bounds. The complete canonical
  DS failure above is the real load RED. The infrastructure package has no Make
  unit target; its local diagnostics supplement, never replace, canonical Make.
- **GREEN**: change only the two test CPU limits; both manifest tests pass. Run
  complete `make test-e2e-datastorage` in a fresh isolated cluster, with the same
  read-only activity/CPU capture and no process/deadline overrides.
- **REFACTOR**: N/A — two declarative resource values leave no new logic to improve.
- **Success**: manifest bounds proven, canonical 143/143 pass without setup-derived
  skips, complete audit/history/control scenarios intact, measured contention
  assessed and all older provenance/resources preserved. If timeouts remain,
  retain failures and investigate; do not increase application deadlines.

Prioritize meaningful reachable business/control behavior, not artificial fault
injection or production seams solely to force every defensive return to execute.
Keep the measured raw coverage unchanged: the source-hash-verified complete unit
gate covers **1,169/1,199 changed non-generated production statements (97.50%)**.
No coverage pattern, denominator, production behavior or test assertion is changed
to obtain a higher number.

The 30 remaining statements are reviewed as follows. These justifications are
limited to current production input/construction contracts; reassess if they
change. They do not excuse a missing real input-validation or control-boundary
test, and do not make a failed deployed gate pass.

| File / residual statements | Grounds for not forcing execution | Existing meaningful proof |
|---|---|---|
| `internal/kubernautagent/investigator/evidence_handoff.go` — 4 | Decode after successful typed encode/sanitization; downstream packet encode after the source already passed JSON serialization and observation capture. Packet DTOs contain JSON-safe typed fields. Invalid/non-finite original data and operator-policy errors are tested before this boundary. | UT-KA-2496-008/010; IT-KA-2496-008: whole-packet budget, source failure, immutable freeze, safe observations and observable review. |
| `internal/kubernautagent/investigator/evidence_scope.go` — 16 | Missing mandatory sanitizer requires bypassing the production constructor. Remaining recursive propagation/encode/decode guards sit behind complete JSON validation or successfully parsed YAML Nodes and an in-memory buffer. Preserve the defensive guards; do not replace standard-library writers/encoders or mutate internal nodes merely for counts. | UT-KA-2496-008/010: malformed and multi-document YAML, invalid JSON/UTF-8, nested Secrets, credentials, hostile text, optional operator rejection, isolation and fail-closed capture. |
| `pkg/kubernautagent/types/live_selection.go` — 2 | `SafeCopy` produces a typed finite, bounded, UTF-8-safe DTO; its JSON object is then decoded to a JSON-shaped map. No custom marshal hook or arbitrary caller object is admitted. | UT-AF-2496-001 and live-selection UT: known zero, missing/invalid scores, partial rejected reasoning, oversized/invalid text omission, deep-copy and gob-safe shape. |
| `pkg/apifrontend/agent/phase_guard.go` — 3; `pkg/apifrontend/launcher/event_bridge.go` — 2; `pkg/apifrontend/launcher/completion.go` — 1 | Error propagation from the same safe typed `ToMap` boundary above. Reachable persisted-state decoding, state-write failures and external response errors remain separately tested; these are not exclusions for those real failure paths. | UT-AF-2496-001/002 plus persisted-state/error cases; IT-AF-2496-001/002: trusted source only, clear stale context, observable errors and recovery without DS current-result reads. |
| `pkg/apifrontend/launcher/schema_validator.go` — 2 | Failure compiling the fixed embedded sub-schemas, or reporting that initialization failure later. Current schema compilation and valid/invalid payload tests prove the shipped schema set; no mutable production schema hook is introduced for coverage. | Schema/launcher UT and IT-AF-2496-001/002: accepted and rejected allow-lists, bounds, privacy, malformed contexts and observable text fallback. |

**Release interpretation**: close real behavioral gaps and prove the approved
wiring/control journeys. Report raw unit and source-attributed merged coverage
honestly. The exclusions above rationalize a narrow defensive statement gap,
not complete release readiness or whole-system compliance certification.

### 16.5 IT-05 fixture repair plan — user approved 2026-10-12

This is **test-only**. Do not serialize the retention group, reduce parallel
processes, add PostgreSQL instances/capacity, omit DS dependencies, alter production
code/config/RBAC, or change production deadlines. The feature plan stays approved.

| Phase | Scope and measurable result | Estimate |
|---|---|---|
| RED (observed) | IT-05: 14 DS failures with shared-parent relation locks; IT-KA-970-003 starts 0/4 readers; IT-KA-2496-001/006 receive unknown resolution instead of seeded false. Latest complete unit Make: UT-SP-437-001/002 cross the existing 30s stale-context safety valve during fixture setup. Preserve raw logs/profiles. | Completed |
| GREEN candidates | Initialize one unique, suite-owned database in `test/integration/datastorage/retention_test.go`, using the same instance and existing real migration/partition helpers. Route only that group's SQL to it; verify its identity differs from the shared database. Keep all retention assertions and all 182 specs. In the cancellation IT, keep real DS enrichment/audit, start the existing 100ms timer after four readers enter Execute, preserve count-four/cancelled/zero-active assertions, and query the persisted typed record by unique correlation ID. Keep approved SP/history fixture corrections. | 20–30m, excluding connectivity |
| REFACTOR | Make test-fixture ownership/lifecycle explicit: close pools before dropping only the newly created database (retain it under the existing preservation flag), check cleanup errors, stop fixture timers and ensure the investigation goroutine exits. No new production type/component. | 10m |
| Verification | Formatting, integration-tagged compilation, repository build/all-package compile and new-diff lint; complete local `make test`. After DNS recovers: fresh global-idle/all-TCP preflight, non-deleting sync excluding local ledger, one-use outside-ephemeral overlay, unchanged complete four-target IT Make. Independently verify source/archive/profile hashes and preservation. | Local ~5m; IT ~8–10m |

**Cancellation control proof:** IT-KA-970-003 explicitly owns AU-2/AU-3 and
SOC2 CC7.2 persistence/wiring evidence: exactly one authenticated DS cancellation
record, valid identifiers, actor/action/outcome/correlation, RCA phase/turn,
session and cumulative token metadata, reconstructable accumulated messages,
no selected workflow, and all four tools cleaned up. Unit tests prove
emission/payload logic; KA E2E-INT-009 proves MCP cancel/Lease release but does not
query this cancellation event. Fleet/fullpipeline evidence does not replace IT.
No new deployed instrumentation is permitted.

**Wiring:** no new production component or entry point. Existing `Investigate` →
`emitCancellationAudit` → `DSAuditStore` → authenticated DS query is exercised by
the strengthened IT. The private database is an external test fixture only. Any
newly exposed production audit defect must be assessed separately, not hidden.

**Rollback:** revert only these new owned test-fixture edits if they fail the
contract; preserve older runs/resources and the shared database. No runtime
GREEN/control-completeness/release-readiness claim until complete gates pass.
SSH failure is DNS (`helios08.lab.eng.tlv2.redhat.com`), not evidence of a database
permission failure; runtime capability validation remains pending.

Capability preflight subsequently succeeds after SSH recovers (revision 1.35);
the existing test role already has CREATEDB/superuser, no grant/config change.
IT-06 subsequently proves private retention isolation and the investigator's
persisted cancellation assertions. The complete DS/AA batch remains failed;
failed-batch profiles are not canonical-success coverage evidence.

### 16.6 IT-06 follow-ups — user approved partition/namespace fixes

The approved feature contract stays unchanged. Fixture preflight confidence is
**95%**: the shared partition cleanup and seconds-only namespace collision are
confirmed in current source and archived runtime failures. The exact blocked
DELETE caller and the Rego timeout cause remain unknown; neither is invented.

| Phase | Scope and measurable result | Estimate |
|---|---|---|
| RED (observed) | IT-06's shared partition DDL contention and three AA namespace AlreadyExists failures; preserve all eleven DS failures without claiming every one has the same cause. | Completed |
| GREEN | One uniquely owned private DB for `partition_ensure_test.go` in the same test PostgreSQL instance, real migrations/partitions/triggers, every original assertion and parallel process retained. Both sequential connections must resolve to this private DB; verify identity before migrations and no shared test partition creation. Replace only `capacityretry`'s namespace helper with UUID suffixes. | 15–20m |
| REFACTOR | Explicit fixture pool/database ownership and checked cleanup; parameterize the existing PostgreSQL connection helper without altering its default action_history DB, connection settings or any other caller's behavior. Keep the retention group unchanged. | 5–10m |
| Verification | Integration-tagged compilation/new-diff lint, repository build/compile/new-diff lint and diff checks. Fresh idle-only non-deleting sync, new outside-ephemeral IT identity, complete unchanged four-target Make, archive/source/profile preservation verification and independent private-DB identities. No timeout, process, capacity, image or production-policy change. | IT ~8–10m plus local checks |

**Wiring:** test fixtures only, no new production component or entry point. Real
partition functions and AA status/approval dispatch stay intact. Retain older
resources, CAs, counters, source provenance and failed profiles; only newly owned
DBs may be dropped on normal cleanup, never force-close other sessions. Keep
private DBs under `KEEP_CONTAINERS_ON_FAILURE`.

**Shutdown:** the user rejects the fake-worker sleep-to-channel direction and
subsequently approves the proxy-gated audit replacement in §16.7. Its scope is the
real audit resource's shutdown, not manager/SIGTERM or database persistence.

**Success / rollback:** all original DS/AA scenarios pass complete gates, no
shared fixture DDL contamination and all preservation checks pass. A successful
retry alone is not a fix or proof of flakiness. Roll back only these owned edits
if the fixture contract fails; investigate remaining failures before publication.

### 16.7 Proxy-gated audit shutdown — user approved 2026-10-12

Use the existing request-gate pattern from DS graceful-shutdown integration tests,
not `InterruptibleProxy.Pause`, which terminates connections. Replace only the
failing sleep-based simulation in `pkg/signalprocessing/controller_shutdown_test.go`.
The real boundary is `cmd/signalprocessing/main.go`'s exit cleanup calling
`BufferedAuditStore.Close`; use real buffering, shutdown flush and the generated
DS adapter, with only the external DS HTTP endpoint replaced by a fixture.

- **RED:** archived full-unit shutdown simulation failure (200ms assertion,
  observed 250ms); existing simulation provides no production resource evidence.
  No production shutdown defect is inferred from that failure.
- **GREEN:** one valid correlated audit event → real buffered store → generated
  client → request-gated HTTP forwarding proxy → DS endpoint fixture. Begin Close,
  observe actual batch request held in the proxy, cancel the original producer
  context, and prove Close does not finish while the delivery is held. Release the
  proxy and retain the existing **200ms / 10ms** completion assertion; verify the
  forwarded typed batch and successful response. No artificial worker sleeps or
  production timeout/configuration change.
- **REFACTOR:** explicit gate ownership and cleanup ordering; release on every
  failure path, wait for Close's worker result, close per-test transports/servers
  and propagate fixture errors without assertions in unprotected goroutines.
- **Verification:** build/all-package compilation/new-diff lint, complete local
  `make test` without process/focus/skip/deadline overrides; immutable source and
  profile capture. Diagnostics are supplemental, not substitutes for Make.
- **Control / scope:** BR-SP-090, ADR-032/DD-007, AU-2/AU-3 and CC7.2 audit-loss
  prevention at the existing resource boundary. Unit proof does **not** establish
  real DS persistence, authenticated storage, whole controller wiring or SIGTERM
  behavior; do not promote it to those IT/E2E claims.

The user also approves passing the verified embedded `db.DB` in the partition
fixture's new shared-isolation assertion. Its initial typecheck failure is retained
in `issue2496-it06-approved-followups-6izi33pb`; no infrastructure is missing.
Estimated implementation 15–20m, local gate ~5m. No new production component or
dependency; the existing production Wiring Manifest remains unchanged.

### 16.8 IT-07 MCP cancellation diagnosis — user approved 2026-10-12

The exact failure is **IT-KA-UX-04 / BR-INTERACTIVE-006**. The scenario, real-stack
helper, timeout manager and start/cancel paths are unchanged against origin/main.
The archive proves an expiration callback was observed, but does not prove it
occurred after an accepted cancellation or identify the timer/cancel ordering.
The test currently ignores MCP `IsError` and structured status. Production starts
tracking before DS reconstruction; real Lease reads precede StopTracking, and
`time.Timer.Stop` cannot retract an already-fired callback. These source facts
are evidence for diagnostic alternatives, not a runtime root-cause conclusion.

Approved **test-only diagnostic RED**: require start/cancel MCP success, `started`
and `cancelled` statuses with matching session ID; assert no expiration already
exists before cancel; record bounded start/cancel elapsed times and the observed
expiry snapshots. Retain real DS/Lease/timeout dependencies, **300ms** inactivity
and **500ms** no-expiration observation, canonical parallelism and all assertions.
Run the complete four-target Make batch after approved edits/compilation and a
fresh isolated idle-only sync. No retry-as-fix, production deadline/clock change,
dependency omission or synthetic timer replacement. The user explicitly approves
these diagnostic assertions and the complete rerun; production repair, if
indicated, still needs its own evidence-based alternatives and approval.

**Applied:** only IT-KA-UX-04 changes. Snapshot `issue2496-it07-approved-mcp-diagnosis-5zyyi7ah`
preserves its original source and the current protected-file hashes; no old guard
list is rewritten. Type references verified against pinned MCP SDK v1.8.0. No new
business component or wiring point. Preflight confidence **95% for this diagnostic
scope**, below90% for the root cause; no claim that diagnostics fix the failure.
RED is the retained IT-07 failure, GREEN awaits accepted-status/timing evidence
from complete Make, and REFACTOR is **N/A** (assertions/observations only; production
logic unchanged). Estimate compilation/lint 1–2m, canonical batch 8–10m. If the
diagnostics pass once, report that result without treating it as a repair or
retroactively assigning a cause to IT-07.

### 16.9 EM partial-grace timing diagnosis — user approved 2026-10-12

**RED:** the retained complete-unit failure UT-EM-254-006a (39.254s scenario,
actual zero requeue versus required 5s). Source fixture stamps creation at now−5s
before fake API/reconciler setup; the production partial-scope branch checks real
30s age after hash/health. Existing `Clock` is validity-only. All these paths match
origin/main, and the raw archive lacks setup-versus-reconciliation stage times.

Approved test-only diagnostics in that scenario: record scheme/fake-client/
reconciler setup time, the persisted EA creation age immediately before dispatch,
real reconciliation elapsed time and returned/fetched phase/assessment reason.
Retain the original timestamp, **30s grace / exact 5s requeue**, complete hash/
health/DS logic and the after-grace companion. Compile/build/new-diff lint, then
complete canonical `make test`, no process/focus/skip/deadline overrides. No new
production clock seam or fixture-time repair before evidence/user approval.
Diagnostic-scope confidence95%; cause confidence below90%. A pass without the
failure does not establish a repair. The user explicitly approves the diagnostic
edit and complete unit rerun, not a clock or timestamp repair.

**Applied:** only UT-EM-254-006a changes. The explicit pre-dispatch/readback fake API
reads also have measured durations, so their observation overhead is visible, not
passed off as part of production reconciliation. All original timestamps and
assertions remain; UT-EM-254-006b is byte-for-byte unchanged. Original source/current
guard snapshot in `issue2496-em-approved-stage-diagnosis-wi7t_fbe`; no original
manifest is rewritten. No production component/dependency/wiring change.
REFACTOR **N/A**, only observations added to the existing scenario.

### 16.10 Measured EM / DLQ fixture repairs — user approved 2026-10-12

**RED evidence:** `issue2496-em-original-window-stage-diagnosis-x78cymi4/independent-proof-v2.json`.
All 3,204 Go hashes/twelve failed profiles and immutable archive are verified;
failed profiles stay excluded. No active test/build operation.

**EM recommendation (test-only, BR-EM-001 / BR-EM-004, UT-EM-254-006a):** construct
the existing fake API and real reconciler first, then create the EA fixture with
the same **now−5s** creation age and original status/25m validity/−1m check times.
Keep real scope/hash/health logic and **30s grace / exact5s requeue**, measured
pre-dispatch age and after-grace companion unchanged. Verify stored preconditions
and assessed components/nonterminal outcome in addition to the original assertion.
Do not mutate a stored creation timestamp, skip expensive setup, or introduce a
production clock. The measured construction was38.878s; Reconcile itself80.68ms.
Alternative: extend the validity-only clock to partial-grace production logic;
this is a broader production change, not recommended for this proved fixture cause.

**DLQ recommendation (test-only, BR-AUDIT-001 / DD-008, FedRAMP SI-10,
UT-DS-1048-QEM2-002):** separate permanent input validation from the unrelated
wall-clock shutdown test. Use a cancellable context **without this scenario's5s
deadline** for the single oversized entry's bounded two-pass drain. Require the
specific two-pass permanent-failure error, `TimedOut=false`, zero processed/written
events, and the same oversized entry retained in Redis. Keep payload/max size,
real DLQ/parser/miniredis/client and original error/zero/retention assertions.
Keep existing deadline and cancellation companions **byte-identical**, including
their timeout/nil-error contracts; production shutdown budget and all canonical
Make/Ginkgo deadlines/processes remain unchanged. This is an explicit change to
this test's context, not an approved global timeout increase or production repair.
Alternative: retain its5s context and add bounded drain/context/statistics probes
first; that preserves timing but still races two different contracts under load.
Changing production cancellation/error policy is out of this test-only proposal.
Logs prove expiry, not why internal processing consumed the budget, nor that the
post-failure stream-retention assertion executed.

**TDD / readiness:** retained failures are RED; GREEN is only these approved test
fixtures, then complete canonical `make test` (~5m) after build/compile/new-diff
lint/diff and source-guard checks. REFACTOR N/A if fixture reorder/context separation
leaves no duplicate logic. No new component/Wiring Manifest row/audit event/control
claim. Success requires the full114-suite/11,524-spec population (two unchanged
kubeconfig skips), original business assertions and preservation, not an isolated
pass. Contingency: retain fresh failed evidence and escalate any other cause rather
than retry, widen timers or remove dependencies. Only our new edits may be undone.
Confidence95% for these test-only contracts,85% implementation overall; no release
readiness assertion. Explicit user approval is recorded below; no broader change
is authorized by these two decisions.

**Approval / implementation:** after asking about the38s setup cost and dismissing
the first questions (not approval), the user requests the questions again and
explicitly chooses **Readiness-first fixture** and **Separate contracts**. Applied
only EM006a and DS1048QEM2-002. Snapshot `issue2496-approved-em-dlq-repairs-f0f4nge2`
preserves both originals, all3,206 other current Go/config hashes and three original
guard manifests. EM creates, rather than mutates, its now−5s fixture after setup;
pre-dispatch age must be5–<30s and status/components unchanged initially. Actual
grace path returns before persisting newly computed component flags, so real hash/
health activity is checked through existing per-test metrics and the stored EA
must remain Assessing/uncompleted. No production status behavior changed. Existing
Prometheus testutil is already used in EM tests, not a new dependency. DS checks
the specific two-pass error, no timeout or writes, and identical retained Redis ID/
payload. Other DS cases and EM006b must be byte-identical; build/compile/lint/guards
and complete114-suite unit gate pending. Underlying setup cost remains a hypothesis.

**Setup-cost clarification (user question, no approval):**38.878s measures the
whole existing `makeReconciler`, not fake-client Build separately. Default
controller-runtime v0.24.1 Build initializes a client-go managed-fields converter;
its process-local lazy parser validates/parses430,734 bytes /959 Kubernetes schema
types. Captured Make uses12 concurrent services ×12 workers (up to144 configured
workers on12 CPUs) with race instrumentation. Cold schema work / contention is a
strong source-backed hypothesis, **not a measured bottleneck**. Metrics registration
and the remaining constructor were not individually timed. No external Kubernetes,
DS or LLM call occurs in this constructor path, and Go compilation is outside the
measured helper. Next finer timings/profile require a diagnostic decision; no
fixture/context edit or new test run followed the initial dismissed questions;
subsequent re-asked approvals and implementation are separately recorded above.

**Own assertion correction (within approved component-observation scope):** the
first full repaired gate passes original EM5s/nonterminal assertions and all new
DLQ two-pass/no-loss assertions, but `CollectAndCount` on a curried vector collects
both component series. Preserve that RED archive and correct only observation:
read each component's three existing outcome counter values, sum them and require
exactlyone recorded assessment. Do not change the production metric, remove the
real component check or claim series cardinality proves activity. Fresh compile/
build/lint/source guards and complete unit Make still required before GREEN.

### 16.11 AF local failure containment — user approved 2026-10-12

This is a failure record, **not a retry authorization**. Preserve the failed
archive/profile and every remaining resource; no recreation of deleted state or
invented pod/OOM attribution.

- **Known runner defects:** both logical lane names set equal causes local
  must-gather to request Fleet namespaces. Keep logical names/kubeconfigs distinct
  even in one-cluster lanes. AF teardown ignores KEEP_CLUSTER/PRESERVE_E2E_CLUSTER;
  DeleteCluster preserves failure only for nonblank IMAGE_REGISTRY. Our deliberately
  blank local-artifact setting selected deletion. Use existing
  **AF_E2E_SKIP_TEARDOWN=true** retention flag (not a test focus/skip filter), prove
  no destructive branch runs in sandbox, and verify fresh workloads after Make.
  Assertions, normal lane labels, process counts, deadlines and images unchanged.
- **Unknown provider/behavior:** multiple Mock LLM EOFs, A2A decision/reasoning/
  metric/approval/severity and session_active cases. Need retained logs; no
  production repair, capacity adjustment, deadline increase or test removal is
  authorized by fixing the runner.
- **Bounded recovery request:** copy only this run's diagnostics from
  `/tmp/kubernaut-must-gather/apifrontend/issue2496-af-local-clean-01` and
  `/tmp/apifrontend-e2e-logs-20261012-150813` into a uniquely owned directory under
  `/root/kubernaut-2496-test/.2496-verification-artifacts/`. Leave originals
  untouched; source/build/test work stays in the authorized checkout. Raw
  `.pre-sanitize` backups can contain credentials: keep private, never print/stage/
  publish raw secrets or weaken CA trust. Ask permission for ONLY these named
  out-of-checkout artifact reads, not unrelated paths.
- **Confidence:**95% for the two source-proved runner causes, below90% for remaining
  EOF/behavior. Stop before guessing a fix; no further gate active.

Successful-only merge uses28 attributable profiles (latestunit, IT08, AFIT05,
fullpipeline05), same1169/1199changedblocks. AFIT05's122covered production files
and12images'2,081compiled/embedded memberships match current source. Actual FP
archiveSHA `d4199307a8d06dfebe097324cfa3166d36fd04153a65c8eec3c913a50ef63446`
includes leadingd. WiderDS3912/5067=77.21% is retained, distinct from37/37changedDS
statements. No manufactured100%, whole-service or final wiring/control pass claim.

**Execution / recovery:** user selects Recover named bundles and Correct runner
guards. DNS failed twice without remote mutation; explicit VPN reconnect restores
SSH. Copies preserve186originalfiles/metadata, private archiveSHA
`667a4222d252b25931681f7f692cd8da604baf3090f7e173405c533d77f99ff6`, localartifact
`issue2496-af-local01-approved-diagnostic-recovery-be6cc7lv`, remotefolder
`af-local01-diagnostic-recovery-01`. Raw backups remain private, originals intact;
no deleted cluster recreation. Do not print credentials/request bodies.

**Runner-only RED/GREEN:** `issue2496-approved-af-runner-containment-1g_ewz5k`
preserves originalrunner and tests generatedenvironment wiring for bothlanes.
Copies actualAF namespace helper and actualteardown guard into a diagnosticGo
program; only destructive externalDelete dependency is replaced by a trap. Positive
control reaches thattrap, oldsettings fail, correctedflag/distinctroles pass.
Canonicalcommands/labels unchanged, no focus/process/deadline override. Add actual
freshclusteravailability/oldercontainerID checks. All3,204repositoryGo files,
Make/dependencies/runtimeisolation+otherrunners unchanged. This is NOT canonical
E2E or live preservation proof; nextfreshfullgate must independently prove retention.

**Corrected skipped population:** start213/224selected,206executed. Eleven default
lane-filtered specs and seven ordered-container followups skipped by Ginkgo after
failure. No userfilter/removal; neither those seven journeys nor failedprofile count
as success. Originalrawarchive/proof retained, `independent-proof-v2.json` corrects
our initial18lane-filtered shorthand explicitly.

**Provider diagnosis:** snapshots show AF/Mock Ready/Running/zero restarts/no recorded
OOM. Stream→EOF60.689–95.025s is not handlerprocessing duration; sourceMockwrite
budget60s is an investigative lead, not proved timer/CPUcause. Savedbundle has no
Mocklatency/throttling histogram; no artificialdelay logged. Additional controlled
diagnostic experiment, not capacity/deadline repair or canonical rerun, needs a
bounded plan. No production change/rebuild/newdeployedinstrumentation authorized.

### 16.12 Controlled unchanged-provider diagnosis — user approved 2026-10-12

Use existing image `localhost/mock-llm:issue2496-fixture-20261011-amd64`, immutable
ID87e235c5685db23c3c085aecfdc5a092ce8e9554f9df24efe42ebb2964d61a0e. Fresh
ownedcontainer/loopback8257, original200mCPU/256Mi limit and source60swritebudget;
no rebuild/AFcluster recreation/newinstrumentation. Compare1and40concurrent
HTTPGemini calls; existingmetrics/cgroupCPU counters read-only, no originalrequest
bodies available. Currentstaticprompt/observedtoolnames reproduce protocol shape,
**not** originalrichschemas/config/SDK/conversations. A negative result does not
explain originalEOFs or authorize capacity/timer/production repair.

Bounded per-request diagnostic100s/timebox~5m; these are experiment controls,
not changes to canonicalMake/Ginkgo40processes or productiontimeouts. Preserve
container/artifacts/allolderresources, originalCAs/counters/mountinode/ledger,
no imagepull/prune/cleanup. Recordsource+image+payload hashes and actualrequest
status/latency/size/hash, never rawcredentials/prompts in publicoutput. Await
automaticcompletion, no polling/sync/build/otherremoteexperiment whileactive.

**Outcome:**1/1+40/40validresponses, noEOF; allseven preservationguards pass. At200m,
the40-clientbatch spans44.792s with36.309–44.607s individualrequests and447/448
throttledperiods. Source-savedmetrics/result verified in
`issue2496-af-provider-probe01-verified-n43p72ad`; resultSHA
`1effd922e738526224a086bbcc38f90cf4483c418126647f34d4ea958fd6b765`.
Owncontainer retained, noimage/cluster/sourcechange. This negative reproduction
does **not** license a higherCPUlimit or timer increase and doesnot fixAF. Need
morefaithful fullAFtool-schema/SDK/config shape or passiveexistingmetric evidence
before a newcause/repair claim; no activegate or furtherexperiment atcheckpoint.

**Real request follow-up completed:** approvedrealAFroot/tool/instruction/model/
ADKfirstcall capture26schemas/58,061bytes, privatebodySHA
`07d3738a7db37258c96591b77925b90df96d9a7529f676adac253958fb54c433`, no tooloperation.
Probe02 unchangedimage/quota/budget:41validresponses/noEOF; allsevenpreservation
guards pass. ResultSHA `2f4fe6b08c975c657ffe0dedb5c7fe9dbdae1d4e2d8af83b803817d839ab2eb3`
and fourmetricshashes verified in `issue2496-af-provider-probe02-verified-admwukml`.
40-clientrequest34.672–48.423s/484of486throttledperiods. No originalruntimeconfig,
conversation, identity or mixedKA workload replay; providernetwork comparison still
controlledHTTPclient, not originalAFpod. Negative result doesnot license a fix.

### 16.13 Full AF workload diagnosis — user approved 2026-10-12

Two isolated unchangedprovider experiments have reached their limits. Savedlogs
show40peakAFstreams and post-detectionlogs180Gemini+31OpenAI; those arenot precise
provider-inflight counts or handlerduration. A source/privateartifact analysis alone
cannot establish the missingmixed-workload timing/throttling/response-error context.

Propose ONEfresh complete normalAF-localMake execution with correctedretention/
distinctlogicalcontexts, while reading only **existing** Mockmetrics, cgroupCPU
stats and savedlogs through ownedcluster/client paths. No injectedcode/logging/
coverage instrumentation, quota/memory/timeout/process/filter/assertion change or
imagebuild. This is an explicit **diagnostic** canonical execution, not claimedfix,
and needs userpermission to relax the previous no-rerun-before-cause condition.
Preserve all oldresources and newcluster afterfailure/success; verify freshworkload
inventory and allprovenance. No blindretry, no pass-as-rootcause inference. Ifnot
approved, remainread-only/blocked; do not alter originalconstraints silently.

### 16.14 Scoped CI gate repair — user approved 2026-10-13

The user authorized early draft publication for CI triage after the single AF
diagnostic did not establish a cause, then approved fixing the findings and
rewriting this feature branch to remove the synthetic secret-scan literals from
reachable history. PR #2503 remains draft; publication is not merge approval.

- **RED:** preserve run `38323849546` at `a64e3d419` as the failing secret/lint/
  CodeQL/Helm evidence; reproduce the chart attribution gap and four flagged
  allocation sites with `UT-PLATFORM-2496-CI-001` and the `ALLOC` source guards.
- **GREEN:** recognize KA's real render assertions under its schema service key;
  use append growth instead of unchecked capacity sums; retain privacy inputs
  and negative assertions with low-entropy synthetic sentinels; reuse the test
  Deployment-kind constant. No new production wiring or policy is introduced.
- **REFACTOR:** clarify canonical chart-service attribution and eliminate the
  duplicate fixture kind; preserve history ordering, packet limits and dispatch.
- **Verification:** affected behavior suites, build/compile, CI-scoped lint,
  `make check-helm-coverage`, unchanged scanner configuration and rewritten
  reachable-history scan; then one CI cycle. Gateway's module-download HTTP/2
  failure warrants no production code or capacity change.

The source guards are static security regressions, not added business scenarios
or manufactured integer-overflow inputs. Existing scenario/control targets and
all remaining scoped runtime gates remain intact. GitHub's historical run/alert
records are not erased by rewriting the branch. Private evidence stays excluded.

**Local repair evidence:** the new chart-attribution spec fails as intended before
the fix, as do all four allocation guards. After repair, all ten affected test
packages pass; `make check-helm-coverage`, `go build ./...`, compile-only validation
and the exact CI lint scope pass. Gitleaks 8.30.1 reports no findings in the staged
fixture replacements with the scanner configuration unchanged. A same-config
comparison of feature history reports **9 findings before rewrite and 0 after**;
the original feature/merge commits are no longer ancestors. The validated code
tree and all fifteen private evidence files remain unchanged by the rewrite.
A full `HEAD` ancestry scan separately reports 21 findings, all inherited from
main commit `80c8a9ad5ce09669b60e0078e80dc7ea63fcfc4e`; no main-history rewrite or
scanner exclusion was applied. CI CodeQL/runtime and the normal secret-scan job
remain pending. These local checks do not establish the earlier AF EOF cause.

### 16.15 Scoped audit repair and scheduling — user approved 2026-10-13

The read-only review at `936627078` identified five in-scope source defects.
Independent source/blame preflight supports a 96–99% approach confidence; the
new `AUDIT` specs must reproduce each defect before production changes.

| Finding | RED / regression proof | Minimal GREEN boundary |
|---|---|---|
| Canonical cluster-scoped history | UT-KA-2496-AUDIT-001; extend IT-KA-2496-002 | Preserve the RO/DS leading-slash target composite for empty namespace |
| Persisted reasoning credentials | UT-KA-2496-AUDIT-002; accepted/rejected AgentSession and AA read-back | Apply existing credential sanitizer to new narratives, never execution inputs |
| Fleet/catalog name collision | UT-KA-2498-AUDIT-003; real catalog dispatch IT | Resolve overlays only during trusted RCA; discovery uses the local catalog |
| Post-sanitization explanation overflow | UT-KA-2496-AUDIT-004; IT-KA-2496-003 | Validate sanitized bounds before acceptance; use existing correction/review |
| Interactive RCA confidence provenance | UT-KA-2496-AUDIT-005; MCP completion/live projection | Carry the trusted Phase 1 score independently of selected confidence |

The user separately chose manual review on total Phase 2 history acquisition
failure. Propagate a safe acquisition-status flag through existing enrichment
and freeze wiring; legitimate empty history, unknown individual assessments and
optional Phase 1 context calls remain valid. Prove review occurs before a
selection-provider request. No baseline history-key redesign is authorized.

The user also approved removing blanket `Serial` from the seven independent
fullpipeline history journeys and scoping A2A request/message IDs to their RR.
UT-PLATFORM-2496-PARALLEL-001/002 fail before repair; all 315 infrastructure specs,
fullpipeline compile and scoped lint pass afterward. Within-spec sequencing,
real cooldown/approval/isolation assertions, default processes, quotas and
deadlines remain unchanged. Actual parallel runtime proof remains pending.

Repairs enhance existing components and wiring; no new service/type architecture
or semantic reviewer is introduced. REFACTOR centralizes safe narrative
projection and phase-aware resolution without weakening existing contracts.
Validate affected UT/production dispatch IT and source-attributed gates, then
batch one commit/push. Static scheduling guards are not extra business scenarios.
