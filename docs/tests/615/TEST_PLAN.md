# Test Plan: Notification Cluster Identification

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-615-v2.1
**Feature**: Add cluster name and UUID to all notification messages via boot-time auto-discovery
**Version**: 2.1
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.2.0-rc3`

---

## 1. Introduction

### 1.1 Purpose

Operators running Kubernaut on multiple clusters (e.g., OCP production and Kind demo) cannot determine which cluster generated a notification. This test plan validates that:
- Cluster identity (name + UUID) is auto-discovered at boot time from the Kubernetes API
- All 5 notification types include cluster identification when available
- Notifications gracefully degrade when cluster info is not discoverable

### 1.2 Objectives

1. **Cluster UUID discovery**: `DiscoverIdentity` reads `kube-system` namespace UID on every Kubernetes cluster
2. **Cluster name discovery**: `DiscoverIdentity` resolves name from OCP infrastructure, Kind node label, or falls back to empty
3. **Cluster line formatting**: `FormatClusterLine()` produces the correct string for all 4 input combinations (name+UUID, name-only, UUID-only, neither)
4. **Body injection**: All 5 body builders (`buildApprovalBody`, `buildCompletionBody`, `buildBulkDuplicateBody`, `buildManualReviewBody`, `buildSelfResolvedBody`) prepend the cluster line when available
5. **Graceful degradation**: When neither cluster name nor UUID is discoverable, notification bodies are identical to the pre-#615 format
6. **Backward compatibility**: All existing notification creator tests pass **without any modification** (setter injection, not constructor change)

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Notification unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/... -ginkgo.focus="615"` |
| Discovery unit test pass rate | 100% | `go test ./test/unit/shared/... -ginkgo.focus="CLUSTER-615"` |
| Unit-testable code coverage (notification) | >=80% | Coverage of `FormatClusterLine` + 5 body builders |
| Unit-testable code coverage (discovery) | >=80% | Coverage of `DiscoverIdentity` |
| Backward compatibility | 0 regressions, 0 existing callers modified | Existing notification creator + shared utility + aianalysis handler tests pass unchanged |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #615: Notification messages should include cluster name and UUID for multi-cluster disambiguation
- Issue #54: Multi-cluster federation (v1.5 refactoring path)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Constructor signature change breaks 109 existing callers | Massive mechanical churn, high regression risk | ~~High~~ Eliminated | ~~109 callers~~ | **Setter injection**: `SetClusterIdentity(name, uuid)` instead of constructor params. Zero existing callers modified. |
| R2 | `FormatClusterLine` produces trailing whitespace or empty lines | Notification body has visual artifacts | Low | UT-RO-615-001..004 | Dedicated formatting tests cover all 4 variants |
| R3 | OCP infrastructure API unavailable on non-OCP clusters | Discovery function errors or panics | Medium | UT-SHARED-615-004 | Graceful fallthrough: OCP discovery failure is swallowed, chain continues to Kind/empty |
| R4 | Kind node label absent on non-Kind clusters | Discovery function errors when listing nodes | Low | UT-SHARED-615-004 | Graceful fallthrough: Kind discovery returns empty name, no error |
| R5 | `kube-system` namespace missing (cluster in bad state) | UUID discovery fails | Very Low | UT-SHARED-615-005 | Error returned; caller falls back to empty `Identity{}` |
| R6 | RBAC rule for `config.openshift.io/infrastructures` causes issues on non-OCP | ClusterRole creation fails | Very Low | N/A (infrastructure) | Kubernetes ignores RBAC rules for non-existent API groups |
| R7 | No existing test pattern for `unstructured.Unstructured` + fake client | UT-SHARED-615-002 uses a new mocking pattern | Medium | UT-SHARED-615-002 | Register OCP GVK via `scheme.AddKnownTypeWithName()`. Pattern is well-supported by the fake client; documented as new convention. |
| R8 | Wiring path: `main.go` cannot reach `NewNotificationCreator` directly | Incorrect wiring could leave cluster info disconnected | ~~Medium~~ Eliminated | N/A | Setter pattern on Reconciler (`SetClusterIdentity`) delegates to `NotificationCreator`, matching existing patterns (`SetRESTMapper`, `SetNotifySelfResolved`) |

### 3.1 Risk-to-Test Traceability

