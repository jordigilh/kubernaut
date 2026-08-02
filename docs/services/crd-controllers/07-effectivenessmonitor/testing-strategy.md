# Effectiveness Monitor Service - Testing Strategy

**Version**: v2.0
**Last Updated**: 2026-08-02
**Service Type**: Kubernetes CRD controller — no HTTP business API to test

---

## Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| **v2.0** | 2026-08-02 | **#1806 CORRECTION**: Full rewrite. Removed the entire fictional stateless-HTTP-service test plan: `httptest.Server` mocks for a nonexistent business REST API, a `HolmesGPTClient`/`monitor.HolmesGPTClient` integration-test suite calling a live `POST /api/v1/postexec/analyze`, a direct-`sql.Open("postgres", ...)` Data Storage integration suite, a fictional `shouldCallAI()` decision-logic unit suite, and invented coverage percentages (70%/50%/10-15%) that were never measured against real code. Replaced with the real test suite verified by direct enumeration of `pkg/effectivenessmonitor/**/*_test.go`, `internal/controller/effectivenessmonitor/**/*_test.go`, `test/integration/effectivenessmonitor/`, and `test/e2e/effectivenessmonitor/`. | [#1806](https://github.com/jordigilh/kubernaut/issues/1806) |
| v1.0 | 2025-10-06 | ⚠️ **STALE (superseded by v2.0)** — Original fictional stateless-HTTP-service test specification (HolmesGPT client mocks, direct PostgreSQL integration tests, `shouldCallAI()` unit tests). Never matched any implementation. | — |

---

## Overview

EM has **no HTTP business API**, so there is no endpoint-mocking test tier to speak of. Its real test suite instead proves three things, mirrored by the three tiers below:

1. **Unit** — the 4 component scorers (health, alert, metrics, hash) and the phase/timing/audit-payload logic around them are correct in isolation
2. **Integration** — the `Reconciler` correctly wires those scorers together against a real Kubernetes API server (`envtest`) and correctly talks to DataStorage/Prometheus/AlertManager over the wire
3. **E2E** — the full `RemediationRequest` → `EffectivenessAssessment` → DataStorage-audit-events journey works end-to-end on a real KIND cluster with real Prometheus/AlertManager/PostgreSQL

