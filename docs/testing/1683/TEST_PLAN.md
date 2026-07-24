# Test Plan: FMC TLS/HTTPS, 3-Port Standard Alignment, and FedRAMP Cipher Support

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1683-v1.0
**Feature**: Fleet Metadata Cache (FMC) currently serves its REST API and `/healthz`/`/readyz`
probes over plain HTTP on a non-standard 2-port layout (`apiAddr`/`metricsAddr`), and its
client (`fmc.HTTPClient`) has no TLS transport option. Every other Kubernaut HTTP-API service
(DataStorage, Gateway) mandates conditional TLS (Issue #493/#678), a 3-port layout (API/Health/
Metrics, Issue #753), and a configurable FedRAMP TLS security profile (Issue #748). This plan
brings FMC's server, client, Helm chart, and E2E lane into line with that standard.
**Version**: 1.0
**Created**: 2026-07-24
**Author**: AI Agent (Cursor)
**Status**: Active
**Branch**: `fix/1683-fmc-tls-3port-fedramp`

---

## 1. Introduction

### 1.1 Purpose

Issue #1683 flagged that FMC's HTTP interface does not meet the TLS/HTTPS requirements the
rest of the fleet-aware HTTP services (DataStorage, Gateway) already enforce. Triage widened
the scope to three related gaps discovered while deriving the TLS pattern: (1) FMC's port
layout deviates from the Issue #753 3-port standard, breaking the same
`ConfigureConditionalTLS`-on-API-port-only pattern DataStorage/Gateway rely on; (2) FMC has no
FedRAMP TLS security profile support (Issue #748), so an operator cannot restrict FMC's cipher
suite/TLS version on a FedRAMP-controlled OpenShift cluster; (3) FMC's client side
(`fmc.HTTPClient`, consumed by GW/RO's fail-closed readiness gate per Issue #1553/ADR-068) has
no CA-verified TLS transport option, forcing either plaintext or `InsecureSkipVerify`. This
plan proves all three gaps are closed with real TLS handshakes, real cipher-suite rejection,
and zero regression to the existing `Ping()`-based readiness contract.

### 1.2 Objectives

1. **Server TLS**: FMC's API server (port 8080) presents a certificate from `sharedtls.
   ConfigureConditionalTLS` with hot-reload, falling back to plain HTTP only when no cert is
   mounted (matching DataStorage/Gateway exactly).
2. **3-port standard**: FMC exposes `apiAddr` (8080, HTTPS-capable), `healthAddr` (8081, plain
   HTTP, kubelet-only), and `metricsAddr` (9090, plain HTTP) — matching Gateway's `ServerSettings`
   naming and the Issue #753 standard.
3. **Zero readiness regression**: `fmc.HTTPClient.Ping()` (GW/RO's fail-closed readiness probe,
   Issue #1553) continues to succeed against FMC's TLS-protected API port without modification,
   via a liveness-only `/healthz` handler dual-registered on both the API mux and the health mux.
4. **Client TLS**: `fmc.HTTPClient` accepts an injectable `*http.Client` (`WithHTTPClient`), and
   `pkg/fleet/scope_factory.go`'s FMC branch builds a CA-verified transport from `FleetConfig.
   TLSCAFile`, mirroring the existing ACM branch.
5. **FedRAMP cipher support**: FMC's `ServiceConfig` gains a `TLSProfile` field wired to
   `sharedtls.SetDefaultSecurityProfileFromConfig` at startup, so an Intermediate/Modern profile
   measurably rejects a client offering only weaker/excluded ciphers or TLS versions.
6. **Helm chart parity**: FMC's chart mounts a `fleetmetadatacache-tls` leaf cert (cert-manager
   mode) and hook-mode equivalent, renders the 3-port ConfigMap/Deployment/Service/NetworkPolicy,
   and the in-cluster URL helpers emit `https://`.
7. **E2E proof**: FMC's dedicated E2E lane (`test/e2e/fleetmetadatacache/`) exercises a real
   TLS handshake end-to-end (client CA trust, cipher restriction) — not just IT-level `httptest`
   TLS, closing the pyramid invariant gap for SC-8/SC-13.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/fleet/... ./cmd/fleetmetadatacache/...` |
| Integration test pass rate | 100% | `go test ./test/integration/fleetmetadatacache/...` |
| Unit-testable code coverage | Line coverage maintained/improved on touched files | `go test -coverprofile` |
| Backward compatibility | 0 regressions | `fmc.HTTPClient.Ping()` and existing FMC/GW/RO test suites pass unmodified in behavior |
| Helm | `helm lint` + `helm template` + `helm unittest` all pass | `make lint-helm` / `helm unittest charts/kubernaut` |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #1683: FMC HTTP interface does not meet TLS/HTTPS requirements
- Issue #493/#678: Conditional TLS for inter-pod HTTP communication (`sharedtls.ConfigureConditionalTLS`)
- Issue #753: 3-port standard (API/Health/Metrics) + dedicated health server
- Issue #748: OCP TLS security profile (Old/Intermediate/Modern) support
- Issue #1553 / ADR-068: Fail-closed Fleet readiness gate (`fmc.HTTPClient.Ping()` contract)
- ADR-068: Fleet Federation Architecture
- BR-INTEGRATION-054 / BR-INTEGRATION-065: Fleet OAuth2 auth + federated scope checking
- DD-PLATFORM-001: Inter-service mTLS CA/Issuer/Certificate provisioning

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [BR-INTEGRATION-054 TEST_PLAN.md](../BR-INTEGRATION-054/TEST_PLAN.md) (FMC's existing E2E lane; this plan extends its scenario inventory and control matrix)

### 2.3 FedRAMP Control Mapping

Tests in this plan provide behavioral assurance for the following NIST 800-53 controls, in the
same style as [BR-INTEGRATION-054's control mapping](../BR-INTEGRATION-054/TEST_PLAN.md):

| Control | Title | Relevance |
|---------|-------|-----------|
| **SC-8** | Transmission Confidentiality | FMC's REST API is served over TLS (`ConfigureConditionalTLS`); UT/IT/E2E tests prove a client without the CA-verified transport cannot silently fall back to plaintext, and that `Ping()`/scope-check calls succeed only over the TLS-protected path. |
| **SC-13** | Cryptographic Protection | FMC's `TLSProfile` (Old/Intermediate/Modern) constrains accepted TLS versions/cipher suites for FedRAMP-controlled clusters; IT/E2E tests prove a downgraded handshake (weak TLS version) is rejected when a restrictive profile is active. |
| **SC-12** | Cryptographic Key Establishment and Management | FMC's server cert is hot-reloaded via `hotreload.FileWatcher` without a restart; existing DataStorage/Gateway precedent covers the reload mechanism itself — this plan verifies FMC is wired into it, not the mechanism's internals. |
| **AC-4** | Information Flow Enforcement | The 3-port split isolates the TLS-protected API port (cross-service, NetworkPolicy-scoped to GW/RO) from the plain-HTTP health/metrics ports (kubelet/Prometheus-only, cluster-internal); IT/E2E tests prove `/readyz` is unreachable on the API port. |
| **IA-5** | Authenticator Management | `pkg/fleet/scope_factory.go`'s FMC branch verifies the server certificate against `FleetConfig.TLSCAFile`, rejecting a server presenting a cert not signed by the configured CA; IT test proves this rejection. |

This mapping is additive to Section 7's BR Coverage Matrix, which lists every test against its
NIST control via the `Control` column in Section 8.

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|-----------------|------------|
| R1 | Splitting `/healthz`/`/readyz` onto a dedicated health port breaks `fmc.HTTPClient.Ping()`, which GW/RO's fail-closed readiness gate depends on (Issue #1553) | High — GW/RO pods would incorrectly report NotReady whenever FMC is otherwise healthy | Medium | IT-FMC-1683-A-002, IT-FMC-1683-A-003 | Dual-register a liveness-only `/healthz` on both the API mux (TLS, 8080) and the new health mux (plain, 8081); `Ping()` is unchanged and keeps hitting the API base URL |
| R2 | FMC's E2E lane deploys via Go-generated raw manifests (`test/infrastructure/fleet_e2e.go`), not Helm — a port/TLS change made only in the Helm chart silently leaves E2E on the old 2-port plaintext layout | Medium — E2E claims coverage it doesn't have (pyramid invariant violation) | Medium | E2E-FMC-1683-016 | Unit F updates `fleet_e2e.go`'s FMC manifest, `interservice_tls.go`'s cert list, and `shared/resilience.go`'s readyz target together with the Helm chart |
| R3 | `ConfigureConditionalTLS` silently falls back to plain HTTP when no cert is mounted — a misconfigured deployment could believe it has TLS when it does not | Low — matches existing DataStorage/Gateway behavior exactly (fail-open by design for bootstrap ordering), not a regression introduced by this plan | Low | N/A (pre-existing accepted behavior, out of scope) | Chart mounts the cert as `optional: true` (matches DataStorage), same operational contract as the reference services |
| R4 | Changing `FleetConfig.EffectiveEndpoint()`'s default scheme from `http://` to `https://` breaks any deployment relying on the old plaintext auto-derived endpoint | Medium — silent breakage for GW/RO if the FMC server isn't actually presenting a cert yet during a rolling upgrade | Low | UT-FLEET-1683-C-001 | `ConfigureConditionalTLS`'s fail-open-to-plain-HTTP behavior means an HTTPS request to a cert-less FMC pod fails loudly (TLS handshake error) rather than silently connecting insecurely — surfaces the misconfiguration immediately instead of masking it |

### 3.1 Risk-to-Test Traceability

R1 (highest risk, functional regression) is covered by two IT tests that exercise the real
`buildFMCServers`/`main.go` wiring: one proving `/healthz` succeeds against the API port over
TLS (unchanged `Ping()` contract), one proving `/readyz` on the API port no longer exists
(moved exclusively to the health port). R2 is covered by the E2E scenario in Unit F, which
runs the real Kind-deployed manifest, not a Helm template render. R4 has no direct test (the
mitigation is an existing invariant of `ConfigureConditionalTLS`, not new code from this plan)
but is documented here as an operational rollout consideration for the accompanying PR
description.

---

## 4. Scope

### 4.1 Features to be Tested

- **FMC server TLS + 3-port split** (`pkg/fleet/fmc/config/config.go`, `cmd/fleetmetadatacache/main.go`):
  conditional TLS on the API port, dedicated plain-HTTP health server, dual-registered `/healthz`.
- **FMC client TLS** (`pkg/fleet/fmc/http_client.go`, `pkg/fleet/scope_factory.go`): injectable
  HTTP client with CA-verified transport built from `FleetConfig.TLSCAFile`.
- **FedRAMP cipher/profile support** (`pkg/fleet/fmc/config/config.go`, `cmd/fleetmetadatacache/main.go`):
  `TLSProfile` field wired to `sharedtls.SetDefaultSecurityProfileFromConfig`.
- **Endpoint scheme** (`pkg/fleet/config.go`): `EffectiveEndpoint()` emits `https://` for the FMC backend.
- **Helm chart** (`charts/kubernaut/templates/fleetmetadatacache/fleetmetadatacache.yaml`,
  `_helpers.tpl`, `interservice/leaf-certs.yaml`, `hooks/tls-cert-job.yaml`): TLS cert mount,
  3-port ConfigMap/Deployment/Service/NetworkPolicy, `https://` URL helper.
- **E2E lane** (`test/infrastructure/fleet_e2e.go`, `test/infrastructure/interservice_tls.go`,
  `test/e2e/fleetmetadatacache/`): real TLS handshake + cipher restriction proof, updated
  readyz/health target.

### 4.2 Features Not to be Tested

- **`kubernaut-operator` FMC deployment overlay**: no FMC TLS issue currently exists in that
  repo (confirmed by triage); a follow-up issue will be filed post-merge, mirroring the
  DataStorage/Gateway precedent. Out of scope for this plan.
- **Custom (operator-defined) cipher lists**: confirmed during triage that no Kubernaut service
  supports arbitrary YAML-specified cipher lists (`ProfileCustom` deliberately resolves to
  `nil` in `pkg/shared/tls/profile.go`); FMC gets the same three built-in profiles
  (Old/Intermediate/Modern) as every other service, not a new capability.
- **mTLS (client certificate auth) for FMC's own API**: out of scope; FMC uses one-way TLS
  (server cert only) exactly like DataStorage/Gateway today. Client authentication to FMC's
  API remains OAuth2/bearer-token based where applicable (GW/RO do not currently authenticate
  to FMC beyond network-policy-scoped access).

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Dual-register a liveness-only `/healthz` on both the API mux (TLS) and the dedicated health mux (plain HTTP), instead of moving it exclusively to the health port like Gateway does | `fmc.HTTPClient.Ping()` (GW/RO's fail-closed readiness gate, Issue #1553) hits `baseURL+HealthzPath` where `baseURL` is FMC's API base URL. Moving `/healthz` exclusively to the health port would require every `fmc.HTTPClient` caller to learn a second base URL, a wider blast-radius change touching GW/RO's config surface. Dual registration keeps `Ping()`'s contract byte-for-byte identical while still giving kubelet a plain-HTTP liveness probe on the standard health port. |
| `/readyz` moves exclusively to the dedicated health port (no dual registration) | No production Go caller outside FMC's own kubelet probe hits `/readyz` (confirmed by codebase-wide grep); only the E2E test harness's own polling needs updating, which Unit F does directly. |
| FMC's `ServiceConfig.TLSProfile` is set by the same `kubernaut-operator`-writes-from-APIServer-CR mechanism as Gateway/DataStorage (Issue #748), not exposed as a new Helm `values.yaml` key | Matches the existing precedent exactly (`pkg/gateway/config/config.go`'s `TLSProfile` field has no corresponding Helm `values.yaml`/`values.schema.json` entry either) — this is an OCP-only, operator-managed field, not a Helm chart concern. |
| `pkg/fleet/scope_factory.go`'s FMC branch builds its own CA transport from `cfg.TLSCAFile` instead of relying on the process-wide `sharedtls.DefaultBaseTransport()`/`$TLS_CA_FILE` singleton | Mirrors the existing ACM branch exactly (same file, ~15 lines away) instead of introducing a second CA-wiring pattern in the same factory function. |

---

## 5. Approach


### 5.1 Coverage Policy

- **Unit**: config field defaults/parsing, `WithHTTPClient` option, `EffectiveEndpoint()` scheme.
- **Integration**: real `httptest`/real-listener TLS handshake through the production
  `buildFMCServers`/`main.go` wiring; real CA-verified `HTTPClient` round-trip; real cipher
  rejection against an `IntermediateProfile()`-configured server.
- **E2E**: real Kind-deployed FMC pod presenting a cert issued by the E2E inter-service CA,
  real TLS handshake from an E2E-harness client, real cipher-suite restriction proof.

### 5.2 Two-Tier Minimum

Every unit below has both a UT (or config-level test) and an IT that exercises the real
production wiring (`cmd/fleetmetadatacache/main.go`'s `buildFMCServers`, or
`pkg/fleet/scope_factory.go`'s `NewScopeChecker`). Units A and E additionally get E2E coverage
per the pyramid invariant requirement raised during triage (SC-8/SC-13 control objectives
cannot be proven by IT `httptest` TLS alone — the FedRAMP control coverage matrix requires at
least one E2E journey per objective in scope).

### 5.3 Business Outcome Quality Bar

Every test proves an operator/consumer-visible outcome: "a client without the CA cert cannot
complete the handshake", "a client offering only excluded ciphers is rejected", "`Ping()` still
succeeds after the port split" — not "`ConfigureConditionalTLS` was called".

### 5.4 Pass/Fail Criteria

**PASS**:
1. All tests below pass.
2. `go build ./...`, `golangci-lint run --timeout=5m` clean.
3. `helm lint`, `helm template`, `helm unittest charts/kubernaut` all pass.
4. Zero regressions in existing FMC/GW/RO/fleet test suites.
5. `fmc.HTTPClient.Ping()`'s existing unit/integration tests pass unmodified.

**FAIL**: any P0 test fails, any existing passing test now fails, or the build/lint gates fail.

### 5.5 Suspension & Resumption Criteria

- Suspend if `go build ./...` breaks and cannot be fixed within the current TDD phase.
- Suspend Unit F (E2E) if Kind/Docker infrastructure is unavailable in the execution
  environment; resume once available, or defer to CI with the code changes staged.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|--------------------|-----------------|
| `pkg/fleet/fmc/config/config.go` | `ServerConfig`, `ServiceConfig.TLSProfile`, `DefaultServiceConfig` | ~30 |
| `pkg/fleet/fmc/http_client.go` | `ClientOption`, `WithHTTPClient`, `NewHTTPClient` | ~15 |
| `pkg/fleet/config.go` | `EffectiveEndpoint` | ~10 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|--------------------|-----------------|
| `cmd/fleetmetadatacache/main.go` | `buildFMCServers`, `runFMCServers`, `run` | ~80 |
| `pkg/fleet/scope_factory.go` | `NewScopeChecker` (FMC branch) | ~15 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|-----------------|-------|
| Code under test | `fix/1683-fmc-tls-3port-fedramp` HEAD | Branched from `main` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Control | Test ID | Status |
|-------|-------------|----------|------|---------|---------|--------|
| BR-INTEGRATION-065 | FMC server presents TLS on its API port, falling back to plain HTTP only when no cert is mounted | P0 | Unit | SC-8 | UT-FMC-1683-A-001 | Passing |
| BR-INTEGRATION-065 | FMC server presents TLS on its API port, falling back to plain HTTP only when no cert is mounted | P0 | Integration | SC-8 | IT-FMC-1683-A-001 | Passing |
| BR-INTEGRATION-065 | FMC's 3-port layout matches the Issue #753 standard (API/Health/Metrics) | P0 | Unit | AC-4 | UT-FMC-1683-A-002 | Passing |
| BR-INTEGRATION-065 | `fmc.HTTPClient.Ping()` continues to succeed against the TLS-protected API port after the port split | P0 | Integration | SC-8, AC-4 | IT-FMC-1683-A-002 | Passing |
| BR-INTEGRATION-065 | `/readyz` is served exclusively on the dedicated health port | P1 | Integration | AC-4 | IT-FMC-1683-A-003 | Passing |
| BR-INTEGRATION-065 | `fmc.HTTPClient` accepts an injectable CA-verified `*http.Client` | P0 | Unit | SC-8 | UT-FMC-1683-B-001 | Passing |
| BR-INTEGRATION-065 | `NewScopeChecker`'s FMC branch builds a CA-verified transport from `TLSCAFile` and rejects a server cert not signed by that CA | P0 | Integration | SC-8, IA-5 | IT-FMC-1683-B-001 | Passing |
| BR-INTEGRATION-065 | `EffectiveEndpoint()` emits `https://` for the auto-derived FMC backend URL | P1 | Unit | SC-8 | UT-FLEET-1683-C-001 | Passing |
| BR-INTEGRATION-065 | FMC's `TLSProfile` config field is parsed and defaults to empty (no-op) | P1 | Unit | SC-13 | UT-FMC-1683-E-001 | Passing |
| BR-INTEGRATION-065 | An `Intermediate`/`Modern` TLS profile measurably restricts FMC's accepted TLS versions/cipher suites | P0 | Integration | SC-13 | IT-FMC-1683-E-001 | Passing |
| BR-INTEGRATION-065 | FMC's Helm chart renders TLS cert mount + 3-port topology | P1 | Helm Unit | SC-8, SC-12 | HELM-FMC-1683-D-001 | Passing |
| BR-INTEGRATION-065 | FMC's E2E lane proves a real TLS handshake + cipher restriction end-to-end | P0 | E2E | SC-8, SC-13 | E2E-FMC-1683-016 | Passing |
| BR-INTEGRATION-065 | `ReadyzHandler` bounds its backend ping so a hung/slow Valkey connection can't outlive the dedicated health server's `WriteTimeout` (discovered by `E2E-FMC-054-012` regressing under Unit F's real Kind cluster: pre-#1683, `/readyz` shared the API server's `http.Server`, which had no `WriteTimeout` at all) | P0 | Unit | SI-4 | UT-FMC-API-014d | Passing |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `pkg/fleet/fmc/config`, `pkg/fleet/fmc` (client), `pkg/fleet` (config.go)

| ID | Business Outcome Under Test | Control | Phase |
|----|------------------------------|---------|-------|
| `UT-FMC-1683-A-001` | `DefaultServiceConfig()` returns the 3-port defaults (`apiAddr=:8080`, `healthAddr=:8081`, `metricsAddr=:9090`) and an empty `TLS`/`TLSProfile` (no-op until configured) | SC-8 | Passing |
| `UT-FMC-1683-A-002` | `LoadFromFile` parses `server.tls.certDir` and the three port fields independently, preserving unset defaults | AC-4 | Passing |
| `UT-FMC-1683-B-001` | `fmc.NewHTTPClient(url, WithHTTPClient(customClient))` uses the injected client instead of the zero-value default | SC-8 | Passing |
| `UT-FLEET-1683-C-001` | `FleetConfig{Backend: BackendFMC}.EffectiveEndpoint()` (no explicit `Endpoint`) returns a `https://` URL | SC-8 | Passing |
| `UT-FMC-1683-E-001` | `ServiceConfig.TLSProfile` parses from YAML and defaults to `""` (no-op) when omitted | SC-13 | Passing |
| `UT-FMC-API-014d` | `ReadyzHandler` returns 503 within its own bounded timeout (well under the health server's 10s `WriteTimeout`) even when the injected `Pinger` blocks for longer than that bound | SI-4 | Passing |

### Tier 2: Integration Tests

**Testable code scope**: `cmd/fleetmetadatacache/main.go`'s server-construction wiring, `pkg/fleet/scope_factory.go`

| ID | Business Outcome Under Test | Control | Phase |
|----|------------------------------|---------|-------|
| `IT-FMC-1683-A-001` | A real FMC API server built via the production `buildFMCServers` path, with a cert mounted, only accepts HTTPS on the API port (plaintext connection fails/upgrades) | SC-8 | Passing |
| `IT-FMC-1683-A-002` | `fmc.HTTPClient.Ping()` succeeds against the TLS-protected API port's dual-registered `/healthz`, unmodified from its pre-split behavior | SC-8, AC-4 | Passing |
| `IT-FMC-1683-A-003` | `/readyz` returns 404 on the API port and 200/503 (dependency-aware) on the dedicated health port | AC-4 | Passing |
| `IT-FMC-1683-B-001` | `NewScopeChecker` with `Backend: BackendFMC` and `TLSCAFile` set builds an `HTTPClient` whose requests succeed against a server presenting a cert signed by that CA, and fail (certificate verification error) against a server presenting a self-signed cert not in that CA's chain | SC-8, IA-5 | Passing |
| `IT-FMC-1683-E-001` | A real FMC API server configured with `TLSProfile=Intermediate` rejects a client `tls.Config` restricted to `MaxVersion: tls.VersionTLS11` (below the profile's floor) with a handshake failure, and accepts a client offering TLS 1.2+ with an allowed cipher | SC-13 | Passing |

### Tier 3: E2E Tests

**Testable code scope**: `test/e2e/fleetmetadatacache/` (Kuadrant lane) and
`test/e2e/fleetmetadatacache/eaigw/` (Envoy AI Gateway lane); the scenario lives in the
gateway-agnostic `shared` package and is wired into both lanes, proving the full production
topology (Kind-deployed FMC pod, real cert-manager-equivalent leaf cert issued by the E2E
inter-service CA, real client from the test harness) under both supported gateway backends.

| ID | Business Outcome Under Test | Control | Phase |
|----|------------------------------|---------|-------|
| `E2E-FMC-1683-016` | FMC's real deployed pod serves its API over HTTPS with a cert trusted by the E2E inter-service CA; the harness's HTTP client (CA-aware) completes real scope-check/cluster-list calls over TLS; `/readyz` is reachable only on the dedicated health NodePort, no longer on the API NodePort | SC-8, SC-13 | Passing (Kuadrant lane: 14/14 specs; EAIGW lane: 14/14 specs) |

### Tier Skip Rationale

None — all three tiers apply.

---

## 9. Test Cases

Representative P0 cases (remaining P1 cases follow the same Given/When/Then structure; see
inline Ginkgo `It` descriptions in the referenced files for full detail).

### IT-FMC-1683-A-002: `Ping()` survives the port split

**BR**: BR-INTEGRATION-065
**Priority**: P0
**Type**: Integration
**File**: `cmd/fleetmetadatacache/main_wiring_test.go` (new)

**Preconditions**: A self-signed TLS cert pair is written to a temp `certDir`; `ServiceConfig`
points `Server.TLS.CertDir` at it.

**Test Steps**:
1. **Given**: `buildFMCServers` constructs the API/health/metrics servers from a `ServiceConfig`
   with TLS configured, exactly as `cmd/fleetmetadatacache/main.go`'s `run()` does.
2. **When**: `fmc.NewHTTPClient(apiBaseURL, fmc.WithHTTPClient(caTrustingClient)).Ping(ctx)` is called.
3. **Then**: `Ping()` returns `nil` (200 OK from the dual-registered `/healthz` on the API mux).

**Acceptance Criteria**: `Ping()`'s call signature, base URL, and success contract are
byte-for-byte unchanged from before the port split.

**Dependencies**: Unit A GREEN complete.

### IT-FMC-1683-E-001: FedRAMP profile rejects a downgraded handshake

**BR**: BR-INTEGRATION-065
**Priority**: P0
**Type**: Integration
**File**: `cmd/fleetmetadatacache/main_wiring_test.go` (new)

**Preconditions**: `sharedtls.SetDefaultSecurityProfileFromConfig("Intermediate")` called before
server construction (matching `run()`'s startup order); reset via `ResetDefaultSecurityProfileForTesting` in `AfterEach`.

**Test Steps**:
1. **Given**: A real FMC API server listening with the Intermediate profile applied (TLS 1.2 floor, AEAD-only ciphers).
2. **When**: A client `tls.Config{MaxVersion: tls.VersionTLS11}` attempts a handshake.
3. **Then**: The handshake fails with a protocol-version error.
4. **When**: A client `tls.Config{MinVersion: tls.VersionTLS12}` (default Go cipher set) attempts a handshake.
5. **Then**: The handshake succeeds.

**Acceptance Criteria**: The profile measurably changes accepted/rejected TLS versions — proves
FedRAMP cipher restriction is live, not just configured.

**Dependencies**: Unit E GREEN complete.

---

## 10. Environmental Needs

### 10.1 Unit Tests
- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: none (pure config/option logic)
- **Location**: `pkg/fleet/fmc/config/`, `pkg/fleet/fmc/`, `pkg/fleet/`

### 10.2 Integration Tests
- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: ZERO — real `net/http`/`crypto/tls` listeners, self-signed certs generated in-test
- **Location**: `cmd/fleetmetadatacache/`, `test/integration/fleetmetadatacache/`, `pkg/fleet/`

### 10.3 E2E Tests
- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: Kind cluster (existing FMC E2E lane), real leaf cert from `GenerateInterServiceTLS`

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — all referenced infrastructure (`sharedtls`, `sharedhealth`, `hotreload.FileWatcher`,
`GenerateInterServiceTLS`) already exists and is used by DataStorage/Gateway today.

### 11.2 Execution Order

1. **Unit A** (RED/GREEN/REFACTOR): server TLS + 3-port split + dual-registered `/healthz`.
2. **Unit B** (RED/GREEN/REFACTOR): client TLS (`WithHTTPClient` + `scope_factory.go` wiring).
3. **Unit C** (RED/GREEN): `EffectiveEndpoint()` scheme change.
4. **Unit E** (RED/GREEN): FedRAMP `TLSProfile` field + wiring.
5. **Unit D** (RED/GREEN): Helm chart parity.
6. **Unit F** (RED/GREEN): E2E lane alignment.
7. **Verification**: build/lint/test/helm gates.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|--------------|
| This test plan | `docs/testing/1683/TEST_PLAN.md` | Strategy and test design |
| Unit tests | `pkg/fleet/fmc/config/config_test.go`, `pkg/fleet/fmc/http_client_test.go`, `pkg/fleet/config_test.go` | Ginkgo BDD |
| Integration tests | `cmd/fleetmetadatacache/main_wiring_test.go` (new), `pkg/fleet/scope_factory_test.go` | Ginkgo BDD |
| Helm unit tests | `charts/kubernaut/tests/fleetmetadatacache_test.yaml` (new), `charts/kubernaut/tests/interservice_mtls_test.yaml` (extended) | helm-unittest |
| E2E test | `test/e2e/fleetmetadatacache/shared/tls_test.go` or extension of `resilience.go` | Ginkgo BDD |

---

## 13. Execution

```bash
# Unit + Integration
go test ./pkg/fleet/... ./cmd/fleetmetadatacache/... -ginkgo.v

# Helm
helm lint charts/kubernaut
helm template charts/kubernaut --set fleetmetadatacache.enabled=true
helm unittest charts/kubernaut

# E2E (existing FMC lane)
ginkgo -v ./test/e2e/fleetmetadatacache/...
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|--------------|------------|-----------|--------|
| `sharedtls.ConfigureConditionalTLS` on FMC API server | `cmd/fleetmetadatacache/main.go: run()` | HTTPS response on API port | `IT-FMC-1683-A-001` | Passing |
| Dual-registered `/healthz` handler | `buildFMCServers` (API mux + health mux) | 200 OK on both ports | `IT-FMC-1683-A-002` | Passing |
| `sharedhealth.NewHealthServer` dedicated health server | `buildFMCServers` | `/healthz`+`/readyz` on health port | `IT-FMC-1683-A-003` | Passing |
| `fmc.WithHTTPClient` -> `scope_factory.go` FMC branch CA transport | `fleet.NewScopeChecker` | CA-verified `HTTPClient` request | `IT-FMC-1683-B-001` | Passing |
| `sharedtls.SetDefaultSecurityProfileFromConfig(cfg.TLSProfile)` | `cmd/fleetmetadatacache/main.go: run()` | Restricted TLS handshake | `IT-FMC-1683-E-001` | Passing |

**Unit tests do NOT count as wiring proof.** All rows above are proven by integration tests
that traverse the real `main.go`/`scope_factory.go` production code paths.

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|----------------------|--------------------|-------------------|--------|
| `pkg/fleet/fmc/config/config_test.go` (`UT-FMC-CFG-001`, `UT-FMC-CFG-003`, `UT-FMC-CFG-004`) | Asserts `cfg.Server.APIAddr`/`MetricsAddr` as the only two server fields, `MetricsAddr` defaulting to `:8081` | Add `HealthAddr` assertions; `MetricsAddr` default changes from `:8081` to `:9090` (3-port standard) | `ServerConfig` gains a third port; `MetricsAddr`'s default value moves to make room for `HealthAddr:8081` |
| `test/e2e/fleetmetadatacache/shared/resilience.go` (`ReadyzStatus`) | Polls `/readyz` via `h.FMCAPIBaseURL` (API port) | Poll via a new `h.FMCHealthBaseURL` (dedicated health port) | `/readyz` moves exclusively to the health port (Design Decision, Section 4.3) |
| `test/e2e/fleetmetadatacache/suite_test.go` + `eaigw/suite_test.go` | `fmcAPIBaseURL = "http://localhost:8150"` | Scheme changes to `https://`; add a new `fmcHealthBaseURL` constant + NodePort | FMC's API port now serves TLS; health port needs its own E2E NodePort exposure |
| `test/infrastructure/fleet_e2e.go` (`deployValkeyAndFMC`), `fleetmetadatacache_e2e.go` (NodePort exposure), Kind configs | 2-port layout (`apiAddr`, `metricsAddr`), single API NodePort, readinessProbe on API port hitting `/healthz` | 3-port layout, TLS cert mount, second NodePort for health, readinessProbe on health port hitting `/readyz` | E2E infra is Go-generated (not Helm) and must be updated in lockstep (Risk R2) |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-07-24 | Initial test plan |
