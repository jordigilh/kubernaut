# Precondition/Postcondition Validation Framework - Implementation Plan

**Version**: 1.0
**Status**: ✅ Approved
**Design Decision**: [DD-002 - Per-Step Validation Framework](../../architecture/DESIGN_DECISIONS.md#dd-002-per-step-validation-framework-alternative-2)
**Business Requirements**: [STEP_VALIDATION_BUSINESS_REQUIREMENTS.md](../../requirements/STEP_VALIDATION_BUSINESS_REQUIREMENTS.md)
**Last Updated**: 2025-10-14
**Estimated Duration**: 5-6 weeks (33 days development)
**Confidence**: 78%

---

## Executive Summary

### Purpose
Implement a per-step precondition/postcondition validation framework across WorkflowExecution and KubernetesExecutor (DEPRECATED - ADR-025) services to improve remediation effectiveness from 70% to 85-90% through defense-in-depth validation.

### Scope
- **Architecture**: Framework design with StepCondition/ActionCondition types, Rego policy integration
- **Representative Example**: Complete implementation for `scale_deployment` action
- **Placeholders**: Condition templates for remaining 26 actions defined during implementation
- **Timeline**: 5-6 weeks phased implementation

### Expected Outcomes
- ✅ **Remediation Effectiveness**: 70% → 85-90% (+15-20%)
- ✅ **Cascade Failure Prevention**: 30% → 10% (-20%)
- ✅ **Reduced MTTR**: 15 min → 8 min (-47%)
- ✅ **Less Manual Intervention**: 40% → 20% (-20%)

### Key Business Value
- **3-month ROI**: 10 hours/month saved × $100/hr = $1000/month benefit, $10K investment
- **Improved Observability**: Step-level failure diagnosis with state evidence
- **Leverages Existing Infrastructure**: Reuses Rego policy engine (BR-REGO-001 to BR-REGO-010)

---

## Architecture Overview

### Design Decision Reference
**DD-002: Per-Step Validation Framework (Alternative 2)**
- **Status**: ✅ Approved (2025-10-14)
- **Confidence**: 78%
- **Full Rationale**: [DD-002](../../architecture/DESIGN_DECISIONS.md#dd-002-per-step-validation-framework-alternative-2)

### CRD Schema Changes

#### WorkflowExecution CRD
```go
// WorkflowStep extensions
type WorkflowStep struct {
    // ... existing fields ...
    PreConditions  []StepCondition `json:"preConditions,omitempty"`  // BR-WF-016
    PostConditions []StepCondition `json:"postConditions,omitempty"` // BR-WF-052
}

// New types
type StepCondition struct {
    Type        string `json:"type"`
    Description string `json:"description"`
    Rego        string `json:"rego"`
    Required    bool   `json:"required"`
    Timeout     string `json:"timeout,omitempty"`
}

// Status extensions
type StepStatus struct {
    // ... existing fields ...
    PreConditionResults  []ConditionResult `json:"preConditionResults,omitempty"`
    PostConditionResults []ConditionResult `json:"postConditionResults,omitempty"`
}

type ConditionResult struct {
    ConditionType   string      `json:"conditionType"`
    Evaluated       bool        `json:"evaluated"`
    Passed          bool        `json:"passed"`
    ErrorMessage    string      `json:"errorMessage,omitempty"`
    EvaluationTime  metav1.Time `json:"evaluationTime"`
}
```

#### KubernetesExecution (DEPRECATED - ADR-025) CRD
```go
// KubernetesExecutionSpec extensions
type KubernetesExecutionSpec struct {
    // ... existing fields ...
    PreConditions  []ActionCondition `json:"preConditions,omitempty"`  // BR-EXEC-016
    PostConditions []ActionCondition `json:"postConditions,omitempty"` // BR-EXEC-036
}

// ActionCondition identical to StepCondition (semantically scoped to actions)
type ActionCondition struct {
    Type        string `json:"type"`
    Description string `json:"description"`
    Rego        string `json:"rego"`
    Required    bool   `json:"required"`
    Timeout     string `json:"timeout,omitempty"`
}

// ValidationResults extensions
type ValidationResults struct {
    // ... existing fields ...
    PreConditionResults  []ConditionResult `json:"preConditionResults,omitempty"`
    PostConditionResults []ConditionResult `json:"postConditionResults,omitempty"`
}
```

### Validation Mechanism

#### Rego Policy Integration
**Leverages Existing Infrastructure**: BR-REGO-001 to BR-REGO-010
- **Policy Source**: ConfigMap (`kubernaut-workflow-conditions`)
- **Policy Format**: Rego decision-making language
- **Evaluation Engine**: Reuse existing Rego evaluator from KubernetesExecutor (DEPRECATED - ADR-025)
- **Policy Loading**: Watch-based ConfigMap updates for hot-reload

#### Cluster State Queries
**Data Sources**:
- Kubernetes API: Deployment status, pod counts, resource availability
- Metrics API: CPU/memory usage, pod health metrics
- Custom Resources: Application-specific state validation

#### Async Verification
**Pattern**: Poll-based verification with configurable timeout
```go
// Postcondition async verification example
func (r *WorkflowExecutionReconciler) verifyPostcondition(
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
        ErrorMessage: "Verification timeout: condition not met within " + condition.Timeout,
    }, nil
}
```

### Data Flow

```
WorkflowExecution Controller:
1. Evaluate step.preConditions[] before creating KubernetesExecution (DEPRECATED - ADR-025) CRD
   - Rego policy evaluation with current cluster state
   - Block if required=true condition fails
   - Log warning if required=false condition fails
2. Create KubernetesExecution (DEPRECATED - ADR-025) CRD (only if preconditions pass)

KubernetesExecutor (DEPRECATED - ADR-025) Controller:
3. Evaluate spec.preConditions[] during validating phase
   - Additional action-specific validation
   - Integrated with existing dry-run validation
4. Create Kubernetes Job (only if preconditions pass)
5. Monitor Job execution
6. Evaluate spec.postConditions[] after Job completion
   - Query cluster state to verify outcome
   - Wait up to condition.timeout for convergence
   - Mark failed if required=true postcondition fails

WorkflowExecution Controller:
7. Detect KubernetesExecution (DEPRECATED - ADR-025) completion
8. Evaluate step.postConditions[] during monitoring phase
   - Workflow-level verification
   - Aggregate results across steps
   - Inform effectiveness score
```

---

## Implementation Phases

### Phase 1: CRD Schema Extensions (Week 1)
**Duration**: 3-5 days
**Deliverables**:
- Update `api/workflowexecution/v1alpha1/workflowexecution_types.go`
- Update `api/kubernetesexecution/v1alpha1/kubernetesexecution_types.go`
- Add StepCondition, ActionCondition, ConditionResult types
- Update kubebuilder markers and regenerate CRDs
- Unit tests for type validation

**Success Criteria**:
- ✅ CRD schema changes compile and generate correctly
- ✅ Backwards compatible (new fields are optional)
- ✅ Unit tests validate type constraints

### Phase 2: Validation Framework (Weeks 1-2)
**Duration**: 7-10 days
**Deliverables**:
- Rego policy evaluator wrapper (reuse from KubernetesExecutor (DEPRECATED - ADR-025))
- Cluster state query utilities
- Async verification framework with timeout handling
- ConfigMap-based policy loading (BR-WF-053)
- Integration with WorkflowExecution reconciliation phases
- Integration with KubernetesExecutor (DEPRECATED - ADR-025) reconciliation phases

**Success Criteria**:
- ✅ Rego policies can be evaluated with cluster state input
- ✅ Async verification respects timeouts
- ✅ Condition results properly recorded in status
- ✅ Unit tests for evaluation logic

### Phase 3: Representative Example - scale_deployment (Weeks 2-3)
**Duration**: 5-7 days
**Deliverables**:
- Complete precondition Rego policies for scale_deployment
  - deployment_exists: Verify deployment exists before scaling
  - current_replicas_match: Check current replicas match baseline
  - cluster_capacity_available: Verify cluster has capacity
  - image_pull_secrets_valid: Validate image pull secrets
  - node_selector_matches: Check node selector requirements
- Complete postcondition Rego policies
  - desired_replicas_running: Verify all desired replicas running
  - deployment_health_check: Confirm deployment Available and Progressing
  - no_crashloop_pods: Ensure no pods in CrashLoopBackOff
  - resource_usage_acceptable: Check pods not throttled or OOMKilled
- Integration with existing dry-run validation
- E2E test scenarios for scale_deployment

**Success Criteria**:
- ✅ scale_deployment action validates preconditions before execution
- ✅ scale_deployment action verifies postconditions after completion
- ✅ Required preconditions block execution when failing
- ✅ Optional preconditions log warnings when failing
- ✅ E2E tests demonstrate complete validation flow

**scale_deployment Condition Examples**:
```yaml
# Precondition: deployment_exists
type: deployment_exists
description: "Deployment must exist before scaling"
rego: |
  package precondition
  import future.keywords.if
  allow if { input.deployment_found == true }
required: true
timeout: "10s"

# Postcondition: desired_replicas_running
type: desired_replicas_running
description: "All desired replicas must be running"
rego: |
  package postcondition
  import future.keywords.if
  allow if {
    input.running_pods >= input.target_replicas
    input.ready_pods >= input.target_replicas
  }
required: true
timeout: "2m"
```

### Phase 4: Integration Testing (Weeks 3-4)
**Duration**: 5-7 days
**Deliverables**:
- Integration tests for WorkflowExecution precondition evaluation
- Integration tests for WorkflowExecution postcondition verification
- Integration tests for KubernetesExecutor (DEPRECATED - ADR-025) precondition evaluation
- Integration tests for KubernetesExecutor (DEPRECATED - ADR-025) postcondition verification
- Integration tests for ConfigMap-based policy loading
- Integration tests for async verification with timeout
- Failure scenario tests (required condition fails, cascading failures prevented)

**Success Criteria**:
- ✅ Integration tests pass with real Kubernetes clusters
- ✅ Precondition failures block execution as expected
- ✅ Postcondition failures trigger appropriate workflow handling
- ✅ Policy updates from ConfigMap applied correctly
- ✅ Timeout handling works for async verification

### Phase 5: Documentation (Weeks 4-5)
**Duration**: 5-7 days
**Deliverables**:
- ✅ DD-002 design decision (COMPLETE)
- ✅ BR specifications (COMPLETE)
- ✅ CRD schema documentation updates (COMPLETE)
- ✅ Reconciliation flow documentation updates (COMPLETE)
- ✅ Representative examples (COMPLETE)
- Operator guide for defining custom conditions
- Troubleshooting guide for condition failures
- Performance tuning guide
- Migration guide for existing workflows

**Success Criteria**:
- ✅ Documentation covers framework architecture
- ✅ scale_deployment example fully documented
- ✅ Placeholder notes for remaining actions
- ✅ Operator guides provide clear usage instructions

### Phase 6: Rollout Preparation (Weeks 5-6)
**Duration**: 3-5 days
**Deliverables**:
- Feature flag for enabling validation framework
- Telemetry and observability instrumentation
- Alerting for false positives (condition failures)
- Dashboard for condition evaluation metrics
- Gradual rollout plan (canary → production)
- Rollback procedures

**Success Criteria**:
- ✅ Feature flag controls validation framework activation
- ✅ Metrics track condition evaluation success/failure rates
- ✅ Alerts fire when false positive threshold exceeded
- ✅ Rollback plan tested in staging

---

## Representative Example: scale_deployment

### Complete Precondition Rego Policies

#### 1. deployment_exists
```rego
package precondition.scale_deployment

import future.keywords.if

# Input schema expected:
# {
#   "deployment_found": true/false,
#   "deployment_name": "web-app",
#   "namespace": "production"
# }

allow if {
    input.deployment_found == true
}
```

#### 2. current_replicas_match
```rego
package precondition.scale_deployment

import future.keywords.if

# Input schema:
# {
#   "current_replicas": 3,
#   "expected_baseline": 3
# }

allow if {
    input.current_replicas == input.expected_baseline
}
```

#### 3. cluster_capacity_available
```rego
package precondition.scale_deployment

import future.keywords.if

# Input schema:
# {
#   "available_cpu": "2000m",
#   "available_memory": "4Gi",
#   "required_cpu_per_pod": "500m",
#   "required_memory_per_pod": "1Gi",
#   "additional_replicas": 2
# }

allow if {
    cpu_available_millicores := parse_cpu(input.available_cpu)
    cpu_needed_millicores := parse_cpu(input.required_cpu_per_pod) * input.additional_replicas

    memory_available_bytes := parse_memory(input.available_memory)
    memory_needed_bytes := parse_memory(input.required_memory_per_pod) * input.additional_replicas

    cpu_available_millicores >= cpu_needed_millicores
    memory_available_bytes >= memory_needed_bytes
}

# Helper functions for resource parsing
parse_cpu(cpu_string) := millicores if {
    endswith(cpu_string, "m")
    millicores := to_number(trim_suffix(cpu_string, "m"))
} else := millicores if {
    # Assume cores if no unit
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
```

### Complete Postcondition Rego Policies

#### 1. desired_replicas_running
```rego
package postcondition.scale_deployment

import future.keywords.if

# Input schema:
# {
#   "running_pods": 5,
#   "ready_pods": 5,
#   "target_replicas": 5
# }

allow if {
    input.running_pods >= input.target_replicas
    input.ready_pods >= input.target_replicas
}
```

#### 2. deployment_health_check
```rego
package postcondition.scale_deployment

import future.keywords.if

# Input schema:
# {
#   "conditions": {
#     "Available": true,
#     "Progressing": true,
#     "ReplicaFailure": false
#   }
# }

allow if {
    input.conditions.Available == true
    input.conditions.Progressing == true
    input.conditions.ReplicaFailure == false
}
```

#### 3. no_crashloop_pods
```rego
package postcondition.scale_deployment

import future.keywords.if

# Input schema:
# {
#   "pods": [
#     {"name": "web-app-1", "status": "Running"},
#     {"name": "web-app-2", "status": "CrashLoopBackOff"},
#     ...
#   ]
# }

allow if {
    crashloop_pods := [p | p := input.pods[_]; p.status == "CrashLoopBackOff"]
    count(crashloop_pods) == 0
}
```

#### 4. resource_usage_acceptable
```rego
package postcondition.scale_deployment

import future.keywords.if

# Input schema:
# {
#   "pods": [
#     {"name": "web-app-1", "throttled": false, "oom_killed": false},
#     {"name": "web-app-2", "throttled": true, "oom_killed": false},
#     ...
#   ]
# }

allow if {
    throttled_pods := [p | p := input.pods[_]; p.throttled == true]
    oom_killed_pods := [p | p := input.pods[_]; p.oom_killed == true]

    count(throttled_pods) == 0
    count(oom_killed_pods) == 0
}
```

### Integration with Existing Validation
```
Validation Flow for scale_deployment:
1. WorkflowExecution: Parameter validation (existing)
2. WorkflowExecution: RBAC validation (existing)
3. WorkflowExecution: Step preconditions (NEW)
   - deployment_exists (blocking)
   - current_replicas_match (warning)
   - cluster_capacity_available (blocking)
4. Create KubernetesExecution (DEPRECATED - ADR-025) CRD
5. KubernetesExecutor (DEPRECATED - ADR-025): Parameter validation (existing)
6. KubernetesExecutor (DEPRECATED - ADR-025): RBAC validation (existing)
7. KubernetesExecutor (DEPRECATED - ADR-025): Resource existence check (existing)
8. KubernetesExecutor (DEPRECATED - ADR-025): Rego policy validation (existing)
9. KubernetesExecutor (DEPRECATED - ADR-025): Action preconditions (NEW)
   - image_pull_secrets_valid (blocking)
   - node_selector_matches (warning)
10. KubernetesExecutor (DEPRECATED - ADR-025): Dry-run execution (existing)
11. Create Kubernetes Job
12. Monitor Job execution
13. KubernetesExecutor (DEPRECATED - ADR-025): Action postconditions (NEW)
    - desired_replicas_running (blocking)
    - deployment_health_check (blocking)
    - resource_usage_acceptable (blocking)
14. Mark KubernetesExecution (DEPRECATED - ADR-025) complete
15. WorkflowExecution: Step postconditions (NEW)
    - Same as action postconditions (workflow-level verification)
16. Mark workflow complete
```

### Test Scenarios

#### Test 1: Precondition Success Path
```yaml
Test: scale_deployment with all preconditions passing
Expected:
  - deployment_exists: PASS
  - cluster_capacity_available: PASS
  - current_replicas_match: PASS (warning, 1 != 3 but allowed)
  - Execution proceeds
  - Job created successfully
```

#### Test 2: Precondition Blocking Failure
```yaml
Test: scale_deployment with insufficient cluster capacity
Expected:
  - deployment_exists: PASS
  - cluster_capacity_available: FAIL (blocking)
  - Execution blocked
  - KubernetesExecution (DEPRECATED - ADR-025) CRD NOT created
  - Step marked as "blocked" with error message
```

#### Test 3: Postcondition Success Path
```yaml
Test: scale_deployment with all postconditions passing
Expected:
  - Job completes successfully
  - desired_replicas_running: PASS (5 pods running)
  - deployment_health_check: PASS (Available=true)
  - no_crashloop_pods: PASS (0 crashloop)
  - resource_usage_acceptable: PASS (no throttling)
  - Step marked as "completed"
```

#### Test 4: Postcondition Verification Failure
```yaml
Test: scale_deployment with insufficient resources (only 2 of 5 pods start)
Expected:
  - Job completes successfully (kubectl scale succeeded)
  - desired_replicas_running: FAIL (only 2/5 pods running)
  - Step marked as "failed"
  - Rollback information captured
  - WorkflowExecution triggers rollback
```

---

## Condition Template Placeholder

**Note**: This implementation plan provides complete condition policies only for the `scale_deployment` action. **Condition templates for the remaining 26 actions will be defined during implementation** following the same pattern.

### Placeholder Structure for Future Conditions

Each action will have condition templates following this structure:
```yaml
action: [action_name]
preconditions:
  - type: [condition_type]
    description: "[human-readable description]"
    rego: |
      package precondition.[action_name]
      import future.keywords.if
      allow if { [validation logic] }
    required: [true/false]
    timeout: "[duration]"

postconditions:
  - type: [condition_type]
    description: "[human-readable description]"
    rego: |
      package postcondition.[action_name]
      import future.keywords.if
      allow if { [verification logic] }
    required: [true/false]
    timeout: "[duration]"
```

### Phased Rollout for Remaining Actions

**Phase 1 Actions** (Top 5, Weeks 1-2 of rollout):
- scale_deployment (COMPLETE - representative example)
- restart_pod
- increase_resources
- rollback_deployment
- expand_pvc

**Phase 2 Actions** (Next 10, Weeks 3-4 of rollout):
- Infrastructure: drain_node, cordon_node, uncordon_node, taint_node, untaint_node, quarantine_pod
- Storage: cleanup_storage, backup_data, compact_storage
- Application: update_hpa

**Phase 3 Actions** (Remaining 12, Weeks 5-6 of rollout):
- Security: rotate_secrets, audit_logs, update_network_policy
- Network: restart_network, reset_service_mesh
- Database: failover_database, repair_database
- Monitoring: enable_debug_mode, create_heap_dump, collect_diagnostics
- Resource: optimize_resources, migrate_workload, scale_statefulset, restart_daemonset
- Fallback: notify_only

---

## Testing Strategy

### Unit Tests
**Coverage Target**: 80%+
**Focus**:
- Rego policy evaluation logic
- Cluster state query utilities
- Async verification timeout handling
- ConditionResult creation and validation
- ConfigMap policy loading

**Example Test**:
```go
func TestPreconditionEvaluation(t *testing.T) {
    tests := []struct {
        name           string
        condition      StepCondition
        clusterState   map[string]interface{}
        expectedResult ConditionResult
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
            },
            expectedResult: ConditionResult{
                Evaluated: true,
                Passed:    true,
            },
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
            expectedResult: ConditionResult{
                Evaluated:    true,
                Passed:       false,
                ErrorMessage: "Deployment not found",
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            evaluator := NewRegoEvaluator()
            result := evaluator.EvaluateCondition(tt.condition, tt.clusterState)

            assert.Equal(t, tt.expectedResult.Evaluated, result.Evaluated)
            assert.Equal(t, tt.expectedResult.Passed, result.Passed)
            if tt.expectedResult.ErrorMessage != "" {
                assert.Contains(t, result.ErrorMessage, tt.expectedResult.ErrorMessage)
            }
        })
    }
}
```

### Integration Tests
**Coverage Target**: 60%+
**Focus**:
- Complete workflow with precondition/postcondition validation
- scale_deployment with all conditions
- Precondition blocking execution
- Postcondition verification triggering rollback
- ConfigMap policy hot-reload
- Async verification with real Kubernetes resources

**Example Test**:
```go
func TestScaleDeploymentWithConditions_Integration(t *testing.T) {
    // Setup real Kubernetes cluster (Kind)
    cluster := setupKindCluster(t)
    defer cluster.Cleanup()

    // Create test deployment
    deployment := createTestDeployment(cluster, "web-app", 3)

    // Create WorkflowExecution with scale_deployment step
    workflow := &workflowv1.WorkflowExecution{
        Spec: workflowv1.WorkflowExecutionSpec{
            WorkflowDefinition: workflowv1.WorkflowDefinition{
                Steps: []workflowv1.WorkflowStep{
                    {
                        StepNumber: 1,
                        Action:     "scale_deployment",
                        Parameters: &workflowv1.StepParameters{
                            Deployment: &workflowv1.DeploymentParams{
                                Name:      "web-app",
                                Namespace: "default",
                                Replicas:  5,
                            },
                        },
                        PreConditions: []workflowv1.StepCondition{
                            {
                                Type:        "deployment_exists",
                                Description: "Deployment must exist",
                                Rego:        loadRegoPolicy("deployment_exists.rego"),
                                Required:    true,
                                Timeout:     "10s",
                            },
                        },
                        PostConditions: []workflowv1.StepCondition{
                            {
                                Type:        "desired_replicas_running",
                                Description: "All replicas must be running",
                                Rego:        loadRegoPolicy("desired_replicas_running.rego"),
                                Required:    true,
                                Timeout:     "2m",
                            },
                        },
                    },
                },
            },
        },
    }

    // Create workflow
    err := cluster.Client.Create(context.TODO(), workflow)
    require.NoError(t, err)

    // Wait for workflow completion
    require.Eventually(t, func() bool {
        _ = cluster.Client.Get(context.TODO(), client.ObjectKeyFromObject(workflow), workflow)
        return workflow.Status.Phase == "completed"
    }, 5*time.Minute, 10*time.Second)

    // Verify precondition was evaluated and passed
    assert.Len(t, workflow.Status.StepStatuses, 1)
    stepStatus := workflow.Status.StepStatuses[0]
    assert.Len(t, stepStatus.PreConditionResults, 1)
    assert.True(t, stepStatus.PreConditionResults[0].Evaluated)
    assert.True(t, stepStatus.PreConditionResults[0].Passed)

    // Verify postcondition was evaluated and passed
    assert.Len(t, stepStatus.PostConditionResults, 1)
    assert.True(t, stepStatus.PostConditionResults[0].Evaluated)
    assert.True(t, stepStatus.PostConditionResults[0].Passed)

    // Verify deployment was actually scaled
    deployment = getDeployment(cluster, "web-app", "default")
    assert.Equal(t, int32(5), *deployment.Spec.Replicas)
}
```

### E2E Tests
**Coverage Target**: 40%+
**Focus**:
- Complete remediation workflow with validation framework
- Multi-step workflow with cascading conditions
- Failure recovery scenarios
- Real-world scale_deployment remediation

**Example Test**:
```go
func TestRemediation_ScaleDeployment_E2E(t *testing.T) {
    // Setup production-like environment
    cluster := setupProductionLikeCluster(t)
    defer cluster.Cleanup()

    // Deploy application with resource constraints
    app := deployAppWithResourceLimits(cluster, "web-app", 3, "500m", "1Gi")

    // Trigger high CPU alert
    triggerHighCPUAlert(cluster, "web-app")

    // Create remediation request (simulates alert handler)
    request := createRemediationRequest(cluster, "web-app-high-cpu")

    // Wait for RemediationOrchestrator to create WorkflowExecution
    workflow := waitForWorkflowCreation(t, cluster, request)

    // Verify workflow has scale_deployment step with conditions
    assert.Len(t, workflow.Spec.WorkflowDefinition.Steps, 1)
    step := workflow.Spec.WorkflowDefinition.Steps[0]
    assert.Equal(t, "scale_deployment", step.Action)
    assert.NotEmpty(t, step.PreConditions)
    assert.NotEmpty(t, step.PostConditions)

    // Wait for workflow completion
    require.Eventually(t, func() bool {
        _ = cluster.Client.Get(context.TODO(), client.ObjectKeyFromObject(workflow), workflow)
        return workflow.Status.Phase == "completed"
    }, 10*time.Minute, 15*time.Second)

    // Verify remediation effectiveness
    assert.Equal(t, "success", workflow.Status.WorkflowResult.Outcome)
    assert.GreaterOrEqual(t, workflow.Status.WorkflowResult.EffectivenessScore, 0.85)

    // Verify application is healthy
    assert.Equal(t, "healthy", workflow.Status.WorkflowResult.ResourceHealth)
    assert.False(t, workflow.Status.WorkflowResult.NewAlertsTriggered)

    // Verify deployment scaled successfully
    deployment := getDeployment(cluster, "web-app", "default")
    assert.Equal(t, int32(5), *deployment.Spec.Replicas)
    assert.True(t, isDeploymentHealthy(deployment))
}
```

---

## Rollout Plan

### Feature Flag
**Flag Name**: `enable_validation_framework`
**Default**: `false` (disabled)
**Scope**: Per-cluster configuration

```yaml
# config/development.yaml
features:
  enable_validation_framework: true
```

### Phased Rollout Strategy

#### Stage 1: Canary (Week 6)
**Duration**: 3-5 days
**Target**: 5% of workflows in non-production clusters
**Success Criteria**:
- False positive rate <10%
- No performance degradation >10%
- Effectiveness improvement >10%

#### Stage 2: Staging (Week 7)
**Duration**: 3-5 days
**Target**: 100% of workflows in staging clusters
**Success Criteria**:
- False positive rate <15%
- Performance impact <5 seconds per step
- Effectiveness improvement >15%

#### Stage 3: Production Gradual (Weeks 8-10)
**Duration**: 2-3 weeks
**Target**:
- Week 8: 10% of production workflows
- Week 9: 50% of production workflows
- Week 10: 100% of production workflows
**Success Criteria**:
- False positive rate <15%
- Cascade failure reduction >15%
- MTTR reduction >40%
- Manual intervention reduction >15%

### Monitoring and Observability

#### Key Metrics
```yaml
Condition Evaluation Metrics:
- condition_evaluations_total (counter)
  labels: [condition_type, service, action, required]

- condition_evaluation_duration_seconds (histogram)
  labels: [condition_type, service]

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
```

#### Alerting Rules
```yaml
# False positive alert
- alert: HighValidationFalsePositiveRate
  expr: |
    (sum(rate(condition_failed_total{required="true"}[1h])) /
     sum(rate(condition_evaluations_total{required="true"}[1h]))) > 0.15
  for: 30m
  annotations:
    summary: "Validation false positive rate exceeds 15%"

# Performance impact alert
- alert: ValidationPerformanceImpact
  expr: |
    histogram_quantile(0.95,
      rate(condition_evaluation_duration_seconds_bucket[5m])
    ) > 10
  for: 15m
  annotations:
    summary: "Condition evaluation taking >10 seconds at p95"
```

#### Dashboard
**Grafana Dashboard**: Validation Framework Overview
- Condition evaluation rates (success/failure)
- Performance impact (evaluation duration)
- False positive trends
- Effectiveness improvement metrics
- Action-specific condition success rates

### Rollback Procedures

#### Rollback Trigger Conditions
- False positive rate >20% for 1 hour
- Performance impact >15 seconds per step for 30 minutes
- Cascade of validation failures causing service disruption
- Critical bug in condition evaluation logic

#### Rollback Steps
1. **Immediate**: Disable feature flag (`enable_validation_framework: false`)
2. **Verify**: Confirm workflows execute without validation
3. **Investigate**: Review logs, metrics, and failure scenarios
4. **Fix**: Address root cause (condition policy, evaluation logic, timeout)
5. **Test**: Validate fix in staging environment
6. **Re-enable**: Resume phased rollout from Stage 1

---

## Risk Mitigation

### Risk 1: False Positives (5-15% estimated)
**Mitigation**:
- Start all new conditions with `required: false` (warning only)
- Monitor false positive rate per condition type
- Gradually tighten conditions based on telemetry
- Provide operator override mechanism via annotations
- Implement condition tuning dashboard

**Operator Override Example**:
```yaml
metadata:
  annotations:
    kubernaut.io/override-condition: "cluster_capacity_available"
```

### Risk 2: Performance Impact (2-5 seconds per step)
**Mitigation**:
- Implement async verification with configurable timeouts
- Cache cluster state queries for 5-10 seconds
- Parallelize condition evaluations where possible
- Profile and optimize Rego policy execution
- Set aggressive timeouts for non-critical conditions

### Risk 3: Maintenance Burden (100+ condition policies)
**Mitigation**:
- Create reusable condition libraries (e.g., common deployment checks)
- Automated testing for all condition policies
- Policy versioning with backwards compatibility
- Clear ownership and review process for condition updates
- Condition template generator tool

**Reusable Condition Library Example**:
```rego
# lib/deployment_common.rego
package lib.deployment

import future.keywords.if

deployment_exists(input) if {
    input.deployment_found == true
}

deployment_healthy(input) if {
    input.conditions.Available == true
    input.conditions.Progressing == true
}

replicas_match(current, desired) if {
    current == desired
}
```

### Risk 4: Operator Learning Curve
**Mitigation**:
- Comprehensive operator documentation
- Example condition templates for common scenarios
- Condition validation tool (syntax checker)
- Condition testing framework
- Office hours and support channels

---

## Success Metrics

### Quantitative Metrics
| Metric | Baseline | Target | Measurement Period |
|--------|----------|--------|-------------------|
| **Remediation Effectiveness** | 70% | 85-90% | 3 months |
| **Cascade Failure Rate** | 30% | <10% | 3 months |
| **MTTR (Failed Remediation)** | 15 min | <8 min | 3 months |
| **Manual Intervention** | 40% | 20% | 3 months |
| **False Positive Rate** | 0% | <15% | Ongoing |
| **Condition Adoption** | 0% | 80% | 6 months |

### Qualitative Metrics
- **Operator Satisfaction**: Survey score >4.0/5.0 for condition framework usability
- **Observability Improvement**: Reduced time to identify failure root cause
- **Confidence in Automation**: Increased willingness to enable auto-remediation

### ROI Calculation
**Investment**:
- Development: 33 days × $800/day = $26,400
- Testing: 10 days × $800/day = $8,000
- **Total**: $34,400

**Return** (monthly):
- Time saved: 10 hours/month × $100/hr = $1,000/month
- **Annual Return**: $12,000/year
- **Payback Period**: 2.9 months

**Additional Value**:
- Reduced production incidents: $5,000-10,000/month
- Improved SLA compliance: $2,000-5,000/month
- **Total Annual Value**: $84,000-180,000

---

## References

- **Design Decision**: [DD-002 - Per-Step Validation Framework](../../architecture/DESIGN_DECISIONS.md#dd-002-per-step-validation-framework-alternative-2)
- **Business Requirements**: [STEP_VALIDATION_BUSINESS_REQUIREMENTS.md](../../requirements/STEP_VALIDATION_BUSINESS_REQUIREMENTS.md)
- **CRD Schema Documentation**:
  - [WorkflowExecution CRD Schema](03-workflowexecution/crd-schema.md)
  - [KubernetesExecution (DEPRECATED - ADR-025) CRD Schema](04-kubernetesexecutor/crd-schema.md)
- **Reconciliation Flow Documentation**:
  - [WorkflowExecution Reconciliation Phases](03-workflowexecution/reconciliation-phases.md)
  - [KubernetesExecutor (DEPRECATED - ADR-025) Reconciliation Phases](04-kubernetesexecutor/reconciliation-phases.md)
- **Rego Policy Integration**: [REGO_POLICY_INTEGRATION.md](04-kubernetesexecutor/implementation/REGO_POLICY_INTEGRATION.md)
- **APDC Methodology**: [.cursor/rules/00-core-development-methodology.mdc](../../../.cursor/rules/00-core-development-methodology.mdc)

---

## Appendix A: Condition Type Taxonomy

### Common Condition Types

#### Resource State Conditions
- `resource_exists`: Verify resource exists before action
- `resource_healthy`: Check resource is in healthy state
- `resource_ready`: Verify resource is ready for action

#### Capacity Conditions
- `cluster_capacity_available`: Sufficient cluster resources
- `node_capacity_available`: Sufficient node resources
- `storage_capacity_available`: Sufficient storage space

#### Deployment-Specific Conditions
- `deployment_exists`: Deployment must exist
- `current_replicas_match`: Current replicas match baseline
- `desired_replicas_running`: Target replicas are running
- `deployment_available`: Deployment is Available
- `deployment_progressing`: Deployment is Progressing

#### Pod-Specific Conditions
- `pod_count_valid`: Pod count meets expectations
- `no_crashloop_pods`: No pods in CrashLoopBackOff
- `no_pending_pods`: No pods stuck in Pending
- `resource_usage_acceptable`: Pods not throttled or OOMKilled

#### Security Conditions
- `rbac_permissions_valid`: RBAC permissions are correct
- `image_pull_secrets_valid`: Image pull secrets are valid
- `security_context_valid`: Security context meets requirements

#### Network Conditions
- `network_reachable`: Target is network reachable
- `dns_resolvable`: DNS name resolves correctly
- `service_endpoints_available`: Service has endpoints

### Condition Naming Conventions
- Use lowercase with underscores
- Start with subject (e.g., `deployment_`, `pod_`)
- End with state or check (e.g., `_exists`, `_valid`, `_available`)
- Be specific and descriptive

---

## Appendix B: Rego Policy Best Practices

### Policy Structure
```rego
# Package naming: precondition.<action> or postcondition.<action>
package precondition.scale_deployment

# Always import future.keywords for cleaner syntax
import future.keywords.if

# Document expected input schema in comments
# Input schema:
# {
#   "deployment_found": true/false,
#   "deployment_name": "web-app"
# }

# Main decision: 'allow' if condition passes
allow if {
    input.deployment_found == true
}

# Optional: Provide detailed error message
error_message := msg if {
    not input.deployment_found
    msg := sprintf("Deployment %s not found in namespace %s",
                   [input.deployment_name, input.namespace])
}
```

### Testing Policies
```go
// Test Rego policies with Go unit tests
func TestDeploymentExistsPolicy(t *testing.T) {
    policy := `
        package precondition
        import future.keywords.if
        allow if { input.deployment_found == true }
    `

    tests := []struct {
        name     string
        input    map[string]interface{}
        expected bool
    }{
        {
            name:     "deployment exists",
            input:    map[string]interface{}{"deployment_found": true},
            expected: true,
        },
        {
            name:     "deployment not found",
            input:    map[string]interface{}{"deployment_found": false},
            expected: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := evaluateRegoPolicy(policy, tt.input)
            assert.Equal(t, tt.expected, result.Allowed)
        })
    }
}
```

---

## Appendix C: Troubleshooting Guide

### Common Issues

#### Issue 1: Condition Always Fails
**Symptoms**: Condition consistently fails even when visually verified as correct
**Causes**:
- Input schema mismatch between expected and actual
- Rego policy syntax error
- Timeout too short for async checks
**Resolution**:
1. Enable debug logging for condition evaluation
2. Inspect actual input sent to Rego policy
3. Validate Rego policy syntax with `opa test`
4. Increase timeout for async conditions

#### Issue 2: Async Verification Timeout
**Symptoms**: Postconditions fail with "verification timeout" message
**Causes**:
- Resources taking longer than expected to converge
- Cluster under load
- Timeout set too aggressively
**Resolution**:
1. Increase `condition.timeout` value
2. Check cluster resource availability
3. Review resource startup/health check times
4. Consider making condition optional (`required: false`)

#### Issue 3: False Positive Rate Too High
**Symptoms**: Conditions block valid executions frequently
**Causes**:
- Condition too strict for production variations
- Input data quality issues
- Timing issues (resources not yet updated)
**Resolution**:
1. Change condition to `required: false` temporarily
2. Analyze failure patterns in metrics
3. Adjust Rego policy to be more lenient
4. Add retry logic for transient failures

### Debug Commands

```bash
# View condition evaluation logs
kubectl logs -n kubernaut-system deployment/workflow-execution-controller \
  | grep "condition_evaluation"

# Check condition results in CRD status
kubectl get workflowexecution <name> -o jsonpath='{.status.stepStatuses[0].preConditionResults}'

# Test Rego policy locally
echo '{"deployment_found": true}' | opa eval --data policy.rego --input - 'data.precondition.allow'

# View condition evaluation metrics
curl http://prometheus:9090/api/v1/query?query=condition_evaluations_total
```

---

## Conclusion

This implementation plan provides a comprehensive roadmap for adding per-step precondition/postcondition validation to the remediation framework. The phased approach with a representative `scale_deployment` example balances immediate value delivery with long-term extensibility. By leveraging existing Rego policy infrastructure and following defense-in-depth validation principles, this framework will significantly improve remediation effectiveness while maintaining acceptable performance and false positive rates.

**Next Steps**:
1. Obtain stakeholder approval for plan
2. Allocate development resources (5-6 weeks)
3. Begin Phase 1: CRD Schema Extensions
4. Execute phased implementation according to timeline
5. Monitor success metrics and adjust as needed

