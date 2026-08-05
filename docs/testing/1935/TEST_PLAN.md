# Test Plan: Thinking-Block Round-Trip Fix + Gate-Retry Message-Propagation Fix + Console ThinkingPanel Fix

**Issue**: [#1935](https://github.com/jordigilh/kubernaut/issues/1935) (`release/v1.5`), [#1936](https://github.com/jordigilh/kubernaut/issues/1936) (`main`)
**Authority**: BR-AI-086 (`docs/requirements/BR-AI-086-llm-reasoning-token-support.md`, AC1/AC3/AC6), BR-AUDIT-005 v2.0 (`docs/requirements/11_SECURITY_ACCESS_CONTROL.md`)
**Branches**: `fix/1935-thinking-block-roundtrip` (off `release/v1.5`)
**Created**: 2026-08-04
**Status**: Draft — implementation in progress (revision 3: canary dropped, message-propagation fix added, console ThinkingPanel fix added)

---

## 1. Purpose

Issue #1935 identified that when `investigator_gates.go`'s `sameKindValidationGate`/`apiVersionValidationGate` fire, the model's resubmitted RCA can be internally self-contradictory: the numeric `confidence` field disagrees with its own `due_diligence.confidence_calibration` narrative, and that narrative's claims about tool availability are demonstrably false against the session's own audit trail (`kubectl_describe`/`kubectl_logs` were called *before* the gate fired, yet the retry narrative claims they "could not be gathered").

### Root cause #1 (confirmed via live API spike): dropped thinking block on `release/v1.5`

A direct Vertex AI API spike (same auth path, prompt, and tools as production) proved `claude-sonnet-5` returns a signed `thinking` content block by default (`thinking_tokens: 14`), while `claude-sonnet-4-6` returns a plain `text` block (`thinking_tokens: 0`) for the identical request. `release/v1.5`'s `llm.Message` type (`pkg/kubernautagent/llm/types.go`) had no field to hold this block, and `vertexanthropic.Client.mapResponse` (`pkg/kubernautagent/llm/vertexanthropic/client.go`) had no `case "thinking"`/`case "redacted_thinking"` — so the block was silently dropped on every Sonnet-5 turn. This is the same failure class as issue #1299 ("orphaned content blocks on replay") and mirrors `main`'s already-fixed `anthropicfamily` client (BR-AI-086/#1580). **Fixed** (Section 2.1, GREEN, tests passing).

### Root cause #2 (confirmed via code reading + live production audit-trail comparison): gate retries never see tool-call history, on both branches

While building an initial defense-in-depth canary for root cause #1 (see Section 2.3, since dropped), instrumented testing revealed that `history` — the parameter `sameKindValidationGate`/`apiVersionValidationGate` receive — **never contains any tool-call turns, regardless of model or how many tools were actually called during the investigation.**

`runRCA` (`internal/kubernautagent/investigator/investigator.go`) builds a local `messages := []llm.Message{system, user}` and passes it **by value** into `runLLMLoop`. Inside the loop, `messages = append(messages, ...)` correctly accumulates tool-call/tool-result turns turn-by-turn (the model itself sees full context while the loop runs) — but `runLLMLoop` returns only `&TextResult{Content: ...}` / `&SubmitResult{Content: ...}` (via `sentinelResult`), neither of which carries the accumulated messages back to the caller (only `CancelledResult.Messages` does). `runRCA`'s own `messages` variable is therefore permanently stuck at its initial `[system, user]` value, and that stale value is what flows into both gates (and `retryRCASubmit`) as `history`. This has existed since the gate was introduced in #847 and is present, byte-for-byte identical, on `main` too (verified against the exact deployed commit `main-60b899a7b`).

**Live production confirmation** (same `RemediationRequest` `rr-618ac7d3b894-ba320bf0` re-run once with each model, 8 minutes apart, pulled from the cluster's `audit_events` table):

| | `claude-sonnet-5` (22:10) | `claude-sonnet-4-6` (22:18) |
|---|---|---|
| Tool calls before gate fired | 9 | 9 |
| Gate retry prompt size (`prompt_length`) | 14,847 bytes | 14,847 bytes (byte-for-byte identical — confirms `history` carries zero tool-call content on either run) |
| Retry `evidenceSufficiency` claim | *"No tool calls...were available/executed in this session"* — false, 9 tools were called | *"All claims are backed by tool output..."* |

This is the actual root cause of the false "no tool access" narrative pattern: the retry call is **structurally deprived of the evidence** it needs to answer honestly, on any model. `claude-sonnet-5` happens to explicitly (and, in this instance, incorrectly) generalize "no tools offered this turn" into "no tool evidence exists in this session"; `claude-sonnet-4-6` does not make this specific over-claim in the same starved context, but neither model is actually working from real information during the retry.

### Root cause #2, extended finding: the shadow/alignment agent's grounding review is equally starved

While fixing root cause #2 for the two validation gates, the same `messages` variable in `runRCA` is also what `alignment.NotifyRCAComplete` passes to `Observer.StartGroundingReview` — the shadow agent's core full-context defense against prompt-injection/hallucination (#1096). That call site sat *before* the fix's reassignment point, so it was still receiving the stale `[system, user]` snapshot: every grounding review to date has been evaluating an almost-empty conversation, regardless of how many tools were actually called. Separately, even once given the real conversation, `renderConversation` (`internal/kubernautagent/alignment/grounding.go`) only rendered `msg.Content`, never `msg.Reasoning.Text` — so the model's thinking-block text (BR-AI-086) would still never reach the shadow LLM's grounding prompt. Both gaps are fixed in the same change (Section 2.2).

### Root cause #1, extended finding: the console's ThinkingPanel (A2A channel) goes blank on the same class of gap

Following up on root cause #1 (dropped thinking block), the question was raised: does the console UI's live investigation narrative suffer the same loss? It does, through a third, independent code path. Three call sites in `internal/kubernautagent/investigator/investigator.go` (`runLLMLoop`'s main turn loop, `attemptRCASubmitRetry`, and the workflow-discovery parse-retry loop) build the `session.EventTypeReasoningDelta` event exclusively from `resp.Message.Content`, never `resp.Message.Reasoning`. This event crosses the KA↔AF MCP boundary and AF's `FormatEventForUser`/`emitEventToA2A` (`pkg/apifrontend/tools/ka_investigate_mcp.go`) route it, unmodified, onto the **A2A artifact channel** as `launcher.EmitReasoningSafe` — the text rendered live in the console's ThinkingPanel.

**Live production confirmation** (same incident `rr-618ac7d3b894-ba320bf0`, correlating `aiagent.llm.request`'s `model` field to the immediately-following `aiagent.llm.response`'s `has_analysis`/`analysis_length`/`tool_call_count`, queried directly from the cluster's `audit_events` table):

| Phase | Model | Turns with `Content` empty despite 1–4 tool calls |
|---|---|---|
| RCA (22:08–22:10) | `claude-sonnet-5` | **5 / 5 (100%)** |
| Later gate-retry/discovery (22:17–22:18) | `claude-sonnet-4-6` | 3 / 6 (50%) |

Every single turn of the Sonnet-5 RCA phase left `Content` empty while still making tool calls — all of that turn's narrative went into the private thinking block this code path never reads, and `emitEventToA2A` no-ops on an empty string, so nothing is sent to the console at all (not even an empty bubble). **Net effect**: the console's ThinkingPanel goes silent for the entire diagnostic phase of a Sonnet-5 investigation. This does not self-heal once root cause #1's capture fix (Section 2.1) ships — `Reasoning.Text` becomes available on `llm.Message`, but these 3 emission sites still only read `.Content` unless also updated. Fixed in Section 2.3.

## 2. Fix Design

### 2.1 `release/v1.5`: backport reasoning round-trip (COMPLETE)

Ported exactly three of BR-AI-086's acceptance criteria from `main`'s `anthropicfamily` client to `release/v1.5`'s `vertexanthropic` client — not the full BR (no opt-in request config, effort dials, or live-stream event type):

- `pkg/kubernautagent/llm/types.go`: `ReasoningBlock` type (`Text`, `Signature`, `Redacted`) and `Message.Reasoning *ReasoningBlock` field — byte-identical port from `main`.
- `pkg/kubernautagent/llm/vertexanthropic/client.go`:
  - `mapResponse()`: `case "thinking"` / `case "redacted_thinking"` capturing into `resp.Message.Reasoning`.
  - `convertAssistantMessage(m llm.Message) (anthropic.MessageParam, bool)` helper, prepending a reasoning content block via `reasoningToContentBlock(r *llm.ReasoningBlock)` whenever `m.Reasoning != nil`, before any `tool_use`/text blocks.

### 2.2 Both branches: gate-retry message-propagation fix (root-cause fix for #1935's reported symptom)

Make `runRCA` see the actual accumulated conversation after `runLLMLoop` returns, instead of its own stale `[system, user]` local variable:

- `internal/kubernautagent/investigator/investigator.go`:
  - Add `Messages []llm.Message` field to `SubmitResult`, `SubmitWithWorkflowResult`, `SubmitNoWorkflowResult`, and `TextResult` (mirroring the field `CancelledResult` already has), populated with the turns accumulated *prior to* the final (winning) turn — i.e., only fully-paired `assistant(tool_use)`/`tool(tool_result)` turns, never a dangling unpaired `tool_use`, so the Anthropic replay contract is never violated.
  - `sentinelResult` gains a `messages []llm.Message` parameter to populate this field.
  - New `loopResultMessages(r LoopResult) []llm.Message` helper. In `runRCA`, immediately after `runLLMLoop` returns and *before* `alignment.NotifyRCAComplete`, reassign the local `messages` variable from this helper — one fix point upstream of every consumer: the shadow-agent grounding review, `retryRCASubmit`, `sameKindValidationGate`, and `apiVersionValidationGate` all benefit.
- No change to `sameKindValidationGate`/`apiVersionValidationGate` signatures — they already accept `history []llm.Message`; they simply receive real data now.
- `internal/kubernautagent/alignment/grounding.go`: `renderConversation` now also renders `msg.Reasoning.Text` (prefixed `[role:thinking]`) immediately before a message's `[role] Content` line, whenever a reasoning block is present, so the shadow LLM's grounding prompt actually includes the model's thinking text instead of silently dropping it.

### 2.3 `release/v1.5`: console ThinkingPanel (A2A channel) reasoning-text fallback

New `reasoningDeltaText(msg llm.Message) string` helper (`internal/kubernautagent/investigator/investigator_tools.go`): returns `msg.Content` unchanged when `msg.Reasoning` is nil or has no visible text (no behavior change for non-thinking models or redacted blocks); falls back to `msg.Reasoning.Text` when `Content` is empty; concatenates reasoning-then-content (separated by a blank line) when both are present. Wired into all three `emitToSink(ctx, session.EventTypeReasoningDelta, ...)` call sites in `internal/kubernautagent/investigator/investigator.go` (main turn loop, RCA parse-retry, workflow-discovery parse-retry), replacing the bare `resp.Message.Content` read at each site. User decision (2026-08-04): fix now in this PR (not fallback-only, not deferred) — a 2–3 minute investigation with a silent ThinkingPanel is poor UX regardless of eventual console-side configurability.

**Follow-up filed with the console team**: `kubernaut-console` should expose a user-facing setting to disable raw-thinking display in the ThinkingPanel (enabled by default), since Anthropic's own guidance treats extended-thinking text as exploratory/unfiltered reasoning rather than a polished user-facing statement. Out of scope for this repo's fix — tracked as [jordigilh/kubernaut-console#53](https://github.com/jordigilh/kubernaut-console/issues/53).

### 2.4 Dropped: tool-access-contradiction canary

An initial design added a phrase-matching audit canary (`checkToolAccessContradiction`) to flag due-diligence narratives that deny tool access. Dropped after review: it is a defense for a specific model's specific wording of a symptom whose actual root cause (Section 1, root cause #2) is now understood and directly fixable. A brittle phrase-list keyed to `claude-sonnet-5`'s current wording would need constant upkeep as models change and provides no value once real history flows into the retry. Superseded by Section 2.2.

### 2.5 Out of scope

- BR-AI-086 AC2/AC5/AC8/AC9/AC10 (opt-in reasoning request config, effort dials, self-hosted overrides, live-stream event type) — a targeted bug-fix backport, not the full BR.
- Re-triggering the live crashloop scenario on the shared dev cluster for a *new* capture — the existing `rr-618ac7d3b894-ba320bf0` audit trail (already captured, both models) is used for verification instead.
- Extending `test/services/mock-llm` with an Anthropic-format response builder (would enable true E2E coverage of this path) — flagged as backlog, not part of this fix (see Section 6).
- Filing an issue for the separately-found cross-phase Confidence-overwrite finding (Phase 3 workflow-selection silently overwriting Phase 1 RCA confidence) — unrelated to this fix, tracked independently.

## 3. FedRAMP / SOC2 Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **CC7.2** | Decision Audit Trail, incl. operator-visible in real time (BR-AI-086 AC6) | UT-KA-1935-001/002 assert the captured `Reasoning` block is present in the persisted message history that audit surfacing reads from; IT-KA-1935-010 asserts the shadow agent's own decision trail (grounding review) is now built from that same real history plus the thinking text, not a stale snapshot; UT-KA-1935-011/IT-KA-1935-012..014 assert the console-facing `reasoning_delta` event (the A2A channel's ThinkingPanel feed, all 3 wiring points) carries the model's thinking-block text instead of going silently blank during Sonnet-5 tool-calling turns, so the human-in-the-loop operator has real-time visibility into the model's reasoning, not just a post-hoc audit record. |
| **AU-3** | Content of Audit Records | IT-KA-1935-008/009 assert the gate-retry audit event's `prompt_length`/`prompt_preview` now reflects a prompt that actually includes prior tool-call evidence, not a hardcoded-looking constant regardless of session activity. |
| **CC8.1** | Audit Completeness / RR Reconstruction (BR-AUDIT-005 v2.0) | IT-KA-1935-008/009 assert the LLM call underlying a gate retry is reconstructable with the tool evidence that informed it; IT-KA-1935-010 extends this to the shadow agent's #1096 grounding review, closing the specific gap that made both #1935's audit trail and the shadow agent's own defense-in-depth review look self-contradictory or uninformative. |

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control / BR | Test File |
|---|---|---|---|---|
| UT-KA-1935-001 | Unit | Captures visible thinking text + signature into `Message.Reasoning`, through the real `Client.Chat()` entry point against a fake external Anthropic server (ported from `UT-KA-1580-107`) | BR-AI-086 AC1/AC3 | `pkg/kubernautagent/llm/vertexanthropic/thinking_1935_test.go` |
| UT-KA-1935-002 | Unit | Captures `redacted_thinking` as an opaque, replayable block with no visible text (ported from `UT-KA-1580-108`) | BR-AI-086 AC1/AC3 | `pkg/kubernautagent/llm/vertexanthropic/thinking_1935_test.go` |
| UT-KA-1935-003 | Unit | Leaves `Message.Reasoning` nil when the response contains no thinking block — no regression for non-reasoning responses (ported from `UT-KA-1580-109`) | BR-AI-086 AC1 | `pkg/kubernautagent/llm/vertexanthropic/thinking_1935_test.go` |
| UT-KA-1935-004 | Unit | Replays a visible thinking block first, before `tool_use`, in a multi-turn self-correction retry, through the real `Client.Chat()` entry point (ported from `UT-KA-1580-110`) | BR-AI-086 AC3, #1299-class regression | `pkg/kubernautagent/llm/vertexanthropic/thinking_1935_test.go` |
| UT-KA-1935-005 | Unit | Replays a `redacted_thinking` block first, preserving the opaque payload verbatim (ported from `UT-KA-1580-111`) | BR-AI-086 AC3, #1299-class regression | `pkg/kubernautagent/llm/vertexanthropic/thinking_1935_test.go` |
| UT-KA-1935-006 | Unit | `runLLMLoop` returns a `SubmitResult` whose `Messages` field contains the accumulated prior tool-call/tool-result turns (not including the dangling final `submit_result` call) when the sentinel tool fires after 1+ tool-calling turns | #847, #1935 root cause #2 | `internal/kubernautagent/investigator/investigator_loop_messages_1935_test.go` |
| UT-KA-1935-007 | Unit | `runLLMLoop` returns a `TextResult` whose `Messages` field contains the accumulated prior turns when the final turn is plain text (no tool call) | #847, #1935 root cause #2 | `internal/kubernautagent/investigator/investigator_loop_messages_1935_test.go` |
| IT-KA-1935-008 | Integration | `sameKindValidationGate`'s retry request (captured via a recording mock `llm.Client`) includes the prior turn's real tool-call/tool-result messages, through the real `runRCA` -> `runLLMLoop` -> gate production path — proving the wiring fix, not just the loop's return value | #847, AU-3, CC8.1 | `internal/kubernautagent/investigator/gate_history_propagation_1935_test.go` |
| IT-KA-1935-009 | Integration | `apiVersionValidationGate`'s retry request includes the prior turn's real tool-call/tool-result messages, through the same real production path | #847, AU-3, CC8.1 | `internal/kubernautagent/investigator/gate_history_propagation_1935_test.go` |
| IT-KA-1935-010 | Integration | The shadow agent's grounding-review request (captured via a mock shadow `llm.Client`) includes both the earlier tool-call/tool-result turn and the model's thinking-block text, through the real `Investigate` -> `runRCA` -> `alignment.NotifyRCAComplete` -> `Observer.StartGroundingReview` -> `Evaluator.EvaluateGrounding` production path with a real `Observer`/`Evaluator` pair | #1096, BR-AI-086, CC7.2, CC8.1 | `internal/kubernautagent/investigator/alignment_grounding_propagation_1935_test.go` |
| UT-KA-1935-011 | Unit | `reasoningDeltaText` falls back to `Reasoning.Text` when `Content` is empty, combines both when present (reasoning-then-content), and is a no-op for non-thinking models / redacted blocks / all-empty input | BR-AI-086, CC7.2 | `internal/kubernautagent/investigator/reasoning_delta_thinking_1935_test.go` |
| IT-KA-1935-012 | Integration | The console-facing `EventTypeReasoningDelta` event emitted by `runLLMLoop`'s main turn loop carries the thinking-block text (not an empty string) for a tool-calling turn with empty `Content`, through the real `Investigate` -> event-sink production path | BR-AI-086, CC7.2 | `internal/kubernautagent/investigator/reasoning_delta_console_1935_test.go` |
| IT-KA-1935-013 | Integration | The console-facing `EventTypeReasoningDelta` event emitted by `retryRCASubmit`'s RCA parse-retry turn carries the thinking-block text when `Content` is empty, through the real `Investigate` production path (second of 3 wiring points, proven independently per CHECKPOINT W) | BR-AI-086, CC7.2 | `internal/kubernautagent/investigator/reasoning_delta_console_1935_test.go` |
| IT-KA-1935-014 | Integration | The console-facing `EventTypeReasoningDelta` event emitted by `retryWorkflowSubmit`'s workflow-discovery parse-retry turn carries the thinking-block text when `Content` is empty, through the real `Investigate` production path (third of 3 wiring points) | BR-AI-086, CC7.2 | `internal/kubernautagent/investigator/reasoning_delta_console_1935_test.go` |

**Note on tiering for UT-KA-1935-001..005**: per this codebase's own established convention (`pkg/kubernautagent/llm/anthropicfamily/thinking_1580_test.go`, `pkg/kubernautagent/llm/vertexanthropic/empty_block_1384_test.go`), tests that call the real `vertexanthropic.Client.Chat()` production entry point against an `httptest`-faked *external* Anthropic API are tagged `UT`, not `IT` — per AGENTS.md's Mock Strategy, the LLM API is the one thing that should always be mocked, and there is no separate internal-only seam to unit-test in isolation from `Chat()`.

### Tier Skip Rationale — E2E

`test/infrastructure/fullpipeline_e2e.go` always routes KA through `MockLLM`, and `test/services/mock-llm/response/` only implements Gemini/OpenAI/Ollama wire formats — there is no Anthropic-format mock, so the fullpipeline E2E suite has zero coverage of the `vertexanthropic` path today, independent of this fix (see Section 6 backlog item). This applies equally to the console ThinkingPanel finding (finding #3): there is no E2E path that exercises a real Sonnet-5 thinking-block turn end-to-end through AF's `FormatEventForUser`/A2A artifact stream. In its place, the journey-level proof for both is the live production audit-trail comparison in Section 1, captured from the real `rr-618ac7d3b894-ba320bf0` incident (both models, and for finding #3 specifically, the per-turn `has_analysis`/`tool_call_count` correlation showing 5/5 Sonnet-5 RCA-phase turns with empty `Content` despite active tool calls).

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `ReasoningBlock` type + `Message.Reasoning` field | N/A (pure data type) | `pkg/kubernautagent/llm/types.go` | UT-KA-1935-001..003 |
| `mapResponse` thinking/redacted_thinking capture | `vertexanthropic.Client.Chat`/`StreamChat` | `pkg/kubernautagent/llm/vertexanthropic/client.go` | UT-KA-1935-001/002/003 |
| `convertAssistantMessage`/`reasoningToContentBlock` reasoning-first replay | `vertexanthropic.Client.buildParams` | `pkg/kubernautagent/llm/vertexanthropic/client.go` | UT-KA-1935-004b/005b |
| `SubmitResult.Messages`/`TextResult.Messages` capture | `runLLMLoop` (RCA phase) | `internal/kubernautagent/investigator/investigator.go` | UT-KA-1935-006/007 |
| `runRCA` reassigns `messages` from loop result before shadow notification, gates, and retry | `runRCA` | `internal/kubernautagent/investigator/investigator.go` | IT-KA-1935-008/009/010 |
| `renderConversation` renders `Reasoning.Text` | `Evaluator.EvaluateGrounding` (via `Observer.StartGroundingReview`) | `internal/kubernautagent/alignment/grounding.go` | IT-KA-1935-010 |
| `reasoningDeltaText` (shared helper) | N/A (pure function) | `internal/kubernautagent/investigator/investigator_tools.go` | UT-KA-1935-011 |
| `reasoningDeltaText` wired into main turn loop's `EventTypeReasoningDelta` | `runLLMLoop` | `internal/kubernautagent/investigator/investigator.go` | IT-KA-1935-012 |
| `reasoningDeltaText` wired into RCA parse-retry's `EventTypeReasoningDelta` | `retryRCASubmit` | `internal/kubernautagent/investigator/investigator.go` | IT-KA-1935-013 |
| `reasoningDeltaText` wired into workflow-discovery parse-retry's `EventTypeReasoningDelta` | `retryWorkflowSubmit` | `internal/kubernautagent/investigator/investigator.go` | IT-KA-1935-014 |

## 6. Out of Scope / Tracked Separately

- **Backlog candidate**: extend `test/services/mock-llm` with an Anthropic-format response builder (mirroring `response/gemini.go`/`response/openai.go`) so future Anthropic/Vertex-specific behavior gets real E2E coverage. Not filed as part of this fix to avoid scope creep.
- **Cross-phase Confidence-overwrite finding** (Phase 3 workflow-selection silently overwriting Phase 1 RCA confidence via `mergeNestedSelectedWorkflow`): unrelated to this fix, applies to both branches, will be filed as a separate GitHub issue.
- **`main` (#1936)**: root cause #2 (message-propagation) is confirmed present, byte-for-byte identical, on `main` as well (verified against deployed commit `main-60b899a7b`) — the same fix (Section 2.2) needs to be ported there. Root cause #1 (thinking-block drop) does not apply to `main` (already has the reasoning round-trip under BR-AI-086/#1580).

## 7. Final Results

_To be filled in after implementation and verification (build/lint/test pass, plus confirmation against the `rr-618ac7d3b894-ba320bf0` audit trail)._
