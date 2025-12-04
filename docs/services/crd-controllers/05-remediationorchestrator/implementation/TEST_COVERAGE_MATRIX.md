# Test Coverage Matrix - Remediation Orchestrator

**Parent Document**: [IMPLEMENTATION_PLAN_V1.1.md](./IMPLEMENTATION_PLAN_V1.1.md)
**Template Source**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE V3.0 §BR Coverage Matrix
**Last Updated**: 2025-12-04

---

## Coverage Summary

| Metric | Target | Status |
|--------|--------|--------|
| **Unit Test Coverage** | 70% | ⏳ Pending |
| **Integration Test Coverage** | <20% | ⏳ Pending |
| **E2E Test Coverage** | <10% | ⏳ Pending |
| **BR Mapping Completeness** | 100% | ✅ Complete |

---

## Per-BR Coverage Breakdown

### BR-ORCH-001: Approval Notification Creation (P0)

**Test File**: `pkg/remediation/orchestrator/controller/reconciler_test.go`

| Test Type | Test Name | Status |
|-----------|-----------|--------|
| Unit | `Describe("Approval Notification (BR-ORCH-001)")` | ⏳ |
| Unit | `It("should create notification when AIAnalysis requires approval")` | ⏳ |
| Unit | `It("should include approval context from AIAnalysis")` | ⏳ |
| Unit | `It("should transition to AwaitingApproval phase")` | ⏳ |
| Integration | `It("should send notification via NotificationRequest CRD")` | ⏳ |

```go
var _ = Describe("Approval Notification (BR-ORCH-001)", func() {
    DescribeTable("approval notification scenarios",
        func(aiAnalysisRequiresApproval bool, expectedPhase string, shouldCreateNotification bool) {
            // Setup AIAnalysis with approval requirement
            ai := &aianalysisv1.AIAnalysis{
                Status: aianalysisv1.AIAnalysisStatus{
                    Phase: "Completed",
                    RequiresApproval: aiAnalysisRequiresApproval,
                    ApprovalContext: &aianalysisv1.ApprovalContext{
                        Reason: "high_risk_action",
                    },
                },
            }

            // Reconcile
            result, err := reconciler.Reconcile(ctx, req)

            // Verify
            Expect(err).ToNot(HaveOccurred())
            Expect(rr.Status.OverallPhase).To(Equal(expectedPhase))

            if shouldCreateNotification {
                nr := &notificationv1.NotificationRequest{}
                err := k8sClient.Get(ctx, client.ObjectKey{
                    Name: fmt.Sprintf("nr-approval-%s", rr.Name),
                    Namespace: rr.Namespace,
                }, nr)
                Expect(err).ToNot(HaveOccurred())
                Expect(nr.Spec.NotificationType).To(Equal("approval_required"))
            }
        },
        Entry("requires approval", true, "AwaitingApproval", true),
        Entry("auto-approved", false, "Executing", false),
    )
})
```

---

### BR-ORCH-025: Workflow Data Pass-Through (P0)

**Test File**: `pkg/remediation/orchestrator/creator/workflowexecution_test.go`

| Test Type | Test Name | Status |
|-----------|-----------|--------|
| Unit | `It("should pass containerImage from AIAnalysis.status.selectedWorkflow")` | ⏳ |
| Unit | `It("should pass containerDigest from AIAnalysis.status.selectedWorkflow")` | ⏳ |
| Unit | `It("should pass parameters from AIAnalysis.status.selectedWorkflow")` | ⏳ |
| Unit | `It("should set targetResource from RemediationRequest.spec")` | ⏳ |
| Integration | `It("should create WorkflowExecution with complete data")` | ⏳ |

