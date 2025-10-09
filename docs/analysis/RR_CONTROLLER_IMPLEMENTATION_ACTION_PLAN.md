# RemediationRequest Controller Implementation Action Plan

**Date**: 2025-01-10  
**Based On**: `docs/services/crd-controllers/05-remediationorchestrator/` documentation  
**Current Status**: Phase 1 (Task 2.2) - RemediationProcessing CRD creation only  
**Target**: Complete orchestrator implementation per specifications

---

## 📋 **Executive Summary**

This action plan implements the RemediationRequest (RR) controller as the central orchestrator for the multi-CRD remediation architecture. The plan follows the APDC-TDD methodology and implements all features documented in `docs/services/crd-controllers/05-remediationorchestrator/`.

**Scope**: Orchestration logic only - business logic is in dedicated controllers  
**Estimated Effort**: 7 days (3.5 days P0 + 2 days P1 + 1.5 days observability)  
**Testing**: 3.5 days (unit + integration)  
**Total**: 10.5 days

---

## 🎯 **Implementation Priorities**

### **P0 - Core Orchestration** (MUST HAVE - 3.5 days)
- AIAnalysis CRD creation
- WorkflowExecution CRD creation
- Phase progression state machine
- Status watching for all downstream CRDs
- RBAC permissions

### **P1 - Resilience** (SHOULD HAVE - 2 days)
- Timeout handling per phase
- Failure recovery orchestration
- 24-hour retention finalizer

### **P2 - Observability** (NICE TO HAVE - 1.5 days)
- Database audit integration
- Notification client integration
- Prometheus metrics

---

## 📚 **Reference Documentation**

| Document | Purpose | Used For |
|---|---|---|
| [overview.md](../services/crd-controllers/05-remediationorchestrator/overview.md) | Architecture & scope | Understanding orchestration pattern |
| [controller-implementation.md](../services/crd-controllers/05-remediationorchestrator/controller-implementation.md) | Code patterns | Reference implementation code |
| [reconciliation-phases.md](../services/crd-controllers/05-remediationorchestrator/reconciliation-phases.md) | Phase transitions | State machine logic |
| [implementation-checklist.md](../services/crd-controllers/05-remediationorchestrator/implementation-checklist.md) | Task breakdown | APDC phase mapping |
| [integration-points.md](../services/crd-controllers/05-remediationorchestrator/integration-points.md) | CRD coordination | Watch setup & mapping |
| [finalizers-lifecycle.md](../services/crd-controllers/05-remediationorchestrator/finalizers-lifecycle.md) | Retention logic | 24-hour cleanup |

---

## 📦 **Phase 0: Pre-Implementation Setup** (30 minutes)

### Verification Steps

**Status**: ✅ Already complete (verified in review)

- [x] **Verify cmd/ structure** - `cmd/remediationorchestrator/main.go` exists
- [x] **Verify controller location** - `internal/controller/remediation/remediationrequest_controller.go` exists
- [x] **Verify CRD types** - `api/remediation/v1alpha1/` exists
- [x] **Verify build** - `go build -o bin/remediation-orchestrator ./cmd/remediationorchestrator` works
- [x] **Verify scheme registration** - All CRD schemes registered in `main.go`

**Result**: Infrastructure ready, proceed to Phase 1

---

## 🚀 **Phase 1: Core Orchestration (P0 - 3.5 days)**

### **Goal**: Enable complete multi-CRD orchestration sequence

### Task 1.1: AIAnalysis CRD Creation (0.5 day)

**APDC Phase**: Analysis → Plan → Do → Check  
**Reference**: [controller-implementation.md lines 116-148](../services/crd-controllers/05-remediationorchestrator/controller-implementation.md)

**Subtasks**:

1. **Analysis** (15 min)
   - [ ] Review AIAnalysis CRD schema in `api/aianalysis/v1alpha1/`
   - [ ] Understand data flow from RemediationProcessing.Status → AIAnalysis.Spec
   - [ ] Identify fields to copy (enriched signal data)

2. **Plan** (15 min)
   - [ ] Design `createAIAnalysis` function signature
   - [ ] Plan field mapping from RemediationProcessing.Status.EnrichedSignal
   - [ ] Define helper functions needed

