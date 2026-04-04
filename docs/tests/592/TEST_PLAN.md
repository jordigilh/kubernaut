# Test Plan: Conversational RAR — Audit-Seeded Investigation Queries

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-592-v1.0
**Feature**: Conversational interaction mode for the Kubernaut Agent, scoped to RemediationApprovalRequest (RAR) review, with Slack bot and kubectl plugin clients
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.4`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the conversational RAR feature introduced by Issue #592. The feature adds a conversation API to the Kubernaut Agent that allows operators to question the KA about a pending approval decision. Conversations are seeded from the investigation audit trail so the LLM "remembers" the full investigation.

Two client interfaces are validated: a Slack bot (thread replies) and a kubectl plugin (`kubectl kubernaut chat`).

### 1.2 Objectives

1. **Audit-seeded context**: Conversation session rehydrates audit trail as LLM history — operator talks to the same "mind" that investigated.
2. **RR-scoped guardrails**: LLM refuses queries about other RemediationRequests.
3. **TLS + Auth**: All endpoints over HTTPS; K8s TokenReview + SubjectAccessReview on every request.
4. **SSE streaming**: Real-time token streaming with reconnection support.
5. **Session model**: Shared session per RAR, multiple participants, identity tracked per turn.
6. **Rate limiting**: Per-user (10/min) and per-session (30 turns) limits.
7. **Override capability** (Phase 1b): Advisory workflow/parameter overrides with validation.
8. **Conversation lifecycle**: Session transitions to read-only on RAR approval/rejection.
9. **Shadow agent integration** (#601): Conversation-mode tool outputs audited.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `ginkgo ./test/unit/kubernautagent/conversation/...` |
| Integration test pass rate | 100% | `ginkgo ./test/integration/kubernautagent/...` |
| Unit-testable code coverage | >=80% | Session, audit reconstruction, auth, rate limit, override |
| Integration-testable code coverage | >=80% | Conversation flow, SSE, auth middleware |

---

## 2. References

### 2.1 Authority

- Issue #592: Enhancement proposal: Conversational mode for Kubernaut Agent — RAR-scoped investigation queries
- Issue #601: Prompt injection guardrails (conversation-mode tool outputs audited)
- Issue #285: NetworkPolicies (KA needs ingress for conversation endpoint)
- Issue #60, #593: PagerDuty/Teams (include kubectl command for conversation bridge)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Audit trail incomplete (ingestion lag) | Session created with partial context | Medium | UT-CS-592-003, IT-CS-592-002 | Retry with backoff (up to 10s); 404 if still empty |
| R2 | SSE connection dropped by corporate proxy | Operator loses streaming response | Medium | IT-CS-592-005 | Event IDs for reconnection; 60s response buffer; proxy-friendly headers |
| R3 | Token budget exceeded with large investigation traces | LLM context window overflow | Medium | UT-CS-592-005 | Summarize older turns; selective reconstruction |
| R4 | Unauthorized access to conversation | Security breach — operator sees other RR data | High | UT-CS-592-010, IT-CS-592-006 | TokenReview + SAR on every request |
| R5 | Session memory leak (no cleanup) | KA OOM over time | Low | UT-CS-592-008 | TTL-based session expiry |
| R6 | RR-scope bypass via prompt injection | LLM discusses other RRs | Medium | UT-CS-592-006 | System prompt scoping + tool call namespace restriction |

### 3.1 Risk-to-Test Traceability

- **R1** (audit lag): UT-CS-592-003, IT-CS-592-002
- **R4** (auth): UT-CS-592-010, IT-CS-592-006
- **R6** (scope bypass): UT-CS-592-006

---

## 4. Scope

### 4.1 Features to be Tested

**Phase 1a (MVP — read-only conversation)**:
- **Audit reconstruction** (`internal/kubernautagent/conversation/audit.go`): Fetch audit chain, reconstruct LLM history
- **Session management** (`internal/kubernautagent/conversation/session.go`): Create/get/delete, TTL, shared sessions
- **TLS + SSE endpoint** (`internal/kubernautagent/conversation/handler.go`): HTTPS, SSE streaming, reconnection
- **Auth middleware** (`internal/kubernautagent/conversation/auth.go`): TokenReview + SAR
- **RR-scoped guardrails** (`internal/kubernautagent/conversation/prompt.go`): System prompt scoping
- **Rate limiting** (`internal/kubernautagent/conversation/ratelimit.go`): Per-user and per-session
- **Conversation audit trail**: Emit audit events per turn

**Phase 1b (Override)**:
- **Override advisory** (`internal/kubernautagent/conversation/override.go`): Validate against catalog/schema
- **Lifecycle management**: Detect RAR state changes, transition to read-only

**Client interfaces (limited scope)**:
- **kubectl plugin** (`cmd/kubectl-kubernaut/`): SSE client, endpoint discovery, auth
- **Slack bot** (`internal/kubernautagent/slack/`): Socket Mode, thread mapping, OAuth link

### 4.2 Features Not to be Tested

- **Full Slack OAuth flow on OCP**: Requires real OCP OAuth server — E2E/manual test only
- **kubectl plugin distribution** (krew): Build/release pipeline concern
- **Session persistence in DataStorage** (v1.5): In-memory only for v1.4
- **Cross-channel bridge** (v1.5): No redirect from PD/Teams to Slack

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% — audit reconstruction, session management, auth, rate limiting, override validation, prompt scoping
- **Integration**: >=80% — end-to-end conversation flow, SSE streaming, auth middleware, lifecycle transitions
- **E2E**: Deferred — requires stable KA in Kind with MockLLM + DataStorage. Blocked on v1.3 CI/CD.

### 5.4 Pass/Fail Criteria

**PASS**: All P0 tests pass; per-tier >=80%; auth enforced on all paths; SSE reconnection works; rate limits enforced

**FAIL**: Any P0 fails; auth bypass possible; session leak on any path

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-CONV-001 | Audit chain reconstructed as LLM history | P0 | Unit | UT-CS-592-001 | Pending |
| BR-CONV-001 | Audit event type → LLM message mapping correct | P0 | Unit | UT-CS-592-002 | Pending |
| BR-CONV-001 | Retry with backoff on incomplete audit chain | P0 | Unit | UT-CS-592-003 | Pending |
| BR-CONV-001 | 404 when audit chain still empty after retries | P0 | Unit | UT-CS-592-004 | Pending |
| BR-CONV-001 | Token budget management (summarize old turns) | P1 | Unit | UT-CS-592-005 | Pending |
| BR-CONV-002 | RR-scoped system prompt refuses out-of-scope queries | P0 | Unit | UT-CS-592-006 | Pending |
| BR-CONV-002 | Tool calls restricted to target RR namespace | P0 | Unit | UT-CS-592-007 | Pending |
| BR-CONV-003 | Session create/get/delete lifecycle | P0 | Unit | UT-CS-592-008 | Pending |
| BR-CONV-003 | Shared session: multiple participants tracked | P0 | Unit | UT-CS-592-009 | Pending |
| BR-CONV-003 | Session TTL: inactive sessions expire | P1 | Unit | UT-CS-592-010-TTL | Pending |
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
| BR-CONV-001 | End-to-end: create session → send message → stream response | P0 | Integration | IT-CS-592-001 | Pending |
| BR-CONV-001 | End-to-end: audit chain → session → conversation | P0 | Integration | IT-CS-592-002 | Pending |
| BR-CONV-005 | SSE streaming end-to-end | P0 | Integration | IT-CS-592-003 | Pending |
| BR-CONV-004 | Auth middleware blocks unauthorized request | P0 | Integration | IT-CS-592-006 | Pending |
| BR-CONV-009 | Lifecycle: RAR state change → session transition | P0 | Integration | IT-CS-592-007 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests (22 tests)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-CS-592-001` | Audit chain (5 events) → reconstructed as 5 LLM messages with correct roles | Pending |
| `UT-CS-592-002` | Each audit event type maps to correct LLM message type (system/user/assistant/function) | Pending |
| `UT-CS-592-003` | Incomplete audit chain → retries with exponential backoff (up to 10s) | Pending |
| `UT-CS-592-004` | Empty audit chain after retries → 404 with descriptive message | Pending |
| `UT-CS-592-005` | Large audit chain exceeds token budget → older turns summarized | Pending |
| `UT-CS-592-006` | System prompt includes RR name/namespace scoping; refuses out-of-scope query | Pending |
| `UT-CS-592-007` | Tool call validator rejects calls outside target namespace | Pending |
| `UT-CS-592-008` | Session CRUD: create with RAR ref, get by ID, delete | Pending |
| `UT-CS-592-009` | Shared session: two user IDs in same session, both tracked in turns | Pending |
| `UT-CS-592-010` | TokenReview: valid token → authenticated identity extracted | Pending |
| `UT-CS-592-010-TTL` | Session TTL: session created, idle beyond TTL, get returns not found | Pending |
| `UT-CS-592-011` | SAR: user can UPDATE rar → authorized | Pending |
| `UT-CS-592-012` | SAR: user cannot UPDATE rar → 403 | Pending |
| `UT-CS-592-013` | SSE writer: events include incrementing ID field | Pending |
| `UT-CS-592-014` | SSE reconnection: Last-Event-ID → resumes from correct event | Pending |
| `UT-CS-592-015` | SSE buffer: response stored for 60s, available for reconnection | Pending |
| `UT-CS-592-016` | Per-user rate limit: 11th request within 1 min → 429 | Pending |
| `UT-CS-592-017` | Per-session rate limit: 31st turn → 429 | Pending |
| `UT-CS-592-018` | Each conversation turn emits audit event with identity + correlation_id | Pending |
| `UT-CS-592-019` | Override: workflow validated against catalog schema | Pending |
| `UT-CS-592-020` | Override: RAR approved → override request returns 409 | Pending |
| `UT-CS-592-021` | RAR approved → session.IsReadOnly() returns true | Pending |
| `UT-CS-592-022` | RAR expired → session.IsClosed() returns true | Pending |

