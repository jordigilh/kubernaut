# Test Plan: Conversational RAR — Audit-Seeded Investigation Queries

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-592-v5.0
**Feature**: Conversational API backend for the Kubernaut Agent, scoped to RemediationApprovalRequest (RAR) review (backend only — Slack bot #633 and kubectl plugin #634 deferred to v1.5)
**Version**: 5.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Active
**Branch**: `development/v1.4`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the conversational RAR backend API introduced by Issue #592. The feature adds a conversation API to the Kubernaut Agent that allows operators to question the KA about a pending approval decision. Conversations are seeded from the investigation audit trail so the LLM "remembers" the full investigation.

**v4.0 scope changes from v2.0**:
- Added **Phase 0: Audit Completeness Fix** — prerequisite ensuring `InvestigationResult` and LLM prompts/responses are stored in full (not truncated) in audit events. Without this, conversation reconstruction would be incomplete.
- Corrected all audit event types to match real `emitter.go` constants (`aiagent.llm.request`, `aiagent.llm.response`, `aiagent.llm.tool_call`, `aiagent.response.complete`).
- Added structured `messages` array schema (not `prompt_content`) for LLM prompt storage (A14 resolution).
- Added missing acceptance criteria tests: read-only toolsets (A6), mutating tool refusal (A7), LLM failure SSE (A8), RR completed lifecycle (A9), TLS enforcement (A10).
- Renumbered `UT-CS-592-010-TTL` to `UT-CS-592-030` to avoid ID collision with `UT-CS-592-010` (A5).
- No backward compatibility with v1.3 audit events (user decision).

### 1.2 Objectives

1. **Audit completeness**: Full `InvestigationResult`, full LLM prompt messages, and full analysis responses are stored in audit events (Phase 0 prerequisite).
2. **Audit-seeded context**: Conversation session rehydrates audit trail as LLM history — operator talks to the same "mind" that investigated.
3. **RR-scoped guardrails**: LLM refuses queries about other RemediationRequests; only read-only toolsets are permitted.
4. **TLS + Auth**: All endpoints over HTTPS; K8s TokenReview + SubjectAccessReview on every request, using dynamic auth middleware.
5. **SSE streaming**: Real-time token streaming with reconnection support; LLM failures surface as SSE error events.
6. **Session model**: Shared session per RAR, multiple participants, identity tracked per turn, TTL-based expiry.
7. **Rate limiting**: Per-user (10/min) and per-session (30 turns) limits.
8. **Override advisory**: Conversation-mode override suggestions with catalog validation. CRD types and webhook are in #594.
9. **Conversation lifecycle**: Session transitions to read-only on RAR approval/rejection/expiry and on RR completion/failure.
10. **Configurable LLM model**: Conversation uses a separately configurable LLM, defaulting to the investigation model (#601 shadow agent pattern). Enables cheaper model for interactive Q&A.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `ginkgo ./test/unit/kubernautagent/investigator/... ./test/unit/kubernautagent/conversation/...` |
| Integration test pass rate | 100% | `ginkgo ./test/integration/kubernautagent/conversation/...` |
| E2E test pass rate | 100% | `make test-e2e-kubernautagent` with `--ginkgo.focus="CS-592"` |
| Unit-testable code coverage | >=80% | Audit completeness, session, audit reconstruction, auth, SSE, rate limit, override, lifecycle, config |
| Integration-testable code coverage | >=80% | Conversation flow, SSE streaming, auth middleware, lifecycle transitions, LLM failure, TLS |
| E2E-testable code coverage | System-level | Binary coverage profiling (Go 1.20+); primary goal is behavioral validation |
| Backward compatibility | 0 regressions | Existing KA tests pass without modification |

---

## 2. References

### 2.1 Authority

- Issue #592: Enhancement proposal: Conversational mode for Kubernaut Agent — RAR-scoped investigation queries
- Issue #601: Prompt injection guardrails / shadow agent pattern (configurable conversation model)
- Issue #594: RAR operator overrides (CRD types, webhook — override advisory depends on catalog)
- Issue #285: NetworkPolicies (KA needs ingress for conversation endpoint)
- DD-AUDIT-005: Hybrid audit trail; Phase 0 fixes v1.3 gaps
- DD-AUTH-014 v3.0: Middleware-based SAR authentication
- ADR-036: Cluster-wide TokenReview/SAR strategy
- ADR-048 + Addendum 001: chi Throttle rate limiting
- DD-HTTP-001: KA chi-based REST API

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Implementation Plan](IMPLEMENTATION_PLAN.md)
- Source of Truth: [592_conversation_api_v4 cursor plan](../../../.cursor/plans/592_conversation_api_v4_1f21facf.plan.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Audit trail incomplete (ingestion lag) | Session created with partial context | Medium | UT-CS-592-003, UT-CS-592-004, IT-CS-592-002 | Retry with exponential backoff (100ms→3200ms); error with descriptive message if still empty |
| R2 | SSE connection dropped by corporate proxy | Operator loses streaming response | Medium | UT-CS-592-013..015, IT-CS-592-003 | Event IDs for reconnection; 60s response buffer; proxy-friendly headers (`Cache-Control: no-cache`, `X-Accel-Buffering: no`) |
| R3 | Token budget exceeded with large investigation traces | LLM context window overflow | Medium | UT-CS-592-005 | Summarize older turns; selective reconstruction |
| R4 | Unauthorized access to conversation | Security breach — operator sees other RR data | High | UT-CS-592-010..012, IT-CS-592-006 | TokenReview + dynamic SAR on every request (DD-AUTH-014 v3.0) |
| R5 | Session memory leak (no cleanup) | KA OOM over time | Low | UT-CS-592-030, UT-CS-592-008 | TTL-based session expiry with reaper goroutine |
| R6 | RR-scope bypass via prompt injection | LLM discusses other RRs or executes mutating commands | Medium | UT-CS-592-006, UT-CS-592-007, UT-CS-592-031, UT-CS-592-032 | System prompt scoping + namespace restriction + verb-based tool filtering |

### 3.1 Risk-to-Test Traceability

- **R1** (audit lag): UT-CS-592-003, UT-CS-592-004, IT-CS-592-002
- **R2** (SSE proxy): UT-CS-592-013..015, IT-CS-592-003
- **R3** (token budget): UT-CS-592-005
- **R4** (auth): UT-CS-592-010..012, IT-CS-592-006
- **R5** (session leak): UT-CS-592-030, UT-CS-592-008
- **R6** (scope bypass): UT-CS-592-006, UT-CS-592-007, UT-CS-592-031, UT-CS-592-032

---

## 4. Scope

### 4.1 Features to be Tested

**Phase 0 (Prerequisite — Audit Completeness)**:
- **Audit event data completeness** (`internal/kubernautagent/investigator/investigator.go`, `internal/kubernautagent/audit/ds_store.go`): Full `InvestigationResult` in `aiagent.response.complete`, full `messages` array in `aiagent.llm.request`, full `analysis_content` in `aiagent.llm.response`
- **DataStorage OpenAPI schema extensions** (`pkg/datastorage/server/middleware/openapi_spec_data.yaml`): `LLMMessage` type, `messages` array, `analysis_content`, expanded `IncidentResponseData`

**Phase 1a (MVP — read-only conversation)**:
- **Audit reconstruction** (`internal/kubernautagent/conversation/audit.go`): Fetch audit chain from DataStorage, reconstruct LLM history using structured `messages` and `analysis_content` fields
- **Session management** (`internal/kubernautagent/conversation/session.go`): Create/get/delete, TTL reaper, shared sessions
- **TLS + SSE endpoint** (`internal/kubernautagent/conversation/handler.go`): HTTPS, SSE streaming with event IDs, reconnection, LLM failure error events
- **Auth middleware** (`internal/kubernautagent/conversation/auth.go`): K8s TokenReview + dynamic SAR per request
- **RR-scoped guardrails** (`internal/kubernautagent/conversation/guardrails.go`): System prompt scoping, namespace enforcement, read-only toolset filtering
- **Conversation prompt template** (`internal/kubernautagent/prompt/templates/conversation.tmpl`): RR-scoped system prompt
- **Conversation config** (`internal/kubernautagent/config/config.go`): `ConversationConfig` with optional LLM override, rate limits, session TTL
- **Rate limiting** (`internal/kubernautagent/conversation/ratelimit.go`): Per-user and per-session
- **Conversation audit trail**: Emit audit events per turn with identity + correlation_id

**Phase 1b (Override + Lifecycle)**:
- **Override advisory** (`internal/kubernautagent/conversation/override.go`): Validate against catalog/schema
- **Lifecycle management** (`internal/kubernautagent/conversation/lifecycle.go`): Detect RAR/RR state changes, transition to read-only/closed
- **RBAC** (`charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml`): `get` permission on `remediationapprovalrequests` and `remediationworkflows`

### 4.2 Features Not to be Tested

- **Slack bot** (#633, v1.5): Deferred — OCP Console Plugin is the v1.4 client
- **kubectl plugin** (#634, v1.5): Deferred — universal fallback client
- **Override CRD types and webhook validation** (#594): Separate issue, separate test plan
- **OCP Console Plugin UI** (#632): Separate repository — consumes this API
- **Session persistence in DataStorage** (v1.5): In-memory only for v1.4
- **Cross-channel bridge** (v1.5): No redirect from PD/Teams to Slack

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Phase 0 before conversation code | Conversation reconstruction depends on complete audit data; truncated previews are insufficient |
| Structured `messages` array (not `prompt_content` string) | Machine-parseable, supports role/tool_call_id metadata, aligns with LLM API standards |
| Dynamic SAR middleware for `/conversations` | Conversation routes need per-request identity; investigation routes use static auth via `r.Group` |
| Configurable conversation LLM defaulting to investigation model | Allows cheaper model for Q&A without disrupting investigation quality (#601 pattern) |
| No backward compatibility for audit events | User decision — simplifies schema evolution for v1.4 |
| `CapturingAuditStore` for Phase 0 tests | Anti-pattern compliance — tests invoke business logic and assert on side effects, never call `StoreAudit` directly |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% — audit completeness, audit reconstruction, session management, auth, SSE, rate limiting, override validation, lifecycle, config, guardrails
- **Integration**: >=80% — end-to-end conversation flow, SSE streaming, auth middleware, lifecycle transitions, LLM failure handling, TLS enforcement
- **E2E**: System-level behavioral validation — KA + DataStorage + MockLLM in Kind cluster. 7 tests validating real K8s auth, SSE streaming, audit persistence, rate limiting. Coverage measured via Go 1.20+ binary profiling.

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers (UT + IT where applicable):
- **Unit tests**: Catch logic and correctness errors (fast feedback, isolated)
- **Integration tests**: Catch wiring, data fidelity, and behavior errors across component boundaries

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes** — behavior, correctness, and data accuracy. Each test scenario answers: "what does the operator/system get?" not "what function is called?"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All 47 tests pass (33 unit + 7 integration + 7 E2E)
2. Per-tier code coverage meets >=80% threshold
3. Auth enforced on all conversation paths (no bypass)
4. SSE reconnection works with `Last-Event-ID`
5. Rate limits enforced per-user and per-session
6. No regressions in existing KA test suite

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80%
3. Auth bypass possible on any conversation path
4. Existing KA tests regress

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Schema regeneration (`make generate-ogen`) breaks existing DS tests
- KA code does not compile (`go build ./...` fails)
- Blocking dependency not available (e.g., #433 KA Go rewrite)

**Resume testing when**:
- Build is green; schema changes verified against DS test suite
- Blocking dependency merged

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `resultToAuditJSON`, `runLLMLoop` (audit emission) | ~80 |
| `internal/kubernautagent/audit/ds_store.go` | `buildEventData` | ~100 |
| `internal/kubernautagent/conversation/audit.go` | `FetchInvestigationHistory`, `eventsToMessages` | ~150 |
| `internal/kubernautagent/conversation/session.go` | `Create`, `Get`, `Delete`, `AddParticipant`, TTL reaper | ~120 |
| `internal/kubernautagent/conversation/guardrails.go` | Namespace enforcement, verb filtering | ~80 |
| `internal/kubernautagent/conversation/auth.go` | `ConversationAuthMiddleware.Handler` | ~60 |
| `internal/kubernautagent/conversation/sse.go` | `WriteEvent`, ring buffer, `Last-Event-ID` replay | ~100 |
| `internal/kubernautagent/conversation/ratelimit.go` | Per-user bucket, per-session counter | ~80 |
| `internal/kubernautagent/conversation/override.go` | `ValidateOverride` | ~60 |
| `internal/kubernautagent/conversation/lifecycle.go` | `CheckLifecycle` (RAR/RR state) | ~80 |
| `internal/kubernautagent/config/config.go` | `ConversationConfig.EffectiveLLM` | ~30 |
| `internal/kubernautagent/prompt/builder.go` | `RenderConversation` | ~40 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/conversation/handler.go` | `CreateSession`, `PostMessage`, `StreamSSE`, `GetSession`, `CloseSession` | ~200 |
| `cmd/kubernautagent/main.go` | Conversation route wiring, dependency injection | ~50 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.4` HEAD | Rebased on `origin/development/v1.3` |
| KA Go rewrite (#433) | v1.3 | Foundation for all KA code |
| DataStorage audit API | v1.3 | `QueryAuditEvents` ogen endpoint |
| Shadow agent (#601) | v1.4 | Configurable conversation model pattern |
| RAR overrides (#594) | v1.4 | Override CRD types for advisory validation |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-CONV-000 | Audit event stores full `messages` array (not truncated preview) | P0 | Unit | UT-CS-592-026 | Pending |
| BR-CONV-000 | Audit event stores full `analysis_content` (not truncated preview) | P0 | Unit | UT-CS-592-027 | Pending |
| BR-CONV-000 | Audit event stores complete `InvestigationResult` (all fields) | P0 | Unit | UT-CS-592-028 | Pending |
| BR-CONV-000 | Audit event serializes both `human_review_reason` and `reason` | P0 | Unit | UT-CS-592-029 | Pending |
| BR-CONV-001 | Audit chain reconstructed as LLM history | P0 | Unit | UT-CS-592-001 | Pending |
| BR-CONV-001 | Audit event type → LLM message mapping correct | P0 | Unit | UT-CS-592-002 | Pending |
| BR-CONV-001 | Retry with backoff on incomplete audit chain | P0 | Unit | UT-CS-592-003 | Pending |
| BR-CONV-001 | 404 when audit chain still empty after retries | P0 | Unit | UT-CS-592-004 | Pending |
| BR-CONV-001 | Token budget management (summarize old turns) | P1 | Unit | UT-CS-592-005 | Pending |
| BR-CONV-002 | RR-scoped system prompt refuses out-of-scope queries | P0 | Unit | UT-CS-592-006 | Pending |
| BR-CONV-002 | Tool calls restricted to target RR namespace | P0 | Unit | UT-CS-592-007 | Pending |
| BR-CONV-003 | Session create/get/delete lifecycle | P0 | Unit | UT-CS-592-008 | Pending |
| BR-CONV-003 | Shared session: multiple participants tracked | P0 | Unit | UT-CS-592-009 | Pending |
| BR-CONV-004 | TokenReview validates bearer token | P0 | Unit | UT-CS-592-010 | Pending |
| BR-CONV-004 | SAR: user must be able to UPDATE target RAR | P0 | Unit | UT-CS-592-011 | Pending |
| BR-CONV-004 | 403 on failed SAR | P0 | Unit | UT-CS-592-012 | Pending |
| BR-CONV-005 | SSE streaming with event IDs | P0 | Unit | UT-CS-592-013 | Pending |
| BR-CONV-005 | SSE reconnection via Last-Event-ID | P1 | Unit | UT-CS-592-014 | Pending |
| BR-CONV-005 | 60s response buffer for reconnection | P1 | Unit | UT-CS-592-015 | Pending |
| BR-CONV-006 | Rate limit: per-user (10/min default) | P0 | Unit | UT-CS-592-016 | Pending |
| BR-CONV-006 | Rate limit: per-session (30 turns default) | P0 | Unit | UT-CS-592-017 | Pending |
| BR-CONV-007 | Conversation audit events with identity | P1 | Unit | UT-CS-592-018 | Pending |
| BR-CONV-008 | Override: validate workflow against catalog | P1 | Unit | UT-CS-592-019 | Pending |
| BR-CONV-008 | Override disabled when RAR approved/rejected | P0 | Unit | UT-CS-592-020 | Pending |
| BR-CONV-009 | RAR approved → session read-only | P0 | Unit | UT-CS-592-021 | Pending |
| BR-CONV-009 | RAR expired → session closed | P0 | Unit | UT-CS-592-022 | Pending |
| BR-CONV-010 | Conversation LLM config defaults to investigation model | P0 | Unit | UT-CS-592-023 | Pending |
| BR-CONV-010 | Conversation LLM config uses override when set | P0 | Unit | UT-CS-592-024 | Pending |
| BR-CONV-009 | RAR pending → session interactive | P0 | Unit | UT-CS-592-025 | Pending |
| BR-CONV-003 | Session TTL: inactive sessions expire | P1 | Unit | UT-CS-592-030 | Pending |
| BR-CONV-002 | Read-only tool call succeeds during conversation | P0 | Unit | UT-CS-592-031 | Pending |
| BR-CONV-002 | Mutating tool call in correct namespace rejected | P0 | Unit | UT-CS-592-032 | Pending |
| BR-CONV-009 | RR completed/failed → session read-only | P0 | Unit | UT-CS-592-033 | Pending |
| BR-CONV-001 | End-to-end: create session → send message → stream response | P0 | Integration | IT-CS-592-001 | Pending |
| BR-CONV-001 | End-to-end: audit chain → session → conversation | P0 | Integration | IT-CS-592-002 | Pending |
| BR-CONV-005 | SSE streaming end-to-end | P0 | Integration | IT-CS-592-003 | Pending |
| BR-CONV-004 | Auth middleware blocks unauthorized request | P0 | Integration | IT-CS-592-006 | Pending |
| BR-CONV-009 | Lifecycle: RAR state change → session transition → 409 | P0 | Integration | IT-CS-592-007 | Pending |
| BR-CONV-005 | LLM failure mid-stream → SSE error event | P0 | Integration | IT-CS-592-008 | Pending |
| BR-CONV-004 | TLS enforcement: non-TLS rejected, TLS accepted | P0 | Integration | IT-CS-592-009 | Pending |
| BR-CONV-004 | Session creation with real K8s TokenReview + SAR auth | P0 | E2E | E2E-CS-592-001 | Pending |
| BR-CONV-001 | Full conversation flow: session → message → SSE response | P0 | E2E | E2E-CS-592-002 | Pending |
| BR-CONV-007 | Conversation turn audit event persisted in DataStorage | P0 | E2E | E2E-CS-592-003 | Pending |
| BR-CONV-004 | Unauthorized access rejected (missing token → 401) | P0 | E2E | E2E-CS-592-004 | Pending |
| BR-CONV-006 | Rate limiting enforced (per-user 10/min → 429) | P1 | E2E | E2E-CS-592-005 | Pending |
| BR-CONV-001 | Investigation-seeded conversation session | P1 | E2E | E2E-CS-592-006 | Pending |
| BR-CONV-005 | SSE event IDs unique across conversation turns | P1 | E2E | E2E-CS-592-007 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-CS-592-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **CS**: Conversation Service abbreviation
- **592**: Issue number
- **SEQUENCE**: Zero-padded 3-digit (001–033 for unit; 001–009 for integration)

### Tier 1: Unit Tests (33 tests)

**Phase 0 — Audit Completeness** (4 tests)

| ID | Business Outcome Under Test | TDD Cycle | Phase |
|----|----------------------------|-----------|-------|
| `UT-CS-592-026` | `aiagent.llm.request` event contains `messages` as structured `[]LLMMessage` array | Cycle 0 | Pending |
| `UT-CS-592-027` | `aiagent.llm.response` event contains `analysis_content` (full response, not 500-char preview) | Cycle 0 | Pending |
| `UT-CS-592-028` | `aiagent.response.complete` event `response_data` includes `detected_labels`, `validation_attempts_history`, `is_actionable`, `signal_name`, full alternative workflows | Cycle 0 | Pending |
| `UT-CS-592-029` | `aiagent.response.complete` event serializes both `human_review_reason` and `reason` fields (bug fix) | Cycle 0 | Pending |

**Audit Reconstruction** (5 tests)

| ID | Business Outcome Under Test | TDD Cycle | Phase |
|----|----------------------------|-----------|-------|
| `UT-CS-592-001` | Audit chain (5 events) → reconstructed as 5 LLM messages with correct roles (from structured `messages`) | Cycle 1 | Pending |
| `UT-CS-592-002` | Each audit event type maps to correct LLM message type (system/user/assistant/function) | Cycle 1 | Pending |
| `UT-CS-592-003` | Incomplete audit chain → retries with exponential backoff (100ms→3200ms) | Cycle 1 | Pending |
| `UT-CS-592-004` | Empty audit chain after retries → error with descriptive message | Cycle 1 | Pending |
| `UT-CS-592-005` | Large audit chain exceeds token budget → older turns summarized | Cycle 1 | Pending |

**Session + Guardrails** (4 tests)

| ID | Business Outcome Under Test | TDD Cycle | Phase |
|----|----------------------------|-----------|-------|
| `UT-CS-592-006` | System prompt includes RR name/namespace scoping | Cycle 2 | Pending |
| `UT-CS-592-007` | Tool call validator rejects calls outside target namespace | Cycle 2 | Pending |
| `UT-CS-592-008` | Session CRUD: create with RAR ref, get by ID, delete | Cycle 2 | Pending |
| `UT-CS-592-009` | Shared session: two user IDs in same session, both tracked in turns | Cycle 2 | Pending |

**Config + TTL + Read-Only Tools** (5 tests)

| ID | Business Outcome Under Test | TDD Cycle | Phase |
|----|----------------------------|-----------|-------|
| `UT-CS-592-023` | Conversation LLM config defaults to investigation model when `LLM` field is nil | Cycle 3 | Pending |
| `UT-CS-592-024` | Conversation LLM config uses override when `LLM` field is explicitly set | Cycle 3 | Pending |
| `UT-CS-592-030` | Session TTL: session created, idle beyond TTL, get returns not found | Cycle 3 | Pending |
| `UT-CS-592-031` | Read-only tool call (`kubectl get pods`) succeeds during conversation | Cycle 3 | Pending |
| `UT-CS-592-032` | Mutating tool call (`kubectl delete pod`) in correct namespace rejected | Cycle 3 | Pending |

**Authentication** (3 tests)

| ID | Business Outcome Under Test | TDD Cycle | Phase |
|----|----------------------------|-----------|-------|
| `UT-CS-592-010` | TokenReview: valid bearer token → authenticated identity extracted | Cycle 4 | Pending |
| `UT-CS-592-011` | SAR: user can UPDATE target RAR → authorized | Cycle 4 | Pending |
| `UT-CS-592-012` | SAR: user cannot UPDATE target RAR → 403 Forbidden | Cycle 4 | Pending |

**SSE Streaming** (3 tests)

| ID | Business Outcome Under Test | TDD Cycle | Phase |
|----|----------------------------|-----------|-------|
| `UT-CS-592-013` | SSE writer: events include incrementing ID field | Cycle 5 | Pending |
| `UT-CS-592-014` | SSE reconnection: `Last-Event-ID` header → resumes from correct event | Cycle 5 | Pending |
| `UT-CS-592-015` | SSE buffer: response stored for 60s, available for reconnection | Cycle 5 | Pending |

**Rate Limiting + Audit Identity** (3 tests)

| ID | Business Outcome Under Test | TDD Cycle | Phase |
|----|----------------------------|-----------|-------|
| `UT-CS-592-016` | Per-user rate limit: 11th request within 1 min → 429 | Cycle 6 | Pending |
| `UT-CS-592-017` | Per-session rate limit: 31st turn → 429 | Cycle 6 | Pending |
| `UT-CS-592-018` | Each conversation turn emits audit event with identity + `correlation_id` | Cycle 6 | Pending |

**Override Advisory** (2 tests)

| ID | Business Outcome Under Test | TDD Cycle | Phase |
|----|----------------------------|-----------|-------|
| `UT-CS-592-019` | Override: workflow validated against catalog schema | Cycle 9 | Pending |
| `UT-CS-592-020` | Override: RAR approved → override request returns error | Cycle 9 | Pending |

**Lifecycle Management** (4 tests)

| ID | Business Outcome Under Test | TDD Cycle | Phase |
|----|----------------------------|-----------|-------|
| `UT-CS-592-021` | RAR `status.decision` = `"Approved"` → `session.IsReadOnly()` returns true | Cycle 10 | Pending |
| `UT-CS-592-022` | RAR `status.decision` = `"Expired"` → `session.IsClosed()` returns true | Cycle 10 | Pending |
| `UT-CS-592-025` | RAR `status.decision` = `""` (pending) → session remains interactive | Cycle 10 | Pending |
| `UT-CS-592-033` | RR `status.phase` = `Completed`/`Failed` → session read-only for post-decision review | Cycle 10 | Pending |

### Tier 2: Integration Tests (7 tests)

| ID | Business Outcome Under Test | TDD Cycle | Phase |
|----|----------------------------|-----------|-------|
| `IT-CS-592-001` | Create session → POST message → receive SSE stream with LLM tokens | Cycle 7 | Pending |
| `IT-CS-592-002` | Audit chain from DS mock → session seeded → first message continues conversation | Cycle 7 | Pending |
| `IT-CS-592-003` | SSE stream delivers tokens incrementally (not all at once) | Cycle 7 | Pending |
| `IT-CS-592-006` | Missing/invalid bearer token → 401; valid token + no SAR → 403 | Cycle 7 | Pending |
| `IT-CS-592-007` | RAR CR status change (approved) → session transitions to read-only → subsequent message → 409 | Cycle 8 | Pending |
| `IT-CS-592-008` | LLM failure mid-stream → SSE `event: error` delivered to client | Cycle 8 | Pending |
| `IT-CS-592-009` | TLS enforcement: non-TLS connection rejected; TLS connection succeeds | Cycle 8 | Pending |

### Tier 3: E2E Tests (7 tests)

**Testable code scope**: Full conversation API stack in Kind cluster — handler wiring, real K8s auth (TokenReview + SAR), SSE streaming, audit persistence in DataStorage, rate limiting.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-CS-592-001` | Operator creates a conversation session with real K8s auth (TokenReview + SAR) and receives a valid session ID | Pending |
| `E2E-CS-592-002` | Operator sends a message and receives an SSE stream with LLM response tokens including valid event IDs and JSON payloads | Pending |
| `E2E-CS-592-003` | Conversation turn audit event (`aiagent.conversation.turn`) is persisted in DataStorage with correct action and outcome fields | Pending |
| `E2E-CS-592-004` | Request without bearer token is rejected with 401 and RFC 7807 problem detail | Pending |
| `E2E-CS-592-005` | Per-user rate limit (10/min) is enforced — 11th request within 1 minute returns 429 | Pending |
| `E2E-CS-592-006` | Conversation session is created and responds to messages even without prior investigation audit trail | Pending |
| `E2E-CS-592-007` | SSE event IDs are unique numeric strings across multiple conversation turns, supporting reconnection | Pending |

---

## 9. Test Cases

### Phase 0 Tests (P0 — Anti-Pattern Compliant)

#### UT-CS-592-026: Full messages stored in LLM request audit event

**BR**: BR-CONV-000
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/audit_completeness_test.go`

**Preconditions**:
- `CapturingAuditStore` implements `audit.AuditStore` with an `Events` slice
- `Investigator` constructed with: `CapturingAuditStore`, mock LLM client (scripted responses), mock `ResultParser` (populated `InvestigationResult`)

**Test Steps**:
1. **Given**: An `Investigator` with a `CapturingAuditStore` and mock LLM returning a scripted response
2. **When**: `inv.Investigate(ctx, signal)` is called
3. **Then**: `capturingStore.EventsByType("aiagent.llm.request")` returns events where `event.Data["messages"]` is a structured `[]LLMMessage` array containing system prompt and user content

**Expected Results**:
1. `messages` field is present and non-empty in the event data
2. Each message has `role` (system/user/assistant) and `content` fields
3. `prompt_preview` field remains populated (backward-compatible dashboard display)

**Acceptance Criteria**:
- **Behavior**: Business logic (`Investigate`) emits complete prompt data
- **Anti-pattern**: No direct `StoreAudit` calls in test; assertion is on side effects of business logic

#### UT-CS-592-028: Complete InvestigationResult in response audit event

**BR**: BR-CONV-000
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/audit_completeness_test.go`

**Preconditions**:
- Mock `ResultParser` returns a fully-populated `InvestigationResult` with `detected_labels`, `validation_attempts_history`, `is_actionable`, `signal_name`, and per-alternative `execution_bundle`/`confidence`

**Test Steps**:
1. **Given**: A fully-populated `InvestigationResult` from the parser mock
2. **When**: `inv.Investigate(ctx, signal)` completes
3. **Then**: `capturingStore.EventsByType("aiagent.response.complete")` returns events where `response_data` contains all fields from the full `InvestigationResult`

**Expected Results**:
1. `detected_labels` present in `response_data`
2. `validation_attempts_history` present in `response_data`
3. `is_actionable` and `signal_name` present
4. Each alternative includes `execution_bundle` and `confidence`

### Core Conversation Tests (P0 — Selected)

#### UT-CS-592-001: Audit chain reconstructed as LLM history

**BR**: BR-CONV-001
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/conversation/audit_test.go`

**Preconditions**:
- `MockAuditReader` returns 5 pre-built `[]ogenclient.AuditEvent` with structured `messages` and `analysis_content`

**Test Steps**:
1. **Given**: 5 audit events (1 `aiagent.llm.request`, 1 `aiagent.llm.response`, 2 `aiagent.llm.tool_call`, 1 `aiagent.response.complete`)
2. **When**: `AuditChainFetcher.FetchInvestigationHistory(ctx, correlationID)` is called
3. **Then**: 5 LLM messages returned with correct roles from structured `messages` fields

**Expected Results**:
1. Message count = 5
2. Roles match: request → user messages, response → assistant, tool_call → function, response.complete → assistant summary

#### IT-CS-592-001: End-to-end conversation flow

**BR**: BR-CONV-001
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/conversation/conversation_test.go`

**Preconditions**:
- `httptest.NewTLSServer` with conversation handler
- Mock DS returning audit events for a known correlation ID
- Mock LLM streaming tokens

**Test Steps**:
1. **Given**: A running conversation handler with mock dependencies
2. **When**: `POST /conversations` → get session ID → `POST /conversations/{id}/messages` with question → `GET /conversations/{id}/stream`
3. **Then**: SSE stream delivers LLM response tokens incrementally

**Expected Results**:
1. Session created with 201 status
2. Message accepted with 200 status
3. SSE stream delivers `event: token` events with incrementing IDs
4. Final `event: done` event marks completion

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**:
  - `CapturingAuditStore` (implements `audit.AuditStore`) — for Phase 0 anti-pattern compliance
  - `MockAuditReader` (implements `AuditReader` interface wrapping `ogenclient.QueryAuditEvents`) — for audit chain reconstruction
  - Mock `llm.Client` (scripted LLM responses) — for Phase 0 and conversation LLM calls
  - Mock `ResultParser` (fully-populated `InvestigationResult`) — for Phase 0
  - Mock `Authenticator` / `Authorizer` (from `pkg/shared/auth`) — for auth middleware
  - Mock `dynamic.Interface` (crafted unstructured RAR/RR objects) — for lifecycle polling
- **Location**: `test/unit/kubernautagent/investigator/` (Phase 0), `test/unit/kubernautagent/conversation/` (all other)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: `httptest.NewTLSServer` with conversation handler, mock DataStorage + LLM + K8s auth
- **Location**: `test/integration/kubernautagent/conversation/`

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: Kind cluster with KA + DataStorage + Mock LLM + PostgreSQL + Redis
- **CRDs**: `kubernaut.ai_remediationapprovalrequests.yaml`, `kubernaut.ai_remediationworkflows.yaml`
- **Auth**: Real K8s TokenReview + SubjectAccessReview via ServiceAccount bearer tokens
- **RBAC**: E2E SA granted `update` on `remediationapprovalrequests` in `kubernaut.ai` API group
- **Config**: KA ConfigMap includes `conversation.enabled: true` with 5m TTL, 10/min rate limit, 30 session turns
- **Location**: `test/e2e/kubernautagent/conversation_e2e_test.go`
- **Resources**: Kind cluster (~4 GB RAM, ~2 CPU), Docker daemon
- **Timeout**: 15 minutes (shared with existing KA E2E suite)

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| ogen | latest | Client regeneration after schema changes |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| KA Go rewrite (#433) | Code | Merged (v1.3) | Server, LLM client, audit store | N/A |
| DataStorage audit events | Code | Available (v1.3) | Audit chain fetch | N/A |
| DataStorage OpenAPI schema | Schema | Must be extended (Phase 0) | `LLMMessage` type, `messages`, `analysis_content` | Phase 0 extends schema before conversation code |
| #601 (shadow agent) | Code | Planned (v1.4) | Configurable conversation model pattern | Can be added post-implementation |
| #594 (RAR overrides) | Code | Planned (v1.4) | Override CRD types for catalog validation | Override advisory is advisory-only |

### 11.2 Execution Order

1. **Phase 0**: Audit Completeness — Schema extension + investigator/ds_store fixes (TDD Cycle 0)
2. **Cycles 1–3**: Core — Audit reconstruction + session + guardrails + config + TTL
3. **Cycles 4–6**: Security — Auth + SSE + rate limiting
4. **Cycles 7–8**: Integration — Handler wiring + lifecycle/LLM-failure/TLS integration tests
5. **Cycles 9–10**: Advanced — Override advisory + lifecycle management

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/592/TEST_PLAN.md` | Strategy and test design (v5.0) |
| Implementation plan | `docs/tests/592/IMPLEMENTATION_PLAN.md` | TDD cycle execution plan (v4.0) |
| Phase 0 unit tests | `test/unit/kubernautagent/investigator/audit_completeness_test.go` | Audit completeness validation |
| Conversation unit tests | `test/unit/kubernautagent/conversation/` | Ginkgo BDD test files (29 tests) |
| Integration test suite | `test/integration/kubernautagent/conversation/` | Ginkgo BDD test files (7 tests) |
| E2E test suite | `test/e2e/kubernautagent/conversation_e2e_test.go` | Ginkgo BDD E2E tests (7 tests) |
| E2E infrastructure | `test/infrastructure/kubernautagent.go` | Kind cluster + CRD + RBAC for conversation E2E |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Phase 0 unit tests (audit completeness)
ginkgo -v ./test/unit/kubernautagent/investigator/...

# Conversation unit tests
ginkgo -v ./test/unit/kubernautagent/conversation/...

# Integration tests
ginkgo -v ./test/integration/kubernautagent/conversation/...

# E2E tests (requires Kind cluster — ~15 min)
make test-e2e-kubernautagent

# E2E conversation tests only
GINKGO_FOCUS="CS-592" make test-e2e-kubernautagent

# All #592 tests (unit + integration)
ginkgo -v --focus="CS-592" ./test/unit/kubernautagent/... ./test/integration/kubernautagent/...

# Coverage
go test ./test/unit/kubernautagent/conversation/... -coverprofile=coverage-unit.out
go test ./test/integration/kubernautagent/conversation/... -coverprofile=coverage-integration.out
go tool cover -func=coverage-unit.out
go tool cover -func=coverage-integration.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| Existing investigator unit tests | Assert on truncated `prompt_preview` / `analysis_preview` | May need to additionally assert `messages` / `analysis_content` fields | Phase 0 adds new fields; existing tests should still pass but may optionally verify new fields |
| DS integration tests | Assert on current `IncidentResponseData` schema | Must pass with extended schema (new optional fields) | Phase 0 extends `IncidentResponseData` with `detected_labels`, `validation_attempts_history`, etc. |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 5.0 | 2026-03-04 | Added E2E tier: 7 tests (E2E-CS-592-001 through 007) covering real K8s auth, SSE streaming, audit persistence, rate limiting, investigation-seeded conversations, SSE reconnection IDs. Infrastructure changes: KA ConfigMap enables conversation, CRD installation for RAR/RW, RBAC for E2E SA. Updated pass criteria from 40 to 47 tests. Removed E2E tier skip rationale. |
| 1.0 | 2026-03-04 | Initial test plan |
| 2.0 | 2026-03-04 | Scope reduction: remove Slack bot (#633/v1.5) and kubectl plugin (#634/v1.5). Add #594 dependency for override CRD types. OCP Console Plugin (#632) is the v1.4 client. |
| 4.0 | 2026-04-07 | Full rewrite: add Phase 0 (audit completeness, 4 tests), correct audit event types to `emitter.go` constants, add structured `messages` schema (not `prompt_content`), fix test count to 33 unit + 7 integration = 40 total, add missing acceptance criteria tests (UT-031 read-only tools, UT-032 mutating refusal, UT-033 RR lifecycle, IT-008 LLM failure SSE, IT-009 TLS), renumber UT-010-TTL to UT-030, add configurable LLM tests (UT-023/024), add pending state test (UT-025), add anti-pattern-compliant test infrastructure, 4 checkpoints. No backward compatibility for audit events. |
