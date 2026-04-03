# Test Plan: FileWatcher Migration + SLACK_WEBHOOK_URL Removal (#244)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-244-v1.0
**Feature**: Replace ConfigMap informer with FileWatcher for routing config hot-reload; remove SLACK_WEBHOOK_URL env var fallback
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.4`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the migration of the notification controller's routing configuration
hot-reload mechanism from a Kubernetes ConfigMap informer to the shared `FileWatcher` pattern
(DD-INFRA-001), and the removal of the legacy `SLACK_WEBHOOK_URL` environment variable fallback.
The plan ensures routing reload, credential resolution, and Slack delivery continue to function
correctly after the architectural change, with no regression in existing notification functionality.

### 1.2 Objectives

1. **Routing reload correctness**: Routing config changes detected from a mounted file produce identical Router state and delivery service registration as the previous ConfigMap-based path
2. **Credential resolution ordering**: Credential files are resolved successfully during routing reload under all startup sequences
3. **Thread safety**: Concurrent routing reloads and notification deliveries do not produce data races or inconsistent state
4. **Legacy removal completeness**: No production code path references `SLACK_WEBHOOK_URL`; all Slack delivery uses the per-receiver credential pattern exclusively
5. **RBAC reduction**: Notification controller operates without `configmaps` or `secrets` namespace permissions
6. **E2E parity**: Mock-slack delivery works end-to-end via `credentialRef` + FileWatcher without `SLACK_WEBHOOK_URL`

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/notification/...` |
| Integration test pass rate | 100% | `go test ./test/integration/notification/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on unit-testable files |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable files |
| Backward compatibility | 0 regressions | Existing notification tests pass without modification |
| Zero SLACK_WEBHOOK_URL references | 0 occurrences | `grep -r SLACK_WEBHOOK_URL cmd/notification/ internal/controller/notification/` |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-NOT-067: Hot-reload routing configuration without service restart
- BR-NOT-104: Per-receiver credential resolution for Slack delivery
- DD-INFRA-001: ConfigMap Hot-Reload Pattern (FileWatcher)
- Issue #244: Notification controller: replace ConfigMap informer with FileWatcher and remove SLACK_WEBHOOK_URL env var
- Issue #118 Gap 11: Legacy SLACK_WEBHOOK_URL fallback (being removed)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [DD-INFRA-001](../../architecture/decisions/DD-INFRA-001-configmap-hotreload-pattern.md)
- [Test Plan: BR-NOT-104](../NOT-104/TEST_PLAN.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Routing file mount missing in Helm/E2E | Controller starts with empty routing; all notifications go to console-only default | Medium | IT-NOT-244-001, E2E-NOT-244-001 | Validate volume mount in Helm template test; IT verifies startup with file present |
| R2 | Credential resolver not ready when FileWatcher fires initial callback | `Resolve()` fails; Slack channels not registered on startup | Medium | UT-NOT-244-005, IT-NOT-244-003 | Enforce startup ordering: credResolver.StartWatching → routingWatcher.Start; test ordering in IT |
| R3 | Concurrent routing reload + notification delivery race | `registeredSlackKeys` corrupted; Slack channel lookup fails mid-delivery | Low | UT-NOT-244-008 | Mutex on `registeredSlackKeys` access; unit test with concurrent goroutines |
| R4 | `SLACK_WEBHOOK_URL` still referenced in code or manifests | Legacy path delivers instead of per-receiver; credential pattern bypassed | Medium | UT-NOT-244-010, E2E-NOT-244-002 | Codebase grep for SLACK_WEBHOOK_URL in production code; E2E runs without env var |
| R5 | Malformed routing YAML causes panic or silent failure | Routing broken after bad config edit; notifications misrouted | Medium | UT-NOT-244-003, UT-NOT-244-004 | FileWatcher callback returns error → keeps previous config (graceful degradation) |
| R6 | ConfigMap cache removal breaks unrelated notification functionality | Reconciler fails to start or process NotificationRequests | Low | IT-NOT-244-001 | Verify reconciler starts and processes NR without ConfigMap cache |

### 3.1 Risk-to-Test Traceability

- **R1 (High)**: IT-NOT-244-001 (startup with file), E2E-NOT-244-001 (full delivery)
- **R2 (High)**: UT-NOT-244-005 (credential validation in reload), IT-NOT-244-003 (startup ordering)
- **R3 (Medium)**: UT-NOT-244-008 (concurrent reload)
- **R4 (Medium)**: UT-NOT-244-010 (grep validation), E2E-NOT-244-002 (delivery without env var)
- **R5 (Medium)**: UT-NOT-244-003 (malformed YAML rejected), UT-NOT-244-004 (previous config preserved)
- **R6 (Low)**: IT-NOT-244-001 (full reconciler lifecycle without CM cache)

---

## 4. Scope

### 4.1 Features to be Tested

- **ReloadRoutingFromContent** (`internal/controller/notification/routing_handler.go`): New file-based routing reload method; validates that routing config YAML parsed from file content produces correct Router state and per-receiver Slack delivery services
- **FileWatcher wiring** (`cmd/notification/main.go`): Startup sequence initializes FileWatcher with correct path, callback, and lifecycle management
- **Dead code removal** (`routing_handler.go`, `routing/router.go`): `handleConfigMapChange`, `loadRoutingConfigFromCluster`, `IsRoutingConfigMap`, `ExtractRoutingConfig`, `GetConfigMapNamespace` removed without breaking remaining functionality
- **SLACK_WEBHOOK_URL removal** (`cmd/notification/main.go`, `deploy/notification/02-deployment.yaml`): Legacy env var path fully excised from production code
- **RBAC cleanup** (Helm template, kubebuilder markers): `configmaps`/`secrets` permissions removed from notification controller
- **E2E credential-only delivery** (`test/infrastructure/notification_e2e.go`): Mock-slack delivery works without SLACK_WEBHOOK_URL via routing ConfigMap volume mount + credentialRef

### 4.2 Features Not to be Tested

- **`pkg/shared/hotreload/file_watcher.go`**: Covered by existing unit tests in `test/unit/shared/hotreload/`; FileWatcher is a stable shared component
- **`pkg/notification/credentials/resolver.go`**: Covered by existing tests in `test/unit/notification/` and `test/integration/notification/`; no changes to credential resolver
- **PagerDuty / Teams channels**: Deferred to #60 and #593 (v1.4 Wave 4)
- **Shared `config/rbac/role.yaml` (manager-role)**: Not touched; notification-specific RBAC only

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Routing file at `/etc/notification-routing/routing.yaml` (separate from `/etc/notification/`) | Avoids subPath (breaks ConfigMap hot-reload) and collision with existing config mount |
| `ReloadRoutingFromContent` as reconciler method, not standalone struct | Avoids new type; reuses existing reconciler fields; testable via direct method call |
| Mutex on `registeredSlackKeys` only (not full routing reload) | `Orchestrator.channels` is `sync.Map` (already thread-safe); only `registeredSlackKeys` slice needs protection |
| Startup order: credResolver → routingWatcher → mgr | Ensures credentials are loaded before routing callback resolves refs |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (pure logic: `ReloadRoutingFromContent` parsing, validation, credential ref checks, graceful degradation, thread safety)
- **Integration**: >=80% of **integration-testable** code (I/O: FileWatcher lifecycle, controller startup without ConfigMap cache, routing reload via file change, delivery service registration)
- **E2E**: Validates full stack delivery via credential-only path (no SLACK_WEBHOOK_URL)

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers (UT + IT):
- **Unit tests**: Catch routing reload logic errors, credential validation, graceful degradation, thread safety
- **Integration tests**: Catch FileWatcher wiring, controller lifecycle, delivery service registration across component boundaries

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes** — behavior, correctness, and data accuracy — not
just code path coverage. Each test scenario answers: "what does the user/operator/system
get?" not "what function is called?"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions approved by reviewer
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing notification test suites
5. `grep -r SLACK_WEBHOOK_URL cmd/notification/ internal/controller/notification/` returns 0 matches
6. Notification controller starts without ConfigMap RBAC errors in logs

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Existing tests that were passing before the change now fail (regression)
4. `SLACK_WEBHOOK_URL` reference found in production code

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- Build broken — code does not compile; unit tests cannot execute
- FileWatcher shared component has breaking changes (unlikely — stable since DD-INFRA-001)
- E2E Kind cluster cannot be provisioned

**Resume testing when**:

- Build fixed and green
- Blocking dependency resolved
- Infrastructure restored

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/notification/routing_handler.go` | `ReloadRoutingFromContent` (new), `rebuildSlackDeliveryServices`, `resolveChannelsFromRouting`, `formatRoutingConditions` | ~180 (after dead code removal) |
| `pkg/notification/routing/config.go` | `ParseConfig`, `Validate`, `ValidateCredentialRefs` | ~120 |
| `pkg/notification/routing/router.go` | `Router.LoadConfig`, `Router.ResolveChannels` | ~100 (after dead code removal) |

