# Notice: AIAnalysis V1.0 Compliance Gaps

**From**: AIAnalysis Team
**To**: All Teams (Cross-Team Visibility)
**Date**: 2025-12-09
**Priority**: 🔴 High
**Status**: 📋 In Progress

---

## 📋 Summary

During V1.0 compliance audit against authoritative documentation, the AIAnalysis team identified several gaps. This document tracks cross-team gaps that require visibility or action from other teams.

---

## 🔴 Critical Gaps

### 1. API Group Mismatch

| Item | Authoritative (DD-CRD-001) | Current Code | Action |
|------|----------------------------|--------------|--------|
| API Group | `aianalysis.kubernaut.ai` | `aianalysis.kubernaut.io` | AIAnalysis team will fix |

**Reference**: `DD-CRD-001-api-group-domain-selection.md` (Nov 30, 2025, APPROVED)

**Impact**: Breaking change for any team already referencing `aianalysis.kubernaut.io`.

**Question for Teams**: Is anyone currently using `aianalysis.kubernaut.io` in production code?

---

### 2. Recovery Flow - HAPI Endpoint Clarification

**Current State**: AIAnalysis uses `/api/v1/incident/analyze` for both initial and recovery attempts.

**HAPI OpenAPI Spec Shows**:
- `/api/v1/incident/analyze` - Initial incidents (no recovery fields)
- `/api/v1/recovery/analyze` - Recovery attempts (has `is_recovery_attempt`, `previous_execution`)

**Question for HAPI Team**: Should AIAnalysis:
- **Option A**: Use `/api/v1/recovery/analyze` when `spec.IsRecoveryAttempt=true`?
- **Option B**: Continue using `/api/v1/incident/analyze` with recovery fields added?

**Reference**: `holmesgpt-api/api/openapi.json`

---

## 🟡 Medium Gaps (AIAnalysis Internal - No Action Required from Other Teams)

These are tracked for visibility only:

| Gap | Status | Owner |
|-----|--------|-------|
| Status fields not populated (TokensUsed, InvestigationID, etc.) | 🔄 In Progress | AIAnalysis |
| Conditions not implemented | 🔄 In Progress | AIAnalysis |
| Timeout annotation → spec field migration | ⏳ Pending RO response | AIAnalysis |

---

## 📝 Response Section

### HAPI Team Response (Recovery Endpoint)

**Date**: _____________
**Responder**: _____________

**Decision**: [ ] Option A (Use `/recovery/analyze`) / [ ] Option B (Use `/incident/analyze` with recovery fields)

**Notes**:
```
[HAPI team to fill in]
```

---

### Other Teams - API Group Impact

| Team | Uses `aianalysis.kubernaut.io`? | Response Date |
|------|--------------------------------|---------------|
| RO | [ ] Yes / [ ] No | _____________ |
| WE | [ ] Yes / [ ] No | _____________ |
| Notification | [ ] Yes / [ ] No | _____________ |
| Gateway | [ ] Yes / [ ] No | _____________ |

---

## 🎯 AIAnalysis Team Action Plan

| Task | Priority | Target Date | Status |
|------|----------|-------------|--------|
| Fix API Group to `.kubernaut.ai` | P0 | Day 11 | ⏳ Pending team responses |
| Implement recovery endpoint logic | P0 | Day 11 | ⏳ Pending HAPI response |
| Populate all status fields | P1 | Day 11 | 🔄 In Progress |
| Implement Conditions | P1 | Day 11 | 🔄 In Progress |
| Migrate timeout to spec field | P2 | Day 12 | ⏳ Pending RO response |

---

## 📚 Related Documents

- `DD-CRD-001-api-group-domain-selection.md` - API Group standard
- `REQUEST_RO_TIMEOUT_PASSTHROUGH_CLARIFICATION.md` - RO timeout question
- `holmesgpt-api/api/openapi.json` - HAPI OpenAPI spec

---

## 📝 Document History

| Date | Author | Change |
|------|--------|--------|
| 2025-12-09 | AIAnalysis Team | Initial gap identification |

