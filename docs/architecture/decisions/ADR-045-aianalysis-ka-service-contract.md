# ADR-045: AIAnalysis ↔ Kubernaut Agent (KA) Service Contract

**ADR ID**: ADR-045
**Status**: ✅ APPROVED (implemented, Go)
**Date**: December 2, 2025
**Decision Makers**: Kubernaut Agent (KA) Team, AIAnalysis Team

**Last Updated**: 2026-08-01 — Rewritten against the actual Go implementation
([Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806)). The original document described
a Python FastAPI service with a single synchronous `POST /api/v1/investigate` endpoint and
`oapi-codegen`-generated Go client. That endpoint was superseded by an **async, session-based**
API before this contract was fully implemented — the actual endpoint set, schemas, and generated
client differ substantially from what this document originally specified. See Version History for
what changed.

---

## Context

AIAnalysis is the **sole consumer** of Kubernaut Agent (KA)'s incident-analysis API. A clear,
authoritative API contract is needed to ensure:
- Correct request/response schemas between services
- Clear ownership of responsibilities
- No ambiguity in integration
- Consistent behavior across service boundaries

This ADR establishes the cross-service contract as an architectural decision affecting both
services.

---

## Decision

We establish a formal API contract between AIAnalysis and Kubernaut Agent (KA) with the following
specifications, reflecting the actual async/session-based Go implementation.

---

## API Specification

### OpenAPI Specification (Source of Truth)

**Source**: `internal/kubernautagent/api/openapi.json` (hand-maintained OpenAPI 3.1 spec; KA is a Go
service, not FastAPI/Pydantic-generated).

**Go Client Generation** (per [DD-KA-003](DD-KA-003-mandatory-openapi-client-usage.md)): the client
is generated with `ogen`, not `oapi-codegen`:

```bash
make generate-agentclient
```

Generated into `pkg/agentclient/` (`oas_client_gen.go`, `oas_schemas_gen.go`). The hand-written
wrapper `pkg/agentclient/client.go` provides the ergonomic Go API AIAnalysis actually calls
(`SubmitInvestigation`, session polling helpers) on top of the generated client — per DD-KA-003,
generated code is never hand-edited.

---

### Endpoints (Async, Session-Based)