3. **Do-RED** (1 hour)
   - [ ] Write unit test: `TestReconcile_CreateAIAnalysisAfterProcessingCompletes`
   - [ ] Test should verify:
     - RemediationProcessing with phase="completed" exists
     - AIAnalysis CRD created with correct spec
     - Owner reference set to RemediationRequest
     - Status updated with AIAnalysisRef

4. **Do-GREEN** (2 hours)
   - [ ] Implement `createAIAnalysis` function:
     ```go
     func (r *RemediationRequestReconciler) createAIAnalysis(
         ctx context.Context,
         remediation *remediationv1alpha1.RemediationRequest,
         processing *remediationprocessingv1alpha1.RemediationProcessing,
     ) error
     ```
   - [ ] Copy enriched signal data from `processing.Status.EnrichedSignal`
   - [ ] Set owner reference: `ctrl.SetControllerReference`
   - [ ] Create CRD: `r.Create(ctx, analysis)`
   - [ ] Update RemediationRequest.Status.AIAnalysisRef

5. **Do-REFACTOR** (30 min)
   - [ ] Add structured logging with correlation ID
   - [ ] Add error handling with detailed messages
   - [ ] Validate required fields before creation

6. **Check** (15 min)
   - [ ] Run unit test: `go test ./internal/controller/remediation/... -run TestReconcile_CreateAIAnalysisAfterProcessingCompletes`
   - [ ] Verify test passes
   - [ ] Run full test suite: `go test ./internal/controller/remediation/...`

**Acceptance Criteria**:
- ✅ Unit test passes
- ✅ AIAnalysis CRD created when RemediationProcessing.Status.Phase == "completed"
- ✅ Owner reference correctly set
- ✅ RemediationRequest.Status.AIAnalysisRef populated

---

### Task 1.2: WorkflowExecution CRD Creation (0.5 day)

**APDC Phase**: Analysis → Plan → Do → Check  
**Reference**: [controller-implementation.md lines 143-148](../services/crd-controllers/05-remediationorchestrator/controller-implementation.md)

**Subtasks**:

1. **Analysis** (15 min)
   - [ ] Review WorkflowExecution CRD schema in `api/workflowexecution/v1alpha1/`
   - [ ] Understand data flow from AIAnalysis.Status → WorkflowExecution.Spec
   - [ ] Identify fields to copy (recommended workflow)

2. **Plan** (15 min)
   - [ ] Design `createWorkflowExecution` function signature
   - [ ] Plan field mapping from AIAnalysis.Status.RecommendedWorkflow
   - [ ] Define helper functions needed

3. **Do-RED** (1 hour)
   - [ ] Write unit test: `TestReconcile_CreateWorkflowExecutionAfterAIAnalysisCompletes`
   - [ ] Test should verify:
     - AIAnalysis with phase="completed" exists
     - WorkflowExecution CRD created with correct spec
     - Owner reference set to RemediationRequest
     - Status updated with WorkflowExecutionRef

4. **Do-GREEN** (2 hours)
   - [ ] Implement `createWorkflowExecution` function:
     ```go
     func (r *RemediationRequestReconciler) createWorkflowExecution(
         ctx context.Context,
         remediation *remediationv1alpha1.RemediationRequest,
         analysis *aianalysisv1alpha1.AIAnalysis,
     ) error
     ```
   - [ ] Copy recommended workflow from `analysis.Status.RecommendedWorkflow`
   - [ ] Set owner reference: `ctrl.SetControllerReference`
   - [ ] Create CRD: `r.Create(ctx, workflow)`
   - [ ] Update RemediationRequest.Status.WorkflowExecutionRef

5. **Do-REFACTOR** (30 min)
   - [ ] Add structured logging
   - [ ] Add error handling
   - [ ] Validate required fields

6. **Check** (15 min)
   - [ ] Run unit test
   - [ ] Verify test passes
   - [ ] Run full test suite

**Acceptance Criteria**:
- ✅ Unit test passes
- ✅ WorkflowExecution CRD created when AIAnalysis.Status.Phase == "completed"
- ✅ Owner reference correctly set
- ✅ RemediationRequest.Status.WorkflowExecutionRef populated

---

### Task 1.3: Phase Progression State Machine (1 day)

**APDC Phase**: Analysis → Plan → Do → Check  
**Reference**: [reconciliation-phases.md](../services/crd-controllers/05-remediationorchestrator/reconciliation-phases.md)

**Subtasks**:

