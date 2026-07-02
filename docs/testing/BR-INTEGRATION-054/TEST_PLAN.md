# Test Plan: Fleet OAuth2 Auth + E2E Lane (BR-INTEGRATION-054, BR-INTEGRATION-065)

**Issue**: #54
**ADR**: [ADR-068-fleet-federation-architecture](../../architecture/decisions/ADR-068-fleet-federation-architecture.md)
**Branch**: `feature/multi-cluster-federation`
**Created**: 2026-06-25
**Status**: Draft

---

## Overview

This test plan covers Phases 2 and 3 of the fleet federation authentication and E2E infrastructure:

- **Phase 2**: Wire OAuth2 `client_credentials` grant for SP, fix hardcoded scopes in KA/WE
- **Phase 3**: Fleet E2E Lane with real EAIGW + K8s MCP Server in Kind, DEX `client_credentials`, SP remote enrichment E2E, auth rejection handling

### Business Requirements

- **BR-INTEGRATION-054**: SP remote enrichment via MCP Gateway -- services authenticate with the MCP Gateway using OAuth2 client_credentials grant to access remote K8s cluster data
- **BR-INTEGRATION-065**: Multi-cluster federation scope gating -- each service identity carries scoped claims; the gateway enforces per-cluster authorization via CEL rules

### Authority

- [ADR-068: Fleet Federation Architecture](../../architecture/decisions/ADR-068-fleet-federation-architecture.md)
- [ADR-065: Fleet Cluster Identity on RR](../../architecture/decisions/ADR-065-fleet-cluster-identity-on-rr.md)

---

## FedRAMP Control Mapping

Tests in this plan provide behavioral assurance for the following NIST 800-53 controls (per ADR-068 FedRAMP Implications):

| Control | Title | Relevance |
|---------|-------|-----------|
| **AC-3** | Access Enforcement | Gateway CEL authorization rules enforce `mcp-read` vs `mcp-write` per service identity. IT tests prove the IdP issues role-bearing tokens and unauthorized callers are rejected. |
| **AC-6** | Least Privilege | K8s MCP Server SA has scoped RBAC; service scopes are operator-configurable. IT tests prove scopes are read from config, not hardcoded. |
| **AU-3** | Audit Content | Cluster provenance recorded per-RR. E2E test proves `status.clusterID` is set after cross-cluster enrichment. |
| **IA-5** | Authenticator Management | Hot-reloadable credentials, bounded token lifetime. UT tests prove default scopes enforce minimal permissions and config validation rejects misconfigured deployments. |
| **SC-7** | Boundary Protection | MCP Gateway as single chokepoint for all remote cluster access. IT/E2E tests prove all tool calls route through the gateway with namespace isolation. |
| **SC-8** | Transmission Confidentiality | OAuth2 + TLS for all MCP connections. UT tests prove token requests include correct grant type and configured scopes. |
| **SI-4** | Monitoring | Cross-cluster correlation via ClusterID. E2E test records cluster provenance. |
| **SI-10** | Input Validation | MCP response parser handles all K8s MCP Server response formats safely. UT tests cover all parsing branches including malformed input. |

---

## Test Scenario Inventory

### Core Plan Tests

