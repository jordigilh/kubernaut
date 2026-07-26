# Test Plan — #54: GatewayDiscoverer (Two-Phase Tool Discovery)

**IEEE 829 Compliant** | **Issue**: [#54](https://github.com/jordigilh/kubernaut/issues/54) | **Milestone**: v1.5

> **⚠️ Superseded in part by [DD-FLEET-004](../../architecture/decisions/DD-FLEET-004-cluster-transparent-tool-exposure.md) (issue #1732)**: `GatewayDiscoverer`,
> `KuadrantDiscoverer`, `EAIGWDiscoverer`, and `Registry` concurrency (§6.1-6.4, §6.6-6.7)
> are unchanged and this plan remains accurate for them. However, §6.5
> (`ListClustersTool`/`ListToolsForClusterTool`) describes types that were
> **deleted** — KA no longer exposes `list_clusters`/`list_tools_for_cluster`
> to the LLM; it calls `GatewayDiscoverer.ToolsForCluster()` itself,
> server-side, once per investigation. §6.8's `IT-KA-FLEET-010/011/012` IDs
> were reused by DD-FLEET-004's implementation for different assertions than
> described below — see `test/integration/kubernautagent/fleet/fleet_wiring_test.go`
> for their current meaning. See the callouts inline below for exactly what
> changed.

## 1. Test Plan Identifier

TP-54-GATEWAY-DISCOVERER

## 2. Introduction

### 2.1 Purpose

At fleet scale (100+ clusters, 1800+ tools), presenting all tools to the LLM wastes context tokens and causes hallucination. The `GatewayDiscoverer` interface provides two-phase tool discovery: `ListClusters` then `ToolsForCluster`. **As of DD-FLEET-004 (#1732), both are called by KA itself, server-side, at investigation launch, using the investigation's own `signal.ClusterID` — never by the LLM.** The original framing below ("the LLM can optionally expand to other clusters") described a two-phase *LLM-facing* discovery flow that was never fully implemented and has since been superseded; see DD-FLEET-004 for the current design and rationale.

### 2.2 Objectives

1. **Factory selection (FedRAMP CM-6)**: `gatewayType` configuration selects the correct discoverer; empty = fleet disabled; invalid value rejected
2. **Kuadrant discovery (FedRAMP AC-3)**: `KuadrantDiscoverer` wraps `discover_tools`/`select_tools` for server-side session scoping
3. **EAIGW discovery (FedRAMP AC-3)**: `EAIGWDiscoverer` scans `tools/list` and extracts `__` prefixes for client-side filtering
4. **Registry concurrency (FedRAMP SC-5)**: `sync.RWMutex` protects concurrent `Register`/`Get` on the tool registry
5. ~~**LLM tool boundary (FedRAMP SC-7)**: `list_clusters` and `list_tools_for_cluster` limit LLM context to relevant clusters~~ — **superseded by DD-FLEET-004**: these tools were removed entirely; the LLM never sees cluster boundaries at all (a stronger form of the same SC-7 control — see `docs/architecture/decisions/DD-FLEET-004-cluster-transparent-tool-exposure.md`)
6. **Singleflight deduplication (FedRAMP SC-5)**: concurrent tool-discovery calls for the same cluster execute the gateway call only once. Originally implemented in `ListToolsForClusterTool.Execute`; reimplemented in `gatewayOverlayResolver.Overlay` (`cmd/kubernautagent/toolregistry.go`, `UT-KA-FLEET-022/023`) after DD-FLEET-004 removed the LLM-facing tool
7. **Audit content (FedRAMP AU-3)**: Discovery operations logged with cluster ID and tool count

### 2.3 Business Requirements

- BR-FLEET-054: Multi-cluster federation with gateway-agnostic tool discovery

### 2.4 Authority

- ADR-068 decision #11: GatewayDiscoverer two-phase tool discovery for LLM context efficiency
- Spike S15 (2026-06-27): Validated Kuadrant `discover_tools`/`select_tools` response format

## 3. Features to be Tested

- F-1: `NewDiscoverer()` factory selects `KuadrantDiscoverer` or `EAIGWDiscoverer` based on `MCPGatewayType`
- F-2: `KuadrantDiscoverer.ListClusters()` calls `discover_tools` and returns cluster metadata
- F-3: `KuadrantDiscoverer.ToolsForCluster()` calls `select_tools` then `ListTools` for scoped schemas
- F-4: `EAIGWDiscoverer.ListClusters()` scans `tools/list` and extracts unique `__` prefixes
- F-5: `EAIGWDiscoverer.ToolsForCluster()` filters `tools/list` by cluster prefix
- F-6: `Registry` concurrent `Register`/`Get` safety via `sync.RWMutex`
- ~~F-7: `ListClustersTool` returns cluster metadata without tool names~~ — **deleted** (DD-FLEET-004); never had an LLM-facing equivalent afterward, since the LLM no longer discovers clusters at all
- ~~F-8: `ListToolsForClusterTool` returns and activates tools for a cluster via `singleflight`~~ — **deleted** (DD-FLEET-004); its dedup behavior now lives in `gatewayOverlayResolver.Overlay` (`cmd/kubernautagent/toolregistry.go`)
- F-9: KA `FleetConfig.GatewayType` parsing and validation
- F-10: `EffectiveMCPGatewayType()` cleanup — empty = error when fleet enabled

## 4. Features Not to be Tested

- MCP Gateway deployment and configuration (infrastructure concern)
- OAuth2 token acquisition (covered by existing `ReloadableOAuth2Transport` tests)
- kube-mcp-server tool responses (covered by existing `mcpclient` tests)
- E2E multi-cluster investigation with real LLM (covered by existing fleet E2E suite)

## 5. Approach

### Test Pyramid

| Tier | Scope | Count |
|------|-------|-------|
| Unit | Factory, discoverers, tools, config, registry concurrency | 28 |
| Integration | KA wiring (registerFleetTools dispatch path) | 3 |
| E2E | Two-phase discovery journey through Kuadrant gateway | 3 |

### FedRAMP Control Mapping

| Control | Title | Behavioral Assurance | Test IDs |
|---------|-------|---------------------|----------|
| CM-6 | Configuration Settings | `gatewayType` selects correct discoverer; empty = fleet disabled; invalid value rejected | UT-DISC-001, UT-DISC-002, UT-DISC-003, UT-KA-CFG-001, UT-KA-CFG-002, UT-FLEET-CFG-035, E2E-FLEET-DISC-001 |
| AC-3 | Access Enforcement | `ListClusters`/`ToolsForCluster` (called by KA server-side, per DD-FLEET-004) return only clusters/tools visible through gateway auth | UT-DISC-KUA-001..003, UT-DISC-EAIGW-001..003, E2E-FLEET-DISC-002, E2E-FLEET-DISC-003 |
| SC-7 | Boundary Protection | LLM never sees cluster boundaries at all — remote-cluster tools are pre-scoped server-side and exposed under the same generic names as local tools (DD-FLEET-004, stronger than the original LLM-facing `list_tools_for_cluster` boundary) | IT-KA-FLEET-011/012 (`test/integration/kubernautagent/fleet/fleet_wiring_test.go`), IT-KA-FLEET-017 (`test/integration/kubernautagent/investigator/fleet_prescoping_test.go`) |
| SC-5 | Denial of Service Protection | Concurrent tool registration protected by RWMutex; concurrent per-cluster discovery calls deduplicated by singleflight in `gatewayOverlayResolver.Overlay` | UT-REG-CONC-001..003, UT-KA-FLEET-022/023 (`cmd/kubernautagent/toolregistry_overlay_singleflight_test.go`) |
| AU-3 | Audit Content | Discovery operations logged with cluster ID and tool count; fleet overlay resolution failure emits `aiagent.fleet.overlay_failed` | UT-DISC-KUA-004, UT-DISC-EAIGW-004, IT-KA-FLEET-020 |

## 6. Test Cases

### 6.1 GatewayDiscoverer Factory (CM-6)

| ID | Test Case | Success Criteria | Control |
|----|-----------|-----------------|---------|
| UT-DISC-001 | NewDiscoverer with `GatewayKuadrant` returns KuadrantDiscoverer | Type assertion succeeds | CM-6 |
| UT-DISC-002 | NewDiscoverer with `GatewayEAIGW` returns EAIGWDiscoverer | Type assertion succeeds | CM-6 |
| UT-DISC-003 | NewDiscoverer with empty/invalid type returns error | Error contains "unsupported gateway type" | CM-6 |

### 6.2 KuadrantDiscoverer (AC-3)

| ID | Test Case | Success Criteria | Control |
|----|-----------|-----------------|---------|
| UT-DISC-KUA-001 | ListClusters calls discover_tools and returns cluster metadata without tool names | Result contains cluster name, categories, hint; no tool names in response | AC-3 |
| UT-DISC-KUA-002 | ListClusters with category filter returns filtered results | Only matching categories returned | AC-3 |
| UT-DISC-KUA-003 | ToolsForCluster calls select_tools then ListTools, returns scoped tool schemas | Returned tools match cluster prefix; full schemas present | AC-3 |
| UT-DISC-KUA-004 | ToolsForCluster for unknown cluster returns error | Error contains cluster ID | AU-3 |
| UT-DISC-KUA-005 | ListClusters when gateway has no servers returns empty slice | No error, empty result | -- |

### 6.3 EAIGWDiscoverer (AC-3)

| ID | Test Case | Success Criteria | Control |
|----|-----------|-----------------|---------|
| UT-DISC-EAIGW-001 | ListClusters extracts unique cluster IDs from `__` prefixed tools | Each unique prefix before `__` becomes a ClusterInfo | AC-3 |
| UT-DISC-EAIGW-002 | ListClusters with multiple clusters returns all | 3 clusters registered, 3 returned | AC-3 |
| UT-DISC-EAIGW-003 | ToolsForCluster filters tools by cluster prefix | Only `{clusterID}__*` tools returned with full schemas | AC-3 |
| UT-DISC-EAIGW-004 | ToolsForCluster for unknown cluster returns error | Error contains cluster ID | AU-3 |
| UT-DISC-EAIGW-005 | ListClusters ignores non-prefixed tools (meta-tools) | Tools without `__` separator not treated as clusters | -- |

### 6.4 Registry Concurrency (SC-5)

| ID | Test Case | Success Criteria | Control |
|----|-----------|-----------------|---------|
| UT-REG-CONC-001 | Concurrent Register and Get do not race | 100 goroutines Register + Get; no data race under `-race` | SC-5 |
| UT-REG-CONC-002 | Concurrent Register and Execute do not race | Register new tool while Execute runs; no panic, correct result | SC-5 |
| UT-REG-CONC-003 | Concurrent Register and ToolsForPhase do not race | Register while reading phase tools; consistent snapshot | SC-5 |

### 6.5 Discovery Tools (SC-7) — **deleted per DD-FLEET-004, kept below for history**

> `ListClustersTool` and `ListToolsForClusterTool` (and this entire section's
> UT-DISC-TOOL-\* tests) were **deleted** by DD-FLEET-004 (#1732): the LLM no
> longer has any tool to discover or select a cluster with. UT-DISC-TOOL-007's
> singleflight dedup guarantee is the one behavior from this section that was
> carried forward — see `UT-KA-FLEET-022/023` in
> `cmd/kubernautagent/toolregistry_overlay_singleflight_test.go`, which prove
> the same guarantee against `gatewayOverlayResolver.Overlay`.

| ID | Test Case | Success Criteria | Control |
|----|-----------|-----------------|---------|
| UT-DISC-TOOL-001 | ~~ListClustersTool.Name() returns "list_clusters"~~ | Exact match | -- |
| UT-DISC-TOOL-002 | ~~ListClustersTool.Execute returns JSON with cluster metadata, no tool names~~ | Response contains `clusters` array with `id`, `hint`, `categories` | SC-7 |
| UT-DISC-TOOL-003 | ~~ListToolsForClusterTool.Execute returns tool names and descriptions~~ | Response contains `tools` array with `name`, `description` | SC-7 |
| UT-DISC-TOOL-004 | ~~ListToolsForClusterTool.Execute with invalid cluster returns error text~~ | IsError true, text contains cluster ID | SC-7 |
| UT-DISC-TOOL-005 | ~~ListClustersTool.Parameters() returns valid JSON schema~~ | Schema has `properties` with optional `category` | -- |
| UT-DISC-TOOL-006 | ~~ListToolsForClusterTool.Parameters() returns valid JSON schema~~ | Schema has `required: ["cluster_id"]` | -- |
| UT-DISC-TOOL-007 | ~~Concurrent list_tools_for_cluster for same cluster deduplicates via singleflight~~ → superseded by **UT-KA-FLEET-022** | Two goroutines call simultaneously; gateway receives only 1 discover_tools call | SC-5 |
| UT-DISC-TOOL-008 | ~~Sequential list_tools_for_cluster for different clusters executes independently~~ → superseded by **UT-KA-FLEET-023** | cluster-a and cluster-b each trigger their own discovery | -- |

### 6.6 KA FleetConfig (CM-6)

| ID | Test Case | Success Criteria | Control |
|----|-----------|-----------------|---------|
| UT-KA-CFG-001 | FleetConfig parses gatewayType from YAML | `cfg.Fleet.GatewayType == "kuadrant"` | CM-6 |
| UT-KA-CFG-002 | FleetConfig with empty gatewayType means fleet disabled | `registerFleetTools` returns nil | CM-6 |

### 6.7 EffectiveMCPGatewayType Cleanup (CM-6)

| ID | Test Case | Success Criteria | Control |
|----|-----------|-----------------|---------|
| UT-FLEET-CFG-035 | Empty MCPGatewayType with non-empty endpoint fails validation | Error: "mcpGatewayType is required when fleet is enabled" | CM-6 |

### 6.8 Integration Tests — KA Wiring (proves wiring)

> **IDs reused by DD-FLEET-004** (#1732): the three rows below describe the
> *original* intent of these IDs, no longer what the code at these IDs
> actually asserts. `test/integration/kubernautagent/fleet/fleet_wiring_test.go`
> repurposed `IT-KA-FLEET-010/011/012` to assert the opposite of the first
> row (discovery tools are **absent**) and to prove generic-named tool
> resolution/execution for the other two. Kept here for historical record;
> do not use this table to look up current test intent.

| ID | Test Case | Success Criteria | Control |
|----|-----------|-----------------|---------|
| ~~IT-KA-FLEET-010~~ | ~~registerFleetTools with gatewayType=kuadrant registers list_clusters tool~~ → now: asserts `list_clusters`/`list_tools_for_cluster` are **absent** from the LLM-facing registry | ~~Tool registry contains "list_clusters"~~ | CM-6, AC-4/AC-6 |
| ~~IT-KA-FLEET-011~~ | ~~registerFleetTools with gatewayType=eaigw registers list_tools_for_cluster tool~~ → now: eaigw pre-scoping resolves a generic name to the correct cluster-prefixed wire tool | ~~Tool registry contains "list_tools_for_cluster"~~ | CM-6, AC-6 |
| ~~IT-KA-FLEET-012~~ | ~~registerFleetTools pre-scopes target cluster tools as BridgeTools~~ → now: automatic pre-scoping resolves the LLM's generic tool call to the one target cluster, never another | Registry contains `{prefix}resources_get` and `{prefix}resources_list` | SC-7, AC-4/AC-6 |

### 6.9 E2E Tests — Two-Phase Discovery Journey (proves journey)

| ID | Test Case | Success Criteria | Control |
|----|-----------|-----------------|---------|
| E2E-FLEET-DISC-001 | ListClusters discovers loopback-cluster via discover_tools meta-tool | `loopback-cluster` in returned clusters | CM-6 |
| E2E-FLEET-DISC-002 | ToolsForCluster returns scoped tools via select_tools for loopback-cluster | All returned tools have `loopback_cluster_` prefix; `namespaces_list` present | AC-3 |
| E2E-FLEET-DISC-003 | Full journey: discover → scope → call tool succeeds | `namespaces_list` tool call returns namespace data with no error | AC-3 |

## 7. Pass/Fail Criteria

- All unit tests pass: `go test ./pkg/fleet/mcpclient/... ./pkg/kubernautagent/tools/registry/... -count=1`
- All integration tests pass: `go test ./test/integration/kubernautagent/fleet/... -count=1`
- All tests pass under `-race`: `go test -race ./pkg/fleet/mcpclient/... ./pkg/kubernautagent/tools/registry/...`
- Zero regressions in existing tests
- 100% UT coverage on business logic (discoverer methods, factory, tools)
- `go build ./...` succeeds
- All E2E tests pass: `FLEET_E2E=true ginkgo -v ./test/e2e/fleet/... --focus="E2E-FLEET-DISC"`
- Pyramid Invariant satisfied: every component has UT, IT, and E2E coverage (where applicable)

## 8. Pyramid Invariant Compliance

| Component | UT (proves logic) | IT (proves wiring) | E2E (proves journey) | Status |
|-----------|-------------------|-------------------|---------------------|--------|
| GatewayDiscoverer factory | UT-DISC-001/002/003 | IT-KA-FLEET-010/011 | E2E-FLEET-DISC-001 | -- |
| KuadrantDiscoverer | UT-DISC-KUA-001..005 | IT-KA-FLEET-010 | E2E-FLEET-DISC-001/002/003 | -- |
| EAIGWDiscoverer | UT-DISC-EAIGW-001..005 | IT-KA-FLEET-011 | -- (no EAIGW in E2E infra) | -- |
| Registry RWMutex | UT-REG-CONC-001/002/003 | IT-KA-FLEET-012 | -- (concurrency, not journey) | -- |
| ~~ListClustersTool~~ (deleted) | ~~UT-DISC-TOOL-001/002/005~~ | ~~IT-KA-FLEET-010/011~~ | ~~E2E-FLEET-DISC-001~~ | superseded, see DD-FLEET-004 |
| ~~ListToolsForClusterTool~~ + singleflight | ~~UT-DISC-TOOL-003/004/006~~; dedup now UT-KA-FLEET-022/023 | ~~IT-KA-FLEET-012~~ | ~~E2E-FLEET-DISC-002~~ | superseded, see DD-FLEET-004 |
| `gatewayOverlayResolver.Overlay` (replaces both tools above) | UT-KA-FLEET-022/023 (singleflight) | IT-KA-FLEET-011/012, IT-KA-FLEET-013/015 | E2E-KA-FLEET-001 | -- |
| FleetConfig.GatewayType | UT-KA-CFG-001/002 | IT-KA-FLEET-010 | -- (config, not journey) | -- |
| EffectiveMCPGatewayType cleanup | UT-FLEET-CFG-035 | -- (config validation) | -- | -- |

## 9. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | UT Test ID | IT Test ID |
|-----------|----------------------|---------------------|------------|------------|
| GatewayDiscoverer interface | NewDiscoverer() | pkg/fleet/mcpclient/discovery.go | UT-DISC-001/002/003 | IT-KA-FLEET-010/011 |
| KuadrantDiscoverer | NewDiscoverer(GatewayKuadrant, session) | pkg/fleet/mcpclient/discovery_kuadrant.go | UT-DISC-KUA-001..005 | IT-KA-FLEET-010 |
| EAIGWDiscoverer | NewDiscoverer(GatewayEAIGW, session) | pkg/fleet/mcpclient/discovery_eaigw.go | UT-DISC-EAIGW-001..005 | IT-KA-FLEET-011 |
| Registry RWMutex | Register(), Get(), Execute() | pkg/kubernautagent/tools/registry/registry.go | UT-REG-CONC-001/002/003 | IT-KA-FLEET-012 |
| ~~ListClustersTool~~ (deleted, DD-FLEET-004) | ~~registerFleetTools()~~ | ~~cmd/kubernautagent/main.go~~ | ~~UT-DISC-TOOL-001/002/005~~ | ~~IT-KA-FLEET-010/011~~ |
| ~~ListToolsForClusterTool~~ (deleted, DD-FLEET-004) | ~~registerFleetTools()~~ | ~~cmd/kubernautagent/main.go~~ | ~~UT-DISC-TOOL-003/004/006/007/008~~ | ~~IT-KA-FLEET-012~~ |
| `gatewayOverlayResolver` (replaces both rows above) | `registerFleetTools()` -> `Investigator.prescopeFleetOverlay()` | cmd/kubernautagent/toolregistry.go, internal/kubernautagent/investigator/fleet_overlay.go | UT-KA-FLEET-022/023 | IT-KA-FLEET-010/011/012/013/015 |
| FleetConfig.GatewayType | KA config load | internal/kubernautagent/config/config.go | UT-KA-CFG-001/002 | IT-KA-FLEET-010 |
| EffectiveMCPGatewayType removal | FleetConfig.Validate() | pkg/fleet/config.go | UT-FLEET-CFG-035 | -- |

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-06-27 | Initial test plan |
| 1.1 | 2026-06-27 | Added E2E tests (E2E-FLEET-DISC-001..003) for Pyramid Invariant journey layer |
| 1.2 | 2026-07-26 | DD-FLEET-004 (#1732): `ListClustersTool`/`ListToolsForClusterTool` deleted -- KA pre-scopes tools server-side instead of via LLM-facing discovery tools. Marked §2.1, §2.2 obj. 5-6, §6.5, §6.8, §8, §9 superseded/struck-through in place rather than deleting, to preserve the historical record of what #54 originally built. `IT-KA-FLEET-010/011/012` IDs were reused with different meanings by the DD-FLEET-004 implementation; singleflight dedup (SC-5) reimplemented as `UT-KA-FLEET-022/023` against `gatewayOverlayResolver.Overlay`. |
