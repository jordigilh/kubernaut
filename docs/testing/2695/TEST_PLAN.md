# Test Plan: fleet-mode AF deployments render `fleet.fmc.endpoint`

**Test Plan Identifier**: TP-2695-v1.0
**Feature**: Restore the Console inventory lane for fleet-mode APIFrontend E2E deployments
**Version**: 1.0
**Created**: 2026-11-23
**Author**: Kubernaut development team
**Status**: Approved
**Branch**: `work/2695-af-fmc-endpoint`
**Issue**: #2695 (carved out of #2661 phase c)

---

## 1. Introduction

### 1.1 Purpose

After #2661 phase c, `fleet.NewInventoryReader` builds the Console inventory reader only from
`fleet.fmc.endpoint` (DD-FLEET-017 D2.1a); an absent key leaves the reader nil and fails every
Console inventory read closed (`unavailable`, 503). The hub-only fleet-mode AF E2E renderer
(`buildAPIFrontendFleetConfig`) wrote the whole `fleet` block without that key, and AF's
ServiceAccount lacked the SAR grant FMC requires for every API request. Result: every
fleet-mode AF lane silently lost Console inventory.

This plan closes both gaps in the hub-only lane and records the operator/chart remainder.

### 1.2 Authority and references

- BR-FLEET-054: fleet cluster-scoped access and correlation
- BR-INTEGRATION-065: multi-cluster federation scope resolution
- BR-RBAC-020: scoped least-privilege registry access
- ADR-068: Fleet Federation Architecture
- DD-FLEET-017: Fleet inventory projection (D2.1a)
- DD-TEST-019: Minimal Fleet-Enabled APIFrontend E2E Topology
- DD-AUTH-014: middleware-based SAR authentication

## 2. Preflight findings

- `buildAPIFrontendFleetConfig` (`test/infrastructure/apifrontend_e2e.go`) replaced the fixture's
  `fleet` block wholesale and never set `fleet.fmc.endpoint`.
- FMC wraps every API route in SAR auth for `get services/fleetmetadatacache-service`
  (`cmd/fleetmetadatacache/main.go`); AF's SA had no grant, so rendering the key alone would have
  moved the failure from 503 to 403.
- The only in-repo fleet-mode AF config producer is the hub-only renderer. The operator CR path
  (fullpipeline-fleet, demo) and the external Helm chart need an operator CRD/chart change plus
  the same RBAC; that half is a named follow-up, not fixed here.
- No in-repo Helm chart exists (`KUBERNAUT_OPERATOR_CHART_PATH` is an external checkout).

## 3. Scope and accepted risk

In scope: hub-only AF E2E renderer, its Fleet RBAC manifest, and the UT/IT proof.

Accepted risk: this plan proves configuration rendering and the least-privilege grant, not a live
200-vs-503 Console inventory response. The issue's fix list stops at render/prove/sweep; AF
readiness is unaffected (its probers are MCP/registry only). The live FMC inventory path is
covered separately by the FMC lane's inventory E2E.

## 4. Approach (TDD)

1. RED: extend `UT-INFRA-AF-FLEET-2462-004` / `-006`, add `IT-INFRA-AF-FLEET-2695-002` / `-003`;
   confirm each fails only for the missing key or grant.
2. GREEN: render `fleet.fmc.endpoint`; append the `apifrontend-fmc-sar-client`
   ClusterRole/ClusterRoleBinding.
3. REFACTOR: hoist the FMC service URL into one value; document the replace-not-merge config
   contract and the FMC SAR verb.
4. Validation: `go build ./...`, `golangci-lint run --timeout=5m`, focused `-race` runs.

## 5. Test scenarios

| ID | Tier | Contract |
|---|---|---|
| UT-INFRA-AF-FLEET-2462-004 | Unit | Rendered `fleet.endpoint` and `fleet.fmc.endpoint` both equal the FMC service URL, derived from `FleetMetadataCacheNamespace` (distinct from the app namespace) |
| UT-INFRA-AF-FLEET-2462-006 | Unit | Fleet RBAC manifest grants only the two registry rules (`backends`, `mcproutes`; `get,list,watch`) and the FMC SAR rule (`services/fleetmetadatacache-service`; `get`), bound to the AF SA |
| IT-INFRA-AF-FLEET-2695-002 | Integration | The ConfigMap rendered by `deployAPIFrontendService` carries `fleet.fmc.endpoint` equal to the FMC service URL |
| IT-INFRA-AF-FLEET-2695-003 | Integration | The manifest rendered by `afDeployFleetRBAC` carries the `apifrontend-fmc-sar-client` ClusterRole and binding |

Key independence (`fmc.endpoint` vs `endpoint`) is proven by `UT-AF-CFG-2661-001` and
`UT-FLEET-CFG-029`, not by the renderer test.

## 6. Wiring manifest

| Component | Production entry point | Wiring location | Proving tests |
|---|---|---|---|
| `fleet.fmc.endpoint` value | `deployAPIFrontendService` → rendered ConfigMap → AF loader → `fleet.NewInventoryReader` | `buildAPIFrontendFleetConfig` | UT-INFRA-AF-FLEET-2462-004, IT-INFRA-AF-FLEET-2695-002 |
| AF→FMC SAR grant | `afDeployFleetRBAC` → rendered ClusterRole/Binding | `buildAFleetRBACManifest` | IT-INFRA-AF-FLEET-2695-003, UT-INFRA-AF-FLEET-2462-006 |

## 7. Control mapping

| Control | How this plan proves it |
|---|---|
| FedRAMP CM-6 | The deployed AF configuration carries the required `fleet.fmc.endpoint` setting |
| FedRAMP AC-4 | Console inventory is reachable only through the vetted FMC endpoint |
| FedRAMP AC-6 / SOC2 CC6.1 / ASVS V4.1 | The grant is limited to `get` on the named FMC Service |

## 8. Follow-up (not in this change)

- In-repo operator CR path: add the FMC endpoint/namespace to `OperatorFleetOptions`,
  `operatorFleetSpec`, `applyOperatorCRFeatureOverrides`, and `operatorFleetOptionsFromHelm`.
- External `kubernaut-operator`: expose the CRD/chart key that renders AF `fleet.fmc.endpoint`,
  and grant AF `get services/fleetmetadatacache-service` for operator-managed deployments.

## 9. Rollback

Revert the renderer, RBAC manifest, and test/doc changes together. No compatibility mode or data
migration is involved.
