# BR-KA-016: History-Aware Remediation Context and Repeat Selection

**Business Requirement ID**: BR-KA-016<br>
**Category**: Kubernaut Agent (KA)<br>
**Priority**: P0<br>
**Target Version**: V1.6<br>
**Status**: ✅ Approved contract; implementation pending<br>
**Date**: 2026-10-09<br>
**Related Issues**: [#2496](https://github.com/jordigilh/kubernaut/issues/2496), [#2498](https://github.com/jordigilh/kubernaut/issues/2498)<br>
**Authority**: [DD-KA-2496](../architecture/decisions/DD-KA-2496-history-aware-rca-and-repeat-selection.md)

---

## Business need

When a target recurs, workflow selection must use trustworthy target-scoped history and preserve enough identity and reasoning for an operator to reconstruct why a workflow was selected again. The current RCA/selection result must reach the Console through the existing KA MCP → AF/A2A path; audit/DS reads are reserved for past-remediation/history retrieval and are not a live-result transport. History must inform the agent without removing eligible catalog workflows or replacing existing confidence, approval and execution-safety controls.

## Approved acceptance criteria

1. Current-version workflow-created history preserves mandatory `workflow_id`, catalog-authoritative `workflow_name` when available, explicitly labeled workflow-selection confidence, effectiveness/resolution assessment, target/cluster scope and recorded chronology. Missing values remain unknown; no legacy recovery/backfill or identity fallback is used.
2. Phase 1 resource-context/history tools are optional and history is not pre-injected. Phase 2 automatically enriches exactly one resolved target and supplies a sanitized immutable frozen handoff to selection. The packet contains one `target` and no duplicated per-item source metadata.
3. A handoff that is unsafe, unsanitizable, unfreezable, undigestible or over `ai.investigation.maxEvidenceHandoffBytes` (default 65,536 bytes/64 KiB, separate from tool-output limits) fails closed to existing review; it is never silently truncated or partially accepted.
4. `workflow_id` is the sole identity and repeat key. `workflow_name` is mandatory catalog context but is not unique; action type/version are descriptive only. Missing ID/name invalidates selection. All candidates remain discoverable.
5. A same-ID repeat requires non-blank, bounded structural justification when a target-scoped prior Full/Partial assessment has a present valid score, whether the prior result was successful/remediated or ineffective. First selections, different IDs and pending/invalid/unknown/SpecDrift/NoExecution history do not require it. NoExecution proves no workflow execution occurred.
6. The justification fields are `evidence_basis`, `prior_outcome_relevance` and `expected_mechanism`, with a 2,000-character maximum per field. No semantic reviewer, keyword scoring, confidence penalty, candidate filter, action-family prohibition or safety bypass is added.
7. Phase 3 exposes and executes only workflow-discovery/result-submission tools. Server-side phase authorization rejects off-phase Kubernetes/context-history calls before registry/external execution and records the rejection observably.
8. Success, low-confidence and partial human-review paths preserve the approved identity, current confidence, justification/requiredness, server-computed prior-match/validation outcome, handoff digest and existing audit envelope through AgentSession, AA status and typed audit/DS paths. The live current-result projection uses the KA MCP event/decision payload and AF/A2A artifact; it does not query audit data. Raw tool payloads, secrets, prior parameters and model-supplied prior RR IDs are not persisted.
9. Rejected/no-workflow proposals retain safe, non-executable review context in sibling `selection_assessment` (KA/audit) / `selectionAssessment` (AA status), never as an executable selected-workflow ID.
10. The live `kubernaut_present_decision` response does not author cluster identity. `cluster_id` is server-owned context: AF projects only the trusted `RRContext`/RemediationRequest value and omits the field when no trusted value exists. A validated `cluster_id` supplied to an RR-creating tool is a separate request-input contract and is not authority for the live decision artifact.

## Control objectives

| Objective | Applicable controls |
|---|---|
| Trusted input, sanitization, target scoping and fail-closed degradation | FedRAMP SI-10, AC-4; OWASP ASVS 5.0.0 V2.2.1–V2.3.1 |
| Server-owned live cluster attribution and fail-closed omission | FedRAMP AC-4, AU-3, SI-10; OWASP ASVS 5.0.0 V2.2.1, V2.2.2 |
| Least-privilege phase tool execution | FedRAMP AC-6/SI-10; OWASP ASVS V2.2.3 |
| Auditable decision and remediation reconstruction | FedRAMP AU-2/AU-3; SOC 2 CC7.2; OWASP ASVS V16.2 |
| Change approval/testing | SOC 2 CC8.1 for this implementation process only |

## Exclusions

This requirement does not include #2497's runtime reviewer, a new service/table, compatibility recovery/backfill, raw previous parameters, a semantic narrative judge, retry thresholds, confidence penalties, failed-workflow exclusion, synchronous audit acknowledgement or actual execution-completion reconstruction.
