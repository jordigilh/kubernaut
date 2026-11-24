# Test Plan: fleet-mode AF deployments render `fleet.fmc.endpoint`

**Test Plan Identifier**: TP-2695-v1.1
**Feature**: Restore the Console inventory lane for fleet-mode APIFrontend E2E deployments
**Version**: 1.1
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
(`buildAPIFrontendFleetConfig`) wrote the whole `fleet` block without that key, so every
fleet-mode AF lane silently lost Console inventory.

FMC's SAR for `get services/fleetmetadatacache-service` is already satisfied by AF's base
ClusterRole (`test/fixtures/kubernetes/apifrontend/base/02-rbac.yaml`, core `services`
`get,list` rule), so the missing config key was the only defect on this lane.

### 1.2 Authority and references

- BR-FLEET-054: fleet cluster-scoped access and correlation
- BR-INTEGRATION-065: multi-cluster federation scope resolution
- ADR-068: Fleet Federation Architecture
- DD-FLEET-017: Fleet inventory projection (D2.1a)
- DD-TEST-019: Minimal Fleet-Enabled APIFrontend E2E Topology
- DD-AUTH-014: middleware-based SAR authentication

## 2. Preflight findings

- `buildAPIFrontendFleetConfig` (`test/infrastructure/apifrontend_e2e.go`) wrote the rendered
  `fleet` block wholesale and never set `fleet.fmc.endpoint`.
- The base AF ClusterRole already grants `get,list` on core `services`
  (`test/fixtures/kubernetes/apifrontend/base/02-rbac.yaml`), which authorizes FMC's
  `get services/fleetmetadatacache-service` SAR; no new grant is needed.
- The only in-repo fleet-mode AF config producer is the hub-only renderer. The operator CR path
  (fullpipeline-fleet, demo) and the external Helm chart need an operator CRD/chart change; that
  half is a named follow-up, not fixed here.
- No in-repo Helm chart exists (`KUBERNAUT_OPERATOR_CHART_PATH` is an external checkout).

## 3. Scope and accepted risk

In scope: hub-only AF E2E renderer and its UT/IT proof.

Accepted risk: this plan proves configuration rendering, not a live 200-vs-503 Console inventory
response. The issue's fix list stops at render/prove/sweep; AF readiness is unaffected — no FMC
prober is added to `/readyz`, and the Fleet gate stays MCP/registry. The live FMC inventory path is
covered separately by the FMC lane's inventory E2E.

## 4. Approach (TDD)

1. RED: extend `UT-INFRA-AF-FLEET-2462-004` and add `IT-INFRA-AF-FLEET-2695-002`; confirm they
   fail only for the missing key.
2. GREEN: render `fleet.fmc.endpoint` from the resolved FMC namespace.
3. REFACTOR: hoist the FMC service URL into one value; document the replace-not-merge config
   contract.
4. Validation: `go build ./...`, `golangci-lint run --timeout=5m`, focused `-race` runs.

## 5. Test scenarios

| ID | Tier | Contract |
|---|---|---|
| UT-INFRA-AF-FLEET-2462-004 | Unit | Rendered `fleet.endpoint` and `fleet.fmc.endpoint` both equal the FMC service URL, derived from `FleetMetadataCacheNamespace` (distinct from the app namespace) |
| IT-INFRA-AF-FLEET-2695-002 | Integration | The ConfigMap rendered by `deployAPIFrontendService` carries `fleet.fmc.endpoint` equal to the FMC service URL |
| UT-OPCR-FLEET-2695-001 | Unit | The Kubernaut CR rendered by `BuildKubernautCRManifest` carries `fleet.fmc.endpoint`/`fleet.fmc.namespace` from `OperatorFleetOptions` (operator CR path: fullpipeline-fleet, demo) |
| UT-OPCR-FLEET-2695-002 | Unit | `operatorFleetOptionsFromHelm` derives `FMCEndpoint`/`FMCNamespace` from `FleetHelmOptions.FleetMetadataCacheNamespace` (no empty fallback per #2298) |

Key independence (`fmc.endpoint` vs `endpoint`) is proven by `UT-AF-CFG-2661-001` and
`UT-FLEET-CFG-029`, not by the renderer test.

## 6. Wiring manifest

| Component | Production entry point | Wiring location | Proving tests |
|---|---|---|---|
| `fleet.fmc.endpoint` value | `deployAPIFrontendService` → rendered ConfigMap → AF loader → `fleet.NewInventoryReader` | `buildAPIFrontendFleetConfig` | UT-INFRA-AF-FLEET-2462-004, IT-INFRA-AF-FLEET-2695-002 |
| `spec.fleet.fmc` CR block | `BuildKubernautCRManifest` → Kubernaut CR → external kubernaut-operator → AF config | `applyOperatorCRFeatureOverrides`, `operatorFleetOptionsFromHelm` (callers: `demo_operator.go`, `fullpipeline_e2e.go`) | UT-OPCR-FLEET-2695-001, UT-OPCR-FLEET-2695-002 |

## 7. Control mapping

| Control | How this plan proves it |
|---|---|
| FedRAMP CM-6 | The rendered AF configuration carries the required `fleet.fmc.endpoint` setting |
| FedRAMP AC-4 | Console inventory is reachable only through the vetted FMC endpoint |

## 8. Follow-up (not in this change)

- External `kubernaut-operator`: consume the new `spec.fleet.fmc` CR block (endpoint,
  namespace) and render it into AF's `fleet.fmc.endpoint`; verify operator-managed AF's RBAC
  still covers `get services/fleetmetadatacache-service`. Until this lands, the CR key is
  carried but inert.

## 9. Rollback

Revert the renderer, test, and doc changes together. No compatibility mode or data migration is
involved.
