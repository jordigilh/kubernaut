# DD-TEST-015: Fleet E2E "Deploy Correctly the First Time" (Chart-Native Fleet Enablement)

**Status**: ✅ Approved & Implemented
**Date**: 2026-07-28
**Author**: AI Assistant
**Related**: Issue #54, ADR-068, DD-TEST-013, DD-TEST-014, DD-PLATFORM-008
(`startupProbe` for fleet-aware services), DD-FLEET-002/003

---

## Context

The `test/e2e/fleet` suite (`SetupFleetE2EInfrastructure`) validates that the
Helm chart deploys every Kubernaut service, fleet federation included, and
that they work end to end. Prior to this decision, the suite's actual
sequence was:

1. Call `SetupFullPipelineInfrastructure` (build images, create the Kind
   cluster, `helm install charts/kubernaut` with `global.fleet.enabled`
   left at the chart's default of `false`).
2. Deploy Keycloak, the Kuadrant MCP Gateway, kube-mcp-server, and a remote
   Kind cluster.
3. `kubectl patch` the already-running Gateway, RemediationOrchestrator,
   SignalProcessing, APIFrontend, EffectivenessMonitor, and
   WorkflowExecution ConfigMaps/Deployments to append a `fleet:` block and
   mount an OAuth2 credentials Secret + the inter-service CA, forcing 1-3
   extra Pod restarts per service.
4. Deploy FleetMetadataCache (FMC) and Valkey via raw `kubectl apply`
   manifests maintained independently of the chart's own
   `templates/fleetmetadatacache/fleetmetadatacache.yaml` and
   `templates/infrastructure/valkey.yaml`.

This design had a structural gap that directly undermined the suite's own
stated purpose: **the chart's own `global.fleet.*`-conditioned templates
were never rendered fleet-enabled by this suite at all.** Every fleet-aware
service was always deployed with `global.fleet.enabled=false` (the chart's
default), so the chart's fleet RBAC (`kubernaut.fleetRBAC` helper),
ConfigMap fleet blocks (`$gatewayFleetPreamble` et al.), and
`oauth2-credentials`/`inter-service-ca` volume mounts were never exercised.
A regression in any of that chart-native fleet templating would have gone
completely undetected -- the Go harness's own kubectl patches would silently
paper over it by reconstructing equivalent config out-of-band. This is the
exact class of gap Issue #1737's broader Helm migration was meant to close.

Additionally, the patch-based design caused real operational pain
(discovered during E2E debugging): patching AF/EM/WE's Deployments to add
the fleet OAuth2 volume forced pod restarts that raced Kuadrant's
asynchronous AuthPolicy/Envoy `ext_authz` convergence, requiring the
`startupProbe` mitigation in DD-PLATFORM-008 and careful re-ordering of the
patch calls relative to `WaitForFleetReady`.

## Decision

Provision all fleet infrastructure (Keycloak, the Kuadrant MCP Gateway,
kube-mcp-server, the remote Kind cluster) **before** the chart's single
`helm install`, and render every fleet-aware service -- including FMC --
fleet-enabled from that one install, via `global.fleet.*` `--set` values.
No post-install patching remains.

### 1. `FleetProvisioner` callback threaded through `SetupFullPipelineInfrastructure`

```go
type FleetProvisioner func(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) (*FleetHelmOptions, error)

func SetupFullPipelineInfrastructure(ctx context.Context, clusterName, kubeconfigPath string,
    fleetProvisioner FleetProvisioner, writer io.Writer) (...)
```

`SetupFullPipelineInfrastructure` invokes `fleetProvisioner` (PHASE 5b) once
the target namespace exists but *before* PHASE 6's `helm install`. This
single shared function now serves both suites:

- `test/e2e/fullpipeline`: passes `nil` -- fleet stays disabled, identical
  to pre-refactor behavior.
- `test/e2e/fleet` (`SetupFleetE2EInfrastructure`): passes a closure that
  deploys Keycloak, the Kuadrant MCP Gateway + kube-mcp-server, and the
  remote Kind cluster, then returns a `*FleetHelmOptions`.

A callback (not a pre-computed struct) is required because fleet
infrastructure deployment needs a live Kind cluster and the target
namespace, both of which only exist partway through
`SetupFullPipelineInfrastructure`'s own cluster-creation/namespace phases --
this lets the `fleet` suite share that single cluster-creation path with
`fullpipeline` instead of duplicating it or splitting the function.

### 2. `FleetHelmOptions` renders `global.fleet.*` on the first `helm install`

`InstallFullPipelineHelmChart` accepts `fleetOpts *FleetHelmOptions` and, when
non-nil, appends `--set global.fleet.enabled=true`,
`mcpGatewayEndpoint`/`mcpGatewayType`, `oauth2.enabled`/`tokenURL`/
`credentialsSecretRef`/`scopes`, `workflowexecution.fleet.oauth2.credentialsSecretRef`
(WE requires its own, no fallback -- AC-6 least privilege), and
`fleetmetadatacache.enabled=true` (+ FMC's image repository/tag, which does
not follow the shared `global.image.*` convention every other chart-managed
service uses). GW/RO/SP/AF/EM's chart templates already derive their own
fleet config, RBAC, and volume mounts from these `global.fleet.*` values via
the existing `kubernaut.fleet.preamble`/`kubernaut.fleetRBAC` helpers -- no
chart template changes were needed to make this work; the chart's fleet
support was already there and simply had never been driven end to end.

### 3. FMC migrates to chart-native deployment

`fleetmetadatacache.enabled=true` replaces the raw-manifest
`deployValkeyAndFMC` call entirely for the `fleet` suite. Valkey was already
chart-managed by default (`valkey.enabled=true`); FMC now is too. The
standalone `fleetmetadatacache` E2E lane (`SetupFMCE2EInfrastructure`, which
does not install the Helm chart at all) is unaffected -- it still uses
`deployValkeyAndFMC` via `DeployFleetCoreInfra`.

### 4. `DeployFleetGatewayInfra` extracted from `DeployFleetCoreInfra`

`DeployFleetCoreInfra`'s Phases 1-3 (Gateway/EAIGW + kube-mcp-server +
registrations) are extracted into `DeployFleetGatewayInfra`, which returns
`mcpGatewayEndpoint` for `FleetHelmOptions`. `DeployFleetCoreInfra` remains
as a thin wrapper (Phases 1-3 + Phase 4 Valkey/FMC) for the FMC-only lane,
which still needs raw-manifest FMC/Valkey.

