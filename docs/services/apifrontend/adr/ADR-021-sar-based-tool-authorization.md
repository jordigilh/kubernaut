# ADR-021: SAR-based Tool Authorization

**Status:** Accepted
**Date:** 2026-05-21
**Context:** Issue #1220, #1221 — Replace file-based RBAC with Kubernetes-native authorization
**Supersedes:** File-based `rbac_roles.yaml` approach (removed in v1.5)

## Decision

Replace the static file-based RBAC system (`rbac_roles.yaml`) with Kubernetes
SubjectAccessReview (SAR) for tool authorization in the API Frontend.

The AF performs a SAR check for every `tools/call` invocation using:

```yaml
verb:     use
group:    kubernaut.ai
resource: tools
name:     <toolName>
```

User identity (username, groups) is extracted from the JWT token and passed to
the SAR API. Results are cached with a configurable TTL (default 30s) to reduce
API server load.

## Rationale

1. **Kubernetes-native**: Leverages the existing Kubernetes RBAC engine rather
   than maintaining a parallel authorization system. ClusterRoles and
   ClusterRoleBindings are standard Kubernetes primitives that operators already
   understand.

2. **Industry alignment**: Projects like Red Hat OpenShift AI (rhoai-mcp) and
   dot-ai use SAR-based authorization for MCP/A2A tool access. This follows
   the emerging pattern for multi-tenant AI agent platforms.

3. **Operational simplicity**: Permissions are managed via standard `kubectl`
   commands and Helm values. No custom configuration files to maintain.

4. **Auditability**: SAR calls are logged by the Kubernetes audit system,
   providing a complete authorization audit trail without custom audit code.

5. **NIST compliance**: Maps to AC-3 (Access Enforcement), AC-6 (Least
   Privilege), and AU-12 (Audit Generation) controls.

## Implementation

### Components

- `pkg/apifrontend/auth/sar.go`: `ToolAuthorizer` interface and `SARChecker`
  implementation with TTL cache
- `pkg/apifrontend/handler/mcp_bridge.go`: `checkRBAC()` delegates to
  `ToolAuthorizer`
- `pkg/apifrontend/agent/root.go`: `newRBACGuard()` delegates to
  `ToolAuthorizer`
- `cmd/apifrontend/main.go`: Wires `SARChecker` with in-cluster K8s client

### Authorization Model

- `tools/list` is **unfiltered** (ADR-020 still applies)
- `tools/call` is **gated** by SAR check
- Fail-closed: SAR errors result in tool denial
- Cache: SHA256-keyed TTL cache with `sync.RWMutex`

### Helm Integration

Per-persona ClusterRoles are shipped via `values.yaml`:

```yaml
apifrontend.config.rbac.personas:
  sre: [tool1, tool2, ...]
  cicd: [tool1, ...]
```

Each persona generates a `kubernaut-tool-<persona>` ClusterRole with
`resourceNames` restricting access to specific tools.

## Consequences

- **Breaking change**: `rbac_roles.yaml` is removed. Customers must create
  ClusterRoleBindings mapping their OIDC groups to the shipped ClusterRoles.
- **Dependency**: Requires in-cluster Kubernetes client and
  `subjectaccessreviews` create permission on the AF ServiceAccount.
- **Performance**: TTL cache (default 30s) means permission changes take up to
  30s to propagate. Configurable via `rbac.sarCacheTTL`.

## Alternatives Considered

1. **Internal persona mapping**: Map JWT groups to roles internally. Rejected
   because it duplicates Kubernetes RBAC and doesn't integrate with existing
   IAM tooling.

2. **OPA/Rego policies**: More flexible but adds operational complexity and a
   new dependency. Kubernetes RBAC is sufficient for tool-level authorization.

3. **Keep file-based RBAC**: Simple but doesn't scale, lacks audit trail, and
   requires pod restart for permission changes.