**Total unit-testable**: ~400 lines. Target: >=80% = >=320 lines covered.

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `cmd/notification/main.go` | FileWatcher initialization, startup ordering, SLACK_WEBHOOK_URL removal | ~30 (changed lines) |
| `internal/controller/notification/notificationrequest_controller.go` | `SetupWithManager` (ConfigMap watch removed), RBAC marker removal | ~20 (changed lines) |
| `internal/controller/notification/routing_handler.go` | `ReloadRoutingFromContent` I/O path (credential resolver interaction) | ~40 |

**Total integration-testable**: ~90 lines. Target: >=80% = >=72 lines covered.

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.4` HEAD | After #243 commits |
| Dependency: `pkg/shared/hotreload` | Stable (DD-INFRA-001) | No changes required |
| Dependency: `pkg/notification/credentials` | Stable (BR-NOT-104) | No changes required |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-NOT-067 | Routing config hot-reload via FileWatcher | P0 | Unit | UT-NOT-244-001 | Pending |
| BR-NOT-067 | Routing config hot-reload via FileWatcher | P0 | Unit | UT-NOT-244-002 | Pending |
| BR-NOT-067 | Graceful degradation on malformed config | P0 | Unit | UT-NOT-244-003 | Pending |
| BR-NOT-067 | Previous config preserved on parse error | P0 | Unit | UT-NOT-244-004 | Pending |
| BR-NOT-104 | Credential refs validated during reload | P0 | Unit | UT-NOT-244-005 | Pending |
| BR-NOT-104 | Per-receiver Slack services rebuilt on reload | P0 | Unit | UT-NOT-244-006 | Pending |
| BR-NOT-104 | Stale Slack channels unregistered on config change | P0 | Unit | UT-NOT-244-007 | Pending |
| BR-NOT-067 | Thread-safe routing reload under concurrency | P1 | Unit | UT-NOT-244-008 | Pending |
| BR-NOT-067 | Empty routing file uses default config | P1 | Unit | UT-NOT-244-009 | Pending |
| #244 | SLACK_WEBHOOK_URL removed from production code | P0 | Unit | UT-NOT-244-010 | Pending |
| BR-NOT-067 | Controller starts and reconciles without ConfigMap cache | P0 | Integration | IT-NOT-244-001 | Pending |
| BR-NOT-067 | File change triggers routing reload and delivery service rebuild | P0 | Integration | IT-NOT-244-002 | Pending |
| BR-NOT-104 | Startup ordering: credentials ready before routing reload | P0 | Integration | IT-NOT-244-003 | Pending |
| BR-NOT-067 | FileWatcher graceful degradation: file missing at startup | P1 | Integration | IT-NOT-244-004 | Pending |
| BR-NOT-104 | Mock-slack delivery via credentialRef without SLACK_WEBHOOK_URL | P0 | E2E | E2E-NOT-244-001 | Pending |
| #244 | No ConfigMap RBAC errors in controller logs | P0 | E2E | E2E-NOT-244-002 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-NOT-244-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **NOT**: Notification service
- **244**: Issue number
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `internal/controller/notification/routing_handler.go` (ReloadRoutingFromContent, rebuildSlackDeliveryServices), `pkg/notification/routing/` (ParseConfig, Validate, Router). Target: >=80%.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-NOT-244-001` | Valid routing YAML from file content produces correct Router state with matching receivers and routes | Pending |
| `UT-NOT-244-002` | Routing reload rebuilds per-receiver Slack delivery services with correct channel keys (`slack:<receiver>`) | Pending |
| `UT-NOT-244-003` | Malformed YAML is rejected with error; operator sees descriptive error in logs; previous routing preserved | Pending |
| `UT-NOT-244-004` | Config with invalid credentialRef (empty string) is rejected; previous routing and delivery services preserved | Pending |
| `UT-NOT-244-005` | Credential refs in routing config are validated against resolver; missing credential file produces actionable error | Pending |
| `UT-NOT-244-006` | Multiple receivers with Slack configs each produce distinct delivery services (e.g. `slack:success`, `slack:failure`) | Pending |
| `UT-NOT-244-007` | Config change from 3 receivers to 1 receiver unregisters stale channels; only active receiver's channels remain | Pending |
| `UT-NOT-244-008` | Concurrent routing reloads do not corrupt `registeredSlackKeys` or produce data races (run with `-race`) | Pending |
| `UT-NOT-244-009` | Empty file content (no routing YAML) uses default router config; existing delivery services remain | Pending |
| `UT-NOT-244-010` | No production code in `cmd/notification/` or `internal/controller/notification/` references `SLACK_WEBHOOK_URL` | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `cmd/notification/main.go` (FileWatcher wiring), `internal/controller/notification/` (controller lifecycle without ConfigMap cache). Target: >=80%.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-NOT-244-001` | Controller starts, reconciles a NotificationRequest through to delivery, without ConfigMap cache or RBAC for configmaps | Pending |
| `IT-NOT-244-002` | Routing config file change on disk triggers FileWatcher callback; new Slack delivery services are registered; old ones unregistered | Pending |
| `IT-NOT-244-003` | FileWatcher starts after credential resolver; initial routing reload resolves all credentialRefs successfully | Pending |
| `IT-NOT-244-004` | FileWatcher starts with missing routing file; controller starts with default config; file created later triggers reload | Pending |

### Tier 3: E2E Tests

**Testable code scope**: Full notification delivery pipeline (Kind cluster + mock-slack)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-NOT-244-001` | NotificationRequest with channel `slack:success` delivers to mock-slack via credentialRef + FileWatcher; no SLACK_WEBHOOK_URL env var present | Pending |
| `E2E-NOT-244-002` | Notification controller logs contain zero ConfigMap RBAC errors; controller operates with reduced permissions | Pending |

