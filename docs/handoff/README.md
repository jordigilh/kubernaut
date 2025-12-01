# Cross-Team Questions from HolmesGPT-API

**Date**: December 1, 2025
**From**: HolmesGPT-API Team
**Context**: HolmesGPT-API v3.2 Release - Integration Questions

---

## Overview

These documents contain questions from the HolmesGPT-API team to various teams regarding integration points, architectural decisions, and implementation details.

---

## Documents by Team

| Team | Document | Priority | Status | Topics |
|------|----------|----------|--------|--------|
| **Workflow Engine** | [QUESTIONS_FOR_WORKFLOW_ENGINE_TEAM.md](./QUESTIONS_FOR_WORKFLOW_ENGINE_TEAM.md) | High | ✅ **RESPONDED** | Parameter validation architecture (DD-HAPI-002), ValidateParameters implementation, metrics |
| **Data Storage** | [QUESTIONS_FOR_DATA_STORAGE_TEAM.md](./QUESTIONS_FOR_DATA_STORAGE_TEAM.md) | Medium | ✅ **RESPONDED** | container_image in search, pagination, detected_labels population |
| **SignalProcessing** | [QUESTIONS_FOR_SIGNALPROCESSING_TEAM.md](./QUESTIONS_FOR_SIGNALPROCESSING_TEAM.md) | Medium | ⏳ Pending | custom_labels source/extraction, RCA context fields, error handling |
| **AIAnalysis** | [QUESTIONS_FOR_AIANALYSIS_TEAM.md](./QUESTIONS_FOR_AIANALYSIS_TEAM.md) | High | ⏳ Pending | Response format, WorkflowExecution creation, confidence thresholds |

---

## Quick Summary of Key Questions

### Workflow Engine Team ✅ RESPONDED
1. **DD-HAPI-002 Approval**: ✅ **APPROVED** - 3-layer validation architecture confirmed
2. **ValidateParameters**: ✅ **PLANNED** - Will implement for WE v1.0, validation failures marked as pre-execution (safe to retry)
3. **Metrics**: ✅ **APPROVED** - Added labels for granularity (workflow_id, failure_reason)

**WE also shared**: New v3.1 schema updates including `wasExecutionFailure` flag to distinguish pre-execution vs during-execution failures.

### Data Storage Team ✅ RESPONDED
1. **container_image**: ✅ **CONFIRMED** - Fields ARE included in search responses (recent fix applied)
2. **detected_labels**: ⚠️ **PARTIAL** - Filtering works, auto-population NOT YET implemented (planned for v2.0)
3. **Pagination**: ✅ **CONFIRMED** - `top_k` max 100, offset-based pagination supported

### SignalProcessing Team
1. **custom_labels**: How are they extracted from Alertmanager webhooks?
2. **RCA Context**: What fields are included (resource_kind, cluster_name)?

### AIAnalysis Team
1. **Response Format**: Is the HolmesGPT-API response format compatible?
2. **WorkflowExecution**: How are parameters mapped to CRD spec?

---

## How to Respond

1. Open the relevant document for your team
2. Scroll to the "Response" section at the bottom
3. Fill in your answers with date and responder name
4. Commit the changes or notify the HolmesGPT-API team

---

## Related Documents

- [DD-HAPI-001: Custom Labels Auto-Append](../architecture/decisions/DD-HAPI-001-custom-labels-auto-append.md)
- [DD-HAPI-002: Workflow Parameter Validation](../architecture/decisions/DD-HAPI-002-workflow-parameter-validation.md)
- [HANDOFF_CUSTOM_LABELS_PASSTHROUGH.md](../services/stateless/holmesgpt-api/HANDOFF_CUSTOM_LABELS_PASSTHROUGH.md)
- [HolmesGPT-API README](../../holmesgpt-api/README.md)

---

## Contact

For questions about these documents, contact the HolmesGPT-API team.

