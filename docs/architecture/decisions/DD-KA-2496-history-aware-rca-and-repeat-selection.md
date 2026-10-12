# DD-KA-2496: History-Aware RCA, Frozen Target Handoff and Repeat Selection

**Status**: Approved for implementation; wiring-first TDD in progress<br>
**Decision Date**: 2026-10-09<br>
**Version**: 1.1 — concrete handoff/review contracts approved 2026-10-10<br>
**Applies To**: Kubernaut Agent, DataStorage, AIAnalysis, Remediation Orchestrator and existing audit/status paths<br>
**Issues**: [#2496](https://github.com/jordigilh/kubernaut/issues/2496), [#2498](https://github.com/jordigilh/kubernaut/issues/2498)

---

## Decision

Implement history-aware RCA and repeat reasoning across two existing paths: past-remediation history is read through the existing RO/DS → KA history path, while the current live RCA/selection result is delivered through KA MCP → AF/A2A → Console. Compliance audit persistence remains independent and is consumed only by explicit past-remediation/history retrieval. Do not add a service, table, semantic reviewer, live-path audit lookup or legacy recovery path.

### 1. Phase-scoped delivery

- **Phase 1/RCA** may advertise resource-context/history tools, but calls are optional. No remediation history is pre-injected and RCA completion must not depend on either tool.
- **Phase 2** automatically resolves and enriches exactly one target. It sanitizes, freezes/deep-copies and serializes the authoritative target-scoped handoff for selection. The packet has one `target` field, not `final_target` or a multiple-target model.
- **Phase 3** exposes only workflow discovery and required result-submission tools. A trusted server-side phase allow-list is checked before registry or external execution. Model-supplied phase arguments are not authoritative; off-phase calls are rejected and observable.

### 2. Frozen target-scoped handoff

The model-facing handoff contains:

- schema version, one fully qualified `target` (cluster ID, API group/version, kind, namespace and name), target-resolution provenance and available timestamps;
- current observations, prior target attempts, comparisons, hypotheses, contrary evidence and explicit unknowns; and
- a server-computed handoff digest and freeze timestamp. The digest is a property of the frozen packet, not a read from the audit trail.

It contains no per-item `source_kind`, `source_type`, `source_ref` or `source_resource` metadata. Existing correlated RCA/tool/audit records retain source details for compliance and past-remediation reconstruction; they are not queried to transport the current live result. Unknown values remain unknown and are never inferred.

The packet is never silently truncated or partially projected. Sanitization, freeze/deep-copy, digest or configured serialized/prompt-budget failure routes automatic selection to the existing review path. No A2 bounded-compaction behavior is introduced.

`ai.investigation.maxEvidenceHandoffBytes` is a positive serialized-packet byte
budget, default **65,536 bytes (64 KiB)**. It is separate from tool-output
summarization/truncation. The entire model-facing packet, including the digest and
freeze timestamp, must fit; the digest covers canonical sanitized content excluding
those two runtime fields. The same packet and digest are reused across corrections.

### 3. Workflow identity and repeat reasoning

- `workflow_id` is mandatory, authoritative and the sole identity/repeat-matching key.
- `workflow_name` is mandatory catalog-authoritative context for operator/audit readability but is not unique. Names may be shared across versions or action types. `action_type` and `workflow_version` are optional descriptive context and never matching keys.
- Missing `workflow_id` or `workflow_name` invalidates a selection. There is no name, version, action-type or tuple fallback. Historical records missing either mandatory identity field cannot establish a repeat match.
- All catalog candidates remain discoverable. A first selection or different workflow ID does not require repeat justification.
- A same-`workflow_id` repeat requires a structurally valid bounded object when a target-scoped prior attempt has a completed valid assessment, regardless of whether that outcome was successful/remediated or ineffective. Full/Partial assessments require a present valid score. Pending, invalid, unknown, SpecDrift and **NoExecution** assessments do not trigger the requirement; NoExecution proves that the workflow did not execute.

The approved object is stored beside, not inside, the execution snapshot:

```json
{
  "evidence_basis": "...",
  "prior_outcome_relevance": "...",
  "expected_mechanism": "..."
}
```

Each field is non-blank and limited to 2,000 characters. Validation is structural only and runs for initial, parse-recovered and corrected results. There is no semantic judge, keyword score, confidence penalty, candidate filtering, action-family ban or execution-safety bypass.

### 4. Live result, persistence and audit

Use existing typed boundaries and persistence paths. Extend the bounded KA complete-event/AF `investigation_summary` artifact (and the existing interactive decision artifact where applicable) with the safe current workflow/repeat projection needed by the live Console. Preserve the workflow identity/name, current selection confidence, repeat justification, whether it was required, server-computed prior-match/validation outcome, handoff digest and existing audit envelope on successful and human-review paths. The audit/DS projection is for compliance and explicit past-remediation/history retrieval, not the live result transport. Do not persist raw tool payloads, secrets, prior parameters or model-supplied prior RR identifiers. Existing audit delivery remains best-effort and observable; no synchronous audit gate, live DS audit lookup or new table is added.

Rejected/no-workflow proposals use a sibling **`selection_assessment`** on KA/audit
wires and **`selectionAssessment`** in AA RCA status. It is non-executable: retain
the proposed identity, bounded reasoning, known current selection confidence and
trusted repeat-match/validation/digest context without putting the rejected ID in
`selected_workflow`. Internal trusted prior-attempt links stay out of the live
Console projection. This contract and the 64 KiB default were explicitly approved
by the user on 2026-10-10.

---

## Control and requirement mapping

| Contract area | Business/control objective |
|---|---|
| Target scoping, sanitizer and trusted validation | BR-KA-016, BR-KA-211; FedRAMP SI-10 and AC-4; OWASP ASVS 5.0.0 V2 |
| Phase allow-list and off-phase rejection | BR-KA-017/#2498; FedRAMP AC-6/SI-10; OWASP ASVS V2.2.1/V2.2.3 |
| Typed audit/status reconstruction | BR-AUDIT-005/ADR-034; FedRAMP AU-2/AU-3; SOC 2 CC7.2; OWASP ASVS V16.2 |
| Human review and existing execution safety | BR-KA-197, BR-KA-200, BR-KA-191; existing confidence, Rego and RO safety controls |

CC8.1 applies to the change-management process for this implementation, not as an audit-event completeness label.

## Explicit exclusions

No #2497 runtime reviewer, legacy AA recovery/backfill, new database/registry service, raw prior-parameter disclosure, semantic narrative reviewer, retry threshold, confidence penalty, failed-workflow exclusion, synchronous audit acknowledgement or actual WFE completion reconstruction is part of this decision.

## Related authority

- [DD-KA-016](DD-KA-016-remediation-history-context.md) — target-scoped history query and assessment semantics.
- [DD-KA-017](DD-KA-017-three-step-workflow-discovery-integration.md) — discovery projection and phase separation.
- [ADR-055](ADR-055-llm-driven-context-enrichment.md) — target-only automatic enrichment timing.
- [DD-KA-001](DD-KA-001-workflow-response-validation-architecture.md) — existing structural validation/self-correction seam.
- [DD-WORKFLOW-012](DD-WORKFLOW-012-workflow-immutability-constraints.md) — UUID identity and name/version metadata semantics.

**Implementation note:** production comments and tests implementing this contract must reference `DD-KA-2496`.