### Tier Skip Rationale

No tiers skipped. All 3 tiers provide coverage.

---

## 9. Test Cases

### UT-NOT-244-001: Valid routing YAML produces correct Router state

**BR**: BR-NOT-067
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/routing_reload_test.go`

**Preconditions**:
- Reconciler constructed with Router, mock CredentialResolver, mock DeliveryOrchestrator

**Test Steps**:
1. **Given**: A valid routing YAML string with `route.receiver: "slack-console"` and one receiver `slack-console` containing a `slackConfig` with `credentialRef: "slack-webhook"`
2. **When**: `ReloadRoutingFromContent(yamlContent)` is called
3. **Then**: Router's config summary shows 1 receiver; `ResolveChannels` for a matching alert returns `["slack:slack-console", "console"]`

**Expected Results**:
1. Method returns `nil` error
2. Router resolves matching alerts to the correct receiver channels
3. Per-receiver Slack delivery service registered under key `slack:slack-console`

**Acceptance Criteria**:
- **Behavior**: Routing YAML parsed and loaded into Router
- **Correctness**: Channel keys match `slack:<receiver-name>` convention
- **Accuracy**: Alert routing produces expected channels for matching labels

---

### UT-NOT-244-003: Malformed YAML rejected with descriptive error

**BR**: BR-NOT-067
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/routing_reload_test.go`