### Tier 2: Integration Tests (5 tests)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-CS-592-001` | Create session → POST message → receive SSE stream with LLM tokens | Pending |
| `IT-CS-592-002` | Audit chain from DataStorage mock → session seeded → first message continues conversation | Pending |
| `IT-CS-592-003` | SSE stream delivers tokens incrementally (not all at once) | Pending |
| `IT-CS-592-006` | Missing/invalid bearer token → 401; valid token + no SAR → 403 | Pending |
| `IT-CS-592-007` | RAR CR status change (approved) → session transitions to read-only → subsequent message → 409 | Pending |

### Tier Skip Rationale

- **E2E**: Requires KA + DataStorage + MockLLM + Slack bot in Kind. Deferred to post-KA-stabilization.
- **Slack bot E2E on OCP**: Requires real OCP OAuth. Manual test only.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: Mock DataStorage client (for audit chain), Mock K8s client (for TokenReview/SAR), Mock LLM client
- **Location**: `test/unit/kubernautagent/conversation/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: `httptest` HTTPS server for SSE, real conversation handler, mock DataStorage + LLM
- **Location**: `test/integration/kubernautagent/`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact | Workaround |
|------------|------|--------|--------|------------|
| KA Go rewrite (#433) | Code | Merged (v1.3) | Server, LLM client, audit store | N/A |
| DataStorage audit events | Code | Available | Audit chain fetch | N/A |
| KA CI/CD stability | Testing | In progress | E2E blocked | Unit + Integration |
| #601 (shadow agent) | Code | Planned | Conversation tool output auditing | Can be added post-implementation |

### 11.2 Execution Order

1. **Phase 1a.1**: Audit reconstruction + session management (core)
2. **Phase 1a.2**: TLS + SSE + auth middleware
3. **Phase 1a.3**: Rate limiting + guardrails + audit trail
4. **Phase 1a.4**: kubectl plugin (client)
5. **Phase 1a.5**: Slack bot (client)
6. **Phase 1b.1**: Override capability
7. **Phase 1b.2**: Lifecycle management

---

## 12. Execution

```bash
ginkgo -v ./test/unit/kubernautagent/conversation/...
ginkgo -v ./test/integration/kubernautagent/...
ginkgo -v --focus="UT-CS-592" ./test/unit/kubernautagent/conversation/...
```

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
