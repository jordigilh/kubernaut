# Test Plan: AF Integration Test Coverage Gap (#1195)

**IEEE 829 Test Plan Identifier**: TP-AF-1195
**Version**: 1.0
**Date**: 2026-05-19
**Author**: AI Assistant (Kubernaut Development)
**Status**: Draft

---

## 1. Introduction

### 1.1 Purpose

Validate behavioral acceptance criteria for all AF integration-testable (I/O-dependent) packages, closing the coverage gap from 6.8% to >=80% per-tier testable code. Tests are behavior-first: each validates a specific business acceptance criterion, and coverage follows naturally.

### 1.2 Scope

10 IT-classified packages in `pkg/apifrontend/` totaling ~2,614 coverable statements:

- `handler/` (1,109 lines, 9 files) -- HTTP request routing, middleware, MCP bridge
- `auth/` (756 lines, 6 I/O files) -- JWT/JWKS validation, TokenReview, middleware chain
- `tools/` (2,329 lines, 12 files) -- MCP tool dispatch to K8s, KA, DS
- `ka/` (740 lines, 5 files) -- Kubernaut Agent REST + MCP SDK client
- `resilience/` (677 lines, 4 files) -- Circuit breaker, retry, K8s dynamic client
- `launcher/` (444 lines, 3 files) -- A2A JSON-RPC handler
- `prometheus/` (355 lines, 3 files) -- Prometheus HTTP client
- `ds/` (282 lines, 2 files) -- DataStorage client
- `session/service.go` (391 lines) -- CRD-backed session lifecycle
- `tlswiring/` (196 lines, 1 file) -- TLS configuration (already partially covered)

### 1.3 References

