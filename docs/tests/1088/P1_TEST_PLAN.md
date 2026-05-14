# Test Plan: Data Storage P1 Operational Readiness — Phases 6-8 (#1088)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1088-P1-v1.0
**Feature**: Performance, Observability, and API Contract hardening for Data Storage
**Version**: 1.0
**Created**: 2026-05-13
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.5`
**Parent Issue**: [#1048](https://github.com/jordigilh/kubernaut/issues/1048)
**Tracking Issue**: [#1088](https://github.com/jordigilh/kubernaut/issues/1088)
**Operator Dependencies**: [kubernaut-operator#92](https://github.com/jordigilh/kubernaut-operator/issues/92) (endpoint propagation delay)

---

## 1. Introduction

### 1.1 Purpose

This test plan validates Phases 6-8 (P1 Operational Readiness) of the Data Storage Readiness Plan (#1088). These phases address performance, observability, and API contract correctness.

| Phase | Scope | Key Items |
|-------|-------|-----------|
| Phase 6 | Performance | Narrow `SELECT *`, DLQ timeout fix, `IdleTimeout`, scalar JSON guard |
| Phase 7 | Observability | Redis readiness, drain/retention/PEL metrics, shutdown error surfacing, `shutdown_id` UUID, configurable endpoint delay |
| Phase 8 | API Contract | verify-chain OpenAPI, health/metrics spec removal, RFC 7807 standardization |

### 1.2 Objectives

1. **Query safety**: All `SELECT *` queries narrowed to explicit column lists, preventing schema-drift fragility.
2. **DLQ responsiveness**: DLQ retry worker responds to shutdown within 5s (no infinite block).
3. **HTTP hardening**: All Data Storage HTTP servers have `IdleTimeout` configured.
4. **Input validation**: Scalar JSON payloads rejected with RFC 7807 422 response.
5. **Readiness accuracy**: Health probe checks both database and Redis connectivity.
6. **Shutdown observability**: All shutdown steps metered with Prometheus metrics and correlated via `shutdown_id` UUID.
7. **Error aggregation**: DLQ drain errors surfaced in `Shutdown()` return value (ARCH-M1).
8. **Configuration**: `endpointPropagationDelay` configurable via config file (SRE-L1).
9. **Retention observability**: Retention worker metered with purge/duration/error counters.
10. **PEL visibility**: PEL pending count and max idle age exposed as Prometheus gauges.
11. **API contract**: verify-chain in OpenAPI spec, RFC 7807 consistency across all handlers.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `make test-unit-datastorage` |
| Integration test pass rate | 100% | `make test-integration-datastorage` |
| Unit-testable code coverage | >=80% | Query narrowing, batch guard, config parsing, metric instrumentation |
| Build success | 100% | `go build ./...` |
| Lint compliance | 0 new errors | `golangci-lint run` |
| 9-category checkpoint audit | All 9 categories satisfied per checkpoint | Checkpoint audit protocol |

---

## 2. References

### 2.1 Authority

- **DD-007 / DD-008 / DD-009**: Graceful shutdown, DLQ drain, DLQ retry worker lifecycle
- **BR-STORAGE-019**: Prometheus metrics endpoint
- **BR-STORAGE-024**: RFC 7807 error responses
- **BR-AUDIT-001**: Complete audit trail with no data loss
- **FedRAMP AU-2**: Auditable events (shutdown correlation)
- **SOC 2 CC8.1**: Tamper-evident audit logs (verify-chain)

### 2.2 Cross-References

- [Phase 5 Test Plan](../1048/PHASE5_TEST_PLAN.md)
- [Issue #1088 — Data Storage Readiness Plan](https://github.com/jordigilh/kubernaut/issues/1088)
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes) — REFACTOR validation
- [Testing Strategy](.cursor/rules/03-testing-strategy.mdc)
- [V1.0 Test Plan Template](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | SELECT * narrowing misses column added by migration | Query returns zero-value for missed field | Medium | UT-DS-1088-P6-001..003 | Derive column list from Go struct `db:` tags; integration test validates real PG |
| R2 | DLQ timeout change causes missed messages | Audit event loss during normal operation | Low | UT-DS-1088-P6-010..012 | 5s timeout matches tick interval; worker re-reads on next cycle |
| R3 | `IdleTimeout` on shared health server affects other services | Unexpected connection drops | Low | UT-DS-1088-P6-030 | Health probes are short-lived; 120s is generous |
| R4 | Scalar JSON `null` silently becomes nil slice | Misleading "batch cannot be empty" error | Medium | UT-DS-1088-P6-042 | Explicit null guard before empty-array check |
| R5 | Redis PING in readiness probe adds latency | Readiness probe exceeds K8s `timeoutSeconds` | Medium | UT-DS-1088-P7-001..003 | Context timeout on PING; operator probe timeout is 3s |
| R6 | Changing `shutdownStep4DrainDLQ` return type breaks callers | Compilation errors | Low | UT-DS-1088-P7-020..022 | Only called from `Shutdown()` — single caller |
| R7 | RFC 7807 standardization changes error format clients depend on | Client-side parsing breaks | Medium | UT-DS-1088-P8-001..004 | `type` field changes are slug suffix only, not structure |
| R8 | `remediation_audit` DDL not in migration files | Column list may not match runtime table | Medium | IT-DS-1088-P6-001 | Integration test against real PG validates column list |

---

## 4. Test Scenarios

### 4.1 Phase 6: Performance

#### 4.1.1 Narrow SELECT * (Items 6.1, 6.3)

**Files Under Test**: `pkg/datastorage/query/service.go`, `pkg/datastorage/repository/workflow/discovery.go`, `pkg/datastorage/repository/workflow/crud.go`
**Scan Targets**: `RemediationAuditResult` ([`pkg/datastorage/query/types.go:25-47`](pkg/datastorage/query/types.go), 20 columns), `RemediationWorkflow` ([`pkg/datastorage/models/workflow.go:46-186`](pkg/datastorage/models/workflow.go), ~40 columns)

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-P6-001 | Unit | RED | `ListRemediationAudits` query uses explicit column list (not `SELECT *`) | Query string contains all 20 `RemediationAuditResult` `db:` tag columns |
| UT-DS-1088-P6-002 | Unit | RED | `ListWorkflowsByActionType` inner query uses explicit columns | Query string contains `RemediationWorkflow` columns, not `SELECT *` in inner subquery |
| UT-DS-1088-P6-003 | Unit | RED | `GetWorkflowWithContextFilters` query uses explicit columns | Query string contains explicit columns |
| UT-DS-1088-P6-050 | Unit | GREEN | CRUD `GetByID` uses explicit columns | Query verified |
| UT-DS-1088-P6-051 | Unit | GREEN | CRUD `GetByNameAndVersion` uses explicit columns | Query verified |
| UT-DS-1088-P6-052 | Unit | GREEN | CRUD `GetLatestVersion` uses explicit columns | Query verified |
| IT-DS-1088-P6-001 | Integration | GREEN | `ListRemediationAudits` returns all fields from real PostgreSQL | All 20 struct fields populated correctly |
| IT-DS-1088-P6-002 | Integration | GREEN | Discovery queries return correct scored results from real PostgreSQL | `final_score` computed, all workflow fields populated |

**Test File**: `test/unit/datastorage/select_narrowing_test.go` (new)

**XRange Drain Validation**: Already covered by `UT-DS-1048-QEM2-003` (150 messages, `drainBatchSize=100`). No new tests needed.

---

#### 4.1.2 DLQ ReadMessages Timeout Fix (Item 6.2)

**File Under Test**: `pkg/datastorage/server/dlq_retry_worker.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-P6-010 | Unit | RED | DLQ retry worker does NOT pass `-1` as timeout to `ReadMessages` | Timeout parameter is positive (`dlqReadTimeout`) |
| UT-DS-1088-P6-011 | Unit | RED | Worker loop responds to context cancellation within timeout window | Worker exits within 2x `dlqReadTimeout` after cancel |
| UT-DS-1088-P6-012 | Unit | RED | `ReadMessages` with positive timeout returns empty on no messages | Returns `[]*DLQMessage{}`, no error, no infinite block |

