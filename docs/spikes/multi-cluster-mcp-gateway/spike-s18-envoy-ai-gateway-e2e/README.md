# Spike S18 — Envoy AI Gateway (EAIGW) Real Kind E2E Validation

## Goal

Empirically validate deploying Envoy AI Gateway's **actual Helm-installed K8s
controller** (`GatewayClass`/`Gateway`/`Backend`/`MCPRoute` CRDs, real OAuth
`SecurityPolicy`) in a throwaway Kind cluster, in front of `kube-mcp-server`
and Keycloak, as a genuine alternative to the Kuadrant MCP Gateway FMC E2E
lane -- and produce a GO/NO-GO before committing to building
`test/e2e/fleetmetadatacache/eaigw/`.

This supersedes the ADR-068 References section's earlier "Spike S11: Envoy
AI Gateway evaluation (2026-06-25)" entry, which validated the `__` prefix
convention, standalone-container mode, memory footprint, and CEL auth against
a bare EAIGW *binary* (no Helm chart, no controller, no CRDs) -- that entry's
"S11" label collided with the unrelated, already-numbered
`spike-s11-we-remote-execution` (WE Remote Execution via MCP Gateway) and is
corrected to cite this spike instead. That standalone-container coverage
still exists today as `IT-FLEET-EAIGW-001`
(`test/integration/fleetmetadatacache/fleet_eaigw_test.go`), complementary
to (not replaced by) this spike's heavier CRD-based E2E coverage.

## Context

Prior to this spike, `pkg/fleet/registry/eaigw_registry.go` (watches
`Backend` CRDs) was fully implemented and IT-tested against `envtest`, but
the only *real* EAIGW binary test ran it as a bare standalone container with
a hand-rolled JSON config -- never through its Helm-installed controller,
`GatewayClass`/`Gateway`, or `Backend`/`MCPRoute`/`SecurityPolicy` CRDs.
`deploy/mcp-gateway/` had draft `Backend`/`MCPRoute` registration YAML but
explicitly documented the controller install itself as an unmet
prerequisite. Unlike Kuadrant (de-risked by
`spike-s14-kuadrant-kind-deployment` before its own E2E lane was built), no
prior art existed in this repo for deploying EAIGW's actual K8s controller.

**Key design decision** (validated, not just assumed, by this spike): the
RFC 8693 Standard Token Exchange that the Kuadrant FMC E2E lane validates
lives entirely *inside* kube-mcp-server (`pkg/kubernetes/sts.go`) -- it is
not a Kuadrant-specific mechanism. The gateway's only job is validating the
caller's original token before proxying unmodified to kube-mcp-server. Since
EAIGW's `MCPRoute.spec.securityPolicy.oauth` provides that same validation
function (issuer/audience checks), the EAIGW lane reuses the exact same
Keycloak realm, `StsScopes`, and passthrough+STS kube-mcp-server config
already built for Kuadrant (Spike S17) -- confirmed end-to-end below, no new
exchange machinery needed.

## Phase A: Controller install + single-backend OAuth validation (2026-07-02, `eaigw-spike`)

Deployed in a throwaway Kind cluster: Envoy Gateway + AI Gateway Helm charts,
a `GatewayClass`/`Gateway`, one `Backend` + one `MCPRoute` with
`securityPolicy.oauth` against the existing `kubernaut-fleet` Keycloak realm.

**Result: GO.** Full chain validated end-to-end: unauthenticated request ->
`401`; wrong-audience Keycloak token -> `403 "Audiences in Jwt are not
allowed"`; valid `kubernaut-fleet-read` client_credentials token -> `200`,
`tools/list` returned real kube-mcp-server tools correctly prefixed
`loopback-cluster__resources_get`, `loopback-cluster__pods_list`, etc. --
exactly the convention `pkg/fleet/registry/eaigw_registry.go` and
`EAIGWDiscoverer` already expect. No application code changes needed; this is
purely a test-infrastructure build.

### Undocumented gaps found and resolved (Phase A)