1. **Analysis** (30 min)
   - [ ] Review current `Reconcile` function implementation
   - [ ] Review phase transition diagram in reconciliation-phases.md
   - [ ] Identify state machine logic needed

2. **Plan** (30 min)
   - [ ] Design state machine flow:
     ```
     pending → processing → analyzing → executing → completed
     ```
   - [ ] Plan `orchestratePhase` function refactor
   - [ ] Define phase transition conditions

3. **Do-RED** (2 hours)
   - [ ] Write unit test: `TestReconcile_PhaseProgression`
   - [ ] Test sequential phases:
     - pending → processing (RemediationProcessing created)
     - processing → analyzing (AIAnalysis created after RemediationProcessing completes)
     - analyzing → executing (WorkflowExecution created after AIAnalysis completes)
     - executing → completed (status updated after WorkflowExecution completes)

4. **Do-GREEN** (3 hours)
   - [ ] Refactor `Reconcile` function to implement state machine
   - [ ] Implement `orchestratePhase` function (reference: controller-implementation.md lines 83-256)
   - [ ] Add phase transition logic:
     ```go
     func (r *RemediationRequestReconciler) orchestratePhase(
         ctx context.Context,
         remediation *remediationv1alpha1.RemediationRequest,
     ) (ctrl.Result, error) {
         switch remediation.Status.OverallPhase {
         case "pending":
             // Create RemediationProcessing (already implemented)
         case "processing":
             // Wait for RemediationProcessing → Create AIAnalysis
         case "analyzing":
             // Wait for AIAnalysis → Create WorkflowExecution
         case "executing":
             // Wait for WorkflowExecution → Update to completed
         }
     }
     ```

5. **Do-REFACTOR** (1 hour)
   - [ ] Add detailed logging for each phase transition
   - [ ] Add error handling for each phase
   - [ ] Emit Kubernetes events on phase transitions

6. **Check** (30 min)
   - [ ] Run unit test
   - [ ] Verify all phases transition correctly
   - [ ] Run full test suite

**Acceptance Criteria**:
- ✅ State machine correctly progresses through all phases
- ✅ Each phase creates correct downstream CRD
- ✅ Status updated on each transition
- ✅ Kubernetes events emitted

---

### Task 1.4: Enhanced SetupWithManager (0.5 day)

**APDC Phase**: Analysis → Plan → Do → Check  
**Reference**: [integration-points.md lines 293-326](../services/crd-controllers/05-remediationorchestrator/integration-points.md)

**Subtasks**:

1. **Analysis** (15 min)
   - [ ] Review current `SetupWithManager` implementation
   - [ ] Review watch patterns in integration-points.md
   - [ ] Identify missing `.Owns()` calls

2. **Plan** (15 min)
   - [ ] Plan to add `.Owns()` for AIAnalysis, WorkflowExecution, KubernetesExecution
   - [ ] Verify watch triggers work correctly

3. **Do-RED** (1 hour)
   - [ ] Write integration test: `TestSetupWithManager_WatchesAllDownstreamCRDs`
   - [ ] Test should verify:
     - Controller watches RemediationProcessing
     - Controller watches AIAnalysis
     - Controller watches WorkflowExecution
     - Controller watches KubernetesExecution

4. **Do-GREEN** (1 hour)
   - [ ] Update `SetupWithManager`:
     ```go
     func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
         return ctrl.NewControllerManagedBy(mgr).
             For(&remediationv1alpha1.RemediationRequest{}).
             Owns(&remediationprocessingv1alpha1.RemediationProcessing{}). // Current
             Owns(&aianalysisv1alpha1.AIAnalysis{}).                       // Add
             Owns(&workflowexecutionv1alpha1.WorkflowExecution{}).         // Add
             Owns(&kubernetesexecutionv1alpha1.KubernetesExecution{}).     // Add
             Named("remediation-remediationrequest").
             Complete(r)
     }
     ```

5. **Check** (30 min)
   - [ ] Run integration test
   - [ ] Verify watch triggers work
   - [ ] Test status change → reconciliation trigger latency (<100ms expected)

**Acceptance Criteria**:
- ✅ All downstream CRDs watched
- ✅ Status changes trigger reconciliation
- ✅ Watch latency <100ms

---

### Task 1.5: RBAC Permissions (0.5 day)