| ID | Tier | Business-Level Behavior Description | Control | BR | Test File |
|----|------|-------------------------------------|---------|-----|-----------|
| UT-FLEET-SCOPE-001 | UT | [IA-5] Service identity requests minimal scopes by default, ensuring no over-permissioned tokens are issued | IA-5 | BR-INTEGRATION-054 | `pkg/fleet/mcpclient/auth_scope_test.go` |
| UT-FLEET-SCOPE-002 | UT | [SC-8] Token request includes only the configured scopes and correct grant type, preventing credential scope escalation | SC-8 | BR-INTEGRATION-054 | `pkg/fleet/mcpclient/auth_scope_test.go` |
| IT-SP-054-OAuth2 | IT | [SC-7] SP authenticates to the MCP Gateway before accessing remote cluster data, proving boundary protection is enforced for every cross-cluster call | SC-7 | BR-INTEGRATION-054 | `test/integration/signalprocessing/fleet/sp_fleet_auth_test.go` |
| IT-FLEET-006 | IT | [AC-6] Service scopes are operator-configurable (not hardcoded), enabling least-privilege enforcement per deployment | AC-6 | BR-INTEGRATION-065 | `test/integration/kubernautagent/fleet/fleet_wiring_test.go` |
| IT-FLEET-EAIGW-001 | IT | [SC-7] All remote cluster tool calls are routed through the gateway chokepoint with per-cluster namespace isolation | SC-7 | BR-INTEGRATION-054 | `test/integration/fleetmetadatacache/fleet_eaigw_test.go` |
| IT-FLEET-DEXCC-001 | IT | [AC-3] IdP issues service identity tokens with role-bearing claims that distinguish read-only from write-capable services | AC-3 | BR-INTEGRATION-054 | `test/integration/kubernautagent/fleet/fleet_dex_cc_test.go` |
| IT-FLEET-AUTH-REJECT-001 | IT | [AC-3] Unauthorized callers are rejected at the gateway boundary and the client surfaces the denial (not silent pass-through) | AC-3 | BR-INTEGRATION-065 | `test/integration/kubernautagent/fleet/fleet_wiring_test.go` |
| E2E-FLEET-SP-001 | E2E | [SC-7] Fleet infrastructure deploys with the gateway as the sole entry point for remote cluster access | SC-7 | BR-INTEGRATION-054 | `test/e2e/signalprocessing/fleet_infra_test.go` |
| E2E-SP-054-REMOTE | E2E | [AU-3] SP enriches an RR with remote cluster context and records cluster provenance, proving the full authenticated cross-cluster journey | AU-3, SC-7 | BR-INTEGRATION-054 | `test/e2e/signalprocessing/fleet_enrichment_test.go` |

### Coverage-Gap Tests (Business Logic Completeness)

| ID | Tier | Business-Level Behavior Description | Control | BR | Test File |
|----|------|-------------------------------------|---------|-----|-----------|
| UT-SP-054-CFG-001 | UT | [IA-5] SP FleetOAuth2 config parses, validates, and defaults correctly at startup, rejecting misconfigured deployments before runtime | IA-5 | BR-INTEGRATION-054 | `pkg/signalprocessing/config/config_test.go` |
| UT-WE-054-CFG-001 | UT | [IA-5] WE FleetOAuth2 config parses, validates, and defaults correctly at startup | IA-5 | BR-INTEGRATION-054 | `pkg/workflowexecution/config/config_test.go` |
| UT-KA-054-CFG-001 | UT | [IA-5] KA FleetOAuth2 config parses scopes from YAML and applies safe defaults when omitted | IA-5 | BR-INTEGRATION-054 | `internal/kubernautagent/config/config_test.go` |
| UT-FLEET-RES-006 | UT | [AC-3] Resilience layer correctly classifies all retryable error patterns (401, session loss, connection reset, EOF, connection refused) and rejects non-retryable errors | AC-3 | BR-INTEGRATION-065 | `pkg/fleet/mcpclient/resilience_test.go` |
| UT-FLEET-PARSE-001 | UT | [SI-10] MCP response parser handles all K8s MCP Server response formats: empty text, single object, list with items key, raw JSON array, and invalid JSON | SI-10 | BR-INTEGRATION-054 | `pkg/fleet/mcpclient/parse_test.go` |
| UT-FLEET-PARSE-002 | UT | [SI-10] ExtractText extracts text content from MCP results and falls back to JSON serialization for non-text content types | SI-10 | BR-INTEGRATION-054 | `pkg/fleet/mcpclient/parse_test.go` |
| UT-FLEET-TOOL-001 | UT | [SC-7] ClusterTool constructs the correct gateway-prefixed tool name for multi-cluster routing | SC-7 | BR-INTEGRATION-054 | `pkg/fleet/mcpclient/tool_names_test.go` |
| UT-FLEET-AUTH-006 | UT | [IA-5] LoadOAuth2ConfigFromFiles rejects missing clientID and missing clientSecret files with actionable error messages | IA-5 | BR-INTEGRATION-065 | `pkg/fleet/mcpclient/auth_test.go` |
| UT-WE-054-003 | UT | [AC-3] WriterClient.Update serializes and routes the update to the correct remote cluster via MCP gateway | AC-3 | BR-INTEGRATION-054 | `pkg/fleet/mcpclient/writer_test.go` |

