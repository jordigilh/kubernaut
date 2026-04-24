# Implementation Plan: Conversational RAR API Backend (#592)

**Issue**: #592 — Conversational mode for Kubernaut Agent, RAR-scoped investigation queries
**Test Plan**: [TEST_PLAN.md](TEST_PLAN.md)
**Source of Truth**: [592_conversation_api_v4 cursor plan](../../../.cursor/plans/592_conversation_api_v4_1f21facf.plan.md)
**Created**: 2026-03-04
**Updated**: 2026-04-07
**Version**: 4.0
**Status**: Draft

---

## Execution Strategy

This plan follows **strict TDD RED -> GREEN -> REFACTOR** with each cycle as a discrete implementation step. Checkpoints perform rigorous due diligence between functional areas.

**Total**: 12 TDD cycles (Phase 0 + 11 conversation cycles) x 3 phases each = 36 phases + 4 checkpoints = 40 steps

**Test inventory**: 33 unit tests + 7 integration tests = 40 total

**Key v4.0 changes from v2.0**:
- Added **Phase 0** (audit completeness prerequisite) before any conversation code
- Corrected all audit event types to match real `emitter.go` constants (F1)
- Added `messages` structured array schema (not `prompt_content`) for LLM prompt storage (A14)
- Added missing acceptance criteria tests: read-only toolsets (A6), mutating tool refusal (A7), LLM failure SSE (A8), RR completed lifecycle (A9), TLS enforcement (A10)
- Renumbered `UT-CS-592-010-TTL` to `UT-CS-592-030` to avoid ID collision (A5)
- No backward compatibility with v1.3 audit events (user decision)

---

## Authoritative Documentation

- **Issue #592**: Enhancement proposal
- **Issue #601**: Shadow agent pattern (configurable conversation model)
- **DD-AUDIT-005**: Hybrid audit trail; Phase 0 fixes v1.3 gaps
- **DD-AUTH-014 v3.0**: Middleware-based SAR authentication
- **ADR-036**: Cluster-wide TokenReview/SAR strategy
- **ADR-048 + Addendum 001**: chi Throttle rate limiting
- **DD-HTTP-001**: KA chi-based REST API
- **TESTING_GUIDELINES.md**: >=80% per-tier, Ginkgo, anti-patterns

---

## Phase 0: Audit Completeness Fix (prerequisite)

All conversation reconstruction depends on complete audit data. v1.3 stores truncated previews and a reduced `InvestigationResult` projection. This phase fixes both before any conversation code is written.

### TDD Cycle 0: Audit Completeness (UT-CS-592-026..029)

#### Phase 1 — RED: Write failing tests for audit completeness

**Tests**: `test/unit/kubernautagent/investigator/audit_completeness_test.go` (new file)

Tests call `investigator.Investigate()` (business logic) and assert on audit events emitted as a side effect via a `CapturingAuditStore`. This follows the TESTING_GUIDELINES.md anti-pattern policy — no direct `StoreAudit` calls.

| Test ID | Assertion |
|---------|-----------|
| `UT-CS-592-026` | `aiagent.llm.request` event contains `messages` as structured `[]LLMMessage` array (not just 500-char `prompt_preview`) |
| `UT-CS-592-027` | `aiagent.llm.response` event contains `analysis_content` with full LLM response text (not 500-char `analysis_preview`) |
| `UT-CS-592-028` | `aiagent.response.complete` event `response_data` includes `detected_labels`, `validation_attempts_history`, `is_actionable`, `signal_name`, full alternative workflows |
| `UT-CS-592-029` | `aiagent.response.complete` event serializes both `human_review_reason` and `reason` fields (bug fix) |

**Actions**:
1. Create test file with Ginkgo suite in `test/unit/kubernautagent/investigator/`
2. Implement `CapturingAuditStore` (implements `audit.AuditStore`) with `Events` slice and `EventsByType()` helper
3. Construct `Investigator` with: `CapturingAuditStore`, mock `llm.Client` (scripted responses), mock `ResultParser` (fully-populated `InvestigationResult`), `ModelName: "test-model"`, `MaxTurns: 5`
4. Each test: call `inv.Investigate(ctx, signal)` -> filter `capturingStore.EventsByType(eventType)` -> assert on `event.Data` map

**Exit criteria**: Tests compile but FAIL (investigator emits truncated previews and reduced projection)

#### Phase 2 — GREEN: Extend schema, fix investigator and ds_store

**Files**:
- `pkg/datastorage/server/middleware/openapi_spec_data.yaml` (modified)
- `pkg/datastorage/ogen-client/` (regenerated)
- `internal/kubernautagent/investigator/investigator.go` (modified)
- `internal/kubernautagent/audit/ds_store.go` (modified)

