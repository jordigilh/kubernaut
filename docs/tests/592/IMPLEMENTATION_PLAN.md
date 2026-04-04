# Implementation Plan: Conversational RAR — Audit-Seeded Investigation Queries

**Issue**: #592
**Test Plan**: [TP-592-v1.0](TEST_PLAN.md)
**Branch**: `development/v1.4`
**Created**: 2026-03-04

---

## Overview

This plan implements a conversational API for the Kubernaut Agent, scoped to RAR review. The conversation is seeded from the investigation audit trail, allowing operators to question the KA about pending approval decisions.

### Phasing

- **Phase 1a** (MVP): Read-only conversation — audit reconstruction, session management, TLS + SSE, auth, guardrails, rate limiting, audit trail (~8-10 days)
- **Phase 1b** (Override + Lifecycle): Advisory workflow/parameter overrides + lifecycle management (~2-3 days)

**Scope change (v2.0)**: Slack bot → #633/v1.5, kubectl plugin → #634/v1.5. OCP Console Plugin (#632) is the v1.4 client. Override CRD types and webhook validation are in #594; this plan covers only the conversation-mode override advisory.

### New packages

| Package | Purpose |
|---------|---------|
| `internal/kubernautagent/conversation/` | Conversation handler, session, audit reconstruction, auth, rate limit, SSE |
| `internal/kubernautagent/conversation/override/` | Override advisory logic |

---

## Phase 1: TDD RED — Core Session + Audit (Phase 1a.1)

### Phase 1.1: Audit reconstruction tests (RED)

**File**: `test/unit/kubernautagent/conversation/audit_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-CS-592-001 | 5 audit events → 5 LLM messages with correct roles | `conversation` package doesn't exist |
| UT-CS-592-002 | Each audit event type maps correctly (prompt→system, tool_call→function, etc.) | Same |
| UT-CS-592-003 | Incomplete chain → retries with backoff | Same |
| UT-CS-592-004 | Empty chain after retries → error with descriptive message | Same |
| UT-CS-592-005 | Large chain → older turns summarized to fit token budget | Same |

### Phase 1.2: Session management tests (RED)

**File**: `test/unit/kubernautagent/conversation/session_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-CS-592-008 | Create/get/delete session lifecycle | Session manager doesn't exist |
| UT-CS-592-009 | Shared session with multiple participants | Same |
| UT-CS-592-010-TTL | TTL-based session expiry | Same |

### Phase 1.3: Guardrails tests (RED)

**File**: `test/unit/kubernautagent/conversation/guardrails_test.go`

| Test ID | What it asserts | Why it fails |
|---------|----------------|-------------|
| UT-CS-592-006 | System prompt scopes to specific RR name/namespace | Guardrails don't exist |
| UT-CS-592-007 | Tool call validator rejects cross-namespace calls | Same |

### Phase 1 Checkpoint

- [ ] All tests compile
- [ ] All tests FAIL
- [ ] Zero lint errors

---

## Phase 2: TDD GREEN — Core Implementation (Phase 1a.1)

### Phase 2.1: Audit reconstruction

**File**: `internal/kubernautagent/conversation/audit.go`

- `ReconstructHistory(ctx, correlationID, dsClient) ([]llm.Message, error)`
- Fetches audit events from DataStorage by `correlation_id`
- Maps event types to LLM message roles:

| Audit event type | LLM message role |
|-----------------|-----------------|
| `aiagent.investigation.prompt` | system |
| `aiagent.tool.call` | function_call |
| `aiagent.tool.result` | function_response |
| `aiagent.rca.completed` | assistant |
| `aiagent.enrichment.completed` | function_response |
| `aiagent.workflow.selected` | assistant |
| `aiagent.rego.evaluation` | system (context) |

- Retry with exponential backoff (100ms, 200ms, 400ms, 800ms, 1600ms, 3200ms — up to 10s total)
- Token budget management: if total tokens exceed limit, summarize oldest tool results

### Phase 2.2: Session management

**File**: `internal/kubernautagent/conversation/session.go`

- `SessionManager` with in-memory store (sync.Map)
- `CreateSession(rarRef, rrRef, auditHistory) → Session`
- `GetSession(id) → Session, error`
- `DeleteSession(id)`
- Session struct: `ID`, `RARRef`, `RRRef`, `Messages []llm.Message`, `Participants map[string]bool`, `CreatedAt`, `LastActivity`, `IsReadOnly`, `IsClosed`
- TTL reaper goroutine: evicts sessions idle > configured TTL

### Phase 2.3: RR-scoped guardrails