- **R1** -> Eliminated by setter design; validated by zero changes to existing test files
- **R2** -> UT-RO-615-001 through UT-RO-615-004 (formatting variants)
- **R3** -> UT-SHARED-615-004 (no OCP, no Kind -> empty name, no error)
- **R4** -> UT-SHARED-615-004 (same test covers Kind label absence)
- **R5** -> UT-SHARED-615-005 (kube-system missing -> error returned)
- **R6** -> Not testable at unit level; validated by Helm smoke test CI job
- **R7** -> UT-SHARED-615-002 (OCP infrastructure test exercises the new pattern)
- **R8** -> Eliminated by setter design; wiring validated by build + E2E regression

---

## 4. Scope

### 4.1 Features to be Tested

- **`DiscoverIdentity()` function** (`pkg/shared/cluster/identity.go`): Boot-time cluster identity resolution via kube-system UID, OCP infrastructure, and Kind node label
- **`FormatClusterLine()` helper** (`pkg/remediationorchestrator/creator/notification.go`): Pure formatting logic for 4 input combinations
- **`SetClusterIdentity()` setter** (`pkg/remediationorchestrator/creator/notification.go`): Post-construction injection of cluster name/UUID into `NotificationCreator`
- **5 body builders** (`pkg/remediationorchestrator/creator/notification.go`): Cluster line prepended to approval, completion, bulk duplicate, manual review, and self-resolved notification bodies

### 4.2 Features Not to be Tested