**Actions**:
1. Define `LLMMessage` schema type in `openapi_spec_data.yaml`: `role` (enum: system/user/assistant/tool), `content`, `tool_call_id`, `name`
2. Add `messages: []LLMMessage` (optional) to `LLMRequestPayload`
3. Add `analysis_content: string` (optional) to `LLMResponsePayload`
4. Add missing fields to `IncidentResponseData`: `is_actionable`, `signal_name`, `detected_labels`, `validation_attempts_history`, per-alternative `execution_bundle`/`confidence`
5. Fix `human_review_reason` typing in `IncidentResponseData`
6. Regenerate ogen client: `make generate-ogen`
7. Update `resultToAuditJSON` in `investigator.go` to serialize complete `InvestigationResult`
8. Update investigator LLM request emission to include full `messages` array alongside `prompt_preview`
9. Update investigator LLM response emission to include full `analysis_content` alongside `analysis_preview`
10. Update `buildEventData` in `ds_store.go` to populate new ogen fields

**Exit criteria**: UT-CS-592-026 through -029 pass

#### Phase 3 — REFACTOR: Audit completeness cleanup

**Actions**:
1. Ensure `prompt_preview`/`analysis_preview` remain populated for dashboard backward compatibility
2. Verify storage impact estimate (~25MB/day at 100 analyses/day) is acceptable
3. Run `go vet ./...` and lint

**Exit criteria**: Tests still pass; no lint errors; preview fields present for dashboards

---

## CHECKPOINT 0: Audit Completeness Due Diligence

**Actions**:
1. All 4 Phase 0 tests pass
2. `go build ./...` succeeds
3. Existing investigator unit tests pass (no regression)
4. DS test suite passes with schema changes
5. Verify `LLMMessage` schema renders correctly in ogen-generated Go types
6. Verify no anti-patterns: no `time.Sleep`, no `Skip()`, no direct `StoreAudit` calls

**Finding resolution**: Address all findings before proceeding to conversation code.

---

## TDD Cycle 1: Audit Reconstruction (UT-CS-592-001..005)

### Phase 4 — RED: Write failing tests for audit chain reconstruction

**Tests**: `test/unit/kubernautagent/conversation/audit_test.go` (new file)

| Test ID | Assertion |
|---------|-----------|
| `UT-CS-592-001` | 5 audit events -> 5 LLM messages with correct roles (using structured `messages` from events) |
| `UT-CS-592-002` | Each audit event type maps to correct LLM message type (system/user/assistant/function) |
| `UT-CS-592-003` | Incomplete audit chain -> retries with exponential backoff |
| `UT-CS-592-004` | Empty chain after retries -> error with descriptive message |
| `UT-CS-592-005` | Large chain exceeds token budget -> older turns summarized |

**Actions**:
1. Create `test/unit/kubernautagent/conversation/suite_test.go` with Ginkgo bootstrap
2. Create `audit_test.go` with `MockAuditReader` (implements `AuditReader` interface, returns pre-built `[]ogenclient.AuditEvent`)
3. Write 5 tests calling `AuditChainFetcher.FetchInvestigationHistory(ctx, correlationID)`
4. Assert message count, roles, content from structured `messages`/`analysis_content` fields

**Exit criteria**: Tests fail to compile (`conversation` package and `AuditReader` do not exist)

### Phase 5 — GREEN: Implement audit reconstruction

**Files**: `internal/kubernautagent/conversation/audit.go` (new)

**Actions**:
1. Define `AuditReader` interface wrapping `ogenclient.QueryAuditEvents`
2. Implement `AuditChainFetcher` struct with `reader AuditReader`
3. Implement `FetchInvestigationHistory(ctx, correlationID)`: query DS with `CorrelationID` param, `Limit: 1000`
4. Implement `eventsToMessages`: switch on `AuditEvent.EventData.Type`, extract `LLMRequestPayload.Messages`, `LLMResponsePayload.AnalysisContent`, `LLMToolCallPayload`, `AIAgentResponsePayload.ResponseData`
5. Implement retry with exponential backoff (100ms, 200ms, 400ms, 800ms, 1600ms, 3200ms) for empty chains
6. Implement token budget management: summarize oldest tool results when total exceeds configurable limit

**Exit criteria**: UT-CS-592-001 through -005 pass

### Phase 6 — REFACTOR: Audit reconstruction cleanup

**Actions**:
1. Extract `retryWithBackoff` as reusable helper
2. Add structured logging with `correlation_id`
3. Document `eventsToMessages` mapping table in code comments