1. **Version pin was wrong.** `ai-gateway-helm` v1.0.0 requires **Envoy
   Gateway v1.8.1**, not v1.7.0. v1.7.0 installs cleanly but the AI Gateway
   controller crash-loops (`no matches for aigateway.envoyproxy.io/v1beta1`)
   -- actually caused by gap #2, but only surfaces this way once CRDs exist;
   the version mismatch separately breaks the extension-server
   cluster-rewrite mechanism in gap #3.
2. **CRDs are a separate chart.** `ai-gateway-helm` v1.0.0 does **not**
   bundle CRDs. Must additionally install
   `oci://docker.io/envoyproxy/ai-gateway-crds-helm --version v1.0.0` or the
   controller crash-loops with `no matches for aigateway.envoyproxy.io/v1beta1`.
3. **Two more manual Envoy Gateway config keys are mandatory**, neither
   mentioned in the MCP quickstart:
   - `extensionApis.enableBackend: true` -- without it, every
     `HTTPRoute`/`MCPRoute` reports `ResolvedRefs: Backend is disabled in
     Envoy Gateway configuration` and returns `500 direct_response`.
   - `extensionManager` wired to the AI Gateway controller's own gRPC
     extension-server port (`ai-gateway-controller.<ns>.svc.cluster.local:1063`,
     hooks `[Translation, Cluster, Route]`). Without this, the MCP proxy
     sidecar's cluster stays pointed at its literal placeholder address
     `192.0.2.42:9856` and every request times out with `503
     upstream_reset_before_response_started{connection_timeout}`.
4. **JWKS over self-signed TLS needs an explicit `Backend` +
   `BackendTLSPolicy`.** `MCPRoute.spec.securityPolicy.oauth.jwks.remoteJWKS.uri`
   alone validates against Envoy's system trust bundle only; Keycloak's
   self-signed cert needs a `Backend` CR for the Keycloak JWKS endpoint plus
   a `BackendTLSPolicy` (`gateway.networking.k8s.io/v1alpha3`, CA from a
   ConfigMap) targeting it, referenced via `remoteJWKS.backendRefs`.

**Resource footprint** (measured via `crictl stats` inside the Kind node,
working-set memory, steady state):

| Component | Memory | Notes |
|---|---|---|
| Envoy Gateway controller | 76 MB | |
| Envoy proxy data-plane pod (envoy + shutdown-manager + extproc, 3 containers) | 153 MB | |
| AI Gateway controller | 63 MB | |
| kube-mcp-server | 24 MB | |
| **EAIGW-specific subtotal** | **~316 MB** | |
| Keycloak | 813 MB | already paid for by the Kuadrant lane; not incremental if both lanes run |
| Kind's own control-plane baseline | ~1089 MB | fixed cost of any Kind cluster, not lane-specific |
| **Total observed** | **~2.2 GB** | in line with the Kuadrant FMC E2E lane's documented ~1.7-2.5 GB |

Conclusion: EAIGW's own footprint (~316 MB) is comparable to or lighter than
Kuadrant's Istio+controller+broker stack. Wall-clock: ~2-3 min for both Helm
installs + CRDs + reconciliation to steady state. No CI resource concern.

## Phase B mini-spike: multi-backend + dynamic Service resolution (2026-07-02, `eaigw-spike-b`)

Before writing Phase B's production code, a targeted mini-spike validated
the two combinations Phase A never exercised: (1) dynamic resolution of
Envoy Gateway's hash-suffixed generated Service via labels + NodePort patch,
and (2) three `Backend`s aggregated behind one shared `MCPRoute` with
automatic per-backend tool-name prefixing (mirroring Kuadrant's 3 fixed
`loopback-cluster`/`prod-east`/`prod-west` `MCPServerRegistration`s).

**Result: GO.** Both confirmed working. Two more undocumented gaps were
found and fixed -- Phase A's "GO" conclusion stands, but its captured gap
list was incomplete:

