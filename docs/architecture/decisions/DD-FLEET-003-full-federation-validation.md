# DD-FLEET-003: Full Federation Validation (`ValidateFullFederation`)

**Status**: ✅ Approved & Implemented
**Date**: 2026-07-04
**Author**: AI Assistant
**Related**: Issue #54, ADR-068, BR-INTEGRATION-065, DD-FLEET-001

---

## Context

`pkg/fleet.FleetConfig` bundles two independent capabilities under a single
`Enabled` flag:

1. **Backend + Endpoint** — the federated scope-check adapter (FMC HTTP API
   or ACM GraphQL), used to resolve owner-chain metadata and determine
   whether a remote resource is managed.
2. **MCPGatewayEndpoint + MCPGatewayType** — remote K8s reads via the MCP
   Gateway, used to fetch live resource state (spec, status, labels) from
   managed clusters.

`FleetConfig.Validate()` treats these as independently optional: a service
only needs to configure the capability it actually uses. This is correct for
AF, EM, and SP, which never call the Backend/Endpoint scope-check adapter —
they discover clusters directly via a `ClusterRegistry` watching
`MCPServerRegistration`/`Backend` CRDs and only need `MCPGatewayEndpoint`.

Gateway (GW) and RemediationOrchestrator (RO) are different. Investigation
during the fleet E2E remote-cluster validation work (this session) confirmed:

- **GW** calls the scope-check backend (`FederatedScopeChecker`,
  `pkg/gateway/server.go`) to verify `kubernaut.ai/managed` labels on the
  originating signal's resource *before* creating a RemediationRequest. It
  additionally needs `MCPGatewayEndpoint` to resolve the full owner chain
  (walking up from Pod → ReplicaSet → Deployment, etc.) when the immediate
  resource's owner reference points to something the scope-check backend
  hasn't indexed yet.
- **RO** calls the same scope-check backend to gate acceptance of
  `RemediationRequest`s with a non-empty `clusterID`, **and** needs
  `MCPGatewayEndpoint` to read the remote resource's live spec (for
  `EffectiveEndpoint()`-derived spec-hash computation used in
  pre-remediation snapshotting, DD-EM-002).

For GW and RO, configuring only one of the two capabilities does not fail
loudly — `Validate()` accepts it — but silently degrades the service:
without `MCPGatewayEndpoint`, GW/RO fall back to local-only reads for
fleet-routed resources, which defeats the purpose of enabling fleet in the
first place, and does so **without any startup error** to signal the
misconfiguration.

## Decision

Add `FleetConfig.ValidateFullFederation() error`, a stricter, opt-in check
that both capabilities are configured when `Enabled=true`:

```go
func (c FleetConfig) ValidateFullFederation() error {
	if !c.Enabled {
		return nil
	}
	if c.Backend == "" && c.Endpoint == "" {
		return fmt.Errorf("fleet: backend+endpoint is required when fleet is enabled " +
			"(federated scope-check; without it, resource ownership cannot be determined)")
	}
	if c.MCPGatewayEndpoint == "" {
		return fmt.Errorf("fleet: mcpGatewayEndpoint is required when fleet is enabled " +
			"(remote reads; without it, fleet-routed resources silently degrade to local-only reads)")
	}
	return nil
}
```

GW and RO call **both** `Validate()` (structural well-formedness of whatever
is configured) **and** `ValidateFullFederation()` (the dual-capability
mandate) during config validation, treating either error as fatal at
startup. AF, EM, and SP call only `Validate()` — they are not required to
have `Backend`/`Endpoint` configured, since they never use the scope-check
capability.

This is implemented as a separate method rather than folding the stricter
check into `Validate()` itself, because `Validate()` is also called by
AF/EM/SP, for which requiring `Backend`/`Endpoint` would force an unused
dependency on every `MCPGatewayEndpoint`-only deployment.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Fold the dual-capability check into `Validate()` unconditionally | Would break AF/EM/SP, which legitimately only need `MCPGatewayEndpoint` |
| Add a `RequireBoth bool` field to `FleetConfig` set per-service at construction | Extra config surface for something that is a static property of the calling service (GW/RO always need both), not something an operator should be able to toggle |
| Leave it as a documentation-only convention ("GW/RO operators must set both") | Silent degradation on misconfiguration is exactly the failure mode this decision exists to close; a doc comment does not fail a broken deployment at startup |
| Validate at the Helm chart level only (schema/fail guards) | Necessary but insufficient — catches Helm-based deployments but not direct binary/flag-based deployments or config file edits post-install |

## Consequences

- **Positive**: GW and RO now fail fast at startup with an actionable error
  message when fleet is enabled but only partially configured, instead of
  silently falling back to local-only behavior for fleet-routed resources.
- **Positive**: AF/EM/SP are unaffected — they never call
  `ValidateFullFederation()`.
- **Neutral**: This is a startup-time check only; it does not affect runtime
  behavior once a service has passed validation.

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | UT Test ID |
|---|---|---|---|
| `FleetConfig.ValidateFullFederation()` | Defined alongside `Validate()` | `pkg/fleet/config.go` | UT-FLEET-CFG-040..043 (`pkg/fleet/fleet_test.go`) |
| GW call site | `ServerConfig.Validate()` | `pkg/gateway/config/config.go` | `Describe("BR-INTEGRATION-065/ADR-068: Fleet full-federation validation")` (`pkg/gateway/config/config_test.go`) |
| RO call site | `Config.Validate()` | `internal/config/remediationorchestrator/config.go` | `Describe("BR-FLEET-054/ADR-068: Fleet full-federation validation")` (`internal/config/remediationorchestrator/config_test.go`) |

## Authority

Issue #54, ADR-068, BR-INTEGRATION-065.