- Issue: [#1195](https://github.com/jordigilh/kubernaut/issues/1195)
- Related: [#1156](https://github.com/jordigilh/kubernaut/issues/1156) (AF SOC2 Audit Normalization)
- Related: [#1196](https://github.com/jordigilh/kubernaut/issues/1196) (AF E2E coverage instrumentation)
- Testing Strategy: `.cursor/rules/03-testing-strategy.mdc`
- No-Mocks Policy: `docs/testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md`
- Testing Guidelines: `docs/development/business-requirements/TESTING_GUIDELINES.md` v2.8.0
- Coverage Tool: `scripts/coverage/coverage_report.py`
- AA IT Gold Standard: `test/integration/aianalysis/suite_test.go` (DD-TEST-010 pattern)

### 1.4 Definitions

- **IT**: Integration Test (Tier 2, `test/integration/`)
- **AC**: Acceptance Criterion (behavioral contract)
- **envtest**: In-memory Kubernetes API server from controller-runtime
- **DS**: DataStorage service (PostgreSQL + Redis + HTTP API)
- **KA**: Kubernaut Agent service (HTTP + MCP)
- **JWKS**: JSON Web Key Set (OIDC key discovery endpoint)

---

## 2. Test Environment

### 2.1 Framework

- Ginkgo/Gomega BDD (mandatory per project rules)
- envtest (real K8s API for CRD operations and TokenReview)
- Real containerized services: DS (PostgreSQL + Redis + DS API), MockLLM, KA
- httptest (OIDC JWKS endpoints only -- truly external; Prometheus client -- truly external)

### 2.2 Infrastructure (DD-TEST-010 Pattern)

Following AA IT suite architecture:

**Phase 1 (Process 1 only -- shared infrastructure):**
- Shared envtest for ServiceAccount/TokenReview/RBAC
- PostgreSQL container (persistence for DS)
- Redis container (caching for DS)
- DataStorage API container (real HTTP service)
- Mock LLM container (OpenAI-compatible, for KA)
- KA HTTP container (real service, uses Mock LLM + DS)

**Phase 2 (All processes -- per-process setup):**
- Per-process envtest (AF CRDs + InvestigationSession)
- Per-process K8s client (controller-runtime)
- Per-process authenticated DS clients
- Per-process SA tokens via TokenRequest API

### 2.3 Test Location

- `test/integration/apifrontend/`

### 2.4 Shared Helpers

- `test/infrastructure/serviceaccount.go` -- SA creation, TokenRequest, kubeconfig generation
- `test/infrastructure/datastorage_bootstrap.go` -- DS container lifecycle
- `test/infrastructure/container_management.go` -- Generic container management
- `test/infrastructure/mock_llm.go` -- Mock LLM container
- `test/shared/auth/` -- MockJWKSServer, ServiceAccountTransport
- `test/shared/integration/` -- AuthenticatedDataStorageClients

---

## 3. Risks and Mitigations

| ID | Risk | Impact | Probability | Mitigation |
|----|------|--------|-------------|------------|
| R1 | Container startup >3min in CI | Test timeout | Medium | Reuse AA-proven infra helpers; set NodeTimeout(10*time.Minute); parallel image build |
| R2 | envtest TokenReview fails for SA tokens | Auth ITs blocked | Low | Pattern proven in GW + AA suites; pre-validated via spike |
| R3 | launcher/ ADK agent requires LLM model | Tests hang on LLM call | Low | Agent created without model (SkipTools or nil model); tools register but no LLM dispatch |
| R4 | KA container unhealthy (port conflict) | KA-dependent tests fail | Low | Unique port allocation per service (DD-TEST-001); health check with 120s timeout |
| R5 | Coverage delta from estimated stmt counts | 80% target missed | Low | Conservative estimates validated with 3 sensitivity scenarios (all hit 81.7-81.8%) |
| R6 | Existing IT compliance violations mask regressions | False confidence | Medium | Fix violations in PR1 (time.Sleep, dynamicfake, fake K8s client) |

---

## 4. Test Scenarios

### 4.1 handler/ -- HTTP Request Path

| ID | Scenario | Acceptance Criterion | Input | Expected | Dependencies |
|----|----------|---------------------|-------|----------|-------------|
| IT-AF-1195-001 | Router dispatches to A2A handler | AC-1 | POST /a2a/invoke with valid Bearer JWT | 200 OK, response from A2A handler | httptest JWKS, real router |
| IT-AF-1195-002 | Router dispatches to MCP handler | AC-1 | POST /mcp with valid Bearer JWT | 200 OK, JSON-RPC response | httptest JWKS, real router |
| IT-AF-1195-003 | /healthz serves without auth | AC-2 | GET /healthz, no Authorization header | 200 OK, body "ok" | real router |
| IT-AF-1195-004 | /readyz serves without auth | AC-2 | GET /readyz, no Authorization header | 200 OK when ready | real router |
| IT-AF-1195-005 | /metrics serves without auth | AC-2 | GET /metrics, no Authorization header | 200 OK, Prometheus text | real router |
| IT-AF-1195-006 | /.well-known/agent-card.json without auth | AC-2 | GET /.well-known/agent-card.json | 200 OK, valid agent card JSON | real router, real AgentCardHandler |
| IT-AF-1195-007 | Authenticated route rejects missing token | AC-1 | POST /a2a/invoke, no Authorization | 401 Unauthorized | real router + real auth middleware |
| IT-AF-1195-008 | Panic recovery returns 500 problem+json | AC-4 | POST to panicking handler | 500, application/problem+json body, panic counter incremented | real RecoverMiddleware |
| IT-AF-1195-009 | Security headers present on all responses | AC-5 | GET /healthz | X-Content-Type-Options: nosniff, X-Frame-Options: DENY, HSTS, Cache-Control: no-store | real router |
| IT-AF-1195-010 | Metrics middleware records request | AC-6 | GET /healthz | af_http_requests_total{method="GET", path="/healthz", status="200"} incremented | real metricsMiddleware |
| IT-AF-1195-011 | Readyz returns 503 when not ready | AC-7 | GET /readyz with checker returning false | 503, problem+json | real ReadyzHandlerFunc |
| IT-AF-1195-012 | Readyz returns 503 when draining | AC-7 | GET /readyz with draining=true | 503, "shutting down" | real ReadyzHandlerFunc |
| IT-AF-1195-013 | Max body size rejects oversized payload | AC-8 | POST /a2a/invoke with >1MB body | 413 or connection reset | real maxBodyMiddleware |

### 4.2 auth/ -- Authentication Chain

| ID | Scenario | Acceptance Criterion | Input | Expected | Dependencies |
|----|----------|---------------------|-------|----------|-------------|
| IT-AF-1195-014 | Valid JWT accepted, identity propagated | AC-9, AC-12 | Bearer token signed by httptest JWKS | 200 OK, UserIdentity in context | httptest JWKS, real JWTValidator |
| IT-AF-1195-015 | Expired JWT rejected | AC-10 | Bearer token with exp in past | 401, problem+json | httptest JWKS, real JWTValidator |
| IT-AF-1195-016 | Wrong audience JWT rejected | AC-10 | Bearer token with wrong aud | 401, problem+json | httptest JWKS, real JWTValidator |
| IT-AF-1195-017 | Missing Authorization header rejected | AC-10 | No Authorization header | 401, problem+json | real middleware |
| IT-AF-1195-018 | Non-bearer scheme rejected | AC-10 | Authorization: Basic xxx | 401, problem+json | real middleware |
| IT-AF-1195-019 | K8s SA token validated via TokenReview | AC-11 | Bearer SA token from envtest TokenRequest | 200 OK, SA identity in context | envtest, real TokenReviewer |
| IT-AF-1195-020 | Auth success emits audit event | AC-15 | Valid JWT | audit.EventAuthSuccess emitted | real middleware, recording emitter |
| IT-AF-1195-021 | Auth failure emits audit event | AC-15 | Expired JWT | audit.EventAuthFailure emitted | real middleware, recording emitter |
| IT-AF-1195-022 | JWKS circuit breaker fail-open with cached keys | AC-13 | JWKS server goes down after initial fetch | Valid token still accepted (cached keys) | httptest JWKS (stop/start), real JWKSCache |
| IT-AF-1195-023 | JWT delegation transport forwards token to KA | AC-14 | Request with Bearer token through delegation transport | KA receives same Bearer token | real KA container, real ContextJWTDelegationTransport |

### 4.3 ka/ -- Kubernaut Agent Client

| ID | Scenario | Acceptance Criterion | Input | Expected | Dependencies |
|----|----------|---------------------|-------|----------|-------------|
| IT-AF-1195-024 | REST client round-trip to KA | AC-16 | ka.Client.HealthCheck() or similar | Successful response from real KA | real KA container |
| IT-AF-1195-025 | Circuit breaker protects KA calls | AC-16 | Multiple requests, KA becomes unhealthy | Circuit opens, subsequent calls fail-fast with ErrOpenState | real KA container (stop/start) |
| IT-AF-1195-026 | MCP SDK client dispatches tool call | AC-17 | SDKMCPClient.SelectWorkflow() | Successful tool call response from real KA | real KA container |
| IT-AF-1195-027 | JWT delegation passes token to KA | AC-18 | Request with SA Bearer token | KA receives and validates same token | real KA container, envtest SA token |

### 4.4 ds/ -- DataStorage Client

| ID | Scenario | Acceptance Criterion | Input | Expected | Dependencies |
|----|----------|---------------------|-------|----------|-------------|
| IT-AF-1195-028 | ListWorkflows queries real DS | AC-19 | ds.Client.ListWorkflows() | Workflow list from real DS (seeded data) | real DS container |
| IT-AF-1195-029 | GetRemediationHistory queries real DS | AC-19 | ds.Client.GetRemediationHistory() | History from real DS | real DS container |
| IT-AF-1195-030 | Error response handled gracefully | AC-20 | Query with invalid parameters | Wrapped error, no panic | real DS container |

### 4.5 resilience/ -- Circuit Breaker and Retry

| ID | Scenario | Acceptance Criterion | Input | Expected | Dependencies |
|----|----------|---------------------|-------|----------|-------------|
| IT-AF-1195-031 | CB trips after consecutive failures | AC-21 | 5 consecutive 500 responses from real KA (or httptest) | gobreaker.ErrOpenState on 6th call | real or httptest backend |
| IT-AF-1195-032 | CB reopens after timeout (half-open) | AC-21 | Wait for CB timeout, send probe request | Request reaches backend, CB transitions to half-open/closed | real or httptest backend |
| IT-AF-1195-033 | Retry transport retries on 503 | AC-22 | Backend returns 503 once then 200 | Client sees 200 after retry | httptest backend |
| IT-AF-1195-034 | K8s dynamic client factory creates impersonated client | AC-23 | UserIdentity with username + groups | Dynamic client with impersonation headers set | envtest |

### 4.6 tools/ -- MCP Tool Dispatch

| ID | Scenario | Acceptance Criterion | Input | Expected | Dependencies |
|----|----------|---------------------|-------|----------|-------------|
| IT-AF-1195-035 | list_remediations against real K8s | AC-24 | HandleListRemediations with envtest dynamic client | List of RR CRDs (or empty) | envtest |
| IT-AF-1195-036 | get_pods against real K8s | AC-24 | HandleGetPods with envtest dynamic client | Pod list from namespace | envtest |
| IT-AF-1195-037 | get_workloads against real K8s | AC-24 | HandleGetWorkloads with envtest dynamic client | Deployment/StatefulSet list | envtest |
| IT-AF-1195-038 | investigate dispatches to real KA | AC-25 | HandleStartInvestigation with real KA client | Investigation started (or error from KA) | real KA container |
| IT-AF-1195-039 | discover_workflows dispatches to real KA | AC-25 | HandleDiscoverWorkflows with real MCP client | Workflow list from KA | real KA container |
| IT-AF-1195-040 | list_workflows queries real DS | AC-26 | HandleListWorkflows with real DS client | Workflow list from DS | real DS container |
| IT-AF-1195-041 | RBAC enforcement blocks unauthorized tool | AC-27 | Tool call with user lacking role | Error: "forbidden: role does not grant access" | real RBAC config |

### 4.7 session/ -- Session Lifecycle

| ID | Scenario | Acceptance Criterion | Input | Expected | Dependencies |
|----|----------|---------------------|-------|----------|-------------|
| IT-AF-1195-042 | CRD session create round-trip | AC-28 | session.Create() with envtest | InvestigationSession CRD created in K8s | envtest |
| IT-AF-1195-043 | Phase transition emits audit event | AC-29 | UpdatePhase(Completed) | EventSessionCompleted with duration_ms | envtest, recording emitter |

### 4.8 launcher/ -- A2A Handler

| ID | Scenario | Acceptance Criterion | Input | Expected | Dependencies |
|----|----------|---------------------|-------|----------|-------------|
| IT-AF-1195-044 | A2A message/send round-trip | AC-30 | POST JSON-RPC {"method":"message/send"} | 200 OK, JSON-RPC result with task ID | real agent, httptest server |
| IT-AF-1195-045 | A2A task status contains state | AC-30 | message/send response | result.status.state is "completed" or "working" | real agent, httptest server |
| IT-AF-1195-046 | BeforeExecute emits audit event | AC-30 | message/send with UserIdentity | EventA2ATaskStarted emitted | recording emitter |
| IT-AF-1195-047 | AfterExecute emits completion audit | AC-30 | message/send completes | EventA2ATaskCompleted emitted | recording emitter |

### 4.9 prometheus/ -- Prometheus Client

| ID | Scenario | Acceptance Criterion | Input | Expected | Dependencies |
|----|----------|---------------------|-------|----------|-------------|
| IT-AF-1195-048 | GetAlerts parses response | AC-32 | httptest returns /api/v1/alerts JSON | []Alert parsed correctly | httptest server |
| IT-AF-1195-049 | GetRules parses response | AC-33 | httptest returns /api/v1/rules JSON | []RuleGroup parsed correctly | httptest server |
| IT-AF-1195-050 | InstantQuery parses response | AC-34 | httptest returns /api/v1/query JSON | *QueryResult parsed correctly | httptest server |
| IT-AF-1195-051 | GetAlerts handles server error | AC-35 | httptest returns 500 | Wrapped error returned | httptest server |
| IT-AF-1195-052 | GetAlerts handles timeout | AC-35 | httptest with delayed response | Context deadline error | httptest server |

---

## 5. TDD Execution Plan

### Implementation Phase 1 (PR1): Infrastructure + Router + Auth

#### Phase 1.1: TDD RED -- Write Failing Tests

**Scope**: IT-AF-1195-001 through IT-AF-1195-023

**Actions:**
1. Rewrite `suite_test.go` with SynchronizedBeforeSuite (DS + KA + envtest infra)
2. Create `router_http_test.go` with tests IT-AF-1195-001 through IT-AF-1195-013
   - Build real `handler.NewRouter` with real `auth.MiddlewareWithConfig`
   - httptest JWKS server for JWT signing/validation
   - All tests should fail: infrastructure not yet wired, handler stubs missing
3. Create `auth_middleware_test.go` with tests IT-AF-1195-014 through IT-AF-1195-023
   - Real `auth.NewJWTValidator` with httptest JWKS
   - Real `auth.NewTokenReviewer` with envtest `kubernetes.NewForConfig(cfg)`
   - SA tokens via `ServiceAccountHelper.GetServiceAccountToken`
   - All tests should fail: validator/middleware not yet composed in test harness

**Verification**: `go test ./test/integration/apifrontend/... -run='^$' -count=1` compiles; running tests produces RED failures.

#### Phase 1.2: TDD GREEN -- Minimal Implementation to Pass

**Actions:**
1. Wire suite infrastructure: DS bootstrap, KA container, shared envtest, SA creation
2. Complete `router_http_test.go` harness: construct `RouterConfig` with real deps
3. Complete `auth_middleware_test.go` harness: compose JWTValidator + TokenReviewer + Middleware
4. Fix `audit_normalization_test.go`: replace `time.Sleep(100ms)` with `Eventually()`
5. Fix `tool_wiring_smoke_test.go`: replace `dynamicfake` with envtest dynamic client

**Verification**: `make test-integration-apifrontend` passes all tests.

#### Phase 1.3: TDD REFACTOR -- Code Quality

**Actions:**
1. Extract shared test helpers (JWT signing, JWKS server setup, recording emitter) into `test/integration/apifrontend/helpers_test.go`
2. Validate against 100 Go Mistakes checklist (see Section 8)
3. Run `golangci-lint run --timeout=5m` on new test files
4. Verify no `time.Sleep`, no `interface{}`, no ignored errors

**Verification**: Lint clean, no anti-patterns.

#### CHECKPOINT 1: GA Readiness Audit (Post-PR1)

**Audit dimensions:**
- [ ] Build: `go build ./...` passes
- [ ] Lint: `golangci-lint run --timeout=5m` clean
- [ ] Tests: `make test-integration-apifrontend` all green
- [ ] Coverage: Run `make coverage-report`, verify handler/ + auth/ coverage delta
- [ ] No-Mocks Policy: No forbidden mocks in new IT files
- [ ] BDD Framework: All tests use Ginkgo/Gomega (no `func TestX(t *testing.T)`)
- [ ] Test IDs: All tests have IT-AF-1195-NNN identifiers
- [ ] time.Sleep: Zero occurrences in `test/integration/apifrontend/`
- [ ] K8s Client Mandate: No `fake.NewClientBuilder()` or `dynamicfake` in ITs
- [ ] Existing test fixes: `audit_normalization_test.go` and `tool_wiring_smoke_test.go` corrected
- [ ] 100 Go Mistakes: Refactor phase checklist completed (Section 8)
- [ ] Confidence: >=95% to proceed

**Escalation**: If any audit dimension fails or confidence drops below 95%, halt and report findings before advancing to Phase 2.

---

### Implementation Phase 2 (PR2): KA + DS + Tools + Resilience

#### Phase 2.1: TDD RED -- Write Failing Tests

**Scope**: IT-AF-1195-024 through IT-AF-1195-041

**Actions:**
1. Create `ka_client_test.go` with tests IT-AF-1195-024 through IT-AF-1195-027
2. Create `ds_client_test.go` with tests IT-AF-1195-028 through IT-AF-1195-030
3. Create `resilience_test.go` with tests IT-AF-1195-031 through IT-AF-1195-034
4. Create `tools_crd_test.go` with tests IT-AF-1195-035 through IT-AF-1195-037, IT-AF-1195-041
5. Create `tools_ka_ds_test.go` with tests IT-AF-1195-038 through IT-AF-1195-040

**Verification**: Tests compile but fail (RED).

#### Phase 2.2: TDD GREEN -- Minimal Implementation to Pass

**Actions:**
1. Wire `ka.NewClient` with real KA container endpoint + SA token transport
2. Wire `ds.NewOgenClient` with real DS container endpoint
3. Wire resilience transports with real/httptest backends
4. Wire CRD tools with envtest dynamic client
5. Wire KA/DS tools with real container clients
6. Seed test data in DS (workflows) and envtest (CRDs, Pods, Deployments)

**Verification**: `make test-integration-apifrontend` passes all tests.

#### Phase 2.3: TDD REFACTOR -- Code Quality

**Actions:**
1. Extract common container client construction into helpers
2. Validate against 100 Go Mistakes checklist (Section 8)
3. Lint validation
4. Ensure circuit breaker tests use `Eventually()` for async state transitions

**Verification**: Lint clean, no anti-patterns.

#### CHECKPOINT 2: GA Readiness Audit (Post-PR2)

**Audit dimensions:**
- [ ] Build: `go build ./...` passes
- [ ] Lint: clean
- [ ] Tests: all green (including PR1 tests)
- [ ] Coverage: cumulative handler/ + auth/ + ka/ + ds/ + tools/ + resilience/ delta
- [ ] No-Mocks Policy: No forbidden mocks; KA and DS are real containers
- [ ] BDD Framework: compliant
- [ ] Test IDs: all present
- [ ] time.Sleep: zero
- [ ] K8s Client Mandate: envtest or real API everywhere
- [ ] 100 Go Mistakes: checklist completed
- [ ] Confidence: >=95% to proceed

**Escalation**: Halt if confidence <95%.

---

### Implementation Phase 3 (PR3): Session + Launcher + Prometheus + Fixes

#### Phase 3.1: TDD RED -- Write Failing Tests

**Scope**: IT-AF-1195-042 through IT-AF-1195-052

**Actions:**
1. Create `session_service_test.go` with tests IT-AF-1195-042, IT-AF-1195-043
2. Create `launcher_a2a_test.go` with tests IT-AF-1195-044 through IT-AF-1195-047
3. Create `prometheus_client_test.go` with tests IT-AF-1195-048 through IT-AF-1195-052
4. Refactor `ttl_controller_test.go` IT-AF-220-001 through 007: replace fake with envtest

**Verification**: Tests compile but fail (RED).

#### Phase 3.2: TDD GREEN -- Minimal Implementation to Pass

**Actions:**
1. Wire session service with envtest K8s client + recording emitter
2. Wire launcher with `agentpkg.NewRootAgent` (real agent, envtest K8s, real DS/KA clients)
3. Wire Prometheus client with httptest server returning realistic payloads
4. Migrate ttl_controller tests from `fake.NewClientBuilder()` to envtest

**Verification**: `make test-integration-apifrontend` passes all tests.

#### Phase 3.3: TDD REFACTOR -- Code Quality

**Actions:**
1. Consolidate test helpers across all PR files
2. Validate against 100 Go Mistakes checklist (Section 8)
3. Final lint pass
4. Remove any dead code or unused imports

**Verification**: Lint clean, no anti-patterns.

#### CHECKPOINT 3: Final GA Readiness Audit (Post-PR3)

**Audit dimensions:**
- [ ] Build: `go build ./...` passes
- [ ] Lint: clean across all test files
- [ ] Tests: all 52 IT scenarios green
- [ ] Coverage: `make coverage-report` shows AF IT >=80%
- [ ] No-Mocks Policy: full compliance across all IT files
- [ ] BDD Framework: 100% Ginkgo/Gomega
- [ ] Test IDs: all 52 scenarios have IT-AF-1195-NNN identifiers
- [ ] time.Sleep: zero occurrences
- [ ] K8s Client Mandate: zero fake/dynamicfake usage in ITs
- [ ] Existing violations: all 3 original violations fixed (time.Sleep, dynamicfake, fake K8s)
- [ ] 100 Go Mistakes: all applicable items checked
- [ ] BR Traceability: all tests map to ACs in traceability matrix
- [ ] Confidence: >=95% for GA

**Escalation**: If final coverage is below 80% or any dimension fails, document gap analysis and propose remediation before declaring GA.

---

## 6. Traceability Matrix

| Test ID | Acceptance Criterion | Package | Issue |
|---------|---------------------|---------|-------|
| IT-AF-1195-001 | AC-1 | handler | #1195 |
| IT-AF-1195-002 | AC-1 | handler | #1195 |
| IT-AF-1195-003 | AC-2 | handler | #1195 |
| IT-AF-1195-004 | AC-2 | handler | #1195 |
| IT-AF-1195-005 | AC-2 | handler | #1195 |
| IT-AF-1195-006 | AC-2 | handler | #1195 |
| IT-AF-1195-007 | AC-1 | handler | #1195 |
| IT-AF-1195-008 | AC-4 | handler | #1195 |
| IT-AF-1195-009 | AC-5 | handler | #1195 |
| IT-AF-1195-010 | AC-6 | handler | #1195 |
| IT-AF-1195-011 | AC-7 | handler | #1195 |
| IT-AF-1195-012 | AC-7 | handler | #1195 |
| IT-AF-1195-013 | AC-8 | handler | #1195 |
| IT-AF-1195-014 | AC-9, AC-12 | auth | #1195 |
| IT-AF-1195-015 | AC-10 | auth | #1195 |
| IT-AF-1195-016 | AC-10 | auth | #1195 |
| IT-AF-1195-017 | AC-10 | auth | #1195 |
| IT-AF-1195-018 | AC-10 | auth | #1195 |
| IT-AF-1195-019 | AC-11 | auth | #1195 |
| IT-AF-1195-020 | AC-15 | auth | #1195 |
| IT-AF-1195-021 | AC-15 | auth | #1195 |
| IT-AF-1195-022 | AC-13 | auth | #1195 |
| IT-AF-1195-023 | AC-14 | auth | #1195 |
| IT-AF-1195-024 | AC-16 | ka | #1195 |
| IT-AF-1195-025 | AC-16 | ka | #1195 |
| IT-AF-1195-026 | AC-17 | ka | #1195 |
| IT-AF-1195-027 | AC-18 | ka | #1195 |
| IT-AF-1195-028 | AC-19 | ds | #1195 |
| IT-AF-1195-029 | AC-19 | ds | #1195 |
| IT-AF-1195-030 | AC-20 | ds | #1195 |
| IT-AF-1195-031 | AC-21 | resilience | #1195 |
| IT-AF-1195-032 | AC-21 | resilience | #1195 |
| IT-AF-1195-033 | AC-22 | resilience | #1195 |
| IT-AF-1195-034 | AC-23 | resilience | #1195 |
| IT-AF-1195-035 | AC-24 | tools | #1195 |
| IT-AF-1195-036 | AC-24 | tools | #1195 |
| IT-AF-1195-037 | AC-24 | tools | #1195 |
| IT-AF-1195-038 | AC-25 | tools | #1195 |
| IT-AF-1195-039 | AC-25 | tools | #1195 |
| IT-AF-1195-040 | AC-26 | tools | #1195 |
| IT-AF-1195-041 | AC-27 | tools | #1195 |
| IT-AF-1195-042 | AC-28 | session | #1195 |
| IT-AF-1195-043 | AC-29 | session | #1195 |
| IT-AF-1195-044 | AC-30 | launcher | #1195 |
| IT-AF-1195-045 | AC-30 | launcher | #1195 |
| IT-AF-1195-046 | AC-30 | launcher | #1195 |
| IT-AF-1195-047 | AC-30 | launcher | #1195 |
| IT-AF-1195-048 | AC-32 | prometheus | #1195 |
| IT-AF-1195-049 | AC-33 | prometheus | #1195 |
| IT-AF-1195-050 | AC-34 | prometheus | #1195 |
| IT-AF-1195-051 | AC-35 | prometheus | #1195 |
| IT-AF-1195-052 | AC-35 | prometheus | #1195 |

---

## 7. Projected Coverage

| Milestone | Covered Stmts | Total IT Stmts | Coverage |
|-----------|--------------|----------------|----------|
| Baseline (current) | 119 | ~2,614 | 6.8% |
| After PR1 (handler + auth) | ~849 | ~2,614 | 32.5% |
| After PR2 (+ ka + ds + tools + resilience) | ~1,709 | ~2,614 | 65.4% |
| After PR3 (+ session + launcher + prometheus) | ~2,124 | ~2,614 | **81.3%** |
| Including existing TLS coverage | ~2,157 | ~2,614 | **82.5%** |

---

## 8. 100 Go Mistakes Validation Checklist

Applied during each TDD REFACTOR phase to both test code and any business code touched.

### Code and Project Organization
- [ ] #1: Unintended variable shadowing -- verify no `:=` inside `if/for` that shadows outer variable
- [ ] #2: Unnecessary nested code -- flatten guard clauses in test helpers
- [ ] #6: Interface on producer side -- verify test interfaces are consumer-defined

### Data Types
- [ ] #17: Creating confusion with octal literals -- no raw octal in test constants
- [ ] #20: Not understanding slice length and capacity -- pre-allocate slices with known size in test helpers
- [ ] #24: Not making slice copies correctly -- verify captured slices in recording emitters use `copy()`

### Control Structures
- [ ] #30: Ignoring elements in range loops -- no `for _, _ = range` that discards useful values
- [ ] #33: Not making accurate comparisons (floats) -- use `BeNumerically("~", val, epsilon)` not `Equal()`

### Strings
- [ ] #36: Substring and memory leaks -- no long-lived substring references from large strings

### Functions and Methods
- [ ] #42: Not knowing which type of receiver to use -- verify test helper methods use consistent receiver types
- [ ] #45: Returning a nil receiver -- verify factory functions return explicit nil on error

### Error Management
- [ ] #48: Panicking -- no `panic()` in test helpers (use `Expect().To(Succeed())` or `GinkgoT().Fatal()`)
- [ ] #49: Ignoring when to wrap an error -- use `fmt.Errorf("context: %w", err)` in test utilities
- [ ] #50: Comparing error types incorrectly -- use `errors.Is()` and `errors.As()`, not `==`
- [ ] #53: Not handling defer errors -- verify `defer resp.Body.Close()` errors are captured in tests

### Concurrency
- [ ] #56: Thinking concurrency is always faster -- no gratuitous goroutines in test setup
- [ ] #61: Misusing sync.WaitGroup -- verify `wg.Add()` before goroutine launch in infra setup
- [ ] #66: Not using sync.Once correctly -- verify shared state initialization uses Once pattern
- [ ] #71: Misusing sync.Cond -- not applicable (no Cond usage expected)

### Standard Library
- [ ] #77: JSON handling errors -- verify `json.Unmarshal` errors checked in test assertions
- [ ] #78: Common SQL mistakes -- not applicable (no direct SQL in AF)
- [ ] #80: Not closing transient resources -- verify all `resp.Body`, `httptest.Server`, containers cleaned up
- [ ] #83: Not using testing utility packages effectively -- use `GinkgoT().TempDir()` for temp files
- [ ] #84: Not dealing with time API correctly -- use `time.Since()` not manual subtraction; no `time.Sleep()`

### Testing
- [ ] #86: Not categorizing tests -- all tests in `test/integration/` with `Label("integration")`
- [ ] #87: Not enabling the race flag -- Makefile `$(RACE_FLAG)` enabled for integration target
- [ ] #88: Not using test execution modes correctly -- use `BeforeEach` for per-test setup, `BeforeSuite` for infra
- [ ] #89: Not using table-driven tests -- use Ginkgo `DescribeTable`/`Entry` for parameterized scenarios where applicable
- [ ] #90: Sleeping in tests -- FORBIDDEN; use `Eventually()` with polling
- [ ] #91: Not dealing with the testing cleanup correctly -- use `DeferCleanup()` / `AfterEach` / `SynchronizedAfterSuite`

---

## 9. Status

| Test ID | Status |
|---------|--------|
| IT-AF-1195-001 | Pending |
| IT-AF-1195-002 | Pending |
| IT-AF-1195-003 | Pending |
| IT-AF-1195-004 | Pending |
| IT-AF-1195-005 | Pending |
| IT-AF-1195-006 | Pending |
| IT-AF-1195-007 | Pending |
| IT-AF-1195-008 | Pending |
| IT-AF-1195-009 | Pending |
| IT-AF-1195-010 | Pending |
| IT-AF-1195-011 | Pending |
| IT-AF-1195-012 | Pending |
| IT-AF-1195-013 | Pending |
| IT-AF-1195-014 | Pending |
| IT-AF-1195-015 | Pending |
| IT-AF-1195-016 | Pending |
| IT-AF-1195-017 | Pending |
| IT-AF-1195-018 | Pending |
| IT-AF-1195-019 | Pending |
| IT-AF-1195-020 | Pending |
| IT-AF-1195-021 | Pending |
| IT-AF-1195-022 | Pending |
| IT-AF-1195-023 | Pending |
| IT-AF-1195-024 | Pending |
| IT-AF-1195-025 | Pending |
| IT-AF-1195-026 | Pending |
| IT-AF-1195-027 | Pending |
| IT-AF-1195-028 | Pending |
| IT-AF-1195-029 | Pending |
| IT-AF-1195-030 | Pending |
| IT-AF-1195-031 | Pending |
| IT-AF-1195-032 | Pending |
| IT-AF-1195-033 | Pending |
| IT-AF-1195-034 | Pending |
| IT-AF-1195-035 | Pending |
| IT-AF-1195-036 | Pending |
| IT-AF-1195-037 | Pending |
| IT-AF-1195-038 | Pending |
| IT-AF-1195-039 | Pending |
| IT-AF-1195-040 | Pending |
| IT-AF-1195-041 | Pending |
| IT-AF-1195-042 | Pending |
| IT-AF-1195-043 | Pending |
| IT-AF-1195-044 | Pending |
| IT-AF-1195-045 | Pending |
| IT-AF-1195-046 | Pending |
| IT-AF-1195-047 | Pending |
| IT-AF-1195-048 | Pending |
| IT-AF-1195-049 | Pending |
| IT-AF-1195-050 | Pending |
| IT-AF-1195-051 | Pending |
| IT-AF-1195-052 | Pending |
