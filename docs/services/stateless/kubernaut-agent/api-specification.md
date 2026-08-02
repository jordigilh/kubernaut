# Kubernaut Agent - API Specification

**Version**: 1.5.0 (per `internal/kubernautagent/api/openapi.json`)
**Last Updated**: 2026-08-02
**Status**: ✅ Current

---

## Purpose

KA exposes an **async, session-based** investigation API. There is no synchronous "submit and get
the answer back" endpoint — a client submits an investigation, receives a `session_id`, and polls
for status/results. The authoritative source of truth is
[`internal/kubernautagent/api/openapi.json`](../../../../internal/kubernautagent/api/openapi.json);
this document is a hand-written summary of it, kept for discoverability.

**Sole client**: the AIAnalysis controller (`SubmitInvestigation()`). No other production service
calls KA's HTTP API directly.

---

## Authentication

Requests must carry `Authorization: Bearer <ServiceAccount JWT>`. Authentication (Kubernetes
`TokenReview`) and authorization (`SubjectAccessReview`) are implemented as **application-level Go
middleware** using dependency-injected interfaces — see
[DD-AUTH-014](../../../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md).

> **⚠️ Known spec staleness**: `openapi.json`'s `securitySchemes.oauthProxyAuth` description still
> describes an `oauth-proxy` **sidecar** design (citing "DD-AUTH-006" and "HAPI"), which predates
> and is superseded by DD-AUTH-014's middleware-based approach. This is a known documentation gap
> tracked by [Issue #1837](https://github.com/jordigilh/kubernaut/issues/1837); not corrected here
> since it requires a Go-source-adjacent OpenAPI spec edit + client regeneration, out of scope for
> this doc-only PR.

---

## Endpoints

### 1. `POST /api/v1/incident/analyze` — Submit Investigation

Submits a new investigation. Returns immediately; the investigation runs as a background task.

**Request body** (`IncidentRequest`) — key fields:

| Field | Type | Required | Notes |
|---|---|---|---|
| `incident_id` | string | ✅ | |
| `remediation_id` | string | ✅ | Correlation ID — propagated through to audit events (see [observability-logging.md](./observability-logging.md)) |
| `signal_name`, `signal_source` | string | ✅ | |
| `severity` | enum | ✅ | `critical` \| `high` \| `warning` \| `info` \| `unknown` |
| `resource_namespace`, `resource_kind`, `resource_name` | string | ✅ | Original signal target (may differ from KA's determined remediation target — see [DD-KA-006](../../../architecture/decisions/DD-KA-006-remediation-target-in-rca.md)) |
| `error_message` | string | ✅ | |
| `environment`, `priority`, `risk_tolerance`, `business_category` | string | ✅ | |
| `cluster_name` | string | ✅ | |
| `signal_labels`, `signal_annotations` | map[string]string | optional | |
| `enrichment_results` | object | optional | Remediation history (Tier 1/Tier 2), see [DD-KA-016](../../../architecture/decisions/DD-KA-016-remediation-history-context.md) |
| `signal_mode` | enum | optional | `reactive` \| `proactive` |
| `interactive` | bool | optional | Enables Interactive MCP Mode for this investigation |

**Response** `202 Accepted` (`AnalyzeAccepted`): `{ "session_id": "<uuid>" }`

**Error responses**: `400`/`401`/`403`/`422`/`500`, all RFC 7807 Problem Details
(`application/problem+json`, schema `HTTPError`).

---

### 2. `GET /api/v1/incident/session/{session_id}` — Session Status

Polls the current status of an in-flight or completed investigation.

**Response** `200` (`SessionStatus`):

| Field | Type | Notes |
|---|---|---|
| `session_id` | string | |
| `status` | string | e.g. `running`, `completed`, `failed`, `cancelled` |
| `error` | string | optional, populated on failure |
| `acting_user`, `acting_user_groups` | string / array | populated for Interactive MCP Mode sessions (operator identity) |

**Errors**: `404` (unknown session), `422` (validation), `500`.

---

### 3. `GET /api/v1/incident/session/{session_id}/result` — Final Result

Returns the completed investigation's result. Returns `409` if the session hasn't finished yet.

**Response** `200` (`IncidentResponse`):

| Field | Type | Notes |
|---|---|---|
| `incident_id` | string | |
| `analysis` | string | Human-readable RCA narrative |
| `root_cause_analysis` | object | Structured RCA payload |
| `selected_workflow` | object \| null | The workflow KA selected from the catalog, if any |
| `confidence` | number | LLM self-reported confidence (0.0-1.0) — see [Known Gaps](./overview.md#known-gaps--deviations) |
| `needs_human_review` | bool | Derived from confidence bands, see [BR-KA-197](../../../requirements/BR-KA-197-needs-human-review-field.md) |
| `human_review_reason` | enum \| null | Why human review is needed, if applicable |
| `is_actionable` | bool \| null | |
| `warnings` | array | e.g. Tier 2 ineffective-remediation-history warnings |
| `alternative_workflows` | array | Other workflows the LLM considered |
| `validation_attempts_history` | array | Self-correction attempts, see [BR-KA-191](../../../requirements/BR-KA-191-workflow-parameter-validation.md) |
| `detected_labels` | object \| null | Infrastructure characteristics, see [DD-KA-018](../../../architecture/decisions/DD-KA-018-detected-labels-detection-specification.md) |
| `alignment_verdict` | object \| null | Shadow Agent's security verdict for this session, see [shadow-agent-configuration.md](./shadow-agent-configuration.md) |
| `timestamp` | string | |

**Errors**: `404` (unknown session), `409` (not yet complete), `422`, `500`.

---

### 4. `GET /api/v1/incident/session/{session_id}/snapshot` — In-Progress Snapshot

Returns a point-in-time snapshot of an in-flight investigation (metadata, token usage so far, RCA
summary if available) without waiting for completion. Useful for the Interactive MCP Mode UI and
for cancellation-time diagnostics.

**Response** `200` (`SessionSnapshot`): `session_id`, `status`, `metadata`, `created_at`, plus
optional `error`, `cancelled_phase`, `cancelled_at_turn`, `rca_summary`, `total_prompt_tokens`,
`total_completion_tokens`.

**Errors**: `404`, `409`.

---

### 5. `GET /api/v1/incident/session/{session_id}/stream` — Streaming Updates

Server-Sent Events (`text/event-stream`) stream of investigation progress. Used by Interactive MCP
Mode clients for live updates.

**Errors**: `404`.

---

### 6. `POST /api/v1/incident/session/{session_id}/cancel` — Cancel Investigation

Cancels an in-flight investigation.

**Response** `200` (`CancelSessionResponse`): `{ "session_id": "...", "status": "..." }`

**Errors**: `404` (unknown session), `409` (already terminal).

---

## Related Documentation

- [overview.md](./overview.md) — architecture and the components these endpoints front
- [integration-points.md](./integration-points.md) — who calls this API and what KA calls in turn
- [observability-logging.md](./observability-logging.md) — correlation ID propagation across these calls
- `internal/kubernautagent/api/openapi.json` — authoritative machine-readable contract
- `pkg/agentclient/` — generated Go client (`ogen`) used by AIAnalysis
