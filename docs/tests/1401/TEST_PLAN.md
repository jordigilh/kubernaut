# Test Plan: KA Audit Correlation ID and Event Data Compliance

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1401-v1
**Feature**: Fix empty correlation_id and missing event_data in KA security audit events
**Version**: 1.0
**Created**: 2026-06-11
**Author**: AI Agent
**Status**: Draft
**Branch**: `feat/structured-decision-payload`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fix for KA security audit events (`aiagent.ratelimit.denied`, `aiagent.auth.failure`, `aiagent.auth.denied`) that currently fail OpenAPI validation and are silently dropped by the buffered audit store. Two defects are addressed:

1. **Empty correlation_id**: `NewEvent(..., "")` passes an empty string that violates `minLength: 1` on the OpenAPI spec's `correlation_id` field.
2. **Missing event_data**: No `buildEventData` case exists for these event types, so `EventData` remains the zero-value union (`{}`), which doesn't match any `oneOf` discriminator member.

Both violations cause the OpenAPI validator in `pkg/audit/openapi_validator.go` to reject these events, resulting in silent data loss for FedRAMP AU-12 compliance-critical security events.

### 1.2 Objectives

1. **Non-empty correlation_id**: All security audit events use a synthetic `"security-" + uuid` correlation ID
2. **Valid event_data**: `buildEventData` returns typed payloads for all 3 security event types
3. **OpenAPI compliance**: Events pass `ValidateAuditEventRequest` without error
4. **Persistence**: Security events reach DataStorage (not silently dropped)
5. **Schema addition**: 3 new OpenAPI schemas added to `data-storage-v1.yaml` with discriminator mappings
6. **ogen regeneration**: Generated client types available for `buildEventData` cases

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./internal/kubernautagent/audit/... -run "UT-KA-1401"` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/... -run "IT-KA-1401"` |
| E2E test pass rate | 100% | `make test-e2e-kubernautagent` (audit pipeline focus) |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on audit logic |
| OpenAPI validation pass rate | 100% | All 3 event types pass `ValidateAuditEventRequest` |
| Backward compatibility | 0 regressions | `UT-KA-PR9-001` buildEventData coverage test passes |
| Event persistence | 3/3 event types persisted | E2E verifies events in DS query |

---

## 2. References

### 2.1 Authority

- Issue #1401: KA audit correlation ID and event_data compliance
- BR-AUDIT-070: Every event type in `AllEventTypes` must have a valid `buildEventData` case
- ADR-038: Audit must never block business logic (best-effort persistence)
- FedRAMP AU-12: Audit generation (all security-relevant events must be recorded)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Wiring Verification](../../.cursor/rules/10-wiring-verification.mdc)
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes) — refactoring validation
- Existing test: `internal/kubernautagent/audit/ds_store_test.go` (UT-KA-PR9-001)
- Existing E2E: `test/e2e/kubernautagent/audit_pipeline_test.go`
- OpenAPI spec: `api/openapi/data-storage-v1.yaml`
- Apifrontend reference: `ApifrontendRatelimitDeniedPayload` (same pattern)

### 2.3 FedRAMP Control Objectives

| Control | NIST Intent | Application to This Feature |
|---------|-------------|----------------------------|
| **AU-3** | Audit content | Security events must include source_ip, path, method, event_id |
| **AU-12** | Audit generation | Rate-limit and auth events MUST be persisted (not silently dropped) |
| **SI-4** | Information system monitoring | Security events enable real-time alerting on brute-force/DDoS |
| **SI-10** | Input validation | OpenAPI schema validates event_data structure before persistence |
| **AC-7** | Unsuccessful login attempts | Auth failure events provide login attempt tracking |

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | OpenAPI YAML syntax error in new schemas | ogen regen fails | Medium | UT-KA-1401-006 | Mirror exact structure of existing `ApifrontendRatelimitDeniedPayload` |
| R2 | ogen version drift causes regen issues | Build break | Low | All | Pin ogen version from existing `gen.go` directive |
| R3 | Discriminator mapping collision | Runtime dispatch failure | Very Low | UT-KA-1401-007 | Event type strings are unique (aiagent.* prefix) |
| R4 | UUID generation adds latency to hot path | Performance impact | Very Low | N/A | `uuid.New()` is <1us; rate-limit path is already slow (429 response) |
| R5 | Large ogen regen diff obscures real changes | Review difficulty | Medium | N/A | Commit regen in separate commit from business logic |