**Framework**: 100% [Ginkgo](https://onsi.github.io/ginkgo/)/[Gomega](https://onsi.github.io/gomega/) BDD. Each tier's Ginkgo bootstrap (`RegisterFailHandler(Fail)` + `RunSpecs(...)`) lives in: `pkg/effectivenessmonitor/suite_test.go` and `pkg/effectivenessmonitor/client/prometheus_http_test.go` (unit — one per Go package), `internal/controller/effectivenessmonitor/completion_reason_test.go` (unit, controller package — no separate `suite_test.go` file in this package), `test/integration/effectivenessmonitor/suite_test.go`, and `test/e2e/effectivenessmonitor/suite_test.go`. Zero standard-library `testing.T`-only tests.

**Test Scenario IDs**: The suite follows two overlapping conventions, both verified by direct grep — neither is universal, and this document does not retrofit IDs onto tests that don't have them:
- **Numeric Test Scenario IDs on every `It()`, descriptive titles on every `Describe()`**: `UT-EM-<area>-<seq>` (unit, e.g. `UT-EM-MC-001` for the metrics scorer, `UT-EM-054-001` for fleet routing), `IT-EM-<area>-<seq>` (integration, e.g. `IT-EM-193-001`), and `E2E-EM-<area>-<seq>` (E2E — e.g. `E2E-EM-HC-001` health check, `E2E-EM-AR-001`/`002` alert resolution, `E2E-EM-MC-001`/`002` metrics comparison, `E2E-EM-SD-001`/`002` spec drift, `E2E-EM-RC-001`/`AE-001`/`SH-001` lifecycle, `E2E-EM-VW-001`/`FF-001`/`GS-001` operational). Every one of the 13 E2E `It()` specs carries an `E2E-EM-*` ID; the `Describe()` wrapping them is a plain descriptive title (e.g. `Describe("EffectivenessMonitor Health Check E2E Tests", Label("e2e"), ...)`).
- **`BR-EM-NNN` business-requirement tags** directly in `Describe`/`Context` titles (e.g. `Describe("cluster-scoped metric/alert-label builders (BR-EM-003, BR-EM-002, DD-EM-005)", ...)` in `internal/controller/effectivenessmonitor/cluster_scoped_metrics_test.go`) — present in the majority of files across all three tiers.
- **Issue-number-driven regression naming**: a substantial minority of files are named directly after the GitHub issue that motivated them rather than a feature name, e.g. `health_issue246_integration_test.go`, `alert_stale_pod_issue269_integration_test.go`, `cluster_scoped_193_integration_test.go`, `isalertdecay_193_test.go`, `coverage_668_test.go`. This is a real, observable convention in this package — not every test traces to a BR.

---

## Unit Tests

**Location**: `pkg/effectivenessmonitor/**/*_test.go` (44 files: 43 in the package root + 1 in `pkg/effectivenessmonitor/client/`) and `internal/controller/effectivenessmonitor/*_test.go` (5 files) — **49 files total, 352 `It()` specs** (329 in `pkg/effectivenessmonitor`, 23 in `internal/controller/effectivenessmonitor`; counts by direct `grep -c '\bIt('` enumeration).

### What's actually tested

| Area | Representative files | What it proves |
|---|---|---|
| **Component scorers** (pure logic) | `metrics_scorer_test.go` (BR-EM-003, `UT-EM-MC-001`–`008`), `alert_test.go` (alert resolution scoring), `hash_test.go` (BR-EM-004), `health_test.go` (BR-EM-001) | Each of the 4 scorers is a deterministic pure function over already-fetched inputs (`metrics.Scorer.Score([]MetricComparison)`, etc.) — no network or K8s client involved |
| **Phase/reconciler state machine** | `phase_test.go`, `reconciler_entry_phase_test.go`, `reconciler_golden_wfp_test.go`, `reconciler_golden_stabilizing_test.go`, `reconciler_status_invariant_test.go`, `reconciler_spec_drift_test.go`, `reconciler_partial_scope_test.go`, `reconciler_alert_deferral_requeue_test.go`, `failed_phase_test.go`, `propagation_phase_test.go` | Pending → WaitingForPropagation → Stabilizing → Assessing → Completed/Failed transitions, using `sigs.k8s.io/controller-runtime/pkg/client/fake.NewClientBuilder()` (e.g. `reconciler_golden_wfp_test.go:64`) — **no real API server** |
| **Alert decay / drift detection** | `alert_decay_test.go`, `alert_deferral_test.go`, `hash_deferral_test.go`, `validity_test.go`, `validity_window_guard_test.go` | DD-EM-004 async hash deferral, BR-EM-012 alert-decay-vs-metrics-reachability logic |
| **Audit event construction** | `audit_test.go`, `audit_manager_test.go` (`UT-EM-AM-001`–`009`, DD-017 typed sub-objects; `ADR-EM-001` completed-payload batch) | The exact JSON payloads emitted to DataStorage for each of the 7 audit event types — no DataStorage call is made; the buffered-writer boundary is the seam |
| **DataStorage query client** | `ds_querier_test.go` | Uses `httptest.NewServer` + `NewOgenDataStorageQuerierWithTransport(url, timeout, http.DefaultTransport)` to exercise the real ogen-generated wire client against a fake HTTP server — the **only** unit test in this tier that opens a real (loopback) socket |
| **Prometheus/AlertManager wire parsing** | `pkg/effectivenessmonitor/client/prometheus_http_test.go` | Pure characterization tests of `parsePromResponse` against in-memory JSON strings (`strings.NewReader`) — no `httptest.Server`; explicitly framed as "characterization tests ... to pin down current behavior before refactoring" |
| **Config/CA-reload/TLS** | `config_test.go`, `config_disabled_test.go`, `config_573_test.go`, `service_config_test.go`, `ca_reloader_test.go`, `tls_client_test.go` | `internal/config/effectivenessmonitor` parsing/validation and the Issue #756/#452 CA-reload + TLS transport logic |
| **Fleet federation routing** | `fleet_routing_test.go` (`internal/controller` package) | BR-FLEET-054 local-vs-remote-cluster routing decision, using `fake.NewClientBuilder()` |
| **Target-resource resolution** | `internal/controller/effectivenessmonitor/target_resources_test.go`, `cluster_scoped_metrics_test.go` (BR-EM-002/003, DD-EM-005) | Node/PV cluster-scoped metric query construction, pod-name/hash resolution for arbitrary target kinds |

### Mock strategy (unit tier)

- **Kubernetes API**: `sigs.k8s.io/controller-runtime/pkg/client/fake.NewClientBuilder()` — never a real API server
- **DataStorage** (query path only): `httptest.NewServer` wrapping the real ogen client (`ds_querier_test.go`)
- **AlertManager/Prometheus** (business-logic tests): hand-written interface fakes implementing `emclient.AlertManagerClient`/`emclient.PrometheusQuerier` (`pkg/effectivenessmonitor/client/interfaces.go`) — e.g. `mockAlertManagerClient` in `alert_test.go`; the HTTP wire-protocol code itself (`prometheus_http.go`, `alertmanager_http.go`) is exercised only by characterization tests (Prometheus) or left to the integration tier (AlertManager — no unit test constructs `alertManagerHTTPClient` directly)

---

## Integration Tests

**Location**: `test/integration/effectivenessmonitor/*_test.go` (24 files) + `test/integration/effectivenessmonitor/fleet/*_test.go` (2 files) — **26 files total, 117 `It()` specs**.

### Infrastructure (per `test/integration/effectivenessmonitor/suite_test.go`, lines 17–34)

> Defense-in-Depth Strategy (per TESTING_GUIDELINES.md):
> External Dependencies:
> - DataStorage: Real PostgreSQL + Redis + DataStorage container (DD-TEST-001)
> - Prometheus: `httptest.NewServer` mock (per TESTING_GUIDELINES.md Section 4a)
> - AlertManager: `httptest.NewServer` mock (per TESTING_GUIDELINES.md Section 4a)

Concretely, the top-level 24-file suite:
- Boots a **shared `envtest.Environment`** (real etcd + kube-apiserver, no mocking) for DataStorage's own auth path, plus a **per-process `envtest.Environment`** with all Kubernaut CRDs registered, for the `Reconciler` under test
- Creates a real ServiceAccount + RBAC binding inside that envtest cluster to exercise DD-AUTH-014 authentication end-to-end when calling the real DataStorage container
- Wires the real `prometheus_http.go`/`alertmanager_http.go` HTTP clients to `httptest.NewServer` instances standing in for Prometheus/AlertManager (never the real services)
- Runs with `ginkgo -p --procs=4`, each spec using a uniquely-named namespace for parallel-safe isolation

The separate **`fleet/` sub-package (2 files, `BR-FLEET-054`)** is lighter-weight: it uses `fake.NewClientBuilder()` rather than `envtest` (`em_fleet_routing_test.go:30`, `IT-EM-054-001` etc.) — it is an "integration" test in the sense of exercising the fleet-routing seam between the reconciler and `pkg/fleet`, not in the sense of hitting a real API server.

### What's actually tested

| Category | Files (examples) | Focus |
|---|---|---|
| **Reconciler lifecycle against envtest** | `reconciler_lifecycle_test.go`, `remaining_integration_test.go` | Full CRD watch → reconcile → status-update loop against a real API server |
| **Health checks** | `health_integration_test.go`, `health_issue246_integration_test.go`, `health_issue275_integration_test.go` (BR-EM-001, `IT-EM-246-*`, `IT-EM-275-*`) | Pod-status decision tree against real `Pod` objects in envtest |
| **Alert resolution / decay** | `alert_integration_test.go`, `alert_decay_integration_test.go`, `alert_stale_pod_issue269_integration_test.go` (`IT-EM-269-*`) | AlertManager `httptest` mock responses driving the alert scorer through the real reconciler |
| **Metrics comparison** | `metrics_integration_test.go` | Prometheus `httptest` mock responses driving the metrics scorer |
| **Spec hash / drift** | `hash_integration_test.go`, `hash_configmap_integration_test.go`, `hash_deferral_integration_test.go`, `spec_drift_integration_test.go` (DD-EM-002, DD-EM-004) | Canonical hash computation and drift detection against real target objects |
| **Cluster-scoped assessment** | `cluster_scoped_193_integration_test.go` (`IT-EM-193-*`, Issue #193) | Node/PV metric+alert routing (DD-EM-005) |
| **Dual-target routing** | `dual_target_routing_integration_test.go` (DD-EM-003) | Signal-target vs. remediation-target assessment routing |
| **Audit trail** | `audit_integration_test.go` | Real audit events written to and read back from the real DataStorage container — proves the DD-AUTH-005 Bearer-token path end-to-end |
| **Timing/derived config** | `derived_timing_integration_test.go`, `propagation_timing_integration_test.go` (BR-EM-009) | Stabilization/validity window computation |
| **Config/TLS** | `config_integration_test.go`, `tls_integration_test.go`, `issue573_integration_test.go` | External-client TLS/CA behavior (Issue #452/#756) against envtest-issued certs |
| **K8s Events** | `events_integration_test.go` | `EffectivenessAssessed` K8s Event emission |
| **Fleet routing** | `fleet/em_fleet_routing_test.go` | `ReaderFor` local-vs-remote-cluster routing (`IT-EM-054-*`) |

---

## E2E Tests

**Location**: `test/e2e/effectivenessmonitor/*_test.go` — **8 files, 13 `It()` specs**.

### Infrastructure (per `test/e2e/effectivenessmonitor/suite_test.go`, lines 17–30)

> - Unit tests: Business logic in isolation
> - Integration tests: Infrastructure interaction with envtest
> - E2E tests: Complete workflow validation with KIND (this package)
>
> Infrastructure:
> - Real Prometheus (metric comparison via OTLP injection)
> - Real AlertManager (alert resolution queries)
> - DataStorage (PostgreSQL + Redis) for audit event verification

Concretely: a real **KIND cluster** (`clusterName = "em-e2e"`) is created once per suite run (`infrastructure.SetupEMInfrastructure`, DD-TEST-002 hybrid-parallel setup) with an isolated kubeconfig (`~/.kube/em-e2e-config`, never overwriting the developer's default). A real ServiceAccount is created and its token retrieved for DD-AUTH-014-authenticated calls to the real DataStorage container. `ginkgo -p --procs=4` parallelism is supported. On failure, `infrastructure.MustGatherPodLogs` collects controller logs, and `E2E_COVERAGE=true` triggers DD-TEST-007 binary coverage collection from the running pod before cluster teardown.

### What's actually tested

| File | Scenario IDs | Focus |
|---|---|---|
| `lifecycle_e2e_test.go` | `E2E-EM-RC-001`, `E2E-EM-AE-001`, `E2E-EM-SH-001` | Full `RemediationRequest` → `EffectivenessAssessment` → Completed pipeline; all 5 audit events emitted to DataStorage; spec-hash computation on a real cluster |
| `health_e2e_test.go` | `E2E-EM-HC-001` | Health score 0.0 when the target Pod is not running, against a real KIND cluster |
| `alert_e2e_test.go` | `E2E-EM-AR-001`, `E2E-EM-AR-002` | Alert score 1.0 when AlertManager reports the alert resolved; 0.0 when it's still active |
| `metrics_e2e_test.go` | `E2E-EM-MC-001`, `E2E-EM-MC-002` | Metrics score > 0 on real improvement; 0.0 on no change, against a real Prometheus |
| `spec_drift_e2e_test.go` | `E2E-EM-SD-001`, `E2E-EM-SD-002` | Spec-drift detected → score 0.0; no drift → score > 0.0 (DD-EM-002 v1.1) |
| `operational_e2e_test.go` | `E2E-EM-VW-001`, `E2E-EM-FF-001`, `E2E-EM-GS-001` | Validity-window expiry marks the EA expired; controller starts successfully when Prometheus is unreachable; graceful SIGTERM shutdown within timeout |
| `helpers_test.go` | — | Shared namespace/fixture helpers (no `It()` specs of its own) |
| `suite_test.go` | — | `SynchronizedBeforeSuite`/`SynchronizedAfterSuite` cluster lifecycle (no `It()` specs of its own) |

**What is *not* tested here**: there is no "Level 1 vs Level 2" or AI-analysis E2E scenario, because Level 2 (the `POST /api/v1/postexec/analyze` AI enrichment) is ⚠️ **Planned V1.1 — NOT YET IMPLEMENTED** ([DD-017](../../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md)). Every E2E scenario in this suite exercises only the deterministic Level 1 path.

---

## Test Execution

```bash
# Unit tests
go test ./pkg/effectivenessmonitor/... ./internal/controller/effectivenessmonitor/...
# or, for Ginkgo-native output:
ginkgo ./pkg/effectivenessmonitor/... ./internal/controller/effectivenessmonitor/...

# Integration tests (requires envtest binaries: setup-envtest / KUBEBUILDER_ASSETS)
ginkgo -p --procs=4 ./test/integration/effectivenessmonitor/...

# E2E tests (requires KIND + Podman/Docker; creates and tears down a real cluster)
ginkgo -p --procs=4 ./test/e2e/effectivenessmonitor/...

# Focus by business requirement
ginkgo --focus="BR-EM-001" ./pkg/effectivenessmonitor/...

# Focus by Test Scenario ID
ginkgo --focus="IT-EM-193" ./test/integration/effectivenessmonitor/...
```

---

## What Is NOT in This Test Suite

The v1.0 draft of this document described tests for a service that was never built. None of the following exist anywhere in the EM test suite:

| Previous claim | Current reality |
|---|---|
| `httptest.Server` mocking a business REST API (`/api/v1/assess/effectiveness`, `/api/v1/context/trends`) | No such endpoints exist. EM has no inbound HTTP business API |
| `monitor.HolmesGPTClient` / `monitor.NewHolmesGPTClient(...)` integration tests against `POST /api/v1/postexec/analyze` | Zero AI/LLM client code exists in EM's V1.0 codebase. That endpoint is ⚠️ **Planned V1.1 — NOT YET IMPLEMENTED** ([DD-017](../../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md)) |
| Direct `sql.Open("postgres", testDB)` Data Storage integration tests, hand-seeded `remediation_audit` table rows | EM has no direct database connection. `audit_integration_test.go` talks to DataStorage exclusively through its authenticated HTTP audit API |
| `shouldCallAI()` "AI Decision Logic" unit suite (P0-failure/new-action-type/anomaly triggers) | No such function exists. Every EM scoring decision in V1.0 is unconditional and deterministic — there is no AI-invocation gate to test |
| `effectiveness.InfrastructureMonitoringClient` tests | No "Infrastructure Monitoring Service" exists in the current architecture |
| Invented coverage targets: Unit 70%+, Integration >50%, E2E 10-15% | Not measured against real code by this document; use `scripts/coverage/coverage_report.py` (per `AGENTS.md`) for the actual, current merged-tier coverage figures rather than the stale percentages this replaces |
| `BR-INS-001` through `BR-INS-010` test-to-requirement mapping | ⚠️ **STALE** — no `BR-INS-*` requirement document backs the current implementation; the code-aligned namespace observed in test files is `BR-EM-001` through `BR-EM-012` (see [overview.md](./overview.md#business-requirements-coverage-v10) for the same STALE-ID flag) |

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [Overview](./overview.md) | Service purpose, architecture, component scorers |
| [CRD Schema](./crd-schema.md) | `EffectivenessAssessment` spec/status fields |
| [Security Configuration](./security-configuration.md) | RBAC, NetworkPolicy, authentication used by/tested in the integration and E2E tiers |
| [Integration Points](./integration-points.md) | Upstream/downstream services exercised by the integration/E2E tiers |
| [DD-017](../../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md) | V1.0 Level 1 / V1.1 Level 2 scoping — authoritative status of the `postexec/analyze` endpoint |
| [DD-EM-002](../../../architecture/decisions/DD-EM-002-canonical-spec-hash.md) | Canonical spec hash algorithm tested by `hash_test.go`/`hash_integration_test.go` |
| [DD-EM-003](../../../architecture/decisions/DD-EM-003-dual-target-assessment.md) | Dual-target assessment tested by `dual_target_routing_integration_test.go` |
| [DD-EM-004](../../../architecture/decisions/DD-EM-004-async-hash-deferral.md) | Async hash deferral tested by `hash_deferral_test.go`/`hash_deferral_integration_test.go` |
| [DD-EM-005](../../../architecture/decisions/DD-EM-005-cluster-scoped-metrics-alert-assessment.md) | Cluster-scoped Node/PV metrics tested by `cluster_scoped_metrics_test.go`/`cluster_scoped_193_integration_test.go` |
| [ADR-EM-001](../../../architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md) | Integration architecture and scoring formula that the audit-payload unit tests validate against |

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2026-08-02
