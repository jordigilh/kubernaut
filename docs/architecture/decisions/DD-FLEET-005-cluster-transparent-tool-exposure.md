# DD-FLEET-005: Cluster-Transparent Tool Exposure for KA's RCA Investigation

**Status**: ✅ Implemented (issue #1732)
**Date**: 2026-07-25
**Author**: AI Assistant
**Related**: Issue #1729, Issue #1732, ADR-068 (decision #11), BR-INTEGRATION-054, BR-INTEGRATION-1489

---

## Context

Issue #1729 investigated why KubernautAgent's (KA) fleet MCP tool discovery
is wired in Go (`cmd/kubernautagent/toolregistry.go:registerFleetTools`) but
never rendered by the Helm chart. That investigation surfaced a second,
more fundamental gap in the *design* itself, independent of the Helm
parity issue: **the LLM is currently the one deciding which cluster to
investigate, and it shouldn't be.**

A single RCA investigation is always scoped to exactly one target cluster —
`RemediationRequest.Spec.ClusterID`, already resolved into
`SignalContext.ClusterID` (`pkg/kubernautagent/types/types.go`) before the
investigator ever runs. There is never a "pick a cluster" decision to make
during a single investigation.

### What the code actually does today

`registerFleetTools()` connects to the MCP Gateway at KA startup and
registers exactly two **LLM-callable** tools into the shared, global tool
registry:

- `list_clusters` (`fleetclient.NewListClustersTool`)
- `list_tools_for_cluster` (`fleetclient.NewListToolsForClusterTool`)

Per ADR-068 decision #11's two-phase discovery design, the LLM is expected
to call `list_clusters` to browse, then `list_tools_for_cluster(cluster_id)`
to fetch and activate a specific cluster's tools — a `cluster_id` string
parameter the LLM supplies itself. Only after that round trip does a real
cluster resource tool (e.g. `{clusterID}__resources_get`) become callable.

ADR-068 decision #11 also describes an intended optimization: KA
"pre-scopes the MCP session to the alert's target cluster at investigation
start," so the LLM begins with that cluster's tools already active and
`list_clusters`/`list_tools_for_cluster` exist only for the rare
cross-cluster-correlation case. **This pre-scoping was never implemented.**
Confirmed by code search: `GatewayDiscoverer.ToolsForCluster()` — the exact
call needed to pre-scope — has exactly one production caller in the
repository: `list_tools_for_cluster`'s own `Execute()`
(`pkg/fleet/mcpclient/discovery_tools.go`). Nothing calls it automatically
using `signal.ClusterID`. In practice, every investigation — single-cluster
or not — depends on the LLM autonomously performing the two-step discovery
dance before it can read a single remote resource.

### Why this is a problem, not just an inefficiency

1. **Unnecessary LLM burden.** The LLM has to reason about fleet topology
   (which it was never asked to reason about) before it can do the RCA work
   it was actually asked to do.
2. **Needless prompt-injection / scope-escape surface.** `list_tools_for_cluster`
   takes a free-form `cluster_id` argument. A compromised or confused LLM
   session could, in principle, call it with a cluster ID other than the
   one it was asked to investigate — a capability that serves no business
   need, since exactly one cluster is ever in scope per investigation.
3. **Tool names leak cluster identity.** Even when the LLM correctly
   targets the right cluster, tool names like `remote_cluster__resources_get`
   are visibly different from the local-cluster equivalent
   (`resources_get`), so the LLM's behavior/prompting differs by deployment
   topology for no business reason — the MCP gateway already makes remote
   resources available exactly as if they were local; KA's tool exposure
   should reflect that.
4. **Untested.** Issue #1729 found no unit, integration, or E2E test that
   exercises an LLM tool-calling loop invoking a genuine fleet-discovered
   tool. The two-step discovery dance is unverified in addition to being
   undesirable.

## Alternatives Considered

### Alternative 1 — Full name transparency (server-side pre-scope + alias to local tool names) — ✅ CHOSEN

At investigation launch, KA uses `signal.ClusterID` to call
`ToolsForCluster()` server-side (no LLM involved), wraps the results as
`BridgeTool`s, and registers them **under the same generic names the local
K8s tools already use** (`resources_get`, not
`remote-cluster__resources_get`) for that investigation's tool set. Local
K8s tools are used unchanged when the signal has no `ClusterID` (hub-local).
`list_clusters` / `list_tools_for_cluster` are never added to the LLM-facing
RCA phase tool list.

**Pros**:
- ✅ Matches "same as if it were local" exactly — identical tool schema
  regardless of target cluster.
- ✅ Removes the prompt-injection/scope-escape surface (no `cluster_id`
  parameter exposed to the LLM at all).
- ✅ Finally implements ADR-068 decision #11's original pre-scoping intent.

**Cons**:
- ⚠️ Requires investigation-scoped tool resolution instead of the
  single global `*registry.Registry` + static `Investigator.phaseTools`
  used today (`internal/kubernautagent/investigator/investigator.go`) —
  moderate refactor of registry construction and `launchInvestigation`
  wiring (`internal/kubernautagent/session/manager.go`).
- ⚠️ Needs a name-collision/adapter layer so a cluster's `BridgeTool`s can
  be exposed under the same names as the local K8s tools without
  colliding when both exist in the same process.

### Alternative 2 — Server-side pre-scope, keep cluster-prefixed names — ❌ REJECTED

Same automatic server-side pre-scoping and same removal of
`list_clusters`/`list_tools_for_cluster` from the LLM's tool list, but skip
the renaming — the LLM would see `remote_cluster__resources_get` instead of
`resources_get`.

**Rejected because**: still satisfies "LLM never chooses a cluster," but
the tool *name* still reveals cluster identity, which does not fully match
the "same as if it were local" requirement. Smaller implementation
(no adapter layer) was not judged worth the incomplete transparency.

### Alternative 3 — Unify local/fleet tool code paths entirely (always via MCP, even for the hub cluster) — ❌ REJECTED

Collapse the local-K8s-client-backed tools and the MCP-gateway-backed
`BridgeTool`s into one code path, so a hub-local investigation is "just
another cluster" registration in the gateway.

**Rejected because**: out of proportion to the problem — every local K8s
tool (`pkg/kubernautagent/tools/k8s`, currently backed directly by
`client-go`/the dynamic client) would need to be rewritten to go through
the MCP client abstraction even when fleet mode is disabled entirely,
regressing every non-fleet KA deployment for a fleet-only concern.

## Decision

**Alternative 1**: KA pre-scopes the RCA phase's tool set server-side, at
investigation launch, using `signal.ClusterID`. Remote-cluster tools are
exposed to the LLM under the exact same names as the equivalent local K8s
tools (full name transparency). `list_clusters` and `list_tools_for_cluster`
are removed from the LLM-facing RCA tool list; `GatewayDiscoverer.ToolsForCluster()`
remains available for KA's own internal/server-side pre-scoping call (this
is what performs the automatic pre-scoping) but is never LLM-callable.

