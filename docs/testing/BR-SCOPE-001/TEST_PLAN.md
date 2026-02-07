# Test Plan: Resource Scope Management (BR-SCOPE-001)

**Feature**: Label-based resource opt-in (`kubernaut.ai/managed`)
**Version**: 1.0
**Created**: 2026-02-07
**Author**: AI Assistant + Jordi Gil
**Status**: Draft
**Branch**: `feat/label-based-resource-opt-in`

**Authority**:
- BR-SCOPE-001: Resource Scope Management
- BR-SCOPE-002: Gateway Signal Filtering
- BR-SCOPE-010: RO Routing Scope Validation
- ADR-053: Resource Scope Management Architecture

---

## 1. Scope

### In Scope

- **Shared Scope Manager**: `pkg/shared/scope/manager.go` — core `IsManaged()` logic
- **Gateway Integration**: Reject unmanaged signals before RR creation
- **RO Integration**: Fail RR when AA-determined target resource is unmanaged
- **Consecutive Failure Muting**: Existing `ConsecutiveFailureBlocker` handles loop prevention

### Out of Scope

- RemediationScope CRD — deferred to V2.0
- SP, AA, WE service changes — not needed (operate on own CRDs)
- New notification types — existing consecutive failure notification covers this
- Dynamic scope policies (Rego) — static labels only

### Design Decisions (Deviations from Original ADR-053)

| Decision | Original ADR-053 | This Implementation |
|----------|-----------------|---------------------|
| RO scope check target | Original signal resource | AA-determined RCA target resource |
| RO failure mode | Blocked (non-terminal) + exponential backoff | Failed (terminal) — AA result is deterministic |
| RO cache strategy | Direct API calls | Controller-runtime metadata-only cache (same as GW) |
| Muting mechanism | Custom retry for UnmanagedResource | Existing ConsecutiveFailureBlocker (3 failures → Blocked → 1hr) |

---

## 2. BR Coverage Matrix