**Preconditions**:
- Reconciler with Router loaded with valid initial config

**Test Steps**:
1. **Given**: Router has valid config from a previous successful load
2. **When**: `ReloadRoutingFromContent("{{invalid yaml: [broken")` is called
3. **Then**: Error returned contains "failed to parse"; Router still has previous valid config

**Expected Results**:
1. Method returns non-nil error with descriptive message
2. Router's config summary is unchanged from the previous valid state
3. No Slack delivery services are unregistered or re-registered

**Acceptance Criteria**:
- **Behavior**: Malformed config rejected; previous config preserved (graceful degradation per DD-INFRA-001)
- **Correctness**: Error message includes parse context for operator debugging
- **Accuracy**: Router state is identical before and after the failed reload

---

### UT-NOT-244-007: Config change unregisters stale channels

**BR**: BR-NOT-104
**Priority**: P0
**Type**: Unit
**File**: `test/unit/notification/routing_reload_test.go`

**Preconditions**:
- Reconciler with 3 Slack receivers registered (`slack:r1`, `slack:r2`, `slack:r3`)

**Test Steps**:
1. **Given**: Three Slack delivery services registered from previous config
2. **When**: `ReloadRoutingFromContent` called with new config containing only receiver `r1`
3. **Then**: `slack:r2` and `slack:r3` are unregistered; only `slack:r1` remains

