# Test Plan: InvestigationSession Production Wiring (SessionServiceDecorator)

**Test Plan Identifier:** TP-AF-056-PW
**Issue:** #56
**Version:** 2.0
**Date:** 2026-05-08
**Status:** Draft

---

## 1. Introduction

This test plan validates the production wiring of `InvestigationSession` CRDs via a `SessionServiceDecorator` that enriches ADK's session creation flow with CRD-specific context. The decorator bridges the gap between the A2A executor (which calls `SessionService.Create` with an empty `State` map) and the `CRDSessionService` (which requires `CreateConfig` in `State` to populate the CRD).

### 1.1 Scope

- `SessionServiceDecorator`: transparent wrapper around `adksession.Service` that enriches `CreateRequest.State`
- Context propagation: `BeforeExecuteCallback` stores task/user metadata in `context.Context`
- Integration with launcher: decorator injected between executor and `CRDSessionService`
- `af_sessions_active` gauge wiring in production path
- Identity propagation: decorator extracts username from auth context

### 1.2 Out of Scope

- `CRDSessionService` CRUD operations (covered by TP-AF-056 v1.0)
- State machine phase transitions (covered by TP-AF-056 v1.0)
- TTL controller behavior (covered by TP-AF-056 v1.0)
- SSE formatting (covered by TP-AF-056 v1.0)

### 1.3 References

- TP-AF-056 v1.0: CRD-backed Session Service test plan
- ADR-005: Session persistence via InvestigationSession CRD
- ARCHITECTURE.md Section 4: CRD Design (InvestigationSession)
- ARCHITECTURE.md Section 7: Observability (`af_sessions_active`)
- `google.golang.org/adk@v1.2.0/session.Service` interface
- FedRAMP Controls: AC-3 (access enforcement), AU-2/AU-12 (audit)

---

## 2. Test Items

| Item | Package | Source |
|------|---------|--------|
| `SessionServiceDecorator` | `internal/session` | New (this PR) |
| `SessionCreateContext` | `internal/session` | New (this PR) |
| `WithSessionCreateContext` / `SessionCreateContextFromContext` | `internal/session` | New (this PR) |
| `buildBeforeExecuteCallback` (enriched) | `internal/launcher` | Modified |
| `main.go` CRDSessionService wiring | `cmd/apifrontend` | Modified |

---

## 3. Business Acceptance Criteria

| ID | Criterion | Source | Priority |
|----|-----------|--------|----------|
| BAC-56-PW-01 | A2A `message/send` creates `InvestigationSession` CRD with populated spec | ARCHITECTURE.md §4 | P0 |
| BAC-56-PW-02 | CRD has labels including `kubernaut.ai/user` and `kubernaut.ai/rr-name` | ARCHITECTURE.md §4.2 | P0 |
| BAC-56-PW-03 | `a2aTaskID` in CRD spec matches A2A task ID from request context | ARCHITECTURE.md §4.1 | P0 |
| BAC-56-PW-04 | Reconnection via `List` by user label returns active sessions | Issue #56 AC | P0 |
| BAC-56-PW-05 | Session creation without RR ref succeeds (exploratory triage) | Issue #56 AC | P1 |
| BAC-56-PW-06 | Decorator is transparent — ADK runner behavior unchanged for Get/List/Delete/AppendEvent | ADK compat | P0 |
| BAC-56-PW-07 | Invalid CRD name (non-RFC-1123 TaskID) is sanitized before CRD creation | K8s spec | P1 |
| BAC-56-PW-08 | Empty/nil identity in context returns descriptive error, not silent failure | FedRAMP AC-3 | P0 |

---

## 4. Test Cases

### 4.1 SessionServiceDecorator (8 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-056-PW-001 | Create enriches State with CreateConfig from context | BAC-56-PW-01 | P0 |
| UT-AF-056-PW-002 | Create without context config passes through unchanged (transparent) | BAC-56-PW-06 | P0 |
| UT-AF-056-PW-003 | Get/List/Delete/AppendEvent delegate to wrapped service unchanged | BAC-56-PW-06 | P0 |
| UT-AF-056-PW-004 | TaskID extracted from context into CreateConfig.A2ATaskID | BAC-56-PW-03 | P0 |
| UT-AF-056-PW-005 | UserIdentity populated from auth.UserIdentityFromContext | BAC-56-PW-02 | P0 |
| UT-AF-056-PW-006 | Concurrent Create calls are safe (10 goroutines, -race) | Concurrency | P0 |
| UT-AF-056-PW-007 | Invalid CRD name (non-RFC-1123 TaskID) is sanitized | BAC-56-PW-07 | P1 |
| UT-AF-056-PW-008 | Empty username / nil identity yields error (not silent acceptance) | BAC-56-PW-08 | P0 |

### 4.2 Context Propagation (4 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-056-PW-009 | WithSessionCreateContext stores and retrieves context value | BAC-56-PW-01 | P0 |
| UT-AF-056-PW-010 | SessionCreateContextFromContext returns nil when not set | BAC-56-PW-06 | P0 |
| UT-AF-056-PW-011 | BeforeExecuteCallback injects TaskID from RequestContext | BAC-56-PW-03 | P0 |
| UT-AF-056-PW-012 | BeforeExecuteCallback injects UserIdentity from auth context | BAC-56-PW-02 | P0 |

### 4.3 Integration (4 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-056-PW-013 | Full flow: A2A message/send -> decorator -> CRDSessionService -> CRD created | BAC-56-PW-01 | P0 |
| UT-AF-056-PW-014 | Decorator + CRDSessionService with WithSessionsActive gauge updates metric | Observability | P0 |
| UT-AF-056-PW-015 | Session creation without RR ref produces CRD with empty remediationRef | BAC-56-PW-05 | P1 |
| UT-AF-056-PW-016 | Reconnection flow: create -> disconnect -> list by label -> reconnect | BAC-56-PW-04 | P0 |

---

## 5. Pass/Fail Criteria

- All 16 tests pass with `-race` flag
- Coverage >= 80% for `internal/session/decorator.go`
- No exported test helpers from production packages
- `golangci-lint run` reports 0 errors on modified files
- Compile-time interface assertion: `var _ adksession.Service = (*SessionServiceDecorator)(nil)`

---

## 6. Test Environment

- Go 1.25.6
- `sigs.k8s.io/controller-runtime/pkg/client/fake` for K8s API simulation
- `google.golang.org/adk/session.InMemoryService()` as inner delegate
- Ginkgo v2 + Gomega test framework (ADR-015)
- `-race` flag mandatory for concurrency tests

---

## 7. Design Notes

### 7.1 Decorator Pattern

```
A2A Executor
    |
    v
SessionServiceDecorator  <-- enriches Create with context data
    |
    v
CRDSessionService        <-- persists to K8s CRD + delegates to InMemoryService
    |
    v
adksession.InMemoryService()
```

### 7.2 Context Flow

```
HTTP Request (with JWT)
    -> auth.Middleware (injects UserIdentity into context)
    -> A2A JSON-RPC handler
    -> BeforeExecuteCallback (injects SessionCreateContext with TaskID + UserIdentity)
    -> ADK Runner calls SessionService.Create(ctx, req)
    -> Decorator reads SessionCreateContext from ctx
    -> Decorator builds CreateConfig and injects into req.State
    -> CRDSessionService.Create reads CreateConfig from req.State
    -> CRD created with populated spec/labels
```
