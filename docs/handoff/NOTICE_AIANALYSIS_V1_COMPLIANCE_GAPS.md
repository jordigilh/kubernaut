# Notice: AIAnalysis V1.0 Compliance Gaps

**From**: AIAnalysis Team
**To**: All Teams (Cross-Team Visibility)
**Date**: 2025-12-09
**Priority**: 🔴 High
**Status**: ✅ **TRIAGED - All Responses Received - Ready for Day 11 Implementation**

---

## 📋 Summary

During V1.0 compliance audit against authoritative documentation, the AIAnalysis team identified several gaps. This document tracks cross-team gaps that require visibility or action from other teams.

---

## 🔴 Critical Gaps

### 1. API Group Mismatch

| Item | Authoritative (DD-CRD-001) | Current Code | Action |
|------|----------------------------|--------------|--------|
| API Group | `kubernaut.ai` | `aianalysis.kubernaut.io` | AIAnalysis team will fix |

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
| Status fields not populated (InvestigationID, etc.) | ✅ Partial | AIAnalysis |
| ~~TokensUsed~~ | ✅ **REMOVED** - Out of scope (HAPI owns LLM cost observability) | AIAnalysis |
| Conditions not implemented | 🔄 In Progress | AIAnalysis |
| Timeout annotation → spec field migration | ✅ **FIXED** | AIAnalysis |

---

## 📝 Response Section

### HAPI Team Response (Recovery Endpoint)

**Date**: December 9, 2025
**Responder**: HAPI Team

**Decision**: [✅] Option A (Use `/recovery/analyze`) / [ ] Option B (Use `/incident/analyze` with recovery fields)

**Notes**:
```
✅ OPTION A IS CORRECT

Authoritative Reference: holmesgpt-api/api/openapi.json

Rationale:
1. The endpoints are INTENTIONALLY separate with different schemas
2. RecoveryRequest has fields IncidentRequest lacks:
   - is_recovery_attempt (bool)
   - recovery_attempt_number (int)
   - previous_execution (struct with failure context, original RCA, selected workflow)
3. HAPI uses these fields to construct DIFFERENT prompts:
   - incident.py → Initial RCA prompt
   - recovery.py → Recovery strategy prompt with failure context
4. Using /incident/analyze for recovery LOSES critical context the LLM needs

Implementation Guidance for AIAnalysis:
- Create InvestigateRecovery() method → POST /api/v1/recovery/analyze
- Create RecoveryRequest struct with fields from OpenAPI spec
- Call InvestigateRecovery() when spec.IsRecoveryAttempt=true
- Call Investigate() when spec.IsRecoveryAttempt=false (existing behavior)

API Contract Reference:
- RecoveryRequest schema: holmesgpt-api/api/openapi.json lines 1130-1343
- RecoveryResponse schema: holmesgpt-api/api/openapi.json (RecoveryResponse)
```

---

### Other Teams - API Group Impact

| Team | Uses `aianalysis.kubernaut.io`? | Response Date | Notes |
|------|--------------------------------|---------------|-------|
| HAPI | [✅] No | Dec 9, 2025 | HAPI is stateless, doesn't consume CRDs |
| RO | [✅] Yes | _____________ | Found in `test/e2e/remediationorchestrator/suite_test.go` |
| WE | [ ] Yes / [ ] No | _____________ | |
| Notification | [ ] Yes / [ ] No | _____________ | |
| Gateway | [ ] Yes / [ ] No | _____________ | |
| **SignalProcessing** | [✅] **No** | Dec 9, 2025 | No direct AIAnalysis CRD dependency (downstream via RO) |

> **📋 HAPI Note**: HAPI does not consume any CRDs (per DD-HOLMESGPT-012 Minimal Internal Service Architecture). The API group migration has NO impact on HAPI.

---

## 🎯 AIAnalysis Team Action Plan

| Task | Priority | Target Date | Status | Response |
|------|----------|-------------|--------|----------|
| Fix API Group to `.kubernaut.ai` | P0 | Day 11 | ✅ **UNBLOCKED** | Only RO E2E tests affected - coordinated migration |
| **Fix HAPI Contract Mismatch** | P0 | Day 11 | ✅ **UNBLOCKED** | See `NOTICE_AIANALYSIS_HAPI_CONTRACT_MISMATCH.md` |
| Implement recovery endpoint logic | P0 | Day 11 | ✅ **UNBLOCKED** | HAPI confirmed: Use `/api/v1/recovery/analyze` |
| Populate all status fields | P1 | Day 11 | 🔄 In Progress | AA internal |
| Implement Conditions | P1 | Day 11 | 🔄 In Progress | AA internal |
| Migrate timeout to spec field | P1 | Day 11 | ✅ **COMPLETE** | Added `spec.TimeoutConfig` to CRD |
| Update RO passthrough | P2 | Day 12 | ✅ **UNBLOCKED** | RO can proceed (AA spec change done) |

---

## 📚 Related Documents

- `DD-CRD-001-api-group-domain-selection.md` - API Group standard
- `REQUEST_RO_TIMEOUT_PASSTHROUGH_CLARIFICATION.md` - RO timeout question
- `holmesgpt-api/api/openapi.json` - HAPI OpenAPI spec (AUTHORITATIVE)
- **[NOTICE_AIANALYSIS_HAPI_CONTRACT_MISMATCH.md](./NOTICE_AIANALYSIS_HAPI_CONTRACT_MISMATCH.md)** - 🔴 **NEW** - Detailed contract gap analysis from HAPI team

---

## 📝 Document History

| Date | Author | Change |
|------|--------|--------|
| 2025-12-09 | AIAnalysis Team | Initial gap identification |
| 2025-12-09 | HAPI Team | Responded to Recovery Endpoint question (Option A), confirmed no API Group impact |
| 2025-12-09 | SignalProcessing Team | Confirmed no AIAnalysis CRD dependency (downstream via RO only) |
| 2025-12-09 | HAPI Team | Created `NOTICE_AIANALYSIS_HAPI_CONTRACT_MISMATCH.md` - detailed contract gap analysis |
| 2025-12-09 | RO Team | Responded to Timeout Passthrough (Option A approved, conditional on AA spec change) |
| 2025-12-09 | AIAnalysis Team | **TRIAGE COMPLETE** - All responses received, Day 11 plan ready |
| 2025-12-09 | HAPI Team | Answered AA clarification questions: `incident_id=metadata.name`, `remediation_id=spec.RemediationID` |

