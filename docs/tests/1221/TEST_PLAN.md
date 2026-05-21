# Test Plan: SAR-based Tool Authorization for API Frontend

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1221-v1.0
**Feature**: Replace file-based RBAC with Kubernetes SAR-based tool authorization
**Version**: 1.0
**Created**: 2026-05-21
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feat/1220-sar-tool-authorization`
**Parent Issue**: [#1220](https://github.com/jordigilh/kubernaut/issues/1220)
**Tracking Issue**: [#1221](https://github.com/jordigilh/kubernaut/issues/1221)

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the behavioral acceptance criteria for replacing the
file-based RBAC system (`rbac_roles.yaml`) in the API Frontend with Kubernetes-native
SubjectAccessReview (SAR) authorization. Tests provide behavioral assurance that tool
invocation authorization works correctly via SAR, covering at least 80% of testable
code per tier.

### 1.2 Feature Description

Three separate RBAC code paths exist in the API Frontend, all backed by a static
`rbac_roles.yaml` file:

1. **MCP Bridge** (`mcp_bridge.go`): `checkRBAC()` iterates `cfg.RBACRoles[group]` map
2. **A2A Agent** (`agent/root.go`): `newRBACGuard()` BeforeToolCallback uses embedded RBAC
3. **Tool filtering** (`agent/root.go`): `FilterToolsByRole()`/`FilterToolsByRoles()`

This change replaces all three with a single `ToolAuthorizer` interface backed by
`SARChecker`, which performs Kubernetes SubjectAccessReview calls with TTL caching.

### 1.3 Objectives

1. Validate `SARChecker.Check()` correctly calls Kubernetes SAR API with user, groups, and tool name
2. Validate TTL cache avoids redundant SAR calls and expires entries correctly
3. Validate fail-closed behavior: SAR API errors result in tool denial
4. Validate MCP Bridge `checkRBAC()` delegates to `ToolAuthorizer`
5. Validate A2A Agent `newRBACGuard()` delegates to `ToolAuthorizer`
6. Validate `list_tools` is unfiltered (no authorization gate)
7. Validate 6 pre-built ClusterRoles grant correct tool subsets via real RBAC
8. Validate `rbac_roles.yaml` and related infrastructure are removed

### 1.4 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/auth/... ./pkg/apifrontend/handler/... ./pkg/apifrontend/agent/... -ginkgo.v` |
| Unit test code coverage (modified files) | >=80% | `go test -coverprofile` |
| Integration test pass rate | 100% | `go test ./pkg/apifrontend/auth/... -tags=integration -ginkgo.v` |
| Integration test code coverage | >=80% | `go test -coverprofile` |
| Race detector | 0 races | `go test -race` |
| Build success | 0 errors | `go build ./...` |
| Lint compliance | 0 new errors | `golangci-lint run --timeout=5m` |
| BR coverage | All 9 ACs | Coverage matrix in Section 7 |

---

## 2. References

### 2.1 Authoritative Documents

