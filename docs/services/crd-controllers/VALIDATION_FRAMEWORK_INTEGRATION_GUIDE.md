# Validation Framework Integration Guide

**Version**: 1.0
**Status**: ✅ **Approved for Implementation**
**Date**: 2025-10-16
**Confidence**: 88%
**Integration Strategy**: Phased Enhancement with B→A Sequential Approach
**Overall Timeline**: 42-47 days development + 6-8 weeks rollout

**Design Decision**: [DD-002 - Per-Step Validation Framework](../../architecture/DESIGN_DECISIONS.md)
**Business Requirements**: [STEP_VALIDATION_BUSINESS_REQUIREMENTS.md](../../requirements/STEP_VALIDATION_BUSINESS_REQUIREMENTS.md)
**Implementation Plans**:
- [WorkflowExecution Implementation Plan](03-workflowexecution/implementation/IMPLEMENTATION_PLAN_V1.0.md)
- [KubernetesExecutor Implementation Plan](04-kubernetesexecutor/implementation/IMPLEMENTATION_PLAN_V1.0.md) (DEPRECATED - ADR-025)

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Integration Architecture](#2-integration-architecture)
3. [WorkflowExecution Integration Points](#3-workflowexecution-integration-points)
4. [KubernetesExecutor Integration Points](#4-kubernetesexecutor-integration-points) (DEPRECATED - ADR-025)
5. [Representative Example: scale_deployment](#5-representative-example-scale_deployment)
6. [Timeline Impact Analysis](#6-timeline-impact-analysis)
7. [Risk Mitigation Strategy](#7-risk-mitigation-strategy)
8. [Testing Strategy](#8-testing-strategy)
9. [Documentation Requirements](#9-documentation-requirements)
10. [Success Metrics & Validation](#10-success-metrics--validation)

---

## 1. Executive Summary

### 1.1 Purpose and Scope

This guide documents the integration of the **DD-002 Per-Step Validation Framework** into the WorkflowExecution and KubernetesExecutor controllers. The validation framework implements precondition and postcondition checks at both the workflow step level and individual action level to improve remediation effectiveness from **70% to 85-90%** through defense-in-depth validation.

**Key Objectives**:
- Prevent cascade failures by validating prerequisites before execution
- Verify successful outcomes after action completion
- Provide observable validation results in CRD status
- Leverage existing Rego policy infrastructure
- Minimize performance impact (<5s per step)
- Maintain acceptable false positive rate (<15%)

### 1.2 Integration Approach

**Strategy**: Phased Enhancement with B→A Sequential Approach

**Rationale**: This approach builds the validation framework **on top of** fully functional base controllers, rather than embedding validation from day one. This strategy:
- ✅ Delivers working controllers early (Phase 0: 23-25 days)
- ✅ Provides clear baseline for measuring validation framework impact
- ✅ Allows independent testing of controller logic
- ✅ Mitigates risk (controllers remain functional even if validation delayed)
- ✅ Enables incremental value delivery with scale_deployment example

**Why B→A (Guide First, Then Plans)**:
- **+5% confidence boost** through validated integration architecture
- **Single source of truth** eliminates inconsistencies between plans
- **Architectural validation** surfaces integration complexities early
- **Reusable reference** for developers during implementation
- **Better risk communication** with explicit integration risk documentation

### 1.3 Timeline Impact

**Base Controllers** (Phase 0):
- WorkflowExecution: 12-13 days
- KubernetesExecutor: 11-12 days
- **Total**: 23-25 days (can be parallelized)

**Validation Framework Integration** (Phases 1-3):
- Phase 1 (Foundation): 7-10 days
- Phase 2 (scale_deployment): 5-7 days
- Phase 3 (Testing): 5-7 days
- **Total**: 17-24 days

**Production Rollout** (Phase 4):
- 6-8 weeks phased deployment
- Parallel to normal operations

**Grand Total**: 42-47 days development + 6-8 weeks rollout

### 1.4 Expected Outcomes

**Quantitative Impact**:
| Metric | Baseline | Target | Improvement |
|---|---|---|---|
| **Remediation Effectiveness** | 70% | 85-90% | +15-20% |
| **Cascade Failure Rate** | 30% | <10% | -20% |
| **MTTR (Failed Remediation)** | 15 min | <8 min | -47% |
| **Manual Intervention** | 40% | 20% | -20% |

**ROI Analysis**:
- **Investment**: ~$35,000 (42-47 days × $800/day)
- **Return**: $12,000/year (time saved) + $84,000-180,000/year (incident reduction)
- **Payback Period**: ~3 months
- **3-Year NPV**: ~$250,000-500,000

### 1.5 Key Stakeholders and Approvals

**Stakeholder Approvals**:
- ✅ **Integration Strategy**: Phased Enhancement approved
- ✅ **Risk Mitigation #1**: False positive management (approved)
- ✅ **Risk Mitigation #2**: Performance impact management (approved)
- ✅ **Risk Mitigation #3**: Integration complexity management (approved)
- ✅ **Risk Mitigation #4**: Operator learning curve (approved)
- ✅ **Risk Mitigation #5**: Maintenance burden management (approved)
- ✅ **Timeline Extension**: 42-47 days accepted
- ✅ **Resource Allocation**: 1-2 developers, 1 QA approved

**Governance**:
- **Architecture Review**: DD-002 approved (2025-10-14, 78% confidence)
- **Business Requirements**: BR-WF-016/052/053, BR-EXEC-016/036 approved
- **Implementation Strategy**: B→A sequential approach approved (90% confidence)

---

## 2. Integration Architecture

### 2.1 Phase Overview

The integration follows a **four-phase approach** with clear dependencies:

```
Phase 0: Base Controllers (23-25 days)
   ↓
Phase 1: Validation Framework Foundation (7-10 days)
   ↓
Phase 2: scale_deployment Representative Example (5-7 days)
   ↓
Phase 3: Integration Testing & Validation (5-7 days)
   ↓
Phase 4: Phased Production Rollout (6-8 weeks)
```

#### Phase 0: Base Controller Implementation
**Status**: Foundation phase
**Duration**: 23-25 days
**Dependencies**: None

**Deliverables**:
- WorkflowExecution controller fully operational (12-13 days)
- KubernetesExecutor controller fully operational (11-12 days)
- Both controllers can orchestrate and execute actions
- No precondition/postcondition validation (baseline functionality)
- All base business requirements covered (35 BRs WF, 39 BRs EXEC)

**Value**: Working system early, clear baseline for comparison

#### Phase 1: Validation Framework Foundation
**Status**: Core infrastructure
**Duration**: 7-10 days
**Dependencies**: Phase 0 complete

**Deliverables**:
- CRD schema extensions (StepCondition, ActionCondition, ConditionResult types)
- Rego policy evaluation framework
- ConfigMap-based policy loading (BR-WF-053)
- Async verification with timeout handling
- Reconciliation phase integration
- New BRs covered: BR-WF-016/052/053, BR-EXEC-016/036

**Value**: Framework ready for condition implementation

#### Phase 2: scale_deployment Representative Example
**Status**: Proof of value
**Duration**: 5-7 days
**Dependencies**: Phase 1 complete

**Deliverables**:
- Complete scale_deployment condition suite (8 policies)
- WorkflowExecution step-level conditions (5 policies)
- KubernetesExecutor action-level conditions (4 policies, 1 shared)
- Integration tests demonstrating defense-in-depth
- E2E validation of complete flow

**Value**: Working example demonstrating effectiveness improvement

#### Phase 3: Integration Testing & Validation
**Status**: Production readiness
**Duration**: 5-7 days
**Dependencies**: Phase 2 complete

**Deliverables**:
- Comprehensive integration test suite (80%+ coverage)
- Performance validation (<5s per step verified)
- False positive scenario testing
- Metrics and observability dashboards
- Operator documentation
- Feature flag implementation
- Rollout plan

**Value**: Production-ready validation framework

#### Phase 4: Phased Production Rollout
**Status**: Value realization
**Duration**: 6-8 weeks
**Dependencies**: Phase 3 complete

**Deliverables**:
- Canary deployment (5% workflows, Week 1-2)
- Staging deployment (100%, Week 3-4)
- Production gradual rollout (10%→50%→100%, Week 5-8)
- Effectiveness improvement validation
- False positive rate monitoring
- Policy tuning based on telemetry

**Value**: Validated effectiveness improvement in production

### 2.2 Shared Infrastructure Components

The validation framework leverages and extends existing infrastructure to minimize implementation complexity and increase confidence.

#### 2.2.1 Rego Policy Engine
**Location**: `pkg/kubernetesexecution/policy/` (existing, from Day 4)
**Status**: ✅ Already implemented in KubernetesExecutor Day 4

**Existing Capabilities**:
- Rego policy loading from filesystem/ConfigMap
- Policy compilation and caching
- Input schema construction
- Policy evaluation with context
- Error handling and timeout management

**Extensions for Validation Framework**:
- Separate condition evaluation from safety policy evaluation
- Add async verification for postconditions
- Support ConfigMap-based policy hot-reload (BR-WF-053)
- Extend input schema for cluster state queries

**Shared By**:
- KubernetesExecutor: Action preconditions/postconditions
- WorkflowExecution: Step preconditions/postconditions (via shared package)

**Benefit**: **~30% reduction in implementation time**, +10% confidence

#### 2.2.2 ConfigMap Pattern for Policy Loading
**Pattern**: ConfigMap-based policy storage with watch-based hot-reload
**Status**: ✅ Established pattern in KubernetesExecutor

**Structure**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-workflow-conditions
  namespace: kubernaut-system
data:
  deployment_exists.rego: |
    package precondition.scale_deployment
    import future.keywords.if
    allow if { input.deployment_found == true }

  desired_replicas_running.rego: |
    package postcondition.scale_deployment
    import future.keywords.if
    allow if {
      input.running_pods >= input.target_replicas
      input.ready_pods >= input.target_replicas
    }
```

**Benefit**: Operator-friendly policy management, no controller restart required

#### 2.2.3 Async Verification Framework
**Purpose**: Poll-based verification for postconditions with configurable timeout
**Status**: New component (Phase 1)

**Design Pattern**:
```go
func (r *Reconciler) verifyPostcondition(
    ctx context.Context,
    condition StepCondition,
) (*ConditionResult, error) {
    timeout, _ := time.ParseDuration(condition.Timeout)
    deadline := time.Now().Add(timeout)

    for time.Now().Before(deadline) {
        // Query cluster state
        state := r.queryClusterState(ctx, condition.Type)

        // Evaluate Rego policy
        result, err := r.regoEvaluator.Evaluate(condition.Rego, state)
        if err == nil && result.Allowed {
            return &ConditionResult{Passed: true}, nil
        }

        // Wait before retry
        time.Sleep(5 * time.Second)
    }

    return &ConditionResult{
        Passed: false,
        ErrorMessage: "Verification timeout",
    }, nil
}
```

**Shared By**: Both controllers for postcondition verification

#### 2.2.4 Cluster State Query Utilities
**Purpose**: Efficient querying of Kubernetes resources for condition evaluation
**Status**: New component (Phase 1)

**Data Sources**:
- Kubernetes API: Deployment/Pod/Node status
- Metrics API: CPU/memory usage, pod health
- Custom Resources: Application-specific state

**Caching Strategy**: 5-10 second cache to reduce API load

**Benefit**: Consistent cluster state access across conditions

### 2.3 Critical Decision Points

#### Decision 1: Extend Existing Safety Engine vs New Component
**Choice**: ✅ Extend existing KubernetesExecutor Day 4 safety engine
**Rationale**:
- Reuses proven Rego integration
- Reduces implementation time by ~30%
- Increases confidence through battle-tested code
- Avoids parallel policy systems

**Clear Separation**:
- **Safety Policies**: Security and organizational constraints (existing)
- **Preconditions**: Business prerequisites for execution (NEW)
- **Postconditions**: Business verification of success (NEW)

#### Decision 2: Required vs Optional Conditions
**Choice**: ✅ Support both `required: true` and `required: false`
**Rationale**:
- Required conditions block execution/mark failure
- Optional conditions log warnings only
- Enables gradual policy tightening based on telemetry
- Mitigates false positive risk

**Risk Mitigation**: Start all new conditions as `required: false` during Phase 4 rollout

#### Decision 3: Synchronous vs Asynchronous Postcondition Verification
**Choice**: ✅ Asynchronous verification with configurable timeout
**Rationale**:
- Kubernetes resources take time to converge (pods starting, deployments progressing)
- Synchronous checks would fail prematurely
- Timeout provides upper bound on wait time
- Performance impact minimized (<5s target)

#### Decision 4: Condition Template Scope for V1
**Choice**: ✅ Complete implementation for scale_deployment only, placeholders for others
**Rationale**:
- scale_deployment is most common action (~40% of scenarios)
- Provides representative example for operators
- Reduces V1 scope and timeline
- Enables effectiveness measurement with single action
- Other actions can follow same pattern in future releases

**Phased Rollout**:
- Phase 2: scale_deployment (8 policies)
- Future: Expand to top 5 actions (restart_pod, increase_resources, rollback_deployment, expand_pvc)
- Future: Remaining 22 actions as needed

---

## 3. WorkflowExecution Integration Points

### 3.1 Overview

The WorkflowExecution controller orchestrates multi-step workflows by creating KubernetesExecution CRDs for each workflow step. The validation framework adds **step-level preconditions and postconditions** to validate cluster state before and after each step.

**Integration Philosophy**: "Validate early, verify often"
- Preconditions prevent invalid workflow steps from executing
- Postconditions verify workflow step success at workflow level
- Complements action-level validation in KubernetesExecutor

### 3.2 Phase 1: CRD Schema Extensions (Days 14-15, 16 hours)

#### 3.2.1 Schema Changes

**File**: `api/workflowexecution/v1alpha1/workflowexecution_types.go`

**New Types**:

```go
// StepCondition defines a validation rule for workflow steps
// +kubebuilder:object:generate=true
type StepCondition struct {
    // Type identifies the condition (e.g., "deployment_exists")
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinLength=1
    Type string `json:"type"`

    // Description provides human-readable explanation
    // +kubebuilder:validation:Required
    Description string `json:"description"`

    // Rego contains the policy definition in Rego language
    // +kubebuilder:validation:Required
    Rego string `json:"rego"`

    // Required indicates if condition failure blocks execution
    // +kubebuilder:validation:Required
    // +kubebuilder:default=true
    Required bool `json:"required"`

    // Timeout specifies maximum verification duration
    // +kubebuilder:validation:Optional
    // +kubebuilder:default="30s"
    Timeout string `json:"timeout,omitempty"`
}

// ConditionResult captures the outcome of condition evaluation
// +kubebuilder:object:generate=true
type ConditionResult struct {
    // ConditionType identifies which condition was evaluated
    // +kubebuilder:validation:Required
    ConditionType string `json:"conditionType"`

    // Evaluated indicates if evaluation was attempted
    // +kubebuilder:validation:Required
    Evaluated bool `json:"evaluated"`

    // Passed indicates if condition succeeded
    // +kubebuilder:validation:Required
    Passed bool `json:"passed"`

    // ErrorMessage contains failure details
    // +kubebuilder:validation:Optional
    ErrorMessage string `json:"errorMessage,omitempty"`

    // EvaluationTime records when evaluation occurred
    // +kubebuilder:validation:Required
    EvaluationTime metav1.Time `json:"evaluationTime"`
}
```

**WorkflowStep Extensions**:

```go
// WorkflowStep defines a single step in the workflow
type WorkflowStep struct {
    // ... existing fields (StepNumber, Action, Parameters, etc.) ...

    // PreConditions validate cluster state before step execution (BR-WF-016)
    // +kubebuilder:validation:Optional
    PreConditions []StepCondition `json:"preConditions,omitempty"`

    // PostConditions verify successful outcomes after step completion (BR-WF-052)
    // +kubebuilder:validation:Optional
    PostConditions []StepCondition `json:"postConditions,omitempty"`
}
```

**StepStatus Extensions**:

```go
// StepStatus tracks execution status of individual workflow steps
type StepStatus struct {
    // ... existing fields (StepNumber, Phase, Message, etc.) ...

    // PreConditionResults contains precondition evaluation outcomes
    // +kubebuilder:validation:Optional
    PreConditionResults []ConditionResult `json:"preConditionResults,omitempty"`

    // PostConditionResults contains postcondition verification outcomes
    // +kubebuilder:validation:Optional
    PostConditionResults []ConditionResult `json:"postConditionResults,omitempty"`
}
```

#### 3.2.2 Implementation Tasks

**Day 14: Type Definitions**
1. Add StepCondition, ConditionResult types to `workflowexecution_types.go`
2. Add PreConditions/PostConditions fields to WorkflowStep
3. Add PreConditionResults/PostConditionResults to StepStatus
4. Add kubebuilder markers for validation
5. Update DeepCopy methods (auto-generated)

**Day 15: CRD Generation and Testing**
1. Run `make manifests` to regenerate CRDs
2. Verify CRD schema correctness
3. Write unit tests for type validation:
   - Required field validation
   - Timeout parsing
   - Condition result creation
4. Verify backwards compatibility (empty conditions allowed)

**Testing Strategy**:
```go
func TestStepConditionValidation(t *testing.T) {
    tests := []struct {
        name      string
        condition StepCondition
        wantErr   bool
    }{
        {
            name: "valid required condition",
            condition: StepCondition{
                Type:        "deployment_exists",
                Description: "Deployment must exist",
                Rego:        "package test\nallow = true",
                Required:    true,
                Timeout:     "30s",
            },
            wantErr: false,
        },
        {
            name: "missing type",
            condition: StepCondition{
                Description: "Test",
                Rego:        "package test\nallow = true",
            },
            wantErr: true,
        },
    }
    // ... test implementation
}
```

### 3.3 Phase 1: Rego Policy Integration (Days 16-18, 24 hours)

#### 3.3.1 Package Structure

**New Package**: `pkg/workflowexecution/conditions/`

**Files to Create**:
1. `engine.go` - Condition evaluation engine
2. `loader.go` - ConfigMap policy loader
3. `verifier.go` - Async verification framework
4. `types.go` - Condition-specific types
5. `engine_test.go` - Unit tests

#### 3.3.2 Condition Evaluation Engine

**File**: `pkg/workflowexecution/conditions/engine.go`

```go
package conditions

import (
    "context"
    "fmt"
    "time"

    "github.com/open-policy-agent/opa/rego"
    workflowv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Engine evaluates step conditions using Rego policies
type Engine struct {
    regoEval   *rego.Rego
    policies   map[string]string // condition type -> rego policy
    stateQuery ClusterStateQuery
}

// ClusterStateQuery interface for querying cluster state
type ClusterStateQuery interface {
    QueryState(ctx context.Context, conditionType string, step *workflowv1.WorkflowStep) (map[string]interface{}, error)
}

// NewEngine creates a new condition evaluation engine
func NewEngine(policies map[string]string, stateQuery ClusterStateQuery) (*Engine, error) {
    return &Engine{
        policies:   policies,
        stateQuery: stateQuery,
    }, nil
}

// EvaluatePreconditions evaluates all preconditions for a workflow step
func (e *Engine) EvaluatePreconditions(
    ctx context.Context,
    step *workflowv1.WorkflowStep,
) ([]workflowv1.ConditionResult, error) {
    results := make([]workflowv1.ConditionResult, 0, len(step.PreConditions))

    for _, condition := range step.PreConditions {
        result := e.evaluateCondition(ctx, condition, step)
        results = append(results, result)

        // If required condition failed, stop evaluation
        if condition.Required && !result.Passed {
            return results, fmt.Errorf("required precondition failed: %s", condition.Type)
        }
    }

    return results, nil
}

// evaluateCondition evaluates a single condition
func (e *Engine) evaluateCondition(
    ctx context.Context,
    condition workflowv1.StepCondition,
    step *workflowv1.WorkflowStep,
) workflowv1.ConditionResult {
    result := workflowv1.ConditionResult{
        ConditionType:  condition.Type,
        Evaluated:      true,
        EvaluationTime: metav1.Now(),
    }

    // Query cluster state
    state, err := e.stateQuery.QueryState(ctx, condition.Type, step)
    if err != nil {
        result.Passed = false
        result.ErrorMessage = fmt.Sprintf("Failed to query cluster state: %v", err)
        return result
    }

    // Evaluate Rego policy
    query := rego.New(
        rego.Query("data.precondition.allow"),
        rego.Module(condition.Type+".rego", condition.Rego),
    )

    rs, err := query.Eval(ctx, rego.EvalInput(state))
    if err != nil {
        result.Passed = false
        result.ErrorMessage = fmt.Sprintf("Policy evaluation failed: %v", err)
        return result
    }

    // Check result
    if len(rs) == 0 || len(rs[0].Expressions) == 0 {
        result.Passed = false
        result.ErrorMessage = "Policy returned no result"
        return result
    }

    allowed, ok := rs[0].Expressions[0].Value.(bool)
    if !ok {
        result.Passed = false
        result.ErrorMessage = "Policy result is not boolean"
        return result
    }

    result.Passed = allowed
    if !allowed && condition.Required {
        result.ErrorMessage = fmt.Sprintf("Required condition %s failed", condition.Type)
    }

    return result
}
```

#### 3.3.3 ConfigMap Policy Loader

**File**: `pkg/workflowexecution/conditions/loader.go`

```go
package conditions

import (
    "context"
    "fmt"

    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// PolicyLoader loads condition policies from ConfigMap
type PolicyLoader struct {
    client        client.Client
    namespace     string
    configMapName string
}

// NewPolicyLoader creates a new policy loader
func NewPolicyLoader(client client.Client, namespace, configMapName string) *PolicyLoader {
    return &PolicyLoader{
        client:        client,
        namespace:     namespace,
        configMapName: configMapName,
    }
}

// LoadPolicies loads all policies from ConfigMap (BR-WF-053)
func (l *PolicyLoader) LoadPolicies(ctx context.Context) (map[string]string, error) {
    cm := &corev1.ConfigMap{}
    key := client.ObjectKey{
        Namespace: l.namespace,
        Name:      l.configMapName,
    }

    if err := l.client.Get(ctx, key, cm); err != nil {
        return nil, fmt.Errorf("failed to load policy ConfigMap: %w", err)
    }

    policies := make(map[string]string, len(cm.Data))
    for filename, content := range cm.Data {
        // Extract condition type from filename (e.g., "deployment_exists.rego" -> "deployment_exists")
        conditionType := filename
        if len(filename) > 5 && filename[len(filename)-5:] == ".rego" {
            conditionType = filename[:len(filename)-5]
        }
        policies[conditionType] = content
    }

    return policies, nil
}
```

#### 3.3.4 Implementation Tasks

**Day 16: Engine Implementation**
1. Create conditions package
2. Implement Engine struct and NewEngine
3. Implement EvaluatePreconditions method
4. Implement evaluateCondition helper
5. Unit tests for policy evaluation logic

**Day 17: Policy Loading and Cluster State Queries**
1. Implement PolicyLoader
2. Implement ClusterStateQuery interface
3. Create concrete implementation for common queries:
   - Deployment state
   - Pod counts and status
   - Resource availability
4. Unit tests for loader and state queries

**Day 18: Async Verification Framework**
1. Implement VerifyPostconditions with async polling
2. Add timeout handling
3. Add retry logic with exponential backoff
4. Integration tests with real ConfigMap
5. Performance validation (<5s target)

### 3.4 Phase 1: Reconciliation Integration (Days 19-20, 16 hours)

#### 3.4.1 Precondition Evaluation in Validating Phase

**File**: `internal/controller/workflowexecution_controller.go`

**Integration Point**: `reconcileValidating()` phase

```go
func (r *WorkflowExecutionReconciler) reconcileValidating(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // ... existing validation (parameter, RBAC, etc.) ...

    // NEW: Evaluate step preconditions before creating KubernetesExecution CRDs
    for i, step := range workflow.Spec.WorkflowDefinition.Steps {
        if len(step.PreConditions) == 0 {
            continue // No preconditions to evaluate
        }

        log.Info("Evaluating step preconditions", "step", step.StepNumber)

        results, err := r.ConditionEngine.EvaluatePreconditions(ctx, &step)
        if err != nil {
            // Required precondition failed - block workflow execution
            log.Error(err, "Precondition evaluation failed", "step", step.StepNumber)

            workflow.Status.Phase = "blocked"
            workflow.Status.Message = fmt.Sprintf("Step %d precondition failed: %v", step.StepNumber, err)

            // Update step status with precondition results
            if len(workflow.Status.StepStatuses) > i {
                workflow.Status.StepStatuses[i].PreConditionResults = results
                workflow.Status.StepStatuses[i].Phase = "blocked"
            }

            return ctrl.Result{}, r.Status().Update(ctx, workflow)
        }

        // Update status with precondition results (all passed or optional failures)
        if len(workflow.Status.StepStatuses) > i {
            workflow.Status.StepStatuses[i].PreConditionResults = results
        }

        log.Info("Step preconditions passed", "step", step.StepNumber, "results", len(results))
    }

    // All preconditions passed - transition to executing
    workflow.Status.Phase = "executing"
    return ctrl.Result{}, r.Status().Update(ctx, workflow)
}
```

#### 3.4.2 Postcondition Verification in Monitoring Phase

**Integration Point**: `reconcileMonitoring()` phase (after step completion)

```go
func (r *WorkflowExecutionReconciler) reconcileMonitoring(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // ... existing monitoring (check KubernetesExecution status) ...

    // NEW: Verify postconditions for completed steps
    for i, step := range workflow.Spec.WorkflowDefinition.Steps {
        stepStatus := workflow.Status.StepStatuses[i]

        // Only verify postconditions for completed steps that haven't been verified yet
        if stepStatus.Phase != "completed" || len(stepStatus.PostConditionResults) > 0 {
            continue
        }

        if len(step.PostConditions) == 0 {
            continue // No postconditions to verify
        }

        log.Info("Verifying step postconditions", "step", step.StepNumber)

        results, err := r.ConditionEngine.VerifyPostconditions(ctx, &step)
        if err != nil {
            // Required postcondition failed - mark step as failed, trigger rollback
            log.Error(err, "Postcondition verification failed", "step", step.StepNumber)

            workflow.Status.StepStatuses[i].PostConditionResults = results
            workflow.Status.StepStatuses[i].Phase = "failed"
            workflow.Status.StepStatuses[i].Message = fmt.Sprintf("Postcondition failed: %v", err)

            // Trigger rollback if configured
            workflow.Status.Phase = "rolling_back"
            workflow.Status.Message = fmt.Sprintf("Step %d postcondition failed, initiating rollback", step.StepNumber)

            return ctrl.Result{}, r.Status().Update(ctx, workflow)
        }

        // Update status with postcondition results
        workflow.Status.StepStatuses[i].PostConditionResults = results

        log.Info("Step postconditions verified", "step", step.StepNumber, "results", len(results))
    }

    // Check if all steps completed and verified
    allCompleted := true
    for _, stepStatus := range workflow.Status.StepStatuses {
        if stepStatus.Phase != "completed" {
            allCompleted = false
            break
        }
    }

    if allCompleted {
        workflow.Status.Phase = "completed"
        workflow.Status.Message = "All workflow steps completed successfully"
        return ctrl.Result{}, r.Status().Update(ctx, workflow)
    }

    // Requeue to continue monitoring
    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}
```

#### 3.4.3 Status Propagation and Metrics

**Metrics Implementation**:

```go
var (
    conditionEvaluations = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_condition_evaluations_total",
            Help: "Total number of condition evaluations",
        },
        []string{"condition_type", "phase", "required"},
    )

    conditionPassed = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_condition_passed_total",
            Help: "Total number of conditions that passed",
        },
        []string{"condition_type", "phase"},
    )

    conditionFailed = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_condition_failed_total",
            Help: "Total number of conditions that failed",
        },
        []string{"condition_type", "phase", "required"},
    )

    conditionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "workflowexecution_condition_duration_seconds",
            Help:    "Duration of condition evaluation",
            Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
        },
        []string{"condition_type", "phase"},
    )
)
```

#### 3.4.4 Implementation Tasks

**Day 19: Reconciliation Integration**
1. Add ConditionEngine field to WorkflowExecutionReconciler
2. Integrate precondition evaluation in reconcileValidating
3. Integrate postcondition verification in reconcileMonitoring
4. Update status propagation logic
5. Handle blocking vs warning conditions
6. Unit tests for reconciliation phases

**Day 20: Metrics and Error Handling**
1. Implement Prometheus metrics
2. Add metrics instrumentation to condition evaluation
3. Enhance error messages with context
4. Add logging for condition results
5. Integration tests with metrics verification

### 3.5 Phase 2: scale_deployment Step Example (Days 21-22, 16 hours)

#### 3.5.1 Step-Level Precondition Policies

**Precondition 1: deployment_exists**

```yaml
type: deployment_exists
description: "Deployment must exist before scaling"
rego: |
  package precondition.scale_deployment
  import future.keywords.if

  allow if {
    input.deployment_found == true
  }

  error_message := msg if {
    not input.deployment_found
    msg := sprintf("Deployment %s not found in namespace %s",
                   [input.deployment_name, input.namespace])
  }
required: true
timeout: "10s"
```

**Precondition 2: cluster_capacity_available**

```yaml
type: cluster_capacity_available
description: "Cluster must have sufficient capacity for additional replicas"
rego: |
  package precondition.scale_deployment
  import future.keywords.if

  allow if {
    cpu_available_millicores := parse_cpu(input.available_cpu)
    cpu_needed_millicores := parse_cpu(input.required_cpu_per_pod) * input.additional_replicas

    memory_available_bytes := parse_memory(input.available_memory)
    memory_needed_bytes := parse_memory(input.required_memory_per_pod) * input.additional_replicas

    cpu_available_millicores >= cpu_needed_millicores
    memory_available_bytes >= memory_needed_bytes
  }

  parse_cpu(cpu_string) := millicores if {
    endswith(cpu_string, "m")
    millicores := to_number(trim_suffix(cpu_string, "m"))
  } else := millicores if {
    cores := to_number(cpu_string)
    millicores := cores * 1000
  }

  parse_memory(mem_string) := bytes if {
    endswith(mem_string, "Gi")
    gibibytes := to_number(trim_suffix(mem_string, "Gi"))
    bytes := gibibytes * 1073741824
  } else := bytes if {
    endswith(mem_string, "Mi")
    mebibytes := to_number(trim_suffix(mem_string, "Mi"))
    bytes := mebibytes * 1048576
  }
required: true
timeout: "10s"
```

**Precondition 3: current_replicas_match** (optional)

```yaml
type: current_replicas_match
description: "Current replica count matches expected baseline"
rego: |
  package precondition.scale_deployment
  import future.keywords.if

  allow if {
    input.current_replicas == input.expected_baseline
  }
required: false  # Optional - log warning if baseline changed
timeout: "5s"
```

#### 3.5.2 Step-Level Postcondition Policies

**Postcondition 1: desired_replicas_running**

```yaml
type: desired_replicas_running
description: "All desired replicas must be running and ready"
rego: |
  package postcondition.scale_deployment
  import future.keywords.if

  allow if {
    input.running_pods >= input.target_replicas
    input.ready_pods >= input.target_replicas
  }
required: true
timeout: "2m"  # Allow time for pods to start
```

**Postcondition 2: deployment_health_check**

```yaml
type: deployment_health_check
description: "Deployment must be in Available and Progressing state"
rego: |
  package postcondition.scale_deployment
  import future.keywords.if

  allow if {
    input.conditions.Available == true
    input.conditions.Progressing == true
    input.conditions.ReplicaFailure == false
  }
required: true
timeout: "2m"
```

#### 3.5.3 ConfigMap Example

**ConfigMap**: `kubernaut-workflow-conditions`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-workflow-conditions
  namespace: kubernaut-system
  labels:
    app: kubernaut
    component: workflow-execution
data:
  deployment_exists.rego: |
    package precondition.scale_deployment
    import future.keywords.if
    allow if { input.deployment_found == true }

  cluster_capacity_available.rego: |
    package precondition.scale_deployment
    import future.keywords.if
    allow if {
      cpu_available_millicores := parse_cpu(input.available_cpu)
      cpu_needed_millicores := parse_cpu(input.required_cpu_per_pod) * input.additional_replicas
      memory_available_bytes := parse_memory(input.available_memory)
      memory_needed_bytes := parse_memory(input.required_memory_per_pod) * input.additional_replicas
      cpu_available_millicores >= cpu_needed_millicores
      memory_available_bytes >= memory_needed_bytes
    }
    parse_cpu(cpu_string) := millicores if {
      endswith(cpu_string, "m")
      millicores := to_number(trim_suffix(cpu_string, "m"))
    } else := millicores if {
      cores := to_number(cpu_string)
      millicores := cores * 1000
    }
    parse_memory(mem_string) := bytes if {
      endswith(mem_string, "Gi")
      gibibytes := to_number(trim_suffix(mem_string, "Gi"))
      bytes := gibibytes * 1073741824
    } else := bytes if {
      endswith(mem_string, "Mi")
      mebibytes := to_number(trim_suffix(mem_string, "Mi"))
      bytes := mebibytes * 1048576
    }

  desired_replicas_running.rego: |
    package postcondition.scale_deployment
    import future.keywords.if
    allow if {
      input.running_pods >= input.target_replicas
      input.ready_pods >= input.target_replicas
    }

  deployment_health_check.rego: |
    package postcondition.scale_deployment
    import future.keywords.if
    allow if {
      input.conditions.Available == true
      input.conditions.Progressing == true
      input.conditions.ReplicaFailure == false
    }
```

#### 3.5.4 Implementation Tasks

**Day 21: Precondition Policies**
1. Create ConfigMap with 3 precondition policies
2. Implement cluster state queries for:
   - Deployment existence check
   - Cluster capacity calculation
   - Current replica count
3. Unit tests for each policy
4. Integration tests with real ConfigMap

**Day 22: Postcondition Policies and E2E Tests**
1. Add 2 postcondition policies to ConfigMap
2. Implement cluster state queries for:
   - Pod counts (running/ready)
   - Deployment conditions
3. Unit tests for each policy
4. E2E test: Complete workflow with scale_deployment step
5. Verify precondition blocking
6. Verify postcondition verification

---


## 4. KubernetesExecutor Integration Points

### 4.1 Overview

The KubernetesExecutor controller executes individual Kubernetes actions via native Kubernetes Jobs. The validation framework adds **action-level preconditions and postconditions** to validate prerequisites and verify successful execution outcomes.

**Integration Philosophy**: "Extend, don't rebuild"
- Leverage existing Day 4 Safety Policy Engine
- Separate safety policies (security) from conditions (business validation)
- Action preconditions provide action-specific validation
- Action postconditions verify execution outcomes

**Key Advantage**: KubernetesExecutor (DEPRECATED - ADR-025) already has Rego policy integration from Day 4, reducing implementation effort by ~30%.

### 4.2 Phase 1: CRD Schema Extensions (Days 13-14, 16 hours)

#### 4.2.1 Schema Changes

**File**: `api/kubernetesexecution/v1alpha1/kubernetesexecution_types.go` (DEPRECATED - ADR-025)

**New Types** (consistent with WorkflowExecution):

```go
// ActionCondition defines a validation rule for Kubernetes actions
// +kubebuilder:object:generate=true
type ActionCondition struct {
    // Type identifies the condition (e.g., "image_pull_secrets_valid")
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinLength=1
    Type string `json:"type"`

    // Description provides human-readable explanation
    // +kubebuilder:validation:Required
    Description string `json:"description"`

    // Rego contains the policy definition in Rego language
    // +kubebuilder:validation:Required
    Rego string `json:"rego"`

    // Required indicates if condition failure blocks execution
    // +kubebuilder:validation:Required
    // +kubebuilder:default=true
    Required bool `json:"required"`

    // Timeout specifies maximum verification duration
    // +kubebuilder:validation:Optional
    // +kubebuilder:default="30s"
    Timeout string `json:"timeout,omitempty"`
}

// ConditionResult captures the outcome of condition evaluation
// (Identical to WorkflowExecution's ConditionResult for consistency)
// +kubebuilder:object:generate=true
type ConditionResult struct {
    // ConditionType identifies which condition was evaluated
    // +kubebuilder:validation:Required
    ConditionType string `json:"conditionType"`

    // Evaluated indicates if evaluation was attempted
    // +kubebuilder:validation:Required
    Evaluated bool `json:"evaluated"`

    // Passed indicates if condition succeeded
    // +kubebuilder:validation:Required
    Passed bool `json:"passed"`

    // ErrorMessage contains failure details
    // +kubebuilder:validation:Optional
    ErrorMessage string `json:"errorMessage,omitempty"`

    // EvaluationTime records when evaluation occurred
    // +kubebuilder:validation:Required
    EvaluationTime metav1.Time `json:"evaluationTime"`
}
```

**KubernetesExecutionSpec Extensions**:

```go
// KubernetesExecutionSpec defines the desired state of KubernetesExecution
type KubernetesExecutionSpec struct {
    // ... existing fields (Action, TargetResource, Parameters, etc.) ...

    // PreConditions validate prerequisites before action execution (BR-EXEC-016)
    // +kubebuilder:validation:Optional
    PreConditions []ActionCondition `json:"preConditions,omitempty"`

    // PostConditions verify successful outcomes after action completion (BR-EXEC-036)
    // +kubebuilder:validation:Optional
    PostConditions []ActionCondition `json:"postConditions,omitempty"`
}
```

**ValidationResults Extensions**:

```go
// ValidationResults contains results from validation phase
type ValidationResults struct {
    // ... existing fields (SafetyPolicyPassed, DryRunSucceeded, etc.) ...

    // PreConditionResults contains precondition evaluation outcomes
    // +kubebuilder:validation:Optional
    PreConditionResults []ConditionResult `json:"preConditionResults,omitempty"`

    // PostConditionResults contains postcondition verification outcomes
    // +kubebuilder:validation:Optional
    PostConditionResults []ConditionResult `json:"postConditionResults,omitempty"`
}
```

### 4.3 Phase 1: Safety Engine Extension (Days 15-17, 24 hours)

#### 4.3.1 Leveraging Existing Day 4 Infrastructure

**🔑 CRITICAL INTEGRATION POINT**: KubernetesExecutor (DEPRECATED - ADR-025) Day 4 already implements a Rego-based safety policy engine. The validation framework **extends** this existing infrastructure rather than creating a parallel system.

**Existing Day 4 Components**:
- ✅ `pkg/kubernetesexecution/policy/engine.go` - PolicyEngine struct
- ✅ Rego policy loading from ConfigMap
- ✅ Policy compilation and caching
- ✅ Input schema construction
- ✅ Policy evaluation with context

**Extensions Needed**:
- Add condition-specific evaluation methods
- Separate condition evaluation from safety policy evaluation
- Add async verification for postconditions
- Extend cluster state query utilities

#### 4.3.2 Extended PolicyEngine

**File**: `pkg/kubernetesexecution/policy/engine.go` (EXTEND EXISTING)

**New Methods**:

```go
// EvaluateActionConditions evaluates preconditions for a Kubernetes action
// This is separate from EvaluateSafetyPolicy to maintain clear separation
func (e *PolicyEngine) EvaluateActionConditions(
    ctx context.Context,
    execution *kubernetesexecutionv1alpha1.KubernetesExecution,
) ([]kubernetesexecutionv1alpha1.ConditionResult, error) {
    log := log.FromContext(ctx)

    preconditions := execution.Spec.PreConditions
    if len(preconditions) == 0 {
        return nil, nil // No preconditions to evaluate
    }

    results := make([]kubernetesexecutionv1alpha1.ConditionResult, 0, len(preconditions))

    for _, condition := range preconditions {
        log.Info("Evaluating action precondition", "type", condition.Type)

        result := e.evaluateCondition(ctx, condition, execution)
        results = append(results, result)

        // If required condition failed, stop evaluation
        if condition.Required && !result.Passed {
            return results, fmt.Errorf("required precondition failed: %s", condition.Type)
        }
    }

    return results, nil
}

// VerifyActionPostconditions verifies postconditions after action execution
// Uses async verification with timeout for eventual consistency
func (e *PolicyEngine) VerifyActionPostconditions(
    ctx context.Context,
    execution *kubernetesexecutionv1alpha1.KubernetesExecution,
) ([]kubernetesexecutionv1alpha1.ConditionResult, error) {
    log := log.FromContext(ctx)

    postconditions := execution.Spec.PostConditions
    if len(postconditions) == 0 {
        return nil, nil // No postconditions to verify
    }

    results := make([]kubernetesexecutionv1alpha1.ConditionResult, 0, len(postconditions))

    for _, condition := range postconditions {
        log.Info("Verifying action postcondition", "type", condition.Type)

        result := e.verifyPostconditionAsync(ctx, condition, execution)
        results = append(results, result)

        // If required postcondition failed, mark execution as failed
        if condition.Required && !result.Passed {
            return results, fmt.Errorf("required postcondition failed: %s", condition.Type)
        }
    }

    return results, nil
}

// evaluateCondition evaluates a single condition (shared by pre/post)
func (e *PolicyEngine) evaluateCondition(
    ctx context.Context,
    condition kubernetesexecutionv1alpha1.ActionCondition,
    execution *kubernetesexecutionv1alpha1.KubernetesExecution,
) kubernetesexecutionv1alpha1.ConditionResult {
    result := kubernetesexecutionv1alpha1.ConditionResult{
        ConditionType:  condition.Type,
        Evaluated:      true,
        EvaluationTime: metav1.Now(),
    }

    // Query cluster state
    state, err := e.queryClusterState(ctx, condition.Type, execution)
    if err != nil {
        result.Passed = false
        result.ErrorMessage = fmt.Sprintf("Failed to query cluster state: %v", err)
        return result
    }

    // Evaluate Rego policy
    query := rego.New(
        rego.Query("data.condition.allow"),
        rego.Module(condition.Type+".rego", condition.Rego),
    )

    rs, err := query.Eval(ctx, rego.EvalInput(state))
    if err != nil {
        result.Passed = false
        result.ErrorMessage = fmt.Sprintf("Policy evaluation failed: %v", err)
        return result
    }

    // Check result
    if len(rs) == 0 || len(rs[0].Expressions) == 0 {
        result.Passed = false
        result.ErrorMessage = "Policy returned no result"
        return result
    }

    allowed, ok := rs[0].Expressions[0].Value.(bool)
    if !ok {
        result.Passed = false
        result.ErrorMessage = "Policy result is not boolean"
        return result
    }

    result.Passed = allowed
    if !allowed && condition.Required {
        result.ErrorMessage = fmt.Sprintf("Required condition %s failed", condition.Type)
    }

    return result
}

// verifyPostconditionAsync performs async verification with timeout
func (e *PolicyEngine) verifyPostconditionAsync(
    ctx context.Context,
    condition kubernetesexecutionv1alpha1.ActionCondition,
    execution *kubernetesexecutionv1alpha1.KubernetesExecution,
) kubernetesexecutionv1alpha1.ConditionResult {
    timeout, _ := time.ParseDuration(condition.Timeout)
    if timeout == 0 {
        timeout = 30 * time.Second // Default timeout
    }

    deadline := time.Now().Add(timeout)

    for time.Now().Before(deadline) {
        result := e.evaluateCondition(ctx, condition, execution)

        if result.Passed {
            return result // Success
        }

        // Wait before retry
        time.Sleep(5 * time.Second)
    }

    // Timeout reached
    return kubernetesexecutionv1alpha1.ConditionResult{
        ConditionType:  condition.Type,
        Evaluated:      true,
        Passed:         false,
        ErrorMessage:   fmt.Sprintf("Verification timeout: condition not met within %s", condition.Timeout),
        EvaluationTime: metav1.Now(),
    }
}
```

#### 4.3.3 Clear Separation: Safety Policies vs Conditions

**Architectural Principle**: Maintain distinct concerns

| Aspect | Safety Policies (Existing) | Preconditions (NEW) | Postconditions (NEW) |
|---|---|---|---|
| **Purpose** | Security and organizational constraints | Business prerequisites | Business verification |
| **Evaluation** | Synchronous, blocking | Synchronous, blocking | **Asynchronous**, verification |
| **Phase** | `reconcileValidating` | `reconcileValidating` | `reconcileExecuting` (after Job) |
| **Examples** | RBAC, resource limits, namespace restrictions | Image secrets, node availability | Pod health, no crashloops |
| **Package** | `data.safety.allow` | `data.condition.allow` | `data.condition.allow` |

**Benefits**:
- Clear intent and purpose
- Independent policy evolution
- Easier troubleshooting (which validation failed?)
- Separate metrics and monitoring

### 4.4 Phase 1: Reconciliation Integration (Days 18-20, 24 hours)

#### 4.4.1 Precondition Evaluation in Validating Phase

**File**: `internal/controller/kubernetesexecution_controller.go`

**Integration Point**: `reconcileValidating()` phase (EXTEND EXISTING)

```go
func (r *KubernetesExecutionReconciler) reconcileValidating(
    ctx context.Context,
    execution *kubernetesexecutionv1alpha1.KubernetesExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // ... existing validation (parameter, RBAC, resource existence) ...

    // Existing: Safety policy evaluation (Day 4)
    policyResult, err := r.PolicyEngine.EvaluateSafetyPolicy(ctx, execution)
    if err != nil || !policyResult.Allowed {
        log.Error(err, "Safety policy evaluation failed")
        execution.Status.Phase = "denied"
        return ctrl.Result{}, r.Status().Update(ctx, execution)
    }

    // NEW: Action precondition evaluation
    if len(execution.Spec.PreConditions) > 0 {
        log.Info("Evaluating action preconditions")

        conditionResults, err := r.PolicyEngine.EvaluateActionConditions(ctx, execution)

        // Update validation results with precondition outcomes
        if execution.Status.ValidationResults == nil {
            execution.Status.ValidationResults = &kubernetesexecutionv1alpha1.ValidationResults{}
        }
        execution.Status.ValidationResults.PreConditionResults = conditionResults

        if err != nil {
            // Required precondition failed - block execution
            log.Error(err, "Precondition evaluation failed")

            execution.Status.Phase = "blocked"
            execution.Status.Message = fmt.Sprintf("Precondition failed: %v", err)

            return ctrl.Result{}, r.Status().Update(ctx, execution)
        }

        log.Info("Action preconditions passed", "results", len(conditionResults))
    }

    // Existing: Dry-run execution
    dryRunResult, err := r.Executor.DryRun(ctx, execution)
    // ... existing dry-run logic ...

    // All validation passed - transition to running
    execution.Status.Phase = "running"
    return ctrl.Result{}, r.Status().Update(ctx, execution)
}
```

#### 4.4.2 Postcondition Verification in Executing Phase

**Integration Point**: `reconcileExecuting()` phase (after Job completion)

```go
func (r *KubernetesExecutionReconciler) reconcileExecuting(
    ctx context.Context,
    execution *kubernetesexecutionv1alpha1.KubernetesExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // ... existing Job monitoring logic ...

    // Check if Job completed
    job := &batchv1.Job{}
    if err := r.Get(ctx, jobKey, job); err != nil {
        return ctrl.Result{}, err
    }

    if job.Status.Succeeded > 0 {
        log.Info("Job completed successfully")

        // NEW: Verify action postconditions
        if len(execution.Spec.PostConditions) > 0 {
            log.Info("Verifying action postconditions")

            conditionResults, err := r.PolicyEngine.VerifyActionPostconditions(ctx, execution)

            // Update validation results with postcondition outcomes
            if execution.Status.ValidationResults == nil {
                execution.Status.ValidationResults = &kubernetesexecutionv1alpha1.ValidationResults{}
            }
            execution.Status.ValidationResults.PostConditionResults = conditionResults

            if err != nil {
                // Required postcondition failed - mark execution as failed
                log.Error(err, "Postcondition verification failed")

                execution.Status.Phase = "failed"
                execution.Status.Message = fmt.Sprintf("Postcondition failed: %v", err)
                execution.Status.Outcome = "failure"

                // Capture rollback information
                execution.Status.RollbackInfo = &kubernetesexecutionv1alpha1.RollbackInfo{
                    Reason: "Postcondition verification failed",
                    // ... existing rollback info capture ...
                }

                return ctrl.Result{}, r.Status().Update(ctx, execution)
            }

            log.Info("Action postconditions verified", "results", len(conditionResults))
        }

        // All postconditions passed - mark as succeeded
        execution.Status.Phase = "succeeded"
        execution.Status.Outcome = "success"
        return ctrl.Result{}, r.Status().Update(ctx, execution)
    }

    // Job still running - requeue
    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}
```

### 4.5 Phase 2: scale_deployment Action Example (Days 21-25, 40 hours) (DEPRECATED - ADR-025)

#### 4.5.1 Action-Level Precondition Policies

**Precondition 1: image_pull_secrets_valid**

```yaml
type: image_pull_secrets_valid
description: "Image pull secrets must be valid and accessible"
rego: |
  package condition.scale_deployment
  import future.keywords.if

  allow if {
    input.image_pull_secrets_exist == true
    input.image_pull_secrets_valid == true
  }
required: true
timeout: "10s"
```

**Precondition 2: node_selector_matches**

```yaml
type: node_selector_matches
description: "Nodes matching selector must be available"
rego: |
  package condition.scale_deployment
  import future.keywords.if

  allow if {
    input.matching_nodes > 0
    input.ready_nodes > 0
  }
required: false  # Optional - log warning if no matching nodes
timeout: "10s"
```

#### 4.5.2 Action-Level Postcondition Policies

**Postcondition 1: no_crashloop_pods**

```yaml
type: no_crashloop_pods
description: "No pods should be in CrashLoopBackOff state"
rego: |
  package condition.scale_deployment
  import future.keywords.if

  allow if {
    crashloop_pods := [p | p := input.pods[_]; p.status == "CrashLoopBackOff"]
    count(crashloop_pods) == 0
  }
required: true
timeout: "2m"
```

**Postcondition 2: resource_usage_acceptable**

```yaml
type: resource_usage_acceptable
description: "Pods should not be throttled or OOMKilled"
rego: |
  package condition.scale_deployment
  import future.keywords.if

  allow if {
    throttled_pods := [p | p := input.pods[_]; p.throttled == true]
    oom_killed_pods := [p | p := input.pods[_]; p.oom_killed == true]

    count(throttled_pods) == 0
    count(oom_killed_pods) == 0
  }
required: true
timeout: "2m"
```

#### 4.5.3 Integration with Existing scale_deployment Action

**File**: `pkg/kubernetesexecution/actions/scale_deployment.go` (Day 6 implementation)

**No changes needed to action implementation** - conditions are evaluated by controller reconciliation loop, not by individual actions.

**Action Definition Update**:

```go
// ScaleDeploymentAction definition with suggested conditions
var ScaleDeploymentAction = &catalog.ActionDefinition{
    Name:        "scale_deployment",
    Description: "Scale deployment to specified replica count",
    Category:    "deployment",

    // Suggested preconditions (operators can override)
    SuggestedPreConditions: []string{
        "deployment_exists",           // From WorkflowExecution
        "cluster_capacity_available",  // From WorkflowExecution
        "image_pull_secrets_valid",    // Action-specific
    },

    // Suggested postconditions
    SuggestedPostConditions: []string{
        "desired_replicas_running",   // From WorkflowExecution
        "deployment_health_check",    // From WorkflowExecution
        "no_crashloop_pods",          // Action-specific
        "resource_usage_acceptable",  // Action-specific
    },

    // ... existing fields (Parameters, RBACRules, etc.) ...
}
```

---

## 5. Representative Example: scale_deployment

### 5.1 Complete Defense-in-Depth Validation Flow

The scale_deployment action demonstrates **8-layer validation** combining WorkflowExecution and KubernetesExecutor conditions:

```
WORKFLOW STEP LEVEL (WorkflowExecution Controller):
1. Parameter validation (existing)
2. RBAC validation (existing)
3. Step preconditions (NEW - 3 policies):
   ✓ deployment_exists (blocking)
   ✓ cluster_capacity_available (blocking)
   ⚠ current_replicas_match (warning)
4. Create KubernetesExecution CRD

ACTION LEVEL (KubernetesExecutor (DEPRECATED - ADR-025) Controller):
5. Parameter validation (existing)
6. RBAC validation (existing)
7. Safety policy validation (existing - Day 4)
8. Action preconditions (NEW - 2 policies):
   ✓ image_pull_secrets_valid (blocking)
   ⚠ node_selector_matches (warning)
9. Dry-run execution (existing)
10. Create Kubernetes Job

EXECUTION:
11. Job executes kubectl scale command
12. Monitor Job completion

POST-EXECUTION VERIFICATION:
13. Action postconditions (NEW - 2 policies):
    ✓ no_crashloop_pods (blocking, async)
    ✓ resource_usage_acceptable (blocking, async)
14. Mark KubernetesExecution complete

WORKFLOW VERIFICATION:
15. Step postconditions (NEW - 2 policies):
    ✓ desired_replicas_running (blocking, async)
    ✓ deployment_health_check (blocking, async)
16. Mark workflow step complete
```

### 5.2 Condition Policy Summary

**Total: 8 unique condition policies**

| Condition | Level | Type | Required | Timeout | Purpose |
|---|---|---|---|---|---|
| `deployment_exists` | Step | Pre | Yes | 10s | Verify deployment before scaling |
| `cluster_capacity_available` | Step | Pre | Yes | 10s | Check resource availability |
| `current_replicas_match` | Step | Pre | No | 5s | Baseline validation (warning) |
| `image_pull_secrets_valid` | Action | Pre | Yes | 10s | Validate image pull configuration |
| `node_selector_matches` | Action | Pre | No | 10s | Verify node availability (warning) |
| `no_crashloop_pods` | Action | Post | Yes | 2m | Confirm no CrashLoopBackOff |
| `resource_usage_acceptable` | Action | Post | Yes | 2m | Check for throttling/OOM |
| `desired_replicas_running` | Step | Post | Yes | 2m | Verify replica count (shared with action) |
| `deployment_health_check` | Step | Post | Yes | 2m | Confirm deployment health (shared with action) |

**Note**: `desired_replicas_running` and `deployment_health_check` are evaluated at both levels for defense-in-depth.

### 5.3 Example WorkflowExecution CR with Conditions

```yaml
apiVersion: workflow.kubernaut.io/v1alpha1
kind: WorkflowExecution
metadata:
  name: scale-web-app-workflow
  namespace: production
spec:
  workflowDefinition:
    steps:
    - stepNumber: 1
      action: scale_deployment
      parameters:
        deployment:
          name: web-app
          namespace: production
          replicas: 5
      preConditions:
      - type: deployment_exists
        description: "Deployment must exist before scaling"
        rego: |
          package precondition.scale_deployment
          import future.keywords.if
          allow if { input.deployment_found == true }
        required: true
        timeout: "10s"
      - type: cluster_capacity_available
        description: "Cluster must have capacity"
        rego: |
          package precondition.scale_deployment
          import future.keywords.if
          allow if {
            cpu_available := parse_cpu(input.available_cpu)
            cpu_needed := parse_cpu(input.required_cpu_per_pod) * input.additional_replicas
            memory_available := parse_memory(input.available_memory)
            memory_needed := parse_memory(input.required_memory_per_pod) * input.additional_replicas
            cpu_available >= cpu_needed
            memory_available >= memory_needed
          }
          # Helper functions omitted for brevity
        required: true
        timeout: "10s"
      postConditions:
      - type: desired_replicas_running
        description: "All replicas must be running"
        rego: |
          package postcondition.scale_deployment
          import future.keywords.if
          allow if {
            input.running_pods >= input.target_replicas
            input.ready_pods >= input.target_replicas
          }
        required: true
        timeout: "2m"
      - type: deployment_health_check
        description: "Deployment must be healthy"
        rego: |
          package postcondition.scale_deployment
          import future.keywords.if
          allow if {
            input.conditions.Available == true
            input.conditions.Progressing == true
          }
        required: true
        timeout: "2m"
```

---

## 6. Timeline Impact Analysis

### 6.1 WorkflowExecution Extended Timeline

**Base Controller** (v1.0): 12-13 days (96-104 hours)

**Validation Framework Extensions**:

| Phase | Days | Hours | Activities |
|---|---|---|---|
| **Phase 1: Foundation** | Days 14-20 | 56h | CRD schema, Rego integration, reconciliation |
| **Phase 2: scale_deployment** | Days 21-22 | 16h | Step conditions, ConfigMap, tests |
| **Phase 3: Testing** | Days 23-26 | 32h | Integration tests, E2E validation |
| **Documentation** | Day 27 | 8h | Operator guides |

**Total Extended Timeline**: **27-30 days** (216-240 hours)
- Base controller: 12-13 days
- Validation integration: +15-17 days

### 6.2 KubernetesExecutor Extended Timeline (DEPRECATED - ADR-025)

**Base Controller** (v1.0): 11-12 days (88-96 hours)

**Validation Framework Extensions**:

| Phase | Days | Hours | Activities |
|---|---|---|---|
| **Phase 1: Foundation** | Days 13-20 | 64h | CRD schema, safety engine extension, reconciliation |
| **Phase 2: scale_deployment** | Days 21-25 | 40h | Action conditions, integration, tests |
| **Phase 3: Testing** | Days 26-27 | 16h | Integration and E2E tests |
| **Documentation** | Day 28 | 8h | Condition templates |

**Total Extended Timeline**: **25-28 days** (200-224 hours)
- Base controller: 11-12 days
- Validation integration: +14-16 days

### 6.3 Critical Path Analysis

**Assuming parallelization where possible**:

```
Phase 0: Base Controllers
├─ WorkflowExecution: Days 1-13 (can run in parallel)
└─ KubernetesExecutor: Days 1-12 (can run in parallel)
Total: 12-13 days (longest wins)

Phase 1: Validation Framework Foundation
├─ WorkflowExecution: Days 14-20 (sequential)
└─ KubernetesExecutor: Days 13-20 (sequential)
Integration point: Day 20 (both controllers ready)
Total: 7-8 days (longest wins, starting from Day 13)

Phase 2: scale_deployment Representative Example
├─ WorkflowExecution: Days 21-22 (2 days)
└─ KubernetesExecutor: Days 21-25 (5 days)
Coordination needed: E2E tests require both
Total: 5 days (longest wins)

Phase 3: Integration Testing & Validation
├─ Both controllers tested together
├─ Days 26-27 minimum
Total: 5-7 days

Phase 4: Phased Rollout
├─ Canary: Weeks 1-2
├─ Staging: Weeks 3-4
├─ Production: Weeks 5-8
Total: 6-8 weeks (parallel to operations)
```

**Grand Total Development Timeline**: 29-37 days
- Optimistic (with full parallelization): 29 days
- Realistic (with coordination overhead): 32-35 days
- Conservative (with some serialization): 37 days

### 6.4 Resource Requirements

**Development Team**:
- **Phase 0** (Base Controllers): 1-2 developers (can parallelize)
- **Phase 1-2** (Validation Framework): 1 developer (sequential dependencies)
- **Phase 3** (Testing): 1 developer + 1 QA engineer
- **Phase 4** (Rollout): 1 developer + 1 SRE

**Infrastructure**:
- Kind clusters for integration testing
- Staging environment for Phase 3
- Production clusters for Phase 4 rollout
- Monitoring/observability infrastructure

---

## 7. Risk Mitigation Strategy

### 7.1 Risk 1: False Positives >15% (Approved Mitigation)

**Probability**: 60% | **Impact**: HIGH | **Mitigation Confidence**: 75%

**Mitigation Strategy**:
1. **Start with Warning Mode**:
   - All new conditions: `required: false` during Phase 4 Week 1-2
   - Monitor false positive rate per condition type
   - Gradually change to `required: true` based on telemetry

2. **Operator Override Mechanism**:
   ```yaml
   metadata:
     annotations:
       kubernaut.io/override-condition: "cluster_capacity_available"
   ```

3. **Condition Tuning Dashboard**:
   - Grafana dashboard showing per-condition failure rates
   - Historical trends and false positive detection
   - Suggested policy adjustments

4. **Telemetry-Driven Tuning**:
   - Week 1-2: Collect baseline false positive rates
   - Week 3-4: Tune condition policies based on data
   - Week 5-8: Gradually tighten conditions

**Success Criteria**: False positive rate <15% after Week 4

### 7.2 Risk 2: Performance Impact >5s/step (Approved Mitigation)

**Probability**: 40% | **Impact**: MEDIUM | **Mitigation Confidence**: 80%

**Mitigation Strategy**:
1. **Async Verification**:
   - Postconditions use async polling (not blocking reconciliation)
   - Configurable timeout per condition
   - Default timeout: 30s (preconditions), 2m (postconditions)

2. **Cluster State Caching**:
   - Cache cluster state queries for 5-10 seconds
   - Reduce Kubernetes API load
   - Shared cache across conditions in same reconciliation

3. **Parallel Condition Evaluation**:
   - Evaluate independent preconditions in parallel
   - Use goroutines with timeout context
   - Aggregate results before proceeding

4. **Aggressive Timeouts**:
   - Non-critical conditions: 5-10s timeout
   - Critical conditions: 30s-2m timeout
   - Fail fast on timeout

**Performance Targets**:
- Precondition evaluation: <1s per condition
- Postcondition verification: <5s per condition (excluding wait time)
- Total step overhead: <5s

**Validation**: Integration tests measure actual timing

### 7.3 Risk 3: Integration Complexity (Approved Mitigation)

**Probability**: 50% | **Impact**: MEDIUM | **Mitigation Confidence**: 85%

**Mitigation Strategy**:
1. **Clear Separation of Concerns**:
   - Safety policies: `data.safety.allow`
   - Preconditions: `data.precondition.allow`
   - Postconditions: `data.postcondition.allow`
   - Different Rego packages prevent collision

2. **Extend, Don't Rebuild**:
   - Leverage existing Day 4 safety engine
   - Add new methods, don't modify existing
   - Reuse PolicyEngine struct and infrastructure
   - ~30% implementation time reduction

3. **Integration Testing**:
   - Test safety policies + conditions together
   - Verify evaluation order (safety → preconditions → dry-run)
   - Ensure no interference between systems

4. **Documentation**:
   - Clear architecture diagrams
   - Integration point documentation
   - Troubleshooting guides

**Success Criteria**: Day 15-17 integration completed without safety policy regression

### 7.4 Risk 4: Operator Learning Curve (Approved Mitigation)

**Probability**: 70% | **Impact**: LOW | **Mitigation Confidence**: 90%

**Mitigation Strategy**:
1. **Comprehensive Operator Documentation**:
   - "How to Define Custom Conditions" guide
   - Rego policy writing tutorial
   - Common condition patterns library

2. **Representative Example**:
   - scale_deployment with complete 8-policy suite
   - Annotated policies with comments
   - Copy-paste templates for similar actions

3. **Condition Template Library**:
   - Reusable condition templates
   - Common validation patterns
   - Resource calculation helpers

4. **Support Channels**:
   - Office hours for condition authoring
   - Slack channel for questions
   - Documented escalation path

**Success Criteria**: 80% of operators can author conditions after reviewing documentation

### 7.5 Risk 5: Maintenance Burden (Approved Mitigation)

**Probability**: 80% | **Impact**: LOW | **Mitigation Confidence**: 75%

**Mitigation Strategy**:
1. **Reusable Condition Libraries**:
   ```rego
   # lib/deployment_common.rego
   package lib.deployment
   import future.keywords.if

   deployment_exists(input) if { input.deployment_found == true }
   deployment_healthy(input) if {
     input.conditions.Available == true
     input.conditions.Progressing == true
   }
   ```

2. **Automated Testing**:
   - Unit tests for each condition policy
   - Integration tests for condition suites
   - Rego policy syntax validation in CI/CD

3. **Policy Versioning**:
   - Version conditions in ConfigMap labels
   - Backwards compatibility guarantees
   - Migration guides for breaking changes

4. **Clear Ownership**:
   - CODEOWNERS file for condition policies
   - Review process for policy updates
   - Change approval workflow

5. **Condition Generator Tool**:
   ```bash
   kubernaut condition generate \
     --action scale_deployment \
     --type precondition \
     --check deployment_exists
   ```

**Success Criteria**: <2 hours/month maintenance per 10 conditions

---

## 8. Testing Strategy

### 8.1 Unit Tests (80%+ coverage target)

**Coverage Areas**:
- Condition type validation
- Rego policy evaluation logic
- Async verification timeout handling
- ConfigMap policy loading
- ConditionResult creation and validation
- Cluster state query utilities

**Example Unit Test**:

```go
func TestPreconditionEvaluation(t *testing.T) {
    tests := []struct {
        name           string
        condition      StepCondition
        clusterState   map[string]interface{}
        expectedPassed bool
        expectedError  string
    }{
        {
            name: "deployment_exists - success",
            condition: StepCondition{
                Type: "deployment_exists",
                Rego: `package precondition
                       import future.keywords.if
                       allow if { input.deployment_found == true }`,
                Required: true,
            },
            clusterState: map[string]interface{}{
                "deployment_found": true,
                "deployment_name":  "web-app",
            },
            expectedPassed: true,
        },
        {
            name: "deployment_exists - failure",
            condition: StepCondition{
                Type: "deployment_exists",
                Rego: `package precondition
                       import future.keywords.if
                       allow if { input.deployment_found == true }`,
                Required: true,
            },
            clusterState: map[string]interface{}{
                "deployment_found": false,
            },
            expectedPassed: false,
            expectedError:  "deployment_exists",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            engine := NewConditionEngine()
            result := engine.evaluateCondition(ctx, tt.condition, tt.clusterState)

            assert.Equal(t, tt.expectedPassed, result.Passed)
            if tt.expectedError != "" {
                assert.Contains(t, result.ErrorMessage, tt.expectedError)
            }
        })
    }
}
```

### 8.2 Integration Tests (60%+ coverage target)

**Coverage Areas**:
- Complete workflow with conditions
- Precondition blocking execution
- Postcondition triggering rollback
- ConfigMap policy hot-reload
- scale_deployment with all conditions
- Async verification with real Kubernetes resources

**Example Integration Test**:

```go
func TestScaleDeploymentWithConditions(t *testing.T) {
    // Setup Kind cluster
    cluster := setupKindCluster(t)
    defer cluster.Cleanup()

    // Create test deployment
    deployment := createTestDeployment(cluster, "web-app", 3)

    // Create WorkflowExecution with scale_deployment + conditions
    workflow := &workflowv1.WorkflowExecution{
        Spec: workflowv1.WorkflowExecutionSpec{
            WorkflowDefinition: workflowv1.WorkflowDefinition{
                Steps: []workflowv1.WorkflowStep{
                    {
                        StepNumber: 1,
                        Action:     "scale_deployment",
                        Parameters: /* ... */,
                        PreConditions: []workflowv1.StepCondition{
                            {Type: "deployment_exists", /* ... */},
                            {Type: "cluster_capacity_available", /* ... */},
                        },
                        PostConditions: []workflowv1.StepCondition{
                            {Type: "desired_replicas_running", /* ... */},
                            {Type: "deployment_health_check", /* ... */},
                        },
                    },
                },
            },
        },
    }

    // Create workflow
    err := cluster.Client.Create(context.TODO(), workflow)
    require.NoError(t, err)

    // Wait for completion
    require.Eventually(t, func() bool {
        _ = cluster.Client.Get(context.TODO(), client.ObjectKeyFromObject(workflow), workflow)
        return workflow.Status.Phase == "completed"
    }, 5*time.Minute, 10*time.Second)

    // Verify preconditions were evaluated
    stepStatus := workflow.Status.StepStatuses[0]
    assert.Len(t, stepStatus.PreConditionResults, 2)
    assert.True(t, stepStatus.PreConditionResults[0].Passed)

    // Verify postconditions were verified
    assert.Len(t, stepStatus.PostConditionResults, 2)
    assert.True(t, stepStatus.PostConditionResults[0].Passed)

    // Verify deployment actually scaled
    deployment = getDeployment(cluster, "web-app", "default")
    assert.Equal(t, int32(5), *deployment.Spec.Replicas)
}
```

### 8.3 E2E Tests (40%+ coverage target)

**Coverage Areas**:
- Complete remediation workflow with validation
- Multi-step workflow with cascading conditions
- Failure recovery scenarios
- Real-world scale_deployment remediation
- Performance validation

**Test Scenarios**:
1. **Success Path**: All conditions pass, workflow completes
2. **Precondition Blocking**: Required precondition fails, execution blocked
3. **Postcondition Failure**: Postcondition fails, rollback triggered
4. **Async Verification**: Postcondition takes time to pass
5. **Optional Condition Warning**: Non-required condition fails, logs warning
6. **ConfigMap Update**: Policy hot-reload works correctly

---

## 9. Documentation Requirements

### 9.1 Operator Documentation

**Operator Guide: Defining Custom Conditions** (~50 pages)

**Sections**:
1. **Introduction to Validation Framework**
   - What are preconditions and postconditions?
   - When to use step-level vs action-level conditions
   - Required vs optional conditions

2. **Rego Policy Writing Guide**
   - Rego basics for operators
   - Common patterns and idioms
   - Input schema structure
   - Testing policies locally

3. **Condition Template Library**
   - Reusable condition templates
   - Common validation patterns
   - Resource calculation helpers
   - Copy-paste examples

4. **scale_deployment Complete Example**
   - All 8 policies annotated
   - Input/output schema documented
   - Test scenarios explained

5. **Troubleshooting Guide**
   - Common condition failures
   - Debugging techniques
   - Performance tuning
   - False positive investigation

### 9.2 Developer Documentation

**Developer Guide: Extending Validation Framework** (~30 pages)

**Sections**:
1. **Architecture Overview**
   - Integration patterns
   - Shared infrastructure
   - Separation of concerns

2. **Adding New Condition Types**
   - Extending cluster state queries
   - Adding new Rego packages
   - Testing new conditions

3. **Performance Optimization**
   - Caching strategies
   - Async verification patterns
   - Profiling and benchmarking

4. **Maintenance Guidelines**
   - Policy versioning
   - Backwards compatibility
   - Breaking change process

---

## 10. Success Metrics & Validation

### 10.1 Phase Gates

**Phase 0 Complete**: Base controllers operational
- ✅ WorkflowExecution can orchestrate multi-step workflows
- ✅ KubernetesExecutor can execute actions via Jobs
- ✅ All base BRs covered (35 WF, 39 EXEC)
- ✅ Integration tests passing

**Phase 1 Complete**: Framework integrated
- ✅ CRD schemas extended with condition types
- ✅ Rego policy evaluation framework operational
- ✅ ConfigMap-based policy loading working
- ✅ Reconciliation phases integrated
- ✅ New BRs covered (BR-WF-016/052/053, BR-EXEC-016/036)
- ✅ Integration tests with framework passing

**Phase 2 Complete**: scale_deployment fully validated
- ✅ 8 condition policies implemented and tested
- ✅ Preconditions block execution as expected
- ✅ Postconditions verify success
- ✅ E2E tests demonstrate defense-in-depth
- ✅ Performance targets met (<5s per step)

**Phase 3 Complete**: Production ready
- ✅ 80%+ test coverage achieved
- ✅ Metrics and observability operational
- ✅ Operator documentation complete
- ✅ Feature flag implemented
- ✅ Rollout plan approved

**Phase 4 Complete**: Production validation successful
- ✅ Effectiveness improvement demonstrated (70%→85-90%)
- ✅ Cascade failure reduction demonstrated (30%→10%)
- ✅ False positive rate acceptable (<15%)
- ✅ Performance impact acceptable (<5s)

### 10.2 Acceptance Criteria

**Functional Requirements**:
- [ ] Preconditions can block workflow step execution
- [ ] Postconditions can trigger rollback
- [ ] Required conditions enforce blocking behavior
- [ ] Optional conditions log warnings only
- [ ] ConfigMap policy updates apply without restart
- [ ] Async verification respects timeout
- [ ] scale_deployment validates with 8 policies

**Non-Functional Requirements**:
- [ ] Test coverage ≥80% (unit), ≥60% (integration), ≥40% (E2E)
- [ ] Precondition evaluation <1s per condition
- [ ] Postcondition verification <5s per condition
- [ ] Total step overhead <5s
- [ ] Memory impact <50MB per controller replica
- [ ] CPU impact <0.1 cores average

**Observability Requirements**:
- [ ] Condition evaluation metrics exposed
- [ ] Performance impact metrics tracked
- [ ] False positive rate monitored
- [ ] Effectiveness improvement measured
- [ ] Grafana dashboards operational

### 10.3 Monitoring & Observability

**Key Metrics**:

```yaml
Condition Evaluation Metrics:
- condition_evaluations_total (counter)
  labels: [condition_type, service, action, required]

- condition_evaluation_duration_seconds (histogram)
  labels: [condition_type, service]
  buckets: [0.1, 0.5, 1, 2, 5, 10]

- condition_passed_total (counter)
  labels: [condition_type, service, action, required]

- condition_failed_total (counter)
  labels: [condition_type, service, action, required]

- condition_timeout_total (counter)
  labels: [condition_type, service]

Validation Impact Metrics:
- executions_blocked_by_precondition_total (counter)
  labels: [action, condition_type]

- executions_failed_by_postcondition_total (counter)
  labels: [action, condition_type]

- cascade_failures_prevented_total (counter)
  labels: [workflow_id]

Effectiveness Metrics:
- remediation_effectiveness_score (gauge)
  labels: [with_validation, without_validation]

- mttr_seconds (histogram)
  labels: [with_validation, without_validation]
  buckets: [60, 300, 600, 900, 1800, 3600]
```

**Grafana Dashboard**: "Validation Framework Overview"
- Condition evaluation rates (success/failure)
- Performance impact (evaluation duration)
- False positive trends
- Effectiveness improvement metrics
- Action-specific condition success rates

### 10.4 Rollout Success Criteria

**Week 1-2 (Canary 5%)**:
- False positive rate <20%
- No performance degradation >15%
- Effectiveness improvement >10%

**Week 3-4 (Staging 100%)**:
- False positive rate <15%
- Performance impact <5s per step
- Effectiveness improvement >15%

**Week 5-8 (Production Gradual)**:
- False positive rate <15%
- Cascade failure reduction >15%
- MTTR reduction >40%
- Manual intervention reduction >15%

---

## Conclusion

This integration guide provides a comprehensive roadmap for adding per-step precondition/postcondition validation to the remediation framework. The **Phased Enhancement** approach with **B→A sequential strategy** (guide first, then implementation plans) ensures:

- ✅ **90% overall confidence** through validated integration architecture
- ✅ **Clear separation of concerns** between safety policies and business conditions
- ✅ **Leverage existing infrastructure** (Day 4 safety engine reduces effort by ~30%)
- ✅ **Incremental value delivery** (scale_deployment demonstrates effectiveness)
- ✅ **Risk mitigation** (all 5 risks explicitly addressed with approved strategies)
- ✅ **Single source of truth** for integration decisions

**Next Steps**:
1. ✅ Integration guide approved (this document)
2. 🔲 Update WorkflowExecution Implementation Plan (Task 2)
3. 🔲 Update KubernetesExecutor (DEPRECATED - ADR-025) Implementation Plan (Task 3)
4. 🔲 Begin Phase 0: Base controller implementation
5. 🔲 Execute Phases 1-4 according to timeline
6. 🔲 Monitor success metrics and adjust

**Success Indicators**:
- Remediation effectiveness: 70% → 85-90% (+15-20%)
- Cascade failure rate: 30% → <10% (-20%)
- MTTR: 15 min → <8 min (-47%)
- Manual intervention: 40% → 20% (-20%)
- False positive rate: <15% (acceptable)

---

**Document Status**: ✅ Complete and Ready for Implementation

