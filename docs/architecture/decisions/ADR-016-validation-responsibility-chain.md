# ADR-016: Validation Responsibility Chain and Data Authority Model

**Status**: ✅ Accepted

**Date**: October 8, 2025

**Deciders**: Architecture Team

**Related**:
- [ADR-001: CRD Microservices Architecture](./ADR-001-crd-microservices-architecture.md)
- [Owner Reference Architecture](./005-owner-reference-architecture.md)
- [APPROVED_MICROSERVICES_ARCHITECTURE.md](../APPROVED_MICROSERVICES_ARCHITECTURE.md)

---

## Context

During the development of the Workflow Execution sequence diagram, three critical architectural questions emerged:

1. **Should WorkflowExecution query Context API for historical effectiveness data during workflow planning?**
2. **Should WorkflowExecution query Kubernetes directly to validate remediation success?**
3. **Who is responsible for validation at each phase of the remediation pipeline?**

These questions highlighted the need for a clear **Validation Responsibility Chain** and **Data Authority Model** to prevent:
- Inconsistencies between AI recommendations and runtime validation
- Redundant validation across services
- Unclear separation of concerns
- Performance overhead from duplicate queries

---

## Decision

We adopt a **layered validation responsibility model** where:

1. **Each layer trusts the data/status from the previous layer**
2. **No redundant validation across services**
3. **Clear single source of truth at each phase**
4. **Validation logic is embedded at the appropriate architectural layer**

### **Principle 1: AI Recommendations Are Authoritative**

**Decision**: WorkflowExecution Controller does NOT query Context API during workflow planning.

**Rationale**:
- AI Analysis Service has already incorporated historical intelligence during investigation phase
- AI recommendations represent the culmination of:
  - Historical pattern analysis (from Context API)
  - Current cluster state analysis (from Kubernetes)
  - LLM-driven root cause identification
  - Safety constraint evaluation
- Re-querying Context API could lead to inconsistencies if data changed between AI analysis and workflow execution
- WorkflowExecution must trust AI recommendations as the **single source of truth**

**Impact**:
- ✅ Eliminates potential inconsistencies between AI recommendations and runtime Context API state
- ✅ Reduces latency (no additional Context API queries during workflow planning)
- ✅ Clear data authority: AI recommendations are authoritative
- ✅ Single source of truth for workflow planning

### **Principle 2: Step-Level Validation with Expected Outcomes**

**Decision**: Each KubernetesExecution step validates its own expected outcome. WorkflowExecution Controller relies on step status.

**Rationale**:
- Validation logic belongs at the execution layer, not orchestration layer
- Each action has a specific expected outcome:
  - Scale deployment → verify replica count matches target
  - Delete pod → verify pod no longer exists
  - Patch configmap → verify new values are present
- KubernetesExecution Controller has the context to validate outcomes correctly
- WorkflowExecution Controller orchestrates flow, doesn't duplicate validation

**Impact**:
- ✅ Clear separation of concerns: orchestration vs execution
- ✅ Validation logic co-located with action execution
- ✅ No redundant Kubernetes queries from workflow layer
- ✅ Step status contains validation results for workflow decisions

### **Principle 3: Context API Is a Data Provider, Not a Validator**

**Decision**: Context API provides historical data only. It does NOT validate Kubernetes resources or introspect cluster state.

**Rationale**:
- Context API's role is to serve historical intelligence for AI decision-making
- Validation requires real-time cluster state, not historical data
- Mixing data provision with validation would blur architectural boundaries
- Validation responsibility belongs to execution layer (KubernetesExecution)

**Impact**:
- ✅ Clear Context API boundaries: data provider only
- ✅ No confusion about validation responsibility
- ✅ Context API remains stateless read-only service
- ✅ Validation logic centralized in execution layer

---

## Validation Responsibility Chain

### **Phase 1: AI Investigation (AI Analysis Service)**

**Responsibility**: Analyze signal and generate recommendations

**Data Authority**: Context API (historical intelligence)

**Validation**:
- Query Context API for:
  - Similar past incidents
  - Historical action effectiveness
  - Environment constraints
- LLM analyzes data and generates recommendations
- Recommendations validated against safety policies

**Output**: Authoritative AI recommendations

---

### **Phase 2: Workflow Planning (WorkflowExecution Controller)**

**Responsibility**: Build executable workflow from AI recommendations

**Data Authority**: AI recommendations (authoritative)

**Validation**:
- ✅ Parse AI recommendations (no Context API revalidation)
- ✅ Build dependency graph
- ✅ Validate safety constraints (Rego policies)
- ✅ Calculate execution order (sequential vs parallel)
- ❌ Does NOT query Context API for historical data
- ❌ Does NOT query Kubernetes for cluster state

