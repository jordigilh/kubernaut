# MCP Write Path Spike -- Decision Matrix

**Date**: 2026-06-18
**Status**: All GO -- MCP server is viable as a unified read/write channel for remote workflow execution
**Cluster**: OCP 4.21 (dev.redhat-internal.com)

## Go/No-Go Questions

| # | Question | Threshold | Result | Evidence |
|---|----------|-----------|--------|----------|
| 1 | **Write capability**: Can K8s MCP server create/update/delete Jobs? | Working CRUD | **GO** | `resources_create_or_update` creates Jobs via server-side apply. `resources_delete` removes them. Requires `patch` verb in RBAC. |
| 2 | **SA boundary**: Does Job run under workflow SA, not MCP server SA? | Clean boundary | **GO** | Job `spec.template.spec.serviceAccountName` controls pod identity. Verified: pod runs as `kubernaut-wf-test`, not `mcp-server-sa`. |
| 3 | **Secret handling**: Does KA's sanitization pipeline handle MCP-transported secrets? | Fail-closed redaction | **GO** | MCP returns raw Secret data. KA's 3-stage pipeline (G4/K8S-SECRET/I1) redacts before LLM sees it. No RBAC exclusion needed. |
| 4 | **Gateway routing**: Do write ops work through a network hop (service/route)? | Write through proxy | **GO** | Job creation through K8s Service (in-cluster) succeeds. Route (edge-terminated TLS) confirmed. |
| 5 | **JWT auth scoping**: Can Keycloak mint differentiated read/write JWTs? | Role-scoped tokens | **GO** | `kubernaut-mcp-spike` client gets `mcp-read` + `mcp-write` roles. `kubernaut-mcp-readonly` gets only `mcp-read`. TTL: 300s. |
| 6 | **Failure detection**: Can KA detect Job failures, timeouts, and RBAC errors? | All modes detectable | **GO** | `BackoffLimitExceeded`, `DeadlineExceeded`, and SA boundary violations all detectable via Job status conditions. |
| 7 | **Idempotency**: Is `resources_create_or_update` safe to retry? | No duplicate creation | **GO** | Server-side apply is idempotent. Second call succeeds without creating duplicates. |
| 8 | **TTL cleanup**: Does `ttlSecondsAfterFinished` auto-remove Jobs? | Automatic cleanup | **GO** | Job with `ttlSecondsAfterFinished: 10` auto-deleted within 18s of completion. |

## Overall Decision

**GO** -- The `containers/kubernetes-mcp-server` is viable as a **unified investigation + execution channel** for Kubernaut's multi-cluster architecture. No separate write API is needed.

## Key Architectural Decisions

### 1. Unified MCP Channel (Read + Write)

The MCP server handles both investigation (read) and workflow execution (write) through the same transport. This eliminates the need for a separate ManifestWork-style API or direct `client-go` connections to remote clusters.

### 2. RBAC Model: Two-Layer

| Layer | SA | Scope | Purpose |
|-------|-----|-------|---------|
| **MCP Server SA** | `mcp-server-sa` | ClusterRole: read-all + create/patch/delete Jobs,PipelineRuns | What tools the MCP server exposes |
| **Workflow SA** | Per-workflow (e.g., `kubernaut-wf-test`) | Namespace-scoped Role: only resources the workflow needs | What the Job pod can actually do |

The MCP server SA has broad permissions (aligned with KA's RBAC). The workflow SA constrains what the remediation pod executes. This is the same privilege separation model KA already uses.

### 3. Secret Handling: Application-Layer Redaction

Secrets are **not** excluded from MCP server RBAC. The MCP server returns raw Secret data, and KA's sanitization pipeline redacts it before the LLM sees it. This matches the existing Kubernaut security model (SOC2-compliant, fail-closed).

### 4. `resources_create_or_update` Requires `patch` Verb

The MCP server's `resources_create_or_update` tool uses server-side apply (PATCH with `application/apply-patch+yaml`), not POST. RBAC must include the `patch` verb for any resource type the MCP server needs to create.

## Spike Findings Summary

| Test | Result | Notes |
|------|--------|-------|
| Create Job via MCP | PASS | Server-side apply with `patch` verb |
| Job runs under workflow SA | PASS | Pod identity = `system:serviceaccount:demo-hpa:kubernaut-wf-test` |
| Workflow SA RBAC enforced | PASS | Can patch HPAs (allowed), cannot delete deployments or access other namespaces (denied) |
| Read Job status via MCP | PASS | `resources_get` returns full Job with status conditions |
| Delete Job via MCP | PASS | `resources_delete` removes Job cleanly |
| Secret read via MCP | PASS | Returns raw data (KA sanitization handles redaction) |
| SA pre-validation | PASS | `resources_get` on SA before Job creation detects missing SAs |
| Job failure detection | PASS | `BackoffLimitExceeded` condition visible in Job status |
| Job timeout detection | PASS | `DeadlineExceeded` condition from `activeDeadlineSeconds` |
| Idempotent creation | PASS | Second `resources_create_or_update` succeeds (server-side apply) |
| TTL auto-cleanup | PASS | `ttlSecondsAfterFinished: 10` removes Job ~8-18s after completion |
| Non-existent SA handling | PASS | Job creates but pod fails to schedule; detectable via status |
| Write through K8s Service | PASS | MCP tool call routes through `kubernetes-mcp-server.spike-mcp-write.svc:8080` |
| JWT minting (full access) | PASS | `resource_access.kubernaut-mcp-spike.roles: [mcp-read, mcp-write]` |
| JWT minting (read-only) | PASS | `resource_access.kubernaut-mcp-readonly.roles: [mcp-read]` |

## Implementation Impact

### WE Controller Changes

1. **SA pre-validation**: Before creating a Job, call `resources_get` on the target ServiceAccount via MCP. Abort if SA doesn't exist.
2. **Job creation**: Use `resources_create_or_update` with the full Job manifest including `serviceAccountName`.
3. **Status polling**: Poll `resources_get` on the Job, check `status.conditions` for `Complete` or `Failed`.
4. **Failure classification**: Map Job condition reasons to WFE failure types:
   - `BackoffLimitExceeded` -> `WorkflowFailed`
   - `DeadlineExceeded` -> `WorkflowTimeout`
   - SA missing -> `PreconditionFailed`
5. **Cleanup**: Use `resources_delete` for explicit cleanup, `ttlSecondsAfterFinished` as safety net.

### MCP Server RBAC Template

The MCP server SA needs a ClusterRole with:
- **Investigation verbs**: `get`, `list`, `watch` on all resource types KA currently reads
- **Execution verbs**: `create`, `get`, `list`, `watch`, `patch`, `delete` on `jobs` (batch) and `pipelineruns`, `taskruns` (tekton.dev)
- **SA lookup**: `get` on `serviceaccounts` (for pre-validation)

### Gateway Auth Model (Production)

| Component | Mechanism |
|-----------|-----------|
| KA -> Gateway | JWT (Keycloak client_credentials grant) |
| Gateway -> MCP Server | In-mesh mTLS (Istio) |
| JWT roles | `mcp-read` (investigation), `mcp-write` (execution) |
| Gateway enforcement | Kuadrant AuthPolicy validates JWT roles per tool prefix |
| Token TTL | 300s (short-lived, refreshed per investigation session) |
