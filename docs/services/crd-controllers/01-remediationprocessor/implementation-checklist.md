## Implementation Checklist

**Note**: Follow APDC-TDD phases for each implementation step (see Development Methodology section)

### Phase 1: ANALYSIS & Package Migration (1-2 days) [RED Phase Preparation]

- [ ] **ANALYSIS**: Search existing implementations (`codebase_search "AlertProcessor implementations"`)
- [ ] **ANALYSIS**: Map business requirements across all V1 BRs:
  - **V1 Scope**: BR-AP-001 to BR-AP-062 (22 BRs total)
    - BR-AP-001 to 050: Core alert processing (16 BRs)
    - BR-AP-051 to 053: Environment classification (3 BRs, migrated from BR-ENV-*)
    - BR-AP-060 to 062: Alert enrichment (3 BRs, migrated from BR-ALERT-*)
  - **Reserved for V2**: BR-AP-063 to BR-AP-180 (multi-source context, advanced correlation)

### Logging Library

- **Library**: `sigs.k8s.io/controller-runtime/pkg/log/zap`
- **Rationale**: Official controller-runtime integration with opinionated defaults for Kubernetes controllers
- **Setup**: Initialize in `main.go` with `ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))`
- **Usage**: `log := ctrl.Log.WithName("remediationprocessor")`

---

- [ ] **ANALYSIS**: Identify integration points in cmd/remediationprocessor/
- [ ] **Package Migration RED**: Write tests validating type-safe interfaces (fail with map[string]interface{})
- [ ] **Package Migration GREEN**: Implement structured types in `pkg/remediationprocessing/types.go`
  - [ ] **Package Rename**: `pkg/alert/` → `pkg/remediationprocessing/`
  - [ ] **Update Package Declarations**: `package alert` → `package alertprocessor`
  - [ ] **Update Imports**: Across ~50 files
  - [ ] **Interface Rename**: `AlertService` → `AlertProcessorService`
  - [ ] **Remove Deduplication**: Delete `AlertDeduplicatorImpl` (move to Gateway Service)
- [ ] **Package Migration REFACTOR**: Enhance error handling and validation logic
- [ ] **Test Directory Migration**:
  - [ ] Rename `test/unit/alert/` → `test/unit/remediationprocessing/`
  - [ ] Rename `test/integration/alert_processing/` → `test/integration/remediationprocessing/`
  - [ ] Create `test/e2e/alertprocessor/` (new directory)
  - [ ] Update package declarations: `package alert` → `package alertprocessor`

### Phase 2: CRD Implementation (3-4 days) [RED-GREEN-REFACTOR]

- [ ] **CRD RED**: Write RemediationProcessingReconciler tests (should fail - no controller yet)
- [ ] **CRD GREEN**: Generate CRD using Kubebuilder + controller skeleton (tests pass)
  - [ ] Generate RemediationProcessing CRD (`api/remediationprocessing/v1/alertprocessing_types.go`)
  - [ ] Implement RemediationProcessingReconciler with 3 phases (enriching, classifying, routing)
  - [ ] Add owner references and finalizers for cascade deletion
- [ ] **CRD REFACTOR**: Enhance controller with phase logic and error handling
  - [ ] Implement phase timeout detection and handling
  - [ ] Add Kubernetes event emission for visibility
  - [ ] Implement optimized requeue strategy
- [ ] **Integration RED**: Write tests for owner reference management (fail initially)
- [ ] **Integration GREEN**: Implement owner references to RemediationRequest (tests pass)

### Phase 3: Business Logic Integration (1-2 days) [RED-GREEN-REFACTOR]

- [ ] **Logic RED**: Write tests for environment classification with mocked Context Service (fail)
- [ ] **Logic GREEN**: Integrate business logic to pass tests
  - [ ] Integrate existing environment classification logic from `pkg/processor/environment/`
  - [ ] Add Context Service HTTP client
  - [ ] Add status update for RemediationRequest reference
