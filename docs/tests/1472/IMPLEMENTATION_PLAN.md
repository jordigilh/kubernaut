# Implementation Plan — #1472: Validate RR Existence Before Session Reactivation

**Issue**: [#1472](https://github.com/jordigilh/kubernaut/issues/1472) | **Test Plan**: [TP-1472](./TEST_PLAN.md)

## 1. Summary

Validate RemediationRequest (RR) existence and lifecycle phase in the `kubernaut_reconnect` tool before allowing session reactivation. If the referenced RR does not exist or has reached a terminal phase, the reconnection is rejected with a `session_expired` status — preventing misleading "reconnecting to your investigation" UX after pod restarts or investigation completion.

> **Note**: An earlier approach placed this validation in `SessionInterceptor` using ADK's `InMemoryService.Get()`. That approach was reverted because it could not distinguish genuinely new `context_id`s from stale ones (misclassifying all first-time IDs). The final implementation validates at the tool level using the Kubernetes API, which is authoritative for RR existence and phase.

## 2. Spike Outcomes (Informing Design)

| Spike | Outcome | Design Impact |
|---|---|---|
| S-1: ADK InMemoryService.Get() | Cannot distinguish new vs. stale context_ids — replaces *all* first-time IDs, breaking tests | **Reverted** — interceptor-based approach abandoned |
| S-2: RR CRD lookup via typed client | `crclient.Client.Get()` reliably checks RR existence and phase | Tool-level validation using K8s typed client |

## 3. Architecture

### 3.1 Validation Point

Validation occurs inside `HandleReconnect` in `pkg/apifrontend/tools/ka_interactive.go`. This is the tool invoked when an operator attempts to reconnect to an existing investigation.

### 3.2 Validation Logic

```go
func HandleReconnect(ctx context.Context, log logr.Logger, registry ActiveContextRegistry,
    k8sClient crclient.Client, namespace string, args ReconnectArgs) InteractiveActionResult {

    // Defensive: skip validation if K8s client or namespace unavailable (fail-open, SC-5)
    if k8sClient != nil && namespace != "" && args.RRID != "" {
        rr := &remediationv1.RemediationRequest{}
        key := crclient.ObjectKey{Namespace: namespace, Name: args.RRID}
        if err := k8sClient.Get(ctx, key, rr); err != nil {
            log.Info("RR not found for reconnect, rejecting", "rrid", args.RRID)
            return InteractiveActionResult{Status: "session_expired", Message: "..."}
        }
        if IsTerminalPhase(string(rr.Status.OverallPhase)) {
            log.Info("RR in terminal phase, rejecting reconnect", "rrid", args.RRID, "phase", rr.Status.OverallPhase)
            return InteractiveActionResult{Status: "session_expired", Message: "..."}
        }
    }
    // ... existing reconnection logic
}
```

### 3.3 Fail-Open Safety

- If `k8sClient == nil`: validation skipped, reconnection proceeds (defensive, enables testing without K8s)
- If `namespace == ""`: validation skipped (same rationale)
- If `args.RRID == ""`: validation skipped (reconnect by other means)
- Rationale: availability over correctness (SC-5 DoS protection)

## 4. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| `HandleReconnect` RR validation | `NewReconnectTool()` callback | `pkg/apifrontend/tools/ka_interactive.go` | UT-AF-1472-001..006 |
| K8s client injection | `root.go` agent setup | `pkg/apifrontend/agent/root.go` | — |
| MCP bridge wiring | `registerTool("kubernaut_reconnect")` | `pkg/apifrontend/handler/mcp_bridge.go` | IT-AF-1234-W06 |

## 5. Files Modified

| File | Change |
|---|---|
| `pkg/apifrontend/tools/ka_interactive.go` | Add `k8sClient`, `namespace` params to `HandleReconnect` and `NewReconnectTool`; add RR validation logic |
| `pkg/apifrontend/tools/ka_interactive_test.go` | Add UT-AF-1472-001 through UT-AF-1472-006 |
| `pkg/apifrontend/agent/root.go` | Pass `cfg.TypedClient` and `cfg.Namespace` to `NewReconnectTool` |
| `pkg/apifrontend/handler/mcp_bridge.go` | Pass `cfg.TypedClient` and `cfg.Namespace` to `HandleReconnect` in tool registration |
| `test/e2e/apifrontend/stale_session_e2e_test.go` | E2E-AF-1472-001 proving full journey |

## 6. TDD Phases

### Phase 1: RED

1. Add UT-AF-1472-001..006 in `ka_interactive_test.go` — tests reference new signature params, compilation fails
2. E2E test asserting fresh conversation after pod restart

### Phase 2: GREEN

1. Modify `HandleReconnect` signature to accept `crclient.Client` and `namespace`
2. Implement RR existence check (`k8sClient.Get()`)
3. Implement terminal phase check (`IsTerminalPhase()`)
4. Update `NewReconnectTool` to pass through new params
5. Update call sites in `root.go` and `mcp_bridge.go`
6. **CHECKPOINT W**: All wiring manifest rows have production callers + passing tests

### Phase 3: REFACTOR

1. Add defensive nil/empty guards for fail-open
2. Structured logging with `rrid`, `phase` fields
3. `golangci-lint` compliance

## 7. Backward Compatibility

- `HandleReconnect` and `NewReconnectTool` signatures changed (add `k8sClient`, `namespace` params)
- Existing tests pass `nil, ""` for backward compatibility
- If `k8sClient` is nil, validation is entirely skipped (no behavioral change for tests without K8s)

## 8. Acceptance Criteria (from Issue #1472)

- [x] After AF pod restart, sending a reconnect to a non-existent RR returns `session_expired`
- [x] Active sessions with valid RRs continue to route correctly
- [x] Terminal-phase RRs (Completed, Failed) are rejected for reconnection
- [x] Fail-open on nil client or empty namespace (availability preserved)
- [x] No interceptor changes — `SessionInterceptor` remains as-is
