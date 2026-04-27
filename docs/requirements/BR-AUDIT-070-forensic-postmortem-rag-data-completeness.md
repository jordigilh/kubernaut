# BR-AUDIT-070: Forensic Post-Mortem RAG Data Completeness

**Document Version**: 1.0
**Date**: April 2026
**Status**: ✅ APPROVED
**Category**: Audit & Compliance
**Related DDs**: DD-AUDIT-004, DD-AUDIT-005, ADR-038
**SOC2 Controls**: CC6.1 (Financial Governance), CC7.2 (Internal Controls), CC8.1 (Audit Completeness)

---

## 1. Purpose & Scope

### 1.1 Business Purpose

Kubernaut's forensic post-mortem capability allows operators to interact with past remediations through an LLM-driven RAG session, reviewing the rationale behind decisions made during automated incident response. This requires comprehensive audit event persistence so that all investigation context — including session lifecycle transitions, token usage, accumulated conversation state, and alignment verdicts — is available for retrieval.

### 1.2 Scope

- **Session Lifecycle Audit**: Persist all `aiagent.session.*` events to DataStorage with structured payloads
- **Investigation Cancellation Forensics**: Capture accumulated messages, token usage, and phase/turn state at cancellation for cost attribution and partial investigation reconstruction
- **Alignment Audit Persistence**: Persist `aiagent.alignment.step` and `aiagent.alignment.verdict` events so shadow agent findings are durable
- **Cross-Event Correlation**: Ensure all events use consistent `correlationID` (remediation_id) for forensic RAG join queries
- **Session Snapshot Enrichment**: Expose cancellation state, RCA summary, and token usage through the snapshot API for operator visibility

---

## 2. Business Requirements

### BR-AUDIT-070: All KA event types must have DataStorage payload schemas

Every event type registered in `AllEventTypes` MUST have:
1. A corresponding payload schema in `data-storage-v1.yaml`
2. A discriminator mapping entry
3. A `buildEventData` case in `ds_store.go`

**Acceptance Criteria**:
- Table-driven unit test asserts `buildEventData` returns `ok == true` for every entry in `AllEventTypes`
- No audit events are silently dropped due to missing payload schemas

### BR-AUDIT-071: Session started events carry investigation context

The `aiagent.session.started` event MUST include:
- `session_id`, `incident_id`, `signal_name`, `severity`, `created_by`

### BR-AUDIT-072: Investigation cancellation events carry forensic state

The `aiagent.investigation.cancelled` event MUST include:
- `cancelled_phase`, `cancelled_at_turn`
- `total_prompt_tokens`, `total_completion_tokens`, `total_tokens` (cost attribution)
- `accumulated_messages` (JSON, capped at 64KB for storage bounds)

### BR-AUDIT-073: Access denied events carry correlation context

The `aiagent.session.access_denied` event MUST include:
- `correlationID` from the target session's `remediation_id`
- `session_owner` identity for forensic cross-reference

### BR-AUDIT-074: Alignment events use remediation_id correlation

All `aiagent.alignment.*` events MUST use `signal.RemediationID` as `correlationID` (not `signal.Name`) for consistent join with investigation events.

### BR-AUDIT-075: Session snapshot exposes investigation result state

The `SessionSnapshot` API response MUST include (when available):
- `cancelled_phase`, `cancelled_at_turn` (for cancelled sessions)
- `rca_summary` (for completed/cancelled sessions with partial results)
- `total_prompt_tokens`, `total_completion_tokens` (for cost visibility)

---

## 3. Implementation Reference

| Requirement | Implementation |
|---|---|
| BR-AUDIT-070 | 9 new payload schemas in `data-storage-v1.yaml`, 9 `buildEventData` cases, table-driven coverage test |
| BR-AUDIT-071 | `emitSessionEvent` enriched with metadata fields at `StartInvestigation` callsite |
| BR-AUDIT-072 | `emitCancellationAudit` enriched with `TokenAccumulator` data and serialized messages |
| BR-AUDIT-073 | `EmitAccessDenied` reads session store for `correlationID` and `session_owner` |
| BR-AUDIT-074 | `emitAlignmentAudit` uses `signal.RemediationID` instead of `signal.Name` |
| BR-AUDIT-075 | `SessionSnapshot` handler populates fields from `InvestigationResult` |

---

## 4. Adversarial Due Diligence Findings Addressed

This BR was created in response to a comprehensive adversarial review that identified 28 findings across 8 dimensions (Security, Correctness, Auditability, Operational Robustness, Performance, Design Quality, Maintainability, Governance). Key findings:

- **COR-1/AUD-1 (Critical)**: 9 event types silently dropped by DataStorage F-3 validation
- **SEC-1 (High)**: Accumulated messages need content cap for storage bounds
- **COR-2 (High)**: Cancellation events lacked token cost attribution
- **AUD-2 (High)**: Session started events lacked investigation context
- **SEC-2 (Medium)**: Access denied events had empty correlationID
- **SEC-3 (Low)**: Alignment events used wrong correlation key
- **SEC-4 (Low)**: Observed events lacked session owner attribution