**Exit criteria**: Tests still pass; retry logic extracted; logging in place

---

## TDD Cycle 2: Session Management + Guardrails (UT-CS-592-006..009)

### Phase 7 — RED: Write failing tests for session and guardrails

**Tests**: `test/unit/kubernautagent/conversation/session_test.go`, `guardrails_test.go` (new files)

| Test ID | Assertion |
|---------|-----------|
| `UT-CS-592-006` | System prompt scopes to RR name/namespace |
| `UT-CS-592-007` | Tool call validator rejects cross-namespace calls |
| `UT-CS-592-008` | Session CRUD: create with RAR ref, get by ID, delete |
| `UT-CS-592-009` | Shared session: two user IDs in same session, both tracked |

**Actions**:
1. Create `session_test.go` with create/get/delete lifecycle test and shared session test
2. Create `guardrails_test.go` with system prompt scoping test and namespace validation test
3. Assert session stores RAR ref, RR ref, participants, state
4. Assert guardrails reject tool calls targeting namespace != session namespace

**Exit criteria**: Tests fail to compile (session manager and guardrails do not exist)

### Phase 8 — GREEN: Implement session and guardrails

**Files**:
- `internal/kubernautagent/conversation/session.go` (new)
- `internal/kubernautagent/conversation/guardrails.go` (new)
- `internal/kubernautagent/conversation/types.go` (new)

**Actions**:
1. Define `Session` struct: `ID`, `RARName`, `RARNamespace`, `CorrelationID`, `Messages`, `Participants`, `State` (`Interactive`/`ReadOnly`/`Closed`), `CreatedAt`, `LastActivity`
2. Implement `SessionManager` with `sync.Map` store: `Create`, `Get`, `Delete`, `AddParticipant`
3. Define `SessionState` enum in `types.go`
4. Implement namespace enforcement in `guardrails.go`: intercept tool name + arguments, reject if target namespace != session namespace
5. Implement read-only tool filtering: reject mutating verbs (create, delete, patch, apply) even in correct namespace

**Exit criteria**: UT-CS-592-006 through -009 pass

### Phase 9 — REFACTOR: Session and guardrails cleanup

**Actions**:
1. Add `ConversationTurn` type to `types.go` for structured turn tracking
2. Ensure session map operations are goroutine-safe
3. Add doc comments on exported types

**Exit criteria**: Tests still pass; types documented

---

## TDD Cycle 3: Config + TTL + Read-Only Tools (UT-CS-592-023, 024, 030, 031, 032)

### Phase 10 — RED: Write failing tests for config and toolset filtering

**Tests**: `test/unit/kubernautagent/conversation/config_test.go`, `session_test.go` (append), `guardrails_test.go` (append)

| Test ID | Assertion |
|---------|-----------|
| `UT-CS-592-023` | Conversation LLM config defaults to investigation model when `LLM` field is nil |
| `UT-CS-592-024` | Conversation LLM config uses override when `LLM` field is explicitly set |
| `UT-CS-592-030` | Session TTL: session created, idle beyond TTL, get returns not found |
| `UT-CS-592-031` | Read-only tool call succeeds during conversation (acceptance: "KA can use read-only toolsets") |
| `UT-CS-592-032` | Mutating tool call in correct namespace rejected (acceptance: "KA refuses mutating tool calls") |

**Actions**:
1. Create `config_test.go` with two tests for `ConversationConfig.EffectiveLLM()` method
2. Add TTL expiry test to `session_test.go`: create session, advance time past TTL, assert `Get` returns error
3. Add read-only tool success test to `guardrails_test.go`: `kubectl get pods` in correct namespace -> allowed
4. Add mutating tool rejection test: `kubectl delete pod` in correct namespace -> rejected

**Exit criteria**: Tests fail (config types, TTL reaper, verb filtering do not exist)

### Phase 11 — GREEN: Implement config, TTL, and verb filtering

**Files**:
- `internal/kubernautagent/config/config.go` (modified)
- `internal/kubernautagent/conversation/session.go` (modified)
- `internal/kubernautagent/conversation/guardrails.go` (modified)
- `internal/kubernautagent/prompt/builder.go` (modified)
- `internal/kubernautagent/prompt/templates/conversation.tmpl` (new)
- `charts/kubernaut/values.yaml` (modified)

