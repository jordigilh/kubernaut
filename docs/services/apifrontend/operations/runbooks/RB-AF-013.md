# RB-AF-013: SAR-based Tool Authorization Troubleshooting

**Severity:** Medium
**Alert:** `af_tool_calls_total{result="denied"}` increasing
**Component:** `pkg/apifrontend/auth/sar.go`, `pkg/apifrontend/handler/mcp_bridge.go`

## Symptom

Users report "permission denied" errors when invoking MCP or A2A tools. The
metric `af_tool_calls_total{result="denied"}` is incrementing.

## Diagnosis

### 1. Identify the Denied Tool and User

```bash
# Check AF logs for SAR denial details
kubectl logs -l app=apifrontend -n kubernaut-system | grep "SAR denied"
```

### 2. Verify ClusterRole Exists

```bash
# List tool-authorization ClusterRoles. NOTE: the label filter covers only
# resources that carry the optional kubernaut.ai/component=tool-authorization
# label (demo/example fixtures); your own roles may omit it, so also check by
# rule shape:
kubectl get clusterrole -l kubernaut.ai/component=tool-authorization
kubectl get clusterroles -o json | jq -r '.items[] | select(.rules[]? | .apiGroups? // [] | index("kubernaut.ai")) | .metadata.name'

# Inspect a role's allowed tools
kubectl get clusterrole kubernaut-tool-sre -o yaml
```

### 3. Verify ClusterRoleBinding Exists

```bash
# Check which OIDC groups are bound to tool-granting ClusterRoles (same
# label caveat as above)
kubectl get clusterrolebinding -l kubernaut.ai/component=tool-authorization
kubectl get clusterrolebindings -o json | jq -r '.items[] | select(.roleRef.name | test("tool|console")) | "\(.metadata.name) -> \(.roleRef.name)"'
```

### 4. Test SAR Manually

```bash
# Test if a specific user/group can use a tool
kubectl auth can-i use tools.kubernaut.ai/kubernaut_list_remediations \
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
| Missing ClusterRoleBinding | Create a binding from the user's OIDC group to a ClusterRole that grants the tool |
| Missing ClusterRole | Create (or re-apply) a ClusterRole enumerating the tool under `kubernaut.ai/tools`, verb `use`, resourceName = tool name. The Helm chart no longer ships user-facing tool roles (kubernaut-operator#682) — compose your own or start from the demo example bundles in `test/fixtures/demo/rbac/` |
| Wrong OIDC group | Verify the JWT `groups` claim matches the group in the ClusterRoleBinding |
| Cached denial | Wait for `sarCacheTTL` to expire, or restart the AF pod |
| AF ServiceAccount lacks SAR permission | Verify the AF ClusterRole includes `authorization.k8s.io/subjectaccessreviews: create` |

## Prevention

- Include ClusterRoleBindings in your deployment automation
- Monitor `af_tool_calls_total{result="denied"}` with an alert threshold
- Use `kubectl auth can-i` in CI to validate RBAC before deployment
