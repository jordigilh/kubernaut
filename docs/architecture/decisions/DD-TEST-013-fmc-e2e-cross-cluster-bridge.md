# DD-TEST-013: FMC E2E Cross-Cluster Bridge (Second Kind Cluster)

**Status**: ✅ Approved
**Date**: 2026-07-02
**Author**: AI Assistant
**Related**: Issue #54, ADR-068, BR-INTEGRATION-065, [Spike S19](../../spikes/multi-cluster-mcp-gateway/spike-s19-fmc-e2e-second-cluster/README.md), DD-TEST-001

---

## Context

The Fleet Metadata Cache (FMC) E2E lanes (`test/e2e/fleetmetadatacache/`
Kuadrant + EAIGW variants) register three "clusters" with the MCP Gateway:
`loopback-cluster`, `prod-east`, `prod-west`. All three currently target the
**same** physical Kind cluster and the same `kube-mcp-server` Deployment (the
"loopback pattern," documented in `SetupFleetE2EInfrastructure`). This proves
the multi-cluster *code path* (three distinct `MCPServerRegistration`/
`Backend` objects, three distinct tool-name prefixes, FMC's per-cluster scope
resolution) but does not prove genuine cross-cluster *data isolation* --
every "cluster" sees identical Kubernetes API state because there is only
one API server behind all three registrations.

For SOC2 CC8.1 / FedRAMP AC-4 (information flow enforcement) coverage of
Issue #54's multi-cluster federation claim, at least one registered cluster
should be backed by a genuinely separate Kubernetes control plane, so tests
can assert that a resource created only in `prod-east`'s backing cluster is
never visible through `loopback-cluster`'s or `prod-west`'s scope, and vice
versa.

## Decision

Bridge `prod-east` to a **second, independent Kind cluster** using only
`Service`+`Endpoints` objects over the shared podman `kind` bridge network --
no service mesh, no Istio multi-cluster, no VPN. This was validated
empirically end-to-end in Spike S19 (2026-07-02, all 4 phases passed) before
being folded into production test infrastructure code.

### Architecture

```
┌─────────────────────────────┐        podman "kind" bridge network        ┌─────────────────────────────┐
│   Primary Kind cluster       │◄───────────────────────────────────────►│   Remote Kind cluster        │
│   (fmc-e2e / fmc-eaigw-e2e)  │        (NodePort ↔ NodePort, no host)     │   (fmc-e2e-remote / ...)     │
│                               │                                            │                               │
│  Keycloak (real IdP)          │◄── bridge Svc "keycloak" (remote:8443) ───│  API server (OIDC-patched     │
│  kube-mcp-server (loopback)   │                                            │   against bridged Keycloak)   │
│  Kuadrant/EAIGW Gateway        │                                            │  kube-mcp-server-2            │
│    - loopback-cluster ─► local kube-mcp-server                            │    (passthrough + STS,        │
│    - prod-east ─► bridge Svc "kube-mcp-server-remote" (remote NodePort) ──┼──► same RFC 8693 exchange)    │
│    - prod-west ─► local kube-mcp-server                                   │                               │
└─────────────────────────────┘                                            └─────────────────────────────┘
```

Both directions use the same primitive: a `Service` with a hand-authored
`Endpoints` object (no selector) whose address is the peer cluster's node
bridge IP (`podman inspect <node> --format
{{.NetworkSettings.Networks.kind.IPAddress}}`) and port is the peer's
NodePort.

### Key implementation details (from Spike S19)

1. **Bridge Service port must match the dialed hostname:port, not the
   NodePort.** The remote cluster's API server dials
   `https://keycloak:8443/...` (hardcoded to match production's Keycloak
   port), so the bridge Service in the remote cluster must expose port
   `8443` with `Endpoints` pointing at the primary's actual Keycloak
   NodePort (`30557`) -- these are two different numbers and must not be
   conflated.
2. **`applyExchangedIdentityRBAC` must run against the remote cluster too**,
   binding the same Keycloak-exchanged identity (`keycloak:service-account-
   kubernaut-fleet-read`) to `view` there. `kube-mcp-server`'s own
   ServiceAccount RBAC is vestigial in passthrough mode.
3. **CA reuse requires zero code changes**: the primary cluster's
   `inter-service-ca.pem` bytes are copied to the same relative path next to
   the remote cluster's kubeconfig, and `patchAPIServerForOIDCConfig`
   (called unmodified) trusts it.