---

## 4. Scope

### 4.1 Features to be Tested

- **correlation_id generation** (`internal/kubernautagent/server/ratelimit.go`, `audit_middleware.go`): Synthetic UUID-based correlation IDs
- **OpenAPI schemas** (`api/openapi/data-storage-v1.yaml`): 3 new payload schemas + discriminator mappings
- **buildEventData cases** (`internal/kubernautagent/audit/ds_store.go`): 3 new switch cases
- **Event persistence** (end-to-end): Security events survive OpenAPI validation and reach DataStorage

### 4.2 Features Not to be Tested

- **Rate limiter logic**: Token bucket algorithm unchanged; tested in existing `ratelimit_test.go`
- **Auth middleware delegation**: Auth decision logic unchanged; tested in existing auth tests
- **BufferedDSAuditStore buffering**: Channel mechanics unchanged; tested in `coverage_668_test.go`
- **DataStorage batch persistence**: Already tested in `test/integration/datastorage/`

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| `"security-" + uuid.New().String()` for correlation_id | No natural correlation (unlike RR-based events); prefix enables filtering; UUID ensures uniqueness |
| Separate payload per event type (not shared) | OpenAPI discriminator requires distinct `event_type` enum per schema |
| Fields: `event_id`, `source_ip`, `path`, `method` | Matches what emitters already set in `event.Data` map |
| Same fields for auth and rate-limit payloads | Consistent structure; all capture HTTP request context |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (correlation_id generation, buildEventData cases)
- **Integration**: >=80% of integration-testable code (full event → OpenAPI validation → DS client)
- **E2E**: >=80% of audit pipeline exercised (HTTP 429 → event persisted in DS)

### 5.2 Pyramid Invariant

> UT proves logic (correlation_id non-empty, buildEventData maps fields correctly). IT proves wiring (events pass OpenAPI validation in buffered store). E2E proves the journey (rate-limited request → audit event in DataStorage).

### 5.3 Pass/Fail Criteria

**PASS**:
1. All P0 tests pass (0 failures)
2. `skipPayloadCheck` map in `ds_store_test.go` no longer contains the 3 security event types
3. `UT-KA-PR9-001` passes without `skipPayloadCheck` exclusions for security events
4. Per-tier coverage >=80%
5. ogen regen produces valid Go code that compiles

**FAIL**:
1. Any P0 test fails
2. `ValidateAuditEventRequest` rejects security events
3. Build fails after ogen regen
4. Security events still silently dropped

### 5.4 Suspension & Resumption Criteria

**Suspend**: ogen regeneration produces uncompilable code; OpenAPI spec has validation errors
**Resume**: ogen issues resolved; spec validated with `oapi-codegen` or `swagger-cli validate`

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/server/ratelimit.go` | `Middleware` (correlation_id change) | ~2 |
| `internal/kubernautagent/server/audit_middleware.go` | `AuditAuthMiddleware` (correlation_id change) | ~4 |
| `internal/kubernautagent/audit/ds_store.go` | `buildEventData` (3 new cases) | ~30 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/audit/ds_store.go` | `StoreAudit` → `buildRequest` → DS client | ~10 |
| `pkg/audit/openapi_validator.go` | `ValidateAuditEventRequest` | ~5 (validates new payloads) |

### 6.3 Schema Files (generated)

| File | Change | Lines (approx) |
|------|--------|-----------------|
| `api/openapi/data-storage-v1.yaml` | 3 new schemas + discriminator entries | ~60 |
| `pkg/datastorage/ogen-client/oas_schemas_gen.go` | Generated payload types | Auto |
| `pkg/audit/openapi_spec_data.yaml` | Embedded copy update | Auto |

---

## 7. BR Coverage Matrix

