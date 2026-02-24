# Kubernaut Kubernetes Conditions Reference - AUTHORITATIVE

**Status**: ✅ **ACTIVE**
**Last Updated**: 2026-02-18
**Authority**: This is the **single source of truth** for all Kubernetes Conditions across Kubernaut CRD controllers
**Maintained By**: Platform Team
**Related**: [DD-CRD-002: Kubernetes Conditions Standard](decisions/DD-CRD-002-kubernetes-conditions-standard.md)

---

## 📋 **Purpose**

This document provides a **comprehensive inventory** of all Kubernetes Conditions implemented across Kubernaut CRD controllers. Use this to:

1. **Understand available conditions** for automation and scripting
2. **Avoid condition name conflicts** when adding new conditions
3. **Ensure consistency** across services
4. **Reference in documentation** and runbooks

---

## 🎯 **Quick Reference: Implementation Status**

| CRD | Service | Conditions Count | Status | File |
|-----|---------|------------------|--------|------|
| AIAnalysis | AIAnalysis | 5 conditions (incl. Ready), 9 reasons | ✅ Complete | `pkg/aianalysis/conditions.go` |
| WorkflowExecution | WorkflowExecution | 6 conditions (incl. Ready), 15 reasons | ✅ Complete | `pkg/workflowexecution/conditions.go` |
| NotificationRequest | Notification | 2 conditions (incl. Ready), 3 reasons | ✅ Complete | `pkg/notification/conditions.go` |
| SignalProcessing | SignalProcessing | 5 conditions (incl. Ready) | ✅ Complete | `pkg/signalprocessing/conditions.go` |
| RemediationRequest | RO | 5 conditions (incl. Ready) | ✅ Complete | `pkg/remediationrequest/conditions.go` |
| RemediationApprovalRequest | RO | 4 conditions (incl. Ready) | ✅ Complete | `pkg/remediationapprovalrequest/conditions.go` |
| EffectivenessAssessment | EM | 3 conditions (incl. Ready) | ✅ Complete | `pkg/effectivenessmonitor/conditions/conditions.go` |
| KubernetesExecution | WE | 0 | ⚠️ **Deprecated/Excluded (ADR-025)** | - |

**Total**: All 7 active CRDs have conditions wired and implemented. All condition setters set `ObservedGeneration`.

---

## 📚 **Conditions by Service**

### **AIAnalysis Service**

**File**: `pkg/aianalysis/conditions.go` (127 lines)
**Pattern**: Phase-based lifecycle (Investigation → Analysis → Resolution)
**Best For**: Multi-phase processing workflows

| Condition Type | Success Reasons | Failure Reasons | When Set |
|---------------|-----------------|-----------------|----------|
| `Ready` | `Ready` | `NotReady` | Aggregate: True on success terminal, False on failure terminal |
| `InvestigationComplete` | `InvestigationSucceeded` | `InvestigationFailed` | After signal analysis and context gathering |
| `AnalysisComplete` | `AnalysisSucceeded` | `AnalysisFailed` | After AI/LLM analysis completes |
| `WorkflowResolved` | `WorkflowSelected`, `NoWorkflowNeeded` | `WorkflowResolutionFailed` | After workflow selection logic |
| `ApprovalRequired` | `LowConfidence`, `PolicyRequiresApproval` | - | When human approval needed |

**kubectl wait Example**:
```bash
kubectl wait --for=condition=AnalysisComplete aianalysis/analysis-123 --timeout=5m
```

**Business Requirements**:
- BR-AA-001: Signal investigation
- BR-AA-010: AI analysis
- BR-AA-020: Workflow resolution

---

### **WorkflowExecution Service**

**File**: `pkg/workflowexecution/conditions.go` (270 lines)
**Pattern**: Tekton pipeline state mapping with detailed failure reasons
**Best For**: Complex execution workflows with multiple failure modes

| Condition Type | Success Reasons | Failure Reasons | When Set |
|---------------|-----------------|-----------------|----------|
| `Ready` | `Ready` | `NotReady` | Aggregate: True on success terminal, False on failure terminal |
| `Initialized` | `Initialized` | `InitializationFailed`, `InvalidSpec` | After validation and setup |
| `PipelineRunCreated` | `PipelineRunCreated` | `PipelineRunCreationFailed` | After Tekton PipelineRun created |
| `Running` | `Running` | `StartupFailed`, `ResourcesUnavailable` | While workflow is executing |
| `Completed` | `Succeeded` | `Failed`, `Timeout`, `Cancelled` | After workflow finishes |
| `AuditRecorded` | `AuditRecorded` | `AuditRecordingFailed` | After audit trail written |

