# Bidirectional Triage Report: AI Analysis & HolmesGPT API Services

**Date**: 2026-03-04  
**Scope**: AI Analysis Service, HolmesGPT API Service  
**Direction 1**: Code → Docs (things in code not documented or documented incorrectly)  
**Direction 2**: Docs → Code (things in docs/BRs planned for v1.0 but not implemented or not wired)

---

## AI Analysis Service

### Direction 1: Code → Docs

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| 1 | GAP-IN-DOCS | HIGH | `kubernaut-docs/docs/api-reference/crds.md` | AIAnalysis Status schema documents `analysisResult` as a single field; actual CRD has separate fields: `rootCause`, `rootCauseAnalysis`, `selectedWorkflow`, `alternativeWorkflows`, `postRCAContext`, `investigationSession`, `needsHumanReview`, `humanReviewReason`, `approvalRequired`, `approvalReason`, `approvalContext`, `validationAttemptsHistory`, etc. | crds.md:113-116 vs `api/aianalysis/v1alpha1/aianalysis_types.go` lines 271-301 |
| 2 | INCONSISTENCY | HIGH | `kubernaut-docs/docs/api-reference/crds.md` | Docs state API Group `aianalysis.kubernaut.io/v1alpha1`; actual CRD uses `kubernaut.ai` group. | crds.md:96 vs `config/crd/bases/kubernaut.ai_aianalyses.yaml` spec.group: kubernaut.ai |
| 3 | GAP-IN-DOCS | MEDIUM | `kubernaut-docs/docs/architecture/ai-analysis.md` | `AIAnalysisSpec.timeoutConfig` (InvestigatingTimeout, AnalyzingTimeout) is not documented. Passed from RR by RO per REQUEST_RO_TIMEOUT_PASSTHROUGH_CLARIFICATION.md. | `api/aianalysis/v1alpha1/aianalysis_types.go` lines 76-88 |
| 4 | GAP-IN-DOCS | MEDIUM | `kubernaut-docs/docs/architecture/ai-analysis.md` | Session poll interval (15s default, configurable 1s–5m) not documented. BR-AA-HAPI-064.8 recommends "10s, 20s, 30s capped" but code uses constant interval per design. | `pkg/aianalysis/handlers/constants.go` DefaultSessionPollInterval=15s; `internal/config/aianalysis/config.go` SessionPollInterval validation |
| 5 | GAP-IN-DOCS | LOW | `kubernaut-docs/docs/user-guide/approval.md` | Investigation threshold (0.7) is hardcoded in `response_processor.go` with TODO for V1.1 configurable per BR-HAPI-198. Docs correctly say "Not yet (V1.1)" but do not reference the code location. | `pkg/aianalysis/handlers/response_processor.go` lines 114, 338 |
| 6 | GAP-IN-DOCS | LOW | `kubernaut-docs/docs/api-reference/crds.md` | AIAnalysis Spec omits `timeoutConfig`; Status omits `postRCAContext`, `investigationSession` structure (ID, Generation, PollCount, LastPolled, CreatedAt), `validationAttemptsHistory`, `needsHumanReview`, `humanReviewReason`. | crds.md:100-116 vs `aianalysis_types.go` full schema |

### Direction 2: Docs → Code

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| 7 | GAP-IN-CODE | LOW | BR-AA-HAPI-064.8 | BR recommends "backoff: 10s, 20s, 30s (capped at 30s)"; implementation uses constant 15s interval. Design decision (polling is normal async behavior, not error recovery) supersedes BR; BR should be updated. | `docs/requirements/BR-AA-HAPI-064-session-based-pull-design.md` line 69 vs `constants.go` DefaultSessionPollInterval |
| 8 | GAP-IN-CODE | LOW | BR-AA-HAPI-064.4 | BR-064.4 struct omits `PollCount`; CRD and code include it for observability. BR is incomplete. | BR-064.4 vs `aianalysis_types.go` InvestigationSession.PollCount |
| 9 | GAP-IN-CODE | MEDIUM | BR-AA-HAPI-064.9 | BR-064.9: "Same async pattern MUST apply to recovery investigations (`/api/v1/recovery/analyze`)". AA controller only uses incident flow; recovery is separate (RemediationProcessor). Recovery session endpoints may exist in HAPI but AA does not call them. | BR line 72; `holmesgpt-api` has recovery endpoint; `cmd/aianalysis` only wires incident flow |
| 10 | GAP-IN-CODE | LOW | BR-AA-HAPI-064.10 | BR-064.10: "10-minute hardcoded timeout workaround MUST be removed". Config uses 180s (3 min) default; no 10m timeout found in current code. May already be resolved. | `internal/config/aianalysis/config.go` HolmesGPT.Timeout: 180*time.Second |

---

## HolmesGPT API Service