**Test File**: `test/unit/datastorage/dlq_timeout_test.go` (new)

---

#### 4.1.3 IdleTimeout on Health/Metrics Servers (Item 6.4)

**Files Under Test**: `pkg/shared/health/server.go` (shared), `cmd/datastorage/main.go`

**Preflight Finding**: `IdleTimeout` is missing on health server (`pkg/shared/health/server.go:52-58`) and metrics server (`cmd/datastorage/main.go:390-396`). API server already has `120 * time.Second`.

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-P6-030 | Unit | RED | Health server `http.Server` has `IdleTimeout` set | `IdleTimeout == 120 * time.Second` |
| UT-DS-1088-P6-031 | Unit | RED | Metrics server `http.Server` has `IdleTimeout` set | `IdleTimeout == 120 * time.Second` |
| UT-DS-1088-P6-032 | Unit | GREEN | API server has `IdleTimeout: 120s` (validate existing) | Already set |

**Test File**: `test/unit/datastorage/idle_timeout_test.go` (new)

**Note**: Health server is in shared `pkg/shared/health/`. Change affects all services. Verify no regressions.

---

#### 4.1.4 Scalar JSON Guard for Batch Payloads (Item 6.5)

**File Under Test**: `pkg/datastorage/server/audit_events_batch_handler.go`

