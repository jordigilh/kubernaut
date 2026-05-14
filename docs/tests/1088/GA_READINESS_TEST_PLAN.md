# Test Plan: Data Storage GA Readiness Remediation (#1088)

> **Template Version**: 2.0 -- Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1088-GA-v1.0
**Feature**: GA readiness remediation for Data Storage -- 58 audit findings across 9 categories
**Version**: 1.0
**Created**: 2026-05-13
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.5`
**Tracking Issue**: [#1088](https://github.com/jordigilh/kubernaut/issues/1088)
**Parent Issue**: [#1048](https://github.com/jordigilh/kubernaut/issues/1048)
**Audit Reference**: GA Readiness Audit (commit 05d895087, May 13 2026)

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the remediation of all 58 findings from the comprehensive GA readiness audit performed on the Data Storage service. The audit covered 9 categories: Architecture, Data Flow, Security, FedRAMP, SRE, Performance, Test Quality, Product/API, and Operations.

### 1.2 Objectives

1. **Data integrity**: Internal audit client participates in hash chain; DLQ trim is observable; drain errors propagate.
2. **Config safety**: Production defaults enforce TLS, retention, and restrictive CORS.
3. **Security controls**: Rate limiting, auth-first middleware ordering, panic recovery, export payload signing.
4. **Performance**: Bounded queries, batch FK lookups, streaming verification.
5. **FedRAMP compliance**: AU-2 auth audit events, AU-9 export integrity, AU-11 category retention floors, AC-6 SAR granularity.
6. **Architecture**: No layer inversions, no god objects, no dead code.
7. **Test quality**: Handler-level coverage, no time.Sleep anti-patterns, behavioral assertions.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/datastorage/...` |
| Integration test pass rate | 100% | `go test ./test/integration/datastorage/...` |
| Unit-testable code coverage | >=80% | `make coverage-report` |
| Integration-testable code coverage | >=80% | `make coverage-report` |
| Critical findings resolved | 8/8 | Checkpoint D |
| High findings resolved | 16/16 | Checkpoint E |
| Medium findings resolved | 22/22 | Checkpoint F |
| Low findings resolved | 12/12 | Checkpoint G |
| Anti-pattern violations | 0 | Pre-commit hook + manual scan |
| Go build clean | 0 errors | `go build ./...` |

---

## 2. References

### 2.1 Authority (governing documents)

- **ADR-034**: Unified audit table design -- `retention_days INTEGER DEFAULT 2555`
- **ADR-032**: Data access layer isolation
- **BR-AUDIT-001**: At-least-once audit event delivery
- **BR-AUDIT-002**: User attribution (actor identity)
- **BR-AUDIT-004**: Immutable audit logs with integrity protection
- **BR-AUDIT-007**: Tamper-evident audit exports (signed)
- **BR-AUDIT-009**: Retention policies meeting regulatory requirements
- **DD-007 / DD-008 / DD-009**: Graceful shutdown, DLQ drain, DLQ retry worker
- **DD-AUTH-014**: In-process middleware authentication
- **FedRAMP**: AU-2, AU-3, AU-9, AU-11, AC-4, AC-6, SC-8, SI-10

### 2.2 Cross-References

