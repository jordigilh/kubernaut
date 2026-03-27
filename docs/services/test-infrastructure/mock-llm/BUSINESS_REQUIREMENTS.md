# Mock LLM Service — Business Requirements

**Service**: Mock LLM
**Service Type**: Stateless HTTP Server (Go)
**Version**: v1.0 (Go Rewrite — Issue #531)
**Last Updated**: 2026-03-04
**Status**: 📋 Specification Phase
**Parent Issue**: #531 (Rewrite Mock LLM service in Go)
**Parent Service**: Part of #433 (KAPI Go rewrite)

---

## 📋 Overview

The **Mock LLM Service** is a test infrastructure service that simulates an LLM provider (OpenAI, Ollama) for deterministic, cost-free testing of Kubernaut's AI-driven remediation pipeline. It is consumed by KAPI (via `LLM_ENDPOINT`) and must be indistinguishable from a real LLM provider at the HTTP API level.

### Architecture

**Service Type**: Stateless HTTP Server (Go, single static binary)

**Key Characteristics**:
- OpenAI and Ollama wire-compatible HTTP endpoints
- DAG-based conversation state machine for extensible conversation flow management
- Self-registering scenario catalog with keyword and signal-based detection
- Deterministic UUID generation shared with DataStorage (eliminates ConfigMap sync)
- HTTP verification API for behavioral test assertions
- Fault injection for resilience testing
- Prometheus metrics for observability

**Relationship with Other Services**:
- **KAPI** (#433): Primary consumer — sends chat completion requests, receives tool calls and content responses
- **DataStorage** (#548): Shares deterministic UUID generation function for workflow identity consistency
- **AIAnalysis Controller**: Indirect consumer via KAPI — validates end-to-end remediation pipeline
- **Integration/E2E Tests**: Direct consumer via verification API for behavioral assertions

### Service Responsibilities

1. **LLM API Simulation**: Serve OpenAI and Ollama-compatible endpoints with deterministic responses
2. **Conversation Management**: Execute multi-turn conversation protocols via DAG state machine
3. **Scenario Routing**: Detect and route to appropriate scenario based on message content
4. **Test Verification**: Expose assertion API for tool call sequences, DAG paths, and scenario detection
5. **Fault Simulation**: Inject configurable failure modes for resilience testing
6. **Observability**: Expose Prometheus metrics for debugging and monitoring

---

## 🎯 Business Requirements

### 📊 Summary

**Total Business Requirements**: 49 BRs across 10 categories
**Priority Breakdown**:
- P0 (Critical): 25 BRs — core functionality required for test parity with Python Mock LLM
- P1 (High): 16 BRs — architectural improvements enabling extensibility
- P2 (Medium): 8 BRs — enhancements for future pillars and advanced testing

**Implementation Status**: All 📋 Planned (Go rewrite not yet started)

**Traceability**: Each BR maps to one or more GitHub sub-issues under #531:

| Sub-Issue | BRs Covered |
|-----------|-------------|
| #560 — DAG conversation engine | BR-MOCK-010 through BR-MOCK-015 |
| #561 — Deterministic UUIDs | BR-MOCK-030, BR-MOCK-031 |
| #562 — Shared OpenAI types | BR-MOCK-060 through BR-MOCK-062 |
| #563 — HTTP verification API | BR-MOCK-040 through BR-MOCK-044 |
| #564 — Scenario registry | BR-MOCK-020 through BR-MOCK-026 |
| #565 — Fault injection | BR-MOCK-050 through BR-MOCK-054 |
| #566 — Declarative YAML scenarios | BR-MOCK-071, BR-MOCK-072 |
| #567 — Pillar composition | BR-MOCK-070, BR-MOCK-073 |
| #568 — Prometheus metrics | BR-MOCK-080 through BR-MOCK-083 |
| #570 — Auth header passthrough | BR-MOCK-006, BR-MOCK-007 |

---

### Category 1: HTTP API Compatibility (BR-MOCK-001 to BR-MOCK-007)

#### BR-MOCK-001: OpenAI Chat Completions Endpoint

**Description**: The Mock LLM service MUST serve OpenAI-compatible chat completions endpoints that accept and respond with OpenAI JSON, including `messages`, `tools`, `tool_calls`, and `choices` structures.

**Priority**: P0 (CRITICAL)

**Rationale**: KAPI sends requests to this endpoint. The mock must be a transparent drop-in for `LLM_ENDPOINT`.

**Acceptance Criteria**:
- [ ] `POST /v1/chat/completions` accepts `ChatCompletionRequest` with `model`, `messages`, `tools` fields
- [ ] `POST /chat/completions` (without `/v1` prefix) also accepted for compatibility
- [ ] Returns `ChatCompletionResponse` with `id`, `object`, `created`, `model`, `choices`, `usage` fields
- [ ] Supports `tool_calls` in response (function name, arguments as JSON string)
- [ ] Supports `content` in response (text/markdown with embedded JSON)
- [ ] Returns appropriate `finish_reason` (`stop`, `tool_calls`)
- [ ] `GET /v1/models` returns model list with at least one entry (e.g., `mock-model`)
- [ ] Response `id` format: `chatcmpl-` + 8 hex characters (UUID-derived)
- [ ] Tool call `id` format: `call_` + 12 hex characters (UUID-derived)
- [ ] Response `created` field: fixed timestamp for determinism (currently `1701388800`)
- [ ] Response `usage` tokens: fixed values per response type for determinism
- [ ] Response `Content-Type: application/json`

**Implementation Status**: 📋 Planned

**Related Issues**: #531, #562 (shared types)

**Audit Note**: Python also accepts POST to any unknown path and returns `{"status":"ok","path":"<path>"}` (200). The Go rewrite SHOULD return 404 for unknown paths instead (see BR-MOCK-005).

---

#### BR-MOCK-002: Ollama API Endpoints

**Description**: The Mock LLM service MUST serve Ollama-compatible endpoints (`POST /api/chat`, `POST /api/generate`) for environments using local Ollama LLM providers.

**Priority**: P0 (CRITICAL)

**Rationale**: Kubernaut supports Ollama as an LLM provider. The mock must handle both API formats.

**Acceptance Criteria**:
- [ ] `POST /api/chat` accepts Ollama chat format and returns Ollama-compatible response
- [ ] `POST /api/generate` accepts Ollama generate format and returns Ollama-compatible response
- [ ] Both endpoints return `done: true` for non-streaming responses
- [ ] `GET /api/tags` returns model list (`{"models":[{"name":"mock-model","size":1000000}]}`)
- [ ] Ollama responses include fixed fields for determinism: `created_at`, `total_duration`, `load_duration`, `prompt_eval_count`, `eval_count`, `context: []`

**Implementation Status**: 📋 Planned

**Related Issues**: #531

---

#### BR-MOCK-003: Health Endpoint

**Description**: The Mock LLM service MUST expose a `GET /health` endpoint returning HTTP 200 when the service is ready to accept requests.

**Priority**: P0 (CRITICAL)

**Rationale**: Kubernetes readiness and liveness probes depend on this endpoint for deployment lifecycle management.

**Acceptance Criteria**:
- [ ] Returns HTTP 200 with `{"status": "ok"}` when ready
- [ ] Usable as both liveness and readiness probe target
- [ ] Available within 1 second of server startup

**Implementation Status**: 📋 Planned

**Related Issues**: #531

---

#### BR-MOCK-004: Force Text Response Mode

**Description**: The Mock LLM service MUST support a `MOCK_LLM_FORCE_TEXT` environment variable that, when set, forces all responses to use plain text content instead of tool calls.

**Priority**: P0 (CRITICAL)

**Rationale**: Some test scenarios validate KAPI's handling of text-only LLM responses (no tool calling). This mode ensures coverage of the non-tool-call code path.

> **Audit Finding**: The Python Mock LLM declares `MOCK_LLM_FORCE_TEXT` in deployment manifests and Go infrastructure, but the **Python server never reads this env var**. The `force_text_response` flag is only settable via the in-process `MockLLMServer(..., force_text_response=True)` constructor. The Go rewrite MUST fix this gap by actually reading the environment variable.

**Acceptance Criteria**:
- [ ] When `MOCK_LLM_FORCE_TEXT=true`, all responses use `content` field instead of `tool_calls`
- [ ] JSON structures are embedded in markdown code fences within text content
- [ ] Behavior reverts to normal when env var is unset or `false`
- [ ] **Fix Python gap**: env var is actually read at startup (not just declared in manifests)

**Implementation Status**: 📋 Planned

**Related Issues**: #531

---

#### BR-MOCK-005: Strict HTTP Routing

**Description**: The Mock LLM service MUST return appropriate HTTP error responses for unknown paths and unsupported methods, replacing the Python implementation's permissive behavior.

**Priority**: P1 (HIGH)

**Rationale**: The Python Mock LLM returns `{"status":"ok"}` for ANY GET path and `{"status":"ok","path":"<path>"}` for ANY POST path, which masks routing errors. The Go rewrite should be strict.

> **Audit Finding**: The permissive routing means `/liveness`, `/readiness`, `/metrics`, and any typo'd path all return 200 OK — hiding misconfigurations. The static manifests in `deploy/mock-llm/01-deployment.yaml` probe `/liveness` and `/readiness` which work only because of this permissiveness.

**Acceptance Criteria**:
- [ ] Unknown GET paths return HTTP 404 with JSON error body
- [ ] Unknown POST paths return HTTP 404 with JSON error body
- [ ] Unsupported HTTP methods (PUT, DELETE, OPTIONS on non-test paths) return HTTP 405
- [ ] `/health` is the canonical probe path; `/liveness` and `/readiness` are NOT supported (deployment manifests must be updated)
- [ ] Structured JSON error responses on all error paths

**Implementation Status**: 📋 Planned

**Related Issues**: #531

---

#### BR-MOCK-006: Auth Header Passthrough

**Description**: The Mock LLM service MUST accept and record custom authentication headers sent by KAPI (#417) without rejecting requests that lack them.

**Priority**: P0 (CRITICAL)

**Rationale**: KAPI v1.3 (#417) injects custom HTTP headers (Authorization, x-api-key, x-tenant-id, filePath-sourced JWTs) into all outbound LLM requests via an `http.RoundTripper` wrapper. The Mock LLM must accept these headers transparently so integration tests can verify the header injection works end-to-end. Existing tests that send no auth headers must continue to work.

**Acceptance Criteria**:
- [ ] Requests with custom auth headers are accepted and processed normally (no validation, no rejection)
- [ ] Requests without auth headers continue to work identically (backward compatibility)
- [ ] Configurable set of header names recorded per request via `MOCK_LLM_RECORD_HEADERS` env var (comma-separated)
- [ ] Default recorded headers: `Authorization`, `x-api-key`, `x-tenant-id`, and any header starting with `x-kubernaut-`
- [ ] Headers recorded per request with request sequence number and conversation ID
- [ ] Sensitive header values redacted from server logs (exposed only via verification API)
- [ ] Thread-safe recording for concurrent requests

**Implementation Status**: 📋 Planned

**Related Issues**: #570, #417, #531

---

#### BR-MOCK-007: Auth Header Verification API

**Description**: The Mock LLM service MUST expose recorded auth headers via the verification API (`GET /api/test/headers`) and provide Go test helper assertions for header verification.

**Priority**: P1 (HIGH)

**Rationale**: #417's acceptance criterion requires "integration test with mock LLM verifying headers are received." Without an HTTP-accessible API, Go tests cannot assert that KAPI's `http.RoundTripper` wrapper correctly injected the expected headers.

**Acceptance Criteria**:
- [ ] `GET /api/test/headers` returns all recorded headers across all requests with request sequence and conversation ID
- [ ] `GET /api/test/headers?name=Authorization` filters by header name
- [ ] `POST /api/test/reset` clears recorded headers (extends BR-MOCK-043)
- [ ] Go test helper: `AssertHeaderReceived(name, matcher)` — assert a header was received with a value matching the Gomega matcher
- [ ] Go test helper: `AssertNoHeaderReceived(name)` — assert a header was NOT received (negative test)
- [ ] Response format includes `request_sequence`, `conversation_id`, and `headers` map per request

**Implementation Status**: 📋 Planned

**Related Issues**: #570, #417, #563

---

### Category 2: Conversation Engine (BR-MOCK-010 to BR-MOCK-015)

#### BR-MOCK-010: DAG-Based Conversation State Machine

**Description**: The Mock LLM service MUST implement conversation flow management using a Directed Acyclic Graph (DAG) where nodes represent conversation states and edges represent transitions triggered by request conditions.

**Priority**: P0 (CRITICAL)

**Rationale**: Replaces the hardcoded if/else routing in the Python Mock LLM (`_handle_openai_request`, `_is_phase3_request`) with a testable, extensible, and declarative structure. Each conversation mode (legacy, three-step, three-phase) is a separate DAG definition.

**Acceptance Criteria**:
- [ ] DAG engine accepts a graph definition with named nodes and typed transitions
- [ ] Each node has a response handler (tool call, content, error)
- [ ] Transitions evaluate conditions on `ConversationContext` (message history, tool results, markers)
- [ ] DAG path traversal is recorded for the verification API (BR-MOCK-041)
- [ ] Engine supports multiple concurrent conversations without state leakage

**Implementation Status**: 📋 Planned

**Related Issues**: #560

---

#### BR-MOCK-011: Legacy Conversation Mode

**Description**: The Mock LLM service MUST support the legacy `search_workflow_catalog` single-step workflow discovery mode.

**Priority**: P0 (CRITICAL)

**Rationale**: Backward compatibility. Existing tests may still use the legacy mode until fully migrated to three-step discovery.

**Acceptance Criteria**:
- [ ] Detects legacy mode when request includes `search_workflow_catalog` tool definition
- [ ] Returns single tool call to `search_workflow_catalog` with matching workflow
- [ ] Second turn returns content response with RCA and workflow selection

**Implementation Status**: 📋 Planned

**Related Issues**: #560

---

#### BR-MOCK-012: Three-Step Discovery Mode (DD-HAPI-017)

**Description**: The Mock LLM service MUST support the three-step workflow discovery protocol: `list_available_actions` → `list_workflows` → `get_workflow`.

**Priority**: P0 (CRITICAL)

**Rationale**: Implements the DD-HAPI-017 protocol. KAPI registers these three tools and expects the LLM to call them in sequence.

**Acceptance Criteria**:
- [ ] First turn: returns `list_available_actions` tool call
- [ ] After tool result: returns `list_workflows` tool call for the selected action type
- [ ] After tool result: returns `get_workflow` tool call for the selected workflow
- [ ] Final turn: returns content response with RCA and workflow selection
- [ ] Detection: activates when request includes three-step tool definitions and tool results count matches expected progression

**Implementation Status**: 📋 Planned

**Related Issues**: #560, DD-HAPI-017

---

#### BR-MOCK-013: Three-Phase RCA Mode (#529)

**Description**: The Mock LLM service MUST support the three-phase RCA protocol: Phase 1 (root cause analysis) → Phase 2 (enrichment, handled by KAPI) → Phase 3 (workflow selection with enriched context).

**Priority**: P0 (CRITICAL)

**Rationale**: Implements the #529 three-phase architecture. Phase 2 is executed by KAPI (not the LLM), so the mock handles Phase 1 and Phase 3 responses.

**Acceptance Criteria**:
- [ ] Phase 1: returns structured RCA content with `root_cause_analysis`, `remediation_target`, and `contributing_factors`
- [ ] Phase 3 detection: message contains markers (`## Enrichment Context (Phase 2`, `## Phase 1 Root Cause Analysis`, `**Root Owner**:`)
- [ ] Phase 3: returns content with `selected_workflow` including workflow UUID, parameters, and justification
- [ ] Phase 3 response uses enrichment context provided in the request (root owner, resource details)

**Implementation Status**: 📋 Planned

**Related Issues**: #560, #529

---

#### BR-MOCK-014: Conversation Context Tracking

**Description**: The Mock LLM service MUST maintain a `ConversationContext` per request that tracks message history, extracted tool results, detected scenario, current DAG node, and metadata.

**Priority**: P0 (CRITICAL)

**Rationale**: The DAG engine and transition conditions operate on conversation context. Without it, stateful conversation decisions cannot be made.

**Acceptance Criteria**:
- [ ] Context includes full message history from the request
- [ ] Context extracts and counts tool result messages
- [ ] Context identifies presence of three-step tools, resource context tools, and Phase 3 markers
- [ ] Context extracts resource names, namespaces, and root owner information from messages
- [ ] Context is created fresh for each request (stateless server, state from message history)

**Implementation Status**: 📋 Planned

**Related Issues**: #560

---

#### BR-MOCK-015: DAG Path Recording

**Description**: The Mock LLM service MUST record the DAG nodes traversed during each conversation for debugging and verification purposes.

**Priority**: P1 (HIGH)

**Rationale**: Enables the verification API (BR-MOCK-041) to report which conversation path was taken, aiding in test debugging when assertions fail.

**Acceptance Criteria**:
- [ ] Each conversation records: starting node, transition conditions evaluated, nodes visited, final node
- [ ] Path recording is accessible via the verification API
- [ ] Path recording is cleared on test reset (BR-MOCK-043)

**Implementation Status**: 📋 Planned

**Related Issues**: #560, #563

---

### Category 3: Scenario Management (BR-MOCK-020 to BR-MOCK-026)

#### BR-MOCK-020: Scenario Registry with Self-Registration

**Description**: The Mock LLM service MUST implement a scenario registry where each scenario is a self-contained Go type that registers itself via `init()`. Adding a new scenario requires only creating a new file — no modifications to existing code.

**Priority**: P0 (CRITICAL)

**Rationale**: Replaces the monolithic `MOCK_SCENARIOS` dict. The Open/Closed Principle enables future pillar scenarios (security, cost) without touching core code.

**Acceptance Criteria**:
- [ ] `Scenario` interface with `Name()`, `Match()`, `DAG()`, `Metadata()` methods
- [ ] Global `ScenarioRegistry` with `Register()`, `Detect()`, `Get()`, `List()` methods
- [ ] Each scenario in its own file with `init()` registration
- [ ] `registry.List()` returns metadata for all registered scenarios (auto-documentation)

**Implementation Status**: 📋 Planned

**Related Issues**: #564

---

#### BR-MOCK-021: Keyword-Based Scenario Detection

**Description**: The Mock LLM service MUST detect scenarios by matching keywords in the combined message content (e.g., `mock_oomkilled`, `mock_no_workflow_found`, `mock rca permanent error`).

**Priority**: P0 (CRITICAL)

**Rationale**: Primary detection method used by integration and E2E tests. Tests inject keywords into alert descriptions to trigger specific mock behaviors.

**Acceptance Criteria**:
- [ ] Keywords matched case-insensitively against concatenated message content
- [ ] Both underscore (`mock_oomkilled`) and space (`mock oomkilled`) variants supported
- [ ] Keyword match takes highest priority (confidence 1.0) in scenario resolution

**Implementation Status**: 📋 Planned

**Related Issues**: #564

---

#### BR-MOCK-022: Signal Name-Based Scenario Detection

**Description**: The Mock LLM service MUST detect scenarios by extracting and matching the signal name from KAPI's structured prompt (pattern: `Signal Name:\s*(\w+)`).

**Priority**: P0 (CRITICAL)

**Rationale**: Real KAPI prompts include the signal name. This detection path handles production-like scenarios where no mock keyword is injected.

**Acceptance Criteria**:
- [ ] Extracts signal name via regex from message content
- [ ] Normalizes signal tokens (e.g., `OOMKilled` → `oomkilled`, `CrashLoopBackOff` → `crashloop`)
- [ ] Maps normalized tokens to scenarios (e.g., `oomkilled` → OOMKilled scenario)
- [ ] Signal name match has lower priority than keyword match (confidence 0.9)

**Implementation Status**: 📋 Planned

**Related Issues**: #564

---

#### BR-MOCK-023: Proactive Scenario Detection

**Description**: The Mock LLM service MUST detect proactive mode signals by matching proactive markers in message content (`proactive mode`, `proactive signal`, `predicted`, `not yet occurred`).

**Priority**: P0 (CRITICAL)

**Rationale**: Kubernaut supports proactive remediation where signals are predictive. The mock must return proactive-specific responses with appropriate messaging.

**Acceptance Criteria**:
- [ ] Detects proactive markers in message content
- [ ] Routes to proactive variant of the matched scenario (e.g., `oomkilled_proactive`)
- [ ] Proactive responses include predictive copy ("predicted to occur", "preventive action")
- [ ] Special case: `predictive_no_action` / `mock_predictive_no_action` returns no-action response

**Implementation Status**: 📋 Planned

**Related Issues**: #564

---

#### BR-MOCK-024: Default Scenario Fallback

**Description**: The Mock LLM service MUST provide a default scenario when no keyword, signal name, or proactive marker matches.

**Priority**: P0 (CRITICAL)

**Rationale**: Tests or exploratory requests that don't match any specific scenario should still receive a valid response.

**Acceptance Criteria**:
- [ ] Default scenario returns a generic OOMKilled-style response
- [ ] Default scenario has the lowest priority in detection
- [ ] Default scenario supports all three conversation modes

**Implementation Status**: 📋 Planned

**Related Issues**: #564

---

#### BR-MOCK-025: Scenario Priority Resolution

**Description**: When multiple scenarios match a request, the Mock LLM service MUST resolve to a single scenario using a defined priority: keyword (1.0) > signal name (0.9) > proactive (0.8) > fallback (0.0).

**Priority**: P1 (HIGH)

**Rationale**: Ensures deterministic behavior when test prompts contain multiple matching signals.

**Acceptance Criteria**:
- [ ] Each scenario's `Match()` returns a confidence score
- [ ] Registry selects the highest-confidence match
- [ ] On tie, registration order breaks the tie
- [ ] Detection reason (keyword/signal/proactive/fallback) is recorded for verification API

**Implementation Status**: 📋 Planned

**Related Issues**: #564

---

#### BR-MOCK-026: Pre-Defined Scenario Catalog

**Description**: The Mock LLM service MUST ship with all existing Python scenarios migrated to Go, preserving identical behavior for each.

**Priority**: P0 (CRITICAL)

**Rationale**: Zero test regressions. Every existing integration and E2E test that uses the Mock LLM must pass without modification.

**Scenarios**:

| Scenario | Keywords | Signal Pattern | Workflow | Engine |
|----------|----------|---------------|----------|--------|
| `oomkilled` | `mock_oomkilled` | `oomkill`, `oom killed` | `oom-recovery` | job |
| `crashloop` | `mock_crashloop` | `crashloop`, `backoff` | `crashloop-config-fix` | job |
| `node_not_ready` | `mock_node_not_ready` | `nodenotready` | `node-drain-reboot` | ansible |
| `no_workflow_found` | `mock_no_workflow_found` | — | — | — |
| `low_confidence` | `mock_low_confidence` | — | varies | varies |
| `problem_resolved` | `mock_problem_resolved`, `mock_not_reproducible` | — | — | — |
| `problem_resolved_contradiction` | `mock_problem_resolved_contradiction` | — | — | — |
| `max_retries_exhausted` | `mock_max_retries_exhausted` | — | varies | varies |
| `rca_incomplete` | `mock_rca_incomplete` | — | — | — |
| `cert_not_ready` | `mock_cert_not_ready` | `certmanagercertnotready` | varies | varies |
| `test_signal` | `mock_test_signal` | — | `generic-restart` | job |
| `oomkilled_proactive` | proactive markers + OOM | — | `memory-optimize` | job |
| `node_not_ready_proactive` | proactive markers + node | — | `node-drain-reboot` | ansible |
| `predictive_no_action` | `mock_predictive_no_action` | — | — | — |
| `default` | (fallback) | — | `oom-recovery` | job |

**Scenario Metadata Fields** (per scenario, affecting response content):

| Field | Description | Example |
|-------|-------------|---------|
| `workflow_name` | DataStorage workflow name | `oom-recovery` |
| `workflow_id` | UUID (deterministic from name + env) | computed |
| `execution_engine` | Engine type for the workflow | `job`, `ansible` |
| `contributing_factors` | RCA contributing factors list | `["container memory limit too low"]` |
| `needs_human_review_override` | Force human review flag | `true` (low_confidence) |
| `include_affected_resource` | Include affected resource in RCA | `true` / `false` |
| `rca_override_prompt_resource` | Override resource in RCA prompt | resource name string |
| `rca_resource_api_version` | API version for remediation target | `apps/v1` |

> **Audit Finding**: `mock_not_reproducible` is an undocumented alias that maps to `problem_resolved`. The `low_confidence` scenario includes alternative workflow UUIDs containing non-valid hex characters (`7cg3`) — tests may depend on these exact strings.

**Acceptance Criteria**:
- [ ] All 15+ scenarios migrated with identical detection rules and response shapes
- [ ] All scenario metadata fields preserved (not just workflow name/UUID)
- [ ] Keyword aliases preserved (`mock_not_reproducible` → `problem_resolved`)
- [ ] Each scenario in its own Go file under `internal/scenarios/`
- [ ] All existing integration tests pass without modification
- [ ] All existing E2E tests pass without modification

**Implementation Status**: 📋 Planned

**Related Issues**: #531, #564

---

### Category 4: Configuration & UUID Management (BR-MOCK-030 to BR-MOCK-033)

#### BR-MOCK-030: Deterministic UUID Generation

**Description**: The Mock LLM service MUST compute workflow UUIDs deterministically from workflow name and environment using the same function as DataStorage, eliminating the need for external configuration files or ConfigMap synchronization.

**Priority**: P0 (CRITICAL)

**Rationale**: Replaces the entire ConfigMap sync infrastructure (DD-TEST-011 v3.0): `load_scenarios_from_file()`, `MOCK_LLM_CONFIG_PATH`, `UpdateMockLLMConfigMap`, `WriteMockLLMConfigFile`, `SortedWorkflowUUIDKeys`, rollout restart cycle. The mock and DataStorage independently produce the same UUIDs.

**Acceptance Criteria**:
- [ ] Uses `pkg/shared/uuid.DeterministicUUID(workflowName, environment)` (or equivalent from #548)
- [ ] UUIDs match DataStorage-seeded values without any external input
- [ ] No `MOCK_LLM_CONFIG_PATH` environment variable required
- [ ] No ConfigMap volume mount required
- [ ] No rollout restart after DataStorage seeding

**Implementation Status**: 📋 Planned (depends on #548)

**Related Issues**: #561, #548

---

#### BR-MOCK-031: Shared UUID Function with DataStorage

**Description**: The deterministic UUID function MUST reside in a shared Go package importable by both DataStorage and Mock LLM.

**Priority**: P0 (CRITICAL)

**Rationale**: Single source of truth. If the UUID algorithm changes, both services update automatically.

**Acceptance Criteria**:
- [ ] Shared package at `pkg/shared/uuid/` or equivalent
- [ ] Function accepts workflow name and environment, returns UUID string
- [ ] Both DataStorage and Mock LLM import and use the same function
- [ ] Unit tests verify consistency between the two consumers

**Implementation Status**: 📋 Planned (depends on #548)

**Related Issues**: #561, #548

---

#### BR-MOCK-032: Environment Variable Configuration

**Description**: The Mock LLM service MUST support configuration via environment variables for runtime behavior that varies across deployment contexts.

**Priority**: P0 (CRITICAL)

**Acceptance Criteria**:
- [ ] `MOCK_LLM_HOST` — bind address (default: `0.0.0.0`)
- [ ] `MOCK_LLM_PORT` — listen port (default: `8080`)
- [ ] `MOCK_LLM_FORCE_TEXT` — force text-only responses (default: `false`)
- [ ] `MOCK_LLM_LOG_LEVEL` — log verbosity (default: `info`)

**Implementation Status**: 📋 Planned

**Related Issues**: #531

---

#### BR-MOCK-033: Optional YAML Scenario Overrides

**Description**: The Mock LLM service MAY support an optional YAML configuration file for overriding scenario behavior (e.g., forcing a specific workflow UUID for a scenario during manual testing).

**Priority**: P2 (MEDIUM)

**Rationale**: While deterministic UUIDs handle the standard case, manual testing and debugging may benefit from explicit overrides. This is optional and backward-compatible.

**Acceptance Criteria**:
- [ ] If `MOCK_LLM_CONFIG_PATH` is set and file exists, overrides are applied
- [ ] Overrides merge on top of deterministic defaults (overrides win)
- [ ] If file is absent or env var is unset, service starts normally with deterministic UUIDs
- [ ] Clear log message indicating overrides are active

**Implementation Status**: 📋 Planned

**Related Issues**: #561

---

### Category 5: Verification & Assertion API (BR-MOCK-040 to BR-MOCK-044)

#### BR-MOCK-040: Tool Call Recording and Query

**Description**: The Mock LLM service MUST record all tool calls generated in responses and expose them via `GET /api/test/tool-calls` for Go test assertions.

**Priority**: P1 (HIGH)

**Rationale**: Currently, `ToolCallTracker` is only accessible in-process (Python). Go E2E tests can only assert end-state. This API enables behavioral assertions from Go tests.

**Acceptance Criteria**:
- [ ] Records tool name, arguments (JSON), call ID, timestamp, sequence number, conversation ID
- [ ] `GET /api/test/tool-calls` returns all recorded tool calls sorted by sequence
- [ ] Supports filtering by conversation ID (query parameter)
- [ ] Thread-safe recording for concurrent requests

**Implementation Status**: 📋 Planned

**Related Issues**: #563

---

#### BR-MOCK-041: Conversation Path Query

**Description**: The Mock LLM service MUST expose `GET /api/test/conversations` and `GET /api/test/conversations/{id}/path` returning the DAG nodes traversed during each conversation.

**Priority**: P1 (HIGH)

**Rationale**: When E2E tests fail, this endpoint answers "which conversation path did the Mock LLM take?" — critical for debugging three-phase vs. legacy flow mismatches.

**Acceptance Criteria**:
- [ ] Returns conversation list with scenario matched, request count, and DAG path
- [ ] Per-conversation path includes: nodes visited, transitions taken, conditions evaluated
- [ ] Path query returns 404 for unknown conversation IDs

**Implementation Status**: 📋 Planned

**Related Issues**: #563, #560

---

#### BR-MOCK-042: Scenario Detection Query

**Description**: The Mock LLM service MUST expose `GET /api/test/scenarios/matched` returning scenario detection results (scenario name, detection method, confidence, evidence).

**Priority**: P1 (HIGH)

**Rationale**: Answers "why did the Mock LLM pick this scenario?" — critical for debugging keyword vs. signal name detection mismatches.

**Acceptance Criteria**:
- [ ] Returns list of scenario detection events with name, method (keyword/signal/proactive/fallback), confidence, and matching evidence
- [ ] Evidence includes the matched keyword or extracted signal name

**Implementation Status**: 📋 Planned

**Related Issues**: #563

---

#### BR-MOCK-043: State Reset for Test Isolation

**Description**: The Mock LLM service MUST expose `POST /api/test/reset` that clears all tracked state (tool calls, conversations, scenario matches, fault injections) for test isolation.

**Priority**: P1 (HIGH)

**Rationale**: Without reset, state leaks between test cases cause flaky assertions. Tests should call reset in `BeforeEach`.

**Acceptance Criteria**:
- [ ] Clears tool call records
- [ ] Clears conversation history
- [ ] Clears scenario detection events
- [ ] Clears injected faults (BR-MOCK-050)
- [ ] Returns HTTP 200 with confirmation

**Implementation Status**: 📋 Planned

**Related Issues**: #563

---

#### BR-MOCK-044: Go Test Helper Package

**Description**: The Mock LLM service MUST provide a Go test helper package (`test/testutil/mockllm/`) with assertion functions for use in Ginkgo/Gomega test suites.

**Priority**: P1 (HIGH)

**Rationale**: Raw HTTP calls to the verification API are verbose. A helper package provides idiomatic Go assertions.

**Acceptance Criteria**:
- [ ] `MockLLMClient` type wrapping HTTP calls to verification endpoints
- [ ] `AssertToolCalled(name)` — assert a tool was called at least once
- [ ] `AssertToolSequence(names...)` — assert tools were called in order
- [ ] `AssertScenarioMatched(name)` — assert specific scenario was detected
- [ ] `AssertDAGPath(nodes...)` — assert specific DAG path was traversed
- [ ] `Reset()` — reset state between test cases

**Implementation Status**: 📋 Planned

**Related Issues**: #563

---

### Category 6: Fault Injection (BR-MOCK-050 to BR-MOCK-054)

#### BR-MOCK-050: Runtime Fault Injection API

**Description**: The Mock LLM service MUST expose `POST /api/test/inject-fault` and `DELETE /api/test/faults` for configuring failure modes at runtime.

**Priority**: P2 (MEDIUM)

**Rationale**: Enables testing of KAPI's retry logic, error handling, and circuit breaker behavior against realistic LLM failure patterns.

**Acceptance Criteria**:
- [ ] `POST /api/test/inject-fault` accepts fault type, count (optional), delay (optional), scenario filter (optional)
- [ ] `DELETE /api/test/faults` clears all injected faults
- [ ] Faults are cleared on `POST /api/test/reset` (BR-MOCK-043)
- [ ] Multiple fault types can be active simultaneously

**Implementation Status**: 📋 Planned

**Related Issues**: #565

---

#### BR-MOCK-051: Timeout Simulation

**Description**: The Mock LLM service MUST support a `timeout` fault that delays the response by a configurable duration.

**Priority**: P2 (MEDIUM)

**Acceptance Criteria**:
- [ ] Configurable delay in milliseconds
- [ ] Delay applied before sending response (not connection delay)
- [ ] Client may timeout before response arrives (depends on client timeout setting)

**Implementation Status**: 📋 Planned

**Related Issues**: #565

---

#### BR-MOCK-052: Rate Limit Simulation (HTTP 429)

**Description**: The Mock LLM service MUST support a `rate_limit` fault that returns HTTP 429 Too Many Requests with a `Retry-After` header.

**Priority**: P2 (MEDIUM)

**Acceptance Criteria**:
- [ ] Returns HTTP 429 with OpenAI-compatible error JSON
- [ ] Includes `Retry-After` header (configurable seconds)
- [ ] Supports `count` parameter (fail N times, then succeed)

**Implementation Status**: 📋 Planned

**Related Issues**: #565

---

#### BR-MOCK-053: Intermittent Failure Mode

**Description**: The Mock LLM service MUST support an `intermittent` fault that fails a configurable number of times before succeeding.

**Priority**: P2 (MEDIUM)

**Rationale**: Tests KAPI's retry logic with realistic intermittent failure patterns.

**Acceptance Criteria**:
- [ ] Configurable failure count before success
- [ ] Configurable failure type (500, 429, timeout, malformed JSON)
- [ ] Thread-safe counter tracks attempts per conversation or globally

**Implementation Status**: 📋 Planned

**Related Issues**: #565

---

#### BR-MOCK-054: Permanent Error (HTTP 500)

**Description**: The Mock LLM service MUST support permanent error simulation via the `mock_rca_permanent_error` keyword (backward-compatible) and via the fault injection API.

**Priority**: P0 (CRITICAL)

**Rationale**: Backward compatibility with existing tests that trigger HTTP 500 via keyword injection.

**Acceptance Criteria**:
- [ ] `mock_rca_permanent_error` keyword in messages triggers HTTP 500 with OpenAI error JSON
- [ ] Same behavior achievable via fault injection API (`server_error` fault type)
- [ ] Error response includes `error.message`, `error.type`, `error.code` fields

**Implementation Status**: 📋 Planned

**Related Issues**: #565, #531

---

### Category 7: Shared Types (BR-MOCK-060 to BR-MOCK-062)

#### BR-MOCK-060: Shared OpenAI Request/Response Types

**Description**: The Mock LLM service MUST use shared Go types from `pkg/shared/types/openai/` for all OpenAI-compatible request and response construction.

**Priority**: P1 (HIGH)

**Rationale**: Compile-time contract enforcement. If KAPI changes a field name, Mock LLM fails to build. No more drift between mock and consumer.

**Acceptance Criteria**:
- [ ] Shared package at `pkg/shared/types/openai/` with `ChatCompletionRequest`, `ChatCompletionResponse`, `Choice`, `Message`, `Usage`
- [ ] Mock LLM imports and uses shared types for response construction
- [ ] KAPI (#433) imports same types for request construction
- [ ] No string-literal OpenAI field names in Mock LLM handler code

**Implementation Status**: 📋 Planned

**Related Issues**: #562, #433

---

#### BR-MOCK-061: Shared Tool Definition Constants

**Description**: Tool names used by both KAPI and Mock LLM MUST be defined as constants in a shared package.

**Priority**: P1 (HIGH)

**Rationale**: Prevents name drift (e.g., `search_workflow_catalog` vs `searchWorkflowCatalog`) between the mock and its consumers.

**Acceptance Criteria**:
- [ ] Constants for: `search_workflow_catalog`, `list_available_actions`, `list_workflows`, `get_workflow`
- [ ] Constants for resource context tools: `fetchKubernetesResourceYaml`, `listNamespacedEvents`, etc.
- [ ] Both Mock LLM and KAPI import from the same package

**Implementation Status**: 📋 Planned

**Related Issues**: #562

---

#### BR-MOCK-062: Shared Ollama Types

**Description**: Ollama request/response types MUST also be defined in the shared package for consistency.

**Priority**: P1 (HIGH)

**Acceptance Criteria**:
- [ ] `OllamaChatRequest`, `OllamaChatResponse`, `OllamaGenerateRequest`, `OllamaGenerateResponse` types
- [ ] Used by Mock LLM for Ollama endpoint handlers

**Implementation Status**: 📋 Planned

**Related Issues**: #562

---

### Category 8: Extensibility (BR-MOCK-070 to BR-MOCK-073)

#### BR-MOCK-070: Pillar Abstraction Framework

**Description**: The Mock LLM service MUST define a `Pillar` abstraction that groups tool registries, default DAGs, and response templates by AIOps domain (alert remediation, threat remediation, cost optimization).

**Priority**: P2 (MEDIUM)

**Rationale**: When #554 (Threat Remediation) and #555 (Cost Optimization) land, they will need different tool sets, RCA patterns, and workflow catalogs. The abstraction prevents restructuring.

**Acceptance Criteria**:
- [ ] `Pillar` type with name, tool registry, default DAG, and response template set
- [ ] Alert remediation scenarios refactored to use the pillar abstraction
- [ ] Adding a new pillar requires defining a `Pillar` instance + scenario registrations
- [ ] No concrete security/cost scenarios required initially

**Implementation Status**: 📋 Planned

**Related Issues**: #567, #554, #555

---

#### BR-MOCK-071: Declarative YAML Scenario Definitions

**Description**: The Mock LLM service MUST support defining scenarios entirely in YAML (detection rules, DAG structure, response templates) alongside Go-defined scenarios.

**Priority**: P2 (MEDIUM)

**Rationale**: Enables QA and platform engineers to author scenarios without writing Go code. Scenarios become test data, not code.

**Acceptance Criteria**:
- [ ] YAML scenario loader with validation and clear error messages
- [ ] YAML scenarios define detection rules, DAG nodes, transition conditions, and response template references
- [ ] At least 3 existing scenarios converted to YAML as proof-of-concept
- [ ] Go-defined scenarios take priority over YAML-defined scenarios on name conflict

**Implementation Status**: 📋 Planned

**Related Issues**: #566

---

#### BR-MOCK-072: Go Template Response Generation

**Description**: The Mock LLM service MUST support Go templates for response generation, enabling dynamic values (resource names, namespaces, UUIDs) in scenario responses.

**Priority**: P2 (MEDIUM)

**Rationale**: Replaces Python f-string interpolation with a standard templating system. Templates are external files, reviewable and modifiable without recompilation.

**Acceptance Criteria**:
- [ ] Response templates use Go `text/template` syntax
- [ ] Template context includes: resource name, namespace, root owner, workflow UUID, action type, scenario metadata
- [ ] Templates loaded from files relative to scenario definition
- [ ] Template errors produce clear, actionable error messages (not panic)

**Implementation Status**: 📋 Planned

**Related Issues**: #566

---

#### BR-MOCK-073: DAG Fragment Composition

**Description**: The Mock LLM service MUST support composing DAGs from reusable fragments (e.g., discovery fragment, analysis fragment, action selection fragment).

**Priority**: P2 (MEDIUM)

**Rationale**: Common patterns across pillars (discovery → analysis → selection) should be reusable building blocks, not duplicated per scenario.

**Acceptance Criteria**:
- [ ] `DAGFragment` type with entry point and exit points
- [ ] Fragments composable into full DAGs via chaining
- [ ] At least one shared fragment (discovery) used by multiple scenarios

**Implementation Status**: 📋 Planned

**Related Issues**: #567, #560

---

### Category 9: Observability (BR-MOCK-080 to BR-MOCK-083)

#### BR-MOCK-080: Prometheus Metrics Endpoint

**Description**: The Mock LLM service MUST expose a `GET /metrics` endpoint with Prometheus-format metrics.

**Priority**: P2 (MEDIUM)

**Rationale**: Observability during E2E test runs. When tests fail, query metrics to understand Mock LLM behavior without reading logs.

**Acceptance Criteria**:
- [ ] `/metrics` endpoint returns Prometheus text format
- [ ] Uses `prometheus/client_golang` (already a project dependency)
- [ ] Metrics reset on `POST /api/test/reset` or via separate reset mechanism

**Implementation Status**: 📋 Planned

**Related Issues**: #568

---

#### BR-MOCK-081: Request Metrics

**Description**: The Mock LLM service MUST expose request count and latency metrics.

**Priority**: P2 (MEDIUM)

**Acceptance Criteria**:
- [ ] `mock_llm_requests_total{endpoint, status_code, scenario}` counter
- [ ] `mock_llm_response_duration_seconds{endpoint, scenario}` histogram

**Implementation Status**: 📋 Planned

**Related Issues**: #568

---

#### BR-MOCK-082: Scenario Detection Metrics

**Description**: The Mock LLM service MUST expose scenario detection counters.

**Priority**: P2 (MEDIUM)

**Acceptance Criteria**:
- [ ] `mock_llm_scenario_detection_total{scenario, method}` counter (method: keyword/signal/proactive/fallback)

**Implementation Status**: 📋 Planned

**Related Issues**: #568

---

#### BR-MOCK-083: Phase Transition Metrics

**Description**: The Mock LLM service MUST expose DAG edge traversal counters.

**Priority**: P2 (MEDIUM)

**Acceptance Criteria**:
- [ ] `mock_llm_conversation_phase_total{scenario, from_node, to_node}` counter
- [ ] Integrated with DAG engine transition recording

**Implementation Status**: 📋 Planned

**Related Issues**: #568, #560

---

### Category 10: Deployment & Container (BR-MOCK-090 to BR-MOCK-093)

#### BR-MOCK-090: Minimal Container Image

**Description**: The Mock LLM service MUST be packaged as a distroless or scratch-based container image.

**Priority**: P0 (CRITICAL)

**Rationale**: Replaces the ~2.5GB Python UBI10 image. A single static Go binary in a minimal base image.

**Acceptance Criteria**:
- [ ] Multi-stage Dockerfile: build stage (Go compiler) + runtime stage (distroless/scratch)
- [ ] No runtime dependencies (no Python, no pip, no system packages)
- [ ] Non-root user (UID 1001) for security

**Implementation Status**: 📋 Planned

**Related Issues**: #531

---

#### BR-MOCK-091: Image Size Under 50MB

**Description**: The Mock LLM container image MUST be under 50MB compressed.

**Priority**: P1 (HIGH)

**Rationale**: Current Python image is ~2.5GB. Target is 50x reduction, achievable with a static Go binary (~15-20MB) in a scratch/distroless base.

**Acceptance Criteria**:
- [ ] `docker images` shows compressed size < 50MB
- [ ] CI pipeline validates image size as part of build

**Implementation Status**: 📋 Planned

**Related Issues**: #531

---

#### BR-MOCK-092: Sub-Second Startup Time

**Description**: The Mock LLM service MUST start and be ready to accept requests within 1 second.

**Priority**: P1 (HIGH)

**Rationale**: Current Python startup is 3-5 seconds. Go binary startup is near-instantaneous. Faster startup reduces E2E test setup time.

**Acceptance Criteria**:
- [ ] `/health` returns 200 within 1 second of process start
- [ ] No blocking I/O during startup (no file loading, no HTTP sync)

**Implementation Status**: 📋 Planned

**Related Issues**: #531

---

#### BR-MOCK-093: Same Container Contract

**Description**: The Mock LLM container MUST maintain the same external contract as the Python version for backward compatibility.

**Priority**: P0 (CRITICAL)

**Rationale**: Existing test infrastructure (`deployMockLLMInNamespace`, integration test helpers) expects specific port, env vars, and probe paths.

**Acceptance Criteria**:
- [ ] Listens on port 8080
- [ ] Non-root user UID 1001
- [ ] Supports `MOCK_LLM_HOST`, `MOCK_LLM_PORT` environment variables
- [ ] `/health` endpoint for probes
- [ ] Deployment manifests require only image name change

**Implementation Status**: 📋 Planned

**Related Issues**: #531

---

## 📊 Test Coverage Strategy

### Test Plan

A formal test plan will be created before implementation per DD-TEST-006. The test plan will define test scenarios (UT-MOCK-xxx, IT-MOCK-xxx, E2E-MOCK-xxx) mapping to each BR.

### Unit Tests (>=80% coverage)

| Component | Test Focus |
|-----------|------------|
| DAG engine | Node traversal, transition conditions, context extraction |
| Scenario registry | Registration, detection priority, keyword/signal matching |
| Response builders | OpenAI/Ollama response construction, template rendering |
| Config loader | YAML parsing, deterministic UUID generation, env var handling |
| Fault injection | Fault application, counter management, reset |

### Integration Tests (>=80% coverage)

| Component | Test Focus |
|-----------|------------|
| HTTP handlers | Request parsing, response serialization, error responses |
| End-to-end conversation | Multi-turn DAG traversal with real HTTP requests |
| Verification API | Tool call recording, path query, reset |
| Backward compatibility | All existing Python Mock LLM test contracts pass |

### E2E Tests

| Component | Test Focus |
|-----------|------------|
| Container contract | Port, probes, env vars, startup time |
| KAPI integration | KAPI → Mock LLM → correct response flow |
| AIAnalysis pipeline | Full signal → RCA → workflow selection flow |

---

## ⚠️ Risk Register

### RISK-MOCK-001: #548 Dependency — Deterministic UUIDs Not Yet Landed

**Severity**: HIGH
**Probability**: MEDIUM
**Impact**: BR-MOCK-030 and BR-MOCK-031 cannot be implemented until #548 delivers the deterministic UUID function in a shared package.

**Mitigation**: BR-MOCK-033 (optional YAML overrides) serves as a fallback. If #548 is delayed, the Go Mock LLM can launch with file-based UUID configuration (DD-TEST-011 v3.0 pattern) and migrate to deterministic UUIDs when available. The shared UUID function package location should be agreed upon early even if #548 implementation is in progress.

**Status**: Open

---

### RISK-MOCK-002: Shared Types Dependency on KAPI (#433)

**Severity**: MEDIUM
**Probability**: LOW
**Impact**: BR-MOCK-060 through BR-MOCK-062 define shared types that both Mock LLM and KAPI will consume. If #433 changes direction or the OpenAI types need different shapes than expected, rework is needed.

**Mitigation**: Mock LLM is the first consumer of the shared types package. Design the types based on the OpenAI API spec (stable, well-documented), not on KAPI-specific assumptions. When KAPI (#433) implementation proceeds, it adopts the existing shared types.

**Status**: Open

---

### RISK-MOCK-003: Static vs. Programmatic Deployment Manifest Divergence

**Severity**: MEDIUM
**Probability**: HIGH (already exists)
**Impact**: `deploy/mock-llm/01-deployment.yaml` uses `/liveness` and `/readiness` probe paths with no ConfigMap, while `holmesgpt_api.go` uses `/health` with ConfigMap. The Go rewrite enforcing strict routing (BR-MOCK-005) will break the static manifests.

**Mitigation**: Reconcile static manifests during Go rewrite:
1. Update `deploy/mock-llm/01-deployment.yaml` to use `/health` for both probes
2. Remove `MOCK_LLM_FORCE_TEXT` from manifests if not needed (or keep with actual implementation)
3. Document the canonical deployment contract in the BR

**Status**: Open — must be resolved during #531 implementation

---

### RISK-MOCK-004: Low-Confidence Scenario Contains Invalid UUIDs

**Severity**: LOW
**Probability**: LOW
**Impact**: The `low_confidence` scenario in Python includes alternative workflow UUIDs with non-valid hex characters (e.g., `7cg3`). If the Go rewrite validates UUID formats in responses, these will fail. Existing tests may assert on these exact strings.

**Mitigation**: Preserve the exact strings from the Python implementation. Do not add UUID format validation to scenario response data. Document as intentional test data.

**Status**: Open — verify during implementation

---

### RISK-MOCK-005: `MOCK_LLM_FORCE_TEXT` Ghost Implementation

**Severity**: MEDIUM
**Probability**: HIGH (already exists)
**Impact**: The env var is set in Go infrastructure (`mock_llm.go`) and E2E deployment manifests but **never read by the Python server**. No existing test actually validates force-text behavior via the container. If the Go rewrite implements it, it may surface hidden bugs in KAPI's text-only code path.

**Mitigation**: Implement BR-MOCK-004 properly in Go. Add integration tests that exercise the force-text path to ensure KAPI handles it correctly. This is a net improvement over the current state.

**Status**: Open — fix validated during implementation

---

### RISK-MOCK-006: `MetricsURL` in Go Infrastructure Points to Non-Existent Endpoint

**Severity**: LOW
**Probability**: HIGH (already exists)
**Impact**: `mock_llm.go` `GetMockLLMContainerInfo` advertises `MetricsURL: http://127.0.0.1:<port>/metrics`, but no `/metrics` endpoint exists. Any consumer of this URL gets `{"status":"ok"}` (Python permissive routing) or will get 404 (Go strict routing).

**Mitigation**: BR-MOCK-080 implements `GET /metrics` with Prometheus format. Until then, remove or guard the `MetricsURL` in Go infrastructure. Resolved when #568 is implemented.

**Status**: Open — resolved by #568

---

## 📋 Audit Findings (2026-03-04)

### Source

Cross-reference audit of: Python Mock LLM source (`server.py`, `__main__.py`, `Dockerfile`), Go infrastructure (`mock_llm.go`, `holmesgpt_api.go`), static manifests (`deploy/mock-llm/`), DD-TEST-011, #531 issue body, and all 9 sub-issues.

### Findings Addressed in This Document

| # | Finding | Severity | Resolution |
|---|---------|----------|------------|
| 1 | `MOCK_LLM_FORCE_TEXT` env var never read by Python server | HIGH | BR-MOCK-004 updated with audit note; RISK-MOCK-005 filed |
| 2 | `GET /v1/models` endpoint missing from BR | MEDIUM | BR-MOCK-001 updated to include `/v1/models` |
| 3 | `POST /chat/completions` (no `/v1` prefix) not documented | MEDIUM | BR-MOCK-001 updated to include both paths |
| 4 | Permissive unknown-path routing (200 for everything) | MEDIUM | BR-MOCK-005 added (strict HTTP routing) |
| 5 | Scenario metadata fields missing (execution_engine, etc.) | HIGH | BR-MOCK-026 updated with full metadata table |
| 6 | `mock_not_reproducible` keyword alias undocumented | MEDIUM | BR-MOCK-026 updated with alias |
| 7 | Fixed response constants undocumented (created, usage, IDs) | MEDIUM | BR-MOCK-001 updated with response format details |
| 8 | Static manifest probe paths diverge from programmatic | MEDIUM | RISK-MOCK-003 filed; BR-MOCK-005 addresses |
| 9 | `MetricsURL` in Go infra points to non-existent endpoint | LOW | RISK-MOCK-006 filed; resolved by #568 |
| 10 | Invalid UUIDs in low_confidence scenario | LOW | RISK-MOCK-004 filed; preserve as-is |
| 11 | DD-TEST-011 v4.0 says CONFIG_PATH removed but BR-MOCK-033 keeps it | MEDIUM | Clarified: v4.0 removes it as required, BR-MOCK-033 retains as optional override |
| 12 | ConfigMap `overrides` section (execution_engine: job) undocumented | MEDIUM | BR-MOCK-026 metadata table documents execution_engine per scenario |

### Open Questions for Implementation

1. **Should Go rewrite reproduce the permissive routing?** Recommendation: No — implement strict routing (BR-MOCK-005) and update static manifests. Verify no test depends on permissive behavior.
2. **Should fixed response constants (created, usage) be configurable?** Recommendation: No — keep fixed for determinism. Document the exact values so tests can assert on them if needed.
3. **Should the Go rewrite support Ollama streaming?** Recommendation: Not in v1.0 — current Python returns one-shot `done: true`. Streaming can be added later if needed.

---

## 🔗 Related Documentation

- **Parent Issue**: [#531](https://github.com/jordigilh/kubernaut/issues/531) — Rewrite Mock LLM service in Go
- **Parent Service**: [#433](https://github.com/jordigilh/kubernaut/issues/433) — KAPI Go rewrite
- **KAPI Business Requirements**: [BUSINESS_REQUIREMENTS.md](../../stateless/holmesgpt-api/BUSINESS_REQUIREMENTS.md)
- **DD-TEST-011**: [Mock LLM Configuration Pattern](../../../architecture/decisions/DD-TEST-011-mock-llm-self-discovery-pattern.md)
- **TD-HAPI-001**: [Extract Mock LLM to External Service](../../../development/technical-debt/TD-HAPI-001-extract-mock-llm-to-external-service.md) (COMPLETE)
- **#548**: Deterministic UUIDs in DataStorage
- **#554**: Threat Remediation enhancement proposal
- **#555**: Cost Optimization enhancement proposal
- **Sub-Issues**: #560, #561, #562, #563, #564, #565, #566, #567, #568

---

**Document Version**: 1.0
**Last Updated**: 2026-03-04
**Maintained By**: Kubernaut Architecture Team
**Status**: 📋 Specification Phase — Go rewrite not yet started