- [Issue #1220](https://github.com/jordigilh/kubernaut/issues/1220) — JWT Persona-based RBAC for API Frontend
- [Issue #1221](https://github.com/jordigilh/kubernaut/issues/1221) — Cross-team tracking issue
- [DD-AUTH-014](docs/architecture/decisions/) — Middleware-Based SAR Authentication
- [TESTING_GUIDELINES.md](docs/development/business-requirements/TESTING_GUIDELINES.md) — Per-tier coverage >=80%
- [ANTI_PATTERN_DETECTION.md](docs/testing/ANTI_PATTERN_DETECTION.md) — Forbidden test patterns
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes) — TDD REFACTOR reference
- [pkg/shared/auth/K8sAuthorizer](pkg/shared/auth/k8s_auth.go) — Existing SAR pattern to follow

### 2.2 Implementation Files

| File | Role |
|------|------|
| `pkg/apifrontend/auth/sar.go` | New: `ToolAuthorizer` interface + `SARChecker` with TTL cache |
| `pkg/apifrontend/handler/mcp_bridge.go` | Modify: replace `checkRBAC()`, swap `RBACRoles` for `ToolAuthorizer` |
| `pkg/apifrontend/agent/root.go` | Modify: rewrite `newRBACGuard()`, remove embedded RBAC and `FilterToolsByRole*` |
| `pkg/apifrontend/config/config.go` | Modify: redefine `RBACConfig` with `SARCacheTTL` |
| `cmd/apifrontend/main.go` | Modify: remove `loadRBACRoles()`, wire `SARChecker` |

### 2.3 Existing Related Tests

| File | Test IDs | Relationship |
|------|----------|-------------|
| `pkg/apifrontend/auth/rbac_tools_test.go` | UT-AF-130-001..007 | FilterToolsByRole tests (to be replaced) |
| `pkg/apifrontend/handler/mcp_bridge_test.go` | Various | MCP bridge RBAC tests (to be updated) |
| `pkg/apifrontend/handler/mcp_bridge_integration_test.go` | Various | MCP bridge integration (to be updated) |
| `pkg/apifrontend/agent/root_test.go` | Various | Agent root tests (to be updated) |
| `cmd/apifrontend/main_wiring_test.go` | Various | Main wiring tests (to be updated) |
| `pkg/shared/auth/k8s_auth_test.go` | Various | SAR test patterns (reference) |

### 2.4 Proven Codebase Patterns

| Pattern | Evidence | Location |
|---------|----------|----------|
| SAR API call with `authorizationv1.SubjectAccessReview` | `K8sAuthorizer.CheckAccessWithGroup()` | `pkg/shared/auth/k8s_auth.go:141-163` |
| `k8sfake.NewSimpleClientset()` + `PrependReactor` for SAR tests | `K8sAuthorizer` unit tests | `pkg/shared/auth/k8s_auth_test.go:156-290` |
| `UserIdentity.Groups` extracted from JWT | `extractGroups()` in JWT validation | `pkg/apifrontend/auth/jwt.go` |
| `auth.UserIdentityFromContext()` for RBAC checks | Both MCP and A2A paths | `mcp_bridge.go:390`, `root.go:164` |

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | envtest does not evaluate RBAC for SAR | IT tests can't verify real ClusterRole grants | Medium | IT-AF-1221-001..005 | Spike: verify envtest SAR evaluation. If unsupported, use `k8sfake` reactor with policy-aware logic |
| R2 | Cache key collision (different users hash to same key) | False positive authorization | Low | UT-AF-1221-007 | Use `sha256(user + "\x00" + sorted(groups) + "\x00" + tool)` with null byte separators |
| R3 | `sync.RWMutex` contention under high concurrency | Latency spike | Low | Race detector | RWMutex read-contention is minimal; write path (cache miss) is infrequent after warmup |
| R4 | Breaking change: customers using customized `rbac_roles.yaml` lose access | Production outage | Medium | E2E-AF-1221-001..002 | Helm NOTES.txt migration warning + CHANGELOG entry |
| R5 | Three RBAC code paths may have subtle behavioral differences | Migration regression | Medium | UT-AF-1221-011..015 | Tests cover both MCP and A2A paths explicitly |

---

## 4. Scope

### 4.1 Features to be Tested

- **SARChecker**: `Check(ctx, user, groups, tool)` with TTL cache, fail-closed
- **MCP Bridge**: `checkRBAC()` rewritten to use `ToolAuthorizer`
- **A2A Agent**: `newRBACGuard()` rewritten to use `ToolAuthorizer`
- **Config**: `RBACConfig` with `SARCacheTTL` (replaces `GroupMapping`)
- **Wiring**: `main.go` creates `SARChecker` from in-cluster authz client
- **ClusterRoles**: 6 pre-built roles grant correct tool subsets
- **list_tools**: Returns unfiltered tool list

### 4.2 Features Not to be Tested

- **K8s API server SAR internals**: Tested by Kubernetes itself
- **JWT extraction**: Already tested by `auth/jwt_test.go` (unchanged)
- **Helm chart rendering**: Covered by existing `helm template` smoke tests
- **Operator ClusterRoleBinding management**: Separate team's scope

### 4.3 Design Decisions

- **SAR over OPA/Casbin**: Leverages native K8s RBAC infrastructure, no new deps
- **TTL cache over no-cache**: Reduces API server load from ~20 SAR calls per tool invocation
- **`sync.RWMutex` over `sync.Map`**: Simpler, cache entries have TTL expiry, keys are structured
- **Interface on producer side**: `ToolAuthorizer` lives in `auth/` (producer) — justified by 3 consumers in `handler/`, `agent/`, and `cmd/` packages (100 Go Mistakes #6)

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: TESTING_GUIDELINES.md — Per-Tier Testable Code Coverage (>=80% per tier).

| Tier | Scope | Target | Code Subset |
|------|-------|--------|-------------|
| Unit | `SARChecker` cache logic, `checkRBAC` delegation, `newRBACGuard` delegation, config parsing | >=80% | Pure logic: cache hit/miss/TTL, input validation, interface delegation |
| Integration | SAR with real K8s RBAC (envtest), MCP bridge full flow | >=80% | I/O: K8s API calls, envtest ClusterRole evaluation |
| E2E | Infrastructure updates (ClusterRoles instead of ConfigMap) | N/A (infra only) | Full stack validation |

### 5.2 TDD Phases

| Phase | Description | Deliverables | Checkpoint |
|-------|-------------|-------------|------------|
| **Phase 2: TDD RED** | Write all failing tests | `sar_test.go`, `sar_integration_test.go`, updated bridge/agent/config/wiring tests | CHECKPOINT 1 |
| **Phase 3: TDD GREEN** | Minimal implementation to pass all tests | `sar.go`, modified `mcp_bridge.go`, `root.go`, `config.go`, `main.go` | CHECKPOINT 2 |
| **Phase 4: TDD REFACTOR** | Code quality: 100 Go Mistakes audit, lint, dedup | Cleaned code, no new lint errors | CHECKPOINT 3 |

### 5.3 Anti-Pattern Compliance

Per ANTI_PATTERN_DETECTION.md:

- Test business outcomes, not implementation details (no NULL-TESTING)
- No `Skip()` or pending tests
- No `time.Sleep()` without approved exception (use `Eventually` for async)
- Use table-driven tests where appropriate (SAR cache scenarios)
- All test descriptions include test ID (e.g., `UT-AF-1221-001`)
- Mock only external dependencies (K8s API via `k8sfake`)
- No `Ordered` container directives (parallel-safe)

---

## 6. Test Design Specification

### 6.1 Unit Tests — SARChecker Core (Tier 1)

**Test file**: `pkg/apifrontend/auth/sar_test.go`

| Test ID | Description | AC | Category |
|---------|-------------|-----|----------|
| UT-AF-1221-001 | Cache hit returns cached result without K8s API call | AC 4 | Happy Path |
| UT-AF-1221-002 | Cache miss calls K8s SAR API and stores result | AC 4 | Happy Path |
| UT-AF-1221-003 | Cache TTL expiry triggers fresh K8s SAR API call | AC 4 | Temporal |
| UT-AF-1221-004 | K8s API error returns `(false, err)` — fail-closed | AC 5 | Error Path |
| UT-AF-1221-005 | SAR denied returns `(false, nil)` | AC 1 | Happy Path |
| UT-AF-1221-006 | SAR allowed returns `(true, nil)` | AC 1 | Happy Path |
| UT-AF-1221-007 | Cache key uniqueness: different user/group/tool combos produce different keys | AC 4 | Edge Case |
| UT-AF-1221-008 | Groups propagated to SAR `spec.groups` field | AC 3 | Spec Compliance |
| UT-AF-1221-009 | Empty user returns error (input validation) | AC 5 | Edge Case |
| UT-AF-1221-010 | Empty tool name returns error (input validation) | AC 5 | Edge Case |

### 6.2 Unit Tests — MCP Bridge RBAC Delegation (Tier 1)

**Test file**: `pkg/apifrontend/handler/mcp_bridge_test.go` (update existing)

| Test ID | Description | AC | Category |
|---------|-------------|-----|----------|
| UT-AF-1221-011 | checkRBAC calls ToolAuthorizer.Check with correct user, groups, tool | AC 1 | Happy Path |
| UT-AF-1221-012 | checkRBAC denies when ToolAuthorizer returns false | AC 1 | Denial |
| UT-AF-1221-013 | checkRBAC denies when no UserIdentity in context | AC 1 | Edge Case |

### 6.3 Unit Tests — A2A Agent RBAC Delegation (Tier 1)

**Test file**: `pkg/apifrontend/agent/root_test.go` (update existing)

| Test ID | Description | AC | Category |
|---------|-------------|-----|----------|
| UT-AF-1221-014 | newRBACGuard calls ToolAuthorizer.Check with identity from context | AC 2 | Happy Path |
| UT-AF-1221-015 | newRBACGuard denies when ToolAuthorizer returns false, emits audit event | AC 2 | Denial |
| UT-AF-1221-016 | NewRootAgent returns all tools unfiltered (no FilterToolsByRole) | AC 7 | Regression |

### 6.4 Unit Tests — Config and Wiring (Tier 1)

**Test file**: `pkg/apifrontend/config/config_test.go` and `cmd/apifrontend/main_wiring_test.go`

| Test ID | Description | AC | Category |
|---------|-------------|-----|----------|
| UT-AF-1221-017 | RBACConfig parses `sarCacheTTL` from YAML | AC 9 | Config |
| UT-AF-1221-018 | RBACConfig defaults `sarCacheTTL` to 30s when absent | AC 9 | Config |
| UT-AF-1221-019 | Main wiring: no `loadRBACRoles` reference, SARChecker injected | AC 8 | Wiring |

### 6.5 Integration Tests — SAR with Real K8s RBAC (Tier 2)

**Test file**: `pkg/apifrontend/auth/sar_integration_test.go`

| Test ID | Description | AC | Category |
|---------|-------------|-----|----------|
| IT-AF-1221-001 | ClusterRole + ClusterRoleBinding grants tool access via SAR | AC 1, AC 6 | Happy Path |
| IT-AF-1221-002 | Missing ClusterRoleBinding denies tool access | AC 1, AC 5 | Denial |
| IT-AF-1221-003 | Multi-group membership grants union of tools | AC 3 | Multi-group |
| IT-AF-1221-004 | Cache TTL expiry: permission change reflected after TTL | AC 4 | Temporal |
| IT-AF-1221-005 | All 6 ClusterRoles grant correct tool subsets (table-driven) | AC 6 | Comprehensive |
| IT-AF-1221-006 | MCP Bridge full call_tool flow with SAR (allowed) | AC 1 | Integration |
| IT-AF-1221-007 | MCP Bridge full call_tool flow with SAR (denied) | AC 1 | Integration |

### 6.6 E2E Infrastructure Updates

**Test file**: `test/e2e/apifrontend/infrastructure/setup.go`, `test/infrastructure/fullpipeline_e2e.go`

| Test ID | Description | AC | Category |
|---------|-------------|-----|----------|
| E2E-AF-1221-001 | AF E2E: ClusterRoles deployed instead of rbac_roles ConfigMap | AC 6, AC 8 | Infra |
| E2E-AF-1221-002 | FP E2E: ClusterRoles deployed instead of rbac_roles ConfigMap | AC 6, AC 8 | Infra |

---

## 7. BR Coverage Matrix

| AC | Description | Test Type | Test IDs | Status |
|----|-------------|-----------|----------|--------|
| AC 1 | SAR replaces file-based RBAC for MCP call_tool | Unit + IT | UT-1221-005, -006, -011, -012, -013, IT-1221-001, -002, -006, -007 | **This plan** |
| AC 2 | SAR replaces file-based RBAC for A2A call_tool | Unit | UT-1221-014, -015 | **This plan** |
| AC 3 | JWT groups propagated to SAR spec.groups | Unit + IT | UT-1221-008, IT-1221-003 | **This plan** |
| AC 4 | TTL cache reduces API server load | Unit + IT | UT-1221-001, -002, -003, -007, IT-1221-004 | **This plan** |
| AC 5 | Fail-closed: SAR errors deny | Unit + IT | UT-1221-004, -009, -010, IT-1221-002 | **This plan** |
| AC 6 | 6 ClusterRoles in Helm chart | IT + E2E | IT-1221-001, -005, E2E-1221-001, -002 | **This plan** |
| AC 7 | list_tools unfiltered | Unit | UT-1221-016 | **This plan** |
| AC 8 | rbac_roles.yaml removed | Unit + E2E | UT-1221-019, E2E-1221-001, -002 | **This plan** |
| AC 9 | values.schema.json updated | Unit | UT-1221-017, -018 | **This plan** |

---

## 8. Test Case Specifications

### 8.1 UT-AF-1221-001: Cache hit returns cached result without API call

**AC**: AC 4
**Type**: Unit
**Category**: Happy Path
**Priority**: P0

**Preconditions**: `SARChecker` instantiated with `k8sfake.NewSimpleClientset()` and 30s TTL

**Steps**:
1. **Given**: `SARChecker` with a fake K8s client that counts SAR calls via `PrependReactor`
2. **When**: `Check(ctx, "alice", ["sre"], "kubernaut_approve")` is called twice within TTL
3. **Then**: First call triggers SAR API call, second does not

**Expected Result**:
- First call: `(true, nil)`, SAR API call count = 1
- Second call: `(true, nil)`, SAR API call count = 1 (cached)

### 8.2 UT-AF-1221-004: API error returns (false, err) — fail-closed

**AC**: AC 5
**Type**: Unit
**Category**: Error Path
**Priority**: P0

**Preconditions**: `SARChecker` with fake client configured to return error

**Steps**:
1. **Given**: Fake K8s client reactor returns `errors.New("api server unreachable")`
2. **When**: `Check(ctx, "alice", ["sre"], "kubernaut_approve")` is called
3. **Then**: Returns `(false, err)`

**Expected Result**:
- `allowed` is `false`
- `err` wraps the original API error
- Error result is NOT cached (next call retries)

### 8.3 UT-AF-1221-008: Groups propagated to SAR spec

**AC**: AC 3
**Type**: Unit
**Category**: Spec Compliance
**Priority**: P0

**Preconditions**: `SARChecker` with fake client that captures SAR request

**Steps**:
1. **Given**: Fake K8s client reactor captures `SubjectAccessReview` objects
2. **When**: `Check(ctx, "alice", ["sre", "system:authenticated"], "kubernaut_approve")` is called
3. **Then**: Captured SAR has correct fields

**Expected Result**:
- `sar.Spec.User` equals `"alice"`
- `sar.Spec.Groups` equals `["sre", "system:authenticated"]`
- `sar.Spec.ResourceAttributes.Verb` equals `"use"`
- `sar.Spec.ResourceAttributes.Group` equals `"kubernaut.ai"`
- `sar.Spec.ResourceAttributes.Resource` equals `"tools"`
- `sar.Spec.ResourceAttributes.Name` equals `"kubernaut_approve"`

### 8.4 IT-AF-1221-005: All 6 ClusterRoles grant correct tool subsets

**AC**: AC 6
**Type**: Integration (table-driven)
**Category**: Comprehensive
**Priority**: P0

**Preconditions**: envtest with 6 ClusterRoles and corresponding ClusterRoleBindings

**Steps** (table-driven, one entry per role):

| Role | User | Groups | Expected Tools (sample) | Expected Denied (sample) |
|------|------|--------|------------------------|--------------------------|
| `kubernaut-tool-sre` | `sre-user` | `["sre"]` | `kubernaut_approve`, `af_create_rr` | *(none — all tools)* |
| `kubernaut-tool-orchestrator` | `orch-user` | `["ai-orchestrator"]` | `kubernaut_start_investigation`, `af_create_rr` | `kubernaut_get_audit_trail` |
| `kubernaut-tool-approver` | `approver-user` | `["remediation-approver"]` | `kubernaut_approve`, `kubernaut_list_remediations` | `af_create_rr` |
| `kubernaut-tool-viewer` | `viewer-user` | `["observability"]` | `kubernaut_list_remediations`, `af_list_events` | `kubernaut_approve` |
| `kubernaut-tool-cicd` | `cicd-user` | `["cicd"]` | `kubernaut_list_remediations`, `kubernaut_watch` | `kubernaut_approve` |
| `kubernaut-tool-audit` | `audit-user` | `["l3-audit"]` | `kubernaut_get_audit_trail`, `kubernaut_get_effectiveness` | `kubernaut_approve` |

**Expected Result**:
- For each role, SAR check with allowed tool returns `(true, nil)`
- For each role, SAR check with denied tool returns `(false, nil)`

---

## 9. Checkpoint Specifications

### CHECKPOINT 0 — After Phase 1 (Test Plan)

**Gate criteria**: Test plan is complete, all ACs mapped.

**Preflight Check**:
- [ ] All 9 ACs mapped to at least 1 test
- [ ] >=80% per-tier coverage target achievable given test count (19 UT + 7 IT)
- [ ] No anti-patterns in test design
- [ ] Table-driven tests used for ClusterRole verification (IT-005)
- [ ] Confidence >= 95%

---

### CHECKPOINT 1 — After Phase 2 (TDD RED)

**Gate criteria**: All tests compile and FAIL (red).

#### 9-Category Audit

| # | Category | Tests That Satisfy | Notes |
|---|----------|--------------------|-------|
| 1 | **Observability wiring** | UT-015 (audit event on denial) | Existing `af_tool_calls_total{result="denied"}` unchanged |
| 2 | **Adversarial inputs** | UT-009 (empty user), UT-010 (empty tool) | All external inputs validated |
| 3 | **Resource bounds** | UT-007 (cache key uniqueness) | TTL prevents unbounded growth |
| 4 | **Concurrency** | IT-004 (cache under concurrent access), race detector | `sync.RWMutex` protection |
| 5 | **Nil/zero edge cases** | UT-009, UT-010, UT-013 (nil identity) | All nil/zero paths covered |
| 6 | **Error-path observability** | UT-004 (API error), UT-012 (denial) | Errors logged and returned |
| 7 | **Cross-phase integration** | IT-001..007 (envtest + real RBAC) | Full K8s API chain |
| 8 | **Spec compliance** | UT-008 (groups in SAR), UT-016 (unfiltered list_tools) | Verified against design |
| 9 | **API surface hygiene** | Only `ToolAuthorizer` interface + `NewSARChecker` exported | Minimal surface |

**Preflight Check**:
- [ ] All 19 unit test specs compile
- [ ] All 7 integration test specs compile
- [ ] All tests FAIL (red)
- [ ] No `Skip()` or pending tests
- [ ] Test descriptions include test IDs
- [ ] All test files use Ginkgo/Gomega BDD framework
- [ ] Confidence >= 95%

---

### CHECKPOINT 2 — After Phase 3 (TDD GREEN)

**Gate criteria**: All tests PASS. `go build ./...` succeeds. `go vet ./...` clean.

#### GA Readiness Audit

| Dimension | Check | Status |
|-----------|-------|--------|
| Data pipeline | JWT groups -> SAR spec.groups -> K8s RBAC evaluation | Pending |
| Security | Fail-closed on error, no AllowAll, no bypass flags | Pending |
| Performance | Cache hit avoids API call (verified via mock call count) | Pending |
| Backward compat | rbac_roles.yaml still exists (deleted in Phase 5) | Pending |
| Build integrity | `go build ./...` zero errors | Pending |
| Race safety | `go test -race` zero races | Pending |
| Regression | Existing AF tests still pass | Pending |

**Preflight Check**:
- [ ] All 26 tests PASS
- [ ] `go build ./...` zero errors
- [ ] `go vet ./...` clean
- [ ] `go test -race ./pkg/apifrontend/auth/...` zero races
- [ ] Existing test suites pass (regression)
- [ ] Confidence >= 95%

---

### CHECKPOINT 3 — After Phase 4 (TDD REFACTOR)

**Gate criteria**: All tests still PASS. Code quality validated.

#### 100 Go Mistakes Audit

| Mistake # | Title | Check | Status |
|-----------|-------|-------|--------|
| #1 | Unintended variable shadowing | No `err :=` inside `if` blocks that shadow outer `err` | Pending |
| #2 | Unnecessary nested code | Use early returns for validation errors | Pending |
| #5 | Interface pollution | `ToolAuthorizer` justified by 3 consumers | Pending |
| #6 | Interface on producer side | Justified: `handler/`, `agent/`, `cmd/` all consume | Pending |
| #8 | `any` says nothing | No `any`/`interface{}` in new code | Pending |
| #16 | Not using linters | `golangci-lint run --timeout=5m` | Pending |
| #21 | Inefficient slice initialization | Pre-size slices where length known | Pending |
| #27 | Inefficient map initialization | Pre-size cache map if applicable | Pending |
| #28 | Maps and memory leaks | Cache entries expire via TTL; verify no unbounded growth | Pending |
| #48 | Forgetting about context.Context | `ctx` passed to SAR API call | Pending |
| #53 | Not handling defer errors | No new defers in SAR module | Pending |
| #56 | Concurrency: not always faster | `sync.RWMutex`, not channels | Pending |
| #58 | Not understanding race conditions | RWMutex protects cache read/write | Pending |
| #77 | JSON handling mistakes | No JSON in SAR module (K8s typed client) | Pending |
| #84 | Not using testing utility packages | Ginkgo/Gomega per project standard | Pending |
| #100 | Not understanding Go diagnostics | `go vet`, race detector, lint | Pending |

#### GA Readiness Audit (Refactored Code)

| Dimension | Check | Status |
|-----------|-------|--------|
| Code quality | golangci-lint zero new errors | Pending |
| 100 Go Mistakes | All applicable checks pass | Pending |
| Test coverage | >=80% per tier on modified files | Pending |
| API surface | Only `ToolAuthorizer` + `NewSARChecker` exported | Pending |
| Race safety | `go test -race` zero races | Pending |

**Preflight Check**:
- [ ] All tests still PASS
- [ ] 100 Go Mistakes audit: all checks pass
- [ ] `golangci-lint run --timeout=5m` zero new errors
- [ ] Coverage >=80% per tier
- [ ] Confidence >= 95%

---

## 10. Implementation Phases (TDD)

### Phase 2: TDD RED — Write Failing Tests

**Files to create/modify**:
1. `pkg/apifrontend/auth/sar_test.go` — UT-AF-1221-001..010
2. `pkg/apifrontend/auth/sar_integration_test.go` — IT-AF-1221-001..005
3. `pkg/apifrontend/handler/mcp_bridge_test.go` — Update UT-AF-1221-011..013
4. `pkg/apifrontend/handler/mcp_bridge_integration_test.go` — Update IT-AF-1221-006..007
5. `pkg/apifrontend/agent/root_test.go` — Update UT-AF-1221-014..016
6. `pkg/apifrontend/config/config_test.go` — Update UT-AF-1221-017..018
7. `cmd/apifrontend/main_wiring_test.go` — Update UT-AF-1221-019

**Stub file** (empty implementation for compilation):
- `pkg/apifrontend/auth/sar.go` — `ToolAuthorizer` interface + `SARChecker` struct with `Check()` returning `(false, nil)` (always-deny stub)

**Expected state**: All tests compile but FAIL.

**CHECKPOINT 1 gate**: 9-category audit + preflight before proceeding.

### Phase 3: TDD GREEN — Minimal Implementation

**Files to modify**:
1. `pkg/apifrontend/auth/sar.go` — Full `SARChecker` with TTL cache
2. `pkg/apifrontend/handler/mcp_bridge.go` — Replace `checkRBAC()`, swap `RBACRoles` for `ToolAuthorizer`
3. `pkg/apifrontend/agent/root.go` — Rewrite `newRBACGuard()`, remove `FilterToolsByRole*`, delete embedded RBAC
4. `pkg/apifrontend/config/config.go` — Redefine `RBACConfig` with `SARCacheTTL`
5. `cmd/apifrontend/main.go` — Remove `loadRBACRoles()`, wire `SARChecker`

**Pattern to follow**: `pkg/shared/auth/K8sAuthorizer.CheckAccessWithGroup()` for SAR call.
Extend with `groups` field (existing version only passes `user`) and TTL cache.

**Expected state**: All tests PASS. `go build ./...` succeeds.

**CHECKPOINT 2 gate**: GA readiness audit + regression check.

### Phase 4: TDD REFACTOR — Code Quality

**Activities**:
1. 100 Go Mistakes audit (table in Section 9)
2. `golangci-lint run --timeout=5m`
3. `go vet ./...`
4. Review cache for memory leak potential (#28)
5. Verify `sync.RWMutex` correctness (#56, #58)
6. Extract shared validation if duplicated
7. Ensure all `Expect` assertions include business-outcome messages

**Expected state**: All tests still PASS. No new lint errors.

**CHECKPOINT 3 gate**: 100 Go Mistakes verification + final GA readiness audit.

---

## 11. Coverage Targets

| Metric | Target | Actual |
|--------|--------|--------|
| Unit test coverage (`sar.go` changes) | >=80% | Pending |
| Unit test coverage (`mcp_bridge.go` changes) | >=80% | Pending |
| Unit test coverage (`root.go` changes) | >=80% | Pending |
| Unit test coverage (`config.go` changes) | >=80% | Pending |
| Integration test coverage (SAR + envtest) | >=80% | Pending |
| Race detector | 0 races | Pending |
| Lint compliance | 0 new errors | Pending |
| Regression (existing tests) | 100% pass | Pending |

---

## 12. Execution Commands

```bash
# Phase 2-4: Unit tests (SAR module)
go test -race -count=1 ./pkg/apifrontend/auth/... -ginkgo.v -ginkgo.focus="1221"

# Phase 2-4: Unit tests (MCP bridge)
go test -race -count=1 ./pkg/apifrontend/handler/... -ginkgo.v -ginkgo.focus="1221"

# Phase 2-4: Unit tests (agent)
go test -race -count=1 ./pkg/apifrontend/agent/... -ginkgo.v -ginkgo.focus="1221"

# Phase 2-4: Integration tests (requires envtest)
go test -race -count=1 ./pkg/apifrontend/auth/... -ginkgo.v -ginkgo.focus="IT-AF-1221"

# Full regression
go test -race -count=1 ./pkg/apifrontend/... -ginkgo.v
go test -race -count=1 ./cmd/apifrontend/... -ginkgo.v

# Coverage
go test -coverprofile=coverage.out ./pkg/apifrontend/auth/... ./pkg/apifrontend/handler/... ./pkg/apifrontend/agent/... && go tool cover -func=coverage.out

# Build + lint
go build ./...
go vet ./...
golangci-lint run --timeout=5m
```

---

## 13. Dependencies

| Dependency | Version | Usage |
|------------|---------|-------|
| `k8s.io/client-go` | v0.35.5 | K8s authorization client (already in go.mod) |
| `k8s.io/api/authorization/v1` | v0.35.5 | SAR types (already transitively available) |
| `k8s.io/client-go/kubernetes/fake` | v0.35.5 | Unit test fake client (already used) |
| `github.com/onsi/ginkgo/v2` | latest | BDD test framework |
| `github.com/onsi/gomega` | latest | Assertion library |
| No new external dependencies | — | All changes use existing packages |

---

## 14. Sign-off

| Role | Name | Date | Status |
|------|------|------|--------|
| Author | AI Assistant | 2026-05-21 | Draft |
| Technical Review | | | Pending |
| QE Review | | | Pending |

---

## 15. Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-05-21 | AI Assistant | Initial test plan for Issue #1221 |
