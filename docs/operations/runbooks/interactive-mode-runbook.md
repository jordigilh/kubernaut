# Interactive Mode Operations Runbook

## Overview

This runbook covers operational procedures for the Kubernaut Agent's Interactive
Mode feature (Issue #703, CP-5).

## Metrics

### Key Metrics to Monitor

| Metric | Type | Description |
|--------|------|-------------|
| `aiagent_mcp_interactive_sessions_active` | Gauge | Currently active interactive sessions |
| `aiagent_mcp_interactive_command_duration_seconds` | Histogram | Tool execution latency |
| `aiagent_mcp_interactive_takeover_total` | Counter | Session start/takeover events |
| `aiagent_mcp_interactive_lease_contention_total` | Counter | Failed lease acquisitions |

### Alert Thresholds

- **Active sessions at max**: `aiagent_mcp_interactive_sessions_active >= maxConcurrentSessions`
  - Action: Check for leaked sessions (timeout not firing)
- **High lease contention**: `rate(aiagent_mcp_interactive_lease_contention_total[5m]) > 5`
  - Action: Multiple users competing for same remediations; verify timeout is releasing stale sessions
- **Gauge drift**: Active gauge stays elevated when no sessions expected
  - Action: Possible bug in session release path; restart agent pod

## Common Issues

### 1. Session Stuck (Not Releasing)

**Symptoms**: `aiagent_mcp_interactive_sessions_active` elevated, users report "session_active" errors.

**Diagnosis**:
```bash
kubectl get leases -n kubernaut-system -l app=kubernaut-agent
kubectl logs -n kubernaut-system deploy/kubernaut-agent | grep "interactive session"
```

**Resolution**:
1. Check if the inactivity timeout is configured (`inactivityTimeout` in ConfigMap)
2. Delete the stuck Lease manually:
   ```bash
   kubectl delete lease kubernaut-interactive-<rr_id> -n kubernaut-system
   ```
3. Restart the agent pod to clear in-memory session index

### 2. Impersonation Failures (403)

**Symptoms**: Users get 403 errors during interactive tool execution.

**Diagnosis**:
```bash
# Check the agent SA has impersonate permissions
kubectl auth can-i impersonate users --as=system:serviceaccount:kubernaut-system:kubernaut-agent-sa

# Check user's actual RBAC
kubectl auth can-i get pods -n <namespace> --as=<username>
```

**Resolution**:
- Verify `interactive.enabled=true` in Helm values (grants impersonate ClusterRole)
- The user simply lacks RBAC for the resource they're trying to access — this is
  expected security behavior, not a bug

### 3. Rate Limiting

**Symptoms**: Users receive `rate_limited` errors.

**Resolution**:
- Default: 10 messages/minute per session
- Adjust via `rateLimitPerUser` in Helm values (max: 100)
- Message size limit: 64KB per message

### 4. Disconnect Not Detected

**Symptoms**: Session stays active after client disconnects.

**Diagnosis**:
- Check `DelegatingEventStore` is configured (not nil)
- Check `SessionClosedHandler` goroutine is running:
  ```bash
  kubectl logs deploy/kubernaut-agent | grep "SessionClosedHandler"
  ```

**Resolution**:
- The MCP SDK fires `SessionClosed` when the HTTP connection drops
- If event store is nil (misconfiguration), disconnect detection is disabled
- Fallback: inactivity timeout will eventually release the session

### 5. Timeout Warnings Not Received

**Symptoms**: Sessions expire without prior warning to the client.

**Diagnosis**:
- Verify `SessionNotifier` is wired (check startup logs)
- Client must have called `SetLoggingLevel` with level <= `warning`

**Resolution**:
- MCP clients that don't set a logging level will not receive server-push log messages
- This is MCP SDK behavior; document for client developers

## Configuration Reference

```yaml
interactive:
  enabled: true                    # Feature gate
  sessionTTL: "30m"               # Max session duration
  inactivityTimeout: "10m"        # Auto-release after inactivity
  maxConcurrentSessions: 5        # Per-agent capacity
  rateLimitPerUser: 10            # Requests/second per user
  maxAnalyzingTimeout: "45m"      # Extended RO timeout during interactive sessions
```

## RBAC Requirements

Interactive mode requires these RBAC grants (auto-provisioned by Helm):

1. **Namespace-scoped Role** (`kubernaut-agent-interactive-leases`):
   - `coordination.k8s.io/leases`: get, create, update, delete

2. **ClusterRole** (added to `kubernaut-agent-investigator`):
   - `users`, `groups`, `serviceaccounts`: impersonate

## Scaling Considerations

- Sessions are **per-pod in-memory** (not distributed)
- In multi-replica deployments, sticky sessions (via MCP session ID) are required
- `maxConcurrentSessions` applies per replica
- Consider `replicas * maxConcurrentSessions` for total platform capacity

## Disaster Recovery

If the agent pod restarts:
- All in-memory sessions are lost
- Kubernetes Leases persist (orphaned)
- On restart, orphaned Leases are not cleaned automatically
- Users can `takeover` to reacquire — the Lease will be overwritten
- The `SessionNotifier` registry is rebuilt as users reconnect