| BR ID | Description | Priority | Test Type | Test ID | Status |
|-------|-------------|----------|-----------|---------|--------|
| BR-SCOPE-001 | 2-level hierarchy: resource → namespace → default unmanaged | P0 | Unit | UT-SCOPE-001-001 | ⏸️ |
| BR-SCOPE-001 | Resource label `true` = managed (explicit opt-in) | P0 | Unit | UT-SCOPE-001-002 | ⏸️ |
| BR-SCOPE-001 | Resource label `false` = unmanaged (explicit opt-out) | P0 | Unit | UT-SCOPE-001-003 | ⏸️ |
| BR-SCOPE-001 | No resource label → inherit from namespace | P0 | Unit | UT-SCOPE-001-004 | ⏸️ |
| BR-SCOPE-001 | Namespace label `true` = managed (inherited) | P0 | Unit | UT-SCOPE-001-005 | ⏸️ |
| BR-SCOPE-001 | Namespace label `false` = unmanaged (inherited) | P0 | Unit | UT-SCOPE-001-006 | ⏸️ |
| BR-SCOPE-001 | No labels anywhere → unmanaged (safe default) | P0 | Unit | UT-SCOPE-001-007 | ⏸️ |
| BR-SCOPE-001 | Resource opt-out overrides namespace opt-in | P0 | Unit | UT-SCOPE-001-008 | ⏸️ |
| BR-SCOPE-001 | Resource opt-in overrides namespace opt-out | P0 | Unit | UT-SCOPE-001-009 | ⏸️ |
| BR-SCOPE-001 | Namespace not found → unmanaged | P1 | Unit | UT-SCOPE-001-010 | ⏸️ |
| BR-SCOPE-001 | Resource not found → check namespace only | P1 | Unit | UT-SCOPE-001-011 | ⏸️ |
| BR-SCOPE-001 | Invalid label value (not "true"/"false") → unmanaged | P1 | Unit | UT-SCOPE-001-012 | ⏸️ |
| BR-SCOPE-001 | Cluster-scoped resource with managed label → managed | P0 | Unit | UT-SCOPE-001-013 | ⏸️ |
| BR-SCOPE-001 | Cluster-scoped resource without label → unmanaged (no NS fallback) | P0 | Unit | UT-SCOPE-001-014 | ⏸️ |
| BR-SCOPE-001 | Cluster-scoped resource with opt-out label → unmanaged | P0 | Unit | UT-SCOPE-001-015 | ⏸️ |
| BR-SCOPE-002 | Gateway rejects signal from unmanaged namespace | P0 | Unit | UT-GW-002-001 | ⏸️ |
| BR-SCOPE-002 | Gateway accepts signal from managed namespace | P0 | Unit | UT-GW-002-002 | ⏸️ |
| BR-SCOPE-002 | Gateway rejects signal when resource has opt-out label | P0 | Unit | UT-GW-002-003 | ⏸️ |
| BR-SCOPE-002 | Gateway returns actionable rejection response | P0 | Unit | UT-GW-002-004 | ⏸️ |
| BR-SCOPE-002 | Gateway increments `gateway_signals_rejected_total` metric | P1 | Unit | UT-GW-002-005 | ⏸️ |
| BR-SCOPE-002 | Gateway logs rejection with structured fields | P2 | Unit | UT-GW-002-006 | ⏸️ |
| BR-SCOPE-002 | Gateway does NOT create RR for unmanaged signal | P0 | Integration | IT-GW-002-001 | ⏸️ |
| BR-SCOPE-002 | Gateway creates RR for managed signal | P0 | Integration | IT-GW-002-002 | ⏸️ |
| BR-SCOPE-002 | Gateway scope validation < 10ms latency | P1 | Integration | IT-GW-002-003 | ⏸️ |
| BR-SCOPE-010 | RO fails RR when AA target resource is unmanaged | P0 | Unit | UT-RO-010-001 | ⏸️ |
| BR-SCOPE-010 | RO proceeds when AA target resource is managed | P0 | Unit | UT-RO-010-002 | ⏸️ |
| BR-SCOPE-010 | RO uses AA-determined target (not original signal source) | P0 | Unit | UT-RO-010-003 | ⏸️ |
| BR-SCOPE-010 | RO failure reason includes target resource details | P1 | Unit | UT-RO-010-004 | ⏸️ |
| BR-SCOPE-010 | RO failure triggers consecutive failure counter | P0 | Unit | UT-RO-010-005 | ⏸️ |
| BR-SCOPE-010 | 3 consecutive unmanaged failures → Blocked (muted) | P0 | Integration | IT-RO-010-001 | ⏸️ |
| BR-SCOPE-010 | Gateway deduplicates signals during muting period | P0 | Integration | IT-RO-010-002 | ⏸️ |
| BR-SCOPE-010 | Consecutive failure notification sent on blocking | P1 | Integration | IT-RO-010-003 | ⏸️ |
| BR-SCOPE-002 | GW E2E: Signal from managed resource → CRD created | P0 | E2E | E2E-GW-002-001 | ⏸️ |
| BR-SCOPE-002 | GW E2E: Signal from unmanaged resource → rejected, no CRD | P0 | E2E | E2E-GW-002-002 | ⏸️ |
| BR-SCOPE-002 | GW E2E: Signal from resource in managed namespace → CRD created | P0 | E2E | E2E-GW-002-003 | ⏸️ |
| BR-SCOPE-002 | GW E2E: Signal from cluster-scoped managed resource → CRD created | P1 | E2E | E2E-GW-002-004 | ⏸️ |
| BR-SCOPE-010 | RO E2E: AA targets managed resource → normal flow continues | P0 | E2E | E2E-RO-010-001 | ⏸️ |
| BR-SCOPE-010 | RO E2E: AA targets unmanaged resource → RR transitions to Failed | P0 | E2E | E2E-RO-010-002 | ⏸️ |
| BR-SCOPE-010 | RO E2E: 3 consecutive unmanaged AA targets → RR blocked (muted) | P0 | E2E | E2E-RO-010-003 | ⏸️ |

---

## 3. Test Cases

### Phase 1: Shared Scope Manager (`pkg/shared/scope/`)

