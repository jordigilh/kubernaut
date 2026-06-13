# ADR-022: AF ServiceAccount Unified Security Model

**Status:** Accepted
**Date:** 2026-05-21
**Deciders:** AF team (supervised)
**Supersedes:** [ADR-018](ADR-018-impersonation-risk-acceptance.md) (impersonation risk acceptance)
**NIST Controls:** AC-2, AC-3, AC-6, AU-2, AU-12
**Issue:** #1232

## Context

Issue #1226 introduced OIDC-direct authentication so MCP bridge CRD tools would
use the user's K8s identity (either via impersonation headers or raw OIDC JWT).
Issue #1230 made internal triage tools (`kubectl_get`, `kubectl_list`,
`kubectl_list_events`, `af_check_existing_rr`, `af_create_rr`) use the AF
ServiceAccount in the A2A path.

The intended security model is: **MCP RBAC is the application-level gate; users
have no K8s RBAC for kubernaut CRDs**. This means user-scoped K8s clients in the
MCP bridge fail (403 Forbidden) and must be switched to AF SA.

### Problems with the Prior Model

1. **Operational burden**: Users would need K8s RBAC bindings for kubernaut CRDs,
   creating dual authorization surfaces (MCP RBAC + K8s RBAC) that must be kept
   in sync.
2. **Privilege leakage**: Granting users K8s RBAC for `remediationrequests`
   allows them to use `kubectl` directly, bypassing AF's audit trail and MCP RBAC.
3. **Split identity model**: Internal triage tools used AF SA while domain CRD
   tools used user identity, creating inconsistent security semantics.

## Decision

**All K8s API calls made by AF use the AF pod ServiceAccount**, regardless of
entry point (MCP or A2A). User identity is preserved in the application audit
trail via tool callbacks (`tool.executed` events with `UserID`).

### What This Means

- The AF SA ClusterRole is the single K8s identity for all CRD and triage
  operations.
- MCP RBAC (SAR-based tool authorization) is the sole application-level access
  control gate.
- Users never need K8s RBAC bindings for kubernaut CRDs.
- Per-user K8s impersonation and OIDC-direct authentication modes are removed.

### Code Changes

| Component | Change |
|-----------|--------|
| `MCPBridgeConfig.K8sClient` | Changed from `DynamicClientFactory` (function) to `dynamic.Interface` (static client) |
| `AgentConfig.ImpersonatingClientFactory` | Removed. A2A tools use `backendDeps.K8sClient()` |
| `auth/dynamic_impersonation.go` | Retained only `DynamicClientFactory` type + `StaticDynamicFactory`. Removed `NewImpersonatingDynamicFactory`, `NewOIDCDirectDynamicFactory`, `AuditingDynamicFactory`, `ClientWrapper`. |
| `auth/impersonation.go` | Deleted entirely |
| `config.RBACConfig.UseOIDCDirect` | Removed |
| `cmd/apifrontend/main.go` | Removed `buildDynFactory()`, `backendDeps.DynFactory` field |
| Auth middleware | Added `stripImpersonationHeaders()` to reject K8s impersonation headers |

## Accepted Risks

### SEC-01 / SEC-03: Namespace Scope Expansion (CRITICAL)

**Risk**: MCP RBAC is tool-level, not namespace-level. Users can reach any
namespace the AF SA can access.

**Mitigation**:
- MCP CRD tools only access `kubernaut.ai` CRDs (not arbitrary K8s resources)
- Write surface is narrow: approve = RAR status patch, cancel = RR status patch
- Workflow execution requires a kubernaut-managed RCA target
- Internal triage tools are read-only with Secret data redaction

**Decision**: Accepted.

### SEC-02: K8s Audit Attribution Loss (CRITICAL)

**Risk**: K8s API server audit logs show AF SA for all operations, not the end
user.

**Mitigation**:
- AF application audit trail (`tool.executed` events with `UserID`) is the
  authoritative source for user attribution
- Cross-layer correlation possible via timestamp + resource path
- Same operational pattern as ArgoCD, Backstage, and other platform tooling

**Decision**: Accepted.

### SEC-04: AF SA Privilege Aggregation (HIGH)

