# Remediation Orchestrator Unit Test Coverage Scope

**Objective**: Recover the 0.9% coverage drop in the Remediation Orchestrator package by adding targeted unit tests for under-covered functions.

**Related**: Coverage analysis from CI run (e.g., CI 21919887710)

---

## 1. Executive Summary

| Metric | Value |
|--------|-------|
| **Target functions** | 6 functions across 3 files |
| **Estimated new test lines** | ~420–480 lines |
| **Estimated coverage improvement** | 0.9–1.2% (package-level) |
| **Shared test infrastructure** | Yes – fake client, scheme, `helpers.NewRemediationRequest`, `helpers.NewCompletedAIAnalysis` |
| **Priority** | `resolveTargetResource` (28.6%) → `CreateCompletionNotification` (77.4%) → `populateManualReviewContext` (66.7%) → others |

---

## 2. Function-by-Function Analysis

### 2.1 CreateCompletionNotification (notification.go L258, 77.4%)

**Source**: `pkg/remediationorchestrator/creator/notification.go`

**Uncovered code paths**:
- **Client Get non-NotFound error** (L281–285): When checking for existing notification, non-`NotFound` errors are returned.
- **Client Create failure** (L348–352): Simulated via client interceptor.
- **RootCauseAnalysis.Summary branch** (L285–288): Uses `RCA.Summary` when present instead of `ai.Status.RootCause`.
- **SelectedWorkflow nil** (L291–296): `workflowID` and `executionEngine` empty when nil.

**Test scenarios**:

| # | Scenario | Lines Est. | File |
|---|----------|------------|------|
| 1 | Client Get returns non-NotFound error → returns error | ~15 | `notification_creator_test.go` |
| 2 | Client Create fails (interceptor) → returns error | ~20 | `notification_creator_test.go` |
| 3 | AI with RootCauseAnalysis.Summary in body | ~18 | `notification_creator_test.go` |
| 4 | AI with nil SelectedWorkflow → body has empty workflowID/executionEngine | ~15 | `notification_creator_test.go` |

**Total for CreateCompletionNotification**: ~68 lines

---

### 2.2 CreateBulkDuplicateNotification (notification.go L399, 63.6%)

**Uncovered code paths**:
- **Client Get non-NotFound error** (L410–414).
- **Client Create failure** (L361–365).
- **Empty UID** (L352–356).
- **Get existing (idempotency)** – likely covered; verify.

**Test scenarios**:

| # | Scenario | Lines Est. | File |
|---|----------|------------|------|
| 1 | Client Get returns non-NotFound error → returns error | ~15 | `notification_creator_test.go` |
| 2 | Client Create fails → returns error | ~18 | `notification_creator_test.go` |
| 3 | Empty UID → returns error | ~12 | `notification_creator_test.go` |

**Total for CreateBulkDuplicateNotification**: ~45 lines

---

### 2.3 CreateApprovalNotification (notification.go L73, 73.3%)

**Uncovered code paths**:
- **Client Get non-NotFound error** (L104–107).
- **Client Create failure** (L165–169).
- **Empty UID** (L154–157) – approval does not yet test this.
- **“Already exists” path** (L99–103) – verify coverage.

**Test scenarios**:

| # | Scenario | Lines Est. | File |
|---|----------|------------|------|
| 1 | Client Get returns non-NotFound error → returns error | ~15 | `notification_creator_test.go` |
| 2 | Client Create fails → returns error | ~18 | `notification_creator_test.go` |
| 3 | Empty UID → returns error | ~12 | `notification_creator_test.go` |

**Total for CreateApprovalNotification**: ~45 lines

---

### 2.4 buildApprovalBody (notification.go L214, 71.4%)

**Indirect coverage via CreateApprovalNotification.**

**Uncovered branches**:
- **RootCauseAnalysis.Summary** (L220–222): Use `RCA.Summary` when present.
- **ApprovalContext.Reason** (L225–227): Use `ApprovalContext.Reason` when present.
- **Cluster-scoped resource** (L41–45): `formatTargetResource` with empty `Namespace`.

**Test scenarios**:

