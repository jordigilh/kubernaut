# Test Plan: Message-Staleness Fix — Combined `main` Port (RCA + Workflow-Discovery + Alignment Rendering)

**Issue**: [#1936](https://github.com/jordigilh/kubernaut/issues/1936) (`main`) — combined port of [#1935](https://github.com/jordigilh/kubernaut/issues/1935)/[#1939](https://github.com/jordigilh/kubernaut/pull/1939) (RCA phase, `release/v1.5`) and [#1945](https://github.com/jordigilh/kubernaut/issues/1945)/[#1947](https://github.com/jordigilh/kubernaut/pull/1947) (Workflow-Discovery phase, `release/v1.5`)
**Authority**: BR-AUDIT-005 v2.0 (`docs/requirements/11_SECURITY_ACCESS_CONTROL.md`)
**Branch**: `fix/1936-message-propagation-main-port` (off `main`)
**Created**: 2026-08-05
**Status**: Draft — implementation in progress

---

## 1. Purpose

`release/v1.5` shipped two fast-follow fixes for the same underlying defect class ("message-staleness": `runLLMLoop`'s accumulated tool-call history never reaching the caller that built its initial `[system, user]` message slice):

- **#1939** (RCA phase): `runRCA`'s `messages` was never reassigned from `runLLMLoop`'s result, starving `sameKindValidationGate`, `apiVersionValidationGate`, `retryRCASubmit`, and `alignment.NotifyRCAComplete`'s shadow-agent grounding review of real tool-call evidence.
- **#1947** (Workflow-Discovery phase): the identical bug in `runWorkflowSelection` — `retryWorkflowSubmit` (both call sites) and the self-correction closure's `messages` all read a stale `[system, user]`-only slice.

Direct triage of `main`'s current source (`internal/kubernautagent/investigator/investigator_rca.go`, `investigator_loop.go`, `investigator_workflow_selection.go`, `internal/kubernautagent/alignment/grounding.go`) confirms `main` shares the identical defect shape in **both** phases, compounded by one structural difference: `main`'s `LoopResult` types (`SubmitResult`, `SubmitWithWorkflowResult`, `SubmitNoWorkflowResult`, `TextResult`) carry **no `Messages` field at all** — only `CancelledResult` does. `release/v1.5` had already added `Messages` to `SubmitResult`/`TextResult` as part of #1939, so #1945's fix only needed to extend 2 more cases; `main`'s port must add the field to all 4 non-terminal `LoopResult` types from scratch.

### Confirmed via direct source read (line-accurate, `origin/main`)

- `investigator_loop.go:293` — `processToolCalls` calls `sentinelResult(tc, resp.Message.Reasoning)`; `sentinelResult` (`investigator.go:138`) builds `SubmitResult`/`SubmitWithWorkflowResult`/`SubmitNoWorkflowResult` with `Content`+`Reasoning` only — no message history threaded through.
- `investigator_loop.go:132` — the final-turn `TextResult{Content: ..., Reasoning: ...}` literal (no tool calls, loop terminates) also carries no message history.
- `investigator_rca.go:54` — `alignment.NotifyRCAComplete(ctx, messages)` reads `runRCA`'s never-reassigned local `messages`; `retryRCASubmit` (line 63), `sameKindValidationGate` (line 93), `apiVersionValidationGate` (line 112) all read the same stale variable downstream of that one point.
- `investigator_workflow_selection.go:65-70` — `runWorkflowSelection`'s top-level `runLLMLoop` result flows into `handleWorkflowSelectionLoopResult`, `workflowSelectionRetryOrHumanReview` (`retryWorkflowSubmit`'s two call sites: line 78 and inside the `TextResult` case at line 178), and `selfCorrectWorkflowSelection` (line 119) — all via the same never-reassigned `messages`.
- `investigator_workflow_selection.go:294-301` — `runSelfCorrectionAttempt`'s nested `runLLMLoop` call (line 297) feeds `classifySelfCorrectionLoopResult` (line 301) via `state.messages`, itself never reassigned from the nested loop's result.
- `internal/kubernautagent/alignment/grounding.go:167-183` — `renderConversation` renders only `msg.Content`; `main`'s `llm.Message` already carries `Reasoning *llm.ReasoningBlock` (BR-AI-086/#1580's `anthropicfamily` capture-and-replay), so once propagation is fixed, the shadow LLM would still never see the model's thinking text without this one-line addition.

