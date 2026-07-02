# Spike S19: FMC E2E Second Kind Cluster (Cross-Cluster Isolation Bridge)

**Date**: 2026-07-02
**Status**: PASSED (all 4 phases) -- confidence raised from 78% to ~92%
**Authority**: Issue #54, follow-up to Spikes S17/S18 (Keycloak migration, EAIGW)

## Goal

The FMC E2E lanes (`test/e2e/fleetmetadatacache/` Kuadrant + EAIGW variants)
register three "clusters" (`loopback-cluster`, `prod-east`, `prod-west`), but
all three are a loopback pattern -- every `MCPServerRegistration`/`Backend`
targets the *same* backend (a single physical Kind cluster). This proves the
multi-cluster *code path* but not genuine cross-cluster *data isolation*.

This spike answers: can `prod-east` be bridged to a genuinely separate,
second Kind cluster -- with its own independent kube-mcp-server and its own
Kubernetes API server -- using only a `Service`+`Endpoints` bridge over the
podman `kind` network (no Istio multi-cluster, no service mesh)?

## Result: YES, with one bug found and fixed during the spike

All 4 phases passed. Confidence for the full implementation plan raised from
78% (pre-spike, static code reading only) to ~92% (post-spike, empirically
validated against real Keycloak, real OIDC, real RFC 8693 token exchange,
real Kubernetes API servers on two independent Kind clusters).

### Phase A: primary cluster setup (baseline, unmodified)

Ran the real `SetupFMCE2EInfrastructure` unmodified to get a realistic
baseline (Keycloak + Kuadrant MCP Gateway + kube-mcp-server + Valkey + FMC).
PASSED, ~369s.

### Phase B: remote cluster + Keycloak bridge + OIDC patch

Created a second Kind cluster (`spike-remote`), bridged a `keycloak` Service
in it back to the primary cluster's real Keycloak (over the podman `kind`
bridge network), copied the primary's inter-service CA bytes locally (no new
CA generated), and OIDC-patched the remote cluster's own API server against
it. PASSED, ~95s.

**Important caveat discovered**: the API server's `readyz` going stable here
does NOT by itself prove the bridge is live end-to-end -- `kube-apiserver`
validates the OIDC flags syntactically at startup but only performs an eager
connectivity check to the issuer lazily, on first real token validation.
Phase C is what actually proves this bridge works.

### Phase C: kube-mcp-server on the remote cluster (the real proof)

Deployed a second, independent kube-mcp-server into the remote cluster,
applied `applyExchangedIdentityRBAC` there too, exposed it via NodePort, and
ran a real authenticated `tools/call` (list Pods) against it using a
Keycloak-issued token.

**First attempt FAILED**: `Error: unable to setup OIDC provider: Get
"https://keycloak:8443/...": dial tcp 10.96.66.113:8443: connect: connection
refused`.

**Root cause**: the bridge `Service`'s `port` field was set to `8080`
(matching the *NodePort* value used elsewhere in this codebase), but
in-cluster clients dial `https://keycloak:8443/...` -- the Service must
expose port **8443** (matching the hardcoded issuer/authorization URL), with
`targetPort`/Endpoints pointing at the actual remote NodePort (`30557`).
DNS resolution alone succeeding masked this until an actual client tried to
connect.

**Fix**: `spikeCreateServiceBridge` takes `servicePort` (what clients dial)
and `remotePort` (the actual NodePort) as separate parameters. This is
directly relevant to the real implementation: `createServiceBridge` must not
assume the Service port equals the NodePort number.

**After the fix**: PASSED immediately (~4s, since the cluster/RBAC/NodePort
Service already existed from the failed attempt). This proves:
- The bridged Keycloak validates kube-mcp-server-2's incoming token
- kube-mcp-server-2 performs a real RFC 8693 exchange through the same bridge
- The remote cluster's own (OIDC-patched) API server validates the exchanged
  token and authorizes it
- **`applyExchangedIdentityRBAC` IS required on the remote cluster too** --
  kube-mcp-server's own ServiceAccount `view` binding is vestigial in
  passthrough mode (only the exchanged identity's RBAC actually matters),
  confirming an open risk item from the implementation plan.

### Phase D: reverse bridge (primary cluster -> remote cluster)

Created a `kube-mcp-server-remote` bridge Service+Endpoints in the *primary*
cluster pointing at the remote cluster's kube-mcp-server NodePort, then
proved in-cluster reachability from a throwaway pod using pure Service DNS
(`kube-mcp-server-remote.kubernaut-system.svc.cluster.local:8080`).

PASSED. Manually re-verified with a verbose curl for certainty:

```
< HTTP/1.1 200 OK
< Content-Length: 0
```

This is exactly the mechanism a Kuadrant `HTTPRoute.backendRefs` or an EAIGW
`Backend.spec.endpoints[].fqdn` would use in the real implementation.

## Key findings for the real implementation

1. **Bridge Service port must match the dialed hostname:port, not the
   NodePort.** `createServiceBridge(serviceName, servicePort, remoteIP,
   remotePort)` needs both as distinct parameters.
2. **`applyExchangedIdentityRBAC` must run against the remote cluster too**,
   not just the primary -- add this as an explicit step in
   `SetupRemoteClusterForFMC`.
3. **CA reuse works with zero code changes**: copying the primary's
   `inter-service-ca.pem` bytes to the same relative path next to the
   remote cluster's kubeconfig makes `patchAPIServerForOIDCConfig` (called
   verbatim, no modification) trust Keycloak's real certificate.
4. **`patchAPIServerForOIDCConfig`/`patchAPIServerPodHostsForIssuer` need
   zero changes** -- they resolve the issuer hostname via `kubectl get svc
   <host> -o jsonpath={.spec.clusterIP}`, which works identically whether
   the Service has real or hand-authored Endpoints.
5. **API server `readyz` stability is not sufficient evidence that OIDC
   trust actually works** -- only a real, eager-validating client (like
   kube-mcp-server's own OIDC provider setup) proves the bridge is live.
   The real implementation's validation step should rely on the E2E
   scenario's real authenticated call, not just readiness checks.
6. The podman `kind` shared-bridge-network assumption (Kind clusters on the
   same host share one bridge network, enabling direct NodePort-to-NodePort
   reachability) held on this second, independent attempt -- consistent
   with CI's `KIND_EXPERIMENTAL_PROVIDER: podman` setting.

## How to re-run

This file uses `//go:build ignore` and lives outside `test/infrastructure/`
so it does not affect normal builds. To re-run: copy
`second_cluster_spike_test.go` (remove the `//go:build ignore` line) and
`kind-spike-remote-config.yaml` into `test/infrastructure/`, then:

```bash
RUN_SPIKE=true go test -run TestSpikeA_PrimarySetup -v -timeout 20m ./test/infrastructure/...
RUN_SPIKE=true go test -run TestSpikeB_RemoteClusterAndKeycloakBridge -v -timeout 10m ./test/infrastructure/...
RUN_SPIKE=true go test -run TestSpikeC_KubeMCPOnRemote -v -timeout 10m ./test/infrastructure/...
RUN_SPIKE=true go test -run TestSpikeD_BridgeBackIntoPrimary -v -timeout 5m ./test/infrastructure/...
```

Cleanup:

```bash
kind delete cluster --name spike-primary
kind delete cluster --name spike-remote
rm -rf /tmp/kubernaut-spike
```

## Relationship to the implementation plan

See the FMC E2E Second Kind Cluster plan (Phase 0 gate). This spike closes
Phase 0 with a YES decision. Findings 1-2 above are already folded into the
plan's implementation steps 2-4 (bridge primitives, remote cluster
provisioning).