| # | Scenario | Lines Est. | File |
|---|----------|------------|------|
| 1 | AI with RootCauseAnalysis.Summary in approval body | ~18 | `notification_creator_test.go` |
| 2 | AI with ApprovalContext.Reason in approval body | ~18 | `notification_creator_test.go` |
| 3 | RR with cluster-scoped target (Node) in approval body | ~15 | `notification_creator_test.go` |

**Total for buildApprovalBody**: ~51 lines

---

### 2.5 buildManualReviewBody (notification.go L746, 85.0%)

**Uncovered branches**:
- **SubReason non-empty** (L599–601).
- **Message non-empty** (L603–605).
- **RootCauseAnalysis non-empty** (L607–609).
- **Warnings slice** (L611–616).
- **WorkflowExecution source**: RetryCount/MaxRetries (L621–623), LastExitCode (L624–626), PreviousExecution (L627–629).

**Note**: Many paths may already be hit by existing manual review tests; add focused body-content checks.

**Test scenarios**:

| # | Scenario | Lines Est. | File |
|---|----------|------------|------|
| 1 | Body includes Warnings when present | ~20 | `notification_creator_test.go` |
| 2 | Body includes retry info for WE source | ~22 | `notification_creator_test.go` |
| 3 | Body includes PreviousExecution for WE source | ~18 | `notification_creator_test.go` |

**Total for buildManualReviewBody**: ~60 lines

---

### 2.6 resolveTargetResource (workflowexecution.go L194, 28.6%)

**Highest impact – lowest coverage.** Package-private; exercised via `WorkflowExecutionCreator.Create()`. **Note**: EA target resolution (`resolveEffectivenessTarget` in RO reconciler) now uses the same AffectedResource logic as the WE creator — AI-identified target when available, fallback to RR.Spec.TargetResource.

**Uncovered branches**:
1. **AffectedResource with namespace** → `"namespace/kind/name"`.
2. **AffectedResource cluster-scoped** (Namespace empty) → `"kind/name"`.
3. **AffectedResource with Kind=="" or Name==""** → fallback to RR.
4. **RootCauseAnalysis nil or AffectedResource nil** → fallback (likely covered today).

**Test scenarios**:

| # | Scenario | Lines Est. | File |
|---|----------|------------|------|
| 1 | AI with RootCauseAnalysis.AffectedResource (namespaced) → WE uses `ns/kind/name` | ~25 | `workflowexecution_creator_test.go` |
| 2 | AI with AffectedResource cluster-scoped (Namespace="") → WE uses `kind/name` | ~25 | `workflowexecution_creator_test.go` |
| 3 | AI with AffectedResource Kind or Name empty → fallback to RR target | ~22 | `workflowexecution_creator_test.go` |

**Total for resolveTargetResource**: ~72 lines

---

### 2.7 populateManualReviewContext (aianalysis.go L299, 66.7%)

**Uncovered branches**:
- **RootCauseAnalysis != nil** → use `RCA.Summary` (L301–302).
- **RootCauseAnalysis nil and RootCause != ""** → use `RootCause` (L303–304) – likely covered.
- **Warnings != nil** → populate `reviewCtx.Warnings` (L308–310).

**Test scenarios** (via `HandleAIAnalysisStatus`):

| # | Scenario | Lines Est. | File |
|---|----------|------------|------|
| 1 | WorkflowResolutionFailed with RootCauseAnalysis.Summary in notification | ~22 | `aianalysis_handler_test.go` |
| 2 | WorkflowResolutionFailed with Warnings in notification metadata/body | ~22 | `aianalysis_handler_test.go` |

**Total for populateManualReviewContext**: ~44 lines

---

## 3. Prioritized Test Scenario List (by Impact)

| Rank | Function | Current % | Scenario | Impact |
|------|----------|-----------|----------|--------|
| 1 | resolveTargetResource | 28.6% | AffectedResource namespaced path | High |
| 2 | resolveTargetResource | 28.6% | AffectedResource cluster-scoped path | High |
| 3 | resolveTargetResource | 28.6% | AffectedResource fallback (invalid) | High |
| 4 | CreateCompletionNotification | 77.4% | Get non-NotFound, Create failure | Medium |
| 5 | populateManualReviewContext | 66.7% | RootCauseAnalysis.Summary, Warnings | Medium |
| 6 | CreateBulkDuplicateNotification | 63.6% | Error paths (Get, Create, empty UID) | Medium |
| 7 | CreateApprovalNotification | 73.3% | Error paths (Get, Create, empty UID) | Medium |
| 8 | buildApprovalBody | 71.4% | RCA.Summary, ApprovalContext.Reason | Medium |
| 9 | buildManualReviewBody | 85.0% | Warnings, WE retry/PreviousExecution | Low |