```go
var _ = Describe("Workflow Data Pass-Through (BR-ORCH-025)", func() {
    DescribeTable("workflow data mapping",
        func(aiWorkflow *aianalysisv1.SelectedWorkflow, expectedWESpec workflowexecutionv1.WorkflowExecutionSpec) {
            // Setup AIAnalysis with selected workflow
            ai := &aianalysisv1.AIAnalysis{
                Status: aianalysisv1.AIAnalysisStatus{
                    Phase: "Completed",
                    SelectedWorkflow: aiWorkflow,
                },
            }

            // Create WorkflowExecution
            weName, err := creator.Create(ctx, rr)
            Expect(err).ToNot(HaveOccurred())

            // Fetch and verify
            we := &workflowexecutionv1.WorkflowExecution{}
            err = k8sClient.Get(ctx, client.ObjectKey{Name: weName, Namespace: ns}, we)
            Expect(err).ToNot(HaveOccurred())

            Expect(we.Spec.WorkflowRef.WorkflowID).To(Equal(expectedWESpec.WorkflowRef.WorkflowID))
            Expect(we.Spec.WorkflowRef.ContainerImage).To(Equal(expectedWESpec.WorkflowRef.ContainerImage))
            Expect(we.Spec.WorkflowRef.ContainerDigest).To(Equal(expectedWESpec.WorkflowRef.ContainerDigest))
        },
        Entry("complete workflow data",
            &aianalysisv1.SelectedWorkflow{
                WorkflowID:      "scale-deployment",
                ContainerImage:  "ghcr.io/kubernaut/workflow-scale:v1.0",
                ContainerDigest: "sha256:abc123",
                Parameters:      map[string]string{"replicas": "3"},
            },
            workflowexecutionv1.WorkflowExecutionSpec{
                WorkflowRef: workflowexecutionv1.WorkflowRef{
                    WorkflowID:      "scale-deployment",
                    ContainerImage:  "ghcr.io/kubernaut/workflow-scale:v1.0",
                    ContainerDigest: "sha256:abc123",
                },
            },
        ),
    )
})
```

---

### BR-ORCH-026: Approval Orchestration (P0)

**Test File**: `pkg/remediation/orchestrator/controller/reconciler_test.go`

| Test Type | Test Name | Status |
|-----------|-----------|--------|
| Unit | `It("should wait in AwaitingApproval until decision made")` | ⏳ |
| Unit | `It("should proceed to Executing on approval")` | ⏳ |
| Unit | `It("should fail on rejection")` | ⏳ |
| Integration | `It("should coordinate with RemediationApprovalRequest CRD")` | ⏳ |

---

### BR-ORCH-027: Global Timeout Management (P0)

**Test File**: `pkg/remediation/orchestrator/timeout/detector_test.go`

| Test Type | Test Name | Status |
|-----------|-----------|--------|
| Unit | `It("should detect global timeout exceeded")` | ⏳ |
| Unit | `It("should use spec.globalTimeout when provided")` | ⏳ |
| Unit | `It("should use default timeout when not provided")` | ⏳ |
| Integration | `It("should transition to TimedOut and escalate")` | ⏳ |

```go
var _ = Describe("Global Timeout (BR-ORCH-027)", func() {
    DescribeTable("global timeout detection",
        func(creationAge time.Duration, specTimeout *time.Duration, shouldTimeout bool) {
            rr := &remediationv1.RemediationRequest{
                ObjectMeta: metav1.ObjectMeta{
                    CreationTimestamp: metav1.NewTime(time.Now().Add(-creationAge)),
                },
                Spec: remediationv1.RemediationRequestSpec{
                    GlobalTimeout: specTimeout,
                },
            }

            timedOut, duration := detector.CheckGlobalTimeout(rr)

            Expect(timedOut).To(Equal(shouldTimeout))
            if shouldTimeout {
                Expect(duration).To(BeNumerically(">", 0))
            }
        },
        Entry("within default timeout", 30*time.Minute, nil, false),
        Entry("exceeds default timeout", 90*time.Minute, nil, true),
        Entry("within custom timeout", 90*time.Minute, ptr(2*time.Hour), false),
        Entry("exceeds custom timeout", 3*time.Hour, ptr(2*time.Hour), true),
    )
})
```