### Deferred Tests (Depends on ADR-068 Phase 3 / Spike S11)

| ID | Tier | Business-Level Behavior Description | Control | BR | Status |
|----|------|-------------------------------------|---------|-----|--------|
| E2E-WE-065-REMOTE | E2E | [AC-3, SC-7] WE executes remediation workflows on a remote cluster through the MCP Gateway using write-scoped credentials | AC-3, SC-7 | BR-INTEGRATION-065 | Not Implemented -- depends on WriterClient, ClientFactory, executor refactoring |

---

### Fleet Metadata Cache (FMC) Dedicated E2E Lane

FMC (`test/e2e/fleetmetadatacache/`) is a separate, lighter-weight E2E lane (Istio +
Kuadrant MCP Gateway + kube-mcp-server + Valkey + FMC + Keycloak, no Gateway/RO/other
Kubernaut services -- see the package doc comment in `suite_test.go`) that proves FMC's
own journeys directly, closing a pyramid invariant gap: before this lane existed, FMC
was only exercised transitively through `FLEET_E2E`-gated tests that were never enabled
in CI (see `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` for the
lane's port allocations).

Keycloak replaces DEX in this lane (Spike S17/S18): kube-mcp-server runs in passthrough
mode and performs a real RFC 8693 Standard Token Exchange against Keycloak to reach the
Kubernetes API server, validating the actual production token-exchange wiring end-to-end
-- DEX does not implement Standard Token Exchange. The "fleet" full-pipeline suite is
unaffected and still uses DEX with kube-mcp-server in kubeconfig/fixed-SA mode.

FMC is **not** in DD-AUDIT-003's audit-emitting service inventory (declared as
"NO audit traces needed", v2.3) -- it is a read-only scope-cache dependency consulted
by Gateway/RO, not a participant in the audited remediation lifecycle. Its E2E control
scope is therefore narrower than the full fleet-architecture set above.

| ID | Tier | Business-Level Behavior Description | Control | BR | Test File |
|----|------|-------------------------------------|---------|-----|-----------|
| E2E-FMC-054-010 | E2E | [SC-7, AC-3] FMC discovers `loopback-cluster`/`prod-east`/`prod-west` via real MCPServerRegistration discovery and marks a `kubernaut.ai/managed=true` Service as managed after a real Keycloak+Kuadrant+kube-mcp-server (RFC 8693 token exchange) sync cycle | SC-7, AC-3 | BR-INTEGRATION-065 | `test/e2e/fleetmetadatacache/sync_journey_test.go` |
| E2E-FMC-054-011 | E2E | [SC-7, AC-6] FMC's scope-check API fails closed for unlabeled resources and unregistered clusters, stops reporting a resource as managed once its label is removed and the cache entry lapses (real resync, no lower tier proves this transition), and FMC's own ServiceAccount is restricted to read-only MCP Gateway RBAC | SC-7, AC-6 | BR-INTEGRATION-065 | `test/e2e/fleetmetadatacache/least_privilege_test.go` |
| E2E-FMC-054-012 | E2E | [SI-4, CP-10] FMC's `/readyz` genuinely degrades to 503 when the real Valkey dependency fails, then auto-recovers and resumes writing fresh cache entries once Valkey self-heals, without requiring FMC's own restart | SI-4, CP-10 | BR-INTEGRATION-065 | `test/e2e/fleetmetadatacache/resilience_test.go` |
| E2E-FMC-054-013 | E2E | [SI-4, CM-6] FMC's cluster registry reacts live to a real MCPServerRegistration CRD being created and deleted, without disturbing the fixed cluster set | SI-4, CM-6 | BR-INTEGRATION-065 | `test/e2e/fleetmetadatacache/dynamic_registration_test.go` |
| E2E-FMC-054-014 | E2E | [AC-6] kube-mcp-server performs a real RFC 8693 Standard Token Exchange against Keycloak (subject token audienced for kube-mcp-server -> exchanged token audienced for k8s-api), and the real Kubernetes API server honors the exchanged token while rejecting the un-exchanged subject token, proving the exchange step is a real security boundary rather than an inert passthrough (Spike S17/S18) | AC-6 | BR-INTEGRATION-065 | `test/e2e/fleetmetadatacache/token_exchange_test.go` |

