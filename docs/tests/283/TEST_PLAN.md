# Test Plan: Standardize Metrics Port to :9090

**Feature**: Standardize Prometheus metrics to :9090 for DataStorage and Notification services
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `release/v1.1.0-rc2`

**Authority**:
- [BR-STORAGE-019]: Logging and metrics (GAP-10: Audit-specific metrics in handlers)
- [BR-NOT-054]: Controller metrics exposure for observability
- [DD-TEST-001]: Port allocation strategy (production vs test infrastructure)

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Issue #283](https://github.com/jordigilh/kubernaut/issues/283)
- [DD-TEST-001 Port Allocation](../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md)

---

## 1. Scope

### In Scope

- **DataStorage metrics server**: New dedicated HTTP listener on configurable `metricsPort` (default 9090) serving `/metrics` via `promhttp.Handler()`
- **DataStorage config**: New `metricsPort` field in `ServerConfig` with default 9090 and validation
- **DataStorage graceful shutdown**: Metrics server included in graceful shutdown path
- **Notification Helm chart**: ConfigMap `metricsAddr` change from `:9186` to `:9090` (code default already 9090)
- **Helm chart alignment**: Prometheus annotations, container ports, and service ports updated to 9090

### Out of Scope

- HolmesGPT API (Python): Metrics remain on application port 8080 due to framework constraints
- Test infrastructure (`test/infrastructure/`): Kind port allocations stay per DD-TEST-001 (unique per-service ports for collision avoidance)
- DataStorage main router `/metrics` endpoint: Kept for backward compatibility with existing E2E tests
- Controller-based services (Gateway, SP, RO, WE, EM, AA): Already on 9090

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Keep `/metrics` on both DataStorage main router (8080) and dedicated server (9090) | E2E tests (`test/e2e/datastorage/17_metrics_api_test.go`) scrape `dataStorageURL + "/metrics"` on the API port; removing it breaks tests outside scope |
| Notification is Helm-only (no Go code changes) | Go code default is already `:9090` (`pkg/notification/config/config.go` L172); only the Helm ConfigMap overrides to `:9186` |
| Default `metricsPort` to 9090 when field is 0 | Follows controller-runtime convention where all Kubernaut controllers default to `:9090` |
| No changes to test infrastructure Kind configs | DD-TEST-001 assigns unique per-service ports to avoid collisions; production standardization is independent |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of new unit-testable code (config parsing, default application, validation)
- **Integration**: Not applicable (no new I/O wiring beyond startup -- validated by E2E)

### 2-Tier Minimum

- **Unit tests**: Validate config defaults, metrics server startup, graceful shutdown
- **Build validation**: `go build ./...` ensures compile-time correctness
- Notification changes are Helm-only (no testable Go code changes)

### Business Outcome Quality Bar

Tests validate that:
1. An operator deploying Kubernaut gets Prometheus metrics on a standard port (9090) across all Go services
2. The DataStorage metrics server responds to `GET /metrics` with valid Prometheus exposition format
3. The DataStorage metrics server shuts down gracefully without blocking the main server
4. Config defaults ensure 9090 is used unless explicitly overridden

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/config/config.go` | `ServerConfig.MetricsPort` field, default application, validation | ~15 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `cmd/datastorage/main.go` | Metrics server goroutine, graceful shutdown | ~20 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-STORAGE-019 | DataStorage exposes Prometheus metrics on dedicated :9090 port | P0 | Unit | UT-DS-283-001 | Pending |
| BR-STORAGE-019 | DataStorage metrics port defaults to 9090 when not configured | P0 | Unit | UT-DS-283-002 | Pending |
| BR-STORAGE-019 | DataStorage metrics port rejects invalid values | P0 | Unit | UT-DS-283-003 | Pending |
| BR-NOT-054 | Notification metrics port standardized to 9090 in Helm | P0 | — | (Helm-only, no Go test) | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: DS (DataStorage), NOT (Notification)
- **BR_NUMBER**: 283 (Issue number)
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `pkg/datastorage/config/config.go` -- config parsing, default application, validation for the new `metricsPort` field.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-283-001` | Operator gets metrics on a dedicated port: when `metricsPort: 9090` is configured, `ServerConfig.MetricsPort` returns 9090 | RED |
| `UT-DS-283-002` | Zero-config works: when `metricsPort` is omitted from YAML, default 9090 is applied so operators always get metrics on the standard port | RED |
| `UT-DS-283-003` | Misconfiguration is caught: when `metricsPort` is set to a value outside 1-65535, validation returns an actionable error | RED |

