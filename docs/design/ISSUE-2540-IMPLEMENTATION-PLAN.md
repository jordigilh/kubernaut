# Issue #2540 — Scoped MCP Gateway TLS trust

## Decision

Add a gateway-specific CA path to the upstream Fleet MCP client contract. Keep
it independent from the Fleet scope-check CA and the OAuth2 token-endpoint CA.
An empty path uses the system trust store; a configured path is loaded and
validated before the MCP transport is constructed.

The operator-side source selection, Secret validation, read-only mounts, and
rollout behavior are tracked in kubernaut-operator#557.

## TDD scope

- RED: prove `ConnectConfig` carries the gateway CA path and that MCP HTTPS
  traffic succeeds with the configured private CA while rejecting the wrong CA.
- GREEN: construct the MCP transport from the gateway CA path while retaining
  the independent OAuth2 token transport.
- REFACTOR: preserve reconnect behavior, avoid logging protected material, and
  run the upstream Fleet/unit/integration checks.

## Non-goals

- Do not change process-wide `TLS_CA_FILE` semantics.
- Do not reuse `FleetConfig.TLSCAFile` for MCP traffic; it remains the
  scope-check backend trust path.
- Do not make the operator repository run the full application Fleet journey.