**Expected Results**:
1. Orchestrator's `HasChannel("slack:r1")` returns true
2. Orchestrator's `HasChannel("slack:r2")` returns false
3. Orchestrator's `HasChannel("slack:r3")` returns false

**Acceptance Criteria**:
- **Behavior**: Stale channels cleaned up on config change
- **Correctness**: Only channels from new config are registered
- **Accuracy**: No orphaned delivery services remain from previous config

---

### UT-NOT-244-008: Concurrent routing reloads are thread-safe

**BR**: BR-NOT-067
**Priority**: P1
**Type**: Unit
**File**: `test/unit/notification/routing_reload_test.go`

**Preconditions**:
- Reconciler with mutex-protected `registeredSlackKeys`

**Test Steps**:
1. **Given**: Reconciler ready to accept routing reloads
2. **When**: 10 goroutines concurrently call `ReloadRoutingFromContent` with different valid configs
3. **Then**: No data race detected (test run with `-race`); final state is consistent

**Expected Results**:
1. No panic or data race
2. `registeredSlackKeys` length matches the last successful config's receiver count
3. All registered channel keys are valid

**Acceptance Criteria**:
- **Behavior**: Concurrent reloads serialize safely via mutex
- **Correctness**: Final state reflects one complete config, not a mix

---

### IT-NOT-244-002: File change triggers routing reload

**BR**: BR-NOT-067
**Priority**: P0
**Type**: Integration
**File**: `test/integration/notification/routing_filewatcher_test.go`

**Preconditions**:
- Real FileWatcher watching a temp directory
- Real Router + mock credential resolver + real Orchestrator

**Test Steps**:
1. **Given**: FileWatcher started with initial routing config (1 receiver `slack:initial`)
2. **When**: Routing config file is overwritten with new content (2 receivers: `slack:alpha`, `slack:beta`)
3. **Then**: Within 5 seconds (Eventually), Orchestrator has channels `slack:alpha` and `slack:beta`; `slack:initial` is gone

**Expected Results**:
1. FileWatcher detects file change via fsnotify
2. Callback parses new config and rebuilds delivery services
3. Old channels unregistered, new channels registered

**Acceptance Criteria**:
- **Behavior**: File change on disk propagates to delivery service registration
- **Correctness**: Channel keys match new config's receiver names
- **Accuracy**: No stale channels from previous config

---

### E2E-NOT-244-001: Mock-slack delivery without SLACK_WEBHOOK_URL