5. **`envoy-gateway`'s own ServiceAccount lacks RBAC for `MCPRoute`.**
   Neither Helm chart grants `envoy-gateway-system:envoy-gateway` list/watch
   access to `aigateway.envoyproxy.io/mcproutes` -- required because
   `extensionManager.resources` (declaring `MCPRoute` as a watched extension
   resource) makes Envoy Gateway's own controller watch it directly.
   Symptom: `mcproutes.aigateway.envoyproxy.io is forbidden` in the
   `envoy-gateway` controller logs; the data-plane pod never becomes ready.
   Fix: an additional `ClusterRole`/`ClusterRoleBinding` granting
   `get/list/watch/patch/update` on `mcproutes`/`mcproutes/status` to the
   `envoy-gateway` ServiceAccount.
6. **The `extensionManager` config needs a `translation` block, not just
   `hooks.xdsTranslator.post`.** Phase A's captured gap #3 only recorded the
   `post: [Translation, Cluster, Route]` hook list; reproducing from scratch
   with only that (omitting
   `hooks.xdsTranslator.translation.{listener,route,cluster,secret}.includeAll: true`,
   `extensionApis.enableEnvoyPatchPolicy: true`, `gateway.controllerName`,
   and `logging.level.default`) reproduces the exact
   `192.0.2.42:9856 connection_timeout` symptom even with RBAC fixed and a
   healthy 3/3 data-plane pod (envoy + shutdown-manager + `ai-gateway-extproc`
   native sidecar).

Also newly confirmed (not previously exercised):

- **Dynamic Service resolution**: the Gateway's generated Service
  (`envoy-<gw-namespace>-<gw-name>-<8-char-hash>`, e.g.
  `envoy-kubernaut-system-mcp-gateway-a0a3182a`) is deterministically found
  via `kubectl get svc -n envoy-gateway-system -l
  gateway.envoyproxy.io/owning-gateway-name=<name>,gateway.envoyproxy.io/owning-gateway-namespace=<namespace>`
  -- automated in Go (`waitForLabeledService`,
  `test/infrastructure/fleet_e2e.go`) for FMC's ConfigMap endpoint and the
  NodePort patch.
- **No `prefix` field exists on `backendRefs`** -- EAIGW auto-prefixes every
  backend's tools with `{backendRefs[].name}__` with zero extra config,
  confirmed for 3 simultaneous backends (`loopback-cluster__`, `prod-east__`,
  `prod-west__` all returned correctly from one shared `MCPRoute.backendRefs`
  list). `toolSelector.includeAll` also does not exist; omit `toolSelector`
  entirely to expose all tools.
