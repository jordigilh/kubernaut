# Spike: AAP MCP Server as Unified Ansible Execution Channel

> **Note**: This spike references Kuadrant MCP Gateway. The project has since switched
> to **Envoy AI Gateway** (see ADR-068). The AAP MCP Server findings remain valid --
> only the gateway infrastructure has changed.

**Date**: 2026-06-18
**Status**: GO -- AAP MCP server works as a third execution engine alongside K8s Jobs and Tekton
**Cluster**: OCP 4.21 (dev.redhat-internal.com), AAP 2.5 (controller v4.6.21)

## Objective

Validate whether the upstream `ansible/aap-mcp-server` can replace direct AWX REST API integration, enabling Kubernaut's WE controller to use a single protocol (MCP) for all three execution engines: K8s Jobs, Tekton PipelineRuns, and Ansible job templates.

## Findings

### AAP MCP Server Deployment

- **Upstream repo**: [ansible/aap-mcp-server](https://github.com/ansible/aap-mcp-server)
- **Official support**: Ships with AAP 2.6+ (operator or container deployment)
- **AAP 2.5 compatibility**: Works with a one-line patch to the token validation endpoint (`/api/gateway/v1/me/` -> `/api/v2/me/`) and disabling the gateway path rewrite (`/api/v2/` -> `/api/controller/v2/`). Both are AAP 2.6 gateway-format changes that don't exist in standalone controller mode.
- **Transport**: Streamable HTTP (JSON-RPC over SSE), same as K8s MCP server
- **Stateless sessions**: Unlike the K8s MCP server (stateful Mcp-Session-Id), the AAP MCP server is fully stateless. Each request carries its own Bearer token.

### Available Tools (job_management toolset)

| Tool | Method | Purpose |
|------|--------|---------|
| `job_templates_list` | GET | List all job templates |
| `job_templates_retrieve` | GET | Get specific template details |
| `job_templates_launch_retrieve` | GET | Check launch requirements (prompts, credentials) |
| `job_templates_launch_create` | POST | **Launch a job template** |
| `workflow_job_templates_list` | GET | List workflow templates |
| `workflow_job_templates_retrieve` | GET | Get workflow template details |
| `workflow_job_templates_launch_create` | POST | **Launch a workflow** |
| `jobs_list` | GET | List jobs |
| `jobs_retrieve` | GET | **Get job status** |
| `jobs_stdout_retrieve` | GET | Get job output |
| `jobs_job_events_list` | GET | Get job events |
| `jobs_cancel_create` | POST | Cancel a running job |
| `jobs_relaunch_create` | POST | Relaunch a failed job |
| `workflow_jobs_list` | GET | List workflow jobs |
| `workflow_jobs_retrieve` | GET | Get workflow job status |
| `workflow_jobs_workflow_nodes_list` | GET | Get workflow node details |

### E2E Validation Results

| Test | Result | Notes |
|------|--------|-------|
| List job templates | PASS | Returns all 3 templates (Demo, kubernaut-gitops-update-memory, kubernaut-migrate-postgres-emptydir-to-pvc) |
| Check launch requirements | PASS | `can_start_without_user_input: true` for Demo template |
| Launch job template | PASS | Returns `job_id=75`, status=`pending` |
| Poll job status | PASS | Status transitions: `pending` -> `successful` in 4.7s |
| Unified test (K8s + AAP MCP simultaneously) | PASS | K8s Job creation and AAP Job launch both succeed in parallel |
| Auth without token | PASS (rejected) | Returns `Unauthorized: Bearer token or session ID required` |
| Auth with valid token | PASS | Token validated against AAP's `/api/v2/me/` |

### Auth Model Comparison

| Aspect | K8s MCP Server | AAP MCP Server |
|--------|---------------|----------------|
| Auth mechanism | SA token (implicit, in-cluster) | Bearer token (AAP API token) |
| Client provides | Nothing (SA mounted in pod) | `Authorization: Bearer <token>` header |
| Session model | Stateful (`Mcp-Session-Id`) | Stateless (token per-request) |
| RBAC enforcement | K8s RBAC (ClusterRole/Role) | AAP RBAC (user permissions) |
| Token source | K8s API server | AAP `/api/v2/tokens/` |
| Token lifetime | Auto-rotated by kubelet | Configurable (no expiry by default) |

### Gateway Auth Implications

When the MCP Gateway fronts both K8s MCP and AAP MCP backends:

1. **KA -> Gateway**: Keycloak JWT with `mcp-read`/`mcp-write` roles
2. **Gateway -> K8s MCP**: No additional auth (in-mesh mTLS, SA implicit)
3. **Gateway -> AAP MCP**: Must inject AAP Bearer token from a stored credential (K8s Secret or Keycloak token exchange)

The Kuadrant MCP Gateway supports per-route `BackendPolicy` which can inject headers from Secrets. This is the mechanism for per-backend credential injection.

Alternatively, Keycloak token exchange (RFC 8693) could mint an AAP-scoped token from the KA's JWT, but this requires AAP to trust Keycloak as an identity provider.

## Three-Engine WE Controller Design

```
WE Controller
    │
    ├── K8s Engine Adapter
    │   └── MCP tools: resources_create_or_update, resources_get, resources_delete
    │   └── Resources: Job, PipelineRun, TaskRun
    │   └── Auth: implicit (SA)
    │   └── Status: Job/PipelineRun status.conditions
    │
    ├── Tekton Engine Adapter
    │   └── MCP tools: resources_create_or_update, resources_get, resources_delete
    │   └── Resources: PipelineRun, TaskRun
    │   └── Auth: implicit (SA)
    │   └── Status: PipelineRun status.conditions
    │
    └── Ansible Engine Adapter
        └── MCP tools: job_templates_launch_create, jobs_retrieve, jobs_cancel_create
        └── Resources: Job Template, Workflow Template
        └── Auth: Bearer token
        └── Status: jobs_retrieve -> status field
```

All three adapters:
- Use the same MCP protocol (JSON-RPC over Streamable HTTP)
- Route through the same MCP Gateway
- Authenticate via the same Keycloak JWT (KA -> Gateway)
- Differ only in tool names, resource types, and status polling logic

### WE Controller Lifecycle Mapping

| Phase | K8s/Tekton | Ansible |
|-------|-----------|---------|
| Pre-validate | `resources_get` (SA exists?) | `job_templates_launch_retrieve` (can launch?) |
| Execute | `resources_create_or_update` (Job/PR) | `job_templates_launch_create` |
| Poll | `resources_get` -> `status.conditions` | `jobs_retrieve` -> `status` |
| Cancel | `resources_delete` | `jobs_cancel_create` |
| Cleanup | `resources_delete` / TTL | AAP manages history |
| Failure types | `BackoffLimitExceeded`, `DeadlineExceeded` | `failed`, `error`, `canceled` |

## Compatibility Notes

### AAP 2.5 vs 2.6+

The AAP MCP server is designed for AAP 2.6+ which includes the Platform Gateway (`/api/controller/v2/` prefix, `/api/gateway/v1/me/` for auth). For AAP 2.5 (standalone controller), two changes are needed:

1. **Token validation**: `/api/gateway/v1/me/` -> `/api/v2/me/`
2. **Path rewrite**: Skip `/api/v2/` -> `/api/controller/v2/` rewrite

Both can be controlled via environment variables in a production deployment. The upstream repo should be patched to support both modes.

### AAP Deployment Topology (unchanged)

AAP remains centralized. The AAP MCP server deploys alongside AAP (same pod/namespace or adjacent). It does NOT deploy per-cluster. From Kubernaut's perspective, this is transparent -- the MCP Gateway routes to the right backend regardless of where it runs.

```
Cluster A                 Cluster B              Management Cluster
┌─────────────┐         ┌─────────────┐         ┌──────────────────────┐
│ K8s MCP     │         │ K8s MCP     │         │ MCP Gateway          │
│ (per-cluster)│         │ (per-cluster)│         │   ├── route: cluster_a_* -> A│
└─────────────┘         └─────────────┘         │   ├── route: cluster_b_* -> B│
                                                │   └── route: aap_*    -> AAP │
                                                │                      │
                                                │ AAP + AAP MCP Server │
                                                │ (centralized)        │
                                                │                      │
                                                │ KA (consumer)        │
                                                └──────────────────────┘
```

## Decision

**GO** -- The AAP MCP server provides a clean, protocol-compatible execution channel for Ansible workflows. The WE controller can use a unified MCP interface for all three engines (K8s Jobs, Tekton, Ansible), eliminating the need for a separate AWX REST API integration.

### Open Items

1. **Gateway credential injection**: Validate Kuadrant `BackendPolicy` for per-backend header injection (AAP Bearer token)
2. **Upstream patch**: Submit env-var-controlled compat mode for AAP 2.5 (skip gateway path rewrite + fallback auth endpoint)
3. **RBAC mapping**: Define how KA's Keycloak roles map to AAP user permissions (token exchange or pre-provisioned service account)