**APDC Phase**: Analysis → Plan → Do → Check  
**Reference**: [security-configuration.md lines 47-85](../services/crd-controllers/05-remediationorchestrator/security-configuration.md)

**Subtasks**:

1. **Analysis** (15 min)
   - [ ] Review current RBAC markers in controller file
   - [ ] Identify missing permissions for downstream CRDs

2. **Plan** (15 min)
   - [ ] List required RBAC verbs for each CRD (get, list, watch, create, update, patch, delete)
   - [ ] Plan kubebuilder marker additions

3. **Do** (1 hour)
   - [ ] Add RBAC markers to controller file:
     ```go
     // +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
     // +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/status,verbs=get;update;patch
     // +kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
     // +kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions/status,verbs=get;update;patch
     // +kubebuilder:rbac:groups=kubernetesexecution.kubernaut.io,resources=kubernetesexecutions,verbs=get;list;watch;create;update;patch;delete
     // +kubebuilder:rbac:groups=kubernetesexecution.kubernaut.io,resources=kubernetesexecutions/status,verbs=get;update;patch
     ```

4. **Check** (1 hour)
   - [ ] Run `make manifests` to regenerate ClusterRole
   - [ ] Verify `config/rbac/role.yaml` includes new permissions
   - [ ] Test controller deployment with new RBAC

**Acceptance Criteria**:
- ✅ RBAC markers added for all downstream CRDs
- ✅ ClusterRole regenerated with correct permissions
- ✅ Controller can create/watch all downstream CRDs

---

### Task 1.6: Status Updates (0.5 day)

**APDC Phase**: Analysis → Plan → Do → Check  
**Reference**: [controller-implementation.md lines 63-69, 95-96, 119-120](../services/crd-controllers/05-remediationorchestrator/controller-implementation.md)

**Subtasks**:

1. **Analysis** (15 min)
   - [ ] Review RemediationRequest status fields in CRD schema
   - [ ] Identify which fields need updates on each phase

2. **Plan** (15 min)
   - [ ] Plan status update logic for each phase transition
   - [ ] Design helper function for status updates

3. **Do-RED** (1 hour)
   - [ ] Write unit test: `TestReconcile_StatusUpdates`
   - [ ] Test should verify:
     - OverallPhase updated on each transition
     - Child CRD references populated
     - Timestamps tracked (StartTime, CompletionTime)

4. **Do-GREEN** (1 hour)
   - [ ] Update status on each phase transition:
     ```go
     remediation.Status.OverallPhase = "analyzing"
     remediation.Status.AIAnalysisRef = &corev1.ObjectReference{
         Name:      analysis.Name,
         Namespace: analysis.Namespace,
     }
     if err := r.Status().Update(ctx, remediation); err != nil {
         return err
     }
     ```

5. **Check** (30 min)
   - [ ] Run unit test
   - [ ] Verify status correctly updated
   - [ ] Check status persists across reconciliation loops

**Acceptance Criteria**:
- ✅ OverallPhase updated on each transition
- ✅ Child CRD references populated correctly
- ✅ Timestamps tracked

---

## 🛡️ **Phase 2: Resilience (P1 - 2 days)**

### **Goal**: Add timeout handling and failure recovery

### Task 2.1: Timeout Handling (0.5 day)

**APDC Phase**: Analysis → Plan → Do → Check  
**Reference**: [controller-implementation.md lines 110-112, 137-139](../services/crd-controllers/05-remediationorchestrator/controller-implementation.md)

**Subtasks**:

1. **Analysis** (15 min)
   - [ ] Review timeout configuration in RemediationRequest spec
   - [ ] Review timeout thresholds in reconciliation-phases.md

2. **Plan** (15 min)
   - [ ] Design `isPhaseTimedOut` helper function
   - [ ] Design `handleTimeout` function

3. **Do-RED** (1 hour)
   - [ ] Write unit test: `TestReconcile_TimeoutDetection`
   - [ ] Test should verify timeout for each phase

4. **Do-GREEN** (2 hours)
   - [ ] Implement timeout detection:
     ```go
     func (r *RemediationRequestReconciler) isPhaseTimedOut(
         crdObj client.Object,
         timeoutConfig *TimeoutConfig,
     ) bool
     ```
   - [ ] Implement timeout handling:
     ```go
     func (r *RemediationRequestReconciler) handleTimeout(
         ctx context.Context,
         remediation *remediationv1alpha1.RemediationRequest,
         phase string,
     ) (ctrl.Result, error)
     ```