**Current behavior**: Object payloads detected (line 84). String/number/bool fall through to generic error with leaked Go error message. `null` silently becomes nil slice, hitting "batch cannot be empty" path.

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-P6-040 | Unit | RED | Batch handler returns RFC 7807 422 for scalar JSON string (`"hello"`) | Status 422, `type` contains "invalid-content-type" or "invalid-payload-type" |
| UT-DS-1088-P6-041 | Unit | RED | Batch handler returns RFC 7807 422 for scalar JSON number (`42`) | Status 422, consistent error slug |
| UT-DS-1088-P6-042 | Unit | RED | Batch handler returns RFC 7807 422 for JSON null (`null`) | Status 422, explicit "null payload" message |
| UT-DS-1088-P6-043 | Unit | GREEN | Batch handler accepts valid JSON array (validate existing behavior) | Status 200/202 |
| IT-DS-1088-P6-040 | Integration | GREEN | Scalar JSON guard via full HTTP round-trip | HTTP 422 with `application/problem+json` content type |

**Test File**: `test/unit/datastorage/batch_scalar_guard_test.go` (new)

**Checkpoint Adversarial Extensions** (added during CHECKPOINT 6):
- Empty body (`Content-Length: 0`)
- Unicode BOM prefix (`\xEF\xBB\xBF[...]`)
- Deeply nested JSON object (covered by existing `MaxBytesReader`)

---

### 4.2 Phase 7: Observability

#### 4.2.1 Redis Readiness Check (Item 7.3)

**File Under Test**: `pkg/datastorage/server/handlers.go` (`handleReadiness`)
**Existing API**: `dlq.Client.HealthCheck(ctx context.Context) error` already implements Redis `PING`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-P7-001 | Unit | RED | `handleReadiness` returns 503 with `reason: "redis_unreachable"` when `dlqClient.HealthCheck` fails | Status 503, JSON body with `reason` field |
| UT-DS-1088-P7-002 | Unit | RED | `handleReadiness` returns 200 when both DB Ping and Redis HealthCheck succeed | Status 200, `{"status":"ready"}` |
| UT-DS-1088-P7-003 | Unit | RED | `handleReadiness` skips Redis check when `dlqClient` is nil | Status 200 if DB is reachable |

**Test File**: `test/unit/datastorage/redis_readiness_test.go` (new)

---

#### 4.2.2 Drain Prometheus Metrics — SRE-H1 (Item 7.4)