**Output**: Executable workflow plan with KubernetesExecution CRDs

---

### **Phase 3: Step Execution (KubernetesExecution Controller)**

**Responsibility**: Execute action and validate expected outcome

**Data Authority**: Kubernetes cluster state (real-time)

**Validation**:
- ✅ Execute action on Kubernetes (scale, delete, patch, etc.)
- ✅ Validate expected outcome:
  - **Scale deployment**: Verify replica count = target
  - **Delete pod**: Verify pod no longer exists
  - **Patch configmap**: Verify new values present
  - **Restart pods**: Verify new pods running and healthy
- ✅ Update CRD status with validation result
- ✅ Store execution result in Data Storage

**Output**: Step status (completed/failed) with validation result

---

### **Phase 4: Workflow Completion (WorkflowExecution Controller)**

**Responsibility**: Monitor workflow progress and completion

**Data Authority**: KubernetesExecution step status (authoritative)

**Validation**:
- ✅ Watch KubernetesExecution CRD status updates
- ✅ Rely on step validation results (no duplicate validation)
- ✅ Determine workflow completion based on step status
- ✅ Trigger rollback if any step fails
- ❌ Does NOT query Kubernetes directly for health validation
- ❌ Does NOT revalidate what steps already validated

**Output**: Workflow status (completed/failed)

---

## Architectural Patterns

### **Pattern 1: Trust the Previous Layer**

Each layer trusts the data/status from the previous layer:

```
AI Investigation → [Recommendations] → Workflow Planning
Workflow Planning → [Step CRDs] → Step Execution
Step Execution → [Step Status] → Workflow Completion
```

**No redundant validation across layers.**

### **Pattern 2: Single Source of Truth at Each Phase**

| Phase | Single Source of Truth |
|-------|----------------------|
| AI Investigation | Context API (historical data) |
| Workflow Planning | AI recommendations |
| Step Execution | Kubernetes cluster state |
| Workflow Completion | Step status |

**Each phase has exactly one authoritative data source.**

### **Pattern 3: Validation at the Right Layer**

| Validation Type | Responsible Layer |
|-----------------|-------------------|
| Historical effectiveness | AI Investigation (queries Context API) |
| Safety policies | Workflow Planning (Rego evaluation) |
| Expected outcomes | Step Execution (validates Kubernetes) |
| Workflow progress | Workflow Completion (monitors step status) |

**Validation logic lives where it has the right context.**

---

## Consequences

### **Positive**

✅ **Clear Separation of Concerns**: Each service has well-defined validation responsibilities
✅ **No Redundant Validation**: Each layer trusts previous layer, eliminating duplicate queries
✅ **Single Source of Truth**: Clear data authority at each phase prevents inconsistencies
✅ **Better Performance**: Reduced latency from fewer redundant queries
✅ **Easier Debugging**: Clear responsibility chain makes troubleshooting straightforward
✅ **Maintainability**: Changes to validation logic are localized to appropriate service

### **Negative**

⚠️ **Trust Model Risk**: If a previous layer has bugs, downstream layers won't catch them
- **Mitigation**: Comprehensive testing at each layer
- **Mitigation**: End-to-end testing validates entire chain

⚠️ **Dependency on Status Updates**: Workflow relies on step CRD status updates
- **Mitigation**: Watch-based coordination ensures real-time status propagation
- **Mitigation**: Timeout mechanisms detect stuck steps

---

## Implementation Guidelines

### **For WorkflowExecution Controller**

**DO**:
- ✅ Parse AI recommendations as authoritative input
- ✅ Build dependency graph from recommendations
- ✅ Validate safety constraints (Rego policies)
- ✅ Watch KubernetesExecution CRD status
- ✅ Rely on step status for completion decisions

**DO NOT**:
- ❌ Query Context API for historical effectiveness
- ❌ Query Kubernetes for resource health validation
- ❌ Revalidate AI recommendations
- ❌ Duplicate step-level validation logic

### **For KubernetesExecution Controller**

**DO**:
- ✅ Execute action on Kubernetes
- ✅ Validate expected outcome after execution
- ✅ Update CRD status with validation result
- ✅ Include validation details in status (e.g., "verified 3 replicas")
- ✅ Store complete execution result in Data Storage

**DO NOT**:
- ❌ Skip expected outcome validation
- ❌ Report success without verification
- ❌ Query Context API for historical data
- ❌ Assume action succeeded without confirmation

### **For Context API**