| BR/Issue | Description | Priority | Tier | Test ID | Status |
|----------|-------------|----------|------|---------|--------|
| #1401 | Rate-limit event has non-empty correlation_id | P0 | Unit | UT-KA-1401-001 | Pending |
| #1401 | Auth failure event has non-empty correlation_id | P0 | Unit | UT-KA-1401-002 | Pending |
| #1401 | Auth denied event has non-empty correlation_id | P0 | Unit | UT-KA-1401-003 | Pending |
| #1401 | correlation_id has "security-" prefix | P1 | Unit | UT-KA-1401-004 | Pending |
| #1401 | correlation_id is valid UUID after prefix | P1 | Unit | UT-KA-1401-005 | Pending |
| #1401 | buildEventData returns typed payload for rate-limit | P0 | Unit | UT-KA-1401-006 | Pending |
| #1401 | buildEventData returns typed payload for auth failure | P0 | Unit | UT-KA-1401-007 | Pending |
| #1401 | buildEventData returns typed payload for auth denied | P0 | Unit | UT-KA-1401-008 | Pending |
| #1401 | Payload contains source_ip, path, method | P0 | Unit | UT-KA-1401-009 | Pending |
| #1401 | UT-KA-PR9-001 passes without skipPayloadCheck for security events | P0 | Unit | UT-KA-1401-010 | Pending |
| #1401 | Security event passes OpenAPI validation | P0 | Integration | IT-KA-1401-001 | Pending |
| #1401 | Rate-limit event persisted to DS (full pipeline) | P0 | Integration | IT-KA-1401-002 | Pending |
| #1401 | Auth failure event persisted to DS | P0 | Integration | IT-KA-1401-003 | Pending |
| #1401 | E2E: HTTP 429 produces persisted audit event | P0 | E2E | E2E-KA-1401-001 | Pending |
| #1401 | E2E: HTTP 401 produces persisted audit event | P1 | E2E | E2E-KA-1401-002 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`
- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: `KA` (KubernautAgent)
- **ISSUE**: `1401`
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| `UT-KA-1401-001` | AU-12: Rate-limit audit event has non-empty correlation_id | AU-12 | A |
| `UT-KA-1401-002` | AU-12: Auth failure audit event has non-empty correlation_id | AU-12 | A |
| `UT-KA-1401-003` | AU-12: Auth denied audit event has non-empty correlation_id | AU-12 | A |
| `UT-KA-1401-004` | AU-3: correlation_id has "security-" prefix for traceability | AU-3 | A |
| `UT-KA-1401-005` | AU-3: correlation_id suffix is valid UUID v4 | AU-3 | A |
| `UT-KA-1401-006` | SI-10: buildEventData returns valid AIAgentRatelimitDeniedPayload | SI-10 | C |
| `UT-KA-1401-007` | SI-10: buildEventData returns valid AIAgentAuthFailurePayload | SI-10 | C |
| `UT-KA-1401-008` | SI-10: buildEventData returns valid AIAgentAuthDeniedPayload | SI-10 | C |
| `UT-KA-1401-009` | AU-3: Payloads contain source_ip, path, method from event.Data | AU-3 | C |
| `UT-KA-1401-010` | AU-12: UT-KA-PR9-001 passes with security events included (no skip) | AU-12 | C |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| `IT-KA-1401-001` | SI-10: Security audit events pass OpenAPI `ValidateAuditEventRequest` | SI-10 | D |
| `IT-KA-1401-002` | AU-12: Rate-limit event round-trips through BufferedDSAuditStore to DS client | AU-12 | D |
| `IT-KA-1401-003` | AU-12: Auth failure event round-trips through BufferedDSAuditStore | AU-12 | D |

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| `E2E-KA-1401-001` | AU-12: HTTP 429 from rate limiter produces queryable audit event in DataStorage | AU-12 | E |
| `E2E-KA-1401-002` | AC-7: HTTP 401 from invalid credentials produces queryable audit event | AC-7 | E |

**Infrastructure**: Existing E2E KA audit pipeline cluster (Kind + DataStorage + KA). Trigger rate-limit by burst requests; trigger 401 by invalid bearer token.

---

## 9. Test Cases

### UT-KA-1401-001: Rate-limit event has non-empty correlation_id

**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/server/ratelimit_test.go` (new Describe block)

**Test Steps**:
1. **Given**: A RateLimiter with auditStore configured, burst=1
2. **When**: 2 sequential requests exceed the rate limit (second triggers 429)
3. **Then**: The audit event stored has a non-empty `CorrelationID`

**Expected Results**:
1. `event.CorrelationID` is not `""`
2. `event.CorrelationID` starts with `"security-"`
3. `event.CorrelationID` has UUID suffix parseable by `uuid.Parse()`

---

### UT-KA-1401-006: buildEventData for rate-limit event

**Priority**: P0
**Type**: Unit
**File**: `internal/kubernautagent/audit/ds_store_test.go` (new Describe block)

**Test Steps**:
1. **Given**: An `AuditEvent` with `EventType = EventTypeRateLimitDenied` and `Data` containing `source_ip`, `path`, `method`
2. **When**: `buildEventData(event)` is called
3. **Then**: Returns `(payload, true)` where payload is an `AIAgentRatelimitDeniedPayload`

**Expected Results**:
1. Second return value is `true` (not the default `false`)
2. Payload's `EventType` field equals `AIAgentRatelimitDeniedPayloadEventTypeAiagentRatelimitDenied`
3. Payload contains `source_ip`, `path`, `method` from `event.Data`
4. Payload contains `event_id` from `event.Data["event_id"]`

---

### IT-KA-1401-001: Security events pass OpenAPI validation

**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/audit/lifecycle_test.go` (extend)