---

### BR-ORCH-028: Per-Phase Timeout Management (P0)

**Test File**: `pkg/remediation/orchestrator/timeout/detector_test.go`

| Test Type | Test Name | Status |
|-----------|-----------|--------|
| Unit | `It("should detect Processing phase timeout")` | ⏳ |
| Unit | `It("should detect Analyzing phase timeout")` | ⏳ |
| Unit | `It("should detect Executing phase timeout")` | ⏳ |
| Unit | `It("should not timeout in terminal states")` | ⏳ |

```go
var _ = Describe("Per-Phase Timeout (BR-ORCH-028)", func() {
    DescribeTable("phase-specific timeout detection",
        func(phase string, phaseAge time.Duration, expectedTimeout bool) {
            rr := &remediationv1.RemediationRequest{
                Status: remediationv1.RemediationRequestStatus{
                    OverallPhase: phase,
                },
            }

            // Set phase start time based on current phase
            setPhaseStartTime(rr, phase, time.Now().Add(-phaseAge))

            timedOut, timedOutPhase, _ := detector.CheckTimeout(rr)

            Expect(timedOut).To(Equal(expectedTimeout))
            if expectedTimeout {
                Expect(string(timedOutPhase)).To(Equal(phase))
            }
        },
        Entry("Processing within timeout", "Processing", 3*time.Minute, false),
        Entry("Processing exceeded", "Processing", 10*time.Minute, true),
        Entry("Analyzing within timeout", "Analyzing", 5*time.Minute, false),
        Entry("Analyzing exceeded", "Analyzing", 15*time.Minute, true),
        Entry("Executing within timeout", "Executing", 20*time.Minute, false),
        Entry("Executing exceeded", "Executing", 45*time.Minute, true),
    )
})
```

---

### BR-ORCH-029: User-Initiated Notification Cancellation (P1)

**Test File**: `pkg/remediation/orchestrator/controller/reconciler_test.go`

| Test Type | Test Name | Status |
|-----------|-----------|--------|
| Unit | `It("should cancel pending notifications on user request")` | ⏳ |
| Integration | `It("should update NotificationRequest status to Cancelled")` | ⏳ |

---

### BR-ORCH-030: Notification Status Tracking (P1)

**Test File**: `pkg/remediation/orchestrator/aggregator/status_test.go`

| Test Type | Test Name | Status |
|-----------|-----------|--------|
| Unit | `It("should track notification delivery status")` | ⏳ |
| Unit | `It("should handle notification failures")` | ⏳ |

---

### BR-ORCH-031: Cascade Deletion Cleanup (P0)

**Test File**: `pkg/remediation/orchestrator/controller/reconciler_test.go`

| Test Type | Test Name | Status |
|-----------|-----------|--------|
| Unit | `It("should set owner reference on all child CRDs")` | ⏳ |
| Unit | `It("should delete children when parent deleted")` | ⏳ |
| Integration | `It("should cascade delete SP, AI, WE, NR on RR deletion")` | ⏳ |

```go
var _ = Describe("Cascade Deletion (BR-ORCH-031)", func() {
    DescribeTable("owner reference verification",
        func(childType string, createChild func() string) {
            // Create child CRD
            childName := createChild()

            // Verify owner reference
            var child client.Object
            switch childType {
            case "SignalProcessing":
                child = &signalprocessingv1.SignalProcessing{}
            case "AIAnalysis":
                child = &aianalysisv1.AIAnalysis{}
            case "WorkflowExecution":
                child = &workflowexecutionv1.WorkflowExecution{}
            }

            err := k8sClient.Get(ctx, client.ObjectKey{Name: childName, Namespace: ns}, child)
            Expect(err).ToNot(HaveOccurred())

            ownerRefs := child.GetOwnerReferences()
            Expect(ownerRefs).To(HaveLen(1))
            Expect(ownerRefs[0].Name).To(Equal(rr.Name))
            Expect(ownerRefs[0].Kind).To(Equal("RemediationRequest"))
        },
        Entry("SignalProcessing", "SignalProcessing", func() string {
            return createSignalProcessing()
        }),
        Entry("AIAnalysis", "AIAnalysis", func() string {
            return createAIAnalysis()
        }),
        Entry("WorkflowExecution", "WorkflowExecution", func() string {
            return createWorkflowExecution()
        }),
    )
})
```