**Actions**:
1. Add `ConversationConfig` to `config.go`: `Enabled`, `LLM *LLMConfig`, `RateLimit`, `Session`, `TokenBudget`, `TLS`
2. Add `EffectiveLLM(mainLLM LLMConfig)` method: returns `c.LLM` if non-nil, else `mainLLM`
3. Implement TTL reaper goroutine in `SessionManager.StartCleanupLoop(ctx, interval)`: scan sessions, evict idle > TTL
4. Implement verb-based filtering in guardrails: maintain list of mutating verbs, reject tool calls whose arguments contain them
5. Create `conversation.tmpl` with RR-scoped system prompt (G2 spec from cursor plan)
6. Extend `Builder` with `conversationTmpl` field and `RenderConversation()` method (G2)
7. Add `kubernautAgent.conversations` section to `values.yaml` (G3)

**Exit criteria**: UT-CS-592-023, -024, -030, -031, -032 pass

### Phase 12 — REFACTOR: Config and template cleanup

**Actions**:
1. Add `ConversationSessionConfig` and `RateLimitConfig` sub-structs for clean YAML mapping
2. Ensure `conversation.tmpl` follows same header style as `incident_investigation.tmpl`
3. Verify `go vet` clean

**Exit criteria**: Tests still pass; config struct maps cleanly to Helm values

---

## CHECKPOINT 1: Core Due Diligence

**Actions**:
1. All 18 unit tests pass (4 Phase 0 + 14 conversation core)
2. Verify F1 audit event mapping against real `emitter.go` constants
3. Verify sessions are leak-free: TTL reaper evicts expired sessions
4. Verify guardrails enforce RR scope AND read-only toolset (namespace + verb filtering)
5. Verify conversation LLM config fallback: nil -> uses investigation model
6. `go build ./...` succeeds; `go vet ./...` clean
7. Measure unit coverage on `internal/kubernautagent/conversation/` -> target >=80%
8. Verify no anti-patterns: no `time.Sleep`, no `Skip()`, all assertions check concrete values

**Finding resolution**: Address all findings before proceeding to auth/SSE.

---

## TDD Cycle 4: Authentication (UT-CS-592-010..012)

### Phase 13 — RED: Write failing tests for TokenReview + SAR

**Tests**: `test/unit/kubernautagent/conversation/auth_test.go` (new file)

| Test ID | Assertion |
|---------|-----------|
| `UT-CS-592-010` | TokenReview: valid bearer token -> authenticated identity extracted |
| `UT-CS-592-011` | SAR: user can UPDATE target RAR -> authorized |
| `UT-CS-592-012` | SAR: user cannot UPDATE target RAR -> 403 Forbidden |

**Actions**:
1. Create `auth_test.go` with mock `Authenticator` and mock `Authorizer` (both from `pkg/shared/auth`)
2. Write test: valid token -> `ValidateToken` returns identity -> middleware sets context
3. Write test: `CheckAccess(user, ns, "remediationapprovalrequests", rarName, "update")` returns true -> next handler called
4. Write test: `CheckAccess` returns false -> 403 response

**Exit criteria**: Tests fail to compile (`conversation/auth.go` does not exist)

### Phase 14 — GREEN: Implement conversation auth middleware

**Files**: `internal/kubernautagent/conversation/auth.go` (new)

**Actions**:
1. Create `ConversationAuthMiddleware` struct with `Authenticator` and `Authorizer` interfaces from `pkg/shared/auth`
2. Implement `Handler(next http.Handler) http.Handler`:
   - Extract bearer token from `Authorization` header
   - Call `authenticator.ValidateToken(ctx, token)` -> get identity
   - Look up session from URL path `{sessionID}` -> get RAR name/namespace
   - Call `authorizer.CheckAccess(ctx, identity, namespace, "remediationapprovalrequests", rarName, "update")`
   - 401 if token invalid; 403 if SAR denied; set identity in context and call next

**Exit criteria**: UT-CS-592-010 through -012 pass

### Phase 15 — REFACTOR: Auth error responses

**Actions**:
1. Use RFC 7807 error format for 401/403 responses
2. Ensure no sensitive data leaked in error bodies
3. Add structured logging for auth failures

**Exit criteria**: Tests still pass; error responses follow RFC 7807

---

## TDD Cycle 5: SSE Streaming (UT-CS-592-013..015)

### Phase 16 — RED: Write failing tests for SSE writer

**Tests**: `test/unit/kubernautagent/conversation/sse_test.go` (new file)

| Test ID | Assertion |
|---------|-----------|
| `UT-CS-592-013` | SSE events include incrementing `id` field |
| `UT-CS-592-014` | SSE reconnection: `Last-Event-ID` header -> resumes from correct event |
| `UT-CS-592-015` | SSE buffer: responses stored for 60s, available for reconnection |

