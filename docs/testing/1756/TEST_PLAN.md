# Test Plan: Fleet Tool Overlay Gateway-Agnostic Prefix Resolution

**Issue**: #1756
**Authority**: [DD-FLEET-005](../../architecture/decisions/DD-FLEET-005-cluster-transparent-tool-exposure.md), [ADR-068](../../architecture/decisions/ADR-068-fleet-federation-architecture.md) decision #10/#11
**Business Requirements**: BR-INTEGRATION-054, BR-INTEGRATION-1489
**Branch**: `fix/1756-kuadrant-tool-prefix`
**Created**: 2026-07-28
**Status**: Active

---

## 1. Purpose

`gatewayOverlayResolver.Overlay()` (`cmd/kubernautagent/toolregistry.go`) computes the
LLM-facing "generic" tool name by unconditionally stripping the EAIGW convention
(`"{clusterID}__"`). For Kuadrant, whose `MCPServerRegistration.spec.prefix` is a free-text,
admin-set value that does not follow that convention (e.g. `prod-east` → `prod_east_`), the
strip is a no-op: the remote tool is never offered to the LLM under its generic name, and the
LLM silently falls back to the hub-local tool of the same name — an incorrect-cluster data
correctness defect for RCA, not merely a missed optimization.

This is a business acceptance criterion failure, not just a code defect: DD-FLEET-005 commits
to "the LLM's tool schema for a fleet-target investigation is byte-identical to a hub-local
investigation's" and "the LLM can never request tools for a cluster other than the one it was
asked to investigate." Every test below asserts one of these two business-level outcomes
directly (map key identity, wire wire-name identity) — not the internal string-manipulation
mechanism — so the tests remain valid even if the extraction algorithm changes later.

## 2. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **AC-4** | Information Flow Enforcement | Primary control. The defect is a cross-cluster information-flow-enforcement failure: an investigation's tool calls must be enforced to the intended target-cluster boundary; on failure they must error, never silently resolve against a different (hub) boundary. Directly analogous to `E2E-FMC-054-015`'s cross-cluster isolation assertion in `docs/testing/BR-INTEGRATION-054/TEST_PLAN.md`. |
| **AC-6** | Least Privilege | Secondary. DD-FLEET-005's own stated guarantee ("the LLM can never request tools for a cluster other than the one it was asked to investigate") is the least-privilege property this defect silently violates by substituting wrong-cluster data instead of failing closed. |

## 3. Pyramid Invariant — Test Scenario Inventory

