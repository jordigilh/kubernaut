# BR-WE-006: Kubernetes Conditions for Observability

**Status**: ✅ Approved
**Priority**: P0 (CRITICAL)
**Version**: 1.0
**Date**: 2025-12-11
**Target**: V4.2

---

## Business Requirement

**Description**: WorkflowExecution MUST implement Kubernetes Conditions to provide operators with detailed status visibility through native Kubernetes tooling (`kubectl describe`).

**Category**: Observability & Operations

**Justification**:
- CRD schema includes `Conditions []metav1.Condition` field (line 173-174) but field is never populated
- Operators cannot see Tekton pipeline execution state without querying PipelineRun directly
- Violates Kubernetes API conventions for status reporting
- Contract with RemediationOrchestrator (DD-CONTRACT-001) expects rich status information

---

## Success Criteria

### Functional Requirements

1. **Five Condition Types Implemented**:
   - `TektonPipelineCreated` - Tracks PipelineRun creation
   - `TektonPipelineRunning` - Tracks pipeline execution state
   - `TektonPipelineComplete` - Tracks completion (success/failure)
   - `AuditRecorded` - Tracks BR-WE-005 audit event persistence
   - `ResourceLocked` - Tracks resource locking (DD-WE-001/003)

2. **Lifecycle Coverage**:
   - All 5 CRD phases represented: Pending, Running, Completed, Failed, Skipped
   - Conditions updated on every phase transition
   - Failure reasons map to CRD FailureReason constants

3. **Kubernetes API Compliance**:
   - `type`, `status`, `reason`, `message`, `lastTransitionTime` fields populated
   - Positive polarity (conditions express desired state)
   - Machine-readable reasons (CamelCase)
   - Human-readable messages

### Operational Requirements

1. **Visibility**:
   ```bash
   $ kubectl describe workflowexecution wfe-123
   Status:
     Phase: Running
     Conditions:
       Type:     TektonPipelineCreated
       Status:   True
       Reason:   PipelineCreated
       Message:  PipelineRun workflow-exec-abc123 created in kubernaut-workflows namespace

       Type:     TektonPipelineRunning
       Status:   True
       Reason:   PipelineStarted
       Message:  Pipeline executing task 2 of 5 (apply-memory-increase)
   ```

2. **Query Support**:
   ```bash
   $ kubectl get wfe -o json | jq '.items[].status.conditions[] | select(.type=="TektonPipelineComplete")'
   ```

3. **Observability Integration**:
   - Prometheus metrics based on condition states
   - Alert rules for stuck pipelines (TektonPipelineRunning > 30m)
   - Grafana dashboards showing condition history

### Performance Requirements

1. **Condition Update Latency**: < 5 seconds after phase transition
2. **Storage Overhead**: < 2KB per WorkflowExecution status
3. **API Server Load**: No additional GET calls (conditions updated during reconciliation)

---

## Detailed Condition Specifications

### Condition 1: TektonPipelineCreated

**Type**: `TektonPipelineCreated`
**Phase**: Pending → Running
**Authority**: CRD Phase validation (line 103)

**Status Values**:
- `True`: PipelineRun created successfully
- `False`: Creation failed (quota, RBAC, image pull)

**Reasons**:
- Success: `PipelineCreated`
- Failure: `QuotaExceeded`, `RBACError`, `ImagePullBackOff`

**Messages**:
- Success: "PipelineRun {name} created in {namespace}"
- Failure: Kubernetes error message

---

### Condition 2: TektonPipelineRunning

**Type**: `TektonPipelineRunning`
**Phase**: Running
**Authority**: PhaseRunning (line 346)

**Status Values**:
- `True`: Pipeline executing
- `False`: Pipeline failed to start

**Reasons**:
- Success: `PipelineStarted`
- Failure: `PipelineFailedToStart`

**Messages**:
- Success: "Pipeline executing task {current} of {total} ({task-name})"
- Failure: Reason pipeline couldn't start

---

### Condition 3: TektonPipelineComplete

**Type**: `TektonPipelineComplete`
**Phase**: Completed OR Failed
**Authority**: PhaseCompleted/PhaseFailed (lines 348-351)

**Status Values**:
- `True`: Pipeline succeeded
- `False`: Pipeline failed

**Reasons**:
- Success: `PipelineSucceeded`
- Failure: Maps to FailureReason constants (lines 385-410):
  - `TaskFailed`
  - `DeadlineExceeded`
  - `OOMKilled`
  - `Forbidden`
  - `ResourceExhausted`

**Messages**:
- Success: "All {n} tasks completed successfully in {duration}"
- Failure: Task failure details

---

### Condition 4: AuditRecorded

**Type**: `AuditRecorded`
**Phase**: All (cross-cutting)
**Authority**: BR-WE-005 (lines 171-191)

**Status Values**:
- `True`: Audit event persisted to DataStorage
- `False`: Audit write failed

**Reasons**:
- Success: `AuditSucceeded`
- Failure: `AuditFailed`

**Messages**:
- Success: "Audit event {type} recorded to DataStorage"
- Failure: DataStorage error message

**Events Tracked**:
- `workflowexecution.workflow.started`
- `workflowexecution.workflow.completed`
- `workflowexecution.workflow.failed`
- `workflowexecution.workflow.skipped`

