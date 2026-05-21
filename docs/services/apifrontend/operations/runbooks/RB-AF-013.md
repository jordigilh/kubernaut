# RB-AF-013: SAR-based Tool Authorization Troubleshooting

**Severity:** Medium
**Alert:** `af_mcp_rbac_denied_total` increasing
**Component:** `pkg/apifrontend/auth/sar.go`, `pkg/apifrontend/handler/mcp_bridge.go`

## Symptom

Users report "permission denied" errors when invoking MCP or A2A tools. The
metric `af_mcp_rbac_denied_total` is incrementing.

## Diagnosis

### 1. Identify the Denied Tool and User

```bash
# Check AF logs for SAR denial details
kubectl logs -l app=apifrontend -n kubernaut-system | grep "SAR denied"
```

### 2. Verify ClusterRole Exists

```bash
# List per-persona ClusterRoles
kubectl get clusterrole -l kubernaut.ai/component=tool-authorization

# Inspect a specific persona's allowed tools
kubectl get clusterrole kubernaut-tool-sre -o yaml
```

### 3. Verify ClusterRoleBinding Exists

```bash
# Check if the user's OIDC group is bound to the correct ClusterRole
kubectl get clusterrolebinding -l kubernaut.ai/component=tool-authorization
```

### 4. Test SAR Manually

```bash
# Test if a specific user/group can use a tool
kubectl auth can-i use tools.kubernaut.ai/af_list_events \
  --as=alice@example.com \
  --as-group=platform-sre
```

### 5. Check Cache TTL

SAR results are cached for `rbac.sarCacheTTL` (default 30s). If a
ClusterRoleBinding was just created, the AF may still have a cached "deny"
result.

```bash
# Check current cache TTL in config
kubectl get cm apifrontend-config -n kubernaut-system -o yaml | grep sarCacheTTL
```

Wait for the TTL to expire, or restart the AF pod to clear the cache.

## Resolution

| Cause | Fix |
|-------|-----|
| Missing ClusterRoleBinding | Create a binding for the user's OIDC group to the appropriate `kubernaut-tool-<persona>` ClusterRole |
| Missing ClusterRole | Ensure the Helm chart deployed all 6 persona ClusterRoles. Re-run `helm upgrade`. |
| Wrong OIDC group | Verify the JWT `groups` claim matches the group in the ClusterRoleBinding |
| Cached denial | Wait for `sarCacheTTL` to expire, or restart the AF pod |
| AF ServiceAccount lacks SAR permission | Verify the AF ClusterRole includes `authorization.k8s.io/subjectaccessreviews: create` |

## Prevention

- Include ClusterRoleBindings in your deployment automation
- Monitor `af_mcp_rbac_denied_total` with an alert threshold
- Use `kubectl auth can-i` in CI to validate RBAC before deployment