**Actions**:
1. Create `sse_test.go` with `httptest.NewRecorder` to capture SSE output
2. Write test: send 3 events -> parse output -> assert IDs are 1, 2, 3
3. Write test: set `Last-Event-ID: 2` -> assert replay starts from event 3
4. Write test: send event -> wait < 60s -> reconnect -> event available; wait > 60s -> event expired

**Exit criteria**: Tests fail to compile (`sse.go` does not exist)

### Phase 17 — GREEN: Implement SSE writer

**Files**: `internal/kubernautagent/conversation/sse.go` (new)

**Actions**:
1. Create `SSEWriter` struct wrapping `http.ResponseWriter` with `http.Flusher`
2. Implement `WriteEvent(id int, eventType string, data string)`: format as `id: N\nevent: type\ndata: ...\n\n`
3. Implement ring buffer (60s TTL) for event replay
4. Implement `Last-Event-ID` handling: parse header, replay buffered events from that ID
5. Set proxy-friendly headers: `Cache-Control: no-cache`, `X-Accel-Buffering: no`, `Connection: keep-alive`

**Exit criteria**: UT-CS-592-013 through -015 pass

### Phase 18 — REFACTOR: SSE cleanup

**Actions**:
1. Extract `SSEEvent` type with `ID`, `Type`, `Data` fields
2. Ensure buffer eviction runs on write (not separate goroutine) to avoid race conditions
3. Verify `Content-Type: text/event-stream` is set

**Exit criteria**: Tests still pass; buffer is race-free

---

## TDD Cycle 6: Rate Limiting + Audit Identity (UT-CS-592-016..018)

### Phase 19 — RED: Write failing tests for rate limits and audit

**Tests**: `test/unit/kubernautagent/conversation/ratelimit_test.go` (new file), `audit_test.go` (append)

| Test ID | Assertion |
|---------|-----------|
| `UT-CS-592-016` | Per-user rate limit: 11th request within 1 min -> 429 |
| `UT-CS-592-017` | Per-session rate limit: 31st turn -> 429 |
| `UT-CS-592-018` | Each conversation turn emits audit event with authenticated identity + `correlation_id` |

**Actions**:
1. Create `ratelimit_test.go` with per-user and per-session limit tests
2. Add audit emission test to conversation `audit_test.go`: post message -> assert audit event emitted with `user_identity` and `correlation_id`

**Exit criteria**: Tests fail to compile (`ratelimit.go` does not exist)

### Phase 20 — GREEN: Implement rate limiting

**Files**: `internal/kubernautagent/conversation/ratelimit.go` (new)

**Actions**:
1. Implement per-user token bucket: keyed by authenticated identity, configurable rate (default 10/min)
2. Implement per-session turn counter: incremented on each `PostMessage`, configurable max (default 30)
3. Return 429 with `Retry-After` header when limits exceeded
4. Emit conversation audit event on each turn via existing `audit.AuditStore`

**Exit criteria**: UT-CS-592-016 through -018 pass

### Phase 21 — REFACTOR: Rate limit configuration

**Actions**:
1. Wire rate limit config from `ConversationConfig.RateLimit`
2. Add `Retry-After` header computation
3. Ensure rate limit state is cleaned up when sessions expire

**Exit criteria**: Tests still pass; rate limits configurable via Helm

---

## CHECKPOINT 2: Security Due Diligence

**Actions**:
1. All 27 unit tests pass (18 core + 9 auth/SSE/rate)
2. Auth enforced on all conversation paths (no bypass possible)
3. SSE reconnection verified with `Last-Event-ID`
4. Rate limits enforced per-user and per-session
5. No sensitive data leaked in error responses (check 401, 403, 429 bodies)
6. Verify `pkg/shared/auth` interfaces reused correctly (DD-AUTH-014 compliance)
7. Run `go build ./...`

**Finding resolution**: Address all findings before proceeding to handler integration.

---

## TDD Cycle 7: Handler + Core Integrations (IT-CS-592-001..003, IT-CS-592-006)

### Phase 22 — RED: Write failing integration tests for core flow

**Tests**: `test/integration/kubernautagent/conversation/conversation_test.go` (new file)

| Test ID | Assertion |
|---------|-----------|
| `IT-CS-592-001` | Create session -> POST message -> receive SSE stream with LLM tokens |
| `IT-CS-592-002` | Audit chain from DS mock -> session seeded -> first message continues conversation |
| `IT-CS-592-003` | SSE stream delivers tokens incrementally (not all at once) |
| `IT-CS-592-006` | Missing/invalid bearer token -> 401; valid token + no SAR -> 403 |