**Note on cross-cluster isolation**: the 3 fixed clusters (`loopback-cluster`,
`prod-east`, `prod-west`) are a loopback pattern -- all 3 `MCPServerRegistration`s
target the *same* `HTTPRoute`/backend (single physical Kind cluster), differentiated
only by MCP tool-name prefix. This validates the multi-cluster *code path* but cannot
prove genuine cross-cluster data isolation (a resource "leaking" as managed across two
truly separate API servers). A dedicated second Kind cluster with real cross-cluster
networking would be required to close that specific gap; deferred as a follow-up.

---

### Fleet Metadata Cache (FMC) E2E Lane -- Envoy AI Gateway (EAIGW) variant

`test/e2e/fleetmetadatacache/eaigw/` is the Envoy AI Gateway sibling of the Kuadrant
FMC E2E lane above (Spike S18, `docs/spikes/multi-cluster-mcp-gateway/spike-s18-envoy-ai-gateway-e2e/`):
same DataStorage + Keycloak + kube-mcp-server + Valkey + FMC stack and the same
passthrough+STS RFC 8693 token-exchange design, but kube-mcp-server is fronted by
Envoy AI Gateway (Envoy Gateway + AI Gateway layer, `Backend`/`MCPRoute` CRDs) instead
of Kuadrant (Istio + controller + broker + `MCPServerRegistration`). The RFC 8693
exchange itself lives entirely inside kube-mcp-server and needed zero gateway-specific
changes -- only the edge routing/OAuth validation layer differs (ADR-068 Decision #9).

Runs in its own isolated Kind cluster
(`test/infrastructure/kind-fleetmetadatacache-eaigw-config.yaml`, NodePort 31976 per
DD-TEST-001) so both lanes can run in CI without port collisions.

| ID | Tier | Business-Level Behavior Description | Control | BR | Test File |
|----|------|-------------------------------------|---------|-----|-----------|
| E2E-FMC-EAIGW-054-010 | E2E | [SC-7, AC-3] FMC discovers `loopback-cluster`/`prod-east`/`prod-west` via real `Backend` CRD discovery and marks a `kubernaut.ai/managed=true` Service as managed after a real Keycloak+EnvoyAIGateway+kube-mcp-server (RFC 8693 token exchange) sync cycle | SC-7, AC-3 | BR-INTEGRATION-065 | `test/e2e/fleetmetadatacache/eaigw/sync_journey_test.go` |
| E2E-FMC-EAIGW-054-011 | E2E | [SC-7, AC-6] FMC's scope-check API fails closed for unlabeled resources and unregistered clusters, stops reporting a resource as managed once its label is removed and the cache entry lapses, and FMC's own ServiceAccount is restricted to read-only `gateway.envoyproxy.io/backends` RBAC | SC-7, AC-6 | BR-INTEGRATION-065 | `test/e2e/fleetmetadatacache/eaigw/least_privilege_test.go` |
| E2E-FMC-EAIGW-054-012 | E2E | [SI-4, CP-10] FMC's `/readyz` genuinely degrades to 503 when the real Valkey dependency fails, then auto-recovers and resumes writing fresh cache entries once Valkey self-heals, without requiring FMC's own restart | SI-4, CP-10 | BR-INTEGRATION-065 | `test/e2e/fleetmetadatacache/eaigw/resilience_test.go` |
| E2E-FMC-EAIGW-054-013 | E2E | [SI-4, CM-6] FMC's cluster registry reacts live to a real `Backend` CRD being created and deleted (no `MCPRoute`/broker indirection), without disturbing the fixed cluster set | SI-4, CM-6 | BR-INTEGRATION-065 | `test/e2e/fleetmetadatacache/eaigw/dynamic_registration_test.go` |
| E2E-FMC-EAIGW-054-014 | E2E | [AC-6] kube-mcp-server performs a real RFC 8693 Standard Token Exchange against Keycloak (subject token audienced for kube-mcp-server -> exchanged token audienced for k8s-api), and the real Kubernetes API server honors the exchanged token while rejecting the un-exchanged subject token, proving the exchange step is a real security boundary rather than an inert passthrough | AC-6 | BR-INTEGRATION-065 | `test/e2e/fleetmetadatacache/eaigw/token_exchange_test.go` |

---

## Coverage Targets

| Tier | Target | Projected |
|------|--------|-----------|
| Unit | >=80% of unit-testable code | ~90-95% |
| Integration | >=80% of integration-testable code | ~85-90% |
| E2E | >=80% of full service code | ~75-80% (WE E2E deferred) |
| All Tiers Merged | >=80% | ~90%+ |

---

## FedRAMP Control Coverage Matrix

| Control | Covered By |
|---------|-----------|
| AC-3 (Access enforcement) | IT-FLEET-DEXCC-001, IT-FLEET-AUTH-REJECT-001, UT-FLEET-RES-006, UT-WE-054-003, E2E-FMC-054-010, E2E-FMC-EAIGW-054-010 |
| AC-6 (Least privilege) | IT-FLEET-006, E2E-FMC-054-011, E2E-FMC-054-014, E2E-FMC-EAIGW-054-011, E2E-FMC-EAIGW-054-014 |
| AU-3 (Audit content) | E2E-SP-054-REMOTE |
| CM-6 (Configuration change) | E2E-FMC-054-013, E2E-FMC-EAIGW-054-013 |
| CP-10 (Auto-reconstitution) | E2E-FMC-054-012, E2E-FMC-EAIGW-054-012 |
| IA-5 (Authenticator management) | UT-FLEET-SCOPE-001, UT-SP-054-CFG-001, UT-WE-054-CFG-001, UT-KA-054-CFG-001, UT-FLEET-AUTH-006 |
| SC-7 (Boundary protection) | IT-SP-054-OAuth2, IT-FLEET-EAIGW-001, E2E-FLEET-SP-001, E2E-SP-054-REMOTE, UT-FLEET-TOOL-001, E2E-FMC-054-010, E2E-FMC-054-011, E2E-FMC-EAIGW-054-010, E2E-FMC-EAIGW-054-011 |
| SC-8 (Transmission confidentiality) | UT-FLEET-SCOPE-002 |
| SI-4 (Monitoring) | E2E-SP-054-REMOTE, E2E-FMC-054-012, E2E-FMC-054-013, E2E-FMC-EAIGW-054-012, E2E-FMC-EAIGW-054-013 |
| SI-10 (Input validation) | UT-FLEET-PARSE-001, UT-FLEET-PARSE-002 |

---

## Prerequisites

- DEX upgrade to `master`/`v2.46.0` with `DEX_CLIENT_CREDENTIAL_GRANT_ENABLED_BY_DEFAULT=true`
- Explicit `grantTypes: ["authorization_code", "password", "client_credentials"]` on static clients
- Fleet static clients with `clientCredentialsClaims` for `groups` propagation

---

## References

- [ADR-068: Fleet Federation Architecture](../../architecture/decisions/ADR-068-fleet-federation-architecture.md)
- [ADR-065: Fleet Cluster Identity on RR](../../architecture/decisions/ADR-065-fleet-cluster-identity-on-rr.md)
- Issue #54: Multi-cluster federation
- Spike S11: WE Remote Execution via MCP Gateway
