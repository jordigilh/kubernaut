# DD-AF-004: Investigation Tool Split — kubernaut_investigate vs kubernaut_investigate_alert

**Status**: Accepted
**Date**: 2026-06-03
**Issue**: [#1372](https://github.com/jordigilh/kubernaut/issues/1372)
**Supersedes**: None

## Context

AF exposes MCP/ADK tools to the LLM for creating investigation requests. The original `kubernaut_investigate` tool accepted `namespace/kind/name` with an optional `rr_id`, and the backend's severity triager deterministically resolved the most relevant alert via `bestAlertMatch`.

Users requested the ability for the LLM to explicitly specify a Prometheus alert name when initiating an investigation, rather than relying solely on the backend's deterministic triage. This is useful when:

1. The user mentions a specific alert by name in the chat
2. Multiple alerts are firing for the same resource and the user wants to investigate a specific one
3. The alert has not yet triggered but the user wants a proactive investigation

## Decision

Split investigation into two distinct tools rather than adding an optional `alert_name` parameter to the existing tool:

### kubernaut_investigate (existing, updated)

- **Purpose**: Resource-first investigation; backend determines the alert
- **Required fields**: `api_version`, `kind`, `name` (or `rr_id` for existing RR)
- **Optional fields**: `namespace` (empty for cluster-scoped resources)
- **Change**: Added required `api_version` field for RESTMapper-aware resource resolution

### kubernaut_investigate_alert (new)

- **Purpose**: Alert-first investigation; LLM specifies the alert
- **Required fields**: `alert_name`, `api_version`, `kind`, `name`
- **Optional fields**: `namespace` (empty for cluster-scoped resources)
- **Backend validation**: Checks alert exists in Prometheus (active alerts or defined rules)

## Rationale

### Why two tools instead of one with optional alert_name?

LLMs struggle with optional parameters that significantly alter tool behavior. When a single tool has an optional `alert_name` that changes the validation path (backend triage vs. alert validation), the LLM may:

- Omit the alert_name when it should provide it
- Provide a partial alert name expecting fuzzy matching
- Not understand when backend triage vs. explicit alert is appropriate

Two explicit tools with clear names give the LLM unambiguous intent signals.

### Why api_version is required on both tools

Without `api_version`, `RESTMapper` cannot distinguish between Kinds that exist in multiple API groups (e.g., `Event` in `v1` and `events.k8s.io/v1`). Making it required ensures deterministic resource resolution. Since AF has not been released, this is not a breaking change.

### Why namespace is optional

Cluster-scoped resources (Node, PersistentVolume, Namespace) have no namespace. The backend determines scope from the empty namespace field. For namespaced resources, the LLM must provide it.

## Consequences

### Positive

- Clear LLM tool selection based on intent
- Fail-close alert validation prevents hallucinated alert names
- `api_version` enables precise resource resolution
- Cluster-scoped resource support (Node, PV)

### Negative

- Two tools increase prompt token usage slightly
- Conditional tool registration (requires PromClient) adds wiring complexity

## Implementation

- `pkg/apifrontend/tools/af_investigate_alert.go` — HandleInvestigateAlert + validateAlertExists
- `pkg/apifrontend/tools/ka_investigate_mcp.go` — Updated InvestigateMCPArgs with APIVersion
- `pkg/apifrontend/tools/af_create_rr.go` — CreateRRArgs with APIVersion/ClusterScoped, buildTargetResource
- `pkg/apifrontend/agent/root.go` — Conditional registration when PromClient is set
- `pkg/apifrontend/validate/k8s.go` — AlertName and APIVersion validators
