# DD-TEST-019: Minimal Fleet-Enabled APIFrontend E2E Topology and CI Lanes

**Status**: ✅ Approved by the user (2026-09-23)
**Decision date**: 2026-09-23; CI-lane amendment approved 2026-09-26
**Related**: Issue #2462, BR-FLEET-054, BR-INTEGRATION-065, ADR-068, DD-TEST-014, DD-TEST-015

## Context

Issue #2462 adds Fleet-attributed APIFrontend (AF) coverage while preserving
the standalone local-mode baseline. The original plan put both environments
in one AF E2E job; the 2026-09-26 amendment below splits them into isolated CI
lanes. The test topology still comprises the local AF environment and a
Fleet-enabled AF environment whose Gateway exposes one registered `hub`
backend.

The initial implementation reused `SetupFullPipelineInfrastructure` because
its `FleetProvisioner` callback already provisions Keycloak and Fleet before
the chart's first Helm install. That path also builds/deploys the unrelated
Gateway signal pipeline, RemediationOrchestrator, Signal Processing,
AIAnalysis, WorkflowExecution, Notification, EffectivenessMonitor, and their
fixtures. The existing AF E2E CI job has a 25-minute timeout, so provisioning
that full stack creates avoidable runtime and resource pressure.

The standalone AF infrastructure already provisions the services these
journeys need: DataStorage, Kubernaut Agent, mock-LLM, APIFrontend, TLS, and
Prometheus test fixtures. The Fleet Metadata Cache E2E lane provides an
established minimal Fleet-core sequence: Keycloak, MCP Gateway, kube-mcp-server,
Valkey, and FMC. Its full lane adds a remote Kind cluster for spoke-isolation
coverage; AF #2462 only needs a hub registration backed by its own Fleet AF
cluster, so no spoke cluster is required.

The local raw-manifest AF setup is DEX-only and does not configure Fleet in
either APIFrontend or Kubernaut Agent. A Fleet-enabled AF journey therefore
must add the chart-equivalent Fleet config, OAuth2 credential mounts, Keycloak
JWT validation, and least-privilege Gateway/FMC RBAC for those two services.

## Alternatives considered

### A. Reuse the complete FullPipeline Fleet setup

Keep the first implementation: call `SetupFullPipelineInfrastructure` with a
hub-only `FleetProvisioner` callback.

- **Pros**: Existing chart templates wire Fleet, OAuth2, and RBAC into AF and
  KA; low risk of configuration drift.
- **Cons**: Starts unrelated controllers and services, uses the larger
  FullPipeline resource footprint, and threatens the existing AF job timeout.

### B. Reuse the standalone AF setup and add minimal Fleet core — selected

Build the AF images once and reuse them in both clusters. Provision the second
cluster with the AF E2E stack plus the FMC lane's Keycloak/Gateway/kube-mcp/
Valkey/FMC components, in hub-only mode. Add the Fleet configuration and
credential/RBAC wiring to the existing AF and KA test manifests.

- **Pros**: Matches the requested two-cluster AF topology, avoids unrelated
  pipeline services, and shares the existing AF fixtures and mocks.
- **Cons**: Requires explicit test-infrastructure wiring for AF/KA Fleet config
  and must keep that wiring aligned with the Helm chart's configuration.

### C. Split local and Fleet-mode AF into separate CI lanes

- **Pros**: Gives each mode independent setup timing and failure visibility;
  each runner provisions only the cluster and fixtures its selected specs need.
- **Cons**: Adds a CI matrix entry and repeats ordinary job startup steps. The
  Fleet lane must continue to use the real hub-only Gateway/FMC topology.

## Decision

Use Alternative B for infrastructure and test topology. The local AF setup
remains DEX-authenticated and local-scoped. The Fleet AF setup uses the
hub-only Gateway, OAuth2 secret, and Keycloak issuer; register exactly one
`hub` backend and do not create a remote cluster.

No production service or chart behavior changes as part of this decision.

## Amendment (2026-09-26): separate local and Fleet-mode AF CI lanes

The user approved splitting the AF tests into two CI matrix lanes to make the
repeated 12–14 minute synchronized setup observable by mode rather than
provisioning both environments serially before either mode's tests run. This
amendment supersedes the original requirement that no new workflow lane be
added; it does not change the selected infrastructure topology or Fleet
coverage contract.

- `E2E (apifrontend)` runs all non-Fleet AF specs against one standalone local
  AF Kind cluster. The `fleet-mode-af` label is excluded.
- `E2E (apifrontend-fleet)` runs only `fleet-mode-af` specs against one
  Fleet-enabled AF Kind cluster with the real hub-only Gateway/FMC setup.
- The two CI jobs remain isolated. They do not provision clusters in parallel
  within a runner, add a third cluster, or create a spoke/remote cluster.
- With `AF_E2E_LANE` unset, the suite retains its current combined two-cluster
  behavior for developers who run the whole package locally.
- The existing 25-minute GitHub Actions budget is initially retained for each
  lane. The setup-stage timing output is used to identify any remaining slow
  provisioning phase before changing timeouts or introducing concurrency.

## Consequences

- Each CI lane owns and tears down only its own Kind cluster, while the
  combined local-development path still diagnoses and tears down both.
- Fleet AF tests prove real `cluster_id=hub` routing through the registered
  Gateway and fail-closed behavior for an unregistered ID.
- FMC retains a dedicated Valkey deployment; the standalone AF DataStorage
  helper's Redis deployment remains dedicated to DataStorage's DLQ.
- Test infrastructure must verify the rendered AF/KA Fleet configuration,
  OAuth2 secret mounts, Backend registry permissions, and absence of a spoke
  Kind cluster.
- Do not infer a provisioning bottleneck from local Podman storage failures;
  use the separated clean-runner CI logs and per-stage timing before changing
  infrastructure concurrency or timeout budgets.
- Existing FullPipeline and Fleet E2E topology/setup remain unchanged.