#### UT-SCOPE-001-001: 2-level hierarchy resolution
**BR**: BR-SCOPE-001 (FR-SCOPE-001)
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Validate the complete 2-level hierarchy: resource label → namespace label → default unmanaged
**Preconditions**: Fake K8s client with namespace and resource objects
**Steps**:
1. Create namespace with `kubernaut.ai/managed=true`
2. Create resource WITHOUT managed label
3. Call `IsManaged(ctx, namespace, kind, name)`
**Expected Result**: Returns `true` (inherited from namespace)

#### UT-SCOPE-001-002: Resource label explicit opt-in
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Resource with `kubernaut.ai/managed=true` is managed regardless of namespace
**Preconditions**: Resource with managed label, namespace without label
**Steps**:
1. Create namespace WITHOUT managed label
2. Create resource WITH `kubernaut.ai/managed=true`
3. Call `IsManaged(ctx, namespace, kind, name)`
**Expected Result**: Returns `true`

#### UT-SCOPE-001-003: Resource label explicit opt-out
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Resource with `kubernaut.ai/managed=false` is unmanaged regardless of namespace
**Preconditions**: Resource with opt-out label, namespace with opt-in label
**Steps**:
1. Create namespace WITH `kubernaut.ai/managed=true`
2. Create resource WITH `kubernaut.ai/managed=false`
3. Call `IsManaged(ctx, namespace, kind, name)`
**Expected Result**: Returns `false` (resource overrides namespace)

#### UT-SCOPE-001-004: No resource label — inherit namespace
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Resource without label falls through to namespace check
**Preconditions**: Resource without label, namespace with label
**Steps**:
1. Create namespace WITH `kubernaut.ai/managed=true`
2. Create resource WITHOUT managed label
3. Call `IsManaged(ctx, namespace, kind, name)`
**Expected Result**: Returns `true` (inherited)

#### UT-SCOPE-001-005: Namespace label managed
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Namespace with `kubernaut.ai/managed=true` makes its resources managed
**Preconditions**: Namespace with managed label
**Steps**:
1. Create namespace WITH `kubernaut.ai/managed=true`
2. Call `IsManaged(ctx, namespace, "Deployment", "app")`
**Expected Result**: Returns `true`

#### UT-SCOPE-001-006: Namespace label unmanaged
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Namespace with `kubernaut.ai/managed=false` makes its resources unmanaged
**Preconditions**: Namespace with opt-out label
**Steps**:
1. Create namespace WITH `kubernaut.ai/managed=false`
2. Call `IsManaged(ctx, namespace, "Deployment", "app")`
**Expected Result**: Returns `false`

#### UT-SCOPE-001-007: No labels — safe default unmanaged
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Security
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Resources in unlabeled namespaces are unmanaged by default
**Preconditions**: Namespace and resource both without labels
**Steps**:
1. Create namespace WITHOUT managed label
2. Create resource WITHOUT managed label
3. Call `IsManaged(ctx, namespace, kind, name)`
**Expected Result**: Returns `false` (safe default)

#### UT-SCOPE-001-008: Resource opt-out overrides namespace opt-in
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Security
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Explicit resource opt-out takes precedence over namespace opt-in
**Preconditions**: Managed namespace, resource with `false` label
**Steps**:
1. Create namespace WITH `kubernaut.ai/managed=true`
2. Create resource WITH `kubernaut.ai/managed=false`
3. Call `IsManaged(ctx, namespace, kind, name)`
**Expected Result**: Returns `false`

#### UT-SCOPE-001-009: Resource opt-in overrides namespace opt-out
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Explicit resource opt-in takes precedence over namespace opt-out
**Preconditions**: Unmanaged namespace, resource with `true` label
**Steps**:
1. Create namespace WITH `kubernaut.ai/managed=false`
2. Create resource WITH `kubernaut.ai/managed=true`
3. Call `IsManaged(ctx, namespace, kind, name)`
**Expected Result**: Returns `true`

