# Implementation Plan: Issue #2496 — Failed Workflow Identity and Repeat-Selection Reasoning

**Status**: Combined #2496/#2498 draft PR #2503 published for authorized CI triage; scoped gate repairs in progress<br>
**Issue**: [#2496](https://github.com/jordigilh/kubernaut/issues/2496)<br>
**Incorporated issue**: [#2498](https://github.com/jordigilh/kubernaut/issues/2498) — strict phase-scoped tool availability and optional RCA context tools<br>
**Created**: 2026-10-09<br>
**Inspection baseline**: `8a907bb57d3530da32de9b3b8250b53587ed6f41`, branch `issue-2496-history-aware-rca`<br>
**Remote main observed**: `8a907bb57d3530da32de9b3b8250b53587ed6f41` — #2490 history-chain fix merged<br>
**Test plan**: [TP-2496-v1.6](TEST_PLAN.md)<br>
**Confidence**: **95% for the pre-implementation approach after the final bounded preflight; not implementation/GA confidence**
**Isolated spike**: [SPIKE_REPORT.md](SPIKE_REPORT.md)

## Current implementation status (supersedes historical approval notes below)

The user approved the combined architecture, BR, test plan, implementation plan and
handoff. The latest explicitly reconfirmed repeat contract is **required at
selection**: a same catalog-authoritative workflow ID with a completed valid
target-scoped assessment requires all three bounded justification fields before
acceptance, using existing bounded correction. Earlier non-blocking-context and
approval-pending notes are historical, not current authorization or requirements.

- Trusted phase dispatch, optional RCA history/context tools, current-version
  RO/DS history metadata, request-local repeat acceptance/correction, one sanitized
  immutable evidence packet, AgentSession/AA mapping and safe live/durable context
  projections are implemented. Catalog membership, confidence controls and safety
  dispatch remain unchanged; rejected proposals are non-executable assessments.
- The v1.6 live `present_decision` contract is server-owned for cluster attribution:
  the AF artifact projects only trusted `RRContext`/RemediationRequest `cluster_id`
  and omits it when unavailable; a model-supplied value is not authoritative. This
  is separate from validated cluster inputs accepted by RR-creating tools.
- Complete local `make test` passes **11,524 specs across 114 suites**. Current
  production source remains unchanged since that pass. Complete AF integration
  passes **217 specs across three suites**; KA E2E passes **110/110**; Mock unit Make
  passes **352 specs**; Helm Make passes **91 suites / 721 tests**. All affected
  complete canonical IT targets and deployed AF/fleet gates remain outstanding.
- Fresh strict-TLS DS canonical capacity gate passes **143/143**, zero failed,
  pending or skipped. The user-approved inline E2E PostgreSQL/DS CPU limits are
  4/2; requests, memory, process counts, deadlines and production/chart policy are
  unchanged. This is not a flaky-test or CI-executed claim.
- Fresh fullpipeline-04 completes **44/45** with one test-only RR phase mismatch.
  BR-ORCH-036 routes human review by selected-workflow presence, independently of
  AA phase: no admitted selection completes RR with ManualReviewRequired; an
  admitted low-confidence selection fails RR. The corrected assertion also proves
  the manual-review flag and no WE after terminal completion. Complete fresh
  fullpipeline-05 passes **45/45**, zero failed/pending/skipped; eleven deployed
  profiles and all preservation checks are independently verified.
- Source/image attribution covers twelve reused services. Preliminary merged
  unit/AF IT coverage is **1,169/1,199 changed statements (97.50%)**; defensible
  defensive exclusions are documented in the test plan. Meaningful business and
  control proof, not artificial literal 100%, is required by the user's subsequent
  clarification. Final merged E2E coverage, fleet registered-origin isolation,
  real remote Job/patch proof and the final Wiring Manifest assessment remain open.
- `go build ./...`, compile-only validation and repository-wide new-diff lint
  pass. Untouched full-lint findings and repository heuristic-scan failures remain
  recorded; no unrelated repair or scanner suppression is authorized.
- Continue using repository **Make targets**, with no process overrides or focused
  diagnostics substituted for canonical gates. Source sync is non-deleting and
  idle-only in `/root/kubernaut-2496-test`; preserve older clusters/data/images,
  counters, mount inode, CAs and provenance. The production evidence ledger stays
  local-only, excluded, unstaged, uncommitted and unsynchronized.
- The user authorizes logical commits, rebase onto `origin/main`, push and one PR
  closing #2496/#2498 **only after verification is complete**, followed by a fresh
  production-readiness subagent audit and repairs. No premature publication or
  manual issue closure. Both issues are session-linked; link the PR immediately.

**Current publication exception (2026-10-13):** after the single fresh AF-local
diagnostic remained non-actionable, the user explicitly authorized publishing
[draft PR #2503](https://github.com/jordigilh/kubernaut/pull/2503) for CI triage.
This supersedes the earlier pre-publication gate only, not merge readiness or
the scoped scenario, critical-control and wiring requirements. The user then
approved the localized CI repairs and removal of synthetic secret-scan literals
from this feature branch's reachable history. See [TEST_PLAN.md §16.14](TEST_PLAN.md#1614-scoped-ci-gate-repair--user-approved-2026-10-13).
Previously successful evidence remains source-attributed to its original
snapshots; later source repairs require affected validation, not wholesale gate
repetition or release certification.

Latest detailed continuation: [IMPLEMENTATION_SESSION_HANDOFF.md](IMPLEMENTATION_SESSION_HANDOFF.md).
Implementation confidence remains **85%**, not a release-readiness assertion.

The original spike/preflight ledger below is retained for provenance; its statements
about production implementation not being approved describe that earlier stage.

**Current compatibility decision (user-directed, 2026-10-09)**: For this enhancement, v1.6 need not support v1.5 deployments or their historical metadata contract. Use one v1.6 producer → history contract. **Legacy AA recovery, backfill, version-detection fallbacks and mixed v1.5/v1.6 deployment support are removed from the proposed scope.** The user reports no known production deployments; that is their understanding, not an independently verified inventory. Historical notes below record earlier proposals, not continuing compatibility requirements. Missing, malformed or pending current-version data must still be handled safely and observably without guessed confidence. This scope decision does not approve production implementation or destructive data/schema operations.

**Old-record observability decision (user-directed, 2026-10-09)**: Old records remain unchanged and are not backfilled/enriched with the new selection metadata. Keep their existing recorded information in history and mark unavailable fields unknown. When encountered during a history read, note the skipped metadata enrichment in structured operational logs; do not silently omit that limitation or modify the immutable audit rows. This is a logging requirement, not permission to start implementation or restore compatibility queries.

**Scope clarification (2026-10-09)**: The user treats the issue as improvement suggestions informed by the rehearsal, not a mandate to add another semantic decision-maker. Use richer history, explicit repeat justification and the existing confidence/review controls. No new LLM judge, automatic confidence penalty, or failed-workflow exclusion is proposed. Full implementation still requires approval of this revised plan.

**Expanded spike authorized (2026-10-09)**: The user now wants recurrence-aware upstream RCA, a structured read-only evidence handoff and historical/current state comparison included in this enhancement and the same eventual PR. Workflow selection/discovery must not gain Kubernetes MCP access or rewrite the RCA. Issue #2498 is incorporated into that same PR: RCA context/history tools are optional in Phase 1, while Phase 3 exposes only workflow-discovery and required result-submission tooling. GPT-5.6 Luna is extending the isolated spike through real production dispatch and persistence, with supervised RED/GREEN checkpoints. The narrower plan below is the prior draft baseline; it must be revised using the expanded spike findings before implementation approval. The earlier test-local prototype is not evidence for the expanded journey.

**Separate future enhancement**: [#2497 — Evaluate evidence-grounded review of repeat-workflow justifications](https://github.com/jordigilh/kubernaut/issues/2497) was filed at the user's request. A runtime reviewing agent is excluded from #2496's initial implementation and from this spike; #2497 has no committed release milestone or gate policy.

**Console delivery decision (Engram-validated, 2026-10-09)**: The current live RCA/selection result uses the existing KA MCP event stream or interactive decision function arguments, which AF projects through its existing A2A status/artifact path. Audit/DS data is consumed only for explicit past-remediation/history retrieval, including the history context used by history-aware RCA; it is not queried to present the current result. No mandatory AgentSession/AA status projection is added solely for live repeat context. The bounded KA complete payload and AF investigation artifact need safe workflow/repeat fields added; this is a live typed-projection change, not an audit-read integration.

**Live artifact preflight result (2026-10-09)**: Engram exact symbol/reference validation and focused tests closed the transport uncertainty. The autonomous path is `InvestigationResult` → `session.MarshalRCASubset` → AF `captureCompleteEventRCA`/`InvestigateRCA` → `emitInvestigationSummaryArtifact`; the interactive path is `PresentDecisionArgs` → `emitDecisionEvent`. Existing `investigation_summary.v1.schema.json` permits additive fields, and `EventBridge.EmitArtifact` already provides JSON/gob-safe normalization and text fallback. The four-package focused test command passed. The new RED slice must assert the safe projection at both artifact boundaries; no DS/audit read is part of either path.

**Phase-isolation/#2498 decision (2026-10-09)**: Forced-call testing and source inspection found that advertised phase tool sets do not by themselves prevent dispatch of a registered off-phase tool. This is a dispatcher finding, not proof of live-cluster access. The user chose to incorporate #2498: resource-context/history tools remain optional Phase 1/RCA tools, and Phase 3 exposes only workflow-discovery plus required result-submission tooling. Add a minimal server-bound phase allow-list before dispatch, preserving permitted RCA/fleet-overlay and catalog/submission behavior. Include the prompt/documentation correction from “must call” to “may call when relevant.” Production approval remains pending. Two initial expanded RED checkpoints were rejected for incomplete behavioral/persistence assertions; neither is accepted as readiness proof. A fresh Luna worker is establishing valid RED coverage and real isolated runtime prerequisites before GREEN permission.

**RED review ledger**: [UPSTREAM_RED_REVIEW.md](UPSTREAM_RED_REVIEW.md) records the completed-checkpoint findings and the exact proof required before prototype GREEN. Upstream handoff and persistence are not approved merely because a provider call or database process ran.

**Expanded RED review outcome (2026-10-09)**: The fresh Luna checkpoint was independently reviewed and still **requires changes**. Exact mapping-value checks and runtime prerequisites improved; repository-supported envtest admission passed independently and production migrations ran in the worker harness. These are not the new persistence contract: status is directly authored in existing rationale fields, DS stops before HTTP ingestion/query, and upstream tests do not capture/freeze actual RCA observations. The repeat test also incorrectly requires rationale for a different workflow. No prototype GREEN or production implementation is approved; see the review ledger for paths, commands and required repairs.

**User-steered next slice (2026-10-09)**: After reviewing the blockers, the user explicitly authorized continuing with the foundational **stored scoped history → DS DTO → optional Phase 1 context-tool availability → authoritative Phase 2 consolidation** slice. GPT-5.6 Luna is preparing a separate history-slice RED plan and runtime tests in the existing detached worktree, with independent review before prototype source changes. Prior identity/name and selected-workflow confidence must come from actual stored matching attempts; existing effectiveness/resolution, assessment validity and chronology are preserved, and unavailable legacy values remain unknown. The user did not approve full production implementation. Repeat correction, the strict #2498 phase allow-list, structured observation freezing and AA justification persistence are excluded from this narrow history slice and remain outstanding for the combined enhancement. The original expanded-spike deadline remains 2026-10-09 14:06:47 UTC; it is not reset by decomposition.

**History-slice evidence ledger**: [HISTORY_SLICE_REPORT.md](HISTORY_SLICE_REPORT.md) separates real retrieval/provider wiring, provisional future RO metadata, missing legacy-recovery proof and the independent RED refinements. The original full-enhancement RED verdict is not overridden by progress on this narrower slice.

**Representative history RED accepted**: After correcting fixtures and assertions, the parent independently reran both history wiring specs (2 selected, 2 intended missing-contract failures, with baseline schema/auth-middleware/scoping/assessment checks passing). A minimal detached-worktree prototype is authorized for optional history metadata and the existing context/enrichment delivery path, with test-first missing-versus-zero checks and mandatory independent GREEN review. The future RO emitter and AA legacy confidence recovery are not proved by fixture-seeded metadata. Main production implementation, full-enhancement GREEN, commit and push remain unapproved.

**History read/delivery spike verified**: The detached prototype preserves stored workflow ID/name/selected confidence and sends them into the first autonomous non-fleet RCA request through its prototype prefetch path. Parent verification passed six focused specs (including three actual Tier 1/Tier 2 absent/zero projection UTs), repository build, scoped lint and generation reproducibility. This is plumbing evidence only; the current contract exposes history tools optionally in Phase 1 and makes Phase 2 automatic enrichment/consolidation authoritative for discovery. **This does not close normal metadata population**: v1.6 RO emission is still absent. Legacy AA recovery was not implemented and is now explicitly out of scope. Next recommended foundational slice is the current-version producer boundary, not a claim that the full history/evidence/selection feature is ready. Details, evidence and production limitations: [HISTORY_SLICE_REPORT.md](HISTORY_SLICE_REPORT.md).

## 1. Outcome and scope

Give the workflow-selection agent trustworthy, chronological evidence of which workflows were previously selected, with their selection confidence and subsequent effectiveness. A catalog candidate remains eligible after failure. A repeat needs an explicit explanation of changed evidence, why the prior failure no longer applies, and the mechanism for a different outcome. The current result is delivered to the Console through KA MCP → AF/A2A; audit/DS reads remain limited to past-remediation/history retrieval. An unsuitable catalog must still lead to the existing no-workflow/manual-review path.

This is a cross-component contract enhancement, not a prompt-only patch. It affects RO/DS history metadata, KA enrichment/prompt/validation/result mappings, AIAnalysis status, and typed audit payloads. The selecting agent judges substantive suitability. Trusted code requires an explicit, structurally complete justification for every same-`workflow_id` repeat with a completed valid prior assessment; existing confidence/review gates determine whether automation proceeds.

### In scope

- Prior workflow identity/name and **workflow-selection** confidence in both history tiers.
- Correct attempt chronology and stable numbering; honest labeling of the existing history timestamp.
- Failed-strategy guidance after **one** proven ineffective attempt; no retry-count threshold.
- Conditional repeat-justification validation on every autonomous selection/retry path.
- End-to-end persistence of the justification, including below-confidence/manual-review context, and correlated validation/outcome evidence.
- Existing no-workflow, catalog-membership, target, parameter, approval, and RO safety behavior.
- Strict phase-scoped tool availability: optional RCA context/history tools in Phase 1; workflow-discovery and required result-submission tools only in Phase 3; no Kubernetes/context-history access during discovery.
- Formal UT/IT/E2E coverage of business outcomes and applicable controls.
- One v1.6 selection-metadata producer/consumer contract, proved using records produced by the current release rather than relying on old deployments.

### Out of scope

- Removing, disabling, re-ranking, or scoring down failed workflows in the catalog.
- Making the platform choose an alternative workflow for the agent.
- Exposing previous parameters, execution bundles, or complete audit payloads to the model.
- Changing EM scoring, RO ineffective-chain thresholds, workflow Job retry policy, or approval policy.
- New database tables, broad audit-framework rewrites, new LLM providers, or interactive operator-selection policy.
- Promising that a justified *selection* bypasses downstream execution safety gates.
- A semantic retry judge, keyword-based rationale scoring, or an automatic prior-confidence/effectiveness penalty.
- Repairing actual workflow-completion timestamp reconstruction or legacy cross-execution-cluster WFE joins; these are independent fidelity improvements, not prerequisites for identity/confidence enrichment.
- Supporting v1.5 deployment/history formats, recovering old AA metadata, backfilling prior records, version-dependent fallback branches or mixed-version rollout compatibility. This exclusion does not authorize removing audit history or existing migrations.

## 2. Preflight evidence

### 2.1 Workspace and conflicting work

- The dedicated branch `issue-2496-history-aware-rca` is active with planning/architecture/BR documents only; no production feature code or RED tests have been added.
- The branch is based on merged remote `main` at `8a907bb57d3530da32de9b3b8250b53587ed6f41`; revalidate the baseline again immediately before source implementation.
- A separate #2490 history worktree exists. Its recursive-history changes are already present in the inspected checkout and merged into remote `main`.
- Open non-dependency PR inventory contained fleet Helm/egress work and a pluggable-harness proposal, not an active #2496 implementation. This is a point-in-time conflict check, not a guarantee against later work.
- Engram recall, indexed discovery, symbol retrieval, and references were used. Indexed results reported possible staleness; relevant evidence was checked against live symbol bodies/source. One indexed timeout recovered on retry; a Makefile text-search schema error was handled with the dedicated file search tool. Engram was not disabled. **Stop/escalate if Engram becomes disabled during implementation.**

### 2.2 Production flow and gaps

| Boundary | Current evidence | Gap / consequence |
|---|---|---|
| RO → stored history skeleton | `internal/controller/remediationorchestrator/audit_events.go:103`, `emitWorkflowCreatedAudit`; `pkg/remediationorchestrator/audit/manager.go`, `RemediationWorkflowCreatedData` / `BuildRemediationWorkflowCreatedEvent` | `remediation.workflow_created` already has `workflow_id`, action/version, target and correlation. It does not carry selection confidence. |
| AA → selection audit | `pkg/aianalysis/audit/audit.go:165`, `buildAnalysisCompletePayload` | `aianalysis.analysis.completed.confidence` is explicitly sourced from `SelectedWorkflow.Confidence`, **not RCA confidence**. The v1.6 RO producer must copy authoritative selection metadata directly; no historical AA recovery join is required. The existing audit type is float32; do not demand bit-identical float64 precision. |
| WFE → execution evidence | `pkg/workflowexecution/audit/manager.go`, `buildWorkflowExecutionAuditPayload`, `setWorkflowIdentifiers`, `recordAuditEvent` | Existing `workflowexecution.workflow.completed` contains workflow ID/name, actual `completed_at`, target and parent RR correlation. This is completion evidence; a workflow-created event is not. |
| DS repository → history API | `pkg/datastorage/repository/remediation_history_repository.go`; `pkg/datastorage/server/remediation_history_logic.go:329,459` | `buildTier1Entry` / `buildTier2Summary` discard the stored workflow ID. Both initialize `CompletedAt` with the **RO workflow-created timestamp**. Only RO/EM are currently correlated. |
| API → KA enrichment | `internal/kubernautagent/enrichment/enricher.go`, `Tier1Entry` / `Tier2Summary`; `ds_adapter.go:52` | Neither tier has workflow identity/confidence. DS returns oldest-first, but `GetRemediationHistory` sorts both tiers newest-first. |
| Enrichment → selection prompt | `investigator/investigator_phases.go`, `enrichmentToPrompt`; `prompt/builder.go:285`; `prompt/history.go` | History reaches Phase 3, but “Workflow” currently means action type. Tier 1 is rendered before older Tier 2. There is no global attempt numbering. |
| Phase tool advertisement → execution | `investigator/types.go:DefaultPhaseToolMap`; `investigator/investigator_tools.go:toolDefinitionsForPhase`; `investigator/investigator_loop.go:processToolCalls` / `executeTool` | Phase 3 schemas omit RCA/Kubernetes/context tools, but execution does not yet validate the trusted phase before dispatch. #2498 requires a strict allow-list and tests that a forced off-phase call never reaches the registry/external tool. |
| Existing failed-history instructions | `internal/kubernautagent/prompt/history.go:151–155`, `buildRecurringRemediationWarnings` | Already emits **“MANDATORY: You MUST NOT re-select”** by action + signal, after multiple completed entries. `AllZeroEffectiveness` also treats missing score/resolution as zero/unresolved. This conflicts with justified retry and can conflate unknown evidence with proven failure. Replace the changed reasoning path with explicit known-failure evidence; do not rely on an action-family prohibition. |
| Parse / retry / validation | `parser/schema.go`; `parser/parser_llm_types.go`; `parser/validator.go`; `investigator/investigator_workflow_selection.go` | No justification field. Successful initial and recovered selections share `selfCorrectWorkflowSelection` / `Validator.SelfCorrect`; this is the existing bounded validation seam. |
| KA → AgentSession → AA | `agentsession/mapping.go:235`, `buildSelectedWorkflowMap`; `pkg/aianalysis/handlers/response_processor.go`, `storeSelectedWorkflow`, `preserveLowConfidenceWorkflow`, `preservePartialSelectedWorkflow`; `api/aianalysis/v1alpha1/aianalysis_types.go`, `SelectedWorkflow` | Extend successful **and human-review** projections; adding a parser property alone loses the field exactly where an operator needs it. |
| KA → audit storage | `investigator/investigator_audit.go`, `ResultToAuditJSON`; `audit/ds_response_mapping.go`, `investigationResultJSON` / `toSelectedWorkflowResponse` | Another typed narrowing boundary can silently discard the new field. Existing response-complete emission is best-effort; do not infer a durable audit guarantee from a successful model result. |
| No workflow | `investigator_workflow_selection.go:196–204` | `SubmitNoWorkflowResult` already returns human review with `no_matching_workflows` and no workflow ID. Preserve it even when catalog candidates exist. |
| Configured confidence/review controls | `cmd/aianalysis/main.go:297–299`; AA `WithLowConfidenceFloor`, `handleLowConfidenceFailure`, `AnalyzingHandler.WithConfidenceThreshold` | `rego.lowConfidenceFloor` gates low-confidence human review; the later `rego.confidenceThreshold` controls Rego auto-approval. Both are already production-wired. They are distinct gates, not a new repeat-specific threshold. |
| RO execution safety | `pkg/remediationorchestrator/routing/blocking.go`, `CheckIneffectiveRemediationChain` | Separate hash-chain/recurrence limits still apply. This issue must not bypass them. |

### 2.3 Business requirement mapping

| Requirement | Use in this change |
|---|---|
| BR-KA-016; DD-KA-016 v1.7 | Identity-preserving, complete, scoped remediation history and reasoning context. Extend the documented contract with #2496's acceptance criteria before RED. |
| BR-INS-001 / BR-INS-002 | Consume actual effectiveness outcomes, distinguish symptom resolution from durable remediation, and do not fabricate assessment data. |
| BR-KA-197 | Unreliable selection / unsuitable candidates reach human review, not automatic execution. |
| BR-KA-200 | Preserve no-workflow outcome routing; do not misclassify an unresolved incident as self-resolved. |
| BR-AI-088; BR-AI-003 | Preserve configured low-confidence review and later Rego approval behavior using the current selected-workflow score, not historical/RCA confidence. |
| BR-KA-191; DD-KA-001 / DD-KA-017 | Trusted selection validation, discovery membership, parameters, bounded self-correction. |
| BR-KA-017 / DD-KA-017; #2498 | Phase 3 workflow-discovery tool contract and strict least-privilege separation from RCA/Kubernetes/context tools. |
| BR-KA-211; DD-KA-005 | Sanitize new audit-derived prompt data and minimize external-provider disclosure. |
| BR-AUDIT-005; ADR-034; DD-AUDIT-003 | Reconstruct decision/history/justification by remediation correlation using typed records and complete envelope metadata. |
| BR-ORCH-042.5; BR-FLEET-004 | Regression constraints: preserve routing safety and distinguish signal-origin from execution-cluster provenance. |

BR-KA-016 is now formalized in `docs/requirements/BR-KA-016-history-aware-remediation-context.md`; the #2496/#2498 sub-contract is normative together with DD-KA-2496. Do not mint or declare an unrelated BR during implementation.

## 3. Focused spikes and results

The initial investigations below were **read-only contract/path spikes**, each bounded below the two-hour limit. At the user's subsequent request, GPT-5.6 Luna ran a separate test-local prototype in an isolated worktree. Its reviewed results and limits are in [SPIKE_REPORT.md](SPIKE_REPORT.md). No production code or infrastructure was changed, and the prototype does not prove production correction/history wiring or model reasoning quality.

| Spike question | YES / NO | Evidence / implication |
|---|---|---|
| Can previous workflow identity and selection confidence be obtained without a new table or model inference? | **YES** | RO already persists workflow ID; AA has authoritative selected-workflow confidence. Copy explicit name/selection-confidence metadata into v1.6 RO snapshots. The original legacy AA recovery option was investigated but not implemented and is no longer required by the user's deployment-support decision. |
| Does the present prompt already allow the intended retry contract? | **NO** | The generated Phase 3 history section contains the action-level `MUST NOT re-select` directive. Replace it, including its existing assertions, rather than adding contradictory guidance elsewhere. |
| Is the model-facing history chronological today? | **NO** | DS ascending order is reversed by the adapter; the template presents recent before historical entries. Build an explicitly chronological prompt view without silently changing RO's history timestamp semantics or effectiveness-trend interpretation. |
| Can one validator seam protect initial, parse-recovered, and self-corrected autonomous selections? | **YES** | `runWorkflowSelection` routes genuine selections through `selfCorrectWorkflowSelection`; `SelfCorrect` validates each candidate. Bind trusted history per investigation at that seam, not to global/shared catalog state. |
| Will adding a tool-schema field alone make the justification auditable? | **NO** | AgentSession, AA CRD, KA audit intermediate DTO and generated DS payload all explicitly project fields. Extend and test each boundary. |
| Can required/non-empty strings prove a materially changed strategy? | **NO — not the selected objective** | The user clarified that prompt-guided agent reasoning, explicit justification, and existing confidence/review gates are sufficient. Validate the explanation contract without pretending to prove its truth or adding a semantic judge. |
| Are the existing configurable confidence controls actually wired? | **YES** | `cmd/aianalysis/main.go` passes both configuration values to the production handlers. `handleLowConfidenceFailure` preserves a partial workflow, flags human review and stops automatic progression; the analyzing handler separately supplies selected confidence to Rego. |

**Executed baseline** (Go 1.26.6, all three packages PASS):

```sh
go test ./internal/kubernautagent/prompt ./internal/kubernautagent/parser ./internal/kubernautagent/enrichment -count=1 -timeout=3m -ginkgo.focus='History|history|Workflow|workflow|confidence|Confidence'
```

This confirms focused existing behavior only. No new RED tests, runtime integration tests, Kind E2E, full build, or repository-wide lint were executed. It is not proof that #2496 is implemented.

## 4. Recommended design for approval

### 4.1 History acquisition

| Option | Approach | Trade-off |
|---|---|---|
| **H-B — selected scope for the revised proposal** | Preserve RO workflow ID; copy workflow name and explicitly named selection confidence from the authoritative AA selected-workflow snapshot into v1.6 `remediation.workflow_created` audits. DS projects that one contract. | Simple, direct producer/consumer path without compatibility queries. Prove recurrence using prior attempts emitted by v1.6; pre-enhancement rehearsal records need not be enriched. Exact field/requiredness contracts still require approval. |
| H-A — retired | Add a matching AA audit recovery query for pre-enhancement records alongside the new RO fields. | Extra provenance/query/compatibility logic for deployments the user does not require supporting. Not part of this proposal. |
| H-C | KA individually fetches prior CRDs/audits for each attempt. | N+1 calls, independent retention/permissions, duplicated DS correlation. Reject for this plan. |

**H-B constraints**:

- RO anchors remain target/cluster/spec-hash/window scoped per #1802/#2490; do not widen discovery to unrelated audit records.
- Select only needed scalar/typed metadata; never materialize full prior parameters into the model-facing DTO.
- Bind emitted metadata to the actual selected workflow, RR, target and origin cluster/namespace. Never substitute top-level RCA/provider confidence or approval `confidence_str`.
- Copy metadata directly from the existing immutable AA selected-workflow snapshot in `emitWorkflowCreatedAudit`, extending authoritative typed payload schemas/mappers and generators. No AA recovery query, model inference, version fallback or WFE execution-cluster join is needed.
- Missing/malformed current-version metadata remains unknown and observable; explicit zero remains known zero. This is defensive data handling, not an old-format compatibility layer. Keep legitimately pending effectiveness assessments distinct from absent selection metadata; final schema requiredness still requires approval.
- Log historical selection-metadata gaps and that backfill/enrichment is not attempted, using the existing logger. Proposed shape: one bounded summary per affected history lookup with query/incident correlation, target/origin scope, affected record count and missing field names; bounded record identifiers may help trace the source. No raw audit payloads, previous parameters or secret values. Do not infer that a record is old solely because a field is missing: use a neutral incomplete-metadata reason unless age/version provenance is established. Empty history and a legitimately pending effectiveness assessment are not old-selection-metadata gaps; known zero confidence is not missing.
- Preserve existing history query cancellation/deadline and complete-result guards. No additional metadata query is introduced. Do not add silent global `LIMIT` truncation or remove #2490's guards.
- Preserve existing history `completedAt` and routing/tier semantics in this slice. The discovered value is RO workflow-created time, so use an honest recorded/created-time label in the prompt. Actual execution-completion reconstruction is a documented follow-up; do not imply the existing date proves completion or widen this feature to repair that contract.

### 4.2 Justification and agent-owned suitability

Proposed shared typed contract: CRD `selectedWorkflow.repeatJustification`, snake-case `selected_workflow.repeat_justification` on KA model/result/audit wires, with three explicit text parts:

```json
{
  "evidence_basis": "What current evidence supports selecting the same workflow",
  "prior_outcome_relevance": "Why the prior completed outcome remains relevant to this recurrence",
  "expected_mechanism": "How the same workflow is expected to remediate the current fault"
}
```

The object is optional globally and required **only** for a selection whose `workflow_id` matches a target-scoped prior attempt with a completed valid assessment. This includes a prior successful/remediated attempt and a prior ineffective attempt; pending, invalid, unknown and SpecDrift assessments do not trigger it. Each part, when the object is present, is non-blank and bounded (maximum 2,000 characters per part). Unknown properties/types cannot stand in for required parts. Do not use keyword blacklists or another inference call as a semantic validator. The structured shape makes the explanation aspects visible for **auditability**; it does not grant the platform authority to judge or choose the strategy. Known zero effectiveness does not identify the failure cause: the agent must acknowledge missing causal evidence rather than fabricate it.

| Option | Approach | Trade-off / acceptance |
|---|---|---|
| **J-B — recommended, consistent with the user's clarified intent** | Structured justification + trusted conditional structural validation + prompt-guided agent reasoning + existing confidence/review gates. | No added model judge, policy-driven selection or confidence penalty. This is the scope selected for the revised plan. |
| J-A — not proposed | Add a separate evidence-grounded semantic reviewer. | More latency/cost and another decision boundary; not warranted for this context enhancement and not requested by the user. |
| J-C — smaller contract alternative | Persisted free-form justification plus the same evidence-focused prompt guidance. | Can meet the auditability purpose if carried through every result/audit boundary; cannot independently enforce the presence of all three explanation aspects. This remains an alternative for approval, not equivalent to a prompt-only field that is never persisted. |

For **J-B**, the prompt says that matching the current symptom/RCA alone is not a sufficient explanation for a same-`workflow_id` repeat: the agent must state the evidence basis, explain the relevance of the prior completed outcome (including why a successful prior remediation is relevant to a recurrence), and describe the expected mechanism. Trusted code rejects missing/incomplete justification and requests correction through the existing bounded loop. A structurally complete explanation is not subjected to another semantic judge. The agent may select the same workflow, a different one, or no workflow, and recalibrates its **current** selection confidence from the richer evidence. AIAnalysis applies `rego.lowConfidenceFloor`, then normal Rego approval applies `rego.confidenceThreshold`; neither gate is changed or bypassed. Prior confidence is context, not a formula that forces score reduction or escalation.

Tests must distinguish these guarantees: prove required context/instructions, structural enforcement and confidence/outcome wiring; do **not** claim a model will always reason correctly or that CI has proven arbitrary narrative adequacy.

**Evidence-grounding spike completed (2026-10-09)**: GPT-5.6 Luna tested a small structural composition with the real catalog validator; the parent independently reran all four prototype specs and the focused suites. Existing audit/tool data is useful evidence but Phase 3 has no inspected trusted citation-resolution/old-current comparison contract. Recommend concrete observed change, an honest prior-failure assessment, consideration of contrary evidence and an observable expected result within the existing response. A reference-existence/provenance check would not verify the inference. New reference fields/ownership validation or confidence policy remain separate unapproved design options. See [SPIKE_REPORT.md](SPIKE_REPORT.md); no empirical model-quality improvement, runtime IT/E2E or release pass is claimed.

### 4.3 Required contract clarifications

1. **Identity**: `workflow_id` is mandatory and authoritative for every selected remediation workflow and is the sole identity/repeat-matching key. `workflow_name` is mandatory and catalog-authoritative but is not unique; names may be shared across versions/action types. `action_type` and `workflow_version` are optional descriptive context and never matching keys. If either mandatory field is missing, the selection is invalid and no remediation workflow is selected. There is no name, revision, or action-type fallback. Historical records missing either mandatory field remain incomplete/unknown and cannot establish a same-workflow repeat match. With distinct IDs, show the history to the agent but do not silently impose a strategy-level ban. This matching contract is approved as a user clarification.
2. **Failure evidence**: known score `0`, explicit `signal_resolved=false`, and a completed, non-SpecDrift assessment from the existing RO→WFE→EM pipeline classify a prior attempt as known ineffective. Missing values, pending assessment or invalidated SpecDrift are not proven failure. This predicate is used for failure-aware guidance, but is not the gate for repeat justification: any same-ID completed valid assessment requires the explanation. Do not depend on a synthetic `outcome=Success` in the workflow-created payload: production does not populate that field. Resolved-but-recurring remains qualitative durability evidence, not this mandatory-zero-failure classification.
3. **Live and historical delivery**: preserve the justification in success **and human-review** result/status/audit contexts, and make delivery errors observable through existing mechanisms. The current live Console projection comes from the KA MCP complete/event or interactive decision payload through AF/A2A; it does not query DS audit data. Existing response-complete and A2A emission are best-effort; this feature does not add a synchronous audit acknowledgement gate or promise an atomic audit/selection transaction. Test live projection separately from successful DS persistence and historical audit-read error observability; broader audit-delivery guarantees remain inherited, not newly certified here.
4. **Evidence provenance and handoff bounds**: the frozen packet is target-scoped and carries the fully qualified target (cluster ID/API group/API version/kind/namespace/name), target-resolution provenance, and observed/recorded timestamps when available. Existing RCA tool/audit records retain source details by correlation; do not duplicate source-resource metadata on every evidence item. Core API group uses its canonical empty-group representation; unavailable target values remain explicitly unknown and are never inferred. The packet is never silently truncated or partially projected. If sanitization, freeze/deep-copy, digest computation or the configured serialized/prompt budget fails, automatic selection routes to existing review; no A2 bounded-compaction path is introduced.

The approved contract is recorded in [DD-KA-2496](../../architecture/decisions/DD-KA-2496-history-aware-rca-and-repeat-selection.md), with amendments to DD-KA-016, DD-KA-017 and ADR-055 and the BR-KA-016 extension. Production comments and tests must reference DD-KA-2496. This records contract approval only; implementation and CHECKPOINT W remain outstanding.

### 4.4 Exact live Console projection

The autonomous KA complete-event path and interactive `present_decision` path use the same additive, allow-listed safe projection. The `rca.confidence` value remains RCA confidence; workflow-selection confidence is never inferred from that field:

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

`selected_workflow` is omitted for no-workflow results. The projection contains no raw audit rows, prior parameters, execution bundles, secrets, prior RR identifiers or full frozen handoff. `repeat_validation_outcome` is server-computed and bounded; `handoff_digest` identifies the frozen packet and is not an audit lookup.

## 5. Implementation sequence and wiring manifest

### 5.1 Required changes

1. **Normative contract**: preserve the formalized BR-KA-016 extension and the user's clarified agent-owned suitability/structural justification/confidence-gate scope; update applicable service audit-catalog rows using SOC2 **CC7.2** for reconstruction, not inherited CC8.1 shorthand.
2. **RO/DS source projection**: copy selected workflow name and explicitly named selection confidence into v1.6 RO workflow-created audits. Extend DS history projection for that single current-version contract; no AA recovery/backfill path. Keep original hash traversal and tier membership intact. Return workflow identity and selection confidence for both tiers; retain existing timestamps with honest prompt labels. Old records stay unmodified with unavailable fields unknown; add bounded, sanitized operational logging of skipped selection-metadata enrichment through the actual history-read path.
3. **Schemas**: extend `api/openapi/data-storage-v1.yaml` history and typed response/audit components. Add a shared typed justification to the KA result and AA `SelectedWorkflow` (not the execution `WorkflowSnapshot`, since justification is selection reasoning, not execution configuration). Regenerate ogen/embedded OpenAPI, deepcopy and AA CRD copies through existing tooling; do not hand-edit generated files.
4. **KA context**: extend existing enrichment DTOs/adapter. Build a non-mutating, globally numbered chronological prompt view across tiers; maintain effectiveness-trend behavior explicitly rather than accidentally reversing its algorithm. Show `workflow_selection_confidence` separately from effectiveness and any RCA confidence. Unknowns remain unknown.
5. **Prompt**: replace action-level hard-prohibition text with workflow-specific failed-strategy guidance. State that presence/rank/RCA fit is not suitability, explain the three repeat requirements, and keep no-workflow available even for a one-candidate catalog.
6. **Phase isolation (#2498)**: keep resource-context/history tools optionally available but not pre-injected in Phase 1; automatically enrich and inject only target context history into the Phase 2 frozen handoff; expose only workflow-discovery and required result-submission tooling in Phase 3; and enforce the trusted phase allow-list before registry/external execution. Update the RCA prompt and authoritative ADR/DD/test-plan wording from mandatory context calls to optional calls.
7. **Trusted structural acceptance**: propagate the object through nested/flat parser paths. Bind request-local history to the existing validator/self-correction seam; implement J-B behavior, including parse retries, text fallback and corrected proposals. Require the catalog-authoritative workflow ID and name on every accepted selection; match repeats by ID only and never use model-supplied identity/name as a fallback. Require the explanation for every same-ID repeat with a completed valid prior assessment; a corrected different workflow does not inherit it. Do not change candidate enumeration or confidence/review policy.
8. **Persistence/audit**: extend `buildSelectedWorkflowMap`, AA `storeSelectedWorkflow`, `preserveLowConfidenceWorkflow`, `preservePartialSelectedWorkflow`, result audit JSON, audit intermediate DTO, typed DS mapper, and AA completion/failure context as needed. These are compliance and past-remediation/history surfaces, not the current live Console transport. Add server-computed prior-attempt links/structural validation outcome where needed for historical reconstruction; do not trust model-supplied prior RR IDs. Preserve the CRD's existing selection immutability behavior.
9. **Live Console projection and journeys**: extend the bounded KA complete-event/AF investigation artifact and existing interactive decision artifact with the safe current workflow/repeat projection. Extend the endpoint-backed Mock LLM fixtures and Kind/full-pipeline journeys; prove justified retry, different strategy, rejected retry, explicit no-workflow outcomes, and live Console delivery with real controllers/storage.

### 5.2 Wiring manifest (all rows required before GREEN completion)

| Component / boundary | Production entry point | Wiring code location | IT proving dispatch and output |
|---|---|---|---|
| Independent evidence packet budget | Helm values → KA config → investigator | `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml` → `config.Load`/`validateAI` → `cmd/kubernautagent/bootstrap.go:buildInvestigator` | IT-PLATFORM-2496-001; IT-KA-2496-008 |
| v1.6 selection metadata in history skeleton | RO workflow-creation dispatch | `internal/controller/remediationorchestrator/audit_events.go:emitWorkflowCreatedAudit` → existing audit manager → DS | IT-RO-2496-002 |
| Bounded prior-attempt metadata projection | Authenticated history HTTP endpoint | DS server construction → `remediation_history_repository.go` / `remediation_history_adapter.go` → `HandleGetRemediationHistoryContext` / tier correlation | IT-DS-2496-001, IT-DS-2496-002, IT-DS-2496-003 |
| Live current RCA/selection → Console artifact | KA MCP complete/event and interactive decision payload | `session.MarshalRCASubset` / KA event stream → `pkg/apifrontend/tools/ka_investigate_bridge.go:emitInvestigationSummaryArtifact` and `launcher/part_converter.go:emitDecisionEvent` → A2A | IT-AF-2496-001, IT-AF-2496-002 |
| A2A artifact schema and gob-safe boundary | Existing investigation summary schema and artifact queue | `launcher/schemas/investigation_summary.v1.schema.json` → `EventBridge.EmitArtifact`/`sanitizeArtifactData` | IT-AF-2496-003 |
| History schema → enrichment → chronological prompt | AgentSession autonomous investigation | `cmd/kubernautagent/datastorage.go` constructs DSAdapter/Enricher; `investigator_phases.go` → `prompt/builder.go:RenderWorkflowSelection` | IT-KA-2496-001 |
| Strict phase-scoped tool availability | Investigator phase dispatch | `DefaultPhaseToolMap` / `toolDefinitionsForPhase` → `processToolCalls` / trusted execution guard | IT-KA-2498-001, IT-KA-2498-002 |
| Sanitized frozen target handoff and request isolation | Autonomous and interactive production selection | `executeTool` pre-transform capture → `runWorkflowSelection` freeze/sanitize/budget/digest → prompt + shared correction | IT-KA-2496-007, IT-KA-2496-009 |
| Positive handoff budget and fail-to-review | Production config and investigator construction | `config.Load` → `cmd/kubernautagent/bootstrap.go:buildInvestigator` → selection | IT-KA-2496-008 |
| Conditional repeat structural validation | Every autonomous workflow submit/recovery | `cmd/kubernautagent/bootstrap.go` constructs investigator/catalog fetcher; `runWorkflowSelection` → `selfCorrectWorkflowSelection` / `Validator.SelfCorrect` | IT-KA-2496-002, IT-KA-2496-003 |
| KA result → persisted AA selection and review context | Completed AgentSession handling | `agentsession/mapping.go:buildSelectedWorkflowMap` → AA handler `ProcessAgentSessionResult` / successful, low-confidence and partial preservation paths | IT-AA-2496-001 |
| Existing confidence/review/approval gates | AA Investigating/Analyzing dispatch | `cmd/aianalysis/main.go` → `WithLowConfidenceFloor` / `WithConfidenceThreshold`; response processor / analyzing handler | IT-AA-2496-004 |
| Typed justification and validation audit → DS | KA/AA selection-result audit emission | `investigator_audit.go` → `audit/ds_response_mapping.go`; `pkg/aianalysis/audit/audit.go` → DS ingestion/query | IT-KA-2496-005, IT-AA-2496-002 |
| No-workflow / rejected-retry → no execution and operator review | AA/RO reconciliation | Existing AA outcome handlers and RO failure/manual-review handlers, without bypassing routing safety | IT-AA-2496-003, IT-RO-2496-001 |

CHECKPOINT W needs a production caller and passing dispatch-path IT for every row. A direct mapper/repository call alone does not prove the full HTTP/controller wiring row. No semantic-review row or skipped semantic-review tests are retained in this revised plan.

### 5.2.1 Frozen-handoff slice (2026-10-10)

Preflight confidence: **96%**. Capture is available in `executeTool` after the
production sanitizer and before summarization/truncation; both autonomous and
interactive entries share `runWorkflowSelection`. Follow existing correlation-
scoped state and cleanup patterns for cross-turn interactive observations; never
share evidence across requests. Remaining risk: source tool observations may
include resource JSON/parameters, so the packet must use a sanitized evidence
projection, not raw payload/audit/conversation persistence.

User approved the concrete contracts in DD-KA-2496 v1.1: independent 64 KiB
`ai.investigation.maxEvidenceHandoffBytes`, sibling non-executable
`selection_assessment` / AA `selectionAssessment`, and NoExecution exclusion.
Issue #2519 supersedes only the operational default: it is now 256 KiB
(`262144` bytes) because the original default caused failures in the default
configuration. Packet immutability, fail-closed overflow and the other DD-KA-2496
contracts remain unchanged.

- **RED (~30–45 min):** supplementary prompt/feedback specs; production config,
  real catalog/provider selection and pre-transform external-observation capture
  tests; packet immutability/digest/unknowns/complete-history and failure UTs.
- **GREEN (~60–90 min):** capture sanitized observations; one frozen serialized
  packet with safe target/history/enrichment/RCA evidence; positive config wiring;
  trusted repeat scope derived from the same snapshot; review on any freeze,
  sanitization or budget failure; shared autonomous/interactive dispatch proof.
- **REFACTOR (~20–30 min):** centralize packet projection and failure handling,
  remove duplicate mutable-history acceptance logic, and bound isolated observation
  state through existing cleanup. Build/tests are validation, not the refactor.
- **Success:** complete packet and server digest reach the first Phase 3 provider
  request and all corrections; no raw secrets/prior parameters/source metadata;
  mutation cannot change the packet/repeat scope; failure invokes no selection
  provider; no off-phase dispatch or candidate filtering.
- **Rollback:** leave automatic selection in existing review on packet failures;
  revert only this session's slice if necessary, never audit rows or unrelated work.

### 5.3 Existing E2E regression manifest

The canonical detailed matrix is [TEST_PLAN.md §15](TEST_PLAN.md#15-existing-e2e-regression-matrix). The implementation owner must use the following treatment; a new #2496/#2498 scenario cannot be substituted for an existing regression gate.

| Treatment | Existing file | Existing scenario IDs | Required action / invariant |
|---|---|---|---|
| **Must update** | `test/e2e/datastorage/25_remediation_history_api_test.go` | `E2E-DS-016-001`–`004` | Extend current-version RO fixtures and HTTP assertions for ID/name/selection-confidence, unknown/zero/SpecDrift semantics, scope and chronology; keep empty history safe and old rows immutable. `E2E-DS-016-005` remains unchanged. |
| **Must update** | `test/e2e/kubernautagent/three_step_discovery_test.go` | `E2E-KA-2459-001`, `E2E-KA-017-001-001b` | Assert catalog-authoritative workflow ID/name and selection confidence separately from RCA confidence; retain audit minimization and sentinel exclusion. |
| **Must update** | `test/e2e/kubernautagent/interactive_discovery_e2e_test.go` | `E2E-KA-DISC-001`, `004`, `005` | Extend successful selection decoding for the safe projection; retain parameters, Lease release, discovery gating and RCA-target GVK isolation. |
| **Must update** | `test/e2e/kubernautagent/audit_pipeline_test.go` | `E2E-KA-2478-001`, `002`, `E2E-KA-433-AP-001`, `002` | Extend selected-result/response-complete correlation checks for persisted safe metadata without raw parameters; preserve actor/action/outcome/UUID and ranked/generic fallback behavior. |
| **Must update** | `test/e2e/kubernautagent/param_validation_e2e_test.go` | `E2E-KA-1170-001`, consolidated `002`, `003` | Keep invalid→corrected parameter proof and add repeat validation on the corrected path; a changed workflow does not inherit a repeat requirement and an invalid same-ID repeat cannot pass. |
| **Must update** | `test/e2e/apifrontend/progressive_flow_e2e_test.go` | `E2E-AF-1408-001` | Extend live `investigation_summary` artifact parsing for the bounded safe projection/fallback; retain schema/version, DataPart and summary assertions, with no DS read. |
| **Must update** | `test/e2e/apifrontend/structured_decision_e2e_test.go` | `E2E-AF-1395-001`, `1396-001`, `1396-002` | Extend interactive decision decoding for the same projection; retain JSON integrity, authoritative RCA/target fields, all options and no hidden choice. |
| **Must update** | `test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go` | `E2E-FP-118-001`, `E2E-FP-118-002` | Keep all existing lifecycle/status/WFE/notification/reconstruction assertions and extend actual v1.6 selected-workflow status/audit provenance checks. |
| **Must update** | `test/e2e/fullpipeline/15_af_a2a_interactive_streaming_test.go` | `E2E-FP-1189-005` | Extend live SSE RCA/decision artifact checks for the safe projection; retain visible RCA, progress completion, WFE and target-parameter assertions. |
| **Rerun unchanged** | `test/e2e/kubernautagent/incident_analysis_test.go`, `adversarial_parity_e2e_test.go` | `E2E-KA-001`–`006`; `E2E-KA-433-ADV-010`–`015`, `E2E-KA-1039-001`–`002` | Preserve no-workflow/manual-review, low-confidence, exhausted-correction, incident-correlation and deleted-resource behavior. Do not weaken assertions to accommodate new fields. |
| **Rerun unchanged** | `test/e2e/kubernautagent/interactive_discovery_e2e_test.go`, `workflow_catalog_test.go`, `empty_tool_result_2482_e2e_test.go` | `E2E-KA-DISC-002`, `003`; `E2E-KA-030`–`044`; `E2E-KA-2482-001` | Preserve no-action/gating, catalog availability/no-ban, and optional-context/no-empty-tool failure behavior. |
| **Rerun unchanged** | `test/e2e/kubernautagent/audit_pipeline_test.go` | `E2E-KA-045`–`048`, `1111-001`–`003`, `433-AP-003`, `1401-001`, `1401-002` | Preserve event presence, correlation, actor attribution and security audit behavior. |
| **Rerun unchanged** | `test/e2e/apifrontend/mcp_fullpath_test.go`, `discover_workflows_test.go`, `ka_integration_test.go` | `TC-E2E-MCP-FULL-02+04`, `03`, `05`, `06`; `E2E-AF-WP-001`–`006`; `TC-E2E-KA-01/02/05`, `TC-E2E-DS-02`–`05`, `TC-E2E-KA-06/07`, `TC-E2E-MULTI-01` | Preserve AF public MCP tool count (23), unknown-tool/RBAC behavior, explicit history/audit retrieval, and live KA/A2A routing. |
| **Rerun unchanged** | `test/e2e/apifrontend/progressive_flow_e2e_test.go`, `fleet_mode_e2e_test.go` | `E2E-AF-1407-001`–`003`; `E2E-AF-FLEET-2462-001`, `001a`–`001f`, `002`–`005` | Preserve no-RCA status-only/fail-closed outcomes and hub/remote target attribution. |
| **Rerun unchanged** | `test/e2e/fullpipeline/05_mcp_interactive_lifecycle_test.go`, `06_af_audit_trace_test.go`, `07_af_a2a_autonomous_test.go`, `08_af_a2a_interactive_test.go`, `12_problem_resolved_no_we_test.go`, `13_escalation_lifecycle_test.go`, `17_af_a2a_full_interactive_remediation_test.go` | `FP-MCP-001`/`002`/`003a`/`005`/`005c`/`006`/`008`/`009`, `E2E-FP-1452-001`; `E2E-FP-AF-001`; `E2E-FP-1189-002`; `E2E-FP-2390-001`; `E2E-FP-1430-001`; `E2E-FP-1456-001`; `E2E-FP-1853-002` | Preserve interactive lifecycle, AF audit trace, autonomous no-IS, selected parameters/dependencies, no-WE and full-interactive completion. |
| **Rerun unchanged** | `test/e2e/fullpipeline/18_af_a2a_consent_gate_test.go`, `03_ka_target_resource_test.go`, `16_standalone_execution_cluster_id_test.go` | `E2E-FP-1899-001`, `002`, `1912-001`, `1918-001`; `E2E-FP-496-001/002/003`; `E2E-FP-2390-005` | Preserve consent-gated phase transitions, terminal no-reinvocation, non-actionable no-WE, target injection and execution-cluster attribution. |
| **Unaffected** | `test/e2e/kubernautagent/jwt_e2e_test.go`, `test/e2e/apifrontend/session_crd_test.go`, `test/e2e/apifrontend/alert_prioritization_e2e_test.go`, `test/e2e/datastorage/05_soc2_compliance_test.go`, `test/e2e/fullpipeline/04_interactive_regression_test.go`, `14_crashloop_real_fix_test.go` | `E2E-KA-JWT-001`–`005`; `TC-E2E-SESS-001`–`004`; `E2E-AF-1412-001/002`; `BR-SOC2-001`–`005`; `E2E-KA-HARM-001`; `E2E-FP-1542-001` | No #2496-specific assertion edit; still included in the final full-suite gate. Any generated-decoder failure moves the case to **must update** without removing its business assertion. |

The initial Engram graph requests failed with `RemoteProtocolError`/`ConnectError`; a bounded retry recovered but returned a highly ambiguous graph-wide result (`82,430` total calls, `43,464` unresolved, `22,049` ambiguous). It did confirm the main production chains from prompt rendering, tool dispatch, RCA event serialization, AF artifact emission and AgentSession mapping, but not network-driven E2E callers and not resolved callers for the RO producer/history handler. Exact live-source inspection remains necessary for this planning-only E2E matrix; no graph-complete claim is made.

## 6. TDD plan and effort

Estimate: **3–4 engineering days**, excluding infrastructure repair/deployment wait. No extra inference/reviewer component is included.

| Slice | RED | GREEN | Concrete REFACTOR |
|---|---|---|---|
| RO/DS history fidelity | 3–4 hours: producer/endpoint IT first, then scalar/provenance/unknown-value UT | 5–7 hours: explicit v1.6 RO metadata, schemas and both-tier projection; no legacy recovery | 1–2 hours: consolidate extraction, ordering and contextual errors |
| KA identity/history/selection | 3–4 hours: investigator wiring IT; prompt/predicate/parser/retry UT | 5–7 hours: chronological context, conditional structural validator/correction wiring | 1–2 hours: deduplicate acceptance checks and clarify confidence/identity provenance |
| Persistence / audit / outcome journey | 3–4 hours: AA/DS audit IT, confidence boundaries, no-workflow/no-WE IT, E2E fixtures | 4–6 hours: explicit DTO/CRD/audit mappings and journey behavior | 1–2 hours: consolidate projection fixtures/nil handling, remove obsolete prompt assertions |
| Phase isolation / optional RCA tools (#2498) | 1–2 hours: Phase 1/3 schema and forced-dispatch IT first, then optional no-call RCA behavior | 2–3 hours: trusted phase allow-list wiring and prompt/docs correction | 30–60 minutes: centralize allow-list lookup and observable rejection handling |

For each slice: write the production-entry IT and logic UT **before** implementation, record expected failures, implement minimal behavior and production wiring, and make both tiers pass before declaring GREEN. Then execute the §5.3 **must update** E2E edits and the full **rerun unchanged** regression matrix. E2E proves the complete journey; a selected UUID or a substring in a canned response alone is not sufficient.

During REFACTOR, apply/fix the Go anti-pattern checklist: no 8+ parameter signatures, context-bearing state structs, shared per-request validator state, unbounded loops, ignored errors or speculative business types. Build/test validation is the safety net, not the content of REFACTOR.

## 7. Success criteria, readiness and risks

### Success / release gates

- All issue scenarios pass: first selection, different workflow after a prior attempt, justified same-ID repeat after both successful/remediated and ineffective prior assessments, and non-empty catalog with no suitable workflow → `selectedWorkflow=null` / `no_matching_workflows` / manual review / no WE.
- Both tiers preserve workflow identity and the **selected-workflow** confidence. The model sees ascending, globally numbered attempt history; missing confidence/name is unknown and dates are not misrepresented as verified execution completion.
- Current-version producer dispatch persists the metadata before a later v1.6 attempt reads it. No legacy recovery/backfill path is a release requirement; defensive unknown/malformed-data coverage remains required.
- Reads encountering old/incomplete historical selection metadata produce a traceable, bounded diagnostic that no backfill was attempted. Existing records remain intact and missing values stay unknown; logging must not expose parameters/secrets or misclassify zero confidence, empty history or pending effectiveness as missing historical selection metadata.
- One proven failed attempt is sufficient for specific guidance. No action-family hard ban, candidate filtering, arbitrary retry threshold, or nil-to-zero inference remains in the changed reasoning path.
- Missing/incomplete repeat justification for a same-ID repeat follows existing bounded correction on every result path. Prompt guidance rejects symptom/RCA fit alone as reasoning; no additional semantic judge claims to prove narrative truth.
- Workflow identity and repeat matching use the authoritative `workflow_id` only. A required catalog-authoritative name may be shared across versions/action types and is never used as an identity fallback; action type/version remain descriptive context.
- The frozen handoff never silently truncates or projects a partial packet. Any sanitization, freeze, digest or configured-size failure routes automatic selection to existing review; no A2 compaction path is implemented.
- Phase 1 may complete without a resource-context/history call; Phase 3 advertises and executes only workflow-discovery and required result-submission tooling, with no Kubernetes/context-history access. Forced off-phase calls are blocked before registry/external execution and are observable.
- Configured low-confidence review and later Rego approval use current selected confidence, retain the justification for operators, and are not bypassed or replaced by prior confidence, RCA confidence or a new repeat threshold.
- Explicit past-remediation/history queries reconstruct prior evidence, current justification and validation outcome by correlation, with event UUID/action/outcome/actor metadata; the live current-result card is delivered from the KA MCP/A2A payload and does not query DS audit data. No new prior-parameter disclosure.
- Every wiring row has passing IT; all applicable control objectives in TP-2496 have a proving E2E journey.
- ≥90% of defined in-scope business scenarios with 100% of critical scenarios proven by passing UTs, 100% wiring/control objective coverage, and ≥80% merged structural coverage; no skipped specs. Raw line/branch coverage is diagnostic, not a literal business-logic target.
- Build, compile-only tests, lint, affected suites, generated-artifact consistency and regressions pass. Runtime IT/E2E must actually run; an environment blocker is not a passing gate.

### Pre-implementation readiness audit

All 13 AGENTS.md dimensions are planned in TP-2496: build/lint, unit/integration, CHECKPOINT W, BDD/IDs, control traceability, Go anti-patterns, BR satisfaction, regression, observable/fail-safe behavior, and domain safety. **Readiness remains conditional** on explicit approval of this synchronized plan and the RED/GREEN gates; no implementation or GA pass is claimed.

| Risk | Mitigation / stop condition |
|---|---|
| Mere structure is misrepresented as semantic proof | Preserve the user's explicit agent-owned reasoning scope. Never declare arbitrary narrative adequacy proven by `minLength` tests. |
| Agent stays overconfident despite richer evidence | Show prior confidence/effectiveness explicitly and require retry explanation; test existing current-confidence review/approval gates. Real-provider rehearsal characterizes reasoning quality, not a flaky CI pass/fail claim. |
| Missing/late/malformed current-version audit evidence | Preserve unknowns; bind producer snapshot provenance and temporal identity; test delayed ingestion and mismatched workflow IDs. No guessed confidence or compatibility fallback. |
| Origin/execution clusters differ | Emit/read RO history metadata in the origin-cluster scope; no AA recovery or WFE execution-cluster joins in this slice. |
| Timestamp changes alter RO safety counts | Keep legacy timestamps/tier/routing behavior untouched; label them honestly and record actual-completion fidelity separately. |
| Live MCP/A2A field dropped, or historical audit field dropped/delivery fails | Add AF live-artifact projection IT plus raw JSON + typed-client + CRD/history read-back IT for past-remediation/compliance paths, including human-review paths; retain existing observable error/retry/delivery handling. No new synchronous-capture policy or live DS audit lookup. |
| Phase tool leakage or mandatory RCA context call | Phase 3 schemas and execution allow-list are tested separately; Phase 1 no-call completion is tested; prompt and ADR/DD wording are updated together. |
| Prior identity/name or explanation injects instructions/secrets | Sanitize model-facing fields with existing injection/credential facilities; minimize projections; JSON-encode audit fields; enforce bounds. |
| Cross-service schema pruning or v1.6 contract mismatch | Deploy matching generated AA CRDs and DS/KA/AA/RO binaries as a coordinated v1.6 release; verify admitted status round trip. Mixed v1.5/v1.6 support is not a gate. |

**Rollback**: revert only this feature's code release as a coordinated unit; do not delete existing audit history. No destructive data migration is planned. If schema copies, provenance, acceptance validation or reconstruction fail, suspend rollout and route unreliable results to existing human review rather than disabling the gate.

**Confidence: 95% for the pre-implementation approach, not implementation/GA readiness.** The final bounded preflight resolved the remaining insertion-point uncertainties: trusted phase state reaches dispatch, sanitized observations are available before truncation, repeat validation converges through the existing self-correction seam, and RO/DS/KA/AgentSession/AA ownership is identified. Focused suites and repository compile-only validation pass. Outstanding work is deliberately implementation work: RED tests, v1.6 producer dispatch, actual freeze/digest failure handling, repeat correction, typed persistence, generated artifacts, runtime wiring and E2E journeys. Legacy AA recovery/backfill, #2497, actual-completion reconstruction, WFE joins, synchronous audit gating and a semantic reviewer remain excluded. User approval is still required; prototype and baseline success cannot certify the feature or CHECKPOINT W for unproved boundaries.