**File**: `internal/kubernautagent/conversation/guardrails.go`

- System prompt template: `"You are discussing RR {rr_name} in namespace {ns}. Do not answer questions about other remediation requests."`
- Tool call namespace validator: intercepts tool Execute, rejects if target namespace != session RR namespace

### Phase 2 Checkpoint

- [ ] UT-CS-592-001 through -009, -010-TTL, -006, -007 pass
- [ ] `go build ./...` succeeds

---

## Phase 3: TDD RED + GREEN — Auth + SSE + Rate Limiting (Phase 1a.2-3)

### Phase 3.1: Auth tests (RED then GREEN)

**File**: `test/unit/kubernautagent/conversation/auth_test.go`

| Test ID | Assertion |
|---------|-----------|
| UT-CS-592-010 | Valid token → authenticated identity |
| UT-CS-592-011 | SAR: user can UPDATE rar → authorized |
| UT-CS-592-012 | SAR: denied → 403 |

**Implementation**: `internal/kubernautagent/conversation/auth.go`
- `AuthMiddleware` wraps conversation endpoints
- `TokenReview` via `k8s.io/client-go/kubernetes` `AuthenticationV1().TokenReviews().Create()`
- `SubjectAccessReview` checking `update` on `remediationapprovalrequests` for target RAR

### Phase 3.2: SSE streaming tests (RED then GREEN)

**File**: `test/unit/kubernautagent/conversation/sse_test.go`

| Test ID | Assertion |
|---------|-----------|
| UT-CS-592-013 | Events include incrementing ID |
| UT-CS-592-014 | Last-Event-ID reconnection |
| UT-CS-592-015 | 60s response buffer |

**Implementation**: `internal/kubernautagent/conversation/sse.go`
- `SSEWriter` wraps `http.ResponseWriter` with `flusher`
- Event buffer (ring buffer, 60s TTL)
- `Last-Event-ID` header handling for resume
- Headers: `Cache-Control: no-cache`, `X-Accel-Buffering: no`, `Connection: keep-alive`

### Phase 3.3: Rate limiting tests (RED then GREEN)

**File**: `test/unit/kubernautagent/conversation/ratelimit_test.go`

| Test ID | Assertion |
|---------|-----------|
| UT-CS-592-016 | 11th request/min → 429 |
| UT-CS-592-017 | 31st turn/session → 429 |

**Implementation**: `internal/kubernautagent/conversation/ratelimit.go`
- Token bucket per user identity (from TokenReview)
- Turn counter per session
- Configurable via Helm values

### Phase 3.4: Conversation audit trail (RED then GREEN)

| Test ID | Assertion |
|---------|-----------|
| UT-CS-592-018 | Each turn emits audit event with identity |

**Implementation**: Emit to existing audit store with `correlation_id` matching investigation.

### Phase 3 Checkpoint

- [ ] All 18 unit tests pass
- [ ] Auth enforced on all endpoints
- [ ] SSE streaming works with reconnection
- [ ] Rate limits enforced

---

## Phase 4: TDD RED + GREEN — HTTP Handler + Integration (Phase 1a.2)

### Phase 4.1: Conversation HTTP handler

**File**: `internal/kubernautagent/conversation/handler.go`

Routes:
- `POST /api/v1/conversations` — Create session (body: `{rar_name, namespace}`)
- `GET /api/v1/conversations/{id}` — Get session info
- `POST /api/v1/conversations/{id}/messages` — Send message, receive SSE stream
- `DELETE /api/v1/conversations/{id}` — Close session

Middleware chain: TLS → Auth → Rate Limit → Handler

### Phase 4.2: Integration tests (RED then GREEN)

**File**: `test/integration/kubernautagent/conversation_test.go`

| Test ID | Assertion |
|---------|-----------|
| IT-CS-592-001 | Create session → POST message → SSE response |
| IT-CS-592-002 | Audit chain → session → first message continues |
| IT-CS-592-003 | SSE delivers tokens incrementally |
| IT-CS-592-006 | Auth: invalid token → 401; valid + no SAR → 403 |

### Phase 4 Checkpoint

- [ ] All 22 unit + 4 integration tests pass
- [ ] End-to-end conversation flow works with MockLLM

---

## Phase 5: TDD RED + GREEN — Override + Lifecycle (Phase 1b)

### Phase 5.1: Override advisory tests (RED then GREEN)

| Test ID | Assertion |
|---------|-----------|
| UT-CS-592-019 | Override validates workflow against catalog |
| UT-CS-592-020 | Override disabled when RAR approved |

