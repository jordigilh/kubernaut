# Test Plan: #2141 — Gate-Retry Audit Fields Never Reach Data Storage

## 1. Purpose

`sameKindValidationGate`/`apiVersionValidationGate`
(`internal/kubernautagent/investigator/investigator_gates.go`) set diagnostic/
outcome fields on `audit.AuditEvent.Data` — `retry_outcome` (added while
porting [#2119](https://github.com/jordigilh/kubernaut/issues/2119)/
[#2121](https://github.com/jordigilh/kubernaut/issues/2121)), `ambiguous_kind`,
`conflicting_groups` — but none of these ever reached Data Storage/Postgres,
regardless of the in-process `StoreBestEffort` ordering fix landed alongside
#2119/#2121 in this same branch.

Gate audit events are emitted with `EventType: audit.EventTypeLLMRequest`
(`aiagent.llm.request`). That event type serializes through
`buildLLMRequestPayload` (`internal/kubernautagent/audit/ds_payloads.go`) into
`ogenclient.LLMRequestPayload`, an OpenAPI-generated struct with a fixed
schema: `model`, `prompt_length`, `prompt_preview`, `event_id`, `incident_id`,
`toolsets_enabled`. There was no free-form/`additionalProperties` passthrough
for `event.Data`, so any key not explicitly mapped by `buildLLMRequestPayload`
(including `retry_outcome`, `ambiguous_kind`, `conflicting_groups`) was
silently dropped before the outbound `AuditEventRequest` was built.

Per `AGENTS.md`'s audit mandate (BR-AUDIT-005, SOC2 CC8.1): "given a
`correlation_id`, the complete lifecycle... MUST be reconstructable from audit
traces alone." Without this fix, the gate-retry diagnostic detail (why a
retry happened, what ambiguity triggered it, whether it resolved) was not
reconstructable from the persisted audit trail — it only ever existed in the
in-process `*audit.AuditEvent.Data` map for the duration of a single
`Investigate()` call.

Discovered while verifying the audit trail for the #2119/#2121 port (this
same branch) — pre-dates both and affects the same-kind/API-version gates'
audit trail in general, likely since [#1044](https://github.com/jordigilh/kubernaut/issues/1044)
introduced these gates. See `docs/testing/2119-2121/TEST_PLAN.md` for the
gate-retry fix this schema gap was discovered alongside.

## 2. Chosen Fix

Added `retry_outcome`, `ambiguous_kind`, `conflicting_groups` as three
optional string fields to the `LLMRequestPayload` schema in
`api/openapi/data-storage-v1.yaml`, regenerated the ogen client
(`pkg/datastorage/ogen-client`) and both embedded spec copies
(`pkg/audit/openapi_spec_data.yaml`,
`pkg/datastorage/server/middleware/openapi_spec_data.yaml`), and wired
`buildLLMRequestPayload` to populate them when present on `event.Data`. Per
preflight, Data Storage's server-side write/read paths treat `event_data` as
opaque JSONB in both directions, so no DS server-side code changed — this is
a client-side-only schema + codegen + mapping change.

No Wiring Manifest row: `buildLLMRequestPayload` is already wired into the
production `DSAuditStore.StoreAudit` path; this only extends the schema it
already maps onto.

### Prerequisite this fix depends on

Persistence can only be verified once `gateEvent.Data["retry_outcome"]` is
actually populated *before* the audit store call fires. That ordering fix
(`audit.StoreBestEffort` changed from an immediate call to a `defer`red one)
is part of the #2119/#2121 gate-retry commit in this same branch, not this
one — see `docs/testing/2119-2121/TEST_PLAN.md` Section 1's "#2124-equivalent
dead-mutation bug" note. This test plan's IT/E2E scenarios below exercise
that already-fixed ordering; they do not re-prove it.

## 3. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **AU-3** | Content of Audit Records | Gate-retry diagnostic fields must actually persist to Data Storage, not just exist transiently in-process, for the audit record to have the content AU-3 requires. |
| **AU-9** | Protection of Audit Information | Fields must survive the full write path into the immutable audit store, not be silently dropped by a schema gap upstream of persistence. |

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-KA-2141-001 | Unit | `buildLLMRequestPayload` maps `event.Data["retry_outcome"]`/`["ambiguous_kind"]`/`["conflicting_groups"]` onto the new `LLMRequestPayload` wire-schema fields when present, and leaves them unset (not empty-string) for ordinary non-gate `llm.request` events | AU-3 | `internal/kubernautagent/audit/ds_store_test.go` |
| IT-KA-2141-001 | Integration (real `DSAuditStore` + Postgres) | A real gate-exhaustion `Investigate()` call, followed by a `QueryAuditEvents` call to the real Data Storage service by `correlation_id`, finds the `api_version_validation_gate` event and decodes `ambiguous_kind`/`conflicting_groups`/`retry_outcome == "exhausted"` from the persisted `LLMRequestPayload` | AU-3, AU-9 | `test/integration/kubernautagent/investigator/apiversion_gate_integration_test.go` |
| E2E-KA-1044-001 (extended) | E2E (real binary, real K8s, real mock LLM, real Data Storage) | Extends the existing ambiguous-kind gate-exhaustion journey: after asserting `HumanReviewNeeded`/`HumanReviewReason` on the business response, also queries Data Storage by `remediation_id` and asserts `ambiguous_kind`/`retry_outcome == "exhausted"` are present on the persisted audit event | AU-3, AU-9 | `test/e2e/kubernautagent/apiversion_gate_e2e_test.go` |

### Independent hardening landed in the same branch: IT-level audit fixture had the identical masking flaw

While investigating the above, `capturingAuditStore`
(`test/integration/kubernautagent/investigator/suite_test.go`) — the shared
IT-tier fixture behind `IT-KA-1044-*`, `IT-KA-433-AP-*`, `IT-KA-851-AP-*`, and
`IT-KA-947-*` — was found to have the exact same pointer-aliasing flaw the
pre-fix unit test double had: `c.events = append(c.events, event)` stores the
raw pointer, so any of those ~15 existing IT specs reading
`auditStore.events` after `Investigate()` returns would see final-state data
regardless of when `StoreAudit` was actually called, masking this same class
of bug at the IT tier too. Fixed to snapshot `event.Data` at call time,
mirroring the unit-level fix and real `DSAuditStore` semantics. This is
fixture hardening, not a new business-behavior test.

### Tier Coverage Rationale

- **UT** covers the mapping logic in isolation (present vs. absent fields).
- **IT/E2E are both net-new wiring points** — data that previously had no
  path to Data Storage now does. `IT-KA-2141-001` proves the wiring directly
  (real `DSAuditStore` write, real `QueryAuditEvents` read-back).
  `E2E-KA-1044-001` proves the full production journey through the real
  binary reaches the same result, closing the AU-3 control objective with an
  actual proving journey rather than only a wiring-level proof.

### Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| `LLMRequestPayload.RetryOutcome`/`AmbiguousKind`/`ConflictingGroups` | `DSAuditStore.StoreAudit`, invoked via the gates' deferred `audit.StoreBestEffort` | `internal/kubernautagent/audit/ds_payloads.go::buildLLMRequestPayload` | `IT-KA-2141-001` |

## 5. Validation Results

- `api/openapi/data-storage-v1.yaml` (`LLMRequestPayload`) extended with 3
  optional string fields; `make generate` regenerated
  `pkg/datastorage/ogen-client` and both embedded spec copies with zero
  unexpected drift (`git status` after `make generate` showed only the files
  intentionally touched — confirms the `gen-diff` CI gate passes).
- `go test ./internal/kubernautagent/audit/...` — pass, including new
  `UT-KA-2141-001` (2 specs: fields populated when present, left unset for
  ordinary events). Confirmed RED before GREEN (test failed against the
  pre-#2141 schema, as expected).
- `IT-KA-2141-001` — executed locally against real envtest + Podman-backed
  Data Storage/Postgres (`KUBEBUILDER_ASSETS=$(setup-envtest use 1.36 -p path)
  ginkgo ./test/integration/kubernautagent/investigator/...`): the
  gate-exhaustion audit event is queried back from the real Postgres-backed
  Data Storage by `correlation_id`, and `retry_outcome`/`ambiguous_kind`/
  `conflicting_groups` are confirmed present and correct on the decoded
  `LLMRequestPayload`. This run also transitively re-validates the
  `capturingAuditStore` fixture fix above (same suite run, same process).
- `E2E-KA-1044-001` extension — executed locally against a real Kind cluster
  (`ginkgo --focus="BR-AI-1044" ./test/e2e/kubernautagent/...`, full infra
  bootstrap: Kind + Data Storage + Postgres + Mock LLM + Kubernaut Agent
  deployment): the extended assertions confirm the full production journey —
  real gate exhaustion in the deployed Kubernaut Agent binary -> real
  `DSAuditStore` -> real Data Storage service -> real Postgres -> queryable
  by `remediation_id` via `QueryAuditEvents`, with `ambiguous_kind` and
  `retry_outcome=exhausted` reconstructable end-to-end.
- `go build ./...`, `go vet ./...`, `golangci-lint run` — clean.