#### UT-SCOPE-001-010: Namespace not found
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Error Handling
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: If namespace doesn't exist, resource is unmanaged
**Preconditions**: Non-existent namespace
**Steps**:
1. Call `IsManaged(ctx, "nonexistent-ns", "Deployment", "app")`
**Expected Result**: Returns `false, nil` (not an error — just unmanaged)

#### UT-SCOPE-001-011: Resource not found — check namespace only
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Error Handling
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: If resource doesn't exist in cache, fall through to namespace check
**Preconditions**: Namespace exists, resource does not
**Steps**:
1. Create namespace WITH `kubernaut.ai/managed=true`
2. Call `IsManaged(ctx, "ns", "Deployment", "nonexistent")`
**Expected Result**: Returns `true` (inherited from namespace)

#### UT-SCOPE-001-012: Invalid label value
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Error Handling
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Non-"true"/"false" label values treated as unset (fall through)
**Preconditions**: Resource with `kubernaut.ai/managed=yes` (invalid)
**Steps**:
1. Create resource WITH `kubernaut.ai/managed=yes`
2. Create namespace WITH `kubernaut.ai/managed=true`
3. Call `IsManaged(ctx, namespace, kind, name)`
**Expected Result**: Returns `true` (invalid resource label ignored, namespace inherited)

#### UT-SCOPE-001-013: Cluster-scoped resource with managed label
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Cluster-scoped resource (e.g., Node) with `kubernaut.ai/managed=true` is managed
**Preconditions**: Resource with managed label, empty namespace
**Steps**:
1. Create Node with `kubernaut.ai/managed=true`
2. Call `IsManaged(ctx, "", "Node", "worker-01")`
**Expected Result**: Returns `true`

#### UT-SCOPE-001-014: Cluster-scoped resource without label — no namespace fallback
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Security
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Cluster-scoped resource without label is unmanaged (no namespace to inherit from)
**Preconditions**: Resource without label, empty namespace
**Steps**:
1. Create Node without managed label
2. Call `IsManaged(ctx, "", "Node", "worker-01")`
**Expected Result**: Returns `false` (no namespace fallback for cluster-scoped resources)

#### UT-SCOPE-001-015: Cluster-scoped resource with opt-out label
**BR**: BR-SCOPE-001
**Type**: Unit
**Category**: Security
**File**: `test/unit/shared/scope/manager_test.go`
**Description**: Cluster-scoped resource with explicit opt-out is unmanaged
**Preconditions**: Resource with `false` label, empty namespace
**Steps**:
1. Create Node with `kubernaut.ai/managed=false`
2. Call `IsManaged(ctx, "", "Node", "worker-01")`
**Expected Result**: Returns `false`

---

### Phase 2: Gateway Integration (`pkg/gateway/`)

#### UT-GW-002-001: Reject signal from unmanaged namespace
**BR**: BR-SCOPE-002 (FR-SCOPE-002-1)
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/gateway/scope_validation_test.go`
**Description**: Gateway rejects signal when namespace is not managed
**Preconditions**: Mock scope manager returning `false`
**Steps**:
1. Configure scope manager to return unmanaged
2. Call `ProcessSignal()` with signal from unmanaged namespace
**Expected Result**: Signal rejected, no RR created, rejection response returned

#### UT-GW-002-002: Accept signal from managed namespace
**BR**: BR-SCOPE-002 (FR-SCOPE-002-1)
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/gateway/scope_validation_test.go`
**Description**: Gateway accepts signal when namespace is managed
**Preconditions**: Mock scope manager returning `true`
**Steps**:
1. Configure scope manager to return managed
2. Call `ProcessSignal()` with signal from managed namespace
**Expected Result**: Signal accepted, RR created

#### UT-GW-002-003: Reject signal with resource opt-out
**BR**: BR-SCOPE-002 (FR-SCOPE-002-1)
**Type**: Unit
**Category**: Security
**File**: `test/unit/gateway/scope_validation_test.go`
**Description**: Gateway rejects signal when resource has explicit opt-out despite managed namespace
**Preconditions**: Mock scope manager returning `false` (resource opt-out)
**Steps**:
1. Configure scope manager to return unmanaged
2. Call `ProcessSignal()` with signal from managed namespace but opt-out resource
**Expected Result**: Signal rejected