KA does **not** expose a single synchronous investigate-and-respond endpoint. The actual contract is
submit-then-poll:

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/incident/analyze` | Submit an investigation request. Returns `202 Accepted` with `{"session_id": "<uuid>"}` immediately; investigation runs as a background task. |
| `GET` | `/api/v1/incident/session/{session_id}` | Poll session status (`pending`/`running`/`completed`/`failed`), including `acting_user`/`acting_user_groups` for interactive-mode attribution. |
| `GET` | `/api/v1/incident/session/{session_id}/result` | Retrieve the completed result. Returns `409` if the session is still in progress, `404` if unknown. |
| `POST` | `/api/v1/incident/session/{session_id}/cancel` | Cancel an in-progress investigation. |
| `GET` | `/api/v1/incident/session/{session_id}/snapshot` | Retrieve a point-in-time snapshot of investigation progress. |
| `GET` | `/api/v1/incident/session/{session_id}/stream` | Stream investigation progress (interactive mode). |

**Base URL**: in-cluster service DNS name for the KA deployment (not a fixed `kubernaut-agent:8080`
constant — resolved via the AIAnalysis controller's configured KA endpoint).

**Authentication**: Bearer token, validated by KA middleware per
[DD-AUTH-014](DD-AUTH-014-middleware-based-sar-authentication.md) (not "K8s network policy, optional
mTLS" as originally specified).

**Called by**: `SubmitInvestigation()` in AIAnalysis's investigating-phase handler
(`pkg/aianalysis/handlers/investigating.go`), which submits then polls at
`h.sessionPollInterval` until `completed`/`failed`.

**Business/Design authority**: `BR-KA-002` (analyze endpoint), `BR-AA-KA-064.1`/`.3` (async
submit/result), `DD-AUTH-006` (user attribution for LLM cost tracking) — cited directly in the
OpenAPI spec's endpoint descriptions.

---

### Request Schema (`IncidentRequest`)

Full schema: `internal/kubernautagent/api/openapi.json` → `components.schemas.IncidentRequest` (the
authoritative source — this document does not duplicate the full YAML to avoid drift). Key fields
carried over from the original design remain conceptually accurate:

- `signalContext` (required): `signalId`, `signalName`, `severity`, `timestamp`, `businessPriority`,
  `targetResource` (`apiVersion`/`kind`/`namespace`/`name`), `enrichmentResults`
  (`kubernetesContext`, `detectedLabels`, `ownerChain`, `customLabels`, `enrichmentQuality`)
- `recoveryContext` (optional): `isRecovery`, `previousExecutionId`, `naturalLanguageSummary`

**`DetectedLabels`** remains authoritatively defined in `pkg/shared/types/enrichment.go` (Go) — the
original document's Python/Pydantic mirror (`kubernaut-agent/src/models/incident_models.py`) no
longer exists; KA is Go end-to-end.

---

### Response Schema (`IncidentResponse`, via `GET .../result`)

Full schema: `internal/kubernautagent/api/openapi.json` → `components.schemas.IncidentResponse`.
Fields (snake_case, unlike the original CamelCase `InvestigateResponse` proposal):

| Field | Purpose |
|-------|---------|
| `incident_id` | Echo of the request's incident identifier |
| `analysis` | Natural-language analysis from the LLM |
| `root_cause_analysis` | Structured RCA object (`summary`, `severity`, `contributing_factors`) |
| `selected_workflow` | Selected workflow object (`workflow_id`, `execution_bundle`, `confidence`, `parameters`), or `null` |
| `confidence` | Overall analysis confidence (LLM self-reported — see [DD-KA-004](DD-KA-004-v1-confidence-scoring.md)) |
| `needs_human_review` | `true` when AI could not produce a reliable result (see [BR-KA-197](../../requirements/BR-KA-197-needs-human-review-field.md)) |
| `human_review_reason` | Structured enum reason for `needs_human_review` (see [BR-KA-197](../../requirements/BR-KA-197-needs-human-review-field.md)) |
| `is_actionable` | LLM's assessment of whether the signal warrants action at all |
| `warnings` | Non-fatal warnings (OwnerChain issues, low confidence, etc.) |
| `alternative_workflows` | Workflows considered but not selected — **audit/context only, never executed** |
| `validation_attempts_history` | Record of parameter-validation retry attempts (see [BR-KA-191](../../requirements/BR-KA-191-workflow-parameter-validation.md)) |

**Unchanged design principle from the original ADR**: only `selected_workflow` is executed by
RemediationOrchestrator. `alternative_workflows` remains informational/audit-only, never a fallback
execution queue.

---

### Error Response Schema

**Format**: RFC 7807 Problem Details (per [DD-004](DD-004-RFC7807-ERROR-RESPONSES.md)) — this part
of the original design is accurate and unchanged. `Content-Type: application/problem+json`, with
`type`/`title`/`status`/`detail`/`instance` and a KA-specific `request_id` extension field.

---

## Responsibility Matrix

| Aspect | Kubernaut Agent (KA) | AIAnalysis |
|--------|----------------------|------------|
| RCA Analysis | ✅ Performs | Consumes result |
| Workflow Selection | ✅ Selects best match | Consumes result |
| Confidence Scoring | ✅ Self-reported by LLM (per [DD-KA-004](DD-KA-004-v1-confidence-scoring.md)) | Uses for Rego policy |
| Approval Decision | ❌ **Not responsible** | ✅ Determines via Rego |
| Parameter Formatting | ✅ `UPPER_SNAKE_CASE` | Passthrough to RO |
| Retry Logic (KA call failures) | ❌ **Not responsible** | ✅ **AIAnalysis's own responsibility** — classified and retried by `ErrorClassifier` (`pkg/aianalysis/handlers/error_classifier.go`, BR-AI-009/BR-AI-010), not delegated to RO as originally specified |
| Audit Trail Storage | ✅ Internal only | Captures response in CRD |

**Correction from original design**: the original "Retry Logic → RO decides (per BR-HAPI-193)" claim
does not match the implementation. RO has no role in AIAnalysis's retry decisions for KA-call
failures — AIAnalysis owns this itself end-to-end (see Timeout and Retry Guidance below). BR-HAPI-193
was about execution outcome reporting (now [BR-KA-193](../../requirements/BR-KA-193-execution-outcome-reporting.md)), unrelated to retry ownership.

---

## Timeout and Retry Guidance (Implemented)

Retry is handled entirely within AIAnalysis's `InvestigatingHandler`, using `ErrorClassifier`
(`pkg/aianalysis/handlers/error_classifier.go`) and the shared backoff library
([DD-SHARED-001](DD-SHARED-001-shared-backoff-library.md)) — not the original document's proposed
"3 retries, exponential 1s/2s/4s, RO decides" scheme:

| Aspect | Actual Value |
|--------|-------------|
| Max Retries | 5 (`pkg/aianalysis/handlers/constants.go: MaxRetries = 5`) |
| Backoff | Exponential with jitter: base 1s, multiplier 2.0, max 5 minutes, ±10% jitter |
| Retry mechanism | Standard controller-runtime `ctrl.Result{RequeueAfter: backoffDuration}` — not an in-process blocking retry loop |
| Retry classification | By HTTP status: 401/403/404/422/400 → non-retryable, alert; 429/5xx/network/timeout → retryable, backoff; unknown → retryable, alert |
| On max-retries exceeded | `AIAnalysis.Status.Phase = Failed`, `Reason = APIError`, `SubReason = MaxRetriesExceeded` |
| Escalation to operator | RemediationOrchestrator's [BR-ORCH-036](../../requirements/BR-ORCH-036-manual-review-notification.md) v3.0 detects the `APIError`/`MaxRetriesExceeded`/`TransientError`/`PermanentError` reasons on a failed AIAnalysis and creates a `NotificationRequest` (`type=manual-review`) — **not** the `AIApprovalRequest` CRD originally proposed (see Known Corrections) |

No circuit breaker exists on this path (confirmed by code search — no `gobreaker`/circuit-breaker
type wraps the AIAnalysis→KA call). Retry-with-backoff plus a bounded max-attempt count is the sole
resilience mechanism today.

---

## Known Corrections vs. Original Design

1. **No single synchronous investigate endpoint.** Replaced by the session-based async API
   (submit → poll → result) documented above, before this contract was ever implemented as
   originally specified.
2. **No `AIApprovalRequest` CRD.** It was renamed to `RemediationApprovalRequest`
   ([ADR-040](ADR-040-remediation-approval-request-architecture.md)) with its own controller and
   admission webhook (`pkg/remediationapprovalrequest/`, `pkg/authwebhook/`) — and manual-review
   escalation for KA-call failures actually flows through RO's `NotificationRequest`
   (`type=manual-review`), not a dedicated approval CRD created directly by AIAnalysis.
3. **`historicalSuccessRate` field removed entirely** (not merely "unused in V1.0" as originally
   noted) — see [DD-KA-004](DD-KA-004-v1-confidence-scoring.md).
4. **Retry ownership corrected**: AIAnalysis owns retry for KA-call failures; RO has no role in this
   decision (see Responsibility Matrix correction above).
5. **`oapi-codegen` → `ogen`** for Go client generation, per [DD-KA-003](DD-KA-003-mandatory-openapi-client-usage.md).

---

## Consequences

### Positive
- ✅ Clear contract between services
- ✅ Single source of truth for integration (`internal/kubernautagent/api/openapi.json`)
- ✅ RFC 7807 error handling for consistency
- ✅ OpenAPI spec enables Go code generation (`ogen`)
- ✅ Async/session-based design decouples AIAnalysis's reconcile loop from long-running LLM calls

### Negative
- ⚠️ Contract changes require coordination between teams
- ⚠️ Session-based polling adds reconciliation complexity vs. a synchronous call

### Mitigation
- Version the API (`/api/v1/`)
- OpenAPI spec is the enforced source of truth; generated client code is never hand-edited (DD-KA-003)

---

## Related Documents

- **ADR-031**: OpenAPI Specification Standard
- **DD-004**: RFC 7807 Error Responses
- **[DD-KA-003](DD-KA-003-mandatory-openapi-client-usage.md)**: Mandatory OpenAPI Client Usage (`ogen` generation)
- **[DD-KA-004](DD-KA-004-v1-confidence-scoring.md)**: V1.0 Confidence Scoring Methodology
- **[ADR-040](ADR-040-remediation-approval-request-architecture.md)**: `RemediationApprovalRequest` architecture (supersedes the `AIApprovalRequest` this document originally referenced)
- **[BR-ORCH-036](../../requirements/BR-ORCH-036-manual-review-notification.md)**: Manual Review & Escalation Notification (actual escalation path for KA-call failures)
- **DD-RECOVERY-002**: Direct AIAnalysis recovery flow
- **DD-RECOVERY-003**: Recovery prompt design with DetectedLabels
- **BR-KA-192**: Recovery Context Consumption (no standalone doc; tracked for the KA service doc set, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806) PR4)
- **[BR-KA-193](../../requirements/BR-KA-193-execution-outcome-reporting.md)**: Execution Outcome Reporting
- `pkg/shared/types/enrichment.go` — authoritative `DetectedLabels` schema (Go)
- `internal/kubernautagent/api/openapi.json` — authoritative API contract (source of truth)

---

## Addendum: MCP `DiscoveredWorkflow` Parameters (#1169)

The MCP `DiscoveredWorkflow` struct (used in `discover_workflows` responses) diverges from
the REST `AlternativeWorkflow` struct by including a `parameters` field (`map[string]interface{}`).
This enables the MCP client (AF) to display per-workflow parameter previews before selection.

The REST `AlternativeWorkflow` (used in the KA→AA HTTP API) was designed for audit/context only
(v1.2) and intentionally excluded `parameters`. The MCP DTO adds them because:

1. AF needs to display parameter previews for operator decision-making
2. `select_workflow` needs to merge the selected workflow's parameters into the final result
3. Parameter values come from the LLM's Phase 3 structured output, not from the catalog

See [docs/mcp/discover-workflows-contract.md](../../mcp/discover-workflows-contract.md) for the
authoritative MCP tool contract.

---

## Version History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 2.0 | 2026-08-01 | Issue #1806 | Full rewrite against Go implementation: replaced the fictional synchronous `POST /api/v1/investigate` with the actual async session-based API (`analyze`/`session/{id}`/`result`/`cancel`/`snapshot`/`stream`), corrected the request/response schemas to match `internal/kubernautagent/api/openapi.json`, corrected retry ownership (AIAnalysis, not RO), corrected the superseded `AIApprovalRequest` → `RemediationApprovalRequest` reference, corrected the Go client generator (`ogen`, not `oapi-codegen`), removed all HAPI/Python terminology. |
| 1.3 | 2026-05-17 | KA Team | Addendum: MCP `DiscoveredWorkflow` includes `parameters` (#1169) |
| 1.2 | 2025-12-05 | HAPI Team | Implemented `alternativeWorkflows[]` for audit/context (NOT for execution) |
| 1.1 | 2025-12-02 | HAPI Team | Added `targetInOwnerChain` and `warnings[]` fields per AIAnalysis request |
| 1.0 | 2025-12-02 | HAPI Team | Initial creation (converted from DD-HAPI-004); described a proposed synchronous API never implemented as specified |
