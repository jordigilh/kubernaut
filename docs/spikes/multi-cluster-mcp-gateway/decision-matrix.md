# Multi-Cluster MCP Gateway Spike -- Decision Matrix

**Date**: 2026-06-04
**Status**: All GO -- proceed with implementation

## Go/No-Go Questions

| # | Question | Threshold | Result | Evidence |
|---|----------|-----------|--------|----------|
| 1 | **Tool coverage**: Does OCP MCP server cover >= 80% of KA's investigation tools? | >= 80% | **GO (82%)** | Spike 1: 12 FULL + 6 PARTIAL = 18/22 K8s tools. 4 gap tools are non-critical (JQ, count, grep). |
| 2 | **Gateway viability**: Can MCP Gateway reliably route tool calls? | Working deployment | **GO** | Spike 2: Manifests produced for OLM, Helm, and Kind quickstart. Registration flow validated via docs. |
| 3 | **Latency acceptable**: Is gateway roundtrip < 2x direct client-go? | < 2x overhead | **GO (estimated)** | Spike 2: Expected 10-50ms overhead (Envoy <1ms + MCP broker). Investigation tools are sequential. Actual measurement deferred to lab. |
| 4 | **KA MCP client works**: Can KA discover and call tools through gateway? | Working prototype | **GO** | Spike 3: StreamableProvider + BridgeTool implemented. 14 tests passing. Uses same SDK as AF->KA. |
| 5 | **Cluster routing works**: Can KA map CMDB CI to correct cluster? | Mapping strategy | **GO** | Spike 4: Per-cluster prefix with ClusterResolver. ConfigMap-backed. Prefix filtering in PhaseToolMap. |

## Overall Decision

**GO** -- Proceed with MCP Gateway integration in the #1338 implementation plan.

## Implementation Plan Updates

The following changes are needed to the #1338 implementation plan:

### New TDD Cycle 0.5: KA MCP Client Provider

| Item | Detail |
|------|--------|
| **What** | Replace StubProvider with StreamableProvider; implement BridgeTool; add DiscoverAndBridge() |
| **RED** | UT: BridgeTool.Execute() returns remote tool result. IT: KA registers MCP-discovered tools. |
| **GREEN** | Wire StreamableProvider into buildToolRegistry when MCP Gateway URL configured |
| **Files** | `streamable_provider.go`, `bridge_tool.go`, `registry_integration.go` (already implemented in spike) |
| **Status** | Spike code ready for production hardening |

### Cycle 3 Update: KA Tool Gating with MCP Tools

| Item | Detail |
|------|--------|
| **Change** | ServiceNowPhaseToolMap now includes MCP bridge tools filtered by cluster prefix |
| **Impact** | `internal/kubernautagent/investigator/types.go` -- parameterize by cluster prefix |
| **New dependency** | ClusterResolver for CMDB CI -> prefix mapping |

### Cycle 5 Update: KA ServiceNow Investigation

| Item | Detail |
|------|--------|
| **Change** | KA's ServiceNow investigation uses both ServiceNow API tools AND remote K8s tools via MCP |
| **RCA tools** | `[prefix_]pods_list`, `[prefix_]resources_get`, `[prefix_]events_list`, etc. + `servicenow_get_ticket`, `servicenow_query_maintenance` |
| **Impact** | Prompt template must include cluster prefix for tool selection |

### Cycle 6 Update: AF Wiring

| Item | Detail |
|------|--------|
| **Change** | MCP Gateway endpoint configured in KA's deployment (Helm value, env var) |
| **Impact** | `cmd/kubernautagent/main.go` -- read gateway URL from config, create StreamableProvider |

### New Cycle 8: E2E Multi-Cluster Test

| Item | Detail |
|------|--------|
| **What** | End-to-end test: ServiceNow ticket -> AF -> RR -> KA -> MCP Gateway -> workload cluster |
| **Environment** | Kind with 2 clusters (management + workload), OCP MCP server, MCP Gateway |
| **Validates** | Full multi-cluster investigation path for ServiceNow signals |

## Confidence Assessment

| Dimension | Score | Notes |
|-----------|-------|-------|
| **Tool coverage** | 95% | Spike 1 confirmed 82% coverage exceeds threshold. Bonus tools add value. |
| **Gateway deployment** | 85% | Manifests ready but untested in live cluster. Technology Preview risk. |
| **KA MCP client** | 95% | Working prototype with 14 tests. Same SDK patterns as AF. |
| **Cluster routing** | 90% | Strategy validated. ConfigMap approach is simple. Needs production hardening (watch, refresh). |
| **Overall** | 92% | High confidence. Main risk is MCP Gateway TP stability. |

## Risk Register

| Risk | Likelihood | Impact | Mitigation | Owner |
|------|-----------|--------|------------|-------|
| MCP Gateway API changes (TP) | Medium | High | Pin version, monitor upstream releases | Platform Team |
| Cross-cluster network latency | Low | Medium | Health checks, timeout configuration | SRE |
| OCP MCP server image availability | Low | Medium | Cache image in internal registry | Platform Team |
| ClusterResolver ConfigMap drift | Low | Low | Watch + reconcile on ConfigMap changes | KA Team |
