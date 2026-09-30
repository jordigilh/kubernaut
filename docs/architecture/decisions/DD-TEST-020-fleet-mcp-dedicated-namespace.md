# DD-TEST-020: Fleet Demo MCP Dedicated Namespace

**Status**: ✅ Approved & Implemented
**Date**: 2026-09-30
**Author**: Kubernaut development team
**Related**: Issue #2476, ADR-068, DD-TEST-013, DD-TEST-015, BR-PLATFORM-014

## Context

Production deployments treat the MCP Gateway and its Kubernetes API MCP server
as platform infrastructure, separate from the Kubernaut application
workloads. The fleet demo previously placed the hub `kube-mcp-server` and its
Gateway registration resources in `kubernaut-system`, while the remote server
already used `mcp-system`. That topology hid namespace-scoping and
cross-namespace prerequisite errors that occur in production.

The fleet demo also has a strict ordering contract: MCP infrastructure must be
ready before the Kubernaut Helm release renders fleet-enabled workloads
(DD-TEST-015).

## Decision

1. Keep Kubernaut workloads in `kubernaut-system`.
2. Put the hub MCP Gateway/server and Gateway-managed registrations in a
   separately configured namespace, defaulting to `mcp-system`.
3. Configure the remote cluster's MCP server namespace independently,
   defaulting to `mcp-system`; it is not implicitly coupled to the hub
   namespace.
4. Before deploying the hub MCP server, create the selected namespace,
   replicate the inter-service CA ConfigMap into it, and create a namespace-
   local `keycloak` alias when the IdP is elsewhere.
5. Pass the selected hub namespace to Helm as
   `global.fleet.mcpGatewayNamespace`, and use it for SignalProcessing and
   FleetMetadataCache Gateway CR watches.
6. Preserve the existing provisioning order: Gateway, MCP server,
   registrations, readiness checks, and prerequisite Secrets complete before
   the single Kubernaut Helm installation.

The EAIGW path places its Gateway resource in the configured namespace. The
Kuadrant v0.7.1 overlay used by the Kind harness is rendered and rewritten at
apply time when a non-default namespace is selected, avoiding a fork of the
upstream manifests while keeping the controller, broker, RBAC subject, and
namespace resource aligned.

## Alternatives considered

1. **Keep all MCP resources in `kubernaut-system`** — rejected because it does
   not exercise the production namespace boundary and allows namespace-local
   DNS/CA assumptions to pass incorrectly.
2. **Move only the hub server and keep registrations in the application
   namespace** — rejected because Gateway-managed CRs, bridge Services, and
   backend DNS would remain coupled to the wrong control-plane namespace.
3. **Use one shared namespace option for hub and remote clusters** — rejected
   because the clusters are independent Kubernetes control planes and their
   namespace lifecycle must be independently configurable.

## Consequences

### Positive

- The demo mirrors the production separation between application and MCP
  platform infrastructure.
- Helm, FMC, and SignalProcessing watch the same namespace where Gateway CRs
  are actually created.
- CA and Keycloak DNS prerequisites are explicit and available before the MCP
  server starts.
- The remote-cluster-only topology remains compatible while gaining an
  independently configurable MCP namespace.

### Negative / risks

- The setup creates and maintains namespace-local copies of shared MCP
  prerequisites.
- Existing manual manifests that place MCP resources in `kubernaut-system`
  must be updated or explicitly treated as legacy examples.

## Verification

- `UT-INFRA-FLEET-MCP-002` through `UT-INFRA-FLEET-MCP-004` cover namespace
  resolution and independent remote configuration.
- `IT-INFRA-FLEET-MCP-005` and `IT-INFRA-FLEET-MCP-006` prove EAIGW and
  Kuadrant registration manifests use the configured namespace.
- `UT-INFRA-FLEETDEMO-031` proves Helm receives
  `global.fleet.mcpGatewayNamespace`.
- DD-TEST-015's pre-Helm provisioning callback remains the ordering proof;
  live Kind validation is tracked by Issue #2476.