---

## 4. Shared Test Infrastructure

| Component | Reuse | Location |
|-----------|-------|----------|
| **Fake client** | All notification/creator tests | `fake.NewClientBuilder().WithScheme(scheme)` |
| **Scheme** | All tests | `remediationv1`, `notificationv1`, `aianalysisv1`, `workflowexecutionv1` |
| **RemediationRequest** | `helpers.NewRemediationRequest(name, ns, opts)` | `test/shared/helpers/remediation.go` |
| **RemediationRequest cluster-scoped** | `RemediationRequestOpts{TargetKind: "Node", TargetName: "x"}` | Existing |
| **AIAnalysis** | `helpers.NewCompletedAIAnalysis(name, ns)` | `test/shared/helpers/remediation.go` |
| **RootCauseAnalysis** | Set `ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{...}` | Manual in tests |
| **Client interceptor** | `interceptor.Funcs{Get:..., Create:...}` | `workflowexecution_creator_test.go` pattern |

**Helper extension** (optional): add `AIAnalysisOpts` support for `RootCauseAnalysis` and `AffectedResource` in `NewCompletedAIAnalysis` / `NewAIAnalysis` to reduce duplication.

---

## 5. Estimated Totals

| Category | Lines |
|----------|-------|
| CreateCompletionNotification | ~68 |
| CreateBulkDuplicateNotification | ~45 |
| CreateApprovalNotification | ~45 |
| buildApprovalBody | ~51 |
| buildManualReviewBody | ~60 |
| resolveTargetResource | ~72 |
| populateManualReviewContext | ~44 |
| **Total** | **~385** |
| Buffer (Describe/Context, setup) | +35–95 |
| **Grand total** | **~420–480 lines** |

---

## 6. Coverage Improvement Estimate

- **resolveTargetResource**: ~28.6% → ~90%+ (~15 LOC, strong impact).
- **CreateCompletionNotification**: ~77.4% → ~92%+ (~8 LOC).
- **CreateBulkDuplicateNotification**: ~63.6% → ~88%+ (~10 LOC).
- **CreateApprovalNotification**: ~73.3% → ~90%+ (~8 LOC).
- **buildApprovalBody**: ~71.4% → ~95%+ (~12 LOC).
- **buildManualReviewBody**: ~85.0% → ~98%+ (~5 LOC).
- **populateManualReviewContext**: ~66.7% → ~95%+ (~8 LOC).

**Package-level**: ~0.9–1.2% recovery is realistic given the LOC counts and branch structure.

---

## 7. Implementation Order

1. **Phase 1** (highest impact): `resolveTargetResource` tests in `workflowexecution_creator_test.go`.
2. **Phase 2**: Error-path tests (Get/Create/empty UID) for CreateApproval, CreateBulk, CreateCompletion in `notification_creator_test.go`.
3. **Phase 3**: Body and context tests (buildApprovalBody, buildManualReviewBody, populateManualReviewContext).
4. **Phase 4**: Remaining edge cases and verification.

---

## 8. Acceptance Criteria

- [ ] All scenarios in Section 3 implemented.
- [ ] `go test ./test/unit/remediationorchestrator/... -cover` shows ≥0.9% package coverage improvement.
- [ ] No new lint issues.
- [ ] Ginkgo/Gomega style and existing patterns preserved.
- [ ] Business requirements (BR-ORCH-*) referenced where relevant.

---

## 9. References

- `pkg/remediationorchestrator/creator/notification.go`
- `pkg/remediationorchestrator/creator/workflowexecution.go`
- `pkg/remediationorchestrator/handler/aianalysis.go`
- `test/unit/remediationorchestrator/notification_creator_test.go`
- `test/unit/remediationorchestrator/workflowexecution_creator_test.go`
- `test/unit/remediationorchestrator/aianalysis_handler_test.go`
- `test/shared/helpers/remediation.go`
