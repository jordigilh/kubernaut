# Test Plan: #2121 — Gate Retry Silently Skips Correction on Undeclared Tool Call (v1.6 port of #2120)

## 1. Purpose

`sameKindValidationGate` and `apiVersionValidationGate`
(`internal/kubernautagent/investigator/investigator_gates.go`) both send
their gate-retry LLM call with only a single declared tool
(`submitOnlyRCATools()`, i.e. just `submit_result`). Discovered during the
#2118 live-LLM spike: the model does not always respect this constraint --
it frequently returns a `tool_use` block naming a tool that was never
declared in the request's `tools` array (e.g. `kubectl_describe`,
`get_namespaced_resource_context`) instead of calling `submit_result`.

Pre-fix, this did not corrupt data or crash: the gate's tool-call scan found
no `submit_result` call, fell back to `resp.Message.Content` (empty, since
the model's turn was pure tool-use), and correctly treated the empty
content as an error path -- logging "no content in retry response, keeping
original" and returning the pre-retry result unchanged.

The problem was **silent correction loss, indistinguishable from a
genuinely empty response**: the gate believed it attempted a correction, but
the LLM never engaged with the correction prompt at all. Neither gate's
audit event distinguished "LLM re-evaluated and confirmed/changed the
target" from "LLM never got the chance to re-evaluate because it wanted a
different tool."

This is the v1.6 port of the fix originally shipped for
[#2120](https://github.com/jordigilh/kubernaut/issues/2120) on
`release/v1.5` (v1.5.6, PR #2122). The live-LLM spike (10/10 undeclared-tool
reproduction, 10/10 recovery after a reminder retry) and the chosen fix are
identical; see #2120's own test plan for the spike details. This document
tracks the v1.6 port and its adapted implementation (main's gate structure
at port time differs from v1.5.6's: `LLMInvocationContext` plumbing, split
`sameKindValidationGate`/`retryForSameKind` and
`apiVersionValidationGate`/`retryForAPIVersion` functions, and post-#1578
Reasoning-block capture on the accepted retry result that #2118/#2120
predate -- `retryGateOnUnexpectedTool` was extended to also return the
reminder response's `Reasoning` block so that capture is preserved on the
reminder-recovery path, not just the first-attempt path).

## 2. Chosen Fix

1. **`gateRetrySubmitContent(resp)`** classifies a gate-retry response:
   returns `submit_result`'s tool-call arguments if present (falling back to
   raw message content, mirroring pre-fix behavior for bare-JSON-text
   responses), plus the names of any other tool(s) called instead.
2. **`retryGateOnUnexpectedTool(...)`** re-issues the gate-retry call once
   more when an undeclared tool was called: replays that call as a
   synthetic `tool_result` "not available" error, plus an explicit reminder
   that only `submit_result` is available, then re-sends with the same
   single-tool declaration. Also returns the reminder turn's `Reasoning`
   block (main-only addition vs. upstream, for BR-AI-086 AC6 parity).
3. **`resolveGateContent(...)`** (main-only extraction, added during the
   port to keep `retryForSameKind`/`retryForAPIVersion` under the
   `funlen`/`gocyclo` budget): shared helper wrapping steps 1-2, used by
   both gates so the undeclared-tool-recovery logic is not duplicated.
4. Both gates now route through these helpers. On recovery, the outcome is
   tagged `resolved_after_other_tool_retry`, distinct from a first-try
   `resolved`. On persistence of the undeclared-tool behavior across both
   attempts, the outcome is tagged `llm_requested_other_tool`, distinct
   from `empty_response`/`parse_error`/generic `exhausted`.
5. `apiVersionGateExhaustion` now takes the outcome as a parameter instead
   of hardcoding `"exhausted"`.

### Secondary finding fixed in the same change: dead `retry_outcome`/`EventOutcome`

While restructuring both gates' audit-event lifecycle to add the new
`retry_outcome` values, a pre-existing bug was found and fixed (also fixed
upstream, tracked there as #2124): `apiVersionValidationGate` called
`audit.StoreBestEffort(gateEvent, ...)` *before* the gate-retry LLM call,
then mutated `gateEvent.Data["retry_outcome"]` *afterward*. `sameKindValidationGate`
had the same shape (call before the retry). Real production audit stores
(`internal/kubernautagent/audit/ds_store.go`'s `DSAuditStore`/
`BufferedDSAuditStore`) synchronously serialize `event.Data` into the
outbound request **at call time**, so that mutation was never actually
persisted. The existing unit-test double (`gateRecordingAuditStore` in
`apiversion_gate_test.go`) masked this by storing the raw
`*audit.AuditEvent` **pointer**, so an in-memory read after `Investigate()`
returned always reflected the final state regardless of when `StoreAudit`
was actually called.

Fix: both gates now `defer audit.StoreBestEffort(...)` immediately after
building `gateEvent`, so the store call fires once, after every `Data`/
`EventOutcome` mutation on every exit path has already happened. The
shared `gateRecordingAuditStore.StoreAudit` now snapshots `event.Data`
(map copy) at call time, matching real production semantics.

No Wiring Manifest row: both gates already wire into the production
`Investigate()` path; this change modifies their internal retry-handling
and audit-emission logic without adding a new production entry point.

## 3. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **AU-3** | Content of Audit Records | The audit trail must distinguish "the LLM re-evaluated and answered" from "the LLM never engaged with the correction at all," and must actually persist that distinction (fixing the dead-mutation defect) rather than silently dropping it before it reaches the real audit store. |
| **SI-10** | Information Input/Output Validation | A gate correction must be actually delivered and answered before its result is trusted; an undeclared-tool response must not be silently conflated with a validated confirmation. |

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-KA-2120-001 (regression) | Unit (via real `Investigate()`) | Same-kind gate's retry calls an undeclared tool on attempt 1, `submit_result` on attempt 2 (reminder recovery) -> final result reflects attempt 2's data; `retry_outcome == "resolved_after_other_tool_retry"`, `EventOutcome == success` | AU-3, SI-10 | `internal/kubernautagent/investigator/gate_undeclared_tool_2120_test.go` |
| UT-KA-2120-002 | Unit | Same-kind gate's retry calls an undeclared tool on both attempts -> original result kept (pre-fix fallback behavior preserved); `retry_outcome == "llm_requested_other_tool"`, `EventOutcome == failure` | AU-3 | same file |
| UT-KA-2120-003 | Unit | API-version gate mirrors 001: undeclared tool then correct `api_version` on reminder -> recovered, no human review, `retry_outcome == "resolved_after_other_tool_retry"` | AU-3, SI-10 | same file |
| UT-KA-2120-004 | Unit | API-version gate mirrors 002: undeclared tool on both attempts -> human review still triggered (`rca_incomplete`) to prevent incorrect RBAC grants, `retry_outcome == "llm_requested_other_tool"`, `EventOutcome == failure` | SI-10 | same file |
| UT-KA-2120-005 (regression for the dead-mutation defect) | Unit | With the fixed, honest `gateRecordingAuditStore` (snapshotting at call time), a plain successful same-kind retry shows `retry_outcome == "resolved"`/`EventOutcome == success`, and a plain exhausted api-version retry shows `retry_outcome == "exhausted"`/`EventOutcome == failure` -- proving both are captured by `StoreAudit` at call time, not just recoverable from a later in-memory pointer read | AU-3 | same file |

(Test/scenario IDs retain the original `UT-KA-2120-*` numbering from the
upstream fix, per this package's convention.)

### Tier Coverage Rationale

- **UT** (via the real `Investigate()` entry point, following this
  package's established convention in `apiversion_gate_test.go` and
  `samekind_confidence_2118_test.go`) covers this fix completely: the
  undeclared-tool-retry behavior and the audit-timing fix are both pure
  business-logic properties of the two gates' retry-handling and
  audit-emission code, fully reproducible with a mock `llm.Client` and no
  external dependencies.
- **IT/E2E**: not added net-new. Both gates' wiring into the production
  `Investigate()` path already exists and is already exercised by the
  existing `apiversion_gate_test.go` suite plus this fix's own UT specs
  (which call through that same real entry point); this change modifies
  internal retry-handling and audit-emission behavior without adding a new
  wiring point.

## 5. Validation Results

- `go test ./internal/kubernautagent/investigator/...` -- pass, 366/366
  specs (including the 6 new UT-KA-2120-XXX specs above plus 4
  UT-KA-2118-XXX specs from the co-ported #2119 fix; UT-KA-2120-003 required
  switching to the package's `newTestInvestigator` helper -- unlike upstream
  v1.5.6, main's post-#1677 hardening fails workflow selection closed to
  human review when no `CatalogFetcher` is configured, which is orthogonal
  to the behavior under test).
- `go test ./internal/kubernautagent/...` (full package) -- pass, no
  regressions.
- `golangci-lint run ./internal/kubernautagent/investigator/...` -- 0
  issues (required extracting `gateContentResult`/`resolveGateContent` and
  `finalizeSameKindRetry`/`finalizeAPIVersionRetry` helpers to stay under
  the `funlen`/`gocyclo`/`revive` argument-limit budgets -- main's gates
  carry more logic per function than v1.5.6's pre-port versions).
- `go build ./...` -- clean.