5. **Check** (30 min)
   - [ ] Run unit test
   - [ ] Verify timeout detection works

**Acceptance Criteria**:
- ✅ Timeout detected for each phase
- ✅ RemediationRequest status updated to "timeout"
- ✅ Kubernetes event emitted

---

### Task 2.2: Failure Recovery (1 day)

**APDC Phase**: Analysis → Plan → Do → Check  
**Reference**: [controller-implementation.md lines 196-285](../services/crd-controllers/05-remediationorchestrator/controller-implementation.md)

**Subtasks**:

1. **Analysis** (30 min)
   - [ ] Review DD-001 (Recovery Context Enrichment)
   - [ ] Understand failure recovery orchestration pattern

2. **Plan** (30 min)
   - [ ] Design `handleFailure` function
   - [ ] Plan Context API integration (if needed)

3. **Do-RED** (2 hours)
   - [ ] Write unit test: `TestReconcile_FailureRecovery`
   - [ ] Test WorkflowExecution failure → new AIAnalysis created

4. **Do-GREEN** (3 hours)
   - [ ] Implement failure detection
   - [ ] Implement recovery orchestration:
     ```go
     func (r *RemediationRequestReconciler) handleFailure(
         ctx context.Context,
         remediation *remediationv1alpha1.RemediationRequest,
         phase string,
         reason string,
     ) (ctrl.Result, error)
     ```

5. **Check** (30 min)
   - [ ] Run unit test
   - [ ] Verify recovery logic works

**Acceptance Criteria**:
- ✅ Failure detected for each phase
- ✅ Recovery AIAnalysis created (if applicable)
- ✅ Status updated to "recovering" or "failed"

---

### Task 2.3: 24-Hour Retention Finalizer (0.5 day)

**APDC Phase**: Analysis → Plan → Do → Check  
**Reference**: [finalizers-lifecycle.md lines 1-145](../services/crd-controllers/05-remediationorchestrator/finalizers-lifecycle.md)

**Subtasks**:

1. **Analysis** (15 min)
   - [ ] Review finalizer pattern in finalizers-lifecycle.md
   - [ ] Understand 24-hour retention requirement

2. **Plan** (15 min)
   - [ ] Design finalizer logic
   - [ ] Plan cleanup function

3. **Do-RED** (1 hour)
   - [ ] Write unit test: `TestReconcile_24HourRetention`
   - [ ] Test finalizer prevents early deletion

4. **Do-GREEN** (2 hours)
   - [ ] Implement finalizer logic (reference: controller-implementation.md lines 39-60)
   - [ ] Add retention expiry check
   - [ ] Implement cleanup function

5. **Check** (30 min)
   - [ ] Run unit test
   - [ ] Verify 24-hour retention works

**Acceptance Criteria**:
- ✅ Finalizer added on creation
- ✅ CRD retained for 24 hours after completion
- ✅ Cleanup executed after 24 hours

---

## 📊 **Phase 3: Observability (P2 - 1.5 days)**

### **Goal**: Add audit trail and notifications

### Task 3.1: Database Audit Integration (0.5 day)

**APDC Phase**: Analysis → Plan → Do → Check  
**Reference**: [database-integration.md lines 1-587](../services/crd-controllers/05-remediationorchestrator/database-integration.md)

**Subtasks**:

1. **Analysis** (15 min)
   - [ ] Review audit schema in database-integration.md
   - [ ] Understand audit record structure

2. **Plan** (15 min)
   - [ ] Design `publishAuditRecord` function
   - [ ] Plan HTTP client for Storage Service

3. **Do-RED** (1 hour)
   - [ ] Write unit test with mock Storage client

4. **Do-GREEN** (2 hours)
   - [ ] Implement audit publishing on phase transitions

5. **Check** (30 min)
   - [ ] Run unit test
   - [ ] Verify audit records published

**Acceptance Criteria**:
- ✅ Audit records published on phase transitions
- ✅ HTTP client handles failures gracefully

---

### Task 3.2: Notification Client Integration (0.5 day)

**APDC Phase**: Analysis → Plan → Do → Check  
**Reference**: [controller-implementation.md lines 28-31](../services/crd-controllers/05-remediationorchestrator/controller-implementation.md)

