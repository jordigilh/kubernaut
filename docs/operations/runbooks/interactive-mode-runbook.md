# Interactive Mode Operations Runbook

**Authority**: Issue #1004, v1.5 Readiness Audit OPS-1
**Version**: 2.0 (v1.5 GA)

## Overview

This runbook covers operational procedures for the Kubernaut Agent's MCP Interactive
Mode (Issue #703). It provides diagnosis and resolution steps for the most common
production scenarios.

---

## 1. Session Stuck in Analyzing

**Symptoms**:
- The `RemediationRequest` remains in `Analyzing` phase longer than expected
- `aiagent_mcp_interactive_sessions_active` stays elevated
- The AA reconciler reports the session as active but no user messages are flowing

**Diagnosis**:
```bash
# Check the RR status
kubectl get remediationrequests -n kubernaut-system -o wide | grep Analyzing

# Check the interactive session Lease
kubectl get leases -n kubernaut-system -l app=kubernaut-agent

# Inspect agent logs for the session
kubectl logs -n kubernaut-system deploy/kubernaut-agent | grep "session_id=<id>"

# Check if maxAnalyzingTimeout is configured
kubectl get configmap kubernaut-agent-config -n kubernaut-system -o yaml | grep maxAnalyzing
```

**Resolution**:
1. If the user simply disconnected, wait for `inactivityTimeout` (default 10m) to auto-release
2. If the session is genuinely stuck (LLM not responding):
   ```bash
   # Delete the Lease to release the lock
   kubectl delete lease kubernaut-interactive-<rr_id> -n kubernaut-system
   # Restart the agent pod to clear in-memory state
   kubectl rollout restart deploy/kubernaut-agent -n kubernaut-system
   ```
3. The RO will time out the RR after `maxAnalyzingTimeout` (default 45m) and mark it failed

**Prevention**:
- Set `interactive.inactivityTimeout` to a reasonable value (5-10m)
- Monitor `aiagent_mcp_interactive_command_duration_seconds` p99 for LLM slowness
- Set up the `KubernautInteractiveCommandLatencyHigh` PrometheusRule alert

---

## 2. Lease Contention

**Symptoms**:
- Users receive `ErrCodeLeaseHeld` / "lease already held" errors
- `aiagent_mcp_interactive_lease_contention_total` counter increases
- The `KubernautLeaseContentionSpike` alert fires

**Diagnosis**:
```bash
# List active Leases and their holders
kubectl get leases -n kubernaut-system -l app=kubernaut-agent \
  -o custom-columns=NAME:.metadata.name,HOLDER:.spec.holderIdentity,RENEW:.spec.renewTime

# Check who holds the Lease for a specific RR
kubectl get lease kubernaut-interactive-<rr_id> -n kubernaut-system -o yaml

# Check contention rate
# PromQL: rate(aiagent_mcp_interactive_lease_contention_total[5m])
```

**Resolution**:
1. Normal behavior: only one operator can hold a session per RR at a time
2. If the holder has disconnected but the Lease persists (stale):
   ```bash
   kubectl delete lease kubernaut-interactive-<rr_id> -n kubernaut-system
   ```
3. The new operator can then acquire the session

**Prevention**:
- Ensure `inactivityTimeout` is set appropriately (default 10m)
- Communicate to operators that sessions are exclusive per RR
- If contention is consistently high, consider whether automated workflows are more appropriate

---

## 3. Takeover Failures

**Symptoms**:
- Users invoke `kubernaut_investigate` with `action: takeover` but get errors
- Logs show `transition autonomous session to user-driving` failures

**Diagnosis**:
```bash
# Check if the autonomous session exists for the RR
kubectl logs -n kubernaut-system deploy/kubernaut-agent | grep "FindByRemediationID"

# Check session store state
kubectl logs -n kubernaut-system deploy/kubernaut-agent | grep "TransitionToUserDriving"

# Verify the RR is in a takeover-eligible state (not terminal)
kubectl get remediationrequests <rr_name> -n kubernaut-system -o yaml | grep phase
```

**Resolution**:
1. **ErrSessionTerminal**: The autonomous session already completed before takeover. This is
   expected in race conditions — the interactive session proceeds normally (Lease is already acquired)
2. **No autonomous session found**: The RR may not have an active investigation. The user should
   use `action: start` instead of `action: takeover`
3. **Lease acquisition failure during takeover**: Another interactive user beat this one. Wait and retry.

**Prevention**:
- Educate operators: takeover is for transferring control from autonomous to human-driven
- Monitor `aiagent_mcp_interactive_takeover_total` by outcome label

---

## 4. Rate Limiting (429)

**Symptoms**:
- Users receive `ErrCodeRateLimited` errors with HTTP 429 status
- `aiagent_http_rate_limited_total` counter increments
- Users report sluggish interactive experience

**Diagnosis**:
```bash
# Check configured rate limit
kubectl get configmap kubernaut-agent-config -n kubernaut-system -o yaml | grep rateLimitPerUser

# Check which users are hitting limits
kubectl logs -n kubernaut-system deploy/kubernaut-agent | grep "rate_limited"

# PromQL: rate(aiagent_http_rate_limited_total[5m])
```

**Resolution**:
1. If legitimate high-frequency usage: increase `interactive.rateLimitPerUser` (default 10 req/s)
2. If abuse: verify the user identity and investigate (possible automated client misconfiguration)
3. The rate limiter is per-authenticated-user, not per-session

**Prevention**:
- Set `rateLimitPerUser` based on expected human interaction cadence (10 req/s is generous)
- If MCP clients batch requests, they should implement client-side throttling
- Monitor `aiagent_http_rate_limited_total` in dashboards

---

## 5. Session TTL / Inactivity Expiry

**Symptoms**:
- Users report their session "disappeared" mid-investigation
- Logs show `session expired: TTL exceeded` or `session expired: inactivity timeout`
- Users receive `ErrCodeSessionExpired` on their next request

**Diagnosis**:
```bash
# Check session duration vs TTL
kubectl logs -n kubernaut-system deploy/kubernaut-agent | grep "session expired"

# Check configured timeouts
kubectl get configmap kubernaut-agent-config -n kubernaut-system -o yaml | grep -E "sessionTTL|inactivityTimeout"
```

**Resolution**:
1. **TTL expired**: The session exceeded `sessionTTL` (default 30m). User must start a new session.
2. **Inactivity expired**: No messages were received within `inactivityTimeout` (default 10m).
   User must start a new session.
3. If long investigations are common: increase `sessionTTL` to 60m or more

**Prevention**:
- MCP clients should inform users of time remaining (server sends warning notifications
  at 80% TTL via MCP logging level `warning`)
- Clients must implement `SetLoggingLevel` to receive timeout warnings
- Set TTL based on actual investigation patterns (track p99 session duration)

---

## 6. Memory Growth (Session Map)

**Symptoms**:
- Agent pod memory usage grows over time
- OOMKill events on the kubernaut-agent pod
- High number of historical sessions in memory

**Diagnosis**:
```bash
# Check agent memory usage
kubectl top pod -n kubernaut-system -l app=kubernaut-agent

# Expose pprof (if enabled)
kubectl port-forward -n kubernaut-system deploy/kubernaut-agent 6060:6060

# Analyze heap profile
go tool pprof http://localhost:6060/debug/pprof/heap
# Look for: session.Store, mcpToSession map entries

# Check session cleanup activity
kubectl logs -n kubernaut-system deploy/kubernaut-agent | grep "Cleanup"
```

**Resolution**:
1. The session store `Cleanup()` runs on a periodic ticker (every 5 minutes). It removes sessions
   whose `CreatedAt` exceeds the configured TTL, excluding `StatusRunning` and `StatusUserDriving`.
2. If cleanup is not removing enough sessions:
   - Reduce `sessionTTL` to limit accumulation
   - Increase agent pod memory limits
3. If memory growth is rapid, check for a Cleanup goroutine failure:
   ```bash
   kubectl logs deploy/kubernaut-agent | grep -c "session cleanup"
   ```

**Prevention**:
- Set memory requests/limits appropriately (256Mi minimum, 512Mi recommended for 5+ concurrent sessions)
- Monitor container memory usage with: `container_memory_working_set_bytes{container="kubernaut-agent"}`
- Set up OOMKill alerting: `kube_pod_container_status_last_terminated_reason{reason="OOMKilled"}`

---

## 7. Prometheus Dashboard Queries

### Active Sessions (Gauge)
```promql
aiagent_mcp_interactive_sessions_active
```

### Session Acquisition Rate (per minute)
```promql
rate(aiagent_mcp_interactive_takeover_total{outcome=~".*_success"}[5m]) * 60
```

### Lease Contention Rate
```promql
rate(aiagent_mcp_interactive_lease_contention_total[5m])
```

### Command Duration P99 by Tool
```promql
histogram_quantile(0.99,
  sum(rate(aiagent_mcp_interactive_command_duration_seconds_bucket[5m])) by (le, tool)
)
```

### Command Duration P50 by Tool
```promql
histogram_quantile(0.50,
  sum(rate(aiagent_mcp_interactive_command_duration_seconds_bucket[5m])) by (le, tool)
)
```

### Takeover Success Rate
```promql
(
  rate(aiagent_mcp_interactive_takeover_total{outcome="start_success"}[5m])
  +
  rate(aiagent_mcp_interactive_takeover_total{outcome="takeover_success"}[5m])
)
/
rate(aiagent_mcp_interactive_takeover_total[5m])
```

### Auth Denials (interactive traffic)
```promql
rate(aiagent_authz_denied_total[5m])
```

### HTTP 429 Rate (rate limiting)
```promql
rate(aiagent_http_rate_limited_total[5m])
```

### Agent Memory (for session map growth)
```promql
container_memory_working_set_bytes{container="kubernaut-agent", namespace="kubernaut-system"}
```

---

## 8. Common Error Codes

| Error Code | HTTP Status | Description | Resolution |
|-----------|-------------|-------------|------------|
| `ErrCodeRRNotFound` | 404 | The RemediationRequest ID does not exist | Verify the RR name/ID; it may have been cleaned up by retention policy |
| `ErrCodeRateLimited` | 429 | Per-user rate limit exceeded | Wait and retry; increase `rateLimitPerUser` if legitimate |
| `ErrCodeSessionExpired` | 410 | Session timed out (TTL or inactivity) | Start a new session; increase timeouts if needed |
| `ErrCodeLeaseHeld` | 409 | Another user holds the interactive session for this RR | Wait for the other session to complete, or ask the holder to release |
| `ErrCodeSessionNotFound` | 404 | No active session for the provided session ID | Session may have expired or pod restarted; start a new session |
| `ErrCodeUnauthorized` | 401 | Authentication failed (invalid token, expired JWT) | Re-authenticate; check JWT provider configuration |
| `ErrCodeForbidden` | 403 | User lacks RBAC for the requested K8s operation | Expected security behavior; user needs appropriate RBAC |
| `ErrCodeMaxSessions` | 503 | Agent has reached `maxConcurrentSessions` capacity | Wait for a session to complete; scale replicas if persistent |

---

## Configuration Reference

```yaml
kubernautAgent:
  interactive:
    enabled: true                    # Feature gate
    sessionTTL: "30m"               # Max session duration
    inactivityTimeout: "10m"        # Auto-release after inactivity
    maxConcurrentSessions: 5        # Per-replica capacity
    rateLimitPerUser: 10            # Requests/second per user
    maxAnalyzingTimeout: "45m"      # Extended RO timeout during interactive sessions
    jwtProviders:                   # Pattern B authentication (DD-AUTH-MCP-001 v2.0)
      - name: "dex"
        issuer: "https://dex.example.com"
        jwksURL: "https://dex.example.com/keys"
        audience: "kubernaut-agent"
        claimMappings:
          username: "preferred_username"
          groups: "groups"
```

## RBAC Requirements

Interactive mode requires these RBAC grants (auto-provisioned by Helm):

1. **Namespace-scoped Role** (`kubernaut-agent-interactive-leases`):
   - `coordination.k8s.io/leases`: get, create, update, delete

2. **ClusterRole** (added to `kubernaut-agent-investigator`):
   - `users`, `groups`, `serviceaccounts`: impersonate

3. **Startup SAR self-check** (#891):
   - Agent validates impersonate permission at startup
   - If denied: interactive mode is soft-disabled, `/readyz` reflects status, K8s Event emitted

## Scaling Considerations

- Sessions are **per-pod in-memory** (not distributed)
- In multi-replica deployments, sticky sessions (via MCP session ID) are required
- `maxConcurrentSessions` applies per replica
- Platform capacity = `replicas * maxConcurrentSessions`
- Consider HPA based on `aiagent_mcp_interactive_sessions_active` if session demand is elastic

## Disaster Recovery

If the agent pod restarts:
1. All in-memory sessions are lost
2. Kubernetes Leases persist (orphaned until TTL or manual cleanup)
3. On restart, orphaned Leases are not cleaned automatically
4. Users can `takeover` to reacquire — the Lease will be overwritten
5. The `SessionNotifier` registry is rebuilt as users reconnect
6. `StatusUserDriving` sessions (takeover in progress) are preserved in the session store
   but will be cleaned up by TTL eventually

## Health Endpoint

The `/readyz` endpoint includes interactive mode status:

```json
{
  "status": "ready",
  "interactive_mode": "enabled",
  "interactive_reason": ""
}
```

Possible `interactive_mode` values:
- `enabled` — interactive mode is active
- `soft_disabled` — SAR check failed; interactive mode unavailable
- `disabled` — `interactive.enabled=false` in config

When `soft_disabled`, `interactive_reason` provides the failure cause (e.g., "SA lacks impersonate permission").