**Actions**:
1. Create integration test file with Ginkgo suite
2. Set up: `httptest.NewTLSServer` with conversation handler, mock DS (returns audit events), mock LLM, mock K8s auth
3. Write 4 integration tests covering the full request flow

**Exit criteria**: Tests fail (`handler.go` does not exist; routes not wired)

### Phase 23 — GREEN: Implement handler and wire routes

**Files**:
- `internal/kubernautagent/conversation/handler.go` (new)
- `cmd/kubernautagent/main.go` (modified)

**Actions**:
1. Implement `Handler` struct with `CreateSession`, `GetSession`, `PostMessage`, `StreamSSE`, `CloseSession` methods
2. Wire Chi routes in `main.go`: mount `/conversations` sub-route BEFORE ogen catch-all, with dynamic auth middleware; wrap investigation routes in `r.Group` with static auth
3. Create optional second `langchaingo.New` LLM client for conversation (or reuse investigation client if config `LLM` is nil)
4. Inject `K8sAuthorizer`, `K8sAuthenticator`, and ogen client as `AuditReader`

**Exit criteria**: IT-CS-592-001 through -003 and IT-CS-592-006 pass

### Phase 24 — REFACTOR: Handler cleanup

**Actions**:
1. Extract `HandlerConfig` struct for dependency injection
2. Ensure all handlers return consistent error format (RFC 7807)
3. Verify route isolation: conversation uses dynamic auth; investigation uses static auth via `r.Group`

**Exit criteria**: Tests still pass; `HandlerConfig` encapsulates all dependencies

---

## TDD Cycle 8: Remaining Integrations (IT-CS-592-007..009)

### Phase 25 — RED: Write failing tests for lifecycle, LLM failure, and TLS

**Tests**: `test/integration/kubernautagent/conversation/conversation_test.go` (append)

| Test ID | Assertion |
|---------|-----------|
| `IT-CS-592-007` | RAR status change (approved) -> session transitions to read-only -> subsequent message -> 409 |
| `IT-CS-592-008` | LLM failure mid-stream -> SSE `event: error` sent (acceptance criterion) |
| `IT-CS-592-009` | TLS enforcement: non-TLS connection rejected; TLS connection succeeds (acceptance criterion) |

**Actions**:
1. IT-007: Create session, mock RAR with `status.decision = "Approved"`, POST message -> assert 409
2. IT-008: Configure mock LLM to return error after first token -> assert SSE stream contains `event: error`
3. IT-009: Start handler with TLS config -> attempt plain HTTP -> assert failure; attempt HTTPS -> assert success

**Exit criteria**: Tests fail (lifecycle polling, SSE error event, TLS not implemented)

### Phase 26 — GREEN: Implement lifecycle polling, SSE errors, and TLS

**Files**:
- `internal/kubernautagent/conversation/handler.go` (modified)
- `internal/kubernautagent/conversation/sse.go` (modified)

**Actions**:
1. In `PostMessage`, before LLM call: poll RAR `status.decision` via `dynamic.Interface.Get()` -> if decided, return 409
2. In SSE streaming: wrap LLM `Chat` call in error handler -> on failure, emit `event: error` SSE event with error message
3. Configure TLS in `http.Server` from `ConversationConfig.TLS`

**Exit criteria**: IT-CS-592-007 through -009 pass

### Phase 27 — REFACTOR: Integration cleanup

**Actions**:
1. Extract mock setup helpers for integration tests: `setupMockDS`, `setupMockLLM`, `setupMockAuth`
2. Ensure `httptest` servers are properly closed in `AfterEach`
3. Verify no resource leaks

**Exit criteria**: Tests still pass; test infrastructure reusable

---

## CHECKPOINT 3: Integration Due Diligence

**Actions**:
1. All 27 unit + 7 integration tests pass (34 total so far)
2. Full conversation flow verified end-to-end with MockLLM
3. Prometheus metrics registered:
   - `kubernaut_conversation_sessions_active` (gauge)
   - `kubernaut_conversation_turns_total` (counter)
   - `kubernaut_conversation_llm_errors_total` (counter)
4. Update `internal/kubernautagent/api/openapi.json` with 5 conversation endpoints, schemas, `Conversation` tag (**G1**)
5. Create `charts/kubernaut/templates/kubernaut-agent/networkpolicy-agent.yaml` with investigation + conversation ingress rules (**G4**)
6. Add `networkPolicies.kubernautAgent` section to `values.yaml` with configurable `conversationIngress` (**G4**)
7. Config validated with defaults (**G3**)
8. `go build ./...`