#### UT-GW-002-004: Actionable rejection response
**BR**: BR-SCOPE-002 (FR-SCOPE-002-2)
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/gateway/scope_validation_test.go`
**Description**: Rejection response contains actionable instructions
**Preconditions**: Unmanaged signal
**Steps**:
1. Process signal from unmanaged namespace "kube-system"
**Expected Result**: Response contains `status: "rejected"`, `reason: "unmanaged_resource"`, `action` with label instructions, and resource details

#### UT-GW-002-005: Prometheus metric incremented
**BR**: BR-SCOPE-002 (FR-SCOPE-002-3)
**Type**: Unit
**Category**: Observability
**File**: `test/unit/gateway/scope_validation_test.go`
**Description**: Rejection increments `gateway_signals_rejected_total` metric
**Preconditions**: Mock metrics recorder
**Steps**:
1. Process signal from unmanaged namespace
**Expected Result**: `gateway_signals_rejected_total{reason="unmanaged_resource"}` incremented

#### UT-GW-002-006: Structured log on rejection
**BR**: BR-SCOPE-002 (FR-SCOPE-002-3)
**Type**: Unit
**Category**: Observability
**File**: `test/unit/gateway/scope_validation_test.go`
**Description**: Rejection produces structured INFO log with all required fields
**Preconditions**: Capture log output
**Steps**:
1. Process signal from unmanaged namespace
**Expected Result**: Log contains `signal_name`, `namespace`, `resource_kind`, `resource_name`, `reason`

#### IT-GW-002-001: No RR created for unmanaged signal (integration)
**BR**: BR-SCOPE-002 (NFR-SCOPE-002-1)
**Type**: Integration
**Category**: Happy Path
**File**: `test/integration/gateway/scope_filtering_test.go`
**Description**: Full Gateway processing flow: unmanaged signal does not create RR in K8s
**Preconditions**: envtest with unlabeled namespace
**Steps**:
1. Create namespace without managed label
2. Send signal to Gateway
3. List RemediationRequests in namespace
**Expected Result**: 0 RRs created

#### IT-GW-002-002: RR created for managed signal (integration)
**BR**: BR-SCOPE-002
**Type**: Integration
**Category**: Happy Path
**File**: `test/integration/gateway/scope_filtering_test.go`
**Description**: Full Gateway processing flow: managed signal creates RR
**Preconditions**: envtest with labeled namespace
**Steps**:
1. Create namespace with `kubernaut.ai/managed=true`
2. Send signal to Gateway
3. List RemediationRequests in namespace
**Expected Result**: 1 RR created

#### IT-GW-002-003: Scope validation latency < 10ms
**BR**: BR-SCOPE-002 (NFR-SCOPE-002)
**Type**: Integration
**Category**: Performance
**File**: `test/integration/gateway/scope_filtering_test.go`
**Description**: Scope validation does not add significant latency
**Preconditions**: envtest with cached namespace
**Steps**:
1. Warm cache with namespace metadata
2. Time 100 `IsManaged()` calls
**Expected Result**: P95 latency < 10ms

---

### Phase 3: RO Integration (`internal/controller/remediationorchestrator/`)

#### UT-RO-010-001: Fail RR when AA target is unmanaged
**BR**: BR-SCOPE-010 (FR-SCOPE-010-1)
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/remediationorchestrator/scope_validation_test.go`
**Description**: RO fails RR when AA-determined target resource is not managed
**Preconditions**: Mock scope manager returning `false`, mock AA CRD with target resource
**Steps**:
1. Create RR with completed AA CRD pointing to unmanaged target
2. Execute routing Check #6
**Expected Result**: Returns blocking condition with `Failed` outcome and `UnmanagedResource` reason