4. Existing helpers (`patchAPIServerForOIDCConfig`,
   `patchAPIServerPodHostsForIssuer`) needed **zero changes** -- both
   resolve the issuer hostname via `kubectl get svc <host> -o
   jsonpath={.spec.clusterIP}`, which is agnostic to whether the Service's
   Endpoints are real (kube-proxy-selected Pods) or hand-authored (a
   bridge).

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Istio multi-cluster / shared control plane | Massive complexity and resource cost for a test-only need; the FMC E2E lane deliberately avoids a service mesh spanning clusters (see `SetupFMCE2EInfrastructure`'s "Skips" list) |
| `kind` cluster with multiple "logical" API servers (e.g. separate namespaces + fake ClusterID labels) | Does not prove genuine control-plane isolation -- same failure mode as today's loopback pattern, just with more indirection |
| VPN / WireGuard tunnel between two Kind clusters on different hosts | Unnecessary: podman's `kind` bridge network already places both clusters' nodes on one routable L3 network when both clusters are created by the same podman daemon (CI already sets `KIND_EXPERIMENTAL_PROVIDER=podman`) |
| `kubectl port-forward`-based bridging | Explicitly rejected project-wide per DD-TEST-001 (NodePort mandated for E2E stability); port-forward processes are also a poor fit for a bridge that must stay up for the whole suite |

## Consequences

- **Positive**: `prod-east` now proves genuine cross-cluster isolation
  end-to-end (SOC2 CC8.1, FedRAMP AC-4), closing a real gap in Issue #54's
  claimed multi-cluster federation coverage.
- **Positive**: `RemoteBridge` is an opt-in, nil-safe field on
  `KubeMCPServerAuthConfig` -- the "fleet" full-pipeline suite
  (`DeployFleetInfra`/`DefaultKubeMCPServerAuthConfig`) is entirely
  unaffected; only the two FMC E2E lanes set it.
- **Negative**: both FMC E2E lanes now create and tear down **two** Kind
  clusters instead of one, increasing CI wall-clock time and resource usage
  (~95s cluster creation + kube-mcp-server-2 deployment, measured in Spike
  S19). This is the primary open risk flagged when this design was approved
  (see confidence assessment): CI resource/time budget for two concurrent
  Kind clusters per lane was not validated in Spike S19 (local-only) and is
  the first thing to check in this feature's initial CI run.
- **Negative**: must-gather diagnostics on failure must now cover both
  clusters' kubeconfigs, not just the primary's.

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT/E2E Test ID |
|---|---|---|---|
| `KindNodeBridgeIP` / `CreateServiceBridge` | Called by `SetupRemoteClusterForFMC` and `deployKuadrantRegistrations`/`deployEnvoyAIGatewayRegistrations` | `test/infrastructure/kind_bridge.go` | E2E-FMC-054-015 / E2E-FMC-EAIGW-054-015 |
| `SetupRemoteClusterForFMC` | Called from `setupFMCE2EInfrastructure` before `DeployFleetCoreInfra` | `test/infrastructure/fleetmetadatacache_remote_cluster.go`, `test/infrastructure/fleetmetadatacache_e2e.go` | E2E-FMC-054-015 / E2E-FMC-EAIGW-054-015 |
| `KubeMCPServerAuthConfig.RemoteBridge` | Read by `deployKuadrantRegistrations`/`deployEnvoyAIGatewayRegistrations` to select `prod-east`'s backend hostname | `test/infrastructure/fleet_e2e.go` | E2E-FMC-054-015 / E2E-FMC-EAIGW-054-015 |
| `shared.Harness.RemoteK8sClient` | Populated in both suites' `SynchronizedBeforeSuite`, used by the cross-cluster isolation scenario to create resources directly against the remote cluster | `test/e2e/fleetmetadatacache/suite_test.go`, `test/e2e/fleetmetadatacache/eaigw/suite_test.go` | E2E-FMC-054-015 / E2E-FMC-EAIGW-054-015 |
| `shared.CrossClusterIsolation` | Registered by both variant packages' `*_test.go` wiring files | `test/e2e/fleetmetadatacache/shared/cross_cluster_isolation.go` | E2E-FMC-054-015 / E2E-FMC-EAIGW-054-015 |

## Authority

Issue #54, ADR-068, BR-INTEGRATION-065, Spike S19
(`docs/spikes/multi-cluster-mcp-gateway/spike-s19-fmc-e2e-second-cluster/`).