**Finding resolution**: Address all findings before proceeding to override/lifecycle.

---

## TDD Cycle 9: Override Advisory (UT-CS-592-019, 020)

### Phase 28 — RED: Write failing tests for override validation

**Tests**: `test/unit/kubernautagent/conversation/override_test.go` (new file)

| Test ID | Assertion |
|---------|-----------|
| `UT-CS-592-019` | Override: workflow validated against catalog schema |
| `UT-CS-592-020` | Override disabled when RAR `status.decision` = `"Approved"` or `"Rejected"` |

**Actions**:
1. Create `override_test.go` with mock workflow catalog
2. Write test: valid workflow ID -> advisory patch command returned
3. Write test: session in read-only state -> override returns error

**Exit criteria**: Tests fail to compile (`override.go` does not exist)

### Phase 29 — GREEN: Implement override advisory

**Files**: `internal/kubernautagent/conversation/override.go` (new)

**Actions**:
1. Implement `ValidateOverride(session, workflowID, params)`: check workflow exists in catalog, validate params against schema
2. Generate advisory kubectl patch command (does NOT mutate RAR CR)
3. Return error if session `IsReadOnly()` or `IsClosed()`

**Exit criteria**: UT-CS-592-019 and -020 pass

### Phase 30 — REFACTOR: Override cleanup

**Actions**:
1. Ensure error messages include workflow ID and rejection reason
2. Document that override is advisory-only; CRD mutation is in #594 webhook path
3. Verify `go vet` clean

**Exit criteria**: Tests still pass; override is clearly advisory-only

---

## TDD Cycle 10: Lifecycle Management (UT-CS-592-021, 022, 025, 033)

### Phase 31 — RED: Write failing tests for RAR/RR lifecycle

**Tests**: `test/unit/kubernautagent/conversation/lifecycle_test.go` (new file)

| Test ID | Assertion |
|---------|-----------|
| `UT-CS-592-021` | RAR `status.decision` = `"Approved"` -> session `IsReadOnly()` returns true |
| `UT-CS-592-022` | RAR `status.decision` = `"Expired"` (`status.expired` = true) -> session `IsClosed()` returns true |
| `UT-CS-592-025` | RAR `status.decision` = `""` (pending) -> session remains interactive |
| `UT-CS-592-033` | RR completed/failed -> session read-only for post-decision review (acceptance criterion) |

**Actions**:
1. Create `lifecycle_test.go` with mock `dynamic.Interface` returning crafted unstructured RAR/RR objects
2. Write tests for all 4 `ApprovalDecision` states + RR completion
3. Assert session state transitions correctly

**Exit criteria**: Tests fail to compile (`lifecycle.go` does not exist)

### Phase 32 — GREEN: Implement lifecycle polling

**Files**:
- `internal/kubernautagent/conversation/lifecycle.go` (new)
- `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml` (modified)

**Actions**:
1. Implement `CheckLifecycle(ctx, session, dynamicClient)`:
   - Poll RAR via `dynamic.Interface.Resource(rarGVR).Namespace(ns).Get(ctx, name, ...)`
   - Extract `status.decision` (`ApprovalDecision` type): `""` -> interactive, `"Approved"`/`"Rejected"` -> read-only, `"Expired"` -> closed
   - Poll RR via `dynamic.Interface.Resource(rrGVR).Namespace(ns).Get(ctx, name, ...)` -> `status.phase` = `Completed`/`Failed` -> read-only
2. Add RBAC rule to `kubernaut-agent-investigator` ClusterRole (**G6/F5**):
   ```yaml
   - apiGroups: ["remediation.kubernaut.ai"]
     resources: ["remediationapprovalrequests", "remediationworkflows"]
     verbs: ["get"]
   ```

**Exit criteria**: UT-CS-592-021, -022, -025, -033 pass

### Phase 33 — REFACTOR: Lifecycle cleanup

**Actions**:
1. Polish lifecycle transitions: handle edge cases (conditions vs decision field)
2. Ensure all `ApprovalDecision` values are covered
3. Add descriptive error messages for read-only/closed session attempts

**Exit criteria**: Tests still pass; all lifecycle paths clean

---

## CHECKPOINT 4: Final Acceptance Audit

**Actions**:
1. Run ALL unit tests (33): `ginkgo -v ./test/unit/kubernautagent/investigator/... ./test/unit/kubernautagent/conversation/...`
2. Run ALL integration tests (7): `ginkgo -v ./test/integration/kubernautagent/conversation/...`
3. Run EXISTING KA tests (no conversation filter): verify 0 regressions
4. Measure per-tier coverage:
   - Unit: `go test -coverprofile` on `internal/kubernautagent/conversation/` -> >=80%
   - Integration: `go test -coverprofile` on conversation handler paths -> >=80%
