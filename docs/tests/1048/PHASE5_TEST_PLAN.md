# Test Plan: Data Storage Phase 5 — FedRAMP AU Fixes (#1048)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1048-P5-v1.0
**Feature**: FedRAMP AU compliance for Data Storage — signer fail-hard, retention worker lifecycle, retention default alignment (ADR-034), actor\_id enrichment, PEL recovery, Redis TLS client, DLQ trim alerting
**Version**: 1.0
**Created**: 2026-05-11
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.5`
**Parent Issue**: [#1048](https://github.com/jordigilh/kubernaut/issues/1048)
**Tracking Issue**: [#1088](https://github.com/jordigilh/kubernaut/issues/1088)
**Operator Dependencies**: [kubernaut-operator#89](https://github.com/jordigilh/kubernaut-operator/issues/89) (Valkey TLS), [kubernaut-operator#90](https://github.com/jordigilh/kubernaut-operator/issues/90) (signing cert mount)

---

## 1. Introduction

### 1.1 Purpose

This test plan validates Phase 5 of the Data Storage Readiness Audit (#1048). Phase 5 addresses FedRAMP AU (Audit) control family requirements:

| FedRAMP Control | Phase 5 Item | Description |
|-----------------|--------------|-------------|
| AU-9 | Item 1 | Signer fail-hard (no self-signed fallback) |
| AU-11 | Item 2 | Retention worker lifecycle |
| AU-11 | Item 3 | Retention DEFAULT alignment with ADR-034 (2555 days) |
| AU-3 | Item 4 | Actor identity from auth token |
| AU-2 | Item 5 | PEL recovery for stuck DLQ messages |
| AU-9, SC-8 | Item 6 | Redis TLS client support |
| AU-11 | Item 7 | DLQ MaxLen trim alerting + RPO documentation |

### 1.2 Objectives

1. **Signer integrity**: Data Storage refuses to start without a valid signing certificate (no ephemeral fallback).
2. **Retention lifecycle**: Background retention worker starts/stops with server, purges expired rows in batches, respects legal hold.
3. **Retention default**: Column default aligns with ADR-034 (2555 days / ~7 years); configurable via Helm.
4. **Actor attribution**: Audit events record the authenticated human identity from `X-Auth-Request-User`, not a synthetic default.
5. **PEL recovery**: Orphaned DLQ messages are reclaimed and reprocessed; poison messages are dead-lettered.
6. **Redis TLS**: Data Storage client supports TLS for Redis/Valkey connections.
7. **Trim visibility**: DLQ stream trim events are metered; RPO trade-off is documented.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/datastorage/...` |
| Integration test pass rate | 100% | `go test ./test/integration/datastorage/...` |
| Unit-testable code coverage | >=80% | Signer init, retention eligibility, actor enrichment, PEL logic |
| Integration-testable code coverage | >=80% | Worker lifecycle, Redis TLS, PEL recovery with real Redis |
| Safety: no fallback cert | 100% | UT-DS-1048-P5-001 through -003 |
| Safety: retention disabled by default | 100% | UT-DS-1048-P5-010 |

---

## 2. References

### 2.1 Authority (governing documents)

- **ADR-034**: Unified audit table design — `retention_days INTEGER DEFAULT 2555` (7 years, SOC 2 / ISO 27001)
- **ADR-032**: Data access layer isolation — "Audit data retention must meet regulatory requirements (7+ years)"
- **BR-AUDIT-007**: Tamper-evident audit exports (signed)
- **BR-AUDIT-009**: Retention policies meeting regulatory requirements
- **BR-AUDIT-004**: Immutable audit logs with integrity protection
- **DD-007 / DD-008 / DD-009**: Graceful shutdown, DLQ drain, DLQ retry worker lifecycle
- **FedRAMP**: AU-2, AU-3, AU-9, AU-11, SC-8

### 2.2 Cross-References