**kubectl wait Examples**:
```bash
# Wait for workflow to start running
kubectl wait --for=condition=Running workflowexecution/exec-456 --timeout=2m

# Wait for completion
kubectl wait --for=condition=Completed workflowexecution/exec-456 --timeout=30m
```

**Business Requirements**:
- BR-WE-001: Workflow execution
- BR-WE-005: Audit trail
- BR-WE-010: Kubernetes job execution

---

### **Notification Service**

**File**: `pkg/notification/conditions.go` (123 lines)
**Pattern**: Minimal single-condition pattern
**Best For**: Simple approval/routing workflows

| Condition Type | Success Reasons | Failure Reasons | When Set |
|---------------|-----------------|-----------------|----------|
| `Ready` | `Ready` | `NotReady` | Aggregate: True on success terminal, False on failure terminal |
| `RoutingResolved` | `RoutingRuleMatched`, `RoutingFallback` | `RoutingFailed` | After routing rules evaluated |

**kubectl wait Example**:
```bash
kubectl wait --for=condition=RoutingResolved notificationrequest/notif-789 --timeout=30s
```

**kubectl describe Output**:
```yaml
Conditions:
  Type: RoutingResolved
  Status: True
  Reason: RoutingRuleMatched
  Message: Matched rule 'production-critical' (severity=critical, env=production) → channels: slack, email, pagerduty
  Last Transition Time: 2025-12-16T10:30:45Z
```

**Business Requirements**:
- BR-NOT-069: Routing rule visibility

---

## 🔮 **Additional Services (All Wired)**

### **SignalProcessing Service** ✅ **COMPLETE**

**File**: `pkg/signalprocessing/conditions.go`
**Pattern**: Phase-based lifecycle (similar to AIAnalysis)

| Condition Type | Success Reasons | Failure Reasons | When Set |
|---------------|-----------------|-----------------|----------|
| `Ready` | `Ready` | `NotReady` | Aggregate: True on success terminal, False on failure terminal |
| `ValidationComplete` | `ValidationSucceeded` | `ValidationFailed`, `InvalidSignalFormat` | After signal validation |
| `EnrichmentComplete` | `EnrichmentSucceeded` | `EnrichmentFailed`, `K8sAPITimeout`, `ResourceNotFound` | After Kubernetes enrichment |
| `ClassificationComplete` | `ClassificationSucceeded` | `ClassificationFailed`, `RegoEvaluationError`, `PolicyNotFound` | After classification |
| `ProcessingComplete` | `ProcessingSucceeded` | `ProcessingFailed` | After complete processing |

---

### **RemediationRequest Service** ✅ **COMPLETE**

**File**: `pkg/remediationrequest/conditions.go`
**Pattern**: Approval workflow with execution tracking

| Condition Type | Success Reasons | Failure Reasons | When Set |
|---------------|-----------------|-----------------|----------|
| `Ready` | `Ready` | `NotReady` | Aggregate: True on success terminal, False on failure terminal |
| `SignalProcessingReady` | `Ready` | `NotReady` | After SP CRD created |
| `AIAnalysisReady` | `Ready` | `NotReady` | After AA CRD created |
| `WorkflowExecutionReady` | `Ready` | `NotReady` | After WE CRD created |
| `RecoveryComplete` | `RecoverySucceeded`, `RecoverySkipped` | `RecoveryFailed` | After recovery lifecycle |

---

### **RemediationApprovalRequest Service** ✅ **COMPLETE**

**File**: `pkg/remediationapprovalrequest/conditions.go`
**Pattern**: Approval decision tracking

| Condition Type | Success Reasons | Failure Reasons | When Set |
|---------------|-----------------|-----------------|----------|
| `Ready` | `Ready` | `NotReady` | Aggregate: True on success terminal, False on failure terminal |
| `DecisionRecorded` | `Approved`, `Rejected` | `DecisionFailed` | After approval decision made |
| `NotificationSent` | `NotificationSucceeded` | `NotificationFailed` | After notification sent |
| `TimeoutExpired` | `TimeoutExpired` | - | When approval timeout reached |

---