**Implementation**: `internal/kubernautagent/conversation/override/override.go`
- Validate workflow exists in catalog
- Validate parameters against workflow schema
- Generate kubectl patch command
- Reject if session is read-only

### Phase 5.2: Lifecycle management tests (RED then GREEN)

| Test ID | Assertion |
|---------|-----------|
| UT-CS-592-021 | RAR approved → session read-only |
| UT-CS-592-022 | RAR expired → session closed |

**Implementation**: `internal/kubernautagent/conversation/lifecycle.go`
- Watch RAR status changes (informer)
- Transition session state on approval/rejection/expiry

### Phase 5.3: Integration test

| Test ID | Assertion |
|---------|-----------|
| IT-CS-592-007 | RAR CR status change → session transition → subsequent message → 409 |

### Phase 5 Checkpoint

- [ ] All 22 unit + 5 integration tests pass
- [ ] Override advisory works
- [ ] Lifecycle transitions verified

---

## Phase 6: TDD REFACTOR — Code Quality

### Phase 6.1: Error handling consistency

Ensure all error paths return proper HTTP status codes with descriptive messages:
- 401 for auth failures
- 403 for authorization failures
- 404 for missing sessions/audit chains
- 409 for read-only/closed sessions
- 429 for rate limit exceeded
- 500 for internal errors (with no sensitive info leaked)

### Phase 6.2: Structured logging

Add structured logging to all conversation components with `correlation_id`, `session_id`, `user_identity`.

### Phase 6.3: Metrics

- `kubernaut_conversation_sessions_active` (gauge)
- `kubernaut_conversation_turns_total` (counter, labels: user, session)
- `kubernaut_conversation_response_duration_seconds` (histogram)
- `kubernaut_conversation_rate_limited_total` (counter)

### Phase 6.4: Helm configuration

```yaml
kubernautAgent:
  conversations:
    enabled: false
    tls:
      secretName: ""
    rateLimit:
      perUser: 10
      perSession: 30
    session:
      ttl: "1h"
    tokenBudget: 128000
```

### Phase 6 Checkpoint

- [ ] All tests pass
- [ ] Structured logging in place
- [ ] Metrics registered
- [ ] Helm values documented

---

## Phase 7: Due Diligence & Commit

### Phase 7.1: Comprehensive audit

- [ ] TLS enforced on all conversation endpoints
- [ ] Auth enforced: no endpoint accessible without valid token + SAR
- [ ] RR scoping: LLM cannot discuss other RRs
- [ ] Rate limits: per-user and per-session enforced
- [ ] SSE: reconnection works with event IDs
- [ ] Session TTL: no memory leaks
- [ ] Audit trail: every turn recorded with identity
- [ ] Lifecycle: read-only transition on RAR approval
- [ ] Override: advisory only, does not mutate RAR CR (mutation is #594 webhook path)
- [ ] No sensitive data in error responses

### Phase 7.2: Commit in logical groups

| Commit # | Scope |
|----------|-------|
| 1 | `test(#592): TDD RED — failing tests for conversation session + audit reconstruction` |
| 2 | `feat(#592): audit chain reconstruction from DataStorage` |
| 3 | `feat(#592): conversation session management with TTL and shared sessions` |
| 4 | `feat(#592): RR-scoped guardrails and tool call namespace validation` |
| 5 | `feat(#592): TokenReview + SAR auth middleware for conversation endpoints` |
| 6 | `feat(#592): SSE streaming with event IDs and reconnection support` |
| 7 | `feat(#592): per-user and per-session rate limiting` |
| 8 | `feat(#592): conversation HTTP handler and route registration` |
| 9 | `feat(#592): conversation audit trail with identity tracking` |
| 10 | `feat(#592): override advisory logic with catalog validation` |
| 11 | `feat(#592): conversation lifecycle management (RAR state transitions)` |
| 12 | `refactor(#592): structured logging, metrics, and Helm configuration` |

---

## Estimated Effort

| Phase | Effort |
|-------|--------|
| Phase 1 (RED — core) | 1 day |
| Phase 2 (GREEN — core) | 2.5 days |
| Phase 3 (Auth + SSE + Rate) | 2.5 days |
| Phase 4 (Handler + Integration) | 1.5 days |
| Phase 5 (Override + Lifecycle) | 2 days |
| Phase 6 (REFACTOR) | 1.5 days |
| Phase 7 (Due Diligence) | 1 day |
| **Total** | **12 days** |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial implementation plan |
| 2.0 | 2026-03-04 | Scope reduction: remove Slack bot (#633/v1.5) and kubectl plugin (#634/v1.5). Reconcile override with #594. Effort reduced from 17d to 12d. |