**Files Under Test**: `pkg/datastorage/metrics/metrics.go`, `pkg/datastorage/server/server.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-P7-010 | Unit | RED | `datastorage_shutdown_drain_total` counter registered with `stream` label | Metric gatherable from registry |
| UT-DS-1088-P7-011 | Unit | RED | `datastorage_shutdown_drain_duration_seconds` histogram registered | Metric gatherable |
| UT-DS-1088-P7-012 | Unit | RED | `datastorage_shutdown_duration_seconds` histogram registered | Metric gatherable |
| UT-DS-1088-P7-013 | Unit | GREEN | `Shutdown()` increments drain counter and observes duration | Counter value > 0 after shutdown with DLQ messages |

**Test File**: `test/unit/datastorage/shutdown_metrics_test.go` (new)

**Metric Definitions**:

```go
var (
    shutdownDrainTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "datastorage_shutdown_drain_total",
        Help: "Total messages drained during shutdown",
    }, []string{"stream"})

    shutdownDrainDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "datastorage_shutdown_drain_duration_seconds",
        Help:    "Duration of DLQ drain during shutdown",
        Buckets: prometheus.DefBuckets,
    })

    shutdownDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "datastorage_shutdown_duration_seconds",
        Help:    "Total duration of graceful shutdown",
        Buckets: prometheus.DefBuckets,
    })
)
```

---

#### 4.2.3 Shutdown Error Surfacing — ARCH-M1 (Item 7.6)

**File Under Test**: `pkg/datastorage/server/server.go` (`shutdownStep4DrainDLQ`, `Shutdown`)

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-P7-020 | Unit | RED | `shutdownStep4DrainDLQ` returns error on DLQ drain failure | Non-nil error returned |
| UT-DS-1088-P7-021 | Unit | RED | `Shutdown()` includes DLQ drain error in returned `errors.Join` | Returned error contains drain error |
| UT-DS-1088-P7-022 | Unit | RED | `Shutdown()` continues cleanup after DLQ drain error (step 5 still runs) | DB closed despite drain error |

**Test File**: `test/unit/datastorage/shutdown_error_surface_test.go` (new)

---

#### 4.2.4 Configurable Endpoint Propagation Delay — SRE-L1 (Item 7.7)

**Files Under Test**: `pkg/datastorage/config/config.go`, `pkg/datastorage/server/server.go`
**Config Pattern**: String field + `GetXxx() time.Duration` + `time.ParseDuration` + `Validate()` check (matches `GetShutdownTimeout()`)

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-P7-030 | Unit | RED | `ServerConfig.EndpointPropagationDelay` parsed from YAML string | Field populated with `"5s"` |
| UT-DS-1088-P7-031 | Unit | RED | `GetEndpointPropagationDelay()` defaults to 5s when field empty | Returns `5 * time.Second` |
| UT-DS-1088-P7-032 | Unit | RED | Server uses configured delay in `shutdownStep2WaitForPropagation` | Custom delay (e.g. `"10s"`) respected |

**Test File**: `test/unit/datastorage/endpoint_delay_config_test.go` (new)

---

#### 4.2.5 Shutdown ID UUID — FEDRAMP-H1 / AU-2 (Item 7.8)

**File Under Test**: `pkg/datastorage/server/server.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-P7-040 | Unit | RED | `Shutdown()` generates unique `shutdown_id` UUID | Log output contains valid UUID v4 |
| UT-DS-1088-P7-041 | Unit | RED | All shutdown step logs contain `shutdown_id` field | Every `Info`/`Error` in shutdown path has `shutdown_id` key |

**Test File**: `test/unit/datastorage/shutdown_id_test.go` (new)

---

#### 4.2.6 Retention Worker Prometheus Metrics — SRE-S3 (Item 7.9)

**Files Under Test**: `pkg/datastorage/metrics/metrics.go`, `pkg/datastorage/retention/worker.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-P7-050 | Unit | RED | `datastorage_retention_purge_total` counter registered | Metric gatherable |
| UT-DS-1088-P7-051 | Unit | RED | `datastorage_retention_duration_seconds` histogram registered | Metric gatherable |
| UT-DS-1088-P7-052 | Unit | RED | `datastorage_retention_errors_total` counter registered | Metric gatherable |
| UT-DS-1088-P7-053 | Unit | GREEN | `RetentionWorker.run()` increments purge counter and observes duration | Counter > 0 after purge run |

**Test File**: `test/unit/datastorage/retention_metrics_test.go` (new)

---

#### 4.2.7 PEL Pending Count and Max Idle Age Gauges — SRE-S1 (Item 7.10)

**Files Under Test**: `pkg/datastorage/dlq/metrics.go`, `pkg/datastorage/server/dlq_retry_worker.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-P7-060 | Unit | RED | `datastorage_dlq_pel_pending_count` gauge registered with `stream` label | Metric gatherable |
| UT-DS-1088-P7-061 | Unit | RED | `datastorage_dlq_pel_max_idle_seconds` gauge registered with `stream` label | Metric gatherable |
| UT-DS-1088-P7-062 | Unit | GREEN | `DLQRetryWorker` updates PEL gauges after recovery cycle | Gauge value > 0 when pending entries exist |