### **EffectivenessAssessment Service** ✅ **COMPLETE**

**File**: `pkg/effectivenessmonitor/conditions/conditions.go`
**Pattern**: Assessment lifecycle tracking

| Condition Type | Success Reasons | Failure Reasons | When Set |
|---------------|-----------------|-----------------|----------|
| `Ready` | `Ready` | `NotReady` | Aggregate: True on success terminal, False on failure terminal |
| `AssessmentComplete` | `AssessmentSucceeded` | `AssessmentFailed`, `AssessmentExpired` | After assessment finishes |
| `SpecIntegrity` | `SpecIntact` | `SpecDrifted` | After spec hash comparison |

---

### **KubernetesExecution Service** ⚠️ **DEPRECATED / EXCLUDED (ADR-025)**

**Status**: Excluded from conditions implementation. KubernetesExecution CRD and KubernetesExecutor service eliminated by ADR-025; replaced by Tekton TaskRun. WorkflowExecution handles job execution via Tekton PipelineRun.

---

## 🎯 **Condition Naming Conventions**

All Kubernaut conditions follow these standards (per DD-CRD-002):

### **Condition Types**

| Pattern | Example | Usage |
|---------|---------|-------|
| `{Phase}Complete` | `ValidationComplete`, `AnalysisComplete` | Phase completion |
| `{Feature}` | `RoutingResolved`, `AuditRecorded` | Feature-specific status |
| `{State}` | `Running`, `Initialized` | Current state |

**Rules**:
- ✅ CamelCase
- ✅ Boolean-style names (`Complete`, not `Completing`)
- ✅ Positive phrasing (`WorkflowResolved`, not `WorkflowNotResolved`)

### **Condition Reasons**

| Pattern | Example | Usage |
|---------|---------|-------|
| `{Phase}Succeeded` | `ValidationSucceeded`, `AnalysisSucceeded` | Success reason |
| `{FailureType}` | `K8sAPITimeout`, `RBACDenied`, `InvalidSpec` | Specific failure |
| `{ContextualReason}` | `LowConfidence`, `PolicyRequiresApproval` | Contextual status |

**Rules**:
- ✅ CamelCase
- ✅ Descriptive and specific
- ✅ Actionable (operator knows what went wrong)

---

## 🔍 **Reserved Condition Names**

To avoid conflicts, these condition names are **reserved** or **commonly used**:

### **Common Kubernetes Conditions**

| Condition | Meaning | Used By |
|-----------|---------|---------|
| `Ready` | Resource is ready (aggregate) | **All 7 Kubernaut CRDs** + Native K8s resources |
| `Available` | Resource is available | Deployments, Services |
| `Progressing` | Resource is progressing | Deployments |
| `Failed` | Resource has failed | Jobs, Pods |

**Note**: All Kubernaut condition setters set `ObservedGeneration` on every condition update (DD-CRD-002 requirement).

### **Kubernaut Reserved Conditions** (Do NOT reuse)

| Condition | Service | Meaning |
|-----------|---------|---------|
| `Ready` | All 7 CRDs | Aggregate: True on success terminal, False on failure terminal |
| `InvestigationComplete` | AIAnalysis | Investigation finished |
| `AnalysisComplete` | AIAnalysis | Analysis finished |
| `WorkflowResolved` | AIAnalysis | Workflow selection done |
| `ApprovalRequired` | AIAnalysis | Approval needed |
| `Initialized` | WorkflowExecution | Workflow initialized |
| `PipelineRunCreated` | WorkflowExecution | Tekton pipeline created |
| `Running` | WorkflowExecution | Workflow running |
| `Completed` | WorkflowExecution | Workflow done |
| `AuditRecorded` | WorkflowExecution | Audit written |
| `RoutingResolved` | Notification | Routing complete |

---

## 📖 **Usage Examples**

### **For Operators**

#### **Check Condition Status**
```bash
# See all conditions for a resource
kubectl describe aianalysis my-analysis

# Filter for specific condition
kubectl get aianalysis my-analysis -o jsonpath='{.status.conditions[?(@.type=="AnalysisComplete")].status}'
```

#### **Wait for Condition**
```bash
# Wait for analysis to complete (5 min timeout)
kubectl wait --for=condition=AnalysisComplete aianalysis/my-analysis --timeout=5m

# Wait for workflow execution to start
kubectl wait --for=condition=Running workflowexecution/my-exec --timeout=2m

# Wait for notification routing
kubectl wait --for=condition=RoutingResolved notificationrequest/my-notif --timeout=30s
```

