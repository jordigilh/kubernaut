# Test Plan: #2120 — Gate Retry Silently Skips Correction on Undeclared Tool Call

## 1. Purpose

`sameKindValidationGate` and `apiVersionValidationGate`
(`internal/kubernautagent/investigator/investigator_gates.go`) both send
their gate-retry LLM call with only a single declared tool
(`submitResultOnlyTools()`, i.e. just `submit_result`). Discovered during the
#2118 live-LLM spike: the model does not always respect this constraint --
it frequently returns a `tool_use` block naming a tool that was never
declared in the request's `tools` array (e.g. `kubectl_describe`,
`get_namespaced_resource_context`) instead of calling `submit_result`.

Pre-#2120, this did not corrupt data or crash: the gate's tool-call scan
found no `submit_result` call, fell back to `resp.Message.Content` (empty,
since the model's turn was pure tool-use), and correctly treated the empty
content as an error path -- logging "no content in retry response, keeping
original" and returning the pre-retry result unchanged.

The problem was **silent correction loss, indistinguishable from a
genuinely empty response**: the gate believed it attempted a correction, but
the LLM never engaged with the correction prompt at all. Neither gate's
audit event distinguished "LLM re-evaluated and confirmed/changed the
target" from "LLM never got the chance to re-evaluate because it wanted a
different tool" -- both fell into the same opaque bucket (no `retry_outcome`
at all for `sameKindValidationGate`; a hardcoded `"exhausted"` for
`apiVersionValidationGate`, indistinguishable from every other failure mode).

### Live-LLM Spike (completed prior to implementation)

A throwaway Go program (`cmd/spike2120main/`, deleted after the run) reused
the production `vertexanthropic` client construction path (`claude-sonnet-5`
on Vertex AI, real project/credentials pulled from the cluster's
`kubernaut-agent-llm-runtime` ConfigMap / `llm-credentials` Secret) to
reproduce the same-kind gate retry with a realistic multi-turn synthetic
investigation history, matching production's exact request shape
(`submitResultOnlyTools()` declared, `JSONMode: true`).

- **10/10 trials**: the first gate-retry attempt called `kubectl_describe`
  instead of `submit_result` -- confirming the bug is not rare, it is the
  *dominant* outcome for this prompt/tool-set shape.
- **10/10 trials**: after replaying that wrong call as a synthetic
  `tool_result` "not available" error plus one explicit reminder message,
  the **second** attempt correctly called `submit_result`.

This gave strong empirical confidence that "detect undeclared tool -> retry
once with reminder" (the issue's suggested fix option 1) is a highly
effective, low-risk fix. Option 3 from the issue (forcing `tool_choice`) was
out of scope: `llm.ChatOptions` (`pkg/kubernautagent/llm/types.go`) has no
`ToolChoice` field today, and production also routes through
`pkg/kubernautagent/llm/langchaingo/adapter.go` for non-`vertex_ai`
providers, where equivalent support is unconfirmed -- left as a separate
future issue if ever needed.

## 2. Chosen Fix

1. **`gateRetrySubmitContent(resp)`** classifies a gate-retry response:
   returns `submit_result`'s tool-call arguments if present (falling back to
   raw message content, mirroring pre-#2120 behavior for bare-JSON-text
   responses), plus the names of any other tool(s) called instead.
2. **`retryGateOnUnexpectedTool(...)`** re-issues the gate-retry call once
   more when an undeclared tool was called: replays that call as a
   synthetic `tool_result` "not available" error, plus an explicit reminder
   that only `submit_result` is available, then re-sends with the same
   single-tool declaration.
3. Both gates now route through these two helpers. On recovery, the outcome
   is tagged `resolved_after_other_tool_retry`, distinct from a first-try
   `resolved`. On persistence of the undeclared-tool behavior across both
   attempts, the outcome is tagged `llm_requested_other_tool`, distinct from
   `empty_response`/`parse_error`/generic `exhausted` -- so production
   frequency of each specific failure mode becomes observable.
4. `apiVersionGateExhaustion` now takes the outcome as a parameter instead
   of hardcoding `"exhausted"`, since it is called for more distinct reasons
   post-#2120. Confirmed via Serena's `find_referencing_symbols` that it has
   exactly 4 call sites, all within `apiVersionValidationGate` in the same
   file -- the signature change is fully self-contained.
5. `submitResultOnlyTools()` extracted as a shared helper, deduplicating the
   identical `[]llm.ToolDefinition{...}` literal previously repeated in both
   gates (REFACTOR phase).

### Secondary finding fixed in the same change: dead `retry_outcome`/`EventOutcome`

