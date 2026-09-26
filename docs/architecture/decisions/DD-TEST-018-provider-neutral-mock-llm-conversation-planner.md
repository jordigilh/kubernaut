# DD-TEST-018: Provider-Neutral Mock LLM Conversation Planner

**Status**: ✅ Approved & Implemented
**Date**: 2026-09-21
**Version**: 1.0
**Scope**: Mock LLM conversation routing, workflow discovery, and provider adapters
**Business Requirements**: BR-MOCK-010, BR-MOCK-012, BR-MOCK-014, BR-TESTING-001
**Related Issue**: #2442

## Context

The Go Mock LLM currently exposes OpenAI Chat Completions, Gemini `generateContent`,
and Ollama-compatible endpoints. It does not expose an Anthropic endpoint. OpenAI and
Gemini therefore need different wire-format serializers, but their workflow-discovery
semantics are the same.

The current implementation mixes protocol translation with conversation semantics:

- OpenAI routes three-step discovery through a generic DAG and tool-result counts.
- Gemini routes three-step discovery through a separate hand-written switch.
- Mode and `force_text` policy is duplicated in both handlers.
- Scenario and workflow override resolution is partly dependent on registration and map
  iteration behavior.

This divergence caused the Issue #2442 failure mode. The OpenAI three-step DAG has a
membership-aware transition, but its generic count fallback can still emit
`get_workflow` when the configured workflow was not present in `list_workflows`. The
Gemini path avoids that call, but can fall through to `submit_result_with_workflow` with
the configured ID without discovery membership. Existing DAG tests also use synthetic
bare tool-result messages, so they validate count progression rather than the actual
provider conversation contract.

The authoritative workflow contract is defined by DD-KA-017 v2.1: a workflow ID must
be returned by `list_workflows` in the current selection context; pagination and
self-correction accumulate membership; `get_workflow` only retrieves the schema and
does not grant membership.

## Proposed Decision

Introduce one provider-neutral workflow-discovery planner in the Mock LLM testing
layer. Keep provider-specific request decoding and response rendering in thin adapters.

```text
OpenAI request ----> OpenAI adapter ----\
                                       +--> canonical transcript
Gemini request ---> Gemini adapter ----/            |
                                                    v
                                       workflow-discovery planner
                                                    |
                                                    v
                                         semantic response plan
                                      /                |       \
                           OpenAI renderer     Gemini renderer  text renderer
```

The planner is the only owner of DD-KA-017 conversation semantics. It must be
stateless: all state is derived from the request transcript supplied by the caller.
No server-side conversation session is authoritative.

### Canonical Transcript

Adapters normalize their wire formats into typed semantic events:

- User and system content.
- Assistant content.
- Assistant tool calls, including the provider tool-call identifier when available.
- Tool results, including the provider tool-call identifier, tool name, and payload.
- Advertised tool names and the current scenario's expected workflow ID.

OpenAI and Gemini remain responsible for parsing their own message/content shapes and
for rendering their own response JSON. The planner must not import OpenAI or Gemini
response types.

### Discovery Planner Contract

The planner returns one semantic response plan per request:

- `CallTool`: call a discovery tool with typed semantic arguments.
- `Text`: return the scenario's final analysis or text response.
- `Unresolved`: terminate discovery safely when the expected workflow cannot be
  established, with a deterministic reason suitable for test diagnostics.

For a configured workflow ID, the state transitions are:

1. Call `get_resource_context` when that optional tool is advertised and required by
   the selected flow.
2. Call `list_available_actions`.
3. Call `list_workflows`.
4. Parse the actual `list_workflows` result. If the target ID is present, call
   `get_workflow`. If it is absent and a next cursor exists, request the next page.
5. If the target is absent and no next cursor exists, return `Unresolved`. Never call
   `get_workflow` and never submit the configured ID as a workflow selection.
6. After the selected workflow definition is returned, produce the final analysis or
   the configured split-submit response.

Membership is accumulated across list pages and self-correction turns in the request
transcript. A `get_workflow` result cannot add membership.

When provider metadata supports direct tool-call correlation, adapters use it. When a
legacy request lacks correlation identifiers, the adapter uses the latest unambiguous
matching tool call/result pair. It must not infer discovery state from total tool-result
counts or from unrelated parallel investigation results.

### Existing Scenario Behavior

- Legacy `search_workflow_catalog` and text-only flows remain supported.
- Explicit non-discovery `ToolCall`, `MultiToolCalls`, and `NextToolCall` overrides
  retain their current behavior.
- An explicit override may not bypass workflow membership by forcing a discovery
  `get_workflow` or workflow submission.
- The generic DAG remains available for legacy and non-discovery flows. The DD-KA-017
  path moves to the typed planner because membership is a semantic invariant, not a
  result-count transition.
- Global `force_text` remains the default for ordinary scenarios. An advertised
  DD-KA-017 tool set may use the planner unless the scenario explicitly sets
  `force_text: true`. Per-scenario configuration remains authoritative.
- Workflow/environment bindings must be deterministic. Exact scenario and
  `workflow:environment` matches take precedence. A name-only fallback is valid only
  when it resolves to one candidate; ambiguous candidates fail configuration rather
  than relying on map iteration or implicit environment preference.

### Provider Scope