#### **Automation Scripts**
```bash
#!/bin/bash
# Wait for analysis and check reason
kubectl wait --for=condition=AnalysisComplete aianalysis/my-analysis --timeout=5m

# Get the reason
REASON=$(kubectl get aianalysis my-analysis -o jsonpath='{.status.conditions[?(@.type=="AnalysisComplete")].reason}')

if [ "$REASON" == "AnalysisSucceeded" ]; then
    echo "✅ Analysis completed successfully"
    # Get workflow selection
    WORKFLOW=$(kubectl get aianalysis my-analysis -o jsonpath='{.status.conditions[?(@.type=="WorkflowResolved")].message}')
    echo "Workflow: $WORKFLOW"
else
    echo "❌ Analysis failed: $REASON"
    exit 1
fi
```

### **For Developers**

#### **Setting Conditions in Controllers**
```go
// Import conditions helper
import "github.com/jordigilh/kubernaut/pkg/aianalysis"

// In reconcile function
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &v1alpha1.AIAnalysis{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, err
    }

    // Set condition on success
    if analysisSucceeded {
        aianalysis.SetAnalysisComplete(obj, true, "Analysis completed successfully with confidence 0.95")
    } else {
        aianalysis.SetAnalysisComplete(obj, false, "Analysis failed: LLM timeout after 30s")
    }

    // Update status
    if err := r.Status().Update(ctx, obj); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

#### **Checking Conditions in Code**
```go
// Check if condition is true
if aianalysis.IsConditionTrue(obj, aianalysis.ConditionAnalysisComplete) {
    // Proceed with next step
}

// Get condition details
cond := aianalysis.GetCondition(obj, aianalysis.ConditionAnalysisComplete)
if cond != nil {
    log.Info("Condition status", "type", cond.Type, "status", cond.Status, "reason", cond.Reason)
}
```

---

## 🔄 **Maintenance Process**

### **When Adding New Conditions**

1. **Check this document** - Ensure condition name doesn't conflict
2. **Update `pkg/{service}/conditions.go`** - Add constants and helpers
3. **Update controller** - Set conditions during reconciliation
4. **Add tests** - Unit and integration tests
5. **Update THIS document** - Add new condition to inventory above
6. **Update DD-CRD-002** - If changing standard pattern

### **When Updating Conditions**

1. **Update code** - Modify `pkg/{service}/conditions.go`
2. **Update THIS document** - Reflect changes in tables above
3. **Update documentation** - Service README, runbooks
4. **Communicate** - Notify operators if behavior changes

### **Review Schedule**

- **Monthly**: Review for consistency and completeness
- **Before V1.0**: Ensure all 7 services implemented
- **After service changes**: Update when conditions added/modified

---

## 📊 **Success Metrics**

| Metric | Current | Target (V1.0) |
|--------|---------|---------------|
| **Services with conditions** | 7/7 (100%) | 7/7 (100%) ✅ |
| **Ready condition wired** | 7/7 (100%) | 7/7 (100%) ✅ |
| **ObservedGeneration set** | All setters | All setters ✅ |
| **Operator debug time** | <1 min (kubectl) | <1 min (kubectl) ✅ |
| **Automation coverage** | 7/7 services | 7/7 services ✅ |

---

## 🔗 **Related Documents**

- **Standard**: [DD-CRD-002: Kubernetes Conditions Standard](decisions/DD-CRD-002-kubernetes-conditions-standard.md)
- **Implementation Status**: [TRIAGE_DD-CRD-002_CONDITIONS_IMPLEMENTATION.md](../handoff/TRIAGE_DD-CRD-002_CONDITIONS_IMPLEMENTATION.md)
- **Team Announcement**: [TEAM_ANNOUNCEMENT_DD-CRD-002_CONDITIONS.md](../handoff/TEAM_ANNOUNCEMENT_DD-CRD-002_CONDITIONS.md)
- **Kubernetes API Conventions**: [Conditions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties)

---

**Document Version**: 1.1
**Created**: 2025-12-16
**Last Updated**: 2026-02-18
**Next Review**: Quarterly (Issue #79 Phase 9)
**Maintained By**: Platform Team
**File**: `docs/architecture/KUBERNAUT_CONDITIONS_REFERENCE.md`