- **Delivery layer** (`pkg/notification/delivery/`): Console, log, file, and Slack channels render `Spec.Body` verbatim; no changes needed. Covered by existing tests.
- **RBAC rule for `config.openshift.io`**: Infrastructure-level validation handled by Helm smoke test CI job, not by unit tests.
- **NotificationContext CRD schema**: No CRD changes in v1.2 (deferred to v1.5 per Issue #54).
- **Config override of cluster identity**: Deferred to a follow-up issue. Auto-discovery only in this release.
- **Existing 109 callers of `NewNotificationCreator`**: Setter injection means zero modifications to existing callers. Backward compatibility validated by running full existing test suites unchanged.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Setter injection** (`SetClusterIdentity`) instead of constructor params | Eliminates 109 mechanical caller updates (67 in notification_creator_test, 40 in aianalysis_handler_test, 1 integration test, 1 reconciler). Follows existing Reconciler setter convention. |
| Auto-discover cluster identity at boot time, not from static config | Zero configuration for OCP and Kind (the two active demo platforms); works out of the box |
| UUID from `kube-system` namespace UID | Universal across all K8s distributions; stable, unique, always available; RO already has RBAC |
| Name from OCP infrastructure -> Kind node label -> empty | Covers both demo platforms; graceful degradation on unsupported distributions |
| OCP resource accessed as `unstructured.Unstructured` | Avoids importing OpenShift client libraries; no new Go dependency |
| Inject cluster info into body at creation time, not at delivery time | All 5 delivery channels render `Spec.Body` verbatim -- body injection gives universal coverage with zero delivery-layer changes |
| No CRD schema change | `NotificationContext.ClusterContext` deferred to v1.5 multi-cluster (Issue #54) |
| Empty discovery = omit cluster line entirely | Graceful degradation -- no visual artifacts when cluster cannot be identified |
| Config override deferred to follow-up issue | Keeps v1.2 scope minimal; config override is additive (just adds priority check before discovery chain) |
| Discovery logic in `pkg/shared/cluster/` | Reusable by other controllers in v1.5 multi-cluster; follows existing `pkg/shared/` package convention |
| Setter wiring via `Reconciler.SetClusterIdentity` | Matches existing pattern: `SetRESTMapper`, `SetAsyncPropagation`, `SetNotifySelfResolved` in `main.go` |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit (notification formatting)**: >=80% of unit-testable code (`FormatClusterLine` + `SetClusterIdentity` + 5 body builders)
- **Unit (cluster discovery)**: >=80% of unit-testable code (`DiscoverIdentity` function covering UUID resolution, OCP name, Kind name, fallback, and error paths)
- **Integration**: Not applicable -- both components are pure logic tested via fake K8s clients. No real I/O or cross-component boundaries involved.
- **E2E**: Existing E2E tests for the full pipeline provide regression coverage. No new E2E tests needed.

### 5.2 Two-Tier Minimum

**Notification formatting**: Unit tests are the primary tier. The second tier is provided by existing fake-client integration tests in `test/unit/remediationorchestrator/notification_creator_test.go` (67 tests) and `test/unit/remediationorchestrator/aianalysis_handler_test.go` (40 tests) which exercise the full `Create*Notification` methods. These tests run **unchanged** and validate backward compatibility.

**Cluster discovery**: Unit tests with fake K8s clients exercise the full discovery chain. The `main.go` wiring is trivial (one setter call) and is validated by build verification + E2E regression.

### 5.3 Business Outcome Quality Bar

Each test validates what the **operator sees**:
- Discovery tests: "Does the controller correctly identify which cluster it runs on?"
- Notification tests: "Does the notification message show the cluster identity to the operator?"

### 5.4 Pass/Fail Criteria

**PASS** -- all of the following:
1. All 10 notification unit tests pass (0 failures)
2. All 5 discovery unit tests pass (0 failures)
3. All 109 existing `NewNotificationCreator` callers compile and pass **without modification**
4. `FormatClusterLine` coverage = 100% (4 branches, 4 tests)
5. `DiscoverIdentity` coverage >= 80% (5 scenarios covering all code paths)
6. Body builder cluster injection coverage >= 80%

**FAIL** -- any of the following:
1. Any new test fails
2. Any existing test regresses
3. Any existing caller of `NewNotificationCreator` requires modification
4. `FormatClusterLine` or `DiscoverIdentity` has uncovered branches below 80%

### 5.5 Suspension & Resumption Criteria

**Suspend**:
- Fake client cannot handle `unstructured.Unstructured` with registered GVK (impacts UT-SHARED-615-002)
- Code does not compile after adding setter

**Resume**:
- Alternative mocking approach validated (e.g., test `client.Reader` stub)
- Compilation restored

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/cluster/identity.go` (new) | `DiscoverIdentity`, `discoverUUID`, `discoverNameOCP`, `discoverNameKind` | ~60 |
| `pkg/remediationorchestrator/creator/notification.go` | `FormatClusterLine` (new), `SetClusterIdentity` (new) | ~15 |
| `pkg/remediationorchestrator/creator/notification.go` | `buildApprovalBody` | ~48 (lines 215-262) |
| `pkg/remediationorchestrator/creator/notification.go` | `buildCompletionBody` | ~45 (lines 384-429) |
| `pkg/remediationorchestrator/creator/notification.go` | `buildBulkDuplicateBody` | ~17 (lines 516-533) |
| `pkg/remediationorchestrator/creator/notification.go` | `buildManualReviewBody` | ~64 (lines 747-811) |
| `pkg/remediationorchestrator/creator/notification.go` | `buildSelfResolvedBody` | ~28 (lines 901-929) |

### 6.2 Integration-Testable Code (wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `cmd/remediationorchestrator/main.go` | `DiscoverIdentity` call + `SetClusterIdentity` setter call | ~8 |
| `internal/controller/remediationorchestrator/reconciler.go` | `SetClusterIdentity` method (delegates to `notificationCreator`) | ~5 |

**Note**: The `main.go` wiring is trivial (call discovery, call setter). Validated by build verification and E2E regression.

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/v1.2.0-rc3` HEAD | Current working branch |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #615-AC1 | Cluster UUID auto-discovered from `kube-system` namespace UID | P0 | Unit | UT-SHARED-615-001 | Pending |
| #615-AC2 | Cluster name auto-discovered from OCP infrastructure on OpenShift | P0 | Unit | UT-SHARED-615-002 | Pending |
| #615-AC3 | Cluster name auto-discovered from Kind node label on Kind clusters | P0 | Unit | UT-SHARED-615-003 | Pending |
| #615-AC4 | Cluster name empty when neither OCP nor Kind detected | P0 | Unit | UT-SHARED-615-004 | Pending |
| #615-AC5 | Discovery returns error when `kube-system` namespace missing | P1 | Unit | UT-SHARED-615-005 | Pending |
| #615-AC6 | Cluster line formatted as `**Cluster**: name (uuid)` when both available | P0 | Unit | UT-RO-615-001 | Pending |
| #615-AC7 | Cluster line shows name only when UUID unavailable | P0 | Unit | UT-RO-615-002 | Pending |
| #615-AC8 | Cluster line shows UUID only when name unavailable | P1 | Unit | UT-RO-615-003 | Pending |
| #615-AC9 | Cluster line omitted entirely when neither available | P0 | Unit | UT-RO-615-004 | Pending |
| #615-AC10 | Approval body contains cluster line at top | P0 | Unit | UT-RO-615-005 | Pending |
| #615-AC11 | Completion body contains cluster line at top | P0 | Unit | UT-RO-615-006 | Pending |
| #615-AC12 | Bulk duplicate body contains cluster line at top | P0 | Unit | UT-RO-615-007 | Pending |
| #615-AC13 | Manual review body contains cluster line at top | P0 | Unit | UT-RO-615-008 | Pending |
| #615-AC14 | Self-resolved body contains cluster line at top | P0 | Unit | UT-RO-615-009 | Pending |
| #615-AC15 | Body builders produce pre-#615 output when cluster info empty | P0 | Unit | UT-RO-615-010 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-615-{SEQUENCE}`

- `UT-SHARED-615-*`: Cluster identity discovery tests (`pkg/shared/cluster/`)
- `UT-RO-615-*`: Notification formatting + body builder tests (`pkg/remediationorchestrator/creator/`)

### Tier 1: Unit Tests -- Cluster Identity Discovery

**Testable code scope**: `DiscoverIdentity` in `pkg/shared/cluster/identity.go`. Target >=80% coverage.

**File**: `test/unit/shared/cluster/identity_test.go` (new)
**Suite file**: `test/unit/shared/cluster/cluster_suite_test.go` (new -- required for Ginkgo bootstrap)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-SHARED-615-001` | `kube-system` namespace exists: UUID = namespace UID | Pending |
| `UT-SHARED-615-002` | OCP `Infrastructure` resource exists: Name = `.status.infrastructureName` | Pending |
| `UT-SHARED-615-003` | No OCP infrastructure, Kind node with label `io.x-k8s.kind.cluster`: Name = label value | Pending |
| `UT-SHARED-615-004` | No OCP infrastructure, no Kind label on nodes: Name = "" (empty, no error) | Pending |
| `UT-SHARED-615-005` | `kube-system` namespace missing: error returned, `Identity` has empty UUID | Pending |

**Note on UT-SHARED-615-002**: This test uses `unstructured.Unstructured` with `fake.NewClientBuilder()` for the OCP `Infrastructure` resource. The GVK `config.openshift.io/v1/Infrastructure` must be registered in the test scheme via `scheme.AddKnownTypeWithName()`. This is a new test pattern for this codebase.

### Tier 1: Unit Tests -- Notification Formatting

**Testable code scope**: `FormatClusterLine` + `SetClusterIdentity` + 5 body builders in `pkg/remediationorchestrator/creator/notification.go`. Target >=80% coverage.

**File**: `test/unit/remediationorchestrator/notification_cluster_test.go` (new)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-615-001` | `FormatClusterLine` returns `**Cluster**: name (uuid)\n\n` when both set | Pending |
| `UT-RO-615-002` | `FormatClusterLine` returns `**Cluster**: name\n\n` when only name set | Pending |
| `UT-RO-615-003` | `FormatClusterLine` returns `**Cluster**: (uuid)\n\n` when only UUID set | Pending |
| `UT-RO-615-004` | `FormatClusterLine` returns empty string when neither set | Pending |
| `UT-RO-615-005` | Approval notification body starts with cluster line when `SetClusterIdentity` called | Pending |
| `UT-RO-615-006` | Completion notification body starts with cluster line when `SetClusterIdentity` called | Pending |
| `UT-RO-615-007` | Bulk duplicate notification body starts with cluster line when `SetClusterIdentity` called | Pending |
| `UT-RO-615-008` | Manual review notification body starts with cluster line when `SetClusterIdentity` called | Pending |
| `UT-RO-615-009` | Self-resolved notification body starts with cluster line when `SetClusterIdentity` called | Pending |
| `UT-RO-615-010` | All 5 notification bodies have no cluster line when `SetClusterIdentity` NOT called (default) | Pending |

### Tier Skip Rationale

- **Integration**: No new I/O or cross-component boundaries. Both the discovery function and the formatting function are tested via fake K8s clients. The `main.go` wiring is a single setter call, validated by build + E2E regression.
- **E2E**: Cluster identity discovery and string formatting do not require E2E validation. Existing E2E pipeline tests provide regression coverage.

---

## 9. Test Cases

### UT-SHARED-615-001: kube-system namespace UID becomes cluster UUID

**BR**: #615-AC1
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/cluster/identity_test.go`

**Preconditions**:
- Fake K8s client seeded with a `kube-system` Namespace object with UID `"test-uuid-abc-123"`

**Test Steps**:
1. **Given**: Fake client contains `kube-system` namespace with UID `"test-uuid-abc-123"`
2. **When**: `DiscoverIdentity(ctx, fakeClient)` is called
3. **Then**: Returned `Identity.UUID` equals `"test-uuid-abc-123"`

**Expected Results**:
1. `Identity.UUID` = `"test-uuid-abc-123"`
2. No error returned

### UT-SHARED-615-002: OCP infrastructure name becomes cluster name

**BR**: #615-AC2
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/cluster/identity_test.go`

**Preconditions**:
- Fake K8s client seeded with `kube-system` namespace AND an unstructured `Infrastructure` object (`config.openshift.io/v1`, name `"cluster"`) with `.status.infrastructureName = "ocp-prod-east"`
- Test scheme has `config.openshift.io/v1/Infrastructure` registered via `AddKnownTypeWithName`

**Test Steps**:
1. **Given**: Fake client contains `kube-system` namespace and OCP `Infrastructure` with `infrastructureName = "ocp-prod-east"`
2. **When**: `DiscoverIdentity(ctx, fakeClient)` is called
3. **Then**: Returned `Identity.Name` equals `"ocp-prod-east"`

**Expected Results**:
1. `Identity.Name` = `"ocp-prod-east"`
2. `Identity.UUID` is populated from `kube-system` UID
3. No error returned

### UT-SHARED-615-003: Kind node label becomes cluster name

**BR**: #615-AC3
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/cluster/identity_test.go`

**Preconditions**:
- Fake K8s client seeded with `kube-system` namespace and a Node with label `io.x-k8s.kind.cluster = "kind-demo"`
- No OCP `Infrastructure` object present

**Test Steps**:
1. **Given**: Fake client contains `kube-system` namespace and a Node with Kind label `"kind-demo"`
2. **And**: No OCP `Infrastructure` resource exists
3. **When**: `DiscoverIdentity(ctx, fakeClient)` is called
4. **Then**: Returned `Identity.Name` equals `"kind-demo"`

**Expected Results**:
1. `Identity.Name` = `"kind-demo"`
2. `Identity.UUID` is populated from `kube-system` UID
3. No error returned

### UT-SHARED-615-004: Neither OCP nor Kind -- empty name, no error

**BR**: #615-AC4
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/cluster/identity_test.go`

**Preconditions**:
- Fake K8s client seeded with `kube-system` namespace and a Node without Kind labels
- No OCP `Infrastructure` object present

**Test Steps**:
1. **Given**: Fake client contains `kube-system` namespace and a bare Node (no Kind label)
2. **And**: No OCP `Infrastructure` resource exists
3. **When**: `DiscoverIdentity(ctx, fakeClient)` is called
4. **Then**: Returned `Identity.Name` equals `""` (empty)
5. **And**: `Identity.UUID` is still populated from `kube-system` UID

**Expected Results**:
1. `Identity.Name` = `""` (empty)
2. `Identity.UUID` is populated
3. No error returned

### UT-SHARED-615-005: kube-system namespace missing -- error returned

**BR**: #615-AC5
**Priority**: P1
**Type**: Unit
**File**: `test/unit/shared/cluster/identity_test.go`

**Preconditions**:
- Fake K8s client with NO `kube-system` namespace

**Test Steps**:
1. **Given**: Fake client has no `kube-system` namespace
2. **When**: `DiscoverIdentity(ctx, fakeClient)` is called
3. **Then**: Error is returned
4. **And**: Returned `Identity` has empty UUID

**Expected Results**:
1. Error is non-nil
2. `Identity.UUID` = `""` (empty)

### UT-RO-615-001: FormatClusterLine with name and UUID

**BR**: #615-AC6
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_cluster_test.go`

**Test Steps**:
1. **Given**: `clusterName = "ocp-prod"`, `clusterUUID = "a1b2c3d4-e5f6"`
2. **When**: `FormatClusterLine("ocp-prod", "a1b2c3d4-e5f6")` is called
3. **Then**: Returns `"**Cluster**: ocp-prod (a1b2c3d4-e5f6)\n\n"`

### UT-RO-615-002: FormatClusterLine with name only

**BR**: #615-AC7
**Priority**: P0
**Type**: Unit

**Test Steps**:
1. **Given**: `clusterName = "kind-demo"`, `clusterUUID = ""`
2. **When**: `FormatClusterLine("kind-demo", "")` is called
3. **Then**: Returns `"**Cluster**: kind-demo\n\n"`

### UT-RO-615-003: FormatClusterLine with UUID only

**BR**: #615-AC8
**Priority**: P1
**Type**: Unit

**Test Steps**:
1. **Given**: `clusterName = ""`, `clusterUUID = "a1b2c3d4-e5f6"`
2. **When**: `FormatClusterLine("", "a1b2c3d4-e5f6")` is called
3. **Then**: Returns `"**Cluster**: (a1b2c3d4-e5f6)\n\n"`

### UT-RO-615-004: FormatClusterLine with neither

**BR**: #615-AC9
**Priority**: P0
**Type**: Unit

**Test Steps**:
1. **Given**: `clusterName = ""`, `clusterUUID = ""`
2. **When**: `FormatClusterLine("", "")` is called
3. **Then**: Returns `""`

### UT-RO-615-005: Approval body includes cluster line

**BR**: #615-AC10
**Priority**: P0
**Type**: Unit

**Test Steps**:
1. **Given**: `NotificationCreator` constructed normally, then `SetClusterIdentity("ocp-prod", "abc-123")` called
2. **And**: A valid `RemediationRequest` and `AIAnalysis` with `SelectedWorkflow`
3. **When**: `CreateApprovalNotification` is called
4. **Then**: The created `NotificationRequest.Spec.Body` starts with `**Cluster**: ocp-prod (abc-123)`
5. **And**: Body still contains `**Signal**:`, `**Severity**:`, `**Affected Resource**:`

### UT-RO-615-006: Completion body includes cluster line

**BR**: #615-AC11
**Priority**: P0
**Type**: Unit

**Test Steps**:
1. **Given**: `NotificationCreator` with `SetClusterIdentity("ocp-prod", "abc-123")` called
2. **And**: A valid `RemediationRequest`, `AIAnalysis`, and execution engine
3. **When**: `CreateCompletionNotification` is called
4. **Then**: The created `NotificationRequest.Spec.Body` starts with `**Cluster**: ocp-prod (abc-123)`

### UT-RO-615-007: Bulk duplicate body includes cluster line

**BR**: #615-AC12
**Priority**: P0
**Type**: Unit

**Test Steps**:
1. **Given**: `NotificationCreator` with `SetClusterIdentity("ocp-prod", "abc-123")` called
2. **And**: A `RemediationRequest` with `DuplicateCount > 0`
3. **When**: `CreateBulkDuplicateNotification` is called
4. **Then**: The created `NotificationRequest.Spec.Body` starts with `**Cluster**: ocp-prod (abc-123)`

### UT-RO-615-008: Manual review body includes cluster line

**BR**: #615-AC13
**Priority**: P0
**Type**: Unit

**Test Steps**:
1. **Given**: `NotificationCreator` with `SetClusterIdentity("ocp-prod", "abc-123")` called
2. **And**: A `ManualReviewContext` with source `AIAnalysis`
3. **When**: `CreateManualReviewNotification` is called
4. **Then**: The created `NotificationRequest.Spec.Body` starts with `**Cluster**: ocp-prod (abc-123)`

### UT-RO-615-009: Self-resolved body includes cluster line

**BR**: #615-AC14
**Priority**: P0
**Type**: Unit

**Test Steps**:
1. **Given**: `NotificationCreator` with `SetClusterIdentity("ocp-prod", "abc-123")` called
2. **And**: A valid `RemediationRequest` and `AIAnalysis`
3. **When**: `CreateSelfResolvedNotification` is called
4. **Then**: The created `NotificationRequest.Spec.Body` starts with `**Cluster**: ocp-prod (abc-123)`

### UT-RO-615-010: All bodies omit cluster line when SetClusterIdentity NOT called

**BR**: #615-AC15
**Priority**: P0
**Type**: Unit

**Test Steps**:
1. **Given**: `NotificationCreator` constructed normally, `SetClusterIdentity` **NOT** called (default empty)
2. **When**: Each of the 5 `Create*Notification` methods is called
3. **Then**: None of the 5 `NotificationRequest.Spec.Body` strings contain `**Cluster**:`
4. **And**: Bodies start with their original first line (e.g., `Remediation requires approval:`, `⚠️ **Manual Review Required**`)

---

## 10. Environmental Needs

### 10.1 Unit Tests (Notification Formatting)

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Fake K8s client (`sigs.k8s.io/controller-runtime/pkg/client/fake`) for `Create*Notification` tests
- **Location**: `test/unit/remediationorchestrator/notification_cluster_test.go`

### 10.2 Unit Tests (Cluster Identity Discovery)

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Fake K8s client seeded with Namespace, Node, and unstructured Infrastructure objects
- **Location**: `test/unit/shared/cluster/identity_test.go`
- **Suite bootstrap**: `test/unit/shared/cluster/cluster_suite_test.go` (required -- each subdirectory under `test/unit/shared/` has its own Ginkgo entry point)
- **Note**: OCP `Infrastructure` resource is created as `unstructured.Unstructured` with GVK registered via `scheme.AddKnownTypeWithName()`. This is a new test pattern for this codebase (no existing test uses unstructured + fake client).

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None. All changes are self-contained. The OCP `Infrastructure` resource is accessed as an unstructured object, so no external dependency on OpenShift client libraries is needed.

### 11.2 Execution Order

1. **TDD Phase 1 (RED)**: Write 10 failing notification tests (UT-RO-615-001..010) using `SetClusterIdentity` setter (no-op stub in RED)
2. **TDD Phase 2 (GREEN)**: Implement `SetClusterIdentity`, `FormatClusterLine`, prepend in 5 body builders -- all 10 pass. Zero existing callers modified.
3. **TDD Phase 3 (REFACTOR)**: Create `DiscoverIdentity` in `pkg/shared/cluster/`, write 5 discovery tests (UT-SHARED-615-001..005), add `Reconciler.SetClusterIdentity` setter, wire in `main.go`, add RBAC -- all 15 tests pass

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/615/TEST_PLAN.md` | Strategy and test design |
| Notification unit tests | `test/unit/remediationorchestrator/notification_cluster_test.go` | 10 Ginkgo BDD tests |
| Discovery suite bootstrap | `test/unit/shared/cluster/cluster_suite_test.go` | Ginkgo entry point |
| Discovery unit tests | `test/unit/shared/cluster/identity_test.go` | 5 Ginkgo BDD tests |

---

## 13. Execution

```bash
# New #615 notification tests only
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="615" -ginkgo.v -count=1

# New #615 discovery tests only
go test ./test/unit/shared/cluster/... -ginkgo.v -count=1

# Full RO notification creator regression (67 tests unchanged)
go test ./test/unit/remediationorchestrator/... -ginkgo.v -count=1

# Full shared utilities regression
go test ./test/unit/shared/... -ginkgo.v -count=1

# Build verification
go build ./...
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | N/A | **None** | Setter injection (`SetClusterIdentity`) means `NewNotificationCreator` signature is unchanged. All 109 existing callers (67 in notification_creator_test, 40 in aianalysis_handler_test, 1 in integration test, 1 in reconciler) compile and pass without modification. |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan (config-based cluster identity) |
| 2.0 | 2026-03-04 | Replaced config/env-var approach with boot-time auto-discovery. Added 5 discovery unit tests. Updated design decisions, risks, scope. Config override deferred. |
| 2.1 | 2026-03-04 | **Due diligence findings applied**: (1) Switched from constructor params to setter injection -- eliminates 109 mechanical caller updates. (2) Fixed wiring path -- setter on Reconciler delegates to NotificationCreator, matching existing patterns. (3) Documented new unstructured+fake-client test pattern for OCP. (4) Added missing Ginkgo suite bootstrap file. (5) Added R7/R8 risks with mitigations. (6) Updated all test steps to use `SetClusterIdentity` instead of constructor params. |
