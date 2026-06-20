# Implementation Plan — #1472: Validate RR Existence Before Session Reactivation

**Issue**: [#1472](https://github.com/jordigilh/kubernaut/issues/1472) | **Test Plan**: [TP-1472](./TEST_PLAN.md)

## 1. Summary

Add a `StaleSessionValidator` interface to the `SessionInterceptor` that checks whether an explicit `context_id` has a backing in-memory session. If not (post-restart staleness), clear the context_id so ADK creates a fresh session instead of silently reactivating a ghost investigation.

## 2. Spike Outcomes (Informing Design)

| Spike | Outcome | Design Impact |
|---|---|---|
| S-1: ADK InMemoryService.Get() | Returns `fmt.Errorf("session %+v not found", ...)` for missing sessions | Use error string detection or simply treat any `Get()` error as "not found" for the validator |
| S-2: IS CRD lookup strategy | No field index for `a2aTaskID`; session hydration disabled (#1451); any context_id absent from memory is stale by definition | **No K8s API call needed** — validation is purely in-memory |

## 3. Architecture

### 3.1 Interface

```go
// StaleSessionValidator determines whether a context_id references a
// valid in-memory session. Used by SessionInterceptor to detect stale
// contexts after pod restarts (issue #1472).
type StaleSessionValidator interface {
    // IsContextValid returns true if the context_id has a backing session
    // in memory OR if an error prevents determination (fail-open).
    IsContextValid(ctx context.Context, contextID, username string) bool
}
```

### 3.2 Implementation

```go
// inMemorySessionValidator implements StaleSessionValidator by probing
// the ADK session service's in-memory store.
type inMemorySessionValidator struct {
    sessionService adksession.Service
    appName        string
    logger         logr.Logger
}
```

The `IsContextValid` method calls `sessionService.Get()` with `appName`, `username`, and `contextID` as the session ID. If the session is found, returns `true`. If "not found" error, returns `false`. For any other error, returns `true` (fail-open, SC-5).

### 3.3 Interceptor Change

Current `Before()` at line 55-57:
```go
if msg.ContextID != "" {
    return ctx, nil
}
```

Changed to:
```go
if msg.ContextID != "" {
    if s.validator != nil && !s.validator.IsContextValid(ctx, msg.ContextID, identity.Username) {
        s.logger.Info("clearing stale context_id — no backing session in memory (post-restart)",
            "user", identity.Username,
            "stale_context_id", msg.ContextID,
        )
        msg.ContextID = ""
        // Fall through to registry check below
    } else {
        return ctx, nil
    }
}
```

### 3.4 Fail-Open Safety

- If `validator` is `nil` (defensive): existing pass-through behavior preserved.
- If `sessionService.Get()` returns a non-"not-found" error: treated as valid (fail-open).
- Rationale: availability over correctness (SC-5 DoS protection).

## 4. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| `StaleSessionValidator` interface | `SessionInterceptor.Before()` | `pkg/apifrontend/launcher/session_interceptor.go` | IT-AF-1472-001 |
| `inMemorySessionValidator` impl | `NewSessionInterceptor()` constructor | `pkg/apifrontend/launcher/session_interceptor.go` | IT-AF-1472-001 |
| Production wiring | `cmd/apifrontend/main.go:992` | `launcher.NewSessionInterceptor(...)` call | IT-AF-1472-001 |

## 5. Files Modified

| File | Change |
|---|---|
| `pkg/apifrontend/launcher/session_interceptor.go` | Add `StaleSessionValidator` interface, `inMemorySessionValidator` struct, modify `Before()`, update `NewSessionInterceptor()` |
| `pkg/apifrontend/launcher/session_interceptor_test.go` | Add UT-AF-1472-001 through UT-AF-1472-006 |
| `pkg/apifrontend/launcher/session_interceptor_it_test.go` | Add IT-AF-1472-001, IT-AF-1472-002 |
| `cmd/apifrontend/main.go` | Pass `sessionService` and `appName` to `NewSessionInterceptor()` |

## 6. TDD Phases

### Phase 1: RED

1. Add test file entries for UT-AF-1472-001..006 in `session_interceptor_test.go`
2. Add test file entries for IT-AF-1472-001, IT-AF-1472-002 in `session_interceptor_it_test.go`
3. Tests reference `StaleSessionValidator` interface and `NewSessionInterceptor` with new params — compilation fails (RED)

### Phase 2: GREEN

1. Define `StaleSessionValidator` interface in `session_interceptor.go`
2. Implement `inMemorySessionValidator`
3. Modify `SessionInterceptor` struct to hold optional validator
4. Update `NewSessionInterceptor()` to accept validator (with option pattern or direct param)
5. Implement the validation gate in `Before()`
6. Update `cmd/apifrontend/main.go` to pass the validator
7. **CHECKPOINT W**: Verify all wiring manifest rows have production callers + passing ITs

### Phase 3: REFACTOR

1. Improve error messages with structured fields
2. Validate edge cases (empty username, nil identity already handled upstream)
3. Consider extracting validator to its own file if it grows
4. Run `golangci-lint` for style compliance

## 7. Backward Compatibility

- `NewSessionInterceptor()` signature changes (adds validator param) — callers in `cmd/` and tests must update
- If `validator` is `nil`, behavior is identical to pre-fix (no breakage during migration)
- All existing tests continue to pass with `nil` validator (or updated to pass a mock)

## 8. Acceptance Criteria (from Issue #1472)

- [ ] After AF pod restart, sending a message with a stale `context_id` starts a fresh conversation (no "reconnecting" message)
- [ ] Active sessions within the same pod lifetime continue to route correctly
- [ ] Stale context clearing is logged for SRE observability
- [ ] No K8s API calls added to the interceptor hot path
- [ ] Fail-open on unexpected errors (availability preserved)