**Subtasks**:

1. **Analysis** (15 min)
   - [ ] Review notification requirements
   - [ ] Understand notification channels

2. **Plan** (15 min)
   - [ ] Design `sendNotification` function
   - [ ] Plan HTTP client for Notification Service

3. **Do-RED** (1 hour)
   - [ ] Write unit test with mock Notification client

4. **Do-GREEN** (2 hours)
   - [ ] Implement notification sending on key events

5. **Check** (30 min)
   - [ ] Run unit test
   - [ ] Verify notifications sent

**Acceptance Criteria**:
- ✅ Notifications sent on timeout/failure
- ✅ HTTP client handles failures gracefully

---

### Task 3.3: Prometheus Metrics (0.5 day)

**APDC Phase**: Analysis → Plan → Do → Check  
**Reference**: [metrics-slos.md](../services/crd-controllers/05-remediationorchestrator/metrics-slos.md)

**Subtasks**:

1. **Analysis** (15 min)
   - [ ] Review metrics in metrics-slos.md
   - [ ] Identify key metrics to track

2. **Plan** (15 min)
   - [ ] Design Prometheus metric definitions
   - [ ] Plan metric collection points

3. **Do** (2 hours)
   - [ ] Implement metrics:
     - `remediation_phase_duration_seconds` (histogram)
     - `remediation_phase_transitions_total` (counter)
     - `remediation_failures_total` (counter)
     - `remediation_timeouts_total` (counter)

4. **Check** (30 min)
   - [ ] Verify metrics exposed at `/metrics`
   - [ ] Test metric collection

**Acceptance Criteria**:
- ✅ Metrics exposed at `/metrics` endpoint
- ✅ Metrics track key orchestration events

---

## 🧪 **Phase 4: Testing (3.5 days)**

### Task 4.1: Unit Tests (2 days)

**Reference**: [testing-strategy.md](../services/crd-controllers/05-remediationorchestrator/testing-strategy.md)

**Test Suites** (10 suites):

1. [ ] `TestReconcile_CreateRemediationProcessing` (existing - verify)
2. [ ] `TestReconcile_CreateAIAnalysisAfterProcessingCompletes`
3. [ ] `TestReconcile_CreateWorkflowExecutionAfterAIAnalysisCompletes`
4. [ ] `TestReconcile_PhaseProgression`
5. [ ] `TestReconcile_StatusUpdates`
6. [ ] `TestReconcile_TimeoutDetection`
7. [ ] `TestReconcile_FailureRecovery`
8. [ ] `TestReconcile_24HourRetention`
9. [ ] `TestSetupWithManager_WatchesAllDownstreamCRDs`
10. [ ] `TestHelperFunctions` (field mapping, deep copy, etc.)

**Coverage Target**: 80%+ for orchestration logic

---

### Task 4.2: Integration Tests (1.5 days)

**Reference**: [testing-strategy.md integration section](../services/crd-controllers/05-remediationorchestrator/testing-strategy.md)

**Test Scenarios** (5 scenarios):

1. [ ] Full orchestration flow with real CRD watches
   - Create RemediationRequest
   - Mock downstream controllers marking status="completed"
   - Verify sequential CRD creation
   - Verify final status="completed"

2. [ ] Timeout scenarios
   - RemediationProcessing doesn't complete within timeout
   - Verify timeout detection and status update

3. [ ] Failure recovery scenarios
   - WorkflowExecution fails
   - Verify recovery AIAnalysis created

4. [ ] 24-hour retention lifecycle
   - CRD completes
   - Verify finalizer prevents deletion
   - Fast-forward 24 hours
   - Verify CRD deleted

5. [ ] Watch-based coordination
   - Status change → reconciliation trigger
   - Verify latency <100ms

**Location**: `test/integration/remediationorchestrator/`

---

## 📈 **Success Metrics**

### Functional Metrics
- ✅ All P0 features implemented and tested
- ✅ Unit test coverage ≥80%
- ✅ All integration tests passing
- ✅ Watch latency <100ms
- ✅ No RBAC permission errors

### Quality Metrics
- ✅ No linter errors
- ✅ All error paths handled
- ✅ Structured logging throughout
- ✅ Kubernetes events emitted on transitions
- ✅ Prometheus metrics exposed

