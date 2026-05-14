# Upgrade Guide: Kubernaut v1.4 to v1.5

## Overview

Kubernaut v1.5 introduces MCP Interactive Mode, session cancellation with SSE streaming,
a configuration restructure, and a logging framework migration. This guide covers
breaking changes and required actions for a smooth upgrade.

---

## Breaking Changes

### 1. Configuration YAML — camelCase Tags

All Kubernaut Agent configuration fields have been migrated to camelCase YAML tags.
If you have a custom `kubernaut-agent.yaml`, update field names accordingly.

**Before (v1.4):**
```yaml
runtime:
  server:
    health_addr: ":8081"
    metrics_addr: ":9090"
```

**After (v1.5):**
```yaml
runtime:
  server:
    healthAddr: ":8081"
    metricsAddr: ":9090"
```

**Action:** Review your `kubernaut-agent.yaml` and update any snake_case fields
to camelCase. The Helm chart handles this automatically if you use `values.yaml`.

### 2. Interactive Mode Configuration

A new top-level `interactive` block is available in the Kubernaut Agent config.
This block is only relevant if you enable interactive mode (`kubernautAgent.interactive.enabled=true`).

```yaml
interactive:
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 10
  rateLimitPerUser: 5
  maxAnalyzingTimeout: 10m
```

**Action:** No action required if you do not enable interactive mode. If enabling,
review and tune the defaults in `values.yaml`.

### 3. Logging Framework — slog to logr (#885)

The Kubernaut Agent has migrated from `log/slog` to `logr.Logger` for consistency
with controller-runtime services. This change is internal and does not affect
user-facing configuration. Log format and verbosity levels remain the same.

**Action:** No action required. If you have custom log parsing pipelines that
depend on slog-specific formatting, verify they work with the new output.

---

## New Features Requiring Configuration

### MCP Interactive Mode (#703)

Enable interactive investigation sessions via Helm:

```yaml
kubernautAgent:
  interactive:
    enabled: true
```

This provisions:
- coordination/v1 Lease Role/RoleBinding (namespace-scoped)
- Impersonate ClusterRole for user-identity K8s API calls
- Interactive ConfigMap with session parameters

### Runtime Profiling (pprof)

v1.5 adds `/debug/pprof/*` endpoints to the health server (port 8081) for
DataStorage, Kubernaut Agent, and Gateway. Profiling is **enabled by default**
(following the `kube-apiserver --profiling` pattern) and has zero overhead
when not actively queried.

To disable profiling in hardened environments, set in the service config YAML:

```yaml
server:
  disableProfiling: true
```

Or via Helm values (when supported):
```yaml
kubernautAgent:
  server:
    disableProfiling: true
```

---

## Helm Chart Changes

### New Values

| Value | Default | Description |
|-------|---------|-------------|
| `kubernautAgent.interactive.enabled` | `false` | Enable MCP interactive mode |
| `kubernautAgent.interactive.sessionTTL` | `30m` | Maximum session lifetime |
| `kubernautAgent.interactive.inactivityTimeout` | `5m` | Idle session timeout |
| `kubernautAgent.interactive.maxConcurrentSessions` | `10` | Max concurrent interactive sessions |
| `kubernautAgent.interactive.rateLimitPerUser` | `5` | Requests/sec per authenticated user |

### New RBAC Resources (when interactive.enabled=true)

- `Role`: coordination/v1 Leases (get, create, update, delete)
- `RoleBinding`: Binds Lease Role to KA ServiceAccount
- `ClusterRole`: Impersonate users/groups/serviceaccounts

---

## CRD Schema Changes

All CRD changes in v1.5 are **additive** (new fields in status). No existing
fields have been removed or renamed. Existing RemediationRequest, AIAnalysis,
and WorkflowExecution resources are forward-compatible.

---

## Upgrade Procedure

1. **Back up** your existing Helm release values:
   ```bash
   helm get values kubernaut -n kubernaut-system > values-backup.yaml
   ```

2. **Update CRDs** (CRDs are not managed by Helm upgrade):
   ```bash
   kubectl apply -f charts/kubernaut/crds/
   ```

3. **Review values changes** — diff your `values-backup.yaml` against the new
   `values.yaml` for any removed or renamed fields.

4. **Upgrade the Helm release**:
   ```bash
   helm upgrade kubernaut charts/kubernaut/ \
     -n kubernaut-system \
     -f values-backup.yaml \
     --set global.image.tag=v1.5.0
   ```

5. **Verify** all pods are running:
   ```bash
   kubectl get pods -n kubernaut-system
   ```

6. **Validate health endpoints**:
   ```bash
   kubectl port-forward svc/kubernaut-agent 8081:8081 -n kubernaut-system &
   curl http://localhost:8081/healthz
   curl http://localhost:8081/readyz
   ```

---

## Rollback

If issues are encountered, roll back to the previous release:

```bash
helm rollback kubernaut -n kubernaut-system
```

CRD changes are additive and do not require rollback. However, if you applied
new CRDs, the extra fields will be ignored by the v1.4 controllers.
