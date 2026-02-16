# HAPI Service Audit Findings

**Audit Date**: 2026-02-14  
**Scope**: HolmesGPT API (HAPI) implementation vs. authoritative BRs and DDs  
**Documents Audited**: DD-HAPI-016, DD-HAPI-017, BUSINESS_REQUIREMENTS.md, DD-AUDIT-003, DD-EM-002

---

## Executive Summary

| Category | Count |
|----------|-------|
| Implementation Gaps | 1 |
| Documentation Drift | 6 |
| **Total Findings** | **7** |

---

## 1. DD-HAPI-016: spec_drift Semantics Not Documented in Prompt Section

| Field | Value |
|-------|-------|
| **Type** | Documentation Drift |
| **Severity** | MEDIUM |
| **Document** | `docs/architecture/decisions/DD-HAPI-016-remediation-history-context.md` |
| **Section** | "Prompt Construction" (lines 291–358) |
| **Implementation** | `holmesgpt-api/src/extensions/remediation_history_prompt.py` |

**Finding**: DD-HAPI-016's "Prompt Construction" section does not describe how `spec_drift` entries are rendered. The implementation (DD-EM-002 v1.1) treats `assessmentReason == "spec_drift"` as **INCONCLUSIVE** with two variants:

- **Default**: "Assessment: INCONCLUSIVE (spec drift)" — suppresses health/metrics/score, adds "may still be viable under different conditions"
- **Causal chain**: "led to follow-up remediation (UID)" when `postRemediationSpecHash` matches a subsequent entry's `preRemediationSpecHash`

**Recommendation**: Add a subsection to DD-HAPI-016 "Prompt Construction" documenting spec_drift INCONCLUSIVE semantics and the two prompt variants, referencing DD-EM-002 v1.1.

---

## 2. DD-HAPI-016: Causal Chain Detection (Hash Linking) Not Documented

| Field | Value |
|-------|-------|
| **Type** | Documentation Drift |
| **Severity** | MEDIUM |
| **Document** | `docs/architecture/decisions/DD-HAPI-016-remediation-history-context.md` |
| **Section** | Entire document |
| **Implementation** | `holmesgpt-api/src/extensions/remediation_history_prompt.py` — `_detect_spec_drift_causal_chains()` |

**Finding**: The implementation detects when a `spec_drift` entry's `postRemediationSpecHash` matches a subsequent entry's `preRemediationSpecHash`, proving the spec_drift led to a follow-up remediation. This drives prompt semantic rewriting (causal chain variant). DD-HAPI-016 does not mention this hash-linking logic.

**Recommendation**: Document causal chain detection in DD-HAPI-016 (or explicitly reference DD-EM-002 v1.1 "Implications for HAPI Remediation History" and the HAPI prompt builder behavior).

---

## 3. DD-HAPI-016: _detect_declining_effectiveness Exclusion of spec_drift Not Documented

| Field | Value |
|-------|-------|
| **Type** | Documentation Drift |
| **Severity** | MEDIUM |
| **Document** | `docs/architecture/decisions/DD-HAPI-016-remediation-history-context.md` |
| **Section** | "Design Principles" / "Full Remediation Chain" |
| **Implementation** | `holmesgpt-api/src/extensions/remediation_history_prompt.py` — `_detect_declining_effectiveness()` |

**Finding**: The implementation excludes `spec_drift` entries from declining effectiveness trend detection because their 0.0 scores are unreliable and would create false declining trends. DD-HAPI-016 describes "Declining effectiveness (0.4 → 0.3 → 0.2 = diminishing returns)" but does not state that spec_drift entries are excluded.

**Recommendation**: Add a note in DD-HAPI-016 that spec_drift entries are excluded from declining effectiveness trend detection per DD-EM-002 v1.1.

---

## 4. DD-HAPI-016: Graceful Degradation (DS Unavailable) Not Documented

| Field | Value |
|-------|-------|
| **Type** | Documentation Drift |
| **Severity** | LOW |
| **Document** | `docs/architecture/decisions/DD-HAPI-016-remediation-history-context.md` |
| **Section** | Entire document |
| **Implementation** | `holmesgpt-api/src/clients/remediation_history_client.py` — `query_remediation_history()`, `fetch_remediation_history_for_request()` |

**Finding**: The implementation gracefully degrades when DS is unavailable: returns `None`, and the prompt is built without remediation history. DD-HAPI-016 does not mention this behavior.

**Recommendation**: Add a "Graceful Degradation" subsection stating that if DS is unavailable (connection error, 5xx, API not configured), HAPI proceeds without remediation history and the LLM prompt is unchanged.

---

## 5. DD-HAPI-017: Legacy search_workflow_catalog References Remain in Test Fixtures

| Field | Value |
|-------|-------|
| **Type** | Implementation Gap / Documentation Drift |
| **Severity** | LOW |
| **Document** | `docs/architecture/decisions/DD-HAPI-017-three-step-workflow-discovery-integration.md` |
| **Section** | BR-HAPI-017-006: "No references to search_workflow_catalog in any Python source file" |
| **Implementation** | `holmesgpt-api/src/clients/datastorage/test/test_llm_tool_call_payload.py`, `test_audit_event_request_event_data.py`, `test/services/mock-llm/src/server.py` |

