# DD-TEST-019: Minimal Fleet-Enabled APIFrontend E2E Topology

**Status**: ✅ Approved by the user (2026-09-23)
**Decision date**: 2026-09-23
**Related**: Issue #2462, BR-FLEET-054, BR-INTEGRATION-065, ADR-068, DD-TEST-014, DD-TEST-015

## Context

Issue #2462 adds Fleet-attributed APIFrontend (AF) coverage to the existing AF
E2E job while preserving its standalone local-mode baseline. The test plan
requires two isolated Kind clusters: the existing local AF topology and a
Fleet-enabled AF topology whose Gateway exposes one registered `hub` backend.

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

### C. Add a new Fleet-only E2E job or use a mock FMC

- **Pros**: Separates Fleet infrastructure costs from the existing AF job, or
  lowers the Fleet dependency footprint with a mock.
- **Cons**: Violates the approved existing-job requirement or weakens the
  end-to-end proof of real Gateway/FMC behavior.

## Decision

Use Alternative B. Keep both Kind clusters in the existing APIFrontend E2E
job. Use the existing standalone AF setup and Kind creation helpers for both
clusters, reuse the AF image set, and add only Fleet core dependencies to the
Fleet cluster. Configure APIFrontend and Kubernaut Agent with the same
hub-only Gateway, OAuth2 secret, and Keycloak issuer; register exactly one
`hub` backend and do not create a remote cluster.

The local AF setup remains DEX-authenticated and local-scoped. The Fleet AF
setup uses the Keycloak A2A credentials and real FMC/Gateway routing. No
production service or chart behavior changes as part of this decision.

## Consequences

- The 25-minute CI timeout remains unchanged while the lean setup is
  implemented and validated.
- Fleet AF tests prove real `cluster_id=hub` routing through the registered
  Gateway and fail-closed behavior for an unregistered ID.
- FMC retains a dedicated Valkey deployment; the standalone AF DataStorage
  helper's Redis deployment remains dedicated to DataStorage's DLQ.
- Test infrastructure must verify the rendered AF/KA Fleet configuration,
  OAuth2 secret mounts, Backend registry permissions, and absence of a spoke
  Kind cluster.
- Keep the existing CI workflow unchanged based on local-host Podman storage
  failures; CI runs on a clean environment, so local disk exhaustion is not
  evidence that the CI job needs disk or tool-install changes.
- Existing FullPipeline and Fleet E2E topology/setup remain unchanged.