### Documentation Metrics
- ✅ Code comments reference design decisions
- ✅ Function signatures match documentation
- ✅ APDC methodology followed

---

## 🚨 **Risks & Mitigation**

### High Risks

1. **CRD Schema Mismatches**
   - **Risk**: Field names don't match between CRDs
   - **Mitigation**: Verify all field mappings during Analysis phase
   - **Detection**: Unit tests will catch schema mismatches

2. **Watch Configuration Errors**
   - **Risk**: Watches don't trigger reconciliation
   - **Mitigation**: Integration tests verify watch triggers
   - **Detection**: Latency tests (<100ms expected)

3. **Timeout Configuration**
   - **Risk**: Incorrect timeout thresholds
   - **Mitigation**: Use defaults from reconciliation-phases.md
   - **Detection**: Integration tests with timeout scenarios

### Medium Risks

1. **Missing RBAC Permissions**
   - **Risk**: Controller can't create downstream CRDs
   - **Mitigation**: Regenerate manifests after adding RBAC markers
   - **Detection**: Integration tests in cluster

2. **Finalizer Logic Errors**
   - **Risk**: CRDs deleted too early or never deleted
   - **Mitigation**: Unit tests for finalizer logic
   - **Detection**: 24-hour retention integration test

---

## 📅 **Timeline**

### Week 1: Core Orchestration (P0)
- **Day 1**: Task 1.1 (AIAnalysis creation) + Task 1.2 (WorkflowExecution creation)
- **Day 2**: Task 1.3 (Phase progression state machine)
- **Day 3**: Task 1.4 (SetupWithManager) + Task 1.5 (RBAC) + Task 1.6 (Status updates)

### Week 2: Resilience + Observability (P1 + P2)
- **Day 4**: Task 2.1 (Timeout handling) + Task 2.2 (Failure recovery)
- **Day 5**: Task 2.3 (24-hour retention) + Task 3.1 (Database audit)
- **Day 6**: Task 3.2 (Notification client) + Task 3.3 (Prometheus metrics)

### Week 3: Testing
- **Day 7-8**: Task 4.1 (Unit tests - 10 suites)
- **Day 9-10**: Task 4.2 (Integration tests - 5 scenarios)

**Total Duration**: 10.5 days (2 weeks + 0.5 week)

---

## ✅ **Definition of Done**

### Code
- [ ] All tasks in Phase 1 (P0) completed and tested
- [ ] All tasks in Phase 2 (P1) completed and tested
- [ ] All tasks in Phase 3 (P2) completed and tested
- [ ] Unit test coverage ≥80%
- [ ] All integration tests passing
- [ ] No linter errors (`golangci-lint run`)
- [ ] No RBAC permission errors

### Documentation
- [ ] Code comments reference design documents
- [ ] Function signatures match documentation patterns
- [ ] APDC methodology followed for all tasks
- [ ] Update `docs/analysis/RR_CONTROLLER_IMPLEMENTATION_REVIEW.md` with completion status

### Deployment
- [ ] RBAC manifests regenerated (`make manifests`)
- [ ] Controller builds successfully
- [ ] Controller deploys to cluster
- [ ] Health checks pass (`/healthz`, `/readyz`)
- [ ] Metrics exposed (`/metrics`)

---

## 🎯 **Approval Request**

This action plan implements the complete RemediationRequest orchestrator per the documentation in `docs/services/crd-controllers/05-remediationorchestrator/`.

**Scope**:
- ✅ **Phase 1 (P0)**: Core orchestration - 3.5 days
- ✅ **Phase 2 (P1)**: Resilience - 2 days
- ✅ **Phase 3 (P2)**: Observability - 1.5 days
- ✅ **Phase 4**: Testing - 3.5 days

**Total Effort**: 10.5 days

**Key Deliverables**:
1. Complete multi-CRD orchestration (AIAnalysis, WorkflowExecution)
2. Phase progression state machine
3. Timeout handling and failure recovery
4. 24-hour retention with finalizer
5. Database audit and notification integration
6. Comprehensive test suite (unit + integration)

**Confidence**: 85%
- ✅ Documentation is comprehensive
- ✅ Architecture is well-defined
- ✅ Current implementation provides foundation
- ⚠️ Some CRD schema fields may need verification during implementation

---

**Ready to proceed?** Please approve this plan to begin Phase 1 implementation.