### Direction 1: Code → Docs

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| 11 | INCONSISTENCY | LOW | `kubernaut-docs/docs/api-reference/holmesgpt-api.md` | Doc shows session status response with `created_at`; actual `SessionStatus` in Go client has `status`, `error`, `progress` (no `created_at`). Python session manager may return different shape. | holmesgpt-api.md:74-79 vs `pkg/holmesgpt/client/holmesgpt.go` SessionStatus struct |
| 12 | GAP-IN-DOCS | MEDIUM | `kubernaut-docs/docs/api-reference/holmesgpt-api.md` | Session endpoints (`/api/v1/incident/session/{id}`, `/api/v1/incident/session/{id}/result`) are not in HAPI OpenAPI spec; Go client uses raw HTTP. Doc should note session endpoints are outside current OpenAPI coverage. | `pkg/holmesgpt/client/holmesgpt.go` lines 76-77, 244-251 |
| 13 | GAP-IN-DOCS | LOW | `kubernaut-docs/docs/architecture/ai-analysis.md` | Workflow selection: HAPI calls DataStorage via three-step discovery (list_available_actions, list_workflows_by_action_type, get_workflow_by_id). Doc says "Queries the workflow catalog" but does not name the protocol or DataStorage. | ai-analysis.md:68-72 vs `holmesgpt-api/src/toolsets/workflow_discovery.py` |
| 14 | GAP-IN-DOCS | LOW | `kubernaut-docs/docs/api-reference/holmesgpt-api.md` | 404 and 409 responses for session/result endpoint documented; 404 triggers session regeneration (BR-064.5) in AA controller. Doc mentions "sessions lost" but does not link to AA regeneration behavior. | holmesgpt-api.md:94-95, 109 |

### Direction 2: Docs → Code

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| 15 | GAP-IN-CODE | MEDIUM | OpenAPI spec | Session endpoints (`GET /api/v1/incident/session/{id}`, `GET /api/v1/incident/session/{id}/result`) exist in HAPI but are not in `holmesgpt-api/api/openapi.json`. Go client uses raw HTTP. | `holmesgpt-api/api/openapi.json` has `/api/v1/incident/analyze` only; session paths in `endpoint.py` lines 142-161 |
| 16 | GAP-IN-CODE | LOW | BR-AA-HAPI-064 | Acceptance criteria: "HAPI exposes async session endpoints for both incident and recovery". Incident has session flow; recovery (`/api/v1/recovery/analyze`) may still be sync. | BR-064 Acceptance Criteria vs `holmesgpt-api/src/main.py` router includes |

---

## Cross-Cutting: Confidence Thresholds

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| 17 | INCONSISTENCY | MEDIUM | Multiple | Two thresholds: (1) Investigation 0.7 in `response_processor.go` — hardcoded, rejects low-confidence workflow selection; (2) Approval 0.8 in Rego — configurable via `input.confidence_threshold` and Helm. Docs (approval.md, ai-analysis.md) describe both correctly; code comment in response_processor.go says "TODO V1.1: Make configurable per BR-HAPI-198". | `response_processor.go` 114, 338; `approval.rego` default 0.8; `analyzing.go` WithConfidenceThreshold |
| 18 | GAP-IN-DOCS | LOW | `kubernaut-docs` | DetectedLabels: Rego uses `input.detected_labels["stateful"]` (snake_case) and `input.failed_detections` with "gitOpsManaged", "pdbProtected" (camelCase). Analyzing handler builds map with snake_case keys. Convention not documented. | `config/rego/aianalysis/approval.rego`; `pkg/aianalysis/handlers/analyzing.go` detectedLabelsToMap |

---

## Cross-Cutting: ADR-056 / DetectedLabels

| # | Category | Severity | File | Detail | Evidence |
|---|----------|----------|------|--------|----------|
| 19 | DOCUMENTED | — | — | DetectedLabels computed by HAPI post-RCA (get_resource_context, resource_context.py), returned in IncidentResponse, stored in PostRCAContext. Flow is implemented and matches ADR-056. | `response_processor.go` populatePostRCAContext; `analyzing.go` resolveDetectedLabels |
| 20 | GAP-IN-DOCS | LOW | `kubernaut-docs` | Workflow discovery: HAPI (not AA) calls DataStorage. Doc says "HAPI queries workflow catalog" but does not specify it uses WorkflowDiscoveryAPI (three-step protocol) against DataStorage. | ai-analysis.md:68-72; `workflow_discovery.py` |

---

## Summary by Severity

| Severity | Count |
|----------|-------|
| HIGH | 2 |
| MEDIUM | 6 |
| LOW | 12 |

## Recommended Actions

1. **HIGH**: Update crds.md AIAnalysis section — fix API group to `kubernaut.ai/v1alpha1`, replace `analysisResult` with actual status fields.
2. **MEDIUM**: Add session endpoints to HAPI OpenAPI spec and regenerate Go client; or document that session endpoints are intentionally outside the spec.
3. **MEDIUM**: Document `timeoutConfig` in AIAnalysis spec and `investigationSession` structure in status.
4. **MEDIUM**: Clarify BR-064.9 (recovery async) — either implement or document as out-of-scope for AA controller.
5. **LOW**: Update BR-AA-HAPI-064.8 to reflect constant poll interval design; add PollCount to BR-064.4 struct.