#### UT-RO-010-002: Proceed when AA target is managed
**BR**: BR-SCOPE-010 (FR-SCOPE-010-1)
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/remediationorchestrator/scope_validation_test.go`
**Description**: RO proceeds normally when AA target is managed
**Preconditions**: Mock scope manager returning `true`
**Steps**:
1. Create RR with completed AA CRD pointing to managed target
2. Execute routing Check #6
**Expected Result**: No blocking condition, routing continues

#### UT-RO-010-003: Uses AA target, not signal source
**BR**: BR-SCOPE-010
**Type**: Unit
**Category**: Security
**File**: `test/unit/remediationorchestrator/scope_validation_test.go`
**Description**: RO validates the AA-determined RCA target resource, not the original signal source resource
**Preconditions**: Signal source in namespace A (managed), AA target in namespace B (unmanaged)
**Steps**:
1. Create RR with signal from managed namespace A
2. AA CRD determines target resource in unmanaged namespace B
3. Execute routing Check #6
**Expected Result**: RR fails (target B is unmanaged, despite signal source A being managed)

#### UT-RO-010-004: Failure reason includes target details
**BR**: BR-SCOPE-010
**Type**: Unit
**Category**: Observability
**File**: `test/unit/remediationorchestrator/scope_validation_test.go`
**Description**: Failure reason message includes the specific unmanaged target resource
**Preconditions**: Unmanaged target resource
**Steps**:
1. Execute routing Check #6 with unmanaged target `production/Deployment/legacy-app`
**Expected Result**: Failure reason contains namespace, kind, and name of target resource, plus label instructions

#### UT-RO-010-005: Failure increments consecutive failure counter
**BR**: BR-SCOPE-010 + BR-ORCH-042
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/remediationorchestrator/scope_validation_test.go`
**Description**: UnmanagedResource terminal failure counts toward consecutive failure threshold
**Preconditions**: 2 existing Failed RRs with same fingerprint
**Steps**:
1. Create 3rd RR with same fingerprint, AA target unmanaged
2. RR transitions to Failed
3. 4th RR arrives with same fingerprint
**Expected Result**: 4th RR is Blocked by ConsecutiveFailureBlocker (3 consecutive failures reached)

#### IT-RO-010-001: 3 consecutive unmanaged failures → Blocked
**BR**: BR-SCOPE-010 + BR-ORCH-042
**Type**: Integration
**Category**: Happy Path
**File**: `test/integration/remediationorchestrator/scope_blocking_test.go`
**Description**: Full integration: 3 consecutive UnmanagedResource failures trigger blocking
**Preconditions**: envtest with full RO controller
**Steps**:
1. Create 3 RRs with same fingerprint, all targeting unmanaged resource
2. Wait for all 3 to reach Failed
3. Create 4th RR with same fingerprint
**Expected Result**: 4th RR transitions to Blocked with `ConsecutiveFailures` reason

#### IT-RO-010-002: Gateway deduplicates during muting
**BR**: BR-SCOPE-010 + BR-GATEWAY-185
**Type**: Integration
**Category**: Happy Path
**File**: `test/integration/remediationorchestrator/scope_blocking_test.go`
**Description**: While RR is Blocked, Gateway deduplicates new signals
**Preconditions**: Blocked RR exists
**Steps**:
1. Verify Blocked RR exists with same fingerprint
2. Simulate new signal with same fingerprint at Gateway
**Expected Result**: Gateway deduplicates (increments occurrence count), no new RR created

#### IT-RO-010-003: Notification sent on consecutive failure blocking
**BR**: BR-SCOPE-010 + BR-ORCH-042
**Type**: Integration
**Category**: Observability
**File**: `test/integration/remediationorchestrator/scope_blocking_test.go`
**Description**: When 3rd consecutive failure triggers blocking, a NotificationRequest is created
**Preconditions**: 3 consecutive failed RRs
**Steps**:
1. Trigger 3rd consecutive failure
2. Check for NotificationRequest CRD
**Expected Result**: NotificationRequest created with `consecutive_failures_blocked` type

---

### Phase 4: E2E Tests

> **Pattern**: Each service's E2E suite runs its real controller in a Kind cluster.
> Other services are simulated by manually updating child CRD statuses (RO pattern)
> or by sending HTTP webhooks and asserting on CRD creation (GW pattern).
> Full multi-service E2E is deferred to a separate task using MockLLM.