---

### BR-ORCH-032: Handle WE Skipped Phase (P0)

**Test File**: `pkg/remediation/orchestrator/controller/reconciler_test.go`

| Test Type | Test Name | Status |
|-----------|-----------|--------|
| Unit | `It("should handle ResourceBusy skip reason")` | ⏳ |
| Unit | `It("should handle RecentlyRemediated skip reason")` | ⏳ |
| Unit | `It("should transition to Skipped phase")` | ⏳ |
| Unit | `It("should record duplicateOf reference")` | ⏳ |

```go
var _ = Describe("WE Skipped Phase (BR-ORCH-032)", func() {
    DescribeTable("skipped phase handling",
        func(skipReason string, duplicateOf string, expectedPhase string) {
            // Setup WorkflowExecution with Skipped status
            we := &workflowexecutionv1.WorkflowExecution{
                Status: workflowexecutionv1.WorkflowExecutionStatus{
                    Phase: "Skipped",
                    SkipDetails: &workflowexecutionv1.SkipDetails{
                        Reason:              skipReason,
                        ActiveRemediationRef: duplicateOf,
                    },
                },
            }

            // Reconcile
            result, err := reconciler.Reconcile(ctx, req)
            Expect(err).ToNot(HaveOccurred())

            // Verify
            updatedRR := &remediationv1.RemediationRequest{}
            k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)

            Expect(updatedRR.Status.OverallPhase).To(Equal(expectedPhase))
            Expect(updatedRR.Status.SkipReason).To(Equal(skipReason))
            Expect(updatedRR.Status.DuplicateOf).To(Equal(duplicateOf))
        },
        Entry("ResourceBusy", "ResourceBusy", "rr-parent-123", "Skipped"),
        Entry("RecentlyRemediated", "RecentlyRemediated", "rr-recent-456", "Skipped"),
    )
})
```

---

### BR-ORCH-033: Track Duplicate Remediations (P1)

**Test File**: `pkg/remediation/orchestrator/escalation/manager_test.go`

| Test Type | Test Name | Status |
|-----------|-----------|--------|
| Unit | `It("should increment duplicate count on parent")` | ⏳ |
| Unit | `It("should add duplicate ref to parent list")` | ⏳ |
| Integration | `It("should track multiple duplicates on same parent")` | ⏳ |

---

### BR-ORCH-034: Bulk Notification for Duplicates (P1)

**Test File**: `pkg/remediation/orchestrator/creator/notification_test.go`

| Test Type | Test Name | Status |
|-----------|-----------|--------|
| Unit | `It("should create bulk notification when parent completes")` | ⏳ |
| Unit | `It("should include all duplicate refs in notification")` | ⏳ |
| Unit | `It("should not send bulk notification if no duplicates")` | ⏳ |