**Test Steps**:
1. **Given**: A properly configured `BufferedDSAuditStore` with OpenAPI validation enabled
2. **When**: A rate-limit event with valid correlation_id and event.Data is stored
3. **Then**: Event passes validation and reaches the DS client (not rejected)

**Expected Results**:
1. No validation error logged
2. DS client receives the `CreateBatchReq` containing the event
3. `correlation_id` field passes `minLength: 1` check
4. `event_data` discriminated union resolves to the correct payload type

---

### E2E-KA-1401-001: HTTP 429 produces persisted audit event

**Priority**: P0
**Type**: E2E
**File**: `test/e2e/kubernautagent/audit_pipeline_test.go` (extend)

**Test Steps**:
1. **Given**: KA running in Kind cluster with DataStorage, rate-limit configured at burst=2
2. **When**: 5 rapid sequential requests are sent to KA's investigate endpoint
3. **Then**: At least one `aiagent.ratelimit.denied` event is queryable in DataStorage

**Expected Results**:
1. HTTP 429 returned for at least one request
2. DS audit events query returns event with `event_type = "aiagent.ratelimit.denied"`
3. Event has non-empty `correlation_id` with `security-` prefix
4. Event data contains `source_ip`, `path`, `method`

---

## 10. Environmental Needs

### 10.1 Unit Tests
- Go 1.22+
- Ginkgo v2 / Gomega
- `net/http/httptest` for rate-limiter middleware testing

### 10.2 Integration Tests
- Go 1.22+
- `pkg/audit/openapi_validator.go` for validation
- Mock DS HTTP server (existing `fakeOgenClient` pattern)

### 10.3 E2E Tests
- Kind cluster (`kubernautagent-e2e`)
- KA binary + DataStorage binary
- Rate-limit configured at low burst for test triggering

### 10.4 Tools & Versions

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.22+ | Compiler |
| Ginkgo | v2.x | Test runner |
| ogen | v1.20.1 | OpenAPI client generation |
| Kind | 0.20+ | Local K8s cluster |
| golangci-lint | 1.55+ | Linting |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Status | Impact if Missing |
|------------|--------|-------------------|
| `api/openapi/data-storage-v1.yaml` | Exists | Must add 3 schemas |
| `ogen` code generator | Available (v1.20.1 in gen.go) | Must regenerate after YAML change |
| `ApifrontendRatelimitDeniedPayload` reference | Exists | Template for new schemas |
| `uuid` package | Already imported in `emitter.go` | Generate correlation IDs |

### 11.2 TDD Execution Order (Phased)

```
Phase A (RED -> GREEN -> REFACTOR): Correlation ID Fix
  ├── UT-KA-1401-001..005
  └── CHECKPOINT A

Phase B (RED -> GREEN -> REFACTOR): OpenAPI Schema + Regen
  ├── Add 3 schemas to data-storage-v1.yaml
  ├── go generate (ogen regen)
  ├── Update embedded audit spec copy
  └── CHECKPOINT B (build validation)

Phase C (RED -> GREEN -> REFACTOR): buildEventData Cases
  ├── UT-KA-1401-006..010
  └── CHECKPOINT C

Phase D (RED -> GREEN -> REFACTOR): Integration Wiring
  ├── IT-KA-1401-001..003
  └── CHECKPOINT W (wiring verification)

Phase E (RED -> GREEN -> REFACTOR): E2E Journey
  ├── E2E-KA-1401-001..002
  └── CHECKPOINT FINAL (Pyramid Invariant)
```

---

## 12. Test Deliverables