---

### Condition 5: ResourceLocked

**Type**: `ResourceLocked`
**Phase**: Skipped
**Authority**: DD-WE-001/003, PhaseSkipped (line 355)

**Status Values**:
- `True`: Target resource locked, execution skipped
- `False`: Resource available (not applicable when absent)

**Reasons**:
- `TargetResourceBusy`: Another workflow running on target
- `RecentlyRemediated`: Cooldown period active
- Maps to SkipReason constants (lines 360-382)

**Messages**:
- Busy: "Another workflow ({name}) is currently executing on target {resource}"
- Cooldown: "Target resource {resource} recently remediated, cooldown until {time}"

---

## Integration Points

### Controller Updates

**Files to Modify**:
1. `pkg/workflowexecution/conditions.go` (NEW)
2. `internal/controller/workflowexecution/workflowexecution_controller.go`

**Integration Points**:
1. After PipelineRun creation (Reconcile)
2. During PipelineRun status sync (syncPipelineRunStatus)
3. After audit event emission (emitAudit)
4. During resource lock check (checkResourceLock)

### Dependencies

- ✅ CRD Schema: Already has Conditions field (line 173-174)
- ✅ FailureReason constants: Defined (lines 385-410)
- ✅ SkipReason constants: Defined (lines 360-382)
- ✅ Reference Implementation: AIAnalysis `pkg/aianalysis/conditions.go`

---

## Non-Functional Requirements

### Backward Compatibility

- ✅ **Non-Breaking**: Conditions field is additive (optional)
- ✅ **Existing WFEs**: Empty conditions array (valid state)
- ✅ **API Clients**: No changes required to read Phase field

### Performance

- **Condition Updates**: O(1) operation using `meta.SetStatusCondition`
- **Storage**: ~150 bytes per condition × 5 = 750 bytes overhead
- **Network**: Included in existing status update (no extra API calls)

### Reliability

- **Idempotency**: Condition updates are idempotent
- **Race Conditions**: Protected by optimistic locking (resourceVersion)
- **Failure Handling**: Condition update failures don't block reconciliation

---

## Testing Requirements

### Unit Tests

**File**: `test/unit/workflowexecution/conditions_test.go`

**Coverage**:
- Each condition type (Set/Get/Check functions)
- Reason/message population
- Transition history
- Edge cases (nil conditions, missing types)

**Target**: 100% coverage of conditions infrastructure

---

### Integration Tests

**File**: `test/integration/workflowexecution/conditions_integration_test.go`

**Scenarios**:
1. **Happy Path**: All conditions True through successful execution
2. **Pipeline Creation Failure**: TektonPipelineCreated=False
3. **Pipeline Execution Failure**: TektonPipelineComplete=False
4. **Audit Failure**: AuditRecorded=False
5. **Resource Locked**: ResourceLocked=True, Phase=Skipped

**Target**: 70%+ integration coverage

---

### E2E Tests

**File**: `test/e2e/workflowexecution/03_conditions_test.go`

**Scenarios**:
1. Full lifecycle conditions in real Kind cluster
2. Operator visibility via `kubectl describe`
3. Prometheus metrics based on conditions

**Target**: 10-15% E2E coverage

---

## Implementation Phases

### Phase 1: Infrastructure (1.5 hours)
- Create `pkg/workflowexecution/conditions.go`
- Define constants and helper functions
- Unit tests

### Phase 2: Controller Integration (2 hours)
- Update reconciliation logic
- Set conditions at integration points
- Integration tests

### Phase 3: Validation (1 hour)
- Run `make generate`
- Full test suite
- Manual `kubectl describe` validation

**Total Effort**: 4-5 hours
**Target Completion**: 2025-12-13

---

## Acceptance Criteria

### Must Have (V4.2)

- [x] All 5 conditions implemented
- [x] Conditions visible in `kubectl describe workflowexecution`
- [x] Unit tests passing (100% coverage of conditions.go)
- [x] Integration tests passing (70%+ coverage)
- [x] Documentation updated

### Should Have (V4.3)

- [ ] E2E tests for conditions (10-15% coverage)
- [ ] Prometheus metrics based on conditions
- [ ] Grafana dashboard showing condition history

### Could Have (V5.0)

- [ ] Condition-based alerting rules
- [ ] Automated remediation based on condition patterns
- [ ] Condition analytics (most common failure reasons)

---

## Related Requirements

- **BR-WE-005**: Audit Events for Execution Lifecycle (P0)
- **DD-CONTRACT-001**: AIAnalysis ↔ WorkflowExecution Contract (v1.4)
- **DD-WE-001**: Resource Locking Safety
- **DD-WE-003**: Resource Lock Persistence
- **DD-WE-004**: Exponential Backoff Cooldown

---

## References

- Kubernetes API Conventions: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
- AIAnalysis Conditions Implementation: `docs/handoff/AIANALYSIS_CONDITIONS_IMPLEMENTATION_STATUS.md`
- WorkflowExecution CRD Schema: `api/workflowexecution/v1alpha1/workflowexecution_types.go`

---

**Document Status**: ✅ Approved
**Created**: 2025-12-11
**Priority**: P0 (CRITICAL)
**Target**: V4.2 (2025-12-13)