5. Full build: `go build ./...`
6. Lint: `go vet ./...`
7. Security audit:
   - Auth enforced on all paths (no bypass)
   - TLS enforced (IT-009)
   - Rate limits enforced per-user and per-session
   - RR scope enforcement (namespace + verb)
   - No static auth leakage into conversation routes
8. RBAC verified: KA can `get` RAR and RR (G6/F5)
9. Conversation LLM config fallback verified (F6)
10. Audit completeness verified: structured `messages` stored, complete `InvestigationResult` (Phase 0)
11. OpenAPI documented (G1), NetworkPolicy created (G4), prompt template exists (G2), `AuditReader` injected (G5)
12. Route isolation verified: `/conversations` uses dynamic auth; `/*` catch-all uses static auth via `r.Group`
13. Verify no anti-patterns in ANY test: no `time.Sleep`, no `Skip()`, no `XIt`
14. Cross-reference test IDs against test plan — ensure 100% coverage of planned scenarios

**Finding resolution**: Address all findings. Document deferred items for v1.5.

---

## Commit Plan

| Commit # | Scope |
|----------|-------|
| 1 | `fix(#592): audit completeness — full messages, analysis_content, complete InvestigationResult` |
| 2 | `test(#592): TDD RED — failing tests for conversation audit reconstruction + session` |
| 3 | `feat(#592): audit chain reconstruction from DataStorage` |
| 4 | `feat(#592): conversation session management with TTL and shared sessions` |
| 5 | `feat(#592): RR-scoped guardrails, read-only toolset filtering, conversation prompt template` |
| 6 | `feat(#592): ConversationConfig with configurable LLM model` |
| 7 | `feat(#592): TokenReview + SAR auth middleware for conversation endpoints` |
| 8 | `feat(#592): SSE streaming with event IDs and reconnection support` |
| 9 | `feat(#592): per-user and per-session rate limiting` |
| 10 | `feat(#592): conversation HTTP handler, route wiring, TLS enforcement` |
| 11 | `feat(#592): override advisory logic with catalog validation` |
| 12 | `feat(#592): conversation lifecycle management (RAR/RR state transitions), Helm RBAC` |
| 13 | `refactor(#592): metrics, OpenAPI docs, NetworkPolicy, Helm config` |

---

## Summary

| Phase | Type | Tests | Description |
|-------|------|-------|-------------|
| 0 (1-3) | TDD Cycle 0 | UT-592-026..029 | Audit completeness fix (prerequisite) |
| CP0 | Checkpoint | — | Audit completeness due diligence |
| 4-6 | TDD Cycle 1 | UT-592-001..005 | Audit reconstruction |
| 7-9 | TDD Cycle 2 | UT-592-006..009 | Session + guardrails |
| 10-12 | TDD Cycle 3 | UT-592-023, 024, 030, 031, 032 | Config + TTL + read-only tools |
| CP1 | Checkpoint | — | Core due diligence |
| 13-15 | TDD Cycle 4 | UT-592-010..012 | Authentication |
| 16-18 | TDD Cycle 5 | UT-592-013..015 | SSE streaming |
| 19-21 | TDD Cycle 6 | UT-592-016..018 | Rate limiting + audit identity |
| CP2 | Checkpoint | — | Security due diligence |
| 22-24 | TDD Cycle 7 | IT-592-001..003, 006 | Handler + core integrations |
| 25-27 | TDD Cycle 8 | IT-592-007..009 | Lifecycle + LLM failure + TLS |
| CP3 | Checkpoint | — | Integration due diligence |
| 28-30 | TDD Cycle 9 | UT-592-019, 020 | Override advisory |
| 31-33 | TDD Cycle 10 | UT-592-021, 022, 025, 033 | Lifecycle management |
| CP4 | Checkpoint | — | Final acceptance audit |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial implementation plan |
| 2.0 | 2026-03-04 | Scope reduction: remove Slack bot (#633/v1.5) and kubectl plugin (#634/v1.5). Reconcile override with #594. |
| 4.0 | 2026-04-07 | Full rewrite: add Phase 0 (audit completeness), correct audit event types (F1), add structured `messages` schema (A14), fix test count (33+7=40), add missing acceptance criteria tests (A6-A10), renumber UT-010-TTL to UT-030 (A5), fine-grained TDD cycles following #60 format, 4 checkpoints, concrete gap specifications (G1-G6). |
