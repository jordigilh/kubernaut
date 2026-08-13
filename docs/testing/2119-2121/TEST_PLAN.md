# Test Plan: #2119 + #2121 — KA Gate-Retry Confidence Drop and Undeclared-Tool Recovery (main port)

## 1. Purpose

`sameKindValidationGate` and `apiVersionValidationGate`
(`internal/kubernautagent/investigator/investigator_gates.go`) both fire a
correction-retry LLM call when their respective triggers are hit (a
same-kind remediation target, or an ambiguous kind missing `api_version`).
This is the `main`-branch port of two fixes already shipped to
`release/v1.5` (v1.5.6) as [#2118](https://github.com/jordigilh/kubernaut/issues/2118)
and [#2120](https://github.com/jordigilh/kubernaut/issues/2120), tracked here
under their v1.6-milestone clone numbers,
[#2119](https://github.com/jordigilh/kubernaut/issues/2119) and
[#2121](https://github.com/jordigilh/kubernaut/issues/2121). See
`release/v1.5`'s `docs/testing/2118/TEST_PLAN.md` and
`docs/testing/2120/TEST_PLAN.md` for the original live-LLM spike evidence
(10/10 trials reproducing the undeclared-tool failure mode; 10/10 trials
confirming the reminder-retry recovery) -- this plan focuses on what
adapting the fix to `main`'s already-refactored gate structure required.

### #2119: silently dropped RCA confidence on retry

`sameKindValidationGate` already had a "lost field, keep original" guard for
`RemediationTarget.Kind`, but none for `Confidence`. Its correction prompt
reads as a narrow yes/no question ("is the target correct?"), so the LLM
frequently responds with a minimal tool call that answers only that
question and omits (or zeroes) the separately-required confidence field --
silently overwriting a real, previously-validated confidence with 0.0.

### #2121: gate retry silently skips the correction on an undeclared tool call

Both gates build their gate-retry LLM call with a submit-result-only tool
declaration, but the model frequently calls an undeclared tool instead
(measured 10/10 on `release/v1.5`'s live-LLM spike). Pre-fix, this fell
through to the same fallback path as a genuinely empty response: the
correction was silently dropped, with no record in the audit trail of *why*
the retry failed (both "resolved" and "exhausted" were the only outcomes
recorded, `apiVersionValidationGate` only). A second attempt -- replaying
the wrong call as a synthetic `tool_result` error plus an explicit reminder
-- recovers the correct `submit_result` call in the referenced spike.

While restructuring both gates' audit-event lifecycle to add the new
`retry_outcome` taxonomy, `main` was found to have the same pre-existing
dead-mutation bug as `release/v1.5`'s [#2124](https://github.com/jordigilh/kubernaut/issues/2124):
both gates called `audit.StoreBestEffort(gateEvent, ...)` immediately, then
mutated `gateEvent.Data["retry_outcome"]`/`EventOutcome` afterward -- a
production audit store snapshots `Data` synchronously at call time, so that
mutation was never actually persisted. Fixed in the same change (both gates
now `defer` the store call), consistent with how `release/v1.5` fixed its
own instance.

## 2. Adaptation Required (this was not a copy-paste port)

`main`'s `investigator_gates.go` had already been refactored (during this
session's earlier #2088/#2090 port) into a shape `release/v1.5` does not
have:

- Thin trigger functions (`sameKindValidationGate`/`apiVersionValidationGate`)
  delegate to extracted retry functions (`retryForSameKind`/
  `retryForAPIVersion`, the latter via a `retryForAPIVersionParams` struct).
- A `LLMInvocationContext` struct (`Tokens`, `CorrelationID`, `Client`,
  `ModelName`, `RuntimeParams`) groups the retry functions' LLM-call
  parameters, per `AGENTS.md`'s 8+-param Options-pattern rule --
  `release/v1.5` still passes these as 6+ individual parameters.
- Already-extracted helpers `submitOnlyRCATools()`/`extractSubmitContent()`
  (release/v1.5's equivalents: `submitResultOnlyTools()`/
  `gateRetrySubmitContent()`).

The port kept `main`'s naming (`submitOnlyRCATools`, `extractSubmitContent`,
`retryForSameKind`, `retryForAPIVersion`) and extended their signatures/
bodies in place, rather than introducing `release/v1.5`'s parallel names --
avoiding two near-duplicate helper sets in the same file. Concretely:

1. `extractSubmitContent` changed from `(resp) string` to
   `(resp) (content string, otherTools []string)`, absorbing
   `release/v1.5`'s `gateRetrySubmitContent` behavior into the existing name.
2. New `retryGateOnUnexpectedTool` method, taking `LLMInvocationContext`
   instead of `release/v1.5`'s 6 individual parameters (`client`,
   `tokens`, `runtimeParams`, `correlationID`, ...).
3. New `gateRetryOutcome` enum (`resolved`, `resolved_after_other_tool_retry`,
   `llm_requested_other_tool`, `empty_response`, `parse_error`, `exhausted`)
   wired through both `retryForSameKind`/`retryForAPIVersion` and
   `apiVersionGateExhaustion` (which now takes the outcome as a parameter
   instead of hardcoding `"exhausted"`).
4. Both `audit.StoreBestEffort` calls changed from immediate to `defer`red
   (fixing `main`'s own #2124-equivalent dead-mutation instance).
5. Confidence-backfill guard + strengthened correction message added to
   `retryForSameKind`/`sameKindCorrectionMessage` (#2119).
6. `main`'s test fixture (`gateRecordingAuditStore.StoreAudit` in
   `apiversion_gate_test.go`) updated to snapshot `event.Data` at call time,
   matching real production semantics (identical fix to `release/v1.5`'s).
7. New test files use `newTestInvestigator` (not `investigator.New`
   directly, as `release/v1.5`'s originals do) to satisfy `main`-only
   [#1677](https://github.com/jordigilh/kubernaut/issues/1677) fail-closed
   catalog-validation hardening for the workflow-selection phase that
   follows each gate in every scenario -- `release/v1.5` does not have this
   hardening, so its original tests call `investigator.New` directly.

No Wiring Manifest row: this modifies existing, already-wired gate logic
(`Investigate()` -> `sameKindValidationGate`/`apiVersionValidationGate`); no
new production entry point.

## 3. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **SI-10** | Information Input/Output Validation | A retry's `confidence` value must be validated against regression, not blindly trusted; a gate correction must be actually delivered and answered before its result is trusted. |
| **AU-3** | Content of Audit Records | Confidence surfaced to operators/Console must reflect genuine RCA certainty, not a silently-dropped placeholder; the audit trail must distinguish "the LLM re-evaluated and answered" from "the LLM never engaged with the correction," and must actually persist that distinction. |

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-KA-2119-001 (regression) | Unit (via real `Investigate()`) | Same-kind gate retry confirms the same target but omits confidence; pre-retry confidence (0.88) must be backfilled, not silently dropped to 0 | SI-10, AU-3 | `samekind_confidence_2119_test.go` |
| UT-KA-2119-002 | Unit | Retry genuinely recomputes a different, non-zero confidence (0.55); must be preserved unchanged | SI-10 | same file |
| UT-KA-2119-003 (edge case) | Unit | Pre-retry confidence was itself never set (0); guard must not fabricate a non-zero confidence out of another zero | SI-10 | same file |
| UT-KA-2119-004 | Unit | Correction message must instruct a full RCA restatement including confidence, not a bare yes/no confirmation | AU-3 | same file |
| UT-KA-2121-001 (regression) | Unit (via real `Investigate()`) | Same-kind gate's retry calls an undeclared tool on attempt 1, `submit_result` on attempt 2 (reminder recovery); `retry_outcome == "resolved_after_other_tool_retry"`, `EventOutcome == success` | AU-3, SI-10 | `gate_undeclared_tool_2121_test.go` |
| UT-KA-2121-002 | Unit | Same-kind gate's retry calls an undeclared tool on both attempts; original result kept, `retry_outcome == "llm_requested_other_tool"`, `EventOutcome == failure` | AU-3 | same file |
| UT-KA-2121-003 | Unit | API-version gate mirrors 001: undeclared tool then correct `api_version` on reminder; recovered, no human review | AU-3, SI-10 | same file |
| UT-KA-2121-004 | Unit | API-version gate mirrors 002: undeclared tool on both attempts; human review still triggered (`rca_incomplete`) | SI-10 | same file |
| UT-KA-2121-005 (regression for #2124-equivalent) | Unit | With the fixed, honest `gateRecordingAuditStore` (snapshotting at call time), a plain successful same-kind retry and a plain exhausted api-version retry both show correct `retry_outcome`/`EventOutcome`, proving both are captured by `StoreAudit` at call time | AU-3 | same file |

### Tier Coverage Rationale

- **UT** (via the real `Investigate()` entry point, following this
  package's established convention in `apiversion_gate_test.go`) covers
  both fixes completely: the confidence-backfill guard and the
  undeclared-tool-retry/audit-timing behavior are pure business-logic
  properties of the two gates' retry-handling and audit-emission code,
  fully reproducible with a mock `llm.Client` and no external dependencies.
- **IT/E2E for this fix itself**: not added net-new. Both gates' wiring into
  the production `Investigate()` path already exists and is already
  exercised by the existing `apiversion_gate_test.go`/
  `gate_history_propagation_1936_test.go`/`gate_keepalive_2088_test.go`
  suites plus this fix's own UT specs (which call through that same real
  entry point); this change modifies internal retry-handling and
  audit-emission behavior without adding a new wiring point.
- **IT/E2E were added net-new for a related, separately-tracked gap**:
  verifying this fix's audit trail surfaced [#2141](https://github.com/jordigilh/kubernaut/issues/2141)
  (the `retry_outcome`/`ambiguous_kind`/`conflicting_groups` fields set here
  never reached Data Storage due to a fixed `LLMRequestPayload` schema,
  independent of this fix's correctness). See
  `docs/testing/2141/TEST_PLAN.md` for that fix's own IT/E2E proof —
  consolidated into this same PR/branch given CI resource constraints.

## 5. Validation Results

- `go build ./...` -- clean.
- `go test ./internal/kubernautagent/investigator/...` -- pass (all existing
  specs plus the 9 new UT-KA-2119/2121-XXX specs above).
- `golangci-lint run internal/kubernautagent/investigator/...` -- see PR
  verification step for full-repo lint run.