## Consequences

**Positive**:
- ✅ The LLM's tool-calling behavior is identical whether an incident is on
  the hub or on any fleet member cluster — no fleet-specific prompting or
  reasoning required.
- ✅ Closes an unnecessary scope-escape surface: the LLM can never request
  tools for a cluster other than the one it was asked to investigate.
- ✅ Finally realizes ADR-068 decision #11's original pre-scoping design.

**Negative**:
- ⚠️ Investigation-scoped tool resolution is a real refactor of KA's
  currently-global tool registry and static `Investigator.phaseTools` —
  **Mitigation**: scope the change to the fleet-target code path only;
  hub-local investigations keep using the existing global registry
  unchanged.
- ⚠️ Cross-cluster correlation (an LLM investigating a dependency on a
  *different* cluster than the incident's own) is no longer possible via
  an LLM-facing tool — **Mitigation**: this capability was never
  implemented as a real production caller of `list_tools_for_cluster`
  beyond the tool itself, and ADR-068 states it's needed in <1% of
  investigations. If a genuine need emerges, it should be a KA-internal,
  non-LLM-facing capability (mirroring how SP/WE/FMC already call
  `GatewayDiscoverer.ToolsForCluster()` programmatically today), not an
  LLM-driven cluster hop.

**Neutral**:
- 🔄 `GatewayDiscoverer` and `ToolsForCluster()` are unchanged as
  interfaces — only their caller changes (KA-internal pre-scoping instead
  of the `list_tools_for_cluster` tool's `Execute()`).

## As-Built Wiring

The design above was implemented as planned, with one deliberate deviation
(noted below) chosen for blast-radius reasons rather than any change in
intent.

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| Per-investigation tool pre-scoping | `Investigator.Investigate()` -> `prescopeFleetOverlay()` | `internal/kubernautagent/investigator/fleet_overlay.go` | IT-KA-FLEET-013 |
| `FleetOverlayResolver` (interface + context carrier) | `Investigator.Config.FleetOverlayResolver`, `New()` | `internal/kubernautagent/investigator/fleet_overlay.go` | IT-KA-FLEET-013 |
| Name-transparent `BridgeTool` aliasing | `gatewayOverlayResolver.Overlay()` / `genericNameTool` | `cmd/kubernautagent/toolregistry.go` | IT-KA-FLEET-011/012, E2E-KA-FLEET-001 |
| Overlay-vs-registry tool resolution | `toolDefinitionsForPhase()`, `executeResolved()` | `internal/kubernautagent/investigator/investigator_tools.go` | IT-KA-FLEET-015 |
| Removal of LLM-facing discovery tools | `registerFleetTools` (no longer takes a `*registry.Registry`) | `cmd/kubernautagent/toolregistry.go` | IT-KA-FLEET-010 |
| Alignment cluster attribution fix | `SubmitToolStep` -> `attributionClusterID()` | `internal/kubernautagent/alignment/toolproxy.go` | IT-KA-FLEET-016 |

**Deviation from the original proposal**: the design above named
`internal/kubernautagent/session/manager.go`'s `launchInvestigation` as the
pre-scoping entry point. `session.NewManager` has ~100 call sites using
positional construction, making a signature change there disruptive out of
proportion to this fix. Pre-scoping was wired one layer down instead, into
`Investigator.Config`/`New()` and `Investigate()` (both already using named
struct-literal construction at their call sites), which are exercised by
every investigation exactly the same way `launchInvestigation` would have
been. No behavioral difference results from this choice.

The implementation plan (Wiring Manifest with concrete IT test IDs) was
tracked in Issue #1732 per the project's Pre-Implementation Workflow.

## Amendment (2026-08-01, issue #1729 close-out): tool-transparency gap for non-colliding overlay names

Closing issue #1729 (wiring `kubernautAgent.fleet` through Helm) surfaced a
gap in the as-built `toolDefinitionsForPhase()`/`executeResolved()` overlay
resolution above: it only ever **overrides** a local-registry tool entry with
the overlay's `BridgeTool` when both share the exact same name. It never
**adds** an overlay tool with no local-registry namesake at all.

In practice this meant fleet-only tools were never advertised to the LLM at
all: `executeResolved()` could still route to them by name, but the LLM never
learns a name exists unless `toolDefinitionsForPhase()` puts it in the
schema. kube-mcp-server's own tool naming convention
(`resources_get`/`resources_list`/..., `pkg/fleet/mcpclient/tool_names.go`)
never collides with KA's local k8s-tool naming convention
(`kubectl_get_by_name`/`kubectl_list`/..., `pkg/kubernautagent/tools/k8s`),
so this gap was total, not partial — the "cluster-transparent tool exposure"
this decision is named for never actually reached kube-mcp-server's tools in
practice, regardless of Helm/gateway wiring.

**Fix**: `toolDefinitionsForPhase()` now appends every overlay tool name not
already covered by the override loop, for the RCA phase only (where the
local read/k8s tool set already lives — `WorkflowDiscovery`/`Validation`
schemas are deliberately left untouched, preserving their existing
least-privilege scoping, AC-6). Overlay-only names are sorted before
appending so the resulting schema is deterministic across calls despite Go's
randomized map iteration order.

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| Non-colliding overlay tool append | `toolDefinitionsForPhase()` -> `appendNonCollidingOverlayTools()` | `internal/kubernautagent/investigator/investigator_tools.go` | IT-KA-FLEET-024 |

## Authority

Issue #1729, Issue #1732, ADR-068 (decision #11), BR-INTEGRATION-054,
BR-INTEGRATION-1489.
