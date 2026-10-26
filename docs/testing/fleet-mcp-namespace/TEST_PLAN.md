# Test Plan: Fleet MCP Infrastructure Namespace Parity

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2476-v1.0
**Feature**: Keep fleet MCP infrastructure outside `kubernaut-system`, matching production namespace boundaries
**Version**: 1.0
**Created**: 2026-09-30
**Author**: Kubernaut development agent
**Status**: Approved
**Issue**: #2476

## 1. Purpose and objectives

The fleet qualification path must provision the MCP Gateway and hub
`kube-mcp-server` before the operator reconciles the Kubernaut CR, while keeping those platform components in the
dedicated `mcp-system` namespace used by the remote-cluster setup. This plan
proves namespace propagation, prerequisite replication, operator CR wiring, and
the existing pre-reconciliation ordering contract without requiring a live cluster for every
regression test.

Success requires:

1. Hub MCP resources and Gateway registration resources use `mcp-system`.
2. The hub MCP namespace has its CA ConfigMap and Keycloak DNS alias before the
   server is deployed.
3. The operator Fleet CR carries `spec.fleet.mcpGateway.namespace=mcp-system` and
   watches the same namespace for Gateway-managed CRs.
4. Existing remote-only and same-namespace FMC callers remain compatible.

## 2. Authority and scope

- **BR-PLATFORM-014** — stable, production-parity fleet demo infrastructure.
- **ADR-068** — MCP Gateway is external infrastructure deployed before Kubernaut.
- **DD-TEST-015** — provision fleet infrastructure before the operator reconciles the application CR.
- `test/infrastructure/fleet_e2e.go`
- `test/infrastructure/fleetmetadatacache_remote_cluster.go`
- `test/infrastructure/fleet_e2e.go`

Out of scope: changing production operator resources or moving Kubernaut workloads
out of `kubernaut-system`; live Kind execution is a follow-up validation gate.

## 3. Risks and mitigations

| ID | Risk | Mitigation |
|---|---|---|
| R1 | Hub server remains coupled to the application namespace. | Assert the resolved MCP namespace and generated Gateway/registration namespace. |
| R2 | The server starts without its CA or Keycloak alias. | Exercise namespace prerequisite provisioning and manifest assertions. |
| R3 | FMC watches the old namespace after registration resources move. | Assert `spec.fleet.mcpGateway.namespace` and the generated FMC namespace wiring. |
| R4 | Existing same-namespace callers regress. | Preserve the resolver fallback and run the focused infrastructure suite. |

## 4. TDD scenarios and wiring manifest

| ID | Tier | Business outcome | Phase |
|---|---|---|---|
| `UT-2492-CR-STATIC-001` | Unit | The operator Fleet CR includes the dedicated MCP namespace when configured. | GREEN |
| `UT-INFRA-FLEET-MCP-002` | Unit | Empty MCP namespace preserves the caller's existing namespace. | GREEN |
| `UT-INFRA-FLEET-MCP-003` | Unit | Explicit MCP namespace overrides the application namespace. | GREEN |
| `UT-INFRA-FLEET-MCP-004` | Unit | Remote MCP namespace has an independent default and override. | GREEN |
| `IT-INFRA-FLEET-MCP-005` | Integration | EAIGW registration manifests target the dedicated MCP namespace and server DNS. | GREEN |
| `IT-INFRA-FLEET-MCP-006` | Integration | Kuadrant registration manifests target the dedicated MCP namespace. | GREEN |
| `UT-INFRA-FLEET-MCP-007` | Unit | Custom Kuadrant rendering rewrites namespace references consistently. | GREEN |
| `E2E-INFRA-FLEET-MCP-008` | E2E | MCP/Gateway becomes ready before the operator reconciles the Kubernaut CR. | Follow-up |

| Component | Production entry point | Wiring location | Proving test |
|---|---|---|---|
| MCP namespace resolution | Fleet demo and fleet E2E setup | `provisionFleetCoreInfra` | `UT-INFRA-FLEET-MCP-002/003/004` |
| Hub MCP/Gateway resources | `DeployFleetGatewayInfra` | `fleet_e2e.go` | `IT-INFRA-FLEET-MCP-005/006` |
| Operator Fleet namespace wiring | `SetupFleetE2EInfrastructure` | Fleet CR and gateway namespace builders | `UT-2492-CR-STATIC-001` |

## 5. Verification

Focused checks:

```bash
go test ./test/infrastructure -run 'TestInfrastructure' -ginkgo.focus='Fleet MCP namespace'
  go test ./test/infrastructure -run 'TestInfrastructure' -ginkgo.focus='Fleet static operator contract|UT-INFRA-FLEET-MCP'
```

Completion checks:

```bash
gofmt -w test/infrastructure/fleet_e2e.go test/infrastructure/fleet_e2e_credentials_test.go
go build ./...
go test ./test/infrastructure -run '^TestInfrastructure$'
```

The live E2E scenario remains pending until an isolated Kind run is available;
it must verify the `mcp-system` Deployment/Service and readiness before the
operator reconciles Kubernaut workloads in `kubernaut-system`.