**Risk**: AF SA is a high-value target with broad read + narrow CRD write
permissions.

**Mitigation**:
- No `delete` verb on `remediationrequests` or `remediationapprovalrequests`
- Secret data redacted in triage tool responses
- SA token not exposed to users or LLM
- Network policies restrict egress

**Decision**: Accepted.

### SEC-09: Approval Without K8s RBAC Safety Net (HIGH)

**Risk**: Persona assignment is the critical security boundary. Misconfiguring a
persona has no K8s RBAC fallback.

**Mitigation**:
- Document persona assignment as a security-sensitive operation
- Audit events track all tool invocations with user identity
- **UPDATE (#1415)**: `kubernaut_approve` structurally removed from A2A agent toolset.
  Approvals now execute exclusively via MCP endpoint (Console UI), which enforces:
  - Kubernetes SAR authorization on `kubernaut.ai/tools/kubernaut_approve`
  - OIDC-authenticated user identity attribution
  - Audit event with human user, never LLM
- See DD-AF-006 for full defense-in-depth rationale

**Decision**: Accepted. Risk significantly reduced by #1415 structural removal.

### SEC-11: `tools/list` Reconnaissance (MEDIUM, Pre-existing)

Already accepted in [ADR-020](ADR-020-mcp-bridge-rbac-runtime.md). Users can see
tool names but cannot execute without the right persona.

### SEC-13: HA Replay Protection (MEDIUM, Pre-existing)

Requires distributed cache infrastructure (Redis). Filed as follow-up issue.
Current in-memory `ReplayCache` is adequate for single-replica deployments.

### SEC-16: Label Selector Pass-through (LOW, Pre-existing)

K8s API server validates label selectors. Accepted.

## Audit Fixes Included in This PR

| ID | Finding | Severity | Fix |
|----|---------|----------|-----|
| SEC-05 | A2A rate limiting gap | HIGH | Added `AllowToolCall` check in `newRateLimitGuard` |
| SEC-06 | Source IP not in audit store | HIGH | Wired existing `actor_ip` column + DS conversion + AF StoreAdapter mapping |
| SEC-07 | A2A RBAC denial field mismatch | HIGH | Fixed `"tool"` to `"tool_name"` in `agent/root.go` |
| SEC-08 | Missing audit for no-identity/authorizer-error denials | HIGH | Added `auditor.Emit` for two denial paths |
| SEC-10 | Internal tool SAR grants missing | MEDIUM | Added tool names to `sre` and `ai-orchestrator` personas |
| SEC-12 | No impersonation header stripping | MEDIUM | Added `stripImpersonationHeaders` in auth middleware |
| SEC-14 | `impersonation.created` audit event removed | MEDIUM | Documented removal; `tool.executed` with `UserID` replaces it |
| SEC-15 | Minimal tool params in audit | MEDIUM | Added `target_namespace`, `target_kind` to tool executed payload |

## Consequences

1. Supersedes #1226 (OIDC-direct authentication mode) and ADR-018 (impersonation
   risk acceptance).
2. Removes impersonation factories, `UseOIDCDirect` config, `buildDynFactory()`,
   and `auth/impersonation.go`.
3. AF SA ClusterRole must include all permissions needed by MCP bridge tools
   (CRD operations + triage reads).
4. User attribution is exclusively via application audit trail, not K8s audit logs.
5. Persona assignment in `values.yaml` becomes a security-critical configuration.

## Alternatives Considered

### A. Keep User-Scoped K8s Clients (Status Quo)

Rejected. Requires users to have K8s RBAC for kubernaut CRDs, creating dual
authorization surfaces and enabling kubectl-direct access that bypasses AF
audit and MCP RBAC.

### B. Hybrid: AF SA for Internal Tools, User Identity for CRD Tools

This was the #1226/#1230 state. Rejected because (a) users shouldn't need K8s
RBAC for kubernaut CRDs, and (b) split identity model creates inconsistent
security semantics.

### C. Namespace-Scoped AF SA (One SA per Namespace)

Rejected. Increases operational complexity without material security benefit.
MCP RBAC is the intended access control layer.