### Tier Skip Rationale

- **Integration**: SKIPPED -- The DataStorage metrics server is a simple `http.ListenAndServe` with `promhttp.Handler()`. There is no complex wiring or cross-component interaction to validate at the integration tier. The E2E test (`test/e2e/datastorage/17_metrics_api_test.go`) already validates metrics scraping from a running DataStorage instance and provides defense-in-depth. The startup wiring is validated by `go build ./...` and will be exercised by the existing E2E suite.
- **E2E**: SKIPPED for new tests -- Existing E2E test `17_metrics_api_test.go` already validates `/metrics` returns Prometheus format data from a running DataStorage. It scrapes the API port (unchanged), providing indirect coverage. A dedicated E2E test for port 9090 would require Kind config changes (out of scope).
- **Notification**: SKIPPED entirely -- No Go code changes. The Helm ConfigMap value change from `:9186` to `:9090` is not testable in Go unit/integration tests. The existing E2E test `test/e2e/notification/05_metrics_validation_test.go` validates metrics scraping and will validate the new port in future Kind-based E2E runs.

---

## 6. Test Cases (Detail)

### UT-DS-283-001: Config parses metricsPort from YAML

**BR**: BR-STORAGE-019
**Type**: Unit
**File**: `test/unit/datastorage/config_test.go`

**Given**: A YAML config string with `server.metricsPort: 9090`
**When**: `LoadFromFile` parses the config (or YAML is unmarshaled into `ServerConfig`)
**Then**: `cfg.Server.MetricsPort` equals 9090

**Acceptance Criteria**:
- `cfg.Server.MetricsPort` == 9090 (exact integer match)
- Existing fields (`Port`, `Host`, `ReadTimeout`, `WriteTimeout`) are unaffected

---

### UT-DS-283-002: Default metricsPort applied when omitted

**BR**: BR-STORAGE-019
**Type**: Unit
**File**: `test/unit/datastorage/config_test.go`

**Given**: A YAML config string that does NOT include `metricsPort` (field is zero-value)
**When**: Config is loaded and defaults are applied
**Then**: `cfg.Server.MetricsPort` equals 9090 (standard Kubernaut convention)

**Acceptance Criteria**:
- `cfg.Server.MetricsPort` == 9090 after default application
- Validates the business outcome: operator deploying without explicit metrics config still gets standard-port metrics

---

### UT-DS-283-003: Invalid metricsPort rejected with actionable error

**BR**: BR-STORAGE-019
**Type**: Unit
**File**: `test/unit/datastorage/config_test.go`

**Given**: A config with `server.metricsPort` set to an invalid value (e.g., -1, 0 after defaults, 70000)
**When**: `Validate()` is called
**Then**: An error is returned containing "metricsPort" and the invalid value for operator troubleshooting

**Acceptance Criteria**:
- `Validate()` returns non-nil error for port < 1 (after defaults) or port > 65535
- Error message contains "metricsPort" substring for grep-ability in operator logs
- Error message contains the invalid value for diagnosis

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None required (pure config parsing logic)
- **Location**: `test/unit/datastorage/config_test.go` (extend existing or create)

---

## 8. Execution

```bash
# Unit tests (DataStorage config)
go test ./test/unit/datastorage/... --ginkgo.focus="UT-DS-283"

# Build validation (all services)
go build ./...
```

---

## 9. Anti-Pattern Compliance

Per TESTING_GUIDELINES.md, this test plan avoids:

| Anti-Pattern | Compliance |
|-------------|------------|
| NULL-TESTING (`Expect(x).ToNot(BeNil())`) | All assertions validate specific business outcomes (exact port value, error message content) |
| time.Sleep() | No async operations; config parsing is synchronous |
| Skip() | No conditional skips; all tests execute unconditionally |
| Direct infrastructure testing | Tests validate config parsing behavior, not Prometheus client internals |
| IMPLEMENTATION-TESTING | Tests validate "operator gets metrics on port 9090", not "function X is called" |

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
