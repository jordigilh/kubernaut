# Test Plan: Real AF-via-A2A Fleet Investigation E2E Coverage

**Issue**: [#1768](https://github.com/jordigilh/kubernaut/issues/1768) (Track 1 — Gaps A + C)
**Authority**: [DD-FLEET-004](../../architecture/decisions/DD-FLEET-004-cluster-transparent-tool-exposure.md), ADR-068
**Business Requirements**: BR-INTEGRATION-054, BR-FLEET-054
**Branch**: `test/1768-af-fleet-e2e-coverage`
**Created**: 2026-07-29
**Status**: Active

---

## 1. Purpose

Issue #1768 identified that no test in the repository drives the real APIFrontend (AF)
binary, over the real A2A protocol, against the real Fleet E2E infrastructure (real second
Kind cluster, real Kuadrant MCP gateway, real kube-mcp-server, real OAuth2). Three existing
suites each cover one leg of this triangle but never all three together:

- `test/e2e/fleet/*` has the real multi-cluster infra but every "AF" test
  (`06_aa_fleet_investigation_test.go`, `05_af_preflight_oauth2_test.go`) bypasses the AF
  binary and talks to the MCP gateway with a raw client (`newFleetMCPClient`).
- `test/e2e/fullpipeline/10_af_fleet_cluster_id_test.go` drives the real AF binary via real
  A2A, but FullPipeline has no second cluster, no gateway, and no fleet config — the
  `cluster_id` it sends is a synthetic string proving RRContext string-plumbing, not a real
  cross-cluster read.
- `test/integration/apifrontend/fleet/af_fleet_routing_test.go` proves `NewKubectlGetTool`
  wiring against a **mock** `ResourceReaderFactory`, never AF's real `FleetReaderFactory`.

Preflight confirmed (2026-07-29, against `origin/main`@`459cf3f42`) that the Fleet E2E
infrastructure already deploys AF with fleet reader wiring fully enabled
(`patchAPIFrontendConfigForFleet`, `test/infrastructure/fleet_e2e.go:848`) and even
pre-declares an unused HTTP client for it (`afBaseURL`/`afHTTPClient`,
`test/e2e/fleet/suite_test.go:135-136,541-547`) — the infra was already prepared for this
test, it was simply never written. This closes the gap with the smallest possible surface:
one new E2E test file plus one new Mock LLM scenario, no infra or production code changes.

## 2. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **SI-4** | Information System Monitoring | Primary. Proves AF's real fleet-investigation tool-calling path (the same path an SRE's live A2A session uses) can actually read remote-cluster state, not just that the underlying gateway/MCP plumbing works in isolation. |
| **AC-4** | Information Flow Enforcement | The test asserts the resource returned is the **remote** cluster's `coredns`, not the hub's — proving the `cluster_id` argument enforces the intended cross-cluster boundary through the real AF binary, matching DD-FLEET-004's per-investigation cluster-scoping guarantee for AF's tool surface. |
| **AU-2/AU-3** | Audit Events / Content | AF's tool-call audit event (existing `audit.Emitter` path, unchanged) is exercised end-to-end for a real fleet-scoped `kubectl_get`, closing a previously-untested audit-completeness gap for cross-cluster reads specifically. |

## 3. Pyramid Invariant — Test Scenario Inventory

The wiring being proven (AF binary → ADK agent loop → `kubectl_get`/`FleetReaderFactory` →
kube-mcp-server → Kuadrant gateway → remote cluster) is 100% pre-existing production code
(`cmd/apifrontend/backend_deps.go:474`, `pkg/apifrontend/tools/kubectl_get.go:115`) with
existing UT (tool-level) and IT (mock-factory) coverage. This plan adds the missing E2E tier
only — no new UT/IT is needed because no production logic is changing, only test coverage.

| ID | Tier | Business-Level Behavior Description | Control | BR | Test File |
|---|---|---|---|---|---|
| E2E-FLEET-016-001 | E2E | A real AF binary, invoked via a real `message/send` A2A call (mock-LLM-driven), calls its own `kubectl_get` tool with `cluster_id="remote-cluster"` and returns the **remote** cluster's `coredns` Deployment (not the hub's) | SI-4, AC-4 | BR-INTEGRATION-054, BR-FLEET-054 | `test/e2e/fleet/16_af_real_fleet_investigation_test.go` |
| E2E-FLEET-016-002 | E2E | The same real-A2A call path with `list_clusters` returns `remote-cluster` in the fleet roster, proving AF's `ClusterRegistry`-backed discovery tool is reachable end-to-end through a live AF binary (not just via the raw MCP client `newFleetMCPClient` uses elsewhere in this suite) | SI-4 | BR-FLEET-054 | `test/e2e/fleet/16_af_real_fleet_investigation_test.go` |