#### Gateway E2E (`test/e2e/gateway/`)

Follows the existing GW E2E pattern: send HTTP webhook to Gateway endpoint, assert on
RemediationRequest CRD creation via `k8sClient`. Tests require creating labeled/unlabeled
resources and namespaces before sending signals.

#### E2E-GW-002-001: Signal from managed resource creates CRD
**BR**: BR-SCOPE-002
**Type**: E2E
**Category**: Happy Path
**File**: `test/e2e/gateway/scope_filtering_e2e_test.go`
**Description**: Signal referencing a resource with `kubernaut.ai/managed=true` is accepted and a RemediationRequest CRD is created
**Preconditions**: Kind cluster with Gateway deployed, test namespace with managed label
**Steps**:
1. Create test namespace with `kubernaut.ai/managed=true`
2. Create a Pod in the namespace (inherits managed from namespace)
3. Send Prometheus webhook for the Pod to `POST /api/v1/signals/prometheus`
4. Assert HTTP 201/202 response
5. `Eventually` assert RemediationRequest CRD exists in the namespace
**Expected Result**: CRD created with correct target resource

#### E2E-GW-002-002: Signal from unmanaged resource is rejected
**BR**: BR-SCOPE-002
**Type**: E2E
**Category**: Security
**File**: `test/e2e/gateway/scope_filtering_e2e_test.go`
**Description**: Signal referencing a resource without managed label in an unlabeled namespace is rejected
**Preconditions**: Kind cluster with Gateway deployed, test namespace without managed label
**Steps**:
1. Create test namespace without `kubernaut.ai/managed` label
2. Create a Pod in the namespace
3. Send Prometheus webhook for the Pod to `POST /api/v1/signals/prometheus`
4. Assert HTTP rejection response (4xx with actionable message)
5. `Consistently` assert no RemediationRequest CRD is created
**Expected Result**: No CRD created, rejection response returned

#### E2E-GW-002-003: Signal from resource in managed namespace creates CRD
**BR**: BR-SCOPE-002
**Type**: E2E
**Category**: Happy Path
**File**: `test/e2e/gateway/scope_filtering_e2e_test.go`
**Description**: Signal referencing a resource without its own label, but in a managed namespace, is accepted
**Preconditions**: Kind cluster with Gateway deployed
**Steps**:
1. Create test namespace with `kubernaut.ai/managed=true`
2. Create a Pod without any kubernaut label
3. Send Prometheus webhook for the Pod
4. Assert HTTP 201/202 response
5. `Eventually` assert RemediationRequest CRD exists
**Expected Result**: CRD created (namespace inheritance)

#### E2E-GW-002-004: Signal from cluster-scoped managed resource creates CRD
**BR**: BR-SCOPE-002
**Type**: E2E
**Category**: Happy Path
**File**: `test/e2e/gateway/scope_filtering_e2e_test.go`
**Description**: Signal referencing a cluster-scoped resource (Node) with `kubernaut.ai/managed=true` is accepted
**Preconditions**: Kind cluster with Gateway deployed
**Steps**:
1. Label the Kind control-plane Node with `kubernaut.ai/managed=true`
2. Send K8s Event webhook referencing the Node
3. Assert HTTP 201/202 response
4. `Eventually` assert RemediationRequest CRD exists
5. Cleanup: remove label from Node
**Expected Result**: CRD created for cluster-scoped resource

#### Remediation Orchestrator E2E (`test/e2e/remediationorchestrator/`)

Follows the existing RO E2E pattern: create RemediationRequest CRD, wait for RO to create
child CRDs (SP, AA), manually update child CRD statuses to simulate other services, assert
on RR phase transitions.

