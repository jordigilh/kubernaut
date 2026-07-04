# DD-TEST-014: Fleet E2E Remote-Cluster-Only Topology

**Status**: ✅ Approved & Implemented
**Date**: 2026-07-04
**Author**: AI Assistant
**Related**: Issue #54, ADR-068, BR-INTEGRATION-065, DD-TEST-013,
Spike S17/S18/S19/S20 (`docs/spikes/multi-cluster-mcp-gateway/`)

---

## Context

The `test/e2e/fleet` suite (the "fleet full-pipeline" lane, distinct from the
narrower `test/e2e/fleetmetadatacache` lanes covered by DD-TEST-013) registers
three logical clusters with the MCP Gateway: `loopback-cluster`, `prod-east`,
and `prod-west`. Historically all three targeted the **same** physical Kind
cluster and the same local `kube-mcp-server` Deployment.

This "loopback" pattern proves the multi-cluster *code path* end-to-end (GW
signal ingestion, RO scope routing, WE remote job execution, AF/EM/SP
`ClusterRegistry` discovery) but does not prove genuine remote-cluster
behavior for the majority of the suite: only whichever registration was
bridged to a second cluster (per DD-TEST-013) ever exercised a real second
control plane. An investigation triggered by an operator-reported gap ("AF
and EM's fleet config fields are not wired into main logic") confirmed the
loopback pattern was actively masking exactly this class of wiring bug --
tests passed against the local loopback cluster even when the
`FleetReaderFactory`/`ClusterRegistry` wiring was silently absent, because
local and "remote" reads were indistinguishable.

## Decision

### 1. `AllRegistrationsRemote`: force every registration onto a genuinely separate cluster

Extend the `RemoteClusterBridgeConfig` bridge mechanism introduced by
DD-TEST-013 (a `Service`+`Endpoints` bridge to a second Kind cluster, no
service mesh) with a new `KubeMCPServerAuthConfig.AllRegistrationsRemote`
flag. When true (the `fleet` suite's mode):

- **All three** registrations (`remote-cluster`, `prod-east`, `prod-west`)
  target the remote bridge's `kube-mcp-server`, instead of only `prod-east`.
- The local `kube-mcp-server` Deployment is **not created at all** --
  `deployKubeMCPServerAndRegister` is skipped entirely.
- This makes the `fleet` suite's coverage strictly stronger than the
  loopback pattern: every fleet-routed reconciliation in this suite must
  reach a genuinely separate Kubernetes control plane, so a missing
  `FleetReaderFactory`/`ClusterRegistry` wiring point fails loudly (empty
  results / connection errors) instead of silently succeeding against local
  state.
- The narrower FMC E2E lanes (`test/e2e/fleetmetadatacache/`) are
  unaffected: they leave `AllRegistrationsRemote` false and keep
  DD-TEST-013's original "prove isolation via exactly one remote
  registration" scope, which is sufficient for their narrower SOC2 CC8.1 /
  FedRAMP AC-4 assertion.

### 2. Keycloak, not Dex, for the `fleet` suite's OAuth2/token-exchange lane

The `fleet` suite's remote `kube-mcp-server` runs with `require_oauth`
enabled (a global server flag, not a per-gateway policy -- discovered during
this investigation), so every MCP client, including raw test clients, must
present a Bearer token. RFC 8693 token exchange (`passthrough`+`sts` mode)
is the production-representative auth pattern for this.

Spikes S17-S20 (this session) evaluated both Keycloak and a 2-instance Dex
topology as the identity provider for this exchange:

- **Keycloak** (Spike S17/S18/S19) works end-to-end and is already the
  proven pattern used by the FMC E2E lanes (DD-TEST-013) -- reusing it here
  is zero new IdP-integration risk, just infrastructure duplication cost
  (~1.25-2GB memory per Keycloak instance).
- **2x Dex** (Spike S20) is protocol-capable in principle (Dex's RFC 8693
  connector-based exchange works and was reproduced live), but is **not
  viable with the current `kube-mcp-server` release**: its generic STS
  client (`golang.org/x/oauth2/google/externalaccount`'s
  `stsexchange.ExchangeToken`) sends a fixed, non-extensible token-exchange
  request with no `connector_id` parameter, and Dex's exchange grant
  mandates `connector_id` with no fallback -- confirmed both by live replay
  (`{"error":"invalid_request","error_description":"Requested connector does
  not exist."}`) and by inspecting `kube-mcp-server`'s embedded binary
  strings (only a `keycloak-v1` STS strategy exists; no `dex-v1` or
  equivalent). This is a real upstream `kube-mcp-server` gap, not a
  configuration problem on our side.

Decision: standardize the `fleet` suite's OAuth2/token-exchange lane on
Keycloak (mirroring FMC exactly), and do not pursue the 2x-Dex alternative
unless/until `kube-mcp-server` gains `connector_id` support upstream. This
was later broadened by the user's explicit standardization decision:
**all** E2E infrastructure should converge on Keycloak and Dex should
eventually be removed entirely, since production only uses Keycloak and
maintaining two E2E identity providers has no offsetting value once neither
protocol gap (this decision) nor cost (Dex's original appeal) favors keeping
Dex. Full removal is scoped as separate follow-up work (see the Dex-removal
tracking issue) because two other E2E lanes (`test/e2e/kubernautagent`,
`test/e2e/apifrontend` full-pipeline) still depend on Dex's `password`
(ROPC) grant and a 7-persona-group realm shape that Keycloak's current fleet
realm (`keycloak-realm-fleet.json`, `client_credentials`+exchange only) does
not yet provide.

### 3. Rename `loopback-cluster` to `remote-cluster`

With `AllRegistrationsRemote=true`, the identity previously named
`loopback-cluster` (implying "same cluster as the caller") now targets the
same genuinely-remote bridge as `prod-east`/`prod-west`. Keeping the old name
would be actively misleading to anyone reading the suite's test files or
infrastructure code. Renamed throughout `test/infrastructure/fleet_e2e.go`,
`test/e2e/fleet/suite_test.go`, and all `test/e2e/fleet/*_test.go` files
(tool-name prefix `loopback_cluster_` → `remote_cluster_` correspondingly).
The FMC E2E lanes, which still use the original loopback pattern for two of
their three registrations, retain the `loopback-cluster` name since it
remains accurate there.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Keep the loopback pattern, add assertions that check for wiring instead | Assertions checking mocked/local behavior cannot distinguish "wiring present, reading local state" from "wiring absent, silently falling back to local state" -- the exact ambiguity that hid the original bug |
| Bridge only one of the three registrations (extend DD-TEST-013's narrower scope as-is) | Leaves two-thirds of the suite's registrations still loopback, meaning most test scenarios would still not exercise genuine remote reads |
| 2x Dex for the `fleet` suite (lighter-weight than Keycloak) | Blocked by `kube-mcp-server`'s missing `connector_id` support in its STS client (Spike S20) -- not resolvable without an upstream code change we do not control |
| Keep `loopback-cluster` name for backward-compat with existing test file references | The name change is a pure rename (mechanical `sed`-style substitution across test files and infra); backward-compat has no value for an internal-only E2E test identifier with zero external consumers |

## Consequences

- **Positive**: The `fleet` suite's coverage is now genuinely stronger --
  every registration hits a real second control plane, closing the class of
  wiring-gap bug (AF/EM not consuming `FleetReaderFactory`) that triggered
  this investigation.
- **Positive**: Reuses the already-proven, already-tested Keycloak
  infrastructure code from the FMC E2E lanes (DD-TEST-013) instead of
  introducing a second, protocol-incompatible IdP pattern.
- **Negative**: The `fleet` suite no longer stands up a local
  `kube-mcp-server` at all, so any future test scenario that specifically
  needs to compare local-vs-remote behavior side by side would need its own
  bridge configuration or a suite-level opt-out of `AllRegistrationsRemote`.
- **Negative**: Two E2E lanes (`kubernautagent`, `apifrontend` full-pipeline)
  still depend on Dex for capabilities Keycloak's fleet realm does not yet
  provide (ROPC grant, 7-persona-group shape), so full Dex removal remains
  a separate, not-yet-scoped effort (tracked in a follow-up issue).

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | E2E Test ID |
|---|---|---|---|
| `KubeMCPServerAuthConfig.AllRegistrationsRemote` | Read by `deployKuadrantRegistrations` / `SetupFleetE2EInfrastructure` | `test/infrastructure/fleet_e2e.go` | E2E-FLEET-DISC-001/002/003 |
| `fleetAuthenticatedHTTPClient` / `keycloakFleetReadTokenFunc` | Called by `newFleetMCPClient` and `WaitForFleetReady` | `test/e2e/fleet/suite_test.go`, `test/infrastructure/fleet_e2e.go` | all `test/e2e/fleet/*_test.go` specs |
| `remote-cluster`/`remote_cluster_` naming | `deployKuadrantRegistrations`, `SetupFleetE2EInfrastructure` | `test/infrastructure/fleet_e2e.go` | all `test/e2e/fleet/*_test.go` specs |

## Authority

Issue #54, ADR-068, BR-INTEGRATION-065, DD-TEST-013,
Spike S17 (`docs/spikes/multi-cluster-mcp-gateway/spike-s17-keycloak-token-exchange/`),
Spike S20 (`docs/spikes/multi-cluster-mcp-gateway/spike-s20-dex-token-exchange-bootstrap/`).