**DO**:
- ✅ Provide historical data (action effectiveness, similar incidents)
- ✅ Query database for patterns
- ✅ Serve read-only context to AI Investigation phase
- ✅ Supply data for AI decision-making

**DO NOT**:
- ❌ Validate Kubernetes resources
- ❌ Query Kubernetes clusters
- ❌ Introspect cluster state
- ❌ Confirm operation success
- ❌ Provide real-time cluster validation

---

## Examples

### **Example 1: Scale Deployment Workflow**

**AI Recommendation** (Authoritative):
```yaml
action: scale_deployment
parameters:
  deployment: payment-api
  namespace: production
  target_replicas: 5
  current_replicas: 2
reasoning: "Historical data shows 5 replicas optimal for this load pattern"
```

**Workflow Planning** (WorkflowExecution Controller):
```go
// ✅ Correct: Trust AI recommendations
workflow := buildWorkflowFromAIRecommendations(aiAnalysis.Recommendations)

// ❌ Wrong: Don't requery Context API
// effectiveness := contextAPI.GetActionEffectiveness("scale_deployment")
```

**Step Execution** (KubernetesExecution Controller):
```go
// ✅ Correct: Execute and validate expected outcome
k8s.ScaleDeployment("payment-api", "production", 5)

// Validate expected outcome
deployment := k8s.GetDeployment("payment-api", "production")
if deployment.Status.Replicas != 5 {
    return fmt.Errorf("validation failed: expected 5 replicas, got %d", deployment.Status.Replicas)
}

// Update status with validation result
stepCRD.Status.Phase = "Completed"
stepCRD.Status.ValidationResult = "verified 5 replicas running"
```

**Workflow Completion** (WorkflowExecution Controller):
```go
// ✅ Correct: Rely on step status
if stepCRD.Status.Phase == "Completed" {
    // Continue to next step
}

// ❌ Wrong: Don't revalidate Kubernetes
// deployment := k8s.GetDeployment("payment-api", "production")
// if deployment.Status.Replicas != 5 { ... }
```

### **Example 2: Restart Pod Workflow**

**Step Execution** (KubernetesExecution Controller):
```go
// Execute: Delete pod
k8s.DeletePod(podName, namespace)

// Validate expected outcome: Verify new pod is running
newPod := waitForNewPodRunning(podName, namespace, 60*time.Second)
if newPod.Status.Phase != "Running" {
    return fmt.Errorf("validation failed: new pod not running")
}

// Update status
stepCRD.Status.Phase = "Completed"
stepCRD.Status.ValidationResult = fmt.Sprintf("verified new pod %s running", newPod.Name)
```

---

## Verification

### **Testing Strategy**

**Unit Tests**:
- WorkflowExecution: Verify no Context API calls during planning
- KubernetesExecution: Verify expected outcome validation logic
- Each layer: Verify trusts previous layer's data/status

**Integration Tests**:
- End-to-end: Verify validation chain from AI recommendations to completion
- Failure scenarios: Verify step failure detected and propagated
- Status updates: Verify watch-based coordination works correctly

**E2E Tests**:
- Complete remediation flow: Verify entire validation chain works
- Rollback scenarios: Verify workflow relies on step status for rollback decisions

### **Monitoring**

**Metrics**:
- `workflow_validation_skipped_total`: Verify workflow doesn't query Context API
- `step_validation_executed_total`: Verify steps perform expected outcome validation
- `workflow_completion_by_step_status_total`: Verify workflow relies on step status

**Logging**:
- WorkflowExecution: Log AI recommendations usage (not Context API queries)
- KubernetesExecution: Log expected outcome validation results
- Validation chain: Trace responsibility at each phase

---

## References

- [Workflow Execution Sequence Diagram](../APPROVED_MICROSERVICES_ARCHITECTURE.md#workflow-execution-sequence-detailed) (lines 385-507)
- [Verification Report](../../analysis/WORKFLOW_EXECUTION_DIAGRAM_CONSISTENCY_VERIFICATION.md)
- [WorkflowExecution Service Spec](../../services/crd-controllers/03-workflowexecution/overview.md)
- [KubernetesExecution Service Spec](../../services/crd-controllers/04-kubernetesexecutor/overview.md)
- [Context API Service Spec](../../services/stateless/context-api/overview.md)

---

## Related Decisions

- **ADR-001**: CRD Microservices Architecture (establishes service boundaries)
- **ADR-005**: Owner Reference Architecture (establishes CRD ownership model)
- **Future ADR**: May need to document specific expected outcome validation patterns per action type

---

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2025-10-08 | Initial decision | Architecture Team |

---

**Status**: ✅ **APPROVED** - Implemented in Workflow Execution sequence diagram and architectural documentation