**Test File**: `test/unit/datastorage/pel_gauges_test.go` (new)

---

### 4.3 Phase 8: API Contract

#### 4.3.1 RFC 7807 Error Slug Standardization (Item 8.3)

**Files Under Test**: `pkg/datastorage/server/reconstruction_handler.go`, `pkg/datastorage/server/audit_export_handler.go`, `pkg/datastorage/server/audit_verify_chain_handler.go`, `pkg/datastorage/server/response/rfc7807.go`

**Current Issues**:
- `reconstruction_handler.go:86`: Passes full URL `"https://kubernaut.ai/problems/reconstruction/unexpected-error"` to `WriteRFC7807InternalError`, which prepends `"https://kubernaut.ai/problems/"` again → double prefix
- `reconstruction_handler.go:114-124`: Passes `resp.Type.String()` (already a full URL from ogen) to `WriteRFC7807Error` → double prefix
- `audit_export_handler.go:436-450`: Custom `writeRFC7807Error` uses `type: "about:blank"` instead of kubernaut.ai domain
- `audit_verify_chain_handler.go:76-104`: Uses plain `http.Error` (not RFC 7807)

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-P8-001 | Unit | RED | `reconstruction_handler` error `type` field has single `https://kubernaut.ai/problems/` prefix | No double prefix |
| UT-DS-1088-P8-002 | Unit | RED | `audit_export_handler` error uses `https://kubernaut.ai/problems/` domain (not `about:blank`) | Standard domain prefix |
| UT-DS-1088-P8-003 | Unit | RED | `verify_chain_handler` returns `application/problem+json` (not `text/plain`) | RFC 7807 content type |
| UT-DS-1088-P8-004 | Unit | RED | All error responses contain required RFC 7807 fields: `type`, `title`, `status`, `detail` | All 4 fields present |
| IT-DS-1088-P8-001 | Integration | GREEN | verify-chain with invalid body returns RFC 7807 via HTTP round-trip | 400 with `application/problem+json` |
| IT-DS-1088-P8-002 | Integration | GREEN | verify-chain with empty `correlation_id` returns RFC 7807 via HTTP round-trip | 400 with `application/problem+json` |

**Test File**: `test/unit/datastorage/rfc7807_consistency_test.go` (new)

---

## 5. Test Execution Order (TDD Phases)

### 5.1 Phase 6 — RED → GREEN → REFACTOR → CHECKPOINT

#### RED (write all failing tests)

| Order | Test IDs | Item |
|-------|----------|------|
| 1 | UT-DS-1088-P6-001..003 | SELECT * narrowing (audit, discovery, context) |
| 2 | UT-DS-1088-P6-010..012 | DLQ ReadMessages timeout |
| 3 | UT-DS-1088-P6-030..031 | IdleTimeout on health/metrics servers |
| 4 | UT-DS-1088-P6-040..042 | Scalar JSON guard |

#### GREEN (minimal implementation)

| Order | Test IDs | Item |
|-------|----------|------|
| 1 | UT-DS-1088-P6-001..003, IT-DS-1088-P6-001..002 | Replace SELECT * with explicit columns |
| 2 | UT-DS-1088-P6-050..052 | CRUD SELECT * narrowing |
| 3 | UT-DS-1088-P6-010..012 | Replace `-1` with `dlqReadTimeout` const |
| 4 | UT-DS-1088-P6-030..032 | Add IdleTimeout to servers |
| 5 | UT-DS-1088-P6-040..043, IT-DS-1088-P6-040 | Scalar JSON guard |

#### REFACTOR