**Finding**: DD-HAPI-017 BR-HAPI-017-006 requires "No references to search_workflow_catalog in any Python source file." The mock-llm server and some datastorage tests still reference `search_workflow_catalog` for legacy flow support.

**Recommendation**: Either (a) update BR-HAPI-017-006 to allow legacy references in mock/test fixtures for backward compatibility, or (b) remove/update those references to use the three-step tools.

---

## 6. BUSINESS_REQUIREMENTS.md: BR-HAPI-016 and BR-HAPI-017 Not Listed

| Field | Value |
|-------|-------|
| **Type** | Documentation Drift |
| **Severity** | MEDIUM |
| **Document** | `docs/services/stateless/holmesgpt-api/BUSINESS_REQUIREMENTS.md` |
| **Section** | Overview, Summary |
| **Implementation** | Remediation history and three-step workflow discovery are fully implemented |

**Finding**: BUSINESS_REQUIREMENTS.md does not list BR-HAPI-016 (Remediation History Context) or BR-HAPI-017-001 through 006 (Three-Step Workflow Discovery) in its summary. The document references BR-HAPI-016 in passing (e.g., "BR-HAPI-192 (Recovery Context Consumption): ✅ Complete") but does not have a dedicated section for remediation history or workflow discovery BRs.

**Recommendation**: Add BR-HAPI-016 and BR-HAPI-017-001 through 006 to the BUSINESS_REQUIREMENTS.md summary with implementation status (✅ Complete).

---

## 7. DD-AUDIT-003: HAPI No-Audit Status Consistent

| Field | Value |
|-------|-------|
| **Type** | Verification (No Gap) |
| **Severity** | N/A |
| **Document** | `docs/architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md` |
| **Implementation** | HAPI does not emit audit traces |

**Finding**: DD-AUDIT-003 states HolmesGPT API Service has NO audit traces (delegated to AI Analysis Controller). Implementation is consistent — HAPI does not generate audit events. No action required.

---

## 8. DD-EM-002: HAPI spec_drift Handling Aligned

| Field | Value |
|-------|-------|
| **Type** | Verification (No Gap) |
| **Severity** | N/A |
| **Document** | `docs/architecture/decisions/DD-EM-002-canonical-spec-hash.md` |
| **Section** | "Implications for HAPI Remediation History" |
| **Implementation** | `holmesgpt-api/src/extensions/remediation_history_prompt.py` |

**Finding**: DD-EM-002 v1.1 states that when HAPI builds remediation history context, a `spec_drift` assessment with score 0.0 provides clear context. The implementation renders spec_drift as INCONCLUSIVE with appropriate guidance. No action required.

---

## 9. Generated Python Client vs OpenAPI Spec

| Field | Value |
|-------|-------|
| **Type** | Verification (No Gap) |
| **Severity** | N/A |
| **Document** | `api/openapi/data-storage-v1.yaml` |
| **Implementation** | `holmesgpt-api/src/clients/datastorage/` |

**Finding**: The generated Python client matches the OpenAPI spec for:
- `GET /api/v1/remediation-history/context` — parameters (targetKind, targetName, targetNamespace, currentSpecHash, tier1Window, tier2Window) and response schema (RemediationHistoryContext with tier1, tier2, regressionDetected)
- RemediationHistoryEntry and RemediationHistorySummary include `assessmentReason` with enum including `spec_drift`
- Workflow discovery endpoints (list_available_actions, list_workflows, get_workflow_by_id)

No action required.

---

## Summary Table

| # | Finding | Severity | Type |
|---|---------|----------|------|
| 1 | spec_drift INCONCLUSIVE semantics not in DD-HAPI-016 | MEDIUM | Documentation Drift |
| 2 | Causal chain detection (hash linking) not in DD-HAPI-016 | MEDIUM | Documentation Drift |
| 3 | _detect_declining_effectiveness exclusion of spec_drift not in DD-HAPI-016 | MEDIUM | Documentation Drift |
| 4 | Graceful degradation (DS unavailable) not in DD-HAPI-016 | LOW | Documentation Drift |
| 5 | search_workflow_catalog references in test/mock fixtures | LOW | Implementation Gap |
| 6 | BR-HAPI-016 and BR-HAPI-017 not in BUSINESS_REQUIREMENTS.md | MEDIUM | Documentation Drift |
| 7–9 | DD-AUDIT-003, DD-EM-002, Python client — no gaps | N/A | Verification |

---

## Recommended Actions (Priority Order)

1. **MEDIUM**: Update DD-HAPI-016 to document spec_drift prompt semantics, causal chain detection, and declining effectiveness exclusion.
2. **MEDIUM**: Add BR-HAPI-016 and BR-HAPI-017 to BUSINESS_REQUIREMENTS.md.
3. **LOW**: Add graceful degradation subsection to DD-HAPI-016.
4. **LOW**: Resolve search_workflow_catalog references in test/mock fixtures per BR-HAPI-017-006 or update the BR.
