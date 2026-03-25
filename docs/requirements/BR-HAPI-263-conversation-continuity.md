# BR-HAPI-263: Conversation Continuity Across Investigation Phases

**Business Requirement ID**: BR-HAPI-263
**Category**: HolmesGPT API Service (SDK + HAPI)
**Priority**: P1
**Target Version**: V1
**Status**: Approved
**Date**: 2026-03-25
**Related Issue**: [#529](https://github.com/jordigilh/kubernaut/issues/529)

---

## Business Need

### Problem Statement

The current self-correction loop restarts the LLM conversation from scratch on each retry. The LLM loses all context from its investigation (tool call results, reasoning chain, RCA analysis) when HAPI detects a validation error and retries. This leads to:

1. Wasted tokens re-investigating the same signal
2. Non-deterministic results across retries (different investigation paths)
3. Inability to build on the LLM's previous reasoning when providing correction feedback

With the three-phase architecture (#529), conversation continuity is critical: Phase 3 (Workflow Selection) must receive the full conversation from Phase 1 (RCA) so the LLM has its investigation context when selecting a workflow.

### Business Objective

Thread LLM message history across retry attempts within each phase AND across Phase 1 -> Phase 3. The LLM never loses context.

---

## Acceptance Criteria

### SDK Level

1. `InvestigationResult` exposes a `messages` field containing the full LLM conversation transcript
2. `InvestigateRequest` accepts an optional `previous_messages` parameter
3. When `previous_messages` is provided, `investigate_issues` / `prompt_call` resumes from that conversation state instead of creating a fresh `[system, user]` pair
4. All existing callers that don't pass `previous_messages` continue to work unchanged (backward compatible)

### HAPI Level

5. Phase 1 retries thread `previous_messages` from the failed attempt (LLM sees its prior reasoning + HAPI's correction feedback)
6. Phase 3 receives `previous_messages` from Phase 1 (LLM sees the full RCA conversation + enrichment context)
7. Phase 3 retries thread `previous_messages` from the failed Phase 3 attempt

---

## Design References

- **DD-HAPI-002 v1.4**: Workflow Response Validation (three-phase loop structure)
- **Issue #529**: RCA Flow Redesign

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-25 | Initial requirement: conversation continuity across phases |