- [Phase 5 readiness audit findings](https://github.com/jordigilh/kubernaut/issues/1088)
- [TP-485 (Retention Enforcement)](../485/TEST_PLAN.md) — retention eligibility and purge SQL tests
- [Testing Strategy](.cursor/rules/03-testing-strategy.mdc)
- [V1.0 Test Plan Template](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Removing signer fallback crashes existing deployments | Service outage | High | UT-DS-1048-P5-001..003 | Configurable cert path; operator issue #90 for cert provisioning |
| R2 | Retention worker purges held rows | Legal/compliance violation | High | UT-DS-1048-P5-013, IT-DS-1048-P5-011 | PurgeSQL WHERE `legal_hold = FALSE`; integration test with trigger |
| R3 | PEL recovery creates duplicate inserts | Noisy logs (at-least-once OK) | Medium | IT-DS-1048-P5-020..022 | PK prevents double rows (accepted: DF-M1) |
| R4 | XAUTOCLAIM not supported in Valkey version | PEL recovery fails | Medium | IT-DS-1048-P5-020 | Version check at startup; fallback to XPENDING+XCLAIM |
| R5 | Redis TLS misconfiguration | Connection failure at startup | Medium | UT-DS-1048-P5-030..032 | Fail-hard on TLS handshake error; clear error message |
| R6 | Actor\_id override policy confusion (header vs body) | Incorrect attribution | Medium | UT-DS-1048-P5-040..045 | Server-side header always wins; documented policy |
| R7 | DLQ trim discards unprocessed audit events | Data loss | Medium | UT-DS-1048-P5-050..051 | Trim counter metric + alert; RPO documentation |

---

## 4. Test Scenarios

### 4.1 Item 1: Signer Fail-Hard (AU-9)

**Business Requirement**: BR-AUDIT-007 (tamper-evident audit exports)
**Files Under Test**: `pkg/datastorage/server/server.go` (`loadSigningCertificate`), `pkg/datastorage/config/config.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1048-P5-001 | Unit | RED | `loadSigningCertificate` returns error when cert file missing | `error` returned, no fallback |
| UT-DS-1048-P5-002 | Unit | RED | `loadSigningCertificate` returns error when key file missing | `error` returned, no fallback |
| UT-DS-1048-P5-003 | Unit | RED | `loadSigningCertificate` returns error when cert is corrupt/invalid | `error` returned, no fallback |
| UT-DS-1048-P5-004 | Unit | RED | `loadSigningCertificate` succeeds with valid cert+key pair | `*cert.Signer` returned |
| UT-DS-1048-P5-005 | Unit | GREEN | `signerCertDir` config field parsed from YAML | Config struct populated |
| UT-DS-1048-P5-006 | Unit | GREEN | `signerCertDir` defaults to `/etc/certs` when not set | Default applied |
| UT-DS-1048-P5-007 | Unit | REFACTOR | `generateFallbackCertificate` removed (dead code) | Build succeeds without it |

**Test File**: `test/unit/datastorage/signer_test.go` (new)

#### Test Approach

Tests use `os.MkdirTemp` with generated test certificates (via `cert.GenerateSelfSigned` in test setup) to validate file system paths. The test creates valid/invalid cert files and asserts `loadSigningCertificate` behavior.

```go
Describe("UT-DS-1048-P5-001: Signer fail-hard on missing cert", func() {
    It("should return error when cert file does not exist", func() {
        // Given: empty temp directory (no tls.crt)
        // When: loadSigningCertificate(logger)
        // Then: error contains "cert" or "not found"
    })
})
```

---

### 4.2 Item 2: Retention Worker Lifecycle

**Business Requirement**: BR-AUDIT-009 (retention policies)
**Files Under Test**: `pkg/datastorage/retention/worker.go` (new), `pkg/datastorage/server/server.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1048-P5-010 | Unit | RED | Worker does not start when `enabled: false` | No goroutine spawned |
| UT-DS-1048-P5-011 | Unit | RED | Worker `Start()` begins periodic purge loop | Tick callback invoked |
| UT-DS-1048-P5-012 | Unit | RED | Worker `Stop()` cancels context and returns | Goroutine exits cleanly |
| UT-DS-1048-P5-013 | Unit | RED | Purge skips rows with `legal_hold = TRUE` | SQL WHERE includes `legal_hold = FALSE` |
| UT-DS-1048-P5-014 | Unit | GREEN | Purge uses `Config.BatchSize` as LIMIT | SQL includes `LIMIT $2` |
| UT-DS-1048-P5-015 | Unit | GREEN | Worker logs purge results (rows\_scanned, rows\_deleted) | Structured log emitted |
| UT-DS-1048-P5-016 | Unit | REFACTOR | Worker writes `retention_operations` record per run | Operation record created with status |
| IT-DS-1048-P5-010 | Integration | RED | Worker purges expired rows from real PostgreSQL | Rows deleted; non-expired remain |
| IT-DS-1048-P5-011 | Integration | RED | Worker skips legal-held rows (trigger interaction) | Legal-held rows survive purge |
| IT-DS-1048-P5-012 | Integration | GREEN | Worker shutdown ordering: stops before DB close | Worker.Stop() called before db.Close() in Shutdown() |
| IT-DS-1048-P5-013 | Integration | REFACTOR | `retention_operations` populated with correct metadata | run\_id, rows\_deleted, duration\_ms present |

**Test Files**:
- `test/unit/datastorage/retention/worker_test.go` (new)
- `test/integration/datastorage/retention_worker_test.go` (new)

#### Config Wiring

```yaml
# config/data-storage.yaml
retention:
  enabled: false          # Opt-in (BR-AUDIT-004: safe default)
  interval: 24h
  batchSize: 1000
  defaultDays: 2555       # ADR-034: 7 years (SOC 2 / ISO 27001)
  partitionDropEnabled: false
```

---

### 4.3 Item 3: Retention DEFAULT Alignment (AU-11)

**Business Requirement**: BR-AUDIT-009, ADR-034
**Files Under Test**: `migrations/009_retention_default_alignment.sql` (new), `pkg/datastorage/config/config.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1048-P5-020 | Unit | RED | Config `retention.defaultDays` parsed from YAML | Field populated with 2555 |
| UT-DS-1048-P5-021 | Unit | GREEN | `retention.defaultDays` used in `ConvertAuditEventRequest` when body omits | Default 2555 applied |
| IT-DS-1048-P5-020 | Integration | RED | After migration 009, column DEFAULT is 2555 | `INSERT` without retention\_days gets 2555 |
| IT-DS-1048-P5-021 | Integration | GREEN | Existing rows with retention\_days=1 (from 008) unaffected | No data migration; column default only |

**Migration 009** (illustrative):

```sql
-- +goose Up
-- Align column default with ADR-034 (SOC 2 / ISO 27001: 7 years)
-- Migration 008 set DEFAULT 1 (minimum); restore to ADR-034 authority.
ALTER TABLE audit_events ALTER COLUMN retention_days SET DEFAULT 2555;

-- +goose Down
ALTER TABLE audit_events ALTER COLUMN retention_days SET DEFAULT 1;
```

**Design Decision**: The `retention.defaultDays` config field allows operators to override the application-level default (e.g., 90 days for non-regulated environments) without changing the DB column default. The column default (2555) is the safety net for bare SQL inserts.

---

### 4.4 Item 4: Actor Identity Enrichment (AU-3)

**Business Requirement**: BR-AUDIT-002 (user attribution), FedRAMP AU-3
**Files Under Test**: `pkg/datastorage/server/audit_events_handler.go`, `pkg/datastorage/server/audit_events_batch_handler.go`, `pkg/datastorage/server/helpers/openapi_conversion.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1048-P5-040 | Unit | RED | `X-Auth-Request-User` header present → overrides body `actor_id` | Stored `actor_id` matches header |
| UT-DS-1048-P5-041 | Unit | RED | `X-Auth-Request-User` header absent → body `actor_id` used | Stored `actor_id` matches body |
| UT-DS-1048-P5-042 | Unit | RED | Both header and body absent → synthetic default `{category}-service` | Backward compatible |
| UT-DS-1048-P5-043 | Unit | RED | `X-Auth-Request-User` header empty string → treated as absent | Body or synthetic used |
| UT-DS-1048-P5-044 | Unit | GREEN | Batch endpoint: single header applies to all events | All N events get header `actor_id` |
| UT-DS-1048-P5-045 | Unit | GREEN | `actor_type` set to `"user"` when header present, `"service"` when absent | Correct type attribution |
| UT-DS-1048-P5-046 | Unit | REFACTOR | `ConvertAuditEventRequest` accepts `authenticatedActorID` parameter | Signature updated |

**Test File**: `test/unit/datastorage/actor_enrichment_test.go` (new)

#### Policy (SEC-S1 resolution)

Server-side attribution from `X-Auth-Request-User` (trusted proxy header) **always overrides** client-provided `actor_id` in the request body. This prevents spoofing and satisfies FedRAMP AU-3 accountability.

| Scenario | Header | Body actor\_id | Stored actor\_id | Stored actor\_type |
|----------|--------|---------------|-----------------|-------------------|
| Authenticated user | `user@example.com` | `other-user` | `user@example.com` | `user` |
| Authenticated user, no body | `user@example.com` | (absent) | `user@example.com` | `user` |
| Service-to-service | (absent) | `gateway-service` | `gateway-service` | `service` |
| No attribution | (absent) | (absent) | `{category}-service` | `service` |

---

### 4.5 Item 5: PEL Recovery (AU-2)

**Business Requirement**: BR-AUDIT-009, FedRAMP AU-2 (no audit event loss)
**Files Under Test**: `pkg/datastorage/dlq/client.go`, `pkg/datastorage/server/dlq_retry_worker.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1048-P5-050 | Unit | RED | Two-phase startup: worker reads PEL (ID `"0"`) before `">"` | `ReadMessages` called with `"0"` first |
| UT-DS-1048-P5-051 | Unit | RED | Worker switches to `">"` after PEL drained (empty response) | Subsequent reads use `">"` |
| UT-DS-1048-P5-052 | Unit | RED | XAUTOCLAIM wrapper claims messages idle > `minIdleTime` | `XAUTOCLAIM` called with correct args |
| UT-DS-1048-P5-053 | Unit | RED | Poison message (delivery count > 5) → dead-letter + XACK | `MoveToDeadLetter` called; message ACKed |
| UT-DS-1048-P5-054 | Unit | GREEN | Janitor sweep runs every 30s (configurable) | Timer triggers claim loop |
| UT-DS-1048-P5-055 | Unit | GREEN | Reclaimed message processed normally (retry logic) | Same path as new message after claim |
| UT-DS-1048-P5-056 | Unit | REFACTOR | Valkey version check at startup for XAUTOCLAIM support | Log warning if < 6.2 |
| IT-DS-1048-P5-050 | Integration | RED | Orphaned message recovered after consumer restart (real Redis) | Message processed and ACKed |
| IT-DS-1048-P5-051 | Integration | RED | Poison message dead-lettered after max deliveries (real Redis) | Message in dead-letter stream; original ACKed |
| IT-DS-1048-P5-052 | Integration | GREEN | PEL drain at startup processes all pending before new messages | All pre-existing pending processed first |

**Test Files**:
- `test/unit/datastorage/pel_recovery_test.go` (new)
- `test/integration/datastorage/pel_recovery_test.go` (new)

#### Design: Two-Phase Startup

```
Worker.Start():
  1. phase = "pel_drain"
  2. loop: ReadMessages(ctx, auditType, group, consumer, "0", count, timeout)
     - if empty → phase = "live"; break
     - else → process each message normally
  3. phase = "live"
  4. loop: ReadMessages(ctx, auditType, group, consumer, ">", count, timeout)
     - normal retry worker behavior
```

#### Design: XAUTOCLAIM Janitor

```
Every 30s:
  1. XAUTOCLAIM stream group consumer minIdleTime(60s) startID("0-0") COUNT 10
  2. For each claimed message:
     a. Check delivery count (from XPENDING or XAUTOCLAIM response)
     b. If deliveryCount > maxDeliveries(5) → MoveToDeadLetter + XACK
     c. Else → process normally (same as processMessage)
```

#### DLQ Client Addition

```go
func (c *Client) AutoClaimMessages(ctx context.Context, auditType, consumerGroup, consumer string,
    minIdleTime time.Duration, startID string, count int64) ([]DLQMessage, string, error)
```

Wraps `redis.XAutoClaim` and returns claimed messages + new start ID for cursor-based pagination.

---

### 4.6 Item 6: Redis TLS Client (AU-9, SC-8)

**Business Requirement**: FedRAMP AU-9, SC-8
**Files Under Test**: `pkg/datastorage/config/config.go` (`RedisConfig.TLS`), `pkg/datastorage/server/server.go`
**Operator Dependency**: [kubernaut-operator#89](https://github.com/jordigilh/kubernaut-operator/issues/89)

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1048-P5-060 | Unit | RED | `RedisConfig.TLS` parsed from YAML with all fields | Config struct populated |
| UT-DS-1048-P5-061 | Unit | RED | `TLS.Enabled = true` without cert paths → startup error | Error returned from NewServer |
| UT-DS-1048-P5-062 | Unit | RED | `TLS.Enabled = false` → `redis.Options.TLSConfig` is nil | Plaintext connection |
| UT-DS-1048-P5-063 | Unit | GREEN | `TLS.Enabled = true` with valid paths → `tls.Config` populated | TLSConfig has CA, cert, key |
| UT-DS-1048-P5-064 | Unit | GREEN | `TLS.InsecureSkipVerify` propagated to `tls.Config` | Field set correctly |
| UT-DS-1048-P5-065 | Unit | REFACTOR | Config validation: TLS enabled requires at minimum `caFile` | Validation error on missing CA |

**Test File**: `test/unit/datastorage/redis_tls_test.go` (new)

#### Config Addition

```yaml
redis:
  addr: "valkey:6379"
  dlqMaxLen: 10000
  secretsFile: "/etc/datastorage/secrets/redis-credentials.yaml" # pre-commit:allow-sensitive
  passwordKey: "password"
  tls:
    enabled: false
    certFile: ""       # Client cert (mTLS)
    keyFile: ""        # Client key (mTLS)
    caFile: ""         # CA bundle to verify server
    insecureSkipVerify: false
```

**Note**: Integration tests for actual TLS handshake require a TLS-enabled Redis/Valkey instance. This depends on operator issue #89. Until then, unit tests validate config wiring and `tls.Config` construction.

---

### 4.7 Item 7: DLQ Trim Alerting (AU-11)

**Business Requirement**: BR-AUDIT-009, FedRAMP AU-11
**Files Under Test**: `pkg/datastorage/dlq/client.go`, `pkg/datastorage/dlq/metrics.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1048-P5-070 | Unit | RED | `datastorage_dlq_stream_trim_total` counter incremented on XADD | Counter +1 per enqueue |
| UT-DS-1048-P5-071 | Unit | RED | Counter has `stream` label (e.g., `notifications`, `audit_events`) | Labels correct |
| UT-DS-1048-P5-072 | Unit | GREEN | Helm `dlqMaxLen` default aligned to `10000` (not `1000`) | Helm template renders 10000 |
| UT-DS-1048-P5-073 | Unit | REFACTOR | RPO documentation added to runbook | Markdown present in docs/ |

**Test File**: `test/unit/datastorage/dlq_trim_metrics_test.go` (new)

#### Metric Definition

```go
var DLQStreamTrimTotal = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: "datastorage",
        Subsystem: "dlq",
        Name:      "stream_xadd_total",
        Help:      "Total XADD operations (each may trigger MAXLEN~ trim)",
    },
    []string{"stream"},
)
```

**Note**: Redis does not report whether a trim actually occurred on `XADD MAXLEN ~`. The metric counts XADD operations; combined with `datastorage_dlq_depth` gauge (existing), operators can detect when depth plateaus at `maxLen` — indicating active trimming.

#### RPO Documentation

A new section in `docs/operations/runbooks/data-storage.md` documenting:
- `dlqMaxLen` × avg message size = max unsynced audit data at risk
- Alert threshold: `datastorage_dlq_depth / dlqMaxLen > 0.95` sustained for 5 minutes
- Response: increase `dlqMaxLen`, investigate Postgres availability, check DLQ retry worker health

---

## 5. Test Execution Order (TDD Phases)

Following RED → GREEN → REFACTOR per Kubernaut core rules.

### Phase RED (write failing tests first)

| Order | Test IDs | Item | Rationale |
|-------|----------|------|-----------|
| 1 | UT-DS-1048-P5-001..004 | Signer fail-hard | Foundation: server won't start without cert |
| 2 | UT-DS-1048-P5-010..013 | Retention worker | Core lifecycle + safety (legal hold) |
| 3 | UT-DS-1048-P5-020..021 | Retention default | Config alignment with ADR-034 |
| 4 | UT-DS-1048-P5-040..043 | Actor enrichment | AU-3 identity attribution |
| 5 | UT-DS-1048-P5-050..053 | PEL recovery | AU-2 no audit loss |
| 6 | UT-DS-1048-P5-060..062 | Redis TLS config | AU-9 transport security |
| 7 | UT-DS-1048-P5-070..071 | Trim metrics | AU-11 visibility |
| 8 | IT-DS-1048-P5-010..011 | Retention integration | Real PostgreSQL purge |
| 9 | IT-DS-1048-P5-020..021 | Retention migration | DDL verification |
| 10 | IT-DS-1048-P5-050..052 | PEL integration | Real Redis XAUTOCLAIM |

### Phase GREEN (minimal implementation to pass)

| Order | Test IDs | Item |
|-------|----------|------|
| 1 | UT-DS-1048-P5-005..006 | Signer config wiring |
| 2 | UT-DS-1048-P5-014..015 | Retention batch + logging |
| 3 | UT-DS-1048-P5-044..045 | Actor batch + type |
| 4 | UT-DS-1048-P5-054..055 | PEL janitor interval + processing |
| 5 | UT-DS-1048-P5-063..064 | Redis TLS construction |
| 6 | UT-DS-1048-P5-072 | Helm alignment |
| 7 | IT-DS-1048-P5-012..013 | Retention shutdown + operations |
| 8 | IT-DS-1048-P5-052 | PEL startup drain ordering |

### Phase REFACTOR (enhance existing code)

| Order | Test IDs | Item |
|-------|----------|------|
| 1 | UT-DS-1048-P5-007 | Dead code removal (fallback) |
| 2 | UT-DS-1048-P5-016 | Retention operations table |
| 3 | UT-DS-1048-P5-046 | ConvertAuditEventRequest signature |
| 4 | UT-DS-1048-P5-056 | Valkey version check |
| 5 | UT-DS-1048-P5-065 | TLS config validation |
| 6 | UT-DS-1048-P5-073 | RPO documentation |

---

## 6. Test Infrastructure Requirements

| Requirement | Status | Notes |
|-------------|--------|-------|
| Ginkgo/Gomega BDD framework | Available | Standard for all kubernaut tests |
| PostgreSQL (integration) | Available | Existing `test/integration/datastorage/` infrastructure |
| Redis/Valkey (integration) | Available | Used by existing DLQ integration tests |
| Test certificates (unit) | Generate in test | Use `cert.GenerateSelfSigned` from `pkg/cert/` |
| `os.MkdirTemp` for cert paths | Standard library | Signer tests write temp cert files |
| Mock `*sql.DB` | Available | Existing `DATA-MOCK` pattern in `repository_test.go` |
| XAUTOCLAIM support | Verify | Must confirm Valkey version >= 6.2 equivalent |

---

## 7. Coverage Targets

| Tier | Scope | Target | Measurement |
|------|-------|--------|-------------|
| Unit | Signer init, retention config, actor enrichment, PEL logic, TLS config, trim metrics | >=80% | `go test -cover ./pkg/datastorage/...` |
| Integration | Retention purge, PEL recovery, migration DDL | >=80% | `go test -cover ./test/integration/datastorage/...` |
| All tiers merged | Phase 5 code | >=80% | Line-by-line dedup |

---

## 8. Compliance Sign-Off

### Pre-Implementation Checklist

| Requirement | Evidence | Status |
|-------------|----------|--------|
| All test scenarios mapped to BR/FedRAMP control | This document §4 | ✅ |
| TDD execution order defined (RED → GREEN → REFACTOR) | This document §5 | ✅ |
| Risk-to-test traceability complete | This document §3 | ✅ |
| Operator dependencies tracked | Issues #89, #90 | ✅ |
| Retention default aligned with ADR-034 | §4.3 | ✅ |
| PEL recovery design specified | §4.5 | ✅ |
| Actor enrichment policy documented | §4.4 | ✅ |

### Post-Implementation Checklist

| Requirement | Evidence | Status |
|-------------|----------|--------|
| All unit tests passing | `go test` output | ⬜ |
| All integration tests passing | `go test` output | ⬜ |
| Build succeeds (`go build ./...`) | Build output | ⬜ |
| No lint errors (`golangci-lint run`) | Lint output | ⬜ |
| Coverage >= 80% per tier | Coverage report | ⬜ |
| Readiness audit confidence >= 95% | Audit document | ⬜ |

### Approval

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Developer | | | ⬜ |
| Reviewer | | | ⬜ |
| Team Lead | | | ⬜ |

---

## Appendix A: Test File Locations

| Test Category | Location |
|---------------|----------|
| Signer fail-hard (unit) | `test/unit/datastorage/signer_test.go` |
| Retention worker (unit) | `test/unit/datastorage/retention/worker_test.go` |
| Retention config (unit) | `test/unit/datastorage/retention/retention_test.go` (existing + extend) |
| Actor enrichment (unit) | `test/unit/datastorage/actor_enrichment_test.go` |
| PEL recovery (unit) | `test/unit/datastorage/pel_recovery_test.go` |
| Redis TLS (unit) | `test/unit/datastorage/redis_tls_test.go` |
| DLQ trim metrics (unit) | `test/unit/datastorage/dlq_trim_metrics_test.go` |
| Retention worker (integration) | `test/integration/datastorage/retention_worker_test.go` |
| Retention migration (integration) | `test/integration/datastorage/retention_test.go` (existing + extend) |
| PEL recovery (integration) | `test/integration/datastorage/pel_recovery_test.go` |

## Appendix B: Deferred to Later Phases

| Item | Phase | Rationale |
|------|-------|-----------|
| PEL metrics (pending count, max idle) | Phase 7 (Observability) | SRE-H1 scope |
| Retention Prometheus metrics (purge count, duration) | Phase 7 (Observability) | SRE-S3 scope |
| Redis TLS integration test (real TLS handshake) | After operator#89 | Requires Valkey TLS server |
| Doc cleanup: DD-AUDIT-003 / security-configuration.md retention inconsistencies | Phase 15 (Product docs) | PROD-M2 scope |