Per the pyramid invariant, no single tier is sufficient: UT proves the extraction logic is
correct in isolation, the same-package UT proves the *actual* production `Overlay()` method
(not a re-derivation, which is exactly how #1756 went undetected) is wired to that logic
correctly for both gateway conventions, IT proves it against a real MCP protocol round trip,
and the journey test proves the full `Investigator.Investigate()` path end-to-end.

| ID | Tier | Business-Level Behavior Description | Control | BR | Test File |
|---|---|---|---|---|---|
| UT-MCP-TN-101..106 | UT | `PrefixFromToolNames` derives the correct gateway-specific wire prefix from a list of discovered tool names for both EAIGW (`{clusterID}__`) and Kuadrant (admin-set, non-`{clusterID}__`) conventions, and errors (not panics/guesses) when no tool name matches the cluster | AC-4 | BR-INTEGRATION-054 | `pkg/fleet/mcpclient/discover_test.go` |
| UT-KA-FLEET-024/025 | UT | The real, unexported `gatewayOverlayResolver.Overlay()` generic-izes a cluster's discovered tool name identically regardless of gateway convention: the LLM sees the tool under the same generic key (`resources_get`) whether the wire name is `cluster-a__resources_get` (EAIGW) or `prod_east_resources_get` (Kuadrant), and the wire call still reaches the gateway under the tool's original prefixed name | AC-4 | BR-INTEGRATION-054, BR-INTEGRATION-1489 | `cmd/kubernautagent/toolregistry_kuadrant_prefix_test.go` |
| IT-KA-FLEET-010 (revised) | IT | The exported `fleetclient.PrefixFromToolNames` helper correctly derives a Kuadrant cluster's wire prefix from tool names discovered via a **real** two-phase `discover_tools`/`select_tools`/`ListTools` MCP protocol round trip against the mock gateway (no hardcoded literal prefix duplicated in test code) | AC-4 | BR-INTEGRATION-054 | `test/integration/kubernautagent/fleet/fleet_wiring_test.go` |
| E2E-KA-FLEET-002 (new) | "E2E" (journey, per this repo's existing DD-FLEET-005 labeling of `fleet_e2e_journey_test.go`) | KA's full `Investigator.Investigate()` -> LLM -> gateway journey resolves and executes a remote-cluster tool correctly when the fleet gateway is Kuadrant (non-bare prefix convention), matching the existing EAIGW-only `E2E-KA-FLEET-001` journey | AC-4 | BR-INTEGRATION-054, BR-INTEGRATION-1489 | `test/integration/kubernautagent/investigator/fleet_e2e_journey_test.go` |

## 4. Why the Existing Suite Missed This (Coverage Gap Being Closed)

- `E2E-KA-FLEET-001` is the only test driving KA's real `Investigator.Investigate()` through
  the fleet overlay end-to-end, but it only exercises EAIGW, where the buggy formula is
  correct by construction.
- `IT-KA-FLEET-010` configured a non-bare Kuadrant prefix but re-derived the trim step with
  the literal correct answer (`strings.TrimPrefix(def.Name, "prod_east_")`) instead of calling
  the production formula, so it could never detect the mismatch.
- No test previously called the real `gatewayOverlayResolver.Overlay()` with a non-`{clusterID}__`
  prefix at all.

This plan closes all three gaps: `UT-KA-FLEET-024/025` calls the real production method
directly; `IT-KA-FLEET-010` is fixed to call the real exported helper against real protocol
traffic instead of a hardcoded literal; `E2E-KA-FLEET-002` extends the one real journey test
to a Kuadrant scenario.

### 4.1 A fourth, orthogonal gap: `cmd/*` unit suites were never executed by CI

While adding `UT-KA-FLEET-024/025` (a same-package unit test calling the real, unexported
`gatewayOverlayResolver.Overlay()` directly), discovered that `make test-unit-kubernautagent`
listed only `./pkg/kubernautagent/... ./internal/kubernautagent/...` — `./cmd/kubernautagent/...`
was never in any make target's package list, so its entire 20+ file Ginkgo suite (96 specs,
including the pre-existing `toolregistry_test.go`, `toolregistry_overlay_singleflight_test.go`,
etc.) had **zero** make target or CI job executing it, ever. `.github/workflows/ci-pipeline.yml`'s
unit-test matrix job runs exactly `make test-unit-${{ matrix.service }}` per service, so this was
a silent, repo-wide CI blind spot, not just a kubernautagent-specific one: the same audit found
5 more services (`gateway`, `fleetmetadatacache`, `datastorage`, `apifrontend` via the generic
`test-unit-%` pattern rule, plus `authwebhook` with no `cmd/` tests yet) with a `cmd/<service>`
Ginkgo/`testing.T` suite that was similarly never run. Fixed in the `Makefile`: the generic
`test-unit-%` pattern rule and all 5 dedicated per-service targets now conditionally include
`./cmd/$*/...` (via `$(wildcard cmd/$*)`, so non-service ad hoc invocations are unaffected), and
all 12 CI-matrix services were re-run locally via their `make test-unit-*` target to confirm zero
regressions. Without this fix, `UT-KA-FLEET-024/025` above would have caught the bug locally but
never run in CI at all.

## 5. Coverage Targets

| Tier | Target |
|---|---|
| Unit | 100% of `PrefixFromToolNames` branches (match, no-match, empty input) |
| Integration | 100% wiring proof for this fix (`IT-KA-FLEET-010` uses the real exported helper against real MCP protocol traffic) |
| Journey | Both gateway conventions (EAIGW `E2E-KA-FLEET-001`, Kuadrant `E2E-KA-FLEET-002`) exercise the identical `Investigator.Investigate()` production path |

## 6. References

- Issue #1756
- [DD-FLEET-005: Cluster-Transparent Tool Exposure](../../architecture/decisions/DD-FLEET-005-cluster-transparent-tool-exposure.md)
- [ADR-068: Fleet Federation Architecture](../../architecture/decisions/ADR-068-fleet-federation-architecture.md)
- [BR-INTEGRATION-054 Test Plan](../BR-INTEGRATION-054/TEST_PLAN.md) (AC-4 precedent: `E2E-FMC-054-015` cross-cluster isolation)
