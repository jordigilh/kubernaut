# Kubernaut Agent - API Specification

**Version**: 2.0 (per `api/agentsession/v1alpha1/agentsession_types.go`)
**Last Updated**: 2026-08-23
**Status**: ✅ Current

---

## Purpose

KA's API contract with AIAnalysis (AA) is the Kubernetes-native **`AgentSession` CRD**
([DD-AA-KA-001](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md)), not
an HTTP request/response cycle. AA creates one `AgentSession` per investigation; KA's dispatch
`Reconciler` watches for it, races other KA replicas for a per-object `coordination/v1.Lease` to
guarantee exactly-once dispatch, and is the **sole writer** of `Status`. There is no synchronous
"submit and get the answer back" endpoint, and no polling — AA observes the result via a
Kubernetes watch on `Status`.

The retired HTTP submit/poll API (`POST /api/v1/incident/analyze`,
`GET /api/v1/incident/session/{id}`, etc.) and its generated client (`pkg/agentclient`,
`ogen`-based OpenAPI) are **fully deleted**
([issue #2190](https://github.com/jordigilh/kubernaut/issues/2190)). The authoritative source of
truth for the schema below is
[`api/agentsession/v1alpha1/agentsession_types.go`](../../../api/agentsession/v1alpha1/agentsession_types.go);
this document is a hand-written summary of it, kept for discoverability.

**Sole client**: the AIAnalysis controller (creates and watches `AgentSession`). apifrontend
(AF) has a **separate, independent** MCP channel into KA for deep investigation
([DD-AF-004](../../architecture/decisions/DD-AF-004-investigation-tool-split.md)) — untouched by
DD-AA-KA-001 and out of scope for this document.

---

## Access Control

There is no HTTP `Authorization: Bearer <JWT>` header on this channel — access is governed by
Kubernetes RBAC on the `AgentSession` resource (see
[security-configuration.md](./security-configuration.md) for KA's and AA's respective RBAC
grants). AA needs `create`/`get`/`list`/`watch` on `agentsessions`; KA needs
`get`/`list`/`watch`/`update`/`patch` (patch for the dispatch-cleanup finalizer) plus
`update` on the `agentsessions/status` subresource.

---

## `AgentSession` Schema

### `Spec` (immutable after Create — `+kubebuilder:validation:XValidation:rule="self == oldSelf"`)

A 1:1, lossless translation of the retired `agentclient.IncidentRequest` HTTP request body — AA
populates every field at Create time from the same `AIAnalysis.Spec.AnalysisRequest` source its
old `RequestBuilder.BuildIncidentRequest` used to read, so no content was lost in removing the
HTTP channel.

| Field | Type | Required | Notes |
|---|---|---|---|
| `remediationRequestRef` | object (`name`, `namespace`) | ✅ | The `RemediationRequest` this session investigates; the `AgentSession` MUST be created in the same namespace |
| `incidentID` | string | ✅ | AIAnalysis CR name |
| `remediationID` | string | ✅ | Audit correlation ONLY — never used for RCA or workflow matching (DD-WORKFLOW-002 v2.2) — propagated through to audit events, see [observability-logging.md](./observability-logging.md) |
| `signalName`, `signalSource` | string | ✅ / optional | |
| `severity` | enum | ✅ | `critical` \| `high` \| `warning` \| `info` \| `unknown` |
| `resourceNamespace`, `resourceKind`, `resourceName`, `resourceAPIVersion` | string | optional | Original signal target (may differ from KA's determined remediation target — see [DD-KA-006](../../architecture/decisions/DD-KA-006-remediation-target-in-rca.md)) |
| `errorMessage`, `description` | string | optional | |
| `environment`, `priority`, `riskTolerance`, `businessCategory` | string | optional | |
| `clusterName`, `cluster` | string | optional | `cluster` is the fleet business classification (BR-FLEET-003) |
| `isDuplicate`, `occurrenceCount`, `deduplicationWindowMinutes` | bool / int / int | optional | |
| `firingTime`, `receivedTime`, `firstSeen`, `lastSeen` | string (ISO timestamp) | optional | |
| `signalLabels`, `signalAnnotations` | map[string]string | optional | |
| `enrichmentResults` | raw JSON | optional | Remediation history (Tier 1/Tier 2), see [DD-KA-016](../../architecture/decisions/DD-KA-016-remediation-history-context.md) |
| `signalMode` | enum | optional | `reactive` \| `proactive` (ADR-054) |
| `timesOutAt` | timestamp | optional | Absolute deadline propagated from `AIAnalysis.Spec.TimesOutAt` (DD-TIMEOUT-002); KA self-enforces this independently of AA (#2170) |

**Note on `interactive`**: deliberately **not** a `Spec` field. Whether an investigation should
be interactive can become true after `Create` (a human can start an interactive session at any
time via AF's MCP tools), and `Spec` is immutable — so there is no create-time snapshot that
stays correct. KA's dispatch `Reconciler` is the sole owner of this determination: it checks
`InvestigationSession` CRD existence at the actual dispatch decision point
([DD-AA-KA-001 Amendment "Gap 1"](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md))
and writes the result to `Status.Interactive`.

### `Status` (KA-only writer — AA and AF only ever watch it, never write it)

| Field | Type | Notes |
|---|---|---|
| `phase` | enum | `Pending` \| `Investigating` \| `Completed` \| `Failed` \| `Cancelled` |
| `sessionID` | string | KA's internal investigation session identifier (audit correlation, AU-2/AU-3); set the instant a KA replica wins the dispatch Lease |
| `interactive` | bool | True once a human driver has taken over via KA's `UpgradeToInteractive` — AA/AF MUST read this field, not infer interactivity from `InvestigationSession` existence |
| `actingUser`, `actingUserGroups` | string / array | The authenticated identity currently driving an interactive session, if any |
| `result` | object (`AgentSessionResult`) | Curated outcome, set on the `Completed` transition (SI-10: never internal workflow/validation/alignment state) — see fields below |
| `error` | string | Curated, user-facing failure message, set on `Failed` (SI-11: never a raw internal error string) |
| `reason` | string | Curated, machine-readable failure classification; currently only `CapacityExceeded` (BR-AI-009, a transient/retryable dispatch-capacity rejection) is defined — empty for all other failures |
| `dispatchedAt` | timestamp | When a KA replica won the dispatch Lease and began investigating |
| `completedAt` | timestamp | When the session reached a terminal phase |
| `conditions` | `[]metav1.Condition` | Detailed status information |

### `Status.Result` (`AgentSessionResult`)

| Field | Type | Notes |
|---|---|---|
| `incidentID` | string | KA's investigation identifier for correlation |
| `analysis` | string | Human-readable RCA narrative |
| `rootCauseAnalysis` | raw JSON | Structured RCA payload (summary, severity, contributing_factors, remediationTarget) |
| `selectedWorkflow` | raw JSON \| null | The workflow KA selected from the catalog, if any |
| `confidence` | number | LLM self-reported confidence (0.0-1.0) — see [Known Gaps](./overview.md#known-gaps--deviations) |
| `needsHumanReview` | bool | Derived from confidence bands, see [BR-KA-197](../../requirements/BR-KA-197-needs-human-review-field.md). When true, AA must NOT create a `WorkflowExecution` |
| `humanReviewReason` | string | Why human review is needed, if applicable |
| `isActionable` | bool \| null | |
| `warnings` | array | e.g. Tier 2 ineffective-remediation-history warnings |
| `alternativeWorkflows` | array | Other workflows the LLM considered (context/audit only, never automatic fallback) |
| `validationAttemptsHistory` | array | Self-correction attempts, see [BR-KA-191](../../requirements/BR-KA-191-workflow-parameter-validation.md) |
| `detectedLabels` | raw JSON \| null | Infrastructure characteristics, see [DD-KA-018](../../architecture/decisions/DD-KA-018-detected-labels-detection-specification.md) |
| `alignmentVerdict` | object \| null | Shadow Agent's security verdict for this session, see [shadow-agent-configuration.md](./shadow-agent-configuration.md) |
| `timestamp` | string | |

---

## Lifecycle

1. **Create**: AA creates `AgentSession` with `ownerRef` set to the `AIAnalysis` that creates it
   (cascade-delete reaches the `RemediationRequest` transitively — RO already owns the RR→AA
   chain). `Status.Phase` starts empty/`Pending`.
2. **Dispatch**: KA's `Dispatcher.Reconcile` observes the Create, adds a
   `kubernautagent.kubernaut.ai/agentsession-dispatch-cleanup` finalizer, and races other KA
   replicas for a per-object `coordination/v1.Lease` (`dispatch-<name>`). The winner writes
   `Status.Phase=Investigating` (or stays `Pending` for a deferred-interactive session,
   BR-INTERACTIVE-010) plus `SessionID`.
3. **Investigate**: KA runs the investigation (LLM calls, tool calls, confidence scoring —
   unchanged from before DD-AA-KA-001; see [overview.md](./overview.md)).
4. **Terminal write**: on completion, `session.Manager`'s `TerminalHook` fires
   `Dispatcher.OnTerminal`, which writes the final `Phase` (`Completed`/`Failed`/`Cancelled`) and
   `Result`/`Error`. AA's watch triggers its next reconcile — no polling.
5. **Cancellation**: AA never writes `Status` to cancel — it **deletes** the `AgentSession`
   (cascading from a deleted `RemediationRequest`/`AIAnalysis`). KA's `Dispatcher.reconcileDelete`
   observes the `DeletionTimestamp`, stops any in-flight investigation goroutine, then removes
   the finalizer to let the delete proceed.
6. **Self-enforced timeout**: independent of AA, KA fails an `AgentSession` whose investigation
   outlives `Spec.TimesOutAt` — there is no `CancelSession` RPC for a partitioned/crashed AA
   replica to call anymore (#2170).

---

## Related Documentation

- [overview.md](./overview.md) — architecture and the components this dispatch model fronts
- [integration-points.md](./integration-points.md) — who calls KA and what KA calls in turn
- [observability-logging.md](./observability-logging.md) — correlation ID propagation across these calls
- [DD-AA-KA-001](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md) — the design decision that replaced the HTTP channel with this CRD
- `api/agentsession/v1alpha1/agentsession_types.go` — authoritative Go type definitions
- `internal/kubernautagent/agentsession/dispatcher.go`, `status_writer.go` — KA's dispatch/Status-write implementation