- [GA Readiness Audit Canvas](../../canvases/ds-ga-readiness-audit.canvas.tsx)
- [Phase 5 Test Plan](../1048/PHASE5_TEST_PLAN.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [100 Go Mistakes](https://100go.co/)

---

## 3. Risks and Mitigations

| ID | Risk | Impact | Probability | Affected Findings | Mitigation |
|----|------|--------|-------------|-------------------|------------|
| R1 | InternalAuditClient hash chain changes break other services | Service integration failure | Medium | DF-C1 | API-compatible change; other services call StoreBatch() unchanged |
| R2 | DLQ MAXLEN trim observability adds overhead | Performance regression | Low | DF-C2 | Counter increment is O(1); structured log only on trim |
| R3 | DrainWithTimeout error contract change breaks shutdown | Shutdown hangs or fails | Medium | DF-H1, SRE-M2 | Errors are joined, not fatal; shutdown continues |
| R4 | Retention category floor enforcement changes purge behavior | Unexpected data retention | Medium | FED-H1 | MAX() only extends retention, never shortens |
| R5 | Middleware reordering breaks OpenAPI validation | 400 errors not caught | Medium | SEC-H2 | Auth before validation; validation still runs for authed requests |
| R6 | Rate limiting blocks legitimate high-volume clients | Service degradation | Medium | SEC-H1 | Configurable threshold; disabled by default in dev |
| R7 | Architecture refactoring introduces import cycles | Build failure | High | ARCH-C1 | Use gopls for type-safe moves; validate with go build |
| R8 | Export payload signing performance impact | Slow export responses | Low | FED-H2 | SHA256 is fast; events already loaded in memory |

---

## 4. Test Scenarios

### 4.1 Phase 9C: Data Integrity

**Business Requirements**: BR-AUDIT-001, BR-AUDIT-004, BR-AUDIT-009
**FedRAMP Controls**: AU-2, AU-9, AU-11

#### 4.1.1 DF-C1: Internal Client Hash Chain Participation

**Files Under Test**: `pkg/audit/internal_client.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-GA-001 | Unit | RED | InternalAuditClient.StoreBatch events have event_hash set | event_hash is non-empty SHA256 |
| UT-DS-1088-GA-002 | Unit | RED | InternalAuditClient events pass verify-chain validation | PrepareEventForHashing + SHA256 matches stored hash |
| IT-DS-1088-GA-003 | Integration | RED | Internal + external events form continuous per-correlation chain | previous_event_hash links to prior event |

**Test File**: `test/unit/datastorage/internal_client_integrity_test.go` (new)

#### 4.1.2 DF-C2: DLQ MAXLEN Trim Observability

**Files Under Test**: `pkg/datastorage/dlq/client.go`, `pkg/datastorage/dlq/metrics.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-GA-010 | Unit | RED | XAdd with MAXLEN emits trim counter when stream exceeds capacity | Counter incremented |
| UT-DS-1088-GA-011 | Unit | RED | Trim event emits structured log with stream name and count | Log entry with "dlq_trim" message |

**Test File**: `test/unit/datastorage/dlq/trim_observability_test.go` (new)

#### 4.1.3 DF-H1 / SRE-M2: Drain Error Contract

**Files Under Test**: `pkg/datastorage/dlq/client.go`, `pkg/datastorage/server/server.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-GA-020 | Unit | RED | DrainWithTimeout returns error when drain has per-message failures | Non-nil error returned |
| UT-DS-1088-GA-021 | Unit | RED | Shutdown propagates drain errors into shutdownErrors | errors.Join includes drain error |

**Test File**: `test/unit/datastorage/dlq/drain_error_contract_test.go` (new)

#### 4.1.4 DF-H2 / DF-M1: Internal Client Alignment

**Files Under Test**: `pkg/audit/internal_client.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-GA-030 | Unit | RED | InternalAuditClient uses configurable RetentionDays | RetentionDays matches config, not hardcoded 90 |
| UT-DS-1088-GA-031 | Unit | RED | InternalAuditClient validates EventData before insert | ValidateEventData called; oversized rejected |

**Test File**: `test/unit/datastorage/internal_client_integrity_test.go` (extends 4.1.1)

#### 4.1.5 SRE-H2: Redis Client Shutdown

**Files Under Test**: `pkg/datastorage/server/server.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-GA-040 | Unit | RED | Shutdown closes Redis client after DLQ drain | redisClient.Close() called |

**Test File**: extends `test/unit/datastorage/phase7_shutdown_test.go`

#### 4.1.6 FED-H1 / FED-M3: Retention Category Floors

**Files Under Test**: `pkg/datastorage/retention/retention.go`

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-GA-050 | Unit | RED | PurgeSQLBatched uses MAX(event.retention_days, category_floor) | SQL WHERE uses GREATEST() |
| UT-DS-1088-GA-051 | Unit | RED | EffectiveRetention applied: event with 30d but category floor 365d survives purge | Event not deleted |

**Test File**: `test/unit/datastorage/retention/category_floor_test.go` (new)

---

### 4.2 Phase 10: Security Controls

**Business Requirements**: BR-AUDIT-007, BR-STORAGE-024
**FedRAMP Controls**: AU-2, AU-9, AC-6, SC-8

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-GA-100 | Unit | RED | Rate limiter returns 429 when threshold exceeded | HTTP 429 response |
| UT-DS-1088-GA-110 | Unit | RED | Unauthenticated request gets 401 before OpenAPI validation | 401 returned; no validation error |
| UT-DS-1088-GA-120 | Unit | RED | Panic in handler returns 500, does not re-panic | 500 response; goroutine survives |
| UT-DS-1088-GA-130 | Unit | RED | Export signature covers event body SHA256 hashes | Signed data includes event_hashes array |
| UT-DS-1088-GA-140 | Unit | RED | Export endpoint requires "export" SAR verb | 403 without "export" verb |
| UT-DS-1088-GA-150 | Unit | RED | 401 response emits structured security audit log | Log entry with security_event type |
| UT-DS-1088-GA-160 | Unit | RED | HTTP access log includes authenticated principal | Log entry has "user" field |

**Test Files**:
- `test/unit/datastorage/security_controls_test.go` (new)
- `test/unit/datastorage/export_signing_test.go` (new)

---

### 4.3 Phase 11: Performance Hardening

**Business Requirements**: BR-STORAGE-001 to BR-STORAGE-020
**FedRAMP Controls**: SI-10

| ID | Tier | TDD Phase | Description | Expected Result |
|----|------|-----------|-------------|-----------------|
| UT-DS-1088-GA-200 | Unit | RED | Batch handler with N parent_event_ids makes <=2 queries | Query count <= 2 |
| UT-DS-1088-GA-210 | Unit | RED | Effectiveness query has LIMIT | SQL contains LIMIT clause |
| UT-DS-1088-GA-220 | Unit | RED | Remediation history EM fetch has LIMIT | SQL contains LIMIT clause |
| UT-DS-1088-GA-230 | Unit | RED | Verify-chain processes rows streaming | No full slice accumulation |
| UT-DS-1088-GA-240 | Unit | RED | ParentEventDate carried through conversion | Repository event has ParentEventDate set |
| UT-DS-1088-GA-250 | Unit | GREEN | Actor trust boundary documented | Code comment present |
| UT-DS-1088-GA-260 | Unit | RED | DLQ LastError field is sanitized | No raw SQL/driver strings |
| UT-DS-1088-GA-270 | Unit | RED | 400 responses use generic validation message | No field-specific details leaked |

**Test Files**:
- `test/unit/datastorage/performance_hardening_test.go` (new)
- `test/unit/datastorage/error_redaction_test.go` (new)

---

## 5. Coverage Targets

| Tier | Code Subset | Target | Files in Scope |
|------|-------------|--------|----------------|
| Unit | pkg/datastorage/{config,dlq,metrics,models,retention,validation,schema,uuid} | >=80% | Pure logic, config, validators |
| Unit | pkg/audit/internal_client.go | >=80% | Hash chain, validation |
| Integration | pkg/datastorage/{server,repository,dlq/client} | >=80% | HTTP handlers, DB, Redis |
| All Tiers | Full pkg/datastorage/ (merged) | >=80% | Line-by-line dedup |

---

## 6. Anti-Pattern Checklist

Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md):

- [ ] No `time.Sleep()` before assertions -- use `Eventually`/`Consistently`
- [ ] No `Skip()` in any test tier
- [ ] No `ToNot(BeNil())` null-testing anti-pattern -- assert behavioral outcomes
- [ ] No direct audit infrastructure testing -- test business logic with audit side effects
- [ ] No `XIt` or `PIt` pending tests
- [ ] All tests use Ginkgo/Gomega BDD framework
- [ ] Test scenario IDs follow format: `{TIER}-DS-1088-GA-{SEQUENCE}`
- [ ] Error path tests present for every success path
- [ ] Mock only external dependencies (Redis via miniredis, DB via sqlmock or real Podman)

---

## 7. Go 100 Mistakes Validation Checklist

Applied during TDD Refactor phases:

### Phase 9C-Refactor (Data Integrity)
- [ ] #1: No variable shadowing in refactored code
- [ ] #28: `defer` used for cleanup (Redis close, DB connections)
- [ ] #48: No ignored returned errors
- [ ] #53: Defer errors handled (shutdown error joins)
- [ ] #54: Errors wrapped with context (`fmt.Errorf("...: %w", err)`)
- [ ] #76: No `time.After` memory leaks in DLQ timers

### Phase 10-Refactor (Security Controls)
- [ ] #5: No interface pollution (rate limiter interface minimal)
- [ ] #6: Interfaces on consumer side (auth middleware)
- [ ] #11: Functional options for rate limit config
- [ ] #49: HTTP request bodies properly closed

### Phase 11-Refactor (Performance)
- [ ] #41: `strings.Builder` for SQL construction (not `+` concatenation)
- [ ] #78: Parameterized SQL (no string interpolation)
- [ ] #79: `defer rows.Close()` on all query paths
- [ ] #80: Connection pool properly configured

### Phase 13-Refactor (Architecture)
- [ ] #1: No variable shadowing after splits
- [ ] #2: Unnecessary nesting reduced (early returns)
- [ ] #5: New interfaces minimal
- [ ] #10: Type embedding does not expose internals
- [ ] #12: Package layout follows Go conventions

---

## 8. Checkpoint Criteria

### Checkpoint D (Post-Critical)
- [ ] All 8 Critical findings resolved (ARCH-C1, DF-C1, DF-C2, FED-C1, FED-C2, SRE-C1, PROD-C1, OPS-C1)
- [ ] `go build ./...` clean
- [ ] Unit tests 100% pass
- [ ] Each finding at >=95% confidence
- [ ] Coverage >=80% for modified code

### Checkpoint E (Post-P1)
- [ ] All 16 High findings resolved
- [ ] Integration tests pass
- [ ] Performance benchmarks for batch FK, unbounded queries
- [ ] Each finding at >=95% confidence

### Checkpoint F (Post-P2)
- [ ] All 22 Medium findings resolved
- [ ] Architecture: no layer inversions
- [ ] No anti-patterns in test suite
- [ ] Each finding at >=95% confidence

### Checkpoint G (Final GA)
- [ ] All 58 findings resolved (including 12 Low)
- [ ] Full test suite passes (unit + integration)
- [ ] Coverage >=80% per tier
- [ ] Helm template renders cleanly
- [ ] OpenAPI spec and embedded spec in sync
- [ ] **Go/no-go for GA**