- Extract column list constants from `db:` tags
- Rename `dlqReadTimeout` consistently with other timeout consts
- Extract scalar guard helper if reusable
- Validate against 100 Go Mistakes (#22-23, #28, #53, #73-76, #78, #89)

#### CHECKPOINT 6

9-category audit — see Section 6.1.

---

### 5.2 Phase 7 — RED → GREEN → REFACTOR → CHECKPOINT

#### RED

| Order | Test IDs | Item |
|-------|----------|------|
| 1 | UT-DS-1088-P7-001..003 | Redis readiness |
| 2 | UT-DS-1088-P7-010..012 | Drain metrics registration |
| 3 | UT-DS-1088-P7-020..022 | Shutdown error surfacing |
| 4 | UT-DS-1088-P7-030..032 | Configurable endpoint delay |
| 5 | UT-DS-1088-P7-040..041 | Shutdown ID UUID |
| 6 | UT-DS-1088-P7-050..052 | Retention metrics registration |
| 7 | UT-DS-1088-P7-060..061 | PEL gauges registration |

#### GREEN

| Order | Test IDs | Item |
|-------|----------|------|
| 1 | UT-DS-1088-P7-001..003 | Add `dlqClient.HealthCheck` to readiness |
| 2 | UT-DS-1088-P7-010..013 | Register drain metrics; instrument Shutdown |
| 3 | UT-DS-1088-P7-020..022 | Change `shutdownStep4DrainDLQ` to return error |
| 4 | UT-DS-1088-P7-030..032 | Add `EndpointPropagationDelay` config + getter |
| 5 | UT-DS-1088-P7-040..041 | Generate UUID, thread through shutdown logs |
| 6 | UT-DS-1088-P7-050..053 | Register retention metrics; instrument `run()` |
| 7 | UT-DS-1088-P7-060..062 | Register PEL gauges; instrument XPENDING call |
| 8 | (docs) | 7.1 metrics doc, 7.2 runbook names, 7.5 PEL edge cases |

#### REFACTOR

- Extract `shutdownContext` or use `logger.WithValues("shutdown_id", id)`
- Unify metric registration pattern (shared namespace/subsystem)
- Validate against 100 Go Mistakes (#53-55, #60-62, #73, #9, #78)

#### CHECKPOINT 7

9-category audit — see Section 6.2.

---

### 5.3 Phase 8 — RED → GREEN → REFACTOR → CHECKPOINT

#### RED

| Order | Test IDs | Item |
|-------|----------|------|
| 1 | UT-DS-1088-P8-001..004 | RFC 7807 consistency |

#### GREEN

| Order | Test IDs | Item |
|-------|----------|------|
| 1 | UT-DS-1088-P8-001..004, IT-DS-1088-P8-001..002 | Fix error slugs; migrate handlers |
| 2 | (spec) | Add verify-chain to OpenAPI; remove health/metrics |

#### REFACTOR

- Remove custom `writeRFC7807Error` from export handler
- Extract error slug constants
- Validate against 100 Go Mistakes (#73-76, #9, #1, #78)

#### CHECKPOINT 8

9-category audit (comprehensive) — see Section 6.3.

---

## 6. Checkpoint Audit Protocol

### 6.1 CHECKPOINT 6 Audit Criteria

| Category | Requirement | Evidence |
|----------|-------------|----------|
| CAT-1: Observability | No new metrics in Phase 6. Existing metrics still fire after query changes. | Cite existing metric test or add regression test |
| CAT-2: Adversarial | Scalar guard: empty body, BOM prefix, oversized, deeply nested. SQL columns: hardcoded (N/A). | UT-DS-1088-P6-040..042 + checkpoint extensions |
| CAT-3: Resource Bounds | Batch handler: 50+ sequential submissions, no map/slice growth. DLQ worker: 50+ empty read cycles. | New checkpoint test |
| CAT-4: Concurrency | DLQ worker `Stop()` with 10+ goroutines under `-race`. Batch handler: 10+ concurrent requests. | New checkpoint test |
| CAT-5: Nil/Zero | Empty query results (nil vs empty slice). Nil `dlqClient`. Empty JSON array `[]`. | UT-DS-1088-P6-042 (null), checkpoint for others |
| CAT-6: Error-Path | Query failures include table name + operation. DLQ errors include stream name. Batch errors include request context. | Code review + log assertion tests |
| CAT-7: Cross-Phase | First phase — no cross-phase deps. Track: Phase 7 drain metrics must instrument Phase 6 DLQ timeout change. | N/A (tracked for CHECKPOINT 7) |
| CAT-8: Spec | RFC 7807 422 response: `type`, `title`, `status`, `detail` present. HTTP 422 semantics correct. | UT-DS-1088-P6-040 assertions |
| CAT-9: API Hygiene | No exported test helpers from `pkg/datastorage/`. Column list constants unexported. | `go vet` + manual scan |

### 6.2 CHECKPOINT 7 Audit Criteria

| Category | Requirement | Evidence |
|----------|-------------|----------|
| CAT-1: Observability | All 10 new metrics (4 drain, 4 retention, 2 PEL) have production code path test proving value change. | UT-DS-1088-P7-013, -053, -062 |
| CAT-2: Adversarial | `EndpointPropagationDelay` config: negative, zero, 1h, unparseable string. Redis error strings with special chars. | Checkpoint tests |
| CAT-3: Resource Bounds | Metric `stream` label bounded to known set. XPENDING results bounded by COUNT. | Code review + assertion |
| CAT-4: Concurrency | `Shutdown()` 10+ concurrent callers. Readiness probe during shutdown transition. `RetentionWorker` Start/Stop race. | Checkpoint tests under `-race` |
| CAT-5: Nil/Zero | Nil `dlqClient` in readiness (skip Redis). Zero `EndpointPropagationDelay` (use default). `RetentionWorker` disabled. PEL 0 pending. | UT-DS-1088-P7-003, -031 |
| CAT-6: Error-Path | Redis PING failure: probe type, error msg, Redis addr. Drain failure: shutdown_id, step, stream, error. Retention: batch, rows, error. | Log assertion tests |
| CAT-7: Cross-Phase | Phase 7 drain metrics instrument Phase 6 DLQ timeout code path. Shutdown error surfacing covers DLQ read timeout scenario. | Integration test proving wiring |
| CAT-8: Spec | Helm `values.schema.json` valid. Readiness JSON response consistent. | `helm lint` + response assertion |
| CAT-9: API Hygiene | New metric variables unexported. No test helpers in production packages. | `go vet` + scan |

### 6.3 CHECKPOINT 8 Audit Criteria (Final P1)

| Category | Requirement | Evidence |
|----------|-------------|----------|
| CAT-1: Observability | All Phase 7 metrics instrumented. Existing metrics unbroken by Phase 6 queries. | Full metric inventory test |
| CAT-2: Adversarial | RFC 7807 slugs: path traversal, null bytes, max-length. verify-chain: malformed JSON, empty body, binary. | Checkpoint tests |
| CAT-3: Resource Bounds | RFC 7807 `detail` bounded (not echoing unbounded input). Metric label cardinality bounded. | Code review |
| CAT-4: Concurrency | verify-chain 10+ concurrent requests under `-race`. All Phase 7 concurrency tests passing. | Checkpoint test |
| CAT-5: Nil/Zero | `HandleVerifyChain` with nil signer. `WriteRFC7807Error` with empty slug, zero status. | Checkpoint tests |
| CAT-6: Error-Path | verify-chain errors: chain ID, block count, error cause. RFC 7807: type, title, status, detail, instance. | Log + response assertions |
| CAT-7: Cross-Phase | Phase 8 OpenAPI removal does NOT break middleware bypass for readiness probes. verify-chain uses correct query columns from Phase 6. | Integration test |
| CAT-8: Spec | RFC 7807: `type` (URI), `title`, `status` (integer). OpenAPI validates. Helm validates. | `openapi-generator validate` + `helm lint` |
| CAT-9: API Hygiene | No exported debug functions. `writeRFC7807Error` in export handler removed. Error slug constants are `const` not `var`. | `go vet` + manual scan |

---

## 7. Test Infrastructure Requirements

| Requirement | Status | Notes |
|-------------|--------|-------|
| Ginkgo/Gomega BDD framework | Available | Standard for all kubernaut tests |
| PostgreSQL (integration) | Available | Existing infrastructure |
| Redis/Valkey (integration) | Available | Existing DLQ integration tests |
| `prometheus.NewRegistry()` per test | Required | Avoid test pollution (100 Go Mistakes #78) |
| Mock `*sql.DB` | Available | Existing pattern |
| Mock `dlq.Client` | Needed | For readiness handler tests (mock `HealthCheck`) |
| HTTP test server (`httptest`) | Available | Standard library |

---

## 8. Coverage Targets

| Tier | Scope | Target | Measurement |
|------|-------|--------|-------------|
| Unit | Query narrowing, batch guard, config parsing, metrics, shutdown | >=80% | `go test -cover ./pkg/datastorage/...` |
| Integration | SELECT * with real PG, scalar guard HTTP round-trip, verify-chain | >=80% | `go test -cover ./test/integration/datastorage/...` |
| All tiers merged | Phases 6-8 code | >=80% | Line-by-line dedup |

---

## 9. Compliance Sign-Off

### Pre-Implementation Checklist

| Requirement | Evidence | Status |
|-------------|----------|--------|
| All test scenarios mapped to issue items | This document §4 | Done |
| TDD execution order defined (RED → GREEN → REFACTOR) | This document §5 | Done |
| Risk-to-test traceability complete | This document §3 | Done |
| 9-category checkpoint criteria defined | This document §6 | Done |
| 100 Go Mistakes validation scope defined | This document §5 per phase | Done |
| Operator dependency tracked | kubernaut-operator issue (SRE-L1) | Pending |

### Post-Implementation Checklist

| Requirement | Evidence | Status |
|-------------|----------|--------|
| All unit tests passing | `make test-unit-datastorage` | Pending |
| All integration tests passing | `make test-integration-datastorage` | Pending |
| Build succeeds | `go build ./...` | Pending |
| No lint errors | `golangci-lint run` | Pending |
| Coverage >= 80% per tier | Coverage report | Pending |
| CHECKPOINT 6 passed (all 9 categories) | Audit document | Pending |
| CHECKPOINT 7 passed (all 9 categories) | Audit document | Pending |
| CHECKPOINT 8 passed (all 9 categories) | Audit document | Pending |

### Approval

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Developer | | | Pending |
| Reviewer | | | Pending |
| Team Lead | | | Pending |

---

## Appendix A: Test File Locations

| Test Category | Location |
|---------------|----------|
| SELECT * narrowing (unit) | `test/unit/datastorage/select_narrowing_test.go` |
| DLQ timeout (unit) | `test/unit/datastorage/dlq_timeout_test.go` |
| IdleTimeout (unit) | `test/unit/datastorage/idle_timeout_test.go` |
| Scalar JSON guard (unit) | `test/unit/datastorage/batch_scalar_guard_test.go` |
| Redis readiness (unit) | `test/unit/datastorage/redis_readiness_test.go` |
| Shutdown metrics (unit) | `test/unit/datastorage/shutdown_metrics_test.go` |
| Shutdown error surface (unit) | `test/unit/datastorage/shutdown_error_surface_test.go` |
| Endpoint delay config (unit) | `test/unit/datastorage/endpoint_delay_config_test.go` |
| Shutdown ID (unit) | `test/unit/datastorage/shutdown_id_test.go` |
| Retention metrics (unit) | `test/unit/datastorage/retention_metrics_test.go` |
| PEL gauges (unit) | `test/unit/datastorage/pel_gauges_test.go` |
| RFC 7807 consistency (unit) | `test/unit/datastorage/rfc7807_consistency_test.go` |

## Appendix B: Deferred to Later Phases

| Item | Phase | Rationale |
|------|-------|-----------|
| verify-chain unit/integration tests (full coverage) | Phase 9 (Test Coverage) | P1 only adds OpenAPI spec; Phase 9 adds full test coverage |
| audit export tests | Phase 9 (Test Coverage) | Separate test coverage phase |
| Lifecycle E2E (startup → traffic → shutdown → verify DLQ drain) | Phase 9 (Test Coverage) | Requires Kind cluster |
| Coverage CI gate (>=80% per tier) | Phase 9 (Test Coverage) | Enforcement after P1 code is stable |
| ogen CLI → v1.20.1 alignment | Phase 10 (Build Integrity) | Separate build tooling phase |
| `make generate` DS wiring | Phase 10 (Build Integrity) | Build tooling |
| gen-diff CI check | Phase 10 (Build Integrity) | CI infrastructure |