| Deliverable | Location | Format |
|-------------|----------|--------|
| Test plan | `docs/tests/1401/TEST_PLAN.md` | IEEE 829 hybrid |
| Unit tests (correlation) | `internal/kubernautagent/server/ratelimit_1401_test.go` | Ginkgo/Gomega |
| Unit tests (buildEventData) | `internal/kubernautagent/audit/ds_store_test.go` | Ginkgo/Gomega |
| Integration tests | `test/integration/kubernautagent/audit/lifecycle_test.go` | Ginkgo/Gomega |
| E2E tests | `test/e2e/kubernautagent/audit_pipeline_test.go` | Ginkgo/Gomega |
| OpenAPI schemas | `api/openapi/data-storage-v1.yaml` | OpenAPI 3.0 |
| Generated types | `pkg/datastorage/ogen-client/oas_schemas_gen.go` | Auto-generated |

---

## 13. Execution

```bash
# Unit tests (Phase A + C)
go test ./internal/kubernautagent/server/... -run "UT-KA-1401" -v
go test ./internal/kubernautagent/audit/... -run "UT-KA-1401" -v

# Existing buildEventData coverage (must pass after Phase C)
go test ./internal/kubernautagent/audit/... -run "UT-KA-PR9-001" -v

# Integration tests (Phase D)
go test ./test/integration/kubernautagent/audit/... -run "IT-KA-1401" -v

# E2E tests (Phase E)
make test-e2e-kubernautagent FOCUS="E2E-KA-1401"

# Coverage
go test ./internal/kubernautagent/audit/... -coverprofile=coverage-audit.out
go test ./internal/kubernautagent/server/... -coverprofile=coverage-server.out

# Regression check
go test ./internal/kubernautagent/audit/... -run "UT-KA-PR9-001" -v
```

---

## 14. Go Anti-Pattern Validation

| # | Mistake | Applicable? | Validation |
|---|---------|-------------|------------|
| 4 | Overusing getters | No | Direct field access on ogen payload structs |
| 28 | Maps and memory leaks | Yes | `event.Data` is per-request; no accumulation |
| 36 | Unnecessary type conversions | Yes | `dataString()` helper handles type assertion safely |
| 54 | Not using testing utility packages | Yes | Reuse existing `fakeOgenClient`, `spyAuditStore` |
| 60 | Not using table-driven tests | Yes | 3 event types tested via table-driven Entry pattern |
| 73 | Not using errgroup | No | No parallel goroutine orchestration |
| 78 | JSON marshaling | Yes | ogen handles marshaling; verify struct tags on payload |
| 83 | Not using io.Reader/Writer properly | No | No streaming I/O |
| 89 | Not closing resources | No | UUID generation has no resources to close |
| 97 | Not using context correctly | Yes | `r.Context()` passed to `StoreBestEffort` (existing pattern) |

---

## 15. Checkpoint Protocol

At each checkpoint (A, B, C, W, FINAL), perform the following GA readiness audit:

1. **Build validation**: `go build ./...` — zero errors
2. **Test pass rate**: All tests in affected packages pass (100%)
3. **Lint compliance**: `golangci-lint run --timeout=5m` — zero new warnings on changed files
4. **Per-tier coverage**: >=80% on tier-specific code subset
5. **Regression guard**: `UT-KA-PR9-001` buildEventData coverage test passes
6. **OpenAPI validation**: `oapi-codegen` or manual `ogen` validates spec (Phase B+)
7. **100-go-mistakes**: Validate against applicable patterns listed in section 14
8. **Escalation gate**: Confidence >=95% to proceed; <95% escalate with actionable findings

---

## 16. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| correlation_id (ratelimit) | `RateLimiter.Middleware()` | `internal/kubernautagent/server/ratelimit.go:155` | IT-KA-1401-002 |
| correlation_id (auth) | `AuditAuthMiddleware()` | `internal/kubernautagent/server/audit_middleware.go:54,62` | IT-KA-1401-003 |
| buildEventData (ratelimit) | `DSAuditStore.StoreAudit()` | `internal/kubernautagent/audit/ds_store.go:85` | IT-KA-1401-002 |
| buildEventData (auth failure) | `DSAuditStore.StoreAudit()` | `internal/kubernautagent/audit/ds_store.go:85` | IT-KA-1401-003 |
| buildEventData (auth denied) | `DSAuditStore.StoreAudit()` | `internal/kubernautagent/audit/ds_store.go:85` | IT-KA-1401-003 |
| OpenAPI schema (3 payloads) | ogen discriminator dispatch | `api/openapi/data-storage-v1.yaml` | IT-KA-1401-001 |