The first implementation includes OpenAI and Gemini adapters because both are active
Mock LLM protocols. Ollama remains text-only and does not need a discovery adapter.
Anthropic is explicitly deferred: production Anthropic support exists, but the Mock LLM
does not expose `/v1/messages` and no current Issue #2442 test path requires it. The
canonical planner must remain provider-neutral so an Anthropic adapter can be added
later without changing discovery semantics.

## Alternatives Considered

### Alternative 1: Minimal Guard Fix

Remove the OpenAI count fallback, add an unresolved branch to Gemini, and align the
deployment fixtures with `force_text: false`.

**Advantages**:

- Smallest immediate change.
- Lowest short-term migration risk.

**Disadvantages**:

- Keeps two discovery implementations with different behavior.
- Leaves count-based state in the OpenAI path.
- Makes future provider additions repeat the same logic.
- Does not provide a shared semantic test matrix.

### Alternative 2: Provider-Neutral Discovery Planner

Normalize OpenAI and Gemini requests into a canonical transcript, run one typed
discovery planner, and render the result through thin provider adapters.

**Advantages**:

- One implementation of the DD-KA-017 membership invariant.
- OpenAI and Gemini behavior is tested from the same scenario matrix.
- Provider-specific code stays limited to serialization and extraction.
- Supports future providers without coupling them to OpenAI types.
- Preserves existing legacy and custom scenario paths.

**Disadvantages**:

- Requires a deliberate migration of current OpenAI and Gemini discovery tests.
- Requires typed transcript extraction for two wire formats.
- Temporary coexistence exists while the old DAG path is retired for discovery.

**Recommendation**: Select this alternative.

### Alternative 3: Generalize Every Mock LLM Flow into One Transcript Engine

Replace the DAG, custom chains, discovery, selector scenarios, and provider handlers
with a single generalized state machine.

**Advantages**:

- Maximum theoretical uniformity.

**Disadvantages**:

- Broad rewrite across stable legacy and test-specific behavior.
- High regression risk for replay, custom, parallel, and fault scenarios.
- Solves a larger problem than Issue #2442 requires.

**Disposition**: Defer. Revisit only if multiple additional protocol-specific flows
require the same abstraction.

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Integration Test |
|---|---|---|---|
| Discovery planner | OpenAI chat completion | `test/services/mock-llm/handlers/openai.go` | `IT-MOCK-2442-011` |
| Discovery planner | Gemini `generateContent` | `test/services/mock-llm/handlers/gemini.go` | `IT-MOCK-2442-012` |
| OpenAI transcript adapter and renderer | `/v1/chat/completions`, `/chat/completions` | `test/services/mock-llm/handlers/router.go` and `openai.go` | `IT-MOCK-2442-013` |
| Gemini transcript adapter and renderer | `/v1beta/models/*:generateContent` | `test/services/mock-llm/handlers/router.go` and `gemini.go` | `IT-MOCK-2442-014` |
| Effective mode and `force_text` policy | OpenAI and Gemini request dispatch | `test/services/mock-llm/handlers/openai.go`, `gemini.go` | `IT-MOCK-2442-015` |
| Deterministic workflow/environment override binding | E2E Mock LLM configuration generation | `test/infrastructure/shared_e2e.go`, `test/integration/aianalysis/test_workflows.go` | `IT-MOCK-2442-016` |

## Acceptance Criteria

1. OpenAI and Gemini produce the same semantic discovery sequence for equivalent
   transcripts.
2. `get_workflow` is never emitted unless the target workflow ID appeared in a
   `list_workflows` result in the current transcript.
3. A target found on a later page is selected; all earlier page IDs remain part of the
   current membership context.
4. An exhausted catalog produces an unresolved/no-workflow response without an invalid
   `get_workflow` or workflow submission.
5. Parallel investigation results do not change discovery state.
6. Conversations are isolated by request transcript; no scenario or discovery state
   leaks across requests.
7. Explicit per-scenario `force_text: true` remains authoritative.
8. Existing legacy, text-only, custom-chain, replay, and non-discovery scenarios do not
   regress.
9. Workflow/environment override resolution is deterministic and rejects ambiguity.
10. No Anthropic endpoint is added by this change; the planner remains ready for a
    future Anthropic adapter.

## Implementation Constraints

- Use Ginkgo/Gomega tests with `UT-`, `IT-`, and `E2E-` scenario IDs.
- Mock only external dependencies; keep planner and adapter business logic real.
- Do not introduce a server-side session store.
- Do not remove legacy DAG support until the existing compatibility tests pass through
  the migration.
- Reference `DD-TEST-018` in implementation comments where the planner replaces the
  discovery-specific DAG routing.

## Related Documents

- [DD-KA-017: Three-Step Workflow Discovery Integration](DD-KA-017-three-step-workflow-discovery-integration.md)
- [DD-TEST-016: Explicit Transcript Scenarios for A2A E2E Tests](DD-TEST-016-a2a-transcript-scenario-harness.md)
- [DD-TEST-017: Structured Mock-LLM Scenario Selectors](DD-TEST-017-structured-mock-llm-scenario-selectors.md)
- [Mock LLM Business Requirements](../../services/test-infrastructure/mock-llm/BUSINESS_REQUIREMENTS.md)
- [Issue #2442 Test Plan](../../testing/2442/TEST_PLAN.md)

## Review Gate

This document records the approved design and implementation boundary for Issue #2442.
The shared planner alternative was approved and implemented with OpenAI and Gemini
handler wiring, regression coverage, and provider-correlation hardening.
