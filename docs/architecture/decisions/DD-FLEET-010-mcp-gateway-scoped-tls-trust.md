# DD-FLEET-010: Scoped TLS Trust for MCP Gateway Client Connections

**Status**: Approved for implementation
**Decision Date**: 2026-10-20
**Issue**: #2540
**Related**: ADR-068, DD-FLEET-009, BR-INTEGRATION-054, BR-INTEGRATION-065, kubernaut-operator#557
**Applies To**: Gateway, RemediationOrchestrator, SignalProcessing, APIFrontend, EffectivenessMonitor, Kubernaut Agent, WorkflowExecution, and Fleet Metadata Cache

## Context

Kubernaut currently has independent trust fields for the Fleet scope-check
backend and the OAuth2 token endpoint, but MCP protocol traffic uses the SDK's
default HTTP transport. An operator-managed private CA for the MCP Gateway can
therefore be configured without being applied to the actual MCP session.

Using process-wide `TLS_CA_FILE` or `SSL_CERT_FILE` for the new lane would
conflate MCP Gateway trust with scope-check, OAuth2, and generic inter-service
trust. It would also make an explicit `system` source impossible to represent.

## Decision

1. Add `MCPGatewayTLSCAFile` to the flat Fleet DTOs and
   `mcpclient.ConnectConfig`. FMC's nested configuration uses
   `mcpGateway.tlsCAFile` and maps it to the shared client field.
2. Preserve `FleetConfig.TLSCAFile` for scope-check backend trust and
   `FleetOAuth2Config.TLSCAFile` for OAuth2 token-endpoint trust. No aliases
   or fallback from either field are permitted.
3. Add a scoped MCP client option that builds a file-only, reloadable CA
   transport. The option must propagate construction errors to `mcpclient.New`
   so an explicitly configured unreadable or malformed CA never falls back to
   system trust.
4. Apply the gateway CA transport before the reloadable OAuth2 transport.
   OAuth2 token acquisition continues to use only its own CA field.
5. Each MCP client attempt owns its CA watcher. Failed handshakes stop the
   watcher; successful clients stop it from `Client.Close`; reconnects close
   the old client before publishing the new one. The watcher uses service
   lifetime rather than a per-attempt timeout context.
6. Valid CA rotation atomically replaces the active transport. Invalid
   rotation retains the last-known-good transport and reports a bounded
   diagnostic; the next reconnect rereads the path.
7. FMC retains a disconnected resilient client, uses dynamic MCP readiness,
   and fails readiness closed during initial failure, reconnect failure, or
   later disconnect instead of exiting or retaining a startup-only snapshot.
8. Helm adds a distinct `global.fleet.mcpGatewayTLSCAFile` contract. The
   existing `global.fleet.tlsCAFile` remains scope-check trust.

An empty gateway path means the process-effective standard Go trust behavior;
the new lane does not mutate ambient process trust. The operator's `system`
source therefore leaves this field empty, while `interService`, `file`, and
`secret` resolve to an explicitly rendered path.

## Alternatives Considered

### Process-wide ambient CA environment

**Rejected.** It conflates independent trust domains, changes unrelated
outbound clients, and cannot express the operator's explicit `system` source.

### Mutation of `http.DefaultTransport`

**Rejected.** It introduces global mutable state, cross-test contamination, and
unbounded blast radius across services and trust lanes.

### Static transport with restart-only rotation

**Rejected as the default.** A restart contract could be documented as a
fallback, but the existing Fleet client already has reconnect/readiness
semantics and the shared CA infrastructure supports atomic reload. The runtime
must preserve those properties for this lane.

### Reuse scope-check or OAuth2 CA fields

**Rejected.** Those fields have distinct endpoints and ownership semantics;
aliasing them would recreate the defect and violate least-trust separation.

## Consequences

### Positive

- Private MCP Gateway PKI is usable without process-wide trust mutation.
- Scope-check, OAuth2, and MCP trust are independently testable and operable.
- Invalid configuration fails closed and remains observable through readiness.
- CA rotation and reconnects do not require a pod restart.

### Costs and risks

- Eight production wiring paths and Helm templates must carry the new field.
- Client lifecycle owns an additional watcher and must close it on every path.
- Operator#557 must render the exact runtime paths and qualify EAIGW and
  Kuadrant HTTPS lanes with the matching artifacts.
- Empty-path semantics remain process-effective trust when ambient CA injection
  is active; this is documented rather than changing generic TLS behavior.

## Verification Contract

- UT: private-CA handshake, system trust, wrong CA, malformed/unreadable CA,
  independent OAuth2 CA, valid/invalid rotation, reconnect, lifecycle cleanup,
  redacted diagnostics, and DTO round-trip behavior.
- IT: production dispatch through all eight service entry points; FMC proves
  process survival, initial NotReady, reconnect recovery, and post-disconnect
  NotReady.
- E2E: operator-resolved `interService`, `system`, `file`, and `secret` source
  lanes against real HTTPS EAIGW/Kuadrant Fleet journeys.

Control mapping is limited to behavior proven by this decision: FedRAMP
SC-8/SC-12/SC-13/SI-10/CM-6/SI-4/CP-10, SOC 2 CC7.2, and OWASP ASVS 5.0.0
V12.1.1, V12.3.1–V12.3.4, V13.1.2–V13.1.3, V13.2.6, V16.2.1, and V16.2.5.
AC-4/AC-6/SC-7 and additional ASVS configuration controls require separate
tests that directly prove information flow, authentication, least privilege,
or allowlisting and are not inferred from CA separation alone.
