# DD-AUDIT-011: Remove the `persona` Field from the `apifrontend.triage.started` Audit Payload

- **Status:** Accepted
- **Date:** 2026-11-27
- **Issue:** [#2716](https://github.com/jordigilh/kubernaut/issues/2716) (follow-up to #2713, umbrella #2701)
- **Related:** [DD-AUDIT-003](DD-AUDIT-003-service-audit-trace-requirements.md) (per-service audit catalog), [ADR-034](ADR-034-unified-audit-table-design.md) (unified audit table schema), kubernaut-operator#682 (operator renders no persona roles)
- **Compliance impact:** FedRAMP AU-3 (content of audit records), SOC2 CC7.2 (reconstruction completeness)

## Context

`ApifrontendTriageStartedPayload.persona` was a **required** field in the
data-storage OpenAPI contract (`api/openapi/data-storage-v1.yaml`), described
as "Detected/assigned persona from JWT role". It was populated by
`resolvePersona()` in `pkg/apifrontend/launcher/launcher.go`, which mapped the
authenticated user's OIDC group membership (`sre`, `ai-orchestrator`, `cicd`,
`observability`, `l3-audit`, `remediation-approver`) onto a fixed
six-value persona enum (`sre`, `orchestrator`, `cicd`, `dashboard`, `audit`,
`approver`).

This violated the product principle confirmed in #2716: **kubernaut exposes
roles/capabilities; operators and SREs decide which user gets which
functionality. Hardcoded personas are an opinionated shortcut the platform
must not enforce.** Although the field was observability metadata and never an
authorization input, it (a) baked persona framing into the AU-3 audit record
and forensic reconstruction, and (b) required production code
(`resolvePersona`) to carry a hardcoded group-to-persona mapping.

## Decision

**Drop the field entirely** (remove, not rename).

Verification (preflight, #2716): no consumer reads it — DataStorage
reconstruction, query, and repository code never reference the field; the
ogen-generated client was the only Go surface; actor attribution is already
carried by the parent audit record's `actor_id` (username) per ADR-034. A
rename (e.g. `detected_role`) was rejected because any value for such a field
still requires production persona-derivation logic, which the product
principle forbids.

Wire-schema changes (pre-GA, no backwards-compatibility commitment per the
kubernaut-operator#682 convention):

1. `api/openapi/data-storage-v1.yaml` — remove `persona` from
   `ApifrontendTriageStartedPayload` (properties and `required`)
2. Embedded spec copies regenerated (`pkg/audit/openapi_spec_data.yaml`,
   `pkg/datastorage/server/middleware/openapi_spec_data.yaml`)
3. ogen client regenerated (`pkg/datastorage/ogen-client`)
4. `buildTriageStartedPayload` (`pkg/apifrontend/audit/store_adapter.go`) no
   longer sets the field
5. `resolvePersona()` / `personaSRE` and the `"persona"` detail key removed
   from `pkg/apifrontend/launcher/launcher.go`

## Compliance Mapping

- **AU-3 (content of audit records):** unchanged in substance — the record
  still carries `event_id`, `event_type`, `event_action`, `event_outcome`,
  `actor_type`, `actor_id`, `correlation_id`, and `session_id`. The removed
  field was redundant with actor attribution and duplicated no unique
  forensic value (verified: no consumer).
- **SOC2 CC7.2 (reconstruction):** `correlation_id`-based reconstruction is
  unaffected; `triage.started` remains the pipeline-start marker with
  `session_id`. Wire acceptance is proven by `IT-DS-2716-001` (DataStorage
  OpenAPI validation middleware accepts a persona-less `triage.started`
  payload) and `UT-AF-2716-001/002` (store adapter wire shape, launcher
  emission). The a2a_streaming IT uses an in-memory auditor and does not
  exercise the DataStorage wire.

## Consequences

- Audit records written before this change that contain `persona` remain
  readable (the field is simply absent from new records; the ogen decoder
  ignores unknown/absent optional fields on read paths that matter).
- The six-value persona enum no longer exists anywhere in the audit contract.
- Any external consumer that grouped forensics by that field must re-derive
  grouping from `actor_id`/groups at the source; none exist in-repo.

## Alternatives Considered

1. **Rename to `role` / `detected_role`** — rejected: keeps production
   persona-derivation logic alive and implies the platform assigns roles.
2. **Keep as optional field** — rejected: same objection; a legacy-framing
   field with no consumer is pure contract debt.
