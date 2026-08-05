# Test Plan: Workflow-Discovery Phase Message-Staleness Fix (v1.5 fast-follow)

**Issue**: [#1945](https://github.com/jordigilh/kubernaut/issues/1945) (`release/v1.5`), expands scope of [#1936](https://github.com/jordigilh/kubernaut/issues/1936) (`main`)
**Authority**: BR-AUDIT-005 v2.0 (`docs/requirements/11_SECURITY_ACCESS_CONTROL.md`)
**Branch**: `fix/1945-workflow-discovery-message-staleness` (off `release/v1.5`)
**Created**: 2026-08-05
**Status**: Draft — implementation in progress

---

## 1. Purpose

Issue #1935/PR #1939 fixed the messages-staleness bug (`runLLMLoop`'s accumulated tool-call history never reaching the caller) **only for the RCA phase** (`runRCA`). Direct triage of the current `release/v1.5` source shows the identical bug is still live in the **Workflow-Discovery phase** (`runWorkflowSelection`).

### Root cause (confirmed via source triage): workflow-discovery retries never see tool-call history

`runWorkflowSelection` (`internal/kubernautagent/investigator/investigator.go`) builds a local `messages := []llm.Message{system, user}` and passes it to `runLLMLoop`. Unlike `runRCA` (already fixed), `runWorkflowSelection` **never reassigns `messages` from the loop's result** — `retryWorkflowSubmit`'s `history` argument (both call sites: the `TextResult` unparseable-text branch and the post-parse-error branch) and the self-correction closure's `messages` (reused across up to `maxSelfCorrectionAttempts` attempts) all keep reading the stale `[system, user]`-only slice, missing every `list_available_actions`/`list_workflows` tool call the LLM actually made.

The read-side helper introduced by #1939 only handles half of the `LoopResult` types it needs to — `SubmitWithWorkflowResult`/`SubmitNoWorkflowResult` (the two workflow-discovery sentinel types) already carry a populated `.Messages` field via `sentinelResult`, but `loopResultMessages` silently returns `nil` for both (`default` case), so even a caller that *did* read it back would get nothing for these two types.

This has two independent effects, exercised by two independent fixes below:

1. **Top-level propagation**: after the *first* `runLLMLoop` call in `runWorkflowSelection` returns, its accumulated messages must flow into `messages` before `retryWorkflowSubmit`'s two call sites.
2. **Self-correction closure propagation**: after *each* nested `runLLMLoop` call inside `selfCorrectWorkflowSelection`'s `correctionFn` closure, its accumulated messages must flow into the closure-captured `messages` before the *next* correction attempt's request is built — otherwise even attempt 2+ requests are missing tool calls made during attempt 1's own nested loop, independent of whatever the top-level fix already captured.

## 2. Fix Design

Mirror the already-shipped, already-tested `runRCA`/`loopResultMessages` fix exactly — no new types, no new abstractions:

- `internal/kubernautagent/investigator/investigator.go`:
  - Extend `loopResultMessages(r LoopResult) []llm.Message`'s type switch with `case *SubmitWithWorkflowResult` / `case *SubmitNoWorkflowResult`, both returning `v.Messages`.
  - In `runWorkflowSelection`, immediately after the top-level `runLLMLoop` call returns (before the result-type switch), reassign: `if extended := loopResultMessages(loopRes); len(extended) > 0 { messages = extended }`. This single fix point feeds **both** `retryWorkflowSubmit` call sites (the `TextResult`-branch call and the post-parse-error call), since both read the same `messages` variable.
  - In the self-correction `correctionFn` closure (inside `runWorkflowSelection`), immediately after its nested `runLLMLoop` call returns (before the closure's own result-type switch), the same reassignment: `if extended := loopResultMessages(corrLoopRes); len(extended) > 0 { messages = extended }`. Because `messages` is a free variable captured by reference from the enclosing `runWorkflowSelection` scope, this persists across attempts — attempt N+1's request is built from attempt N's reassigned value.

No signature changes to `retryWorkflowSubmit`, `loopResultMessages`'s callers, or `SelfCorrect`'s contract — this is a pure data-flow correction using existing plumbing.

## 3. FedRAMP / SOC2 Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **AU-3** | Content of Audit Records | `retryWorkflowSubmit`'s and the self-correction loop's `audit.StoreBestEffort` calls compute `prompt_length`/`prompt_preview` from the same stale `history`/`messages` — today those audit records themselves misrepresent what was actually sent to the LLM during workflow discovery. IT-KA-1945-002/003 assert the audit-visible prompt now reflects real tool-call evidence. |
| **CC8.1** | Audit Completeness / RR Reconstruction (BR-AUDIT-005 v2.0) | IT-KA-1945-002/003 assert the workflow-discovery LLM calls underlying a parse-retry or self-correction attempt are reconstructable with the tool evidence that informed them — closing the same class of gap #1935's RCA-phase fix closed, for the workflow-selection phase. |

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control / BR | Test File |
|---|---|---|---|---|
| UT-KA-1945-001 | Unit | `loopResultMessages` returns the accumulated `.Messages` for `*SubmitWithWorkflowResult` and `*SubmitNoWorkflowResult` (currently silently returns `nil` for both) | #1935 root cause #2 (workflow-discovery extension) | `internal/kubernautagent/investigator/investigator_workflow_history_propagation_1945_test.go` |
| IT-KA-1945-002 | Integration | `retryWorkflowSubmit`'s retry request (captured via mock `llm.Client`) includes the earlier `list_available_actions` tool_use/tool_result pair, through the real `Investigate` -> `runWorkflowSelection` -> `runLLMLoop` -> `retryWorkflowSubmit` production path, when the LLM's first workflow-discovery response is unparseable text | AU-3, CC8.1 | `internal/kubernautagent/investigator/workflow_history_propagation_1945_test.go` |
| IT-KA-1945-003 | Integration | Across two self-correction attempts (catalog-validation failure on a hallucinated `workflow_id`), the second attempt's request includes the first attempt's own nested-loop tool-call turn (`list_workflows`), not just the top-level `[system, user]` + correction messages — proving the closure's own nested reassignment, distinct from the top-level one | AU-3, CC8.1 | `internal/kubernautagent/investigator/workflow_history_propagation_1945_test.go` |

**Note on file split**: UT-KA-1945-001 requires white-box access to the unexported `loopResultMessages` function and lives in `package investigator` (mirroring `investigator_loop_messages_1935_test.go`'s established pattern). IT-KA-1945-002/003 exercise the real `Investigate()` production entry point and live in `package investigator_test` (mirroring `gate_history_propagation_1935_test.go`, reusing its `gateMockLLMClient`/`gateRecordingAuditStore`/`gateK8sClient`/`gateDSClient`/`gateWfToolResp` helpers), consistent with the split already established by #1935's own UT-006/007 vs. IT-008/009.

### Tier Skip Rationale — E2E

Same rationale as `docs/testing/1935/TEST_PLAN.md` Section 4: this is wiring-level data-flow correctness (does the right history reach the right retry call), which the Pyramid Invariant assigns to IT, not E2E. No existing E2E scenario asserts on internal message-history content; E2E proves journey completion, not this. Not filed as a gap since it mirrors the already-accepted precedent from the parent fix (#1935).

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `loopResultMessages` extended for workflow-discovery sentinel types | `runLLMLoop` (workflow-discovery phase) | `internal/kubernautagent/investigator/investigator.go` | UT-KA-1945-001 |
| `runWorkflowSelection` reassigns `messages` from top-level loop result before `retryWorkflowSubmit`/self-correction | `runWorkflowSelection` | `internal/kubernautagent/investigator/investigator.go` | IT-KA-1945-002 |
| Self-correction closure reassigns `messages` from each nested loop result | `selfCorrectWorkflowSelection`'s `correctionFn` (the closure defined inline in `runWorkflowSelection`) | `internal/kubernautagent/investigator/investigator.go` | IT-KA-1945-003 |

## 6. Out of Scope / Tracked Separately

- **`main` (#1936)**: confirmed to share the same defect shape in both the RCA and Workflow-Discovery phases, compounded by `main` currently lacking `Messages` fields on any `LoopResult` type. Scope expansion tracked via comment on #1936; the combined port (RCA + workflow-discovery + alignment rendering) happens as Phase 2, after this fix merges into `release/v1.5`.
- Alignment/grounding rendering fix (`renderConversation` rendering `msg.Reasoning.Text`) — already shipped for `release/v1.5` via #1939; no analogous gap exists for workflow-discovery since `alignment.NotifyRCAComplete` is only called once, for the RCA phase.

## 7. Final Results

_To be filled in after implementation and verification (build/lint/test pass)._