- **`Backend.spec.endpoints[].fqdn.hostname` requires a fully-qualified
  name** (`kube-mcp-server.kubernaut-system.svc.cluster.local`, not the bare
  `kube-mcp-server` Kuadrant's Istio-based Backend config tolerates).
- **`MCPRoute.spec.backendRefs[].kind` defaults to `Service`**, not
  `Backend` -- must set `group: gateway.envoyproxy.io` and `kind: Backend`
  explicitly per backendRef or the route silently targets a nonexistent
  Service.
- **`MCPRoute.spec.path` is a plain string** (`path: /mcp`), not an object
  (`path: {value: /mcp}` is invalid schema).

Working `envoy-gateway-config` ConfigMap body and the additional RBAC (gaps
#5/#6) are implemented verbatim in
`test/infrastructure/fleet_e2e.go`'s `deployEnvoyAIGatewayInfra`.

## Phase C: Full E2E lane implementation (2026-07-02)

Wiring the mini-spike's findings into `test/e2e/fleetmetadatacache/eaigw/`
and the shared `WaitForFleetReady` readiness probe (also used by the
Kuadrant FMC lane and the "fleet" full-pipeline suite) surfaced three more
gaps, all resolved before the suite reached 11/11 green:

7. **`ai-gateway-extproc` does not forward the client's `Authorization`
   header to backends by default.** `securityPolicy.oauth` only authenticates
   the downstream (client-to-gateway) hop; the extproc sidecar's own
   internal MCP sessions to each backend -- including proactive session
   pooling that happens before any client request -- omit `Authorization`
   unless told otherwise. Symptom: `kube-mcp-server` (in
   `require_oauth=true` passthrough+STS mode) 401s with "Bearer token
   required" on every backend session the proxy establishes, and
   `fleetmetadatacache` pods crash-loop during `initialize`. Fix: add
   `forwardHeaders: [{name: Authorization}]` to **every** entry in
   `MCPRoute.spec.backendRefs` (not just one) in `deployEnvoyAIGatewayRegistrations`.
8. **EAIGW's `securityPolicy.oauth` protects the entire `MCPRoute`,
   including the `initialize` handshake** -- unlike Kuadrant's `AuthPolicy`,
   which lets an unauthenticated `initialize` through and only enforces on
   the subsequent authenticated `tools/call`. A bare, unauthenticated
   `initialize` against EAIGW correctly returns `401`, not `200`. The
   shared `WaitForFleetReady` probe's first ("is the gateway even up") check
   was hardcoded to require `200`, so it always timed out against EAIGW
   even though the gateway and route had fully converged. Fix: treat `401`
   as equally proving reachability for that first check (the real
   convergence proof is the subsequent authenticated `tools/call`, which is
   unchanged and still gateway-agnostic).
9. **Transient RBAC-cache-not-synced `Forbidden` immediately after patching
   the API server's static pod for OIDC.** Restarting the API server pod to
   pick up `--oidc-*` flags has a brief window (a few seconds) where the
   freshly-started process serves requests but its RBAC authorizer cache
   has not yet resynced `ClusterRoleBinding`s from etcd -- even
   `kubernetes-admin` (group `kubeadm:cluster-admins`) can transiently get a
   `Forbidden` on an ordinary `get svc`. Not EAIGW-specific (the retrofit
   applies to `patchAPIServerPodHostsForIssuer`, shared with the Kuadrant
   lane), but only observed once the EAIGW lane's Kind-cluster boot become
   fast enough to hit the window. Fix: poll for up to 30s instead of
   failing on the first attempt.

Also confirmed: EAIGW has no `MCPServerRegistration.spec.prefix` equivalent
-- its auto-derived `{Backend name}__` prefix (gap: none, already documented
in Phase B) meant the readiness probe's authenticated `tools/call` needed a
different `toolPrefix` argument (`loopback-cluster__`) than Kuadrant's
explicit `loopback_cluster_`; `WaitForFleetReady` was parameterized by both
`nodePort` and `toolPrefix` to serve both lanes from one implementation.

**Result: 11/11 specs GREEN** (`make test-e2e-fleetmetadatacache-eaigw`,
~9m40s wall clock), zero regressions in the Kuadrant FMC lane (shared
`WaitForFleetReady`/`fleet_e2e.go` code paths re-verified via
`make test-e2e-fleetmetadatacache-kuadrant`).

## Outcome

Both phases GO. Phase B/C (full `test/e2e/fleetmetadatacache/eaigw/` E2E
lane, mirroring the Kuadrant FMC lane's sync-journey/least-privilege/
resilience/dynamic-registration/token-exchange coverage) implemented
directly on this spike's findings -- see
`docs/testing/BR-INTEGRATION-054/TEST_PLAN.md` (E2E-FMC-EAIGW-054-01{0..4})
and `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md`
(NodePort 31976).

## References

- Issue #54: Multi-cluster federation
- [ADR-068: Fleet Federation Architecture](../../../architecture/decisions/ADR-068-fleet-federation-architecture.md) (Decision #9)
- Spike S17: Live Keycloak Standard Token Exchange Validation (`../spike-s17-keycloak-token-exchange/README.md`)
- `spike-s11-we-remote-execution/` (unrelated spike; corrected the numbering collision this doc's title note describes)
- `IT-FLEET-EAIGW-001` (`test/integration/fleetmetadatacache/fleet_eaigw_test.go`) -- prior standalone-container EAIGW coverage