#### E2E-RO-010-001: AA targets managed resource — normal flow continues
**BR**: BR-SCOPE-010
**Type**: E2E
**Category**: Happy Path
**File**: `test/e2e/remediationorchestrator/scope_validation_e2e_test.go`
**Description**: When AA determines a target resource that is managed, RO proceeds normally
**Preconditions**: Kind cluster with RO deployed, test namespace with managed label
**Steps**:
1. Create test namespace with `kubernaut.ai/managed=true`
2. Create a Pod with `kubernaut.ai/managed=true` (AA target)
3. Create RemediationRequest CRD
4. Wait for RO to create SignalProcessing → manually update to Completed
5. Wait for RO to create AIAnalysis → manually update to Completed with target = the managed Pod
6. Assert RR proceeds to WorkflowExecution phase (not Failed)
**Expected Result**: RR transitions beyond routing phase (scope check passes)

#### E2E-RO-010-002: AA targets unmanaged resource — RR transitions to Failed
**BR**: BR-SCOPE-010
**Type**: E2E
**Category**: Security
**File**: `test/e2e/remediationorchestrator/scope_validation_e2e_test.go`
**Description**: When AA determines a target resource that is unmanaged, RO fails the RR
**Preconditions**: Kind cluster with RO deployed
**Steps**:
1. Create managed namespace (signal source is managed)
2. Create unmanaged namespace (AA target will be here)
3. Create an unmanaged Pod in the unmanaged namespace
4. Create RemediationRequest CRD in managed namespace
5. Wait for RO to create SignalProcessing → manually update to Completed
6. Wait for RO to create AIAnalysis → manually update to Completed with target = the unmanaged Pod
7. `Eventually` assert RR transitions to `Failed` phase
8. Assert RR status reason includes "UnmanagedResource"
**Expected Result**: RR Failed (terminal) with UnmanagedResource reason

#### E2E-RO-010-003: Consecutive unmanaged AA targets trigger blocking (muting)
**BR**: BR-SCOPE-010, BR-ORCH-042
**Type**: E2E
**Category**: Security
**File**: `test/e2e/remediationorchestrator/scope_validation_e2e_test.go`
**Description**: After 3 consecutive failures due to unmanaged AA targets for the same fingerprint, the RR is blocked
**Preconditions**: Kind cluster with RO deployed, ConsecutiveFailureBlocker threshold = 3
**Steps**:
1. Create managed namespace and unmanaged target namespace
2. Create 3 RemediationRequests with the same `SignalFingerprint`
3. For each: wait for SP → complete, wait for AA → complete with unmanaged target
4. Assert first 2 RRs transition to `Failed`
5. `Eventually` assert 3rd RR transitions to `Blocked` phase
6. Assert `BlockedUntil` timestamp is set
7. Verify a NotificationRequest CRD was created (consecutive failure notification)
**Expected Result**: 3rd RR Blocked, notification sent, subsequent signals muted

---

## 4. Coverage Targets

| Metric | Target | Actual |
|--------|--------|--------|
| Shared Scope Manager Unit (incl. cluster-scoped) | 90% | ⏸️ |
| Gateway Scope Unit | 80% | ⏸️ |
| RO Scope Unit | 80% | ⏸️ |
| Gateway Scope E2E | 70% | ⏸️ |
| RO Scope E2E | 70% | ⏸️ |
| BR Coverage | 100% | ⏸️ |
| Critical Path (P0) Coverage | 100% | ⏸️ |

---

## 5. Test File Locations

| Component | Unit Tests | Integration Tests | E2E Tests |
|-----------|-----------|-------------------|-----------|
| Shared Scope Manager | `test/unit/shared/scope/manager_test.go` | — | — |
| Gateway Scope | `test/unit/gateway/scope_validation_test.go` | `test/integration/gateway/scope_filtering_test.go` | `test/e2e/gateway/scope_filtering_e2e_test.go` |
| RO Scope | `test/unit/remediationorchestrator/scope_validation_test.go` | `test/integration/remediationorchestrator/scope_blocking_test.go` | `test/e2e/remediationorchestrator/scope_validation_e2e_test.go` |

---

## 6. Sign-off

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Author | AI Assistant | 2026-02-07 | ⏸️ |
| Reviewer | Jordi Gil | | ⏸️ |
| Approver | | | ⏸️ |
