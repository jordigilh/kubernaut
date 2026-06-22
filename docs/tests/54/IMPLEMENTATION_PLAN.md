# Implementation Plan: Fleet Federation — FMC HTTP API & Client Migration

**Issue**: #54
**ADR**: [ADR-068-fleet-federation-architecture](../../architecture/decisions/ADR-068-fleet-federation-architecture.md)
**Branch**: `feature/multi-cluster-federation`
**Created**: 2026-06-22

---

## Overview

GW and RO currently connect to Valkey directly for federated scope checks. This violates the ADR-068 architecture: only FMC should know about Valkey. This plan migrates GW/RO to query FMC's HTTP API, then removes the legacy direct Valkey path entirely.

### Current State

```
GW/RO → ValkeyCacheReader → Valkey     (direct, violates ADR-068)
FMC   → ValkeyWriter      → Valkey     (write-only, no read API)
```

### Target State

```
GW/RO → fmc.HTTPClient → FMC HTTP API → ValkeyCacheReader → Valkey
FMC   → ValkeyWriter   → Valkey
```

### Completed Prior Work

| Item | Status |
|------|--------|
| Unified `scope.ScopeChecker` interface + `ResourceIdentity` | Complete |
| `FederatedScopeChecker` moved to `pkg/fleet/`, decoupled from Valkey | Complete |
| Factory centralized in `pkg/fleet/scope_factory.go` | Complete |
| `scopecache.Client` implements `scope.ScopeChecker` | Complete |
| Helm `values.yaml` updated with `backend`/`endpoint` fields | Complete |
| Operator team issue (#180 on kubernaut-operator) | Complete |
| Factory unit tests (`pkg/fleet/scope_factory_test.go`) | Complete |

---

## FedRAMP Control Mapping

Tests in this plan provide behavioral assurance for the following NIST 800-53 controls:

| Control | Title | Relevance |
|---------|-------|-----------|
| **AC-4** | Information Flow Enforcement | Scope check API is the enforcement point: GW/RO can only act on resources FMC reports as managed. IT tests must prove GW/RO respects the response. |
| **SC-7** | Boundary Protection | FMC defines the managed-resource boundary across clusters. Fail-safe behavior (error → unmanaged) ensures conservative boundary under failure. |
| **SC-8** | Transmission Confidentiality and Integrity | FMC HTTP client must support TLS for the GW/RO → FMC connection. (Deferred to Helm/deployment; noted as gap.) |
| **SI-10** | Information Input Validation | FMC API validates required parameters before processing scope queries. Adversarial input must not cause panics or incorrect scope decisions. |
| **CM-6** | Configuration Settings | Cluster listing API enables audit of which clusters are federated. Factory configuration determines which backend adapter is used. |

---

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT/E2E Test ID | Status |
|-----------|----------------------|---------------------|------------|--------|
| `fmc.Handler` | `fmc.NewHandler(scopeClient, clusterRegistry, logger)` | `cmd/fmc/main.go:135` | UT-FMC-API-001, **IT-FMC-054-001** | PASS |
| `fmc.Handler.RegisterRoutes` | `apiHandler.RegisterRoutes(apiMux)` | `cmd/fmc/main.go:137` | UT-FMC-API-001, **IT-FMC-054-001** | PASS |
| API server (`:8080`) | `apiServer.ListenAndServe()` | `cmd/fmc/main.go` | **IT-FMC-054-001** | PASS |
| `ValkeyCacheReader.Ping` via `ReadyzHandler` | `/readyz` handler | `cmd/fmc/main.go:142` | **UT-FMC-API-014/014b/014c** | PASS |
| `fmc.HTTPClient` | `fmc.NewHTTPClient(endpoint)` | `pkg/fleet/scope_factory.go:52` | **IT-FMC-054-010** | PASS |
| `registry.CRDWatcher.Get` guard | `registry.NewCRDWatcher(dynClient, ...)` | `cmd/fmc/main.go:111`, `pkg/fleet/fmc/handler.go:89` | **IT-FMC-054-020** | PASS |
| Factory `BackendFMC` → HTTP | `fleet.NewScopeChecker(...)` | `pkg/fleet/scope_factory.go:39` | UT-SF-054-001, **E2E-FMC-054-001** | PASS |
| `FederatedScopeChecker` | `NewFederatedScopeChecker(local, remote, logger)` | `pkg/fleet/scope_factory.go:53` | **E2E-FMC-054-001** | PASS |
| `scopecache.Client` | `scopecache.NewClient(cacheReader)` | `cmd/fmc/main.go` | IT-FLEET-VALKEY-004, **E2E-FMC-054-001** | PASS |
| Remove `BackendValkey` | Factory no longer accepts `"valkey"` | `pkg/fleet/scope_factory.go` | UT-SF-054-002 | PASS |

**Pyramid Invariant coverage** (added 2026-06-22): IT-FMC-054-001/010/020 prove HTTP wiring through real Valkey + real envtest CRDWatcher. E2E-FMC-054-001 proves the full factory-to-Valkey journey. Test files: `test/integration/fmc/fmc_http_api_test.go`, `test/integration/fmc/fmc_e2e_test.go`.

---

## Phase 1: FMC HTTP API (COMPLETE)

**Goal**: FMC serves scope check and cluster listing over HTTP.

### Phase 1 Summary

| Sub-phase | Deliverable | Status |
|-----------|-------------|--------|
| 1A | `GET /api/v1/scope/check` handler | Complete |
| 1B | `GET /api/v1/clusters` handler | Complete |
| 1C | Dual HTTP server in `cmd/fmc/main.go` (API `:8080` + metrics `:8081`) | Complete |
| 1D | 12 handler unit tests | Complete |

### Files Created/Modified

| File | Change |
|------|--------|
| `pkg/fleet/fmc/handler.go` | New: Handler with scope check + cluster listing endpoints |
| `pkg/fleet/fmc/handler_test.go` | New: 12 unit tests using `httptest` |
| `pkg/fleet/scopecache/valkey_reader.go` | Added `Ping()` for readiness checks |
| `cmd/fmc/main.go` | Dual HTTP server, `FMC_API_ADDR` config, wired `fmc.Handler` |

### Test Inventory

| Test ID | Description | Control |
|---------|-------------|---------|
| UT-FMC-API-001 [AC-4] | Scope check returns managed=true for in-scope resource — proves API emits correct flow decision | AC-4 |
| UT-FMC-API-002 [AC-4] | Scope check returns managed=false for out-of-scope resource — proves boundary excludes unknown resources | AC-4, SC-7 |
| UT-FMC-API-003..005 [SI-10] | Required parameters (cluster, kind, name) validated — rejects incomplete scope queries | SI-10 |
| UT-FMC-API-006 [SI-10] | Non-GET method rejected on scope check endpoint | SI-10 |
| UT-FMC-API-007 [SC-7] | Cache error falls back to managed=false — boundary is conservative under failure | SC-7 |
| UT-FMC-API-008 [AC-4] | Core group resources (empty group) queryable — no false negatives for core API resources | AC-4 |
| UT-FMC-API-010 [CM-6] | Empty cluster list when none registered — configuration audit reflects actual state | CM-6 |
| UT-FMC-API-011 [CM-6] | Cluster list returns registered clusters — federated configuration is auditable | CM-6 |
| UT-FMC-API-012 [SI-10] | Non-GET method rejected on cluster listing endpoint | SI-10 |
| UT-FMC-API-013 [SI-10] | Response Content-Type is application/json — well-formed API contract | SI-10 |

### Gaps Identified

| Gap | Control | Resolution | Status |
|-----|---------|------------|--------|
| No adversarial input testing (injection, overflow) | SI-10 | UT-FMC-API-015, UT-FMC-API-016 added | **Closed** |
| No authn/authz on FMC API endpoints | AC-3 | Deferred: FMC runs in-cluster behind NetworkPolicy; mTLS planned for Helm phase | Open |
| AC-4 enforcement not proven end-to-end | AC-4 | IT-FMC-054-001/010 + E2E-FMC-054-001 prove GW/RO -> FMC -> Valkey path | **Closed** |

### Phase 1 Remaining: REFACTOR — Close SI-10 Gap (COMPLETE)

Adversarial input tests added to `handler_test.go`:

| Test ID | Description | Control | Status |
|---------|-------------|---------|--------|
| UT-FMC-API-015 [SI-10] | Excessively long cluster param does not panic or produce incorrect result | SI-10 | PASS |
| UT-FMC-API-016 [SI-10] | Special characters in name param handled safely | SI-10 | PASS |

---

## Phase 2: FMC HTTP Client Adapter

**Goal**: Create an HTTP client that GW/RO use to query FMC, replacing direct Valkey access.

### Phase 2 RED — Failing Tests

**File**: `pkg/fleet/fmc/http_client_test.go`

| Test ID | Description | Why it fails | Control |
|---------|-------------|-------------|---------|
| UT-FMC-HC-001 [AC-4] | `IsManagedResource` returns true when FMC responds `{"managed":true}` | `HTTPClient` type doesn't exist | AC-4 |
| UT-FMC-HC-002 [AC-4] | `IsManagedResource` returns false when FMC responds `{"managed":false}` | Same | AC-4 |
| UT-FMC-HC-003 [SC-7] | `IsManagedResource` returns false on HTTP error (fail-safe) | Same | SC-7 |
| UT-FMC-HC-004 [SC-7] | `IsManagedResource` returns false on non-200 response (fail-safe) | Same | SC-7 |
| UT-FMC-HC-005 [SC-7] | `IsManagedResource` returns false on malformed JSON (fail-safe) | Same | SC-7 |
| UT-FMC-HC-006 [SI-10] | Query parameters are URL-encoded correctly | Same | SI-10 |

**File**: `pkg/fleet/fmc/http_client_test.go` (compile-time)

| Test ID | Description | Why it fails | Control |
|---------|-------------|-------------|---------|
| UT-FMC-HC-007 | `HTTPClient` satisfies `scope.ScopeChecker` interface | Same | — |

### Phase 2 GREEN — Minimal Implementation

**File**: `pkg/fleet/fmc/http_client.go` (new)

```go
type HTTPClient struct {
    baseURL    string
    httpClient *http.Client
}

func NewHTTPClient(baseURL string, opts ...HTTPClientOption) *HTTPClient

func (c *HTTPClient) IsManagedResource(ctx context.Context, r scope.ResourceIdentity) (bool, error)
```

Implementation:
1. Build URL: `{baseURL}/api/v1/scope/check?cluster=...&group=...&version=...&kind=...&namespace=...&name=...`
2. HTTP GET with context
3. Decode `ScopeCheckResponse`
4. On any error (network, non-200, decode) → return `(false, nil)` (fail-safe, matching SC-7)

**Tests passing after**: UT-FMC-HC-001 through UT-FMC-HC-007

### Phase 2 GREEN — Update Factory

**File**: `pkg/fleet/scope_factory.go`

Change `BackendFMC` branch from:
```go
reader := scopecache.NewValkeyCacheReader(endpoint)
remoteChecker := scopecache.NewClient(reader)
```
To:
```go
remoteChecker := fmc.NewHTTPClient(endpoint)
```

**File**: `pkg/fleet/scope_factory_test.go`

| Test ID | Description | Control |
|---------|-------------|---------|
| UT-SF-054-001 [AC-4] | `BackendFMC` factory creates `fmc.HTTPClient` (not Valkey reader) — proves production path uses HTTP | AC-4 |

### Phase 2 REFACTOR

- Extract `fail-safe` logging into shared helper if pattern repeats
- Ensure `HTTPClient` respects `http.Client.Timeout`

### Phase 2 Checkpoint

- [x] All UT-FMC-HC tests PASS (GREEN)
- [x] Factory UT-SF-054-001 PASS
- [x] `go build ./...` succeeds
- [x] All existing fleet tests still pass (no regressions)
- [x] CHECKPOINT W: `fmc.NewHTTPClient` called in `scope_factory.go` (production path)

---

## Phase 3: Remove Legacy Direct Valkey Path

**Goal**: `BackendValkey` and `ValkeyAddr` removed from GW/RO config. `scopecache` becomes FMC-internal.

### Phase 3 RED — Failing Tests

**File**: `pkg/fleet/scope_factory_test.go`

| Test ID | Description | Why it fails | Control |
|---------|-------------|-------------|---------|
| UT-SF-054-002 [CM-6] | `BackendValkey` returns error ("removed, use fmc") | Currently succeeds with Valkey path | CM-6 |
| UT-SF-054-003 [CM-6] | `ValkeyAddr` fallback removed — empty endpoint returns error | Currently falls back to ValkeyAddr | CM-6 |

### Phase 3 GREEN — Remove Legacy Code

**Note**: FMC's own `valkeyAddr` (`charts/kubernaut/values.yaml` fmcWriter section, `cmd/fmc/main.go`) is **kept** — FMC legitimately owns its Valkey connection. Only GW/RO references are removed.

| # | File | Change |
|---|------|--------|
| 1 | `pkg/fleet/config.go` | Remove `ValkeyAddr` field, `EffectiveEndpoint` fallback, `effectiveBackend` ValkeyAddr fallback |
| 2 | `pkg/fleet/scope_factory.go` | Remove `BackendValkey` case; remove `scopecache` import |
| 3 | `pkg/fleet/scope_factory_test.go` | Remove UT-FLEET-FAC-004 (BackendValkey), UT-FLEET-FAC-007 (ValkeyAddr legacy) |
| 4 | `pkg/fleet/fleet_test.go` | Update UT-FLEET-CFG-001..003 to use `Backend`+`Endpoint` instead of `ValkeyAddr` |
| 5 | `charts/kubernaut/values.yaml` | Remove `valkeyAddr` from `gateway.fleet` and `remediationorchestrator.fleet` |
| 6 | `charts/kubernaut/values.schema.json` | Remove `valkeyAddr` property from GW and RO fleet sections (keep fmcWriter section) |
| 7 | `charts/kubernaut/templates/gateway/gateway.yaml` | Remove `valkeyAddr` conditional rendering |
| 8 | `charts/kubernaut/templates/remediationorchestrator/remediationorchestrator.yaml` | Remove `valkeyAddr` conditional rendering |
| 9 | `test/infrastructure/fleet_e2e.go` | Replace `ValkeyAddr` with `Endpoint` in `FleetE2EConfig` |

### Phase 3 REFACTOR

- Verify `scopecache` package has no imports outside `pkg/fleet/fmc/` and `cmd/fmc/`
- Update ADR-068 migration status

### Phase 3 Checkpoint

- [x] UT-SF-054-002, UT-SF-054-003 PASS
- [x] `go build ./...` succeeds
- [x] `helm template` succeeds without `valkeyAddr`
- [x] No `scopecache` imports from `pkg/fleet/scope_factory.go`
- [x] ADR-068 migration status updated

---

## Phase 4: ClusterID Validation

**Goal**: FMC validates `ClusterID` against its cluster registry before performing scope checks, rejecting queries for unknown clusters.

### Preflight Finding

GW and RO have **zero access** to `registry.ClusterRegistry` — the registry only exists in `cmd/fmc/`. Therefore, ClusterID validation cannot live in `FederatedScopeChecker` (which runs in GW/RO). It must be **server-side in the FMC handler**, where the registry is available. This is architecturally correct: FMC is the source of truth for known clusters.

### Phase 4 RED — Failing Tests

**File**: `pkg/fleet/fmc/handler_test.go`

| Test ID | Description | Why it fails | Control |
|---------|-------------|-------------|---------|
| UT-FMC-API-020 [AC-4] | Scope check with unknown ClusterID returns managed=false without cache lookup | Handler does not validate ClusterID against registry | AC-4 |
| UT-FMC-API-021 [AC-4] | Scope check with known ClusterID proceeds to cache lookup | Same | AC-4 |
| UT-FMC-API-022 [CM-6] | Cluster list reflects registry state — unknown cluster not in list | Already passes (regression guard) | CM-6 |

### Phase 4 GREEN

**File**: `pkg/fleet/fmc/handler.go`

In `handleScopeCheck`, after parameter validation and before calling `h.checker.IsManagedResource`:

```go
if _, known := h.registry.Get(resource.ClusterID); !known {
    h.logger.V(1).Info("scope check for unknown cluster, returning unmanaged",
        "cluster", resource.ClusterID)
    writeJSON(w, http.StatusOK, ScopeCheckResponse{Managed: false})
    return
}
```

No changes to `FederatedScopeChecker`, factory, or GW/RO code.

**Tests passing after**: UT-FMC-API-020, UT-FMC-API-021, UT-FMC-API-022

### Phase 4 REFACTOR

- Structured logging at V(1) for rejected unknown clusters (avoids log spam in production)
- Consider returning a distinct JSON field (e.g., `"unknown_cluster": true`) for observability

### Phase 4 Checkpoint

- [x] UT-FMC-API-020..022 PASS
- [x] All existing FMC handler tests still pass (no regressions)
- [x] All existing fleet tests still pass
- [x] `go build ./...` succeeds

---

## Phase 5: ACM Search Adapter

**Goal**: Implement ACM Search GraphQL adapter implementing `scope.ScopeChecker`.

**Confidence**: ~55% (BLOCKED — below 90% threshold)

**Blocker**: The GraphQL contract documented in ADR-068 is based on ACM documentation but has never been validated against a live ACM Search instance. No spike exists, no GraphQL client dependency in go.mod, no test environment confirmed. Building against unvalidated docs risks silent incorrect scope decisions.

### Phase 5 Prerequisite: Spike — Validate ACM Search Contract

**Environment**: OCP 4.21 cluster (available)

| Step | Task |
|------|------|
| S1 | Install ACM Search GraphQL plugin on OCP 4.21 cluster |
| S2 | Send the `scopeCheck` GraphQL query from ADR-068 against the live endpoint |
| S3 | Confirm response shape matches ADR-068 contract (field names, nesting, types) |
| S4 | Confirm `searchComplete` query returns valid cluster list |
| S5 | Test auth flow: SA bearer token creation, RBAC, TLS CA trust |
| S6 | Evaluate GraphQL client options for Go (hand-rolled HTTP+JSON vs library) |
| S7 | Document findings in `docs/spikes/multi-cluster-mcp-gateway/spike-acm-search/` |

**Exit criteria**: Spike raises confidence to 90%+. If the contract differs from ADR-068, update the ADR before proceeding.

### Phase 5 RED — Failing Tests (after spike passes)

**File**: `pkg/fleet/acm/client_test.go`

| Test ID | Description | Why it fails | Control |
|---------|-------------|-------------|---------|
| UT-ACM-054-001 [AC-4] | `IsManagedResource` returns true for ACM-managed resource | `acm.Client` doesn't exist | AC-4 |
| UT-ACM-054-002 [AC-4] | `IsManagedResource` returns false for unmanaged resource | Same | AC-4 |
| UT-ACM-054-003 [SC-7] | GraphQL error falls back to unmanaged (fail-safe) | Same | SC-7 |
| UT-ACM-054-004 | `acm.Client` satisfies `scope.ScopeChecker` interface | Same | — |

### Phase 5 GREEN

**File**: `pkg/fleet/acm/client.go` (new package)

```go
type Client struct {
    endpoint   string
    httpClient *http.Client
}

func NewClient(endpoint string) *Client
func (c *Client) IsManagedResource(ctx context.Context, r scope.ResourceIdentity) (bool, error)
```

**File**: `pkg/fleet/scope_factory.go`

Update `BackendACM` case from error to `acm.NewClient(endpoint)`.

### Phase 5 Checkpoint

- [ ] Spike documented with validated contract
- [ ] UT-ACM-054-001..004 PASS
- [ ] Factory `BackendACM` path works
- [ ] `go build ./...` succeeds

---

## Phase Execution Order

| Phase | Depends On | Confidence | Status | Key Deliverable |
|-------|-----------|-----------|--------|-----------------|
| **1** | — | 92% | **Complete** | FMC HTTP API serving scope checks |
| **2** | Phase 1 | 95% | **Complete** | HTTP client adapter; factory uses HTTP instead of Valkey |
| **3** | Phase 2 | 93% | **Complete** | Legacy Valkey path removed; `scopecache` internal to FMC (9 files) |
| **4** | Phase 1 | 92% | **Complete** | Server-side ClusterID validation in FMC handler |
| **5** | Phase 5 Spike (90%+) | 55% (BLOCKED) | Pending | ACM Search adapter for non-FMC environments |

Phases 1-4 are complete. Phase 5 is blocked on its spike reaching 90%+ confidence.

---

## Acceptance Criteria

1. GW/RO connect to FMC over HTTP, never to Valkey directly
2. `BackendValkey` and `ValkeyAddr` removed from GW/RO configuration
3. All scope check failures fall back to unmanaged (SC-7 fail-safe at every layer)
4. Every new component has a production caller verified by CHECKPOINT W
5. FedRAMP controls AC-4, SC-7, SI-10, CM-6 have behavioral test assurance
6. `go build ./...` and all fleet tests pass at every phase boundary