```go
var _ = Describe("Bulk Duplicate Notification (BR-ORCH-034)", func() {
    DescribeTable("bulk notification scenarios",
        func(duplicateCount int, duplicateRefs []string, shouldNotify bool) {
            rr := &remediationv1.RemediationRequest{
                Status: remediationv1.RemediationRequestStatus{
                    OverallPhase:   "Completed",
                    DuplicateCount: duplicateCount,
                    DuplicateRefs:  duplicateRefs,
                },
            }

            if shouldNotify {
                nrName, err := creator.CreateBulkDuplicateNotification(ctx, rr)
                Expect(err).ToNot(HaveOccurred())

                nr := &notificationv1.NotificationRequest{}
                k8sClient.Get(ctx, client.ObjectKey{Name: nrName, Namespace: ns}, nr)

                Expect(nr.Spec.NotificationType).To(Equal("remediation_completed_with_duplicates"))
                Expect(nr.Spec.Context.DuplicateCount).To(Equal(duplicateCount))
            }
        },
        Entry("no duplicates", 0, nil, false),
        Entry("one duplicate", 1, []string{"rr-dup-1"}, true),
        Entry("multiple duplicates", 3, []string{"rr-dup-1", "rr-dup-2", "rr-dup-3"}, true),
    )
})
```

---

## Coverage Gap Analysis

| BR | Unit Tests | Integration | E2E | Gap |
|----|------------|-------------|-----|-----|
| BR-ORCH-001 | 4 | 1 | 0 | E2E |
| BR-ORCH-025 | 4 | 1 | 0 | E2E |
| BR-ORCH-026 | 3 | 1 | 0 | E2E |
| BR-ORCH-027 | 4 | 1 | 0 | E2E |
| BR-ORCH-028 | 4 | 0 | 0 | Integration, E2E |
| BR-ORCH-029 | 1 | 1 | 0 | E2E |
| BR-ORCH-030 | 2 | 0 | 0 | Integration, E2E |
| BR-ORCH-031 | 2 | 1 | 0 | E2E |
| BR-ORCH-032 | 4 | 0 | 0 | Integration, E2E |
| BR-ORCH-033 | 2 | 1 | 0 | E2E |
| BR-ORCH-034 | 3 | 0 | 0 | Integration, E2E |

---

## Test Distribution Analysis

### By Test Type

| Type | Count | Percentage |
|------|-------|------------|
| Unit | 33 | 70% |
| Integration | 7 | 15% |
| E2E | 0 | 0% |
| **Total** | **40** | - |

### By Package

| Package | Tests | Coverage Target |
|---------|-------|-----------------|
| `controller` | 15 | 80% |
| `phase` | 8 | 90% |
| `creator` | 10 | 75% |
| `aggregator` | 4 | 70% |
| `timeout` | 6 | 85% |
| `escalation` | 5 | 70% |

---

## Test File Reference Index

| File | BRs Covered | Test Count |
|------|-------------|------------|
| `controller/reconciler_test.go` | 001, 026, 029, 031, 032 | 15 |
| `phase/manager_test.go` | 025, 026 | 8 |
| `creator/signalprocessing_test.go` | 031 | 3 |
| `creator/aianalysis_test.go` | 025 | 3 |
| `creator/workflowexecution_test.go` | 025 | 4 |
| `creator/notification_test.go` | 001, 034 | 6 |
| `aggregator/status_test.go` | 030 | 4 |
| `timeout/detector_test.go` | 027, 028 | 6 |
| `escalation/manager_test.go` | 033 | 5 |

---

## Validation Checklist

- [ ] All 11 BRs have at least one unit test
- [ ] All P0 BRs have integration tests
- [ ] DescribeTable patterns used for coverage efficiency
- [ ] Test file structure matches package structure
- [ ] Mock usage limited to external dependencies
- [ ] Real business logic components used in tests

---

## Coverage Maintenance

### Pre-Commit Validation

```bash
# Run unit tests with coverage
go test -cover -coverprofile=coverage.out ./pkg/remediation/orchestrator/...

# Check coverage threshold
go tool cover -func=coverage.out | grep total | awk '{if ($3 < 70) exit 1}'

# Generate HTML report
go tool cover -html=coverage.out -o coverage.html
```

### CI Pipeline Integration

```yaml
test:
  script:
    - go test -race -coverprofile=coverage.out ./pkg/remediation/orchestrator/...
    - go tool cover -func=coverage.out
  coverage: '/total:\s+\(statements\)\s+(\d+\.\d+)%/'
```