### Confirmed NOT needed on `main` (scope exclusions, per #1936 triage comments)

- **Root cause #1 (dropped-thinking-block)**: `main`'s `anthropicfamily` client already implements correct capture/replay under BR-AI-086/#1580 — the `vertexanthropic`-specific defect that motivated `release/v1.5`'s separate `Reasoning`-field-on-response fix does not exist on `main`.
- **Console `ThinkingPanel` gap**: `main` already has a dedicated, fully-wired channel (`emitReasoningContentEvent` → `session.EventTypeReasoningContentDelta` → `ka_investigate_bridge.go` → `launcher.EmitReasoningContentSafe`, #1635/DD-LLM-009) — no `reasoning_delta`-field-overload workaround needed, unlike `release/v1.5`.
- **Contradiction canary**: explicitly dropped per #1935/#1936 decision — this fix targets the confirmed, reproduced defect only.

## 2. Fix Design

Mirrors the already-shipped `release/v1.5` pattern (both #1939 and #1947), adapted to `main`'s current type shapes:

### 2a. `internal/kubernautagent/investigator/investigator.go`
- Add `Messages []llm.Message` to `SubmitResult`, `SubmitWithWorkflowResult`, `SubmitNoWorkflowResult`, `TextResult` (additive fields; confirmed via repo-wide grep that every existing construction site — production and test — uses named-field literals, so this is non-breaking).
- Change `sentinelResult`'s signature to `sentinelResult(tc llm.ToolCall, reasoning *llm.ReasoningBlock, messages []llm.Message) LoopResult`, threading `messages` (the pre-this-turn accumulated history — i.e. every prior fully-paired assistant/tool turn, excluding the sentinel call itself, matching `release/v1.5`'s established "exclude the dangling tool_use" contract) into each of the three constructed results.
- Add a `loopResultMessages(r LoopResult) []llm.Message` helper (does not exist on `main` yet) with a type switch covering `*SubmitResult`, `*TextResult`, `*SubmitWithWorkflowResult`, `*SubmitNoWorkflowResult` (all four return `v.Messages`); `default` (Cancelled/Exhausted) returns `nil`.

### 2b. `internal/kubernautagent/investigator/investigator_loop.go`
- `processToolCalls`'s sentinel-detection call site (line 293): `sentinelResult(tc, resp.Message.Reasoning, messages)` — `messages` is the parameter already in scope (the pre-turn accumulated history passed into `processToolCalls`).
- `runLoopTurn`'s final-turn `TextResult` literal (line 132): add `Messages: messages`.

### 2c. `internal/kubernautagent/investigator/investigator_rca.go`
- In `runRCA`, immediately after `loopRes, err := inv.runLLMLoop(...)` returns (line 49) and before `alignment.NotifyRCAComplete(ctx, messages)` (line 54): `if extended := loopResultMessages(loopRes); len(extended) > 0 { messages = extended }`. This single point feeds `alignment.NotifyRCAComplete`, `retryRCASubmit` (line 63), `sameKindValidationGate` (line 93), and `apiVersionValidationGate` (line 112) — all downstream reads of the same variable.

### 2d. `internal/kubernautagent/investigator/investigator_workflow_selection.go`
- In `runWorkflowSelection`, immediately after `loopRes, err := inv.runLLMLoop(...)` returns (line 65) and before `handleWorkflowSelectionLoopResult` (line 70): same reassignment pattern. Feeds `workflowSelectionRetryOrHumanReview`/`retryWorkflowSubmit` (both the line-78 and line-178 call sites) and `selfCorrectWorkflowSelection` (line 119).
- In `runSelfCorrectionAttempt`, immediately after `corrLoopRes, corrErr := inv.runLLMLoop(ctx, state.messages, ...)` returns (line 297) and before `classifySelfCorrectionLoopResult` (line 301): `if extended := loopResultMessages(corrLoopRes); len(extended) > 0 { state.messages = extended }`. Because `state` is a pointer shared across `validator.SelfCorrect`'s repeated `correctionFn` invocations, this persists correctly across up to `maxSelfCorrectionAttempts` attempts.

### 2e. `internal/kubernautagent/alignment/grounding.go`
- `renderConversation` (line 167): render `msg.Reasoning.Text` alongside `msg.Content` when present (one-line addition, identical to the fix already shipped on `release/v1.5` via #1939).

No signature changes to `runLLMLoop`, `LLMInvocationContext`, `retryRCASubmit`, `retryWorkflowSubmit`, `selfCorrectionState`, or `validator.SelfCorrect`'s contract — this is a pure data-flow correction plus one new field-per-type and one new helper, using existing plumbing throughout.

## 3. FedRAMP / SOC2 Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **AU-3** | Content of Audit Records | `emitLLMRequestAudit`/`emitRetryAudit`/gate audit events compute `prompt_length`/`prompt_preview`/full `messages` from the same stale variables fixed here — today those audit records misrepresent what was actually sent to the LLM in both phases. IT-KA-1936-004/005/007/008 assert the audit-visible prompt now reflects real tool-call evidence. |
| **CC7.2** | Internal Controls (Decision Audit Trails) | The same-kind and apiVersion validation gates, and the workflow-discovery self-correction loop, are decision-audit-trail mechanisms; IT-KA-1936-004/005/008 prove their retry decisions are now made with complete context, not a stale approximation. |
| **CC8.1** | Audit Completeness / RR Reconstruction (BR-AUDIT-005 v2.0) | IT-KA-1936-004/005/007/008/009 assert that LLM calls underlying a gate retry, parse-retry, self-correction attempt, or shadow-agent grounding review are reconstructable with the tool evidence that actually informed them — closing the same class of gap #1935/#1945 closed on `release/v1.5`, now on `main`. |

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control / BR | Test File |
|---|---|---|---|---|
| UT-KA-1936-001 | Unit | `sentinelResult` populates `.Messages` on `*SubmitResult`/`*SubmitWithWorkflowResult`/`*SubmitNoWorkflowResult` from its `messages` parameter (currently has no `messages` parameter at all) | #1935/#1945 root cause #2 (main port) | `internal/kubernautagent/investigator/investigator_message_propagation_1936_test.go` |
| UT-KA-1936-002 | Unit | `runLoopTurn`'s final-turn `TextResult` carries `.Messages` (the accumulated history at the point the LLM responds with plain text and no tool call) | same | `internal/kubernautagent/investigator/investigator_message_propagation_1936_test.go` |
| UT-KA-1936-003 | Unit | `loopResultMessages` returns `.Messages` for all four of `*SubmitResult`/`*TextResult`/`*SubmitWithWorkflowResult`/`*SubmitNoWorkflowResult`; returns `nil` for `*CancelledResult`/`*ExhaustedResult` (unchanged, handled separately by their own callers) | same | `internal/kubernautagent/investigator/investigator_message_propagation_1936_test.go` |
| IT-KA-1936-004 | Integration | `sameKindValidationGate`'s retry request (captured via mock `llm.Client`), through the real `Investigate()` production path, includes the earlier `kubectl_describe` tool_use/tool_result pair | AU-3, CC7.2, CC8.1 | `internal/kubernautagent/investigator/gate_history_propagation_1936_test.go` |
| IT-KA-1936-005 | Integration | `apiVersionValidationGate`'s retry request includes the earlier `kubectl_describe` tool_use/tool_result pair, through the same production path | AU-3, CC7.2, CC8.1 | `internal/kubernautagent/investigator/gate_history_propagation_1936_test.go` |
| IT-KA-1936-006 | Integration | Shadow agent's grounding request (via `alignment.NotifyRCAComplete` → `Observer` → `Evaluator`) includes both the earlier tool-call history and the model's `Reasoning.Text`, through the real `Investigate()` path with a real `Observer`/`Evaluator` pair | AU-3, CC8.1 | `internal/kubernautagent/investigator/alignment_grounding_propagation_1936_test.go` |
| IT-KA-1936-007 | Integration | `retryWorkflowSubmit`'s retry request includes the earlier `list_available_actions` tool_use/tool_result pair, through the real `Investigate()` production path, when the LLM's first workflow-discovery response is unparseable text | AU-3, CC8.1 | `internal/kubernautagent/investigator/workflow_history_propagation_1936_test.go` |
| IT-KA-1936-008 | Integration | Across two self-correction attempts (catalog-validation failure on a hallucinated `workflow_id`), the second attempt's request includes the first attempt's own nested-loop tool-call turn (`list_workflows`), not just the top-level `[system, user]` + correction messages | AU-3, CC7.2, CC8.1 | `internal/kubernautagent/investigator/workflow_history_propagation_1936_test.go` |

**Note on file split**: UT-KA-1936-001/002/003 require white-box access to unexported `sentinelResult`/`loopResultMessages`/`runLoopTurn` and live in `package investigator`. IT-KA-1936-004 through 008 exercise the real `Investigate()` production entry point and live in `package investigator_test`, reusing the `gateMockLLMClient`/`gateRecordingAuditStore`/`gateK8sClient`/`gateDSClient`/`gateWfToolResp`/`stubCatalogFetcher` helpers already established on `main` (`apiversion_gate_test.go`, `wiring_test.go`) — this is the same shared harness `release/v1.5`'s #1935/#1945 fixes used, confirmed present and API-identical on `main` by direct source read. This mirrors the split already established by #1935 (UT-006/007 vs. IT-008/009/010) and #1945 (UT-001a/001b vs. IT-002/003).

### Tier Skip Rationale — E2E

Same rationale as `docs/testing/1935/TEST_PLAN.md` Section 4 and `docs/testing/1945/TEST_PLAN.md`'s Tier Skip Rationale: this is wiring-level data-flow correctness (does the right history reach the right retry/grounding call), which the Pyramid Invariant assigns to IT, not E2E. No existing E2E scenario asserts on internal message-history content; E2E proves journey completion, not this. Not filed as a gap since it mirrors the already-accepted precedent from both parent fixes.

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `Messages` field added to `SubmitResult`/`SubmitWithWorkflowResult`/`SubmitNoWorkflowResult`/`TextResult`; `sentinelResult` threads it through | `runLLMLoop` (both phases) | `internal/kubernautagent/investigator/investigator.go`, `investigator_loop.go` | UT-KA-1936-001, UT-KA-1936-002 |
| `loopResultMessages` helper (new) | `runRCA`, `runWorkflowSelection`, `runSelfCorrectionAttempt` | `internal/kubernautagent/investigator/investigator.go` | UT-KA-1936-003 |
| `runRCA` reassigns `messages` from top-level loop result before gates/retry/alignment notify | `runRCA` | `internal/kubernautagent/investigator/investigator_rca.go` | IT-KA-1936-004, IT-KA-1936-005, IT-KA-1936-006 |
| `runWorkflowSelection` reassigns `messages` from top-level loop result before `retryWorkflowSubmit`/self-correction | `runWorkflowSelection` | `internal/kubernautagent/investigator/investigator_workflow_selection.go` | IT-KA-1936-007 |
| `runSelfCorrectionAttempt` reassigns `state.messages` from each nested loop result | `selfCorrectWorkflowSelection` → `runSelfCorrectionAttempt` | `internal/kubernautagent/investigator/investigator_workflow_selection.go` | IT-KA-1936-008 |
| `renderConversation` renders `msg.Reasoning.Text` | `alignment.NotifyRCAComplete` → `Observer.StartGroundingReview` → `Evaluator.EvaluateGrounding` | `internal/kubernautagent/alignment/grounding.go` | IT-KA-1936-006 |

## 6. Out of Scope / Tracked Separately

- Root cause #1 (dropped-thinking-block) and the console `ThinkingPanel` gap — both confirmed not applicable to `main` (see Section 1).
- The tool-access-contradiction canary — explicitly dropped per user decision on #1935/#1936; this fix targets only the confirmed, reproduced message-propagation defect.

## 7. Final Results

_To be filled in after implementation and verification (build/lint/test pass)._