### 5. Pre-provisioned inter-service CA breaks the Keycloak/chart circular dependency

The chart's `templates/hooks/tls-cert-job.yaml` pre-install hook normally
generates the inter-service CA (`authwebhook-tls` Secret) and the
`inter-service-ca` ConfigMap as a side effect of `helm install`. Since
Keycloak (and now kube-mcp-server's required `tls-ca` volume) must be fully
deployed *before* `helm install` runs, a new `provisionInterServiceCA`
function self-signs an ECDSA P-256 CA and creates both resources directly,
matching the hook's own shape exactly. The hook is idempotent (checks
`authwebhook-tls`'s expiry and `ca.crt`/`ca.key` presence before
regenerating), so it detects the pre-created secret as valid and reuses that
CA to sign every other chart-managed service's leaf certificate -- one
shared root of trust across Keycloak, kube-mcp-server, and every
chart-managed service, generated once, before anything that needs it exists.

## Consequences

### Positive

- The chart's own fleet templates (RBAC, ConfigMap fleet blocks, volume
  mounts) are now genuinely exercised end to end by this suite -- closing
  the coverage gap described in Context.
- Every fleet-aware service boots exactly once, already fleet-configured --
  no more 1-3 extra restarts per service from post-install patching.
- ~130 net lines removed from `fleet_e2e.go` (six `patch*ConfigForFleet`
  functions, `patchDeploymentAddFleetOAuth2Volume`, `fleetOAuth2YAMLBlock`,
  `fleetTLSCAFile`/`fleetTLSCAMountDir`, `appendYAMLBlockToConfigMap`,
  `DeployFleetInfra`) despite the CA-provisioning and orchestration code
  added.
- `FleetProvisioner` establishes a reusable pattern: any future suite
  needing pre-`helm install` infrastructure follows the same shape instead
  of inventing its own post-install patch cycle.

### Negative / Risks

- `SetupFullPipelineInfrastructure`'s signature changed (new
  `fleetProvisioner` parameter); the one external call site
  (`test/e2e/fullpipeline/suite_test.go`) was updated to pass `nil`.
- FMC's image does not follow the `kubernaut.image` helper convention every
  other chart-managed service uses (see `fleetmetadatacache.yaml`'s
  dedicated `image.repository`/`image.tag` values vs. `gateway.yaml`'s
  `{{ include "kubernaut.image" ... }}`). This is a pre-existing chart
  inconsistency, worked around here via an explicit `--set` override
  reusing the same shared registry/tag every other service already got;
  fixing the chart template itself was judged out of scope for this
  refactor and is called out here for future consideration.

## Alternatives Considered

1. **Split `SetupFullPipelineInfrastructure` into "create cluster" and
   "deploy chart" halves**, letting the `fleet` suite interleave fleet infra
   between them without a callback. Rejected: a much larger blast radius to
   the already-green `fullpipeline` suite for no behavioral benefit over the
   callback approach, and two call sites to keep in sync instead of one.
2. **Keep the patch-based design but reorder patches earlier relative to
   `WaitForFleetReady`** (a narrower fix considered mid-investigation).
   Rejected: this does not address the root gap -- the chart's fleet
   templates would still never render fleet-enabled in this suite.
3. **Leave FMC on raw-manifest deployment, only migrate GW/RO/SP/AF/EM/WE**.
   Rejected per explicit user decision: full consistency (one `helm install`
   for everything) was preferred over a partially chart-native fleet stack.