## 4. Why the Existing Suite Missed This (Coverage Gap Being Closed)

- `test/e2e/fleet/suite_test.go` already sets up `afBaseURL`/`afHTTPClient` (NodePort 30443,
  self-signed TLS) at `SynchronizedBeforeSuite` time, but **zero** test file in the `fleet`
  package references either variable — confirmed via `grep -rn afBaseURL\|afHTTPClient
  test/e2e/fleet/`. The harness plumbing was added but never consumed.
- `05_af_preflight_oauth2_test.go` is the only fleet-suite file with "af" in its name that
  touches OAuth2/Keycloak, but it authenticates as `kube-mcp-server`'s own OAuth2 client
  directly against the gateway — it never constructs an AF A2A client at all.
- `10_af_fleet_cluster_id_test.go` (FullPipeline) is the closest existing precedent for
  driving AF via real A2A (`fpA2ATasksSend`/`fpA2AInvokeWithTimeout`,
  `test/e2e/fullpipeline/af_helpers_test.go:52,154`), but that suite's Kind cluster is single
  and has no gateway/second-cluster infra to route a fleet tool call to.

This plan closes the gap by porting the FullPipeline A2A-driving pattern into the fleet
package (which already has the real infra the FullPipeline pattern lacks), rather than
inventing a new mechanism.

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | E2E Test ID |
|---|---|---|---|
| AF `/a2a/invoke` handler | `cmd/apifrontend/main.go` router | `pkg/apifrontend/handler/router.go` | E2E-FLEET-016-001/002 (pre-existing; newly exercised in this suite) |
| `kubectl_get`/`list_clusters` tool registration | AF agent startup | `pkg/apifrontend/agent/root.go:150-164` | E2E-FLEET-016-001/002 |
| `FleetReaderFactory` (AF→gateway→remote cluster) | AF startup | `cmd/apifrontend/backend_deps.go:474` (`buildFleetReaderDeps`) | E2E-FLEET-016-001 |
| Mock LLM fleet-kubectl scenario | `test/services/mock-llm` scenario registry | `test/services/mock-llm/scenarios/registry_default.go` (new entry) | E2E-FLEET-016-001/002 |

No new production components are introduced — this Wiring Manifest documents pre-existing
wiring that had no E2E test exercising it, per Gap A/C.

## 6. Test Data / Fixtures

- Target resource: `coredns` Deployment in `kube-system` on the remote cluster (matches the
  existing `06_aa_fleet_investigation_test.go` precedent for a resource guaranteed present on
  any Kind cluster).
- Cluster identity: `"remote-cluster"` (matches the DD-TEST-013 identity already used by
  `13_cluster_scoped_workflow_targeting_test.go`, `03_ro_clusterid_routing_test.go`).
- Auth: Dex `sre@kubernaut.ai` password-grant token (same mechanism as
  `test/e2e/fullpipeline/suite_test.go:getAFToken`) — Dex is deployed in the fleet suite too,
  inherited from `SetupFullPipelineInfrastructure` (confirmed via
  `test/infrastructure/fullpipeline_e2e.go:1199`); the fleet-suite-only Keycloak realm
  (`kubernaut-fleet`) is a separate, additive IdP for MCP-gateway service-to-service auth and
  is not touched by this plan.

## 7. Out of Scope (tracked separately)

- Gap D (AF↔KA interactive bridge fleet-scoping) — see `docs/spikes/1768-af-ka-interactive-fleet-scoping/README.md` and issue #1768 Track 2.
- `list_clusters` vs DD-FLEET-004 asymmetry design question — issue #1768 Track 3 (CHECKPOINT DD, pending user decision).
- Issue #1729 (KA Helm chart fleet parity) — separate issue, unrelated to AF's own fleet wiring.

## 8. Coverage Target

E2E tier only (per Testing Requirements: E2E targets 100% of SOC2/FedRAMP control objectives
in scope, not line %). Both control objectives in Section 2 (SI-4, AC-4) get at least one
proving journey via E2E-FLEET-016-001. AU-2/AU-3 audit-event assertion is a secondary
assertion within the same test (no dedicated test needed — the existing audit emission path
is unchanged production code).