- [ ] **Logic REFACTOR**: Enhance with sophisticated algorithms
  - [ ] Add degraded mode fallback when Context Service unavailable
  - [ ] Optimize classification heuristics and performance
- [ ] **Audit Integration**: Integrate audit storage for long-term tracking
- [ ] **Main App Integration**: Verify RemediationProcessingReconciler instantiated in cmd/remediationprocessor/ (MANDATORY)

### Phase 4: Testing & Validation (1 day) [CHECK Phase]

- [ ] **CHECK**: Verify 70%+ unit test coverage (test/unit/remediationprocessing/)
  - [ ] Write unit tests for each reconciliation phase
  - [ ] Use fake K8s client, mock Context Service only
- [ ] **CHECK**: Run integration tests - 20% coverage target (test/integration/remediationprocessing/)
  - [ ] Add integration tests with real Context Service
  - [ ] Test CRD lifecycle with real K8s API (KIND)
- [ ] **CHECK**: Execute E2E tests for critical workflows (test/e2e/alertprocessor/)
  - [ ] Add E2E tests for complete alert-to-remediation workflow
- [ ] **CHECK**: Validate business requirement coverage (all 22 V1 BRs)
  - [ ] BR-AP-001 to 050: Core alert processing
  - [ ] BR-AP-051 to 053: Environment classification
  - [ ] BR-AP-060 to 062: Alert enrichment
- [ ] **CHECK**: Configure RBAC for controller
- [ ] **CHECK**: Add Prometheus metrics for phase durations
- [ ] **CHECK**: Provide confidence assessment (85% - high confidence, see Development Methodology)

## Critical Architectural Patterns (from MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)

### 1. Owner References & Cascade Deletion
**Pattern**: RemediationProcessing CRD owned by RemediationRequest
```go
controllerutil.SetControllerReference(&alertRemediation, &alertProcessing, scheme)
```
**Purpose**: Automatic cleanup when RemediationRequest is deleted (24h retention)

### 2. Finalizers for Cleanup Coordination
**Pattern**: Add finalizer before processing, remove after cleanup
```go
const alertProcessingFinalizer = "alertprocessing.kubernaut.io/finalizer"
```
**Purpose**: Ensure audit data persisted before CRD deletion

### 3. Watch-Based Status Coordination
**Pattern**: Status updates trigger RemediationRequest reconciliation automatically
```go
// Status update here triggers RemediationRequest watch
r.Status().Update(ctx, &alertProcessing)
```
**Purpose**: No manual RemediationRequest updates needed - watch handles aggregation

### 4. Phase Timeout Detection & Escalation
**Pattern**: Per-phase timeout with degraded mode fallback
```go
defaultPhaseTimeout = 5 * time.Minute
```
**Purpose**: Prevent stuck processing, enable degraded mode continuation

### 5. Event Emission for Visibility
**Pattern**: Emit Kubernetes events for operational tracking
```go
r.Recorder.Event(&alertProcessing, "Normal", "PhaseCompleted", message)
```
**Purpose**: Operational visibility in kubectl events and monitoring

### 6. Optimized Requeue Strategy
**Pattern**: Phase-based requeue intervals, no requeue for terminal states
```go
// Completed state: no requeue (watch handles updates)
// Active phases: 10s requeue
// Unknown states: 30s conservative requeue
```
**Purpose**: Efficient reconciliation, reduced API server load

### 7. Cross-CRD Reference Validation
**Pattern**: Validate RemediationRequestRef exists before processing
```go
r.Get(ctx, alertRemediationRef, &alertRemediation)
```
**Purpose**: Ensure parent CRD exists, prevent orphaned processing

### 8. Metrics for Reconciliation Performance
**Pattern**: Track controller performance separately from business metrics
```go
// Controller-specific metrics
ControllerReconciliationDuration
ControllerErrorsTotal
ControllerRequeueTotal
```
**Purpose**: Monitor controller health vs business logic performance