**BR**: BR-NOT-104
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/notification/` (existing E2E framework)

**Preconditions**:
- Kind cluster with notification controller deployed
- Mock-slack nginx service running
- Routing ConfigMap mounted as volume at `/etc/notification-routing/`
- Credential Secret mounted at `/etc/notification/credentials/`
- No `SLACK_WEBHOOK_URL` env var on the controller pod

**Test Steps**:
1. **Given**: Notification controller running with FileWatcher-based routing config and projected credential volume
2. **When**: NotificationRequest created with labels matching the `slack:success` receiver
3. **Then**: Mock-slack nginx receives HTTP POST at `/webhook`; NR status transitions to Delivered

**Expected Results**:
1. NotificationRequest status is `Delivered`
2. Mock-slack received the notification payload
3. Controller logs show no ConfigMap RBAC errors
4. Controller env does NOT contain `SLACK_WEBHOOK_URL`

**Acceptance Criteria**:
- **Behavior**: End-to-end Slack delivery works via credentialRef + FileWatcher
- **Correctness**: Notification payload reaches mock-slack at the URL from the credential file
- **Accuracy**: No legacy fallback path used

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock credential resolver (returns configured webhook URLs), mock delivery orchestrator (tracks Register/Unregister calls)
- **Location**: `test/unit/notification/`
- **Resources**: Standard; `-race` flag for UT-NOT-244-008

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks for K8s API (envtest); mock credential resolver for credential files
- **Infrastructure**: envtest (K8s API), real FileWatcher watching temp directory, real Orchestrator
- **Location**: `test/integration/notification/`
- **Resources**: Standard

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster, mock-slack nginx, notification controller with FileWatcher
- **Location**: `test/e2e/notification/`
- **Resources**: Kind cluster (4 CPU, 4GB RAM minimum)

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| Kind | v0.20+ | E2E cluster (if applicable) |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| `pkg/shared/hotreload` (DD-INFRA-001) | Code | Stable | FileWatcher not available | None needed — already in codebase |
| `pkg/notification/credentials` (BR-NOT-104) | Code | Stable | Credential resolver not available | None needed |
| #243 merged | Code | Merged | No impact — independent | N/A |

### 11.2 Execution Order

1. **Phase 1**: Unit tests for `ReloadRoutingFromContent` (UT-NOT-244-001 through UT-NOT-244-010) — core logic
2. **Phase 2**: Implementation of `ReloadRoutingFromContent`, dead code removal, SLACK_WEBHOOK_URL removal
3. **Phase 3**: Integration tests (IT-NOT-244-001 through IT-NOT-244-004) — FileWatcher wiring, controller lifecycle
4. **Phase 4**: Helm/deploy manifest changes, RBAC cleanup
5. **Phase 5**: E2E migration (manifest updates) + E2E tests (E2E-NOT-244-001, E2E-NOT-244-002)

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/testing/244/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/notification/routing_reload_test.go` | ReloadRoutingFromContent tests |
| Integration test suite | `test/integration/notification/routing_filewatcher_test.go` | FileWatcher wiring + controller lifecycle |
| E2E validation | `test/e2e/notification/` (existing framework) | Credential-only delivery |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests (all #244)
go test ./test/unit/notification/... --ginkgo.focus="244"

# Unit tests with race detector (UT-NOT-244-008)
go test ./test/unit/notification/... --ginkgo.focus="UT-NOT-244-008" -race

# Integration tests (all #244)
go test ./test/integration/notification/... --ginkgo.focus="244"

# Specific test by ID
go test ./test/unit/notification/... --ginkgo.focus="UT-NOT-244-001"

# Coverage (unit-testable)
go test ./test/unit/notification/... -coverprofile=coverage-ut.out
go tool cover -func=coverage-ut.out

# Verify SLACK_WEBHOOK_URL removal
grep -r SLACK_WEBHOOK_URL cmd/notification/ internal/controller/notification/
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/notification/routing_hotreload_test.go` (IsRoutingConfigMap tests) | Tests `IsRoutingConfigMap` with various name/namespace combos | Remove entire test block (~5 tests at lines 289-326) | `IsRoutingConfigMap` function removed (dead code) |
| `test/unit/notification/routing_hotreload_test.go` (ConfigMap reload tests) | Tests `loadRoutingConfigFromCluster` via K8s API mock | Replace with `ReloadRoutingFromContent` tests using file content | Method replaced; K8s API path removed |
| `test/infrastructure/notification_e2e.go` (resolveSlackWebhookURL) | Resolves webhook URL from env var or file | Remove function entirely; routing config provides URL via credentialRef | SLACK_WEBHOOK_URL fallback removed |
| `test/infrastructure/notification_e2e.go` (notificationControllerManifest) | Includes `SLACK_WEBHOOK_URL` env var in deployment YAML | Remove env var; add `routing-config` volume mount | Per-receiver credential pattern only |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan based on due diligence analysis of #244 |