While restructuring both gates' audit-event lifecycle to add the new
`retry_outcome` values, a **pre-existing** bug was found and fixed --
tracked separately for traceability as
[#2124](https://github.com/jordigilh/kubernaut/issues/2124):
`apiVersionValidationGate` called `audit.StoreBestEffort(gateEvent, ...)`
*before* the gate-retry LLM call, then mutated `gateEvent.Data["retry_outcome"]`
*afterward*. Real production audit stores
(`internal/kubernautagent/audit/ds_store.go`'s `DSAuditStore`/
`BufferedDSAuditStore`) synchronously serialize `event.Data` into the
outbound request **at call time**, so that mutation was never actually
persisted. The existing unit-test double (`gateRecordingAuditStore`) masked
this by storing the raw `*audit.AuditEvent` **pointer**, so an in-memory
read after `Investigate()` returned always reflected the final state
regardless of when `StoreAudit` was actually called.

Fix: both gates now `defer audit.StoreBestEffort(...)` immediately after
building `gateEvent`, so the store call fires once, after every `Data`/
`EventOutcome` mutation on every exit path has already happened. The shared
`gateRecordingAuditStore.StoreAudit` (`apiversion_gate_test.go`, used by 11
test files) now snapshots `event.Data` (map copy) at call time, matching
real production semantics -- verified via Serena's `search_for_pattern`
that no other `StoreBestEffort` call site in this package mutates `Data`
after storing, so this test-fidelity fix carries zero regression risk for
the other 10 files sharing the fixture. A related smaller defect fixed
alongside: both gates previously hardcoded `gateEvent.EventOutcome =
audit.OutcomeSuccess` unconditionally at construction; both now set it per
exit path (`Success` only on genuine resolution, `Failure` on every
failure/exhaustion branch), matching the sibling `CheckWorkflowTargetAlignment`
gate's existing convention.

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
| UT-KA-2120-002 | Unit | Same-kind gate's retry calls an undeclared tool on both attempts -> original result kept (pre-#2120 fallback behavior preserved); `retry_outcome == "llm_requested_other_tool"`, `EventOutcome == failure` | AU-3 | same file |
| UT-KA-2120-003 | Unit | API-version gate mirrors 001: undeclared tool then correct `api_version` on reminder -> recovered, no human review, `retry_outcome == "resolved_after_other_tool_retry"` | AU-3, SI-10 | same file |
| UT-KA-2120-004 | Unit | API-version gate mirrors 002: undeclared tool on both attempts -> human review still triggered (`rca_incomplete`) to prevent incorrect RBAC grants, `retry_outcome == "llm_requested_other_tool"`, `EventOutcome == failure` | SI-10 | same file |
| UT-KA-2120-005 (regression for #2124) | Unit | With the fixed, honest `gateRecordingAuditStore` (snapshotting at call time), a plain successful same-kind retry shows `retry_outcome == "resolved"`/`EventOutcome == success`, and a plain exhausted api-version retry shows `retry_outcome == "exhausted"`/`EventOutcome == failure` -- proving both are captured by `StoreAudit` at call time, not just recoverable from a later in-memory pointer read | AU-3 | same file |

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
  existing `apiversion_gate_test.go`/`gate_history_propagation_1935_test.go`/
  `gate_keepalive_2086_test.go` suites plus this fix's own UT specs (which
  call through that same real entry point); this change modifies internal
  retry-handling and audit-emission behavior without adding a new wiring
  point. `test/integration/kubernautagent/investigator/...` was re-run to
  confirm no regression (its `recordingAuditStore` test double has the same
  by-pointer capture pattern as the pre-fix unit double, but no IT asserts
  on `retry_outcome`/`EventOutcome` values or store-call ordering, so it is
  unaffected either way).

## 5. Validation Results

- `go test ./internal/kubernautagent/investigator/...` -- pass, 286/286
  specs (including the 6 new UT-KA-2120-XXX specs above; all 6 failed
  against the unmodified gates, confirming RED before the fix was applied).
- `go test ./internal/kubernautagent/...` (full package) -- pass, no
  regressions.
- `go test ./test/integration/kubernautagent/investigator/...` -- pass,
  117/117 specs, no regressions.
- `go test ./test/integration/kubernautagent/...` (full package) -- pass
  except two pre-existing, unrelated envtest infrastructure failures
  (`enrichment`, `tools/custom`: missing local `/usr/local/kubebuilder/bin/etcd`
  binary in this environment) -- unaffected by this change.
- `golangci-lint run internal/kubernautagent/investigator/...` -- 0 issues.
- `go build ./...` -- clean.
